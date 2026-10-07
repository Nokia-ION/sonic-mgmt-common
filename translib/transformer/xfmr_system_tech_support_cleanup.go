package transformer

import (
	"errors"
	"strconv"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	log "github.com/golang/glog"
)

const autoTsMaxTechsupportLimitField = "max_techsupport_limit"

func init() {
	XlateFuncBind("YangToDb_tech_support_cleanup_xfmr", YangToDb_tech_support_cleanup_xfmr)
	XlateFuncBind("DbToYang_tech_support_cleanup_xfmr", DbToYang_tech_support_cleanup_xfmr)
	XlateFuncBind("Subscribe_tech_support_cleanup_xfmr", Subscribe_tech_support_cleanup_xfmr)
}

// YangToDb_tech_support_cleanup_xfmr maps /system/tech-support-cleanup/config
// (openconfig-system-ext) to CONFIG_DB AUTO_TECHSUPPORT|GLOBAL:max_techsupport_limit.
// generate_dump runs techsupport_cleanup.py after every dump, which reads that
// field and deletes the oldest /var/dump files until they fit under the limit.
var YangToDb_tech_support_cleanup_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("Inside YangToDb_tech_support_cleanup_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)

	if inParams.oper == DELETE {
		// max-techsupport-limit is the only config leaf; leave the rest of the
		// AUTO_TECHSUPPORT|GLOBAL row (state, core limit) untouched.
		subOpMap := map[db.DBNum]map[string]map[string]db.Value{
			db.ConfigDB: {
				AUTO_TECHSUPPORT_TABLE_NAME: {
					AUTO_TECHSUPPORT_GLOBAL_KEY: {Field: map[string]string{autoTsMaxTechsupportLimitField: autoTsLimitDisabled}},
				},
			},
		}
		updateSubOpDataMap(subOpMap, UPDATE, inParams)
		return res, nil
	}

	sysObj := getAppRootObject(inParams)
	if sysObj == nil || sysObj.TechSupportCleanup == nil || sysObj.TechSupportCleanup.Config == nil {
		return res, nil
	}
	cfg := sysObj.TechSupportCleanup.Config

	if cfg.MaxTechsupportLimit == nil {
		return res, nil
	}
	val := db.Value{Field: map[string]string{autoTsMaxTechsupportLimitField: formatAutoTsLimit(*cfg.MaxTechsupportLimit)}}
	res[AUTO_TECHSUPPORT_TABLE_NAME] = map[string]db.Value{AUTO_TECHSUPPORT_GLOBAL_KEY: val}
	return res, nil
}

// DbToYang_tech_support_cleanup_xfmr populates /system/tech-support-cleanup
// config and state from CONFIG_DB AUTO_TECHSUPPORT|GLOBAL.
var DbToYang_tech_support_cleanup_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_tech_support_cleanup_xfmr URI: %s", inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.TechSupportCleanup == nil {
		deviceObj.System.TechSupportCleanup = &ocbinds.OpenconfigSystem_System_TechSupportCleanup{}
	}
	cleanup := deviceObj.System.TechSupportCleanup
	if cleanup.Config == nil {
		cleanup.Config = &ocbinds.OpenconfigSystem_System_TechSupportCleanup_Config{}
	}
	if cleanup.State == nil {
		cleanup.State = &ocbinds.OpenconfigSystem_System_TechSupportCleanup_State{}
	}

	entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: AUTO_TECHSUPPORT_TABLE_NAME}, db.Key{Comp: []string{AUTO_TECHSUPPORT_GLOBAL_KEY}})
	if err != nil {
		// No AUTO_TECHSUPPORT|GLOBAL row configured yet; nothing to return.
		return nil
	}

	if v := entry.Get(autoTsMaxTechsupportLimitField); v != "" {
		if f, e := strconv.ParseFloat(v, 64); e == nil {
			cfgVal, stateVal := f, f
			cleanup.Config.MaxTechsupportLimit = &cfgVal
			cleanup.State.MaxTechsupportLimit = &stateVal
		}
	}
	if v := entry.Get(autoTsStateField); v != "" {
		enabled := v == autoTsStateEnabled
		cleanup.State.Enabled = &enabled
	}
	return nil
}

// Subscribe_tech_support_cleanup_xfmr maps the subtree to AUTO_TECHSUPPORT|GLOBAL.
var Subscribe_tech_support_cleanup_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		AUTO_TECHSUPPORT_TABLE_NAME: {AUTO_TECHSUPPORT_GLOBAL_KEY: {}},
	}
	return result, nil
}
