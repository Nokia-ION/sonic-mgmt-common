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
	"regexp"
	"runtime/debug"
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

// The VLANs of the default network instance are the SONiC VLANs, one
// CONFIG_DB VLAN|Vlan<id> row each, the same rows the top-level /vlans tree
// maps, read and written by the same rules: vlan-id is vlanid, the name is
// derived as Vlan<id>, status is admin_status, and state and members are what
// vlanmgrd applied to APPL_DB. SONiC has a single L2 domain, so no other
// network instance holds a VLAN: a write there is refused and a read reports
// nothing.
const (
	niVlanNiListNode   = "network-instance"
	niVlanListNode     = "vlan"
	niVlanIdLeaf       = "vlan-id"
	niVlanNameLeaf     = "name"
	niVlanStatusLeaf   = "status"
	niVlanAdminStatFld = "admin_status"

	niVlanTargetRoot    = "ni-vlans-root"
	niVlanTargetInst    = "ni-vlan-instance"
	niVlanTargetCfg     = "ni-vlan-config"
	niVlanTargetState   = "ni-vlan-state"
	niVlanTargetMembers = "ni-vlan-members"
)

const niVlanUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/vlans`

const niVlanLeaves = `(vlan-id|name|status)`

var niVlanRootReqUriRe = regexp.MustCompile(niVlanUriPfx + `(?:/vlan)?$`)
var niVlanInstReqUriRe = regexp.MustCompile(niVlanUriPfx + `/vlan\[vlan-id=([^\]]+)\](?:/(vlan-id))?$`)
var niVlanCfgReqUriRe = regexp.MustCompile(niVlanUriPfx + `/vlan\[vlan-id=([^\]]+)\]/config(?:/` + niVlanLeaves + `)?$`)
var niVlanStateReqUriRe = regexp.MustCompile(niVlanUriPfx + `/vlan\[vlan-id=([^\]]+)\]/state(?:/` + niVlanLeaves + `)?$`)
var niVlanMembersReqUriRe = regexp.MustCompile(niVlanUriPfx + `/vlan\[vlan-id=([^\]]+)\]/members(?:/.*)?$`)

// A wildcard key is removed by translib before the request reaches here.
var niVlanWildcardReqUriRe = regexp.MustCompile(niVlanUriPfx + `/vlan/(config|state|members)(?:/(.*))?$`)

type niVlanReqInfo struct {
	niName         string
	vlanId         uint16
	keyed          bool
	targetLeaf     string
	targetPathKind string
}

func init() {
	XlateFuncBind("YangToDb_ni_vlans_xfmr", YangToDb_ni_vlans_xfmr)
	XlateFuncBind("DbToYang_ni_vlans_xfmr", DbToYang_ni_vlans_xfmr)
	XlateFuncBind("Subscribe_ni_vlans_xfmr", Subscribe_ni_vlans_xfmr)
	XlateFuncBind("DbToYangPath_ni_vlans_path_xfmr", DbToYangPath_ni_vlans_path_xfmr)
}

func parseNiVlanRequestUri(uri string) (*niVlanReqInfo, error) {
	mk := func(m []string, kind string, idIdx, leafIdx int) (*niVlanReqInfo, error) {
		r := &niVlanReqInfo{niName: m[1], targetPathKind: kind}
		if idIdx > 0 && len(m) > idIdx && m[idIdx] != "" {
			id, err := strconv.ParseUint(m[idIdx], 10, 16)
			if err != nil {
				return nil, tlerr.InvalidArgsError{Format: "invalid VLAN id " + m[idIdx]}
			}
			r.vlanId, r.keyed = uint16(id), true
		}
		if leafIdx > 0 && len(m) > leafIdx {
			r.targetLeaf = m[leafIdx]
		}
		return r, nil
	}
	if m := niVlanCfgReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niVlanTargetCfg, 2, 3)
	}
	if m := niVlanStateReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niVlanTargetState, 2, 3)
	}
	if m := niVlanMembersReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niVlanTargetMembers, 2, 0)
	}
	if m := niVlanInstReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niVlanTargetInst, 2, 3)
	}
	if m := niVlanWildcardReqUriRe.FindStringSubmatch(uri); m != nil {
		kind := map[string]string{"config": niVlanTargetCfg, "state": niVlanTargetState, "members": niVlanTargetMembers}[m[2]]
		r, err := mk(m, kind, 0, 0)
		if err == nil && kind != niVlanTargetMembers && m[3] != "" {
			r.targetLeaf = m[3]
		}
		return r, err
	}
	if m := niVlanRootReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niVlanTargetRoot, 0, 0)
	}
	return nil, tlerr.NotSupportedError{Format: "unsupported path under network-instance vlans: " + uri, Path: uri}
}

func niVlanServesNi(niName string) bool {
	return niContainerIsDefaultInstanceName(niName)
}

func niVlanOtherInstanceErr(niName string) error {
	return tlerr.NotSupportedError{
		Format: fmt.Sprintf("VLANs exist only in the default network instance; SONiC has no per-instance VLAN, so network-instance %q holds none", niName),
	}
}

// wrapNiVlanRootAsDevice normalises the bind root to a Device tree. The
// request binder anchors the root at the requested node.
func wrapNiVlanRootAsDevice(s *ygot.GoStruct, reqInfo *niVlanReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for network-instance VLAN mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}
	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	niObj, err := devObj.NetworkInstances.NewNetworkInstance(reqInfo.niName)
	if err != nil || niObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for VLAN mapping"}
	}
	ygot.BuildEmptyTree(niObj)
	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans:
		niObj.Vlans = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan:
		if !reqInfo.keyed {
			return nil, tlerr.InvalidArgsError{Format: "network-instance VLAN id missing in request path"}
		}
		niObj.Vlans.Vlan = map[uint16]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan{reqInfo.vlanId: root}
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for network-instance VLAN mapping"}
	}
	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}

func getNiVlansObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans {
	if !createIfMissing {
		if s == nil || *s == nil {
			return nil
		}
		devObj, ok := (*s).(*ocbinds.Device)
		if !ok || devObj.NetworkInstances == nil {
			return nil
		}
		niObj := devObj.NetworkInstances.NetworkInstance[niName]
		if niObj == nil {
			return nil
		}
		return niObj.Vlans
	}
	niObj, err := niContainerGetOrCreateNi(s, niName)
	if err != nil || niObj == nil {
		return nil
	}
	if niObj.Vlans == nil {
		niObj.Vlans = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans{}
	}
	return niObj.Vlans
}

// niVlanDbFields renders the VLAN row of one payload entry, by the rules the
// top-level /vlans tree writes it with.
func niVlanDbFields(vlanId uint16, vlanObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan, oper Operation, statusLeaf bool, cfgDb *db.DB) (map[string]string, error) {
	if err := validateSonicVlanId(vlanId); err != nil {
		return nil, err
	}
	var cfg *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan_Config
	if vlanObj != nil {
		if vlanObj.State != nil || vlanObj.Members != nil {
			return nil, tlerr.NotSupportedError{Format: "network-instance VLAN state and members are read-only; write to the config container instead"}
		}
		cfg = vlanObj.Config
	}
	if cfg != nil {
		if cfg.VlanId != nil && *cfg.VlanId != vlanId {
			return nil, tlerr.InvalidArgsError{Format: "VLAN id in payload does not match " + strconv.Itoa(int(vlanId))}
		}
		if err := validateOcVlanName(vlanId, cfg.Name); err != nil {
			return nil, err
		}
	}
	fields := map[string]string{"vlanid": strconv.Itoa(int(vlanId))}
	statusSpecified := cfg != nil && cfg.Status != ocVlanStatusUnset
	if oper == REPLACE || !vlanEntryExists(cfgDb, sonicVlanName(vlanId)) || statusSpecified || statusLeaf {
		admin := "up"
		if statusSpecified {
			admin = adminStatusFromOc(cfg.Status)
		}
		fields[niVlanAdminStatFld] = admin
	}
	return fields, nil
}

func niVlanDeleteAll(cfgDb *db.DB) (map[string]map[string]db.Value, error) {
	vlanMap := make(map[string]db.Value)
	names, err := listConfigDbVlanKeys(cfgDb)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := addVlanDeleteKeys(cfgDb, name, vlanMap); err != nil {
			return nil, err
		}
	}
	return ocVlansDeleteResult(vlanMap), nil
}

var YangToDb_ni_vlans_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (res map[string]map[string]db.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance VLAN transformer panic in SET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			res = make(map[string]map[string]db.Value)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance VLAN transformer panic in SET path: %v", r)}
		}
	}()
	res = make(map[string]map[string]db.Value)

	reqInfo, err := parseNiVlanRequestUri(niHandlerReqUri(inParams))
	if err != nil {
		return res, err
	}
	if !niVlanServesNi(reqInfo.niName) {
		return res, niVlanOtherInstanceErr(reqInfo.niName)
	}
	if reqInfo.targetPathKind == niVlanTargetState || reqInfo.targetPathKind == niVlanTargetMembers {
		return res, tlerr.NotSupportedError{
			Format: "network-instance VLAN state and members are read-only; write to the config container instead",
			Path:   inParams.requestUri,
		}
	}
	cfgDb := ocVlansCfgDb(inParams)
	if cfgDb == nil {
		return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance VLAN mapping"}
	}

	if inParams.oper == DELETE {
		if !reqInfo.keyed {
			return niVlanDeleteAll(cfgDb)
		}
		if err := validateSonicVlanId(reqInfo.vlanId); err != nil {
			return res, err
		}
		switch reqInfo.targetLeaf {
		case niVlanIdLeaf, niVlanNameLeaf:
			if reqInfo.targetPathKind == niVlanTargetCfg {
				return res, tlerr.NotSupportedError{
					Format: "network-instance VLAN config " + reqInfo.targetLeaf + " cannot be deleted on its own; delete the VLAN instead",
					Path:   inParams.requestUri,
				}
			}
		case niVlanStatusLeaf:
			// status has no unset value: dropping it sets the VLAN back to
			// up, which is what a VLAN written without one gets.
			updateSubOpDataMap(map[db.DBNum]map[string]map[string]db.Value{db.ConfigDB: {
				VLAN_TN: {sonicVlanName(reqInfo.vlanId): {Field: map[string]string{niVlanAdminStatFld: "up"}}},
			}}, UPDATE, inParams)
			return res, nil
		}
		vlanMap := make(map[string]db.Value)
		if err := addVlanDeleteKeys(cfgDb, sonicVlanName(reqInfo.vlanId), vlanMap); err != nil {
			return res, err
		}
		return ocVlansDeleteResult(vlanMap), nil
	}

	wrappedRoot, err := wrapNiVlanRootAsDevice(inParams.ygRoot, reqInfo)
	if err != nil {
		return res, err
	}
	payload := map[uint16]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan{}
	if vObj := getNiVlansObj(wrappedRoot, reqInfo.niName, false); vObj != nil && vObj.Vlan != nil {
		payload = vObj.Vlan
	}
	vlanMap := make(map[string]db.Value)
	if reqInfo.keyed {
		fields, err := niVlanDbFields(reqInfo.vlanId, payload[reqInfo.vlanId], inParams.oper, reqInfo.targetLeaf == niVlanStatusLeaf, cfgDb)
		if err != nil {
			return res, err
		}
		vlanMap[sonicVlanName(reqInfo.vlanId)] = db.Value{Field: fields}
	} else {
		if len(payload) == 0 && inParams.oper != REPLACE {
			return res, tlerr.InvalidArgsError{Format: "network-instance vlans payload does not include any VLAN"}
		}
		for vlanId, vlanObj := range payload {
			fields, err := niVlanDbFields(vlanId, vlanObj, inParams.oper, false, cfgDb)
			if err != nil {
				return make(map[string]map[string]db.Value), err
			}
			vlanMap[sonicVlanName(vlanId)] = db.Value{Field: fields}
		}
		if inParams.oper == REPLACE {
			// The list is replaced as a whole, so a VLAN the payload leaves
			// out goes, refused while it has members as any VLAN delete is.
			names, err := listConfigDbVlanKeys(cfgDb)
			if err != nil {
				return make(map[string]map[string]db.Value), err
			}
			gone := make(map[string]db.Value)
			for _, name := range names {
				if _, keep := vlanMap[name]; keep {
					continue
				}
				if err := addVlanDeleteKeys(cfgDb, name, gone); err != nil {
					return make(map[string]map[string]db.Value), err
				}
			}
			if len(gone) > 0 {
				updateSubOpDataMap(map[db.DBNum]map[string]map[string]db.Value{db.ConfigDB: ocVlansDeleteResult(gone)}, DELETE, inParams)
			}
		}
	}
	if reqInfo.targetPathKind != niVlanTargetCfg && inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}
	if len(vlanMap) == 0 {
		return res, nil
	}
	return map[string]map[string]db.Value{VLAN_TN: vlanMap}, nil
}

// populateNiVlan fills one VLAN: config from its CONFIG_DB row, state and
// members from what vlanmgrd applied to APPL_DB, as top-level /vlans reports
// them.
func populateNiVlan(vObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans, vlanName string, vlanId uint16,
	cfgEntry db.Value, applEntry db.Value, applied bool, members []string, reqInfo *niVlanReqInfo) error {
	eObj, ok := vObj.Vlan[vlanId]
	if !ok || eObj == nil {
		var err error
		if eObj, err = vObj.NewVlan(vlanId); err != nil {
			return err
		}
	}
	kind, leaf := reqInfo.targetPathKind, reqInfo.targetLeaf
	all := kind == niVlanTargetRoot || kind == niVlanTargetInst
	if all || kind == niVlanTargetCfg {
		if eObj.Config == nil {
			eObj.Config = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan_Config{}
		}
		if shouldFillLeaf(leaf, niVlanIdLeaf) {
			id := vlanId
			eObj.Config.VlanId = &id
		}
		if shouldFillLeaf(leaf, niVlanNameLeaf) {
			name := vlanName
			eObj.Config.Name = &name
		}
		if shouldFillLeaf(leaf, niVlanStatusLeaf) {
			eObj.Config.Status = vlanStatusFromAdmin(cfgEntry.Field[niVlanAdminStatFld])
		}
	}
	if applied && (all || kind == niVlanTargetState) {
		if eObj.State == nil {
			eObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan_State{}
		}
		if shouldFillLeaf(leaf, niVlanIdLeaf) {
			id := vlanId
			eObj.State.VlanId = &id
		}
		if shouldFillLeaf(leaf, niVlanNameLeaf) {
			name := vlanName
			eObj.State.Name = &name
		}
		if shouldFillLeaf(leaf, niVlanStatusLeaf) {
			eObj.State.Status = vlanStatusFromAdmin(applEntry.Field[niVlanAdminStatFld])
		}
	}
	if applied && (all || kind == niVlanTargetMembers) {
		if eObj.Members == nil {
			eObj.Members = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan_Members{}
		}
		for _, ifName := range members {
			ifNameCopy := ifName
			eObj.Members.Member = append(eObj.Members.Member, &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan_Members_Member{
				State: &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Vlans_Vlan_Members_Member_State{Interface: &ifNameCopy},
			})
		}
	}
	return nil
}

// niVlanNamesFromDb lists the VLANs the default instance holds.
func niVlanNamesFromDb(cfgDb *db.DB) []string {
	names, err := listConfigDbVlanKeys(cfgDb)
	if err != nil {
		return nil
	}
	return names
}

var DbToYang_ni_vlans_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance VLAN transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance VLAN transformer panic in GET path: %v", r)}
		}
	}()

	reqInfo, err := parseNiVlanRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	if !niVlanServesNi(reqInfo.niName) {
		if reqInfo.keyed {
			return tlerr.NotFoundError{Format: fmt.Sprintf("network-instance %s holds no VLAN; VLANs exist only in the default network instance", reqInfo.niName)}
		}
		return nil
	}
	cfgDb := ocVlansCfgDb(inParams)
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance VLAN mapping"}
	}
	appDb := ocVlansApplDb(inParams)

	var wrappedRoot *ygot.GoStruct
	if reqInfo.targetPathKind == niVlanTargetRoot || reqInfo.targetPathKind == niVlanTargetInst {
		if wrappedRoot, err = wrapNiVlanRootAsDevice(inParams.ygRoot, reqInfo); err != nil {
			return err
		}
	} else {
		if !reqInfo.keyed {
			return tlerr.NotSupportedError{Format: "a wildcard or absent network-instance VLAN id is served at the vlan list; address vlans/vlan rather than a container below it"}
		}
		wrappedRoot = inParams.ygRoot
	}
	vObj := getNiVlansObj(wrappedRoot, reqInfo.niName, true)
	if vObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate network-instance vlans in YANG tree"}
	}

	stateOnly := reqInfo.targetPathKind == niVlanTargetState || reqInfo.targetPathKind == niVlanTargetMembers
	found := false
	for _, vlanName := range niVlanNamesFromDb(cfgDb) {
		cfgEntry, err := cfgDb.GetEntry(vlanCfgTableSpec(), db.Key{Comp: []string{vlanName}})
		if err != nil {
			if tlerr.IsTranslibRedisClientEntryNotExist(err) {
				continue
			}
			return err
		}
		vlanId, ok := vlanIdFromEntry(vlanName, cfgEntry)
		if !ok || (reqInfo.keyed && vlanId != reqInfo.vlanId) {
			continue
		}
		applEntry, applied, err := getApplVlanEntry(appDb, vlanName)
		if err != nil {
			return err
		}
		if stateOnly && !applied {
			continue
		}
		var members []string
		if applied && reqInfo.targetPathKind != niVlanTargetCfg && reqInfo.targetPathKind != niVlanTargetState {
			if members, err = listVlanMembers(appDb, vlanMemberAppTableSpec(), vlanName); err != nil {
				return err
			}
		}
		if err := populateNiVlan(vObj, vlanName, vlanId, cfgEntry, applEntry, applied, members, reqInfo); err != nil {
			return err
		}
		found = true
	}
	if reqInfo.keyed && !found {
		return tlerr.NotFoundError{Format: fmt.Sprintf("network-instance %s has no VLAN %d", reqInfo.niName, reqInfo.vlanId)}
	}
	return nil
}

var niVlanSubscReqUriRe = regexp.MustCompile(niVlanUriPfx + `(?:/vlan(?:\[vlan-id=([^\]]+)\])?(?:/(.*))?)?$`)

// Subscribe_ni_vlans_xfmr maps the VLAN rows on change: CONFIG_DB VLAN for the
// config, APPL_DB VLAN_TABLE for the state and VLAN_MEMBER_TABLE for the
// members, as top-level /vlans watches them.
var Subscribe_ni_vlans_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.isVirtualTbl = true
	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		return result, nil
	}
	anchor := inParams.uri
	if anchor == "" {
		anchor = inParams.requestURI
	}
	m := niVlanSubscReqUriRe.FindStringSubmatch(anchor)
	if m == nil || !niVlanServesNi(m[1]) {
		return result, nil
	}
	vlanName := ocVlanSubscribeName(m[2])
	below := m[3]
	result.dbDataMap = make(RedisDbSubscribeMap)
	if below == "" || below == "vlan-id" || below == "config" || strings.HasPrefix(below, "config/") {
		ocVlansEnsureSubMap(result.dbDataMap, db.ConfigDB)[VLAN_TN] = map[string]map[string]string{vlanName: {niVlanAdminStatFld: niVlanStatusLeaf}}
	}
	if below == "" || below == "state" || strings.HasPrefix(below, "state/") {
		ocVlansEnsureSubMap(result.dbDataMap, db.ApplDB)[VLAN_APP_TBL] = map[string]map[string]string{vlanName: {niVlanAdminStatFld: niVlanStatusLeaf}}
	}
	if below == "" || strings.HasPrefix(below, "members") {
		ocVlansEnsureSubMap(result.dbDataMap, db.ApplDB)[VLAN_MEMBER_APP_TBL] = map[string]map[string]string{ocVlanMemberSubscribeKey(vlanName): {KEY_COMP_CNT: "2"}}
	}
	result.isVirtualTbl = false
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	return result, nil
}

func niVlanSubscribedNi(p *gnmi.Path) string {
	for _, e := range p.GetElem() {
		if e.GetName() == niVlanNiListNode {
			if name := e.GetKey()["name"]; name != "*" {
				return name
			}
			return ""
		}
	}
	return ""
}

// DbToYangPath_ni_vlans_path_xfmr names the VLAN a VLAN, VLAN_TABLE or
// VLAN_MEMBER_TABLE row belongs to, for a subscription that left the key
// open.
var DbToYangPath_ni_vlans_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	switch params.tblName {
	case VLAN_TN, VLAN_APP_TBL, VLAN_MEMBER_APP_TBL:
	default:
		return fmt.Errorf("ni_vlans_path_xfmr: notification from table %q: %w", params.tblName, apis.ErrUnrelatedEntry)
	}
	if len(params.tblKeyComp) == 0 {
		return fmt.Errorf("ni_vlans_path_xfmr: %s key names no VLAN: %w", params.tblName, apis.ErrUnrelatedEntry)
	}
	vlanId, ok := parseVlanIdFromName(params.tblKeyComp[0])
	if !ok {
		return fmt.Errorf("ni_vlans_path_xfmr: %s key %v names no VLAN: %w", params.tblName, params.tblKeyComp, apis.ErrUnrelatedEntry)
	}
	niName := niVlanSubscribedNi(params.subscribePath)
	if niName == "" {
		niName = niContainerDefaultNameLower
	} else if !niVlanServesNi(niName) {
		return fmt.Errorf("ni_vlans_path_xfmr: network-instance %s holds no VLAN: %w", niName, apis.ErrUnrelatedEntry)
	}
	niPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niVlanNiListNode)
	if !ok {
		return nil
	}
	params.ygPathKeys[niPath+"/name"] = niName
	if vlanPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niVlanListNode); ok {
		params.ygPathKeys[vlanPath+"/"+niVlanIdLeaf] = strconv.Itoa(int(vlanId))
	}
	return nil
}
