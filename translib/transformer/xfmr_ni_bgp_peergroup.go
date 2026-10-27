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
	niBgpPeerGroupTbl        = "BGP_PEER_GROUP"
	niBgpPeerGroupAfTbl      = "BGP_PEER_GROUP_AF"
	niBgpPgTargetCfg         = "peer-group-config"
	niBgpPgTargetTimersCfg   = "peer-group-timers-config"
	niBgpPgTargetAfApplyCfg  = "peer-group-af-apply-policy-config"
	niBgpPgPeerGroupNameLeaf = "peer-group-name"
	niBgpPgPeerAsLeaf        = "peer-as"
	niBgpPgKeepaliveLeaf     = "keepalive-interval"
	niBgpPgHoldTimeLeaf      = "hold-time"
	niBgpPgExportPolicyLeaf  = "export-policy"
	niBgpPgImportPolicyLeaf  = "import-policy"
	niBgpDefExportPolicyLeaf = "default-export-policy"
	niBgpDefImportPolicyLeaf = "default-import-policy"
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

// The apply-policy container and its config container are one target: the
// config container is the only writable child.
var niBgpPgAfApplyReqUriRe = regexp.MustCompile(niBgpPgUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/apply-policy(?:/config(?:/(export-policy|import-policy|default-export-policy|default-import-policy))?)?$`)

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
func niBgpPgAfInstFields(afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi, afiSafi string) (map[string]string, error) {
	fields := map[string]string{}
	enabled := true
	if afObj != nil && afObj.Config != nil && afObj.Config.Enabled != nil {
		enabled = *afObj.Config.Enabled
	}
	fields[niBgpPgAdminStatusFld] = niBgpPgEnabledToAdminStatus(enabled)
	if afObj == nil {
		return fields, nil
	}
	if afObj.Config != nil && len(afObj.Config.SendCommunityType) > 0 {
		comm, err := niBgpSendCommunityTypeToField(nil, afObj.Config.SendCommunityType, "peer-group")
		if err != nil {
			return nil, err
		}
		fields[niBgpPgSendCommFld] = comm
	}
	if afObj.ApplyPolicy != nil && afObj.ApplyPolicy.Config != nil {
		cfg := afObj.ApplyPolicy.Config
		if err := niBgpApplyPolicyDefaultsErr("peer-group", cfg.DefaultImportPolicy, cfg.DefaultExportPolicy); err != nil {
			return nil, err
		}
		policy, ok, err := niBgpSinglePolicyName(cfg.ImportPolicy, "peer-group", niBgpPgImportPolicyLeaf)
		if err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpPgRouteMapInFld] = policy
		}
		if policy, ok, err = niBgpSinglePolicyName(cfg.ExportPolicy, "peer-group", niBgpPgExportPolicyLeaf); err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpPgRouteMapOutFld] = policy
		}
	}
	leafFields, err := niBgpPeerAfFields(niBgpPgKind, reflect.ValueOf(afObj), afiSafi)
	if err != nil {
		return nil, err
	}
	for f, v := range leafFields {
		fields[f] = v
	}
	return fields, nil
}

// niBgpPgAfInstOwnedFlds are the BGP_PEER_GROUP_AF fields an afi-safi instance
// maps. A REPLACE of the instance clears those the payload leaves out and
// keeps every other field of the row.
var niBgpPgAfInstOwnedFlds = map[string]bool{
	niBgpPgAdminStatusFld: true,
	niBgpPgSendCommFld:    true,
	niBgpPgRouteMapInFld:  true,
	niBgpPgRouteMapOutFld: true,
	niBgpPgMaxPfxLimitFld: true,
}

// niBgpPgAfInstWrite maps the row of one afi-safi instance a write carries. A
// nil row means the write has already been staged and nothing is to be returned
// for it.
func niBgpPgAfInstWrite(inParams XfmrParams, reqInfo *niBgpPgReqInfo, afiSafi string, fields map[string]string) (map[string]string, error) {
	existing := niBgpCfgDbEntry(inParams, niBgpPeerGroupAfTbl, reqInfo.niName, reqInfo.peerGroupName, afiSafi)
	fields, clear := niBgpSplitClearFlds(fields)
	if inParams.oper == REPLACE {
		return niBgpReplaceRowFields(existing, fields, niBgpPgAfInstOwnedFlds), nil
	}
	stale := map[string]string{}
	for f := range clear {
		if _, ok := existing.Field[f]; ok {
			stale[f] = ""
		}
	}
	if len(stale) > 0 {
		if err := niBgpStageSubOp(inParams, DELETE, makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, afiSafi, stale)); err != nil {
			return nil, err
		}
	}
	staged, err := niBgpPgStaleRouteMaps(inParams, reqInfo.niName, reqInfo.peerGroupName, afiSafi, fields)
	if err != nil || staged {
		return nil, err
	}
	return fields, nil
}

// niBgpPgParentRowIfMissing adds the BGP_PEER_GROUP row an address-family
// write depends on, unless CONFIG_DB already holds it. A REPLACE anchored above
// the config containers rewrites each row it returns as a whole, so returning
// an existing parent would empty it.
func niBgpPgParentRowIfMissing(inParams XfmrParams, res map[string]map[string]db.Value, niName, pgName string) map[string]map[string]db.Value {
	if parent := niBgpCfgDbEntry(inParams, niBgpPeerGroupTbl, niName, pgName); parent.IsPopulated() {
		return res
	}
	return ensureNiBgpPgParentRow(res, niName, pgName)
}

// buildNiBgpPgAfisResMap maps every afi-safi an afi-safis container payload
// carries, so that one request can create or update both families.
func buildNiBgpPgAfisResMap(inParams XfmrParams, s *ygot.GoStruct, reqInfo *niBgpPgReqInfo) (map[string]map[string]db.Value, error) {
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
		fields, err := niBgpPgAfInstFields(afObj, afiSafi)
		if err != nil {
			return nil, err
		}
		if fields, err = niBgpPgAfInstWrite(inParams, reqInfo, afiSafi, fields); err != nil {
			return nil, err
		}
		if fields != nil {
			rows[reqInfo.niName+"|"+reqInfo.peerGroupName+"|"+afiSafi] = db.Value{Field: fields}
		}
	}
	return niBgpPgParentRowIfMissing(inParams, map[string]map[string]db.Value{niBgpPeerGroupAfTbl: rows}, reqInfo.niName, reqInfo.peerGroupName), nil
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
		if comm, ok := niBgpSendCommunityTypeFromDb(entry.Field[niBgpPgSendCommFld]); ok {
			afObj.Config.SendCommunityType = comm
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
	if err := niBgpPeerAfLeavesToYang(niBgpPgKind, ygRoot, niName, protoName, pgName, afiSafi, "", "", entry); err != nil {
		return err
	}
	niBgpPgMirrorAfStateInTree(reflect.ValueOf(afObj))
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

// niBgpPolicyNames collects the policy names an apply-policy leaf-list value
// carries, in whichever shape translib hands the value over.
func niBgpPolicyNames(v interface{}) []string {
	var names []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			names = append(names, s)
		}
	}
	switch val := v.(type) {
	case string:
		add(val)
	case *string:
		if val != nil {
			add(*val)
		}
	case []string:
		for _, s := range val {
			add(s)
		}
	case []interface{}:
		for _, item := range val {
			if s, ok := item.(string); ok {
				add(s)
			}
		}
	}
	return names
}

// niBgpSinglePolicyName picks the policy to store out of an apply-policy
// leaf-list value. CONFIG_DB holds one route-map per direction, so a request
// naming more than one is refused rather than cut down to its first name.
func niBgpSinglePolicyName(v interface{}, scope, leaf string) (string, bool, error) {
	names := niBgpPolicyNames(v)
	switch {
	case len(names) == 0:
		return "", false, nil
	case len(names) > 1:
		return "", false, tlerr.NotSupportedError{Format: fmt.Sprintf("BGP %s apply-policy %s accepts one policy, since CONFIG_DB holds a single route-map per direction, and the request carries %d", scope, leaf, len(names))}
	}
	return names[0], true, nil
}

// niBgpApplyPolicyDefaultLeafErr refuses default-import-policy and
// default-export-policy. CONFIG_DB has no field for either, so a write would
// otherwise succeed without changing anything. A policy whose statements end in
// ACCEPT_ROUTE or REJECT_ROUTE, attached through import-policy or
// export-policy, is how either outcome is configured.
func niBgpApplyPolicyDefaultLeafErr(scope, leaf string) error {
	return tlerr.NotSupportedError{Format: fmt.Sprintf("BGP %s apply-policy %s is not supported; attach a policy ending in ACCEPT_ROUTE or REJECT_ROUTE through import-policy or export-policy instead", scope, leaf)}
}

func niBgpApplyPolicyDefaultsErr(scope string, defImport, defExport ocbinds.E_OpenconfigRoutingPolicy_DefaultPolicyType) error {
	if defImport != ocbinds.OpenconfigRoutingPolicy_DefaultPolicyType_UNSET {
		return niBgpApplyPolicyDefaultLeafErr(scope, "default-import-policy")
	}
	if defExport != ocbinds.OpenconfigRoutingPolicy_DefaultPolicyType_UNSET {
		return niBgpApplyPolicyDefaultLeafErr(scope, "default-export-policy")
	}
	return nil
}

// niBgpStageSubOp adds rows to one sub-operation of the request. A subtree
// transformer only returns rows for the operation it was called for, so a
// write that also has to clear fields, or must not run as the request's own
// operation, rides along this way.
func niBgpStageSubOp(inParams XfmrParams, oper Operation, rows map[string]map[string]db.Value) error {
	if len(rows) == 0 {
		return nil
	}
	if inParams.subOpDataMap == nil {
		return tlerr.InternalError{Format: "BGP apply-policy write has no sub-operation map to stage into"}
	}
	if inParams.subOpDataMap[oper] == nil {
		subOpMap := make(RedisDbMap)
		inParams.subOpDataMap[oper] = &subOpMap
	}
	subOpMap := *inParams.subOpDataMap[oper]
	subOpMap[db.ConfigDB] = mergeDbResMapFields(subOpMap[db.ConfigDB], rows)
	return nil
}

// niBgpCfgDbEntry reads a CONFIG_DB row through the request's handle, so that
// rows staged earlier in the same transaction are seen. A missing row reads as
// an empty one.
func niBgpCfgDbEntry(inParams XfmrParams, table string, key ...string) db.Value {
	if inParams.d == nil {
		return db.Value{}
	}
	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: key})
	if err != nil {
		return db.Value{}
	}
	return entry
}

// niBgpReplaceRowFields renders the row a REPLACE of an afi-safi instance
// writes. The row is replaced as a whole, so the fields that other subtrees own
// are carried over from CONFIG_DB, and only the fields the instance owns follow
// the payload.
func niBgpReplaceRowFields(existing db.Value, fields map[string]string, owned map[string]bool) map[string]string {
	row := make(map[string]string, len(fields))
	for f, v := range existing.Field {
		if f != "NULL" && !owned[f] {
			row[f] = v
		}
	}
	for f, v := range fields {
		row[f] = v
	}
	return row
}

// niBgpPgStaleRouteMaps stages the removal of a route-map a write is about to
// change. The route-map fields of BGP_PEER_GROUP_AF are leaf-lists, which an
// update merges into rather than replaces, and CVL holds each to one element.
// Deleting every field of a row leaves a NULL placeholder that the update then
// keeps, so a row holding nothing but stale route-maps is instead replaced with
// fields as a whole. It reports true when it has staged that replace, in which
// case the caller must not return the row itself.
func niBgpPgStaleRouteMaps(inParams XfmrParams, niName, pgName, afiSafi string, fields map[string]string) (bool, error) {
	existing := niBgpCfgDbEntry(inParams, niBgpPeerGroupAfTbl, niName, pgName, afiSafi)
	stale := map[string]string{}
	for _, fld := range []string{niBgpPgRouteMapInFld, niBgpPgRouteMapOutFld} {
		v, ok := fields[fld]
		if !ok || !existing.Has(fld) {
			continue
		}
		if cur := strings.TrimSpace(existing.Get(fld)); cur != "" && cur != v {
			stale[fld] = ""
		}
	}
	if len(stale) == 0 {
		return false, nil
	}
	for fld := range existing.Field {
		if _, ok := stale[fld]; !ok && fld != "NULL" {
			return false, niBgpStageSubOp(inParams, DELETE, makeNiBgpPgAfResMap(niName, pgName, afiSafi, stale))
		}
	}
	return true, niBgpStageSubOp(inParams, REPLACE, makeNiBgpPgAfResMap(niName, pgName, afiSafi, fields))
}

// niBgpPgApplyPolicyFields renders the CONFIG_DB fields of an apply-policy
// request. A request naming one leaf carries that leaf's value in param, while
// a request at the container carries either leaf, or both, in the ygot tree.
func niBgpPgApplyPolicyFields(inParams XfmrParams, reqInfo *niBgpPgReqInfo) (map[string]string, error) {
	var importPolicies, exportPolicies []string
	if applyCfgObj := getNiBgpPeerGroupAfApplyPolicyCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, false); applyCfgObj != nil {
		if err := niBgpApplyPolicyDefaultsErr("peer-group", applyCfgObj.DefaultImportPolicy, applyCfgObj.DefaultExportPolicy); err != nil {
			return nil, err
		}
		importPolicies = applyCfgObj.ImportPolicy
		exportPolicies = applyCfgObj.ExportPolicy
	}

	// A request naming one leaf carries its value in param; the ygot tree
	// holds it otherwise.
	leafPolicy := func(leaf string, fromTree []string) (string, error) {
		policy, ok, err := niBgpSinglePolicyName(inParams.param, "peer-group", leaf)
		if err == nil && !ok {
			policy, ok, err = niBgpSinglePolicyName(fromTree, "peer-group", leaf)
		}
		if err != nil {
			return "", err
		}
		if !ok {
			return "", tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI apply-policy " + leaf + " is missing in request payload"}
		}
		return policy, nil
	}

	fields := map[string]string{}
	switch reqInfo.targetLeaf {
	case niBgpPgExportPolicyLeaf:
		policy, err := leafPolicy(niBgpPgExportPolicyLeaf, exportPolicies)
		if err != nil {
			return nil, err
		}
		fields[niBgpPgRouteMapOutFld] = policy
	case niBgpPgImportPolicyLeaf:
		policy, err := leafPolicy(niBgpPgImportPolicyLeaf, importPolicies)
		if err != nil {
			return nil, err
		}
		fields[niBgpPgRouteMapInFld] = policy
	case "":
		// The container itself. param names the container rather than a leaf,
		// so the value of each leaf can only come from the ygot tree.
		policy, ok, err := niBgpSinglePolicyName(exportPolicies, "peer-group", niBgpPgExportPolicyLeaf)
		if err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpPgRouteMapOutFld] = policy
		}
		if policy, ok, err = niBgpSinglePolicyName(importPolicies, "peer-group", niBgpPgImportPolicyLeaf); err != nil {
			return nil, err
		}
		if ok {
			fields[niBgpPgRouteMapInFld] = policy
		}
		if len(fields) == 0 && inParams.oper != REPLACE {
			return nil, tlerr.InvalidArgsError{Format: "BGP peer-group AFI-SAFI apply-policy config payload carries neither import-policy nor export-policy"}
		}
	case niBgpDefImportPolicyLeaf, niBgpDefExportPolicyLeaf:
		return nil, niBgpApplyPolicyDefaultLeafErr("peer-group", reqInfo.targetLeaf)
	default:
		return nil, tlerr.NotSupportedError{Format: "unsupported leaf under peer-group AFI-SAFI apply-policy config"}
	}
	return fields, nil
}

// niBgpPgApplyPolicyDelete maps a DELETE of apply-policy to the route-map
// fields the row actually holds. Deleting an absent leaf-list field queues no
// write, which translib refuses, so a delete with nothing to remove returns no
// rows and succeeds as a no-op.
func niBgpPgApplyPolicyDelete(inParams XfmrParams, reqInfo *niBgpPgReqInfo, flds ...string) map[string]map[string]db.Value {
	existing := niBgpCfgDbEntry(inParams, niBgpPeerGroupAfTbl, reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi)
	fields := map[string]string{}
	for _, fld := range flds {
		if existing.Has(fld) {
			fields[fld] = ""
		}
	}
	if len(fields) == 0 {
		return map[string]map[string]db.Value{}
	}
	return makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, fields)
}

// niBgpPgApplyPolicyWrite maps a write to apply-policy. A REPLACE of either
// container clears the direction its payload leaves out. A REPLACE runs as an
// update of the fields it names: the apply-policy container is not a terminal
// node, so translib would otherwise rewrite the whole row from what this
// returns.
func niBgpPgApplyPolicyWrite(inParams XfmrParams, reqInfo *niBgpPgReqInfo) (map[string]map[string]db.Value, error) {
	fields, err := niBgpPgApplyPolicyFields(inParams, reqInfo)
	if err != nil {
		return nil, err
	}
	staged, err := niBgpPgStaleRouteMaps(inParams, reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, fields)
	if err != nil {
		return nil, err
	}
	if staged {
		return map[string]map[string]db.Value{}, nil
	}
	res := makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, fields)
	if inParams.oper != REPLACE {
		return res, nil
	}
	if reqInfo.targetLeaf == "" {
		existing := niBgpCfgDbEntry(inParams, niBgpPeerGroupAfTbl, reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi)
		clear := map[string]string{}
		for _, fld := range []string{niBgpPgRouteMapInFld, niBgpPgRouteMapOutFld} {
			if _, ok := fields[fld]; !ok && existing.Has(fld) {
				clear[fld] = ""
			}
		}
		if err := niBgpStageSubOp(inParams, DELETE, makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, clear)); err != nil {
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

// yangToDbNiBgpPgCfgBase maps the peer-group leaves niBgpPgLeafFlds leaves out.
func yangToDbNiBgpPgCfgBase(inParams XfmrParams) (map[string]map[string]db.Value, error) {
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
		case niBgpPgTargetAfApplyCfg:
			switch reqInfo.targetLeaf {
			case "":
				// The container delete takes both directions with it.
				return niBgpPgApplyPolicyDelete(inParams, reqInfo, niBgpPgRouteMapInFld, niBgpPgRouteMapOutFld), nil
			case niBgpPgExportPolicyLeaf:
				return niBgpPgApplyPolicyDelete(inParams, reqInfo, niBgpPgRouteMapOutFld), nil
			case niBgpPgImportPolicyLeaf:
				return niBgpPgApplyPolicyDelete(inParams, reqInfo, niBgpPgRouteMapInFld), nil
			case niBgpDefImportPolicyLeaf, niBgpDefExportPolicyLeaf:
				return res, niBgpApplyPolicyDefaultLeafErr("peer-group", reqInfo.targetLeaf)
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
			return buildNiBgpPgAfisResMap(inParams, wrappedRoot, reqInfo)
		}
		afObj := getNiBgpPeerGroupAfObjByName(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, reqInfo.afiSafi, false)
		fields, err := niBgpPgAfInstFields(afObj, reqInfo.afiSafi)
		if err != nil {
			return res, err
		}
		if fields, err = niBgpPgAfInstWrite(inParams, reqInfo, reqInfo.afiSafi, fields); err != nil {
			return res, err
		}
		if fields == nil {
			return niBgpPgParentRowIfMissing(inParams, res, reqInfo.niName, reqInfo.peerGroupName), nil
		}
		return niBgpPgParentRowIfMissing(inParams,
			makeNiBgpPgAfResMap(reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi, fields),
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
	if reqInfo.targetPath == niBgpPgTargetAfApplyCfg {
		switch reqInfo.targetLeaf {
		case "", niBgpPgExportPolicyLeaf, niBgpPgImportPolicyLeaf:
			return niBgpPgApplyPolicyWrite(inParams, reqInfo)
		case niBgpDefImportPolicyLeaf, niBgpDefExportPolicyLeaf:
			return res, niBgpApplyPolicyDefaultLeafErr("peer-group", reqInfo.targetLeaf)
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
		if len(fields) == 0 && !niBgpPeerPayloadHasLeaves(niBgpPgKind, inParams, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, "timers/config") {
			return res, tlerr.InvalidArgsError{Format: "BGP peer-group timers config payload carries none of connect-retry, hold-time, keepalive-interval or minimum-advertisement-interval"}
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

// dbToYangNiBgpPgCfgBase reads the peer-group leaves niBgpPgLeafFlds leaves out.
func dbToYangNiBgpPgCfgBase(inParams XfmrParams) error {
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
		stateFamilies := niBgpPgAppliedGroups(inParams.dbs[db.StateDB], reqInfo.niName)[reqInfo.peerGroupName]
		return populateNiBgpPgStateReplay(inParams, wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, stateFamilies, false)
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
