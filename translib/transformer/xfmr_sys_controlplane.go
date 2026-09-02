////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                     //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//  http://www.apache.org/licenses/LICENSE-2.0                                //
//                                                                            //
//  Unless required by applicable law or agreed to in writing, software       //
//  distributed under the License is distributed on an "AS IS" BASIS,         //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  //
//  See the License for the specific language governing permissions or        //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

package transformer

import (
	"fmt"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	cpAclTable = "ACL_TABLE"

	// CONTROL_PLANE_ACL records which ports this binding contributed to
	// ACL_TABLE, which is what makes a withdrawal able to leave the ports
	// another binding added. See sonic-control-plane-acl.yang.
	cpAclBindTable = "CONTROL_PLANE_ACL"

	// A control-plane ACL has no CPU bind point in SAI as consumed by this
	// build, so it is realised as an ingress ACL on the ports named by the
	// sonic-system-controlplane-ext bind-interfaces leaf-list.
	cpAclStage = "INGRESS"

	cpAclTypeField  = "type"
	cpAclStageField = "stage"
	cpAclPortsField = "ports"

	cpAclBindLeaf = "bind-interfaces"

	// A CTRLPLANE table is the openconfig-acl-ext 'control-plane' ACL, which
	// caclmgrd renders into iptables on the CPU. It shares ACL_TABLE with the
	// data-plane types but has a different backend, so an ACL set cannot be
	// both that and a hardware-programmed control-plane ACL.
	cpAclTypeCtrlPlane = "CTRLPLANE"
)

// cpAclOcToSonicType maps an OpenConfig ACL type to the CONFIG_DB ACL_TABLE
// type. The values are those of the SONIC_ACL_TYPE_* constants in acl_app.go.
var cpAclOcToSonicType = map[string]string{
	"ACL_IPV4": "L3",
	"ACL_IPV6": "L3V6",
	"ACL_L2":   "L2",
}

var cpAclSonicToOcType = map[string]string{
	"L3":   "ACL_IPV4",
	"L3V6": "ACL_IPV6",
	"L2":   "ACL_L2",
}

func init() {
	XlateFuncBind("YangToDb_cp_acl_xfmr", YangToDb_cp_acl_xfmr)
	XlateFuncBind("DbToYang_cp_acl_xfmr", DbToYang_cp_acl_xfmr)
	XlateFuncBind("Subscribe_cp_acl_xfmr", Subscribe_cp_acl_xfmr)
}

// A binding shows up in both tables: CONTROL_PLANE_ACL says which ports it
// claims and ACL_TABLE says which of them are programmed, and what the tree
// reports depends on both.
var Subscribe_cp_acl_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	aclKey, _, _, err := cpAclKeyFromUri(inParams.uri)
	if err != nil {
		return result, err
	}
	if aclKey == "" {
		aclKey = "*"
	}

	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		cpAclBindTable: {aclKey: {}},
		cpAclTable:     {aclKey: {}},
	}
	return result, nil
}

// cpAclOcTypeName returns the bare OpenConfig ACL type name, dropping any
// module prefix. The type reaches the transformer as a URI key ("ACL_IPV4" or
// "openconfig-acl:ACL_IPV4") or as a ygot enumeration.
func cpAclOcTypeName(param interface{}) (string, error) {
	var name string

	switch v := param.(type) {
	case string:
		name = v
	case ygot.GoEnum:
		n, err := ygot.EnumName(v)
		if err != nil {
			return "", err
		}
		name = n
	case fmt.Stringer:
		name = v.String()
	default:
		name = fmt.Sprint(v)
	}

	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}
	return name, nil
}

// cpAclKeyFromOc composes the ACL_TABLE key exactly as acl_app.go does in
// getAclKeyStrFromOCKey. Both producers have to agree on the key or a binding
// lands on a different row than the rules it is meant to activate.
func cpAclKeyFromOc(setName string, ocType string) string {
	name := strings.Replace(strings.Replace(setName, " ", "_", -1), "-", "_", -1)
	return name + "_" + ocType
}

// cpAclSplitKey recovers the set name and OpenConfig type from an ACL_TABLE
// key. Recovery is lossy in the same way acl_app.go is lossy: a set name that
// contained a space or a hyphen comes back with an underscore, because the key
// keeps no record of the original separator.
func cpAclSplitKey(key string) (string, string, bool) {
	for ocType := range cpAclOcToSonicType {
		suffix := "_" + ocType
		if strings.HasSuffix(key, suffix) {
			return strings.TrimSuffix(key, suffix), ocType, true
		}
	}
	return "", "", false
}

// cpAclConfigDb returns a handle on the config db. The write path is given one
// directly, the read path only the per-database set. Either can be absent, so
// callers have to cope with a nil handle.
func cpAclConfigDb(inParams XfmrParams) *db.DB {
	if inParams.d != nil {
		return inParams.d
	}
	return inParams.dbs[db.ConfigDB]
}

func cpAclGetEntry(d *db.DB, table string, key string) (db.Value, bool) {
	if d == nil {
		return db.Value{}, false
	}
	entry, err := d.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{key}})
	if err != nil || len(entry.Field) == 0 {
		return db.Value{}, false
	}
	return entry, true
}

// cpAclEffectiveBind returns the ports this binding contributed that ACL_TABLE
// still holds on ingress.
//
// It is the intersection rather than what was recorded because the two can
// drift: deleting an ACL set through /openconfig-acl:acl removes the ACL_TABLE
// entry without clearing CONTROL_PLANE_ACL, so a record can outlive the thing
// it described. Reporting the intersection makes such a leftover inert instead
// of advertising a binding that is not programmed, and it self-heals if the
// same ACL set is created again.
func cpAclEffectiveBind(d *db.DB, aclKey string) []string {
	bindEntry, ok := cpAclGetEntry(d, cpAclBindTable, aclKey)
	if !ok {
		return nil
	}
	recorded := bindEntry.GetList(cpAclPortsField)
	if len(recorded) == 0 {
		return nil
	}

	aclEntry, ok := cpAclGetEntry(d, cpAclTable, aclKey)
	if !ok {
		return nil
	}
	if aclEntry.Get(cpAclStageField) != cpAclStage {
		return nil
	}
	if _, ok := cpAclSonicToOcType[aclEntry.Get(cpAclTypeField)]; !ok {
		return nil
	}

	live := make(map[string]bool)
	for _, p := range aclEntry.GetList(cpAclPortsField) {
		live[p] = true
	}

	var effective []string
	for _, p := range recorded {
		if live[p] {
			effective = append(effective, p)
		}
	}
	return effective
}

// cpAclRequestedBind pulls the bind-interfaces of the acl-set being written out
// of the request. The leaf-list is the payload of the binding, so a write
// carrying none is a request to bind nothing.
func cpAclRequestedBind(inParams XfmrParams, setName string, ocType string) ([]string, bool) {
	if inParams.ygRoot == nil || *inParams.ygRoot == nil {
		return nil, false
	}
	deviceObj, ok := (*inParams.ygRoot).(*ocbinds.Device)
	if !ok || deviceObj.System == nil || deviceObj.System.ControlPlaneTraffic == nil {
		return nil, false
	}
	ingress := deviceObj.System.ControlPlaneTraffic.Ingress
	if ingress == nil || ingress.Acl == nil || ingress.Acl.AclSet == nil {
		return nil, false
	}

	for key, aclSet := range ingress.Acl.AclSet {
		keyType, err := cpAclOcTypeName(key.Type)
		if err != nil {
			continue
		}
		if cpAclKeyFromOc(key.SetName, keyType) != cpAclKeyFromOc(setName, ocType) {
			continue
		}
		if aclSet == nil || aclSet.Config == nil {
			return nil, true
		}
		return aclSet.Config.BindInterfaces, true
	}
	return nil, false
}

func cpAclUnion(base []string, add []string) []string {
	seen := make(map[string]bool, len(base))
	out := make([]string, 0, len(base)+len(add))
	for _, v := range base {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range add {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func cpAclIntersect(base []string, keep []string) []string {
	want := make(map[string]bool, len(keep))
	for _, v := range keep {
		want[v] = true
	}
	out := make([]string, 0, len(base))
	for _, v := range base {
		if want[v] {
			out = append(out, v)
		}
	}
	return out
}

func cpAclSubtract(base []string, remove []string) []string {
	drop := make(map[string]bool, len(remove))
	for _, v := range remove {
		drop[v] = true
	}
	out := make([]string, 0, len(base))
	for _, v := range base {
		if !drop[v] {
			out = append(out, v)
		}
	}
	return out
}

// cpAclStageDelete adds one entry to the request's DELETE sub-operation, which
// is how a write withdraws a port it no longer names.
func cpAclStageDelete(inParams XfmrParams, table string, key string, val db.Value) {
	if inParams.subOpDataMap == nil {
		return
	}
	if inParams.subOpDataMap[DELETE] == nil {
		subOpMap := make(map[db.DBNum]map[string]map[string]db.Value)
		inParams.subOpDataMap[DELETE] = &subOpMap
	}
	if (*inParams.subOpDataMap[DELETE])[db.ConfigDB] == nil {
		(*inParams.subOpDataMap[DELETE])[db.ConfigDB] = make(map[string]map[string]db.Value)
	}
	delMap := (*inParams.subOpDataMap[DELETE])[db.ConfigDB]
	if delMap[table] == nil {
		delMap[table] = make(map[string]db.Value)
	}
	delMap[table][key] = val
}

// cpAclEmitWrite records something to be written, which only a request that
// writes ever does: a delete withdraws by naming what to remove instead.
func cpAclEmitWrite(inParams XfmrParams, res_map map[string]map[string]db.Value, table string, key string, val db.Value) {
	if res_map[table] == nil {
		res_map[table] = make(map[string]db.Value)
	}
	res_map[table][key] = val
}

// cpAclEmitRemove records something to be removed. res_map carries entries of
// the operation the transformer was called for, so on a delete request a
// removal belongs there, and on a write it has to ride on a sub-operation.
func cpAclEmitRemove(inParams XfmrParams, res_map map[string]map[string]db.Value, table string, key string, val db.Value) {
	if inParams.oper == DELETE {
		if res_map[table] == nil {
			res_map[table] = make(map[string]db.Value)
		}
		res_map[table][key] = val
		return
	}
	cpAclStageDelete(inParams, table, key, val)
}

func cpAclNewValue() db.Value {
	return db.Value{Field: make(map[string]string)}
}

// cpAclKeyFromUri resolves the acl-set instance a request addresses.
func cpAclKeyFromUri(uri string) (string, string, string, error) {
	pathInfo := NewPathInfo(uri)
	setName := pathInfo.Var("set-name")
	rawType := pathInfo.Var("type")
	if setName == "" || rawType == "" {
		return "", "", "", nil
	}

	ocType, err := cpAclOcTypeName(rawType)
	if err != nil {
		return "", "", "", err
	}
	if _, ok := cpAclOcToSonicType[ocType]; !ok {
		return "", "", "", tlerr.NotSupported("Unsupported ACL type %s for a control-plane ACL", ocType)
	}
	return cpAclKeyFromOc(setName, ocType), setName, ocType, nil
}

// cpAclBindValueFromUri returns the single bind-interfaces value a request
// addresses, for a delete of one entry of the leaf-list. An empty result means
// the request addresses more than one entry, so the whole binding goes.
func cpAclBindValueFromUri(uri string) string {
	if !strings.Contains(uri, cpAclBindLeaf) {
		return ""
	}
	// Deciding what a leaf-list instance URI addresses is left to translib, so
	// that this agrees with the form translib itself produces.
	value, err := extractLeafListInstFromUri(uri)
	if err != nil {
		return ""
	}
	return value
}

// Binding an ACL set to the control plane adds the stage and the ports that
// make aclorch program the set's existing rules into the ASIC. The set itself,
// and its rules, belong to /openconfig-acl:acl, so a binding never creates or
// destroys them.
//
// Both the stage and the ports are decided here rather than through per-leaf
// mappings because withdrawing a binding has to weigh them together: the stage
// may only go once no port this binding contributed is left.
var YangToDb_cp_acl_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res_map := make(map[string]map[string]db.Value)

	aclKey, setName, ocType, err := cpAclKeyFromUri(inParams.uri)
	if err != nil {
		return res_map, err
	}
	if aclKey == "" {
		// A request on the whole list carries no instance to act on.
		return res_map, nil
	}

	d := cpAclConfigDb(inParams)
	sonicType, ok := cpAclOcToSonicType[ocType]
	if !ok {
		return res_map, tlerr.NotSupported("Unsupported ACL type %s for a control-plane ACL", ocType)
	}

	aclEntry, aclExists := cpAclGetEntry(d, cpAclTable, aclKey)
	existingType := aclEntry.Get(cpAclTypeField)
	existingStage := aclEntry.Get(cpAclStageField)
	existingPorts := aclEntry.GetList(cpAclPortsField)

	recorded := []string{}
	if bindEntry, ok := cpAclGetEntry(d, cpAclBindTable, aclKey); ok {
		recorded = bindEntry.GetList(cpAclPortsField)
	}

	// Every operation comes down to the same three answers: which ports this
	// binding claims afterwards, which of them to add to ACL_TABLE and which
	// to take out of it. Deciding them the same way for all operations is what
	// keeps a withdrawal from touching another binding. Only a write adds, so
	// a delete can never put a port back.
	var newRecorded, withdraw, add []string

	if inParams.oper == DELETE {
		withdraw = cpAclEffectiveBind(d, aclKey)
		if single := cpAclBindValueFromUri(inParams.uri); single != "" {
			// A delete of one entry of the leaf-list withdraws just that port.
			withdraw = cpAclIntersect(withdraw, []string{single})
			newRecorded = cpAclSubtract(recorded, []string{single})
		}
		// Deleting the instance ends the binding, so nothing stays recorded,
		// including a port that was recorded but never programmed because the
		// ACL set had meanwhile been deleted through /openconfig-acl:acl.
	} else {
		requested, present := cpAclRequestedBind(inParams, setName, ocType)
		if !present {
			log.Infof("YangToDb_cp_acl_xfmr: no acl-set payload for %s", aclKey)
			return res_map, nil
		}

		if existingType == cpAclTypeCtrlPlane {
			return res_map, tlerr.NotSupported(
				"ACL set %s is a control-plane ACL enforced by iptables and cannot also be programmed into hardware; use a separate ACL set", aclKey)
		}

		// aclorch takes an ACL table to be ingress or egress, never both, so
		// an egress binding made through /openconfig-acl:acl/interfaces
		// cannot be silently turned into an ingress one.
		if existingStage != "" && existingStage != cpAclStage {
			return res_map, tlerr.NotSupported(
				"ACL set %s is already bound in the %s direction and cannot also filter control-plane traffic on ingress; use a separate ACL set", aclKey, existingStage)
		}

		for _, intf := range requested {
			if intf == "" {
				return res_map, tlerr.InvalidArgs("An empty interface name cannot be bound to the control plane")
			}
		}

		// An ACL set has a single ports leaf-list covering all of its
		// bindings, so there is nowhere to record that an interface was bound
		// both here and through /openconfig-acl:acl/interfaces. Taking a share
		// of one silently would leave a withdrawal here no way to tell whether
		// the interface is still wanted, and it would be unbound from hardware
		// while the other binding still expects it. Refusing says so at the
		// point the ambiguity would be created.
		if shared := cpAclIntersect(requested, cpAclSubtract(existingPorts, recorded)); len(shared) > 0 {
			return res_map, tlerr.NotSupported(
				"Interface %s is already bound to ACL set %s through /openconfig-acl:acl/interfaces, and an ACL set cannot be bound to the same interface twice; remove that binding or use a separate ACL set", shared[0], aclKey)
		}

		add = requested
		if inParams.oper == REPLACE {
			// A replace states the whole binding, so ports it no longer names
			// stop being bound.
			newRecorded = requested
			withdraw = cpAclSubtract(recorded, requested)
		} else {
			newRecorded = cpAclUnion(recorded, requested)
		}
	}

	if inParams.oper == DELETE && len(withdraw) == 0 && len(newRecorded) == len(recorded) {
		// Nothing of this binding is programmed, so there is nothing to take
		// out of ACL_TABLE and no record to correct.
		log.Infof("YangToDb_cp_acl_xfmr: nothing bound to withdraw for %s", aclKey)
		return res_map, nil
	}

	remaining := cpAclSubtract(existingPorts, withdraw)

	if len(newRecorded) == 0 {
		// An empty value removes the whole entry.
		cpAclEmitRemove(inParams, res_map, cpAclBindTable, aclKey, cpAclNewValue())
	} else if inParams.oper == DELETE {
		// The record shrinks the same way ACL_TABLE does below, by naming the
		// entries to take out of the leaf-list.
		if dropped := cpAclSubtract(recorded, newRecorded); len(dropped) > 0 {
			val := cpAclNewValue()
			val.SetList(cpAclPortsField, dropped)
			cpAclEmitRemove(inParams, res_map, cpAclBindTable, aclKey, val)
		}
	} else {
		val := cpAclNewValue()
		val.SetList(cpAclPortsField, newRecorded)
		cpAclEmitWrite(inParams, res_map, cpAclBindTable, aclKey, val)
	}

	if inParams.oper == DELETE {
		if !aclExists {
			return res_map, nil
		}
		// A withdrawal names the ports to take out rather than the ports to
		// keep, because translib removes the named entries of a leaf-list and
		// leaves the rest untouched. Stating the remainder instead would be
		// merged into what is already there, which can only ever grow the
		// leaf-list, so the ports of another binding would be kept and the
		// withdrawn one would come straight back.
		aclVal := cpAclNewValue()
		if len(withdraw) > 0 {
			aclVal.SetList(cpAclPortsField, withdraw)
		}
		if len(remaining) == 0 {
			// Nothing is bound any more, so the stage goes with the ports and
			// the ACL set is left as an unbound set of rules. The type stays:
			// it belongs to the ACL set, not to the binding.
			aclVal.Field[cpAclStageField] = ""
		}
		if len(aclVal.Field) > 0 {
			cpAclEmitRemove(inParams, res_map, cpAclTable, aclKey, aclVal)
		}
		log.Infof("YangToDb_cp_acl_xfmr: %s oper %v recorded %v withdrawn %v remaining %v",
			aclKey, inParams.oper, newRecorded, withdraw, remaining)
		return res_map, nil
	}

	newPorts := cpAclUnion(remaining, add)

	if len(newPorts) == 0 {
		if !aclExists {
			return res_map, nil
		}
		aclVal := cpAclNewValue()
		aclVal.Field[cpAclPortsField+"@"] = ""
		aclVal.Field[cpAclStageField] = ""
		cpAclEmitRemove(inParams, res_map, cpAclTable, aclKey, aclVal)
		return res_map, nil
	}

	aclVal := cpAclNewValue()
	// Updating a list instance reaches the transformer as a replace, which
	// writes the entry rather than merging into it. The binding owns only the
	// stage and its own ports, so the entry's other fields, the type and the
	// description that belong to the ACL set, have to be carried across or
	// they would be dropped and leave an entry aclorch cannot act on.
	for field, value := range aclEntry.Field {
		aclVal.Field[field] = value
	}
	// The type is written every time rather than only when absent, both to
	// restore it if an earlier write dropped it and because the key already
	// fixes what it has to be.
	aclVal.Field[cpAclTypeField] = sonicType
	aclVal.Field[cpAclStageField] = cpAclStage
	aclVal.SetList(cpAclPortsField, newPorts)
	cpAclEmitWrite(inParams, res_map, cpAclTable, aclKey, aclVal)

	log.Infof("YangToDb_cp_acl_xfmr: %s oper %v recorded %v ports %v", aclKey, inParams.oper, newRecorded, newPorts)
	return res_map, nil
}

// The control-plane tree reports the bindings this transformer made, not every
// ACL set that exists. An ACL set that is merely configured under
// /openconfig-acl:acl is not bound to the control plane, and neither is one
// bound only through /openconfig-acl:acl/interfaces.
var DbToYang_cp_acl_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	d := cpAclConfigDb(inParams)
	if d == nil {
		return nil
	}

	deviceObj, ok := (*inParams.ygRoot).(*ocbinds.Device)
	if !ok {
		return nil
	}
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.ControlPlaneTraffic == nil {
		deviceObj.System.ControlPlaneTraffic = &ocbinds.OpenconfigSystem_System_ControlPlaneTraffic{}
	}
	if deviceObj.System.ControlPlaneTraffic.Ingress == nil {
		deviceObj.System.ControlPlaneTraffic.Ingress = &ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress{}
	}
	ingress := deviceObj.System.ControlPlaneTraffic.Ingress
	if ingress.Acl == nil {
		ingress.Acl = &ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress_Acl{}
	}
	if ingress.Acl.AclSet == nil {
		ingress.Acl.AclSet = make(map[ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress_Acl_AclSet_Key]*ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress_Acl_AclSet)
	}

	targetKey, _, _, err := cpAclKeyFromUri(inParams.uri)
	if err != nil {
		return err
	}

	var keys []string
	if targetKey != "" {
		keys = []string{targetKey}
	} else {
		dbKeys, err := d.GetKeys(&db.TableSpec{Name: cpAclBindTable})
		if err != nil {
			log.Infof("DbToYang_cp_acl_xfmr: cannot list %s: %v", cpAclBindTable, err)
			return nil
		}
		for _, k := range dbKeys {
			if len(k.Comp) > 0 {
				keys = append(keys, k.Comp[0])
			}
		}
	}

	for _, aclKey := range keys {
		setName, ocType, ok := cpAclSplitKey(aclKey)
		if !ok {
			log.Infof("DbToYang_cp_acl_xfmr: %s is not an OpenConfig ACL key", aclKey)
			continue
		}

		bound := cpAclEffectiveBind(d, aclKey)
		if len(bound) == 0 {
			continue
		}

		typeVal, err := cpAclOcTypeEnum(ocType)
		if err != nil {
			log.Warningf("DbToYang_cp_acl_xfmr: %v", err)
			continue
		}

		key := ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress_Acl_AclSet_Key{
			SetName: setName,
			Type:    typeVal,
		}
		ingress.Acl.AclSet[key] = &ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress_Acl_AclSet{
			SetName: ygot.String(setName),
			Type:    typeVal,
			Config: &ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress_Acl_AclSet_Config{
				SetName:        ygot.String(setName),
				Type:           typeVal,
				BindInterfaces: bound,
			},
			State: &ocbinds.OpenconfigSystem_System_ControlPlaneTraffic_Ingress_Acl_AclSet_State{
				SetName:        ygot.String(setName),
				Type:           typeVal,
				BindInterfaces: bound,
			},
		}
	}

	return nil
}

// cpAclOcTypeEnum resolves an OpenConfig ACL type name to its ygot enumeration.
func cpAclOcTypeEnum(ocType string) (ocbinds.E_OpenconfigAcl_ACL_TYPE, error) {
	switch ocType {
	case "ACL_IPV4":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4, nil
	case "ACL_IPV6":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV6, nil
	case "ACL_L2":
		return ocbinds.OpenconfigAcl_ACL_TYPE_ACL_L2, nil
	}
	return ocbinds.OpenconfigAcl_ACL_TYPE_UNSET,
		fmt.Errorf("unsupported ACL type %s for a control-plane ACL", ocType)
}
