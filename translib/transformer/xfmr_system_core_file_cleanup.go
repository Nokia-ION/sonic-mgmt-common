package transformer

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	log "github.com/golang/glog"
)

const (
	AUTO_TECHSUPPORT_TABLE_NAME = "AUTO_TECHSUPPORT"
	AUTO_TECHSUPPORT_GLOBAL_KEY = "GLOBAL"
	autoTsMaxCoreLimitField     = "max_core_limit"
	autoTsStateField            = "state"
	autoTsStateEnabled          = "enabled"
	// coredump_gen_handler.py and techsupport_cleanup.py treat 0.0 as "no
	// cleanup" but raise on a missing field, so DELETE writes this instead of
	// removing the field.
	autoTsLimitDisabled = "0.0"
)

func init() {
	XlateFuncBind("YangToDb_core_file_cleanup_xfmr", YangToDb_core_file_cleanup_xfmr)
	XlateFuncBind("DbToYang_core_file_cleanup_xfmr", DbToYang_core_file_cleanup_xfmr)
	XlateFuncBind("Subscribe_core_file_cleanup_xfmr", Subscribe_core_file_cleanup_xfmr)
}

// formatAutoTsLimit renders a percent value the way the SONiC defaults and
// CLI store it (e.g. "5.0", "8.25").
func formatAutoTsLimit(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// YangToDb_core_file_cleanup_xfmr maps /system/core-file-cleanup/config
// (openconfig-system-ext) to CONFIG_DB AUTO_TECHSUPPORT|GLOBAL:max_core_limit.
// coredump_gen_handler.py reads that field on every new core and deletes the
// oldest /var/core files until they fit under the limit.
var YangToDb_core_file_cleanup_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("Inside YangToDb_core_file_cleanup_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)

	if inParams.oper == DELETE {
		// max-core-limit is the only config leaf; leave the rest of the
		// AUTO_TECHSUPPORT|GLOBAL row (state, tech-support limits) untouched.
		subOpMap := map[db.DBNum]map[string]map[string]db.Value{
			db.ConfigDB: {
				AUTO_TECHSUPPORT_TABLE_NAME: {
					AUTO_TECHSUPPORT_GLOBAL_KEY: {Field: map[string]string{autoTsMaxCoreLimitField: autoTsLimitDisabled}},
				},
			},
		}
		updateSubOpDataMap(subOpMap, UPDATE, inParams)
		return res, nil
	}

	sysObj := getAppRootObject(inParams)
	if sysObj == nil || sysObj.CoreFileCleanup == nil || sysObj.CoreFileCleanup.Config == nil {
		return res, nil
	}
	cfg := sysObj.CoreFileCleanup.Config

	if cfg.MaxCoreLimit == nil {
		return res, nil
	}
	val := db.Value{Field: map[string]string{autoTsMaxCoreLimitField: formatAutoTsLimit(*cfg.MaxCoreLimit)}}
	res[AUTO_TECHSUPPORT_TABLE_NAME] = map[string]db.Value{AUTO_TECHSUPPORT_GLOBAL_KEY: val}
	return res, nil
}

// DbToYang_core_file_cleanup_xfmr populates /system/core-file-cleanup config
// and state from CONFIG_DB AUTO_TECHSUPPORT|GLOBAL.
var DbToYang_core_file_cleanup_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_core_file_cleanup_xfmr URI: %s", inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.CoreFileCleanup == nil {
		deviceObj.System.CoreFileCleanup = &ocbinds.OpenconfigSystem_System_CoreFileCleanup{}
	}
	cleanup := deviceObj.System.CoreFileCleanup
	if cleanup.Config == nil {
		cleanup.Config = &ocbinds.OpenconfigSystem_System_CoreFileCleanup_Config{}
	}
	if cleanup.State == nil {
		cleanup.State = &ocbinds.OpenconfigSystem_System_CoreFileCleanup_State{}
	}

	entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: AUTO_TECHSUPPORT_TABLE_NAME}, db.Key{Comp: []string{AUTO_TECHSUPPORT_GLOBAL_KEY}})
	if err != nil {
		// No AUTO_TECHSUPPORT|GLOBAL row configured yet; nothing to return.
		return nil
	}

	if v := entry.Get(autoTsMaxCoreLimitField); v != "" {
		if f, e := strconv.ParseFloat(v, 64); e == nil {
			cfgVal, stateVal := f, f
			cleanup.Config.MaxCoreLimit = &cfgVal
			cleanup.State.MaxCoreLimit = &stateVal
		}
	}
	if v := entry.Get(autoTsStateField); v != "" {
		enabled := v == autoTsStateEnabled
		cleanup.State.Enabled = &enabled
	}
	return nil
}

// Subscribe_core_file_cleanup_xfmr maps the subtree to AUTO_TECHSUPPORT|GLOBAL.
var Subscribe_core_file_cleanup_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		AUTO_TECHSUPPORT_TABLE_NAME: {AUTO_TECHSUPPORT_GLOBAL_KEY: {}},
	}
	return result, nil
}
