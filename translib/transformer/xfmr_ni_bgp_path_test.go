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

package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/openconfig/ygot/ygot"
)

const niBgpPathProto = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=BGP][name=bgp]/bgp"

// A notification for a path that leaves a neighbor, peer-group or afi-safi key
// open is resolved from the Redis key of the row that changed. These go
// through translib's own notification translation, so they also cover the
// path transformer being inherited from the list it is annotated on.
func TestDbToYangPath_ni_bgp_list_path_xfmr(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		dbNum db.DBNum
		table string
		key   []string
		want  string
	}{
		{"neighbor afi-safi apply-policy state", niBgpPathProto + "/neighbors/neighbor[neighbor-address=11.0.0.155]/afi-safis/afi-safi[afi-safi-name=*]/apply-policy/state",
			db.StateDB, "BGP_NEIGH_AF_STATS", []string{"default", "11.0.0.155", "ipv6_unicast"},
			niBgpPathProto + "/neighbors/neighbor[neighbor-address=11.0.0.155]/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/apply-policy/state"},
		{"neighbor afi-safi config across neighbors", niBgpPathProto + "/neighbors/neighbor[neighbor-address=*]/afi-safis/afi-safi[afi-safi-name=*]/apply-policy/config",
			db.ConfigDB, "BGP_NEIGHBOR_AF", []string{"default", "20.0.0.1", "ipv4_unicast"},
			niBgpPathProto + "/neighbors/neighbor[neighbor-address=20.0.0.1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/config"},
		{"neighbor session state keyed by address alone", niBgpPathProto + "/neighbors/neighbor[neighbor-address=*]/state",
			db.StateDB, "NEIGH_STATE_TABLE", []string{"11.0.0.155"},
			niBgpPathProto + "/neighbors/neighbor[neighbor-address=11.0.0.155]/state"},
		{"neighbor transport state", niBgpPathProto + "/neighbors/neighbor[neighbor-address=*]/transport/state",
			db.StateDB, "BGP_NEIGH_APPLIED", []string{"default", "11.0.0.155"},
			niBgpPathProto + "/neighbors/neighbor[neighbor-address=11.0.0.155]/transport/state"},
		{"peer-group state", niBgpPathProto + "/peer-groups/peer-group[peer-group-name=*]/state",
			db.StateDB, "BGP_PEER_GROUP_APPLIED", []string{"default", "PG1"},
			niBgpPathProto + "/peer-groups/peer-group[peer-group-name=PG1]/state"},
		{"peer-group afi-safi state", niBgpPathProto + "/peer-groups/peer-group[peer-group-name=*]/afi-safis/afi-safi[afi-safi-name=*]/state",
			db.StateDB, "BGP_PEER_GROUP_AF_APPLIED", []string{"default", "PG1", "ipv4_unicast"},
			niBgpPathProto + "/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/state"},
		// A path with every key bound reaches the transformer too, and a table
		// it does not describe leaves the path as it was.
		{"keyed path from an undescribed table", niBgpPathProto + "/neighbors/neighbor[neighbor-address=11.0.0.155]/state",
			db.StateDB, "SOME_OTHER_TABLE", []string{"x"},
			niBgpPathProto + "/neighbors/neighbor[neighbor-address=11.0.0.155]/state"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gPath, err := ygot.StringToStructuredPath(tc.path)
			if err != nil {
				t.Fatalf("bad path %s: %v", tc.path, err)
			}
			entry := db.Value{Field: map[string]string{"x": "y"}}
			xlator, err := NewSubscribeNotfRespXlator("ut", gPath, tc.dbNum, &db.TableSpec{Name: tc.table},
				&db.Key{Comp: tc.key}, &entry, [db.MaxDB]*db.DB{}, nil)
			if err != nil {
				t.Fatalf("NewSubscribeNotfRespXlator failed: %v", err)
			}
			out, err := xlator.Translate()
			if err != nil {
				t.Fatalf("Translate failed: %v", err)
			}
			got, err := ygot.PathToString(out)
			if err != nil {
				t.Fatalf("PathToString failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("path=%s\nwant %s", got, tc.want)
			}
		})
	}
}
