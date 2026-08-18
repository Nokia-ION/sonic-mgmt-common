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
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	LLDP_ENTRY_TABLE       = "LLDP_ENTRY_TABLE"
	LLDP_LOC_CHASSIS_TABLE = "LLDP_LOC_CHASSIS"
	LLDP_CFG_TABLE         = "LLDP"
	LLDP_PORT_TABLE        = "LLDP_PORT"
	LLDP_GLOBAL_KEY        = "GLOBAL"

	LLDP_REMOTE_CAP_ENABLED      = "lldp_rem_sys_cap_enabled"
	LLDP_REMOTE_SYS_NAME         = "lldp_rem_sys_name"
	LLDP_REMOTE_PORT_DESC        = "lldp_rem_port_desc"
	LLDP_REMOTE_CHASS_ID         = "lldp_rem_chassis_id"
	LLDP_REMOTE_CAP_SUPPORTED    = "lldp_rem_sys_cap_supported"
	LLDP_REMOTE_PORT_ID_SUBTYPE  = "lldp_rem_port_id_subtype"
	LLDP_REMOTE_SYS_DESC         = "lldp_rem_sys_desc"
	LLDP_REMOTE_PORT_ID          = "lldp_rem_port_id"
	LLDP_REMOTE_REM_ID           = "lldp_rem_index"
	LLDP_REMOTE_REM_TIME         = "lldp_rem_time_mark"
	LLDP_REMOTE_CHASS_ID_SUBTYPE = "lldp_rem_chassis_id_subtype"
	LLDP_REMOTE_MAN_ADDR         = "lldp_rem_man_addr"

	LLDP_LOC_CHASS_ID         = "lldp_loc_chassis_id"
	LLDP_LOC_CHASS_ID_SUBTYPE = "lldp_loc_chassis_id_subtype"
	LLDP_LOC_SYS_NAME         = "lldp_loc_sys_name"
	LLDP_LOC_SYS_DESC         = "lldp_loc_sys_desc"
	LLDP_LOC_MAN_ADDR         = "lldp_loc_man_addr"

	LLDP_CFG_ENABLED            = "enabled"
	LLDP_CFG_HELLO_TIME         = "hello_time"
	LLDP_CFG_SYSTEM_NAME        = "system_name"
	LLDP_CFG_SYSTEM_DESC        = "system_description"
	LLDP_CFG_SUPP_MGMT_ADDR_TLV = "supp_mgmt_address_tlv"
	LLDP_CFG_SUPP_SYS_CAP_TLV   = "supp_system_capabilities_tlv"

	LLDP_CONFIG     = "/openconfig-lldp:lldp/config"
	LLDP_STATE      = "/openconfig-lldp:lldp/state"
	LLDP_INTERFACES = "/openconfig-lldp:lldp/interfaces"
	LLDP_INTERFACE  = "/openconfig-lldp:lldp/interfaces/interface"

	lldpDefaultEnabled    = true
	lldpDefaultHelloTimer = uint64(30)
)

type lldpCapInfo struct {
	name    ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY
	keyName string
	bitMask uint8
}

// Bit positions follow IEEE 802.1AB / SONiC lldp_syncd (128 >> bit_index).
var lldpCapabilityBits = []lldpCapInfo{
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_OTHER, keyName: "OTHER", bitMask: 128 >> 0},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_REPEATER, keyName: "REPEATER", bitMask: 128 >> 1},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_MAC_BRIDGE, keyName: "MAC_BRIDGE", bitMask: 128 >> 2},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_WLAN_ACCESS_POINT, keyName: "WLAN_ACCESS_POINT", bitMask: 128 >> 3},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_ROUTER, keyName: "ROUTER", bitMask: 128 >> 4},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_TELEPHONE, keyName: "TELEPHONE", bitMask: 128 >> 5},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_DOCSIS_CABLE_DEVICE, keyName: "DOCSIS_CABLE_DEVICE", bitMask: 128 >> 6},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_STATION_ONLY, keyName: "STATION_ONLY", bitMask: 128 >> 7},
}

func init() {
	XlateFuncBind("DbToYang_lldp_config_xfmr", DbToYang_lldp_config_xfmr)
	XlateFuncBind("YangToDb_lldp_config_xfmr", YangToDb_lldp_config_xfmr)
	XlateFuncBind("Subscribe_lldp_config_xfmr", Subscribe_lldp_config_xfmr)

	XlateFuncBind("DbToYang_lldp_state_xfmr", DbToYang_lldp_state_xfmr)
	XlateFuncBind("YangToDb_lldp_state_xfmr", YangToDb_lldp_state_xfmr)
	XlateFuncBind("Subscribe_lldp_state_xfmr", Subscribe_lldp_state_xfmr)

	XlateFuncBind("DbToYang_lldp_interfaces_xfmr", DbToYang_lldp_interfaces_xfmr)
	XlateFuncBind("YangToDb_lldp_interfaces_xfmr", YangToDb_lldp_interfaces_xfmr)
	XlateFuncBind("Subscribe_lldp_interfaces_xfmr", Subscribe_lldp_interfaces_xfmr)

	XlateFuncBind("DbToYang_lldp_system_name_fld_xfmr", DbToYang_lldp_system_name_fld_xfmr)
	XlateFuncBind("DbToYang_lldp_system_description_fld_xfmr", DbToYang_lldp_system_description_fld_xfmr)
	XlateFuncBind("DbToYang_lldp_chassis_id_fld_xfmr", DbToYang_lldp_chassis_id_fld_xfmr)
	XlateFuncBind("DbToYang_lldp_chassis_id_type_fld_xfmr", DbToYang_lldp_chassis_id_type_fld_xfmr)
}

func getLldpRoot(s *ygot.GoStruct) *ocbinds.OpenconfigLldp_Lldp {
	deviceObj := (*s).(*ocbinds.Device)
	return deviceObj.Lldp
}

func getLldpGlobalConfigEntry(cfgDb *db.DB) (db.Value, error) {
	if cfgDb == nil {
		return db.Value{}, errors.New("CONFIG_DB handle is not available")
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: LLDP_CFG_TABLE}, db.Key{Comp: []string{LLDP_GLOBAL_KEY}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return db.Value{}, err
	}
	return entry, nil
}

func getLldpLocChassisEntry(appDb *db.DB) (db.Value, error) {
	if appDb == nil {
		return db.Value{}, errors.New("APPL_DB handle is not available")
	}
	ts := &db.TableSpec{Name: LLDP_LOC_CHASSIS_TABLE}
	keys, err := appDb.GetKeys(ts)
	if err == nil && len(keys) > 0 {
		return appDb.GetEntry(ts, keys[0])
	}
	entry, err := appDb.GetEntry(ts, db.Key{Comp: []string{}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return db.Value{}, err
	}
	return entry, nil
}

func getTableKeys(d *db.DB, table string) ([]string, error) {
	if d == nil {
		return nil, nil
	}
	tbl, err := d.GetTable(&db.TableSpec{Name: table})
	if err != nil {
		return nil, err
	}
	keys, err := tbl.GetKeys()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key.Comp) > 0 {
			names = append(names, key.Get(0))
		}
	}
	return names, nil
}

func getLldpPortEntry(cfgDb *db.DB, ifName string) (db.Value, error) {
	if cfgDb == nil {
		return db.Value{}, errors.New("CONFIG_DB handle is not available")
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: LLDP_PORT_TABLE}, db.Key{Comp: []string{ifName}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return db.Value{}, err
	}
	return entry, nil
}

func parseBoolString(val string, defaultVal bool) *bool {
	if val == "" {
		return boolPtr(defaultVal)
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return boolPtr(defaultVal)
	}
	return boolPtr(parsed)
}

func parseUint64String(val string) *uint64 {
	if val == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseChassisIdType(val string) ocbinds.E_OpenconfigLldp_ChassisIdType {
	if val == "" {
		return ocbinds.OpenconfigLldp_ChassisIdType_UNSET
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return ocbinds.OpenconfigLldp_ChassisIdType_UNSET
	}
	return ocbinds.E_OpenconfigLldp_ChassisIdType(parsed)
}

func parsePortIdType(val string) ocbinds.E_OpenconfigLldp_PortIdType {
	if val == "" {
		return ocbinds.OpenconfigLldp_PortIdType_UNSET
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return ocbinds.OpenconfigLldp_PortIdType_UNSET
	}
	return ocbinds.E_OpenconfigLldp_PortIdType(parsed)
}

func inferMgmtAddrType(addr string) string {
	if addr == "" {
		return ""
	}
	first := strings.Split(addr, ",")[0]
	if strings.Contains(first, ":") {
		return "ipv6"
	}
	if strings.Contains(first, ".") {
		return "ipv4"
	}
	return ""
}

func suppressTlvsFromConfig(globalEntry db.Value) []ocbinds.E_OpenconfigLldpTypes_LLDP_TLV {
	var tlvList []ocbinds.E_OpenconfigLldpTypes_LLDP_TLV
	if enabled := parseBoolString(globalEntry.Get(LLDP_CFG_SUPP_MGMT_ADDR_TLV), false); enabled != nil && *enabled {
		tlvList = append(tlvList, ocbinds.OpenconfigLldpTypes_LLDP_TLV_MANAGEMENT_ADDRESS)
	}
	if enabled := parseBoolString(globalEntry.Get(LLDP_CFG_SUPP_SYS_CAP_TLV), false); enabled != nil && *enabled {
		tlvList = append(tlvList, ocbinds.OpenconfigLldpTypes_LLDP_TLV_SYSTEM_CAPABILITIES)
	}
	return tlvList
}

func suppressTlvsToConfig(tlvs []ocbinds.E_OpenconfigLldpTypes_LLDP_TLV) map[string]string {
	fields := map[string]string{
		LLDP_CFG_SUPP_MGMT_ADDR_TLV: "false",
		LLDP_CFG_SUPP_SYS_CAP_TLV:   "false",
	}
	for _, tlv := range tlvs {
		switch tlv {
		case ocbinds.OpenconfigLldpTypes_LLDP_TLV_MANAGEMENT_ADDRESS:
			fields[LLDP_CFG_SUPP_MGMT_ADDR_TLV] = "true"
		case ocbinds.OpenconfigLldpTypes_LLDP_TLV_SYSTEM_CAPABILITIES:
			fields[LLDP_CFG_SUPP_SYS_CAP_TLV] = "true"
		}
	}
	return fields
}

func lldpStringPtrUnset(s *string) bool {
	return s == nil || *s == ""
}

func fillLldpGlobalEnabled(enabled **bool, globalEntry db.Value) {
	if globalEntry.IsPopulated() {
		if val := globalEntry.Get(LLDP_CFG_ENABLED); val != "" {
			*enabled = parseBoolString(val, lldpDefaultEnabled)
			return
		}
	}
	*enabled = boolPtr(lldpDefaultEnabled)
}

func fillLldpGlobalHelloTimer(helloTimer **uint64, globalEntry db.Value) {
	if globalEntry.IsPopulated() {
		if val := globalEntry.Get(LLDP_CFG_HELLO_TIME); val != "" {
			*helloTimer = parseUint64String(val)
			return
		}
	}
	*helloTimer = uint64Ptr(lldpDefaultHelloTimer)
}

func fillGlobalConfigLeaves(cfg *ocbinds.OpenconfigLldp_Lldp_Config, globalEntry db.Value, locChassis db.Value) {
	if cfg == nil {
		return
	}
	fillLldpGlobalEnabled(&cfg.Enabled, globalEntry)
	fillLldpGlobalHelloTimer(&cfg.HelloTimer, globalEntry)
	if globalEntry.IsPopulated() {
		if val := globalEntry.Get(LLDP_CFG_SYSTEM_NAME); val != "" {
			cfg.SystemName = &val
		}
		if val := globalEntry.Get(LLDP_CFG_SYSTEM_DESC); val != "" {
			cfg.SystemDescription = &val
		}
		cfg.SuppressTlvAdvertisement = suppressTlvsFromConfig(globalEntry)
	}
	if locChassis.IsPopulated() {
		if val := locChassis.Get(LLDP_LOC_CHASS_ID); val != "" {
			cfg.ChassisId = &val
		}
		if subtype := locChassis.Get(LLDP_LOC_CHASS_ID_SUBTYPE); subtype != "" {
			cfg.ChassisIdType = parseChassisIdType(subtype)
		}
		if lldpStringPtrUnset(cfg.SystemName) {
			if val := locChassis.Get(LLDP_LOC_SYS_NAME); val != "" {
				cfg.SystemName = &val
			}
		}
		if lldpStringPtrUnset(cfg.SystemDescription) {
			if val := locChassis.Get(LLDP_LOC_SYS_DESC); val != "" {
				cfg.SystemDescription = &val
			}
		}
	}
}

func fillGlobalStateLeaves(state *ocbinds.OpenconfigLldp_Lldp_State, globalEntry db.Value, locChassis db.Value, includeCounters bool) {
	if state == nil {
		return
	}
	fillLldpGlobalEnabled(&state.Enabled, globalEntry)
	fillLldpGlobalHelloTimer(&state.HelloTimer, globalEntry)
	if globalEntry.IsPopulated() {
		if val := globalEntry.Get(LLDP_CFG_SYSTEM_NAME); val != "" {
			state.SystemName = &val
		}
		if val := globalEntry.Get(LLDP_CFG_SYSTEM_DESC); val != "" {
			state.SystemDescription = &val
		}
		state.SuppressTlvAdvertisement = suppressTlvsFromConfig(globalEntry)
	}
	if locChassis.IsPopulated() {
		if val := locChassis.Get(LLDP_LOC_CHASS_ID); val != "" {
			state.ChassisId = &val
		}
		if subtype := locChassis.Get(LLDP_LOC_CHASS_ID_SUBTYPE); subtype != "" {
			state.ChassisIdType = parseChassisIdType(subtype)
		}
		if lldpStringPtrUnset(state.SystemName) {
			if val := locChassis.Get(LLDP_LOC_SYS_NAME); val != "" {
				state.SystemName = &val
			}
		}
		if lldpStringPtrUnset(state.SystemDescription) {
			if val := locChassis.Get(LLDP_LOC_SYS_DESC); val != "" {
				state.SystemDescription = &val
			}
		}
	}
	if includeCounters && state.Counters == nil {
		state.Counters = &ocbinds.OpenconfigLldp_Lldp_State_Counters{}
	}
}

func lldpGlobalDbEntries(inParams XfmrParams) (db.Value, db.Value, error) {
	globalEntry, err := getLldpGlobalConfigEntry(inParams.dbs[db.ConfigDB])
	if err != nil {
		return db.Value{}, db.Value{}, err
	}
	locChassis, err := getLldpLocChassisEntry(inParams.dbs[db.ApplDB])
	if err != nil {
		return globalEntry, db.Value{}, err
	}
	return globalEntry, locChassis, nil
}

func lldpSystemNameFromDb(globalEntry, locChassis db.Value) (string, bool) {
	if globalEntry.IsPopulated() {
		if val := globalEntry.Get(LLDP_CFG_SYSTEM_NAME); val != "" {
			return val, true
		}
	}
	if locChassis.IsPopulated() {
		if val := locChassis.Get(LLDP_LOC_SYS_NAME); val != "" {
			return val, true
		}
	}
	return "", false
}

func lldpSystemDescriptionFromDb(globalEntry, locChassis db.Value) (string, bool) {
	if globalEntry.IsPopulated() {
		if val := globalEntry.Get(LLDP_CFG_SYSTEM_DESC); val != "" {
			return val, true
		}
	}
	if locChassis.IsPopulated() {
		if val := locChassis.Get(LLDP_LOC_SYS_DESC); val != "" {
			return val, true
		}
	}
	return "", false
}

func lldpChassisIdTypeFromDb(locChassis db.Value) (interface{}, bool) {
	if !locChassis.IsPopulated() {
		return nil, false
	}
	subtype := locChassis.Get(LLDP_LOC_CHASS_ID_SUBTYPE)
	if subtype == "" {
		return nil, false
	}
	enumVal := parseChassisIdType(subtype)
	if enumVal == ocbinds.OpenconfigLldp_ChassisIdType_UNSET {
		return nil, false
	}
	name, err := ygot.EnumName(enumVal)
	if err != nil {
		return nil, false
	}
	return name, true
}

var DbToYang_lldp_system_name_fld_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	globalEntry, locChassis, err := lldpGlobalDbEntries(inParams)
	if err != nil {
		return result, err
	}
	if val, ok := lldpSystemNameFromDb(globalEntry, locChassis); ok {
		result["system-name"] = val
	}
	return result, nil
}

var DbToYang_lldp_system_description_fld_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	globalEntry, locChassis, err := lldpGlobalDbEntries(inParams)
	if err != nil {
		return result, err
	}
	if val, ok := lldpSystemDescriptionFromDb(globalEntry, locChassis); ok {
		result["system-description"] = val
	}
	return result, nil
}

var DbToYang_lldp_chassis_id_fld_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	_, locChassis, err := lldpGlobalDbEntries(inParams)
	if err != nil {
		return result, err
	}
	if locChassis.IsPopulated() {
		if val := locChassis.Get(LLDP_LOC_CHASS_ID); val != "" {
			result["chassis-id"] = val
		}
	}
	return result, nil
}

var DbToYang_lldp_chassis_id_type_fld_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	_, locChassis, err := lldpGlobalDbEntries(inParams)
	if err != nil {
		return result, err
	}
	if val, ok := lldpChassisIdTypeFromDb(locChassis); ok {
		result["chassis-id-type"] = val
	}
	return result, nil
}

func fillInterfaceConfig(ifInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface, portEntry db.Value) {
	if ifInfo == nil || ifInfo.Name == nil {
		return
	}
	ygot.BuildEmptyTree(ifInfo)
	if ifInfo.Config == nil {
		ifInfo.Config = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Config{}
	}
	ifInfo.Config.Name = ifInfo.Name
	defaultEnabled := true
	if portEntry.IsPopulated() {
		ifInfo.Config.Enabled = parseBoolString(portEntry.Get(LLDP_CFG_ENABLED), defaultEnabled)
	} else {
		ifInfo.Config.Enabled = boolPtr(defaultEnabled)
	}
}

func fillInterfaceState(ifInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface, portEntry db.Value, includeCounters bool) {
	if ifInfo == nil || ifInfo.Name == nil {
		return
	}
	ygot.BuildEmptyTree(ifInfo)
	if ifInfo.State == nil {
		ifInfo.State = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_State{}
	}
	ifInfo.State.Name = ifInfo.Name
	defaultEnabled := true
	if portEntry.IsPopulated() {
		ifInfo.State.Enabled = parseBoolString(portEntry.Get(LLDP_CFG_ENABLED), defaultEnabled)
	} else {
		ifInfo.State.Enabled = boolPtr(defaultEnabled)
	}
	if includeCounters && ifInfo.State.Counters == nil {
		ifInfo.State.Counters = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_State_Counters{}
	}
}

func isLldpInterfacePath(path string) bool {
	return path == LLDP_INTERFACES || path == LLDP_INTERFACE || strings.HasPrefix(path, LLDP_INTERFACE+"/")
}

func pathNeedsCounters(path string) bool {
	return strings.Contains(path, "/counters")
}

func shouldMapLldpGlobalConfigLeaf(targetUriPath, leaf string) bool {
	if targetUriPath == LLDP_CONFIG || targetUriPath == "/openconfig-lldp:lldp" {
		return true
	}
	return targetUriPath == LLDP_CONFIG+"/"+leaf
}

type lldpGlobalConfigSnapshot struct {
	enabled         *bool
	helloTimer      *uint64
	systemName      *string
	systemDesc      *string
	suppressTlvs    []ocbinds.E_OpenconfigLldpTypes_LLDP_TLV
	suppressTlvsSet bool
}

func snapshotLldpGlobalConfig(cfg *ocbinds.OpenconfigLldp_Lldp_Config) lldpGlobalConfigSnapshot {
	if cfg == nil {
		return lldpGlobalConfigSnapshot{}
	}
	snap := lldpGlobalConfigSnapshot{
		enabled:    cfg.Enabled,
		helloTimer: cfg.HelloTimer,
		systemName: cfg.SystemName,
		systemDesc: cfg.SystemDescription,
	}
	if cfg.SuppressTlvAdvertisement != nil {
		snap.suppressTlvsSet = true
		snap.suppressTlvs = append([]ocbinds.E_OpenconfigLldpTypes_LLDP_TLV(nil), cfg.SuppressTlvAdvertisement...)
	}
	return snap
}

// --- Global config transformer ---

var YangToDb_lldp_config_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	device := (*inParams.ygRoot).(*ocbinds.Device)
	var cfgSnap lldpGlobalConfigSnapshot
	if device.Lldp != nil && device.Lldp.Config != nil {
		cfgSnap = snapshotLldpGlobalConfig(device.Lldp.Config)
	}
	if device.Lldp == nil {
		ygot.BuildEmptyTree(device)
	}
	lldpObj := device.Lldp
	ygot.BuildEmptyTree(lldpObj)
	if lldpObj.Config == nil {
		lldpObj.Config = &ocbinds.OpenconfigLldp_Lldp_Config{}
	}
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	globalMap := map[string]db.Value{
		LLDP_GLOBAL_KEY: {Field: map[string]string{}},
	}
	res := map[string]map[string]db.Value{
		LLDP_CFG_TABLE: globalMap,
	}

	if inParams.oper == DELETE {
		switch targetUriPath {
		case LLDP_CONFIG + "/enabled":
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_ENABLED] = ""
		case LLDP_CONFIG + "/hello-timer":
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_HELLO_TIME] = ""
		case LLDP_CONFIG + "/system-name":
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_SYSTEM_NAME] = ""
		case LLDP_CONFIG + "/system-description":
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_SYSTEM_DESC] = ""
		case LLDP_CONFIG + "/suppress-tlv-advertisement":
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_SUPP_MGMT_ADDR_TLV] = ""
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_SUPP_SYS_CAP_TLV] = ""
		case LLDP_CONFIG:
			return res, nil
		default:
			return nil, tlerr.NotSupported("DELETE not supported on " + targetUriPath)
		}
		return res, nil
	}

	if shouldMapLldpGlobalConfigLeaf(targetUriPath, "enabled") && cfgSnap.enabled != nil {
		globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_ENABLED] = strconv.FormatBool(*cfgSnap.enabled)
	}
	if shouldMapLldpGlobalConfigLeaf(targetUriPath, "hello-timer") && cfgSnap.helloTimer != nil {
		globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_HELLO_TIME] = strconv.FormatUint(*cfgSnap.helloTimer, 10)
	}
	if shouldMapLldpGlobalConfigLeaf(targetUriPath, "system-name") && cfgSnap.systemName != nil {
		globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_SYSTEM_NAME] = *cfgSnap.systemName
	}
	if shouldMapLldpGlobalConfigLeaf(targetUriPath, "system-description") && cfgSnap.systemDesc != nil {
		globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_SYSTEM_DESC] = *cfgSnap.systemDesc
	}
	if shouldMapLldpGlobalConfigLeaf(targetUriPath, "suppress-tlv-advertisement") && cfgSnap.suppressTlvsSet {
		for k, v := range suppressTlvsToConfig(cfgSnap.suppressTlvs) {
			globalMap[LLDP_GLOBAL_KEY].Field[k] = v
		}
	}
	return res, nil
}

var DbToYang_lldp_config_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	lldpObj := getLldpRoot(inParams.ygRoot)
	ygot.BuildEmptyTree(lldpObj)
	if lldpObj.Config == nil {
		lldpObj.Config = &ocbinds.OpenconfigLldp_Lldp_Config{}
	}
	ygot.BuildEmptyTree(lldpObj.Config)

	globalEntry, _ := getLldpGlobalConfigEntry(inParams.dbs[db.ConfigDB])
	locChassis, _ := getLldpLocChassisEntry(inParams.dbs[db.ApplDB])
	fillGlobalConfigLeaves(lldpObj.Config, globalEntry, locChassis)
	return nil
}

var Subscribe_lldp_config_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = false
	} else {
		fillLldpGlobalSubscribe(&result)
	}
	return result, nil
}

// --- Global state transformer ---

var YangToDb_lldp_state_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	return nil, tlerr.NotSupported("LLDP operational state is read-only")
}

var DbToYang_lldp_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return err
	}

	lldpObj := getLldpRoot(inParams.ygRoot)
	ygot.BuildEmptyTree(lldpObj)
	if lldpObj.State == nil {
		lldpObj.State = &ocbinds.OpenconfigLldp_Lldp_State{}
	}
	ygot.BuildEmptyTree(lldpObj.State)

	globalEntry, _ := getLldpGlobalConfigEntry(inParams.dbs[db.ConfigDB])
	locChassis, _ := getLldpLocChassisEntry(inParams.dbs[db.ApplDB])
	fillGlobalStateLeaves(lldpObj.State, globalEntry, locChassis, pathNeedsCounters(targetUriPath) || targetUriPath == LLDP_STATE)
	return nil
}

var Subscribe_lldp_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = false
	} else {
		fillLldpGlobalSubscribe(&result)
	}
	return result, nil
}

// fillLldpGlobalSubscribe maps both CONFIG_DB LLDP|GLOBAL and the APPL_DB
// LLDP_LOC_CHASSIS singleton. Chassis-id / system-name / system-description
// fall back to loc chassis, so subscribe (ONCE/SAMPLE/ON_CHANGE) must watch
// that hash. The empty key is required: Redis stores it as the table name
// with no key component.
func fillLldpGlobalSubscribe(result *XfmrSubscOutParams) {
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		LLDP_CFG_TABLE: {LLDP_GLOBAL_KEY: {}},
	}
	result.dbDataMap[db.ApplDB] = map[string]map[string]map[string]string{
		LLDP_LOC_CHASSIS_TABLE: {"": {}},
	}
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	result.isVirtualTbl = false
}

// --- Interfaces transformer ---

var YangToDb_lldp_interfaces_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	lldpObj := getLldpRoot(inParams.ygRoot)
	if lldpObj.Interfaces == nil {
		return nil, nil
	}
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	ifName := NewPathInfo(inParams.uri).Var("name")
	portMap := make(map[string]db.Value)
	res := map[string]map[string]db.Value{LLDP_PORT_TABLE: portMap}

	if inParams.oper == DELETE {
		switch {
		case targetUriPath == LLDP_INTERFACE+"/config/enabled" && ifName != "":
			portMap[ifName] = db.Value{Field: map[string]string{LLDP_CFG_ENABLED: ""}}
		case (targetUriPath == LLDP_INTERFACE+"/config" || targetUriPath == LLDP_INTERFACE) && ifName != "":
			portMap[ifName] = db.Value{}
		case strings.HasPrefix(targetUriPath, LLDP_INTERFACE+"/neighbors"):
			return nil, tlerr.NotSupported("LLDP neighbor data is read-only")
		default:
			return nil, tlerr.NotSupported("DELETE not supported on " + targetUriPath)
		}
		return res, nil
	}

	if strings.Contains(targetUriPath, "/neighbors") {
		return nil, tlerr.NotSupported("LLDP neighbor data is read-only")
	}

	for name, ifInfo := range lldpObj.Interfaces.Interface {
		if ifName != "" && name != ifName {
			continue
		}
		if ifInfo == nil || ifInfo.Config == nil || ifInfo.Config.Enabled == nil {
			continue
		}
		portMap[name] = db.Value{
			Field: map[string]string{
				LLDP_CFG_ENABLED: strconv.FormatBool(*ifInfo.Config.Enabled),
			},
		}
	}
	return res, nil
}

var DbToYang_lldp_interfaces_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return err
	}
	if !isLldpInterfacePath(targetUriPath) {
		return nil
	}

	ifName := pathInfo.Var("name")
	neighId := pathInfo.Var("id")
	capKey := lldpCapabilityKeyFromPath(pathInfo)
	appDb := inParams.dbs[db.ApplDB]
	cfgDb := inParams.dbs[db.ConfigDB]

	lldpObj := getLldpRoot(inParams.ygRoot)
	ygot.BuildEmptyTree(lldpObj)
	return populateLldpInterfaces(lldpObj, appDb, cfgDb, ifName, neighId, capKey, targetUriPath)
}

var Subscribe_lldp_interfaces_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)

	pathInfo := NewPathInfo(inParams.uri)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return result, err
	}

	ifName := pathInfo.Var("name")
	if ifName == "" {
		ifName = "*"
	}

	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = true
		return result, nil
	}

	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}

	// Path-based subscribe / parent-table mapping (same pattern as lacp_intfs_xfmr).
	// Do not register both LLDP_ENTRY_TABLE and LLDP_PORT for keyed existence checks:
	// verifyParentTblSubtree requires every returned table/key to exist, but interface
	// data is a union of the two tables and LLDP_PORT is optional for config/state.
	switch {
	case strings.Contains(targetUriPath, "/neighbors"):
		result.dbDataMap[db.ApplDB] = map[string]map[string]map[string]string{
			LLDP_ENTRY_TABLE: {ifName: {}},
		}
	case strings.Contains(targetUriPath, "/config"):
		result.isVirtualTbl = true
	case strings.HasPrefix(targetUriPath, LLDP_INTERFACE+"/state"):
		result.isVirtualTbl = true
	case targetUriPath == LLDP_INTERFACES || targetUriPath == LLDP_INTERFACE:
		if ifName == "*" {
			result.dbDataMap[db.ApplDB] = map[string]map[string]map[string]string{
				LLDP_ENTRY_TABLE: {ifName: {}},
			}
			result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
				LLDP_PORT_TABLE: {ifName: {}},
			}
		} else {
			// Keyed interface container: neighbor and/or port config (DbToYang union).
			result.isVirtualTbl = true
		}
	default:
		if strings.HasPrefix(targetUriPath, LLDP_INTERFACE+"/") {
			result.isVirtualTbl = true
		}
	}

	return result, nil
}

func collectLldpInterfaceNames(appDb, cfgDb *db.DB, ifName string) []string {
	nameSet := make(map[string]struct{})
	addName := func(name string) {
		if name == "" {
			return
		}
		if ifName != "" && name != ifName {
			return
		}
		nameSet[name] = struct{}{}
	}

	if appDb != nil {
		if keys, err := getTableKeys(appDb, LLDP_ENTRY_TABLE); err == nil {
			for _, name := range keys {
				addName(name)
			}
		}
	}
	if cfgDb != nil {
		if keys, err := getTableKeys(cfgDb, LLDP_PORT_TABLE); err == nil {
			for _, name := range keys {
				addName(name)
			}
		}
	}
	if ifName != "" {
		if _, ok := nameSet[ifName]; !ok {
			nameSet[ifName] = struct{}{}
		}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	return names
}

// lldpCapabilityKeyFromPath returns the capability list key when the GET path
// includes capability[name=…] (second "name" key in the URI becomes name#2).
func lldpCapabilityKeyFromPath(pathInfo *PathInfo) string {
	if pathInfo.HasVar("name#2") {
		return pathInfo.Var("name#2")
	}
	return ""
}

func parseLldpCapabilityKey(key string) (ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY, bool) {
	if key == "" {
		return ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_UNSET, false
	}
	key = strings.TrimPrefix(key, "openconfig-lldp-types:")
	if idx := strings.LastIndex(key, ":"); idx >= 0 {
		key = key[idx+1:]
	}
	for _, cap := range lldpCapabilityBits {
		if cap.keyName == key {
			return cap.name, true
		}
	}
	return ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_UNSET, false
}

func getOrCreateLldpInterface(interfaces *ocbinds.OpenconfigLldp_Lldp_Interfaces, ifNameKey string) (*ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface, error) {
	ifInfo, ok := interfaces.Interface[ifNameKey]
	if !ok {
		var err error
		ifInfo, err = interfaces.NewInterface(ifNameKey)
		if err != nil {
			return nil, err
		}
	}
	ygot.BuildEmptyTree(ifInfo)
	return ifInfo, nil
}

func getOrCreateLldpNeighbor(ifInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface, neighKey string) (*ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor, error) {
	if ifInfo.Neighbors == nil {
		ifInfo.Neighbors = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors{}
	}
	ygot.BuildEmptyTree(ifInfo.Neighbors)
	ngInfo, ok := ifInfo.Neighbors.Neighbor[neighKey]
	if !ok {
		var err error
		ngInfo, err = ifInfo.Neighbors.NewNeighbor(neighKey)
		if err != nil {
			return nil, err
		}
	}
	ygot.BuildEmptyTree(ngInfo)
	return ngInfo, nil
}

func getOrCreateLldpCapability(ngInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor, capName ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY) (*ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_Capabilities_Capability, error) {
	if ngInfo.Capabilities == nil {
		ngInfo.Capabilities = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_Capabilities{}
	}
	ygot.BuildEmptyTree(ngInfo.Capabilities)
	capInfo, ok := ngInfo.Capabilities.Capability[capName]
	if !ok {
		var err error
		capInfo, err = ngInfo.Capabilities.NewCapability(capName)
		if err != nil {
			return nil, err
		}
	}
	ygot.BuildEmptyTree(capInfo)
	if capInfo.State == nil {
		capInfo.State = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_Capabilities_Capability_State{}
	}
	return capInfo, nil
}

func populateLldpInterfaces(lldpObj *ocbinds.OpenconfigLldp_Lldp, appDb, cfgDb *db.DB, ifName, neighId, capKey, targetUriPath string) error {
	if lldpObj.Interfaces == nil {
		lldpObj.Interfaces = &ocbinds.OpenconfigLldp_Lldp_Interfaces{}
	}
	ygot.BuildEmptyTree(lldpObj.Interfaces)

	includeCounters := pathNeedsCounters(targetUriPath)
	neighOnly := strings.Contains(targetUriPath, "/neighbors")
	skipNeighbors := strings.Contains(targetUriPath, "/config") ||
		(strings.Contains(targetUriPath, "/state") && !strings.Contains(targetUriPath, "/neighbors"))

	for _, ifNameKey := range collectLldpInterfaceNames(appDb, cfgDb, ifName) {
		ifInfo, err := getOrCreateLldpInterface(lldpObj.Interfaces, ifNameKey)
		if err != nil {
			return err
		}

		portEntry, _ := getLldpPortEntry(cfgDb, ifNameKey)
		if !neighOnly {
			fillInterfaceConfig(ifInfo, portEntry)
			fillInterfaceState(ifInfo, portEntry, includeCounters)
		}

		if skipNeighbors {
			continue
		}

		entry, err := getLldpNeighborEntry(appDb, ifNameKey)
		if err != nil || !entry.IsPopulated() {
			continue
		}
		neighKey := ifNameKey
		if neighId != "" {
			if neighId != ifNameKey {
				continue
			}
			neighKey = neighId
		}
		if err := populateLldpNeighbor(ifInfo, neighKey, entry, capKey, targetUriPath); err != nil {
			return err
		}
	}
	return nil
}

func getLldpNeighborEntry(appDb *db.DB, ifName string) (db.Value, error) {
	if appDb == nil {
		return db.Value{}, errors.New("APPL_DB handle is not available")
	}
	return appDb.GetEntry(&db.TableSpec{Name: LLDP_ENTRY_TABLE}, db.Key{Comp: []string{ifName}})
}

func populateLldpNeighbor(ifInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface, neighKey string, entry db.Value, capKey, targetUriPath string) error {
	ngInfo, err := getOrCreateLldpNeighbor(ifInfo, neighKey)
	if err != nil {
		log.V(lvl.DEBUG).Info("Failed to create LLDP neighbor subtree: ", err)
		return err
	}
	if ngInfo.State == nil {
		ngInfo.State = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_State{}
	}

	if val := entry.Get(LLDP_REMOTE_SYS_NAME); val != "" {
		ngInfo.State.SystemName = &val
	}
	if val := entry.Get(LLDP_REMOTE_PORT_DESC); val != "" {
		ngInfo.State.PortDescription = &val
	}
	if val := entry.Get(LLDP_REMOTE_CHASS_ID); val != "" {
		ngInfo.State.ChassisId = &val
	}
	if val := entry.Get(LLDP_REMOTE_SYS_DESC); val != "" {
		ngInfo.State.SystemDescription = &val
	}
	if val := entry.Get(LLDP_REMOTE_PORT_ID); val != "" {
		ngInfo.State.PortId = &val
	}
	if val := entry.Get(LLDP_REMOTE_REM_ID); val != "" {
		ngInfo.State.Id = &val
	}
	if val := entry.Get(LLDP_REMOTE_MAN_ADDR); val != "" {
		ngInfo.State.ManagementAddress = &val
		if addrType := inferMgmtAddrType(val); addrType != "" {
			ngInfo.State.ManagementAddressType = &addrType
		}
	}
	if val := entry.Get(LLDP_REMOTE_REM_TIME); val != "" {
		ngInfo.State.Age = parseUint64String(val)
	}
	if val := entry.Get(LLDP_REMOTE_PORT_ID_SUBTYPE); val != "" {
		ngInfo.State.PortIdType = parsePortIdType(val)
	}
	if val := entry.Get(LLDP_REMOTE_CHASS_ID_SUBTYPE); val != "" {
		ngInfo.State.ChassisIdType = parseChassisIdType(val)
	}

	skipCapabilities := strings.Contains(targetUriPath, "/state") && !strings.Contains(targetUriPath, "/capabilities")
	if !skipCapabilities {
		return populateLldpCapabilities(ngInfo, entry, capKey)
	}
	return nil
}

func populateLldpCapabilities(ngInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor, entry db.Value, capKey string) error {
	capStates := make(map[ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY]bool)
	parseLldpCapBitmap(entry.Get(LLDP_REMOTE_CAP_SUPPORTED), capStates, false)
	parseLldpCapBitmap(entry.Get(LLDP_REMOTE_CAP_ENABLED), capStates, true)

	if capKey != "" {
		capEnum, ok := parseLldpCapabilityKey(capKey)
		if !ok {
			return nil
		}
		enabled, exists := capStates[capEnum]
		if !exists {
			return nil
		}
		capStates = map[ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY]bool{capEnum: enabled}
	}

	for capName, enabled := range capStates {
		capInfo, err := getOrCreateLldpCapability(ngInfo, capName)
		if err != nil {
			continue
		}
		capInfo.State.Name = capName
		capInfo.State.Enabled = boolPtr(enabled)
	}
	return nil
}

func parseLldpCapBitmap(capb string, capStates map[ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY]bool, setEnabled bool) {
	if capb == "" {
		return
	}
	numStr := strings.Split(capb, " ")
	if len(numStr) < 2 {
		return
	}
	raw, err := hex.DecodeString(numStr[0] + numStr[1])
	if err != nil || len(raw) < 2 {
		return
	}
	sysCap := raw[0] | raw[1]

	for _, cap := range lldpCapabilityBits {
		if sysCap&cap.bitMask == 0 {
			continue
		}
		if _, ok := capStates[cap.name]; !ok {
			capStates[cap.name] = false
		}
		if setEnabled {
			capStates[cap.name] = true
		}
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}
