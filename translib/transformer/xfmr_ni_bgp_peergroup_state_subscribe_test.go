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

// Subscribe callback tests for BGP peer-group state. These live in the
// internal package because XfmrSubscInParams cannot be built from outside it.

package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

const niBgpPgSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]"

// Each path watches the fields of its own applied row and nothing else, and is
// offered on-change, since bgpmon updates the row in place and the values move
// only when the peer-group is reconfigured.
func TestSubscribe_ni_bgp_pg_state_paths(t *testing.T) {
	pgState := niBgpPgSubscUrl + "/state"
	afState := niBgpPgSubscUrl + "/afi-safis/afi-safi[afi-safi-name=openconfig-bgp-types:IPV6_UNICAST]/state"
	tests := []struct {
		name       string
		uri        string
		requestURI string
		wantTbl    string
		wantKey    string
		wantFlds   map[string]string
	}{
		{"state container", pgState, pgState, "BGP_PEER_GROUP_APPLIED", "default|PG1",
			map[string]string{"peer_group_name": "peer-group-name", "peer_as": "peer-as", "peer_type": "peer-type"}},
		{"state peer-as leaf", pgState, pgState + "/peer-as", "BGP_PEER_GROUP_APPLIED", "default|PG1",
			map[string]string{"peer_as": "peer-as"}},
		{"peer-group instance anchor", pgState, niBgpPgSubscUrl, "BGP_PEER_GROUP_APPLIED", "default|PG1",
			map[string]string{"peer_group_name": "peer-group-name", "peer_as": "peer-as", "peer_type": "peer-type"}},
		{"afi-safi state container", afState, afState, "BGP_PEER_GROUP_AF_APPLIED", "default|PG1|ipv6_unicast",
			map[string]string{"enabled": "enabled"}},
		{"afi-safi state afi-safi-name leaf", afState, afState + "/afi-safi-name", "BGP_PEER_GROUP_AF_APPLIED", "default|PG1|ipv6_unicast",
			map[string]string{"enabled": "afi-safi-name"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_bgp_pg_state_xfmr(XfmrSubscInParams{uri: tc.uri, requestURI: tc.requestURI, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("Subscribe_ni_bgp_pg_state_xfmr failed: %v", err)
			}
			if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
				t.Fatalf("onChange=%v nOpts=%v, want on-change", out.onChange, out.nOpts)
			}
			flds, ok := out.dbDataMap[db.StateDB][tc.wantTbl][tc.wantKey]
			if !ok {
				t.Fatalf("missing %s[%s]: %v", tc.wantTbl, tc.wantKey, out.dbDataMap)
			}
			if !reflect.DeepEqual(flds, tc.wantFlds) {
				t.Fatalf("fields=%v, want %v", flds, tc.wantFlds)
			}
			if len(out.dbDataMap[db.StateDB]) != 1 {
				t.Fatalf("expected %s alone, got %v", tc.wantTbl, out.dbDataMap)
			}
		})
	}
}

// A request leaving the peer-group open stays on-change: the list path
// transformer resolves the notification key from the Redis key.
func TestSubscribe_ni_bgp_pg_state_unkeyed_stays_on_change(t *testing.T) {
	uri := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group/state"
	out, err := Subscribe_ni_bgp_pg_state_xfmr(XfmrSubscInParams{uri: uri, requestURI: uri, subscProc: TRANSLATE_SUBSCRIBE})
	if err != nil {
		t.Fatalf("Subscribe_ni_bgp_pg_state_xfmr failed: %v", err)
	}
	if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
		t.Fatalf("onChange=%v nOpts=%v, want on-change", out.onChange, out.nOpts)
	}
	if _, ok := out.dbDataMap[db.StateDB]["BGP_PEER_GROUP_APPLIED"]["default|*"]; !ok {
		t.Fatalf("missing wildcard key: %v", out.dbDataMap)
	}
}

func TestSubscribe_ni_bgp_pg_state_non_translate_is_virtual(t *testing.T) {
	out, err := Subscribe_ni_bgp_pg_state_xfmr(XfmrSubscInParams{uri: niBgpPgSubscUrl + "/state", subscProc: TRANSLATE_EXISTS})
	if err != nil || !out.isVirtualTbl {
		t.Fatalf("err=%v isVirtualTbl=%v, want a virtual table", err, out.isVirtualTbl)
	}
}
