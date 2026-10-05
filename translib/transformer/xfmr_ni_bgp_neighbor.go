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
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niBgpNbrTbl           = "BGP_NEIGHBOR"
	niBgpNbrAfTbl         = "BGP_NEIGHBOR_AF"
	niBgpNbrPeerGroupFld  = "peer_group_name"
	niBgpNbrAsnFld        = "asn"
	niBgpNbrAdminStatus   = "admin_status"
	niBgpNbrRouteMapInFld = "route_map_in"
	// The outbound direction is spelled the way the inbound one already is in
	// this table. BGP_PEER_GROUP_AF suffixes both of its route-map fields with
	// the leaf-list "@" instead; frrcfgd renders either spelling.
	niBgpNbrRouteMapOutFld = "route_map_out"
	niBgpNbrAllowAsInFld   = "allow_as_in"
	niBgpNbrAllowAsCntFld  = "allow_as_count"
	niBgpNbrSendCommFld    = "send_community"
	niBgpNbrAuthPwdFld     = "auth_password"
	niBgpNbrConnRetryFld   = "conn_retry"
	niBgpNbrHoldTimeFld    = "holdtime"
	niBgpNbrKeepaliveFld   = "keepalive"
	niBgpNbrAfiSafiIPv4    = "ipv4_unicast"
	niBgpNbrAfiSafiIPv6    = "ipv6_unicast"
	// The timers container holds its own target kind because its leaves land in
	// BGP_NEIGHBOR beside the ones the config container maps.
	niBgpNbrTargetTimersCfg  = "neighbor-timers-config"
	niBgpNbrConnectRetryLeaf = "connect-retry"
	niBgpNbrHoldTimeLeaf     = "hold-time"
	niBgpNbrKeepaliveLeaf    = "keepalive-interval"
	// transport/config is the same shape as timers/config: its leaves land in
	// BGP_NEIGHBOR beside the neighbor config container, so it has its own
	// target kind. Only local-address and passive-mode are mapped; tcp-mss
	// and mtu-discovery have no CONFIG_DB field.
	niBgpNbrTargetTransportCfg = "neighbor-transport-config"
	niBgpNbrLocalAddrFld       = "local_addr"
	niBgpNbrPassiveModeFld     = "passive_mode"
	niBgpNbrPassiveModeLeaf    = "passive-mode"
	// The levels of the neighbor hierarchy that sit above the config
	// containers. Each resolves to a target kind the write and read paths
	// switch on, the same shape the table-connection mapping uses.
	niBgpNbrTargetAfis      = "neighbor-afi-safis"
	niBgpNbrTargetAfInst    = "neighbor-afi-safi-instance"
	niBgpNbrAfiSafiNameLeaf = "afi-safi-name"
	// The leaf-list that replaced the deprecated singular send-community on
	// the neighbor config container, which is no longer served.
	niBgpNbrSendCommTypeLeaf  = "send-community-type"
	niBgpNbrSendCommunityLeaf = "send-community"
)

// niBgpNbrUriPfx names a neighbor list instance, which every path below it
// repeats. The levels above the config containers are matched against it.
const niBgpNbrUriPfx = niBgpProtoUriPfx + `/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]`

var niBgpNbrReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/config(?:/(peer-group|enabled|peer-as|send-community|auth-password))?$`)
var niBgpNbrTimersReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/timers/config(?:/(connect-retry|hold-time|keepalive-interval))?$`)
var niBgpNbrTransportCfgReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/transport/config(?:/(local-address|passive-mode))?$`)
var niBgpNbrAsPathOptReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/as-path-options/config(?:/(allow-own-as))?$`)

// The apply-policy container and its config container are one target: the
// config container is the only writable child.
var niBgpNbrAfApplyReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/apply-policy(?:/config(?:/(import-policy|export-policy|default-import-policy|default-export-policy))?)?$`)
var niBgpNbrAfCfgReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/config(?:/(afi-safi-name|enabled|send-community-type))?$`)

// The afi-safi key leaf below the list instance repeats a key already in the
// URI, so it resolves to the same row as the instance itself.
var niBgpNbrAfInstKeyLeafReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/afi-safi-name$`)
var niBgpNbrAfInstReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]$`)
var niBgpNbrAfisReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/afi-safis$`)

// The neighbor list instance and the neighbors container are annotated to the
// BGP container dispatch rather than to this transformer, so these two are
// matched by the delete dispatch it calls rather than by the request parser.
var niBgpNbrInstKeyLeafReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/neighbor-address$`)
var niBgpNbrInstReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `$`)
var niBgpNbrsReqUriRe = regexp.MustCompile(niBgpProtoUriPfx + `/bgp/neighbors$`)

type niBgpNbrReqInfo struct {
	niName       string
	protoName    string
	neighborAddr string
	afiSafi      string
	targetLeaf   string // "", "peer-group", "enabled", "peer-as"
	targetPath   string
}

func init() {
	XlateFuncBind("YangToDb_ni_bgp_nbr_cfg_xfmr", YangToDb_ni_bgp_nbr_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_bgp_nbr_cfg_xfmr", DbToYang_ni_bgp_nbr_cfg_xfmr)
}

func parseNiBgpNbrRequestUri(uri string) (*niBgpNbrReqInfo, error) {
	reqInfo, err := matchNiBgpNbrRequestUri(uri)
	if err != nil {
		return nil, err
	}
	if err := validateNiBgpProtoName(reqInfo.protoName); err != nil {
		return nil, err
	}
	return reqInfo, nil
}

func matchNiBgpNbrRequestUri(uri string) (*niBgpNbrReqInfo, error) {
	m := niBgpNbrReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			targetLeaf:   leaf,
			targetPath:   "neighbor-config",
		}, nil
	}
	m = niBgpNbrTimersReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			targetLeaf:   leaf,
			targetPath:   niBgpNbrTargetTimersCfg,
		}, nil
	}
	m = niBgpNbrTransportCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			targetLeaf:   leaf,
			targetPath:   niBgpNbrTargetTransportCfg,
		}, nil
	}
	m = niBgpNbrAsPathOptReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			targetLeaf:   leaf,
			targetPath:   "neighbor-as-path-options-config",
		}, nil
	}
	m = niBgpNbrAfApplyReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 5 {
		leaf := ""
		if len(m) >= 6 {
			leaf = m[5]
		}
		afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP neighbor apply-policy mapping: " + m[4],
			}
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			afiSafi:      afiSafi,
			targetLeaf:   leaf,
			targetPath:   "neighbor-af-apply-policy-config",
		}, nil
	}
	m = niBgpNbrAfCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 5 {
		leaf := ""
		if len(m) >= 6 {
			leaf = m[5]
		}
		afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP neighbor AFI-SAFI config mapping: " + m[4],
			}
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			afiSafi:      afiSafi,
			targetLeaf:   leaf,
			targetPath:   "neighbor-af-config",
		}, nil
	}
	if m := niBgpNbrAfInstKeyLeafReqUriRe.FindStringSubmatch(uri); len(m) >= 5 {
		afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP neighbor AFI-SAFI mapping: " + m[4],
			}
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			afiSafi:      afiSafi,
			targetLeaf:   niBgpNbrAfiSafiNameLeaf,
			targetPath:   niBgpNbrTargetAfInst,
		}, nil
	}
	if m := niBgpNbrAfInstReqUriRe.FindStringSubmatch(uri); len(m) >= 5 {
		afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP neighbor AFI-SAFI mapping: " + m[4],
			}
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			afiSafi:      afiSafi,
			targetPath:   niBgpNbrTargetAfInst,
		}, nil
	}
	if m := niBgpNbrAfisReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		return &niBgpNbrReqInfo{
			niName:       m[1],
			protoName:    m[2],
			neighborAddr: m[3],
			targetPath:   niBgpNbrTargetAfis,
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP neighbor config mapping: " + uri,
	}
}

func niBgpNbrAfiSafiEnumFromName(afiSafi string) (ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE, bool) {
	switch afiSafi {
	case niBgpNbrAfiSafiIPv4:
		return ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST, true
	case niBgpNbrAfiSafiIPv6:
		return ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV6_UNICAST, true
	default:
		return ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_UNSET, false
	}
}

func normalizeNiBgpNbrAfiSafiName(v string) (string, bool) {
	s := strings.TrimSpace(v)
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.ToLower(strings.ReplaceAll(s, "-", "_"))
	switch s {
	case niBgpNbrAfiSafiIPv4:
		return niBgpNbrAfiSafiIPv4, true
	case niBgpNbrAfiSafiIPv6:
		return niBgpNbrAfiSafiIPv6, true
	default:
		return "", false
	}
}

func getNiBgpNbrAsPathOptionsCfgObj(s *ygot.GoStruct, niName, protoName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AsPathOptions_Config {
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
		Name:       protoName,
	}
	if pObj, ok := niObj.Protocols.Protocol[wantKey]; ok && pObj != nil {
		buildNiBgpEmptySubtree(pObj)
		if nbrObj, ok := pObj.Bgp.Neighbors.Neighbor[neighborAddr]; ok && nbrObj != nil {
			buildNiBgpEmptySubtree(nbrObj)
			return nbrObj.AsPathOptions.Config
		}
		if !createIfMissing {
			return nil
		}
		nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
		if err != nil || nbrObj == nil {
			return nil
		}
		buildNiBgpEmptySubtree(nbrObj)
		return nbrObj.AsPathOptions.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, protoName)
	if err != nil || pObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pObj)
	nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
	if err != nil || nbrObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(nbrObj)
	return nbrObj.AsPathOptions.Config
}

func getNiBgpNbrAfObj(s *ygot.GoStruct, niName, protoName, neighborAddr, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi {
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
		Name:       protoName,
	}
	pObj, ok := niObj.Protocols.Protocol[wantKey]
	if !ok || pObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pObj, err = niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, protoName)
		if err != nil || pObj == nil {
			return nil
		}
	}
	buildNiBgpEmptySubtree(pObj)

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
	buildNiBgpEmptySubtree(nbrObj)

	afiKey, ok2 := niBgpNbrAfiSafiEnumFromName(afiSafi)
	if !ok2 {
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
	buildNiBgpEmptySubtree(afObj)
	return afObj
}

func getNiBgpNbrAfApplyPolicyCfgObj(s *ygot.GoStruct, niName, protoName, neighborAddr, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy_Config {
	afObj := getNiBgpNbrAfObj(s, niName, protoName, neighborAddr, afiSafi, createIfMissing)
	if afObj == nil || afObj.ApplyPolicy == nil {
		return nil
	}
	return afObj.ApplyPolicy.Config
}

func getNiBgpNbrAfCfgObj(s *ygot.GoStruct, niName, protoName, neighborAddr, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Config {
	afObj := getNiBgpNbrAfObj(s, niName, protoName, neighborAddr, afiSafi, createIfMissing)
	if afObj == nil {
		return nil
	}
	return afObj.Config
}

// wrapNiBgpNbrRootAsDevice normalizes the request root to a device object so
// that one set of tree walks serves every anchor path in the hierarchy. A
// request anchored on the afi-safis container or on an afi-safi instance
// arrives with that node as the root rather than with a full device tree.
func wrapNiBgpNbrRootAsDevice(s *ygot.GoStruct, reqInfo *niBgpNbrReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for BGP neighbor mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}

	devObj := &ocbinds.Device{}
	var devRoot ygot.GoStruct = devObj
	nbrObj := getNiBgpNbrObj(&devRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
	if nbrObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create neighbor context for BGP neighbor mapping"}
	}

	// Reuse the original subtree pointers so that DbToYang population updates
	// the caller owned objects regardless of the anchor path.
	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis:
		nbrObj.AfiSafis = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi:
		afiKey, ok := niBgpNbrAfiSafiEnumFromName(reqInfo.afiSafi)
		if !ok {
			return nil, tlerr.InvalidArgsError{Format: "unsupported afi-safi-name for network-instance BGP neighbor AFI-SAFI mapping: " + reqInfo.afiSafi}
		}
		if nbrObj.AfiSafis.AfiSafi == nil {
			nbrObj.AfiSafis.AfiSafi = map[ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi{}
		}
		nbrObj.AfiSafis.AfiSafi[afiKey] = root
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root for BGP neighbor mapping"}
	}

	return &devRoot, nil
}

// niBgpNbrPayloadAfiSafiName reads an afi-safi-name leaf from the payload. An
// unrecognized shape yields false, which skips the comparison rather than
// refusing a write whose key translib has already validated.
func niBgpNbrPayloadAfiSafiName(param interface{}) (string, bool) {
	switch v := param.(type) {
	case ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE:
		return niBgpAfiSafiNameFromEnum(v)
	case string:
		return normalizeNiBgpNbrAfiSafiName(v)
	case *string:
		if v != nil {
			return normalizeNiBgpNbrAfiSafiName(*v)
		}
	}
	return "", false
}

// niBgpNbrAfInstFields renders the BGP_NEIGHBOR_AF fields an afi-safi instance
// payload carries. Naming an address family under a neighbor activates it
// there, so the row carries admin_status even when enabled is absent, which is
// what frrcfgd needs to emit "neighbor <addr> activate".
func niBgpNbrAfInstFields(afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi) (map[string]string, error) {
	fields := map[string]string{}
	enabled := true
	if afObj != nil && afObj.Config != nil && afObj.Config.Enabled != nil {
		enabled = *afObj.Config.Enabled
	}
	fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(enabled)
	if afObj != nil && afObj.Config != nil && len(afObj.Config.SendCommunityType) > 0 {
		comm, err := niBgpSendCommunityTypeToField(nil, afObj.Config.SendCommunityType, "neighbor")
		if err != nil {
			return nil, err
		}
		fields[niBgpNbrSendCommFld] = comm
	}
	if afObj != nil && afObj.ApplyPolicy != nil && afObj.ApplyPolicy.Config != nil {
		cfg := afObj.ApplyPolicy.Config
		if err := niBgpApplyPolicyDefaultsErr("neighbor", cfg.DefaultImportPolicy, cfg.DefaultExportPolicy); err != nil {
			return nil, err
		}
		policy, ok, err := niBgpSinglePolicyName(cfg.ImportPolicy, "neighbor", "import-policy")
		if err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpNbrRouteMapInFld] = policy
		}
		if policy, ok, err = niBgpSinglePolicyName(cfg.ExportPolicy, "neighbor", "export-policy"); err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpNbrRouteMapOutFld] = policy
		}
	}
	return fields, nil
}

// niBgpNbrAfInstOwnedFlds are the BGP_NEIGHBOR_AF fields an afi-safi instance
// maps. A REPLACE of the instance clears those the payload leaves out and
// keeps every other field of the row, such as the allowas_in pair that
// as-path-options writes onto both families.
var niBgpNbrAfInstOwnedFlds = map[string]bool{
	niBgpNbrAdminStatus:    true,
	niBgpNbrSendCommFld:    true,
	niBgpNbrRouteMapInFld:  true,
	niBgpNbrRouteMapOutFld: true,
}

// niBgpNbrAfInstWrite maps the row of one afi-safi instance a write carries.
func niBgpNbrAfInstWrite(inParams XfmrParams, reqInfo *niBgpNbrReqInfo, afiSafi string, fields map[string]string) map[string]string {
	if inParams.oper != REPLACE {
		return fields
	}
	existing := niBgpCfgDbEntry(inParams, niBgpNbrAfTbl, reqInfo.niName, reqInfo.neighborAddr, afiSafi)
	return niBgpReplaceRowFields(existing, fields, niBgpNbrAfInstOwnedFlds)
}

// niBgpNbrParentRowIfMissing adds the BGP_NEIGHBOR row an address-family write
// depends on, unless CONFIG_DB already holds it. A REPLACE anchored above the
// config containers rewrites each row it returns as a whole, so returning an
// existing parent would empty it.
func niBgpNbrParentRowIfMissing(inParams XfmrParams, res map[string]map[string]db.Value, niName, neighborAddr string) map[string]map[string]db.Value {
	if parent := niBgpCfgDbEntry(inParams, niBgpNbrTbl, niName, neighborAddr); parent.IsPopulated() {
		return res
	}
	return ensureNiBgpNbrParentRow(res, niName, neighborAddr)
}

// buildNiBgpNbrAfisResMap maps every afi-safi an afi-safis container payload
// carries, so that one request can create or update both families.
func buildNiBgpNbrAfisResMap(inParams XfmrParams, s *ygot.GoStruct, reqInfo *niBgpNbrReqInfo) (map[string]map[string]db.Value, error) {
	nbrObj := getNiBgpNbrObj(s, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if nbrObj == nil || nbrObj.AfiSafis == nil || len(nbrObj.AfiSafis.AfiSafi) == 0 {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor afi-safis payload does not name any afi-safi"}
	}

	rows := make(map[string]db.Value)
	for afiEnum, afObj := range nbrObj.AfiSafis.AfiSafi {
		afiSafi, ok := niBgpAfiSafiNameFromEnum(afiEnum)
		if !ok {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported afi-safi-name for network-instance BGP neighbor AFI-SAFI mapping: %v", afiEnum)}
		}
		fields, err := niBgpNbrAfInstFields(afObj)
		if err != nil {
			return nil, err
		}
		rows[reqInfo.niName+"|"+reqInfo.neighborAddr+"|"+afiSafi] = db.Value{Field: niBgpNbrAfInstWrite(inParams, reqInfo, afiSafi, fields)}
	}
	return niBgpNbrParentRowIfMissing(inParams, map[string]map[string]db.Value{niBgpNbrAfTbl: rows}, reqInfo.niName, reqInfo.neighborAddr), nil
}

// niBgpNbrInstDeleteResMap removes a neighbor together with the address-family
// rows that hang off it. Both tables go in one result map: translib orders
// BGP_NEIGHBOR_AF ahead of BGP_NEIGHBOR by table dependency, and CVL refuses to
// delete a neighbor while one of its address-family rows still exists.
func niBgpNbrInstDeleteResMap(niName, neighborAddr string) map[string]map[string]db.Value {
	res := makeNiBgpNbrAfResMap(niName, neighborAddr, nil)
	res[niBgpNbrTbl] = map[string]db.Value{niName + "|" + neighborAddr: {}}
	return res
}

// niBgpNbrHierDeleteDispatch maps a DELETE anchored on the neighbor list
// instance or on the neighbors container. Those two nodes are annotated to the
// BGP container dispatch rather than to this transformer, so the container
// dispatch hands the request here. It reports whether the request was one of
// its own.
func niBgpNbrHierDeleteDispatch(inParams XfmrParams) (bool, map[string]map[string]db.Value, error) {
	uri := inParams.requestUri
	if niBgpNbrInstKeyLeafReqUriRe.MatchString(uri) {
		// The address is the row identity, so there is no field to clear;
		// removing it means removing the neighbor.
		return true, nil, tlerr.NotSupportedError{
			Format: "DELETE is not supported for the neighbor-address key; delete the neighbor instead",
			Path:   uri,
		}
	}
	if m := niBgpNbrInstReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		if err := validateNiBgpProtoName(m[2]); err != nil {
			return true, nil, err
		}
		return true, niBgpNbrInstDeleteResMap(m[1], m[3]), nil
	}
	if niBgpNbrsReqUriRe.MatchString(uri) {
		return true, nil, tlerr.NotSupportedError{
			Format: "DELETE is not supported at the neighbors container level; delete a neighbor instead",
			Path:   uri,
		}
	}
	return false, nil, nil
}

// niBgpNbrDbAfiSafis lists the address families a neighbor holds a row for,
// which is what a read anchored on the afi-safis container has to enumerate.
func niBgpNbrDbAfiSafis(cfgDb *db.DB, niName, neighborAddr string) ([]string, error) {
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpNbrAfTbl})
	if err != nil {
		return nil, err
	}
	afiSafis := make([]string, 0, 2)
	for _, k := range keys {
		if len(k.Comp) == 3 && k.Comp[0] == niName && k.Comp[1] == neighborAddr {
			afiSafis = append(afiSafis, k.Comp[2])
		}
	}
	sort.Strings(afiSafis)
	return afiSafis, nil
}

// populateNiBgpNbrAfFromDb fills one afi-safi instance of a neighbor, which is
// everything a read at or above the instance reports for that family.
func populateNiBgpNbrAfFromDb(cfgDb *db.DB, ygRoot *ygot.GoStruct, niName, protoName, neighborAddr, afiSafi string) error {
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{niName, neighborAddr, afiSafi}})
	if err != nil {
		return err
	}
	afObj := getNiBgpNbrAfObj(ygRoot, niName, protoName, neighborAddr, afiSafi, true)
	if afObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor AFI-SAFI object in YANG tree"}
	}
	if afiSafiName, ok := niBgpNbrAfiSafiEnumFromName(afiSafi); ok {
		afObj.AfiSafiName = afiSafiName
		if afObj.Config != nil {
			afObj.Config.AfiSafiName = afiSafiName
		}
	}
	if afObj.Config != nil {
		if status, ok := entry.Field[niBgpNbrAdminStatus]; ok && strings.TrimSpace(status) != "" {
			if enabled, ok := niBgpNbrAdminStatusToEnabled(status); ok {
				afObj.Config.Enabled = enabled
			}
		}
		if comm, ok := niBgpSendCommunityTypeFromDb(entry.Field[niBgpNbrSendCommFld]); ok {
			afObj.Config.SendCommunityType = comm
		}
	}
	if afObj.ApplyPolicy != nil && afObj.ApplyPolicy.Config != nil {
		if policy := strings.TrimSpace(entry.Field[niBgpNbrRouteMapInFld]); policy != "" {
			afObj.ApplyPolicy.Config.ImportPolicy = []string{policy}
		}
		if policy := strings.TrimSpace(entry.Field[niBgpNbrRouteMapOutFld]); policy != "" {
			afObj.ApplyPolicy.Config.ExportPolicy = []string{policy}
		}
	}
	return nil
}

func getNiBgpNbrCfgObj(s *ygot.GoStruct, niName, protoName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config {
	nbrObj := getNiBgpNbrObj(s, niName, protoName, neighborAddr, createIfMissing)
	if nbrObj == nil {
		return nil
	}
	return nbrObj.Config
}

func getNiBgpNbrTimersCfgObj(s *ygot.GoStruct, niName, protoName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Timers_Config {
	nbrObj := getNiBgpNbrObj(s, niName, protoName, neighborAddr, createIfMissing)
	if nbrObj == nil || nbrObj.Timers == nil {
		return nil
	}
	return nbrObj.Timers.Config
}

func getNiBgpNbrTransportCfgObj(s *ygot.GoStruct, niName, protoName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Transport_Config {
	nbrObj := getNiBgpNbrObj(s, niName, protoName, neighborAddr, createIfMissing)
	if nbrObj == nil || nbrObj.Transport == nil {
		return nil
	}
	return nbrObj.Transport.Config
}

// parseNiBgpNbrTimerValue reads a timer out of a leaf targeted payload. The
// OpenConfig leaves are decimal64 while BGP_NEIGHBOR stores whole seconds, so
// only values that survive the narrowing are accepted.
func parseNiBgpNbrTimerValue(v interface{}) (*uint16, bool) {
	switch val := v.(type) {
	case uint16:
		u := val
		return &u, true
	case *uint16:
		if val != nil {
			return val, true
		}
	case uint32:
		if val <= 65535 {
			u := uint16(val)
			return &u, true
		}
	case int:
		if val >= 0 && val <= 65535 {
			u := uint16(val)
			return &u, true
		}
	case float64:
		if val >= 0 && val <= 65535 && val == float64(uint16(val)) {
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

func parseNiBgpNbrTimerFromParam(param interface{}) (*uint16, bool) {
	if param == nil {
		return nil, false
	}
	return parseNiBgpNbrTimerValue(param)
}

// niBgpNbrTimerField resolves a leaf targeted timers request to the
// BGP_NEIGHBOR field it writes and the value to write there.
func niBgpNbrTimerField(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (string, *uint16, error) {
	tmObj := getNiBgpNbrTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	var fld string
	var payloadVal *uint16
	switch reqInfo.targetLeaf {
	case niBgpNbrConnectRetryLeaf:
		fld = niBgpNbrConnRetryFld
		if tmObj != nil {
			payloadVal = tmObj.ConnectRetry
		}
	case niBgpNbrHoldTimeLeaf:
		fld = niBgpNbrHoldTimeFld
		if tmObj != nil {
			payloadVal = tmObj.HoldTime
		}
	case niBgpNbrKeepaliveLeaf:
		fld = niBgpNbrKeepaliveFld
		if tmObj != nil {
			payloadVal = tmObj.KeepaliveInterval
		}
	}
	if timer, ok := parseNiBgpNbrTimerFromParam(inParams.param); ok {
		return fld, timer, nil
	}
	if payloadVal == nil {
		return "", nil, tlerr.InvalidArgsError{Format: "BGP neighbor " + reqInfo.targetLeaf + " is missing in request payload"}
	}
	timer := *payloadVal
	return fld, &timer, nil
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

func parseNiBgpNbrLocalAddressFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if addr, ok := parseNiBgpNbrPeerGroupValue(param); ok {
		return addr, true
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
		fv := rv.FieldByName("LocalAddress")
		if fv.IsValid() && fv.CanInterface() {
			if addr, ok := parseNiBgpNbrPeerGroupValue(fv.Interface()); ok {
				return addr, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == niBgpNbrLocalAddrLeaf || strings.HasSuffix(k, ":"+niBgpNbrLocalAddrLeaf) {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if addr, ok := parseNiBgpNbrPeerGroupValue(mv.Interface()); ok {
						return addr, true
					}
				}
			}
		}
	}
	return "", false
}

func parseNiBgpNbrLocalAddress(inParams XfmrParams) (string, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return "", err
	}
	if addr, ok := parseNiBgpNbrLocalAddressFromParam(inParams.param); ok {
		return addr, nil
	}
	trObj := getNiBgpNbrTransportCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if trObj == nil || trObj.LocalAddress == nil || strings.TrimSpace(*trObj.LocalAddress) == "" {
		return "", tlerr.InvalidArgsError{Format: "BGP neighbor transport local-address is missing in request payload"}
	}
	return strings.TrimSpace(*trObj.LocalAddress), nil
}

func parseNiBgpNbrPassiveModeFromParam(param interface{}) (*bool, bool) {
	if param == nil {
		return nil, false
	}
	if passive, ok := parseNiBgpNbrEnabledValue(param); ok {
		return passive, true
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
		fv := rv.FieldByName("PassiveMode")
		if fv.IsValid() && fv.CanInterface() {
			if passive, ok := parseNiBgpNbrEnabledValue(fv.Interface()); ok {
				return passive, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == niBgpNbrPassiveModeLeaf || strings.HasSuffix(k, ":"+niBgpNbrPassiveModeLeaf) {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if passive, ok := parseNiBgpNbrEnabledValue(mv.Interface()); ok {
						return passive, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseNiBgpNbrPassiveMode(inParams XfmrParams) (*bool, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if passive, ok := parseNiBgpNbrPassiveModeFromParam(inParams.param); ok {
		return passive, nil
	}
	trObj := getNiBgpNbrTransportCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if trObj == nil || trObj.PassiveMode == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor transport passive-mode is missing in request payload"}
	}
	return trObj.PassiveMode, nil
}

func niBgpNbrTransportCfgFields(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (map[string]string, error) {
	fields := map[string]string{}
	switch reqInfo.targetLeaf {
	case niBgpNbrLocalAddrLeaf:
		addr, err := parseNiBgpNbrLocalAddress(inParams)
		if err != nil {
			return nil, err
		}
		fields[niBgpNbrLocalAddrFld] = addr
	case niBgpNbrPassiveModeLeaf:
		passive, err := parseNiBgpNbrPassiveMode(inParams)
		if err != nil {
			return nil, err
		}
		fields[niBgpNbrPassiveModeFld] = strconv.FormatBool(*passive)
	case "":
		if addr, ok := parseNiBgpNbrLocalAddressFromParam(inParams.param); ok {
			fields[niBgpNbrLocalAddrFld] = addr
		}
		if passive, ok := parseNiBgpNbrPassiveModeFromParam(inParams.param); ok {
			fields[niBgpNbrPassiveModeFld] = strconv.FormatBool(*passive)
		}
		if trObj := getNiBgpNbrTransportCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false); trObj != nil {
			if trObj.LocalAddress != nil && strings.TrimSpace(*trObj.LocalAddress) != "" {
				fields[niBgpNbrLocalAddrFld] = strings.TrimSpace(*trObj.LocalAddress)
			}
			if trObj.PassiveMode != nil {
				fields[niBgpNbrPassiveModeFld] = strconv.FormatBool(*trObj.PassiveMode)
			}
		}
		if len(fields) == 0 {
			return nil, tlerr.InvalidArgsError{Format: "BGP neighbor transport config payload carries neither local-address nor passive-mode"}
		}
	default:
		return nil, tlerr.NotSupportedError{Format: "unsupported leaf under neighbor transport config", Path: inParams.requestUri}
	}
	return fields, nil
}

func parseNiBgpNbrPeerGroup(inParams XfmrParams) (string, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return "", err
	}
	if pg, ok := parseNiBgpNbrPeerGroupFromParam(inParams.param); ok {
		return pg, nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.PeerGroup == nil || strings.TrimSpace(*cfgObj.PeerGroup) == "" {
		return "", tlerr.InvalidArgsError{Format: "BGP neighbor peer-group is missing in request payload"}
	}
	return strings.TrimSpace(*cfgObj.PeerGroup), nil
}

func parseNiBgpNbrAuthPasswordValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		// The value is a secret, so it is kept exactly as sent rather than
		// trimmed the way the other string leaves of this container are.
		if strings.TrimSpace(val) != "" {
			return val, true
		}
	case *string:
		if val != nil && strings.TrimSpace(*val) != "" {
			return *val, true
		}
	}
	return "", false
}

func parseNiBgpNbrAuthPasswordFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if pwd, ok := parseNiBgpNbrAuthPasswordValue(param); ok {
		return pwd, true
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
		fv := rv.FieldByName("AuthPassword")
		if fv.IsValid() && fv.CanInterface() {
			if pwd, ok := parseNiBgpNbrAuthPasswordValue(fv.Interface()); ok {
				return pwd, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "auth-password" || strings.HasSuffix(k, ":auth-password") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if pwd, ok := parseNiBgpNbrAuthPasswordValue(mv.Interface()); ok {
						return pwd, true
					}
				}
			}
		}
	}
	return "", false
}

func parseNiBgpNbrAuthPassword(inParams XfmrParams) (string, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return "", err
	}
	if pwd, ok := parseNiBgpNbrAuthPasswordFromParam(inParams.param); ok {
		return pwd, nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.AuthPassword == nil || strings.TrimSpace(*cfgObj.AuthPassword) == "" {
		return "", tlerr.InvalidArgsError{Format: "BGP neighbor auth-password is missing in request payload"}
	}
	return *cfgObj.AuthPassword, nil
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

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.Enabled == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor enabled is missing in request payload"}
	}
	return cfgObj.Enabled, nil
}

func parseNiBgpNbrAfEnabled(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (*bool, error) {
	if enabled, ok := parseNiBgpNbrEnabledFromParam(inParams.param); ok {
		return enabled, nil
	}

	cfgObj := getNiBgpNbrAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, false)
	if cfgObj == nil || cfgObj.Enabled == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor AFI-SAFI enabled is missing in request payload"}
	}
	return cfgObj.Enabled, nil
}

// niBgpNbrAfCfgFields collects the BGP_NEIGHBOR_AF fields for a neighbor AFI-SAFI
// config request. An AFI-SAFI list entry without an explicit "enabled" leaf still
// means the address-family is activated for the neighbor, so admin_status is seeded
// to up in that case; frrcfgd needs it to emit "neighbor <addr> activate".
func niBgpNbrAfCfgFields(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (map[string]string, error) {
	fields := make(map[string]string)
	switch reqInfo.targetLeaf {
	case niBgpNbrSendCommTypeLeaf:
		cfgObj := getNiBgpNbrAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, false)
		var fromTree []ocbinds.E_OpenconfigBgp_CommunityType
		if cfgObj != nil {
			fromTree = cfgObj.SendCommunityType
		}
		comm, err := niBgpSendCommunityTypeToField(inParams.param, fromTree, "neighbor")
		if err != nil {
			return nil, err
		}
		fields[niBgpNbrSendCommFld] = comm
	case "enabled":
		enabled, err := parseNiBgpNbrAfEnabled(inParams, reqInfo)
		if err != nil {
			return nil, err
		}
		fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
	case "afi-safi-name":
		fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(true)
	default:
		if enabled, ok := parseNiBgpNbrEnabledFromParam(inParams.param); ok {
			fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
			break
		}
		cfgObj := getNiBgpNbrAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, false)
		if cfgObj != nil && len(cfgObj.SendCommunityType) > 0 {
			comm, err := niBgpSendCommunityTypeToField(nil, cfgObj.SendCommunityType, "neighbor")
			if err != nil {
				return nil, err
			}
			fields[niBgpNbrSendCommFld] = comm
		}
		if cfgObj != nil && cfgObj.Enabled != nil {
			fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*cfgObj.Enabled)
			break
		}
		fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(true)
	}
	return fields, nil
}

func parseNiBgpNbrPeerAsFromParam(param interface{}) (*uint32, bool) {
	if param == nil {
		return nil, false
	}
	if asn, ok := parseNiBgpAsValue(param); ok {
		return asn, true
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
		fv := rv.FieldByName("PeerAs")
		if fv.IsValid() && fv.CanInterface() {
			if asn, ok := parseNiBgpAsValue(fv.Interface()); ok {
				return asn, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "peer-as" || strings.HasSuffix(k, ":peer-as") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if asn, ok := parseNiBgpAsValue(mv.Interface()); ok {
						return asn, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseNiBgpNbrPeerAs(inParams XfmrParams) (*uint32, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if asn, ok := parseNiBgpNbrPeerAsFromParam(inParams.param); ok {
		return asn, nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.PeerAs == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor peer-as is missing in request payload"}
	}
	return cfgObj.PeerAs, nil
}

func makeNiBgpNbrResMap(niName, neighborAddr string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpNbrTbl: {
			niName + "|" + neighborAddr: db.Value{Field: fields},
		},
	}
}

// The singular send-community leaf on the neighbor config container is
// deprecated in openconfig-bgp 9.4.0 and superseded by the per-address-family
// leaf-list. It is refused rather than served: it has no address family of its
// own, so it could only ever write both families at once and report whichever
// answered first, which stops being meaningful once the two can differ.
func niBgpNbrSendCommunityDeprecated(requestUri string) error {
	return tlerr.NotSupportedError{
		Format: "send-community on the neighbor config container is deprecated and not served; use afi-safis/afi-safi/config/send-community-type",
		Path:   requestUri,
	}
}

// send-community-type is per address family in the model and per address family
// in CONFIG_DB, in BGP_NEIGHBOR_AF and BGP_PEER_GROUP_AF alike. frrcfgd renders
// the field as "neighbor <addr> send-community <type>", so the DB spelling is
// the FRR one, "large" included: the model carried no LARGE until
// openconfig-bgp-types 5.5.0 came with the 9.6.0 uplift.
func niBgpCommunityTypeToDb(v ocbinds.E_OpenconfigBgp_CommunityType) (string, bool) {
	switch v {
	case ocbinds.OpenconfigBgp_CommunityType_STANDARD:
		return "standard", true
	case ocbinds.OpenconfigBgp_CommunityType_EXTENDED:
		return "extended", true
	case ocbinds.OpenconfigBgp_CommunityType_LARGE:
		return "large", true
	case ocbinds.OpenconfigBgp_CommunityType_BOTH:
		return "both", true
	case ocbinds.OpenconfigBgp_CommunityType_NONE:
		return "none", true
	}
	return "", false
}

func niBgpCommunityTypeFromDb(v string) (ocbinds.E_OpenconfigBgp_CommunityType, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "standard":
		return ocbinds.OpenconfigBgp_CommunityType_STANDARD, true
	case "extended":
		return ocbinds.OpenconfigBgp_CommunityType_EXTENDED, true
	case "large":
		return ocbinds.OpenconfigBgp_CommunityType_LARGE, true
	case "both":
		return ocbinds.OpenconfigBgp_CommunityType_BOTH, true
	case "none":
		return ocbinds.OpenconfigBgp_CommunityType_NONE, true
	}
	return ocbinds.OpenconfigBgp_CommunityType_UNSET, false
}

// A leaf-targeted request carries the value as a scalar rather than through the
// ygot tree, and a client may spell it with the module prefix the model uses.
func niBgpCommunityTypeNameToDb(v string) (string, bool) {
	s := strings.TrimSpace(v)
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[idx+1:]
	}
	switch strings.ToUpper(s) {
	case "STANDARD":
		return "standard", true
	case "EXTENDED":
		return "extended", true
	case "LARGE":
		return "large", true
	case "BOTH":
		return "both", true
	case "NONE":
		return "none", true
	}
	return "", false
}

// The one field CONFIG_DB holds cannot carry a list, so a request naming more
// than one community type is refused rather than honoured in part. HLD 2145
// would take the first element and drop the rest; a caller told its request
// succeeded has no way to learn that most of it did not.
func niBgpSendCommunityTypeToField(param interface{}, fromTree []ocbinds.E_OpenconfigBgp_CommunityType, scope string) (string, error) {
	unsupported := func(v string) error {
		return tlerr.InvalidArgsError{Format: "BGP " + scope + " send-community-type carries an unsupported value: " + v}
	}
	var vals []string
	appendName := func(s string) error {
		d, ok := niBgpCommunityTypeNameToDb(s)
		if !ok {
			return unsupported(s)
		}
		vals = append(vals, d)
		return nil
	}
	appendEnum := func(e ocbinds.E_OpenconfigBgp_CommunityType) error {
		d, ok := niBgpCommunityTypeToDb(e)
		if !ok {
			return unsupported(strconv.FormatInt(int64(e), 10))
		}
		vals = append(vals, d)
		return nil
	}
	switch v := param.(type) {
	case string:
		if err := appendName(v); err != nil {
			return "", err
		}
	case []string:
		for _, s := range v {
			if err := appendName(s); err != nil {
				return "", err
			}
		}
	case []interface{}:
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return "", tlerr.InvalidArgsError{Format: "BGP " + scope + " send-community-type carries a value of an unexpected type"}
			}
			if err := appendName(s); err != nil {
				return "", err
			}
		}
	case ocbinds.E_OpenconfigBgp_CommunityType:
		if err := appendEnum(v); err != nil {
			return "", err
		}
	case []ocbinds.E_OpenconfigBgp_CommunityType:
		for _, e := range v {
			if err := appendEnum(e); err != nil {
				return "", err
			}
		}
	}
	if len(vals) == 0 {
		for _, e := range fromTree {
			if err := appendEnum(e); err != nil {
				return "", err
			}
		}
	}
	switch {
	case len(vals) == 0:
		return "", tlerr.InvalidArgsError{Format: "BGP " + scope + " send-community-type is missing in request payload"}
	case len(vals) > 1:
		return "", tlerr.NotSupportedError{Format: "BGP " + scope + " send-community-type accepts one community type, since CONFIG_DB holds a single send_community per address family, and the request carries " + strconv.Itoa(len(vals))}
	}
	return vals[0], nil
}

func niBgpSendCommunityTypeFromDb(v string) ([]ocbinds.E_OpenconfigBgp_CommunityType, bool) {
	if t, ok := niBgpCommunityTypeFromDb(v); ok {
		return []ocbinds.E_OpenconfigBgp_CommunityType{t}, true
	}
	return nil, false
}

func makeNiBgpNbrAfResMap(niName, neighborAddr string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpNbrAfTbl: {
			niName + "|" + neighborAddr + "|" + niBgpNbrAfiSafiIPv4: db.Value{Field: fields},
			niName + "|" + neighborAddr + "|" + niBgpNbrAfiSafiIPv6: db.Value{Field: fields},
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

func ensureNiBgpNbrParentRow(res map[string]map[string]db.Value, niName, neighborAddr string) map[string]map[string]db.Value {
	if res == nil {
		res = make(map[string]map[string]db.Value)
	}
	if _, ok := res[niBgpNbrTbl]; !ok {
		res[niBgpNbrTbl] = make(map[string]db.Value)
	}
	// Ensure parent key exists for AF-table writes that have BGP_NEIGHBOR dependency.
	res[niBgpNbrTbl][niName+"|"+neighborAddr] = db.Value{Field: map[string]string{}}
	return res
}

// ygotField and payloadKey name the leaf being written, since param may arrive
// as the bare leaf-list, as the ygot config struct, or as the decoded payload
// map.
func parseNiBgpNbrApplyPolicyFromParam(param interface{}, ygotField, payloadKey string) []string {
	if param == nil {
		return nil
	}
	if names := niBgpPolicyNames(param); len(names) > 0 {
		return names
	}

	rv := reflect.ValueOf(param)
	for rv.IsValid() && (rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr) {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil
	}
	if rv.Kind() == reflect.Struct {
		fv := rv.FieldByName(ygotField)
		if fv.IsValid() && fv.CanInterface() {
			if names := niBgpPolicyNames(fv.Interface()); len(names) > 0 {
				return names
			}
		}
	}
	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == payloadKey || strings.HasSuffix(k, ":"+payloadKey) {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if names := niBgpPolicyNames(mv.Interface()); len(names) > 0 {
						return names
					}
				}
			}
		}
	}
	return nil
}

// niBgpNbrApplyPolicyFields renders the CONFIG_DB fields of an apply-policy
// request. A request naming one leaf carries that leaf's value in param, while
// a request at the container carries either leaf, or both, in the ygot tree.
func niBgpNbrApplyPolicyFields(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (map[string]string, error) {
	var importPolicies, exportPolicies []string
	if cfgObj := getNiBgpNbrAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, false); cfgObj != nil {
		if err := niBgpApplyPolicyDefaultsErr("neighbor", cfgObj.DefaultImportPolicy, cfgObj.DefaultExportPolicy); err != nil {
			return nil, err
		}
		importPolicies = cfgObj.ImportPolicy
		exportPolicies = cfgObj.ExportPolicy
	}

	// param is looked up by name before falling back to the ygot tree.
	policyOf := func(ygotField, leaf string, fromTree []string) (string, bool, error) {
		policy, ok, err := niBgpSinglePolicyName(parseNiBgpNbrApplyPolicyFromParam(inParams.param, ygotField, leaf), "neighbor", leaf)
		if err == nil && !ok {
			policy, ok, err = niBgpSinglePolicyName(fromTree, "neighbor", leaf)
		}
		return policy, ok, err
	}

	fields := map[string]string{}
	switch reqInfo.targetLeaf {
	case "import-policy":
		policy, ok, err := policyOf("ImportPolicy", "import-policy", importPolicies)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, tlerr.InvalidArgsError{Format: "BGP neighbor AFI-SAFI apply-policy import-policy is missing in request payload"}
		}
		fields[niBgpNbrRouteMapInFld] = policy
	case "export-policy":
		policy, ok, err := policyOf("ExportPolicy", "export-policy", exportPolicies)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, tlerr.InvalidArgsError{Format: "BGP neighbor AFI-SAFI apply-policy export-policy is missing in request payload"}
		}
		fields[niBgpNbrRouteMapOutFld] = policy
	case "":
		// The container itself, where param is the config struct or the decoded
		// payload map. Either names both leaves.
		policy, ok, err := policyOf("ImportPolicy", "import-policy", importPolicies)
		if err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpNbrRouteMapInFld] = policy
		}
		if policy, ok, err = policyOf("ExportPolicy", "export-policy", exportPolicies); err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpNbrRouteMapOutFld] = policy
		}
		if len(fields) == 0 && inParams.oper != REPLACE {
			return nil, tlerr.InvalidArgsError{Format: "BGP neighbor AFI-SAFI apply-policy config payload carries neither import-policy nor export-policy"}
		}
	case niBgpDefImportPolicyLeaf, niBgpDefExportPolicyLeaf:
		return nil, niBgpApplyPolicyDefaultLeafErr("neighbor", reqInfo.targetLeaf)
	default:
		return nil, tlerr.NotSupportedError{Format: "unsupported leaf under neighbor AFI-SAFI apply-policy config"}
	}
	return fields, nil
}

// niBgpNbrApplyPolicyWrite maps a write to apply-policy. A REPLACE of either
// container clears the direction its payload leaves out. A REPLACE runs as an
// update of the fields it names: the apply-policy container is not a terminal
// node, so translib would otherwise rewrite the whole row from what this
// returns.
func niBgpNbrApplyPolicyWrite(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (map[string]map[string]db.Value, error) {
	fields, err := niBgpNbrApplyPolicyFields(inParams, reqInfo)
	if err != nil {
		return nil, err
	}
	res := ensureNiBgpNbrParentRow(makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, fields), reqInfo.niName, reqInfo.neighborAddr)
	if inParams.oper != REPLACE {
		return res, nil
	}
	if reqInfo.targetLeaf == "" {
		existing := niBgpCfgDbEntry(inParams, niBgpNbrAfTbl, reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi)
		clear := map[string]string{}
		for _, fld := range []string{niBgpNbrRouteMapInFld, niBgpNbrRouteMapOutFld} {
			if _, ok := fields[fld]; !ok && existing.Has(fld) {
				clear[fld] = ""
			}
		}
		if err := niBgpStageSubOp(inParams, DELETE, makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, clear)); err != nil {
			return nil, err
		}
	}
	if len(fields) > 0 {
		if err := niBgpStageSubOp(inParams, UPDATE, res); err != nil {
			return nil, err
		}
	}
	return map[string]map[string]db.Value{}, nil
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

	asPathCfgObj := getNiBgpNbrAsPathOptionsCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
	if asPathCfgObj == nil || asPathCfgObj.AllowOwnAs == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor as-path-options allow-own-as is missing in request payload"}
	}
	return asPathCfgObj.AllowOwnAs, nil
}

var YangToDb_ni_bgp_nbr_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseNiBgpNbrRequestUri(niHandlerReqUri(inParams))
	if err != nil {
		fallback, nameErr := niBgpContainerLevelFallback(inParams.requestUri)
		if nameErr != nil {
			return res, nameErr
		}
		if fallback {
			// Container-level dispatch already mapped the whole payload.
			return res, nil
		}
		return res, err
	}

	if inParams.oper == DELETE {
		if reqInfo.targetPath == niBgpNbrTargetAfis {
			return makeNiBgpNbrAfResMap(reqInfo.niName, reqInfo.neighborAddr, nil), nil
		}
		if reqInfo.targetPath == niBgpNbrTargetAfInst {
			// The key leaf is the row identity, so clearing it means removing
			// the row the instance names.
			return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, nil), nil
		}
		if reqInfo.targetPath == "neighbor-af-apply-policy-config" {
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes both directions with it.
				return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{
					niBgpNbrRouteMapInFld:  "",
					niBgpNbrRouteMapOutFld: "",
				}), nil
			case "import-policy":
				return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{niBgpNbrRouteMapInFld: ""}), nil
			case "export-policy":
				return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{niBgpNbrRouteMapOutFld: ""}), nil
			case niBgpDefImportPolicyLeaf, niBgpDefExportPolicyLeaf:
				return res, niBgpApplyPolicyDefaultLeafErr("neighbor", reqInfo.targetLeaf)
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/afi-safis/afi-safi/apply-policy/config, /neighbors/neighbor/afi-safis/afi-safi/apply-policy/config/import-policy, or /neighbors/neighbor/afi-safis/afi-safi/apply-policy/config/export-policy", Path: inParams.requestUri}
			}
		}
		if reqInfo.targetPath == "neighbor-af-config" {
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes both fields it maps.
				return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{
					niBgpNbrAdminStatus: "",
					niBgpNbrSendCommFld: "",
				}), nil
			case "afi-safi-name", "enabled":
				return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{niBgpNbrAdminStatus: ""}), nil
			case niBgpNbrSendCommTypeLeaf:
				return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, map[string]string{niBgpNbrSendCommFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/afi-safis/afi-safi/config or its enabled and send-community-type leaves", Path: inParams.requestUri}
			}
		}
		if reqInfo.targetPath == niBgpNbrTargetTimersCfg {
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes all three timers with it.
				return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
					niBgpNbrConnRetryFld: "",
					niBgpNbrHoldTimeFld:  "",
					niBgpNbrKeepaliveFld: "",
				}), nil
			case niBgpNbrConnectRetryLeaf:
				return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrConnRetryFld: ""}), nil
			case niBgpNbrHoldTimeLeaf:
				return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrHoldTimeFld: ""}), nil
			case niBgpNbrKeepaliveLeaf:
				return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrKeepaliveFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/timers/config, its connect-retry leaf, its hold-time leaf, or its keepalive-interval leaf", Path: inParams.requestUri}
			}
		}
		if reqInfo.targetPath == niBgpNbrTargetTransportCfg {
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes both mapped transport leaves and
				// leaves the BGP_NEIGHBOR row in place. frrcfgd turns each
				// cleared field into the corresponding "no neighbor ..." command.
				return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
					niBgpNbrLocalAddrFld:   "",
					niBgpNbrPassiveModeFld: "",
				}), nil
			case niBgpNbrLocalAddrLeaf:
				return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrLocalAddrFld: ""}), nil
			case niBgpNbrPassiveModeLeaf:
				return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrPassiveModeFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/transport/config, its local-address leaf, or its passive-mode leaf", Path: inParams.requestUri}
			}
		}
		if reqInfo.targetPath == "neighbor-as-path-options-config" {
			switch reqInfo.targetLeaf {
			case "", "allow-own-as":
				return makeNiBgpNbrAfResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
					niBgpNbrAllowAsInFld:  "",
					niBgpNbrAllowAsCntFld: "",
				}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/as-path-options/config or /neighbors/neighbor/as-path-options/config/allow-own-as", Path: inParams.requestUri}
			}
		}
		switch reqInfo.targetLeaf {
		case "":
			// The container delete takes every leaf it maps with it. The list
			// instance itself survives, so the BGP_NEIGHBOR row is emptied
			// rather than removed; deleting the neighbor is what removes it.
			// Nothing here reaches BGP_NEIGHBOR_AF any more: send-community-type
			// belongs to the address family and is deleted there.
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
				niBgpNbrPeerGroupFld: "",
				niBgpNbrAsnFld:       "",
				niBgpNbrAdminStatus:  "",
				niBgpNbrAuthPwdFld:   "",
			}), nil
		case "peer-group":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrPeerGroupFld: ""}), nil
		case "peer-as":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrAsnFld: ""}), nil
		case "enabled":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrAdminStatus: ""}), nil
		case niBgpNbrSendCommunityLeaf:
			return res, niBgpNbrSendCommunityDeprecated(inParams.requestUri)
		case "auth-password":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrAuthPwdFld: ""}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/config, /neighbors/neighbor/config/peer-group, /neighbors/neighbor/config/peer-as, /neighbors/neighbor/config/enabled, /neighbors/neighbor/config/send-community, or /neighbors/neighbor/config/auth-password", Path: inParams.requestUri}
		}
	}

	if reqInfo.targetPath == niBgpNbrTargetAfis || reqInfo.targetPath == niBgpNbrTargetAfInst {
		// A payload key that disagrees with the one named in the path would
		// land on the row the path names and quietly ignore the payload.
		if reqInfo.targetLeaf == niBgpNbrAfiSafiNameLeaf {
			if name, ok := niBgpNbrPayloadAfiSafiName(inParams.param); ok && name != reqInfo.afiSafi {
				return res, tlerr.InvalidArgsError{
					Format: fmt.Sprintf("BGP neighbor afi-safi-name payload (%s) does not match key (%s)", name, reqInfo.afiSafi),
				}
			}
		}
		wrappedRoot, wrapErr := wrapNiBgpNbrRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return res, wrapErr
		}
		if inParams.invokeCRUSubtreeOnce != nil {
			// The whole subtree is mapped in this call, so translib must not
			// invoke this transformer again for the child nodes.
			*inParams.invokeCRUSubtreeOnce = true
		}
		if reqInfo.targetPath == niBgpNbrTargetAfis {
			return buildNiBgpNbrAfisResMap(inParams, wrappedRoot, reqInfo)
		}
		afObj := getNiBgpNbrAfObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, false)
		fields, err := niBgpNbrAfInstFields(afObj)
		if err != nil {
			return res, err
		}
		return niBgpNbrParentRowIfMissing(inParams,
			makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, niBgpNbrAfInstWrite(inParams, reqInfo, reqInfo.afiSafi, fields)),
			reqInfo.niName, reqInfo.neighborAddr), nil
	}

	if reqInfo.targetPath == "neighbor-af-apply-policy-config" {
		switch reqInfo.targetLeaf {
		case "", "import-policy", "export-policy":
			return niBgpNbrApplyPolicyWrite(inParams, reqInfo)
		case niBgpDefImportPolicyLeaf, niBgpDefExportPolicyLeaf:
			return res, niBgpApplyPolicyDefaultLeafErr("neighbor", reqInfo.targetLeaf)
		default:
			return res, tlerr.NotSupportedError{Format: "unsupported leaf under neighbor AFI-SAFI apply-policy config", Path: inParams.requestUri}
		}
	}

	if reqInfo.targetPath == "neighbor-af-config" {
		fields, err := niBgpNbrAfCfgFields(inParams, reqInfo)
		if err != nil {
			return res, err
		}
		return ensureNiBgpNbrParentRow(makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, fields), reqInfo.niName, reqInfo.neighborAddr), nil
	}

	if reqInfo.targetPath == niBgpNbrTargetTimersCfg {
		switch reqInfo.targetLeaf {
		case "":
			// A request at the timers config container can carry any of them.
			fields := map[string]string{}
			if tmObj := getNiBgpNbrTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false); tmObj != nil {
				if tmObj.ConnectRetry != nil {
					fields[niBgpNbrConnRetryFld] = strconv.FormatUint(uint64(*tmObj.ConnectRetry), 10)
				}
				if tmObj.HoldTime != nil {
					fields[niBgpNbrHoldTimeFld] = strconv.FormatUint(uint64(*tmObj.HoldTime), 10)
				}
				if tmObj.KeepaliveInterval != nil {
					fields[niBgpNbrKeepaliveFld] = strconv.FormatUint(uint64(*tmObj.KeepaliveInterval), 10)
				}
			}
			if len(fields) == 0 {
				return res, tlerr.InvalidArgsError{Format: "BGP neighbor timers config payload carries none of connect-retry, hold-time or keepalive-interval"}
			}
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, fields), nil
		case niBgpNbrConnectRetryLeaf, niBgpNbrHoldTimeLeaf, niBgpNbrKeepaliveLeaf:
			fld, timer, err := niBgpNbrTimerField(inParams, reqInfo)
			if err != nil {
				return res, err
			}
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{fld: strconv.FormatUint(uint64(*timer), 10)}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "unsupported leaf under neighbor timers config", Path: inParams.requestUri}
		}
	}

	if reqInfo.targetPath == niBgpNbrTargetTransportCfg {
		fields, err := niBgpNbrTransportCfgFields(inParams, reqInfo)
		if err != nil {
			return res, err
		}
		return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, fields), nil
	}

	if reqInfo.targetPath == "neighbor-as-path-options-config" {
		switch reqInfo.targetLeaf {
		case "", "allow-own-as":
			allowOwnAs, err := parseNiBgpNbrAllowOwnAs(inParams)
			if err != nil {
				return res, err
			}
			if *allowOwnAs == 0 {
				return ensureNiBgpNbrParentRow(makeNiBgpNbrAfResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
					niBgpNbrAllowAsInFld:  "",
					niBgpNbrAllowAsCntFld: "",
				}), reqInfo.niName, reqInfo.neighborAddr), nil
			}
			return ensureNiBgpNbrParentRow(makeNiBgpNbrAfResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{
				niBgpNbrAllowAsInFld:  "true",
				niBgpNbrAllowAsCntFld: strconv.FormatUint(uint64(*allowOwnAs), 10),
			}), reqInfo.niName, reqInfo.neighborAddr), nil
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
	case "peer-as":
		asn, err := parseNiBgpNbrPeerAs(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpNbrAsnFld] = strconv.FormatUint(uint64(*asn), 10)
	case "enabled":
		enabled, err := parseNiBgpNbrEnabled(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
	case niBgpNbrSendCommunityLeaf:
		return res, niBgpNbrSendCommunityDeprecated(inParams.requestUri)
	case "auth-password":
		pwd, err := parseNiBgpNbrAuthPassword(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpNbrAuthPwdFld] = pwd
	default:
		if pg, ok := parseNiBgpNbrPeerGroupFromParam(inParams.param); ok {
			fields[niBgpNbrPeerGroupFld] = pg
		}
		if asn, ok := parseNiBgpNbrPeerAsFromParam(inParams.param); ok {
			fields[niBgpNbrAsnFld] = strconv.FormatUint(uint64(*asn), 10)
		}
		if enabled, ok := parseNiBgpNbrEnabledFromParam(inParams.param); ok {
			fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
		}
		if pwd, ok := parseNiBgpNbrAuthPasswordFromParam(inParams.param); ok {
			fields[niBgpNbrAuthPwdFld] = pwd
		}
		if cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false); cfgObj != nil &&
			cfgObj.SendCommunity != ocbinds.OpenconfigBgp_CommunityType_UNSET {
			return res, niBgpNbrSendCommunityDeprecated(inParams.requestUri)
		}
		if len(fields) == 0 {
			cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, false)
			if cfgObj != nil {
				if cfgObj.PeerGroup != nil && strings.TrimSpace(*cfgObj.PeerGroup) != "" {
					fields[niBgpNbrPeerGroupFld] = strings.TrimSpace(*cfgObj.PeerGroup)
				}
				if cfgObj.PeerAs != nil {
					fields[niBgpNbrAsnFld] = strconv.FormatUint(uint64(*cfgObj.PeerAs), 10)
				}
				if cfgObj.Enabled != nil {
					fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*cfgObj.Enabled)
				}
				if cfgObj.AuthPassword != nil && strings.TrimSpace(*cfgObj.AuthPassword) != "" {
					fields[niBgpNbrAuthPwdFld] = *cfgObj.AuthPassword
				}
			}
		}
		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "BGP neighbor config payload missing supported fields (peer-group/peer-as/enabled/auth-password)"}
		}
	}
	return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, fields), nil
}

var DbToYang_ni_bgp_nbr_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		fallback, nameErr := niBgpContainerLevelFallback(inParams.requestUri)
		if nameErr != nil {
			return nameErr
		}
		if fallback {
			// Container-level dispatch already populated the whole subtree.
			return nil
		}
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP neighbor config"}
	}

	if reqInfo.targetPath == niBgpNbrTargetAfis || reqInfo.targetPath == niBgpNbrTargetAfInst {
		wrappedRoot, wrapErr := wrapNiBgpNbrRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return wrapErr
		}
		if reqInfo.targetPath == niBgpNbrTargetAfInst {
			return populateNiBgpNbrAfFromDb(cfgDb, wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi)
		}
		afiSafis, err := niBgpNbrDbAfiSafis(cfgDb, reqInfo.niName, reqInfo.neighborAddr)
		if err != nil {
			return err
		}
		for _, afiSafi := range afiSafis {
			if err := populateNiBgpNbrAfFromDb(cfgDb, wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, afiSafi); err != nil {
				return err
			}
		}
		return nil
	}

	if reqInfo.targetPath == "neighbor-af-config" {
		afEntry, afErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi}})
		if afErr != nil {
			return afErr
		}
		afCfgObj := getNiBgpNbrAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, true)
		if afCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor AFI-SAFI config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "afi-safi-name" {
			if afiSafiName, ok := niBgpNbrAfiSafiEnumFromName(reqInfo.afiSafi); ok {
				afCfgObj.AfiSafiName = afiSafiName
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "enabled" {
			if status, ok := afEntry.Field[niBgpNbrAdminStatus]; ok && strings.TrimSpace(status) != "" {
				if enabled, ok := niBgpNbrAdminStatusToEnabled(status); ok {
					afCfgObj.Enabled = enabled
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrSendCommTypeLeaf {
			if comm, ok := niBgpSendCommunityTypeFromDb(afEntry.Field[niBgpNbrSendCommFld]); ok {
				afCfgObj.SendCommunityType = comm
			}
		}
		return nil
	}

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr}})
	if err != nil {
		return err
	}

	if reqInfo.targetPath == niBgpNbrTargetTimersCfg {
		tmObj := getNiBgpNbrTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
		if tmObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor timers config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrConnectRetryLeaf {
			connRetry, ok := entry.Field[niBgpNbrConnRetryFld]
			if ok && strings.TrimSpace(connRetry) != "" {
				ival, err := strconv.ParseUint(strings.TrimSpace(connRetry), 10, 16)
				if err != nil {
					return err
				}
				iv := uint16(ival)
				tmObj.ConnectRetry = &iv
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrHoldTimeLeaf {
			hold, ok := entry.Field[niBgpNbrHoldTimeFld]
			if ok && strings.TrimSpace(hold) != "" {
				ival, err := strconv.ParseUint(strings.TrimSpace(hold), 10, 16)
				if err != nil {
					return err
				}
				iv := uint16(ival)
				tmObj.HoldTime = &iv
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrKeepaliveLeaf {
			keepalive, ok := entry.Field[niBgpNbrKeepaliveFld]
			if ok && strings.TrimSpace(keepalive) != "" {
				ival, err := strconv.ParseUint(strings.TrimSpace(keepalive), 10, 16)
				if err != nil {
					return err
				}
				iv := uint16(ival)
				tmObj.KeepaliveInterval = &iv
			}
		}
		return nil
	}

	if reqInfo.targetPath == niBgpNbrTargetTransportCfg {
		trObj := getNiBgpNbrTransportCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
		if trObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor transport config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrLocalAddrLeaf {
			if addr := strings.TrimSpace(entry.Field[niBgpNbrLocalAddrFld]); addr != "" {
				trObj.LocalAddress = &addr
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrPassiveModeLeaf {
			if raw := strings.TrimSpace(entry.Field[niBgpNbrPassiveModeFld]); raw != "" {
				passive, err := strconv.ParseBool(raw)
				if err != nil {
					return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR.passive_mode value %q for neighbor %s in %s", raw, reqInfo.neighborAddr, reqInfo.niName)}
				}
				trObj.PassiveMode = &passive
			}
		}
		return nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
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
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "peer-as" {
		asn, ok := entry.Field[niBgpNbrAsnFld]
		if ok && strings.TrimSpace(asn) != "" {
			asnU64, err := strconv.ParseUint(strings.TrimSpace(asn), 10, 32)
			if err != nil {
				return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR.asn value %q for neighbor %s in %s", asn, reqInfo.neighborAddr, reqInfo.niName)}
			}
			asnU32 := uint32(asnU64)
			cfgObj.PeerAs = &asnU32
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
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "auth-password" {
		pwd, ok := entry.Field[niBgpNbrAuthPwdFld]
		if ok && strings.TrimSpace(pwd) != "" {
			cfgObj.AuthPassword = &pwd
		}
	}
	if reqInfo.targetPath == "neighbor-af-apply-policy-config" {
		afEntry, afErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi}})
		if afErr != nil {
			return afErr
		}
		applyCfgObj := getNiBgpNbrAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, true)
		if applyCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor AFI-SAFI apply-policy config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "import-policy" {
			importPolicy, ok := afEntry.Field[niBgpNbrRouteMapInFld]
			if ok && strings.TrimSpace(importPolicy) != "" {
				applyCfgObj.ImportPolicy = []string{strings.TrimSpace(importPolicy)}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "export-policy" {
			exportPolicy, ok := afEntry.Field[niBgpNbrRouteMapOutFld]
			if ok && strings.TrimSpace(exportPolicy) != "" {
				applyCfgObj.ExportPolicy = []string{strings.TrimSpace(exportPolicy)}
			}
		}
		return nil
	}
	if reqInfo.targetPath == "neighbor-as-path-options-config" {
		v4Entry, errV4 := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, niBgpNbrAfiSafiIPv4}})
		v6Entry, errV6 := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, niBgpNbrAfiSafiIPv6}})
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

		asPathCfgObj := getNiBgpNbrAsPathOptionsCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
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
