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
	"time"

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

	// bgpmon also publishes the neighbor configuration as FRR actually applied
	// it, as BGP_NEIGH_APPLIED|<network-instance>|<neighbor-address>. Unlike
	// NEIGH_STATE_TABLE this key does carry the network instance, because the
	// scrape behind it covers every VRF FRR knows rather than the default one.
	//
	// These values deliberately come from FRR and not from CONFIG_DB. Reading
	// back what was written would only echo the request, and could not show
	// whether the daemon accepted it; for local_address it could not answer at
	// all, since FRR binds an address of its own choosing when none is
	// configured.
	niBgpNbrAppliedTbl = "BGP_NEIGH_APPLIED"

	niBgpNbrAppliedEnabledFld   = "enabled"
	niBgpNbrAppliedNbrAddrFld   = "neighbor_address"
	niBgpNbrAppliedPeerAsFld    = "peer_as"
	niBgpNbrAppliedPeerGrpFld   = "peer_group"
	niBgpNbrAppliedLocalAddrFld = "local_address"
	niBgpNbrAppliedRmtAddrFld   = "remote_address"

	niBgpNbrAppliedMetaTbl     = "BGP_NEIGH_APPLIED_META"
	niBgpNbrAppliedMetaKey     = "global"
	niBgpNbrAppliedLastSyncFld = "last_sync"

	// bgpmon rescrapes every 15 seconds. Four intervals is loose enough that
	// one slow or missed scrape does not hide the applied values, and tight
	// enough that a dead producer is not reported as current.
	niBgpNbrAppliedMaxAge = 60 * time.Second

	niBgpNbrSessionStateLeaf = "session-state"
	niBgpNbrEnabledLeaf      = "enabled"
	niBgpNbrNbrAddrLeaf      = "neighbor-address"
	niBgpNbrPeerAsLeaf       = "peer-as"
	niBgpNbrPeerGrpLeaf      = "peer-group"
	niBgpNbrLocalAddrLeaf    = "local-address"
	niBgpNbrRmtAddrLeaf      = "remote-address"

	niBgpNbrStateSubtree     = "state"
	niBgpNbrTransportSubtree = "transport"
)

var niBgpNbrStateReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/state(?:/(session-state|enabled|neighbor-address|peer-as|peer-group))?$`)

// The transport state leaves sit in a sibling container of state, so they need
// their own pattern. Both the transport container and its state container are
// matched so that a request anchored at either is served rather than falling
// back to the BGP container dispatch, which maps CONFIG_DB only.
var niBgpNbrTransportReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/transport(?:/state(?:/(local-address|remote-address))?)?$`)

const niBgpNbrStateUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]` +
	`/protocols/protocol\[identifier=(?:[-A-Za-z0-9_.]+:)?BGP\]\[name=([^\]]+)\]` +
	`/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]`

// niBgpNbrAppliedStateLeaves are the leaves of the state container that come
// from BGP_NEIGH_APPLIED rather than from NEIGH_STATE_TABLE.
var niBgpNbrAppliedStateLeaves = map[string]bool{
	niBgpNbrEnabledLeaf: true,
	niBgpNbrNbrAddrLeaf: true,
	niBgpNbrPeerAsLeaf:  true,
	niBgpNbrPeerGrpLeaf: true,
}

type niBgpNbrStateReqInfo struct {
	niName       string
	neighborAddr string
	targetLeaf   string // "" for the whole container, else the leaf name
	subtree      string // niBgpNbrStateSubtree or niBgpNbrTransportSubtree
}

func init() {
	XlateFuncBind("DbToYang_ni_bgp_nbr_state_xfmr", DbToYang_ni_bgp_nbr_state_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_nbr_state_xfmr", Subscribe_ni_bgp_nbr_state_xfmr)
}

func parseNiBgpNbrStateRequestUri(uri string) (*niBgpNbrStateReqInfo, error) {
	subtree := niBgpNbrStateSubtree
	m := niBgpNbrStateReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		subtree = niBgpNbrTransportSubtree
		m = niBgpNbrTransportReqUriRe.FindStringSubmatch(uri)
	}
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
		subtree:      subtree,
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

// niBgpNbrAppliedIsFresh reports whether bgpmon's last successful scrape is
// recent enough for the applied values to be reported at all.
//
// The producer omits a field entirely when FRR has no value for it, which is
// what makes an absent OpenConfig leaf mean "FRR is not applying anything
// here". That only holds while the producer is actually sampling: rows left
// behind by a producer that died an hour ago would otherwise be served as if
// current, and a configuration acceptance check comparing them against what was
// just written would report a rejection that never happened.
//
// A stale table suppresses the applied leaves rather than failing the request,
// for the same reason an absent NEIGH_STATE_TABLE row does not fail one: a
// sample subscription has to survive a producer restart. This is deliberately
// not logged, because the condition would repeat on every sample of every
// subscription. A client that needs to tell "FRR has no value" apart from "the
// producer is behind" reads BGP_NEIGH_APPLIED_META|global over the STATE_DB
// target, where the marker is a plain epoch timestamp.
func niBgpNbrAppliedIsFresh(stateDb *db.DB) bool {
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpNbrAppliedMetaTbl}, db.Key{Comp: []string{niBgpNbrAppliedMetaKey}})
	if err != nil {
		// No marker at all means bgpmon has not completed a scrape since it
		// last started, so there is nothing trustworthy to report.
		return false
	}
	raw := strings.TrimSpace(entry.Field[niBgpNbrAppliedLastSyncFld])
	if raw == "" {
		return false
	}
	epoch, convErr := strconv.ParseInt(raw, 10, 64)
	if convErr != nil {
		return false
	}
	age := time.Since(time.Unix(epoch, 0))
	// A marker dated in the future means the clock was stepped, not that the
	// producer is behind. Hiding live data over a time adjustment would be the
	// worse failure.
	if age < 0 {
		return true
	}
	return age <= niBgpNbrAppliedMaxAge
}

// niBgpNbrAppliedEntry reads one neighbor's applied row. A neighbor that the
// producer has never reported is reported as absent rather than as an error,
// since that is the normal state before the first scrape completes.
func niBgpNbrAppliedEntry(stateDb *db.DB, niName, neighborAddr string) (db.Value, bool, error) {
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpNbrAppliedTbl}, db.Key{Comp: []string{niName, neighborAddr}})
	if err != nil {
		if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); notPresent {
			return db.Value{}, false, nil
		}
		return db.Value{}, false, err
	}
	return entry, true, nil
}

// niBgpNbrAppliedFresh returns the applied row for a neighbor, or reports it as
// absent when the producer is behind.
func niBgpNbrAppliedFresh(stateDb *db.DB, niName, neighborAddr string) (db.Value, bool, error) {
	if !niBgpNbrAppliedIsFresh(stateDb) {
		return db.Value{}, false, nil
	}
	return niBgpNbrAppliedEntry(stateDb, niName, neighborAddr)
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

func getNiBgpNbrTransportStateObj(s *ygot.GoStruct, niName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Transport_State {
	nbrObj := getNiBgpNbrObj(s, niName, neighborAddr, createIfMissing)
	if nbrObj == nil || nbrObj.Transport == nil {
		return nil
	}
	return nbrObj.Transport.State
}

// niBgpNbrAppliedStr returns a trimmed applied field, and whether it carries a
// value. An empty field is treated as absent, so that a leaf is left unset
// rather than reported as an empty string.
func niBgpNbrAppliedStr(entry db.Value, field string) (string, bool) {
	raw := strings.TrimSpace(entry.Field[field])
	return raw, raw != ""
}

// populateNiBgpNbrAppliedStateLeaves fills the state container leaves that come
// from the applied row. Only the requested leaf is filled when the request
// targets one, so that a leaf request does not report its siblings.
func populateNiBgpNbrAppliedStateLeaves(stateObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State, entry db.Value, targetLeaf, neighborAddr string) error {
	wants := func(leaf string) bool { return targetLeaf == "" || targetLeaf == leaf }

	if wants(niBgpNbrEnabledLeaf) {
		if raw, ok := niBgpNbrAppliedStr(entry, niBgpNbrAppliedEnabledFld); ok {
			enabled, convErr := strconv.ParseBool(raw)
			if convErr != nil {
				return tlerr.InvalidArgsError{
					Format: fmt.Sprintf("unrecognized %s.%s value %q for neighbor %s", niBgpNbrAppliedTbl, niBgpNbrAppliedEnabledFld, raw, neighborAddr),
				}
			}
			stateObj.Enabled = &enabled
		}
	}
	if wants(niBgpNbrNbrAddrLeaf) {
		if raw, ok := niBgpNbrAppliedStr(entry, niBgpNbrAppliedNbrAddrFld); ok {
			addr := raw
			stateObj.NeighborAddress = &addr
		}
	}
	if wants(niBgpNbrPeerAsLeaf) {
		if raw, ok := niBgpNbrAppliedStr(entry, niBgpNbrAppliedPeerAsFld); ok {
			asVal, convErr := strconv.ParseUint(raw, 10, 32)
			if convErr != nil {
				return tlerr.InvalidArgsError{
					Format: fmt.Sprintf("unrecognized %s.%s value %q for neighbor %s", niBgpNbrAppliedTbl, niBgpNbrAppliedPeerAsFld, raw, neighborAddr),
				}
			}
			peerAs := uint32(asVal)
			stateObj.PeerAs = &peerAs
		}
	}
	if wants(niBgpNbrPeerGrpLeaf) {
		if raw, ok := niBgpNbrAppliedStr(entry, niBgpNbrAppliedPeerGrpFld); ok {
			pg := raw
			stateObj.PeerGroup = &pg
		}
	}
	return nil
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

	if reqInfo.subtree == niBgpNbrTransportSubtree {
		return populateNiBgpNbrTransportStateFromDb(inParams, stateDb, reqInfo)
	}
	return populateNiBgpNbrStateFromDb(inParams, stateDb, reqInfo)
}

// populateNiBgpNbrStateFromDb fills the state container. Its leaves come from
// two independent tables, so neither lookup may short circuit the other: a
// neighbor can have a session state published before its applied
// configuration, and an applied row exists for neighbors in VRFs that
// NEIGH_STATE_TABLE does not cover at all.
func populateNiBgpNbrStateFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpNbrStateReqInfo) error {
	sessionRaw := ""
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrSessionStateLeaf {
		entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpNbrStateTbl}, db.Key{Comp: []string{reqInfo.neighborAddr}})
		if err != nil {
			// A neighbor with no NEIGH_STATE_TABLE row has no published state
			// yet, which is the case before bgpmon takes its first sample.
			// Leaving the leaf unset keeps a sample subscription alive instead
			// of failing it.
			if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); !notPresent {
				return err
			}
		} else {
			sessionRaw = strings.TrimSpace(entry.Field[niBgpNbrStateFld])
		}
	}

	var appliedEntry db.Value
	appliedFound := false
	if reqInfo.targetLeaf == "" || niBgpNbrAppliedStateLeaves[reqInfo.targetLeaf] {
		entry, found, err := niBgpNbrAppliedFresh(stateDb, reqInfo.niName, reqInfo.neighborAddr)
		if err != nil {
			return err
		}
		appliedEntry, appliedFound = entry, found
	}

	if sessionRaw == "" && !appliedFound {
		return nil
	}

	stateObj := getNiBgpNbrStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, true)
	if stateObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor state object in YANG tree"}
	}

	if sessionRaw != "" {
		sessionState, ok := niBgpNbrSessionStateFromFrr(sessionRaw)
		if !ok {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("unrecognized %s.%s value %q for neighbor %s", niBgpNbrStateTbl, niBgpNbrStateFld, sessionRaw, reqInfo.neighborAddr),
			}
		}
		stateObj.SessionState = sessionState
	}
	if appliedFound {
		return populateNiBgpNbrAppliedStateLeaves(stateObj, appliedEntry, reqInfo.targetLeaf, reqInfo.neighborAddr)
	}
	return nil
}

func populateNiBgpNbrTransportStateFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpNbrStateReqInfo) error {
	entry, found, err := niBgpNbrAppliedFresh(stateDb, reqInfo.niName, reqInfo.neighborAddr)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	localAddr, hasLocal := niBgpNbrAppliedStr(entry, niBgpNbrAppliedLocalAddrFld)
	remoteAddr, hasRemote := niBgpNbrAppliedStr(entry, niBgpNbrAppliedRmtAddrFld)
	wantLocal := (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrLocalAddrLeaf) && hasLocal
	wantRemote := (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrRmtAddrLeaf) && hasRemote
	if !wantLocal && !wantRemote {
		return nil
	}

	transportObj := getNiBgpNbrTransportStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, true)
	if transportObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor transport state object in YANG tree"}
	}
	if wantLocal {
		addr := localAddr
		transportObj.LocalAddress = &addr
	}
	if wantRemote {
		addr := remoteAddr
		transportObj.RemoteAddress = &addr
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
	niName := strings.TrimSpace(pathInfo.Var("name"))
	if niName == "" {
		niName = "*"
	}
	appliedKey := niName + "|" + neighborAddr

	result.isVirtualTbl = false

	// The subscribe path trims a trailing leaf off the subtree uri before it
	// calls this callback, so a subscription on state/session-state and one on
	// the state container both arrive with a uri ending in "/state". Which leaf
	// was actually asked for survives only in requestURI, and reading it from
	// inParams.uri instead silently downgrades session-state to sampling.
	//
	// The CRUD parent-table check reaches this callback with requestURI unset
	// and an untrimmed uri, hence the fallback rather than a bare read.
	leafUri := inParams.requestURI
	if leafUri == "" {
		leafUri = inParams.uri
	}

	// A subscription anchored on session-state alone reads NEIGH_STATE_TABLE
	// only, and keeps the on-change support it was given when that was the one
	// mapped leaf.
	if strings.HasSuffix(strings.TrimSuffix(leafUri, "/"), "/"+niBgpNbrSessionStateLeaf) {
		result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrStateTbl: {neighborAddr: {
			niBgpNbrStateFld: niBgpNbrSessionStateLeaf}}}}
		if keyed {
			result.onChange = OnchangeEnable
			result.nOpts = &notificationOpts{pType: OnChange}
			return result, nil
		}
		// Without a bound neighbor-address the notification path carries a
		// wildcard key that has to be resolved back from the Redis key. That
		// needs a path transformer, because NEIGH_STATE_TABLE has no SONiC YANG
		// model for translib to convert the key directly. Offer sampling rather
		// than an on-change subscription that would only fail on the first
		// notification.
		result.onChange = OnchangeDisable
		result.nOpts = &notificationOpts{pType: Sample}
		return result, nil
	}

	if strings.Contains(inParams.uri, "/"+niBgpNbrTransportSubtree) {
		result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrAppliedTbl: {appliedKey: {}}}}
	} else {
		result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {
			niBgpNbrStateTbl:   {neighborAddr: {niBgpNbrStateFld: niBgpNbrSessionStateLeaf}},
			niBgpNbrAppliedTbl: {appliedKey: {}},
		}}
	}

	// Anything reading BGP_NEIGH_APPLIED is offered sampling only. The producer
	// rewrites a changed row as a delete followed by a set, so that a field FRR
	// has stopped reporting disappears instead of going stale; on-change would
	// turn each of those rewrites into a spurious delete notification. Making
	// these paths on-change capable is a producer change, replacing the rewrite
	// with a set plus an explicit delete of the fields that went away.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
