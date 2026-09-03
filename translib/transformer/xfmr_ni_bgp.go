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
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	log "github.com/golang/glog"

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
	niBgpTargetProtocolRoot      = "protocol-root"
	niBgpTargetBgpRoot           = "bgp-root"
	// niBgpMaxTreeDepth bounds the container recursion done while initializing a
	// BGP YANG subtree.
	niBgpMaxTreeDepth = 32
)

var niBgpProtocolRootReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]$`)
var niBgpRootReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp$`)
var niBgpProtoInstanceReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\](?:/.*)?$`)
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
	m := niBgpProtocolRootReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		return &niBgpReqInfo{
			niName:         m[1],
			targetLeaf:     "",
			targetPathKind: niBgpTargetProtocolRoot,
		}, nil
	}
	m = niBgpRootReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		return &niBgpReqInfo{
			niName:         m[1],
			targetLeaf:     "",
			targetPathKind: niBgpTargetBgpRoot,
		}, nil
	}

	m = niBgpGlobalCfgReqUriRe.FindStringSubmatch(uri)
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
	// Any other node of the BGP protocol instance is served by the container
	// dispatch, which maps or populates the whole subtree and lets translib
	// return just the requested node. Without this, intermediate containers such
	// as /bgp/global or /bgp/neighbors would be rejected, since the annotation
	// routes every unannotated descendant of the protocol instance here.
	m = niBgpProtoInstanceReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		return &niBgpReqInfo{
			niName:         m[1],
			targetLeaf:     "",
			targetPathKind: niBgpTargetBgpRoot,
		}, nil
	}

	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP global config mapping: " + uri,
	}
}

// buildNiBgpEmptySubtree initializes the nil container pointers of obj and of
// its already-allocated descendants. ygot.BuildEmptyTree skips any container
// that is non-nil without descending into it, so a request anchored at an inner
// container (for example .../protocol[identifier=BGP][name=bgp]/bgp) arrives
// with that container allocated but all of its children still nil.
func buildNiBgpEmptySubtree(obj ygot.GoStruct) {
	v := reflect.ValueOf(obj)
	if !v.IsValid() || v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	buildNiBgpEmptyContainers(v.Elem(), 0)
}

func buildNiBgpEmptyContainers(v reflect.Value, depth int) {
	if depth >= niBgpMaxTreeDepth || v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Ptr || f.Type().Elem().Kind() != reflect.Struct || !f.CanSet() {
			continue
		}
		if f.IsNil() {
			f.Set(reflect.New(f.Type().Elem()))
		}
		buildNiBgpEmptyContainers(f.Elem(), depth+1)
	}
}

// isNiBgpContainerLevelReqUri reports whether uri targets a node of the BGP
// protocol instance. Such a request is mapped or populated in a single pass by
// the container dispatch in YangToDb_ni_bgp_global_cfg_xfmr, but translib still
// invokes the transformers annotated on child nodes and passes them this same
// requestUri. A sibling transformer that does not recognise the request as one
// of its own leaf paths must therefore stay silent rather than fail it as an
// unsupported path.
func isNiBgpContainerLevelReqUri(uri string) bool {
	return niBgpProtoInstanceReqUriRe.MatchString(uri)
}

// niBgpAfiSafiNameFromEnum maps an OC AFI-SAFI identity to its CONFIG_DB
// spelling. The generated enums carry no String method, so they must be switched
// on rather than formatted.
func niBgpAfiSafiNameFromEnum(v ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE) (string, bool) {
	switch v {
	case ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST:
		return niBgpAfiSafiIPv4, true
	case ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV6_UNICAST:
		return niBgpAfiSafiIPv6, true
	}
	return "", false
}

// niBgpMappedPayloadLeaves lists every leaf, relative to the bgp container, that
// a container-level request can carry. List keys are included because a payload
// repeats them alongside the data they identify.
var niBgpMappedPayloadLeaves = map[string]bool{
	"global/config/as":        true,
	"global/config/router-id": true,
	"global/route-selection-options/config/always-compare-med":                                true,
	"global/use-multiple-paths/ebgp/config/maximum-paths":                                     true,
	"global/use-multiple-paths/ibgp/config/maximum-paths":                                     true,
	"global/afi-safis/afi-safi/afi-safi-name":                                                 true,
	"global/afi-safis/afi-safi/config/afi-safi-name":                                          true,
	"neighbors/neighbor/neighbor-address":                                                     true,
	"neighbors/neighbor/config/neighbor-address":                                              true,
	"neighbors/neighbor/config/enabled":                                                       true,
	"neighbors/neighbor/config/peer-as":                                                       true,
	"neighbors/neighbor/config/peer-group":                                                    true,
	"neighbors/neighbor/config/send-community":                                                true,
	"neighbors/neighbor/as-path-options/config/allow-own-as":                                  true,
	"neighbors/neighbor/afi-safis/afi-safi/afi-safi-name":                                     true,
	"neighbors/neighbor/afi-safis/afi-safi/config/afi-safi-name":                              true,
	"neighbors/neighbor/afi-safis/afi-safi/config/enabled":                                    true,
	"neighbors/neighbor/afi-safis/afi-safi/apply-policy/config/import-policy":                 true,
	"neighbors/neighbor/afi-safis/afi-safi/apply-policy/config/export-policy":                 true,
	"peer-groups/peer-group/peer-group-name":                                                  true,
	"peer-groups/peer-group/config/peer-group-name":                                           true,
	"peer-groups/peer-group/config/peer-as":                                                   true,
	"peer-groups/peer-group/timers/config/hold-time":                                          true,
	"peer-groups/peer-group/timers/config/keepalive-interval":                                 true,
	"peer-groups/peer-group/afi-safis/afi-safi/afi-safi-name":                                 true,
	"peer-groups/peer-group/afi-safis/afi-safi/config/afi-safi-name":                          true,
	"peer-groups/peer-group/afi-safis/afi-safi/config/enabled":                                true,
	"peer-groups/peer-group/afi-safis/afi-safi/apply-policy/config/export-policy":             true,
	"peer-groups/peer-group/afi-safis/afi-safi/apply-policy/config/import-policy":             true,
	"peer-groups/peer-group/afi-safis/afi-safi/ipv4-unicast/prefix-limit/config/max-prefixes": true,
	"peer-groups/peer-group/afi-safis/afi-safi/ipv6-unicast/prefix-limit/config/max-prefixes": true,
}

// lookupNiBgpProtoObj returns the bgp container of the BGP protocol instance
// without creating anything, so that a payload can be inspected as sent.
func lookupNiBgpProtoObj(s *ygot.GoStruct, niName string) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp {
	if s == nil || *s == nil {
		return nil
	}
	if bgpObj, ok := (*s).(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp); ok {
		return bgpObj
	}
	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil || deviceObj.NetworkInstances == nil {
		return nil
	}
	niObj, ok := deviceObj.NetworkInstances.NetworkInstance[niName]
	if !ok || niObj == nil || niObj.Protocols == nil {
		return nil
	}
	pObj, ok := niObj.Protocols.Protocol[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}]
	if !ok || pObj == nil {
		return nil
	}
	return pObj.Bgp
}

// findNiBgpUnmappedPayloadLeaves returns the payload leaves under the bgp
// container that carry a value but have no CONFIG_DB mapping, so that a
// container-level request is rejected as a whole instead of applying part of it
// and dropping the rest silently.
func findNiBgpUnmappedPayloadLeaves(s *ygot.GoStruct, niName string) []string {
	bgpObj := lookupNiBgpProtoObj(s, niName)
	if bgpObj == nil {
		return nil
	}
	var unmapped []string
	collectOcUnmappedPayloadLeaves(reflect.ValueOf(bgpObj), niBgpMappedPayloadLeaves, "", "bgp", 0, &unmapped)
	sort.Strings(unmapped)
	return unmapped
}

// collectOcUnmappedPayloadLeaves walks a payload subtree and reports the leaves
// that carry a value but are absent from mapped, the set of leaf paths the
// caller can write to CONFIG_DB.
func collectOcUnmappedPayloadLeaves(v reflect.Value, mapped map[string]bool, schemaPath, displayPath string, depth int, unmapped *[]string) {
	if depth >= niBgpMaxTreeDepth {
		return
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Tag.Get("path")
		if name == "" {
			continue
		}
		f := v.Field(i)
		childSchema := name
		if schemaPath != "" {
			childSchema = schemaPath + "/" + name
		}
		childDisplay := displayPath + "/" + name

		switch f.Kind() {
		case reflect.Ptr:
			if f.IsNil() {
				continue
			}
			if f.Type().Elem().Kind() == reflect.Struct {
				collectOcUnmappedPayloadLeaves(f, mapped, childSchema, childDisplay, depth+1, unmapped)
				continue
			}
		case reflect.Map:
			iter := f.MapRange()
			for iter.Next() {
				keyed := fmt.Sprintf("%s[%s]", childDisplay, niBgpPayloadListKey(iter.Key()))
				collectOcUnmappedPayloadLeaves(iter.Value(), mapped, childSchema, keyed, depth+1, unmapped)
			}
			continue
		case reflect.Slice:
			if f.Len() == 0 {
				continue
			}
		case reflect.Interface:
			if f.IsNil() {
				continue
			}
		case reflect.Int64:
			// Generated enum leaf; the zero value means unset.
			if f.Int() == 0 {
				continue
			}
		default:
			continue
		}

		if !mapped[childSchema] {
			*unmapped = append(*unmapped, childDisplay)
		}
	}
}

func niBgpPayloadListKey(k reflect.Value) string {
	if k.Kind() == reflect.String {
		return k.String()
	}
	if afi, ok := k.Interface().(ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE); ok {
		if name, ok := niBgpAfiSafiNameFromEnum(afi); ok {
			return name
		}
	}
	return fmt.Sprint(k.Interface())
}

// formatOcLeafPathList renders a bounded, comma separated list of YANG paths
// for an error message, shared by the container level payload validations.
func formatOcLeafPathList(paths []string) string {
	const maxReported = 10
	if len(paths) <= maxReported {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(paths[:maxReported], ", "), len(paths)-maxReported)
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
		buildNiBgpEmptySubtree(pObj)
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
		buildNiBgpEmptySubtree(pObj)
		return pObj.Bgp.Global.Config
	}

	if !createIfMissing {
		return nil
	}

	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pObj)
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
		buildNiBgpEmptySubtree(pObj)
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
		buildNiBgpEmptySubtree(pObj)
		return pObj.Bgp.Global.RouteSelectionOptions.Config
	}

	if !createIfMissing {
		return nil
	}

	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pObj)
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
		buildNiBgpEmptySubtree(pObj)
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
		buildNiBgpEmptySubtree(pObj)
		return pObj.Bgp.Global.UseMultiplePaths.Ebgp.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pObj)
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
		buildNiBgpEmptySubtree(pObj)
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
		buildNiBgpEmptySubtree(pObj)
		return pObj.Bgp.Global.UseMultiplePaths.Ibgp.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pObj)
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

func getNiBgpProtoObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp {
	if s != nil && *s != nil {
		if bgpObj, ok := (*s).(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp); ok && bgpObj != nil {
			buildNiBgpEmptySubtree(bgpObj)
			return bgpObj
		}
	}
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
		buildNiBgpEmptySubtree(pObj)
		return pObj.Bgp
	}
	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pObj)
	return pObj.Bgp
}

func wrapNiBgpRootAsDevice(s *ygot.GoStruct, niName string) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for BGP mapping"}
	}
	// If the root is already a full Device tree, no wrapping required.
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}
	// For /protocol/.../bgp anchored requests, bind root can be the bgp container.
	bgpObj, ok := (*s).(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp)
	if !ok || bgpObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for BGP mapping"}
	}

	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	if devObj.NetworkInstances == nil {
		devObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}
	niObj, err := devObj.NetworkInstances.NewNetworkInstance(niName)
	if err != nil || niObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for BGP mapping"}
	}
	ygot.BuildEmptyTree(niObj)
	if niObj.Protocols == nil {
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create protocol context for BGP mapping"}
	}
	// Reuse the original bgp subtree pointer so DbToYang population updates the
	// caller-owned object regardless of anchor path.
	pObj.Bgp = bgpObj
	buildNiBgpEmptySubtree(pObj)

	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}

func buildNiBgpHierResMap(s *ygot.GoStruct, niName string) map[string]map[string]db.Value {
	res := make(map[string]map[string]db.Value)
	bgpObj := getNiBgpProtoObj(s, niName, false)
	if bgpObj == nil {
		return res
	}

	if bgpObj.Global != nil {
		gFields := map[string]string{}
		if bgpObj.Global.Config != nil {
			if bgpObj.Global.Config.As != nil {
				gFields[niBgpAsFld] = strconv.FormatUint(uint64(*bgpObj.Global.Config.As), 10)
			}
			if bgpObj.Global.Config.RouterId != nil && strings.TrimSpace(*bgpObj.Global.Config.RouterId) != "" {
				gFields[niBgpRouterIdFld] = strings.TrimSpace(*bgpObj.Global.Config.RouterId)
			}
		}
		if bgpObj.Global.RouteSelectionOptions != nil && bgpObj.Global.RouteSelectionOptions.Config != nil && bgpObj.Global.RouteSelectionOptions.Config.AlwaysCompareMed != nil {
			gFields[niBgpAlwaysCompareMedFld] = strconv.FormatBool(*bgpObj.Global.RouteSelectionOptions.Config.AlwaysCompareMed)
		}
		if len(gFields) > 0 {
			res = mergeDbResMapFields(res, makeNiBgpGlobalsResMap(niName, gFields))
		}

		gAfFields := map[string]string{}
		if bgpObj.Global.UseMultiplePaths != nil {
			if bgpObj.Global.UseMultiplePaths.Ebgp != nil && bgpObj.Global.UseMultiplePaths.Ebgp.Config != nil && bgpObj.Global.UseMultiplePaths.Ebgp.Config.MaximumPaths != nil {
				gAfFields[niBgpMaxEbgpPathsFld] = strconv.FormatUint(uint64(*bgpObj.Global.UseMultiplePaths.Ebgp.Config.MaximumPaths), 10)
			}
			if bgpObj.Global.UseMultiplePaths.Ibgp != nil && bgpObj.Global.UseMultiplePaths.Ibgp.Config != nil && bgpObj.Global.UseMultiplePaths.Ibgp.Config.MaximumPaths != nil {
				gAfFields[niBgpMaxIbgpPathsFld] = strconv.FormatUint(uint64(*bgpObj.Global.UseMultiplePaths.Ibgp.Config.MaximumPaths), 10)
			}
		}
		if len(gAfFields) > 0 {
			res = mergeDbResMapFields(res, makeNiBgpGlobalsAfResMap(niName, gAfFields))
		}
	}

	if bgpObj.Neighbors != nil {
		for nbrAddr, nbrObj := range bgpObj.Neighbors.Neighbor {
			if nbrObj == nil {
				continue
			}
			nbrFields := map[string]string{}
			if nbrObj.Config != nil {
				if nbrObj.Config.PeerGroup != nil && strings.TrimSpace(*nbrObj.Config.PeerGroup) != "" {
					nbrFields[niBgpNbrPeerGroupFld] = strings.TrimSpace(*nbrObj.Config.PeerGroup)
				}
				if nbrObj.Config.PeerAs != nil {
					nbrFields[niBgpNbrAsnFld] = strconv.FormatUint(uint64(*nbrObj.Config.PeerAs), 10)
				}
				if nbrObj.Config.Enabled != nil {
					nbrFields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*nbrObj.Config.Enabled)
				}
			}
			// A payload may name a list instance while setting no mapped leaf
			// under it; that still asks for the row itself to exist.
			res = mergeDbResMapFields(res, makeNiBgpNbrResMap(niName, nbrAddr, nbrFields))
			if nbrObj.Config != nil {
				if comm, ok := niBgpNbrSendCommunityToDb(nbrObj.Config.SendCommunity); ok {
					res = mergeDbResMapFields(res, ensureNiBgpNbrParentRow(makeNiBgpNbrAfResMap(niName, nbrAddr, map[string]string{
						niBgpNbrSendCommFld: comm,
					}), niName, nbrAddr))
				}
			}
			if nbrObj.AsPathOptions != nil && nbrObj.AsPathOptions.Config != nil && nbrObj.AsPathOptions.Config.AllowOwnAs != nil {
				allowOwnAs := *nbrObj.AsPathOptions.Config.AllowOwnAs
				if allowOwnAs == 0 {
					res = mergeDbResMapFields(res, ensureNiBgpNbrParentRow(makeNiBgpNbrAfResMap(niName, nbrAddr, map[string]string{
						niBgpNbrAllowAsInFld:  "",
						niBgpNbrAllowAsCntFld: "",
					}), niName, nbrAddr))
				} else {
					res = mergeDbResMapFields(res, ensureNiBgpNbrParentRow(makeNiBgpNbrAfResMap(niName, nbrAddr, map[string]string{
						niBgpNbrAllowAsInFld:  "true",
						niBgpNbrAllowAsCntFld: strconv.FormatUint(uint64(allowOwnAs), 10),
					}), niName, nbrAddr))
				}
			}
			if nbrObj.AfiSafis != nil {
				for afiEnum, afiObj := range nbrObj.AfiSafis.AfiSafi {
					if afiObj == nil {
						continue
					}
					afiSafi, ok := niBgpAfiSafiNameFromEnum(afiEnum)
					if !ok {
						continue
					}
					afFields := map[string]string{}
					// Naming an AFI-SAFI under a neighbor activates it there, so
					// the row carries admin_status even when enabled is absent.
					enabled := true
					if afiObj.Config != nil && afiObj.Config.Enabled != nil {
						enabled = *afiObj.Config.Enabled
					}
					afFields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(enabled)
					if afiObj.ApplyPolicy != nil && afiObj.ApplyPolicy.Config != nil {
						if importPolicy, ok := niBgpFirstPolicyName(afiObj.ApplyPolicy.Config.ImportPolicy); ok {
							afFields[niBgpNbrRouteMapInFld] = importPolicy
						}
						if exportPolicy, ok := niBgpFirstPolicyName(afiObj.ApplyPolicy.Config.ExportPolicy); ok {
							afFields[niBgpNbrRouteMapOutFld] = exportPolicy
						}
					}
					res = mergeDbResMapFields(res, ensureNiBgpNbrParentRow(makeNiBgpNbrAfSingleResMap(niName, nbrAddr, afiSafi, afFields), niName, nbrAddr))
				}
			}
		}
	}

	if bgpObj.PeerGroups != nil {
		for pgName, pgObj := range bgpObj.PeerGroups.PeerGroup {
			if pgObj == nil {
				continue
			}
			pgFields := map[string]string{}
			if pgObj.Config != nil && pgObj.Config.PeerAs != nil {
				pgFields[niBgpPgAsnFld] = strconv.FormatUint(uint64(*pgObj.Config.PeerAs), 10)
			}
			if pgObj.Timers != nil && pgObj.Timers.Config != nil {
				if pgObj.Timers.Config.KeepaliveInterval != nil {
					pgFields[niBgpPgKeepaliveFld] = strconv.FormatUint(uint64(*pgObj.Timers.Config.KeepaliveInterval), 10)
				}
				if pgObj.Timers.Config.HoldTime != nil {
					pgFields[niBgpPgHoldTimeFld] = strconv.FormatUint(uint64(*pgObj.Timers.Config.HoldTime), 10)
				}
			}
			res = mergeDbResMapFields(res, makeNiBgpPgResMap(niName, pgName, pgFields))
			if pgObj.AfiSafis != nil {
				for afiEnum, afiObj := range pgObj.AfiSafis.AfiSafi {
					if afiObj == nil {
						continue
					}
					afiSafi, ok := niBgpAfiSafiNameFromEnum(afiEnum)
					if !ok {
						continue
					}
					// Naming an AFI-SAFI under a peer-group activates it there,
					// so the row carries admin_status even when enabled is
					// absent, as it does for a neighbor.
					pgAfEnabled := true
					if afiObj.Config != nil && afiObj.Config.Enabled != nil {
						pgAfEnabled = *afiObj.Config.Enabled
					}
					res = mergeDbResMapFields(res, makeNiBgpPgAfResMap(niName, pgName, afiSafi, map[string]string{
						niBgpPgAdminStatusFld: niBgpPgEnabledToAdminStatus(pgAfEnabled),
					}))
					if afiObj.Ipv4Unicast != nil && afiObj.Ipv4Unicast.PrefixLimit != nil && afiObj.Ipv4Unicast.PrefixLimit.Config != nil && afiObj.Ipv4Unicast.PrefixLimit.Config.MaxPrefixes != nil {
						res = mergeDbResMapFields(res, makeNiBgpPgAfResMap(niName, pgName, niBgpPgAfiSafiIPv4, map[string]string{
							niBgpPgMaxPfxLimitFld: strconv.FormatUint(uint64(*afiObj.Ipv4Unicast.PrefixLimit.Config.MaxPrefixes), 10),
						}))
					}
					if afiObj.Ipv6Unicast != nil && afiObj.Ipv6Unicast.PrefixLimit != nil && afiObj.Ipv6Unicast.PrefixLimit.Config != nil && afiObj.Ipv6Unicast.PrefixLimit.Config.MaxPrefixes != nil {
						res = mergeDbResMapFields(res, makeNiBgpPgAfResMap(niName, pgName, niBgpPgAfiSafiIPv6, map[string]string{
							niBgpPgMaxPfxLimitFld: strconv.FormatUint(uint64(*afiObj.Ipv6Unicast.PrefixLimit.Config.MaxPrefixes), 10),
						}))
					}
					if afiObj.ApplyPolicy != nil && afiObj.ApplyPolicy.Config != nil {
						applyFields := map[string]string{}
						if exportPolicy, ok := niBgpFirstPolicyName(afiObj.ApplyPolicy.Config.ExportPolicy); ok {
							applyFields[niBgpPgRouteMapOutFld] = exportPolicy
						}
						if importPolicy, ok := niBgpFirstPolicyName(afiObj.ApplyPolicy.Config.ImportPolicy); ok {
							applyFields[niBgpPgRouteMapInFld] = importPolicy
						}
						if len(applyFields) > 0 {
							res = mergeDbResMapFields(res, makeNiBgpPgAfResMap(niName, pgName, afiSafi, applyFields))
						}
					}
				}
			}
		}
	}

	return res
}

func populateNiBgpHierToYangFromDb(cfgDb *db.DB, ygRoot *ygot.GoStruct, niName string) error {
	// Global fields from BGP_GLOBALS.
	if gEntry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsTbl}, db.Key{Comp: []string{niName}}); err == nil {
		gCfg := getNiBgpGlobalCfgObj(ygRoot, niName, true)
		if gCfg != nil {
			if asStr, ok := gEntry.Field[niBgpAsFld]; ok && strings.TrimSpace(asStr) != "" {
				if asVal, convErr := strconv.ParseUint(strings.TrimSpace(asStr), 10, 32); convErr == nil {
					asU32 := uint32(asVal)
					gCfg.As = &asU32
				}
			}
			if rid, ok := gEntry.Field[niBgpRouterIdFld]; ok && strings.TrimSpace(rid) != "" {
				rid = strings.TrimSpace(rid)
				gCfg.RouterId = &rid
			}
		}
		routeCfg := getNiBgpGlobalRouteSelectCfgObj(ygRoot, niName, true)
		if routeCfg != nil {
			if medStr, ok := gEntry.Field[niBgpAlwaysCompareMedFld]; ok && strings.TrimSpace(medStr) != "" {
				if med, convErr := strconv.ParseBool(strings.TrimSpace(medStr)); convErr == nil {
					routeCfg.AlwaysCompareMed = &med
				}
			}
		}
	}

	// Global AF fields from BGP_GLOBALS_AF.
	if afKeys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpGlobalsAfTbl}); err == nil {
		for _, k := range afKeys {
			parts := k.Comp
			if len(parts) != 2 || parts[0] != niName {
				continue
			}
			afEntry, afErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsAfTbl}, db.Key{Comp: []string{parts[0], parts[1]}})
			if afErr != nil {
				continue
			}
			if mpStr, ok := afEntry.Field[niBgpMaxEbgpPathsFld]; ok && strings.TrimSpace(mpStr) != "" {
				if mpVal, convErr := strconv.ParseUint(strings.TrimSpace(mpStr), 10, 32); convErr == nil {
					ebgpCfg := getNiBgpGlobalUseMpEbgpCfgObj(ygRoot, niName, true)
					if ebgpCfg != nil {
						mp := uint32(mpVal)
						ebgpCfg.MaximumPaths = &mp
					}
				}
			}
			if mpStr, ok := afEntry.Field[niBgpMaxIbgpPathsFld]; ok && strings.TrimSpace(mpStr) != "" {
				if mpVal, convErr := strconv.ParseUint(strings.TrimSpace(mpStr), 10, 32); convErr == nil {
					ibgpCfg := getNiBgpGlobalUseMpIbgpCfgObj(ygRoot, niName, true)
					if ibgpCfg != nil {
						mp := uint32(mpVal)
						ibgpCfg.MaximumPaths = &mp
					}
				}
			}
		}
	}

	// Neighbor config from BGP_NEIGHBOR.
	if nbrKeys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpNbrTbl}); err == nil {
		for _, k := range nbrKeys {
			parts := k.Comp
			if len(parts) != 2 || parts[0] != niName {
				continue
			}
			nbrAddr := parts[1]
			nbrEntry, nbrErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrTbl}, db.Key{Comp: []string{niName, nbrAddr}})
			if nbrErr != nil {
				continue
			}
			nbrCfg := getNiBgpNbrCfgObj(ygRoot, niName, nbrAddr, true)
			if nbrCfg != nil {
				if pg, ok := nbrEntry.Field[niBgpNbrPeerGroupFld]; ok && strings.TrimSpace(pg) != "" {
					pg = strings.TrimSpace(pg)
					nbrCfg.PeerGroup = &pg
				}
				if asnStr, ok := nbrEntry.Field[niBgpNbrAsnFld]; ok && strings.TrimSpace(asnStr) != "" {
					if asnVal, convErr := strconv.ParseUint(strings.TrimSpace(asnStr), 10, 32); convErr == nil {
						asn := uint32(asnVal)
						nbrCfg.PeerAs = &asn
					}
				}
				if enStr, ok := nbrEntry.Field[niBgpNbrAdminStatus]; ok && strings.TrimSpace(enStr) != "" {
					if en, ok := niBgpNbrAdminStatusToEnabled(enStr); ok {
						nbrCfg.Enabled = en
					}
				}
			}
		}
	}

	// Neighbor AF config from BGP_NEIGHBOR_AF.
	if nbrAfKeys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpNbrAfTbl}); err == nil {
		for _, k := range nbrAfKeys {
			parts := k.Comp
			if len(parts) != 3 || parts[0] != niName {
				continue
			}
			nbrAddr := parts[1]
			afi := parts[2]
			nbrAfEntry, afErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{niName, nbrAddr, afi}})
			if afErr != nil {
				continue
			}
			if allowCnt, ok := nbrAfEntry.Field[niBgpNbrAllowAsCntFld]; ok && strings.TrimSpace(allowCnt) != "" {
				if cntVal, convErr := strconv.ParseUint(strings.TrimSpace(allowCnt), 10, 8); convErr == nil {
					asPathCfg := getNiBgpNbrAsPathOptionsCfgObj(ygRoot, niName, nbrAddr, true)
					if asPathCfg != nil {
						v := uint8(cntVal)
						asPathCfg.AllowOwnAs = &v
					}
				}
			}
			if comm, ok := niBgpNbrSendCommunityFromDb(nbrAfEntry.Field[niBgpNbrSendCommFld]); ok {
				if nbrCfg := getNiBgpNbrCfgObj(ygRoot, niName, nbrAddr, true); nbrCfg != nil {
					nbrCfg.SendCommunity = comm
				}
			}
			if imp, ok := nbrAfEntry.Field[niBgpNbrRouteMapInFld]; ok && strings.TrimSpace(imp) != "" {
				applyCfg := getNiBgpNbrAfApplyPolicyCfgObj(ygRoot, niName, nbrAddr, afi, true)
				if applyCfg != nil {
					applyCfg.ImportPolicy = []string{strings.TrimSpace(imp)}
				}
			}
			if exp, ok := nbrAfEntry.Field[niBgpNbrRouteMapOutFld]; ok && strings.TrimSpace(exp) != "" {
				applyCfg := getNiBgpNbrAfApplyPolicyCfgObj(ygRoot, niName, nbrAddr, afi, true)
				if applyCfg != nil {
					applyCfg.ExportPolicy = []string{strings.TrimSpace(exp)}
				}
			}
			if afCfg := getNiBgpNbrAfCfgObj(ygRoot, niName, nbrAddr, afi, true); afCfg != nil {
				if afiSafiName, ok := niBgpNbrAfiSafiEnumFromName(afi); ok {
					afCfg.AfiSafiName = afiSafiName
				}
				if status, ok := nbrAfEntry.Field[niBgpNbrAdminStatus]; ok && strings.TrimSpace(status) != "" {
					if en, ok := niBgpNbrAdminStatusToEnabled(status); ok {
						afCfg.Enabled = en
					}
				}
			}
		}
	}

	// Peer-group config from BGP_PEER_GROUP.
	if pgKeys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpPeerGroupTbl}); err == nil {
		for _, k := range pgKeys {
			parts := k.Comp
			if len(parts) != 2 || parts[0] != niName {
				continue
			}
			pgName := parts[1]
			pgEntry, pgErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupTbl}, db.Key{Comp: []string{niName, pgName}})
			if pgErr != nil {
				continue
			}
			pgCfg := getNiBgpPeerGroupCfgObj(ygRoot, niName, pgName, true)
			if pgCfg != nil {
				if asnStr, ok := pgEntry.Field[niBgpPgAsnFld]; ok && strings.TrimSpace(asnStr) != "" {
					if asnVal, convErr := strconv.ParseUint(strings.TrimSpace(asnStr), 10, 32); convErr == nil {
						asn := uint32(asnVal)
						pgCfg.PeerAs = &asn
					}
				}
			}
			tmCfg := getNiBgpPeerGroupTimersCfgObj(ygRoot, niName, pgName, true)
			if tmCfg != nil {
				if keepStr, ok := pgEntry.Field[niBgpPgKeepaliveFld]; ok && strings.TrimSpace(keepStr) != "" {
					if keepVal, convErr := strconv.ParseUint(strings.TrimSpace(keepStr), 10, 64); convErr == nil {
						keep := float64(uint16(keepVal))
						tmCfg.KeepaliveInterval = &keep
					}
				}
				if holdStr, ok := pgEntry.Field[niBgpPgHoldTimeFld]; ok && strings.TrimSpace(holdStr) != "" {
					if holdVal, convErr := strconv.ParseUint(strings.TrimSpace(holdStr), 10, 64); convErr == nil {
						hold := float64(uint16(holdVal))
						tmCfg.HoldTime = &hold
					}
				}
			}
		}
	}

	// Peer-group AF config from BGP_PEER_GROUP_AF.
	if pgAfKeys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpPeerGroupAfTbl}); err == nil {
		for _, k := range pgAfKeys {
			parts := k.Comp
			if len(parts) != 3 || parts[0] != niName {
				continue
			}
			pgName := parts[1]
			afi := parts[2]
			pgAfEntry, afErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupAfTbl}, db.Key{Comp: []string{niName, pgName, afi}})
			if afErr != nil {
				continue
			}
			if afCfg := getNiBgpPeerGroupAfCfgObj(ygRoot, niName, pgName, afi, true); afCfg != nil {
				if afiSafiName, ok := niBgpPgAfiSafiEnumFromName(afi); ok {
					afCfg.AfiSafiName = afiSafiName
				}
				if en, ok := niBgpPgAdminStatusToEnabled(pgAfEntry.Field[niBgpPgAdminStatusFld]); ok {
					afCfg.Enabled = en
				}
			}
			if maxStr, ok := pgAfEntry.Field[niBgpPgMaxPfxLimitFld]; ok && strings.TrimSpace(maxStr) != "" {
				if maxVal, convErr := strconv.ParseUint(strings.TrimSpace(maxStr), 10, 32); convErr == nil {
					v := uint32(maxVal)
					if afi == niBgpPgAfiSafiIPv6 {
						if pfxCfg := getNiBgpPeerGroupAfV6PrefixLimitCfgObj(ygRoot, niName, pgName, true); pfxCfg != nil {
							pfxCfg.MaxPrefixes = &v
						}
					} else if pfxCfg := getNiBgpPeerGroupAfV4PrefixLimitCfgObj(ygRoot, niName, pgName, true); pfxCfg != nil {
						pfxCfg.MaxPrefixes = &v
					}
				}
			}
			if exp, ok := pgAfEntry.Field[niBgpPgRouteMapOutFld]; ok && strings.TrimSpace(exp) != "" {
				applyCfg := getNiBgpPeerGroupAfApplyPolicyCfgObj(ygRoot, niName, pgName, afi, true)
				if applyCfg != nil {
					applyCfg.ExportPolicy = []string{strings.TrimSpace(exp)}
				}
			}
			if imp, ok := pgAfEntry.Field[niBgpPgRouteMapInFld]; ok && strings.TrimSpace(imp) != "" {
				applyCfg := getNiBgpPeerGroupAfApplyPolicyCfgObj(ygRoot, niName, pgName, afi, true)
				if applyCfg != nil {
					applyCfg.ImportPolicy = []string{strings.TrimSpace(imp)}
				}
			}
		}
	}

	return nil
}

var YangToDb_ni_bgp_global_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (res map[string]map[string]db.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance BGP transformer panic in SET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			res = make(map[string]map[string]db.Value)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance BGP transformer panic in SET path: %v", r)}
		}
	}()
	res = make(map[string]map[string]db.Value)

	// The protocol list carries this transformer for every protocol instance,
	// so requests for the STATIC instance are handed to the static route
	// mapping instead of being failed as unsupported BGP paths.
	if handled, staticRes, staticErr := niStaticRouteYangToDbDispatch(inParams); handled {
		if staticRes == nil {
			staticRes = make(map[string]map[string]db.Value)
		}
		return staticRes, staticErr
	}

	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	// DELETE: clear only the targeted field in BGP_GLOBALS|<ni-name>.
	if inParams.oper == DELETE {
		// The neighbor and peer-group list instances, and the containers
		// holding them, carry this transformer rather than their own, so their
		// deletes are mapped by those files before the container refusal below.
		if handled, nbrRes, nbrErr := niBgpNbrHierDeleteDispatch(inParams); handled {
			if nbrRes == nil {
				nbrRes = make(map[string]map[string]db.Value)
			}
			return nbrRes, nbrErr
		}
		if handled, pgRes, pgErr := niBgpPgHierDeleteDispatch(inParams); handled {
			if pgRes == nil {
				pgRes = make(map[string]map[string]db.Value)
			}
			return pgRes, pgErr
		}
		if reqInfo.targetPathKind == niBgpTargetProtocolRoot || reqInfo.targetPathKind == niBgpTargetBgpRoot {
			return res, tlerr.NotSupportedError{Format: "DELETE is not supported at BGP container level; delete mapped child paths or list instances instead", Path: inParams.requestUri}
		}
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
	if reqInfo.targetPathKind == niBgpTargetProtocolRoot || reqInfo.targetPathKind == niBgpTargetBgpRoot {
		wrappedRoot, wrapErr := wrapNiBgpRootAsDevice(inParams.ygRoot, reqInfo.niName)
		if wrapErr != nil {
			return res, wrapErr
		}
		if unmapped := findNiBgpUnmappedPayloadLeaves(wrappedRoot, reqInfo.niName); len(unmapped) > 0 {
			return res, tlerr.NotSupportedError{
				Format: "BGP container payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
				Path:   inParams.requestUri,
			}
		}
		res = buildNiBgpHierResMap(wrappedRoot, reqInfo.niName)
		if len(res) == 0 {
			return res, tlerr.InvalidArgsError{Format: "BGP container payload does not include currently supported mapped fields"}
		}
		if inParams.invokeCRUSubtreeOnce != nil {
			// The whole subtree is mapped in this call, so translib must not
			// invoke this transformer again for the child nodes.
			*inParams.invokeCRUSubtreeOnce = true
		}
		return res, nil
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

var DbToYang_ni_bgp_global_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance BGP transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance BGP transformer panic in GET path: %v", r)}
		}
	}()

	if handled, staticErr := niStaticRouteDbToYangDispatch(inParams); handled {
		return staticErr
	}

	reqInfo, err := parseNiBgpRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP global config"}
	}
	if reqInfo.targetPathKind == niBgpTargetProtocolRoot || reqInfo.targetPathKind == niBgpTargetBgpRoot {
		wrappedRoot, wrapErr := wrapNiBgpRootAsDevice(inParams.ygRoot, reqInfo.niName)
		if wrapErr != nil {
			return wrapErr
		}
		return populateNiBgpHierToYangFromDb(cfgDb, wrappedRoot, reqInfo.niName)
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
