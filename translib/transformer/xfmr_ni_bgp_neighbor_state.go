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
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/golang/glog"

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
	// The negotiated capabilities, as a list; the trailing @ is SONiC's
	// mark for a field holding one.
	niBgpNbrAppliedSuppCapsFld = "supported_capabilities@"

	// The per-session counters come from the same scrape as the applied row,
	// published as BGP_NEIGH_STATS|<network-instance>|<neighbor-address>. They
	// are a separate table because they change on nearly every scrape while the
	// applied values sit still, and keeping them apart stops a counter tick
	// from rewriting the applied row.
	niBgpNbrStatsTbl = "BGP_NEIGH_STATS"

	niBgpNbrStatsEstTransFld         = "established_transitions"
	niBgpNbrStatsLastEstFld          = "last_established"
	niBgpNbrStatsUpdRcvdFld          = "updates_received"
	niBgpNbrStatsUpdSentFld          = "updates_sent"
	niBgpNbrStatsLastNotifErrCodeFld = "last_notification_error_code"

	niBgpNbrLastNotifErrCodeLeaf = "last-notification-error-code"

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
	niBgpNbrSuppCapsLeaf     = "supported-capabilities"
	niBgpNbrEstTransLeaf     = "established-transitions"
	niBgpNbrLastEstLeaf      = "last-established"
	niBgpNbrUpdateLeaf       = "UPDATE"

	niBgpNbrSentDir = "sent"
	niBgpNbrRcvdDir = "received"

	niBgpNbrStateSubtree     = "state"
	niBgpNbrTransportSubtree = "transport"
	niBgpNbrMessagesSubtree  = "messages"

	// last-established is oc-types:timeticks64, which counts nanoseconds since
	// the epoch, while FRR reports whole seconds. The producer stores what FRR
	// gave it rather than a scaled value, so the scale factor is applied here.
	niBgpNbrLastEstNsPerSec = uint64(time.Second / time.Nanosecond)
)

var niBgpNbrStateReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/state(?:/(session-state|enabled|neighbor-address|peer-as|peer-group|established-transitions|last-established|supported-capabilities))?$`)

// The message counters sit two levels below the state container, split by
// direction, so they need a pattern of their own rather than another
// alternative in the state leaf list. Both the messages container and each
// direction container are matched so that a request anchored at any of them is
// served the counters below it.
var niBgpNbrMessagesReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/state/messages(?:/(sent|received)(?:/(UPDATE|last-notification-error-code))?)?$`)

// The transport state leaves sit in a sibling container of state, so they need
// their own pattern. Both the transport container and its state container are
// matched so that a request anchored at either is served rather than falling
// back to the BGP container dispatch, which maps CONFIG_DB only.
var niBgpNbrTransportReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/transport(?:/state(?:/(local-address|remote-address))?)?$`)

const niBgpNbrStateUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]` +
	`/protocols/protocol\[identifier=(?:[-A-Za-z0-9_.]+:)?BGP\]\[name=([^\]]+)\]` +
	`/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]`

// niBgpNbrAppliedFldForLeaf maps the leaves of the state container that come
// from BGP_NEIGH_APPLIED, rather than from NEIGH_STATE_TABLE, onto their
// producer field. The transport leaves are not listed because they hang below
// the transport container rather than off the state container.
var niBgpNbrAppliedFldForLeaf = map[string]string{
	niBgpNbrEnabledLeaf:  niBgpNbrAppliedEnabledFld,
	niBgpNbrNbrAddrLeaf:  niBgpNbrAppliedNbrAddrFld,
	niBgpNbrPeerAsLeaf:   niBgpNbrAppliedPeerAsFld,
	niBgpNbrPeerGrpLeaf:  niBgpNbrAppliedPeerGrpFld,
	niBgpNbrSuppCapsLeaf: niBgpNbrAppliedSuppCapsFld,
}

// niBgpNbrAppliedFldForTransportLeaf maps the transport state leaves onto their
// producer field. They are kept apart from the state container leaves above
// because they hang below the transport container rather than off state.
var niBgpNbrAppliedFldForTransportLeaf = map[string]string{
	niBgpNbrLocalAddrLeaf: niBgpNbrAppliedLocalAddrFld,
	niBgpNbrRmtAddrLeaf:   niBgpNbrAppliedRmtAddrFld,
}

// niBgpNbrStatsFldForLeaf maps the leaves of the state container that come from
// BGP_NEIGH_STATS onto their producer field. The message counters are not
// listed because they hang below the messages container rather than off the
// state container directly.
var niBgpNbrStatsFldForLeaf = map[string]string{
	niBgpNbrEstTransLeaf: niBgpNbrStatsEstTransFld,
	niBgpNbrLastEstLeaf:  niBgpNbrStatsLastEstFld,
}

type niBgpNbrStateReqInfo struct {
	niName       string
	protoName    string
	neighborAddr string
	targetLeaf   string // "" for the whole container, else the leaf name
	subtree      string // niBgpNbrStateSubtree, niBgpNbrTransportSubtree or niBgpNbrMessagesSubtree
	direction    string // messages subtree only: "" for both, else niBgpNbrSentDir or niBgpNbrRcvdDir
}

func init() {
	XlateFuncBind("DbToYang_ni_bgp_nbr_state_xfmr", DbToYang_ni_bgp_nbr_state_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_nbr_state_xfmr", Subscribe_ni_bgp_nbr_state_xfmr)
}

func parseNiBgpNbrStateRequestUri(uri string) (*niBgpNbrStateReqInfo, error) {
	subtree := niBgpNbrStateSubtree
	direction := ""
	m := niBgpNbrStateReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		subtree = niBgpNbrTransportSubtree
		m = niBgpNbrTransportReqUriRe.FindStringSubmatch(uri)
	}
	if m == nil {
		subtree = niBgpNbrMessagesSubtree
		if m = niBgpNbrMessagesReqUriRe.FindStringSubmatch(uri); m != nil {
			// The messages pattern carries the direction ahead of the leaf, so
			// its captures are one wider than the other two.
			direction, m = m[4], []string{m[0], m[1], m[2], m[3], m[5]}
		}
	}
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance BGP neighbor state path: " + uri}
	}
	if err := validateNiBgpProtoName(m[2]); err != nil {
		return nil, err
	}
	return &niBgpNbrStateReqInfo{
		niName:       m[1],
		protoName:    m[2],
		neighborAddr: m[3],
		targetLeaf:   m[4],
		subtree:      subtree,
		direction:    direction,
	}, nil
}

// niBgpNbrSessionStateFromFrr maps the FSM state string that FRR reports in
// "show bgp summary json", and that bgpmon copies verbatim into STATE_DB, onto
// the OpenConfig session-state enumeration.
func niBgpNbrLastNotifErrCodeFromFrr(code uint8) (ocbinds.E_OpenconfigBgpTypes_BGP_ERROR_CODE, bool) {
	switch code {
	case 1:
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_CODE_MESSAGE_HEADER_ERROR, true
	case 2:
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_CODE_OPEN_MESSAGE_ERROR, true
	case 3:
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_CODE_UPDATE_MESSAGE_ERROR, true
	case 4:
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_CODE_HOLD_TIMER_EXPIRED, true
	case 5:
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_CODE_FINITE_STATE_MACHINE_ERROR, true
	case 6:
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_CODE_CEASE, true
	default:
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_CODE_UNSET, false
	}
}

func niBgpNbrSessionStateFromFrr(state string) (ocbinds.E_OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState, bool) {
	s := strings.ToLower(strings.TrimSpace(state))
	switch {
	// FRR qualifies the down state with the reason it is down, for example
	// "Idle (Admin)" when the peer is shut down.
	case strings.HasPrefix(s, "idle"):
		return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_IDLE, true
	case s == "connect":
		return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_CONNECT, true
	case s == "active":
		return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_ACTIVE, true
	case s == "opensent":
		return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_OPENSENT, true
	case s == "openconfirm":
		return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_OPENCONFIRM, true
	case s == "established":
		return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_ESTABLISHED, true
	// Clearing and Deleted are FRR teardown states with no OpenConfig
	// counterpart. The session is down in both, which IDLE conveys.
	case s == "clearing", s == "deleted":
		return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_IDLE, true
	}
	return ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_SessionState_UNSET, false
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

// niBgpNbrStatsFresh returns the counter row for a neighbor. It is gated on the
// same marker as the applied row because one scrape writes both tables, so a
// marker that has gone stale has left both behind. Serving counters past that
// point would report a frozen number as a live one, which is worse here than
// for the applied values: a client watching for update activity cannot tell a
// quiet session from a dead producer.
func niBgpNbrStatsFresh(stateDb *db.DB, niName, neighborAddr string) (db.Value, bool, error) {
	if !niBgpNbrAppliedIsFresh(stateDb) {
		return db.Value{}, false, nil
	}
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpNbrStatsTbl}, db.Key{Comp: []string{niName, neighborAddr}})
	if err != nil {
		if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); notPresent {
			return db.Value{}, false, nil
		}
		return db.Value{}, false, err
	}
	return entry, true, nil
}

// niBgpNbrStatsUint reads one counter field. An absent field means FRR reported
// no value, which leaves the leaf unset; a zero is a real count and is
// reported. A field that is present but not a number is a producer fault rather
// than a missing value, so it fails the request instead of being dropped.
func niBgpNbrStatsUint(entry db.Value, field, neighborAddr string) (uint64, bool, error) {
	raw, ok := niBgpNbrAppliedStr(entry, field)
	if !ok {
		return 0, false, nil
	}
	val, convErr := strconv.ParseUint(raw, 10, 64)
	if convErr != nil {
		return 0, false, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("unrecognized %s.%s value %q for neighbor %s", niBgpNbrStatsTbl, field, raw, neighborAddr),
		}
	}
	return val, true, nil
}

// getNiBgpNbrObj locates the neighbor list entry of the BGP protocol instance
// in the YANG tree, creating the ancestors when asked to.
func getNiBgpNbrObj(s *ygot.GoStruct, niName, protoName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor {
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
		return niBgpNbrObjOfProtocol(pObj, neighborAddr, createIfMissing)
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Neighbors == nil {
			continue
		}
		if !strings.Contains(fmt.Sprint(pKey.Identifier), "BGP") {
			continue
		}
		if pKey.Name != protoName {
			if pObj.Name == nil || *pObj.Name != protoName {
				continue
			}
		}
		return niBgpNbrObjOfProtocol(pObj, neighborAddr, createIfMissing)
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, protoName)
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

func getNiBgpNbrStateObj(s *ygot.GoStruct, niName, protoName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State {
	nbrObj := getNiBgpNbrObj(s, niName, protoName, neighborAddr, createIfMissing)
	if nbrObj == nil {
		return nil
	}
	return nbrObj.State
}

func getNiBgpNbrTransportStateObj(s *ygot.GoStruct, niName, protoName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Transport_State {
	nbrObj := getNiBgpNbrObj(s, niName, protoName, neighborAddr, createIfMissing)
	if nbrObj == nil || nbrObj.Transport == nil {
		return nil
	}
	return nbrObj.Transport.State
}

// niBgpNbrLastEstToNanos scales FRR's epoch seconds to the nanoseconds the leaf
// is defined in. A value large enough to overflow the multiplication is a
// producer fault, and is reported rather than wrapped into a plausible-looking
// timestamp.
func niBgpNbrLastEstToNanos(epochSecs uint64, neighborAddr string) (uint64, error) {
	if epochSecs > math.MaxUint64/niBgpNbrLastEstNsPerSec {
		return 0, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("%s.%s value %d for neighbor %s does not fit a nanosecond timestamp",
				niBgpNbrStatsTbl, niBgpNbrStatsLastEstFld, epochSecs, neighborAddr),
		}
	}
	return epochSecs * niBgpNbrLastEstNsPerSec, nil
}

// niBgpNbrCapabilityFromToken maps one element of the producer's capability
// list onto the OpenConfig identity it names.
//
// openconfig-bgp-types defines six identities for BGP_CAPABILITY and no more,
// upstream master included, so this leaf-list cannot express anything else. A
// session may well negotiate a capability outside them, extendedMessage and
// enhancedRouteRefresh being the common ones, and the producer drops those
// because the model gives them nowhere to go. That is a limit of the model
// rather than of the mapping, and it is why an unrecognised element is skipped
// rather than refused: the alternative denies a caller the capabilities that
// can be named on account of one that cannot.
func niBgpNbrCapabilityFromToken(token string) (ocbinds.E_OpenconfigBgpTypes_BGP_CAPABILITY, bool) {
	switch strings.ToUpper(strings.TrimSpace(token)) {
	case "MPBGP":
		return ocbinds.OpenconfigBgpTypes_BGP_CAPABILITY_MPBGP, true
	case "ROUTE_REFRESH":
		return ocbinds.OpenconfigBgpTypes_BGP_CAPABILITY_ROUTE_REFRESH, true
	case "ASN32":
		return ocbinds.OpenconfigBgpTypes_BGP_CAPABILITY_ASN32, true
	case "GRACEFUL_RESTART":
		return ocbinds.OpenconfigBgpTypes_BGP_CAPABILITY_GRACEFUL_RESTART, true
	case "ADD_PATHS":
		return ocbinds.OpenconfigBgpTypes_BGP_CAPABILITY_ADD_PATHS, true
	case "EXTENDED_NEXTHOP_ENCODING":
		return ocbinds.OpenconfigBgpTypes_BGP_CAPABILITY_EXTENDED_NEXTHOP_ENCODING, true
	}
	return ocbinds.OpenconfigBgpTypes_BGP_CAPABILITY_UNSET, false
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
	if wants(niBgpNbrSuppCapsLeaf) {
		if raw, ok := niBgpNbrAppliedStr(entry, niBgpNbrAppliedSuppCapsFld); ok {
			var caps []ocbinds.E_OpenconfigBgpTypes_BGP_CAPABILITY
			for _, token := range strings.Split(raw, ",") {
				if strings.TrimSpace(token) == "" {
					continue
				}
				capability, ok := niBgpNbrCapabilityFromToken(token)
				if !ok {
					// Logged rather than returned: the capabilities that do map
					// are still worth reporting, and this is how a producer
					// writing a name this build does not know becomes visible.
					log.Warningf("%s.%s carries element %q for neighbor %s, which is not an OpenConfig BGP capability identity; skipping it",
						niBgpNbrAppliedTbl, niBgpNbrAppliedSuppCapsFld, token, neighborAddr)
					continue
				}
				caps = append(caps, capability)
			}
			if len(caps) > 0 {
				stateObj.SupportedCapabilities = caps
			}
		}
	}
	return nil
}

// populateNiBgpNbrStatsStateLeaves fills the counter leaves that hang off the
// state container directly. The message counters are filled separately because
// they sit below the messages container.
func populateNiBgpNbrStatsStateLeaves(stateObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State, entry db.Value, targetLeaf, neighborAddr string) error {
	wants := func(leaf string) bool { return targetLeaf == "" || targetLeaf == leaf }

	if wants(niBgpNbrEstTransLeaf) {
		val, ok, err := niBgpNbrStatsUint(entry, niBgpNbrStatsEstTransFld, neighborAddr)
		if err != nil {
			return err
		}
		if ok {
			transitions := val
			stateObj.EstablishedTransitions = &transitions
		}
	}
	if wants(niBgpNbrLastEstLeaf) {
		val, ok, err := niBgpNbrStatsUint(entry, niBgpNbrStatsLastEstFld, neighborAddr)
		if err != nil {
			return err
		}
		if ok {
			// FRR drops this field while the session is down, so the leaf is
			// absent rather than reporting the last time it came up.
			nanos, convErr := niBgpNbrLastEstToNanos(val, neighborAddr)
			if convErr != nil {
				return convErr
			}
			stateObj.LastEstablished = &nanos
		}
	}
	return nil
}

// populateNiBgpNbrMessageCounters fills the UPDATE counters below the messages
// container. reqInfo narrows what is filled: a direction limits it to one of the
// two direction containers, and a target leaf to that counter alone.
//
// The state object is looked up only once a counter is known to be present, so
// that a neighbor whose row carries no message counters is not reported as an
// empty state container.
func populateNiBgpNbrLastNotificationErrorCode(inParams XfmrParams, entry db.Value, reqInfo *niBgpNbrStateReqInfo) error {
	if reqInfo.targetLeaf != "" && reqInfo.targetLeaf != niBgpNbrLastNotifErrCodeLeaf {
		return nil
	}
	if reqInfo.direction != "" && reqInfo.direction != niBgpNbrRcvdDir {
		return nil
	}
	codeStr, ok := entry.Field[niBgpNbrStatsLastNotifErrCodeFld]
	if !ok || strings.TrimSpace(codeStr) == "" {
		return nil
	}
	codeVal, err := strconv.ParseUint(strings.TrimSpace(codeStr), 10, 8)
	if err != nil {
		return tlerr.InvalidArgsError{Format: fmt.Sprintf("unrecognized BGP_NEIGH_STATS.%s value %q for neighbor %s", niBgpNbrStatsLastNotifErrCodeFld, codeStr, reqInfo.neighborAddr)}
	}
	ocCode, ok := niBgpNbrLastNotifErrCodeFromFrr(uint8(codeVal))
	if !ok {
		return nil
	}
	stateObj := getNiBgpNbrStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
	if stateObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor state object in YANG tree"}
	}
	if stateObj.Messages == nil {
		stateObj.Messages = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_Messages{}
	}
	if stateObj.Messages.Received == nil {
		stateObj.Messages.Received = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_Messages_Received{}
	}
	stateObj.Messages.Received.LastNotificationErrorCode = ocCode
	return nil
}

func populateNiBgpNbrMessageCounters(inParams XfmrParams, entry db.Value, reqInfo *niBgpNbrStateReqInfo) error {
	if reqInfo.targetLeaf != "" && reqInfo.targetLeaf != niBgpNbrUpdateLeaf {
		return nil
	}

	sent, hasSent, err := niBgpNbrStatsUint(entry, niBgpNbrStatsUpdSentFld, reqInfo.neighborAddr)
	if err != nil {
		return err
	}
	rcvd, hasRcvd, err := niBgpNbrStatsUint(entry, niBgpNbrStatsUpdRcvdFld, reqInfo.neighborAddr)
	if err != nil {
		return err
	}

	wantSent := (reqInfo.direction == "" || reqInfo.direction == niBgpNbrSentDir) && hasSent
	wantRcvd := (reqInfo.direction == "" || reqInfo.direction == niBgpNbrRcvdDir) && hasRcvd
	if !wantSent && !wantRcvd {
		return nil
	}

	stateObj := getNiBgpNbrStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
	if stateObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor state object in YANG tree"}
	}
	if stateObj.Messages == nil {
		stateObj.Messages = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_Messages{}
	}
	if wantSent {
		if stateObj.Messages.Sent == nil {
			stateObj.Messages.Sent = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_Messages_Sent{}
		}
		updates := sent
		stateObj.Messages.Sent.UPDATE = &updates
	}
	if wantRcvd {
		if stateObj.Messages.Received == nil {
			stateObj.Messages.Received = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_State_Messages_Received{}
		}
		updates := rcvd
		stateObj.Messages.Received.UPDATE = &updates
	}
	return nil
}

var DbToYang_ni_bgp_nbr_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpNbrStateRequestUri(inParams.requestUri)
	if err != nil {
		fallback, nameErr := niBgpContainerLevelFallback(inParams.requestUri)
		if nameErr != nil {
			return nameErr
		}
		if fallback {
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

	switch reqInfo.subtree {
	case niBgpNbrTransportSubtree:
		return populateNiBgpNbrTransportStateFromDb(inParams, stateDb, reqInfo)
	case niBgpNbrMessagesSubtree:
		return populateNiBgpNbrMessagesFromDb(inParams, stateDb, reqInfo)
	}
	return populateNiBgpNbrStateFromDb(inParams, stateDb, reqInfo)
}

func populateNiBgpNbrMessagesFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpNbrStateReqInfo) error {
	entry, found, err := niBgpNbrStatsFresh(stateDb, reqInfo.niName, reqInfo.neighborAddr)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := populateNiBgpNbrMessageCounters(inParams, entry, reqInfo); err != nil {
		return err
	}
	return populateNiBgpNbrLastNotificationErrorCode(inParams, entry, reqInfo)
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
	appliedFld, appliedLeafTargeted := niBgpNbrAppliedFldForLeaf[reqInfo.targetLeaf]
	if reqInfo.targetLeaf == "" || appliedLeafTargeted {
		entry, found, err := niBgpNbrAppliedFresh(stateDb, reqInfo.niName, reqInfo.neighborAddr)
		if err != nil {
			return err
		}
		appliedEntry, appliedFound = entry, found

		// A request for one applied leaf that FRR is not reporting must not
		// reach the point of creating the state container, for the same reason
		// as the counters below: translib fills a targeted leaf with its type's
		// zero value once the container exists, so a peer-group that the
		// producer omitted because the peer is in no group would be reported as
		// the empty string rather than as absent.
		if appliedFound && appliedLeafTargeted {
			if _, present := niBgpNbrAppliedStr(entry, appliedFld); !present {
				appliedFound = false
			}
		}
	}

	var statsEntry db.Value
	statsFound := false
	statsFld, statsLeafTargeted := niBgpNbrStatsFldForLeaf[reqInfo.targetLeaf]
	if reqInfo.targetLeaf == "" || statsLeafTargeted {
		entry, found, err := niBgpNbrStatsFresh(stateDb, reqInfo.niName, reqInfo.neighborAddr)
		if err != nil {
			return err
		}
		statsEntry, statsFound = entry, found

		// A request for one counter that FRR is not reporting must not reach
		// the point of creating the state container. translib fills a targeted
		// leaf with its type's zero value once the container exists, so a
		// last-established that the producer omitted because the session has
		// never come up would be reported as the epoch rather than as absent.
		if statsFound && statsLeafTargeted {
			if _, present := niBgpNbrAppliedStr(entry, statsFld); !present {
				statsFound = false
			}
		}
	}

	if sessionRaw == "" && !appliedFound && !statsFound {
		return nil
	}

	stateObj := getNiBgpNbrStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
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
		if err := populateNiBgpNbrAppliedStateLeaves(stateObj, appliedEntry, reqInfo.targetLeaf, reqInfo.neighborAddr); err != nil {
			return err
		}
	}
	if statsFound {
		if err := populateNiBgpNbrStatsStateLeaves(stateObj, statsEntry, reqInfo.targetLeaf, reqInfo.neighborAddr); err != nil {
			return err
		}
		// A request for the whole state container has to reach the counters
		// below messages as well, since no separate callback runs for them.
		if reqInfo.targetLeaf == "" {
			if err := populateNiBgpNbrMessageCounters(inParams, statsEntry, reqInfo); err != nil {
				return err
			}
			return populateNiBgpNbrLastNotificationErrorCode(inParams, statsEntry, reqInfo)
		}
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

	transportObj := getNiBgpNbrTransportStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, true)
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

	// The protocol instance name is part of no watched key, so without this an
	// unserved spelling would be given a working watch on a path that a GET
	// refuses.
	if err := validateNiBgpProtoNameInUri(inParams.uri); err != nil {
		return result, err
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
	// BGP_NEIGH_APPLIED and BGP_NEIGH_STATS are both keyed by network instance
	// and neighbor, unlike NEIGH_STATE_TABLE which is keyed by address alone.
	nbrKey := niName + "|" + neighborAddr

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

	// Which leaf the request is anchored on, if any. The state container mixes
	// three tables, so the leaf decides which of them has to be watched, and
	// with it whether the path can be served on-change.
	anchoredUri := strings.TrimSuffix(leafUri, "/")
	targetLeaf := anchoredUri[strings.LastIndex(anchoredUri, "/")+1:]

	// Whether the request reads BGP_NEIGH_APPLIED and nothing else.
	appliedOnly := false

	switch {
	case strings.Contains(inParams.uri, "/"+niBgpNbrTransportSubtree):
		// Both transport leaves come from the applied row. A request anchored
		// on one of them watches that field alone, so the other one moving does
		// not wake it.
		transportFlds := map[string]string{
			niBgpNbrAppliedLocalAddrFld: niBgpNbrLocalAddrLeaf,
			niBgpNbrAppliedRmtAddrFld:   niBgpNbrRmtAddrLeaf,
		}
		if fld, ok := niBgpNbrAppliedFldForTransportLeaf[targetLeaf]; ok {
			transportFlds = map[string]string{fld: targetLeaf}
		}
		result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrAppliedTbl: {nbrKey: transportFlds}}}
		appliedOnly = true
	case strings.Contains(inParams.uri, "/"+niBgpNbrMessagesSubtree):
		result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrStatsTbl: {nbrKey: {}}}}
	default:
		if appliedFld, ok := niBgpNbrAppliedFldForLeaf[targetLeaf]; ok {
			// A state leaf that comes from the applied row alone. Registering
			// the other two tables as well, as the container below has to, would
			// hold it to their sampling for no reason.
			result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {
				niBgpNbrAppliedTbl: {nbrKey: {appliedFld: targetLeaf}},
			}}
			appliedOnly = true
			break
		}
		result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {
			niBgpNbrStateTbl:   {neighborAddr: {niBgpNbrStateFld: niBgpNbrSessionStateLeaf}},
			niBgpNbrAppliedTbl: {nbrKey: {}},
			niBgpNbrStatsTbl:   {nbrKey: {}},
		}}
	}

	// bgpmon updates a changed applied row in place and names the fields that
	// went away, rather than rewriting the row as a delete followed by a set,
	// so a value change reaches a subscriber as an update and not as the row
	// being removed and rebuilt. That is what makes these paths on-change
	// capable. BGP_NEIGH_STATS shares the write pattern but stays sampled,
	// because its counters move on nearly every scrape and a subscriber gains
	// nothing from being told each time.
	//
	// BGP_NEIGH_APPLIED_META is deliberately not registered. Its last_sync
	// marker is rewritten every scrape, so watching it would wake every
	// subscriber on the poll interval and undo the point of on-change. The cost
	// is that a producer that has stopped publishing is noticed on the next
	// read of the path rather than announced.
	if appliedOnly && keyed {
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		return result, nil
	}

	// Everything else is sampled: the state container because it reads the
	// counter tables too, the messages subtree because it is counters, and any
	// unkeyed request because its notification key would have to be resolved
	// back from the Redis key, which needs a path transformer these tables have
	// no SONiC YANG model to support.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
