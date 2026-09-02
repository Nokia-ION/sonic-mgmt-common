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
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niBgpPeerGroupTbl        = "BGP_PEER_GROUP"
	niBgpPeerGroupAfTbl      = "BGP_PEER_GROUP_AF"
	niBgpPgTargetCfg         = "peer-group-config"
	niBgpPgTargetTimersCfg   = "peer-group-timers-config"
	niBgpPgTargetAfPfxCfg    = "peer-group-af-prefix-limit-config"
	niBgpPgTargetAfApplyCfg  = "peer-group-af-apply-policy-config"
	niBgpPgPeerGroupNameLeaf = "peer-group-name"
	niBgpPgPeerAsLeaf        = "peer-as"
	niBgpPgKeepaliveLeaf     = "keepalive-interval"
	niBgpPgHoldTimeLeaf      = "hold-time"
	niBgpPgMaxPrefixesLeaf   = "max-prefixes"
	niBgpPgExportPolicyLeaf  = "export-policy"
	niBgpPgAsnFld            = "asn"
	niBgpPgKeepaliveFld      = "keepalive"
	niBgpPgHoldTimeFld       = "holdtime"
	niBgpPgMaxPfxLimitFld    = "max_prefix_limit"
	niBgpPgRouteMapOutFld    = "route_map_out@"
	niBgpPgAfiSafiIPv4       = "ipv4_unicast"
	niBgpPgAfiSafiIPv6       = "ipv6_unicast"
)

var niBgpPgCfgReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/peer-groups/peer-group\[peer-group-name=([^\]]+)\]/config(?:/(peer-group-name|peer-as))?$`)
var niBgpPgTimersReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/peer-groups/peer-group\[peer-group-name=([^\]]+)\]/timers/config(?:/(keepalive-interval|hold-time))?$`)
var niBgpPgAfV4PfxReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/peer-groups/peer-group\[peer-group-name=([^\]]+)\]/afi-safis/afi-safi(?:\[afi-safi-name=(?:IPV4_UNICAST|ipv4-unicast)\])?/ipv4-unicast/prefix-limit/config(?:/(max-prefixes))?$`)
var niBgpPgAfV6PfxReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/peer-groups/peer-group\[peer-group-name=([^\]]+)\]/afi-safis/afi-safi(?:\[afi-safi-name=(?:IPV6_UNICAST|ipv6-unicast)\])?/ipv6-unicast/prefix-limit/config(?:/(max-prefixes))?$`)
var niBgpPgAfApplyReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/peer-groups/peer-group\[peer-group-name=([^\]]+)\]/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/apply-policy/config(?:/(export-policy))?$`)

type niBgpPgReqInfo struct {
	niName        string
	peerGroupName string
	afiSafi       string
	targetLeaf    string // "", "peer-group-name", "peer-as"
	targetPath    string
}

func init() {
	XlateFuncBind("YangToDb_ni_bgp_pg_cfg_xfmr", YangToDb_ni_bgp_pg_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_bgp_pg_cfg_xfmr", DbToYang_ni_bgp_pg_cfg_xfmr)
}

func parseNiBgpPgRequestUri(uri string) (*niBgpPgReqInfo, error) {
	m := niBgpPgCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			peerGroupName: m[2],
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetCfg,
		}, nil
	}
	m = niBgpPgTimersReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			peerGroupName: m[2],
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetTimersCfg,
		}, nil
	}
	m = niBgpPgAfV4PfxReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			peerGroupName: m[2],
			afiSafi:       niBgpPgAfiSafiIPv4,
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetAfPfxCfg,
		}, nil
	}
	m = niBgpPgAfV6PfxReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			peerGroupName: m[2],
			afiSafi:       niBgpPgAfiSafiIPv6,
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetAfPfxCfg,
		}, nil
	}
	m = niBgpPgAfApplyReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		afiSafi, ok := normalizeNiBgpPgAfiSafiName(m[3])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP peer-group apply-policy mapping: " + m[3],
			}
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			peerGroupName: m[2],
			afiSafi:       afiSafi,
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetAfApplyCfg,
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP peer-group config mapping: " + uri,
	}
}

func normalizeNiBgpPgAfiSafiName(v string) (string, bool) {
	s := strings.TrimSpace(v)
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.ToLower(strings.ReplaceAll(s, "-", "_"))
	switch s {
	case niBgpPgAfiSafiIPv4:
		return niBgpPgAfiSafiIPv4, true
	case niBgpPgAfiSafiIPv6:
		return niBgpPgAfiSafiIPv6, true
	default:
		return "", false
	}
}

// niBgpPgAfiSafiUriName renders a family back in the form the OpenConfig path
// uses, so an error message names the path the client asked for.
func niBgpPgAfiSafiUriName(afiSafi string) string {
	if afiSafi == niBgpPgAfiSafiIPv6 {
		return "ipv6-unicast"
	}
	return "ipv4-unicast"
}

func getNiBgpPeerGroupCfgObj(s *ygot.GoStruct, niName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_Config {
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

	pkey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	pObj, ok := niObj.Protocols.Protocol[pkey]
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
	buildNiBgpEmptySubtree(pObj)

	if pgObj, ok := pObj.Bgp.PeerGroups.PeerGroup[peerGroup]; ok && pgObj != nil {
		buildNiBgpEmptySubtree(pgObj)
		return pgObj.Config
	}
	if !createIfMissing {
		return nil
	}
	pgObj, err := pObj.Bgp.PeerGroups.NewPeerGroup(peerGroup)
	if err != nil || pgObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pgObj)
	return pgObj.Config
}

func getNiBgpPeerGroupTimersCfgObj(s *ygot.GoStruct, niName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_Timers_Config {
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

	pkey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	pObj, ok := niObj.Protocols.Protocol[pkey]
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
	buildNiBgpEmptySubtree(pObj)

	if pgObj, ok := pObj.Bgp.PeerGroups.PeerGroup[peerGroup]; ok && pgObj != nil {
		buildNiBgpEmptySubtree(pgObj)
		return pgObj.Timers.Config
	}
	if !createIfMissing {
		return nil
	}
	pgObj, err := pObj.Bgp.PeerGroups.NewPeerGroup(peerGroup)
	if err != nil || pgObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(pgObj)
	return pgObj.Timers.Config
}

func getNiBgpPeerGroupAfObj(s *ygot.GoStruct, niName, peerGroup string, afiKey ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi {
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

	pkey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	pObj, ok := niObj.Protocols.Protocol[pkey]
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
	buildNiBgpEmptySubtree(pObj)

	var pgObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup
	pgObj, ok = pObj.Bgp.PeerGroups.PeerGroup[peerGroup]
	if !ok || pgObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pgObj, err = pObj.Bgp.PeerGroups.NewPeerGroup(peerGroup)
		if err != nil || pgObj == nil {
			return nil
		}
	}
	buildNiBgpEmptySubtree(pgObj)

	var afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi
	afObj, ok = pgObj.AfiSafis.AfiSafi[afiKey]
	if !ok || afObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		afObj, err = pgObj.AfiSafis.NewAfiSafi(afiKey)
		if err != nil || afObj == nil {
			return nil
		}
	}
	buildNiBgpEmptySubtree(afObj)
	return afObj
}

func getNiBgpPeerGroupAfV4PrefixLimitCfgObj(s *ygot.GoStruct, niName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_Ipv4Unicast_PrefixLimit_Config {
	afObj := getNiBgpPeerGroupAfObj(s, niName, peerGroup, ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST, createIfMissing)
	if afObj == nil {
		return nil
	}
	return afObj.Ipv4Unicast.PrefixLimit.Config
}

func getNiBgpPeerGroupAfV6PrefixLimitCfgObj(s *ygot.GoStruct, niName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_Ipv6Unicast_PrefixLimit_Config {
	afObj := getNiBgpPeerGroupAfObj(s, niName, peerGroup, ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV6_UNICAST, createIfMissing)
	if afObj == nil {
		return nil
	}
	return afObj.Ipv6Unicast.PrefixLimit.Config
}

// getNiBgpPgMaxPrefixesFromYgot reads max-prefixes out of the family specific
// prefix-limit container the request named. The two families carry the leaf on
// separate ygot types, so the family has to pick the object before the leaf can
// be read.
func getNiBgpPgMaxPrefixesFromYgot(s *ygot.GoStruct, niName, peerGroup, afiSafi string) (*uint32, bool) {
	if afiSafi == niBgpPgAfiSafiIPv6 {
		pfxObj := getNiBgpPeerGroupAfV6PrefixLimitCfgObj(s, niName, peerGroup, false)
		if pfxObj != nil && pfxObj.MaxPrefixes != nil {
			return pfxObj.MaxPrefixes, true
		}
		return nil, false
	}
	pfxObj := getNiBgpPeerGroupAfV4PrefixLimitCfgObj(s, niName, peerGroup, false)
	if pfxObj != nil && pfxObj.MaxPrefixes != nil {
		return pfxObj.MaxPrefixes, true
	}
	return nil, false
}

func getNiBgpPeerGroupAfApplyPolicyCfgObj(s *ygot.GoStruct, niName, peerGroup, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_ApplyPolicy_Config {
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

	pkey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	pObj, ok := niObj.Protocols.Protocol[pkey]
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
	buildNiBgpEmptySubtree(pObj)

	var pgObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup
	pgObj, ok = pObj.Bgp.PeerGroups.PeerGroup[peerGroup]
	if !ok || pgObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pgObj, err = pObj.Bgp.PeerGroups.NewPeerGroup(peerGroup)
		if err != nil || pgObj == nil {
			return nil
		}
	}
	buildNiBgpEmptySubtree(pgObj)

	afiKey := ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE(0)
	switch afiSafi {
	case niBgpPgAfiSafiIPv4:
		afiKey = ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST
	case niBgpPgAfiSafiIPv6:
		afiKey = ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV6_UNICAST
	default:
		return nil
	}

	var afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi
	afObj, ok = pgObj.AfiSafis.AfiSafi[afiKey]
	if !ok || afObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		afObj, err = pgObj.AfiSafis.NewAfiSafi(afiKey)
		if err != nil || afObj == nil {
			return nil
		}
	}
	buildNiBgpEmptySubtree(afObj)
	return afObj.ApplyPolicy.Config
}

func makeNiBgpPgResMap(niName, pgName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpPeerGroupTbl: {
			niName + "|" + pgName: db.Value{Field: fields},
		},
	}
}

func makeNiBgpPgAfResMap(niName, pgName, afiSafi string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpPeerGroupAfTbl: {
			niName + "|" + pgName + "|" + afiSafi: db.Value{Field: fields},
		},
	}
}

func parseNiBgpPgNameFromParam(param interface{}) (string, bool) {
	switch v := param.(type) {
	case string:
		s := strings.TrimSpace(v)
		return s, s != ""
	case *string:
		if v != nil {
			s := strings.TrimSpace(*v)
			return s, s != ""
		}
	}
	return "", false
}

func parseNiBgpPgAsnFromParam(param interface{}) (*uint32, bool) {
	if param == nil {
		return nil, false
	}
	if as, ok := parseNiBgpAsValue(param); ok {
		return as, true
	}
	return nil, false
}

func parseNiBgpPgIntervalValue(v interface{}) (*uint16, bool) {
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

func parseNiBgpPgKeepaliveFromParam(param interface{}) (*uint16, bool) {
	if param == nil {
		return nil, false
	}
	if iv, ok := parseNiBgpPgIntervalValue(param); ok {
		return iv, true
	}
	return nil, false
}

func parseNiBgpPgHoldTimeFromParam(param interface{}) (*uint16, bool) {
	if param == nil {
		return nil, false
	}
	if iv, ok := parseNiBgpPgIntervalValue(param); ok {
		return iv, true
	}
	return nil, false
}

func parseNiBgpPgMaxPrefixesFromParam(param interface{}) (*uint32, bool) {
	if param == nil {
		return nil, false
	}
	switch val := param.(type) {
	case uint32:
		v := val
		return &v, true
	case *uint32:
		if val != nil {
			return val, true
		}
	case int:
		if val >= 0 {
			v := uint32(val)
			return &v, true
		}
	case float64:
		if val >= 0 && val <= float64(^uint32(0)) && val == float64(uint32(val)) {
			v := uint32(val)
			return &v, true
		}
	case string:
		if u64, err := strconv.ParseUint(strings.TrimSpace(val), 10, 32); err == nil {
			v := uint32(u64)
			return &v, true
		}
	}
	return nil, false
}

func parseNiBgpPgExportPolicyFromParam(param interface{}) (string, bool) {
	switch val := param.(type) {
	case string:
		v := strings.TrimSpace(val)
		return v, v != ""
	case *string:
		if val != nil {
			v := strings.TrimSpace(*val)
			return v, v != ""
		}
	case []string:
		for _, item := range val {
			v := strings.TrimSpace(item)
			if v != "" {
				return v, true
			}
		}
	case []interface{}:
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}
			v := strings.TrimSpace(s)
			if v != "" {
				return v, true
			}
		}
	}
	return "", false
}

var YangToDb_ni_bgp_pg_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseNiBgpPgRequestUri(inParams.requestUri)
	if err != nil {
		if isNiBgpContainerLevelReqUri(inParams.requestUri) {
			// Container-level dispatch already mapped the whole payload.
			return res, nil
		}
		return res, err
	}

	if inParams.oper == DELETE {
		switch reqInfo.targetLeaf {
		case niBgpPgPeerGroupNameLeaf:
			// Key leaf delete implies peer-group node deletion.
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, nil), nil
		case niBgpPgPeerAsLeaf:
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgAsnFld: ""}), nil
		case niBgpPgKeepaliveLeaf:
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgKeepaliveFld: ""}), nil
		case niBgpPgHoldTimeLeaf:
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgHoldTimeFld: ""}), nil
		case niBgpPgMaxPrefixesLeaf:
			return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{niBgpPgMaxPfxLimitFld: ""}), nil
		case niBgpPgExportPolicyLeaf:
			return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{niBgpPgRouteMapOutFld: ""}), nil
		}
		return res, tlerr.NotSupportedError{Format: "DELETE supported only for /peer-groups/peer-group/config/peer-group-name, /peer-groups/peer-group/config/peer-as, /peer-groups/peer-group/timers/config/keepalive-interval, /peer-groups/peer-group/timers/config/hold-time, /peer-groups/peer-group/afi-safis/afi-safi/ipv4-unicast/prefix-limit/config/max-prefixes, /peer-groups/peer-group/afi-safis/afi-safi/ipv6-unicast/prefix-limit/config/max-prefixes, or /peer-groups/peer-group/afi-safis/afi-safi/apply-policy/config/export-policy", Path: inParams.requestUri}
	}

	if reqInfo.targetPath == niBgpPgTargetCfg {
		switch reqInfo.targetLeaf {
		case "":
			// A request at the config container itself can carry any mapped
			// leaf, so map each one the payload actually sets.
			fields := map[string]string{}
			if cfgObj := getNiBgpPeerGroupCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, false); cfgObj != nil {
				if cfgObj.PeerGroupName != nil {
					if pg := strings.TrimSpace(*cfgObj.PeerGroupName); pg != "" && pg != reqInfo.peerGroupName {
						return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("peer-group-name payload (%s) does not match key (%s)", pg, reqInfo.peerGroupName)}
					}
				}
				if cfgObj.PeerAs != nil {
					fields[niBgpPgAsnFld] = strconv.FormatUint(uint64(*cfgObj.PeerAs), 10)
				}
			}
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, fields), nil
		case niBgpPgPeerGroupNameLeaf:
			if pg, ok := parseNiBgpPgNameFromParam(inParams.param); ok && pg != reqInfo.peerGroupName {
				return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("peer-group-name payload (%s) does not match key (%s)", pg, reqInfo.peerGroupName)}
			}
			// peer-group-name is the list key in BGP_PEER_GROUP and should not be
			// written as a non-key field ("peer_group_name"), which CVL rejects.
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{}), nil
		case niBgpPgPeerAsLeaf:
			asn, ok := parseNiBgpPgAsnFromParam(inParams.param)
			if !ok {
				cfgObj := getNiBgpPeerGroupCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, false)
				if cfgObj == nil || cfgObj.PeerAs == nil {
					return res, tlerr.InvalidArgsError{Format: "BGP peer-group peer-as is missing in request payload"}
				}
				asn = cfgObj.PeerAs
			}
			return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgAsnFld: strconv.FormatUint(uint64(*asn), 10)}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "unsupported leaf under peer-group config", Path: inParams.requestUri}
		}
	}
	if reqInfo.targetPath == niBgpPgTargetAfPfxCfg {
		switch reqInfo.targetLeaf {
		case "", niBgpPgMaxPrefixesLeaf:
			max, ok := parseNiBgpPgMaxPrefixesFromParam(inParams.param)
			if !ok {
				max, ok = getNiBgpPgMaxPrefixesFromYgot(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi)
				if !ok {
					return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP peer-group %s prefix-limit max-prefixes is missing in request payload", niBgpPgAfiSafiUriName(reqInfo.afiSafi))}
				}
			}
			return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{
				niBgpPgMaxPfxLimitFld: strconv.FormatUint(uint64(*max), 10),
			}), nil
		default:
			return res, tlerr.NotSupportedError{Format: fmt.Sprintf("unsupported leaf under peer-group %s prefix-limit config", niBgpPgAfiSafiUriName(reqInfo.afiSafi)), Path: inParams.requestUri}
		}
	}
	if reqInfo.targetPath == niBgpPgTargetAfApplyCfg {
		switch reqInfo.targetLeaf {
		case "", niBgpPgExportPolicyLeaf:
			exportPolicy, ok := parseNiBgpPgExportPolicyFromParam(inParams.param)
			if !ok {
				applyCfgObj := getNiBgpPeerGroupAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, false)
				if applyCfgObj == nil || len(applyCfgObj.ExportPolicy) == 0 {
					return res, tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI apply-policy export-policy is missing in request payload"}
				}
				exportPolicy = strings.TrimSpace(applyCfgObj.ExportPolicy[0])
				if exportPolicy == "" {
					return res, tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI apply-policy export-policy is missing in request payload"}
				}
			}
			return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{
				niBgpPgRouteMapOutFld: exportPolicy,
			}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "unsupported leaf under peer-group AFI-SAFI apply-policy config", Path: inParams.requestUri}
		}
	}

	switch reqInfo.targetLeaf {
	case "":
		// A request at the timers config container can carry either timer.
		fields := map[string]string{}
		if tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, false); tmObj != nil {
			if tmObj.KeepaliveInterval != nil {
				fields[niBgpPgKeepaliveFld] = strconv.FormatUint(uint64(uint16(*tmObj.KeepaliveInterval)), 10)
			}
			if tmObj.HoldTime != nil {
				fields[niBgpPgHoldTimeFld] = strconv.FormatUint(uint64(uint16(*tmObj.HoldTime)), 10)
			}
		}
		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "BGP peer-group timers config payload carries neither hold-time nor keepalive-interval"}
		}
		return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, fields), nil
	case niBgpPgKeepaliveLeaf:
		keepalive, ok := parseNiBgpPgKeepaliveFromParam(inParams.param)
		if !ok {
			tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, false)
			if tmObj == nil || tmObj.KeepaliveInterval == nil {
				return res, tlerr.InvalidArgsError{Format: "BGP peer-group keepalive-interval is missing in request payload"}
			}
			iv := uint16(*tmObj.KeepaliveInterval)
			keepalive = &iv
		}
		return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgKeepaliveFld: strconv.FormatUint(uint64(*keepalive), 10)}), nil
	case niBgpPgHoldTimeLeaf:
		hold, ok := parseNiBgpPgHoldTimeFromParam(inParams.param)
		if !ok {
			tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, false)
			if tmObj == nil || tmObj.HoldTime == nil {
				return res, tlerr.InvalidArgsError{Format: "BGP peer-group hold-time is missing in request payload"}
			}
			iv := uint16(*tmObj.HoldTime)
			hold = &iv
		}
		return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgHoldTimeFld: strconv.FormatUint(uint64(*hold), 10)}), nil
	default:
		return res, tlerr.NotSupportedError{Format: "unsupported leaf under peer-group timers config", Path: inParams.requestUri}
	}
}

var DbToYang_ni_bgp_pg_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpPgRequestUri(inParams.requestUri)
	if err != nil {
		if isNiBgpContainerLevelReqUri(inParams.requestUri) {
			// Container-level dispatch already populated the whole subtree.
			return nil
		}
		return err
	}
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP peer-group config"}
	}

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.peerGroupName}})
	if err != nil {
		return err
	}
	if reqInfo.targetPath == niBgpPgTargetAfPfxCfg {
		afEntry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi}})
		if err != nil {
			return err
		}
		var maxPrefixes *uint32
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgMaxPrefixesLeaf {
			maxStr, ok := afEntry.Field[niBgpPgMaxPfxLimitFld]
			if ok && strings.TrimSpace(maxStr) != "" {
				maxU64, err := strconv.ParseUint(strings.TrimSpace(maxStr), 10, 32)
				if err != nil {
					return err
				}
				max := uint32(maxU64)
				maxPrefixes = &max
			}
		}
		if reqInfo.afiSafi == niBgpPgAfiSafiIPv6 {
			pfxObj := getNiBgpPeerGroupAfV6PrefixLimitCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, true)
			if pfxObj == nil {
				return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI prefix-limit config object in YANG tree"}
			}
			pfxObj.MaxPrefixes = maxPrefixes
			return nil
		}
		pfxObj := getNiBgpPeerGroupAfV4PrefixLimitCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, true)
		if pfxObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI prefix-limit config object in YANG tree"}
		}
		pfxObj.MaxPrefixes = maxPrefixes
		return nil
	}
	if reqInfo.targetPath == niBgpPgTargetAfApplyCfg {
		afEntry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi}})
		if err != nil {
			return err
		}
		applyCfgObj := getNiBgpPeerGroupAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, true)
		if applyCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI apply-policy config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgExportPolicyLeaf {
			exportPolicy, ok := afEntry.Field[niBgpPgRouteMapOutFld]
			if ok && strings.TrimSpace(exportPolicy) != "" {
				applyCfgObj.ExportPolicy = []string{strings.TrimSpace(exportPolicy)}
			}
		}
		return nil
	}
	if reqInfo.targetPath == niBgpPgTargetCfg {
		cfgObj := getNiBgpPeerGroupCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgPeerGroupNameLeaf {
			pg := reqInfo.peerGroupName
			cfgObj.PeerGroupName = &pg
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgPeerAsLeaf {
			asn, ok := entry.Field[niBgpPgAsnFld]
			if ok && strings.TrimSpace(asn) != "" {
				asnU64, err := strconv.ParseUint(strings.TrimSpace(asn), 10, 32)
				if err != nil {
					return err
				}
				asnU32 := uint32(asnU64)
				cfgObj.PeerAs = &asnU32
			}
		}
		return nil
	}

	tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.peerGroupName, true)
	if tmObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group timers config object in YANG tree"}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgKeepaliveLeaf {
		keepalive, ok := entry.Field[niBgpPgKeepaliveFld]
		if ok && strings.TrimSpace(keepalive) != "" {
			ival, err := strconv.ParseUint(strings.TrimSpace(keepalive), 10, 16)
			if err != nil {
				return err
			}
			iv := float64(uint16(ival))
			tmObj.KeepaliveInterval = &iv
		}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgHoldTimeLeaf {
		hold, ok := entry.Field[niBgpPgHoldTimeFld]
		if ok && strings.TrimSpace(hold) != "" {
			ival, err := strconv.ParseUint(strings.TrimSpace(hold), 10, 16)
			if err != nil {
				return err
			}
			iv := float64(uint16(ival))
			tmObj.HoldTime = &iv
		}
	}
	return nil
}
