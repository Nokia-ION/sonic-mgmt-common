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
	"math"
	"strconv"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	stormCtrlTbl       = "PORT_STORM_CONTROL"
	stormCtrlKbpsFld   = "kbps"
	stormCtrlPortTbl   = "PORT"
	stormCtrlMaxKbps   = 100000000
	stormCtrlIntfRoot  = "/openconfig-interfaces:interfaces/interface"
	stormCtrlEntryPath = stormCtrlIntfRoot + "/openconfig-if-ethernet:ethernet" +
		"/openconfig-storm-control-ext:storm-control/entries/entry"
)

type stormCtrlEntriesObj = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Ethernet_StormControl_Entries
type stormCtrlEntryCfgObj = ocbinds.OpenconfigInterfaces_Interfaces_Interface_Ethernet_StormControl_Entries_Entry_Config
type stormCtrlTypeEnum = ocbinds.E_OpenconfigStormControlExt_StormControlType

// The storm types, in their three spellings: the ygot enum, the yang enum name
// as it appears in a gNMI list key, and the CONFIG_DB storm_type. PolicerOrch
// binds each db value to its own SAI port attribute - broadcast to BROADCAST,
// unknown-unicast to FLOOD and unknown-multicast to MULTICAST.
var stormCtrlTypes = []struct {
	ocType stormCtrlTypeEnum
	ocName string
	dbName string
}{
	{ocbinds.OpenconfigStormControlExt_StormControlType_BROADCAST, "BROADCAST", "broadcast"},
	{ocbinds.OpenconfigStormControlExt_StormControlType_UNKNOWN_UNICAST, "UNKNOWN_UNICAST", "unknown-unicast"},
	{ocbinds.OpenconfigStormControlExt_StormControlType_UNKNOWN_MULTICAST, "UNKNOWN_MULTICAST", "unknown-multicast"},
}

var (
	stormCtrlTypeToDb   = map[stormCtrlTypeEnum]string{}
	stormCtrlTypeFromDb = map[string]stormCtrlTypeEnum{}
	stormCtrlNameToDb   = map[string]string{}
	stormCtrlDbToName   = map[string]string{}
)

func init() {
	for _, t := range stormCtrlTypes {
		stormCtrlTypeToDb[t.ocType] = t.dbName
		stormCtrlTypeFromDb[t.dbName] = t.ocType
		stormCtrlNameToDb[t.ocName] = t.dbName
		stormCtrlDbToName[t.dbName] = t.ocName
	}

	XlateFuncBind("YangToDb_storm_control_xfmr", YangToDb_storm_control_xfmr)
	XlateFuncBind("DbToYang_storm_control_xfmr", DbToYang_storm_control_xfmr)
	XlateFuncBind("Subscribe_storm_control_xfmr", Subscribe_storm_control_xfmr)
	XlateFuncBind("DbToYangPath_storm_control_path_xfmr", DbToYangPath_storm_control_path_xfmr)
}

// stormCtrlTypeFromUri resolves the storm-type list key as it appears in a
// gNMI path, e.g. entry[storm-type=BROADCAST], to its CONFIG_DB spelling.
func stormCtrlTypeFromUri(uriType string) (string, error) {
	if dbName, ok := stormCtrlNameToDb[uriType]; ok {
		return dbName, nil
	}
	return "", tlerr.InvalidArgs("Invalid storm-control type %s", uriType)
}

func stormCtrlValidateIntf(ifName string) error {
	intfType, _, err := getIntfTypeByName(ifName)
	if err != nil {
		return tlerr.InvalidArgs("Invalid interface name %s", ifName)
	}
	// PolicerOrch only accepts physical ports; it logs "Unsupported" and drops
	// the request for LAG and VLAN interfaces.
	if intfType != IntfTypeEthernet {
		return tlerr.NotSupported("Storm control is supported on physical interfaces only, not %s", ifName)
	}
	return nil
}

// stormCtrlPortSpeedKbps returns the configured line rate of ifName in kbps.
// The PORT table holds it in Mbps.
func stormCtrlPortSpeedKbps(d *db.DB, ifName string) (uint64, error) {
	portEntry, err := d.GetEntry(&db.TableSpec{Name: stormCtrlPortTbl}, db.Key{Comp: []string{ifName}})
	if err != nil {
		return 0, tlerr.InvalidArgs("Interface %s not found", ifName)
	}
	speedMbps, err := strconv.ParseUint(portEntry.Get(PORT_SPEED), 10, 64)
	if err != nil || speedMbps == 0 {
		return 0, tlerr.InvalidArgs("Speed of interface %s is unknown, configure 'kbps' instead of 'level'", ifName)
	}
	return speedMbps * 1000, nil
}

// stormCtrlRateToKbps resolves the configured rate to the absolute kbps value
// PolicerOrch expects. A level is a percentage of the port's line rate.
func stormCtrlRateToKbps(d *db.DB, ifName string, cfg *stormCtrlEntryCfgObj) (uint64, error) {
	if cfg.Kbps != nil && cfg.Level != nil {
		return 0, tlerr.InvalidArgs("Storm control 'level' and 'kbps' are mutually exclusive")
	}

	if cfg.Kbps != nil {
		return *cfg.Kbps, nil
	}

	if cfg.Level == nil {
		return 0, tlerr.InvalidArgs("Storm control requires either 'level' or 'kbps'")
	}

	speedKbps, err := stormCtrlPortSpeedKbps(d, ifName)
	if err != nil {
		return 0, err
	}

	kbps := uint64(math.Round(*cfg.Level * float64(speedKbps) / 100))
	if kbps > stormCtrlMaxKbps {
		return 0, tlerr.InvalidArgs("Storm control level %v%% of interface %s resolves to %d kbps, above the supported maximum of %d kbps",
			*cfg.Level, ifName, kbps, uint64(stormCtrlMaxKbps))
	}
	return kbps, nil
}

func stormCtrlEntriesFromRoot(ygRoot *ygot.GoStruct, ifName string) *stormCtrlEntriesObj {
	intfsObj := getIntfsRoot(ygRoot)
	if intfsObj == nil {
		return nil
	}
	intfObj, ok := intfsObj.Interface[ifName]
	if !ok || intfObj == nil || intfObj.Ethernet == nil || intfObj.Ethernet.StormControl == nil {
		return nil
	}
	return intfObj.Ethernet.StormControl.Entries
}

var YangToDb_storm_control_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	entryMap := make(map[string]db.Value)

	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")
	if ifName == "" {
		return nil, tlerr.InvalidArgs("Interface name is required for storm control")
	}
	if err := stormCtrlValidateIntf(ifName); err != nil {
		return nil, err
	}

	/* Set invokeCRUSubtreeOnce flag to invoke subtree once */
	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}

	log.V(3).Infof("YangToDb_storm_control_xfmr: oper %v uri %s", inParams.oper, inParams.uri)

	// A rate limit with no rate is meaningless, so a delete anywhere at or
	// below an entry removes the whole row and with it the policer.
	if inParams.oper == DELETE {
		if uriType := pathInfo.Var("storm-type"); uriType != "" {
			dbType, err := stormCtrlTypeFromUri(uriType)
			if err != nil {
				return nil, err
			}
			entryMap[ifName+"|"+dbType] = db.Value{Field: make(map[string]string)}
		} else {
			for _, dbType := range stormCtrlTypeToDb {
				entryMap[ifName+"|"+dbType] = db.Value{Field: make(map[string]string)}
			}
		}
		resMap[stormCtrlTbl] = entryMap
		return resMap, nil
	}

	entries := stormCtrlEntriesFromRoot(inParams.ygRoot, ifName)
	if entries == nil || len(entries.Entry) == 0 {
		log.V(3).Infof("YangToDb_storm_control_xfmr: no storm control entries for %s", ifName)
		return resMap, nil
	}

	for ocType, entry := range entries.Entry {
		dbType, ok := stormCtrlTypeToDb[ocType]
		if !ok {
			return nil, tlerr.InvalidArgs("Invalid storm-control type %v", ocType)
		}
		if entry == nil || entry.Config == nil {
			continue
		}

		kbps, err := stormCtrlRateToKbps(inParams.dbs[db.ConfigDB], ifName, entry.Config)
		if err != nil {
			return nil, err
		}

		dbValue := db.Value{Field: make(map[string]string)}
		dbValue.Field[stormCtrlKbpsFld] = strconv.FormatUint(kbps, 10)
		entryMap[ifName+"|"+dbType] = dbValue
	}

	resMap[stormCtrlTbl] = entryMap
	return resMap, nil
}

// stormCtrlFillEntry populates the config and state containers of one entry
// from its CONFIG_DB row. speedKbps of 0 means the line rate is unknown, in
// which case the equivalent level cannot be reported.
func stormCtrlFillEntry(entries *stormCtrlEntriesObj, ocType stormCtrlTypeEnum, kbps uint64, speedKbps uint64) error {
	entry, found := entries.Entry[ocType]
	if !found {
		var err error
		entry, err = entries.NewEntry(ocType)
		if err != nil {
			return err
		}
	}
	ygot.BuildEmptyTree(entry)

	entry.StormType = ocType
	fill := func(stormType *stormCtrlTypeEnum, level **float64, rate **uint64) {
		*stormType = ocType
		rateVal := kbps
		*rate = &rateVal
		if speedKbps != 0 {
			pct := float64(kbps) * 100 / float64(speedKbps)
			*level = &pct
		}
	}
	fill(&entry.Config.StormType, &entry.Config.Level, &entry.Config.Kbps)
	fill(&entry.State.StormType, &entry.State.Level, &entry.State.Kbps)

	return nil
}

var DbToYang_storm_control_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")
	if ifName == "" {
		return tlerr.InvalidArgs("Interface name is required for storm control")
	}
	if err := stormCtrlValidateIntf(ifName); err != nil {
		return err
	}

	intfsObj := getIntfsRoot(inParams.ygRoot)
	if intfsObj == nil {
		return tlerr.InvalidArgs("Nil root object received for storm control Get")
	}
	intfObj, ok := intfsObj.Interface[ifName]
	if !ok || intfObj == nil {
		return tlerr.InvalidArgs("Interface %s not found", ifName)
	}
	ygot.BuildEmptyTree(intfObj)
	ygot.BuildEmptyTree(intfObj.Ethernet)
	ygot.BuildEmptyTree(intfObj.Ethernet.StormControl)
	entries := intfObj.Ethernet.StormControl.Entries

	d := inParams.dbs[db.ConfigDB]

	// The line rate is only needed to report the equivalent level; an unknown
	// speed must not fail the whole Get.
	speedKbps, err := stormCtrlPortSpeedKbps(d, ifName)
	if err != nil {
		log.V(3).Infof("DbToYang_storm_control_xfmr: speed of %s unknown, omitting level: %v", ifName, err)
		speedKbps = 0
	}

	dbTypeFilter := ""
	if uriType := pathInfo.Var("storm-type"); uriType != "" {
		if dbTypeFilter, err = stormCtrlTypeFromUri(uriType); err != nil {
			return err
		}
	}

	keys, err := d.GetKeysPattern(&db.TableSpec{Name: stormCtrlTbl}, db.Key{Comp: []string{ifName, "*"}})
	if err != nil {
		log.V(3).Infof("DbToYang_storm_control_xfmr: no storm control config for %s: %v", ifName, err)
		return nil
	}

	for _, key := range keys {
		if len(key.Comp) < 2 {
			continue
		}
		dbType := key.Get(1)
		if dbTypeFilter != "" && dbType != dbTypeFilter {
			continue
		}
		ocType, ok := stormCtrlTypeFromDb[dbType]
		if !ok {
			log.Warningf("DbToYang_storm_control_xfmr: unknown storm type %s on %s", dbType, ifName)
			continue
		}

		dbEntry, err := d.GetEntry(&db.TableSpec{Name: stormCtrlTbl}, key)
		if err != nil {
			return err
		}
		kbps, err := strconv.ParseUint(dbEntry.Get(stormCtrlKbpsFld), 10, 64)
		if err != nil {
			log.Warningf("DbToYang_storm_control_xfmr: invalid kbps for %s|%s", ifName, dbType)
			continue
		}

		if err := stormCtrlFillEntry(entries, ocType, kbps, speedKbps); err != nil {
			return err
		}
	}

	return nil
}

var Subscribe_storm_control_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("name")
	if ifName == "" {
		ifName = "*"
	}

	dbType := "*"
	if uriType := pathInfo.Var("storm-type"); uriType != "" {
		var err error
		if dbType, err = stormCtrlTypeFromUri(uriType); err != nil {
			return result, err
		}
	}

	result.dbDataMap = RedisDbSubscribeMap{db.ConfigDB: {stormCtrlTbl: {ifName + "|" + dbType: {
		stormCtrlKbpsFld: "kbps"}}}}
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	result.isVirtualTbl = false

	return result, nil
}

var DbToYangPath_storm_control_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != stormCtrlTbl {
		log.V(3).Infof("DbToYangPath_storm_control_path_xfmr: unsupported table %s", params.tblName)
		return nil
	}
	if len(params.tblKeyComp) < 2 {
		return tlerr.InvalidArgs("Invalid %s key %v", stormCtrlTbl, params.tblKeyComp)
	}

	ocName, ok := stormCtrlDbToName[params.tblKeyComp[1]]
	if !ok {
		return tlerr.InvalidArgs("Invalid storm type %s", params.tblKeyComp[1])
	}

	params.ygPathKeys[stormCtrlIntfRoot+"/name"] = params.tblKeyComp[0]
	params.ygPathKeys[stormCtrlEntryPath+"/storm-type"] = ocName

	log.V(3).Info("DbToYangPath_storm_control_path_xfmr: ygPathKeys ", params.ygPathKeys)
	return nil
}
