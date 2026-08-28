////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
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

package transformer_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const niBgpNbrStateUrl = niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]/state"

const niBgpNbrTransportUrl = niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]/transport"

// The network instance of niBgpProtoUrl. BGP_NEIGH_APPLIED carries it in the
// key, unlike NEIGH_STATE_TABLE, because the scrape behind it covers every VRF
// FRR knows rather than the default one.
const niBgpNbrAppliedNi = "default"

// loadNiBgpNbrState writes one NEIGH_STATE_TABLE entry, the way bgpmon does.
func loadNiBgpNbrState(neighborAddr, state string) {
	loadDB(db.StateDB, map[string]interface{}{
		"NEIGH_STATE_TABLE": map[string]interface{}{
			neighborAddr: map[string]interface{}{
				"state":    state,
				"peerType": "e-BGP",
			},
		},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiBgpNbrState(neighborAddr string) {
	unloadDB(db.StateDB, map[string]interface{}{
		"NEIGH_STATE_TABLE": map[string]interface{}{neighborAddr: ""},
	})
}

func Test_openconfig_network_instance_bgp_neighbor_state_session_state(t *testing.T) {
	loadNiBgpNbrState("11.0.0.155", "Established")

	expectedGetJSON := `{"openconfig-network-instance:session-state":"ESTABLISHED"}`
	t.Run("GET BGP neighbor session-state leaf",
		processGetRequest(niBgpNbrStateUrl+"/session-state", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:state":{"session-state":"ESTABLISHED"}}`
	t.Run("GET BGP neighbor state container",
		processGetRequest(niBgpNbrStateUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrState("11.0.0.155")
}

// The FRR FSM spelling differs from the OpenConfig enumeration, and FRR has two
// teardown states plus a reason-qualified idle state that OpenConfig does not
// model at all.
func Test_openconfig_network_instance_bgp_neighbor_state_session_state_fsm_mapping(t *testing.T) {
	cases := []struct {
		frrState string
		expected string
	}{
		{"Idle", "IDLE"},
		{"Idle (Admin)", "IDLE"},
		{"Connect", "CONNECT"},
		{"Active", "ACTIVE"},
		{"OpenSent", "OPENSENT"},
		{"OpenConfirm", "OPENCONFIRM"},
		{"Established", "ESTABLISHED"},
		{"Clearing", "IDLE"},
		{"Deleted", "IDLE"},
	}

	for _, tc := range cases {
		loadNiBgpNbrState("11.0.0.155", tc.frrState)
		expectedGetJSON := `{"openconfig-network-instance:session-state":"` + tc.expected + `"}`
		t.Run("GET BGP neighbor session-state for FRR state "+tc.frrState,
			processGetRequest(niBgpNbrStateUrl+"/session-state", nil, expectedGetJSON, false))
		time.Sleep(1 * time.Second)
		unloadNiBgpNbrState("11.0.0.155")
	}
}

func Test_openconfig_network_instance_bgp_neighbor_state_session_state_unknown_rejected(t *testing.T) {
	loadNiBgpNbrState("11.0.0.155", "Bogus")

	expErr := tlerr.InvalidArgsError{
		Format: "unrecognized NEIGH_STATE_TABLE.state value \"Bogus\" for neighbor 11.0.0.155",
	}
	t.Run("GET BGP neighbor session-state with unmapped FRR state",
		processGetRequest(niBgpNbrStateUrl+"/session-state", nil, "", true, expErr))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrState("11.0.0.155")
}

func Test_openconfig_network_instance_bgp_neighbor_state_absent(t *testing.T) {
	// With no NEIGH_STATE_TABLE row the leaf is simply unset. It must not be an
	// error, otherwise a sample subscription taken out before bgpmon's first
	// sample would be torn down instead of reporting nothing.
	unloadNiBgpNbrState("11.0.0.155")
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor session-state with no state published",
		processGetRequest(niBgpNbrStateUrl+"/session-state", nil, `{}`, false))
	time.Sleep(1 * time.Second)
}

// niBgpNbrAppliedRow is the row bgpmon publishes for the lab peer, with every
// mapped field present.
func niBgpNbrAppliedRow() map[string]interface{} {
	return map[string]interface{}{
		"enabled":          "true",
		"neighbor_address": "11.0.0.155",
		"peer_as":          "65300",
		"peer_group":       "PG1",
		"local_address":    "11.0.0.154",
		"remote_address":   "11.0.0.155",
	}
}

// loadNiBgpNbrAppliedMarker writes the freshness marker bgpmon refreshes on
// every successful scrape.
//
// Every applied-leaf test needs a current one. The transformer suppresses all
// six applied leaves when the marker is missing or older than 60 seconds, so a
// test that omits it fails as though the mapping itself were broken.
func loadNiBgpNbrAppliedMarker(epoch int64) {
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{
			"global": map[string]interface{}{
				"last_sync": strconv.FormatInt(epoch, 10),
			},
		},
	})
}

func unloadNiBgpNbrAppliedMarker() {
	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{"global": ""},
	})
}

// loadNiBgpNbrApplied writes one applied row along with a current marker, the
// way bgpmon leaves STATE_DB after a successful scrape.
func loadNiBgpNbrApplied(neighborAddr string, fields map[string]interface{}) {
	loadNiBgpNbrAppliedMarker(time.Now().Unix())
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_APPLIED": map[string]interface{}{
			niBgpNbrAppliedNi + "|" + neighborAddr: fields,
		},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiBgpNbrApplied(neighborAddr string) {
	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_APPLIED": map[string]interface{}{
			niBgpNbrAppliedNi + "|" + neighborAddr: "",
		},
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{"global": ""},
	})
}

func Test_openconfig_network_instance_bgp_neighbor_applied_state_leaves(t *testing.T) {
	loadNiBgpNbrState("11.0.0.155", "Established")
	loadNiBgpNbrApplied("11.0.0.155", niBgpNbrAppliedRow())

	// Each leaf is fetched on its own as well as through the container, so that
	// a leaf request reporting its siblings would be caught.
	cases := []struct {
		leaf     string
		expected string
	}{
		{"enabled", `{"openconfig-network-instance:enabled":true}`},
		{"neighbor-address", `{"openconfig-network-instance:neighbor-address":"11.0.0.155"}`},
		{"peer-as", `{"openconfig-network-instance:peer-as":65300}`},
		{"peer-group", `{"openconfig-network-instance:peer-group":"PG1"}`},
	}
	for _, tc := range cases {
		t.Run("GET BGP neighbor applied state leaf "+tc.leaf,
			processGetRequest(niBgpNbrStateUrl+"/"+tc.leaf, nil, tc.expected, false))
		time.Sleep(1 * time.Second)
	}

	// The state container is filled from two tables at once: session-state from
	// NEIGH_STATE_TABLE and the applied leaves from BGP_NEIGH_APPLIED.
	expectedGetJSON := `{"openconfig-network-instance:state":{"session-state":"ESTABLISHED",` +
		`"enabled":true,"neighbor-address":"11.0.0.155","peer-as":65300,"peer-group":"PG1"}}`
	t.Run("GET BGP neighbor state container across both tables",
		processGetRequest(niBgpNbrStateUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrApplied("11.0.0.155")
	unloadNiBgpNbrState("11.0.0.155")
}

func Test_openconfig_network_instance_bgp_neighbor_applied_transport_state(t *testing.T) {
	loadNiBgpNbrApplied("11.0.0.155", niBgpNbrAppliedRow())

	t.Run("GET BGP neighbor applied transport local-address",
		processGetRequest(niBgpNbrTransportUrl+"/state/local-address", nil,
			`{"openconfig-network-instance:local-address":"11.0.0.154"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor applied transport remote-address",
		processGetRequest(niBgpNbrTransportUrl+"/state/remote-address", nil,
			`{"openconfig-network-instance:remote-address":"11.0.0.155"}`, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:state":` +
		`{"local-address":"11.0.0.154","remote-address":"11.0.0.155"}}`
	t.Run("GET BGP neighbor transport state container",
		processGetRequest(niBgpNbrTransportUrl+"/state", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// transport carries an annotation of its own, so a request anchored on the
	// container is served here. Without it the nearest annotated ancestor is the
	// neighbor list, whose dispatch maps CONFIG_DB and would report nothing.
	expectedGetJSON = `{"openconfig-network-instance:transport":{"state":` +
		`{"local-address":"11.0.0.154","remote-address":"11.0.0.155"}}}`
	t.Run("GET BGP neighbor transport container",
		processGetRequest(niBgpNbrTransportUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrApplied("11.0.0.155")
}

// bgpmon omits a field when FRR reports no value for it, which is what lets an
// absent leaf mean "FRR is applying nothing here". A peer outside a peer group
// has no peerGroup, and a peer that never opened a socket has no local address.
func Test_openconfig_network_instance_bgp_neighbor_applied_absent_fields(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")
	loadNiBgpNbrApplied("11.0.0.155", map[string]interface{}{
		"enabled":          "true",
		"neighbor_address": "11.0.0.155",
		"peer_as":          "65300",
		"remote_address":   "11.0.0.155",
	})

	t.Run("GET BGP neighbor applied peer-group for a peer in no group",
		processGetRequest(niBgpNbrStateUrl+"/peer-group", nil, `{}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor applied local-address when FRR bound none",
		processGetRequest(niBgpNbrTransportUrl+"/state/local-address", nil, `{}`, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:state":` +
		`{"enabled":true,"neighbor-address":"11.0.0.155","peer-as":65300}}`
	t.Run("GET BGP neighbor state container omitting the unreported leaf",
		processGetRequest(niBgpNbrStateUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:state":{"remote-address":"11.0.0.155"}}`
	t.Run("GET BGP neighbor transport state container omitting the unreported leaf",
		processGetRequest(niBgpNbrTransportUrl+"/state", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrApplied("11.0.0.155")
}

// The two lookups behind the state container must stay independent. A neighbor
// in a non-default VRF has an applied row but no NEIGH_STATE_TABLE row at all,
// because the summary scrape behind that table covers the default VRF only.
// Returning early on the missing session row, as this transformer did when
// session-state was its only leaf, would suppress the applied leaves for every
// non-default VRF.
func Test_openconfig_network_instance_bgp_neighbor_applied_without_session_state(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")
	loadNiBgpNbrApplied("11.0.0.155", niBgpNbrAppliedRow())

	t.Run("GET BGP neighbor applied enabled with no session state published",
		processGetRequest(niBgpNbrStateUrl+"/enabled", nil,
			`{"openconfig-network-instance:enabled":true}`, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:state":{"enabled":true,` +
		`"neighbor-address":"11.0.0.155","peer-as":65300,"peer-group":"PG1"}}`
	t.Run("GET BGP neighbor state container with no session state published",
		processGetRequest(niBgpNbrStateUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrApplied("11.0.0.155")
}

// The reverse direction of the same independence: session-state is served
// before the applied producer has published anything, which is the committed
// behaviour this change must not regress.
func Test_openconfig_network_instance_bgp_neighbor_session_state_without_applied(t *testing.T) {
	unloadNiBgpNbrApplied("11.0.0.155")
	loadNiBgpNbrState("11.0.0.155", "Established")

	t.Run("GET BGP neighbor session-state with no applied row",
		processGetRequest(niBgpNbrStateUrl+"/session-state", nil,
			`{"openconfig-network-instance:session-state":"ESTABLISHED"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor state container with no applied row",
		processGetRequest(niBgpNbrStateUrl, nil,
			`{"openconfig-network-instance:state":{"session-state":"ESTABLISHED"}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor applied enabled with no applied row",
		processGetRequest(niBgpNbrStateUrl+"/enabled", nil, `{}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrState("11.0.0.155")
}

// Rows left behind by a producer that has stopped are not reported. Serving
// them would break the whole point of the applied leaves: a configuration
// acceptance check comparing a stale value against what was just written would
// report a rejection that never happened.
func Test_openconfig_network_instance_bgp_neighbor_applied_stale_marker_suppressed(t *testing.T) {
	loadNiBgpNbrState("11.0.0.155", "Established")
	loadNiBgpNbrApplied("11.0.0.155", niBgpNbrAppliedRow())
	// Older than four of bgpmon's fifteen second poll intervals.
	loadNiBgpNbrAppliedMarker(time.Now().Unix() - 300)
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor applied enabled with a stale freshness marker",
		processGetRequest(niBgpNbrStateUrl+"/enabled", nil, `{}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor applied transport state with a stale freshness marker",
		processGetRequest(niBgpNbrTransportUrl+"/state", nil, `{}`, false))
	time.Sleep(1 * time.Second)

	// Suppression is scoped to the applied leaves. session-state comes from a
	// different table filled by a different scrape, so it stays served.
	t.Run("GET BGP neighbor state container with a stale freshness marker",
		processGetRequest(niBgpNbrStateUrl, nil,
			`{"openconfig-network-instance:state":{"session-state":"ESTABLISHED"}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrApplied("11.0.0.155")
	unloadNiBgpNbrState("11.0.0.155")
}

func Test_openconfig_network_instance_bgp_neighbor_applied_unusable_marker_suppressed(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")
	loadNiBgpNbrApplied("11.0.0.155", niBgpNbrAppliedRow())

	// Rows present but no marker: bgpmon has not completed a scrape since it
	// last started, so nothing in the table is trustworthy yet.
	unloadNiBgpNbrAppliedMarker()
	time.Sleep(1 * time.Second)
	t.Run("GET BGP neighbor applied enabled with no freshness marker",
		processGetRequest(niBgpNbrStateUrl+"/enabled", nil, `{}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor state container with no freshness marker",
		processGetRequest(niBgpNbrStateUrl, nil, `{}`, false))
	time.Sleep(1 * time.Second)

	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{
			"global": map[string]interface{}{"last_sync": "not-an-epoch"},
		},
	})
	time.Sleep(1 * time.Second)
	t.Run("GET BGP neighbor applied enabled with an unparseable freshness marker",
		processGetRequest(niBgpNbrStateUrl+"/enabled", nil, `{}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrApplied("11.0.0.155")
}

// A marker dated in the future means the clock was stepped, not that the
// producer is behind. Hiding live data over a time adjustment would be the
// worse failure.
func Test_openconfig_network_instance_bgp_neighbor_applied_future_marker_is_fresh(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")
	loadNiBgpNbrApplied("11.0.0.155", niBgpNbrAppliedRow())
	loadNiBgpNbrAppliedMarker(time.Now().Unix() + 3600)
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor applied enabled with a future freshness marker",
		processGetRequest(niBgpNbrStateUrl+"/enabled", nil,
			`{"openconfig-network-instance:enabled":true}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrApplied("11.0.0.155")
}

// A value the producer should never publish is rejected rather than guessed at.
// This is the same treatment session-state gives an unmapped FRR state.
func Test_openconfig_network_instance_bgp_neighbor_applied_invalid_values_rejected(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")

	loadNiBgpNbrApplied("11.0.0.155", map[string]interface{}{"enabled": "maybe"})
	expErr := tlerr.InvalidArgsError{
		Format: "unrecognized BGP_NEIGH_APPLIED.enabled value \"maybe\" for neighbor 11.0.0.155",
	}
	t.Run("GET BGP neighbor applied enabled with an unparseable boolean",
		processGetRequest(niBgpNbrStateUrl+"/enabled", nil, "", true, expErr))
	time.Sleep(1 * time.Second)
	unloadNiBgpNbrApplied("11.0.0.155")

	loadNiBgpNbrApplied("11.0.0.155", map[string]interface{}{"peer_as": "sixty-five-thousand"})
	expErr = tlerr.InvalidArgsError{
		Format: "unrecognized BGP_NEIGH_APPLIED.peer_as value \"sixty-five-thousand\" for neighbor 11.0.0.155",
	}
	t.Run("GET BGP neighbor applied peer-as with an unparseable AS number",
		processGetRequest(niBgpNbrStateUrl+"/peer-as", nil, "", true, expErr))
	time.Sleep(1 * time.Second)
	unloadNiBgpNbrApplied("11.0.0.155")
}

const niBgpNbrMessagesUrl = niBgpNbrStateUrl + "/messages"

// niBgpNbrStatsRow is the counter row bgpmon publishes for the lab peer, with
// every mapped field present. last_established carries FRR's epoch in seconds,
// which is what FRR reports; the leaf is defined in nanoseconds.
func niBgpNbrStatsRow() map[string]interface{} {
	return map[string]interface{}{
		"established_transitions": "3",
		"last_established":        "1756400000",
		"updates_received":        "87",
		"updates_sent":            "120",
	}
}

// loadNiBgpNbrStats writes one counter row along with a current marker. The
// counters are gated on the same marker as the applied row, because a single
// vtysh scrape fills both tables.
func loadNiBgpNbrStats(neighborAddr string, fields map[string]interface{}) {
	loadNiBgpNbrAppliedMarker(time.Now().Unix())
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_STATS": map[string]interface{}{
			niBgpNbrAppliedNi + "|" + neighborAddr: fields,
		},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiBgpNbrStats(neighborAddr string) {
	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_STATS": map[string]interface{}{
			niBgpNbrAppliedNi + "|" + neighborAddr: "",
		},
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{"global": ""},
	})
}

func Test_openconfig_network_instance_bgp_neighbor_stats_state_leaves(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")
	loadNiBgpNbrStats("11.0.0.155", niBgpNbrStatsRow())

	// Both counters are uint64, which RFC7951 encodes as a JSON string. The
	// uint32 peer-as above is a bare number, so the quoting is not incidental.
	t.Run("GET BGP neighbor established-transitions leaf",
		processGetRequest(niBgpNbrStateUrl+"/established-transitions", nil,
			`{"openconfig-network-instance:established-transitions":"3"}`, false))
	time.Sleep(1 * time.Second)

	// The producer stores FRR's 1756400000 seconds unscaled, so this also
	// checks the conversion rather than just the plumbing.
	t.Run("GET BGP neighbor last-established leaf scaled to nanoseconds",
		processGetRequest(niBgpNbrStateUrl+"/last-established", nil,
			`{"openconfig-network-instance:last-established":"1756400000000000000"}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrStats("11.0.0.155")
}

// The message counters are reached at four different depths, and each one is
// annotated separately. A missing annotation at any level would leave that
// request to the BGP container dispatch, which maps CONFIG_DB and would report
// nothing.
func Test_openconfig_network_instance_bgp_neighbor_stats_message_counters(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")
	loadNiBgpNbrStats("11.0.0.155", niBgpNbrStatsRow())

	t.Run("GET BGP neighbor messages sent UPDATE leaf",
		processGetRequest(niBgpNbrMessagesUrl+"/sent/UPDATE", nil,
			`{"openconfig-network-instance:UPDATE":"120"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor messages received UPDATE leaf",
		processGetRequest(niBgpNbrMessagesUrl+"/received/UPDATE", nil,
			`{"openconfig-network-instance:UPDATE":"87"}`, false))
	time.Sleep(1 * time.Second)

	// A direction container must report its own counter only, not its sibling's.
	t.Run("GET BGP neighbor messages sent container",
		processGetRequest(niBgpNbrMessagesUrl+"/sent", nil,
			`{"openconfig-network-instance:sent":{"UPDATE":"120"}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor messages received container",
		processGetRequest(niBgpNbrMessagesUrl+"/received", nil,
			`{"openconfig-network-instance:received":{"UPDATE":"87"}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor messages container",
		processGetRequest(niBgpNbrMessagesUrl, nil,
			`{"openconfig-network-instance:messages":{"sent":{"UPDATE":"120"},`+
				`"received":{"UPDATE":"87"}}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrStats("11.0.0.155")
}

// The state container is now filled from three independent tables. A request
// for the container has to reach the counters below messages as well, since no
// separate callback runs for them.
func Test_openconfig_network_instance_bgp_neighbor_state_container_with_counters(t *testing.T) {
	loadNiBgpNbrState("11.0.0.155", "Established")
	loadNiBgpNbrApplied("11.0.0.155", niBgpNbrAppliedRow())
	loadNiBgpNbrStats("11.0.0.155", niBgpNbrStatsRow())

	expectedGetJSON := `{"openconfig-network-instance:state":{"session-state":"ESTABLISHED",` +
		`"enabled":true,"neighbor-address":"11.0.0.155","peer-as":65300,"peer-group":"PG1",` +
		`"established-transitions":"3","last-established":"1756400000000000000",` +
		`"messages":{"sent":{"UPDATE":"120"},"received":{"UPDATE":"87"}}}}`
	t.Run("GET BGP neighbor state container across all three tables",
		processGetRequest(niBgpNbrStateUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrStats("11.0.0.155")
	unloadNiBgpNbrApplied("11.0.0.155")
	unloadNiBgpNbrState("11.0.0.155")
}

// bgpmon omits a counter FRR does not report, which is what lets an absent leaf
// mean "FRR has no value here". FRR drops bgpTimerUpEstablishedEpoch as soon as
// the session leaves Established, and reports no messageStats at all for a peer
// that has never opened one.
func Test_openconfig_network_instance_bgp_neighbor_stats_absent_fields(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")
	loadNiBgpNbrStats("11.0.0.155", map[string]interface{}{
		"established_transitions": "0",
	})

	// Zero is a real count and must not be confused with an absent field: a
	// peer that has never come up reports zero transitions, not none.
	t.Run("GET BGP neighbor established-transitions of zero",
		processGetRequest(niBgpNbrStateUrl+"/established-transitions", nil,
			`{"openconfig-network-instance:established-transitions":"0"}`, false))
	time.Sleep(1 * time.Second)

	// A targeted node the transformer populates nothing for is refused rather
	// than answered empty. That is what makes the absence meaningful: before
	// the transformer learned to leave the state container alone here, translib
	// filled the leaf with its type's zero value and reported a session that
	// has never come up as having come up at the epoch.
	expNotFound := tlerr.NotFoundError{Format: "Resource not found"}
	t.Run("GET BGP neighbor last-established with the session down",
		processGetRequest(niBgpNbrStateUrl+"/last-established", nil, "", true, expNotFound))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor messages container with no counters published",
		processGetRequest(niBgpNbrMessagesUrl, nil, "", true, expNotFound))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor state container with only one counter published",
		processGetRequest(niBgpNbrStateUrl, nil,
			`{"openconfig-network-instance:state":{"established-transitions":"0"}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrStats("11.0.0.155")
}

// Counters left behind by a producer that has stopped are not reported. A
// client watching update activity cannot otherwise tell a quiet session from a
// dead producer, since both leave the number unchanged.
func Test_openconfig_network_instance_bgp_neighbor_stats_stale_marker_suppressed(t *testing.T) {
	loadNiBgpNbrState("11.0.0.155", "Established")
	loadNiBgpNbrStats("11.0.0.155", niBgpNbrStatsRow())
	// Older than four of bgpmon's fifteen second poll intervals.
	loadNiBgpNbrAppliedMarker(time.Now().Unix() - 300)
	time.Sleep(1 * time.Second)

	expNotFound := tlerr.NotFoundError{Format: "Resource not found"}
	t.Run("GET BGP neighbor established-transitions with a stale freshness marker",
		processGetRequest(niBgpNbrStateUrl+"/established-transitions", nil, "", true, expNotFound))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor messages container with a stale freshness marker",
		processGetRequest(niBgpNbrMessagesUrl, nil, "", true, expNotFound))
	time.Sleep(1 * time.Second)

	// Suppression is scoped to the tables behind the marker. session-state
	// comes from a different table filled by a different scrape.
	t.Run("GET BGP neighbor state container with a stale freshness marker",
		processGetRequest(niBgpNbrStateUrl, nil,
			`{"openconfig-network-instance:state":{"session-state":"ESTABLISHED"}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrStats("11.0.0.155")
	unloadNiBgpNbrState("11.0.0.155")
}

// A counter value the producer should never publish is rejected rather than
// reported as a plausible number, the same treatment the applied leaves give an
// unparseable value.
func Test_openconfig_network_instance_bgp_neighbor_stats_invalid_values_rejected(t *testing.T) {
	unloadNiBgpNbrState("11.0.0.155")

	loadNiBgpNbrStats("11.0.0.155", map[string]interface{}{"established_transitions": "many"})
	expErr := tlerr.InvalidArgsError{
		Format: "unrecognized BGP_NEIGH_STATS.established_transitions value \"many\" for neighbor 11.0.0.155",
	}
	t.Run("GET BGP neighbor established-transitions with an unparseable counter",
		processGetRequest(niBgpNbrStateUrl+"/established-transitions", nil, "", true, expErr))
	time.Sleep(1 * time.Second)
	unloadNiBgpNbrStats("11.0.0.155")

	loadNiBgpNbrStats("11.0.0.155", map[string]interface{}{"updates_sent": "lots"})
	expErr = tlerr.InvalidArgsError{
		Format: "unrecognized BGP_NEIGH_STATS.updates_sent value \"lots\" for neighbor 11.0.0.155",
	}
	t.Run("GET BGP neighbor messages sent UPDATE with an unparseable counter",
		processGetRequest(niBgpNbrMessagesUrl+"/sent/UPDATE", nil, "", true, expErr))
	time.Sleep(1 * time.Second)
	unloadNiBgpNbrStats("11.0.0.155")

	// An epoch this large would wrap when scaled to nanoseconds. Reporting the
	// wrapped value would give a timestamp that looks entirely reasonable.
	loadNiBgpNbrStats("11.0.0.155", map[string]interface{}{"last_established": "18446744074"})
	expErr = tlerr.InvalidArgsError{
		Format: "BGP_NEIGH_STATS.last_established value 18446744074 for neighbor 11.0.0.155 does not fit a nanosecond timestamp",
	}
	t.Run("GET BGP neighbor last-established with an epoch that overflows nanoseconds",
		processGetRequest(niBgpNbrStateUrl+"/last-established", nil, "", true, expErr))
	time.Sleep(1 * time.Second)
	unloadNiBgpNbrStats("11.0.0.155")
}
