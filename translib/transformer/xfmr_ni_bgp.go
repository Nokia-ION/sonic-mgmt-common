////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                 //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
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
	"encoding/json"
	"fmt"
	"math"
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
	niBgpGlobalsTbl              = "BGP_GLOBALS"
	niBgpGlobalsAfTbl            = "BGP_GLOBALS_AF"
	niBgpAfiSafiIPv4             = "ipv4_unicast"
	niBgpAfiSafiIPv6             = "ipv6_unicast"
	niBgpAsFld                   = "local_asn"
	niBgpRouterIdFld             = "router_id"
	niBgpAlwaysCompareMedFld     = "always_compare_med"
	niBgpMaxEbgpPathsFld         = "max_ebgp_paths"
	niBgpMaxIbgpPathsFld         = "max_ibgp_paths"
	niBgpTargetGlobalCfg         = "global-config"
	niBgpTargetRouteSelectConfig = "route-selection-options-config"
	niBgpTargetUseMpEbgpConfig   = "use-multiple-paths-ebgp-config"
	niBgpTargetUseMpIbgpConfig   = "use-multiple-paths-ibgp-config"
)

var niBgpGlobalCfgReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/global/config(?:/(as|router-id))?$`)
var niBgpGlobalRouteSelectReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/global/route-selection-options/config(?:/(always-compare-med))?$`)
var niBgpGlobalUseMpEbgpReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/global/use-multiple-paths/ebgp/config(?:/(maximum-paths))?$`)
var niBgpGlobalUseMpIbgpReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/global/use-multiple-paths/ibgp/config(?:/(maximum-paths))?$`)

type niBgpReqInfo struct {
	niName         string
	targetLeaf     string // "", "as", "router-id", "always-compare-med", "maximum-paths"
	targetPathKind string
}

func init() {
	XlateFuncBind("YangToDb_ni_bgp_global_cfg_xfmr", YangToDb_ni_bgp_global_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_bgp_global_cfg_xfmr", DbToYang_ni_bgp_global_cfg_xfmr)
}

func parseNiBgpRequestUri(uri string) (*niBgpReqInfo, error) {
	m := niBgpGlobalCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &niBgpReqInfo{
			niName:         m[1],
			targetLeaf:     leaf,
			targetPathKind: niBgpTargetGlobalCfg,
		}, nil
	}

	m = niBgpGlobalRouteSelectReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &niBgpReqInfo{
			niName:         m[1],
			targetLeaf:     leaf,
			targetPathKind: niBgpTargetRouteSelectConfig,
		}, nil
	}
	m = niBgpGlobalUseMpEbgpReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &niBgpReqInfo{
			niName:         m[1],
			targetLeaf:     leaf,
			targetPathKind: niBgpTargetUseMpEbgpConfig,
		}, nil
	}
	m = niBgpGlobalUseMpIbgpReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &niBgpReqInfo{
			niName:         m[1],
			targetLeaf:     leaf,
			targetPathKind: niBgpTargetUseMpIbgpConfig,
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP global config mapping: " + uri,
	}
}

func getNiBgpGlobalCfgObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config {
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
		return pObj.Bgp.Global.Config
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Global == nil {
			continue
		}
		if !strings.Contains(fmt.Sprint(pKey.Identifier), "BGP") {
			continue
		}
		// Name might be nil in some request bind flows; accept key match too.
		if pKey.Name != "bgp" {
			if pObj.Name == nil || *pObj.Name != "bgp" {
				continue
			}
		}
		ygot.BuildEmptyTree(pObj)
		return pObj.Bgp.Global.Config
	}

	if !createIfMissing {
		return nil
	}

	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	return pObj.Bgp.Global.Config
}

func getNiBgpGlobalRouteSelectCfgObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_RouteSelectionOptions_Config {
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
		return pObj.Bgp.Global.RouteSelectionOptions.Config
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Global == nil {
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
		return pObj.Bgp.Global.RouteSelectionOptions.Config
	}

	if !createIfMissing {
		return nil
	}

	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	return pObj.Bgp.Global.RouteSelectionOptions.Config
}

func getNiBgpGlobalUseMpEbgpCfgObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_UseMultiplePaths_Ebgp_Config {
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
		return pObj.Bgp.Global.UseMultiplePaths.Ebgp.Config
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Global == nil {
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
		return pObj.Bgp.Global.UseMultiplePaths.Ebgp.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	return pObj.Bgp.Global.UseMultiplePaths.Ebgp.Config
}

func getNiBgpGlobalUseMpIbgpCfgObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_UseMultiplePaths_Ibgp_Config {
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
		return pObj.Bgp.Global.UseMultiplePaths.Ibgp.Config
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Global == nil {
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
		return pObj.Bgp.Global.UseMultiplePaths.Ibgp.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	return pObj.Bgp.Global.UseMultiplePaths.Ibgp.Config
}

func parseNiBgpAsValue(v interface{}) (*uint32, bool) {
	switch val := v.(type) {
	case uint32:
		as := val
		return &as, true
	case *uint32:
		if val != nil {
			return val, true
		}
	case uint64:
		if val <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case uint:
		if uint64(val) <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case int:
		if val >= 0 && uint64(val) <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case int32:
		if val >= 0 {
			as := uint32(val)
			return &as, true
		}
	case int64:
		if val >= 0 && uint64(val) <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case float64:
		if val >= 0 && val <= float64(math.MaxUint32) && math.Trunc(val) == val {
			as := uint32(val)
			return &as, true
		}
	case float32:
		f := float64(val)
		if f >= 0 && f <= float64(math.MaxUint32) && math.Trunc(f) == f {
			as := uint32(f)
			return &as, true
		}
	case json.Number:
		if i64, err := val.Int64(); err == nil && i64 >= 0 && uint64(i64) <= math.MaxUint32 {
			as := uint32(i64)
			return &as, true
		}
	case string:
		if u64, err := strconv.ParseUint(val, 10, 32); err == nil {
			as := uint32(u64)
			return &as, true
		}
	case *string:
		if val != nil {
			if u64, err := strconv.ParseUint(*val, 10, 32); err == nil {
				as := uint32(u64)
				return &as, true
			}
		}
	}
	return nil, false
}

func parseNiBgpAsFromParam(param interface{}) (*uint32, bool) {
	if param == nil {
		return nil, false
	}
	if as, ok := parseNiBgpAsValue(param); ok {
		return as, true
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
		fv := rv.FieldByName("As")
		if fv.IsValid() && fv.CanInterface() {
			if as, ok := parseNiBgpAsValue(fv.Interface()); ok {
				return as, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "as" || strings.HasSuffix(k, ":as") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if as, ok := parseNiBgpAsValue(mv.Interface()); ok {
						return as, true
					}
				}
			}
		}
	}

	return nil, false
}

func parseNiBgpAs(inParams XfmrParams) (*uint32, error) {
	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if asPtr, ok := parseNiBgpAsFromParam(inParams.param); ok {
		return asPtr, nil
	}

	cfgObj := getNiBgpGlobalCfgObj(inParams.ygRoot, reqInfo.niName, false)
	if cfgObj == nil || cfgObj.As == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP global AS is missing in request payload"}
	}

	return cfgObj.As, nil
}

func parseNiBgpRouterIdValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val), true
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

func parseNiBgpRouterIdFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if rid, ok := parseNiBgpRouterIdValue(param); ok {
		return rid, true
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
		fv := rv.FieldByName("RouterId")
		if fv.IsValid() && fv.CanInterface() {
			if rid, ok := parseNiBgpRouterIdValue(fv.Interface()); ok {
				return rid, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "router-id" || strings.HasSuffix(k, ":router-id") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if rid, ok := parseNiBgpRouterIdValue(mv.Interface()); ok {
						return rid, true
					}
				}
			}
		}
	}

	return "", false
}

func parseNiBgpRouterId(inParams XfmrParams) (string, error) {
	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return "", err
	}
	if rid, ok := parseNiBgpRouterIdFromParam(inParams.param); ok {
		return rid, nil
	}

	cfgObj := getNiBgpGlobalCfgObj(inParams.ygRoot, reqInfo.niName, false)
	if cfgObj == nil || cfgObj.RouterId == nil || strings.TrimSpace(*cfgObj.RouterId) == "" {
		return "", tlerr.InvalidArgsError{Format: "BGP global router-id is missing in request payload"}
	}

	return strings.TrimSpace(*cfgObj.RouterId), nil
}

func parseNiBgpBoolValue(v interface{}) (*bool, bool) {
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

func parseNiBgpAlwaysCompareMedFromParam(param interface{}) (*bool, bool) {
	if param == nil {
		return nil, false
	}
	if med, ok := parseNiBgpBoolValue(param); ok {
		return med, true
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
		fv := rv.FieldByName("AlwaysCompareMed")
		if fv.IsValid() && fv.CanInterface() {
			if med, ok := parseNiBgpBoolValue(fv.Interface()); ok {
				return med, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "always-compare-med" || strings.HasSuffix(k, ":always-compare-med") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if med, ok := parseNiBgpBoolValue(mv.Interface()); ok {
						return med, true
					}
				}
			}
		}
	}

	return nil, false
}

func parseNiBgpAlwaysCompareMed(inParams XfmrParams) (*bool, error) {
	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if med, ok := parseNiBgpAlwaysCompareMedFromParam(inParams.param); ok {
		return med, nil
	}

	cfgObj := getNiBgpGlobalRouteSelectCfgObj(inParams.ygRoot, reqInfo.niName, false)
	if cfgObj == nil || cfgObj.AlwaysCompareMed == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP global always-compare-med is missing in request payload"}
	}

	return cfgObj.AlwaysCompareMed, nil
}

func parseNiBgpMaxPathsValue(v interface{}) (*uint16, bool) {
	switch val := v.(type) {
	case uint16:
		u := val
		return &u, true
	case *uint16:
		if val != nil {
			return val, true
		}
	case uint32:
		if val <= math.MaxUint16 {
			u := uint16(val)
			return &u, true
		}
	case int:
		if val >= 0 && val <= math.MaxUint16 {
			u := uint16(val)
			return &u, true
		}
	case float64:
		if val >= 0 && val <= float64(math.MaxUint16) && math.Trunc(val) == val {
			u := uint16(val)
			return &u, true
		}
	case string:
		if u64, err := strconv.ParseUint(strings.TrimSpace(val), 10, 16); err == nil {
			u := uint16(u64)
			return &u, true
		}
	}
	return nil, false
}

func parseNiBgpUseMpEbgpMaxPathsFromParam(param interface{}) (*uint16, bool) {
	if param == nil {
		return nil, false
	}
	if mp, ok := parseNiBgpMaxPathsValue(param); ok {
		return mp, true
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
		fv := rv.FieldByName("MaximumPaths")
		if fv.IsValid() && fv.CanInterface() {
			if mp, ok := parseNiBgpMaxPathsValue(fv.Interface()); ok {
				return mp, true
			}
		}
	}
	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "maximum-paths" || strings.HasSuffix(k, ":maximum-paths") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if mp, ok := parseNiBgpMaxPathsValue(mv.Interface()); ok {
						return mp, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseNiBgpUseMpEbgpMaxPaths(inParams XfmrParams) (*uint16, error) {
	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if mp, ok := parseNiBgpUseMpEbgpMaxPathsFromParam(inParams.param); ok {
		return mp, nil
	}
	cfgObj := getNiBgpGlobalUseMpEbgpCfgObj(inParams.ygRoot, reqInfo.niName, false)
	if cfgObj == nil || cfgObj.MaximumPaths == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP global use-multiple-paths eBGP maximum-paths is missing in request payload"}
	}
	u := uint16(*cfgObj.MaximumPaths)
	return &u, nil
}

func parseNiBgpUseMpIbgpMaxPaths(inParams XfmrParams) (*uint16, error) {
	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if mp, ok := parseNiBgpUseMpEbgpMaxPathsFromParam(inParams.param); ok {
		return mp, nil
	}
	cfgObj := getNiBgpGlobalUseMpIbgpCfgObj(inParams.ygRoot, reqInfo.niName, false)
	if cfgObj == nil || cfgObj.MaximumPaths == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP global use-multiple-paths iBGP maximum-paths is missing in request payload"}
	}
	u := uint16(*cfgObj.MaximumPaths)
	return &u, nil
}

func makeNiBgpGlobalsResMap(niName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpGlobalsTbl: {
			niName: db.Value{
				Field: fields,
			},
		},
	}
}

func makeNiBgpGlobalsAfResMap(niName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpGlobalsAfTbl: {
			niName + "|" + niBgpAfiSafiIPv4: db.Value{Field: fields},
			niName + "|" + niBgpAfiSafiIPv6: db.Value{Field: fields},
		},
	}
}

var YangToDb_ni_bgp_global_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	// DELETE: clear only the targeted field in BGP_GLOBALS|<ni-name>.
	if inParams.oper == DELETE {
		switch reqInfo.targetLeaf {
		case "as":
			return makeNiBgpGlobalsResMap(reqInfo.niName, map[string]string{niBgpAsFld: ""}), nil
		case "router-id":
			return makeNiBgpGlobalsResMap(reqInfo.niName, map[string]string{niBgpRouterIdFld: ""}), nil
		case "always-compare-med":
			return makeNiBgpGlobalsResMap(reqInfo.niName, map[string]string{niBgpAlwaysCompareMedFld: ""}), nil
		case "maximum-paths":
			if reqInfo.targetPathKind == niBgpTargetUseMpEbgpConfig {
				return makeNiBgpGlobalsAfResMap(reqInfo.niName, map[string]string{niBgpMaxEbgpPathsFld: ""}), nil
			}
			if reqInfo.targetPathKind == niBgpTargetUseMpIbgpConfig {
				return makeNiBgpGlobalsAfResMap(reqInfo.niName, map[string]string{niBgpMaxIbgpPathsFld: ""}), nil
			}
			return res, tlerr.NotSupportedError{Format: "DELETE not supported for this maximum-paths path", Path: inParams.requestUri}
		default:
			return res, tlerr.NotSupportedError{Format: "DELETE supported only for /config/as, /config/router-id, /route-selection-options/config/always-compare-med, /use-multiple-paths/ebgp/config/maximum-paths, or /use-multiple-paths/ibgp/config/maximum-paths", Path: inParams.requestUri}
		}
	}

	fields := make(map[string]string)
	switch reqInfo.targetLeaf {
	case "as":
		asPtr, err := parseNiBgpAs(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpAsFld] = strconv.FormatUint(uint64(*asPtr), 10)
	case "router-id":
		rid, err := parseNiBgpRouterId(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpRouterIdFld] = rid
	case "always-compare-med":
		med, err := parseNiBgpAlwaysCompareMed(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpAlwaysCompareMedFld] = strconv.FormatBool(*med)
	case "maximum-paths":
		if reqInfo.targetPathKind != niBgpTargetUseMpEbgpConfig && reqInfo.targetPathKind != niBgpTargetUseMpIbgpConfig {
			return res, tlerr.NotSupportedError{Format: "unsupported maximum-paths target path", Path: inParams.requestUri}
		}
		if reqInfo.targetPathKind == niBgpTargetUseMpEbgpConfig {
			mp, err := parseNiBgpUseMpEbgpMaxPaths(inParams)
			if err != nil {
				return res, err
			}
			fields[niBgpMaxEbgpPathsFld] = strconv.FormatUint(uint64(*mp), 10)
			return makeNiBgpGlobalsAfResMap(reqInfo.niName, fields), nil
		}
		mp, err := parseNiBgpUseMpIbgpMaxPaths(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpMaxIbgpPathsFld] = strconv.FormatUint(uint64(*mp), 10)
		return makeNiBgpGlobalsAfResMap(reqInfo.niName, fields), nil
	default:
		switch reqInfo.targetPathKind {
		case niBgpTargetGlobalCfg:
			// For global config container updates, accept any subset of supported leaves.
			if asPtr, ok := parseNiBgpAsFromParam(inParams.param); ok {
				fields[niBgpAsFld] = strconv.FormatUint(uint64(*asPtr), 10)
			}
			if rid, ok := parseNiBgpRouterIdFromParam(inParams.param); ok {
				fields[niBgpRouterIdFld] = rid
			}
			if len(fields) == 0 {
				cfgObj := getNiBgpGlobalCfgObj(inParams.ygRoot, reqInfo.niName, false)
				if cfgObj != nil {
					if cfgObj.As != nil {
						fields[niBgpAsFld] = strconv.FormatUint(uint64(*cfgObj.As), 10)
					}
					if cfgObj.RouterId != nil && strings.TrimSpace(*cfgObj.RouterId) != "" {
						fields[niBgpRouterIdFld] = strings.TrimSpace(*cfgObj.RouterId)
					}
				}
			}
			if len(fields) == 0 {
				return res, tlerr.InvalidArgsError{Format: "BGP global config payload missing supported fields (as/router-id)"}
			}
		case niBgpTargetRouteSelectConfig:
			// For route-selection-options config container updates, support always-compare-med.
			if med, ok := parseNiBgpAlwaysCompareMedFromParam(inParams.param); ok {
				fields[niBgpAlwaysCompareMedFld] = strconv.FormatBool(*med)
			}
			if len(fields) == 0 {
				cfgObj := getNiBgpGlobalRouteSelectCfgObj(inParams.ygRoot, reqInfo.niName, false)
				if cfgObj != nil && cfgObj.AlwaysCompareMed != nil {
					fields[niBgpAlwaysCompareMedFld] = strconv.FormatBool(*cfgObj.AlwaysCompareMed)
				}
			}
			if len(fields) == 0 {
				return res, tlerr.InvalidArgsError{Format: "BGP route-selection-options config payload missing supported field (always-compare-med)"}
			}
		case niBgpTargetUseMpEbgpConfig:
			if mp, ok := parseNiBgpUseMpEbgpMaxPathsFromParam(inParams.param); ok {
				fields[niBgpMaxEbgpPathsFld] = strconv.FormatUint(uint64(*mp), 10)
			}
			if len(fields) == 0 {
				cfgObj := getNiBgpGlobalUseMpEbgpCfgObj(inParams.ygRoot, reqInfo.niName, false)
				if cfgObj != nil && cfgObj.MaximumPaths != nil {
					fields[niBgpMaxEbgpPathsFld] = strconv.FormatUint(uint64(*cfgObj.MaximumPaths), 10)
				}
			}
			if len(fields) == 0 {
				return res, tlerr.InvalidArgsError{Format: "BGP global use-multiple-paths eBGP config payload missing supported field (maximum-paths)"}
			}
			return makeNiBgpGlobalsAfResMap(reqInfo.niName, fields), nil
		case niBgpTargetUseMpIbgpConfig:
			if mp, ok := parseNiBgpUseMpEbgpMaxPathsFromParam(inParams.param); ok {
				fields[niBgpMaxIbgpPathsFld] = strconv.FormatUint(uint64(*mp), 10)
			}
			if len(fields) == 0 {
				cfgObj := getNiBgpGlobalUseMpIbgpCfgObj(inParams.ygRoot, reqInfo.niName, false)
				if cfgObj != nil && cfgObj.MaximumPaths != nil {
					fields[niBgpMaxIbgpPathsFld] = strconv.FormatUint(uint64(*cfgObj.MaximumPaths), 10)
				}
			}
			if len(fields) == 0 {
				return res, tlerr.InvalidArgsError{Format: "BGP global use-multiple-paths iBGP config payload missing supported field (maximum-paths)"}
			}
			return makeNiBgpGlobalsAfResMap(reqInfo.niName, fields), nil
		}
	}

	// Return the primary result map directly so translib commits the write for
	// both container and leaf targeted requests.
	return makeNiBgpGlobalsResMap(reqInfo.niName, fields), nil
}

var DbToYang_ni_bgp_global_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP global config"}
	}

	if reqInfo.targetPathKind == niBgpTargetGlobalCfg {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsTbl}, db.Key{Comp: []string{reqInfo.niName}})
		if err != nil {
			return err
		}
		cfgObj := getNiBgpGlobalCfgObj(inParams.ygRoot, reqInfo.niName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP global config object in YANG tree"}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "as" {
			asStr, ok := entry.Field[niBgpAsFld]
			if ok && asStr != "" {
				asVal, err := strconv.ParseUint(asStr, 10, 32)
				if err != nil {
					return err
				}
				asU32 := uint32(asVal)
				cfgObj.As = &asU32
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "router-id" {
			rid, ok := entry.Field[niBgpRouterIdFld]
			if ok && strings.TrimSpace(rid) != "" {
				rid = strings.TrimSpace(rid)
				cfgObj.RouterId = &rid
			}
		}
		return nil
	}
	if reqInfo.targetPathKind == niBgpTargetRouteSelectConfig {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsTbl}, db.Key{Comp: []string{reqInfo.niName}})
		if err != nil {
			return err
		}
		rsoCfgObj := getNiBgpGlobalRouteSelectCfgObj(inParams.ygRoot, reqInfo.niName, true)
		if rsoCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP global route-selection-options config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "always-compare-med" {
			medStr, ok := entry.Field[niBgpAlwaysCompareMedFld]
			if ok && strings.TrimSpace(medStr) != "" {
				med, err := strconv.ParseBool(strings.TrimSpace(medStr))
				if err != nil {
					return err
				}
				rsoCfgObj.AlwaysCompareMed = &med
			}
		}
		return nil
	}

	if reqInfo.targetPathKind == niBgpTargetUseMpEbgpConfig {
		ebgpCfgObj := getNiBgpGlobalUseMpEbgpCfgObj(inParams.ygRoot, reqInfo.niName, true)
		if ebgpCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP global use-multiple-paths eBGP config object in YANG tree"}
		}
		for _, afi := range []string{niBgpAfiSafiIPv4, niBgpAfiSafiIPv6} {
			entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsAfTbl}, db.Key{Comp: []string{reqInfo.niName, afi}})
			if err != nil {
				continue
			}
			mpStr, ok := entry.Field[niBgpMaxEbgpPathsFld]
			if !ok || strings.TrimSpace(mpStr) == "" {
				continue
			}
			mpVal, err := strconv.ParseUint(strings.TrimSpace(mpStr), 10, 32)
			if err != nil {
				return err
			}
			mp := uint32(mpVal)
			ebgpCfgObj.MaximumPaths = &mp
			break
		}
		return nil
	}
	if reqInfo.targetPathKind == niBgpTargetUseMpIbgpConfig {
		ibgpCfgObj := getNiBgpGlobalUseMpIbgpCfgObj(inParams.ygRoot, reqInfo.niName, true)
		if ibgpCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP global use-multiple-paths iBGP config object in YANG tree"}
		}
		for _, afi := range []string{niBgpAfiSafiIPv4, niBgpAfiSafiIPv6} {
			entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsAfTbl}, db.Key{Comp: []string{reqInfo.niName, afi}})
			if err != nil {
				continue
			}
			mpStr, ok := entry.Field[niBgpMaxIbgpPathsFld]
			if !ok || strings.TrimSpace(mpStr) == "" {
				continue
			}
			mpVal, err := strconv.ParseUint(strings.TrimSpace(mpStr), 10, 32)
			if err != nil {
				return err
			}
			mp := uint32(mpVal)
			ibgpCfgObj.MaximumPaths = &mp
			break
		}
		return nil
	}

	return nil
}
