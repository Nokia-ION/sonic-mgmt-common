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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
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
	LLDP_REMOTE_TTL              = "lldp_rem_ttl"
	LLDP_REMOTE_MAX_FRAME_SIZE   = "lldp_rem_max_frame_size"
	LLDP_REMOTE_PORT_VLAN_ID     = "lldp_rem_port_vlan_id"
	LLDP_REMOTE_CUSTOM_TLVS      = "lldp_rem_custom_tlvs"
	LLDP_REMOTE_MED_INV_SERIAL   = "lldp_rem_med_inv_serial"
	LLDP_REMOTE_AGG_PORT_ID      = "lldp_rem_agg_port_id"

	LLDP_LOC_CHASS_ID         = "lldp_loc_chassis_id"
	LLDP_LOC_CHASS_ID_SUBTYPE = "lldp_loc_chassis_id_subtype"
	LLDP_LOC_SYS_NAME         = "lldp_loc_sys_name"
	LLDP_LOC_SYS_DESC         = "lldp_loc_sys_desc"
	LLDP_LOC_MAN_ADDR         = "lldp_loc_man_addr"
	LLDP_LOC_TTL              = "lldp_loc_ttl"

	LLDP_STATISTICS_TABLE      = "LLDP_STATISTICS"
	LLDP_STATISTICS_GLOBAL_KEY = "GLOBAL"
	LLDP_STAT_FRAME_IN         = "frame_in"
	LLDP_STAT_FRAME_OUT        = "frame_out"
	LLDP_STAT_FRAME_DISCARD    = "frame_discard"
	LLDP_STAT_TLV_UNKNOWN      = "tlv_unknown"
	LLDP_STAT_TLV_ACCEPTED     = "tlv_accepted"
	LLDP_STAT_ENTRIES_AGED_OUT = "entries_aged_out"

	lldpOrgSpecificTlvType = int32(127)

	LLDP_CFG_ENABLED            = "enabled"
	LLDP_CFG_HELLO_TIME         = "hello_time"
	LLDP_CFG_SYSTEM_NAME        = "system_name"
	LLDP_CFG_SYSTEM_DESC        = "system_description"
	LLDP_CFG_SUPP_MGMT_ADDR_TLV = "supp_mgmt_address_tlv"
	LLDP_CFG_SUPP_SYS_CAP_TLV   = "supp_system_capabilities_tlv"
	LLDP_CFG_MGMT_INTERFACE     = "management_interface"

	LLDP_CUSTOM_TLV_TABLE   = "LLDP_CUSTOM_TLV"
	LLDP_CUSTOM_TLV_TYPE    = "type"
	LLDP_CUSTOM_TLV_OUI     = "oui"
	LLDP_CUSTOM_TLV_SUBTYPE = "oui_subtype"
	LLDP_CUSTOM_TLV_VALUE   = "value"

	LLDP_CONFIG         = "/openconfig-lldp:lldp/config"
	LLDP_STATE          = "/openconfig-lldp:lldp/state"
	LLDP_INTERFACES     = "/openconfig-lldp:lldp/interfaces"
	LLDP_INTERFACE      = "/openconfig-lldp:lldp/interfaces/interface"
	LLDP_MGMT_ADDRESSES = "/openconfig-lldp:lldp/mgmt-addresses"
	LLDP_CUSTOM_TLVS    = "/openconfig-lldp:lldp/custom-tlvs"
	LLDP_CUSTOM_TLV     = "/openconfig-lldp:lldp/custom-tlvs/tlv"

	lldpDefaultEnabled    = true
	lldpDefaultHelloTimer = uint64(30)
)

// OpenConfig management-interface is a single ifname, not an lldpd glob.
var lldpMgmtIfNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._/-]{0,31}$`)

// lldpYgotField looks up an exported ygot field by name so the transformer can
// reject or clear 1.2.0 leaves/containers without requiring those fields
// to exist in the currently generated ocbinds (they appear after the
// translib ygot rebuild).
func lldpYgotField(obj interface{}, name string) (reflect.Value, bool) {
	if obj == nil {
		return reflect.Value{}, false
	}
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return reflect.Value{}, false
	}
	return f, true
}

func lldpYgotFieldSet(obj interface{}, name string) bool {
	f, ok := lldpYgotField(obj, name)
	if !ok {
		return false
	}
	switch f.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface:
		return !f.IsNil()
	default:
		return !f.IsZero()
	}
}

func lldpYgotSetNil(obj interface{}, name string) {
	f, ok := lldpYgotField(obj, name)
	if !ok || !f.CanSet() {
		return
	}
	switch f.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface:
		f.Set(reflect.Zero(f.Type()))
	}
}

func lldpClearUnpopulatedRootContainers(lldpObj *ocbinds.OpenconfigLldp_Lldp) {
	if lldpObj == nil {
		return
	}
	if lldpObj.CustomTlvs != nil && len(lldpObj.CustomTlvs.Tlv) == 0 {
		lldpObj.CustomTlvs = nil
	}
	if lldpObj.MgmtAddresses != nil && len(lldpObj.MgmtAddresses.MgmtAddress) == 0 {
		lldpObj.MgmtAddresses = nil
	}
}

func lldpClearUnpopulatedNeighborContainers(ng interface{}) {
	lldpYgotSetNil(ng, "LinkAggregation")
	lldpYgotSetNil(ng, "MgmtAddresses")
}

func rejectUnsupportedLldpGlobalConfig(path string, cfg *ocbinds.OpenconfigLldp_Lldp_Config) error {
	switch path {
	case LLDP_CONFIG + "/ttl":
		return tlerr.NotSupported("LLDP configuration not supported on " + path)
	}
	if path == LLDP_CONFIG || path == "/openconfig-lldp:lldp" {
		if lldpYgotFieldSet(cfg, "Ttl") {
			return tlerr.NotSupported("LLDP configuration not supported on " + LLDP_CONFIG + "/ttl")
		}
	}
	return nil
}

func rejectUnsupportedLldpInterfaceConfig(path string, cfg interface{}) error {
	switch path {
	case LLDP_INTERFACE + "/config/port-description",
		LLDP_INTERFACE + "/config/custom-tlv-names":
		return tlerr.NotSupported("LLDP configuration not supported on " + path)
	}
	if path == LLDP_INTERFACE+"/config" || path == LLDP_INTERFACE || path == LLDP_INTERFACES {
		if lldpYgotFieldSet(cfg, "PortDescription") {
			return tlerr.NotSupported("LLDP configuration not supported on " + LLDP_INTERFACE + "/config/port-description")
		}
		if lldpYgotFieldSet(cfg, "CustomTlvNames") {
			return tlerr.NotSupported("LLDP configuration not supported on " + LLDP_INTERFACE + "/config/custom-tlv-names")
		}
	}
	return nil
}

type lldpCapInfo struct {
	name    ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY
	keyName string
	bitMask uint16
}

// Bit positions follow IEEE 802.1AB / SNMP LldpSystemCapabilitiesMap:
// bit 0 is the MSB of the first octet (0x8000 in the 16-bit word).
var lldpCapabilityBits = []lldpCapInfo{
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_OTHER, keyName: "OTHER", bitMask: 0x8000 >> 0},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_REPEATER, keyName: "REPEATER", bitMask: 0x8000 >> 1},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_MAC_BRIDGE, keyName: "MAC_BRIDGE", bitMask: 0x8000 >> 2},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_WLAN_ACCESS_POINT, keyName: "WLAN_ACCESS_POINT", bitMask: 0x8000 >> 3},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_ROUTER, keyName: "ROUTER", bitMask: 0x8000 >> 4},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_TELEPHONE, keyName: "TELEPHONE", bitMask: 0x8000 >> 5},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_DOCSIS_CABLE_DEVICE, keyName: "DOCSIS_CABLE_DEVICE", bitMask: 0x8000 >> 6},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_STATION_ONLY, keyName: "STATION_ONLY", bitMask: 0x8000 >> 7},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_C_VLAN, keyName: "C_VLAN", bitMask: 0x8000 >> 8},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_S_VLAN, keyName: "S_VLAN", bitMask: 0x8000 >> 9},
	{name: ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_TWO_PORT_MAC_RELAY, keyName: "TWO_PORT_MAC_RELAY", bitMask: 0x8000 >> 10},
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

	XlateFuncBind("DbToYang_lldp_mgmt_addresses_xfmr", DbToYang_lldp_mgmt_addresses_xfmr)
	XlateFuncBind("YangToDb_lldp_mgmt_addresses_xfmr", YangToDb_lldp_mgmt_addresses_xfmr)
	XlateFuncBind("Subscribe_lldp_mgmt_addresses_xfmr", Subscribe_lldp_mgmt_addresses_xfmr)

	XlateFuncBind("DbToYang_lldp_custom_tlvs_xfmr", DbToYang_lldp_custom_tlvs_xfmr)
	XlateFuncBind("YangToDb_lldp_custom_tlvs_xfmr", YangToDb_lldp_custom_tlvs_xfmr)
	XlateFuncBind("Subscribe_lldp_custom_tlvs_xfmr", Subscribe_lldp_custom_tlvs_xfmr)
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

func lldpConfigDb(inParams XfmrParams) *db.DB {
	if inParams.dbs[db.ConfigDB] != nil {
		return inParams.dbs[db.ConfigDB]
	}
	return inParams.d
}

func lldpInterfaceKeyExists(cfgDb *db.DB, table, ifname string) bool {
	if cfgDb == nil || ifname == "" {
		return false
	}
	_, err := cfgDb.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{ifname}})
	return err == nil
}

func lldpInterfacePrefixExists(cfgDb *db.DB, table, ifname string) bool {
	names, err := getTableKeys(cfgDb, table)
	if err != nil {
		return false
	}
	for _, name := range names {
		if name == ifname {
			return true
		}
	}
	return false
}

func validateLldpManagementInterface(cfgDb *db.DB, ifname string) error {
	if ifname == "" {
		return nil
	}
	if !lldpMgmtIfNameRe.MatchString(ifname) {
		return tlerr.InvalidArgs("Invalid LLDP management-interface '%s'", ifname)
	}
	if cfgDb == nil {
		return nil
	}
	if lldpInterfaceKeyExists(cfgDb, "PORT", ifname) ||
		lldpInterfaceKeyExists(cfgDb, "MGMT_PORT", ifname) ||
		lldpInterfaceKeyExists(cfgDb, "PORTCHANNEL", ifname) ||
		lldpInterfaceKeyExists(cfgDb, "VLAN", ifname) ||
		lldpInterfacePrefixExists(cfgDb, "MGMT_INTERFACE", ifname) ||
		lldpInterfacePrefixExists(cfgDb, "LOOPBACK_INTERFACE", ifname) ||
		lldpInterfacePrefixExists(cfgDb, "VLAN_INTERFACE", ifname) ||
		lldpInterfacePrefixExists(cfgDb, "VLAN_SUB_INTERFACE", ifname) {
		return nil
	}
	return tlerr.InvalidArgs("Invalid LLDP management-interface '%s': interface not found", ifname)
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

func getLldpStatisticsEntry(countersDb *db.DB, key string) (db.Value, error) {
	if countersDb == nil || key == "" {
		return db.Value{}, nil
	}
	entry, err := countersDb.GetEntry(&db.TableSpec{Name: LLDP_STATISTICS_TABLE}, db.Key{Comp: []string{key}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return db.Value{}, err
	}
	return entry, nil
}

func fillLldpGlobalCounters(state *ocbinds.OpenconfigLldp_Lldp_State, entry db.Value, includeCounters bool) {
	if state == nil {
		return
	}
	if !entry.IsPopulated() {
		if includeCounters && state.Counters == nil {
			state.Counters = &ocbinds.OpenconfigLldp_Lldp_State_Counters{}
		}
		return
	}
	if state.Counters == nil {
		state.Counters = &ocbinds.OpenconfigLldp_Lldp_State_Counters{}
	}
	c := state.Counters
	c.FrameIn = parseUint64String(entry.Get(LLDP_STAT_FRAME_IN))
	c.FrameOut = parseUint64String(entry.Get(LLDP_STAT_FRAME_OUT))
	c.FrameDiscard = parseUint64String(entry.Get(LLDP_STAT_FRAME_DISCARD))
	c.TlvUnknown = parseUint64String(entry.Get(LLDP_STAT_TLV_UNKNOWN))
	c.TlvAccepted = parseUint64String(entry.Get(LLDP_STAT_TLV_ACCEPTED))
	c.EntriesAgedOut = parseUint64String(entry.Get(LLDP_STAT_ENTRIES_AGED_OUT))
}

func fillLldpInterfaceCounters(ifInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface, entry db.Value, includeCounters bool) {
	if ifInfo == nil || ifInfo.State == nil {
		return
	}
	if !entry.IsPopulated() {
		if includeCounters && ifInfo.State.Counters == nil {
			ifInfo.State.Counters = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_State_Counters{}
		}
		return
	}
	if ifInfo.State.Counters == nil {
		ifInfo.State.Counters = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_State_Counters{}
	}
	c := ifInfo.State.Counters
	c.FrameIn = parseUint64String(entry.Get(LLDP_STAT_FRAME_IN))
	c.FrameOut = parseUint64String(entry.Get(LLDP_STAT_FRAME_OUT))
	c.FrameDiscard = parseUint64String(entry.Get(LLDP_STAT_FRAME_DISCARD))
	c.TlvUnknown = parseUint64String(entry.Get(LLDP_STAT_TLV_UNKNOWN))
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

func parseUint16String(val string) *uint16 {
	if val == "" {
		return nil
	}
	parsed, err := strconv.ParseUint(val, 10, 16)
	if err != nil {
		return nil
	}
	u := uint16(parsed)
	return &u
}

func uint32Ptr(v uint32) *uint32 {
	return &v
}

func splitLldpMgmtAddresses(joined string) []string {
	var addrs []string
	for _, part := range strings.Split(joined, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			addrs = append(addrs, part)
		}
	}
	return addrs
}

func decodeLldpHexBytes(s string) []byte {
	cleaned := strings.NewReplacer(":", "", ",", "", "-", "", " ", "").Replace(s)
	if cleaned == "" {
		return nil
	}
	b, err := hex.DecodeString(cleaned)
	if err != nil {
		return nil
	}
	return b
}

func encodeLldpHexBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

func normalizeLldpOui(s string) (string, error) {
	b := decodeLldpHexBytes(s)
	if len(b) != 3 {
		return "", tlerr.InvalidArgs("LLDP custom-tlv OUI must be 3 bytes")
	}
	return fmt.Sprintf("%02x:%02x:%02x", b[0], b[1], b[2]), nil
}

type lldpCustomTlvRecord struct {
	Type       int32  `json:"type"`
	Oui        string `json:"oui"`
	OuiSubtype string `json:"oui-subtype"`
	Value      string `json:"value"`
}

func parseLldpCustomTlvRecords(raw string) []lldpCustomTlvRecord {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var recs []lldpCustomTlvRecord
	if err := json.Unmarshal([]byte(raw), &recs); err != nil {
		return nil
	}
	return recs
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

func suppressTlvsToConfig(tlvs []ocbinds.E_OpenconfigLldpTypes_LLDP_TLV) (map[string]string, error) {
	fields := map[string]string{
		LLDP_CFG_SUPP_MGMT_ADDR_TLV: "false",
		LLDP_CFG_SUPP_SYS_CAP_TLV:   "false",
	}
	for _, tlv := range tlvs {
		switch tlv {
		case ocbinds.OpenconfigLldpTypes_LLDP_TLV_UNSET:
			continue
		case ocbinds.OpenconfigLldpTypes_LLDP_TLV_MANAGEMENT_ADDRESS:
			fields[LLDP_CFG_SUPP_MGMT_ADDR_TLV] = "true"
		case ocbinds.OpenconfigLldpTypes_LLDP_TLV_SYSTEM_CAPABILITIES:
			fields[LLDP_CFG_SUPP_SYS_CAP_TLV] = "true"
		default:
			return nil, tlerr.NotSupported("LLDP suppress-tlv-advertisement identity not supported by lldpd")
		}
	}
	return fields, nil
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
		if val := globalEntry.Get(LLDP_CFG_MGMT_INTERFACE); val != "" {
			cfg.ManagementInterface = &val
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
		if val := globalEntry.Get(LLDP_CFG_MGMT_INTERFACE); val != "" {
			state.ManagementInterface = &val
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
		if val := locChassis.Get(LLDP_LOC_TTL); val != "" {
			state.Ttl = parseUint16String(val)
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
	enabled                *bool
	helloTimer             *uint64
	systemName             *string
	systemDesc             *string
	managementInterface    *string
	managementInterfaceSet bool
	suppressTlvs           []ocbinds.E_OpenconfigLldpTypes_LLDP_TLV
	suppressTlvsSet        bool
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
	if cfg.ManagementInterface != nil {
		snap.managementInterfaceSet = true
		snap.managementInterface = cfg.ManagementInterface
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
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	var cfg *ocbinds.OpenconfigLldp_Lldp_Config
	if device.Lldp != nil {
		cfg = device.Lldp.Config
	}
	if err := rejectUnsupportedLldpGlobalConfig(targetUriPath, cfg); err != nil {
		return nil, err
	}
	var cfgSnap lldpGlobalConfigSnapshot
	if cfg != nil {
		cfgSnap = snapshotLldpGlobalConfig(cfg)
	}
	if device.Lldp == nil {
		ygot.BuildEmptyTree(device)
	}
	lldpObj := device.Lldp
	ygot.BuildEmptyTree(lldpObj)
	lldpClearUnpopulatedRootContainers(lldpObj)
	if lldpObj.Config == nil {
		lldpObj.Config = &ocbinds.OpenconfigLldp_Lldp_Config{}
	}
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
		case LLDP_CONFIG + "/management-interface":
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_MGMT_INTERFACE] = ""
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
	if shouldMapLldpGlobalConfigLeaf(targetUriPath, "management-interface") && cfgSnap.managementInterfaceSet {
		if cfgSnap.managementInterface == nil || *cfgSnap.managementInterface == "" {
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_MGMT_INTERFACE] = ""
		} else {
			if err := validateLldpManagementInterface(lldpConfigDb(inParams), *cfgSnap.managementInterface); err != nil {
				return nil, err
			}
			globalMap[LLDP_GLOBAL_KEY].Field[LLDP_CFG_MGMT_INTERFACE] = *cfgSnap.managementInterface
		}
	}
	if shouldMapLldpGlobalConfigLeaf(targetUriPath, "suppress-tlv-advertisement") && cfgSnap.suppressTlvsSet {
		fields, err := suppressTlvsToConfig(cfgSnap.suppressTlvs)
		if err != nil {
			return nil, err
		}
		for k, v := range fields {
			globalMap[LLDP_GLOBAL_KEY].Field[k] = v
		}
	}
	return res, nil
}

var DbToYang_lldp_config_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	lldpObj := getLldpRoot(inParams.ygRoot)
	ygot.BuildEmptyTree(lldpObj)
	lldpClearUnpopulatedRootContainers(lldpObj)
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
	lldpClearUnpopulatedRootContainers(lldpObj)
	if lldpObj.State == nil {
		lldpObj.State = &ocbinds.OpenconfigLldp_Lldp_State{}
	}
	ygot.BuildEmptyTree(lldpObj.State)

	globalEntry, _ := getLldpGlobalConfigEntry(inParams.dbs[db.ConfigDB])
	locChassis, _ := getLldpLocChassisEntry(inParams.dbs[db.ApplDB])
	includeCounters := pathNeedsCounters(targetUriPath) || targetUriPath == LLDP_STATE
	fillGlobalStateLeaves(lldpObj.State, globalEntry, locChassis, includeCounters)
	countersEntry, _ := getLldpStatisticsEntry(inParams.dbs[db.CountersDB], LLDP_STATISTICS_GLOBAL_KEY)
	fillLldpGlobalCounters(lldpObj.State, countersEntry, includeCounters)
	return nil
}

var Subscribe_lldp_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = false
		return result, nil
	}

	targetUriPath, err := getYangPathFromUri(NewPathInfo(inParams.uri).Path)
	if err != nil {
		return result, err
	}

	// Counters-only paths watch COUNTERS_DB so SAMPLE and ON_CHANGE
	// (every lldp_syncd scrape) do not depend on chassis/config updates.
	if pathNeedsCounters(targetUriPath) {
		fillLldpCountersSubscribe(&result, LLDP_STATISTICS_GLOBAL_KEY)
		result.nOpts = &notificationOpts{mInterval: 30, pType: Sample}
		return result, nil
	}

	fillLldpGlobalSubscribe(&result)
	if targetUriPath == LLDP_STATE {
		fillLldpCountersSubscribe(&result, LLDP_STATISTICS_GLOBAL_KEY)
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

// fillLldpCountersSubscribe maps COUNTERS_DB LLDP_STATISTICS so gNMI
// SAMPLE and ON_CHANGE both work. OnChangeEnable is required: translib
// rejects ON_CHANGE on CountersDB unless the subtree xfmr opts in.
// SAMPLE uses a 30s interval (same as interface counters); ON_CHANGE
// fires when lldp_syncd rewrites the hash on each scrape.
func fillLldpCountersSubscribe(result *XfmrSubscOutParams, key string) {
	if result.dbDataMap == nil {
		result.dbDataMap = make(RedisDbSubscribeMap)
	}
	if key == "" {
		key = "*"
	}
	result.dbDataMap[db.CountersDB] = map[string]map[string]map[string]string{
		LLDP_STATISTICS_TABLE: {key: {}},
	}
	result.onChange = OnchangeEnable
	result.isVirtualTbl = false
	result.needCache = true
}

// --- Management addresses (1.2.0). ---

var YangToDb_lldp_mgmt_addresses_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	if targetUriPath == "" {
		targetUriPath = LLDP_MGMT_ADDRESSES
	}
	return nil, tlerr.NotSupported("LLDP management addresses are read-only")
}

var DbToYang_lldp_mgmt_addresses_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	lldpObj := getLldpRoot(inParams.ygRoot)
	locChassis, _ := getLldpLocChassisEntry(inParams.dbs[db.ApplDB])
	fillLldpRootMgmtAddresses(lldpObj, locChassis.Get(LLDP_LOC_MAN_ADDR))
	return nil
}

var Subscribe_lldp_mgmt_addresses_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = false
		return result, nil
	}
	result.dbDataMap[db.ApplDB] = map[string]map[string]map[string]string{
		LLDP_LOC_CHASSIS_TABLE: {"": {}},
	}
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	result.isVirtualTbl = false
	return result, nil
}

func fillLldpRootMgmtAddresses(lldpObj *ocbinds.OpenconfigLldp_Lldp, joined string) {
	if lldpObj == nil {
		return
	}
	addrs := splitLldpMgmtAddresses(joined)
	if len(addrs) == 0 {
		lldpObj.MgmtAddresses = nil
		return
	}
	if lldpObj.MgmtAddresses == nil {
		lldpObj.MgmtAddresses = &ocbinds.OpenconfigLldp_Lldp_MgmtAddresses{}
	}
	for _, addr := range addrs {
		addr := addr
		ma, err := lldpObj.MgmtAddresses.NewMgmtAddress(addr)
		if err != nil {
			continue
		}
		ma.State = &ocbinds.OpenconfigLldp_Lldp_MgmtAddresses_MgmtAddress_State{
			Address: &addr,
		}
	}
}

func customTlvNameFromUri(uri string) string {
	return NewPathInfo(uri).Var("name")
}

func customTlvFieldsFromConfig(name string, cfg *ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv_Config) (map[string]string, error) {
	fields := map[string]string{}
	if cfg == nil {
		return fields, nil
	}
	tlvType := lldpOrgSpecificTlvType
	if cfg.Type != nil {
		tlvType = *cfg.Type
	}
	fields[LLDP_CUSTOM_TLV_TYPE] = strconv.FormatInt(int64(tlvType), 10)
	if cfg.Oui != nil && *cfg.Oui != "" {
		oui, err := normalizeLldpOui(*cfg.Oui)
		if err != nil {
			return nil, err
		}
		fields[LLDP_CUSTOM_TLV_OUI] = oui
	}
	if cfg.OuiSubtype != nil && *cfg.OuiSubtype != "" {
		fields[LLDP_CUSTOM_TLV_SUBTYPE] = *cfg.OuiSubtype
	}
	if len(cfg.Value) > 0 {
		fields[LLDP_CUSTOM_TLV_VALUE] = encodeLldpHexBytes(cfg.Value)
	}
	_ = name
	return fields, nil
}

func fillAdvertisedCustomTlv(tlv *ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv, name string, entry db.Value) {
	oui := entry.Get(LLDP_CUSTOM_TLV_OUI)
	subtype := entry.Get(LLDP_CUSTOM_TLV_SUBTYPE)
	typeStr := entry.Get(LLDP_CUSTOM_TLV_TYPE)
	tlvType := lldpOrgSpecificTlvType
	if typeStr != "" {
		if parsed, err := strconv.ParseInt(typeStr, 10, 32); err == nil {
			tlvType = int32(parsed)
		}
	}
	n := name
	value := decodeLldpHexBytes(entry.Get(LLDP_CUSTOM_TLV_VALUE))
	cfg := &ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv_Config{
		Name:       &n,
		Type:       &tlvType,
		Oui:        &oui,
		OuiSubtype: &subtype,
		Value:      value,
	}
	st := &ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv_State{
		Name:       &n,
		Type:       &tlvType,
		Oui:        &oui,
		OuiSubtype: &subtype,
		Value:      value,
	}
	if oui == "" {
		cfg.Oui = nil
		st.Oui = nil
	}
	if subtype == "" {
		cfg.OuiSubtype = nil
		st.OuiSubtype = nil
	}
	tlv.Name = &n
	tlv.Config = cfg
	tlv.State = st
}

// --- Global advertised custom-tlvs. ---

var YangToDb_lldp_custom_tlvs_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	cfgDb := inParams.dbs[db.ConfigDB]
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	if targetUriPath == "" {
		targetUriPath = LLDP_CUSTOM_TLVS
	}
	name := customTlvNameFromUri(inParams.requestUri)
	if name == "" {
		name = customTlvNameFromUri(inParams.uri)
	}

	tlvOps := make(map[string]db.Value)
	res := map[string]map[string]db.Value{LLDP_CUSTOM_TLV_TABLE: tlvOps}

	isList := targetUriPath == LLDP_CUSTOM_TLVS
	isItem := strings.HasPrefix(targetUriPath, LLDP_CUSTOM_TLV)

	if inParams.oper == DELETE {
		if isList && name == "" {
			keys, err := getTableKeys(cfgDb, LLDP_CUSTOM_TLV_TABLE)
			if err != nil {
				return nil, err
			}
			for _, key := range keys {
				tlvOps[key] = db.Value{}
			}
			return res, nil
		}
		if name != "" {
			tlvOps[name] = db.Value{}
			return res, nil
		}
		return nil, tlerr.NotSupported("DELETE not supported on " + targetUriPath)
	}

	lldpObj := getLldpRoot(inParams.ygRoot)
	var items map[string]*ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv
	if lldpObj != nil && lldpObj.CustomTlvs != nil {
		items = lldpObj.CustomTlvs.Tlv
	}

	writeOne := func(key string, tlv *ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv) error {
		if key == "" {
			return tlerr.InvalidArgs("LLDP custom-tlv name is required")
		}
		var cfg *ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv_Config
		if tlv != nil {
			cfg = tlv.Config
		}
		fields, err := customTlvFieldsFromConfig(key, cfg)
		if err != nil {
			return err
		}
		if inParams.oper == REPLACE || (fields[LLDP_CUSTOM_TLV_OUI] == "" && fields[LLDP_CUSTOM_TLV_SUBTYPE] == "") {
			if fields[LLDP_CUSTOM_TLV_OUI] == "" || fields[LLDP_CUSTOM_TLV_SUBTYPE] == "" {
				return tlerr.InvalidArgs("LLDP custom-tlv requires oui and oui-subtype")
			}
		}
		tlvOps[key] = db.Value{Field: fields}
		return nil
	}

	if isList && name == "" {
		if inParams.oper == REPLACE {
			keys, err := getTableKeys(cfgDb, LLDP_CUSTOM_TLV_TABLE)
			if err != nil {
				return nil, err
			}
			keep := map[string]bool{}
			for key := range items {
				keep[key] = true
			}
			for _, key := range keys {
				if !keep[key] {
					tlvOps[key] = db.Value{}
				}
			}
		}
		for key, tlv := range items {
			if err := writeOne(key, tlv); err != nil {
				return nil, err
			}
		}
		return res, nil
	}

	if name == "" {
		return nil, tlerr.InvalidArgs("LLDP custom-tlv name is required")
	}
	if err := writeOne(name, items[name]); err != nil {
		return nil, err
	}
	_ = isItem
	return res, nil
}

var DbToYang_lldp_custom_tlvs_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	lldpObj := getLldpRoot(inParams.ygRoot)
	if lldpObj == nil {
		return nil
	}
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return nil
	}
	name := customTlvNameFromUri(inParams.uri)
	if lldpObj.CustomTlvs == nil {
		lldpObj.CustomTlvs = &ocbinds.OpenconfigLldp_Lldp_CustomTlvs{}
	}
	if lldpObj.CustomTlvs.Tlv == nil {
		lldpObj.CustomTlvs.Tlv = make(map[string]*ocbinds.OpenconfigLldp_Lldp_CustomTlvs_Tlv)
	}

	loadOne := func(key string) error {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: LLDP_CUSTOM_TLV_TABLE}, db.Key{Comp: []string{key}})
		if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
			return err
		}
		if !entry.IsPopulated() {
			return nil
		}
		tlv, ok := lldpObj.CustomTlvs.Tlv[key]
		if !ok {
			tlv, err = lldpObj.CustomTlvs.NewTlv(key)
			if err != nil {
				return err
			}
		}
		fillAdvertisedCustomTlv(tlv, key, entry)
		return nil
	}

	if name != "" {
		if err := loadOne(name); err != nil {
			return err
		}
	} else {
		keys, err := getTableKeys(cfgDb, LLDP_CUSTOM_TLV_TABLE)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := loadOne(key); err != nil {
				return err
			}
		}
	}
	if len(lldpObj.CustomTlvs.Tlv) == 0 {
		lldpObj.CustomTlvs = nil
	}
	return nil
}

var Subscribe_lldp_custom_tlvs_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = false
		return result, nil
	}
	name := customTlvNameFromUri(inParams.uri)
	if name == "" {
		name = "*"
	}
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		LLDP_CUSTOM_TLV_TABLE: {name: {}},
	}
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	result.isVirtualTbl = false
	return result, nil
}

// --- Interfaces transformer ---

var YangToDb_lldp_interfaces_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	lldpObj := getLldpRoot(inParams.ygRoot)
	if lldpObj.Interfaces == nil {
		return nil, nil
	}
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	if err := rejectUnsupportedLldpInterfaceConfig(targetUriPath, nil); err != nil {
		return nil, err
	}
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
		if ifInfo == nil || ifInfo.Config == nil {
			continue
		}
		if err := rejectUnsupportedLldpInterfaceConfig(targetUriPath, ifInfo.Config); err != nil {
			return nil, err
		}
		if ifInfo.Config.Enabled == nil {
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
	countersDb := inParams.dbs[db.CountersDB]

	lldpObj := getLldpRoot(inParams.ygRoot)
	ygot.BuildEmptyTree(lldpObj)
	return populateLldpInterfaces(lldpObj, appDb, cfgDb, countersDb, ifName, neighId, capKey, targetUriPath)
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
	case strings.Contains(targetUriPath, "/counters"):
		fillLldpCountersSubscribe(&result, ifName)
		result.nOpts = &notificationOpts{mInterval: 30, pType: Sample}
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

func collectLldpInterfaceNames(appDb, cfgDb, countersDb *db.DB, ifName string) []string {
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
	if countersDb != nil {
		if keys, err := getTableKeys(countersDb, LLDP_STATISTICS_TABLE); err == nil {
			for _, name := range keys {
				if name == LLDP_STATISTICS_GLOBAL_KEY {
					continue
				}
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
	// Do not BuildEmptyTree(ngInfo): 1.2.0 adds link-aggregation and
	// mgmt-addresses containers that would serialize empty on GET.
	lldpClearUnpopulatedNeighborContainers(ngInfo)
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

func populateLldpInterfaces(lldpObj *ocbinds.OpenconfigLldp_Lldp, appDb, cfgDb, countersDb *db.DB, ifName, neighId, capKey, targetUriPath string) error {
	if lldpObj.Interfaces == nil {
		lldpObj.Interfaces = &ocbinds.OpenconfigLldp_Lldp_Interfaces{}
	}
	ygot.BuildEmptyTree(lldpObj.Interfaces)

	includeCounters := pathNeedsCounters(targetUriPath)
	neighOnly := strings.Contains(targetUriPath, "/neighbors")
	skipNeighbors := strings.Contains(targetUriPath, "/config") ||
		(strings.Contains(targetUriPath, "/state") && !strings.Contains(targetUriPath, "/neighbors"))

	for _, ifNameKey := range collectLldpInterfaceNames(appDb, cfgDb, countersDb, ifName) {
		ifInfo, err := getOrCreateLldpInterface(lldpObj.Interfaces, ifNameKey)
		if err != nil {
			return err
		}

		portEntry, _ := getLldpPortEntry(cfgDb, ifNameKey)
		if !neighOnly {
			fillInterfaceConfig(ifInfo, portEntry)
			fillInterfaceState(ifInfo, portEntry, includeCounters)
			countersEntry, _ := getLldpStatisticsEntry(countersDb, ifNameKey)
			fillLldpInterfaceCounters(ifInfo, countersEntry, includeCounters)
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
	fillNeighborMgmtAddresses(ngInfo, entry.Get(LLDP_REMOTE_MAN_ADDR))
	if val := entry.Get(LLDP_REMOTE_TTL); val != "" {
		ngInfo.State.Ttl = parseUint16String(val)
	}
	if val := entry.Get(LLDP_REMOTE_MAX_FRAME_SIZE); val != "" {
		ngInfo.State.MaxFrameSize = parseUint16String(val)
	}
	if val := entry.Get(LLDP_REMOTE_PORT_VLAN_ID); val != "" {
		ngInfo.State.PortVlanId = parseUint16String(val)
	}
	if val := entry.Get(LLDP_REMOTE_MED_INV_SERIAL); val != "" {
		ngInfo.State.MedInventorySerialNumber = &val
	}
	fillNeighborLinkAggregation(ngInfo, entry.Get(LLDP_REMOTE_AGG_PORT_ID))
	fillNeighborCustomTlvs(ngInfo, entry.Get(LLDP_REMOTE_CUSTOM_TLVS))
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
	numStr := strings.Fields(capb)
	if len(numStr) == 0 {
		return
	}
	hexStr := strings.Join(numStr, "")
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	raw, err := hex.DecodeString(hexStr)
	if err != nil || len(raw) == 0 {
		return
	}
	var sysCap uint16
	if len(raw) >= 2 {
		sysCap = uint16(raw[0])<<8 | uint16(raw[1])
	} else {
		sysCap = uint16(raw[0]) << 8
	}

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

func fillNeighborMgmtAddresses(ngInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor, joined string) {
	if ngInfo == nil {
		return
	}
	addrs := splitLldpMgmtAddresses(joined)
	if len(addrs) == 0 {
		ngInfo.MgmtAddresses = nil
		return
	}
	if ngInfo.MgmtAddresses == nil {
		ngInfo.MgmtAddresses = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_MgmtAddresses{}
	}
	for _, addr := range addrs {
		addr := addr
		ma, err := ngInfo.MgmtAddresses.NewMgmtAddress(addr)
		if err != nil {
			continue
		}
		ma.State = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_MgmtAddresses_MgmtAddress_State{
			Address: &addr,
		}
	}
}

func fillNeighborLinkAggregation(ngInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor, aggregid string) {
	if ngInfo == nil || aggregid == "" {
		return
	}
	parsed, err := strconv.ParseUint(aggregid, 10, 32)
	if err != nil {
		return
	}
	id := uint32(parsed)
	ngInfo.LinkAggregation = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_LinkAggregation{
		State: &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_LinkAggregation_State{
			Capable: boolPtr(true),
			Enabled: boolPtr(id > 0),
			PortId:  uint32Ptr(id),
		},
	}
}

func fillNeighborCustomTlvs(ngInfo *ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor, raw string) {
	if ngInfo == nil {
		return
	}
	recs := parseLldpCustomTlvRecords(raw)
	if len(recs) == 0 {
		ngInfo.CustomTlvs = nil
		return
	}
	if ngInfo.CustomTlvs == nil {
		ngInfo.CustomTlvs = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_CustomTlvs{}
	}
	for _, rec := range recs {
		tlvType := rec.Type
		if tlvType == 0 {
			tlvType = lldpOrgSpecificTlvType
		}
		oui := rec.Oui
		subtype := rec.OuiSubtype
		tlv, err := ngInfo.CustomTlvs.NewTlv(tlvType, oui, subtype)
		if err != nil {
			continue
		}
		tlv.State = &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_Neighbors_Neighbor_CustomTlvs_Tlv_State{
			Type:       &tlvType,
			Oui:        &oui,
			OuiSubtype: &subtype,
			Value:      decodeLldpHexBytes(rec.Value),
		}
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}
