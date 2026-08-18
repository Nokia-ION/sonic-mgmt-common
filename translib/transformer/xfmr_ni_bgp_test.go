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

	preReqMap = map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"blue": map[string]interface{}{
				"local_asn": "65250",
			},
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)
	url = "/openconfig-network-instance:network-instances/network-instance[name=blue]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/config/as"
	urlInputBodyJSON = "{\"openconfig-network-instance:as\":65300}"
	t.Run("PATCH non-default network-instance accepted", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"blue": map[string]interface{}{
				"local_asn": "65300",
			},
		},
	}
	t.Run("Verify non-default VRF BGP_GLOBALS local_asn mapping", verifyDbResult(rclient, "BGP_GLOBALS|blue", expectedMap, false))
	time.Sleep(1 * time.Second)
	unloadDB(db.ConfigDB, map[string]interface{}{"BGP_GLOBALS": map[string]interface{}{"blue": ""}})

	expectedInvalidProtoErr := tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP global config mapping: /openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=STATIC][name=static]/bgp/global/config/as",
	}
	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=STATIC][name=static]/bgp/global/config/as"
	urlInputBodyJSON = "{\"openconfig-network-instance:as\":65300}"
	t.Run("PATCH non-BGP protocol key rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedInvalidProtoErr))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_global_router_id(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
				"router_id": "1.1.1.1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{"BGP_GLOBALS": map[string]interface{}{"default": ""}}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/config/router-id"
	urlInputBodyJSON = "{\"openconfig-network-instance:router-id\":\"2.2.2.2\"}"
	t.Run("PATCH network-instance BGP global router-id", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
				"router_id": "2.2.2.2",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS router_id mapping", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:router-id\":\"2.2.2.2\"}"
	t.Run("GET network-instance BGP global router-id", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	cfgURL := "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/config"
	urlInputBodyJSON = "{\"openconfig-network-instance:router-id\":\"3.3.3.3\"}"
	t.Run("PATCH network-instance BGP global config with router-id", processSetRequest(cfgURL, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
				"router_id": "3.3.3.3",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS router_id mapping via config container", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP global router-id", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS router_id cleared", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_global_always_compare_med(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn":          "65100",
				"always_compare_med": "false",
			},
		},
	}
	cleanupTbl := map[string]interface{}{"BGP_GLOBALS": map[string]interface{}{"default": ""}}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/route-selection-options/config/always-compare-med"
	urlInputBodyJSON = "{\"openconfig-network-instance:always-compare-med\":true}"
	t.Run("PATCH network-instance BGP always-compare-med", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn":          "65100",
				"always_compare_med": "true",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS always_compare_med mapping", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:always-compare-med\":true}"
	t.Run("GET network-instance BGP always-compare-med", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP always-compare-med", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS always_compare_med cleared", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_peer_group(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR":   map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/config/peer-group"
	urlInputBodyJSON = "{\"openconfig-network-instance:peer-group\":\"PG1\"}"
	t.Run("PATCH network-instance BGP neighbor peer-group", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":             "65200",
				"peer_group_name": "PG1",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR peer_group_name mapping", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:peer-group\":\"PG1\"}"
	t.Run("GET network-instance BGP neighbor peer-group", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor peer-group", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR peer_group_name cleared", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_enabled(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":          "65200",
				"admin_status": "up",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_GLOBALS":  map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/config/enabled"
	urlInputBodyJSON = "{\"openconfig-network-instance:enabled\":false}"
	t.Run("PATCH network-instance BGP neighbor enabled false", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":          "65200",
				"admin_status": "down",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR admin_status down mapping", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:enabled\":false}"
	t.Run("GET network-instance BGP neighbor enabled false", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor enabled", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR admin_status cleared", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_global_use_multiple_paths_ebgp_maximum_paths(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": map[string]interface{}{
				"max_ebgp_paths": "2",
			},
			"default|ipv6_unicast": map[string]interface{}{
				"max_ebgp_paths": "2",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": "",
			"default|ipv6_unicast": "",
		},
		"BGP_GLOBALS": map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/use-multiple-paths/ebgp/config/maximum-paths"
	urlInputBodyJSON = "{\"openconfig-network-instance:maximum-paths\":16}"
	t.Run("PATCH network-instance BGP eBGP maximum-paths", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": map[string]interface{}{
				"max_ebgp_paths": "16",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv4 max_ebgp_paths mapping", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv6_unicast": map[string]interface{}{
				"max_ebgp_paths": "16",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv6 max_ebgp_paths mapping", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:maximum-paths\":16}"
	t.Run("GET network-instance BGP eBGP maximum-paths", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP eBGP maximum-paths", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv4 max_ebgp_paths cleared", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv6 max_ebgp_paths cleared", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_global_use_multiple_paths_ibgp_maximum_paths(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": map[string]interface{}{
				"max_ibgp_paths": "2",
			},
			"default|ipv6_unicast": map[string]interface{}{
				"max_ibgp_paths": "2",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": "",
			"default|ipv6_unicast": "",
		},
		"BGP_GLOBALS": map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/use-multiple-paths/ibgp/config/maximum-paths"
	urlInputBodyJSON = "{\"openconfig-network-instance:maximum-paths\":24}"
	t.Run("PATCH network-instance BGP iBGP maximum-paths", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": map[string]interface{}{
				"max_ibgp_paths": "24",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv4 max_ibgp_paths mapping", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv6_unicast": map[string]interface{}{
				"max_ibgp_paths": "24",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv6 max_ibgp_paths mapping", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:maximum-paths\":24}"
	t.Run("GET network-instance BGP iBGP maximum-paths", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP iBGP maximum-paths", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv4 max_ibgp_paths cleared", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv6 max_ibgp_paths cleared", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_name(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/config/peer-group-name"
	urlInputBodyJSON = "{\"openconfig-network-instance:peer-group-name\":\"PG1\"}"
	t.Run("PATCH network-instance BGP peer-group-name", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:peer-group-name\":\"PG1\"}"
	t.Run("GET network-instance BGP peer-group-name", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "peer-group-name payload (PG2) does not match key (PG1)",
	}
	urlInputBodyJSON = "{\"openconfig-network-instance:peer-group-name\":\"PG2\"}"
	t.Run("PATCH peer-group-name mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_peer_as(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/config/peer-as"
	urlInputBodyJSON = "{\"openconfig-network-instance:peer-as\":65300}"
	t.Run("PATCH network-instance BGP peer-group peer-as", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65300",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP asn mapping", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:peer-as\":65300}"
	t.Run("GET network-instance BGP peer-group peer-as", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group peer-as", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_PEER_GROUP asn cleared", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_keepalive_interval(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/timers/config/keepalive-interval"
	urlInputBodyJSON = "{\"openconfig-network-instance:keepalive-interval\":30}"
	t.Run("PATCH network-instance BGP peer-group keepalive-interval", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"keepalive": "30",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP keepalive mapping", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:keepalive-interval\":30}"
	t.Run("GET network-instance BGP peer-group keepalive-interval", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group keepalive-interval", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP keepalive cleared", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_hold_time(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/timers/config/hold-time"
	urlInputBodyJSON = "{\"openconfig-network-instance:hold-time\":90}"
	t.Run("PATCH network-instance BGP peer-group hold-time", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"holdtime": "90",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP holdtime mapping", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:hold-time\":90}"
	t.Run("GET network-instance BGP peer-group hold-time", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group hold-time", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP holdtime cleared", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_afi_safi_ipv4_unicast_prefix_limit_max_prefixes(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"max_prefix_limit": "100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PG1|ipv4_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/ipv4-unicast/prefix-limit/config/max-prefixes"
	urlInputBodyJSON = "{\"openconfig-network-instance:max-prefixes\":200}"
	t.Run("PATCH network-instance BGP peer-group ipv4-unicast max-prefixes", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"max_prefix_limit": "200",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF max_prefix_limit mapping", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:max-prefixes\":200}"
	t.Run("GET network-instance BGP peer-group ipv4-unicast max-prefixes", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group ipv4-unicast max-prefixes", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF max_prefix_limit cleared", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_afi_safi_apply_policy_export_policy(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_out@": "RM-OUT-1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PG1|ipv4_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/config/export-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-2\"]}"
	t.Run("PATCH network-instance BGP peer-group AFI-SAFI apply-policy export-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_out@": "RM-OUT-2",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF route_map_out mapping", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-2\"]}"
	t.Run("GET network-instance BGP peer-group AFI-SAFI apply-policy export-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group AFI-SAFI apply-policy export-policy", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF route_map_out cleared", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_send_community(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{},
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": "",
			"default|20.0.0.1|ipv6_unicast": "",
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/config/send-community"
	urlInputBodyJSON = "{\"openconfig-network-instance:send-community\":\"STANDARD\"}"
	t.Run("PATCH network-instance BGP neighbor send-community", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"send_community": "standard",
			},
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"send_community": "standard",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF send_community mapping", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:send-community\":\"STANDARD\"}"
	t.Run("GET network-instance BGP neighbor send-community", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor send-community", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{},
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF send_community cleared", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_as_path_options_allow_own_as(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{},
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": "",
			"default|20.0.0.1|ipv6_unicast": "",
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/as-path-options/config/allow-own-as"
	urlInputBodyJSON = "{\"openconfig-network-instance:allow-own-as\":3}"
	t.Run("PATCH network-instance BGP neighbor allow-own-as", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"allow_as_in":    "true",
				"allow_as_count": "3",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 allow-as mapping", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"allow_as_in":    "true",
				"allow_as_count": "3",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 allow-as mapping", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:allow-own-as\":3}"
	t.Run("GET network-instance BGP neighbor allow-own-as", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor allow-own-as", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 allow-as cleared", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 allow-as cleared", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_afi_safi_apply_policy_import_policy(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V4-OLD",
			},
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V6-KEEP",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": "",
			"default|20.0.0.1|ipv6_unicast": "",
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/config/import-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:import-policy\":[\"RM-IN-V4-NEW\"]}"
	t.Run("PATCH network-instance BGP neighbor AFI-SAFI apply-policy import-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V4-NEW",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 route_map_in mapping", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V6-KEEP",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 route_map_in unchanged", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:import-policy\":[\"RM-IN-V4-NEW\"]}"
	t.Run("GET network-instance BGP neighbor AFI-SAFI apply-policy import-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor AFI-SAFI apply-policy import-policy", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 route_map_in cleared", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V6-KEEP",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 route_map_in still unchanged after delete", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_global_afi_safi_ipv6_unicast_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv6_unicast": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/ipv6-unicast/config"
	urlInputBodyJSON = "{\"openconfig-network-instance:config\":{}}"
	t.Run("PATCH network-instance BGP global AFI-SAFI ipv6-unicast config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv6_unicast row presence", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:config\":{}}"
	t.Run("GET network-instance BGP global AFI-SAFI ipv6-unicast config", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP global AFI-SAFI ipv6-unicast config", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS_AF": map[string]interface{}{
			"default|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_GLOBALS_AF ipv6_unicast row after delete", verifyDbResult(rclient, "BGP_GLOBALS_AF|default|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
