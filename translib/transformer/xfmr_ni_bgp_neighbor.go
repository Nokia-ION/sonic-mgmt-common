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
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niBgpNbrTbl            = "BGP_NEIGHBOR"
	niBgpNbrAfTbl          = "BGP_NEIGHBOR_AF"
	niBgpNbrPeerGroupFld   = "peer_group_name"
	niBgpNbrAdminStatus    = "admin_status"
	niBgpNbrRouteMapInFld  = "route_map_in@"
	niBgpNbrAllowAsInFld   = "allow_as_in"
	niBgpNbrAllowAsCntFld  = "allow_as_count"
	niBgpNbrAllowAsOrigFld = "allow_as_origin"
	niBgpAfiSafiIPv4       = "ipv4_unicast"
	niBgpAfiSafiIPv6       = "ipv6_unicast"
)

var niBgpNbrReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]/config(?:/(peer-group|enabled))?$`)
var niBgpNbrAsPathOptReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]/as-path-options/config(?:/(allow-own-as))?$`)
var niBgpNbrAfApplyReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/apply-policy/config(?:/(import-policy))?$`)

type niBgpNbrReqInfo struct {
	niName       string
	neighborAddr string
	afiSafi      string
	targetLeaf   string // "", "peer-group", "enabled"
	targetPath   string
}

func init() {
	XlateFuncBind("YangToDb_ni_bgp_nbr_cfg_xfmr", YangToDb_ni_bgp_nbr_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_bgp_nbr_cfg_xfmr", DbToYang_ni_bgp_nbr_cfg_xfmr)
}

func parseNiBgpNbrRequestUri(uri string) (*niBgpNbrReqInfo, error) {
	m := niBgpNbrReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			neighborAddr: m[2],
			targetLeaf:   leaf,
			targetPath:   "neighbor-config",
		}, nil
	}
	m = niBgpNbrAsPathOptReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			neighborAddr: m[2],
			targetLeaf:   leaf,
			targetPath:   "neighbor-as-path-options-config",
		}, nil
	}
	m = niBgpNbrAfApplyReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[3])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP neighbor apply-policy mapping: " + m[3],
			}
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			neighborAddr: m[2],
			afiSafi:      afiSafi,
			targetLeaf:   leaf,
			targetPath:   "neighbor-af-apply-policy-config",
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP neighbor config mapping: " + uri,
	}
}

func normalizeNiBgpNbrAfiSafiName(v string) (string, bool) {
	s := strings.TrimSpace(v)
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.ToLower(strings.ReplaceAll(s, "-", "_"))
	switch s {
	case niBgpAfiSafiIPv4:
		return niBgpAfiSafiIPv4, true
	case niBgpAfiSafiIPv6:
		return niBgpAfiSafiIPv6, true
	default:
		return "", false
	}
}

func getNiBgpNbrAsPathOptionsCfgObj(s *ygot.GoStruct, niName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AsPathOptions_Config {
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

	if niObj.Protocols == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	if niObj.Protocols.Protocol == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols.Protocol = make(map[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol)
	}

	wantKey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	if pObj, ok := niObj.Protocols.Protocol[wantKey]; ok && pObj != nil {
		ygot.BuildEmptyTree(pObj)
		if nbrObj, ok := pObj.Bgp.Neighbors.Neighbor[neighborAddr]; ok && nbrObj != nil {
			ygot.BuildEmptyTree(nbrObj)
			return nbrObj.AsPathOptions.Config
		}
		if !createIfMissing {
			return nil
		}
		nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
		if err != nil || nbrObj == nil {
			return nil
		}
		ygot.BuildEmptyTree(nbrObj)
		return nbrObj.AsPathOptions.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
	if err != nil || nbrObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(nbrObj)
	return nbrObj.AsPathOptions.Config
}

func getNiBgpNbrAfApplyPolicyCfgObj(s *ygot.GoStruct, niName, neighborAddr, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy_Config {
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

	if niObj.Protocols == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	if niObj.Protocols.Protocol == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols.Protocol = make(map[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol)
	}

	wantKey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	pObj, ok := niObj.Protocols.Protocol[wantKey]
	if !ok || pObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pObj, err = niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
		if err != nil || pObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(pObj)

	var nbrObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor
	nbrObj, ok = pObj.Bgp.Neighbors.Neighbor[neighborAddr]
	if !ok || nbrObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		nbrObj, err = pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
		if err != nil || nbrObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(nbrObj)

	afiKey := ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE(0)
	switch afiSafi {
	case niBgpAfiSafiIPv4:
		afiKey = ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST
	case niBgpAfiSafiIPv6:
		afiKey = ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV6_UNICAST
	default:
		return nil
	}

	var afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi
	afObj, ok = nbrObj.AfiSafis.AfiSafi[afiKey]
	if !ok || afObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		afObj, err = nbrObj.AfiSafis.NewAfiSafi(afiKey)
		if err != nil || afObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(afObj)
	return afObj.ApplyPolicy.Config
}

func getNiBgpNbrCfgObj(s *ygot.GoStruct, niName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config {
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

	if niObj.Protocols == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	if niObj.Protocols.Protocol == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols.Protocol = make(map[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol)
	}

	wantKey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	if pObj, ok := niObj.Protocols.Protocol[wantKey]; ok && pObj != nil {
		ygot.BuildEmptyTree(pObj)
		if nbrObj, ok := pObj.Bgp.Neighbors.Neighbor[neighborAddr]; ok && nbrObj != nil {
			ygot.BuildEmptyTree(nbrObj)
			return nbrObj.Config
		}
		if !createIfMissing {
			return nil
		}
		nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
		if err != nil || nbrObj == nil {
			return nil
		}
		ygot.BuildEmptyTree(nbrObj)
		return nbrObj.Config
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Neighbors == nil {
			continue
		}
		if !strings.Contains(fmt.Sprint(pKey.Identifier), "BGP") {
			continue
		}
		if pKey.Name != "bgp" {
			if pObj.Name == nil || *pObj.Name != "bgp" {
				continue
			}
		}
		ygot.BuildEmptyTree(pObj)
		if nbrObj, ok := pObj.Bgp.Neighbors.Neighbor[neighborAddr]; ok && nbrObj != nil {
			ygot.BuildEmptyTree(nbrObj)
			return nbrObj.Config
		}
		if !createIfMissing {
			return nil
		}
		nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
		if err != nil || nbrObj == nil {
			return nil
		}
		ygot.BuildEmptyTree(nbrObj)
		return nbrObj.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
	if err != nil || nbrObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(nbrObj)
	return nbrObj.Config
}

func parseNiBgpNbrPeerGroupValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s != "" {
			return s, true
		}
	case *string:
		if val != nil {
			s := strings.TrimSpace(*val)
			if s != "" {
				return s, true
			}
		}
	}
	return "", false
}

func parseNiBgpNbrPeerGroupFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if pg, ok := parseNiBgpNbrPeerGroupValue(param); ok {
		return pg, true
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
		fv := rv.FieldByName("PeerGroup")
		if fv.IsValid() && fv.CanInterface() {
			if pg, ok := parseNiBgpNbrPeerGroupValue(fv.Interface()); ok {
				return pg, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "peer-group" || strings.HasSuffix(k, ":peer-group") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if pg, ok := parseNiBgpNbrPeerGroupValue(mv.Interface()); ok {
						return pg, true
					}
				}
			}
		}
	}
	return "", false
}

func parseNiBgpNbrPeerGroup(inParams XfmrParams) (string, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return "", err
	}
	if pg, ok := parseNiBgpNbrPeerGroupFromParam(inParams.param); ok {
		return pg, nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.PeerGroup == nil || strings.TrimSpace(*cfgObj.PeerGroup) == "" {
		return "", tlerr.InvalidArgsError{Format: "BGP neighbor peer-group is missing in request payload"}
	}
	return strings.TrimSpace(*cfgObj.PeerGroup), nil
}

func niBgpNbrAdminStatusToEnabled(status string) (*bool, bool) {
	s := strings.TrimSpace(strings.ToLower(status))
	switch s {
	case "up", "true":
		v := true
		return &v, true
	case "down", "false":
		v := false
		return &v, true
	default:
		return nil, false
	}
}

func niBgpNbrEnabledToAdminStatus(enabled bool) string {
	if enabled {
		return "up"
	}
	return "down"
}

func parseNiBgpNbrEnabledValue(v interface{}) (*bool, bool) {
	switch val := v.(type) {
	case bool:
		b := val
		return &b, true
	case *bool:
		if val != nil {
			return val, true
		}
	case string:
		s := strings.TrimSpace(val)
		if b, err := strconv.ParseBool(s); err == nil {
			return &b, true
		}
	case *string:
		if val != nil {
			s := strings.TrimSpace(*val)
			if b, err := strconv.ParseBool(s); err == nil {
				return &b, true
			}
		}
	}
	return nil, false
}

func parseNiBgpNbrEnabledFromParam(param interface{}) (*bool, bool) {
	if param == nil {
		return nil, false
	}
	if enabled, ok := parseNiBgpNbrEnabledValue(param); ok {
		return enabled, true
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
		fv := rv.FieldByName("Enabled")
		if fv.IsValid() && fv.CanInterface() {
			if enabled, ok := parseNiBgpNbrEnabledValue(fv.Interface()); ok {
				return enabled, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "enabled" || strings.HasSuffix(k, ":enabled") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if enabled, ok := parseNiBgpNbrEnabledValue(mv.Interface()); ok {
						return enabled, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseNiBgpNbrEnabled(inParams XfmrParams) (*bool, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if enabled, ok := parseNiBgpNbrEnabledFromParam(inParams.param); ok {
		return enabled, nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.Enabled == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor enabled is missing in request payload"}
	}
	return cfgObj.Enabled, nil
}

func makeNiBgpNbrResMap(niName, neighborAddr string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpNbrTbl: {
			niName + "|" + neighborAddr: db.Value{Field: fields},
		},
	}
}

func makeNiBgpNbrAfResMap(niName, neighborAddr string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpNbrAfTbl: {
			niName + "|" + neighborAddr + "|" + niBgpAfiSafiIPv4: db.Value{Field: fields},
			niName + "|" + neighborAddr + "|" + niBgpAfiSafiIPv6: db.Value{Field: fields},
		},
	}
}

func makeNiBgpNbrAfSingleResMap(niName, neighborAddr, afiSafi string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpNbrAfTbl: {
			niName + "|" + neighborAddr + "|" + afiSafi: db.Value{Field: fields},
		},
	}
}

func parseNiBgpNbrImportPolicyValue(v interface{}) (string, bool) {
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
		for _, item := range val {
			s := strings.TrimSpace(item)
			if s != "" {
				return s, true
			}
		}
	case []interface{}:
		for _, item := range val {
			if s, ok := item.(string); ok {
				v := strings.TrimSpace(s)
				if v != "" {
					return v, true
				}
			}
		}
	}
	return "", false
}

func parseNiBgpNbrImportPolicyFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if p, ok := parseNiBgpNbrImportPolicyValue(param); ok {
		return p, true
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
			if p, ok := parseNiBgpNbrImportPolicyValue(fv.Interface()); ok {
				return p, true
			}
		}
	}
	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "import-policy" || strings.HasSuffix(k, ":import-policy") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if p, ok := parseNiBgpNbrImportPolicyValue(mv.Interface()); ok {
						return p, true
					}
				}
			}
		}
	}
	return "", false
}

func parseNiBgpNbrImportPolicy(inParams XfmrParams) (string, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return "", err
	}
	if p, ok := parseNiBgpNbrImportPolicyFromParam(inParams.param); ok {
		return p, nil
	}
	cfgObj := getNiBgpNbrAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, false)
	if cfgObj == nil || len(cfgObj.ImportPolicy) == 0 {
		return "", tlerr.InvalidArgsError{Format: "BGP neighbor AFI-SAFI apply-policy import-policy is missing in request payload"}
	}
	p := strings.TrimSpace(cfgObj.ImportPolicy[0])
	if p == "" {
		return "", tlerr.InvalidArgsError{Format: "BGP neighbor AFI-SAFI apply-policy import-policy is missing in request payload"}
	}
	return p, nil
}

func parseNiBgpNbrAllowOwnAsValue(v interface{}) (*uint8, bool) {
	switch val := v.(type) {
	case uint8:
		x := val
		return &x, true
	case uint16:
		if val <= 255 {
			x := uint8(val)
			return &x, true
		}
	case uint32:
		if val <= 255 {
			x := uint8(val)
			return &x, true
		}
	case int:
		if val >= 0 && val <= 255 {
			x := uint8(val)
			return &x, true
		}
	case float64:
		if val >= 0 && val <= 255 && val == float64(uint8(val)) {
			x := uint8(val)
			return &x, true
		}
	case string:
		if u, err := strconv.ParseUint(strings.TrimSpace(val), 10, 8); err == nil {
			x := uint8(u)
			return &x, true
		}
	case *uint8:
		if val != nil {
			return val, true
		}
	}
	return nil, false
}

func parseNiBgpNbrAllowOwnAsFromParam(param interface{}) (*uint8, bool) {
	if param == nil {
		return nil, false
	}
	if allow, ok := parseNiBgpNbrAllowOwnAsValue(param); ok {
		return allow, true
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
		fv := rv.FieldByName("AllowOwnAs")
		if fv.IsValid() && fv.CanInterface() {
			if allow, ok := parseNiBgpNbrAllowOwnAsValue(fv.Interface()); ok {
				return allow, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "allow-own-as" || strings.HasSuffix(k, ":allow-own-as") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if allow, ok := parseNiBgpNbrAllowOwnAsValue(mv.Interface()); ok {
						return allow, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseNiBgpNbrAllowOwnAs(inParams XfmrParams) (*uint8, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if allow, ok := parseNiBgpNbrAllowOwnAsFromParam(inParams.param); ok {
		return allow, nil
	}

	asPathCfgObj := getNiBgpNbrAsPathOptionsCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, false)
	if asPathCfgObj == nil || asPathCfgObj.AllowOwnAs == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor as-path-options allow-own-as is missing in request payload"}
	}
	return asPathCfgObj.AllowOwnAs, nil
}

var YangToDb_ni_bgp_nbr_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		if reqInfo.targetPath == "neighbor-af-apply-policy-config" {
			switch reqInfo.targetLeaf {
			case "", "import-policy":
				return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{niBgpNbrRouteMapInFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/afi-safis/afi-safi/apply-policy/config or /neighbors/neighbor/afi-safis/afi-safi/apply-policy/config/import-policy", Path: inParams.requestUri}
			}
		}
		if reqInfo.targetPath == "neighbor-as-path-options-config" {
			switch reqInfo.targetLeaf {
			case "", "allow-own-as":
				return makeNiBgpNbrAfResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
					niBgpNbrAllowAsInFld:   "",
					niBgpNbrAllowAsCntFld:  "",
					niBgpNbrAllowAsOrigFld: "",
				}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/as-path-options/config or /neighbors/neighbor/as-path-options/config/allow-own-as", Path: inParams.requestUri}
			}
		}
		switch reqInfo.targetLeaf {
		case "peer-group":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrPeerGroupFld: ""}), nil
		case "enabled":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrAdminStatus: ""}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/config/peer-group or /neighbors/neighbor/config/enabled", Path: inParams.requestUri}
		}
	}

	if reqInfo.targetPath == "neighbor-af-apply-policy-config" {
		switch reqInfo.targetLeaf {
		case "", "import-policy":
			importPolicy, err := parseNiBgpNbrImportPolicy(inParams)
			if err != nil {
				return res, err
			}
			return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{niBgpNbrRouteMapInFld: importPolicy}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "unsupported leaf under neighbor AFI-SAFI apply-policy config", Path: inParams.requestUri}
		}
	}

	if reqInfo.targetPath == "neighbor-as-path-options-config" {
		switch reqInfo.targetLeaf {
		case "", "allow-own-as":
			allowOwnAs, err := parseNiBgpNbrAllowOwnAs(inParams)
			if err != nil {
				return res, err
			}
			if *allowOwnAs == 0 {
				return makeNiBgpNbrAfResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
					niBgpNbrAllowAsInFld:   "",
					niBgpNbrAllowAsCntFld:  "",
					niBgpNbrAllowAsOrigFld: "",
				}), nil
			}
			return makeNiBgpNbrAfResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
				niBgpNbrAllowAsInFld:   "true",
				niBgpNbrAllowAsCntFld:  strconv.FormatUint(uint64(*allowOwnAs), 10),
				niBgpNbrAllowAsOrigFld: "",
			}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "unsupported leaf under neighbor as-path-options config", Path: inParams.requestUri}
		}
	}

	fields := make(map[string]string)
	switch reqInfo.targetLeaf {
	case "peer-group":
		pg, err := parseNiBgpNbrPeerGroup(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpNbrPeerGroupFld] = pg
	case "enabled":
		enabled, err := parseNiBgpNbrEnabled(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
	default:
		if pg, ok := parseNiBgpNbrPeerGroupFromParam(inParams.param); ok {
			fields[niBgpNbrPeerGroupFld] = pg
		}
		if enabled, ok := parseNiBgpNbrEnabledFromParam(inParams.param); ok {
			fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
		}
		if len(fields) == 0 {
			cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, false)
			if cfgObj != nil {
				if cfgObj.PeerGroup != nil && strings.TrimSpace(*cfgObj.PeerGroup) != "" {
					fields[niBgpNbrPeerGroupFld] = strings.TrimSpace(*cfgObj.PeerGroup)
				}
				if cfgObj.Enabled != nil {
					fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*cfgObj.Enabled)
				}
			}
		}
		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "BGP neighbor config payload missing supported fields (peer-group/enabled)"}
		}
	}
	return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, fields), nil
}

var DbToYang_ni_bgp_nbr_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP neighbor config"}
	}

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr}})
	if err != nil {
		return err
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, true)
	if cfgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor config object in YANG tree"}
	}

	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "peer-group" {
		pg, ok := entry.Field[niBgpNbrPeerGroupFld]
		if ok && strings.TrimSpace(pg) != "" {
			pg = strings.TrimSpace(pg)
			cfgObj.PeerGroup = &pg
		}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "enabled" {
		status, ok := entry.Field[niBgpNbrAdminStatus]
		if ok && strings.TrimSpace(status) != "" {
			if enabled, ok := niBgpNbrAdminStatusToEnabled(status); ok {
				cfgObj.Enabled = enabled
			}
		}
	}
	if reqInfo.targetPath == "neighbor-af-apply-policy-config" {
		afEntry, afErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi}})
		if afErr != nil {
			return afErr
		}
		applyCfgObj := getNiBgpNbrAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, true)
		if applyCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor AFI-SAFI apply-policy config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "import-policy" {
			importPolicy, ok := afEntry.Field[niBgpNbrRouteMapInFld]
			if ok && strings.TrimSpace(importPolicy) != "" {
				applyCfgObj.ImportPolicy = []string{strings.TrimSpace(importPolicy)}
			}
		}
		return nil
	}
	if reqInfo.targetPath == "neighbor-as-path-options-config" {
		v4Entry, errV4 := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, niBgpAfiSafiIPv4}})
		v6Entry, errV6 := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, niBgpAfiSafiIPv6}})
		if errV4 != nil && errV6 != nil {
			if errV4 != nil {
				return errV4
			}
			return errV6
		}
		getAllowOwnAs := func(entry db.Value) *uint8 {
			allowIn := strings.ToLower(strings.TrimSpace(entry.Field[niBgpNbrAllowAsInFld])) == "true"
			if !allowIn {
				return nil
			}
			cntStr := strings.TrimSpace(entry.Field[niBgpNbrAllowAsCntFld])
			if cntStr == "" {
				def := uint8(1)
				return &def
			}
			cnt, err := strconv.ParseUint(cntStr, 10, 8)
			if err != nil {
				return nil
			}
			val := uint8(cnt)
			return &val
		}
		var v4Allow, v6Allow *uint8
		if errV4 == nil {
			v4Allow = getAllowOwnAs(v4Entry)
		}
		if errV6 == nil {
			v6Allow = getAllowOwnAs(v6Entry)
		}
		if v4Allow != nil && v6Allow != nil && *v4Allow != *v6Allow {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("inconsistent allow-own-as across AFI-SAFIs for neighbor %s in %s (ipv4=%d, ipv6=%d)", reqInfo.neighborAddr, reqInfo.niName, *v4Allow, *v6Allow)}
		}

		asPathCfgObj := getNiBgpNbrAsPathOptionsCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, true)
		if asPathCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor as-path-options config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "allow-own-as" {
			if v4Allow != nil {
				asPathCfgObj.AllowOwnAs = v4Allow
			} else if v6Allow != nil {
				asPathCfgObj.AllowOwnAs = v6Allow
			}
		}
		return nil
	}
	return nil
}
