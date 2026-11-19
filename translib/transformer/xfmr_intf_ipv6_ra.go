package transformer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const (
	ipv6RaFieldInterval      = "ipv6_ra_interval"
	ipv6RaFieldLifetime      = "ipv6_ra_lifetime"
	ipv6RaFieldReachableTime = "ipv6_ra_reachable_time"
	ipv6RaFieldSuppress      = "ipv6_ra_suppress"
	ipv6RaFieldEnable        = "ipv6_ra_enable"
	ipv6RaFieldManaged       = "ipv6_ra_managed"
	ipv6RaFieldOtherConfig   = "ipv6_ra_other_config"
	vlanSubIntfTable         = "VLAN_SUB_INTERFACE"
)

func init() {
	XlateFuncBind("YangToDb_ipv6_ra_interval_xfmr", YangToDb_ipv6_ra_interval_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_interval_xfmr", DbToYang_ipv6_ra_interval_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_lifetime_xfmr", YangToDb_ipv6_ra_lifetime_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_lifetime_xfmr", DbToYang_ipv6_ra_lifetime_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_reachable_time_xfmr", YangToDb_ipv6_ra_reachable_time_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_reachable_time_xfmr", DbToYang_ipv6_ra_reachable_time_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_suppress_xfmr", YangToDb_ipv6_ra_suppress_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_suppress_xfmr", DbToYang_ipv6_ra_suppress_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_enable_xfmr", YangToDb_ipv6_ra_enable_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_enable_xfmr", DbToYang_ipv6_ra_enable_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_managed_xfmr", YangToDb_ipv6_ra_managed_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_managed_xfmr", DbToYang_ipv6_ra_managed_xfmr)
	XlateFuncBind("YangToDb_ipv6_ra_other_config_xfmr", YangToDb_ipv6_ra_other_config_xfmr)
	XlateFuncBind("DbToYang_ipv6_ra_other_config_xfmr", DbToYang_ipv6_ra_other_config_xfmr)
}

func isIpv6RaPath(uriPath string) bool {
	if uriPath == "" {
		return false
	}
	return isIpv6RaPrefixPath(uriPath) ||
		strings.Contains(uriPath, "/ipv6/router-advertisement") ||
		strings.Contains(uriPath, "openconfig-if-ip:ipv6/router-advertisement") ||
		strings.Contains(uriPath, "openconfig-if-ip:router-advertisement")
}

// ipv6RaCfgTableName selects the CONFIG_DB L3 interface table for RA fields.
func ipv6RaCfgTableName(ifName, index string) (string, error) {
	if ifName == "" {
		return "", tlerr.InvalidArgsError{Format: "Interface KEY not present"}
	}
	if index != "" && index != "0" {
		return vlanSubIntfTable, nil
	}
	intfType, _, err := getIntfTypeByName(ifName)
	if err != nil || intfType == IntfTypeUnset {
		return "", tlerr.InvalidArgsError{Format: "Unsupported Interface: " + ifName}
	}
	if intfType == IntfTypeLoopback {
		return "", tlerr.NotSupported("IPv6 router advertisement is not supported on Loopback interfaces")
	}
	return IntfTypeTblMap[intfType].cfgDb.intfTN, nil
}

func ipv6RaDbKey(ifName, index string) string {
	if index != "" && index != "0" {
		return ifName + "." + index
	}
	return ifName
}

func ipv6RaUintParam(param interface{}) (string, error) {
	switch v := param.(type) {
	case *uint32:
		if v == nil {
			return "", nil
		}
		return strconv.FormatUint(uint64(*v), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(v), 10), nil
	case *uint64:
		if v == nil {
			return "", nil
		}
		return strconv.FormatUint(*v, 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case string:
		if strings.TrimSpace(v) == "" {
			return "", nil
		}
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
		if err != nil {
			return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid IPv6 RA integer %q", v)}
		}
		return strconv.FormatUint(n, 10), nil
	case float64:
		return strconv.FormatUint(uint64(v), 10), nil
	default:
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported IPv6 RA integer type %T", param)}
	}
}

func ipv6RaBoolParam(param interface{}) (string, error) {
	switch v := param.(type) {
	case *bool:
		if v == nil {
			return "", nil
		}
		return strconv.FormatBool(*v), nil
	case bool:
		return strconv.FormatBool(v), nil
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		switch s {
		case "true", "false":
			return s, nil
		default:
			return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid IPv6 RA boolean value %q", v)}
		}
	default:
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported IPv6 RA boolean type %T", param)}
	}
}

func ipv6RaIfacePathInfo(uri, requestUri string) *PathInfo {
	if isIpv6RaPath(requestUri) {
		return NewPathInfo(requestUri)
	}
	if isIpv6RaPath(uri) {
		return NewPathInfo(uri)
	}
	return NewPathInfo(requestUri)
}

func yangToDbIpv6RaField(inParams XfmrParams, field string, stringify func(interface{}) (string, error)) (map[string]string, error) {
	res := make(map[string]string)
	pathInfo := ipv6RaIfacePathInfo(inParams.uri, inParams.requestUri)
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

func dbToYangIpv6RaField(inParams XfmrParams, field string, yangLeaf string, asUint bool) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	if inParams.dbDataMap == nil {
		return res, nil
	}
	pathInfo := ipv6RaIfacePathInfo(inParams.uri, inParams.requestUri)
	ifName := pathInfo.Var("name")
	index := pathInfo.Var("index")
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return res, err
	}
	key := inParams.key
	if key == "" {
		key = ipv6RaDbKey(ifName, index)
	}
	data := (*inParams.dbDataMap)[inParams.curDb]
	entry, ok := data[tbl][key]
	if !ok || entry.Field == nil {
		return res, nil
	}
	raw, ok := entry.Field[field]
	if !ok || raw == "" {
		return res, nil
	}
	if asUint {
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return res, nil
		}
		u := uint32(n)
		res[yangLeaf] = u
		return res, nil
	}
	res[yangLeaf] = strings.EqualFold(raw, "true")
	return res, nil
}

func dbToYangIpv6RaStringField(inParams XfmrParams, field string, yangLeaf string) (map[string]interface{}, error) {
	res := make(map[string]interface{})
	if inParams.dbDataMap == nil {
		return res, nil
	}
	pathInfo := ipv6RaIfacePathInfo(inParams.uri, inParams.requestUri)
	ifName := pathInfo.Var("name")
	index := pathInfo.Var("index")
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return res, err
	}
	key := inParams.key
	if key == "" {
		key = ipv6RaDbKey(ifName, index)
	}
	data := (*inParams.dbDataMap)[inParams.curDb]
	entry, ok := data[tbl][key]
	if !ok || entry.Field == nil {
		return res, nil
	}
	raw, ok := entry.Field[field]
	if !ok || raw == "" {
		return res, nil
	}
	res[yangLeaf] = raw
	return res, nil
}

var YangToDb_ipv6_ra_interval_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaField(inParams, ipv6RaFieldInterval, ipv6RaUintParam)
}

var DbToYang_ipv6_ra_interval_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaField(inParams, ipv6RaFieldInterval, "interval", true)
}

var YangToDb_ipv6_ra_lifetime_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaField(inParams, ipv6RaFieldLifetime, ipv6RaUintParam)
}

var DbToYang_ipv6_ra_lifetime_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaField(inParams, ipv6RaFieldLifetime, "lifetime", true)
}

var YangToDb_ipv6_ra_reachable_time_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaField(inParams, ipv6RaFieldReachableTime, ipv6RaUintParam)
}

var DbToYang_ipv6_ra_reachable_time_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaField(inParams, ipv6RaFieldReachableTime, "reachable-time", true)
}

var YangToDb_ipv6_ra_suppress_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaField(inParams, ipv6RaFieldSuppress, ipv6RaBoolParam)
}

var DbToYang_ipv6_ra_suppress_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaField(inParams, ipv6RaFieldSuppress, "suppress", false)
}

var YangToDb_ipv6_ra_enable_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaField(inParams, ipv6RaFieldEnable, ipv6RaBoolParam)
}

var DbToYang_ipv6_ra_enable_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaField(inParams, ipv6RaFieldEnable, "enable", false)
}

var YangToDb_ipv6_ra_managed_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaField(inParams, ipv6RaFieldManaged, ipv6RaBoolParam)
}

var DbToYang_ipv6_ra_managed_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaField(inParams, ipv6RaFieldManaged, "managed", false)
}

var YangToDb_ipv6_ra_other_config_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	return yangToDbIpv6RaField(inParams, ipv6RaFieldOtherConfig, ipv6RaBoolParam)
}

var DbToYang_ipv6_ra_other_config_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return dbToYangIpv6RaField(inParams, ipv6RaFieldOtherConfig, "other-config", false)
}

// ipv6RaTableList is used by intf_table_xfmr for RA config/state paths.
func ipv6RaTableList(ifName, index string, curDb db.DBNum) ([]string, error) {
	tbl, err := ipv6RaCfgTableName(ifName, index)
	if err != nil {
		return nil, err
	}
	if curDb == db.ApplDB {
		// RA is CONFIG_DB only; GET state still reads config fields.
		return []string{tbl}, nil
	}
	return []string{tbl}, nil
}
