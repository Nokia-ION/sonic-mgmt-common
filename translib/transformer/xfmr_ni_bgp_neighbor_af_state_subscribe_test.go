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

// Subscribe callback tests for the BGP neighbor AFI-SAFI state and apply-policy
// subtrees. These live in the internal package, unlike the GET tests in
// xfmr_ni_bgp_neighbor_af_state_test.go, because XfmrSubscInParams cannot be
// built from outside it.

package transformer

import (
	"reflect"
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
			// Sampling only: the counters move on every poll, so an
			// on-change request for them is refused rather than streamed.
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

// apply-policy/state watches the policy fields of the row the counters share,
// and nothing else, so a counter moving on that row does not wake a policy
// subscriber. The subscribe path trims a leaf off uri before the callback, so
// the leaf a request names is carried by requestURI alone.
func TestSubscribe_ni_bgp_nbr_af_apply_policy_state_paths(t *testing.T) {
	v4State := niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/apply-policy/state"
	both := map[string]string{
		niBgpNbrAfStatsImportPolicyFld: niBgpNbrImportPolicyLeaf,
		niBgpNbrAfStatsExportPolicyFld: niBgpNbrExportPolicyLeaf,
	}
	tests := []struct {
		name       string
		uri        string
		requestURI string
		wantKey    string
		wantFlds   map[string]string
	}{
		{
			name:       "state container",
			uri:        v4State,
			requestURI: v4State,
			wantKey:    "default|11.0.0.155|ipv4_unicast",
			wantFlds:   both,
		},
		{
			name:       "import-policy leaf",
			uri:        v4State,
			requestURI: v4State + "/import-policy",
			wantKey:    "default|11.0.0.155|ipv4_unicast",
			wantFlds:   map[string]string{niBgpNbrAfStatsImportPolicyFld: niBgpNbrImportPolicyLeaf},
		},
		{
			name:       "export-policy leaf",
			uri:        v4State,
			requestURI: v4State + "/export-policy",
			wantKey:    "default|11.0.0.155|ipv4_unicast",
			wantFlds:   map[string]string{niBgpNbrAfStatsExportPolicyFld: niBgpNbrExportPolicyLeaf},
		},
		// A request anchored on the apply-policy container reaches this
		// callback for its state child, with requestURI naming the container.
		{
			name:       "apply-policy container anchor",
			uri:        v4State,
			requestURI: niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/apply-policy",
			wantKey:    "default|11.0.0.155|ipv4_unicast",
			wantFlds:   both,
		},
		{
			name:       "IPv6 unicast keys its own family",
			uri:        niBgpNbrAfSubscUrl("IPV6_UNICAST") + "/apply-policy/state",
			requestURI: niBgpNbrAfSubscUrl("IPV6_UNICAST") + "/apply-policy/state",
			wantKey:    "default|11.0.0.155|ipv6_unicast",
			wantFlds:   both,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.requestURI,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_bgp_nbr_af_state_xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a non-virtual subscribe mapping, got map %v", out.dbDataMap)
			}
			// bgpmon updates the row in place, so the policies are offered
			// on-change, unlike the counters beside them.
			if out.onChange != OnchangeEnable {
				t.Fatalf("onChange=%v, want %v", out.onChange, OnchangeEnable)
			}
			if out.nOpts == nil || out.nOpts.pType != OnChange {
				t.Fatalf("nOpts=%v, want pType %v", out.nOpts, OnChange)
			}
			flds, ok := out.dbDataMap[db.StateDB][niBgpNbrAfStatsTbl][tc.wantKey]
			if !ok {
				t.Fatalf("missing %s[%s]: %v", niBgpNbrAfStatsTbl, tc.wantKey, out.dbDataMap)
			}
			if !reflect.DeepEqual(flds, tc.wantFlds) {
				t.Fatalf("fields=%v, want %v", flds, tc.wantFlds)
			}
			// The freshness marker is rewritten every scrape, and watching it
			// would wake every subscriber on the poll interval.
			if len(out.dbDataMap[db.StateDB]) != 1 {
				t.Fatalf("expected %s alone, got %v", niBgpNbrAfStatsTbl, out.dbDataMap)
			}
		})
	}
}

// A request leaving a key open stays on-change: the list path transformer
// resolves the notification key from the Redis key.
func TestSubscribe_ni_bgp_nbr_af_apply_policy_state_unkeyed_stays_on_change(t *testing.T) {
	for _, uri := range []string{
		niBgpNbrSubscNbrUrl + "/afi-safis/afi-safi/apply-policy/state",
		"/openconfig-network-instance:network-instances/network-instance[name=default]" +
			"/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor" +
			"/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/state",
	} {
		out, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
			uri:        uri,
			requestURI: uri,
			subscProc:  TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("Subscribe_ni_bgp_nbr_af_state_xfmr(%s) failed: %v", uri, err)
		}
		if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
			t.Fatalf("%s: onChange=%v nOpts=%v, want on-change", uri, out.onChange, out.nOpts)
		}
		if len(out.dbDataMap[db.StateDB][niBgpNbrAfStatsTbl]) != 1 {
			t.Fatalf("%s: expected one wildcard key, got %v", uri, out.dbDataMap)
		}
	}
}

// The default policy leaves have no producer field, so there is nothing to
// watch; the subscription is accepted and reports nothing, as a GET does.
func TestSubscribe_ni_bgp_nbr_af_apply_policy_state_default_policy_is_virtual(t *testing.T) {
	v4State := niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/apply-policy/state"
	for _, leaf := range []string{"default-import-policy", "default-export-policy"} {
		out, err := Subscribe_ni_bgp_nbr_af_state_xfmr(XfmrSubscInParams{
			uri:        v4State,
			requestURI: v4State + "/" + leaf,
			subscProc:  TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("Subscribe_ni_bgp_nbr_af_state_xfmr(%s) failed: %v", leaf, err)
		}
		if !out.isVirtualTbl || len(out.dbDataMap) != 0 {
			t.Fatalf("%s: expected a virtual table with no DB map, got %v", leaf, out.dbDataMap)
		}
	}
}

// The neighbor config transformer binds a subscribe callback for the
// apply-policy subtree. The container is the target of a container
// subscription and holds no field of its own; its config child watches the
// two route-map fields of the BGP_NEIGHBOR_AF row.
func TestSubscribe_ni_bgp_nbr_cfg_apply_policy_paths(t *testing.T) {
	apply := niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/apply-policy"
	cfg := apply + "/config"

	out, err := Subscribe_ni_bgp_nbr_cfg_xfmr(XfmrSubscInParams{uri: apply, requestURI: apply, subscProc: TRANSLATE_SUBSCRIBE})
	if err != nil {
		t.Fatalf("Subscribe_ni_bgp_nbr_cfg_xfmr(apply-policy) failed: %v", err)
	}
	if !out.isVirtualTbl || out.onChange != OnchangeEnable {
		t.Fatalf("apply-policy: isVirtualTbl=%v onChange=%v, want a virtual on-change node", out.isVirtualTbl, out.onChange)
	}

	tests := []struct {
		name       string
		requestURI string
		wantFlds   map[string]string
	}{
		{"config container", cfg, map[string]string{"route_map_in": "import-policy", "route_map_out": "export-policy"}},
		{"import-policy leaf", cfg + "/import-policy", map[string]string{"route_map_in": "import-policy"}},
		{"export-policy leaf", cfg + "/export-policy", map[string]string{"route_map_out": "export-policy"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_bgp_nbr_cfg_xfmr(XfmrSubscInParams{uri: cfg, requestURI: tc.requestURI, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("Subscribe_ni_bgp_nbr_cfg_xfmr failed: %v", err)
			}
			if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
				t.Fatalf("onChange=%v nOpts=%v, want on-change", out.onChange, out.nOpts)
			}
			flds, ok := out.dbDataMap[db.ConfigDB]["BGP_NEIGHBOR_AF"]["default|11.0.0.155|ipv4_unicast"]
			if !ok {
				t.Fatalf("missing BGP_NEIGHBOR_AF[default|11.0.0.155|ipv4_unicast]: %v", out.dbDataMap)
			}
			if !reflect.DeepEqual(flds, tc.wantFlds) {
				t.Fatalf("fields=%v, want %v", flds, tc.wantFlds)
			}
		})
	}

	// The config side refuses the default policies, so there is no field.
	out, err = Subscribe_ni_bgp_nbr_cfg_xfmr(XfmrSubscInParams{uri: cfg, requestURI: cfg + "/default-import-policy", subscProc: TRANSLATE_SUBSCRIBE})
	if err != nil || !out.isVirtualTbl || len(out.dbDataMap) != 0 {
		t.Fatalf("default-import-policy: err=%v isVirtualTbl=%v map=%v, want a virtual table", err, out.isVirtualTbl, out.dbDataMap)
	}

	unkeyed := niBgpNbrSubscNbrUrl + "/afi-safis/afi-safi/apply-policy/config"
	out, err = Subscribe_ni_bgp_nbr_cfg_xfmr(XfmrSubscInParams{uri: unkeyed, requestURI: unkeyed, subscProc: TRANSLATE_SUBSCRIBE})
	if err != nil {
		t.Fatalf("Subscribe_ni_bgp_nbr_cfg_xfmr(unkeyed) failed: %v", err)
	}
	if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
		t.Fatalf("unkeyed: onChange=%v nOpts=%v, want on-change", out.onChange, out.nOpts)
	}
	if _, ok := out.dbDataMap[db.ConfigDB]["BGP_NEIGHBOR_AF"]["default|11.0.0.155|*"]; !ok {
		t.Fatalf("unkeyed: missing BGP_NEIGHBOR_AF[default|11.0.0.155|*]: %v", out.dbDataMap)
	}
}

// A node the neighbor config transformer owns but does not describe, and every
// call that is not a subscription, gets what translib assumed before the
// callback was bound. The CRUD parent-table check calls it with requestURI unset, and a
// mapping returned there would make a write depend on a row existing.
func TestSubscribe_ni_bgp_nbr_cfg_other_paths_keep_default(t *testing.T) {
	cfg := niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/apply-policy/config"
	cases := []struct {
		name string
		in   XfmrSubscInParams
	}{
		{"CRUD parent-table check", XfmrSubscInParams{uri: cfg, subscProc: TRANSLATE_SUBSCRIBE}},
		{"exists check", XfmrSubscInParams{uri: cfg, requestURI: cfg, subscProc: TRANSLATE_EXISTS}},
		{"process subscribe", XfmrSubscInParams{uri: cfg, requestURI: cfg, subscProc: PROCESS_SUBSCRIBE}},
		{"afi-safi ipv4-unicast", XfmrSubscInParams{uri: niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/ipv4-unicast",
			requestURI: niBgpNbrAfSubscUrl("IPV4_UNICAST") + "/ipv4-unicast", subscProc: TRANSLATE_SUBSCRIBE}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_bgp_nbr_cfg_xfmr(tc.in)
			if err != nil {
				t.Fatalf("Subscribe_ni_bgp_nbr_cfg_xfmr failed: %v", err)
			}
			if out.isVirtualTbl || out.dbDataMap != nil || out.nOpts != nil || out.onChange != OnchangeDisable {
				t.Fatalf("got isVirtualTbl=%v map=%v nOpts=%v onChange=%v, want the unbound default",
					out.isVirtualTbl, out.dbDataMap, out.nOpts, out.onChange)
			}
		})
	}
}
