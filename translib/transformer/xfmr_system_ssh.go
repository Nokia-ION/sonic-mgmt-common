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
	SSH_SERVER_TABLE_NAME     = "SSH_SERVER"
	SSH_SERVER_POLICIES_KEY   = "POLICIES"
	sshInactivityTimeoutField = "inactivity_timeout"
	sshDscpField              = "dscp"
	// OpenConfig /system/ssh-server/config/timeout is uint16 seconds.
	sshMaxTimeoutSeconds = 65535
)

func init() {
	XlateFuncBind("YangToDb_ssh_server_config_xfmr", YangToDb_ssh_server_config_xfmr)
	XlateFuncBind("DbToYang_ssh_server_config_xfmr", DbToYang_ssh_server_config_xfmr)
	XlateFuncBind("Subscribe_ssh_server_config_xfmr", Subscribe_ssh_server_config_xfmr)
}

// YangToDb_ssh_server_config_xfmr maps /system/ssh-server/config to the CONFIG_DB
// SSH_SERVER|POLICIES entry. It handles two leaves:
//   - timeout (OpenConfig, seconds)      <-> inactivity_timeout (SONiC, minutes)
//   - dscp    (openconfig-system-ext)    <-> dscp (SONiC, 0..63; backend renders IPQoS)
//
// The SONiC field inactivity_timeout is in minutes while OpenConfig timeout is in
// seconds, so we convert on write (sec/60) and read (min*60).
var YangToDb_ssh_server_config_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("Inside YangToDb_ssh_server_config_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		return nil, err
	}

	if inParams.oper == DELETE {
		val := db.Value{Field: map[string]string{}}
		switch {
		case strings.HasSuffix(targetUriPath, "/timeout"):
			val.Field[sshInactivityTimeoutField] = ""
		case strings.HasSuffix(targetUriPath, "/dscp"):
			val.Field[sshDscpField] = ""
		default:
			// DELETE on the whole config subtree: remove only the fields we own.
			val.Field[sshInactivityTimeoutField] = ""
			val.Field[sshDscpField] = ""
		}
		res[SSH_SERVER_TABLE_NAME] = map[string]db.Value{SSH_SERVER_POLICIES_KEY: val}
		return res, nil
	}

	sysObj := getAppRootObject(inParams)
	if sysObj == nil || sysObj.SshServer == nil || sysObj.SshServer.Config == nil {
		return res, nil
	}
	cfg := sysObj.SshServer.Config

	val := db.Value{Field: map[string]string{}}
	if cfg.Timeout != nil {
		// OpenConfig seconds -> SONiC minutes.
		val.Field[sshInactivityTimeoutField] = strconv.FormatUint(uint64(*cfg.Timeout)/60, 10)
	}
	if cfg.Dscp != nil {
		val.Field[sshDscpField] = strconv.FormatUint(uint64(*cfg.Dscp), 10)
	}

	if len(val.Field) == 0 {
		return res, nil
	}
	res[SSH_SERVER_TABLE_NAME] = map[string]db.Value{SSH_SERVER_POLICIES_KEY: val}
	return res, nil
}

// DbToYang_ssh_server_config_xfmr populates /system/ssh-server/config from the
// CONFIG_DB SSH_SERVER|POLICIES entry.
var DbToYang_ssh_server_config_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_ssh_server_config_xfmr URI: %s", inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.SshServer == nil {
		deviceObj.System.SshServer = &ocbinds.OpenconfigSystem_System_SshServer{}
	}
	if deviceObj.System.SshServer.Config == nil {
		deviceObj.System.SshServer.Config = &ocbinds.OpenconfigSystem_System_SshServer_Config{}
	}
	cfg := deviceObj.System.SshServer.Config

	entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: SSH_SERVER_TABLE_NAME}, db.Key{Comp: []string{SSH_SERVER_POLICIES_KEY}})
	if err != nil {
		// No SSH_SERVER|POLICIES row configured yet; nothing to return.
		return nil
	}

	if v, ok := entry.Field[sshInactivityTimeoutField]; ok && v != "" {
		if m, e := strconv.ParseUint(v, 10, 32); e == nil {
			sec := m * 60
			if sec > sshMaxTimeoutSeconds {
				sec = sshMaxTimeoutSeconds
			}
			t := uint16(sec)
			cfg.Timeout = &t
		}
	}
	if v, ok := entry.Field[sshDscpField]; ok && v != "" {
		if d, e := strconv.ParseUint(v, 10, 8); e == nil {
			dscp := uint8(d)
			cfg.Dscp = &dscp
		}
	}
	return nil
}

// Subscribe_ssh_server_config_xfmr maps the config subtree to SSH_SERVER|POLICIES.
var Subscribe_ssh_server_config_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		SSH_SERVER_TABLE_NAME: {SSH_SERVER_POLICIES_KEY: {}},
	}
	return result, nil
}
