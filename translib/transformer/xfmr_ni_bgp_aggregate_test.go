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

func loadNiBgpGlobalsApplied(niName string, fields map[string]interface{}) {
	loadDB(db.StateDB, map[string]interface{}{
		"BGP_GLOBALS_APPLIED": map[string]interface{}{
			niName: fields,
		},
		"BGP_GLOBALS_APPLIED_META": map[string]interface{}{
			"global": map[string]interface{}{
				"last_sync": time.Now().Unix(),
			},
		},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiBgpGlobalsApplied(niName string) {
	unloadDB(db.StateDB, map[string]interface{}{
		"BGP_GLOBALS_APPLIED":      map[string]interface{}{niName: ""},
		"BGP_GLOBALS_APPLIED_META": map[string]interface{}{"global": ""},
	})
}

func Test_openconfig_network_instance_bgp_global_state_leaf(t *testing.T) {
	loadNiBgpGlobalsApplied("default", map[string]interface{}{
		"local_asn":      "65200",
		"router_id":      "2.2.2.2",
		"total_paths":    "42",
		"total_prefixes": "17",
	})
	defer unloadNiBgpGlobalsApplied("default")

	url := niBgpProtoUrl + "/bgp/global/state"
	expectedGetJSON := `{"openconfig-network-instance:state":{"as":65200,"router-id":"2.2.2.2","total-paths":42,"total-prefixes":17}}`
	t.Run("GET BGP global state container",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)
}

func Test_openconfig_network_instance_bgp_global_aggregate_state(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
				"router_id": "2.2.2.2",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	loadNiBgpGlobalsApplied("default", map[string]interface{}{
		"local_asn":      "65100",
		"router_id":      "1.1.1.1",
		"total_paths":    "9",
		"total_prefixes": "3",
	})
	defer func() {
		unloadNiBgpGlobalsApplied("default")
		unloadDB(db.ConfigDB, cleanupTbl)
	}()

	globalJSON := `{"config":{"as":65200,"router-id":"2.2.2.2"},` +
		`"state":{"as":65100,"router-id":"1.1.1.1","total-paths":9,"total-prefixes":3}}`
	expectedGetJSON := `{"openconfig-network-instance:global":` + globalJSON + `}`
	t.Run("GET BGP global container merges state",
		processGetRequest(niBgpProtoUrl+"/bgp/global", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)
}
