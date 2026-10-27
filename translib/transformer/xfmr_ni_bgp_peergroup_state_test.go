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

const niBgpPgStateUrl = niBgpProtoUrl + "/bgp/peer-groups/peer-group[peer-group-name=PGS1]"

// loadNiBgpPgApplied writes the rows bgpmon publishes for one peer-group,
// with a marker of the given age.
func loadNiBgpPgApplied(group map[string]interface{}, families []string, markerAge time.Duration) {
	afRows := map[string]interface{}{}
	for _, af := range families {
		afRows["default|PGS1|"+af] = map[string]interface{}{"enabled": "true"}
	}
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_PEER_GROUP_APPLIED":    map[string]interface{}{"default|PGS1": group},
		"BGP_PEER_GROUP_AF_APPLIED": afRows,
		"BGP_PEER_GROUP_APPLIED_META": map[string]interface{}{"global": map[string]interface{}{
			"last_sync": strconv.FormatInt(time.Now().Add(-markerAge).Unix(), 10)}},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiBgpPgApplied() {
	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_PEER_GROUP_APPLIED": map[string]interface{}{"default|PGS1": ""},
		"BGP_PEER_GROUP_AF_APPLIED": map[string]interface{}{
			"default|PGS1|ipv4_unicast": "", "default|PGS1|ipv6_unicast": ""},
		"BGP_PEER_GROUP_APPLIED_META": map[string]interface{}{"global": ""},
	})
}

func niBgpPgStateCfg() (map[string]interface{}, map[string]interface{}) {
	cfg := map[string]interface{}{
		"BGP_GLOBALS":       map[string]interface{}{"default": map[string]interface{}{"local_asn": "65100"}},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PGS1": map[string]interface{}{"asn": "65300"}},
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PGS1|ipv4_unicast": map[string]interface{}{"admin_status": "up"}},
	}
	cleanup := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PGS1|ipv4_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PGS1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	return cfg, cleanup
}

// Peer-group state reports what FRR applies to the group: its name, remote AS
// and type, and the families it has activated. Each leaf is read on its own as
// well as through every container above it up to the bgp container, which
// translib reaches only through the state replay.
func Test_openconfig_network_instance_bgp_peer_group_state(t *testing.T) {
	cfg, cleanup := niBgpPgStateCfg()
	unloadDB(db.ConfigDB, cleanup)
	loadDB(db.ConfigDB, cfg)
	loadNiBgpPgApplied(map[string]interface{}{
		"peer_group_name": "PGS1", "peer_as": "65300", "peer_type": "external"}, []string{"ipv4_unicast"}, 0)

	stateJSON := `{"peer-as":65300,"peer-group-name":"PGS1","peer-type":"EXTERNAL"}`
	afStateJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":true}`
	afJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST",` +
		`"config":{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":true},"state":` + afStateJSON + `}`
	pgJSON := `{"afi-safis":{"afi-safi":[` + afJSON + `]},"config":{"peer-as":65300},` +
		`"peer-group-name":"PGS1","state":` + stateJSON + `}`
	afUrl := niBgpPgStateUrl + "/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]"

	cases := []struct {
		name     string
		url      string
		expected string
	}{
		{"state peer-group-name", niBgpPgStateUrl + "/state/peer-group-name", `{"openconfig-network-instance:peer-group-name":"PGS1"}`},
		{"state peer-as", niBgpPgStateUrl + "/state/peer-as", `{"openconfig-network-instance:peer-as":65300}`},
		{"state peer-type", niBgpPgStateUrl + "/state/peer-type", `{"openconfig-network-instance:peer-type":"EXTERNAL"}`},
		{"state container", niBgpPgStateUrl + "/state", `{"openconfig-network-instance:state":` + stateJSON + `}`},
		{"afi-safi state enabled", afUrl + "/state/enabled", `{"openconfig-network-instance:enabled":true}`},
		{"afi-safi state container", afUrl + "/state", `{"openconfig-network-instance:state":` + afStateJSON + `}`},
		{"afi-safi instance", afUrl, `{"openconfig-network-instance:afi-safi":[` + afJSON + `]}`},
		{"afi-safis container", niBgpPgStateUrl + "/afi-safis", `{"openconfig-network-instance:afi-safis":{"afi-safi":[` + afJSON + `]}}`},
		{"peer-group instance", niBgpPgStateUrl, `{"openconfig-network-instance:peer-group":[` + pgJSON + `]}`},
		{"peer-groups container", niBgpProtoUrl + "/bgp/peer-groups",
			`{"openconfig-network-instance:peer-groups":{"peer-group":[` + pgJSON + `]}}`},
		{"bgp container", niBgpProtoUrl + "/bgp",
			`{"openconfig-network-instance:bgp":{"global":{"config":{"as":65100}},"peer-groups":{"peer-group":[` + pgJSON + `]}}}`},
	}
	for _, tc := range cases {
		t.Run("GET BGP peer-group "+tc.name, processGetRequest(tc.url, nil, tc.expected, false))
		time.Sleep(1 * time.Second)
	}

	unloadNiBgpPgApplied()
	unloadDB(db.ConfigDB, cleanup)
}

// FRR reports a type for a group with no remote AS, derived from an AS of
// zero, so bgpmon publishes neither; the name alone is reported. A family the
// group has not activated is not listed by FRR and has no state.
func Test_openconfig_network_instance_bgp_peer_group_state_without_remote_as(t *testing.T) {
	cfg, cleanup := niBgpPgStateCfg()
	unloadDB(db.ConfigDB, cleanup)
	loadDB(db.ConfigDB, cfg)
	loadNiBgpPgApplied(map[string]interface{}{"peer_group_name": "PGS1"}, nil, 0)

	notFound := tlerr.NotFoundError{Format: "Resource not found"}
	t.Run("GET BGP peer-group state with no remote AS",
		processGetRequest(niBgpPgStateUrl+"/state", nil, `{"openconfig-network-instance:state":{"peer-group-name":"PGS1"}}`, false))
	time.Sleep(1 * time.Second)
	t.Run("GET BGP peer-group state peer-type with no remote AS",
		processGetRequest(niBgpPgStateUrl+"/state/peer-type", nil, "", true, notFound))
	time.Sleep(1 * time.Second)
	t.Run("GET BGP peer-group afi-safi state of a family FRR has not activated",
		processGetRequest(niBgpPgStateUrl+"/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/state/enabled", nil, "", true, notFound))
	time.Sleep(1 * time.Second)

	unloadNiBgpPgApplied()
	unloadDB(db.ConfigDB, cleanup)
}

// Behind a stale marker nothing is reported, at the leaf or through the
// peer-group instance, which then carries its config alone.
func Test_openconfig_network_instance_bgp_peer_group_state_stale_marker_suppressed(t *testing.T) {
	cfg, cleanup := niBgpPgStateCfg()
	unloadDB(db.ConfigDB, cleanup)
	loadDB(db.ConfigDB, cfg)
	loadNiBgpPgApplied(map[string]interface{}{
		"peer_group_name": "PGS1", "peer_as": "65300", "peer_type": "external"}, []string{"ipv4_unicast"}, time.Hour)

	notFound := tlerr.NotFoundError{Format: "Resource not found"}
	t.Run("GET BGP peer-group state peer-as with a stale marker",
		processGetRequest(niBgpPgStateUrl+"/state/peer-as", nil, "", true, notFound))
	time.Sleep(1 * time.Second)
	t.Run("GET BGP peer-group instance with a stale marker",
		processGetRequest(niBgpPgStateUrl, nil, `{"openconfig-network-instance:peer-group":[{"afi-safis":{"afi-safi":[`+
			`{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","config":{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":true}}]},`+
			`"config":{"peer-as":65300},"peer-group-name":"PGS1"}]}`, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpPgApplied()
	unloadDB(db.ConfigDB, cleanup)
}

// FRR reports a peer-group's remote AS, type and families alone, so its
// other state reads what is configured: each mirrored container and leaf
// reports its config twin, the leaves FRR reports stay FRR's, and a read
// above the state containers carries both.
func Test_openconfig_network_instance_bgp_peer_group_state_mirrors_config(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{"default": map[string]interface{}{"local_asn": "65100"}},
		"BGP_PEER_GROUP": map[string]interface{}{"default|PGS1": map[string]interface{}{
			"asn": "65300", "local_asn": "65111", "name": "pg-m", "conn_retry": "15", "keepalive": "30", "holdtime": "90",
			"tcp_mss": "1400", "passive_mode": "true", "graceful_restart_enable": "true"}},
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PGS1|ipv4_unicast": map[string]interface{}{
			"admin_status": "up", "send_community": "standard", "route_map_in@": "RM_IN", "max_prefix_limit": "500",
			"max_prefix_warning_threshold": "80", "send_default_route": "true", "tx_add_paths": "tx_all_paths"}},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PGS1|ipv4_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PGS1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	v4 := niBgpPgStateUrl + "/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]"
	for _, c := range []struct{ name, url, want string }{
		{"timers state", niBgpPgStateUrl + "/timers/state",
			`{"openconfig-network-instance:state":{"connect-retry":15,"hold-time":90,"keepalive-interval":30}}`},
		{"transport state tcp-mss", niBgpPgStateUrl + "/transport/state/tcp-mss", `{"openconfig-network-instance:tcp-mss":1400}`},
		{"graceful-restart state", niBgpPgStateUrl + "/graceful-restart/state", `{"openconfig-network-instance:state":{"enabled":true}}`},
		{"state local-as", niBgpPgStateUrl + "/state/local-as", `{"openconfig-network-instance:local-as":65111}`},
		{"state description", niBgpPgStateUrl + "/state/description", `{"openconfig-network-instance:description":"pg-m"}`},
		{"afi-safi state send-community-type", v4 + "/state/send-community-type",
			`{"openconfig-network-instance:send-community-type":["STANDARD"]}`},
		{"apply-policy state", v4 + "/apply-policy/state", `{"openconfig-network-instance:state":{"import-policy":["RM_IN"]}}`},
		{"ipv4-unicast state", v4 + "/ipv4-unicast/state", `{"openconfig-network-instance:state":{"send-default-route":true}}`},
		{"prefix-limit state", v4 + "/ipv4-unicast/prefix-limit/state",
			`{"openconfig-network-instance:state":{"max-prefixes":500,"warning-threshold-pct":80}}`},
		{"add-paths state", v4 + "/add-paths/state", `{"openconfig-network-instance:state":{"send":true}}`},
		{"transport container", niBgpPgStateUrl + "/transport",
			`{"openconfig-network-instance:transport":{"config":{"passive-mode":true,"tcp-mss":1400},"state":{"passive-mode":true,"tcp-mss":1400}}}`},
	} {
		t.Run("GET BGP peer-group "+c.name, processGetRequest(c.url, nil, c.want, false))
		time.Sleep(1 * time.Second)
	}
	t.Run("GET BGP peer-group state peer-as without FRR is not found",
		processGetRequest(niBgpPgStateUrl+"/state/peer-as", nil, "", true, tlerr.NotFoundError{Format: "Resource not found"}))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
