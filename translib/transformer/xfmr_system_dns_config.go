package transformer

import (
	"errors"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	log "github.com/golang/glog"
)

const (
	DNS_OPTIONS_TABLE_NAME = "DNS_OPTIONS"
	DNS_OPTIONS_GLOBAL_KEY = "GLOBAL"
	dnsSearchLeaf          = "search"
	dnsSearchField         = "search@"
)

func init() {
	XlateFuncBind("YangToDb_dns_config_xfmr", YangToDb_dns_config_xfmr)
	XlateFuncBind("DbToYang_dns_config_xfmr", DbToYang_dns_config_xfmr)
	XlateFuncBind("Subscribe_dns_config_xfmr", Subscribe_dns_config_xfmr)
	XlateFuncBind("DbToYang_dns_state_xfmr", DbToYang_dns_state_xfmr)
	XlateFuncBind("Subscribe_dns_state_xfmr", Subscribe_dns_config_xfmr)
}

// YangToDb_dns_config_xfmr maps /system/dns/config/search to the CONFIG_DB
// DNS_OPTIONS|GLOBAL search@ leaf-list, which resolv.conf.j2 renders as the
// resolver "search" line. The row's other fields (ndots, timeout, attempts)
// have no OpenConfig leaf and are never written here.
//
// The common app merges a search@ UPDATE into the existing list and swaps it
// on a north-bound REPLACE, so this only has to emit the requested domains.
var YangToDb_dns_config_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("Inside YangToDb_dns_config_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)

	if inParams.oper == DELETE {
		// An empty value clears the whole list; a value names the one domain
		// a leaf-list instance delete removes.
		val := db.Value{Field: map[string]string{
			dnsSearchField: NewPathInfo(inParams.requestUri).Var(dnsSearchLeaf),
		}}
		res[DNS_OPTIONS_TABLE_NAME] = map[string]db.Value{DNS_OPTIONS_GLOBAL_KEY: val}
		return res, nil
	}

	var search []string
	if sysObj := getAppRootObject(inParams); sysObj != nil && sysObj.Dns != nil && sysObj.Dns.Config != nil {
		search = sysObj.Dns.Config.Search
	}

	if inParams.oper == REPLACE {
		// A REPLACE result row is written with SetEntry when the target is the
		// config container, which would drop the row's other fields. An UPDATE
		// sub-operation is written with ModEntry and, under a north-bound
		// REPLACE, still swaps the leaf-list instead of merging it.
		var subOpMap map[db.DBNum]map[string]map[string]db.Value
		if len(search) == 0 {
			subOpMap = dnsOptionsSubOp(db.Value{Field: map[string]string{dnsSearchField: ""}})
			updateSubOpDataMap(subOpMap, DELETE, inParams)
		} else {
			val := db.Value{Field: map[string]string{}}
			val.SetList(dnsSearchField, search)
			subOpMap = dnsOptionsSubOp(val)
			updateSubOpDataMap(subOpMap, UPDATE, inParams)
		}
		return res, nil
	}

	if len(search) == 0 {
		return res, nil
	}
	val := db.Value{Field: map[string]string{}}
	val.SetList(dnsSearchField, search)
	res[DNS_OPTIONS_TABLE_NAME] = map[string]db.Value{DNS_OPTIONS_GLOBAL_KEY: val}
	return res, nil
}

func dnsOptionsSubOp(val db.Value) map[db.DBNum]map[string]map[string]db.Value {
	return map[db.DBNum]map[string]map[string]db.Value{
		db.ConfigDB: {
			DNS_OPTIONS_TABLE_NAME: {DNS_OPTIONS_GLOBAL_KEY: val},
		},
	}
}

// dnsSearchFromDb returns the configured search domains, or nil when the
// DNS_OPTIONS|GLOBAL row or its search@ field is absent.
func dnsSearchFromDb(inParams XfmrParams) ([]string, error) {
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return nil, errors.New("CONFIG_DB not found")
	}
	entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: DNS_OPTIONS_TABLE_NAME}, db.Key{Comp: []string{DNS_OPTIONS_GLOBAL_KEY}})
	if err != nil {
		return nil, nil
	}
	search := entry.GetList(dnsSearchField)
	if len(search) == 0 {
		return nil, nil
	}
	return search, nil
}

func dnsRootObject(inParams XfmrParams) *ocbinds.OpenconfigSystem_System_Dns {
	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Dns == nil {
		deviceObj.System.Dns = &ocbinds.OpenconfigSystem_System_Dns{}
	}
	return deviceObj.System.Dns
}

// DbToYang_dns_config_xfmr populates /system/dns/config from DNS_OPTIONS|GLOBAL.
var DbToYang_dns_config_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_dns_config_xfmr URI: %s", inParams.uri)
	search, err := dnsSearchFromDb(inParams)
	if err != nil {
		return err
	}
	dns := dnsRootObject(inParams)
	if dns.Config == nil {
		dns.Config = &ocbinds.OpenconfigSystem_System_Dns_Config{}
	}
	dns.Config.Search = search
	return nil
}

// DbToYang_dns_state_xfmr populates /system/dns/state. SONiC has no separate
// operational record of the resolver search list, so state mirrors config.
var DbToYang_dns_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_dns_state_xfmr URI: %s", inParams.uri)
	search, err := dnsSearchFromDb(inParams)
	if err != nil {
		return err
	}
	dns := dnsRootObject(inParams)
	if dns.State == nil {
		dns.State = &ocbinds.OpenconfigSystem_System_Dns_State{}
	}
	dns.State.Search = search
	return nil
}

// Subscribe_dns_config_xfmr maps the config and state subtrees to DNS_OPTIONS|GLOBAL.
var Subscribe_dns_config_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		DNS_OPTIONS_TABLE_NAME: {DNS_OPTIONS_GLOBAL_KEY: {}},
	}
	return result, nil
}
