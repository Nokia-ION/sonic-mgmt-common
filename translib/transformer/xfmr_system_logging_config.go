package transformer

import (
	"errors"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	log "github.com/golang/glog"
)

const (
	SYSLOG_CONFIG_TABLE_NAME = "SYSLOG_CONFIG"
	SYSLOG_GLOBAL_KEY        = "GLOBAL"
	syslogSrcIntfField       = "src_intf"
)

func init() {
	XlateFuncBind("YangToDb_logging_config_xfmr", YangToDb_logging_config_xfmr)
	XlateFuncBind("DbToYang_logging_config_xfmr", DbToYang_logging_config_xfmr)
	XlateFuncBind("Subscribe_logging_config_xfmr", Subscribe_logging_config_xfmr)
}

// YangToDb_logging_config_xfmr maps the openconfig-system-ext logging
// source-interface leaf to CONFIG_DB SYSLOG_CONFIG|GLOBAL:src_intf. The backend
// (rsyslog-config) resolves the interface to an IP and uses it as the source
// address for remote syslog forwarding (EOS 'logging source-interface').
var YangToDb_logging_config_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("Inside YangToDb_logging_config_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)

	if inParams.oper == DELETE {
		// Clear only the field this transformer owns; leave the rest of the
		// SYSLOG_CONFIG|GLOBAL row (rate-limit, format, severity) untouched.
		val := db.Value{Field: map[string]string{syslogSrcIntfField: ""}}
		res[SYSLOG_CONFIG_TABLE_NAME] = map[string]db.Value{SYSLOG_GLOBAL_KEY: val}
		return res, nil
	}

	sysObj := getAppRootObject(inParams)
	if sysObj == nil || sysObj.Logging == nil || sysObj.Logging.Config == nil {
		return res, nil
	}
	cfg := sysObj.Logging.Config

	if cfg.SourceInterface == nil {
		return res, nil
	}
	val := db.Value{Field: map[string]string{syslogSrcIntfField: *cfg.SourceInterface}}
	res[SYSLOG_CONFIG_TABLE_NAME] = map[string]db.Value{SYSLOG_GLOBAL_KEY: val}
	return res, nil
}

// DbToYang_logging_config_xfmr populates the logging source-interface from
// CONFIG_DB SYSLOG_CONFIG|GLOBAL.
var DbToYang_logging_config_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_logging_config_xfmr URI: %s", inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Logging == nil {
		deviceObj.System.Logging = &ocbinds.OpenconfigSystem_System_Logging{}
	}
	if deviceObj.System.Logging.Config == nil {
		deviceObj.System.Logging.Config = &ocbinds.OpenconfigSystem_System_Logging_Config{}
	}
	cfg := deviceObj.System.Logging.Config

	entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: SYSLOG_CONFIG_TABLE_NAME}, db.Key{Comp: []string{SYSLOG_GLOBAL_KEY}})
	if err != nil {
		// No SYSLOG_CONFIG|GLOBAL row configured yet; nothing to return.
		return nil
	}

	if v := entry.Get(syslogSrcIntfField); v != "" {
		cfg.SourceInterface = &v
	}
	return nil
}

// Subscribe_logging_config_xfmr maps the config subtree to SYSLOG_CONFIG|GLOBAL.
var Subscribe_logging_config_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		SYSLOG_CONFIG_TABLE_NAME: {SYSLOG_GLOBAL_KEY: {}},
	}
	return result, nil
}
