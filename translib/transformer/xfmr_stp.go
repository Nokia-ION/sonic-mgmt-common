////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                     //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
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
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	// CONFIG_DB tables/keys for RSTP mapped to MSTP instance 0 (CIST).
	STP_GLOBAL_TBL     = "STP"
	STP_GLOBAL_KEY     = "GLOBAL"
	STP_PORT_TBL       = "STP_PORT"
	STP_MST_TBL        = "STP_MST"
	STP_MST_GLOBAL_KEY = "GLOBAL"
	STP_MST_INST_TBL   = "STP_MST_INST"
	STP_MST_PORT_TBL   = "STP_MST_PORT"
	VLAN_MEMBER_TBL    = "VLAN_MEMBER"

	// APPL_DB operational tables for MSTI 0.
	STP_MST_INST_APP_TBL = "STP_MST_INST_TABLE"
	STP_MST_PORT_APP_TBL = "STP_MST_PORT_TABLE"
	// ProducerStateTable entries are stored in Redis with a leading "_".
	applStateTablePrefix = "_"

	RSTP_MSTI_ID = 0

	STP_MODE_MST = "mst"

	// Default values aligned with sonic-utilities/config/stp.py MST defaults.
	DEFAULT_BRIDGE_PRIORITY = "32768"
	DEFAULT_PORT_PRIORITY   = "128"
	DEFAULT_PATH_COST       = "1"
	DEFAULT_EDGE_PORT       = "false"
	DEFAULT_LINK_TYPE       = "auto"
	DEFAULT_HELLO_TIME      = "2"
	DEFAULT_MAX_AGE         = "20"
	DEFAULT_FORWARD_DELAY   = "15"
	DEFAULT_HOLD_COUNT      = "6"

	STP_GLOBAL           = "/openconfig-spanning-tree:stp/global"
	STP_GLOBAL_CONFIG    = "/openconfig-spanning-tree:stp/global/config"
	STP_GLOBAL_STATE     = "/openconfig-spanning-tree:stp/global/state"
	STP_RSTP             = "/openconfig-spanning-tree:stp/rstp"
	STP_RSTP_CONFIG      = "/openconfig-spanning-tree:stp/rstp/config"
	STP_RSTP_STATE       = "/openconfig-spanning-tree:stp/rstp/state"
	STP_RSTP_INTERFACES  = "/openconfig-spanning-tree:stp/rstp/interfaces"
	STP_RSTP_INTERFACE   = "/openconfig-spanning-tree:stp/rstp/interfaces/interface"
	STP_INTF_COMMON      = "/openconfig-spanning-tree:stp/interfaces"
	STP_INTF_COMMON_INTF = "/openconfig-spanning-tree:stp/interfaces/interface"
)

func init() {
	XlateFuncBind("DbToYang_stp_global_xfmr", DbToYang_stp_global_xfmr)
	XlateFuncBind("YangToDb_stp_global_xfmr", YangToDb_stp_global_xfmr)
	XlateFuncBind("Subscribe_stp_global_xfmr", Subscribe_stp_global_xfmr)

	XlateFuncBind("DbToYang_stp_rstp_xfmr", DbToYang_stp_rstp_xfmr)
	XlateFuncBind("YangToDb_stp_rstp_xfmr", YangToDb_stp_rstp_xfmr)
	XlateFuncBind("Subscribe_stp_rstp_xfmr", Subscribe_stp_rstp_xfmr)

	XlateFuncBind("DbToYang_stp_rstp_intf_xfmr", DbToYang_stp_rstp_intf_xfmr)
	XlateFuncBind("YangToDb_stp_rstp_intf_xfmr", YangToDb_stp_rstp_intf_xfmr)
	XlateFuncBind("Subscribe_stp_rstp_intf_xfmr", Subscribe_stp_rstp_intf_xfmr)
	XlateFuncBind("DbToYangPath_stp_rstp_intf_path_xfmr", DbToYangPath_stp_rstp_intf_path_xfmr)
	XlateFuncBind("YangToDb_stp_rstp_intf_key_xfmr", YangToDb_stp_rstp_intf_key_xfmr)
	XlateFuncBind("DbToYang_stp_rstp_intf_key_xfmr", DbToYang_stp_rstp_intf_key_xfmr)

	XlateFuncBind("DbToYang_stp_intf_common_xfmr", DbToYang_stp_intf_common_xfmr)
	XlateFuncBind("YangToDb_stp_intf_common_xfmr", YangToDb_stp_intf_common_xfmr)
	XlateFuncBind("Subscribe_stp_intf_common_xfmr", Subscribe_stp_intf_common_xfmr)
	XlateFuncBind("DbToYangPath_stp_intf_common_path_xfmr", DbToYangPath_stp_intf_common_path_xfmr)

	XlateFuncBind("YangToDb_stp_enabled_protocol_xfmr", YangToDb_stp_enabled_protocol_xfmr)
	XlateFuncBind("DbToYang_stp_enabled_protocol_xfmr", DbToYang_stp_enabled_protocol_xfmr)
	XlateFuncBind("YangToDb_stp_bridge_priority_xfmr", YangToDb_stp_bridge_priority_xfmr)
	XlateFuncBind("DbToYang_stp_bridge_priority_xfmr", DbToYang_stp_bridge_priority_xfmr)
	XlateFuncBind("YangToDb_stp_edge_port_xfmr", YangToDb_stp_edge_port_xfmr)
	XlateFuncBind("DbToYang_stp_edge_port_xfmr", DbToYang_stp_edge_port_xfmr)
	XlateFuncBind("YangToDb_stp_link_type_xfmr", YangToDb_stp_link_type_xfmr)
	XlateFuncBind("DbToYang_stp_link_type_xfmr", DbToYang_stp_link_type_xfmr)
	XlateFuncBind("stp_post_xfmr", stp_post_xfmr)
}

func getStpRoot(s *ygot.GoStruct) *ocbinds.OpenconfigSpanningTree_Stp {
	if s == nil {
		return nil
	}
	deviceObj := (*s).(*ocbinds.Device)
	if deviceObj.Stp == nil {
		deviceObj.Stp = &ocbinds.OpenconfigSpanningTree_Stp{}
	}
	return deviceObj.Stp
}

func ensureStpCommonInterfaces(stpRoot *ocbinds.OpenconfigSpanningTree_Stp) map[string]*ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface {
	if stpRoot.Interfaces == nil {
		stpRoot.Interfaces = &ocbinds.OpenconfigSpanningTree_Stp_Interfaces{}
	}
	if stpRoot.Interfaces.Interface == nil {
		stpRoot.Interfaces.Interface = make(map[string]*ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface)
	}
	return stpRoot.Interfaces.Interface
}

func ensureStpRstpInterfaces(stpRoot *ocbinds.OpenconfigSpanningTree_Stp) map[string]*ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface {
	if stpRoot.Rstp == nil {
		stpRoot.Rstp = &ocbinds.OpenconfigSpanningTree_Stp_Rstp{}
	}
	if stpRoot.Rstp.Interfaces == nil {
		stpRoot.Rstp.Interfaces = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces{}
	}
	if stpRoot.Rstp.Interfaces.Interface == nil {
		stpRoot.Rstp.Interfaces.Interface = make(map[string]*ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface)
	}
	return stpRoot.Rstp.Interfaces.Interface
}

func mstInstCfgKey() string {
	return strconv.Itoa(RSTP_MSTI_ID)
}

func mstPortCfgKey(ifName string) string {
	// CONFIG_DB/CVL key follows YANG list keys (inst_id|ifname). CLI may use
	// MST_INSTANCE|<id>|<ifname>; stpmgr accepts both formats.
	return fmt.Sprintf("%d|%s", RSTP_MSTI_ID, ifName)
}

func mstPortAppKey(ifName string) string {
	return fmt.Sprintf("%d:%s", RSTP_MSTI_ID, ifName)
}

func isRstpEnabled(cfgDb *db.DB) (bool, error) {
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: STP_GLOBAL_TBL}, db.Key{Comp: []string{STP_GLOBAL_KEY}})
	if err != nil {
		if tlerr.IsTranslibRedisClientEntryNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return entry.Get("mode") == STP_MODE_MST, nil
}

func getVlanMemberInterfaces(cfgDb *db.DB) ([]string, error) {
	if cfgDb == nil {
		return nil, nil
	}
	keys, err := cfgDb.GetKeysByPattern(&db.TableSpec{Name: VLAN_MEMBER_TBL}, "*")
	if err != nil {
		return nil, err
	}
	intfSet := make(map[string]struct{})
	for _, key := range keys {
		if len(key.Comp) < 2 {
			continue
		}
		intfSet[key.Comp[1]] = struct{}{}
	}
	intfs := make([]string, 0, len(intfSet))
	for ifName := range intfSet {
		intfs = append(intfs, ifName)
	}
	return intfs, nil
}

func defaultStpPortFields() map[string]string {
	return map[string]string{
		"enabled":               "true",
		"edge_port":             DEFAULT_EDGE_PORT,
		"link_type":             DEFAULT_LINK_TYPE,
		"bpdu_guard":            "false",
		"bpdu_guard_do_disable": "false",
		"root_guard":            "false",
		"path_cost":             DEFAULT_PATH_COST,
		"priority":              DEFAULT_PORT_PRIORITY,
	}
}

// defaultStpPortBootstrapFields omits edge_port and link_type. Those leaves have
// YANG must expressions requiring STP|GLOBAL mode=mst, but CVL validates each
// CONFIG_DB CREATE separately and drops already-validated entries from the batch.
// STP|GLOBAL is not visible when STP_PORT is validated on cold start.
// path_cost is also omitted so stpmgr passes 0 to stpd and the daemon derives cost
// from interface speed (same behavior as adding a VLAN member after MST is enabled).
func defaultStpPortBootstrapFields() map[string]string {
	return map[string]string{
		"enabled":               "true",
		"bpdu_guard":            "false",
		"bpdu_guard_do_disable": "false",
		"root_guard":            "false",
		"priority":              DEFAULT_PORT_PRIORITY,
	}
}

func defaultMstPortFields() map[string]string {
	return map[string]string{
		"priority": DEFAULT_PORT_PRIORITY,
	}
}

func enableRstpGlobal(res map[string]map[string]db.Value) {
	globalMap := make(map[string]db.Value)
	globalMap[STP_GLOBAL_KEY] = db.Value{Field: map[string]string{"mode": STP_MODE_MST}}
	res[STP_GLOBAL_TBL] = globalMap
}

// stripStpGlobalYangDefaultsForMst blocks translib from merging YANG schema defaults
// (e.g. rootguard_timeout) into STP|GLOBAL on MST/RSTP enable. CLI writes only
// mode=mst; persisting PVST-only defaults breaks config save/reload validation.
func stripStpGlobalYangDefaultsForMst(yangDefValMap map[string]map[string]db.Value) {
	if yangDefValMap == nil {
		return
	}
	delete(yangDefValMap[STP_GLOBAL_TBL], STP_GLOBAL_KEY)
	if len(yangDefValMap[STP_GLOBAL_TBL]) == 0 {
		delete(yangDefValMap, STP_GLOBAL_TBL)
	}
}

func enableRstpMsti0(res map[string]map[string]db.Value) {
	enableRstpGlobal(res)
	mstInstMap := make(map[string]db.Value)
	mstInstMap[mstInstCfgKey()] = db.Value{
		Field: map[string]string{"bridge_priority": DEFAULT_BRIDGE_PRIORITY},
	}
	res[STP_MST_INST_TBL] = mstInstMap
}

func populateMstPortEntries(cfgDb *db.DB, res map[string]map[string]db.Value) error {
	intfs, err := getVlanMemberInterfaces(cfgDb)
	if err != nil {
		return err
	}
	if len(intfs) == 0 {
		return nil
	}
	portMap := res[STP_PORT_TBL]
	if portMap == nil {
		portMap = make(map[string]db.Value)
		res[STP_PORT_TBL] = portMap
	}
	mstPortMap := res[STP_MST_PORT_TBL]
	if mstPortMap == nil {
		mstPortMap = make(map[string]db.Value)
		res[STP_MST_PORT_TBL] = mstPortMap
	}
	for _, ifName := range intfs {
		portMap[ifName] = db.Value{Field: defaultStpPortBootstrapFields()}
		mstPortMap[mstPortCfgKey(ifName)] = db.Value{Field: defaultMstPortFields()}
	}
	return nil
}

func disableRstpMsti0(res map[string]map[string]db.Value) {
	// Match CLI disable_global_mst(): delete_table for port tables. An empty inner
	// map signals translib to DeleteTable, avoiding per-row CVL must failures on
	// link_type/edge_port when STP|GLOBAL is removed before row deletes.
	res[STP_MST_PORT_TBL] = map[string]db.Value{}
	res[STP_PORT_TBL] = map[string]db.Value{}
	res[STP_MST_INST_TBL] = map[string]db.Value{mstInstCfgKey(): {}}
	res[STP_MST_TBL] = map[string]db.Value{STP_MST_GLOBAL_KEY: {}}
	res[STP_GLOBAL_TBL] = map[string]db.Value{STP_GLOBAL_KEY: {}}
}

func hasRstpProtocol(protoList []ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL) bool {
	for _, proto := range protoList {
		if proto == ocbinds.OpenconfigSpanningTreeTypes_STP_PROTOCOL_RSTP {
			return true
		}
	}
	return false
}

func setEnabledProtocolRstp(cfg *ocbinds.OpenconfigSpanningTree_Stp_Global_Config) {
	if cfg == nil {
		return
	}
	cfg.EnabledProtocol = []ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL{
		ocbinds.OpenconfigSpanningTreeTypes_STP_PROTOCOL_RSTP,
	}
}

func fillGlobalConfigFromDb(cfg *ocbinds.OpenconfigSpanningTree_Stp_Global_Config, cfgDb *db.DB) error {
	if cfg == nil {
		return nil
	}
	enabled, err := isRstpEnabled(cfgDb)
	if err != nil {
		return err
	}
	if enabled {
		setEnabledProtocolRstp(cfg)
	}
	return nil
}

func fillGlobalStateFromDb(state *ocbinds.OpenconfigSpanningTree_Stp_Global_State, cfgDb *db.DB) error {
	if state == nil {
		return nil
	}
	enabled, err := isRstpEnabled(cfgDb)
	if err != nil {
		return err
	}
	if enabled {
		state.EnabledProtocol = []ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL{
			ocbinds.OpenconfigSpanningTreeTypes_STP_PROTOCOL_RSTP,
		}
	}
	return nil
}

func getMstGlobalEntry(cfgDb *db.DB) (db.Value, error) {
	if cfgDb == nil {
		return db.Value{}, nil
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: STP_MST_TBL}, db.Key{Comp: []string{STP_MST_GLOBAL_KEY}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return db.Value{}, err
	}
	return entry, nil
}

func getMstInstCfgEntry(cfgDb *db.DB) (db.Value, error) {
	if cfgDb == nil {
		return db.Value{}, nil
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: STP_MST_INST_TBL}, db.Key{Comp: []string{mstInstCfgKey()}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return db.Value{}, err
	}
	return entry, nil
}

func getApplProducerStateEntry(appDb *db.DB, table, keyComp string) (db.Value, error) {
	if appDb != nil {
		for _, tbl := range []string{applStateTablePrefix + table, table} {
			entry, err := appDb.GetEntry(&db.TableSpec{Name: tbl}, db.Key{Comp: []string{keyComp}})
			if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
				return db.Value{}, err
			}
			if entry.IsPopulated() {
				return entry, nil
			}
		}
	}
	// ProducerStateTable hashes are stored as _<TABLE>:<key> in APPL_DB.
	// Use the same wildcard lookup as show spanning-tree.
	client := db.RedisClient(db.ApplDB)
	if client == nil {
		return db.Value{}, nil
	}
	keys, err := client.Keys(context.Background(), fmt.Sprintf("*%s:%s", table, keyComp)).Result()
	if err != nil {
		return db.Value{}, err
	}
	for _, redisKey := range keys {
		fields, err := client.HGetAll(context.Background(), redisKey).Result()
		if err != nil {
			return db.Value{}, err
		}
		if len(fields) > 0 {
			return db.Value{Field: fields}, nil
		}
	}
	return db.Value{}, nil
}

func interfaceNameFromMstPortRedisKey(redisKey string) (string, bool) {
	marker := STP_MST_PORT_APP_TBL + ":"
	idx := strings.LastIndex(redisKey, marker)
	if idx < 0 {
		return "", false
	}
	rest := redisKey[idx+len(marker):]
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 {
		return "", false
	}
	return parts[1], true
}

func getMstInstAppEntry(appDb *db.DB) (db.Value, error) {
	return getApplProducerStateEntry(appDb, STP_MST_INST_APP_TBL, mstInstCfgKey())
}

func parseUint8Default(val, def string) uint8 {
	if val == "" {
		val = def
	}
	parsed, err := strconv.ParseUint(val, 10, 8)
	if err != nil {
		parsed64, _ := strconv.ParseUint(def, 10, 8)
		return uint8(parsed64)
	}
	return uint8(parsed)
}

func parseUint16Default(val, def string) uint16 {
	if val == "" {
		val = def
	}
	parsed, err := strconv.ParseUint(val, 10, 16)
	if err != nil {
		parsed64, _ := strconv.ParseUint(def, 10, 16)
		return uint16(parsed64)
	}
	return uint16(parsed)
}

func parseUint32Default(val, def string) uint32 {
	if val == "" {
		val = def
	}
	parsed, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		parsed64, _ := strconv.ParseUint(def, 10, 32)
		return uint32(parsed64)
	}
	return uint32(parsed)
}

func parseUint64Default(val string) uint64 {
	parsed, _ := strconv.ParseUint(val, 10, 64)
	return parsed
}

func fillRstpConfigFromDb(cfg *ocbinds.OpenconfigSpanningTree_Stp_Rstp_Config, cfgDb *db.DB) error {
	if cfg == nil {
		return nil
	}
	mstGlobal, err := getMstGlobalEntry(cfgDb)
	if err != nil {
		return err
	}
	mstInst, err := getMstInstCfgEntry(cfgDb)
	if err != nil {
		return err
	}

	hello := parseUint8Default(mstGlobal.Get("hello_time"), DEFAULT_HELLO_TIME)
	maxAge := parseUint8Default(mstGlobal.Get("max_age"), DEFAULT_MAX_AGE)
	fwdDelay := parseUint8Default(mstGlobal.Get("forward_delay"), DEFAULT_FORWARD_DELAY)
	holdCount := parseUint8Default(mstGlobal.Get("hold_count"), DEFAULT_HOLD_COUNT)
	cfg.HelloTime = &hello
	cfg.MaxAge = &maxAge
	cfg.ForwardingDelay = &fwdDelay
	cfg.HoldCount = &holdCount

	priority := parseUint32Default(mstInst.Get("bridge_priority"), DEFAULT_BRIDGE_PRIORITY)
	cfg.BridgePriority = &priority
	return nil
}

func bridgeIdToMacPriority(bridgeId string) (uint32, string) {
	bridgeId = strings.TrimSpace(bridgeId)
	if bridgeId == "" {
		return 0, ""
	}

	// MSTP stores bridge IDs in dotted format, e.g. "8000.781f.7cfc.b617".
	if strings.Contains(bridgeId, ".") {
		parts := strings.Split(bridgeId, ".")
		if len(parts) < 2 {
			return 0, ""
		}
		priorityVal, err := strconv.ParseUint(parts[0], 16, 32)
		mac := formatMacFromHex(strings.Join(parts[1:], ""))
		if err != nil {
			return 0, mac
		}
		return bridgePriorityFromField(uint32(priorityVal)), mac
	}

	if len(bridgeId) < 4 {
		return 0, ""
	}
	priorityHex := bridgeId[:4]
	macHex := bridgeId[4:]
	priorityVal, err := strconv.ParseUint(priorityHex, 16, 32)
	if err != nil {
		return 0, formatMacFromHex(macHex)
	}
	return bridgePriorityFromField(uint32(priorityVal)), formatMacFromHex(macHex)
}

func bridgePriorityFromField(priorityField uint32) uint32 {
	return priorityField & 0xF000
}

// bridgeIdMacToOcFormat converts the MAC portion of a bridge ID to oc-yang:mac-address format.
func bridgeIdMacToOcFormat(bridgeId string) string {
	bridgeId = strings.TrimSpace(bridgeId)
	if bridgeId == "" {
		return ""
	}
	if strings.Contains(bridgeId, ".") {
		parts := strings.Split(bridgeId, ".")
		// Full bridge ID: 8000.781f.7cfc.b617 (priority + 3 MAC groups).
		if len(parts) == 4 && len(parts[0]) == 4 {
			return formatMacFromHex(strings.Join(parts[1:], ""))
		}
		// MAC-only dotted: 781f.7cfc.b617.
		if len(parts) == 3 {
			return formatMacFromHex(strings.Join(parts, ""))
		}
		return ""
	}
	hexStr := strings.ReplaceAll(bridgeId, ":", "")
	if len(hexStr) == 16 {
		return formatMacFromHex(hexStr[4:])
	}
	return formatMacFromHex(hexStr)
}

func formatMacFromHex(hexStr string) string {
	hexStr = strings.ReplaceAll(hexStr, ":", "")
	if len(hexStr) != 12 {
		return hexStr
	}
	parts := make([]string, 6)
	for i := 0; i < 6; i++ {
		parts[i] = hexStr[i*2 : i*2+2]
	}
	return strings.Join(parts, ":")
}

func mapPortStateToOc(state string) ocbinds.E_OpenconfigSpanningTreeTypes_STP_PORT_STATE {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "DISABLED":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_STATE_DISABLED
	case "LISTENING":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_STATE_LISTENING
	case "LEARNING":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_STATE_LEARNING
	case "BLOCKING":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_STATE_BLOCKING
	case "FORWARDING":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_STATE_FORWARDING
	default:
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_STATE_UNSET
	}
}

func resolveRstpRootPort(appDb *db.DB, rootPortName string) (uint16, bool) {
	rootPortName = strings.TrimSpace(rootPortName)
	if rootPortName == "" {
		return 0, false
	}
	if strings.EqualFold(rootPortName, "Root") {
		return 0, false
	}
	appEntry, err := getApplProducerStateEntry(appDb, STP_MST_PORT_APP_TBL, mstPortAppKey(rootPortName))
	if err != nil || !appEntry.IsPopulated() {
		return 0, false
	}
	if val := appEntry.Get("port_number"); val != "" {
		return parseUint16Default(val, "0"), true
	}
	return 0, false
}

func normalizeStpBridgeId(bridgeId string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(bridgeId), ":", ""))
}

func isRstpRootBridge(appEntry db.Value) bool {
	rootPortName := strings.TrimSpace(appEntry.Get("root_port"))
	if strings.EqualFold(rootPortName, "Root") {
		return true
	}
	bridgeAddr := normalizeStpBridgeId(appEntry.Get("bridge_address"))
	rootAddr := normalizeStpBridgeId(appEntry.Get("root_address"))
	return bridgeAddr != "" && bridgeAddr == rootAddr
}

func computeRstpRootCostFromPort(portEntry db.Value) (uint32, bool) {
	if !portEntry.IsPopulated() {
		return 0, false
	}
	pathCost := parseUint32Default(portEntry.Get("path_cost"), "0")
	extCost := parseUint32Default(portEntry.Get("external_cost"), "0")
	if extCost > 0 || pathCost > 0 {
		return extCost + pathCost, true
	}
	desigCost := parseUint32Default(portEntry.Get("desig_cost"), "0")
	if desigCost > 0 || pathCost > 0 {
		return desigCost + pathCost, true
	}
	return 0, false
}

func resolveRstpRootCost(appDb *db.DB, appEntry db.Value) (uint32, bool) {
	if isRstpRootBridge(appEntry) {
		return 0, true
	}

	rootPathCostVal := appEntry.Get("root_path_cost")
	if rootPathCostVal != "" {
		cost := parseUint32Default(rootPathCostVal, "0")
		if cost > 0 {
			return cost, true
		}
	}

	rootPortName := strings.TrimSpace(appEntry.Get("root_port"))
	if rootPortName != "" && !strings.EqualFold(rootPortName, "Root") {
		portEntry, err := getApplProducerStateEntry(appDb, STP_MST_PORT_APP_TBL, mstPortAppKey(rootPortName))
		if err == nil {
			if cost, ok := computeRstpRootCostFromPort(portEntry); ok && cost > 0 {
				return cost, true
			}
		}
	}

	if rootPathCostVal != "" {
		return parseUint32Default(rootPathCostVal, "0"), true
	}
	return 0, false
}

func mapPortRoleToOc(role string) ocbinds.E_OpenconfigSpanningTreeTypes_STP_PORT_ROLE {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "root":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_ROLE_ROOT
	case "designated":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_ROLE_DESIGNATED
	case "alternate":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_ROLE_ALTERNATE
	case "backup":
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_ROLE_BACKUP
	default:
		return ocbinds.OpenconfigSpanningTreeTypes_STP_PORT_ROLE_UNSET
	}
}

func fillRstpStateFromDb(state *ocbinds.OpenconfigSpanningTree_Stp_Rstp_State, cfgDb, appDb *db.DB) error {
	if state == nil {
		return nil
	}
	tmpCfg := &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Config{}
	if err := fillRstpConfigFromDb(tmpCfg, cfgDb); err != nil {
		return err
	}
	state.HelloTime = tmpCfg.HelloTime
	state.MaxAge = tmpCfg.MaxAge
	state.ForwardingDelay = tmpCfg.ForwardingDelay
	state.HoldCount = tmpCfg.HoldCount
	appEntry, err := getMstInstAppEntry(appDb)
	if err != nil || !appEntry.IsPopulated() {
		return err
	}

	// Operational bridge priority is published by stpd in APPL/STATE DB.
	if val := appEntry.Get("bridge_priority"); val != "" {
		priority := parseUint32Default(val, "0")
		state.BridgePriority = &priority
	} else {
		state.BridgePriority = tmpCfg.BridgePriority
	}

	if mac := appEntry.Get("bridge_address"); mac != "" {
		if formatted := bridgeIdMacToOcFormat(mac); formatted != "" {
			state.BridgeAddress = &formatted
		}
	}
	rootAddr := appEntry.Get("root_address")
	if rootAddr != "" {
		priority, mac := bridgeIdToMacPriority(rootAddr)
		if mac != "" {
			state.DesignatedRootAddress = &mac
		}
		state.DesignatedRootPriority = &priority
	}
	// When this bridge is the root, bridge and root IDs share the same MAC.
	// Use the operational bridge_priority from APPL DB for both.
	if state.BridgeAddress != nil && state.DesignatedRootAddress != nil &&
		strings.EqualFold(*state.BridgeAddress, *state.DesignatedRootAddress) &&
		state.BridgePriority != nil {
		state.DesignatedRootPriority = state.BridgePriority
	}
	if cost, ok := resolveRstpRootCost(appDb, appEntry); ok {
		state.RootCost = &cost
	}
	if !isRstpRootBridge(appEntry) {
		if rootPortName := strings.TrimSpace(appEntry.Get("root_port")); rootPortName != "" {
			if portNum, ok := resolveRstpRootPort(appDb, rootPortName); ok {
				state.RootPort = &portNum
			}
		}
	}
	if val := appEntry.Get("hold_time"); val != "" {
		hold := parseUint8Default(val, "0")
		state.HoldTime = &hold
	}
	if val := appEntry.Get("topology_change_count"); val != "" {
		tc := parseUint64Default(val)
		state.TopologyChanges = &tc
	}
	return nil
}

func fillRstpInterfaceConfig(intf *ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface, cfgDb *db.DB, ifName string) error {
	if intf == nil {
		return nil
	}
	ygot.BuildEmptyTree(intf)
	name := ifName
	intf.Name = &name
	if intf.Config == nil {
		intf.Config = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface_Config{}
	}
	intf.Config.Name = &name

	mstPortEntry, err := cfgDb.GetEntry(&db.TableSpec{Name: STP_MST_PORT_TBL}, db.Key{Comp: []string{mstPortCfgKey(ifName)}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return err
	}
	if mstPortEntry.IsPopulated() {
		if val := mstPortEntry.Get("path_cost"); val != "" {
			cost := parseUint32Default(val, DEFAULT_PATH_COST)
			intf.Config.Cost = &cost
		}
		if val := mstPortEntry.Get("priority"); val != "" {
			priority := parseUint8Default(val, DEFAULT_PORT_PRIORITY)
			intf.Config.PortPriority = &priority
		}
	}
	return nil
}

func fillRstpInterfaceState(intf *ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface, cfgDb, appDb *db.DB, ifName string) error {
	if intf == nil {
		return nil
	}
	if err := fillRstpInterfaceConfig(intf, cfgDb, ifName); err != nil {
		return err
	}
	ygot.BuildEmptyTree(intf)
	if intf.State == nil {
		intf.State = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface_State{}
	}
	name := ifName
	intf.State.Name = &name

	if appDb == nil {
		return nil
	}
	appEntry, err := getApplProducerStateEntry(appDb, STP_MST_PORT_APP_TBL, mstPortAppKey(ifName))
	if err != nil {
		return err
	}
	if !appEntry.IsPopulated() {
		return nil
	}

	if val := appEntry.Get("port_number"); val != "" {
		portNum := parseUint16Default(val, "0")
		intf.State.PortNum = &portNum
	}
	if val := appEntry.Get("priority"); val != "" {
		priority := parseUint8Default(val, DEFAULT_PORT_PRIORITY)
		intf.State.PortPriority = &priority
	}
	if val := appEntry.Get("path_cost"); val != "" {
		cost := parseUint32Default(val, DEFAULT_PATH_COST)
		intf.State.Cost = &cost
	}
	if val := appEntry.Get("port_state"); val != "" {
		intf.State.PortState = mapPortStateToOc(val)
	}
	if val := appEntry.Get("role"); val != "" {
		intf.State.Role = mapPortRoleToOc(val)
	}
	if val := appEntry.Get("desig_cost"); val != "" {
		cost := parseUint32Default(val, "0")
		intf.State.DesignatedCost = &cost
	}
	if root := appEntry.Get("desig_root"); root != "" {
		priority, mac := bridgeIdToMacPriority(root)
		if mac != "" {
			intf.State.DesignatedRootAddress = &mac
		}
		if priority != 0 {
			intf.State.DesignatedRootPriority = &priority
		}
	}
	if bridge := appEntry.Get("desig_bridge"); bridge != "" {
		priority, mac := bridgeIdToMacPriority(bridge)
		if mac != "" {
			intf.State.DesignatedBridgeAddress = &mac
		}
		if priority != 0 {
			intf.State.DesignatedBridgePriority = &priority
		}
	}
	if val := appEntry.Get("desig_port"); val != "" {
		portNum := parseUint16Default(val, "0")
		intf.State.DesignatedPortNum = &portNum
	}
	if val := appEntry.Get("fwd_transitions"); val != "" {
		ft := parseUint64Default(val)
		intf.State.ForwardTransisitions = &ft
	}
	if intf.State.Counters == nil {
		intf.State.Counters = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface_State_Counters{}
	}
	if val := appEntry.Get("bpdu_sent"); val != "" {
		sent := parseUint64Default(val)
		intf.State.Counters.BpduSent = &sent
	}
	if val := appEntry.Get("bpdu_received"); val != "" {
		rcvd := parseUint64Default(val)
		intf.State.Counters.BpduReceived = &rcvd
	}
	return nil
}

func dbLinkTypeToOc(linkType string) ocbinds.E_OpenconfigSpanningTree_StpLinkType {
	switch strings.ToLower(strings.TrimSpace(linkType)) {
	case "point-to-point", "p2p":
		return ocbinds.OpenconfigSpanningTree_StpLinkType_P2P
	case "shared":
		return ocbinds.OpenconfigSpanningTree_StpLinkType_SHARED
	default:
		return ocbinds.OpenconfigSpanningTree_StpLinkType_UNSET
	}
}

func ocLinkTypeToDb(linkType ocbinds.E_OpenconfigSpanningTree_StpLinkType) (string, error) {
	switch linkType {
	case ocbinds.OpenconfigSpanningTree_StpLinkType_P2P:
		return "point-to-point", nil
	case ocbinds.OpenconfigSpanningTree_StpLinkType_SHARED:
		return "shared", nil
	default:
		return "", errors.New("unsupported link-type value")
	}
}

func dbEdgePortToOc(edgePort string) ocbinds.E_OpenconfigSpanningTreeTypes_STP_EDGE_PORT {
	if edgePort == "true" {
		return ocbinds.OpenconfigSpanningTreeTypes_STP_EDGE_PORT_EDGE_ENABLE
	}
	return ocbinds.OpenconfigSpanningTreeTypes_STP_EDGE_PORT_EDGE_DISABLE
}

func ocEdgePortToDb(edgePort ocbinds.E_OpenconfigSpanningTreeTypes_STP_EDGE_PORT) (string, error) {
	switch edgePort {
	case ocbinds.OpenconfigSpanningTreeTypes_STP_EDGE_PORT_EDGE_ENABLE:
		return "true", nil
	case ocbinds.OpenconfigSpanningTreeTypes_STP_EDGE_PORT_EDGE_DISABLE, ocbinds.OpenconfigSpanningTreeTypes_STP_EDGE_PORT_EDGE_AUTO:
		return "false", nil
	default:
		return "", errors.New("unsupported edge-port value")
	}
}

func fillCommonInterfaceConfig(intf *ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface, cfgDb *db.DB, ifName string) error {
	if intf == nil {
		return nil
	}
	ygot.BuildEmptyTree(intf)
	name := ifName
	intf.Name = &name
	if intf.Config == nil {
		intf.Config = &ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface_Config{}
	}
	intf.Config.Name = &name

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: STP_PORT_TBL}, db.Key{Comp: []string{ifName}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return err
	}
	if entry.IsPopulated() {
		intf.Config.EdgePort = dbEdgePortToOc(entry.Get("edge_port"))
		if lt := entry.Get("link_type"); lt != "" && lt != "auto" {
			intf.Config.LinkType = dbLinkTypeToOc(lt)
		}
	}
	return nil
}

func fillCommonInterfaceState(intf *ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface, cfgDb *db.DB, ifName string) error {
	if intf == nil {
		return nil
	}
	if err := fillCommonInterfaceConfig(intf, cfgDb, ifName); err != nil {
		return err
	}
	ygot.BuildEmptyTree(intf)
	if intf.State == nil {
		intf.State = &ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface_State{}
	}
	intf.State.Name = &ifName
	if intf.Config != nil {
		intf.State.EdgePort = intf.Config.EdgePort
		intf.State.LinkType = intf.Config.LinkType
	}
	return nil
}

func listStpPortInterfaces(cfgDb *db.DB) ([]string, error) {
	if cfgDb == nil {
		return nil, nil
	}
	tbl, err := cfgDb.GetTable(&db.TableSpec{Name: STP_PORT_TBL})
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
			names = append(names, key.Comp[0])
		}
	}
	return names, nil
}

func listMstPortInterfacesFromTable(appDb *db.DB, table, prefix string) ([]string, error) {
	keys, err := appDb.GetKeysByPattern(&db.TableSpec{Name: table}, prefix+"*")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		parts := strings.SplitN(key.Comp[0], ":", 2)
		if len(parts) == 2 {
			names = append(names, parts[1])
		}
	}
	return names, nil
}

func listMstPortInterfaces(appDb *db.DB) ([]string, error) {
	prefix := fmt.Sprintf("%d:", RSTP_MSTI_ID)
	if appDb != nil {
		names, err := listMstPortInterfacesFromTable(appDb, applStateTablePrefix+STP_MST_PORT_APP_TBL, prefix)
		if err != nil {
			return nil, err
		}
		if len(names) > 0 {
			return names, nil
		}
		names, err = listMstPortInterfacesFromTable(appDb, STP_MST_PORT_APP_TBL, prefix)
		if err != nil {
			return nil, err
		}
		if len(names) > 0 {
			return names, nil
		}
	}
	client := db.RedisClient(db.ApplDB)
	if client == nil {
		return nil, nil
	}
	keys, err := client.Keys(context.Background(), fmt.Sprintf("*%s:%s*", STP_MST_PORT_APP_TBL, prefix)).Result()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(keys))
	for _, redisKey := range keys {
		if name, ok := interfaceNameFromMstPortRedisKey(redisKey); ok {
			names = append(names, name)
		}
	}
	return names, nil
}

func shouldMapStpGlobalConfigLeaf(targetUriPath, leaf string) bool {
	if targetUriPath == STP_GLOBAL_CONFIG || targetUriPath == STP_GLOBAL {
		return true
	}
	return targetUriPath == STP_GLOBAL_CONFIG+"/"+leaf
}

func shouldMapStpRstpConfigLeaf(targetUriPath, leaf string) bool {
	if targetUriPath == STP_RSTP_CONFIG || targetUriPath == STP_RSTP {
		return true
	}
	return targetUriPath == STP_RSTP_CONFIG+"/"+leaf
}

func enabledProtocolsFromParam(param interface{}) []ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL {
	if param == nil {
		return nil
	}
	val := reflect.ValueOf(param)
	if val.Kind() != reflect.Slice {
		return nil
	}
	protocols := make([]ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL, 0, val.Len())
	for idx := 0; idx < val.Len(); idx++ {
		item := val.Index(idx)
		switch item.Kind() {
		case reflect.Int, reflect.Int32, reflect.Int64:
			protocols = append(protocols, ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL(item.Int()))
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			protocols = append(protocols, ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL(item.Uint()))
		}
	}
	return protocols
}

func ensureStpGlobalConfigTree(stpRoot *ocbinds.OpenconfigSpanningTree_Stp) *ocbinds.OpenconfigSpanningTree_Stp_Global_Config {
	if stpRoot == nil {
		return nil
	}
	ygot.BuildEmptyTree(stpRoot)
	if stpRoot.Global == nil {
		stpRoot.Global = &ocbinds.OpenconfigSpanningTree_Stp_Global{}
	}
	ygot.BuildEmptyTree(stpRoot.Global)
	if stpRoot.Global.Config == nil {
		stpRoot.Global.Config = &ocbinds.OpenconfigSpanningTree_Stp_Global_Config{}
	}
	ygot.BuildEmptyTree(stpRoot.Global.Config)
	return stpRoot.Global.Config
}

func getEnabledProtocols(inParams XfmrParams) []ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL {
	if inParams.ygRoot != nil {
		deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
		ygot.BuildEmptyTree(deviceObj)
		if deviceObj.Stp != nil && deviceObj.Stp.Global != nil &&
			deviceObj.Stp.Global.Config != nil &&
			deviceObj.Stp.Global.Config.EnabledProtocol != nil {
			return deviceObj.Stp.Global.Config.EnabledProtocol
		}
	}
	if protocols := enabledProtocolsFromParam(inParams.param); len(protocols) > 0 {
		return protocols
	}
	stpRoot := getStpRoot(inParams.ygRoot)
	cfg := ensureStpGlobalConfigTree(stpRoot)
	if cfg != nil && cfg.EnabledProtocol != nil {
		return cfg.EnabledProtocol
	}
	return nil
}

func rstpEnableRequested(inParams XfmrParams) bool {
	return hasRstpProtocol(getEnabledProtocols(inParams))
}

func rstpEnableInPayload(inParams XfmrParams) bool {
	if hasRstpProtocol(getEnabledProtocols(inParams)) {
		return true
	}
	if inParams.ygRoot == nil {
		return false
	}
	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	jsonBytes, err := ocbinds.EmitJSON(deviceObj, nil)
	if err != nil {
		return false
	}
	return strings.Contains(string(jsonBytes), "RSTP")
}

func rstpEnableInRequest(inParams XfmrParams) bool {
	if rstpEnableRequested(inParams) {
		return true
	}
	if strings.Contains(inParams.requestUri, "stp/global") && rstpEnableInPayload(inParams) {
		return true
	}
	return false
}

func rstpGlobalConfigSetRequested(inParams XfmrParams) bool {
	if inParams.oper == DELETE {
		return false
	}
	reqPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	return shouldMapStpGlobalConfigLeaf(reqPath, "enabled-protocol")
}

func rstpEnableBootstrapRequested(inParams XfmrParams) bool {
	return rstpEnableInRequest(inParams) || rstpGlobalConfigSetRequested(inParams)
}

func shouldApplyStpGlobalEnable(inParams XfmrParams) bool {
	reqPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	if !shouldMapStpGlobalConfigLeaf(reqPath, "enabled-protocol") {
		return false
	}
	uriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.uri)
	// Skip the virtual /global parent hop when the request targets /global/config.
	if uriPath == STP_GLOBAL && reqPath != STP_GLOBAL {
		return false
	}
	// Leaf UPDATE targets /enabled-protocol; run bootstrap only from /config subtree.
	if strings.HasSuffix(reqPath, "/enabled-protocol") && uriPath != STP_GLOBAL_CONFIG {
		return false
	}
	return true
}

func markStpSubtreeInvoked(inParams XfmrParams) {
	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}
}

func markStpSubtreeOnceWhenPopulated(inParams XfmrParams, res map[string]map[string]db.Value) {
	if inParams.invokeCRUSubtreeOnce == nil {
		return
	}
	for _, keys := range res {
		if len(keys) > 0 {
			*inParams.invokeCRUSubtreeOnce = true
			return
		}
	}
}

func mergeRstpEnableDbMap(dst map[string]map[string]db.Value, cfgDb *db.DB) error {
	enableRstpMsti0(dst)
	return populateMstPortEntries(cfgDb, dst)
}

func stpCfgDb(inParams XfmrParams) *db.DB {
	if inParams.dbs[db.ConfigDB] != nil {
		return inParams.dbs[db.ConfigDB]
	}
	return inParams.d
}

func requireRstpEnabled(cfgDb *db.DB) error {
	enabled, err := isRstpEnabled(cfgDb)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("RSTP is not enabled; configure enabled-protocol first")
	}
	return nil
}

// ---------------- Field transformers ----------------

var YangToDb_stp_enabled_protocol_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	res := make(map[string]string)
	if inParams.oper == DELETE {
		return res, nil
	}
	protocols := getEnabledProtocols(inParams)
	if len(protocols) == 0 {
		return res, errors.New("enabled-protocol is required to configure RSTP")
	}
	if !hasRstpProtocol(protocols) {
		return res, errors.New("only RSTP enabled-protocol is supported")
	}
	res["mode"] = STP_MODE_MST
	return res, nil
}

var DbToYang_stp_enabled_protocol_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	enabled, err := isRstpEnabled(stpCfgDb(inParams))
	if err != nil {
		return res, err
	}
	if enabled {
		res["enabled-protocol"] = []ocbinds.E_OpenconfigSpanningTreeTypes_STP_PROTOCOL{
			ocbinds.OpenconfigSpanningTreeTypes_STP_PROTOCOL_RSTP,
		}
	}
	return res, nil
}

var YangToDb_stp_bridge_priority_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	res := make(map[string]string)
	if inParams.oper == DELETE {
		res["bridge_priority"] = DEFAULT_BRIDGE_PRIORITY
		return res, nil
	}
	if err := requireRstpEnabled(stpCfgDb(inParams)); err != nil {
		return res, err
	}
	stpRoot := getStpRoot(inParams.ygRoot)
	if stpRoot != nil && stpRoot.Rstp != nil && stpRoot.Rstp.Config != nil &&
		stpRoot.Rstp.Config.BridgePriority != nil {
		res["bridge_priority"] = strconv.FormatUint(uint64(*stpRoot.Rstp.Config.BridgePriority), 10)
		return res, nil
	}
	if priority, ok := inParams.param.(*uint32); ok && priority != nil {
		res["bridge_priority"] = strconv.FormatUint(uint64(*priority), 10)
		return res, nil
	}
	return res, errors.New("bridge-priority is required")
}

var DbToYang_stp_bridge_priority_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	entry, err := getMstInstCfgEntry(stpCfgDb(inParams))
	if err != nil {
		return res, err
	}
	priority := parseUint32Default(entry.Get("bridge_priority"), DEFAULT_BRIDGE_PRIORITY)
	res["bridge-priority"] = priority
	return res, nil
}

var YangToDb_stp_edge_port_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	res := make(map[string]string)
	if inParams.oper == DELETE {
		res["edge_port"] = DEFAULT_EDGE_PORT
		return res, nil
	}
	if err := requireRstpEnabled(stpCfgDb(inParams)); err != nil {
		return res, err
	}
	stpRoot := getStpRoot(inParams.ygRoot)
	ifName := NewPathInfo(inParams.uri).Var("name")
	if stpRoot != nil && stpRoot.Interfaces != nil && ifName != "" {
		if intf, ok := stpRoot.Interfaces.Interface[ifName]; ok && intf.Config != nil &&
			intf.Config.EdgePort != ocbinds.OpenconfigSpanningTreeTypes_STP_EDGE_PORT_UNSET {
			edgePort, err := ocEdgePortToDb(intf.Config.EdgePort)
			if err != nil {
				return res, err
			}
			res["edge_port"] = edgePort
			return res, nil
		}
	}
	if edgePort, ok := inParams.param.(ocbinds.E_OpenconfigSpanningTreeTypes_STP_EDGE_PORT); ok {
		val, err := ocEdgePortToDb(edgePort)
		if err != nil {
			return res, err
		}
		res["edge_port"] = val
		return res, nil
	}
	return res, errors.New("edge-port is required")
}

var DbToYang_stp_edge_port_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	if inParams.key == "" {
		return res, nil
	}
	entry, err := stpCfgDb(inParams).GetEntry(&db.TableSpec{Name: STP_PORT_TBL}, db.Key{Comp: []string{inParams.key}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return res, err
	}
	if entry.IsPopulated() {
		res["edge-port"] = dbEdgePortToOc(entry.Get("edge_port"))
	}
	return res, nil
}

var YangToDb_stp_link_type_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	res := make(map[string]string)
	if inParams.oper == DELETE {
		res["link_type"] = DEFAULT_LINK_TYPE
		return res, nil
	}
	if err := requireRstpEnabled(stpCfgDb(inParams)); err != nil {
		return res, err
	}
	stpRoot := getStpRoot(inParams.ygRoot)
	ifName := NewPathInfo(inParams.uri).Var("name")
	if stpRoot != nil && stpRoot.Interfaces != nil && ifName != "" {
		if intf, ok := stpRoot.Interfaces.Interface[ifName]; ok && intf.Config != nil &&
			intf.Config.LinkType != ocbinds.OpenconfigSpanningTree_StpLinkType_UNSET {
			linkType, err := ocLinkTypeToDb(intf.Config.LinkType)
			if err != nil {
				return res, err
			}
			res["link_type"] = linkType
			return res, nil
		}
	}
	if linkType, ok := inParams.param.(ocbinds.E_OpenconfigSpanningTree_StpLinkType); ok {
		val, err := ocLinkTypeToDb(linkType)
		if err != nil {
			return res, err
		}
		res["link_type"] = val
		return res, nil
	}
	return res, errors.New("link-type is required")
}

var DbToYang_stp_link_type_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	if inParams.key == "" {
		return res, nil
	}
	entry, err := stpCfgDb(inParams).GetEntry(&db.TableSpec{Name: STP_PORT_TBL}, db.Key{Comp: []string{inParams.key}})
	if err != nil && !tlerr.IsTranslibRedisClientEntryNotExist(err) {
		return res, err
	}
	if entry.IsPopulated() {
		if lt := entry.Get("link_type"); lt != "" && lt != "auto" {
			res["link-type"] = dbLinkTypeToOc(lt)
		}
	}
	return res, nil
}

var stp_post_xfmr PostXfmrFunc = func(inParams XfmrParams) error {
	if inParams.dbDataMap == nil || inParams.oper == DELETE {
		return nil
	}
	retDbDataMap := (*inParams.dbDataMap)[db.ConfigDB]
	if retDbDataMap == nil {
		retDbDataMap = make(map[string]map[string]db.Value)
		(*inParams.dbDataMap)[db.ConfigDB] = retDbDataMap
	}

	needEnable := rstpEnableBootstrapRequested(inParams)
	if !needEnable {
		if stpTbl, ok := retDbDataMap[STP_GLOBAL_TBL]; ok {
			if entry, ok := stpTbl[STP_GLOBAL_KEY]; ok && entry.Get("mode") == STP_MODE_MST {
				needEnable = true
			}
		}
	}
	if !needEnable {
		return nil
	}

	tempRes := make(map[string]map[string]db.Value)
	var err error
	if rstpEnableBootstrapRequested(inParams) {
		err = applyRstpEnableToDbMap(tempRes, inParams)
	} else {
		err = mergeRstpEnableDbMap(tempRes, stpCfgDb(inParams))
	}
	if err != nil {
		return err
	}
	for tbl, keys := range tempRes {
		if _, ok := retDbDataMap[tbl]; !ok {
			retDbDataMap[tbl] = make(map[string]db.Value)
		}
		for key, val := range keys {
			if tbl == STP_GLOBAL_TBL && key == STP_GLOBAL_KEY {
				if existing, ok := retDbDataMap[tbl][key]; ok {
					for field, fieldVal := range val.Field {
						existing.Field[field] = fieldVal
					}
					retDbDataMap[tbl][key] = existing
					continue
				}
			}
			retDbDataMap[tbl][key] = val
		}
	}
	stripStpGlobalYangDefaultsForMst(inParams.yangDefValMap)
	return nil
}

// ---------------- Global (/stp/global) ----------------

var DbToYang_stp_global_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	stpRoot := getStpRoot(inParams.ygRoot)
	if stpRoot == nil {
		return errors.New("stp root is nil")
	}
	ygot.BuildEmptyTree(stpRoot)
	if stpRoot.Global == nil {
		stpRoot.Global = &ocbinds.OpenconfigSpanningTree_Stp_Global{}
	}
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	cfgDb := inParams.dbs[db.ConfigDB]

	if strings.HasPrefix(targetUriPath, STP_GLOBAL_STATE) || targetUriPath == STP_GLOBAL {
		if stpRoot.Global.State == nil {
			stpRoot.Global.State = &ocbinds.OpenconfigSpanningTree_Stp_Global_State{}
		}
		return fillGlobalStateFromDb(stpRoot.Global.State, cfgDb)
	}
	if stpRoot.Global.Config == nil {
		stpRoot.Global.Config = &ocbinds.OpenconfigSpanningTree_Stp_Global_Config{}
	}
	return fillGlobalConfigFromDb(stpRoot.Global.Config, cfgDb)
}

func applyRstpEnableToDbMap(res map[string]map[string]db.Value, inParams XfmrParams) error {
	protocols := getEnabledProtocols(inParams)
	if len(protocols) == 0 {
		if rstpEnableInPayload(inParams) || rstpGlobalConfigSetRequested(inParams) {
			return mergeRstpEnableDbMap(res, stpCfgDb(inParams))
		}
		return errors.New("enabled-protocol is required to configure RSTP")
	}
	if !hasRstpProtocol(protocols) {
		return errors.New("only RSTP enabled-protocol is supported")
	}
	return mergeRstpEnableDbMap(res, stpCfgDb(inParams))
}

var YangToDb_stp_global_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)

	if inParams.oper == DELETE {
		if targetUriPath == STP_GLOBAL_CONFIG || strings.HasSuffix(targetUriPath, "/enabled-protocol") {
			disableRstpMsti0(res)
			markStpSubtreeInvoked(inParams)
			return res, nil
		}
		return res, errors.New("DELETE not supported on attribute")
	}

	if !shouldApplyStpGlobalEnable(inParams) {
		return res, nil
	}
	if !rstpEnableBootstrapRequested(inParams) {
		return res, nil
	}
	if err := applyRstpEnableToDbMap(res, inParams); err != nil {
		return res, err
	}
	markStpSubtreeOnceWhenPopulated(inParams, res)
	return res, nil
}

var Subscribe_stp_global_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	if inParams.subscProc == TRANSLATE_EXISTS {
		result.isVirtualTbl = true
		return result, nil
	}
	if inParams.subscProc == TRANSLATE_SUBSCRIBE {
		enabled, err := isRstpEnabled(inParams.dbs[db.ConfigDB])
		if err != nil {
			return result, err
		}
		if !enabled {
			result.isVirtualTbl = true
			return result, nil
		}
	}
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		STP_GLOBAL_TBL: {STP_GLOBAL_KEY: {"mode": ""}},
	}
	result.isVirtualTbl = false
	return result, nil
}

// ---------------- RSTP (/stp/rstp) ----------------

var DbToYang_stp_rstp_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	stpRoot := getStpRoot(inParams.ygRoot)
	if stpRoot == nil {
		return errors.New("stp root is nil")
	}
	ygot.BuildEmptyTree(stpRoot)
	if stpRoot.Rstp == nil {
		stpRoot.Rstp = &ocbinds.OpenconfigSpanningTree_Stp_Rstp{}
	}
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	cfgDb := inParams.dbs[db.ConfigDB]
	appDb := inParams.dbs[db.ApplDB]

	if strings.HasPrefix(targetUriPath, STP_RSTP_STATE) || targetUriPath == STP_RSTP {
		if stpRoot.Rstp.State == nil {
			stpRoot.Rstp.State = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_State{}
		}
		return fillRstpStateFromDb(stpRoot.Rstp.State, cfgDb, appDb)
	}
	if stpRoot.Rstp.Config == nil {
		stpRoot.Rstp.Config = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Config{}
	}
	return fillRstpConfigFromDb(stpRoot.Rstp.Config, cfgDb)
}

var YangToDb_stp_rstp_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	stpRoot := getStpRoot(inParams.ygRoot)
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)

	if inParams.oper == DELETE {
		if strings.HasSuffix(targetUriPath, "/bridge-priority") {
			mstInstMap := map[string]db.Value{
				mstInstCfgKey(): {Field: map[string]string{"bridge_priority": DEFAULT_BRIDGE_PRIORITY}},
			}
			res[STP_MST_INST_TBL] = mstInstMap
			return res, nil
		}
		return res, errors.New("DELETE not supported on attribute")
	}

	if !shouldMapStpRstpConfigLeaf(targetUriPath, "bridge-priority") {
		return res, nil
	}
	if stpRoot == nil || stpRoot.Rstp == nil || stpRoot.Rstp.Config == nil {
		return res, errors.New("missing rstp config in request")
	}
	if stpRoot.Rstp.Config.BridgePriority == nil {
		return res, errors.New("only bridge-priority configuration is supported under /stp/rstp/config")
	}
	if stpRoot.Rstp.Config.HelloTime != nil || stpRoot.Rstp.Config.MaxAge != nil ||
		stpRoot.Rstp.Config.ForwardingDelay != nil || stpRoot.Rstp.Config.HoldCount != nil {
		return res, errors.New("timer configuration is not supported via OpenConfig RSTP; backend defaults apply")
	}

	enabled, err := isRstpEnabled(inParams.dbs[db.ConfigDB])
	if err != nil {
		return res, err
	}
	if !enabled {
		return res, errors.New("RSTP is not enabled; configure enabled-protocol first")
	}

	priority := strconv.FormatUint(uint64(*stpRoot.Rstp.Config.BridgePriority), 10)
	res[STP_MST_INST_TBL] = map[string]db.Value{
		mstInstCfgKey(): {Field: map[string]string{"bridge_priority": priority}},
	}
	return res, nil
}

var Subscribe_stp_rstp_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		STP_MST_TBL:      {STP_MST_GLOBAL_KEY: {"hello_time": "", "max_age": "", "forward_delay": "", "hold_count": ""}},
		STP_MST_INST_TBL: {mstInstCfgKey(): {"bridge_priority": ""}},
	}
	result.dbDataMap[db.ApplDB] = map[string]map[string]map[string]string{
		STP_MST_INST_APP_TBL: {mstInstCfgKey(): {"bridge_address": "", "bridge_priority": "", "root_address": "", "root_path_cost": "", "root_port": "", "hold_time": "", "topology_change_count": ""}},
		STP_MST_PORT_APP_TBL: {mstPortAppKey("*"): {"path_cost": "", "desig_cost": "", "external_cost": "", "role": ""}},
	}
	return result, nil
}

// ---------------- RSTP interfaces (/stp/rstp/interfaces) ----------------

var DbToYang_stp_rstp_intf_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	stpRoot := getStpRoot(inParams.ygRoot)
	if stpRoot == nil {
		return errors.New("stp root is nil")
	}
	ygot.BuildEmptyTree(stpRoot)
	intfMap := ensureStpRstpInterfaces(stpRoot)
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")

	cfgDb := inParams.dbs[db.ConfigDB]
	appDb := inParams.dbs[db.ApplDB]

	if ifName != "" {
		intf, ok := intfMap[ifName]
		if !ok {
			intf = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface{}
			intfMap[ifName] = intf
		}
		if strings.Contains(targetUriPath, "/config") && !strings.Contains(targetUriPath, "/state") {
			return fillRstpInterfaceConfig(intf, cfgDb, ifName)
		}
		return fillRstpInterfaceState(intf, cfgDb, appDb, ifName)
	}

	ifNames, err := listStpPortInterfaces(cfgDb)
	if err != nil {
		return err
	}
	if len(ifNames) == 0 {
		ifNames, err = listMstPortInterfaces(appDb)
		if err != nil {
			return err
		}
	}
	fillState := strings.Contains(targetUriPath, "/state") ||
		targetUriPath == STP_RSTP_INTERFACES ||
		targetUriPath == STP_RSTP ||
		strings.HasSuffix(targetUriPath, "/interface")
	for _, name := range ifNames {
		intf, ok := intfMap[name]
		if !ok {
			intf = &ocbinds.OpenconfigSpanningTree_Stp_Rstp_Interfaces_Interface{}
			intfMap[name] = intf
		}
		if fillState {
			if err := fillRstpInterfaceState(intf, cfgDb, appDb, name); err != nil {
				return err
			}
		} else {
			if err := fillRstpInterfaceConfig(intf, cfgDb, name); err != nil {
				return err
			}
		}
	}
	return nil
}

var YangToDb_stp_rstp_intf_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	return nil, errors.New("RSTP interface configuration is read-only; use /stp/interfaces for edge-port and link-type")
}

var Subscribe_stp_rstp_intf_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ApplDB] = map[string]map[string]map[string]string{
		STP_MST_PORT_APP_TBL: {mstPortAppKey("*"): {"*": ""}},
	}
	return result, nil
}

var DbToYangPath_stp_rstp_intf_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != STP_MST_PORT_APP_TBL {
		return errors.New("wrong APPL_DB table for RSTP interface path xfmr")
	}
	if len(params.tblKeyComp) == 0 {
		return errors.New("invalid STP_MST_PORT_TABLE key")
	}
	parts := strings.SplitN(params.tblKeyComp[0], ":", 2)
	if len(parts) != 2 {
		return errors.New("invalid STP_MST_PORT_TABLE key format")
	}
	params.ygPathKeys[STP_RSTP_INTERFACE+"/name"] = parts[1]
	return nil
}

var YangToDb_stp_rstp_intf_key_xfmr KeyXfmrYangToDb = func(inParams XfmrParams) (string, error) {
	ifName := NewPathInfo(inParams.uri).Var("name")
	if ifName == "" {
		ifName = NewPathInfo(inParams.requestUri).Var("name")
	}
	if ifName == "" {
		return "", errors.New("interface name is required")
	}
	if ifName == "*" {
		return fmt.Sprintf("%d:*", RSTP_MSTI_ID), nil
	}
	return mstPortAppKey(ifName), nil
}

var DbToYang_stp_rstp_intf_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	parts := strings.SplitN(inParams.key, ":", 2)
	if len(parts) == 2 {
		res["name"] = parts[1]
	} else {
		res["name"] = inParams.key
	}
	return res, nil
}

// ---------------- Common interfaces (/stp/interfaces) ----------------

var DbToYang_stp_intf_common_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	stpRoot := getStpRoot(inParams.ygRoot)
	if stpRoot == nil {
		return errors.New("stp root is nil")
	}
	ygot.BuildEmptyTree(stpRoot)
	intfMap := ensureStpCommonInterfaces(stpRoot)
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)
	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")
	cfgDb := inParams.dbs[db.ConfigDB]

	if ifName != "" {
		intf, ok := intfMap[ifName]
		if !ok {
			intf = &ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface{}
			intfMap[ifName] = intf
		}
		if strings.Contains(targetUriPath, "/state") {
			return fillCommonInterfaceState(intf, cfgDb, ifName)
		}
		return fillCommonInterfaceConfig(intf, cfgDb, ifName)
	}

	ifNames, err := listStpPortInterfaces(cfgDb)
	if err != nil {
		return err
	}
	for _, name := range ifNames {
		intf, ok := intfMap[name]
		if !ok {
			intf = &ocbinds.OpenconfigSpanningTree_Stp_Interfaces_Interface{}
			intfMap[name] = intf
		}
		if strings.Contains(targetUriPath, "/state") || targetUriPath == STP_INTF_COMMON {
			if err := fillCommonInterfaceState(intf, cfgDb, name); err != nil {
				return err
			}
		} else {
			if err := fillCommonInterfaceConfig(intf, cfgDb, name); err != nil {
				return err
			}
		}
	}
	return nil
}

var YangToDb_stp_intf_common_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	stpRoot := getStpRoot(inParams.ygRoot)
	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")
	targetUriPath, _, _ := XfmrRemoveXPATHPredicates(inParams.requestUri)

	if ifName == "" {
		return res, errors.New("interface name is required")
	}

	enabled, err := isRstpEnabled(inParams.dbs[db.ConfigDB])
	if err != nil {
		return res, err
	}
	if !enabled {
		return res, errors.New("RSTP is not enabled; configure enabled-protocol first")
	}

	portMap := make(map[string]db.Value)
	fields := make(map[string]string)

	if inParams.oper == DELETE {
		switch {
		case strings.HasSuffix(targetUriPath, "/edge-port"):
			fields["edge_port"] = DEFAULT_EDGE_PORT
		case strings.HasSuffix(targetUriPath, "/link-type"):
			fields["link_type"] = DEFAULT_LINK_TYPE
		default:
			return res, errors.New("DELETE not supported on attribute")
		}
		portMap[ifName] = db.Value{Field: fields}
		res[STP_PORT_TBL] = portMap
		return res, nil
	}

	if stpRoot == nil || stpRoot.Interfaces == nil {
		return res, errors.New("missing interface config in request")
	}
	intf, ok := stpRoot.Interfaces.Interface[ifName]
	if !ok || intf.Config == nil {
		return res, errors.New("missing interface config in request")
	}

	if intf.Config.EdgePort != ocbinds.OpenconfigSpanningTreeTypes_STP_EDGE_PORT_UNSET {
		edgePort, err := ocEdgePortToDb(intf.Config.EdgePort)
		if err != nil {
			return res, err
		}
		fields["edge_port"] = edgePort
	}
	if intf.Config.LinkType != ocbinds.OpenconfigSpanningTree_StpLinkType_UNSET {
		linkType, err := ocLinkTypeToDb(intf.Config.LinkType)
		if err != nil {
			return res, err
		}
		fields["link_type"] = linkType
	}
	if len(fields) == 0 {
		return res, errors.New("only edge-port and link-type configuration are supported under /stp/interfaces")
	}

	portMap[ifName] = db.Value{Field: fields}
	res[STP_PORT_TBL] = portMap
	return res, nil
}

var Subscribe_stp_intf_common_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		STP_PORT_TBL: {"*": {"edge_port": "", "link_type": ""}},
	}
	return result, nil
}

var DbToYangPath_stp_intf_common_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != STP_PORT_TBL {
		return errors.New("wrong CONFIG_DB table for STP interface path xfmr")
	}
	if len(params.tblKeyComp) == 0 {
		return errors.New("invalid STP_PORT key")
	}
	params.ygPathKeys[STP_INTF_COMMON_INTF+"/name"] = params.tblKeyComp[0]
	return nil
}

// Silence unused import when log level is high.
var _ = log.V(3)
