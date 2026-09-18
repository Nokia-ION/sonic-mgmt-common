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
	"sort"
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
	niBgpPgImportPolicyLeaf  = "import-policy"
	niBgpPgAsnFld            = "asn"
	niBgpPgKeepaliveFld      = "keepalive"
	niBgpPgHoldTimeFld       = "holdtime"
	niBgpPgMaxPfxLimitFld    = "max_prefix_limit"
	niBgpPgRouteMapOutFld    = "route_map_out@"
	niBgpPgRouteMapInFld     = "route_map_in@"
	niBgpPgAdminStatusFld    = "admin_status"
	niBgpPgAfiSafiIPv4       = "ipv4_unicast"
	niBgpPgAfiSafiIPv6       = "ipv6_unicast"
	// The levels of the peer-group hierarchy that sit above the config
	// containers. Each resolves to a target kind the write and read paths
	// switch on, the same shape the neighbor mapping uses.
	niBgpPgTargetAfis      = "peer-group-afi-safis"
	niBgpPgTargetAfInst    = "peer-group-afi-safi-instance"
	niBgpPgTargetAfCfg     = "peer-group-af-config"
	niBgpPgAfiSafiNameLeaf = "afi-safi-name"
	niBgpPgEnabledLeaf     = "enabled"
	// Held per address family, the same field the neighbor AF rows carry.
	niBgpPgSendCommTypeLeaf = "send-community-type"
	niBgpPgSendCommFld      = "send_community"
)

// niBgpPgUriPfx names a peer-group list instance, which every path below it
// repeats. The levels above the config containers are matched against it.
const niBgpPgUriPfx = niBgpProtoUriPfx + `/bgp/peer-groups/peer-group\[peer-group-name=([^\]]+)\]`

var niBgpPgCfgReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/config(?:/(peer-group-name|peer-as))?$`)
var niBgpPgTimersReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/timers/config(?:/(keepalive-interval|hold-time))?$`)
var niBgpPgAfV4PfxReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis/afi-safi(?:\[afi-safi-name=(?:IPV4_UNICAST|ipv4-unicast)\])?/ipv4-unicast/prefix-limit/config(?:/(max-prefixes))?$`)
var niBgpPgAfV6PfxReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis/afi-safi(?:\[afi-safi-name=(?:IPV6_UNICAST|ipv6-unicast)\])?/ipv6-unicast/prefix-limit/config(?:/(max-prefixes))?$`)
var niBgpPgAfApplyReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/apply-policy/config(?:/(export-policy|import-policy))?$`)

var niBgpPgAfCfgReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/config(?:/(afi-safi-name|enabled|send-community-type))?$`)

// The afi-safi key leaf below the list instance repeats a key already in the
// URI, so it resolves to the same row as the instance itself.
var niBgpPgAfInstKeyLeafReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/afi-safi-name$`)
var niBgpPgAfInstReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]$`)
var niBgpPgAfisReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis$`)

// The peer-group list instance and the peer-groups container are annotated to
// the BGP container dispatch rather than to this transformer, so these two are
// matched by the delete dispatch it calls rather than by the request parser.
var niBgpPgInstKeyLeafReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/peer-group-name$`)
var niBgpPgInstReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `$`)
var niBgpPgsReqUriRe = regexp.MustCompile(niBgpProtoUriPfx + `/bgp/peer-groups$`)

type niBgpPgReqInfo struct {
	niName        string
	protoName     string
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
	reqInfo, err := matchNiBgpPgRequestUri(uri)
	if err != nil {
		return nil, err
	}
	if err := validateNiBgpProtoName(reqInfo.protoName); err != nil {
		return nil, err
	}
	return reqInfo, nil
}

func matchNiBgpPgRequestUri(uri string) (*niBgpPgReqInfo, error) {
	m := niBgpPgCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetCfg,
		}, nil
	}
	m = niBgpPgTimersReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetTimersCfg,
		}, nil
	}
	m = niBgpPgAfV4PfxReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			afiSafi:       niBgpPgAfiSafiIPv4,
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetAfPfxCfg,
		}, nil
	}
	m = niBgpPgAfV6PfxReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			afiSafi:       niBgpPgAfiSafiIPv6,
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetAfPfxCfg,
		}, nil
	}
	m = niBgpPgAfApplyReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 5 {
		leaf := ""
		if len(m) >= 6 {
			leaf = m[5]
		}
		afiSafi, ok := normalizeNiBgpPgAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP peer-group apply-policy mapping: " + m[4],
			}
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			afiSafi:       afiSafi,
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetAfApplyCfg,
		}, nil
	}
	m = niBgpPgAfCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 5 {
		leaf := ""
		if len(m) >= 6 {
			leaf = m[5]
		}
		afiSafi, ok := normalizeNiBgpPgAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP peer-group AFI-SAFI config mapping: " + m[4],
			}
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			afiSafi:       afiSafi,
			targetLeaf:    leaf,
			targetPath:    niBgpPgTargetAfCfg,
		}, nil
	}
	if m := niBgpPgAfInstKeyLeafReqUriRe.FindStringSubmatch(uri); len(m) >= 5 {
		afiSafi, ok := normalizeNiBgpPgAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP peer-group AFI-SAFI mapping: " + m[4],
			}
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			afiSafi:       afiSafi,
			targetLeaf:    niBgpPgAfiSafiNameLeaf,
			targetPath:    niBgpPgTargetAfInst,
		}, nil
	}
	if m := niBgpPgAfInstReqUriRe.FindStringSubmatch(uri); len(m) >= 5 {
		afiSafi, ok := normalizeNiBgpPgAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: "unsupported afi-safi-name for network-instance BGP peer-group AFI-SAFI mapping: " + m[4],
			}
		}
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			afiSafi:       afiSafi,
			targetPath:    niBgpPgTargetAfInst,
		}, nil
	}
	if m := niBgpPgAfisReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		return &niBgpPgReqInfo{
			niName:        m[1],
			protoName:     m[2],
			peerGroupName: m[3],
			targetPath:    niBgpPgTargetAfis,
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP peer-group config mapping: " + uri,
	}
}

func niBgpPgAfiSafiEnumFromName(afiSafi string) (ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE, bool) {
	switch afiSafi {
	case niBgpPgAfiSafiIPv4:
		return ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST, true
	case niBgpPgAfiSafiIPv6:
		return ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV6_UNICAST, true
	default:
		return ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_UNSET, false
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

// getNiBgpPeerGroupObj walks to a peer-group list instance, which is the one
// node every mapping below it starts from.
func getNiBgpPeerGroupObj(s *ygot.GoStruct, niName, protoName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup {
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
		Name:       protoName,
	}
	pObj, ok := niObj.Protocols.Protocol[pkey]
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

	pgObj, ok := pObj.Bgp.PeerGroups.PeerGroup[peerGroup]
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
	return pgObj
}

func getNiBgpPeerGroupCfgObj(s *ygot.GoStruct, niName, protoName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_Config {
	pgObj := getNiBgpPeerGroupObj(s, niName, protoName, peerGroup, createIfMissing)
	if pgObj == nil {
		return nil
	}
	return pgObj.Config
}

func getNiBgpPeerGroupTimersCfgObj(s *ygot.GoStruct, niName, protoName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_Timers_Config {
	pgObj := getNiBgpPeerGroupObj(s, niName, protoName, peerGroup, createIfMissing)
	if pgObj == nil || pgObj.Timers == nil {
		return nil
	}
	return pgObj.Timers.Config
}

func getNiBgpPeerGroupAfObj(s *ygot.GoStruct, niName, protoName, peerGroup string, afiKey ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi {
	pgObj := getNiBgpPeerGroupObj(s, niName, protoName, peerGroup, createIfMissing)
	if pgObj == nil || pgObj.AfiSafis == nil {
		return nil
	}

	afObj, ok := pgObj.AfiSafis.AfiSafi[afiKey]
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

func getNiBgpPeerGroupAfV4PrefixLimitCfgObj(s *ygot.GoStruct, niName, protoName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_Ipv4Unicast_PrefixLimit_Config {
	afObj := getNiBgpPeerGroupAfObj(s, niName, protoName, peerGroup, ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST, createIfMissing)
	if afObj == nil {
		return nil
	}
	return afObj.Ipv4Unicast.PrefixLimit.Config
}

func getNiBgpPeerGroupAfV6PrefixLimitCfgObj(s *ygot.GoStruct, niName, protoName, peerGroup string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_Ipv6Unicast_PrefixLimit_Config {
	afObj := getNiBgpPeerGroupAfObj(s, niName, protoName, peerGroup, ocbinds.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV6_UNICAST, createIfMissing)
	if afObj == nil {
		return nil
	}
	return afObj.Ipv6Unicast.PrefixLimit.Config
}

// getNiBgpPgMaxPrefixesFromYgot reads max-prefixes out of the family specific
// prefix-limit container the request named. The two families carry the leaf on
// separate ygot types, so the family has to pick the object before the leaf can
// be read.
func getNiBgpPgMaxPrefixesFromYgot(s *ygot.GoStruct, niName, protoName, peerGroup, afiSafi string) (*uint32, bool) {
	if afiSafi == niBgpPgAfiSafiIPv6 {
		pfxObj := getNiBgpPeerGroupAfV6PrefixLimitCfgObj(s, niName, protoName, peerGroup, false)
		if pfxObj != nil && pfxObj.MaxPrefixes != nil {
			return pfxObj.MaxPrefixes, true
		}
		return nil, false
	}
	pfxObj := getNiBgpPeerGroupAfV4PrefixLimitCfgObj(s, niName, protoName, peerGroup, false)
	if pfxObj != nil && pfxObj.MaxPrefixes != nil {
		return pfxObj.MaxPrefixes, true
	}
	return nil, false
}

func getNiBgpPeerGroupAfObjByName(s *ygot.GoStruct, niName, protoName, peerGroup, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi {
	afiKey, ok := niBgpPgAfiSafiEnumFromName(afiSafi)
	if !ok {
		return nil
	}
	return getNiBgpPeerGroupAfObj(s, niName, protoName, peerGroup, afiKey, createIfMissing)
}

func getNiBgpPeerGroupAfApplyPolicyCfgObj(s *ygot.GoStruct, niName, protoName, peerGroup, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_ApplyPolicy_Config {
	afObj := getNiBgpPeerGroupAfObjByName(s, niName, protoName, peerGroup, afiSafi, createIfMissing)
	if afObj == nil || afObj.ApplyPolicy == nil {
		return nil
	}
	return afObj.ApplyPolicy.Config
}

func getNiBgpPeerGroupAfCfgObj(s *ygot.GoStruct, niName, protoName, peerGroup, afiSafi string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_Config {
	afObj := getNiBgpPeerGroupAfObjByName(s, niName, protoName, peerGroup, afiSafi, createIfMissing)
	if afObj == nil {
		return nil
	}
	return afObj.Config
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

// makeNiBgpPgAfAllResMap addresses both families at once, which is what a
// request at the afi-safis container or at the peer-group itself covers.
func makeNiBgpPgAfAllResMap(niName, pgName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpPeerGroupAfTbl: {
			niName + "|" + pgName + "|" + niBgpPgAfiSafiIPv4: db.Value{Field: fields},
			niName + "|" + pgName + "|" + niBgpPgAfiSafiIPv6: db.Value{Field: fields},
		},
	}
}

// ensureNiBgpPgParentRow keeps an address-family write from landing without the
// peer-group it hangs off, since BGP_PEER_GROUP_AF_LIST/peer_group_name is a
// leafref onto BGP_PEER_GROUP_LIST and CVL rejects the orphan.
func ensureNiBgpPgParentRow(res map[string]map[string]db.Value, niName, pgName string) map[string]map[string]db.Value {
	if res == nil {
		res = make(map[string]map[string]db.Value)
	}
	if _, ok := res[niBgpPeerGroupTbl]; !ok {
		res[niBgpPeerGroupTbl] = make(map[string]db.Value)
	}
	res[niBgpPeerGroupTbl][niName+"|"+pgName] = db.Value{Field: map[string]string{}}
	return res
}

// niBgpPgInstDeleteResMap removes a peer-group together with the address-family
// rows that hang off it. Both tables go in one result map: translib orders
// BGP_PEER_GROUP_AF ahead of BGP_PEER_GROUP by table dependency, and CVL
// refuses to delete a peer-group while one of its address-family rows exists.
func niBgpPgInstDeleteResMap(niName, pgName string) map[string]map[string]db.Value {
	res := makeNiBgpPgAfAllResMap(niName, pgName, nil)
	res[niBgpPeerGroupTbl] = map[string]db.Value{niName + "|" + pgName: {}}
	return res
}

// niBgpPgHierDeleteDispatch maps a DELETE anchored on the peer-group list
// instance or on the peer-groups container. Those two nodes are annotated to
// the BGP container dispatch rather than to this transformer, so the container
// dispatch hands the request here. It reports whether the request was one of
// its own.
func niBgpPgHierDeleteDispatch(inParams XfmrParams) (bool, map[string]map[string]db.Value, error) {
	uri := inParams.requestUri
	if niBgpPgInstKeyLeafReqUriRe.MatchString(uri) {
		// The name is the row identity, so there is no field to clear;
		// removing it means removing the peer-group.
		return true, nil, tlerr.NotSupportedError{
			Format: "DELETE is not supported for the peer-group-name key; delete the peer-group instead",
			Path:   uri,
		}
	}
	if m := niBgpPgInstReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		if err := validateNiBgpProtoName(m[2]); err != nil {
			return true, nil, err
		}
		return true, niBgpPgInstDeleteResMap(m[1], m[3]), nil
	}
	if niBgpPgsReqUriRe.MatchString(uri) {
		return true, nil, tlerr.NotSupportedError{
			Format: "DELETE is not supported at the peer-groups container level; delete a peer-group instead",
			Path:   uri,
		}
	}
	return false, nil, nil
}

// wrapNiBgpPgRootAsDevice normalizes the request root to a device object so
// that one set of tree walks serves every anchor path in the hierarchy. A
// request anchored on the afi-safis container or on an afi-safi instance
// arrives with that node as the root rather than with a full device tree.
func wrapNiBgpPgRootAsDevice(s *ygot.GoStruct, reqInfo *niBgpPgReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for BGP peer-group mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}

	devObj := &ocbinds.Device{}
	var devRoot ygot.GoStruct = devObj
	pgObj := getNiBgpPeerGroupObj(&devRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, true)
	if pgObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create peer-group context for BGP peer-group mapping"}
	}

	// Reuse the original subtree pointers so that DbToYang population updates
	// the caller owned objects regardless of the anchor path.
	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis:
		pgObj.AfiSafis = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi:
		afiKey, ok := niBgpPgAfiSafiEnumFromName(reqInfo.afiSafi)
		if !ok {
			return nil, tlerr.InvalidArgsError{Format: "unsupported afi-safi-name for network-instance BGP peer-group AFI-SAFI mapping: " + reqInfo.afiSafi}
		}
		if pgObj.AfiSafis.AfiSafi == nil {
			pgObj.AfiSafis.AfiSafi = map[ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi{}
		}
		pgObj.AfiSafis.AfiSafi[afiKey] = root
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root for BGP peer-group mapping"}
	}

	return &devRoot, nil
}

func niBgpPgEnabledToAdminStatus(enabled bool) string {
	if enabled {
		return "up"
	}
	return "down"
}

func parseNiBgpPgEnabledFromParam(param interface{}) (*bool, bool) {
	switch v := param.(type) {
	case bool:
		b := v
		return &b, true
	case *bool:
		if v != nil {
			return v, true
		}
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return &b, true
		}
	case *string:
		if v != nil {
			if b, err := strconv.ParseBool(strings.TrimSpace(*v)); err == nil {
				return &b, true
			}
		}
	}
	return nil, false
}

// niBgpPgAfCfgFields renders the BGP_PEER_GROUP_AF fields of an afi-safi config
// request. An afi-safi carrying no explicit enabled leaf still means the
// address family is activated for the peer-group, so admin_status is seeded to
// up in that case, the same reading the instance level above it takes.
func niBgpPgAfCfgFields(inParams XfmrParams, reqInfo *niBgpPgReqInfo) (map[string]string, error) {
	fields := map[string]string{}
	switch reqInfo.targetLeaf {
	case niBgpPgSendCommTypeLeaf:
		cfgObj := getNiBgpPeerGroupAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, false)
		var fromTree []ocbinds.E_OpenconfigBgp_CommunityType
		if cfgObj != nil {
			fromTree = cfgObj.SendCommunityType
		}
		comm, err := niBgpSendCommunityTypeToField(inParams.param, fromTree, "peer-group")
		if err != nil {
			return nil, err
		}
		fields[niBgpPgSendCommFld] = comm
	case niBgpPgEnabledLeaf:
		enabled, ok := parseNiBgpPgEnabledFromParam(inParams.param)
		if !ok {
			cfgObj := getNiBgpPeerGroupAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, false)
			if cfgObj == nil || cfgObj.Enabled == nil {
				return nil, tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI enabled is missing in request payload"}
			}
			enabled = cfgObj.Enabled
		}
		fields[niBgpPgAdminStatusFld] = niBgpPgEnabledToAdminStatus(*enabled)
	case niBgpPgAfiSafiNameLeaf:
		fields[niBgpPgAdminStatusFld] = niBgpPgEnabledToAdminStatus(true)
	default:
		if enabled, ok := parseNiBgpPgEnabledFromParam(inParams.param); ok {
			fields[niBgpPgAdminStatusFld] = niBgpPgEnabledToAdminStatus(*enabled)
			break
		}
		cfgObj := getNiBgpPeerGroupAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, false)
		if cfgObj != nil && len(cfgObj.SendCommunityType) > 0 {
			comm, err := niBgpSendCommunityTypeToField(nil, cfgObj.SendCommunityType, "peer-group")
			if err != nil {
				return nil, err
			}
			fields[niBgpPgSendCommFld] = comm
		}
		if cfgObj != nil && cfgObj.Enabled != nil {
			fields[niBgpPgAdminStatusFld] = niBgpPgEnabledToAdminStatus(*cfgObj.Enabled)
			break
		}
		fields[niBgpPgAdminStatusFld] = niBgpPgEnabledToAdminStatus(true)
	}
	return fields, nil
}

func niBgpPgAdminStatusToEnabled(status string) (*bool, bool) {
	switch strings.TrimSpace(strings.ToLower(status)) {
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

// niBgpPgPayloadAfiSafiName reads an afi-safi-name leaf from the payload. An
// unrecognized shape yields false, which skips the comparison rather than
// refusing a write whose key translib has already validated.
func niBgpPgPayloadAfiSafiName(param interface{}) (string, bool) {
	switch v := param.(type) {
	case ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE:
		return niBgpAfiSafiNameFromEnum(v)
	case string:
		return normalizeNiBgpPgAfiSafiName(v)
	case *string:
		if v != nil {
			return normalizeNiBgpPgAfiSafiName(*v)
		}
	}
	return "", false
}

// niBgpPgAfInstFields renders the BGP_PEER_GROUP_AF fields an afi-safi instance
// payload carries. Naming an address family under a peer-group activates it
// there, so the row carries admin_status even when enabled is absent, which is
// what frrcfgd needs to emit "neighbor <peer-group> activate".
func niBgpPgAfInstFields(afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi, afiSafi string) map[string]string {
	fields := map[string]string{}
	enabled := true
	if afObj != nil && afObj.Config != nil && afObj.Config.Enabled != nil {
		enabled = *afObj.Config.Enabled
	}
	fields[niBgpPgAdminStatusFld] = niBgpPgEnabledToAdminStatus(enabled)
	if afObj == nil {
		return fields
	}
	if afObj.ApplyPolicy != nil && afObj.ApplyPolicy.Config != nil {
		if policy, ok := niBgpFirstPolicyName(afObj.ApplyPolicy.Config.ImportPolicy); ok {
			fields[niBgpPgRouteMapInFld] = policy
		}
		if policy, ok := niBgpFirstPolicyName(afObj.ApplyPolicy.Config.ExportPolicy); ok {
			fields[niBgpPgRouteMapOutFld] = policy
		}
	}
	// The two families carry max-prefixes on separate containers, so the row
	// only takes the one belonging to the family it is keyed on.
	if afiSafi == niBgpPgAfiSafiIPv6 {
		if afObj.Ipv6Unicast != nil && afObj.Ipv6Unicast.PrefixLimit != nil && afObj.Ipv6Unicast.PrefixLimit.Config != nil && afObj.Ipv6Unicast.PrefixLimit.Config.MaxPrefixes != nil {
			fields[niBgpPgMaxPfxLimitFld] = strconv.FormatUint(uint64(*afObj.Ipv6Unicast.PrefixLimit.Config.MaxPrefixes), 10)
		}
		return fields
	}
	if afObj.Ipv4Unicast != nil && afObj.Ipv4Unicast.PrefixLimit != nil && afObj.Ipv4Unicast.PrefixLimit.Config != nil && afObj.Ipv4Unicast.PrefixLimit.Config.MaxPrefixes != nil {
		fields[niBgpPgMaxPfxLimitFld] = strconv.FormatUint(uint64(*afObj.Ipv4Unicast.PrefixLimit.Config.MaxPrefixes), 10)
	}
	return fields
}

// buildNiBgpPgAfisResMap maps every afi-safi an afi-safis container payload
// carries, so that one request can create or update both families.
func buildNiBgpPgAfisResMap(s *ygot.GoStruct, reqInfo *niBgpPgReqInfo) (map[string]map[string]db.Value, error) {
	pgObj := getNiBgpPeerGroupObj(s, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, false)
	if pgObj == nil || pgObj.AfiSafis == nil || len(pgObj.AfiSafis.AfiSafi) == 0 {
		return nil, tlerr.InvalidArgsError{Format: "BGP peer-group afi-safis payload does not name any afi-safi"}
	}

	rows := make(map[string]db.Value)
	for afiEnum, afObj := range pgObj.AfiSafis.AfiSafi {
		afiSafi, ok := niBgpAfiSafiNameFromEnum(afiEnum)
		if !ok {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported afi-safi-name for network-instance BGP peer-group AFI-SAFI mapping: %v", afiEnum)}
		}
		rows[reqInfo.niName+"|"+reqInfo.peerGroupName+"|"+afiSafi] = db.Value{Field: niBgpPgAfInstFields(afObj, afiSafi)}
	}
	return ensureNiBgpPgParentRow(map[string]map[string]db.Value{niBgpPeerGroupAfTbl: rows}, reqInfo.niName, reqInfo.peerGroupName), nil
}

// niBgpPgDbAfiSafis lists the address families a peer-group holds a row for,
// which is what a read anchored on the afi-safis container has to enumerate.
func niBgpPgDbAfiSafis(cfgDb *db.DB, niName, pgName string) ([]string, error) {
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpPeerGroupAfTbl})
	if err != nil {
		return nil, err
	}
	afiSafis := make([]string, 0, 2)
	for _, k := range keys {
		if len(k.Comp) == 3 && k.Comp[0] == niName && k.Comp[1] == pgName {
			afiSafis = append(afiSafis, k.Comp[2])
		}
	}
	sort.Strings(afiSafis)
	return afiSafis, nil
}

// populateNiBgpPgAfFromDb fills one afi-safi instance of a peer-group, which is
// everything a read at or above the instance reports for that family.
func populateNiBgpPgAfFromDb(cfgDb *db.DB, ygRoot *ygot.GoStruct, niName, protoName, pgName, afiSafi string) error {
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupAfTbl}, db.Key{Comp: []string{niName, pgName, afiSafi}})
	if err != nil {
		return err
	}
	afiKey, ok := niBgpPgAfiSafiEnumFromName(afiSafi)
	if !ok {
		return tlerr.InvalidArgsError{Format: "unsupported afi-safi-name for network-instance BGP peer-group AFI-SAFI mapping: " + afiSafi}
	}
	afObj := getNiBgpPeerGroupAfObj(ygRoot, niName, protoName, pgName, afiKey, true)
	if afObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI object in YANG tree"}
	}
	afObj.AfiSafiName = afiKey
	if afObj.Config != nil {
		afObj.Config.AfiSafiName = afiKey
		if enabled, ok := niBgpPgAdminStatusToEnabled(entry.Field[niBgpPgAdminStatusFld]); ok {
			afObj.Config.Enabled = enabled
		}
	}
	if afObj.ApplyPolicy != nil && afObj.ApplyPolicy.Config != nil {
		if policy := strings.TrimSpace(entry.Field[niBgpPgRouteMapInFld]); policy != "" {
			afObj.ApplyPolicy.Config.ImportPolicy = []string{policy}
		}
		if policy := strings.TrimSpace(entry.Field[niBgpPgRouteMapOutFld]); policy != "" {
			afObj.ApplyPolicy.Config.ExportPolicy = []string{policy}
		}
	}
	maxStr := strings.TrimSpace(entry.Field[niBgpPgMaxPfxLimitFld])
	if maxStr == "" {
		return nil
	}
	maxU64, err := strconv.ParseUint(maxStr, 10, 32)
	if err != nil {
		return err
	}
	max := uint32(maxU64)
	if afiSafi == niBgpPgAfiSafiIPv6 {
		if afObj.Ipv6Unicast != nil && afObj.Ipv6Unicast.PrefixLimit != nil && afObj.Ipv6Unicast.PrefixLimit.Config != nil {
			afObj.Ipv6Unicast.PrefixLimit.Config.MaxPrefixes = &max
		}
		return nil
	}
	if afObj.Ipv4Unicast != nil && afObj.Ipv4Unicast.PrefixLimit != nil && afObj.Ipv4Unicast.PrefixLimit.Config != nil {
		afObj.Ipv4Unicast.PrefixLimit.Config.MaxPrefixes = &max
	}
	return nil
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

// The two apply-policy leaves are leaf-lists of policy names and are parsed
// the same way, so the leaf being written is what the caller tracks, not this
// function.
func parseNiBgpPgApplyPolicyFromParam(param interface{}) (string, bool) {
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

// niBgpFirstPolicyName picks the policy to store out of an apply-policy
// leaf-list. CONFIG_DB holds one route-map per direction, so only the first
// name a payload carries can be kept.
func niBgpFirstPolicyName(policies []string) (string, bool) {
	for _, p := range policies {
		if v := strings.TrimSpace(p); v != "" {
			return v, true
		}
	}
	return "", false
}

// niBgpPgApplyPolicyFields renders the CONFIG_DB fields of an apply-policy
// request. A request naming one leaf carries that leaf's value in param, while
// a request at the container carries either leaf, or both, in the ygot tree.
func niBgpPgApplyPolicyFields(inParams XfmrParams, reqInfo *niBgpPgReqInfo) (map[string]string, error) {
	var importPolicies, exportPolicies []string
	if applyCfgObj := getNiBgpPeerGroupAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, false); applyCfgObj != nil {
		importPolicies = applyCfgObj.ImportPolicy
		exportPolicies = applyCfgObj.ExportPolicy
	}

	fields := map[string]string{}
	switch reqInfo.targetLeaf {
	case niBgpPgExportPolicyLeaf:
		policy, ok := parseNiBgpPgApplyPolicyFromParam(inParams.param)
		if !ok {
			if policy, ok = niBgpFirstPolicyName(exportPolicies); !ok {
				return nil, tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI apply-policy export-policy is missing in request payload"}
			}
		}
		fields[niBgpPgRouteMapOutFld] = policy
	case niBgpPgImportPolicyLeaf:
		policy, ok := parseNiBgpPgApplyPolicyFromParam(inParams.param)
		if !ok {
			if policy, ok = niBgpFirstPolicyName(importPolicies); !ok {
				return nil, tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI apply-policy import-policy is missing in request payload"}
			}
		}
		fields[niBgpPgRouteMapInFld] = policy
	case "":
		// The container itself. param names the container rather than a leaf,
		// so the value of each leaf can only come from the ygot tree.
		if policy, ok := niBgpFirstPolicyName(exportPolicies); ok {
			fields[niBgpPgRouteMapOutFld] = policy
		}
		if policy, ok := niBgpFirstPolicyName(importPolicies); ok {
			fields[niBgpPgRouteMapInFld] = policy
		}
		if len(fields) == 0 {
			return nil, tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI apply-policy config payload carries neither import-policy nor export-policy"}
		}
	default:
		return nil, tlerr.NotSupportedError{Format: "unsupported leaf under peer-group AFI-SAFI apply-policy config"}
	}
	return fields, nil
}

var YangToDb_ni_bgp_pg_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseNiBgpPgRequestUri(niHandlerReqUri(inParams))
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
		switch reqInfo.targetPath {
		case niBgpPgTargetAfis:
			// Every family the peer-group holds a row for. The peer-group
			// itself survives, so its BGP_PEER_GROUP row stays.
			return makeNiBgpPgAfAllResMap(reqInfo.niName, reqInfo.peerGroupName, nil), nil
		case niBgpPgTargetAfInst:
			// The key leaf is the row identity, so clearing it means removing
			// the row the instance names.
			return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, nil), nil
		case niBgpPgTargetAfCfg:
			switch reqInfo.targetLeaf {
			case "":
				// Both fields this container maps. The key leaf below it names
				// the row rather than a field of it.
				return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{
					niBgpPgAdminStatusFld: "",
					niBgpPgSendCommFld:    "",
				}), nil
			case niBgpPgAfiSafiNameLeaf, niBgpPgEnabledLeaf:
				return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{niBgpPgAdminStatusFld: ""}), nil
			case niBgpPgSendCommTypeLeaf:
				return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{niBgpPgSendCommFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /peer-groups/peer-group/afi-safis/afi-safi/config or its enabled and send-community-type leaves", Path: inParams.requestUri}
			}
		case niBgpPgTargetAfPfxCfg:
			switch reqInfo.targetLeaf {
			case "", niBgpPgMaxPrefixesLeaf:
				return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{niBgpPgMaxPfxLimitFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: fmt.Sprintf("DELETE supported only for /peer-groups/peer-group/afi-safis/afi-safi/%s/prefix-limit/config or its max-prefixes leaf", niBgpPgAfiSafiUriName(reqInfo.afiSafi)), Path: inParams.requestUri}
			}
		case niBgpPgTargetAfApplyCfg:
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes both directions with it.
				return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{
					niBgpPgRouteMapInFld:  "",
					niBgpPgRouteMapOutFld: "",
				}), nil
			case niBgpPgExportPolicyLeaf:
				return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{niBgpPgRouteMapOutFld: ""}), nil
			case niBgpPgImportPolicyLeaf:
				return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, map[string]string{niBgpPgRouteMapInFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /peer-groups/peer-group/afi-safis/afi-safi/apply-policy/config, its import-policy leaf, or its export-policy leaf", Path: inParams.requestUri}
			}
		case niBgpPgTargetTimersCfg:
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes both timers with it.
				return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{
					niBgpPgKeepaliveFld: "",
					niBgpPgHoldTimeFld:  "",
				}), nil
			case niBgpPgKeepaliveLeaf:
				return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgKeepaliveFld: ""}), nil
			case niBgpPgHoldTimeLeaf:
				return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgHoldTimeFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /peer-groups/peer-group/timers/config, its keepalive-interval leaf, or its hold-time leaf", Path: inParams.requestUri}
			}
		case niBgpPgTargetCfg:
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes every leaf it maps with it, which
				// here is peer-as alone: peer-group-name is the row key rather
				// than a field. The list instance survives, so the row is
				// emptied rather than removed.
				return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgAsnFld: ""}), nil
			case niBgpPgPeerGroupNameLeaf:
				// Deleting the key leaf below config has meant deleting the
				// peer-group since this mapping was written, so it keeps that
				// reading. It leaves the address-family rows alone, which is
				// what makes CVL refuse it while any of them exists; deleting
				// the list instance is what takes them along.
				return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, nil), nil
			case niBgpPgPeerAsLeaf:
				return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgAsnFld: ""}), nil
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /peer-groups/peer-group/config, its peer-group-name leaf, or its peer-as leaf", Path: inParams.requestUri}
			}
		}
		return res, tlerr.NotSupportedError{Format: "DELETE is not supported for this network-instance BGP peer-group path", Path: inParams.requestUri}
	}

	if reqInfo.targetPath == niBgpPgTargetAfis || reqInfo.targetPath == niBgpPgTargetAfInst {
		// A payload key that disagrees with the one named in the path would
		// land on the row the path names and quietly ignore the payload.
		if reqInfo.targetLeaf == niBgpPgAfiSafiNameLeaf {
			if name, ok := niBgpPgPayloadAfiSafiName(inParams.param); ok && name != reqInfo.afiSafi {
				return res, tlerr.InvalidArgsError{
					Format: fmt.Sprintf("BGP peer-group afi-safi-name payload (%s) does not match key (%s)", name, reqInfo.afiSafi),
				}
			}
		}
		wrappedRoot, wrapErr := wrapNiBgpPgRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return res, wrapErr
		}
		if inParams.invokeCRUSubtreeOnce != nil {
			// The whole subtree is mapped in this call, so translib must not
			// invoke this transformer again for the child nodes.
			*inParams.invokeCRUSubtreeOnce = true
		}
		if reqInfo.targetPath == niBgpPgTargetAfis {
			return buildNiBgpPgAfisResMap(wrappedRoot, reqInfo)
		}
		afObj := getNiBgpPeerGroupAfObjByName(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, false)
		return ensureNiBgpPgParentRow(
			makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, niBgpPgAfInstFields(afObj, reqInfo.afiSafi)),
			reqInfo.niName, reqInfo.peerGroupName), nil
	}

	if reqInfo.targetPath == niBgpPgTargetAfCfg {
		fields, err := niBgpPgAfCfgFields(inParams, reqInfo)
		if err != nil {
			return res, err
		}
		return ensureNiBgpPgParentRow(
			makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, fields),
			reqInfo.niName, reqInfo.peerGroupName), nil
	}

	if reqInfo.targetPath == niBgpPgTargetCfg {
		switch reqInfo.targetLeaf {
		case "":
			// A request at the config container itself can carry any mapped
			// leaf, so map each one the payload actually sets.
			fields := map[string]string{}
			if cfgObj := getNiBgpPeerGroupCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, false); cfgObj != nil {
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
				cfgObj := getNiBgpPeerGroupCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, false)
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
				max, ok = getNiBgpPgMaxPrefixesFromYgot(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi)
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
		case "", niBgpPgExportPolicyLeaf, niBgpPgImportPolicyLeaf:
			fields, err := niBgpPgApplyPolicyFields(inParams, reqInfo)
			if err != nil {
				return res, err
			}
			return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, fields), nil
		default:
			return res, tlerr.NotSupportedError{Format: "unsupported leaf under peer-group AFI-SAFI apply-policy config", Path: inParams.requestUri}
		}
	}

	switch reqInfo.targetLeaf {
	case "":
		// A request at the timers config container can carry either timer.
		fields := map[string]string{}
		if tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, false); tmObj != nil {
			if tmObj.KeepaliveInterval != nil {
				fields[niBgpPgKeepaliveFld] = strconv.FormatUint(uint64(*tmObj.KeepaliveInterval), 10)
			}
			if tmObj.HoldTime != nil {
				fields[niBgpPgHoldTimeFld] = strconv.FormatUint(uint64(*tmObj.HoldTime), 10)
			}
		}
		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "BGP peer-group timers config payload carries neither hold-time nor keepalive-interval"}
		}
		return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, fields), nil
	case niBgpPgKeepaliveLeaf:
		keepalive, ok := parseNiBgpPgKeepaliveFromParam(inParams.param)
		if !ok {
			tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, false)
			if tmObj == nil || tmObj.KeepaliveInterval == nil {
				return res, tlerr.InvalidArgsError{Format: "BGP peer-group keepalive-interval is missing in request payload"}
			}
			iv := *tmObj.KeepaliveInterval
			keepalive = &iv
		}
		return makeNiBgpPgResMap(reqInfo.niName, reqInfo.peerGroupName, map[string]string{niBgpPgKeepaliveFld: strconv.FormatUint(uint64(*keepalive), 10)}), nil
	case niBgpPgHoldTimeLeaf:
		hold, ok := parseNiBgpPgHoldTimeFromParam(inParams.param)
		if !ok {
			tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, false)
			if tmObj == nil || tmObj.HoldTime == nil {
				return res, tlerr.InvalidArgsError{Format: "BGP peer-group hold-time is missing in request payload"}
			}
			iv := *tmObj.HoldTime
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
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP peer-group config"}
	}

	if reqInfo.targetPath == niBgpPgTargetAfis || reqInfo.targetPath == niBgpPgTargetAfInst {
		wrappedRoot, wrapErr := wrapNiBgpPgRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return wrapErr
		}
		if reqInfo.targetPath == niBgpPgTargetAfInst {
			return populateNiBgpPgAfFromDb(cfgDb, wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi)
		}
		afiSafis, err := niBgpPgDbAfiSafis(cfgDb, reqInfo.niName, reqInfo.peerGroupName)
		if err != nil {
			return err
		}
		for _, afiSafi := range afiSafis {
			if err := populateNiBgpPgAfFromDb(cfgDb, wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, afiSafi); err != nil {
				return err
			}
		}
		return nil
	}

	if reqInfo.targetPath == niBgpPgTargetAfCfg {
		afEntry, afErr := cfgDb.GetEntry(&db.TableSpec{Name: niBgpPeerGroupAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi}})
		if afErr != nil {
			return afErr
		}
		afCfgObj := getNiBgpPeerGroupAfCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, true)
		if afCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgAfiSafiNameLeaf {
			if afiSafiName, ok := niBgpPgAfiSafiEnumFromName(reqInfo.afiSafi); ok {
				afCfgObj.AfiSafiName = afiSafiName
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgEnabledLeaf {
			if enabled, ok := niBgpPgAdminStatusToEnabled(afEntry.Field[niBgpPgAdminStatusFld]); ok {
				afCfgObj.Enabled = enabled
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgSendCommTypeLeaf {
			if comm, ok := niBgpSendCommunityTypeFromDb(afEntry.Field[niBgpPgSendCommFld]); ok {
				afCfgObj.SendCommunityType = comm
			}
		}
		return nil
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
			pfxObj := getNiBgpPeerGroupAfV6PrefixLimitCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, true)
			if pfxObj == nil {
				return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI prefix-limit config object in YANG tree"}
			}
			pfxObj.MaxPrefixes = maxPrefixes
			return nil
		}
		pfxObj := getNiBgpPeerGroupAfV4PrefixLimitCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, true)
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
		applyCfgObj := getNiBgpPeerGroupAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, true)
		if applyCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI apply-policy config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgExportPolicyLeaf {
			exportPolicy, ok := afEntry.Field[niBgpPgRouteMapOutFld]
			if ok && strings.TrimSpace(exportPolicy) != "" {
				applyCfgObj.ExportPolicy = []string{strings.TrimSpace(exportPolicy)}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgImportPolicyLeaf {
			importPolicy, ok := afEntry.Field[niBgpPgRouteMapInFld]
			if ok && strings.TrimSpace(importPolicy) != "" {
				applyCfgObj.ImportPolicy = []string{strings.TrimSpace(importPolicy)}
			}
		}
		return nil
	}
	if reqInfo.targetPath == niBgpPgTargetCfg {
		cfgObj := getNiBgpPeerGroupCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, true)
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

	tmObj := getNiBgpPeerGroupTimersCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, true)
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
			iv := uint16(ival)
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
			iv := uint16(ival)
			tmObj.HoldTime = &iv
		}
	}
	return nil
}
