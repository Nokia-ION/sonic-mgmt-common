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

// Subscribe callback tests for the BGP config containers and the BGP global
// state. These live in the internal package because XfmrSubscInParams cannot
// be built from outside it.

package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

const niBgpSubscProtoUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=BGP][name=bgp]"

type niBgpSubscCase struct {
	name       string
	uri        string
	requestURI string
	dbNum      db.DBNum
	table      string
	key        string
	fields     map[string]string
	pType      NotificationType
}

func runNiBgpSubscCases(t *testing.T, cb SubTreeXfmrSubscribe, cases []niBgpSubscCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := cb(XfmrSubscInParams{uri: tc.uri, requestURI: tc.requestURI, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("subscribe callback failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a mapping, got a virtual table")
			}
			if out.nOpts == nil || out.nOpts.pType != tc.pType {
				t.Fatalf("nOpts=%v, want pType %v", out.nOpts, tc.pType)
			}
			if wantOnChange := tc.pType == OnChange; (out.onChange == OnchangeEnable) != wantOnChange {
				t.Fatalf("onChange=%v for pType %v", out.onChange, tc.pType)
			}
			flds, ok := out.dbDataMap[tc.dbNum][tc.table][tc.key]
			if !ok {
				t.Fatalf("missing %s[%s]: %v", tc.table, tc.key, out.dbDataMap)
			}
			if !reflect.DeepEqual(flds, tc.fields) {
				t.Fatalf("fields=%v, want %v", flds, tc.fields)
			}
		})
	}
}

// Each neighbor config container watches the fields its GET reads, on-change,
// including a request that leaves a key open.
func TestSubscribe_ni_bgp_nbr_cfg_containers(t *testing.T) {
	nbr := niBgpSubscProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]"
	af := nbr + "/afi-safis/afi-safi[afi-safi-name=openconfig-bgp-types:IPV6_UNICAST]"
	runNiBgpSubscCases(t, Subscribe_ni_bgp_nbr_cfg_xfmr, []niBgpSubscCase{
		{"config", nbr + "/config", nbr + "/config", db.ConfigDB, "BGP_NEIGHBOR", "default|11.0.0.155",
			map[string]string{"peer_group_name": "peer-group", "asn": "peer-as", "admin_status": "enabled", "auth_password": "auth-password"}, OnChange},
		{"config peer-as leaf", nbr + "/config", nbr + "/config/peer-as", db.ConfigDB, "BGP_NEIGHBOR", "default|11.0.0.155",
			map[string]string{"asn": "peer-as"}, OnChange},
		{"config of every neighbor", niBgpSubscProtoUrl + "/bgp/neighbors/neighbor/config", niBgpSubscProtoUrl + "/bgp/neighbors/neighbor/config",
			db.ConfigDB, "BGP_NEIGHBOR", "default|*",
			map[string]string{"peer_group_name": "peer-group", "asn": "peer-as", "admin_status": "enabled", "auth_password": "auth-password"}, OnChange},
		{"timers config", nbr + "/timers/config", nbr + "/timers/config", db.ConfigDB, "BGP_NEIGHBOR", "default|11.0.0.155",
			map[string]string{"conn_retry": "connect-retry", "holdtime": "hold-time", "keepalive": "keepalive-interval"}, OnChange},
		{"transport config", nbr + "/transport/config", nbr + "/transport/config", db.ConfigDB, "BGP_NEIGHBOR", "default|11.0.0.155",
			map[string]string{"local_addr": "local-address", "passive_mode": "passive-mode"}, OnChange},
		{"as-path-options config watches every family", nbr + "/as-path-options/config", nbr + "/as-path-options/config",
			db.ConfigDB, "BGP_NEIGHBOR_AF", "default|11.0.0.155|*",
			map[string]string{"allow_as_in": "allow-own-as", "allow_as_count": "allow-own-as"}, OnChange},
		{"afi-safi config", af + "/config", af + "/config", db.ConfigDB, "BGP_NEIGHBOR_AF", "default|11.0.0.155|ipv6_unicast",
			map[string]string{"admin_status": "enabled", "send_community": "send-community-type"}, OnChange},
		{"afi-safi config reached from the instance", af + "/config", af, db.ConfigDB, "BGP_NEIGHBOR_AF", "default|11.0.0.155|ipv6_unicast",
			map[string]string{"admin_status": "enabled", "send_community": "send-community-type"}, OnChange},
		// Both prefix-limit containers read one limit, and the field that moves
		// it between them is watched with max-prefixes.
		{"prefix-limit config", af + "/ipv6-unicast/prefix-limit/config", af + "/ipv6-unicast/prefix-limit/config",
			db.ConfigDB, "BGP_NEIGHBOR_AF", "default|11.0.0.155|ipv6_unicast",
			map[string]string{"max_prefix_limit": "max-prefixes", "max_prefix_force": "max-prefixes",
				"max_prefix_warning_only": "prevent-teardown", "max_prefix_warning_threshold": "warning-threshold-pct"}, OnChange},
		{"prefix-limit-received warning-threshold-pct leaf", af + "/ipv6-unicast/prefix-limit-received/config",
			af + "/ipv6-unicast/prefix-limit-received/config/warning-threshold-pct", db.ConfigDB, "BGP_NEIGHBOR_AF", "default|11.0.0.155|ipv6_unicast",
			map[string]string{"max_prefix_warning_threshold": "warning-threshold-pct"}, OnChange},
	})
}

func TestSubscribe_ni_bgp_pg_cfg_containers(t *testing.T) {
	pg := niBgpSubscProtoUrl + "/bgp/peer-groups/peer-group[peer-group-name=PG1]"
	af := pg + "/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]"
	runNiBgpSubscCases(t, Subscribe_ni_bgp_pg_cfg_xfmr, []niBgpSubscCase{
		{"config", pg + "/config", pg + "/config", db.ConfigDB, "BGP_PEER_GROUP", "default|PG1",
			map[string]string{"asn": "peer-as"}, OnChange},
		{"timers config", pg + "/timers/config", pg + "/timers/config", db.ConfigDB, "BGP_PEER_GROUP", "default|PG1",
			map[string]string{"keepalive": "keepalive-interval", "holdtime": "hold-time"}, OnChange},
		{"afi-safi config", af + "/config", af + "/config", db.ConfigDB, "BGP_PEER_GROUP_AF", "default|PG1|ipv4_unicast",
			map[string]string{"admin_status": "enabled", "send_community": "send-community-type"}, OnChange},
		{"apply-policy config", af + "/apply-policy/config", af + "/apply-policy/config", db.ConfigDB, "BGP_PEER_GROUP_AF", "default|PG1|ipv4_unicast",
			map[string]string{"route_map_in@": "import-policy", "route_map_out@": "export-policy"}, OnChange},
		{"ipv6 prefix-limit config", pg + "/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/ipv6-unicast/prefix-limit/config",
			pg + "/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/ipv6-unicast/prefix-limit/config",
			db.ConfigDB, "BGP_PEER_GROUP_AF", "default|PG1|ipv6_unicast", map[string]string{"max_prefix_limit": "max-prefixes"}, OnChange},
	})
}

func TestSubscribe_ni_bgp_global_cfg_containers(t *testing.T) {
	global := niBgpSubscProtoUrl + "/bgp/global"
	af := global + "/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]"
	runNiBgpSubscCases(t, Subscribe_ni_bgp_global_cfg_xfmr, []niBgpSubscCase{
		{"config", global + "/config", global + "/config", db.ConfigDB, "BGP_GLOBALS", "default",
			map[string]string{"local_asn": "as", "router_id": "router-id"}, OnChange},
		{"route-selection-options config", global + "/route-selection-options/config", global + "/route-selection-options/config",
			db.ConfigDB, "BGP_GLOBALS", "default", map[string]string{"always_compare_med": "always-compare-med"}, OnChange},
		{"ebgp maximum-paths", global + "/use-multiple-paths/ebgp/config", global + "/use-multiple-paths/ebgp/config/maximum-paths",
			db.ConfigDB, "BGP_GLOBALS", "default", map[string]string{"max_ebgp_paths": "maximum-paths"}, OnChange},
		// Activating a family is its default unicast flag on the BGP_GLOBALS
		// row, so the leaf watches that family's field alone.
		{"afi-safi enabled", af + "/config", af + "/config/enabled", db.ConfigDB, "BGP_GLOBALS", "default",
			map[string]string{"default_ipv6_unicast": "enabled"}, OnChange},
		{"afi-safi ibgp maximum-paths", af + "/use-multiple-paths/ibgp/config", af + "/use-multiple-paths/ibgp/config",
			db.ConfigDB, "BGP_GLOBALS_AF", "default|ipv6_unicast", map[string]string{"max_ibgp_paths": "maximum-paths"}, OnChange},
	})
}

// The as and router-id FRR applies change only with configuration and are
// offered on-change; the totals move with the routes, so the container that
// carries them is sampled, as every counter is.
func TestSubscribe_ni_bgp_global_state(t *testing.T) {
	state := niBgpSubscProtoUrl + "/bgp/global/state"
	runNiBgpSubscCases(t, Subscribe_ni_bgp_global_state_xfmr, []niBgpSubscCase{
		{"as leaf", state, state + "/as", db.StateDB, "BGP_GLOBALS_APPLIED", "default", map[string]string{"local_asn": "as"}, OnChange},
		{"router-id leaf", state, state + "/router-id", db.StateDB, "BGP_GLOBALS_APPLIED", "default", map[string]string{"router_id": "router-id"}, OnChange},
		{"total-prefixes leaf", state, state + "/total-prefixes", db.StateDB, "BGP_GLOBALS_APPLIED", "default",
			map[string]string{"total_prefixes": "total-prefixes"}, Sample},
		{"state container", state, state, db.StateDB, "BGP_GLOBALS_APPLIED", "default",
			map[string]string{"local_asn": "as", "router_id": "router-id", "total_paths": "total-paths", "total_prefixes": "total-prefixes"}, Sample},
	})
}

// The container and list nodes that hold no field of their own are virtual
// tables, so a subscription at them reaches their children's mappings.
func TestSubscribe_ni_bgp_cfg_virtual_nodes(t *testing.T) {
	for _, tc := range []struct {
		cb  SubTreeXfmrSubscribe
		uri string
	}{
		{Subscribe_ni_bgp_global_cfg_xfmr, niBgpSubscProtoUrl + "/bgp"},
		{Subscribe_ni_bgp_global_cfg_xfmr, niBgpSubscProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]"},
		{Subscribe_ni_bgp_global_cfg_xfmr, niBgpSubscProtoUrl + "/bgp/peer-groups/peer-group[peer-group-name=PG1]/timers"},
		{Subscribe_ni_bgp_nbr_cfg_xfmr, niBgpSubscProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]/afi-safis"},
		{Subscribe_ni_bgp_nbr_cfg_xfmr, niBgpSubscProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/ipv4-unicast"},
		{Subscribe_ni_bgp_nbr_cfg_xfmr, niBgpSubscProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=11.0.0.155]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/ipv4-unicast/prefix-limit-received"},
		{Subscribe_ni_bgp_pg_cfg_xfmr, niBgpSubscProtoUrl + "/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]"},
	} {
		out, err := tc.cb(XfmrSubscInParams{uri: tc.uri, requestURI: tc.uri, subscProc: TRANSLATE_SUBSCRIBE})
		if err != nil || !out.isVirtualTbl || out.onChange != OnchangeEnable {
			t.Fatalf("%s: err=%v isVirtualTbl=%v onChange=%v, want a virtual on-change node", tc.uri, err, out.isVirtualTbl, out.onChange)
		}
	}
}

// The global transformer is annotated on the protocol list every protocol
// shares, so another protocol's nodes, the protocol instance itself, and the
// CRUD parent-table check keep what translib assumed without a callback.
func TestSubscribe_ni_bgp_global_cfg_keeps_default(t *testing.T) {
	static := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=STATIC][name=static]"
	for _, in := range []XfmrSubscInParams{
		{uri: static + "/static-routes", requestURI: static + "/static-routes", subscProc: TRANSLATE_SUBSCRIBE},
		{uri: niBgpSubscProtoUrl, requestURI: niBgpSubscProtoUrl, subscProc: TRANSLATE_SUBSCRIBE},
		{uri: niBgpSubscProtoUrl + "/bgp/global/config", subscProc: TRANSLATE_SUBSCRIBE},
	} {
		out, err := Subscribe_ni_bgp_global_cfg_xfmr(in)
		if err != nil {
			t.Fatalf("%s: %v", in.uri, err)
		}
		if out.isVirtualTbl || out.dbDataMap != nil || out.nOpts != nil || out.onChange != OnchangeDisable {
			t.Fatalf("%s: got isVirtualTbl=%v map=%v nOpts=%v onChange=%v, want the unbound default",
				in.uri, out.isVirtualTbl, out.dbDataMap, out.nOpts, out.onChange)
		}
	}
}
