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

// Subscribe callback tests for the BGP neighbor state subtree. These live in
// the internal package, unlike the GET tests in xfmr_ni_bgp_neighbor_state_test.go,
// because XfmrSubscInParams cannot be built from outside it.
//
// Every leaf case passes the leaf in requestURI and the enclosing container in
// uri, because that is what the callback is really given: handleSubtreeNodeXlate
// trims a trailing leaf off the subtree uri before calling it. A subscription on
// state/session-state and one on the state container therefore arrive with the
// same uri, and only requestURI tells them apart. Tests that asserted on a uri
// still carrying the leaf passed while the DUT refused the subscription.

package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

const niBgpNbrSubscNbrUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]"

// The same path with no neighbor-address bound, as a wildcard subscription
// arrives.
const niBgpNbrSubscWildcardUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor"

func TestSubscribe_ni_bgp_nbr_state_paths(t *testing.T) {
	tests := []struct {
		name         string
		uri          string
		requestURI   string
		wantOnChange OnchangeMode
		wantPType    NotificationType
		wantStateKey string // NEIGH_STATE_TABLE key, "" when the table must be absent
		wantApplKey  string // BGP_NEIGH_APPLIED key, "" when the table must be absent
		wantStatsKey string // BGP_NEIGH_STATS key, "" when the table must be absent
	}{
		// The Phase 1 guarantee. An on-change subscription on session-state
		// reads NEIGH_STATE_TABLE alone, and must not also watch
		// BGP_NEIGH_APPLIED, whose rewrites would surface as spurious deletes.
		{
			name:         "session-state leaf keyed is on-change",
			uri:          niBgpNbrSubscNbrUrl + "/state",
			requestURI:   niBgpNbrSubscNbrUrl + "/state/session-state",
			wantOnChange: OnchangeEnable,
			wantPType:    OnChange,
			wantStateKey: "11.0.0.155",
		},
		{
			name:         "session-state leaf unkeyed falls back to sampling",
			uri:          niBgpNbrSubscWildcardUrl + "/state",
			requestURI:   niBgpNbrSubscWildcardUrl + "/state/session-state",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStateKey: "*",
		},
		// The ratified narrowing: the state container now reads the applied
		// table as well, so it gets sampling where Phase 1 gave it on-change.
		{
			name:         "state container is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/state",
			requestURI:   niBgpNbrSubscNbrUrl + "/state",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStateKey: "11.0.0.155",
			wantApplKey:  "default|11.0.0.155",
			wantStatsKey: "default|11.0.0.155",
		},
		{
			name:         "applied state leaf is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/state",
			requestURI:   niBgpNbrSubscNbrUrl + "/state/peer-as",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStateKey: "11.0.0.155",
			wantApplKey:  "default|11.0.0.155",
			wantStatsKey: "default|11.0.0.155",
		},
		{
			name:         "counter state leaf is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/state",
			requestURI:   niBgpNbrSubscNbrUrl + "/state/established-transitions",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStateKey: "11.0.0.155",
			wantApplKey:  "default|11.0.0.155",
			wantStatsKey: "default|11.0.0.155",
		},
		// The message counters read BGP_NEIGH_STATS alone, so they register
		// neither of the other two tables.
		{
			name:         "messages container is sampled from the stats table only",
			uri:          niBgpNbrSubscNbrUrl + "/state/messages",
			requestURI:   niBgpNbrSubscNbrUrl + "/state/messages",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStatsKey: "default|11.0.0.155",
		},
		{
			name:         "messages direction container is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/state/messages/sent",
			requestURI:   niBgpNbrSubscNbrUrl + "/state/messages/sent",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStatsKey: "default|11.0.0.155",
		},
		{
			name:         "messages UPDATE leaf is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/state/messages/received",
			requestURI:   niBgpNbrSubscNbrUrl + "/state/messages/received/UPDATE",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStatsKey: "default|11.0.0.155",
		},
		// transport reads the applied table only, so it never registers
		// NEIGH_STATE_TABLE.
		{
			name:         "transport state leaf is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/transport/state",
			requestURI:   niBgpNbrSubscNbrUrl + "/transport/state/remote-address",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantApplKey:  "default|11.0.0.155",
		},
		{
			name:         "transport container is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/transport",
			requestURI:   niBgpNbrSubscNbrUrl + "/transport",
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantApplKey:  "default|11.0.0.155",
		},
		// A request anchored above the state container descends into it, and
		// must not be mistaken for a session-state request.
		{
			name:         "neighbor container descends into state and is sampled",
			uri:          niBgpNbrSubscNbrUrl + "/state",
			requestURI:   niBgpNbrSubscNbrUrl,
			wantOnChange: OnchangeDisable,
			wantPType:    Sample,
			wantStateKey: "11.0.0.155",
			wantApplKey:  "default|11.0.0.155",
			wantStatsKey: "default|11.0.0.155",
		},
		// verifyParentTblSubtree reaches this callback with requestURI unset
		// and an untrimmed uri, so the leaf has to be read from uri there.
		{
			name:         "crud parent check without requestURI keeps the leaf",
			uri:          niBgpNbrSubscNbrUrl + "/state/session-state",
			requestURI:   "",
			wantOnChange: OnchangeEnable,
			wantPType:    OnChange,
			wantStateKey: "11.0.0.155",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_bgp_nbr_state_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.requestURI,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_bgp_nbr_state_xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a non-virtual subscribe mapping, got map %v", out.dbDataMap)
			}
			if out.onChange != tc.wantOnChange {
				t.Fatalf("onChange=%v, want %v", out.onChange, tc.wantOnChange)
			}
			if out.nOpts == nil {
				t.Fatalf("nOpts not set, so the preferred type would default")
			}
			if out.nOpts.pType != tc.wantPType {
				t.Fatalf("pType=%v, want %v", out.nOpts.pType, tc.wantPType)
			}

			stateTbl, hasStateTbl := out.dbDataMap[db.StateDB][niBgpNbrStateTbl]
			if tc.wantStateKey == "" {
				if hasStateTbl {
					t.Fatalf("unexpected %s mapping: %v", niBgpNbrStateTbl, out.dbDataMap)
				}
			} else {
				flds, ok := stateTbl[tc.wantStateKey]
				if !ok {
					t.Fatalf("missing %s[%s]: %v", niBgpNbrStateTbl, tc.wantStateKey, out.dbDataMap)
				}
				if flds[niBgpNbrStateFld] != niBgpNbrSessionStateLeaf {
					t.Fatalf("%s[%s] field map=%v, want %s mapped to %s",
						niBgpNbrStateTbl, tc.wantStateKey, flds, niBgpNbrStateFld, niBgpNbrSessionStateLeaf)
				}
			}

			applTbl, hasApplTbl := out.dbDataMap[db.StateDB][niBgpNbrAppliedTbl]
			if tc.wantApplKey == "" {
				if hasApplTbl {
					t.Fatalf("unexpected %s mapping: %v", niBgpNbrAppliedTbl, out.dbDataMap)
				}
			} else if _, ok := applTbl[tc.wantApplKey]; !ok {
				t.Fatalf("missing %s[%s]: %v", niBgpNbrAppliedTbl, tc.wantApplKey, out.dbDataMap)
			}

			statsTbl, hasStatsTbl := out.dbDataMap[db.StateDB][niBgpNbrStatsTbl]
			if tc.wantStatsKey == "" {
				if hasStatsTbl {
					t.Fatalf("unexpected %s mapping: %v", niBgpNbrStatsTbl, out.dbDataMap)
				}
			} else if _, ok := statsTbl[tc.wantStatsKey]; !ok {
				t.Fatalf("missing %s[%s]: %v", niBgpNbrStatsTbl, tc.wantStatsKey, out.dbDataMap)
			}
		})
	}
}

// BGP_NEIGH_APPLIED is keyed by network instance, and the request URI carries
// two "name" keys: the network instance and the protocol. Picking the wrong one
// would key the subscription on "bgp" and quietly watch a row that never exists.
func TestSubscribe_ni_bgp_nbr_state_applied_key_carries_network_instance(t *testing.T) {
	const vrfNbrUrl = "/openconfig-network-instance:network-instances/network-instance[name=Vrf1]" +
		"/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]"

	out, err := Subscribe_ni_bgp_nbr_state_xfmr(XfmrSubscInParams{
		uri:        vrfNbrUrl + "/transport/state",
		requestURI: vrfNbrUrl + "/transport/state/local-address",
		subscProc:  TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ni_bgp_nbr_state_xfmr failed: %v", err)
	}
	if _, ok := out.dbDataMap[db.StateDB][niBgpNbrAppliedTbl]["Vrf1|11.0.0.155"]; !ok {
		t.Fatalf("missing %s[Vrf1|11.0.0.155]: %v", niBgpNbrAppliedTbl, out.dbDataMap)
	}

	// BGP_NEIGH_STATS is keyed the same way, and the messages subtree is the
	// only path that reads it on its own.
	out, err = Subscribe_ni_bgp_nbr_state_xfmr(XfmrSubscInParams{
		uri:        vrfNbrUrl + "/state/messages/sent",
		requestURI: vrfNbrUrl + "/state/messages/sent/UPDATE",
		subscProc:  TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ni_bgp_nbr_state_xfmr failed: %v", err)
	}
	if _, ok := out.dbDataMap[db.StateDB][niBgpNbrStatsTbl]["Vrf1|11.0.0.155"]; !ok {
		t.Fatalf("missing %s[Vrf1|11.0.0.155]: %v", niBgpNbrStatsTbl, out.dbDataMap)
	}
}

// Only the subscribe translation maps these tables. The other processing modes
// share the callback and must be told the subtree owns no table of its own.
func TestSubscribe_ni_bgp_nbr_state_non_translate_is_virtual(t *testing.T) {
	for _, proc := range []SubscProcType{TRANSLATE_EXISTS, PROCESS_SUBSCRIBE} {
		out, err := Subscribe_ni_bgp_nbr_state_xfmr(XfmrSubscInParams{
			uri:        niBgpNbrSubscNbrUrl + "/state",
			requestURI: niBgpNbrSubscNbrUrl + "/state/session-state",
			subscProc:  proc,
		})
		if err != nil {
			t.Fatalf("Subscribe_ni_bgp_nbr_state_xfmr failed for %v: %v", proc, err)
		}
		if !out.isVirtualTbl {
			t.Fatalf("expected a virtual table for %v, got map %v", proc, out.dbDataMap)
		}
		if len(out.dbDataMap) != 0 {
			t.Fatalf("expected no DB map for %v, got %v", proc, out.dbDataMap)
		}
	}
}
