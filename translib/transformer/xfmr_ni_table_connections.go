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
	"sort"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niTableConnTbl              = "ROUTE_REDISTRIBUTE"
	niTableConnRouteMapFld      = "route_map"
	niTableConnTargetRoot       = "table-connections-root"
	niTableConnTargetInst       = "table-connection-instance"
	niTableConnTargetCfg        = "table-connection-config"
	niTableConnTargetState      = "table-connection-state"
	niTableConnImportPolicyLeaf = "import-policy"
	niTableConnSrcProtoLeaf     = "src-protocol"
	niTableConnDstProtoLeaf     = "dst-protocol"
	niTableConnAddrFamilyLeaf   = "address-family"
)

const niTableConnUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/table-connections`

// The three list keys are the CONFIG_DB row identity, so every path below the
// list carries all of them. The block is captured whole and split afterwards,
// which is what lets the keys arrive in any order.
const niTableConnKeyBlock = `((?:\[[^\]]+\]){3})`

var niTableConnCfgReqUriRe = regexp.MustCompile(niTableConnUriPfx + `/table-connection` + niTableConnKeyBlock + `/config(?:/(import-policy|src-protocol|dst-protocol|address-family))?$`)
var niTableConnStateReqUriRe = regexp.MustCompile(niTableConnUriPfx + `/table-connection` + niTableConnKeyBlock + `/state(?:/(import-policy|src-protocol|dst-protocol|address-family))?$`)
var niTableConnInstKeyLeafReqUriRe = regexp.MustCompile(niTableConnUriPfx + `/table-connection` + niTableConnKeyBlock + `/(src-protocol|dst-protocol|address-family)$`)
var niTableConnInstReqUriRe = regexp.MustCompile(niTableConnUriPfx + `/table-connection` + niTableConnKeyBlock + `$`)
var niTableConnListReqUriRe = regexp.MustCompile(niTableConnUriPfx + `/table-connection$`)
var niTableConnRootReqUriRe = regexp.MustCompile(niTableConnUriPfx + `$`)
var niTableConnListKeyRe = regexp.MustCompile(`\[(src-protocol|dst-protocol|address-family)=([^\]]+)\]`)

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

// splitNiTableConnKeys pulls the three list keys out of a captured key block.
// They are named in the URI, so the order they appear in does not matter.
func splitNiTableConnKeys(keyBlock, uri string) (string, string, string, error) {
	unsupported := tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance table-connections mapping: " + uri,
	}
	keyPairs := niTableConnListKeyRe.FindAllStringSubmatch(keyBlock, -1)
	if len(keyPairs) != 3 {
		return "", "", "", unsupported
	}
	keys := map[string]string{}
	for _, p := range keyPairs {
		if len(p) == 3 {
			keys[p[1]] = p[2]
		}
	}
	srcProtocol, srcOK := keys[niTableConnSrcProtoLeaf]
	dstProtocol, dstOK := keys[niTableConnDstProtoLeaf]
	addressFamily, afOK := keys[niTableConnAddrFamilyLeaf]
	if !srcOK || !dstOK || !afOK {
		return "", "", "", unsupported
	}
	return srcProtocol, dstProtocol, addressFamily, nil
}

func parseNiTableConnRequestUri(uri string) (*niTableConnReqInfo, error) {
	if m := niTableConnCfgReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		srcProtocol, dstProtocol, addressFamily, err := splitNiTableConnKeys(m[2], uri)
		if err != nil {
			return nil, err
		}
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niTableConnReqInfo{
			niName:        m[1],
			srcProtocol:   srcProtocol,
			dstProtocol:   dstProtocol,
			addressFamily: addressFamily,
			targetLeaf:    leaf,
			targetPath:    niTableConnTargetCfg,
		}, nil
	}
	if m := niTableConnStateReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		srcProtocol, dstProtocol, addressFamily, err := splitNiTableConnKeys(m[2], uri)
		if err != nil {
			return nil, err
		}
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niTableConnReqInfo{
			niName:        m[1],
			srcProtocol:   srcProtocol,
			dstProtocol:   dstProtocol,
			addressFamily: addressFamily,
			targetLeaf:    leaf,
			targetPath:    niTableConnTargetState,
		}, nil
	}
	// A key leaf below the list instance repeats a key already in the URI, so it
	// resolves to the same row as the instance itself.
	if m := niTableConnInstKeyLeafReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		srcProtocol, dstProtocol, addressFamily, err := splitNiTableConnKeys(m[2], uri)
		if err != nil {
			return nil, err
		}
		return &niTableConnReqInfo{
			niName:        m[1],
			srcProtocol:   srcProtocol,
			dstProtocol:   dstProtocol,
			addressFamily: addressFamily,
			targetLeaf:    m[3],
			targetPath:    niTableConnTargetInst,
		}, nil
	}
	if m := niTableConnInstReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		srcProtocol, dstProtocol, addressFamily, err := splitNiTableConnKeys(m[2], uri)
		if err != nil {
			return nil, err
		}
		return &niTableConnReqInfo{
			niName:        m[1],
			srcProtocol:   srcProtocol,
			dstProtocol:   dstProtocol,
			addressFamily: addressFamily,
			targetPath:    niTableConnTargetInst,
		}, nil
	}
	// A request naming the list but no instance covers every table-connection,
	// and is mapped and populated the same way as one naming the container.
	if m := niTableConnListReqUriRe.FindStringSubmatch(uri); len(m) >= 2 {
		return &niTableConnReqInfo{niName: m[1], targetPath: niTableConnTargetRoot}, nil
	}
	if m := niTableConnRootReqUriRe.FindStringSubmatch(uri); len(m) >= 2 {
		return &niTableConnReqInfo{niName: m[1], targetPath: niTableConnTargetRoot}, nil
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
	case "LOCAL_AGGREGATE":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE, nil
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

// niTableConnProtocolEnumToDb maps a payload or key enum onto the CONFIG_DB key
// component, which is how a container level request names its rows.
func niTableConnProtocolEnumToDb(proto ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE) (string, error) {
	switch proto {
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP:
		return "bgp", nil
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC:
		return "static", nil
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED:
		return "connected", nil
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_OSPF:
		return "ospf", nil
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_ISIS:
		return "isis", nil
	default:
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection protocol: %v", proto)}
	}
}

func niTableConnAddressFamilyEnumToDb(af ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY) (string, error) {
	switch af {
	case ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4:
		return "ipv4", nil
	case ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6:
		return "ipv6", nil
	default:
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection address-family: %v", af)}
	}
}

func niTableConnProtocolDbToEnum(proto string) (ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE, error) {
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "bgp":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, nil
	case "static":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC, nil
	case "connected":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED, nil
	case "ospf":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_OSPF, nil
	case "isis":
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_ISIS, nil
	default:
		return ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_UNSET, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection protocol in CONFIG_DB: %s", proto)}
	}
}

func niTableConnAddressFamilyDbToEnum(af string) (ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY, error) {
	switch strings.ToLower(strings.TrimSpace(af)) {
	case "ipv4":
		return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4, nil
	case "ipv6":
		return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6, nil
	default:
		return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_UNSET, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection address-family in CONFIG_DB: %s", af)}
	}
}

func niTableConnYgotKey(reqInfo *niTableConnReqInfo) (ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key, error) {
	var key ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key
	srcProto, err := niTableConnProtocolToEnum(reqInfo.srcProtocol)
	if err != nil {
		return key, err
	}
	dstProto, err := niTableConnProtocolToEnum(reqInfo.dstProtocol)
	if err != nil {
		return key, err
	}
	af, err := niTableConnAddressFamilyToEnum(reqInfo.addressFamily)
	if err != nil {
		return key, err
	}
	key.SrcProtocol = srcProto
	key.DstProtocol = dstProto
	key.AddressFamily = af
	return key, nil
}

// getNiTableConnsObj reaches the table-connections container, which is the
// anchor for a container or keyless list request.
func getNiTableConnsObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections {
	if s == nil || *s == nil {
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
	niObj, ok := deviceObj.NetworkInstances.NetworkInstance[niName]
	if !ok || niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(niName)
		if err != nil || niObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(niObj)
	return niObj.TableConnections
}

func getNiTableConnInstObj(s *ygot.GoStruct, reqInfo *niTableConnReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection {
	key, err := niTableConnYgotKey(reqInfo)
	if err != nil {
		return nil
	}
	return getNiTableConnInstObjByKey(s, reqInfo.niName, key, createIfMissing)
}

func getNiTableConnInstObjByKey(s *ygot.GoStruct, niName string, key ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection {
	tcsObj := getNiTableConnsObj(s, niName, createIfMissing)
	if tcsObj == nil {
		return nil
	}
	if tcObj, ok := tcsObj.TableConnection[key]; ok && tcObj != nil {
		ygot.BuildEmptyTree(tcObj)
		return tcObj
	}
	if !createIfMissing {
		return nil
	}
	tcObj, newErr := tcsObj.NewTableConnection(key.SrcProtocol, key.DstProtocol, key.AddressFamily)
	if newErr != nil || tcObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(tcObj)
	return tcObj
}

func getNiTableConnCfgObj(s *ygot.GoStruct, reqInfo *niTableConnReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Config {
	tcObj := getNiTableConnInstObj(s, reqInfo, createIfMissing)
	if tcObj == nil {
		return nil
	}
	return tcObj.Config
}

// wrapNiTableConnRootAsDevice normalizes the request root to a device object so
// that one set of tree walks serves every anchor path in the hierarchy.
func getNiTableConnStateObj(s *ygot.GoStruct, reqInfo *niTableConnReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_State {
	tcObj := getNiTableConnInstObj(s, reqInfo, createIfMissing)
	if tcObj == nil {
		return nil
	}
	return tcObj.State
}

func wrapNiTableConnRootAsDevice(s *ygot.GoStruct, reqInfo *niTableConnReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for table-connection mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}

	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	if devObj.NetworkInstances == nil {
		devObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}
	niObj, err := devObj.NetworkInstances.NewNetworkInstance(reqInfo.niName)
	if err != nil || niObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for table-connection mapping"}
	}
	ygot.BuildEmptyTree(niObj)

	// Reuse the original subtree pointers so that DbToYang population updates
	// the caller owned objects regardless of the anchor path.
	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections:
		niObj.TableConnections = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection:
		key, keyErr := niTableConnYgotKey(reqInfo)
		if keyErr != nil {
			return nil, keyErr
		}
		if niObj.TableConnections == nil {
			niObj.TableConnections = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections{}
		}
		if niObj.TableConnections.TableConnection == nil {
			niObj.TableConnections.TableConnection = map[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection{}
		}
		niObj.TableConnections.TableConnection[key] = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Config:
		var devRoot ygot.GoStruct = devObj
		tcObj := getNiTableConnInstObj(&devRoot, reqInfo, true)
		if tcObj == nil {
			return nil, tlerr.InvalidArgsError{Format: "unable to create table-connection context for table-connection mapping"}
		}
		tcObj.Config = root
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root for table-connection mapping"}
	}

	var wrapped ygot.GoStruct = devObj
	return &wrapped, nil
}

// niTableConnMappedPayloadLeaves lists every leaf, relative to the
// table-connections container, that a request may carry. The list keys are
// included because a payload repeats them alongside the data they identify.
var niTableConnMappedPayloadLeaves = map[string]bool{
	"table-connection/src-protocol":          true,
	"table-connection/dst-protocol":          true,
	"table-connection/address-family":        true,
	"table-connection/config/src-protocol":   true,
	"table-connection/config/dst-protocol":   true,
	"table-connection/config/address-family": true,
	"table-connection/config/import-policy":  true,
}

// findNiTableConnUnmappedPayloadLeaves reports payload leaves that have no
// CONFIG_DB mapping, so that a request carrying them is refused instead of
// being silently reduced to the leaves that do map. Each instance is walked
// separately so that the reported path names its keys the way the URI does.
func findNiTableConnUnmappedPayloadLeaves(s *ygot.GoStruct, niName string) []string {
	tcsObj := getNiTableConnsObj(s, niName, false)
	if tcsObj == nil {
		return nil
	}
	var unmapped []string
	for key, tcObj := range tcsObj.TableConnection {
		if tcObj == nil {
			continue
		}
		display := fmt.Sprintf("table-connections/table-connection[src-protocol=%s][dst-protocol=%s][address-family=%s]",
			niTableConnProtocolEnumToOcName(key.SrcProtocol),
			niTableConnProtocolEnumToOcName(key.DstProtocol),
			niTableConnAddressFamilyEnumToOcName(key.AddressFamily))
		collectOcUnmappedPayloadLeaves(reflect.ValueOf(tcObj), niTableConnMappedPayloadLeaves, "table-connection", display, 0, &unmapped)
	}
	sort.Strings(unmapped)
	return unmapped
}

func niTableConnProtocolEnumToOcName(proto ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE) string {
	switch proto {
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP:
		return "BGP"
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC:
		return "STATIC"
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED:
		return "DIRECTLY_CONNECTED"
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_OSPF:
		return "OSPF"
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_ISIS:
		return "ISIS"
	case ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE:
		return "LOCAL_AGGREGATE"
	default:
		return fmt.Sprintf("%v", int64(proto))
	}
}

func niTableConnAddressFamilyEnumToOcName(af ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY) string {
	switch af {
	case ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4:
		return "IPV4"
	case ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6:
		return "IPV6"
	default:
		return fmt.Sprintf("%v", int64(af))
	}
}

func parseNiTableConnImportPolicyValues(v interface{}) []string {
	out := make([]string, 0)
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s != "" {
			out = append(out, s)
		}
	case *string:
		if val != nil {
			s := strings.TrimSpace(*val)
			if s != "" {
				out = append(out, s)
			}
		}
	case []string:
		for _, p := range val {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
	case []interface{}:
		for _, it := range val {
			out = append(out, parseNiTableConnImportPolicyValues(it)...)
		}
	}
	return out
}

func parseNiTableConnImportPolicyFromParam(param interface{}) ([]string, bool) {
	if param == nil {
		return nil, false
	}
	if values := parseNiTableConnImportPolicyValues(param); len(values) > 0 {
		return values, true
	}

	rv := reflect.ValueOf(param)
	for rv.IsValid() && (rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr) {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil, false
	}

	if rv.Kind() == reflect.Struct {
		fv := rv.FieldByName("ImportPolicy")
		if fv.IsValid() && fv.CanInterface() {
			if values := parseNiTableConnImportPolicyValues(fv.Interface()); len(values) > 0 {
				return values, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == niTableConnImportPolicyLeaf || strings.HasSuffix(k, ":"+niTableConnImportPolicyLeaf) {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if values := parseNiTableConnImportPolicyValues(mv.Interface()); len(values) > 0 {
						return values, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseNiTableConnSingleImportPolicy(values []string) (string, error) {
	if len(values) == 0 {
		return "", tlerr.InvalidArgsError{Format: "table-connection import-policy is missing in request payload"}
	}
	if len(values) > 1 {
		return "", tlerr.InvalidArgsError{
			Format: fmt.Sprintf("table-connection import-policy supports exactly one policy, got %d", len(values)),
		}
	}
	return values[0], nil
}

// niTableConnImportPolicyFromReq resolves the route-map a request names, and
// reports whether it named one at all. A request that carries no policy is
// valid at and above the config container: it asks for a redistribution with
// no route-map attached.
func niTableConnImportPolicyFromReq(inParams XfmrParams, root *ygot.GoStruct, reqInfo *niTableConnReqInfo) (string, bool, error) {
	values, ok := parseNiTableConnImportPolicyFromParam(inParams.param)
	if !ok {
		if cfgObj := getNiTableConnCfgObj(root, reqInfo, false); cfgObj != nil {
			for _, p := range cfgObj.ImportPolicy {
				if s := strings.TrimSpace(p); s != "" {
					values = append(values, s)
				}
			}
		}
	}
	if len(values) == 0 {
		return "", false, nil
	}
	policy, err := parseNiTableConnSingleImportPolicy(values)
	if err != nil {
		return "", true, err
	}
	return policy, true, nil
}

// niTableConnPayloadKeyLeafDbValue reads a list key leaf from the payload. An
// unrecognized shape yields false, which skips the comparison rather than
// refusing a write whose keys translib has already validated.
func niTableConnPayloadKeyLeafDbValue(param interface{}, leaf string) (string, bool) {
	toDb := niTableConnProtocolToDb
	if leaf == niTableConnAddrFamilyLeaf {
		toDb = niTableConnAddressFamilyToDb
	}
	switch v := param.(type) {
	case string:
		if dbVal, err := toDb(v); err == nil {
			return dbVal, true
		}
	case *string:
		if v == nil {
			return "", false
		}
		if dbVal, err := toDb(*v); err == nil {
			return dbVal, true
		}
	case ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE:
		if dbVal, err := niTableConnProtocolEnumToDb(v); err == nil {
			return dbVal, true
		}
	case ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY:
		if dbVal, err := niTableConnAddressFamilyEnumToDb(v); err == nil {
			return dbVal, true
		}
	}
	return "", false
}

// niTableConnCheckPayloadKeyLeaf refuses a key leaf write whose value disagrees
// with the key named in the URI, which would otherwise land on the row the URI
// names and quietly ignore the payload.
func niTableConnCheckPayloadKeyLeaf(inParams XfmrParams, reqInfo *niTableConnReqInfo) error {
	var uriValue string
	switch reqInfo.targetLeaf {
	case niTableConnSrcProtoLeaf:
		uriValue = reqInfo.srcProtocol
	case niTableConnDstProtoLeaf:
		uriValue = reqInfo.dstProtocol
	case niTableConnAddrFamilyLeaf:
		uriValue = reqInfo.addressFamily
	default:
		return nil
	}

	payloadValue, ok := niTableConnPayloadKeyLeafDbValue(inParams.param, reqInfo.targetLeaf)
	if !ok {
		return nil
	}
	toDb := niTableConnProtocolToDb
	if reqInfo.targetLeaf == niTableConnAddrFamilyLeaf {
		toDb = niTableConnAddressFamilyToDb
	}
	expected, err := toDb(uriValue)
	if err != nil {
		return err
	}
	if payloadValue != expected {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("table-connection %s in the payload does not match %q named in the path", reqInfo.targetLeaf, uriValue),
		}
	}
	return nil
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

// buildNiTableConnHierResMap maps every table-connection a container level
// payload carries, so that one request can create or update several rows.
func buildNiTableConnHierResMap(s *ygot.GoStruct, niName string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	tcsObj := getNiTableConnsObj(s, niName, false)
	if tcsObj == nil || len(tcsObj.TableConnection) == 0 {
		return res, nil
	}

	rows := make(map[string]db.Value)
	for key, tcObj := range tcsObj.TableConnection {
		if tcObj == nil || key.SrcProtocol == ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE {
			continue
		}
		srcDb, err := niTableConnProtocolEnumToDb(key.SrcProtocol)
		if err != nil {
			return nil, err
		}
		dstDb, err := niTableConnProtocolEnumToDb(key.DstProtocol)
		if err != nil {
			return nil, err
		}
		afDb, err := niTableConnAddressFamilyEnumToDb(key.AddressFamily)
		if err != nil {
			return nil, err
		}

		fields := map[string]string{}
		if tcObj.Config != nil {
			values := make([]string, 0, len(tcObj.Config.ImportPolicy))
			for _, p := range tcObj.Config.ImportPolicy {
				if s := strings.TrimSpace(p); s != "" {
					values = append(values, s)
				}
			}
			if len(values) > 0 {
				policy, err := parseNiTableConnSingleImportPolicy(values)
				if err != nil {
					return nil, err
				}
				fields[niTableConnRouteMapFld] = policy
			}
		}
		rows[niName+"|"+srcDb+"|"+dstDb+"|"+afDb] = db.Value{Field: fields}
	}

	if len(rows) > 0 {
		res[niTableConnTbl] = rows
	}
	return res, nil
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

	reqInfo, err := parseNiTableConnRequestUri(niHandlerReqUri(inParams))
	if err != nil {
		return res, err
	}

	// The state container mirrors the config one for reads only, so a write
	// aimed at it is refused rather than quietly landing on the config fields.
	if reqInfo.targetPath == niTableConnTargetState {
		return res, tlerr.NotSupportedError{
			Format: "table-connection state is read-only; write to the config container instead",
			Path:   inParams.requestUri,
		}
	}

	if reqInfo.targetPath != niTableConnTargetRoot && niTableConnIsLocalAggr(reqInfo.srcProtocol) {
		root, wrapErr := wrapNiTableConnRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			root = inParams.ygRoot
		}
		return niLocalAggrTableConnYangToDb(inParams, reqInfo, root)
	}

	if inParams.oper == DELETE {
		switch reqInfo.targetPath {
		case niTableConnTargetRoot:
			return res, tlerr.NotSupportedError{
				Format: "DELETE is not supported at the table-connections container level; delete a table-connection instead",
				Path:   inParams.requestUri,
			}
		case niTableConnTargetInst:
			if reqInfo.targetLeaf != "" {
				// The three keys are the row identity, so there is no field to
				// clear; removing one of them means removing the row.
				return res, tlerr.NotSupportedError{
					Format: "DELETE is not supported for a table-connection list key; delete the table-connection instead",
					Path:   inParams.requestUri,
				}
			}
			return makeNiTableConnResMap(reqInfo.niName, reqInfo.srcProtocol, reqInfo.dstProtocol, reqInfo.addressFamily, nil)
		case niTableConnTargetCfg:
			switch reqInfo.targetLeaf {
			case "", niTableConnImportPolicyLeaf:
				return makeNiTableConnResMap(reqInfo.niName, reqInfo.srcProtocol, reqInfo.dstProtocol, reqInfo.addressFamily, map[string]string{
					niTableConnRouteMapFld: "",
				})
			default:
				return res, tlerr.NotSupportedError{
					Format: "DELETE is not supported for a table-connection list key; delete the table-connection instead",
					Path:   inParams.requestUri,
				}
			}
		}
		return res, tlerr.NotSupportedError{Format: "unsupported DELETE path under table-connection mapping", Path: inParams.requestUri}
	}

	// An anchor this mapping cannot place in a device tree still carries its
	// values in inParams.param, so the walk is skipped rather than the request
	// refused.
	wrappedRoot, wrapErr := wrapNiTableConnRootAsDevice(inParams.ygRoot, reqInfo)
	if wrapErr != nil {
		wrappedRoot = inParams.ygRoot
	} else if unmapped := findNiTableConnUnmappedPayloadLeaves(wrappedRoot, reqInfo.niName); len(unmapped) > 0 {
		return res, tlerr.NotSupportedError{
			Format: "table-connections payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
			Path:   inParams.requestUri,
		}
	}

	switch reqInfo.targetPath {
	case niTableConnTargetRoot:
		res, err = buildNiTableConnHierResMap(wrappedRoot, reqInfo.niName)
		if err != nil {
			return make(map[string]map[string]db.Value), err
		}
		aggrRes, aggrErr := niLocalAggrTableConnHierRows(inParams.dbs[db.ConfigDB], wrappedRoot, reqInfo.niName)
		if aggrErr != nil {
			return make(map[string]map[string]db.Value), aggrErr
		}
		for tbl, rows := range aggrRes {
			res[tbl] = rows
		}
		if len(res) == 0 && !niLocalAggrTableConnInPayload(wrappedRoot, reqInfo.niName) {
			return res, tlerr.InvalidArgsError{Format: "table-connections container payload does not include any table-connection"}
		}
		if inParams.invokeCRUSubtreeOnce != nil {
			// The whole subtree is mapped in this call, so translib must not
			// invoke this transformer again for the child nodes.
			*inParams.invokeCRUSubtreeOnce = true
		}
		return res, nil
	case niTableConnTargetInst, niTableConnTargetCfg:
		if err := niTableConnCheckPayloadKeyLeaf(inParams, reqInfo); err != nil {
			return res, err
		}

		fields := map[string]string{}
		switch reqInfo.targetLeaf {
		case niTableConnImportPolicyLeaf:
			// A leaf targeted write names the policy, so a missing one is an error.
			policy, _, policyErr := niTableConnImportPolicyFromReq(inParams, wrappedRoot, reqInfo)
			if policyErr != nil {
				return res, policyErr
			}
			if policy == "" {
				return res, tlerr.InvalidArgsError{Format: "table-connection import-policy is missing in request payload"}
			}
			fields[niTableConnRouteMapFld] = policy
		case "":
			policy, found, policyErr := niTableConnImportPolicyFromReq(inParams, wrappedRoot, reqInfo)
			if policyErr != nil {
				return res, policyErr
			}
			if found {
				fields[niTableConnRouteMapFld] = policy
			}
		}

		if reqInfo.targetPath == niTableConnTargetInst && inParams.invokeCRUSubtreeOnce != nil {
			*inParams.invokeCRUSubtreeOnce = true
		}
		return makeNiTableConnResMap(reqInfo.niName, reqInfo.srcProtocol, reqInfo.dstProtocol, reqInfo.addressFamily, fields)
	}

	return res, tlerr.NotSupportedError{Format: "unsupported path for network-instance table-connections mapping", Path: inParams.requestUri}
}

// niTableConnDbRowsForNi lists the CONFIG_DB rows that belong to one
// network-instance, which is what a container level read has to enumerate.
func niTableConnDbRowsForNi(cfgDb *db.DB, niName string) ([]db.Key, error) {
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niTableConnTbl})
	if err != nil {
		return nil, err
	}
	rows := make([]db.Key, 0, len(keys))
	for _, k := range keys {
		if len(k.Comp) == 4 && k.Comp[0] == niName {
			rows = append(rows, k)
		}
	}
	return rows, nil
}

func niTableConnSetInstanceKeys(tcObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection, key ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key) {
	tcObj.SrcProtocol = key.SrcProtocol
	tcObj.DstProtocol = key.DstProtocol
	tcObj.AddressFamily = key.AddressFamily
	if tcObj.Config != nil {
		tcObj.Config.SrcProtocol = key.SrcProtocol
		tcObj.Config.DstProtocol = key.DstProtocol
		tcObj.Config.AddressFamily = key.AddressFamily
	}
	// The state container has no datastore of its own; it reports back the same
	// ROUTE_REDISTRIBUTE row the config container is mapped to.
	if tcObj.State != nil {
		tcObj.State.SrcProtocol = key.SrcProtocol
		tcObj.State.DstProtocol = key.DstProtocol
		tcObj.State.AddressFamily = key.AddressFamily
	}
}

func niTableConnSetStateImportPolicy(stObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_State, entry db.Value) {
	if stObj == nil {
		return
	}
	if policy, ok := entry.Field[niTableConnRouteMapFld]; ok {
		if first := firstPolicyValueFromDb(policy); first != "" {
			stObj.ImportPolicy = []string{first}
		}
	}
}

func niTableConnSetImportPolicy(cfgObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Config, entry db.Value) {
	if cfgObj == nil {
		return
	}
	if policy, ok := entry.Field[niTableConnRouteMapFld]; ok {
		if first := firstPolicyValueFromDb(policy); first != "" {
			cfgObj.ImportPolicy = []string{first}
		}
	}
}

// readNiTableConnAll populates every table-connection of a network-instance,
// which serves a read anchored on the container or on the keyless list.
func readNiTableConnAll(inParams XfmrParams, reqInfo *niTableConnReqInfo, cfgDb *db.DB) error {
	rows, err := niTableConnDbRowsForNi(cfgDb, reqInfo.niName)
	if err != nil {
		return err
	}

	wrappedRoot, wrapErr := wrapNiTableConnRootAsDevice(inParams.ygRoot, reqInfo)
	if wrapErr != nil {
		return wrapErr
	}

	for _, row := range rows {
		srcProto, err := niTableConnProtocolDbToEnum(row.Comp[1])
		if err != nil {
			return err
		}
		dstProto, err := niTableConnProtocolDbToEnum(row.Comp[2])
		if err != nil {
			return err
		}
		af, err := niTableConnAddressFamilyDbToEnum(row.Comp[3])
		if err != nil {
			return err
		}
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niTableConnTbl}, row)
		if err != nil {
			return err
		}

		key := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key{
			SrcProtocol:   srcProto,
			DstProtocol:   dstProto,
			AddressFamily: af,
		}
		tcObj := getNiTableConnInstObjByKey(wrappedRoot, reqInfo.niName, key, true)
		if tcObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate network-instance table-connection object in YANG tree"}
		}
		niTableConnSetInstanceKeys(tcObj, key)
		niTableConnSetImportPolicy(tcObj.Config, entry)
		niTableConnSetStateImportPolicy(tcObj.State, entry)
	}
	for af, entry := range niLocalAggrTableConnEntries(cfgDb, reqInfo.niName) {
		key := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key{
			SrcProtocol:   ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE,
			DstProtocol:   ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
			AddressFamily: af,
		}
		tcObj := getNiTableConnInstObjByKey(wrappedRoot, reqInfo.niName, key, true)
		if tcObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate network-instance table-connection object in YANG tree"}
		}
		niTableConnSetInstanceKeys(tcObj, key)
		niTableConnSetImportPolicy(tcObj.Config, entry)
		niTableConnSetStateImportPolicy(tcObj.State, entry)
	}
	return nil
}

// niTableConnEntryFor reads the row behind one table-connection instance. The
// LOCAL_AGGREGATE one has none and is derived from the aggregates instead.
func niTableConnEntryFor(cfgDb *db.DB, reqInfo *niTableConnReqInfo) (db.Value, error) {
	if niTableConnIsLocalAggr(reqInfo.srcProtocol) {
		afiSafi, err := niLocalAggrTableConnFamily(reqInfo.dstProtocol, reqInfo.addressFamily)
		if err != nil {
			return db.Value{}, err
		}
		entry, ok := niLocalAggrTableConnEntry(cfgDb, reqInfo.niName, afiSafi)
		if !ok {
			return db.Value{}, tlerr.NotFoundError{Format: "table-connection not found"}
		}
		return entry, nil
	}
	srcDb, err := niTableConnProtocolToDb(reqInfo.srcProtocol)
	if err != nil {
		return db.Value{}, err
	}
	dstDb, err := niTableConnProtocolToDb(reqInfo.dstProtocol)
	if err != nil {
		return db.Value{}, err
	}
	afDb, err := niTableConnAddressFamilyToDb(reqInfo.addressFamily)
	if err != nil {
		return db.Value{}, err
	}
	return cfgDb.GetEntry(&db.TableSpec{Name: niTableConnTbl}, db.Key{Comp: []string{reqInfo.niName, srcDb, dstDb, afDb}})
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

	if reqInfo.targetPath == niTableConnTargetRoot {
		return readNiTableConnAll(inParams, reqInfo, cfgDb)
	}

	entry, err := niTableConnEntryFor(cfgDb, reqInfo)
	if err != nil {
		return err
	}

	key, err := niTableConnYgotKey(reqInfo)
	if err != nil {
		return err
	}

	if reqInfo.targetPath == niTableConnTargetInst {
		tcObj := getNiTableConnInstObj(inParams.ygRoot, reqInfo, true)
		if tcObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate network-instance table-connection object in YANG tree"}
		}
		switch reqInfo.targetLeaf {
		case niTableConnSrcProtoLeaf:
			tcObj.SrcProtocol = key.SrcProtocol
		case niTableConnDstProtoLeaf:
			tcObj.DstProtocol = key.DstProtocol
		case niTableConnAddrFamilyLeaf:
			tcObj.AddressFamily = key.AddressFamily
		default:
			niTableConnSetInstanceKeys(tcObj, key)
			niTableConnSetImportPolicy(tcObj.Config, entry)
			niTableConnSetStateImportPolicy(tcObj.State, entry)
		}
		return nil
	}

	if reqInfo.targetPath == niTableConnTargetState {
		stObj := getNiTableConnStateObj(inParams.ygRoot, reqInfo, true)
		if stObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate network-instance table-connection state object in YANG tree"}
		}
		switch reqInfo.targetLeaf {
		case niTableConnSrcProtoLeaf:
			stObj.SrcProtocol = key.SrcProtocol
		case niTableConnDstProtoLeaf:
			stObj.DstProtocol = key.DstProtocol
		case niTableConnAddrFamilyLeaf:
			stObj.AddressFamily = key.AddressFamily
		case niTableConnImportPolicyLeaf:
			niTableConnSetStateImportPolicy(stObj, entry)
		default:
			stObj.SrcProtocol = key.SrcProtocol
			stObj.DstProtocol = key.DstProtocol
			stObj.AddressFamily = key.AddressFamily
			niTableConnSetStateImportPolicy(stObj, entry)
		}
		return nil
	}

	cfgObj := getNiTableConnCfgObj(inParams.ygRoot, reqInfo, true)
	if cfgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate network-instance table-connection config object in YANG tree"}
	}

	switch reqInfo.targetLeaf {
	case niTableConnSrcProtoLeaf:
		cfgObj.SrcProtocol = key.SrcProtocol
	case niTableConnDstProtoLeaf:
		cfgObj.DstProtocol = key.DstProtocol
	case niTableConnAddrFamilyLeaf:
		cfgObj.AddressFamily = key.AddressFamily
	case niTableConnImportPolicyLeaf:
		niTableConnSetImportPolicy(cfgObj, entry)
	default:
		cfgObj.SrcProtocol = key.SrcProtocol
		cfgObj.DstProtocol = key.DstProtocol
		cfgObj.AddressFamily = key.AddressFamily
		niTableConnSetImportPolicy(cfgObj, entry)
	}
	return nil
}
