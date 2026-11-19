package transformer

import (
	"fmt"
	"net"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	ipv6RaPrefixFieldValidLifetime     = "ipv6_ra_prefix_valid_lifetime"
	ipv6RaPrefixFieldPreferredLifetime = "ipv6_ra_prefix_preferred_lifetime"
	ipv6RaPrefixFieldDisableAdv        = "ipv6_ra_prefix_disable_advertisement"
	ipv6RaPrefixFieldDisableAutoconfig = "ipv6_ra_prefix_disable_autoconfig"
	ipv6RaPrefixFieldEnableOnlink      = "ipv6_ra_prefix_enable_onlink"
)

func isVlanRoutedVlanRaPrefixURI(uri, requestUri string) bool {
	p := ipv6RaPrefixReqPath(uri, requestUri)
	return strings.Contains(p, "openconfig-vlan:routed-vlan") ||
		strings.Contains(p, "/routed-vlan/")
}

func rejectNonVlanRaPrefixDisableAdv(inParams XfmrParams) error {
	if isVlanRoutedVlanRaPrefixURI(inParams.uri, inParams.requestUri) {
		return nil
	}
	path := inParams.requestUri
	if path == "" {
		path = inParams.uri
	}
	return tlerr.NotSupportedError{
		Format: "disable-advertisement is supported on Vlan routed-vlan RA prefixes only",
		Path:   path,
	}
}

func init() {
	XlateFuncBind("Subscribe_ipv6_ra_prefixes_xfmr", Subscribe_ipv6_ra_prefixes_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_prefixes_xfmr", DbToYang_ipv6_ra_prefixes_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_prefixes_xfmr", YangToDb_ipv6_ra_prefixes_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_prefix_valid_lifetime_xfmr", YangToDb_ipv6_ra_prefix_valid_lifetime_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr", DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_prefix_preferred_lifetime_xfmr", YangToDb_ipv6_ra_prefix_preferred_lifetime_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_prefix_preferred_lifetime_xfmr", DbToYang_ipv6_ra_prefix_preferred_lifetime_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_prefix_disable_advertisement_xfmr", YangToDb_ipv6_ra_prefix_disable_advertisement_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_prefix_disable_advertisement_xfmr", DbToYang_ipv6_ra_prefix_disable_advertisement_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_prefix_disable_autoconfig_xfmr", YangToDb_ipv6_ra_prefix_disable_autoconfig_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_prefix_disable_autoconfig_xfmr", DbToYang_ipv6_ra_prefix_disable_autoconfig_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_prefix_enable_onlink_xfmr", YangToDb_ipv6_ra_prefix_enable_onlink_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_prefix_enable_onlink_xfmr", DbToYang_ipv6_ra_prefix_enable_onlink_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_prefix_prefix_xfmr", YangToDb_ipv6_ra_prefix_prefix_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_prefix_prefix_xfmr", DbToYang_ipv6_ra_prefix_prefix_xfmr)
}

func isIpv6RaPrefixPath(uriPath string) bool {
	if uriPath == "" {
		return false
	}
	return strings.Contains(uriPath, "/router-advertisement/prefixes/prefix")
}

func isIpv6RaPrefixesContainerPath(uriPath string) bool {
	if uriPath == "" {
		return false
	}
	return strings.Contains(uriPath, "/router-advertisement/prefixes") &&
		!strings.Contains(uriPath, "/router-advertisement/prefixes/prefix")
}

func entryHasIpv6RaPrefixConfig(entry db.Value) bool {
	if entry.Field == nil {
		return false
	}
	for _, field := range []string{
		ipv6RaPrefixFieldValidLifetime,
		ipv6RaPrefixFieldPreferredLifetime,
		ipv6RaPrefixFieldDisableAdv,
		ipv6RaPrefixFieldDisableAutoconfig,
		ipv6RaPrefixFieldEnableOnlink,
	} {
		if v, ok := entry.Field[field]; ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

// RA prefix rows share INTERFACE|<if>|<prefix> with assigned addresses; they are
// identified by ipv6_ra_prefix_* fields and no L3 address family.
func isIpv6RaPrefixOnlyEntry(entry db.Value) bool {
	if !entry.IsPopulated() || entry.Field == nil {
		return false
	}
	if !entryHasIpv6RaPrefixConfig(entry) {
		return false
	}
	if family, ok := entry.Field["family"]; ok && strings.TrimSpace(family) != "" {
		return false
	}
	// SONiC assigned IPv6 on VLAN/INTERFACE uses NULL placeholder on the same key as RA prefix config.
	if _, ok := entry.Field["NULL"]; ok {
		return false
	}
	return true
}

func ipv6RaPrefixConfigFieldNames() []string {
	return []string{
		ipv6RaPrefixFieldValidLifetime,
		ipv6RaPrefixFieldPreferredLifetime,
		ipv6RaPrefixFieldDisableAdv,
		ipv6RaPrefixFieldDisableAutoconfig,
		ipv6RaPrefixFieldEnableOnlink,
	}
}

func ipv6RaPrefixFieldsToClear(entry db.Value) map[string]string {
	out := make(map[string]string)
	if entry.Field == nil {
		return out
	}
	for _, f := range ipv6RaPrefixConfigFieldNames() {
		if _, ok := entry.Field[f]; ok {
			out[f] = ""
		}
	}
	return out
}

func isIpv6RaPrefixInstanceDeletePath(uriPath string) bool {
	if !isIpv6RaPrefixPath(uriPath) {
		return false
	}
	if ipv6RaPrefixLeafSpecForUri(uriPath) != nil {
		return false
	}
	trimmed := strings.TrimSuffix(strings.TrimSpace(uriPath), "/")
	if strings.HasSuffix(trimmed, "]") {
		return true
	}
	if strings.Contains(trimmed, "/prefix[") {
		return strings.HasSuffix(trimmed, "/config") || strings.HasSuffix(trimmed, "/state")
	}
	return false
}

func ipv6RaPrefixOnlyDbKeys(d *db.DB, tbl, ifName string) ([]string, error) {
	var keys []string
	if d == nil || ifName == "" || tbl == "" {
		return keys, nil
	}
	ipKeys, err := doGetIntfIpKeys(d, tbl, ifName)
	if err != nil {
		return keys, err
	}
	for _, key := range ipKeys {
		if len(key.Comp) < 2 || key.Get(0) != ifName {
			continue
		}
		suffix := key.Get(1)
		if !strings.Contains(suffix, ":") {
			continue
		}
		entry, err := d.GetEntry(&db.TableSpec{Name: tbl}, db.Key{Comp: []string{ifName, suffix}})
		if err != nil || !isIpv6RaPrefixOnlyEntry(entry) {
			continue
		}
		keys = append(keys, ifName+"|"+suffix)
	}
	return keys, nil
}

// ipv6RaPrefixReqPath returns the URI that carries interface + prefix list keys for leaf SETs.
func ipv6RaPrefixReqPath(uri, requestUri string) string {
	reqHasName := isIpv6RaPrefixPath(requestUri) && NewPathInfo(requestUri).Var("name") != ""
	uriHasName := isIpv6RaPrefixPath(uri) && NewPathInfo(uri).Var("name") != ""
	if reqHasName {
		return requestUri
	}
	if uriHasName {
		return uri
	}
	if isIpv6RaPrefixPath(requestUri) {
		return requestUri
	}
	if isIpv6RaPrefixPath(uri) {
		return uri
	}
	if strings.Contains(uri, "/prefixes/prefix") && len(uri) > len(requestUri) {
		return uri
	}
	return requestUri
}

func ipv6RaPrefixPathInfo(uri, requestUri string) *PathInfo {
	return NewPathInfo(ipv6RaPrefixReqPath(uri, requestUri))
}

// ipv6RaPrefixPathVar returns list keys from the longest URI that still carries
// prefix[...] (GET often uses a short requestUri while translib walks with a
// relative uri that omits list keys).
func ipv6RaPrefixPathVar(uri, requestUri, varName string) string {
	for _, p := range []string{ipv6RaPrefixReqPath(uri, requestUri), requestUri, uri} {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if v := NewPathInfo(p).Var(varName); v != "" {
			return v
		}
	}
	return ""
}

func ipv6RaPrefixWithConfigDb(inParams XfmrParams) XfmrParams {
	out := inParams
	out.curDb = db.ConfigDB
	if out.dbs[db.ConfigDB] != nil {
		out.d = out.dbs[db.ConfigDB]
	}
	return out
}

func ipv6RaPrefixDbToYangScope(uri, requestUri string) (fillConfig, fillState bool) {
	p := strings.TrimSpace(ipv6RaPrefixReqPath(uri, requestUri))
	if p == "" {
		return true, true
	}
	if !isIpv6RaPrefixPath(p) {
		return true, true
	}
	trimmed := strings.TrimSuffix(strings.TrimSpace(p), "/")
	inConfig := strings.HasSuffix(trimmed, "/config") || strings.Contains(trimmed, "/config/")
	inState := strings.HasSuffix(trimmed, "/state") || strings.Contains(trimmed, "/state/")
	if inConfig && !inState {
		return true, false
	}
	if inState && !inConfig {
		return false, true
	}
	return true, true
}

func ipv6RaPreparePrefixYgot(pref *ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv6_RouterAdvertisement_Prefixes_Prefix, fillConfig, fillState bool) {
	if pref == nil {
		return
	}
	ygot.BuildEmptyTree(pref)
	if fillConfig && pref.Config == nil {
		pref.Config = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv6_RouterAdvertisement_Prefixes_Prefix_Config{}
	}
	if fillState && pref.State == nil {
		pref.State = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv6_RouterAdvertisement_Prefixes_Prefix_State{}
	}
}

func ipv6RaPrepareSubintfPrefixYgot(pref *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_RouterAdvertisement_Prefixes_Prefix, fillConfig, fillState bool) {
	if pref == nil {
		return
	}
	ygot.BuildEmptyTree(pref)
	if fillConfig && pref.Config == nil {
		pref.Config = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_RouterAdvertisement_Prefixes_Prefix_Config{}
	}
	if fillState && pref.State == nil {
		pref.State = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_RouterAdvertisement_Prefixes_Prefix_State{}
	}
}

func ipv6PrefixNetwork(prefix string) (*net.IPNet, error) {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(prefix))
	if err != nil {
		return nil, err
	}
	if ipNet.IP.To4() != nil {
		return nil, fmt.Errorf("not an IPv6 prefix")
	}
	return ipNet, nil
}

func ipv6PrefixNetworksEqual(a, b string) bool {
	an, errA := ipv6PrefixNetwork(a)
	bn, errB := ipv6PrefixNetwork(b)
	if errA != nil || errB != nil {
		return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
	}
	return an.String() == bn.String()
}

func ipv6RaPrefixDbKeyFromMap(tbl, ifName, ocPrefix string, dbDataMap *RedisDbMap, curDb db.DBNum) (string, bool) {
	if dbDataMap == nil {
		return "", false
	}
	data := (*dbDataMap)[curDb]
	direct := ifName + "|" + ocPrefix
	if entry, ok := data[tbl][direct]; ok && entry.Field != nil {
		return direct, true
	}
	for key := range data[tbl] {
		if !strings.HasPrefix(key, ifName+"|") {
			continue
		}
		cfgPrefix := strings.SplitN(key, "|", 2)[1]
		if !strings.Contains(cfgPrefix, ":") {
			continue
		}
		if ipv6PrefixNetworksEqual(ocPrefix, cfgPrefix) {
			return key, true
		}
	}
	return "", false
}

func lookupLiveIpv6RaPrefixKey(d *db.DB, tbl, ifName, ocPrefix string) (string, bool) {
	if d == nil || ifName == "" || ocPrefix == "" {
		return "", false
	}
	keys, err := doGetIntfIpKeys(d, tbl, ifName)
	if err != nil {
		return "", false
	}
	for _, key := range keys {
		if len(key.Comp) < 2 || key.Get(0) != ifName {
			continue
		}
		cfgPrefix := key.Get(1)
		if !strings.Contains(cfgPrefix, ":") {
			continue
		}
		if ipv6PrefixNetworksEqual(ocPrefix, cfgPrefix) {
			return ifName + "|" + cfgPrefix, true
		}
	}
	return "", false
}

func ipv6RaPrefixSeedDbMap(inParams XfmrParams, tbl, key string, entry db.Value) {
	if inParams.dbDataMap == nil {
		return
	}
	if (*inParams.dbDataMap)[inParams.curDb] == nil {
		(*inParams.dbDataMap)[inParams.curDb] = make(map[string]map[string]db.Value)
	}
	if (*inParams.dbDataMap)[inParams.curDb][tbl] == nil {
		(*inParams.dbDataMap)[inParams.curDb][tbl] = make(map[string]db.Value)
	}
	(*inParams.dbDataMap)[inParams.curDb][tbl][key] = entry
}

func ipv6RaPrefixLoadEntry(inParams XfmrParams, tbl, key string) (db.Value, bool) {
	inParams = ipv6RaPrefixWithConfigDb(inParams)
	if inParams.dbDataMap != nil {
		if data, ok := (*inParams.dbDataMap)[inParams.curDb]; ok {
			if entry, ok := data[tbl][key]; ok && entry.Field != nil {
				return entry, true
			}
		}
	}
	if inParams.d == nil || !strings.Contains(key, "|") {
		return db.Value{}, false
	}
	parts := strings.SplitN(key, "|", 2)
	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: tbl}, db.Key{Comp: parts})
	if err != nil || !entry.IsPopulated() {
		return db.Value{}, false
	}
	ipv6RaPrefixSeedDbMap(inParams, tbl, key, entry)
	return entry, true
}

// ipv6RaPrefixListEntries returns CONFIG_DB rows for IPv6 L3 prefixes on ifName
// (assigned addresses and prefix-only rows), keyed as ifName|cfgPrefix.
func ipv6RaPrefixListEntries(d *db.DB, tbl, ifName string) (map[string]db.Value, error) {
	out := make(map[string]db.Value)
	if d == nil || ifName == "" || tbl == "" {
		return out, nil
	}
	ipKeys, err := doGetIntfIpKeys(d, tbl, ifName)
	if err != nil {
		return nil, err
	}
	for _, key := range ipKeys {
		if len(key.Comp) < 2 || key.Get(0) != ifName {
			continue
		}
		suffix := key.Get(1)
		if !strings.Contains(suffix, ":") {
			continue
		}
		entry, err := d.GetEntry(&db.TableSpec{Name: tbl}, db.Key{Comp: []string{ifName, suffix}})
		if err != nil || !entry.IsPopulated() {
			continue
		}
		out[ifName+"|"+suffix] = entry
	}
	return out, nil
}

func ipv6RaPrefixOcNetworkFromComposite(compositeKey string) (string, error) {
	if !strings.Contains(compositeKey, "|") {
		return "", fmt.Errorf("invalid composite key")
	}
	return ipv6RaPrefixOcNetwork(strings.SplitN(compositeKey, "|", 2)[1])
}

func ipv6RaPrefixOcNetwork(cfgPrefix string) (string, error) {
	ipNet, err := ipv6PrefixNetwork(cfgPrefix)
	if err != nil {
		return "", err
	}
	return ipNet.String(), nil
}

func ipv6RaBoolPtr(v bool) *bool {
	b := v
	return &b
}

func ipv6RaUint32Ptr(v uint32) *uint32 {
	u := v
	return &u
}

func ipv6RaStrPtr(v string) *string {
	s := v
	return &s
}

func ipv6RaMergeRoutedVlanPrefixMap(m map[string]interface{}, cfg *ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv6_RouterAdvertisement_Prefixes_Prefix_Config, st *ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv6_RouterAdvertisement_Prefixes_Prefix_State, fillConfig, fillState bool) {
	if v, ok := m["prefix"].(string); ok {
		if fillConfig && cfg != nil {
			cfg.Prefix = ipv6RaStrPtr(v)
		}
		if fillState && st != nil {
			st.Prefix = ipv6RaStrPtr(v)
		}
	}
	if v, ok := m["valid-lifetime"].(uint32); ok {
		if fillConfig && cfg != nil {
			cfg.ValidLifetime = ipv6RaUint32Ptr(v)
		}
		if fillState && st != nil {
			st.ValidLifetime = ipv6RaUint32Ptr(v)
		}
	}
	if v, ok := m["preferred-lifetime"].(uint32); ok {
		if fillConfig && cfg != nil {
			cfg.PreferredLifetime = ipv6RaUint32Ptr(v)
		}
		if fillState && st != nil {
			st.PreferredLifetime = ipv6RaUint32Ptr(v)
		}
	}
	if v, ok := m["disable-advertisement"].(bool); ok {
		if fillConfig && cfg != nil {
			cfg.DisableAdvertisement = ipv6RaBoolPtr(v)
		}
		if fillState && st != nil {
			st.DisableAdvertisement = ipv6RaBoolPtr(v)
		}
	}
	if v, ok := m["disable-autoconfiguration"].(bool); ok {
		if fillConfig && cfg != nil {
			cfg.DisableAutoconfiguration = ipv6RaBoolPtr(v)
		}
		if fillState && st != nil {
			st.DisableAutoconfiguration = ipv6RaBoolPtr(v)
		}
	}
	if v, ok := m["enable-onlink"].(bool); ok {
		if fillConfig && cfg != nil {
			cfg.EnableOnlink = ipv6RaBoolPtr(v)
		}
		if fillState && st != nil {
			st.EnableOnlink = ipv6RaBoolPtr(v)
		}
	}
}

func ipv6RaMergeSubintfPrefixMap(m map[string]interface{}, cfg *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_RouterAdvertisement_Prefixes_Prefix_Config, st *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_RouterAdvertisement_Prefixes_Prefix_State, fillConfig, fillState bool) {
	if v, ok := m["prefix"].(string); ok {
		if fillConfig && cfg != nil {
			cfg.Prefix = ipv6RaStrPtr(v)
		}
		if fillState && st != nil {
			st.Prefix = ipv6RaStrPtr(v)
		}
	}
	if v, ok := m["valid-lifetime"].(uint32); ok {
		if fillConfig && cfg != nil {
			cfg.ValidLifetime = ipv6RaUint32Ptr(v)
		}
		if fillState && st != nil {
			st.ValidLifetime = ipv6RaUint32Ptr(v)
		}
	}
	if v, ok := m["preferred-lifetime"].(uint32); ok {
		if fillConfig && cfg != nil {
			cfg.PreferredLifetime = ipv6RaUint32Ptr(v)
		}
		if fillState && st != nil {
			st.PreferredLifetime = ipv6RaUint32Ptr(v)
		}
	}
	if v, ok := m["disable-autoconfiguration"].(bool); ok {
		if fillConfig && cfg != nil {
			cfg.DisableAutoconfiguration = ipv6RaBoolPtr(v)
		}
		if fillState && st != nil {
			st.DisableAutoconfiguration = ipv6RaBoolPtr(v)
		}
	}
	if v, ok := m["enable-onlink"].(bool); ok {
		if fillConfig && cfg != nil {
			cfg.EnableOnlink = ipv6RaBoolPtr(v)
		}
		if fillState && st != nil {
			st.EnableOnlink = ipv6RaBoolPtr(v)
		}
	}
}

func ipv6RaPrefixApplyDbToYangFields(inParams XfmrParams, compositeKey string, vlanRouted bool, fillConfig, fillState bool,
	mergeVlan func(map[string]interface{}),
	mergeSubintf func(map[string]interface{})) {
	leafIn := inParams
	leafIn.key = compositeKey

	if m, _ := DbToYang_ipv6_ra_prefix_prefix_xfmr(leafIn); len(m) > 0 {
		if vlanRouted {
			mergeVlan(m)
		} else {
			mergeSubintf(m)
		}
	}
	fieldFns := []func(XfmrParams) (map[string]interface{}, error){
		DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr,
		DbToYang_ipv6_ra_prefix_preferred_lifetime_xfmr,
		DbToYang_ipv6_ra_prefix_disable_autoconfig_xfmr,
		DbToYang_ipv6_ra_prefix_enable_onlink_xfmr,
	}
	if vlanRouted {
		fieldFns = append(fieldFns, DbToYang_ipv6_ra_prefix_disable_advertisement_xfmr)
	}
	for _, fn := range fieldFns {
		m, err := fn(leafIn)
		if err != nil || len(m) == 0 {
			continue
		}
		if vlanRouted {
			mergeVlan(m)
		} else {
			mergeSubintf(m)
		}
	}
}

func ipv6RaPrefixDbKey(ifName, index, ocPrefix string, dbDataMap *RedisDbMap, curDb db.DBNum, d *db.DB) (string, error) {
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return "", err
	}
	ocPrefix = strings.TrimSpace(ocPrefix)
	if ocPrefix == "" {
		return "", tlerr.InvalidArgsError{Format: "IPv6 RA prefix key not present"}
	}
	if key, ok := ipv6RaPrefixDbKeyFromMap(tbl, ifName, ocPrefix, dbDataMap, curDb); ok {
		return key, nil
	}
	if curDb != db.ConfigDB {
		if key, ok := ipv6RaPrefixDbKeyFromMap(tbl, ifName, ocPrefix, dbDataMap, db.ConfigDB); ok {
			return key, nil
		}
	}
	if key, ok := lookupLiveIpv6RaPrefixKey(d, tbl, ifName, ocPrefix); ok {
		return key, nil
	}
	return "", tlerr.NotFoundError{Format: "Resource not found"}
}

func yangToDbIpv6RaPrefixField(inParams XfmrParams, field string, stringify func(interface{}) (string, error)) (map[string]string, error) {
	res := make(map[string]string)
	pathInfo := ipv6RaPrefixPathInfo(inParams.uri, inParams.requestUri)
	ifName := pathInfo.Var("name")
	index := pathInfo.Var("index")
	if _, err := ipv6RaCfgTableName(ifName, index); err != nil {
		return res, err
	}
	if inParams.oper == DELETE {
		res[field] = ""
		return res, nil
	}
	if inParams.param == nil {
		return res, nil
	}
	val, err := stringify(inParams.param)
	if err != nil || val == "" {
		return res, err
	}
	res[field] = val
	return res, nil
}

func dbToYangIpv6RaPrefixField(inParams XfmrParams, field string, yangLeaf string, asUint bool) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	inParams = ipv6RaPrefixWithConfigDb(inParams)
	ifName := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "name")
	index := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "index")
	ocPrefix := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "prefix")
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return res, err
	}
	key := inParams.key
	if key == "" || !strings.Contains(key, "|") {
		key, err = ipv6RaPrefixDbKey(ifName, index, ocPrefix, inParams.dbDataMap, inParams.curDb, inParams.d)
		if err != nil {
			return res, err
		}
	}
	entry, ok := ipv6RaPrefixLoadEntry(inParams, tbl, key)
	if !ok {
		return res, nil
	}
	raw, ok := entry.Field[field]
	if !ok || raw == "" {
		return res, nil
	}
	if asUint {
		n, err := ipv6RaUintParam(raw)
		if err != nil {
			return res, nil
		}
		u, err := parseUint32(n)
		if err != nil {
			return res, nil
		}
		res[yangLeaf] = u
		return res, nil
	}
	res[yangLeaf] = strings.EqualFold(raw, "true")
	return res, nil
}

func parseUint32(raw string) (uint32, error) {
	n, err := ipv6RaUintParam(raw)
	if err != nil {
		return 0, err
	}
	var u uint32
	_, err = fmt.Sscanf(n, "%d", &u)
	return u, err
}

func dbToYangIpv6RaPrefixNetwork(inParams XfmrParams) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	inParams = ipv6RaPrefixWithConfigDb(inParams)
	key := inParams.key
	if key == "" || !strings.Contains(key, "|") {
		ifName := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "name")
		index := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "index")
		ocPrefix := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "prefix")
		if ifName != "" && ocPrefix != "" {
			var err error
			key, err = ipv6RaPrefixDbKey(ifName, index, ocPrefix, inParams.dbDataMap, inParams.curDb, inParams.d)
			if err != nil {
				return res, nil
			}
		}
	}
	if key == "" || !strings.Contains(key, "|") {
		return res, nil
	}
	cfgPrefix := strings.SplitN(key, "|", 2)[1]
	if !strings.Contains(cfgPrefix, ":") {
		return res, nil
	}
	ipNet, err := ipv6PrefixNetwork(cfgPrefix)
	if err != nil {
		return res, nil
	}
	res["prefix"] = ipNet.String()
	return res, nil
}

var YangToDb_ipv6_ra_prefix_valid_lifetime_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaPrefixField(inParams, ipv6RaPrefixFieldValidLifetime, ipv6RaUintParam)
}

var DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaPrefixField(inParams, ipv6RaPrefixFieldValidLifetime, "valid-lifetime", true)
}

var YangToDb_ipv6_ra_prefix_preferred_lifetime_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaPrefixField(inParams, ipv6RaPrefixFieldPreferredLifetime, ipv6RaUintParam)
}

var DbToYang_ipv6_ra_prefix_preferred_lifetime_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaPrefixField(inParams, ipv6RaPrefixFieldPreferredLifetime, "preferred-lifetime", true)
}

var YangToDb_ipv6_ra_prefix_disable_advertisement_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	if err := rejectNonVlanRaPrefixDisableAdv(inParams); err != nil {
		return nil, err
	}
	return yangToDbIpv6RaPrefixField(inParams, ipv6RaPrefixFieldDisableAdv, ipv6RaBoolParam)
}

var DbToYang_ipv6_ra_prefix_disable_advertisement_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	if err := rejectNonVlanRaPrefixDisableAdv(inParams); err != nil {
		return nil, err
	}
	return dbToYangIpv6RaPrefixField(inParams, ipv6RaPrefixFieldDisableAdv, "disable-advertisement", false)
}

var YangToDb_ipv6_ra_prefix_disable_autoconfig_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaPrefixField(inParams, ipv6RaPrefixFieldDisableAutoconfig, ipv6RaBoolParam)
}

var DbToYang_ipv6_ra_prefix_disable_autoconfig_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaPrefixField(inParams, ipv6RaPrefixFieldDisableAutoconfig, "disable-autoconfiguration", false)
}

var YangToDb_ipv6_ra_prefix_enable_onlink_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaPrefixField(inParams, ipv6RaPrefixFieldEnableOnlink, ipv6RaBoolParam)
}

var DbToYang_ipv6_ra_prefix_enable_onlink_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaPrefixField(inParams, ipv6RaPrefixFieldEnableOnlink, "enable-onlink", false)
}

var YangToDb_ipv6_ra_prefix_prefix_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return map[string]string{}, nil
}

var DbToYang_ipv6_ra_prefix_prefix_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaPrefixNetwork(inParams)
}

type ipv6RaPrefixLeafSpec struct {
	suffix   string
	leafXfmr FieldXfmrYangToDb
}

func ipv6RaPrefixLeafSpecs() []ipv6RaPrefixLeafSpec {
	return []ipv6RaPrefixLeafSpec{
		{suffix: "/config/valid-lifetime", leafXfmr: YangToDb_ipv6_ra_prefix_valid_lifetime_xfmr},
		{suffix: "/state/valid-lifetime", leafXfmr: YangToDb_ipv6_ra_prefix_valid_lifetime_xfmr},
		{suffix: "/config/preferred-lifetime", leafXfmr: YangToDb_ipv6_ra_prefix_preferred_lifetime_xfmr},
		{suffix: "/state/preferred-lifetime", leafXfmr: YangToDb_ipv6_ra_prefix_preferred_lifetime_xfmr},
		{suffix: "/config/disable-advertisement", leafXfmr: YangToDb_ipv6_ra_prefix_disable_advertisement_xfmr},
		{suffix: "/state/disable-advertisement", leafXfmr: YangToDb_ipv6_ra_prefix_disable_advertisement_xfmr},
		{suffix: "/config/disable-autoconfiguration", leafXfmr: YangToDb_ipv6_ra_prefix_disable_autoconfig_xfmr},
		{suffix: "/state/disable-autoconfiguration", leafXfmr: YangToDb_ipv6_ra_prefix_disable_autoconfig_xfmr},
		{suffix: "/config/enable-onlink", leafXfmr: YangToDb_ipv6_ra_prefix_enable_onlink_xfmr},
		{suffix: "/state/enable-onlink", leafXfmr: YangToDb_ipv6_ra_prefix_enable_onlink_xfmr},
	}
}

func ipv6RaPrefixLeafSpecForUri(uriPath string) *ipv6RaPrefixLeafSpec {
	specs := ipv6RaPrefixLeafSpecs()
	for i := range specs {
		if strings.HasSuffix(uriPath, specs[i].suffix) {
			return &specs[i]
		}
	}
	return nil
}

func ipv6RaPrefixLeafParam(inParams XfmrParams) interface{} {
	req := strings.TrimSpace(inParams.requestUri)
	if req != "" && inParams.ygRoot != nil {
		node, err := yangNodeForUriGet(req, inParams.ygRoot)
		if err == nil && node != nil {
			return node
		}
	}
	return inParams.param
}

func ipv6RaPrefixLeafMutation(inParams XfmrParams, target string, spec *ipv6RaPrefixLeafSpec) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	pathInfo := ipv6RaPrefixPathInfo(inParams.uri, inParams.requestUri)
	ifName := pathInfo.Var("name")
	index := pathInfo.Var("index")
	ocPrefix := pathInfo.Var("prefix")
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return res, err
	}
	key, err := ipv6RaPrefixDbKey(ifName, index, ocPrefix, inParams.dbDataMap, inParams.curDb, inParams.d)
	if err != nil {
		return res, err
	}
	leafIn := inParams
	leafIn.uri = target
	leafIn.requestUri = inParams.requestUri
	leafIn.param = ipv6RaPrefixLeafParam(inParams)
	fields, err := spec.leafXfmr(leafIn)
	if err != nil {
		return res, err
	}
	if len(fields) == 0 {
		return res, nil
	}
	res[tbl] = map[string]db.Value{key: db.Value{Field: fields}}
	return res, nil
}

func ipv6RaPrefixInstanceDeleteMutation(inParams XfmrParams, target string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	pathInfo := ipv6RaPrefixPathInfo(inParams.uri, inParams.requestUri)
	ifName := pathInfo.Var("name")
	index := pathInfo.Var("index")
	ocPrefix := pathInfo.Var("prefix")
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return res, err
	}
	key, err := ipv6RaPrefixDbKey(ifName, index, ocPrefix, inParams.dbDataMap, inParams.curDb, inParams.d)
	if err != nil {
		return res, err
	}
	if inParams.d == nil {
		return res, tlerr.InternalError{Format: "CONFIG_DB handle not available"}
	}
	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: tbl}, db.Key{Comp: strings.SplitN(key, "|", 2)})
	if err != nil {
		return res, tlerr.NotFoundError{Format: "Resource not found"}
	}
	if isIpv6RaPrefixOnlyEntry(entry) {
		res[tbl] = map[string]db.Value{key: db.Value{}}
		return res, nil
	}
	fields := ipv6RaPrefixFieldsToClear(entry)
	if len(fields) == 0 {
		return res, nil
	}
	res[tbl] = map[string]db.Value{key: db.Value{Field: fields}}
	return res, nil
}

func getIpv6RaPrefixReadPathInfo(inParams XfmrParams) *PathInfo {
	pathInfo := NewPathInfo(inParams.uri)
	if inParams.requestUri == "" || inParams.requestUri == inParams.uri {
		return pathInfo
	}
	reqPathInfo := NewPathInfo(inParams.requestUri)
	if reqPathInfo.Var("name") != "" && pathInfo.Var("name") != "" &&
		isMoreSpecificPath(reqPathInfo.YangPath, pathInfo.YangPath) {
		return reqPathInfo
	}
	if pathInfo.Var("name") != "" {
		return pathInfo
	}
	if reqPathInfo.Var("name") != "" {
		return reqPathInfo
	}
	return pathInfo
}

func isIpv6RaPrefixKeyedGetPath(yangPath string) bool {
	norm := normalizedYangPath(yangPath)
	if !strings.Contains(norm, "/router-advertisement/prefixes") {
		return false
	}
	return !strings.HasSuffix(norm, "/prefixes")
}

// ipv6RaPrefixYgotListKey selects the OpenConfig prefix list map key. gNMI GET uses the
// URI list key (e.g. 2001:db8:104::1/64) while CONFIG_DB stores 2001:db8:104::/64.
func ipv6RaPrefixYgotListKey(uriPrefix, ocNet string) string {
	if p := strings.TrimSpace(uriPrefix); p != "" {
		return p
	}
	return ocNet
}

func ipv6RaPrefixPruneRoutedVlanDuplicateKeys(pfxs *ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv6_RouterAdvertisement_Prefixes, keepKey, ocNet string) {
	if pfxs == nil || pfxs.Prefix == nil {
		return
	}
	for k := range pfxs.Prefix {
		if k == keepKey {
			continue
		}
		if ipv6PrefixNetworksEqual(k, ocNet) {
			delete(pfxs.Prefix, k)
		}
	}
}

func ipv6RaPrefixPruneSubintfDuplicateKeys(pfxs *ocbinds.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv6_RouterAdvertisement_Prefixes, keepKey, ocNet string) {
	if pfxs == nil || pfxs.Prefix == nil {
		return
	}
	for k := range pfxs.Prefix {
		if k == keepKey {
			continue
		}
		if ipv6PrefixNetworksEqual(k, ocNet) {
			delete(pfxs.Prefix, k)
		}
	}
}

func ipv6RaPrefixFillYgotEntry(intfsObj *ocbinds.OpenconfigInterfaces_Interfaces, ifName, index, composite, ocNet, listKey string,
	inParams XfmrParams, vlanRouted, fillConfig, fillState bool) error {
	if listKey == "" {
		listKey = ocNet
	}
	if vlanRouted {
		intfObj := getIntfRoutedVlanObject(intfsObj, ifName)
		if intfObj.RoutedVlan == nil {
			ygot.BuildEmptyTree(intfObj)
			ygot.BuildEmptyTree(intfObj.RoutedVlan)
		}
		if intfObj.RoutedVlan.Ipv6 == nil {
			ygot.BuildEmptyTree(intfObj.RoutedVlan)
			ygot.BuildEmptyTree(intfObj.RoutedVlan.Ipv6)
		}
		if intfObj.RoutedVlan.Ipv6.RouterAdvertisement == nil {
			ygot.BuildEmptyTree(intfObj.RoutedVlan.Ipv6)
			ygot.BuildEmptyTree(intfObj.RoutedVlan.Ipv6.RouterAdvertisement)
		}
		pfxs := intfObj.RoutedVlan.Ipv6.RouterAdvertisement.Prefixes
		if pfxs == nil {
			ygot.BuildEmptyTree(intfObj.RoutedVlan.Ipv6.RouterAdvertisement)
			pfxs = intfObj.RoutedVlan.Ipv6.RouterAdvertisement.Prefixes
		}
		pref := pfxs.Prefix[listKey]
		if pref == nil {
			var err error
			pref, err = pfxs.NewPrefix(listKey)
			if err != nil {
				return err
			}
		}
		pref.Prefix = ipv6RaStrPtr(listKey)
		ipv6RaPreparePrefixYgot(pref, fillConfig, fillState)
		ipv6RaPrefixApplyDbToYangFields(inParams, composite, true, fillConfig, fillState,
			func(m map[string]interface{}) {
				ipv6RaMergeRoutedVlanPrefixMap(m, pref.Config, pref.State, fillConfig, fillState)
			},
			nil)
		ipv6RaPrefixPruneRoutedVlanDuplicateKeys(pfxs, listKey, ocNet)
		return nil
	}
	intfObj := getIntfObject(intfsObj, ifName)
	subIntfObj, err := getSubinterfaceObject(intfObj, parseSubIntfIndex(index))
	if err != nil {
		return err
	}
	if subIntfObj.Ipv6 == nil {
		ygot.BuildEmptyTree(subIntfObj)
		ygot.BuildEmptyTree(subIntfObj.Ipv6)
	}
	if subIntfObj.Ipv6.RouterAdvertisement == nil {
		ygot.BuildEmptyTree(subIntfObj.Ipv6)
		ygot.BuildEmptyTree(subIntfObj.Ipv6.RouterAdvertisement)
	}
	pfxs := subIntfObj.Ipv6.RouterAdvertisement.Prefixes
	if pfxs == nil {
		ygot.BuildEmptyTree(subIntfObj.Ipv6.RouterAdvertisement)
		pfxs = subIntfObj.Ipv6.RouterAdvertisement.Prefixes
	}
	pref := pfxs.Prefix[listKey]
	if pref == nil {
		pref, err = pfxs.NewPrefix(listKey)
		if err != nil {
			return err
		}
	}
	pref.Prefix = ipv6RaStrPtr(listKey)
	ipv6RaPrepareSubintfPrefixYgot(pref, fillConfig, fillState)
	ipv6RaPrefixApplyDbToYangFields(inParams, composite, false, fillConfig, fillState,
		nil,
		func(m map[string]interface{}) {
			ipv6RaMergeSubintfPrefixMap(m, pref.Config, pref.State, fillConfig, fillState)
		})
	ipv6RaPrefixPruneSubintfDuplicateKeys(pfxs, listKey, ocNet)
	return nil
}

func handleIpv6RaPrefixGetByTargetURI(inParams XfmrParams, pathInfo *PathInfo) error {
	ifName := strings.TrimSpace(pathInfo.Var("name"))
	index := strings.TrimSpace(pathInfo.Var("index"))
	ocPrefix := strings.TrimSpace(pathInfo.Var("prefix"))
	if ifName == "" {
		return nil
	}
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return err
	}
	composite, err := ipv6RaPrefixDbKey(ifName, index, ocPrefix, inParams.dbDataMap, inParams.curDb, inParams.d)
	if err != nil {
		return err
	}
	entry, ok := ipv6RaPrefixLoadEntry(inParams, tbl, composite)
	if !ok {
		return tlerr.NotFoundError{Format: "Resource not found"}
	}
	ipv6RaPrefixSeedDbMap(inParams, tbl, composite, entry)
	ocNet, err := ipv6RaPrefixOcNetworkFromComposite(composite)
	if err != nil {
		return err
	}
	fillConfig, fillState := ipv6RaPrefixDbToYangScope(inParams.uri, inParams.requestUri)
	intfsObj := getIntfsRoot(inParams.ygRoot)
	if intfsObj == nil {
		return nil
	}
	vlanRouted := strings.Contains(pathInfo.YangPath, "routed-vlan") ||
		strings.Contains(inParams.uri, "openconfig-vlan:routed-vlan") ||
		strings.Contains(inParams.requestUri, "openconfig-vlan:routed-vlan")
	listKey := ipv6RaPrefixYgotListKey(ocPrefix, ocNet)
	return ipv6RaPrefixFillYgotEntry(intfsObj, ifName, index, composite, ocNet, listKey, inParams, vlanRouted, fillConfig, fillState)
}

var DbToYang_ipv6_ra_prefixes_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	if inParams.oper != GET {
		return nil
	}
	inParams = ipv6RaPrefixWithConfigDb(inParams)
	readPI := getIpv6RaPrefixReadPathInfo(inParams)
	if isIpv6RaPrefixKeyedGetPath(readPI.YangPath) {
		return handleIpv6RaPrefixGetByTargetURI(inParams, readPI)
	}
	reqPath := ipv6RaPrefixReqPath(inParams.uri, inParams.requestUri)
	if !isIpv6RaPrefixesContainerPath(reqPath) && !isIpv6RaPrefixPath(reqPath) {
		return nil
	}
	ifName := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "name")
	index := ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "index")
	filterOC := strings.TrimSpace(ipv6RaPrefixPathVar(inParams.uri, inParams.requestUri, "prefix"))
	fillConfig, fillState := ipv6RaPrefixDbToYangScope(inParams.uri, inParams.requestUri)
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return err
	}
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		cfgDb = inParams.d
	}
	if cfgDb == nil {
		return nil
	}
	entries, err := ipv6RaPrefixListEntries(cfgDb, tbl, ifName)
	if err != nil {
		return err
	}
	for composite, entry := range entries {
		ipv6RaPrefixSeedDbMap(inParams, tbl, composite, entry)
	}
	intfsObj := getIntfsRoot(inParams.ygRoot)
	if intfsObj == nil {
		return nil
	}
	targetPath := NewPathInfo(reqPath).YangPath
	vlanRouted := strings.Contains(targetPath, "routed-vlan") ||
		strings.Contains(inParams.uri, "openconfig-vlan:routed-vlan") ||
		strings.Contains(inParams.requestUri, "openconfig-vlan:routed-vlan")

	for composite := range entries {
		ocNet, err := ipv6RaPrefixOcNetworkFromComposite(composite)
		if err != nil {
			continue
		}
		if filterOC != "" && !ipv6PrefixNetworksEqual(filterOC, ocNet) {
			continue
		}
		listKey := ocNet
		if filterOC != "" {
			listKey = ipv6RaPrefixYgotListKey(filterOC, ocNet)
		}
		if err := ipv6RaPrefixFillYgotEntry(intfsObj, ifName, index, composite, ocNet, listKey, inParams, vlanRouted, fillConfig, fillState); err != nil {
			return err
		}
		if filterOC != "" && !isIpv6RaPrefixesContainerPath(reqPath) {
			break
		}
	}
	return nil
}

// Subscribe_ipv6_ra_prefixes_xfmr: Parent-existence for .../prefixes uses verifyParentTblSubtree,
// which only invokes the Subscribe_* callback. Without this handler, leaf SET/GET under
// prefix[...] fails with "Resource not found". Use a virtual table so existence is enforced
// by intf_tbl_key_xfmr and field xfmrs (and YangToDb_ipv6_ra_prefixes_xfmr on DELETE).
var Subscribe_ipv6_ra_prefixes_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc == TRANSLATE_EXISTS || inParams.requestURI == "" {
		result.isVirtualTbl = true
		return result, nil
	}

	target := ipv6RaPrefixReqPath(inParams.uri, inParams.requestURI)
	if isIpv6RaPrefixPath(target) || isIpv6RaPrefixesContainerPath(target) {
		result.isVirtualTbl = true
		return result, nil
	}

	result.isVirtualTbl = true
	return result, nil
}

var YangToDb_ipv6_ra_prefixes_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	target := ipv6RaPrefixReqPath(inParams.uri, inParams.requestUri)

	if spec := ipv6RaPrefixLeafSpecForUri(target); spec != nil {
		switch inParams.oper {
		case UPDATE, REPLACE, CREATE, DELETE:
			if inParams.invokeCRUSubtreeOnce != nil {
				*inParams.invokeCRUSubtreeOnce = true
			}
			return ipv6RaPrefixLeafMutation(inParams, target, spec)
		}
	}

	if inParams.oper != DELETE {
		return res, nil
	}
	if isIpv6RaPrefixInstanceDeletePath(target) {
		if inParams.invokeCRUSubtreeOnce != nil {
			*inParams.invokeCRUSubtreeOnce = true
		}
		return ipv6RaPrefixInstanceDeleteMutation(inParams, target)
	}
	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}
	if !isIpv6RaPrefixesContainerPath(target) {
		return res, nil
	}
	pathInfo := NewPathInfo(inParams.uri)
	reqPI := NewPathInfo(inParams.requestUri)
	ifName := reqPI.Var("name")
	if ifName == "" {
		ifName = pathInfo.Var("name")
	}
	index := reqPI.Var("index")
	if index == "" {
		index = pathInfo.Var("index")
	}
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return res, err
	}
	dbKeys, err := ipv6RaPrefixOnlyDbKeys(inParams.d, tbl, ifName)
	if err != nil {
		return res, err
	}
	if len(dbKeys) == 0 {
		return res, nil
	}
	res[tbl] = make(map[string]db.Value)
	for _, key := range dbKeys {
		res[tbl][key] = db.Value{}
	}
	return res, nil
}
