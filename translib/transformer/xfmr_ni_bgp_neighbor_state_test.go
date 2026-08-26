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
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const niBgpNbrStateUrl = niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]/state"

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
