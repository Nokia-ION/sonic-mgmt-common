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

// Subscribe callback tests for the BGP neighbor AFI-SAFI state subtree. These
// live in the internal package, unlike the GET tests in
// xfmr_ni_bgp_neighbor_af_state_test.go, because XfmrSubscInParams cannot be
// built from outside it.

package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

func niBgpNbrAfSubscUrl(afiSafiName string) string {
	return niBgpNbrSubscNbrUrl + "/afi-safis/afi-safi[afi-safi-name=" + afiSafiName + "]"
}

func TestSubscribe_ni_bgp_nbr_af_state_paths(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantKey string
	}{
		// Every path in this subtree reads BGP_NEIGH_AF_STATS alone, so the key
		// carries the network instance, the neighbor and the address family.
		{
			name:    "state container",
			uri:     niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/state",
			wantKey: "default|11.0.0.155|ipv4_unicast",
		},
		{
			name:    "prefixes container",
			uri:     niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/state/prefixes",
			wantKey: "default|11.0.0.155|ipv4_unicast",
		},
		{
			name:    "IPv6 unicast prefixes container keys its own family",
			uri:     niBgpNbrAfSubscUrl("IPV6_UNICAST") + "/state/prefixes",
			wantKey: "default|11.0.0.155|ipv6_unicast",
		},
		// The afi-safi-name arrives module qualified over gNMI, which has to
		// normalise to the same CONFIG_DB spelling the row is keyed by.
		{
			name:    "module qualified afi-safi-name normalises",
			uri:     niBgpNbrAfSubscUrl("openconfig-bgp-types:IPV4_UNICAST") + "/state/prefixes",
			wantKey: "default|11.0.0.155|ipv4_unicast",
		},
		// A subscription spanning the afi-safi list has no family bound, so the
		// family is left as a wildcard rather than failing the subscription.
		{
			name:    "unkeyed afi-safi wildcards the family",
			uri:     niBgpNbrSubscNbrUrl + "/afi-safis/afi-safi/state/prefixes",
			wantKey: "default|11.0.0.155|*",
		},
		{
			name: "unkeyed neighbor wildcards the neighbor",
			uri: "/openconfig-network-instance:network-instances/network-instance[name=default]" +
				"/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor" +
				"/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/state/prefixes",
			wantKey: "default|*|ipv4_unicast",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.uri,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_bgp_nbr_af_state_xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a non-virtual subscribe mapping, got map %v", out.dbDataMap)
			}
			// Sampling only, because the producer rewrites a changed row as a
			// delete followed by a set and on-change would turn each rewrite
			// into a spurious delete.
			if out.onChange != OnchangeDisable {
				t.Fatalf("onChange=%v, want %v", out.onChange, OnchangeDisable)
			}
			if out.nOpts == nil {
				t.Fatalf("nOpts not set, so the preferred type would default")
			}
			if out.nOpts.pType != Sample {
				t.Fatalf("pType=%v, want %v", out.nOpts.pType, Sample)
			}

			afTbl, ok := out.dbDataMap[db.StateDB][niBgpNbrAfStatsTbl]
			if !ok {
				t.Fatalf("missing %s mapping: %v", niBgpNbrAfStatsTbl, out.dbDataMap)
			}
			if _, ok := afTbl[tc.wantKey]; !ok {
				t.Fatalf("missing %s[%s]: %v", niBgpNbrAfStatsTbl, tc.wantKey, out.dbDataMap)
			}
			// The other neighbor state tables belong to the sibling subtree, and
			// registering them here would watch rows this callback never reads.
			if _, ok := out.dbDataMap[db.StateDB][niBgpNbrStatsTbl]; ok {
				t.Fatalf("unexpected %s mapping: %v", niBgpNbrStatsTbl, out.dbDataMap)
			}
			if _, ok := out.dbDataMap[db.StateDB][niBgpNbrAppliedTbl]; ok {
				t.Fatalf("unexpected %s mapping: %v", niBgpNbrAppliedTbl, out.dbDataMap)
			}
		})
	}
}

// The request URI carries two "name" keys, the network instance and the
// protocol. Picking the wrong one would key the subscription on "bgp" and
// quietly watch a row that never exists.
func TestSubscribe_ni_bgp_nbr_af_state_key_carries_network_instance(t *testing.T) {
	const vrfAfUrl = "/openconfig-network-instance:network-instances/network-instance[name=Vrf1]" +
		"/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]" +
		"/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/state/prefixes"

	out, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
		uri:        vrfAfUrl,
		requestURI: vrfAfUrl + "/received",
		subscProc:  TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ni_bgp_nbr_af_state_xfmr failed: %v", err)
	}
	if _, ok := out.dbDataMap[db.StateDB][niBgpNbrAfStatsTbl]["Vrf1|11.0.0.155|ipv4_unicast"]; !ok {
		t.Fatalf("missing %s[Vrf1|11.0.0.155|ipv4_unicast]: %v", niBgpNbrAfStatsTbl, out.dbDataMap)
	}
}

// Only the subscribe translation maps this table. The other processing modes
// share the callback and must be told the subtree owns no table of its own.
func TestSubscribe_ni_bgp_nbr_af_state_non_translate_is_virtual(t *testing.T) {
	for _, proc := range []SubscProcType{TRANSLATE_EXISTS, PROCESS_SUBSCRIBE} {
		out, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
			uri:        niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/state/prefixes",
			requestURI: niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/state/prefixes/received",
			subscProc:  proc,
		})
		if err != nil {
			t.Fatalf("Subscribe_ni_bgp_nbr_af_state_xfmr failed for %v: %v", proc, err)
		}
		if !out.isVirtualTbl {
			t.Fatalf("expected a virtual table for %v, got map %v", proc, out.dbDataMap)
		}
		if len(out.dbDataMap) != 0 {
			t.Fatalf("expected no DB map for %v, got %v", proc, out.dbDataMap)
		}
	}
}

// As for the neighbor state subtree, the served spellings both reach the same
// counter rows and every other name is refused by the callback itself.
func TestSubscribe_ni_bgp_nbr_af_state_protocol_instance_name(t *testing.T) {
	afUrlFor := func(protoName string) string {
		return "/openconfig-network-instance:network-instances/network-instance[name=default]" +
			"/protocols/protocol[identifier=BGP][name=" + protoName + "]" +
			"/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]" +
			"/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/state/prefixes"
	}

	for _, protoName := range []string{"bgp", "BGP"} {
		url := afUrlFor(protoName)
		out, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
			uri:        url,
			requestURI: url + "/received",
			subscProc:  TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("protocol instance name %q was refused: %v", protoName, err)
		}
		if _, ok := out.dbDataMap[db.StateDB][niBgpNbrAfStatsTbl]["default|11.0.0.155|ipv4_unicast"]; !ok {
			t.Fatalf("missing %s[default|11.0.0.155|ipv4_unicast] for protocol instance name %q: %v",
				niBgpNbrAfStatsTbl, protoName, out.dbDataMap)
		}
	}

	url := afUrlFor("bgp2")
	if _, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
		uri:        url,
		requestURI: url + "/received",
		subscProc:  TRANSLATE_SUBSCRIBE,
	}); err == nil {
		t.Fatal("expected an unserved protocol instance name to be refused, got no error")
	}
}
