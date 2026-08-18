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

func Test_openconfig_network_instance_bgp_global_as(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{"BGP_GLOBALS": map[string]interface{}{"default": ""}}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/config/as"
	urlInputBodyJSON = "{\"openconfig-network-instance:as\":65200}"
	t.Run("PATCH network-instance BGP global AS", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS local_asn mapping", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:as\":65200}"
	t.Run("GET network-instance BGP global AS", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP global AS", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_GLOBALS local_asn cleared", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedInvalidPathErr := tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP global AS mapping: /openconfig-network-instance:network-instances/network-instance[name=blue]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/config/as",
	}
	url = "/openconfig-network-instance:network-instances/network-instance[name=blue]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/config/as"
	urlInputBodyJSON = "{\"openconfig-network-instance:as\":65300}"
	t.Run("PATCH non-default network-instance rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedInvalidPathErr))
	time.Sleep(1 * time.Second)

	expectedInvalidProtoErr := tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP global AS mapping: /openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=STATIC][name=static]/bgp/global/config/as",
	}
	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=STATIC][name=static]/bgp/global/config/as"
	urlInputBodyJSON = "{\"openconfig-network-instance:as\":65300}"
	t.Run("PATCH non-BGP protocol key rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedInvalidProtoErr))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
