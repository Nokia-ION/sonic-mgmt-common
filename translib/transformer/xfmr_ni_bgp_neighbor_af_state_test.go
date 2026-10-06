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

const niBgpNbrAfIPv4 = "ipv4_unicast"
const niBgpNbrAfIPv6 = "ipv6_unicast"

func niBgpNbrAfStateUrl(afiSafiName string) string {
	return niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]" +
		"/afi-safis/afi-safi[afi-safi-name=" + afiSafiName + "]/state"
}

// loadNiBgpNbrAfStats writes one prefix counter row along with a current
// marker, the way bgpmon leaves STATE_DB after a successful scrape. The counters
// share the applied freshness marker, so a test that omitted it would fail as
// though the mapping itself were broken.
func loadNiBgpNbrAfStats(neighborAddr, afiSafi string, fields map[string]interface{}) {
	loadNiBgpNbrAppliedMarker(time.Now().Unix())
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_AF_STATS": map[string]interface{}{
			niBgpNbrAppliedNi + "|" + neighborAddr + "|" + afiSafi: fields,
		},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiBgpNbrAfStats(neighborAddr, afiSafi string) {
	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_AF_STATS": map[string]interface{}{
			niBgpNbrAppliedNi + "|" + neighborAddr + "|" + afiSafi: "",
		},
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{"global": ""},
	})
}

func Test_openconfig_network_instance_bgp_neighbor_af_prefix_counters(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "7",
		"prefixes_sent":     "4",
	})

	// Each counter is fetched on its own as well as through both enclosing
	// containers, so that a leaf request reporting its siblings would be caught.
	cases := []struct {
		path     string
		expected string
	}{
		{"/prefixes/received", `{"openconfig-network-instance:received":7}`},
		{"/prefixes/sent", `{"openconfig-network-instance:sent":4}`},
		{"/prefixes", `{"openconfig-network-instance:prefixes":{"received":7,"sent":4}}`},
		{"", `{"openconfig-network-instance:state":{"prefixes":{"received":7,"sent":4}}}`},
	}
	for _, tc := range cases {
		t.Run("GET BGP neighbor IPv4 unicast"+tc.path,
			processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+tc.path, nil, tc.expected, false))
		time.Sleep(1 * time.Second)
	}

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// The address family is part of the row key, so a request naming one family must
// not be served the other family's counts.
func Test_openconfig_network_instance_bgp_neighbor_af_families_are_isolated(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "7",
		"prefixes_sent":     "4",
	})
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv6, map[string]interface{}{
		"prefixes_received": "2",
		"prefixes_sent":     "1",
	})

	t.Run("GET BGP neighbor IPv4 unicast prefixes",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil,
			`{"openconfig-network-instance:prefixes":{"received":7,"sent":4}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv6 unicast prefixes",
		processGetRequest(niBgpNbrAfStateUrl("IPV6_UNICAST")+"/prefixes", nil,
			`{"openconfig-network-instance:prefixes":{"received":2,"sent":1}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv6)
}

// Zero prefixes is a real count, and the one a session under RFC 8212 policy
// reports, so it must be served rather than suppressed as though absent.
func Test_openconfig_network_instance_bgp_neighbor_af_zero_counters_reported(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "0",
		"prefixes_sent":     "0",
	})

	t.Run("GET BGP neighbor IPv4 unicast prefixes with zero counts",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil,
			`{"openconfig-network-instance:prefixes":{"received":0,"sent":0}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// received-pre-policy is annotated so the path is valid, but FRR reports no
// pre-policy count and the accepted count it does report is the post-policy one,
// so the leaf is never populated even when the rest of the row is.
func Test_openconfig_network_instance_bgp_neighbor_af_received_pre_policy_unpopulated(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "7",
		"prefixes_sent":     "4",
	})

	notFound := tlerr.NotFoundError{Format: "Resource not found"}
	t.Run("GET BGP neighbor IPv4 unicast received-pre-policy",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes/received-pre-policy",
			nil, "", true, notFound))
	time.Sleep(1 * time.Second)

	// The container must still report the two counters that do have a source,
	// rather than being suppressed by the leaf that has none.
	t.Run("GET BGP neighbor IPv4 unicast prefixes omits received-pre-policy",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil,
			`{"openconfig-network-instance:prefixes":{"received":7,"sent":4}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// FRR guards sentPrefixCounter on the peer belonging to an outbound update
// subgroup, so the field is genuinely missing between establishment and subgroup
// formation. The leaf must answer as absent rather than as zero prefixes sent.
func Test_openconfig_network_instance_bgp_neighbor_af_absent_sent_counter(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "7",
	})

	notFound := tlerr.NotFoundError{Format: "Resource not found"}
	t.Run("GET BGP neighbor IPv4 unicast sent with no sent counter published",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes/sent", nil, "", true, notFound))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast received with no sent counter published",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes/received", nil,
			`{"openconfig-network-instance:received":7}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast prefixes with no sent counter published",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil,
			`{"openconfig-network-instance:prefixes":{"received":7}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// A family that is not activated on the neighbor has no row at all, which is the
// normal state rather than an error.
func Test_openconfig_network_instance_bgp_neighbor_af_absent_row(t *testing.T) {
	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
	loadNiBgpNbrAppliedMarker(time.Now().Unix())
	time.Sleep(1 * time.Second)

	// A container that nothing populated is answered as empty rather than as
	// NotFound, which is how translib reports a container request differently
	// from a request naming one leaf.
	t.Run("GET BGP neighbor IPv4 unicast prefixes with no row published",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAppliedMarker()
}

// The counters share the applied freshness marker, because one scrape writes
// both tables. Serving them past it would report a frozen count as a live one.
func Test_openconfig_network_instance_bgp_neighbor_af_stale_marker_suppressed(t *testing.T) {
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_AF_STATS": map[string]interface{}{
			niBgpNbrAppliedNi + "|11.0.0.155|" + niBgpNbrAfIPv4: map[string]interface{}{
				"prefixes_received": "7",
				"prefixes_sent":     "4",
			},
		},
	})
	// Well beyond the 60 second window the applied leaves use.
	loadNiBgpNbrAppliedMarker(time.Now().Unix() - 3600)
	time.Sleep(1 * time.Second)

	notFound := tlerr.NotFoundError{Format: "Resource not found"}
	t.Run("GET BGP neighbor IPv4 unicast received with a stale marker",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes/received", nil, "", true, notFound))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast prefixes with a stale marker",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// No marker at all means bgpmon has not completed a scrape since it last
// started, so there is nothing trustworthy to report.
func Test_openconfig_network_instance_bgp_neighbor_af_missing_marker_suppressed(t *testing.T) {
	unloadNiBgpNbrAppliedMarker()
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_AF_STATS": map[string]interface{}{
			niBgpNbrAppliedNi + "|11.0.0.155|" + niBgpNbrAfIPv4: map[string]interface{}{
				"prefixes_received": "7",
			},
		},
	})
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast prefixes with no marker",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// A value that is present but not a number, or that does not fit the leaf, is a
// producer fault rather than a missing value, so it fails the request instead of
// being silently dropped.
func Test_openconfig_network_instance_bgp_neighbor_af_invalid_values_rejected(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "not-a-number",
	})

	expErr := tlerr.InvalidArgsError{
		Format: "unrecognized BGP_NEIGH_AF_STATS.prefixes_received value \"not-a-number\" for neighbor 11.0.0.155 address family ipv4_unicast",
	}
	t.Run("GET BGP neighbor IPv4 unicast received with an unparseable count",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes/received", nil, "", true, expErr))
	time.Sleep(1 * time.Second)

	// The leaf is uint32, so a count that overflows it cannot be reported and
	// must not be truncated into a plausible looking number.
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "4294967296",
	})

	expErr = tlerr.InvalidArgsError{
		Format: "unrecognized BGP_NEIGH_AF_STATS.prefixes_received value \"4294967296\" for neighbor 11.0.0.155 address family ipv4_unicast",
	}
	t.Run("GET BGP neighbor IPv4 unicast received with an out of range count",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes/received", nil, "", true, expErr))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// The row key carries the network instance, so a row published under another VRF
// must not be served for the default one.
func Test_openconfig_network_instance_bgp_neighbor_af_vrf_scoped(t *testing.T) {
	loadNiBgpNbrAppliedMarker(time.Now().Unix())
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_AF_STATS": map[string]interface{}{
			"Vrf1|11.0.0.155|" + niBgpNbrAfIPv4: map[string]interface{}{
				"prefixes_received": "7",
				"prefixes_sent":     "4",
			},
		},
	})
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast prefixes for a row published under another VRF",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST")+"/prefixes", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_AF_STATS": map[string]interface{}{
			"Vrf1|11.0.0.155|" + niBgpNbrAfIPv4: "",
		},
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{"global": ""},
	})
}

func niBgpNbrAfApplyPolicyStateUrl(neighborAddr, afiSafiName string) string {
	return niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=" + neighborAddr + "]" +
		"/afi-safis/afi-safi[afi-safi-name=" + afiSafiName + "]/apply-policy/state"
}

// apply-policy/state reports the route-maps FRR applies, each direction as a one
// element list, from the row the prefix counters share. Each leaf is fetched on
// its own as well as through the container, so that a leaf request reporting
// its sibling would be caught.
func Test_openconfig_network_instance_bgp_neighbor_af_apply_policy_state(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "7",
		"prefixes_sent":     "4",
		"import_policy":     "RM_IN",
		"export_policy":     "RM_OUT",
	})

	cases := []struct {
		path     string
		expected string
	}{
		{"/import-policy", `{"openconfig-network-instance:import-policy":["RM_IN"]}`},
		{"/export-policy", `{"openconfig-network-instance:export-policy":["RM_OUT"]}`},
		{"", `{"openconfig-network-instance:state":{"export-policy":["RM_OUT"],"import-policy":["RM_IN"]}}`},
	}
	for _, tc := range cases {
		t.Run("GET BGP neighbor IPv4 unicast apply-policy state"+tc.path,
			processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV4_UNICAST")+tc.path, nil, tc.expected, false))
		time.Sleep(1 * time.Second)
	}

	// The policies share their row with the counters, and must not leak into
	// the afi-safi state container the counters are served under.
	t.Run("GET BGP neighbor IPv4 unicast state carries no policy",
		processGetRequest(niBgpNbrAfStateUrl("IPV4_UNICAST"), nil,
			`{"openconfig-network-instance:state":{"prefixes":{"received":7,"sent":4}}}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// FRR omits a direction that has no route-map, which has to answer as absent
// rather than as the other direction's policy. translib answers a leaf-list
// read with nothing to report by an empty body, as it does for the config
// policies, where a leaf read is answered NotFound.
func Test_openconfig_network_instance_bgp_neighbor_af_apply_policy_state_one_direction(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"prefixes_received": "7",
		"import_policy":     "RM_IN",
	})

	t.Run("GET BGP neighbor IPv4 unicast export-policy with no outbound route-map",
		processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV4_UNICAST")+"/export-policy", nil, "{}", false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast apply-policy state with no outbound route-map",
		processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV4_UNICAST"), nil,
			`{"openconfig-network-instance:state":{"import-policy":["RM_IN"]}}`, false))
	time.Sleep(1 * time.Second)

	// A row carrying counters alone has no policy to report at all.
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv6, map[string]interface{}{
		"prefixes_received": "2",
	})
	t.Run("GET BGP neighbor IPv6 unicast apply-policy state with no route-map",
		processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV6_UNICAST"), nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv6)
}

// The default policies are annotated so the paths are valid, but FRR has no
// such setting to report, so they are never populated even beside a policy
// that is.
func Test_openconfig_network_instance_bgp_neighbor_af_apply_policy_state_default_policies_unpopulated(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"import_policy": "RM_IN",
		"export_policy": "RM_OUT",
	})

	notFound := tlerr.NotFoundError{Format: "Resource not found"}
	for _, leaf := range []string{"default-import-policy", "default-export-policy"} {
		t.Run("GET BGP neighbor IPv4 unicast apply-policy state "+leaf,
			processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV4_UNICAST")+"/"+leaf, nil, "", true, notFound))
		time.Sleep(1 * time.Second)
	}

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// Each family keys its own row, so one family's policies must not be reported
// for the other.
func Test_openconfig_network_instance_bgp_neighbor_af_apply_policy_state_families_are_isolated(t *testing.T) {
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4, map[string]interface{}{
		"import_policy": "RM_V4_IN",
	})
	loadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv6, map[string]interface{}{
		"import_policy": "RM_V6_IN",
	})

	t.Run("GET BGP neighbor IPv4 unicast import-policy",
		processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV4_UNICAST")+"/import-policy", nil,
			`{"openconfig-network-instance:import-policy":["RM_V4_IN"]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv6 unicast import-policy",
		processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV6_UNICAST")+"/import-policy", nil,
			`{"openconfig-network-instance:import-policy":["RM_V6_IN"]}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv6)
}

// The policies share the applied freshness marker with the counters, so a
// policy FRR may since have dropped is not reported past it.
func Test_openconfig_network_instance_bgp_neighbor_af_apply_policy_state_stale_marker_suppressed(t *testing.T) {
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_AF_STATS": map[string]interface{}{
			niBgpNbrAppliedNi + "|11.0.0.155|" + niBgpNbrAfIPv4: map[string]interface{}{
				"import_policy": "RM_IN",
			},
		},
	})
	loadNiBgpNbrAppliedMarker(time.Now().Unix() - 3600)
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast import-policy with a stale marker",
		processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV4_UNICAST")+"/import-policy", nil, "{}", false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor IPv4 unicast apply-policy state with a stale marker",
		processGetRequest(niBgpNbrAfApplyPolicyStateUrl("11.0.0.155", "IPV4_UNICAST"), nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("11.0.0.155", niBgpNbrAfIPv4)
}

// A read anchored above apply-policy/state, at the apply-policy container or
// the afi-safi instance, reports the state beside the config. The two differ
// on purpose: state is what FRR applies, and only CONFIG_DB knows what was
// configured.
func Test_openconfig_network_instance_bgp_neighbor_af_apply_policy_state_hierarchy(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{"local_asn": "65100"},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{"asn": "65200"},
		},
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
				"route_map_in": "RM-CFG-IN",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{"default|20.0.0.1|ipv4_unicast": ""},
		"BGP_NEIGHBOR":    map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_GLOBALS":     map[string]interface{}{"default": ""},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	loadNiBgpNbrAfStats("20.0.0.1", niBgpNbrAfIPv4, map[string]interface{}{
		"import_policy": "RM-FRR-IN",
		"export_policy": "RM-FRR-OUT",
	})

	nbrUrl := niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]"
	afisUrl := nbrUrl + "/afi-safis"
	v4Url := afisUrl + "/afi-safi[afi-safi-name=IPV4_UNICAST]"
	applyJSON := `{"config":{"import-policy":["RM-CFG-IN"]},` +
		`"state":{"export-policy":["RM-FRR-OUT"],"import-policy":["RM-FRR-IN"]}}`
	v4EntryJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST",` +
		`"apply-policy":` + applyJSON + `,` +
		`"config":{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":true}}`

	t.Run("GET BGP neighbor apply-policy container with config and state",
		processGetRequest(v4Url+"/apply-policy", nil,
			`{"openconfig-network-instance:apply-policy":`+applyJSON+`}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor afi-safi instance with apply-policy state",
		processGetRequest(v4Url, nil,
			`{"openconfig-network-instance:afi-safi":[`+v4EntryJSON+`]}`, false))
	time.Sleep(1 * time.Second)

	// A neighbor running a family it inherits from its peer-group has no row
	// of its own, and FRR still reports the inherited policies. The container
	// read reports them rather than failing on the absent config.
	unloadDB(db.ConfigDB, map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{"default|20.0.0.1|ipv4_unicast": ""},
	})
	time.Sleep(1 * time.Second)

	stateJSON := `{"export-policy":["RM-FRR-OUT"],"import-policy":["RM-FRR-IN"]}`
	t.Run("GET BGP neighbor apply-policy container with an inherited family",
		processGetRequest(v4Url+"/apply-policy", nil,
			`{"openconfig-network-instance:apply-policy":{"state":`+stateJSON+`}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP neighbor apply-policy state with an inherited family",
		processGetRequest(v4Url+"/apply-policy/state", nil,
			`{"openconfig-network-instance:state":`+stateJSON+`}`, false))
	time.Sleep(1 * time.Second)

	// The config itself is still absent, and a read naming it says so.
	noRow := tlerr.TranslibRedisClientEntryNotExist{Entry: "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast"}
	t.Run("GET BGP neighbor apply-policy config import-policy with an inherited family",
		processGetRequest(v4Url+"/apply-policy/config/import-policy", nil, "", true, noRow))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrAfStats("20.0.0.1", niBgpNbrAfIPv4)
	unloadDB(db.ConfigDB, cleanupTbl)
}
