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
