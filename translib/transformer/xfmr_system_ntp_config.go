package transformer

import (
	"errors"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	log "github.com/golang/glog"
)

const (
	NTP_GLOBAL_TABLE_NAME = "NTP"
	NTP_GLOBAL_KEY        = "global"
	// NTP|global:src_intf is stored/consumed as a scalar (the chrony config
	// template and init_cfg both treat it as a single interface name), so it is
	// written as a plain field rather than the leaf-list "src_intf@" form.
	ntpSrcIntfField = "src_intf"
)

func init() {
	XlateFuncBind("YangToDb_ntp_config_xfmr", YangToDb_ntp_config_xfmr)
	XlateFuncBind("DbToYang_ntp_config_xfmr", DbToYang_ntp_config_xfmr)
	XlateFuncBind("Subscribe_ntp_config_xfmr", Subscribe_ntp_config_xfmr)
}

// YangToDb_ntp_config_xfmr maps /system/ntp/config to the CONFIG_DB NTP|global
// entry. Today it only owns the openconfig-system-ext source-interface leaf,
// which maps to the SONiC src_intf field (EOS 'ntp local-interface').
//
// The subtree is bound at the whole /ntp/config container (source-interface is
// augmented into it), so the standard oc-system leaves enabled/enable-ntp-auth
// are also routed here. Those are intentionally NOT written: they have no
// backing field in the SONiC NTP|global schema and no backend consumer
// (hostcfgd/chrony drive NTP enable/auth via admin_state, not via gNMI), so a
// SET that carries them is accepted but only src_intf is persisted. This keeps
// combined config SETs working while giving the subtree precise control over
// DELETE (clearing only src_intf and never the hostcfgd-managed row fields).
var YangToDb_ntp_config_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("Inside YangToDb_ntp_config_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)

	if inParams.oper == DELETE {
		// The subtree owns the whole /ntp/config container, so a DELETE on any
		// sibling leaf (e.g. enabled) is also routed here. Only clear src_intf
		// when the request actually targets the source-interface leaf or the
		// whole /ntp/config container; otherwise a sibling-leaf DELETE would
		// wipe the configured source interface.
		reqPath, perr := getYangPathFromUri(inParams.requestUri)
		if perr != nil {
			return nil, perr
		}
		if uriPathEndsWithLeaf(reqPath, "source-interface") || uriPathEndsWithLeaf(reqPath, "config") {
			val := db.Value{Field: map[string]string{ntpSrcIntfField: ""}}
			res[NTP_GLOBAL_TABLE_NAME] = map[string]db.Value{NTP_GLOBAL_KEY: val}
		}
		return res, nil
	}

	sysObj := getAppRootObject(inParams)
	if sysObj == nil || sysObj.Ntp == nil || sysObj.Ntp.Config == nil {
		return res, nil
	}
	cfg := sysObj.Ntp.Config

	val := db.Value{Field: map[string]string{}}
	if cfg.SourceInterface != nil {
		val.Field[ntpSrcIntfField] = *cfg.SourceInterface
	}

	if len(val.Field) == 0 {
		// Nothing this transformer owns was present in the payload
		// (e.g. only standard leaves like enabled were set).
		return res, nil
	}
	res[NTP_GLOBAL_TABLE_NAME] = map[string]db.Value{NTP_GLOBAL_KEY: val}
	return res, nil
}

// DbToYang_ntp_config_xfmr populates /system/ntp/config from CONFIG_DB NTP|global.
var DbToYang_ntp_config_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_ntp_config_xfmr URI: %s", inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Ntp == nil {
		deviceObj.System.Ntp = &ocbinds.OpenconfigSystem_System_Ntp{}
	}
	if deviceObj.System.Ntp.Config == nil {
		deviceObj.System.Ntp.Config = &ocbinds.OpenconfigSystem_System_Ntp_Config{}
	}
	cfg := deviceObj.System.Ntp.Config

	entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: NTP_GLOBAL_TABLE_NAME}, db.Key{Comp: []string{NTP_GLOBAL_KEY}})
	if err != nil {
		// No NTP|global row configured yet; nothing to return.
		return nil
	}

	if v := entry.Get(ntpSrcIntfField); v != "" {
		cfg.SourceInterface = &v
	}
	return nil
}

// Subscribe_ntp_config_xfmr maps the config subtree to NTP|global.
var Subscribe_ntp_config_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		NTP_GLOBAL_TABLE_NAME: {NTP_GLOBAL_KEY: {}},
	}
	return result, nil
}
