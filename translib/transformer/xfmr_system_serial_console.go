package transformer

import (
	"errors"
	"strconv"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	log "github.com/golang/glog"
)

const (
	SERIAL_CONSOLE_TABLE_NAME    = "SERIAL_CONSOLE"
	SERIAL_CONSOLE_POLICIES_KEY  = "POLICIES"
	serialInactivityTimeoutField = "inactivity_timeout"
	// OpenConfig .../console/config/timeout is uint16 seconds.
	serialMaxTimeoutSeconds = 65535
)

func init() {
	XlateFuncBind("YangToDb_serial_console_xfmr", YangToDb_serial_console_xfmr)
	XlateFuncBind("DbToYang_serial_console_xfmr", DbToYang_serial_console_xfmr)
	XlateFuncBind("Subscribe_serial_console_xfmr", Subscribe_serial_console_xfmr)
}

// YangToDb_serial_console_xfmr maps /system/console/config (openconfig-system-ext)
// to the CONFIG_DB SERIAL_CONSOLE|POLICIES entry. OpenConfig timeout is in seconds
// while SONiC inactivity_timeout is in minutes, so we convert on write (sec/60).
var YangToDb_serial_console_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("Inside YangToDb_serial_console_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)

	if inParams.oper == DELETE {
		// Only inactivity_timeout is owned by this transformer today, so DELETE on
		// the console container, its config, or the timeout leaf all clear it.
		val := db.Value{Field: map[string]string{serialInactivityTimeoutField: ""}}
		res[SERIAL_CONSOLE_TABLE_NAME] = map[string]db.Value{SERIAL_CONSOLE_POLICIES_KEY: val}
		return res, nil
	}

	sysObj := getAppRootObject(inParams)
	if sysObj == nil || sysObj.Console == nil || sysObj.Console.Config == nil {
		return res, nil
	}
	cfg := sysObj.Console.Config

	val := db.Value{Field: map[string]string{}}
	if cfg.Timeout != nil {
		// OpenConfig seconds -> SONiC minutes.
		val.Field[serialInactivityTimeoutField] = strconv.FormatUint(uint64(*cfg.Timeout)/60, 10)
	}

	if len(val.Field) == 0 {
		return res, nil
	}
	res[SERIAL_CONSOLE_TABLE_NAME] = map[string]db.Value{SERIAL_CONSOLE_POLICIES_KEY: val}
	return res, nil
}

// DbToYang_serial_console_xfmr populates /system/console/config and state from the
// CONFIG_DB SERIAL_CONSOLE|POLICIES entry.
var DbToYang_serial_console_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_serial_console_xfmr URI: %s", inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Console == nil {
		deviceObj.System.Console = &ocbinds.OpenconfigSystem_System_Console{}
	}
	if deviceObj.System.Console.Config == nil {
		deviceObj.System.Console.Config = &ocbinds.OpenconfigSystem_System_Console_Config{}
	}

	entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: SERIAL_CONSOLE_TABLE_NAME}, db.Key{Comp: []string{SERIAL_CONSOLE_POLICIES_KEY}})
	if err != nil {
		// No SERIAL_CONSOLE|POLICIES row configured yet; nothing to return.
		return nil
	}

	if v, ok := entry.Field[serialInactivityTimeoutField]; ok && v != "" {
		if m, e := strconv.ParseUint(v, 10, 32); e == nil {
			sec := m * 60
			if sec > serialMaxTimeoutSeconds {
				sec = serialMaxTimeoutSeconds
			}
			t := uint16(sec)
			deviceObj.System.Console.Config.Timeout = &t
		}
	}
	return nil
}

// Subscribe_serial_console_xfmr maps the console subtree to SERIAL_CONSOLE|POLICIES.
var Subscribe_serial_console_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		SERIAL_CONSOLE_TABLE_NAME: {SERIAL_CONSOLE_POLICIES_KEY: {}},
	}
	return result, nil
}
