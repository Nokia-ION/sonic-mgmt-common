////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");          //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//     http://www.apache.org/licenses/LICENSE-2.0                             //
//                                                                            //
//  Unless required by applicable law or agreed to in writing, software       //
//  distributed under the License is distributed on an "AS IS" BASIS,         //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  //
//  See the License for the specific language governing permissions and       //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

package transformer

import (
	"fmt"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	log "github.com/golang/glog"
	"github.com/openconfig/gnmi/proto/gnmi"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// The interfaces of a network instance are the L3 interfaces bound to it: the
// base row of each in the table of its type, keyed by the interface name. A
// row naming a VRF in vrf_name is bound to that instance, a row naming none to
// the default instance. The id is the interface name, so interface and
// subinterface are derived from it, and the state from the configuration.
//
// intfmgrd does not move an interface between VRFs in place: a vrf_name
// written over another is skipped. Binding an interface another instance holds
// is therefore refused, and the interface is removed from there first.
// Removing one deletes its base row, which for a subinterface is the
// subinterface itself. intfmgrd holds that back while an address is left, so
// it is refused too until the addresses are gone.
const (
	niIntfVrfFld  = "vrf_name"
	niIntfVnetFld = "vnet_name"
	niIntfVrfTbl  = "VRF"
	niIntfSubTbl  = "VLAN_SUB_INTERFACE"

	niIntfNiListNode = "network-instance"
	niIntfListNode   = "interface"

	niIntfTargetRoot  = "ni-interfaces-root"
	niIntfTargetInst  = "ni-interface-instance"
	niIntfTargetCfg   = "ni-interface-config"
	niIntfTargetState = "ni-interface-state"

	niIntfIdLeaf  = "id"
	niIntfIfLeaf  = "interface"
	niIntfSubLeaf = "subinterface"
	niIntfAfsLeaf = "associated-address-families"
)

// The base row tables by interface type, in the order a listing reads them.
var niIntfTables = []string{"INTERFACE", "LOOPBACK_INTERFACE", "PORTCHANNEL_INTERFACE", "VLAN_INTERFACE", niIntfSubTbl}

const niIntfUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/interfaces`

const niIntfLeaves = `(id|interface|subinterface|associated-address-families)`

var niIntfRootReqUriRe = regexp.MustCompile(niIntfUriPfx + `(?:/interface)?$`)
var niIntfInstReqUriRe = regexp.MustCompile(niIntfUriPfx + `/interface\[id=([^\]]+)\](?:/(id))?$`)
var niIntfCfgReqUriRe = regexp.MustCompile(niIntfUriPfx + `/interface\[id=([^\]]+)\]/config(?:/` + niIntfLeaves + `)?$`)
var niIntfStateReqUriRe = regexp.MustCompile(niIntfUriPfx + `/interface\[id=([^\]]+)\]/state(?:/` + niIntfLeaves + `)?$`)

// A wildcard key is removed by translib before the request reaches here.
var niIntfWildcardReqUriRe = regexp.MustCompile(niIntfUriPfx + `/interface/(config|state)(?:/` + niIntfLeaves + `)?$`)

// The leaves a write may carry, relative to the interfaces container.
var niIntfMappedPayloadLeaves = map[string]bool{
	"interface/id":                  true,
	"interface/config/id":           true,
	"interface/config/interface":    true,
	"interface/config/subinterface": true,
}

type niIntfReqInfo struct {
	niName         string
	id             string
	targetLeaf     string
	targetPathKind string
}

func init() {
	XlateFuncBind("YangToDb_ni_intf_xfmr", YangToDb_ni_intf_xfmr)
	XlateFuncBind("DbToYang_ni_intf_xfmr", DbToYang_ni_intf_xfmr)
	XlateFuncBind("Subscribe_ni_intf_xfmr", Subscribe_ni_intf_xfmr)
	XlateFuncBind("DbToYangPath_ni_intf_path_xfmr", DbToYangPath_ni_intf_path_xfmr)
}

func parseNiIntfRequestUri(uri string) (*niIntfReqInfo, error) {
	mk := func(m []string, kind string, idIdx, leafIdx int) *niIntfReqInfo {
		r := &niIntfReqInfo{niName: m[1], targetPathKind: kind}
		if idIdx > 0 && len(m) > idIdx {
			r.id = niStaticRouteUnescapeKey(m[idIdx])
		}
		if leafIdx > 0 && len(m) > leafIdx {
			r.targetLeaf = m[leafIdx]
		}
		return r
	}
	if m := niIntfCfgReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niIntfTargetCfg, 2, 3), nil
	}
	if m := niIntfStateReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niIntfTargetState, 2, 3), nil
	}
	if m := niIntfInstReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niIntfTargetInst, 2, 3), nil
	}
	if m := niIntfWildcardReqUriRe.FindStringSubmatch(uri); m != nil {
		kind := niIntfTargetCfg
		if m[2] == "state" {
			kind = niIntfTargetState
		}
		return mk(m, kind, 0, 3), nil
	}
	if m := niIntfRootReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niIntfTargetRoot, 0, 0), nil
	}
	return nil, tlerr.NotSupportedError{
		Format: "unsupported path under network-instance interfaces: " + uri,
		Path:   uri,
	}
}

// niIntfTableOf names the table that holds the base row of an interface.
func niIntfTableOf(id string) (string, error) {
	switch {
	case strings.Contains(id, "."):
		return niIntfSubTbl, nil
	case strings.HasPrefix(id, "Ethernet"):
		return "INTERFACE", nil
	case strings.HasPrefix(id, "PortChannel"):
		return "PORTCHANNEL_INTERFACE", nil
	case strings.HasPrefix(id, "Vlan"):
		return "VLAN_INTERFACE", nil
	case strings.HasPrefix(id, "Loopback"):
		return "LOOPBACK_INTERFACE", nil
	}
	return "", tlerr.NotSupportedError{
		Format: fmt.Sprintf("interface %q cannot be bound to a network instance: only Ethernet, PortChannel, Vlan and Loopback interfaces and subinterfaces can", id),
	}
}

func niIntfIsTable(table string) bool {
	for _, t := range niIntfTables {
		if t == table {
			return true
		}
	}
	return false
}

// niIntfParts splits an id into the interface and the subinterface index it
// names. A subinterface is named <interface>.<index>.
func niIntfParts(id string) (string, uint32, bool, error) {
	i := strings.LastIndex(id, ".")
	if i < 0 {
		return id, 0, false, nil
	}
	idx, err := strconv.ParseUint(id[i+1:], 10, 32)
	if err != nil || i == 0 {
		return "", 0, false, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("network-instance interface id %q names no subinterface index after its dot", id),
		}
	}
	return id[:i], uint32(idx), true, nil
}

// niIntfRowNi names the instance a base row is bound to. A row bound to a VNET
// belongs to no network instance.
func niIntfRowNi(entry db.Value) (string, bool) {
	if strings.TrimSpace(entry.Field[niIntfVnetFld]) != "" {
		return "", false
	}
	if vrf := strings.TrimSpace(entry.Field[niIntfVrfFld]); vrf != "" {
		return vrf, true
	}
	return niContainerDefaultNameLower, true
}

func niIntfSameNi(a, b string) bool {
	if niContainerIsDefaultInstanceName(a) || niContainerIsDefaultInstanceName(b) {
		return niContainerIsDefaultInstanceName(a) && niContainerIsDefaultInstanceName(b)
	}
	return a == b
}

// niIntfDb is the CONFIG_DB a write reads through: the request's handle, so a
// row staged earlier in the same transaction is seen.
func niIntfDb(inParams XfmrParams) *db.DB {
	if inParams.d != nil {
		return inParams.d
	}
	return inParams.dbs[db.ConfigDB]
}

// niIntfRows lists the interfaces bound to niName, id to table.
func niIntfRows(d *db.DB, niName string) map[string]string {
	rows := map[string]string{}
	if d == nil {
		return rows
	}
	for _, table := range niIntfTables {
		keys, err := d.GetKeys(&db.TableSpec{Name: table})
		if err != nil {
			continue
		}
		for _, k := range keys {
			if len(k.Comp) != 1 || k.Comp[0] == "" {
				continue
			}
			entry, err := d.GetEntry(&db.TableSpec{Name: table}, k)
			if err != nil {
				continue
			}
			if holder, ok := niIntfRowNi(entry); ok && niIntfSameNi(holder, niName) {
				rows[k.Comp[0]] = table
			}
		}
	}
	return rows
}

func niIntfSortedIds(rows map[string]string) []string {
	ids := make([]string, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// niIntfInstanceNames lists the VRFs interfaces are bound to.
func niIntfInstanceNames(d *db.DB) []string {
	seen := map[string]bool{}
	for _, table := range niIntfTables {
		keys, err := d.GetKeys(&db.TableSpec{Name: table})
		if err != nil {
			continue
		}
		for _, k := range keys {
			if len(k.Comp) != 1 {
				continue
			}
			entry, err := d.GetEntry(&db.TableSpec{Name: table}, k)
			if err != nil {
				continue
			}
			if holder, ok := niIntfRowNi(entry); ok && !niContainerIsDefaultInstanceName(holder) {
				seen[holder] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func niIntfCheckVrf(d *db.DB, niName string) error {
	if niContainerIsDefaultInstanceName(niName) {
		return nil
	}
	if _, err := d.GetEntry(&db.TableSpec{Name: niIntfVrfTbl}, db.Key{Comp: []string{niName}}); err != nil {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("network-instance %q has no VRF in CONFIG_DB; create the VRF before binding interfaces to it", niName),
		}
	}
	return nil
}

// niIntfBindRow renders the base row that binds id to niName, or none when it
// is bound there already.
func niIntfBindRow(d *db.DB, niName, id string) (string, map[string]string, error) {
	table, err := niIntfTableOf(id)
	if err != nil {
		return "", nil, err
	}
	if entry, err := d.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{id}}); err == nil {
		holder, ok := niIntfRowNi(entry)
		if !ok {
			return "", nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("interface %q is bound to VNET %q, not to a network instance", id, entry.Field[niIntfVnetFld]),
			}
		}
		if niIntfSameNi(holder, niName) {
			return table, nil, nil
		}
		return "", nil, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("interface %q is bound to network-instance %q; remove it from that network instance first", id, holder),
		}
	}
	if niContainerIsDefaultInstanceName(niName) {
		return table, map[string]string{"NULL": "NULL"}, nil
	}
	return table, map[string]string{niIntfVrfFld: niName}, nil
}

// niIntfUnbindRow names the base row that removing id from niName deletes, if
// id is bound there.
func niIntfUnbindRow(d *db.DB, niName, id string) (string, bool, error) {
	table, err := niIntfTableOf(id)
	if err != nil {
		return "", false, err
	}
	entry, err := d.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{id}})
	if err != nil {
		return "", false, nil
	}
	if holder, ok := niIntfRowNi(entry); !ok || !niIntfSameNi(holder, niName) {
		return "", false, nil
	}
	if addrs, err := d.GetKeysPattern(&db.TableSpec{Name: table}, db.Key{Comp: []string{id, "*"}}); err == nil && len(addrs) > 0 {
		return "", false, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("interface %q still has IP addresses; remove them before removing it from network-instance %q", id, niName),
		}
	}
	return table, true, nil
}

func niIntfUnbindRows(d *db.DB, niName string, ids []string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	for _, id := range ids {
		table, bound, err := niIntfUnbindRow(d, niName, id)
		if err != nil {
			return nil, err
		}
		if !bound {
			continue
		}
		if _, ok := res[table]; !ok {
			res[table] = make(map[string]db.Value)
		}
		res[table][id] = db.Value{Field: map[string]string{}}
	}
	return res, nil
}

// niIntfCheckIdentity refuses an interface or subinterface leaf that does not
// name what the id does.
func niIntfCheckIdentity(id string, ifName *string, sub *uint32) error {
	base, idx, hasSub, err := niIntfParts(id)
	if err != nil {
		return err
	}
	if ifName != nil {
		if strings.Contains(*ifName, ".") {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("interface %q names a subinterface; give the parent interface and the subinterface index", *ifName),
			}
		}
		if *ifName != base {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("interface %q does not match the interface %q that id %q names", *ifName, base, id),
			}
		}
	}
	if sub != nil {
		if !hasSub {
			if *sub == 0 {
				return tlerr.NotSupportedError{
					Format: fmt.Sprintf("subinterface 0 is not supported; interface %q is bound by its id alone", id),
				}
			}
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("subinterface %d does not match id %q, which names no subinterface", *sub, id),
			}
		}
		if *sub != idx {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("subinterface %d does not match the index %d that id %q names", *sub, idx, id),
			}
		}
	}
	return nil
}

// wrapNiIntfRootAsDevice normalises the bind root to a Device tree. The
// request binder anchors the root at the requested node.
func wrapNiIntfRootAsDevice(s *ygot.GoStruct, reqInfo *niIntfReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for network-instance interface mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}
	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	niObj, err := devObj.NetworkInstances.NewNetworkInstance(reqInfo.niName)
	if err != nil || niObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for interface mapping"}
	}
	ygot.BuildEmptyTree(niObj)
	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces:
		niObj.Interfaces = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface:
		if reqInfo.id == "" {
			return nil, tlerr.InvalidArgsError{Format: "network-instance interface id missing in request path"}
		}
		niObj.Interfaces.Interface = map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface{reqInfo.id: root}
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for network-instance interface mapping"}
	}
	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}

func getNiIntfsObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces {
	if s == nil || *s == nil {
		return nil
	}
	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	if deviceObj.NetworkInstances == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}
	niObj := deviceObj.NetworkInstances.NetworkInstance[niName]
	if niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		if niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(niName); err != nil {
			return nil
		}
	}
	if niObj.Interfaces == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Interfaces = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces{}
	}
	return niObj.Interfaces
}

func getNiIntfObj(s *ygot.GoStruct, niName, id string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface {
	if s != nil && *s != nil {
		if iObj, ok := (*s).(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface); ok && iObj != nil {
			return iObj
		}
	}
	isObj := getNiIntfsObj(s, niName, createIfMissing)
	if isObj == nil {
		return nil
	}
	iObj := isObj.Interface[id]
	if iObj == nil && createIfMissing {
		var err error
		if iObj, err = isObj.NewInterface(id); err != nil {
			return nil
		}
	}
	return iObj
}

// niIntfCheckPayload refuses what a write carries beyond the identity of each
// interface, and an identity that disagrees with its id.
func niIntfCheckPayload(s *ygot.GoStruct, niName string) error {
	isObj := getNiIntfsObj(s, niName, false)
	if isObj == nil {
		return nil
	}
	var unmapped []string
	collectOcUnmappedPayloadLeaves(reflect.ValueOf(isObj), niIntfMappedPayloadLeaves, "", "interfaces", 0, &unmapped)
	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		return tlerr.NotSupportedError{
			Format: "network-instance interface payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
		}
	}
	for _, id := range niIntfPayloadIds(isObj) {
		iObj := isObj.Interface[id]
		if iObj.Config == nil {
			continue
		}
		if iObj.Config.Id != nil && *iObj.Config.Id != id {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("network-instance interface id %q in payload does not match list key %q", *iObj.Config.Id, id),
			}
		}
		if err := niIntfCheckIdentity(id, iObj.Config.Interface, iObj.Config.Subinterface); err != nil {
			return err
		}
	}
	return nil
}

func niIntfPayloadIds(isObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces) []string {
	ids := make([]string, 0, len(isObj.Interface))
	for id, iObj := range isObj.Interface {
		if iObj != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func niIntfAddRow(res map[string]map[string]db.Value, table, id string, fields map[string]string) {
	if fields == nil {
		return
	}
	if _, ok := res[table]; !ok {
		res[table] = make(map[string]db.Value)
	}
	res[table][id] = db.Value{Field: fields}
}

var YangToDb_ni_intf_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (res map[string]map[string]db.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance interface transformer panic in SET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			res = make(map[string]map[string]db.Value)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance interface transformer panic in SET path: %v", r)}
		}
	}()
	res = make(map[string]map[string]db.Value)

	reqInfo, err := parseNiIntfRequestUri(niHandlerReqUri(inParams))
	if err != nil {
		return res, err
	}
	d := niIntfDb(inParams)
	if d == nil {
		return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance interface mapping"}
	}

	if reqInfo.targetPathKind == niIntfTargetState {
		return res, tlerr.NotSupportedError{
			Format: "network-instance interface state is read-only; write to the config container instead",
			Path:   inParams.requestUri,
		}
	}
	if reqInfo.targetLeaf == niIntfAfsLeaf {
		return res, tlerr.NotSupportedError{
			Format: "network-instance interface associated-address-families has no CONFIG_DB mapping",
			Path:   inParams.requestUri,
		}
	}

	if inParams.oper == DELETE {
		switch reqInfo.targetPathKind {
		case niIntfTargetRoot:
			return niIntfUnbindRows(d, reqInfo.niName, niIntfSortedIds(niIntfRows(d, reqInfo.niName)))
		case niIntfTargetInst, niIntfTargetCfg:
			// The id, the interface and the subinterface are the identity of
			// the binding, so clearing any of them removes it.
			return niIntfUnbindRows(d, reqInfo.niName, []string{reqInfo.id})
		}
		return res, tlerr.NotSupportedError{Format: "unsupported DELETE path under network-instance interfaces", Path: inParams.requestUri}
	}

	if err := niIntfCheckVrf(d, reqInfo.niName); err != nil {
		return res, err
	}
	wrappedRoot, wrapErr := wrapNiIntfRootAsDevice(inParams.ygRoot, reqInfo)
	if wrapErr != nil {
		return res, wrapErr
	}
	if err := niIntfCheckPayload(wrappedRoot, reqInfo.niName); err != nil {
		return res, err
	}

	ids := []string{reqInfo.id}
	if reqInfo.targetPathKind == niIntfTargetRoot {
		ids = nil
		if isObj := getNiIntfsObj(wrappedRoot, reqInfo.niName, false); isObj != nil {
			ids = niIntfPayloadIds(isObj)
		}
		if len(ids) == 0 {
			return res, tlerr.InvalidArgsError{Format: "network-instance interfaces payload does not include any interface"}
		}
	}
	for _, id := range ids {
		table, fields, err := niIntfBindRow(d, reqInfo.niName, id)
		if err != nil {
			return make(map[string]map[string]db.Value), err
		}
		niIntfAddRow(res, table, id, fields)
	}

	if reqInfo.targetPathKind == niIntfTargetRoot && inParams.oper == REPLACE {
		// The list is replaced as a whole, so an interface the payload leaves
		// out is removed from the instance.
		inPayload := map[string]bool{}
		for _, id := range ids {
			inPayload[id] = true
		}
		var gone []string
		for _, id := range niIntfSortedIds(niIntfRows(d, reqInfo.niName)) {
			if !inPayload[id] {
				gone = append(gone, id)
			}
		}
		clear, err := niIntfUnbindRows(d, reqInfo.niName, gone)
		if err != nil {
			return make(map[string]map[string]db.Value), err
		}
		if err := niBgpStageSubOp(inParams, DELETE, clear); err != nil {
			return make(map[string]map[string]db.Value), err
		}
	}
	if reqInfo.targetPathKind != niIntfTargetCfg && inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}
	return res, nil
}

// populateNiIntf fills the config and the state of one interface, or the leaf
// leaf names of the one kind names. Both are derived from the id.
func populateNiIntf(iObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface, id, kind, leaf string) {
	base, idx, hasSub, err := niIntfParts(id)
	if err != nil {
		return
	}
	idv := id
	iObj.Id = &idv
	want := func(name string) bool { return leaf == "" || leaf == name }
	var pId, pIf **string
	var pSub **uint32
	fill := func() {
		if want(niIntfIdLeaf) {
			v := id
			*pId = &v
		}
		if want(niIntfIfLeaf) {
			v := base
			*pIf = &v
		}
		if hasSub && want(niIntfSubLeaf) {
			v := idx
			*pSub = &v
		}
	}
	if kind != niIntfTargetState {
		if iObj.Config == nil {
			iObj.Config = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface_Config{}
		}
		pId, pIf, pSub = &iObj.Config.Id, &iObj.Config.Interface, &iObj.Config.Subinterface
		fill()
	}
	if kind != niIntfTargetCfg {
		if iObj.State == nil {
			iObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface_State{}
		}
		pId, pIf, pSub = &iObj.State.Id, &iObj.State.Interface, &iObj.State.Subinterface
		fill()
	}
}

var DbToYang_ni_intf_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance interface transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance interface transformer panic in GET path: %v", r)}
		}
	}()

	reqInfo, err := parseNiIntfRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance interface mapping"}
	}
	if reqInfo.targetLeaf == niIntfAfsLeaf {
		// No CONFIG_DB mapping, so nothing to report.
		return nil
	}

	rows := niIntfRows(cfgDb, reqInfo.niName)
	if reqInfo.targetPathKind == niIntfTargetRoot {
		wrappedRoot, wrapErr := wrapNiIntfRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return wrapErr
		}
		for _, id := range niIntfSortedIds(rows) {
			if iObj := getNiIntfObj(wrappedRoot, reqInfo.niName, id, true); iObj != nil {
				populateNiIntf(iObj, id, niIntfTargetInst, "")
			}
		}
		return nil
	}
	if reqInfo.id == "" {
		return tlerr.NotSupportedError{Format: "a wildcard or absent network-instance interface id is served at the interface list; address interfaces/interface rather than a container below it"}
	}
	if _, ok := rows[reqInfo.id]; !ok {
		return tlerr.NotFoundError{Format: fmt.Sprintf("network-instance %s has no interface %s", reqInfo.niName, reqInfo.id)}
	}
	var iObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface
	if reqInfo.targetPathKind == niIntfTargetInst {
		wrappedRoot, wrapErr := wrapNiIntfRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return wrapErr
		}
		iObj = getNiIntfObj(wrappedRoot, reqInfo.niName, reqInfo.id, true)
	} else {
		iObj = getNiIntfObj(inParams.ygRoot, reqInfo.niName, reqInfo.id, true)
	}
	if iObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate network-instance interface object in YANG tree"}
	}
	populateNiIntf(iObj, reqInfo.id, reqInfo.targetPathKind, reqInfo.targetLeaf)
	return nil
}

var niIntfSubscReqUriRe = regexp.MustCompile(niIntfUriPfx + `(?:/interface(?:\[id=([^\]]+)\])?(?:/(.*))?)?$`)

// Subscribe_ni_intf_xfmr maps the base rows of the interfaces. Config and
// state are both read from them and change only with the configuration, so
// every node is on-change. Which instance a row belongs to is in its vrf_name,
// not its key, so the rows of every instance are watched and the path
// transformer names the instance of each.
var Subscribe_ni_intf_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}
	anchor := inParams.uri
	if anchor == "" {
		anchor = inParams.requestURI
	}
	m := niIntfSubscReqUriRe.FindStringSubmatch(anchor)
	if m == nil {
		result.isVirtualTbl = true
		return result, nil
	}
	id := niStaticRouteUnescapeKey(m[2])
	tables := map[string]map[string]map[string]string{}
	if id != "" && id != "*" {
		table, err := niIntfTableOf(id)
		if err != nil {
			return result, err
		}
		tables[table] = map[string]map[string]string{id: {}}
	} else {
		for _, table := range niIntfTables {
			tables[table] = map[string]map[string]string{"*": {}}
		}
	}
	result.dbDataMap = RedisDbSubscribeMap{db.ConfigDB: tables}
	result.isVirtualTbl = false
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	return result, nil
}

// niIntfSubscribedNi names the network instance a subscription path keys, or
// "" when it leaves it open.
func niIntfSubscribedNi(p *gnmi.Path) string {
	if p == nil {
		return ""
	}
	for _, e := range p.GetElem() {
		if e.GetName() == niIntfNiListNode {
			if name := e.GetKey()["name"]; name != "*" {
				return name
			}
			return ""
		}
	}
	return ""
}

// DbToYangPath_ni_intf_path_xfmr names the instance and the interface a base
// row belongs to. An address row, or a row of another instance than the one
// subscribed to, is unrelated.
var DbToYangPath_ni_intf_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if !niIntfIsTable(params.tblName) || len(params.tblKeyComp) != 1 || params.tblKeyComp[0] == "" {
		return fmt.Errorf("ni_intf_path_xfmr: %s key %v is not an interface base row: %w", params.tblName, params.tblKeyComp, apis.ErrUnrelatedEntry)
	}
	niName := ""
	if params.tblEntry != nil {
		holder, ok := niIntfRowNi(*params.tblEntry)
		if !ok {
			return fmt.Errorf("ni_intf_path_xfmr: %s %s is bound to a VNET: %w", params.tblName, params.tblKeyComp[0], apis.ErrUnrelatedEntry)
		}
		niName = holder
	}
	if want := niIntfSubscribedNi(params.subscribePath); want != "" {
		if niName != "" && !niIntfSameNi(niName, want) {
			return fmt.Errorf("ni_intf_path_xfmr: %s %s is bound to %s, not %s: %w", params.tblName, params.tblKeyComp[0], niName, want, apis.ErrUnrelatedEntry)
		}
		niName = want
	}
	if niName == "" {
		return fmt.Errorf("ni_intf_path_xfmr: %s %s names no network instance: %w", params.tblName, params.tblKeyComp[0], apis.ErrUnrelatedEntry)
	}
	niPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niIntfNiListNode)
	if !ok {
		return nil
	}
	params.ygPathKeys[niPath+"/name"] = niName
	if intfPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niIntfListNode); ok {
		params.ygPathKeys[intfPath+"/"+niIntfIdLeaf] = params.tblKeyComp[0]
	}
	return nil
}
