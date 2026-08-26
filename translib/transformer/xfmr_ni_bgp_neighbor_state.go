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
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	// bgpmon publishes the BGP neighbor FSM state to STATE_DB as
	// NEIGH_STATE_TABLE|<neighbor-address>. The key has no network instance
	// component because bgpmon reports the default VRF only, so state is
	// looked up by address alone. Widening the key to carry the VRF is part of
	// the bgpmon work that introduces the per-neighbor counter tables.
	niBgpNbrStateTbl = "NEIGH_STATE_TABLE"
	niBgpNbrStateFld = "state"

	niBgpNbrSessionStateLeaf = "session-state"
)

var niBgpNbrStateReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx + `/state(?:/(session-state))?$`)

const niBgpNbrStateUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]` +
	`/protocols/protocol\[identifier=(?:[-A-Za-z0-9_.]+:)?BGP\]\[name=([^\]]+)\]` +
	`/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]`

type niBgpNbrStateReqInfo struct {
	niName       string
	neighborAddr string
	targetLeaf   string // "" for the whole state container, else the leaf name
}

func init() {
	XlateFuncBind("DbToYang_ni_bgp_nbr_state_xfmr", DbToYang_ni_bgp_nbr_state_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_nbr_state_xfmr", Subscribe_ni_bgp_nbr_state_xfmr)
}

func parseNiBgpNbrStateRequestUri(uri string) (*niBgpNbrStateReqInfo, error) {
	m := niBgpNbrStateReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance BGP neighbor state path: " + uri}
	}
	if m[2] != "bgp" {
		return nil, tlerr.NotSupportedError{
			Format: fmt.Sprintf("unsupported BGP protocol instance name %q for neighbor state; only \"bgp\" is mapped", m[2]),
		}
	}
	return &niBgpNbrStateReqInfo{
		niName:       m[1],
		neighborAddr: m[3],
		targetLeaf:   m[4],
	}, nil
}

// niBgpNbrSessionStateFromFrr maps the FSM state string that FRR reports in
// "show bgp summary json", and that bgpmon copies verbatim into STATE_DB, onto
// the OpenConfig session-state enumeration.
func niBgpNbrSessionStateFromFrr(state string) (ocbinds.E_OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState, bool) {
	s := strings.ToLower(strings.TrimSpace(state))
	switch {
	// FRR qualifies the down state with the reason it is down, for example
	// "Idle (Admin)" when the peer is shut down.
	case strings.HasPrefix(s, "idle"):
		return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_IDLE, true
	case s == "connect":
		return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_CONNECT, true
	case s == "active":
		return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_ACTIVE, true
	case s == "opensent":
		return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_OPENSENT, true
	case s == "openconfirm":
		return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_OPENCONFIRM, true
	case s == "established":
		return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_ESTABLISHED, true
	// Clearing and Deleted are FRR teardown states with no OpenConfig
	// counterpart. The session is down in both, which IDLE conveys.
	case s == "clearing", s == "deleted":
		return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_IDLE, true
	}
	return ocbinds.OpenconfigBgp_Bgp_Neighbors_Neighbor_State_SessionState_UNSET, false
}

// getNiBgpNbrObj locates the neighbor list entry of the BGP protocol instance
// in the YANG tree, creating the ancestors when asked to.
func getNiBgpNbrObj(s *ygot.GoStruct, niName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor {
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
		return niBgpNbrObjOfProtocol(pObj, neighborAddr, createIfMissing)
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
		return niBgpNbrObjOfProtocol(pObj, neighborAddr, createIfMissing)
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	return niBgpNbrObjOfProtocol(pObj, neighborAddr, createIfMissing)
}

func niBgpNbrObjOfProtocol(pObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor {
	buildNiBgpEmptySubtree(pObj)
	if nbrObj, ok := pObj.Bgp.Neighbors.Neighbor[neighborAddr]; ok && nbrObj != nil {
		buildNiBgpEmptySubtree(nbrObj)
		return nbrObj
	}
	if !createIfMissing {
		return nil
	}
	nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
	if err != nil || nbrObj == nil {
		return nil
	}
	buildNiBgpEmptySubtree(nbrObj)
	return nbrObj
}

func getNiBgpNbrStateObj(s *ygot.GoStruct, niName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State {
	nbrObj := getNiBgpNbrObj(s, niName, neighborAddr, createIfMissing)
	if nbrObj == nil {
		return nil
	}
	return nbrObj.State
}

var DbToYang_ni_bgp_nbr_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpNbrStateRequestUri(inParams.requestUri)
	if err != nil {
		if isNiBgpContainerLevelReqUri(inParams.requestUri) {
			// The request targets a node above the neighbor state subtree, so
			// the BGP container dispatch owns it and has already populated what
			// it maps. Reporting an error here would fail that request.
			return nil
		}
		return err
	}

	stateDb := inParams.dbs[db.StateDB]
	if stateDb == nil {
		return tlerr.InvalidArgsError{Format: "StateDB nil for network-instance BGP neighbor state"}
	}

	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpNbrStateTbl}, db.Key{Comp: []string{reqInfo.neighborAddr}})
	if err != nil {
		// A neighbor with no NEIGH_STATE_TABLE row has no published state yet,
		// which is the case before bgpmon takes its first sample. Leaving the
		// leaf unset keeps a sample subscription alive instead of failing it.
		if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); notPresent {
			return nil
		}
		return err
	}

	stateObj := getNiBgpNbrStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, true)
	if stateObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor state object in YANG tree"}
	}

	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrSessionStateLeaf {
		if raw := strings.TrimSpace(entry.Field[niBgpNbrStateFld]); raw != "" {
			sessionState, ok := niBgpNbrSessionStateFromFrr(raw)
			if !ok {
				return tlerr.InvalidArgsError{
					Format: fmt.Sprintf("unrecognized %s.%s value %q for neighbor %s", niBgpNbrStateTbl, niBgpNbrStateFld, raw, reqInfo.neighborAddr),
				}
			}
			stateObj.SessionState = sessionState
		}
	}

	return nil
}

var Subscribe_ni_bgp_nbr_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	pathInfo := NewPathInfo(inParams.uri)
	neighborAddr := pathInfo.Var("neighbor-address")
	keyed := neighborAddr != ""
	if !keyed {
		neighborAddr = "*"
	}

	result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrStateTbl: {neighborAddr: {
		niBgpNbrStateFld: niBgpNbrSessionStateLeaf}}}}
	result.isVirtualTbl = false

	if keyed {
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		return result, nil
	}

	// Without a bound neighbor-address the notification path carries a
	// wildcard key that has to be resolved back from the Redis key. That needs
	// a path transformer, because NEIGH_STATE_TABLE has no SONiC YANG model for
	// translib to convert the key directly. Offer sampling rather than an
	// on-change subscription that would only fail on the first notification.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
