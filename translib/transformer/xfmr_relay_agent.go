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
//  See the License for the specific language governing permissions and       //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

package transformer

import (
	"net"
	"sort"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	dhcpRelayTable          = "DHCP_RELAY"
	dhcpv6ServersField      = "dhcpv6_servers"
	ocRelayDhcpv6Path       = "/openconfig-relay-agent:relay-agent/dhcpv6"
	ocRelayDhcpv6IntfPath   = "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface"
	ocRelayDhcpv6ConfigPath = "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface/config"
	ocRelayDhcpv6StatePath  = "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface/state"
)

func init() {
	XlateFuncBind("YangToDb_dhcpv4_relay_agent_xfmr", YangToDb_dhcpv4_relay_agent_xfmr)
	XlateFuncBind("DbToYang_dhcpv4_relay_agent_xfmr", DbToYang_dhcpv4_relay_agent_xfmr)
	XlateFuncBind("Subscribe_dhcpv4_relay_agent_xfmr", Subscribe_dhcpv4_relay_agent_xfmr)
	XlateFuncBind("DbToYangPath_dhcpv4_relay_agent_intf_path_xfmr", DbToYangPath_dhcpv4_relay_agent_intf_path_xfmr)
	XlateFuncBind("YangToDb_dhcpv6_relay_agent_xfmr", YangToDb_dhcpv6_relay_agent_xfmr)
	XlateFuncBind("DbToYang_dhcpv6_relay_agent_xfmr", DbToYang_dhcpv6_relay_agent_xfmr)
	XlateFuncBind("Subscribe_dhcpv6_relay_agent_xfmr", Subscribe_dhcpv6_relay_agent_xfmr)
	XlateFuncBind("DbToYangPath_dhcpv6_relay_agent_intf_path_xfmr", DbToYangPath_dhcpv6_relay_agent_intf_path_xfmr)
}

func dhcpv6RelayCfgDb(inParams XfmrParams) *db.DB {
	if inParams.dbs[db.ConfigDB] != nil {
		return inParams.dbs[db.ConfigDB]
	}
	return inParams.d
}

func dhcpv6RelayTargetPath(inParams XfmrParams) string {
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	if targetUriPath == "" {
		targetUriPath, _, _ = XfmrRemoveXPATHPredicates(inParams.uri)
	}
	return targetUriPath
}

func dhcpv6RelayIfIdFromParams(inParams XfmrParams) string {
	id := NewPathInfo(inParams.uri).Var("id")
	if id == "" {
		id = NewPathInfo(inParams.requestUri).Var("id")
	}
	return id
}

func validateDhcpv6RelayIfId(ifId string) error {
	if ifId == "" {
		return tlerr.InvalidArgsError{Format: "DHCPv6 relay interface id is required"}
	}
	vlanId, ok := parseVlanIdFromName(ifId)
	if !ok {
		return tlerr.InvalidArgsError{Format: "DHCPv6 relay interface id must be a VLAN interface (Vlan<id>)"}
	}
	return validateSonicVlanId(vlanId)
}

func validateRelayAgentVlanExists(cfgDb *db.DB, ifId string) error {
	if cfgDb == nil {
		return nil
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: VLAN_TN}, db.Key{Comp: []string{ifId}})
	if err != nil {
		if tlerr.IsTranslibRedisClientEntryNotExist(err) {
			return tlerr.InvalidArgsError{Format: ifId + " doesn't exist"}
		}
		return err
	}
	if !entry.IsPopulated() {
		return tlerr.InvalidArgsError{Format: ifId + " doesn't exist"}
	}
	return nil
}

func validateDhcpv6HelperAddress(addr string) error {
	ip := net.ParseIP(addr)
	if ip == nil || ip.To4() != nil {
		return tlerr.InvalidArgsError{Format: "Invalid DHCPv6 helper-address " + addr}
	}
	return nil
}

func validateDhcpv6HelperAddresses(addrs []string) error {
	seen := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		if err := validateDhcpv6HelperAddress(addr); err != nil {
			return err
		}
		if _, dup := seen[addr]; dup {
			return tlerr.InvalidArgsError{Format: "Duplicate DHCPv6 helper-address " + addr}
		}
		seen[addr] = struct{}{}
	}
	return nil
}

func helperAddressInstFromUri(uri string) string {
	const marker = "helper-address["
	idx := strings.LastIndex(uri, marker)
	if idx < 0 {
		return ""
	}
	rest := uri[idx+len(marker):]
	end := strings.Index(rest, "]")
	if end < 0 {
		return ""
	}
	expr := strings.TrimSpace(rest[:end])
	if !strings.HasPrefix(expr, ".=") {
		return ""
	}
	val := strings.TrimPrefix(expr, ".=")
	return strings.Trim(val, "'\"")
}

func uniqueDhcpv6Helpers(vals []string) []string {
	out := make([]string, 0, len(vals))
	seen := make(map[string]struct{}, len(vals))
	for _, val := range vals {
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func mergeHelperAddresses(existing, added []string) []string {
	return uniqueDhcpv6Helpers(append(append([]string{}, existing...), added...))
}

func removeHelperAddress(existing []string, addr string) []string {
	out := make([]string, 0, len(existing))
	for _, cur := range existing {
		if cur != addr {
			out = append(out, cur)
		}
	}
	return out
}

// dhcpRelayLeafListDeleteMap builds a DELETE map that translib applies as a
// leaf-list field delete. UPDATE cannot be used: common_app merges leaf-lists
// on UPDATE (append only) and never removes instances.
//
// An empty remove list clears the whole leaf-list (field@=""). When
// deleteWholeKey is set and remove is empty, the table key itself is deleted.
func dhcpRelayLeafListDeleteMap(table, field, ifId string, remove []string, deleteWholeKey bool) map[string]map[string]db.Value {
	if deleteWholeKey && len(remove) == 0 {
		return map[string]map[string]db.Value{table: {ifId: db.Value{}}}
	}
	val := db.Value{Field: map[string]string{}}
	if len(remove) == 0 {
		name := field
		if !strings.HasSuffix(name, "@") {
			name += "@"
		}
		val.Field[name] = ""
	} else {
		val.SetList(field, remove)
	}
	return map[string]map[string]db.Value{table: {ifId: val}}
}

func dhcpv6ServersFromEntry(entry db.Value) []string {
	return uniqueDhcpv6Helpers(entry.GetList(dhcpv6ServersField))
}

func getDhcpv6RelayRoot(s *ygot.GoStruct) *ocbinds.OpenconfigRelayAgent_RelayAgent {
	deviceObj := (*s).(*ocbinds.Device)
	if deviceObj.RelayAgent == nil {
		ygot.BuildEmptyTree(deviceObj)
	}
	if deviceObj.RelayAgent == nil {
		deviceObj.RelayAgent = &ocbinds.OpenconfigRelayAgent_RelayAgent{}
	}
	if deviceObj.RelayAgent.Dhcpv6 == nil {
		deviceObj.RelayAgent.Dhcpv6 = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6{}
	}
	if deviceObj.RelayAgent.Dhcpv6.Interfaces == nil {
		deviceObj.RelayAgent.Dhcpv6.Interfaces = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces{}
	}
	return deviceObj.RelayAgent
}

type dhcpv6RelayFill struct {
	config     bool
	state      bool
	configLeaf string
	stateLeaf  string
}

func dhcpv6RelayFillFromPath(targetUriPath string) dhcpv6RelayFill {
	switch {
	case targetUriPath == ocRelayDhcpv6ConfigPath+"/id":
		return dhcpv6RelayFill{config: true, configLeaf: "id"}
	case targetUriPath == ocRelayDhcpv6ConfigPath+"/enable":
		return dhcpv6RelayFill{config: true, configLeaf: "enable"}
	case targetUriPath == ocRelayDhcpv6ConfigPath+"/helper-address":
		return dhcpv6RelayFill{config: true, configLeaf: "helper-address"}
	case targetUriPath == ocRelayDhcpv6StatePath+"/id":
		return dhcpv6RelayFill{state: true, stateLeaf: "id"}
	case targetUriPath == ocRelayDhcpv6StatePath+"/enable":
		return dhcpv6RelayFill{state: true, stateLeaf: "enable"}
	case targetUriPath == ocRelayDhcpv6StatePath+"/helper-address":
		return dhcpv6RelayFill{state: true, stateLeaf: "helper-address"}
	case targetUriPath == ocRelayDhcpv6ConfigPath:
		return dhcpv6RelayFill{config: true}
	case targetUriPath == ocRelayDhcpv6StatePath:
		return dhcpv6RelayFill{state: true}
	default:
		return dhcpv6RelayFill{config: true, state: true}
	}
}

func dhcpv6RelayReadOnlyPath(targetUriPath string) bool {
	return strings.HasPrefix(targetUriPath, ocRelayDhcpv6StatePath)
}

func fillDhcpv6RelayConfig(cfg *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface_Config, ifId string, servers []string, leaf string) {
	if shouldFillLeaf(leaf, "id") {
		id := ifId
		cfg.Id = &id
	}
	if shouldFillLeaf(leaf, "enable") {
		enabled := len(servers) > 0
		cfg.Enable = &enabled
	}
	if shouldFillLeaf(leaf, "helper-address") {
		cfg.HelperAddress = append([]string{}, servers...)
	}
}

func fillDhcpv6RelayState(st *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface_State, ifId string, servers []string, leaf string) {
	if shouldFillLeaf(leaf, "id") {
		id := ifId
		st.Id = &id
	}
	if shouldFillLeaf(leaf, "enable") {
		enabled := len(servers) > 0
		st.Enable = &enabled
	}
	if shouldFillLeaf(leaf, "helper-address") {
		st.HelperAddress = append([]string{}, servers...)
	}
}

func populateDhcpv6RelayIntf(intfs *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces, ifId string, servers []string, fill dhcpv6RelayFill) error {
	intfObj, ok := intfs.Interface[ifId]
	if !ok {
		var err error
		intfObj, err = intfs.NewInterface(ifId)
		if err != nil {
			return err
		}
	}
	if fill.config {
		if intfObj.Config == nil {
			intfObj.Config = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface_Config{}
		}
		fillDhcpv6RelayConfig(intfObj.Config, ifId, servers, fill.configLeaf)
	}
	if fill.state {
		if intfObj.State == nil {
			intfObj.State = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface_State{}
		}
		fillDhcpv6RelayState(intfObj.State, ifId, servers, fill.stateLeaf)
	}
	return nil
}

func listDhcpRelayKeys(cfgDb *db.DB) ([]string, error) {
	if cfgDb == nil {
		return nil, nil
	}
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: dhcpRelayTable})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		name := key.Get(0)
		if _, ok := parseVlanIdFromName(name); ok {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		idi, _ := parseVlanIdFromName(names[i])
		idj, _ := parseVlanIdFromName(names[j])
		return idi < idj
	})
	return names, nil
}

func getDhcpRelayEntry(cfgDb *db.DB, ifId string) (db.Value, bool, error) {
	if cfgDb == nil {
		return db.Value{Field: map[string]string{}}, false, nil
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: dhcpRelayTable}, db.Key{Comp: []string{ifId}})
	if err != nil {
		if tlerr.IsTranslibRedisClientEntryNotExist(err) {
			return db.Value{Field: map[string]string{}}, false, nil
		}
		return db.Value{}, false, err
	}
	if !entry.IsPopulated() {
		return db.Value{Field: map[string]string{}}, false, nil
	}
	if entry.Field == nil {
		entry.Field = map[string]string{}
	}
	return entry, true, nil
}

func dhcpRelayValueWithServers(existing db.Value, servers []string) db.Value {
	fields := map[string]string{}
	if existing.Field != nil {
		for k, v := range existing.Field {
			if k == dhcpv6ServersField || k == dhcpv6ServersField+"@" {
				continue
			}
			fields[k] = v
		}
	}
	val := db.Value{Field: fields}
	val.SetList(dhcpv6ServersField, servers)
	return val
}

func dhcpRelayEntryEmpty(val db.Value) bool {
	if val.Field == nil {
		return true
	}
	for k, v := range val.Field {
		if k == dhcpv6ServersField || k == dhcpv6ServersField+"@" {
			if v == "" {
				continue
			}
			return false
		}
		if v != "" {
			return false
		}
	}
	return true
}

func dhcpv6RelayDeleteOrUpdate(inParams XfmrParams, ifId string, existing db.Value, servers []string) (map[string]map[string]db.Value, error) {
	existingServers := dhcpv6ServersFromEntry(existing)
	if len(servers) == 0 {
		return dhcpRelayLeafListDeleteMap(dhcpRelayTable, dhcpv6ServersField, ifId, nil, true), nil
	}
	removed := make([]string, 0)
	keep := make(map[string]struct{}, len(servers))
	for _, s := range servers {
		keep[s] = struct{}{}
	}
	for _, s := range existingServers {
		if _, ok := keep[s]; !ok {
			removed = append(removed, s)
		}
	}
	if len(removed) == 0 {
		return map[string]map[string]db.Value{}, nil
	}
	return dhcpRelayLeafListDeleteMap(dhcpRelayTable, dhcpv6ServersField, ifId, removed, false), nil
}

func dhcpv6RelayPayloadMap(inParams XfmrParams) map[string]*ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface {
	root := getDhcpv6RelayRoot(inParams.ygRoot)
	if root == nil || root.Dhcpv6 == nil || root.Dhcpv6.Interfaces == nil || root.Dhcpv6.Interfaces.Interface == nil {
		return map[string]*ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface{}
	}
	return root.Dhcpv6.Interfaces.Interface
}

func payloadHelperAddresses(intfObj *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface) []string {
	if intfObj == nil || intfObj.Config == nil {
		return nil
	}
	return uniqueDhcpv6Helpers(intfObj.Config.HelperAddress)
}

func payloadEnable(intfObj *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface) (bool, bool) {
	if intfObj == nil || intfObj.Config == nil || intfObj.Config.Enable == nil {
		return false, false
	}
	return *intfObj.Config.Enable, true
}

func dhcpv6RelayYangToDbWrite(inParams XfmrParams, cfgDb *db.DB, targetUriPath string) (map[string]map[string]db.Value, error) {
	payload := dhcpv6RelayPayloadMap(inParams)
	reqId := dhcpv6RelayIfIdFromParams(inParams)

	if reqId != "" {
		if err := validateDhcpv6RelayIfId(reqId); err != nil {
			return nil, err
		}
		return dhcpv6RelayWriteOne(inParams, cfgDb, targetUriPath, reqId, payload[reqId])
	}

	if len(payload) == 0 {
		if inParams.oper == REPLACE {
			return dhcpv6RelayYangToDbDelete(inParams, cfgDb, targetUriPath)
		}
		return map[string]map[string]db.Value{}, nil
	}

	result := make(map[string]db.Value)
	for ifId, intfObj := range payload {
		if err := validateDhcpv6RelayIfId(ifId); err != nil {
			return nil, err
		}
		one, err := dhcpv6RelayWriteOne(inParams, cfgDb, targetUriPath, ifId, intfObj)
		if err != nil {
			return nil, err
		}
		for k, v := range one[dhcpRelayTable] {
			result[k] = v
		}
	}

	if inParams.oper == REPLACE && inParams.subOpDataMap != nil {
		existing, err := listDhcpRelayKeys(cfgDb)
		if err != nil {
			return nil, err
		}
		delMap := make(map[string]db.Value)
		for _, ifId := range existing {
			if _, keep := payload[ifId]; keep {
				continue
			}
			delMap[ifId] = db.Value{}
		}
		if len(delMap) > 0 {
			updateSubOpDataMap(map[db.DBNum]map[string]map[string]db.Value{
				db.ConfigDB: {dhcpRelayTable: delMap},
			}, DELETE, inParams)
		}
	}

	return map[string]map[string]db.Value{dhcpRelayTable: result}, nil
}

func dhcpv6RelayWriteOne(inParams XfmrParams, cfgDb *db.DB, targetUriPath string, ifId string, intfObj *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface) (map[string]map[string]db.Value, error) {
	if err := validateRelayAgentVlanExists(cfgDb, ifId); err != nil {
		return nil, err
	}
	existing, _, err := getDhcpRelayEntry(cfgDb, ifId)
	if err != nil {
		return nil, err
	}
	servers := dhcpv6ServersFromEntry(existing)
	helpers := payloadHelperAddresses(intfObj)
	enabled, enableSet := payloadEnable(intfObj)
	helperLeaf := targetUriPath == ocRelayDhcpv6ConfigPath+"/helper-address"
	enableLeaf := targetUriPath == ocRelayDhcpv6ConfigPath+"/enable"
	idLeaf := targetUriPath == ocRelayDhcpv6ConfigPath+"/id"

	if idLeaf {
		return map[string]map[string]db.Value{
			dhcpRelayTable: {
				ifId: dhcpRelayValueWithServers(existing, servers),
			},
		}, nil
	}

	if helperLeaf || len(helpers) > 0 {
		if err = validateDhcpv6HelperAddresses(helpers); err != nil {
			return nil, err
		}
		switch inParams.oper {
		case REPLACE:
			servers = helpers
		default:
			if helperLeaf {
				servers = mergeHelperAddresses(servers, helpers)
			} else {
				servers = helpers
			}
		}
	}

	if enableLeaf || enableSet {
		if enableSet && !enabled {
			servers = nil
		} else if enableSet && enabled && len(servers) == 0 {
			return nil, tlerr.InvalidArgsError{Format: "DHCPv6 relay enable requires at least one helper-address"}
		}
	}

	val := dhcpRelayValueWithServers(existing, servers)
	if dhcpRelayEntryEmpty(val) {
		if enableLeaf || (enableSet && !enabled) || (helperLeaf && inParams.oper == REPLACE) {
			return map[string]map[string]db.Value{
				dhcpRelayTable: {ifId: db.Value{}},
			}, nil
		}
		return map[string]map[string]db.Value{}, nil
	}
	return map[string]map[string]db.Value{
		dhcpRelayTable: {ifId: val},
	}, nil
}

func dhcpv6RelayYangToDbDelete(inParams XfmrParams, cfgDb *db.DB, targetUriPath string) (map[string]map[string]db.Value, error) {
	reqId := dhcpv6RelayIfIdFromParams(inParams)
	if targetUriPath == ocRelayDhcpv6ConfigPath+"/id" {
		return nil, tlerr.NotSupported("DELETE not supported on " + targetUriPath)
	}

	if reqId != "" {
		if err := validateDhcpv6RelayIfId(reqId); err != nil {
			return nil, err
		}

		if targetUriPath != ocRelayDhcpv6ConfigPath+"/helper-address" && targetUriPath != ocRelayDhcpv6ConfigPath+"/enable" {
			return map[string]map[string]db.Value{dhcpRelayTable: {reqId: db.Value{}}}, nil
		}

		existing, exists, err := getDhcpRelayEntry(cfgDb, reqId)
		if err != nil {
			return nil, err
		}
		if !exists {
			return map[string]map[string]db.Value{dhcpRelayTable: {reqId: db.Value{}}}, nil
		}

		if targetUriPath == ocRelayDhcpv6ConfigPath+"/helper-address" {
			inst := helperAddressInstFromUri(inParams.requestUri)
			if inst == "" {
				inst = helperAddressInstFromUri(inParams.uri)
			}
			servers := dhcpv6ServersFromEntry(existing)
			if inst != "" {
				servers = removeHelperAddress(servers, inst)
			} else {
				servers = nil
			}
			return dhcpv6RelayDeleteOrUpdate(inParams, reqId, existing, servers)
		}

		if targetUriPath == ocRelayDhcpv6ConfigPath+"/enable" {
			return dhcpv6RelayDeleteOrUpdate(inParams, reqId, existing, nil)
		}

		return map[string]map[string]db.Value{dhcpRelayTable: {reqId: db.Value{}}}, nil
	}

	names, err := listDhcpRelayKeys(cfgDb)
	if err != nil {
		return nil, err
	}
	delMap := make(map[string]db.Value, len(names))
	for _, ifId := range names {
		delMap[ifId] = db.Value{}
	}
	return map[string]map[string]db.Value{dhcpRelayTable: delMap}, nil
}

var YangToDb_dhcpv6_relay_agent_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}

	targetUriPath := dhcpv6RelayTargetPath(inParams)
	if dhcpv6RelayReadOnlyPath(targetUriPath) {
		return nil, tlerr.NotSupported("SET not supported on " + targetUriPath)
	}

	cfgDb := dhcpv6RelayCfgDb(inParams)
	switch inParams.oper {
	case DELETE:
		return dhcpv6RelayYangToDbDelete(inParams, cfgDb, targetUriPath)
	case CREATE, UPDATE, REPLACE:
		return dhcpv6RelayYangToDbWrite(inParams, cfgDb, targetUriPath)
	default:
		return nil, tlerr.NotSupported("Unsupported operation on " + ocRelayDhcpv6Path)
	}
}

var DbToYang_dhcpv6_relay_agent_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	cfgDb := dhcpv6RelayCfgDb(inParams)
	if cfgDb == nil {
		return tlerr.InternalError{Format: "CONFIG_DB handle is not available for DHCPv6 relay-agent GET"}
	}

	root := getDhcpv6RelayRoot(inParams.ygRoot)
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.uri)
	if targetUriPath == "" {
		targetUriPath, _, _ = XfmrRemoveXPATHPredicates(inParams.requestUri)
	}
	fill := dhcpv6RelayFillFromPath(targetUriPath)
	reqId := dhcpv6RelayIfIdFromParams(inParams)

	names, err := listDhcpRelayKeys(cfgDb)
	if err != nil {
		return err
	}

	matched := false
	for _, ifId := range names {
		if reqId != "" && ifId != reqId {
			continue
		}
		entry, exists, getErr := getDhcpRelayEntry(cfgDb, ifId)
		if getErr != nil {
			return getErr
		}
		if !exists {
			continue
		}
		if err = populateDhcpv6RelayIntf(root.Dhcpv6.Interfaces, ifId, dhcpv6ServersFromEntry(entry), fill); err != nil {
			return err
		}
		matched = true
	}

	if reqId != "" && !matched {
		return tlerr.NotFoundError{Format: "Resource not found"}
	}
	return nil
}

var Subscribe_dhcpv6_relay_agent_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)

	ifId := NewPathInfo(inParams.uri).Var("id")
	if ifId == "" {
		ifId = "*"
	}

	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = true
		return result, nil
	}

	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	// Parent existence for SET is validated against VLAN. DHCP_RELAY is created
	// by the first helper-address write and must not be required beforehand.
	// Wildcard subscribe still watches DHCP_RELAY for helper on-change.
	if ifId == "*" {
		result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
			dhcpRelayTable: {ifId: {}},
		}
	} else {
		result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
			VLAN_TN: {ifId: {}},
		}
	}
	return result, nil
}

var DbToYangPath_dhcpv6_relay_agent_intf_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != dhcpRelayTable && params.tblName != VLAN_TN {
		return tlerr.InternalError{Format: "wrong config DB table sent"}
	}
	if len(params.tblKeyComp) == 0 {
		return tlerr.InternalError{Format: "Missing DB key"}
	}
	params.ygPathKeys[ocRelayDhcpv6IntfPath+"/id"] = params.tblKeyComp[0]
	return nil
}

const (
	dhcpv4RelayTable         = "DHCPV4_RELAY"
	dhcpv4ServersField       = "dhcpv4_servers"
	vlanDhcpServersField     = "dhcp_servers"
	deviceMetadataTable      = "DEVICE_METADATA"
	deviceMetadataLocalhost  = "localhost"
	hasSonicDhcpv4RelayField = "has_sonic_dhcpv4_relay"

	ocRelayDhcpPath       = "/openconfig-relay-agent:relay-agent/dhcp"
	ocRelayDhcpIntfPath   = "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface"
	ocRelayDhcpConfigPath = "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface/config"
	ocRelayDhcpStatePath  = "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface/state"
)

type dhcpv4RelayStore struct {
	tableName      string
	serversField   string
	deleteWholeKey bool
}

func useSonicDhcpv4Relay(cfgDb *db.DB) bool {
	if cfgDb == nil {
		return false
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: deviceMetadataTable}, db.Key{Comp: []string{deviceMetadataLocalhost}})
	if err != nil {
		return false
	}
	return entry.Get(hasSonicDhcpv4RelayField) == "True"
}

func dhcpv4RelayStoreFromCfg(cfgDb *db.DB) dhcpv4RelayStore {
	if useSonicDhcpv4Relay(cfgDb) {
		return dhcpv4RelayStore{
			tableName:      dhcpv4RelayTable,
			serversField:   dhcpv4ServersField,
			deleteWholeKey: true,
		}
	}
	return dhcpv4RelayStore{
		tableName:      VLAN_TN,
		serversField:   vlanDhcpServersField,
		deleteWholeKey: false,
	}
}

func dhcpv4RelayCfgDb(inParams XfmrParams) *db.DB {
	if inParams.dbs[db.ConfigDB] != nil {
		return inParams.dbs[db.ConfigDB]
	}
	return inParams.d
}

func dhcpv4RelayTargetPath(inParams XfmrParams) string {
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	if targetUriPath == "" {
		targetUriPath, _, _ = XfmrRemoveXPATHPredicates(inParams.uri)
	}
	return targetUriPath
}

func dhcpv4RelayIfIdFromParams(inParams XfmrParams) string {
	id := NewPathInfo(inParams.uri).Var("id")
	if id == "" {
		id = NewPathInfo(inParams.requestUri).Var("id")
	}
	return id
}

func validateDhcpv4RelayIfId(ifId string) error {
	if ifId == "" {
		return tlerr.InvalidArgsError{Format: "DHCPv4 relay interface id is required"}
	}
	vlanId, ok := parseVlanIdFromName(ifId)
	if !ok {
		return tlerr.InvalidArgsError{Format: "DHCPv4 relay interface id must be a VLAN interface (Vlan<id>)"}
	}
	return validateSonicVlanId(vlanId)
}

func validateDhcpv4VlanExists(cfgDb *db.DB, ifId string) error {
	return validateRelayAgentVlanExists(cfgDb, ifId)
}

func validateDhcpv4HelperAddress(addr string) error {
	ip := net.ParseIP(addr)
	if ip == nil || ip.To4() == nil {
		return tlerr.InvalidArgsError{Format: "Invalid DHCPv4 helper-address " + addr}
	}
	return nil
}

func validateDhcpv4HelperAddresses(addrs []string) error {
	seen := make(map[string]struct{}, len(addrs))
	for _, addr := range addrs {
		if err := validateDhcpv4HelperAddress(addr); err != nil {
			return err
		}
		if _, dup := seen[addr]; dup {
			return tlerr.InvalidArgsError{Format: "Duplicate DHCPv4 helper-address " + addr}
		}
		seen[addr] = struct{}{}
	}
	return nil
}

func uniqueRelayHelpers(vals []string) []string {
	out := make([]string, 0, len(vals))
	seen := make(map[string]struct{}, len(vals))
	for _, val := range vals {
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func dhcpv4ServersFromEntry(entry db.Value, field string) []string {
	return uniqueRelayHelpers(entry.GetList(field))
}

func getDhcpv4RelayEntry(cfgDb *db.DB, store dhcpv4RelayStore, ifId string) (db.Value, bool, error) {
	if cfgDb == nil {
		return db.Value{Field: map[string]string{}}, false, nil
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: store.tableName}, db.Key{Comp: []string{ifId}})
	if err != nil {
		if tlerr.IsTranslibRedisClientEntryNotExist(err) {
			return db.Value{Field: map[string]string{}}, false, nil
		}
		return db.Value{}, false, err
	}
	if !entry.IsPopulated() {
		return db.Value{Field: map[string]string{}}, false, nil
	}
	if entry.Field == nil {
		entry.Field = map[string]string{}
	}
	servers := dhcpv4ServersFromEntry(entry, store.serversField)
	if len(servers) == 0 {
		return entry, false, nil
	}
	return entry, true, nil
}

func dhcpv4RelayValueWithServers(existing db.Value, servers []string, field string) db.Value {
	fields := map[string]string{}
	if existing.Field != nil {
		for k, v := range existing.Field {
			if k == field || k == field+"@" {
				continue
			}
			fields[k] = v
		}
	}
	val := db.Value{Field: fields}
	if len(servers) > 0 {
		val.SetList(field, servers)
	}
	return val
}

func dhcpv4RelayEntryEmpty(val db.Value, store dhcpv4RelayStore) bool {
	if val.Field == nil {
		return true
	}
	for k, v := range val.Field {
		if k == store.serversField || k == store.serversField+"@" {
			if v == "" {
				continue
			}
			return false
		}
		if v != "" {
			return false
		}
	}
	return true
}

func listDhcpv4RelayKeys(cfgDb *db.DB, store dhcpv4RelayStore) ([]string, error) {
	if cfgDb == nil {
		return nil, nil
	}
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: store.tableName})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		name := key.Get(0)
		if _, ok := parseVlanIdFromName(name); !ok {
			continue
		}
		entry, exists, getErr := getDhcpv4RelayEntry(cfgDb, store, name)
		if getErr != nil {
			return nil, getErr
		}
		if !exists {
			continue
		}
		if len(dhcpv4ServersFromEntry(entry, store.serversField)) == 0 {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		idi, _ := parseVlanIdFromName(names[i])
		idj, _ := parseVlanIdFromName(names[j])
		return idi < idj
	})
	return names, nil
}

func getDhcpv4RelayRoot(s *ygot.GoStruct) *ocbinds.OpenconfigRelayAgent_RelayAgent {
	deviceObj := (*s).(*ocbinds.Device)
	if deviceObj.RelayAgent == nil {
		ygot.BuildEmptyTree(deviceObj)
	}
	if deviceObj.RelayAgent == nil {
		deviceObj.RelayAgent = &ocbinds.OpenconfigRelayAgent_RelayAgent{}
	}
	if deviceObj.RelayAgent.Dhcp == nil {
		deviceObj.RelayAgent.Dhcp = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp{}
	}
	if deviceObj.RelayAgent.Dhcp.Interfaces == nil {
		deviceObj.RelayAgent.Dhcp.Interfaces = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces{}
	}
	return deviceObj.RelayAgent
}

type dhcpv4RelayFill struct {
	config     bool
	state      bool
	configLeaf string
	stateLeaf  string
}

func dhcpv4RelayFillFromPath(targetUriPath string) dhcpv4RelayFill {
	switch {
	case targetUriPath == ocRelayDhcpConfigPath+"/id":
		return dhcpv4RelayFill{config: true, configLeaf: "id"}
	case targetUriPath == ocRelayDhcpConfigPath+"/enable":
		return dhcpv4RelayFill{config: true, configLeaf: "enable"}
	case targetUriPath == ocRelayDhcpConfigPath+"/helper-address":
		return dhcpv4RelayFill{config: true, configLeaf: "helper-address"}
	case targetUriPath == ocRelayDhcpStatePath+"/id":
		return dhcpv4RelayFill{state: true, stateLeaf: "id"}
	case targetUriPath == ocRelayDhcpStatePath+"/enable":
		return dhcpv4RelayFill{state: true, stateLeaf: "enable"}
	case targetUriPath == ocRelayDhcpStatePath+"/helper-address":
		return dhcpv4RelayFill{state: true, stateLeaf: "helper-address"}
	case targetUriPath == ocRelayDhcpConfigPath:
		return dhcpv4RelayFill{config: true}
	case targetUriPath == ocRelayDhcpStatePath:
		return dhcpv4RelayFill{state: true}
	default:
		return dhcpv4RelayFill{config: true, state: true}
	}
}

func dhcpv4RelayReadOnlyPath(targetUriPath string) bool {
	return strings.HasPrefix(targetUriPath, ocRelayDhcpStatePath)
}

func fillDhcpv4RelayConfig(cfg *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_Config, ifId string, servers []string, leaf string) {
	if shouldFillLeaf(leaf, "id") {
		id := ifId
		cfg.Id = &id
	}
	if shouldFillLeaf(leaf, "enable") {
		enabled := len(servers) > 0
		cfg.Enable = &enabled
	}
	if shouldFillLeaf(leaf, "helper-address") {
		cfg.HelperAddress = append([]string{}, servers...)
	}
}

func fillDhcpv4RelayState(st *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_State, ifId string, servers []string, leaf string) {
	if shouldFillLeaf(leaf, "id") {
		id := ifId
		st.Id = &id
	}
	if shouldFillLeaf(leaf, "enable") {
		enabled := len(servers) > 0
		st.Enable = &enabled
	}
	if shouldFillLeaf(leaf, "helper-address") {
		st.HelperAddress = append([]string{}, servers...)
	}
}

func populateDhcpv4RelayIntf(intfs *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces, ifId string, servers []string, fill dhcpv4RelayFill) error {
	intfObj, ok := intfs.Interface[ifId]
	if !ok {
		var err error
		intfObj, err = intfs.NewInterface(ifId)
		if err != nil {
			return err
		}
	}
	if fill.config {
		if intfObj.Config == nil {
			intfObj.Config = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_Config{}
		}
		fillDhcpv4RelayConfig(intfObj.Config, ifId, servers, fill.configLeaf)
	}
	if fill.state {
		if intfObj.State == nil {
			intfObj.State = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_State{}
		}
		fillDhcpv4RelayState(intfObj.State, ifId, servers, fill.stateLeaf)
	}
	return nil
}

func dhcpv4RelayDeleteOrUpdate(inParams XfmrParams, store dhcpv4RelayStore, ifId string, existing db.Value, servers []string) (map[string]map[string]db.Value, error) {
	existingServers := dhcpv4ServersFromEntry(existing, store.serversField)
	if len(servers) == 0 {
		return dhcpRelayLeafListDeleteMap(store.tableName, store.serversField, ifId, nil, store.deleteWholeKey), nil
	}
	removed := make([]string, 0)
	keep := make(map[string]struct{}, len(servers))
	for _, s := range servers {
		keep[s] = struct{}{}
	}
	for _, s := range existingServers {
		if _, ok := keep[s]; !ok {
			removed = append(removed, s)
		}
	}
	if len(removed) == 0 {
		return map[string]map[string]db.Value{}, nil
	}
	return dhcpRelayLeafListDeleteMap(store.tableName, store.serversField, ifId, removed, false), nil
}

func dhcpv4RelayPayloadMap(inParams XfmrParams) map[string]*ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface {
	root := getDhcpv4RelayRoot(inParams.ygRoot)
	if root == nil || root.Dhcp == nil || root.Dhcp.Interfaces == nil || root.Dhcp.Interfaces.Interface == nil {
		return map[string]*ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface{}
	}
	return root.Dhcp.Interfaces.Interface
}

func dhcpv4PayloadHelperAddresses(intfObj *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface) []string {
	if intfObj == nil || intfObj.Config == nil {
		return nil
	}
	return uniqueRelayHelpers(intfObj.Config.HelperAddress)
}

func dhcpv4PayloadEnable(intfObj *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface) (bool, bool) {
	if intfObj == nil || intfObj.Config == nil || intfObj.Config.Enable == nil {
		return false, false
	}
	return *intfObj.Config.Enable, true
}

func dhcpv4RelayWriteOne(inParams XfmrParams, cfgDb *db.DB, store dhcpv4RelayStore, targetUriPath string, ifId string, intfObj *ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface) (map[string]map[string]db.Value, error) {
	if err := validateDhcpv4VlanExists(cfgDb, ifId); err != nil {
		return nil, err
	}
	existing, _, err := getDhcpv4RelayEntry(cfgDb, store, ifId)
	if err != nil {
		return nil, err
	}
	servers := dhcpv4ServersFromEntry(existing, store.serversField)
	helpers := dhcpv4PayloadHelperAddresses(intfObj)
	enabled, enableSet := dhcpv4PayloadEnable(intfObj)
	helperLeaf := targetUriPath == ocRelayDhcpConfigPath+"/helper-address"
	enableLeaf := targetUriPath == ocRelayDhcpConfigPath+"/enable"
	idLeaf := targetUriPath == ocRelayDhcpConfigPath+"/id"

	if idLeaf {
		return map[string]map[string]db.Value{
			store.tableName: {
				ifId: dhcpv4RelayValueWithServers(existing, servers, store.serversField),
			},
		}, nil
	}

	if helperLeaf || len(helpers) > 0 {
		if err = validateDhcpv4HelperAddresses(helpers); err != nil {
			return nil, err
		}
		switch inParams.oper {
		case REPLACE:
			servers = helpers
		default:
			if helperLeaf {
				servers = mergeHelperAddresses(servers, helpers)
			} else {
				servers = helpers
			}
		}
	}

	if enableLeaf || enableSet {
		if enableSet && !enabled {
			servers = nil
		} else if enableSet && enabled && len(servers) == 0 {
			return nil, tlerr.InvalidArgsError{Format: "DHCPv4 relay enable requires at least one helper-address"}
		}
	}

	val := dhcpv4RelayValueWithServers(existing, servers, store.serversField)
	if store.deleteWholeKey && dhcpv4RelayEntryEmpty(val, store) {
		if enableLeaf || (enableSet && !enabled) || (helperLeaf && inParams.oper == REPLACE) {
			return map[string]map[string]db.Value{
				store.tableName: {ifId: db.Value{}},
			}, nil
		}
		return map[string]map[string]db.Value{}, nil
	}
	if !store.deleteWholeKey && len(servers) == 0 {
		if enableLeaf || (enableSet && !enabled) || (helperLeaf && inParams.oper == REPLACE) {
			return dhcpv4RelayDeleteOrUpdate(inParams, store, ifId, existing, servers)
		}
		return map[string]map[string]db.Value{}, nil
	}
	return map[string]map[string]db.Value{
		store.tableName: {ifId: val},
	}, nil
}

func dhcpv4RelayYangToDbWrite(inParams XfmrParams, cfgDb *db.DB, store dhcpv4RelayStore, targetUriPath string) (map[string]map[string]db.Value, error) {
	payload := dhcpv4RelayPayloadMap(inParams)
	reqId := dhcpv4RelayIfIdFromParams(inParams)

	if reqId != "" {
		if err := validateDhcpv4RelayIfId(reqId); err != nil {
			return nil, err
		}
		return dhcpv4RelayWriteOne(inParams, cfgDb, store, targetUriPath, reqId, payload[reqId])
	}

	if len(payload) == 0 {
		if inParams.oper == REPLACE {
			return dhcpv4RelayYangToDbDelete(inParams, cfgDb, store, targetUriPath)
		}
		return map[string]map[string]db.Value{}, nil
	}

	result := make(map[string]db.Value)
	for ifId, intfObj := range payload {
		if err := validateDhcpv4RelayIfId(ifId); err != nil {
			return nil, err
		}
		one, err := dhcpv4RelayWriteOne(inParams, cfgDb, store, targetUriPath, ifId, intfObj)
		if err != nil {
			return nil, err
		}
		for k, v := range one[store.tableName] {
			result[k] = v
		}
	}

	if inParams.oper == REPLACE && inParams.subOpDataMap != nil {
		existing, err := listDhcpv4RelayKeys(cfgDb, store)
		if err != nil {
			return nil, err
		}
		delMap := make(map[string]db.Value)
		for _, ifId := range existing {
			if _, keep := payload[ifId]; keep {
				continue
			}
			delMap[ifId] = db.Value{}
		}
		if len(delMap) > 0 {
			updateSubOpDataMap(map[db.DBNum]map[string]map[string]db.Value{
				db.ConfigDB: {store.tableName: delMap},
			}, DELETE, inParams)
		}
	}

	return map[string]map[string]db.Value{store.tableName: result}, nil
}

func dhcpv4RelayYangToDbDelete(inParams XfmrParams, cfgDb *db.DB, store dhcpv4RelayStore, targetUriPath string) (map[string]map[string]db.Value, error) {
	reqId := dhcpv4RelayIfIdFromParams(inParams)
	if targetUriPath == ocRelayDhcpConfigPath+"/id" {
		return nil, tlerr.NotSupported("DELETE not supported on " + targetUriPath)
	}

	if reqId != "" {
		if err := validateDhcpv4RelayIfId(reqId); err != nil {
			return nil, err
		}

		if targetUriPath != ocRelayDhcpConfigPath+"/helper-address" && targetUriPath != ocRelayDhcpConfigPath+"/enable" {
			if store.deleteWholeKey {
				return map[string]map[string]db.Value{store.tableName: {reqId: db.Value{}}}, nil
			}
			existing, exists, err := getDhcpv4RelayEntry(cfgDb, store, reqId)
			if err != nil {
				return nil, err
			}
			if !exists {
				return map[string]map[string]db.Value{}, nil
			}
			return dhcpv4RelayDeleteOrUpdate(inParams, store, reqId, existing, nil)
		}

		existing, exists, err := getDhcpv4RelayEntry(cfgDb, store, reqId)
		if err != nil {
			return nil, err
		}
		if !exists {
			if store.deleteWholeKey {
				return map[string]map[string]db.Value{store.tableName: {reqId: db.Value{}}}, nil
			}
			return map[string]map[string]db.Value{}, nil
		}

		if targetUriPath == ocRelayDhcpConfigPath+"/helper-address" {
			inst := helperAddressInstFromUri(inParams.requestUri)
			if inst == "" {
				inst = helperAddressInstFromUri(inParams.uri)
			}
			servers := dhcpv4ServersFromEntry(existing, store.serversField)
			if inst != "" {
				servers = removeHelperAddress(servers, inst)
			} else {
				servers = nil
			}
			return dhcpv4RelayDeleteOrUpdate(inParams, store, reqId, existing, servers)
		}

		if targetUriPath == ocRelayDhcpConfigPath+"/enable" {
			return dhcpv4RelayDeleteOrUpdate(inParams, store, reqId, existing, nil)
		}

		if store.deleteWholeKey {
			return map[string]map[string]db.Value{store.tableName: {reqId: db.Value{}}}, nil
		}
		return dhcpv4RelayDeleteOrUpdate(inParams, store, reqId, existing, nil)
	}

	names, err := listDhcpv4RelayKeys(cfgDb, store)
	if err != nil {
		return nil, err
	}
	delMap := make(map[string]db.Value, len(names))
	for _, ifId := range names {
		one := dhcpRelayLeafListDeleteMap(store.tableName, store.serversField, ifId, nil, store.deleteWholeKey)
		delMap[ifId] = one[store.tableName][ifId]
	}
	return map[string]map[string]db.Value{store.tableName: delMap}, nil
}

var YangToDb_dhcpv4_relay_agent_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}

	targetUriPath := dhcpv4RelayTargetPath(inParams)
	if dhcpv4RelayReadOnlyPath(targetUriPath) {
		return nil, tlerr.NotSupported("SET not supported on " + targetUriPath)
	}

	cfgDb := dhcpv4RelayCfgDb(inParams)
	store := dhcpv4RelayStoreFromCfg(cfgDb)
	switch inParams.oper {
	case DELETE:
		return dhcpv4RelayYangToDbDelete(inParams, cfgDb, store, targetUriPath)
	case CREATE, UPDATE, REPLACE:
		return dhcpv4RelayYangToDbWrite(inParams, cfgDb, store, targetUriPath)
	default:
		return nil, tlerr.NotSupported("Unsupported operation on " + ocRelayDhcpPath)
	}
}

var DbToYang_dhcpv4_relay_agent_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	cfgDb := dhcpv4RelayCfgDb(inParams)
	if cfgDb == nil {
		return tlerr.InternalError{Format: "CONFIG_DB handle is not available for DHCPv4 relay-agent GET"}
	}

	root := getDhcpv4RelayRoot(inParams.ygRoot)
	store := dhcpv4RelayStoreFromCfg(cfgDb)
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.uri)
	if targetUriPath == "" {
		targetUriPath, _, _ = XfmrRemoveXPATHPredicates(inParams.requestUri)
	}
	fill := dhcpv4RelayFillFromPath(targetUriPath)
	reqId := dhcpv4RelayIfIdFromParams(inParams)

	names, err := listDhcpv4RelayKeys(cfgDb, store)
	if err != nil {
		return err
	}

	matched := false
	for _, ifId := range names {
		if reqId != "" && ifId != reqId {
			continue
		}
		entry, exists, getErr := getDhcpv4RelayEntry(cfgDb, store, ifId)
		if getErr != nil {
			return getErr
		}
		if !exists {
			continue
		}
		if err = populateDhcpv4RelayIntf(root.Dhcp.Interfaces, ifId, dhcpv4ServersFromEntry(entry, store.serversField), fill); err != nil {
			return err
		}
		matched = true
	}

	if reqId != "" && !matched {
		return tlerr.NotFoundError{Format: "Resource not found"}
	}
	return nil
}

var Subscribe_dhcpv4_relay_agent_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)

	ifId := NewPathInfo(inParams.uri).Var("id")
	if ifId == "" {
		ifId = "*"
	}

	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = true
		return result, nil
	}

	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	// Parent existence for SET is validated against VLAN; relay helpers may live
	// in VLAN (legacy) or DHCPV4_RELAY (sonic relay). Do not require both tables.
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		VLAN_TN: {ifId: {}},
	}
	return result, nil
}

var DbToYangPath_dhcpv4_relay_agent_intf_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != dhcpv4RelayTable && params.tblName != VLAN_TN {
		return tlerr.InternalError{Format: "wrong config DB table sent"}
	}
	if len(params.tblKeyComp) == 0 {
		return tlerr.InternalError{Format: "Missing DB key"}
	}
	params.ygPathKeys[ocRelayDhcpIntfPath+"/id"] = params.tblKeyComp[0]
	return nil
}
