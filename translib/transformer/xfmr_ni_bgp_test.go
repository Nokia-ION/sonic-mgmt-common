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

func Test_openconfig_network_instance_bgp_neighbor_peer_as(t *testing.T) {
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
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_GLOBALS":  map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/config/peer-as"
	urlInputBodyJSON = "{\"openconfig-network-instance:peer-as\":65300}"
	t.Run("PATCH network-instance BGP neighbor peer-as", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65300",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR asn mapping", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:peer-as\":65300}"
	t.Run("GET network-instance BGP neighbor peer-as", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor peer-as", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_NEIGHBOR asn cleared", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
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

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP row unchanged for peer-group-name key update", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
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

func Test_openconfig_network_instance_bgp_peer_group_afi_safi_ipv6_unicast_prefix_limit_max_prefixes(t *testing.T) {
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
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"max_prefix_limit": "100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PG1|ipv6_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/ipv6-unicast/prefix-limit/config/max-prefixes"
	urlInputBodyJSON = "{\"openconfig-network-instance:max-prefixes\":200}"
	t.Run("PATCH network-instance BGP peer-group ipv6-unicast max-prefixes", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"max_prefix_limit": "200",
			},
		},
	}
	t.Run("Verify IPv6 BGP_PEER_GROUP_AF max_prefix_limit mapping", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:max-prefixes\":200}"
	t.Run("GET network-instance BGP peer-group ipv6-unicast max-prefixes", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The two families keep separate BGP_PEER_GROUP_AF rows, so writing the
	// IPv6 limit must not leave anything behind on the IPv4 row.
	t.Run("Verify IPv4 BGP_PEER_GROUP_AF row untouched",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group ipv6-unicast max-prefixes", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify IPv6 BGP_PEER_GROUP_AF max_prefix_limit cleared", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
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

func Test_openconfig_network_instance_bgp_peer_group_afi_safi_apply_policy_export_policy_ipv6(t *testing.T) {
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
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"route_map_out@": "RM-OUT-1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PG1|ipv6_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/apply-policy/config/export-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-2\"]}"
	t.Run("PATCH network-instance BGP peer-group IPv6 AFI-SAFI apply-policy export-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"route_map_out@": "RM-OUT-2",
			},
		},
	}
	t.Run("Verify IPv6 BGP_PEER_GROUP_AF route_map_out mapping", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-2\"]}"
	t.Run("GET network-instance BGP peer-group IPv6 AFI-SAFI apply-policy export-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group IPv6 AFI-SAFI apply-policy export-policy", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify IPv6 BGP_PEER_GROUP_AF route_map_out cleared", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_afi_safi_apply_policy_import_policy(t *testing.T) {
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
		// The outbound policy shares the row and must survive every write and
		// the delete below: the two directions are separate fields.
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_in@":  "RM-IN-1",
				"route_map_out@": "RM-OUT-KEEP",
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

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/config/import-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:import-policy\":[\"RM-IN-2\"]}"
	t.Run("PATCH network-instance BGP peer-group AFI-SAFI apply-policy import-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_in@":  "RM-IN-2",
				"route_map_out@": "RM-OUT-KEEP",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF route_map_in mapping leaves route_map_out alone", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:import-policy\":[\"RM-IN-2\"]}"
	t.Run("GET network-instance BGP peer-group AFI-SAFI apply-policy import-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group AFI-SAFI apply-policy import-policy", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_out@": "RM-OUT-KEEP",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF route_map_in cleared and route_map_out kept", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_peer_group_afi_safi_apply_policy_import_policy_ipv6(t *testing.T) {
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
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V6-1",
			},
			// Proves the IPv6 write is keyed on its own row.
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V4-KEEP",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": "",
			"default|PG1|ipv4_unicast": "",
		},
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/apply-policy/config/import-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:import-policy\":[\"RM-IN-V6-2\"]}"
	t.Run("PATCH network-instance BGP peer-group IPv6 AFI-SAFI apply-policy import-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V6-2",
			},
		},
	}
	t.Run("Verify IPv6 BGP_PEER_GROUP_AF route_map_in mapping", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_in@": "RM-IN-V4-KEEP",
			},
		},
	}
	t.Run("Verify IPv4 BGP_PEER_GROUP_AF route_map_in unchanged", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:import-policy\":[\"RM-IN-V6-2\"]}"
	t.Run("GET network-instance BGP peer-group IPv6 AFI-SAFI apply-policy import-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group IPv6 AFI-SAFI apply-policy import-policy", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify IPv6 BGP_PEER_GROUP_AF route_map_in cleared", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
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

func Test_openconfig_network_instance_bgp_neighbor_afi_safi_apply_policy_export_policy(t *testing.T) {
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
		// route_map_in shares the row and must survive: the two directions are
		// separate fields. The IPv6 row proves the write is keyed per family.
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V4-OLD",
				"route_map_in":  "RM-IN-KEEP",
			},
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V6-KEEP",
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

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/config/export-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-V4-NEW\"]}"
	t.Run("PATCH network-instance BGP neighbor AFI-SAFI apply-policy export-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V4-NEW",
				"route_map_in":  "RM-IN-KEEP",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 route_map_out mapping leaves route_map_in alone", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V6-KEEP",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 route_map_out unchanged", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-V4-NEW\"]}"
	t.Run("GET network-instance BGP neighbor AFI-SAFI apply-policy export-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor AFI-SAFI apply-policy export-policy", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"route_map_in": "RM-IN-KEEP",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 route_map_out cleared and route_map_in kept", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V6-KEEP",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 route_map_out still unchanged after delete", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_afi_safi_apply_policy_export_policy_ipv6(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|fc00::1": map[string]interface{}{
				"asn": "65200",
			},
		},
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|fc00::1|ipv6_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V6-OLD",
			},
			"default|fc00::1|ipv4_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V4-KEEP",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|fc00::1|ipv6_unicast": "",
			"default|fc00::1|ipv4_unicast": "",
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|fc00::1": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=fc00::1]/afi-safis/afi-safi[afi-safi-name=IPV6_UNICAST]/apply-policy/config/export-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-V6-NEW\"]}"
	t.Run("PATCH network-instance BGP IPv6 neighbor AFI-SAFI apply-policy export-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|fc00::1|ipv6_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V6-NEW",
			},
		},
	}
	t.Run("Verify IPv6 BGP_NEIGHBOR_AF route_map_out mapping", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|fc00::1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|fc00::1|ipv4_unicast": map[string]interface{}{
				"route_map_out": "RM-OUT-V4-KEEP",
			},
		},
	}
	t.Run("Verify IPv4 BGP_NEIGHBOR_AF route_map_out unchanged", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|fc00::1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:export-policy\":[\"RM-OUT-V6-NEW\"]}"
	t.Run("GET network-instance BGP IPv6 neighbor AFI-SAFI apply-policy export-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP IPv6 neighbor AFI-SAFI apply-policy export-policy", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|fc00::1|ipv6_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify IPv6 BGP_NEIGHBOR_AF route_map_out cleared", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|fc00::1|ipv6_unicast", expectedMap, false))
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
	urlInputBodyJSON = "{}"
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

func Test_openconfig_network_instance_table_connections_table_connection_config_import_policy(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": map[string]interface{}{},
			"RM-REDIST-2": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": "",
			"RM-REDIST-2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections/table-connection[src-protocol=STATIC][dst-protocol=BGP][address-family=IPV4]/config/import-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:import-policy\":[\"RM-REDIST-1\"]}"
	t.Run("PATCH network-instance table-connection import-policy", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"route_map": "RM-REDIST-1",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE route_map mapping", verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:import-policy\":[\"RM-REDIST-1\"]}"
	t.Run("GET network-instance table-connection import-policy", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	reorderedUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections/table-connection[address-family=IPV4][dst-protocol=BGP][src-protocol=STATIC]/config/import-policy"
	t.Run("GET network-instance table-connection import-policy with reordered list keys", processGetRequest(reorderedUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections/table-connection[src-protocol=STATIC][dst-protocol=BGP][address-family=IPV4]/config"
	urlInputBodyJSON = "{\"import-policy\":[\"RM-REDIST-2\"]}"
	t.Run("PATCH network-instance table-connection config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"route_map": "RM-REDIST-2",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE route_map update", verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections/table-connection[src-protocol=STATIC][dst-protocol=BGP][address-family=IPV4]/config/import-policy"
	urlInputBodyJSON = "{\"openconfig-network-instance:import-policy\":[\"RM-REDIST-1\",\"RM-REDIST-2\"]}"
	expectedErr := tlerr.InvalidArgsError{
		Format: "table-connection import-policy supports exactly one policy, got 2",
	}
	t.Run("PATCH network-instance table-connection import-policy rejects multiple values", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_REDISTRIBUTE route_map unchanged after invalid multi-value import-policy", verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections/table-connection[src-protocol=STATIC][dst-protocol=BGP][address-family=IPV4]/config"
	t.Run("DELETE network-instance table-connection config import-policy", processDeleteRequest(url+"/import-policy", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE route_map cleared", verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

const niBgpProtoUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]"

func Test_openconfig_network_instance_bgp_container_hierarchical_set(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65100",
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

	urlInputBodyJSON := `{"openconfig-network-instance:bgp":{` +
		`"global":{"config":{"as":65200,"router-id":"2.2.2.2"}},` +
		`"neighbors":{"neighbor":[{"neighbor-address":"20.0.0.1","config":{"enabled":true,"peer-group":"PG1"}}]},` +
		`"peer-groups":{"peer-group":[{"peer-group-name":"PG1","config":{"peer-as":65300},` +
		`"timers":{"config":{"hold-time":"90","keepalive-interval":"15"}}}]}}}`
	t.Run("PATCH network-instance BGP container with nested global, neighbor and peer-group payload",
		processSetRequest(niBgpProtoUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
				"router_id": "2.2.2.2",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS mapping from container payload", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	// enabled true maps to the admin_status enum, not to a boolean string.
	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"admin_status":    "up",
				"peer_group_name": "PG1",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR mapping from container payload", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn":       "65300",
				"holdtime":  "90",
				"keepalive": "15",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP mapping from container payload", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = `{` +
		`"global":{"config":{"as":65201}},` +
		`"neighbors":{"neighbor":[{"neighbor-address":"20.0.0.1","config":{"enabled":false}}]}}`
	t.Run("PATCH network-instance BGP container at the bgp node",
		processSetRequest(niBgpProtoUrl+"/bgp", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65201",
				"router_id": "2.2.2.2",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS update from bgp node payload", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"admin_status":    "down",
				"peer_group_name": "PG1",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR update from bgp node payload", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_get(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
				"router_id": "2.2.2.2",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn":       "65300",
				"holdtime":  "90",
				"keepalive": "15",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"admin_status":    "up",
				"peer_group_name": "PG1",
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

	globalJSON := `{"config":{"as":65200,"router-id":"2.2.2.2"}}`
	neighborEntryJSON := `{"config":{"enabled":true,"peer-group":"PG1"},"neighbor-address":"20.0.0.1"}`
	peerGroupEntryJSON := `{"config":{"peer-as":65300},"peer-group-name":"PG1",` +
		`"timers":{"config":{"hold-time":"90","keepalive-interval":"15"}}}`

	expectedGetJSON := `{"openconfig-network-instance:bgp":{"global":` + globalJSON +
		`,"neighbors":{"neighbor":[` + neighborEntryJSON + `]}` +
		`,"peer-groups":{"peer-group":[` + peerGroupEntryJSON + `]}}}`
	t.Run("GET network-instance BGP container", processGetRequest(niBgpProtoUrl+"/bgp", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:global":` + globalJSON + `}`
	t.Run("GET network-instance BGP global container", processGetRequest(niBgpProtoUrl+"/bgp/global", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:neighbors":{"neighbor":[` + neighborEntryJSON + `]}}`
	t.Run("GET network-instance BGP neighbors container", processGetRequest(niBgpProtoUrl+"/bgp/neighbors", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The whole neighbor list must come from BGP_NEIGHBOR only. A generically
	// expanded list reports one synthetic entry keyed by the network instance.
	expectedGetJSON = `{"openconfig-network-instance:neighbor":[` + neighborEntryJSON + `]}`
	t.Run("GET network-instance BGP neighbor list", processGetRequest(niBgpProtoUrl+"/bgp/neighbors/neighbor", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP neighbor list instance",
		processGetRequest(niBgpProtoUrl+"/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:peer-groups":{"peer-group":[` + peerGroupEntryJSON + `]}}`
	t.Run("GET network-instance BGP peer-groups container", processGetRequest(niBgpProtoUrl+"/bgp/peer-groups", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:peer-group":[` + peerGroupEntryJSON + `]}`
	t.Run("GET network-instance BGP peer-group list", processGetRequest(niBgpProtoUrl+"/bgp/peer-groups/peer-group", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP peer-group list instance",
		processGetRequest(niBgpProtoUrl+"/bgp/peer-groups/peer-group[peer-group-name=PG1]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_unmapped_payload_rejected(t *testing.T) {
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

	urlInputBodyJSON := `{"openconfig-network-instance:bgp":{"neighbors":{"neighbor":[` +
		`{"neighbor-address":"20.0.0.1","config":{"peer-group":"PG1"},` +
		`"timers":{"config":{"hold-time":"90"}}}]}}}`
	expectedErr := tlerr.NotSupportedError{
		Format: "BGP container payload carries leaves that have no CONFIG_DB mapping: bgp/neighbors/neighbor[20.0.0.1]/timers/config/hold-time",
	}
	t.Run("PATCH network-instance BGP container rejects unmapped leaf",
		processSetRequest(niBgpProtoUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// The whole request is rejected, so the mapped leaves must not be applied.
	expectedMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS unchanged after rejected container payload", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_NEIGHBOR not created after rejected container payload",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_delete_not_supported(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
			},
		},
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
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

	expectedErr := tlerr.NotSupportedError{
		Format: "DELETE is not supported at BGP container level; delete mapped child paths or list instances instead",
	}
	t.Run("DELETE network-instance BGP container rejected", processDeleteRequest(niBgpProtoUrl+"/bgp", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbors container rejected", processDeleteRequest(niBgpProtoUrl+"/bgp/neighbors", true))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS retained after rejected container delete", verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR retained after rejected container delete", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_non_bgp_protocol_rejected(t *testing.T) {
	staticProtoUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=STATIC][name=static]"

	urlInputBodyJSON := `{"neighbor":[` +
		`{"neighbor-address":"20.0.0.1","config":{"enabled":true}}]}`
	expectedErr := tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP global config mapping: " + staticProtoUrl + "/bgp/neighbors",
	}
	t.Run("PATCH BGP container path under non-BGP protocol rejected",
		processSetRequest(staticProtoUrl+"/bgp/neighbors", urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("GET BGP container path under non-BGP protocol rejected",
		processGetRequest(staticProtoUrl+"/bgp/neighbors", nil, "", true))
	time.Sleep(1 * time.Second)
}

func Test_openconfig_network_instance_bgp_peer_group_config_container_set(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	pgUrl := niBgpProtoUrl + "/bgp/peer-groups/peer-group[peer-group-name=PG1]"

	// A request at the config container carries peer-as alongside the key, and
	// both have to be honoured rather than dropped with the key.
	urlInputBodyJSON := `{"peer-group-name":"PG1","peer-as":65300}`
	t.Run("PATCH BGP peer-group config container with peer-as",
		processSetRequest(pgUrl+"/config", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65300",
			},
		},
	}
	t.Run("Verify peer-as from config container request", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A payload naming a different peer-group than the key is still rejected.
	urlInputBodyJSON = `{"peer-group-name":"PG2"}`
	t.Run("PATCH BGP peer-group config container with mismatched key rejected",
		processSetRequest(pgUrl+"/config", urlInputBodyJSON, "PATCH", true, nil))
	time.Sleep(1 * time.Second)

	// A request at the timers config container carries either or both timers.
	urlInputBodyJSON = `{"hold-time":"90","keepalive-interval":"30"}`
	t.Run("PATCH BGP peer-group timers config container",
		processSetRequest(pgUrl+"/timers/config", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn":       "65300",
				"holdtime":  "90",
				"keepalive": "30",
			},
		},
	}
	t.Run("Verify both timers from timers config container request", verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_multi_leaf_row_merge(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF":   map[string]interface{}{"default|20.0.0.1|ipv4_unicast": ""},
		"BGP_NEIGHBOR":      map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PG1|ipv4_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// One neighbor contributes fields to BGP_NEIGHBOR from config and to
	// BGP_NEIGHBOR_AF from as-path-options; the config fields must survive the
	// parent-row write that the AF mapping performs for the same key.
	urlInputBodyJSON := `{` +
		`"neighbors":{"neighbor":[{"neighbor-address":"20.0.0.1",` +
		`"config":{"peer-group":"PG1","enabled":true},` +
		`"as-path-options":{"config":{"allow-own-as":3}}}]},` +
		`"peer-groups":{"peer-group":[{"peer-group-name":"PG1",` +
		`"afi-safis":{"afi-safi":[{"afi-safi-name":"IPV4_UNICAST",` +
		`"ipv4-unicast":{"prefix-limit":{"config":{"max-prefixes":200}}},` +
		`"apply-policy":{"config":{"export-policy":["RM-OUT-2"]}}}]}}]}}`
	t.Run("PATCH BGP container with several leaves mapping onto one row",
		processSetRequest(niBgpProtoUrl+"/bgp", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"admin_status":    "up",
				"peer_group_name": "PG1",
			},
		},
	}
	t.Run("Verify neighbor config fields survive the AF parent-row write",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"max_prefix_limit": "200",
				"route_map_out@":   "RM-OUT-2",
			},
		},
	}
	t.Run("Verify both peer-group AF leaves land on one row",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_apply_policy_both_directions(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF":   map[string]interface{}{"default|20.0.0.1|ipv4_unicast": ""},
		"BGP_NEIGHBOR":      map[string]interface{}{"default|20.0.0.1": ""},
		"BGP_PEER_GROUP_AF": map[string]interface{}{"default|PG1|ipv4_unicast": ""},
		"BGP_PEER_GROUP":    map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":       map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// Both apply-policy directions of both a peer-group and a neighbor, named
	// from the BGP container rather than at their own leaves. Each direction is
	// a separate CONFIG_DB field, so a container payload carrying both has to
	// land both on the one row.
	urlInputBodyJSON := `{` +
		`"neighbors":{"neighbor":[{"neighbor-address":"20.0.0.1",` +
		`"config":{"peer-as":65200},` +
		`"afi-safis":{"afi-safi":[{"afi-safi-name":"IPV4_UNICAST",` +
		`"apply-policy":{"config":{"import-policy":["RM-NBR-IN"],"export-policy":["RM-NBR-OUT"]}}}]}}]},` +
		`"peer-groups":{"peer-group":[{"peer-group-name":"PG1",` +
		`"afi-safis":{"afi-safi":[{"afi-safi-name":"IPV4_UNICAST",` +
		`"apply-policy":{"config":{"import-policy":["RM-PG-IN"],"export-policy":["RM-PG-OUT"]}}}]}}]}}`
	t.Run("PATCH BGP container with both apply-policy directions",
		processSetRequest(niBgpProtoUrl+"/bgp", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"route_map_in@":  "RM-PG-IN",
				"route_map_out@": "RM-PG-OUT",
			},
		},
	}
	t.Run("Verify both peer-group apply-policy directions on one row",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status":  "up",
				"route_map_in":  "RM-NBR-IN",
				"route_map_out": "RM-NBR-OUT",
			},
		},
	}
	t.Run("Verify both neighbor apply-policy directions on one row",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	pgUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/peer-groups/peer-group[peer-group-name=PG1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/config"
	expectedGetJSON := "{\"openconfig-network-instance:config\":{\"export-policy\":[\"RM-PG-OUT\"],\"import-policy\":[\"RM-PG-IN\"]}}"
	t.Run("GET peer-group apply-policy config reports both directions",
		processGetRequest(pgUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	nbrUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/apply-policy/config"
	expectedGetJSON = "{\"openconfig-network-instance:config\":{\"export-policy\":[\"RM-NBR-OUT\"],\"import-policy\":[\"RM-NBR-IN\"]}}"
	t.Run("GET neighbor apply-policy config reports both directions",
		processGetRequest(nbrUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_key_only_payload(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG2": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// A container payload that names a list instance without setting any other
	// leaf is a request to create that instance, not an unmapped payload.
	urlInputBodyJSON := `{` +
		`"peer-groups":{"peer-group":[{"peer-group-name":"PG2"}]}}`
	t.Run("PATCH BGP container with a key-only peer-group payload",
		processSetRequest(niBgpProtoUrl+"/bgp", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG2": map[string]interface{}{},
		},
	}
	t.Run("Verify key-only payload creates the peer-group row",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG2", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_afi_safi_config_enabled(t *testing.T) {
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
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": "",
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

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/config"
	urlInputBodyJSON = "{\"afi-safi-name\":\"IPV4_UNICAST\",\"enabled\":true}"
	t.Run("PATCH network-instance BGP neighbor AFI-SAFI config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// admin_status on the AF row is what makes frrcfgd emit
	// "neighbor <addr> activate" under the address-family.
	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 admin_status mapping", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP neighbor AFI-SAFI config enabled",
		processGetRequest(url+"/enabled", nil, "{\"openconfig-network-instance:enabled\":true}", false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"openconfig-network-instance:enabled\":false}"
	t.Run("PATCH network-instance BGP neighbor AFI-SAFI config enabled false",
		processSetRequest(url+"/enabled", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status": "down",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 admin_status down", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor AFI-SAFI config enabled", processDeleteRequest(url+"/enabled", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 admin_status cleared", verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_neighbor_afi_safi_enabled(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": "",
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

	// A neighbor payload that names an AFI-SAFI activates it there, and the
	// import-policy under the same AFI-SAFI lands on the same row.
	urlInputBodyJSON := `{` +
		`"neighbors":{"neighbor":[{"neighbor-address":"20.0.0.1",` +
		`"config":{"peer-as":65200},` +
		`"afi-safis":{"afi-safi":[{"afi-safi-name":"IPV4_UNICAST",` +
		`"config":{"afi-safi-name":"IPV4_UNICAST","enabled":true},` +
		`"apply-policy":{"config":{"import-policy":["RM-IN-V4"]}}}]}}]}}`
	t.Run("PATCH BGP container with neighbor AFI-SAFI enabled",
		processSetRequest(niBgpProtoUrl+"/bgp", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status":  "up",
				"route_map_in@": "RM-IN-V4",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF row from container payload",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
