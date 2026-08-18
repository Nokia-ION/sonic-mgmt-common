////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");          //
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
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niTableConnTbl              = "ROUTE_REDISTRIBUTE"
	niTableConnRouteMapFld      = "route_map"
	niTableConnTargetCfg        = "table-connection-config"
	niTableConnImportPolicyLeaf = "import-policy"
)

var niTableConnCfgReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/table-connections/table-connection\[src-protocol=([^\]]+)\]\[dst-protocol=([^\]]+)\]\[address-family=([^\]]+)\]/config(?:/(import-policy))?$`)

type niTableConnReqInfo struct {
	niName        string
	srcProtocol   string
	dstProtocol   string
	addressFamily string
	targetLeaf    string
	targetPath    string
}

func init() {
	XlateFuncBind("YangToDb_ni_table_conn_cfg_xfmr", YangToDb_ni_table_conn_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_table_conn_cfg_xfmr", DbToYang_ni_table_conn_cfg_xfmr)
}

func parseNiTableConnRequestUri(uri string) (*niTableConnReqInfo, error) {
	m := niTableConnCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 5 {
		leaf := ""
		if len(m) >= 6 {
			leaf = m[5]
		}
		return &niTableConnReqInfo{
			niName:        m[1],
			srcProtocol:   m[2],
			dstProtocol:   m[3],
			addressFamily: m[4],
			targetLeaf:    leaf,
			targetPath:    niTableConnTargetCfg,
		}, nil
	}

	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance table-connections mapping: " + uri,
	}
}

func niTableConnNormalizeToken(token string) string {
	v := strings.TrimSpace(token)
	if idx := strings.LastIndex(v, ":"); idx >= 0 && idx < len(v)-1 {
		v = v[idx+1:]
	}
	return strings.ToUpper(v)
}

func niTableConnProtocolToDb(proto string) (string, error) {
	switch niTableConnNormalizeToken(proto) {
	case "BGP":
		return "bgp", nil
	case "STATIC":
		return "static", nil
	case "DIRECTLY_CONNECTED":
		return "connected", nil
	case "OSPF":
		return "ospf", nil
	case "ISIS":
		return "isis", nil
	default:
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection protocol: %s", proto)}
	}
}

func niTableConnAddressFamilyToDb(af string) (string, error) {
	switch niTableConnNormalizeToken(af) {
	case "IPV4":
		return "ipv4", nil
	case "IPV6":
		return "ipv6", nil
	default:
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection address-family: %s", af)}
	}
}

func niTableConnProtocolToEnum(proto string) (ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE, error) {
	switch niTableConnNormalizeToken(proto) {
	case "BGP":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, nil
	case "STATIC":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC, nil
	case "DIRECTLY_CONNECTED":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED, nil
	case "OSPF":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_OSPF, nil
	case "ISIS":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_ISIS, nil
	default:
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_UNSET, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection protocol: %s", proto)}
	}
}

func niTableConnAddressFamilyToEnum(af string) (ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY, error) {
	switch niTableConnNormalizeToken(af) {
	case "IPV4":
		return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4, nil
	case "IPV6":
		return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6, nil
	default:
		return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_UNSET, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection address-family: %s", af)}
	}
}

func getNiTableConnCfgObj(s *ygot.GoStruct, reqInfo *niTableConnReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Config {
	if s == nil || *s == nil || reqInfo == nil {
		return nil
	}
	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(deviceObj)

	if deviceObj.NetworkInstances == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}
	niObj, ok := deviceObj.NetworkInstances.NetworkInstance[reqInfo.niName]
	if !ok || niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(reqInfo.niName)
		if err != nil || niObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(niObj)

	srcProto, err := niTableConnProtocolToEnum(reqInfo.srcProtocol)
	if err != nil {
		return nil
	}
	dstProto, err := niTableConnProtocolToEnum(reqInfo.dstProtocol)
	if err != nil {
		return nil
	}
	af, err := niTableConnAddressFamilyToEnum(reqInfo.addressFamily)
	if err != nil {
		return nil
	}

	key := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key{
		SrcProtocol:   srcProto,
		DstProtocol:   dstProto,
		AddressFamily: af,
	}
	if tcObj, ok := niObj.TableConnections.TableConnection[key]; ok && tcObj != nil {
		ygot.BuildEmptyTree(tcObj)
		return tcObj.Config
	}
	if !createIfMissing {
		return nil
	}
	tcObj, err := niObj.TableConnections.NewTableConnection(srcProto, dstProto, af)
	if err != nil || tcObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(tcObj)
	return tcObj.Config
}

func parseNiTableConnImportPolicyValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		return s, s != ""
	case *string:
		if val != nil {
			s := strings.TrimSpace(*val)
			return s, s != ""
		}
	case []string:
		for _, p := range val {
			if s := strings.TrimSpace(p); s != "" {
				return s, true
			}
		}
	case []interface{}:
		for _, it := range val {
			if s, ok := parseNiTableConnImportPolicyValue(it); ok {
				return s, true
			}
		}
	}
	return "", false
}

func parseNiTableConnImportPolicyFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if v, ok := parseNiTableConnImportPolicyValue(param); ok {
		return v, true
	}

	rv := reflect.ValueOf(param)
	for rv.IsValid() && (rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr) {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return "", false
	}

	if rv.Kind() == reflect.Struct {
		fv := rv.FieldByName("ImportPolicy")
		if fv.IsValid() && fv.CanInterface() {
			if v, ok := parseNiTableConnImportPolicyValue(fv.Interface()); ok {
				return v, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == niTableConnImportPolicyLeaf || strings.HasSuffix(k, ":"+niTableConnImportPolicyLeaf) {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if v, ok := parseNiTableConnImportPolicyValue(mv.Interface()); ok {
						return v, true
					}
				}
			}
		}
	}
	return "", false
}

func parseNiTableConnImportPolicy(inParams XfmrParams, reqInfo *niTableConnReqInfo) (string, error) {
	if v, ok := parseNiTableConnImportPolicyFromParam(inParams.param); ok {
		return v, nil
	}
	cfgObj := getNiTableConnCfgObj(inParams.ygRoot, reqInfo, false)
	if cfgObj == nil || len(cfgObj.ImportPolicy) == 0 {
		return "", tlerr.InvalidArgsError{Format: "table-connection import-policy is missing in request payload"}
	}
	for _, p := range cfgObj.ImportPolicy {
		if s := strings.TrimSpace(p); s != "" {
			return s, nil
		}
	}
	return "", tlerr.InvalidArgsError{Format: "table-connection import-policy is missing in request payload"}
}

func makeNiTableConnResMap(niName, srcProto, dstProto, af string, fields map[string]string) (map[string]map[string]db.Value, error) {
	srcDb, err := niTableConnProtocolToDb(srcProto)
	if err != nil {
		return nil, err
	}
	dstDb, err := niTableConnProtocolToDb(dstProto)
	if err != nil {
		return nil, err
	}
	afDb, err := niTableConnAddressFamilyToDb(af)
	if err != nil {
		return nil, err
	}

	return map[string]map[string]db.Value{
		niTableConnTbl: {
			niName + "|" + srcDb + "|" + dstDb + "|" + afDb: db.Value{Field: fields},
		},
	}, nil
}

func firstPolicyValueFromDb(in string) string {
	s := strings.TrimSpace(in)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ",")
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			return v
		}
	}
	return ""
}

var YangToDb_ni_table_conn_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseNiTableConnRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		if reqInfo.targetLeaf != "" && reqInfo.targetLeaf != niTableConnImportPolicyLeaf {
			return res, tlerr.NotSupportedError{Format: "DELETE supported only for /table-connections/table-connection/config or /table-connections/table-connection/config/import-policy", Path: inParams.requestUri}
		}
		return makeNiTableConnResMap(reqInfo.niName, reqInfo.srcProtocol, reqInfo.dstProtocol, reqInfo.addressFamily, map[string]string{
			niTableConnRouteMapFld: "",
		})
	}

	if reqInfo.targetLeaf != "" && reqInfo.targetLeaf != niTableConnImportPolicyLeaf {
		return res, tlerr.NotSupportedError{Format: "unsupported leaf under table-connection config mapping", Path: inParams.requestUri}
	}

	routeMap, err := parseNiTableConnImportPolicy(inParams, reqInfo)
	if err != nil {
		return res, err
	}

	return makeNiTableConnResMap(reqInfo.niName, reqInfo.srcProtocol, reqInfo.dstProtocol, reqInfo.addressFamily, map[string]string{
		niTableConnRouteMapFld: routeMap,
	})
}

var DbToYang_ni_table_conn_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiTableConnRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance table-connection config"}
	}

	srcDb, err := niTableConnProtocolToDb(reqInfo.srcProtocol)
	if err != nil {
		return err
	}
	dstDb, err := niTableConnProtocolToDb(reqInfo.dstProtocol)
	if err != nil {
		return err
	}
	afDb, err := niTableConnAddressFamilyToDb(reqInfo.addressFamily)
	if err != nil {
		return err
	}

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niTableConnTbl}, db.Key{Comp: []string{reqInfo.niName, srcDb, dstDb, afDb}})
	if err != nil {
		return err
	}

	cfgObj := getNiTableConnCfgObj(inParams.ygRoot, reqInfo, true)
	if cfgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate network-instance table-connection config object in YANG tree"}
	}

	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niTableConnImportPolicyLeaf {
		if policy, ok := entry.Field[niTableConnRouteMapFld]; ok {
			if first := firstPolicyValueFromDb(policy); first != "" {
				cfgObj.ImportPolicy = []string{first}
			}
		}
	}
	return nil
}
