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
)

func Test_openconfig_network_instance_bgp_neighbor_aggregate_state(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
				"router_id": "2.2.2.2",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"admin_status":    "up",
				"peer_group_name": "PG1",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"peer_as": "65300",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR":   map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	loadNiBgpNbrState("20.0.0.1", "Established")
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{
			"global": map[string]interface{}{"last_sync": time.Now().Unix()},
		},
		"BGP_NEIGH_APPLIED": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"enabled":          "true",
				"neighbor_address": "20.0.0.1",
				"peer_as":          "65300",
				"peer_group":       "PG1",
			},
		},
	})
	time.Sleep(1 * time.Second)

	neighborEntryJSON := `{"config":{"enabled":true,"peer-group":"PG1"},"neighbor-address":"20.0.0.1",` +
		`"state":{"enabled":true,"neighbor-address":"20.0.0.1","peer-as":65300,"peer-group":"PG1","session-state":"ESTABLISHED"}}`
	expectedGetJSON := `{"openconfig-network-instance:neighbor":[` + neighborEntryJSON + `]}`
	t.Run("GET BGP neighbor list instance includes state",
		processGetRequest(niBgpProtoUrl+"/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiBgpNbrState("20.0.0.1")
	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_NEIGH_APPLIED":      map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_NEIGH_APPLIED_META": map[string]interface{}{"global": ""},
	})
	unloadDB(db.ConfigDB, cleanupTbl)
}
