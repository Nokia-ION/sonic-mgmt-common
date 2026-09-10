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

// The protocol instance name is not a CONFIG_DB key component, so both served
// spellings reach the same BGP_GLOBALS row and each is echoed back as sent.
// Every other name would be one more alias for that row and is refused.
func Test_openconfig_network_instance_bgp_protocol_instance_name(t *testing.T) {
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

	niProtoUrlPfx := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=BGP][name="
	upperUrl := niProtoUrlPfx + "BGP]/bgp/global/config/as"
	unknownUrl := niProtoUrlPfx + "bgp2]/bgp/global/config/as"
	// The identifier may arrive module qualified, which is how a client that
	// round-trips a GET payload spells it, since the payload reports the
	// identity with its defining module.
	qualifiedUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=openconfig-policy-types:BGP][name=bgp]/bgp/global/config/as"
	// The peer-group and the neighbor are written through parsers of their own,
	// whose parse failure is otherwise left to the BGP container dispatch. An
	// unserved name has to be refused there too: the protocol instance prefix
	// matches whatever follows it, so a name error taken for a container-level
	// request would accept the write and map nothing.
	unknownPgUrl := niProtoUrlPfx + "bgp2]/bgp/peer-groups/peer-group[peer-group-name=PGT9]/config/peer-group-name"
	unknownNbrUrl := niProtoUrlPfx + "bgp2]/bgp/neighbors/neighbor[neighbor-address=20.0.9.1]/config/peer-as"

	t.Run("GET BGP global AS with upper case protocol instance name",
		processGetRequest(upperUrl, nil, "{\"openconfig-network-instance:as\":65100}", false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH BGP global AS with upper case protocol instance name",
		processSetRequest(upperUrl, "{\"openconfig-network-instance:as\":65200}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_GLOBALS reached via upper case protocol instance name",
		verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The transformers accept the qualified identifier, but nothing above them
	// does: ygot renders a list key as the bare BGP and compares path keys as
	// plain strings, so the qualified spelling matches no protocol instance and
	// the read reports an empty container rather than the row it names.
	t.Run("GET BGP global AS with module qualified identifier reads empty",
		processGetRequest(qualifiedUrl, nil, "{}", false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.NotSupportedError{
		Format: "unsupported BGP protocol instance name \"bgp2\"; expected \"bgp\" or \"BGP\"",
	}
	t.Run("GET BGP global AS with unknown protocol instance name rejected",
		processGetRequest(unknownUrl, nil, "", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("PATCH BGP global AS with unknown protocol instance name rejected",
		processSetRequest(unknownUrl, "{\"openconfig-network-instance:as\":65300}", "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_GLOBALS unchanged after rejected protocol instance name",
		verifyDbResult(rclient, "BGP_GLOBALS|default", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH BGP peer-group with unknown protocol instance name rejected",
		processSetRequest(unknownPgUrl, "{\"openconfig-network-instance:peer-group-name\":\"PGT9\"}", "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify no BGP_PEER_GROUP row created for unknown protocol instance name",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PGT9", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH BGP neighbor with unknown protocol instance name rejected",
		processSetRequest(unknownNbrUrl, "{\"openconfig-network-instance:peer-as\":65309}", "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify no BGP_NEIGHBOR row created for unknown protocol instance name",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.9.1", map[string]interface{}{}, false))
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

func Test_openconfig_network_instance_bgp_neighbor_auth_password(t *testing.T) {
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

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/config/auth-password"
	urlInputBodyJSON = "{\"openconfig-network-instance:auth-password\":\"Sonic123\"}"
	t.Run("PATCH network-instance BGP neighbor auth-password", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":           "65200",
				"auth_password": "Sonic123",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR auth_password mapping", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:auth-password\":\"Sonic123\"}"
	t.Run("GET network-instance BGP neighbor auth-password", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor auth-password", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR auth_password cleared", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_connect_retry(t *testing.T) {
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

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/timers/config/connect-retry"
	urlInputBodyJSON = "{\"openconfig-network-instance:connect-retry\":45}"
	t.Run("PATCH network-instance BGP neighbor connect-retry", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":        "65200",
				"conn_retry": "45",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR conn_retry mapping", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:connect-retry\":45}"
	t.Run("GET network-instance BGP neighbor connect-retry", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor connect-retry", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR conn_retry cleared", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_hold_time(t *testing.T) {
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

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/timers/config/hold-time"
	urlInputBodyJSON = "{\"openconfig-network-instance:hold-time\":90}"
	t.Run("PATCH network-instance BGP neighbor hold-time", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":      "65200",
				"holdtime": "90",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR holdtime mapping", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:hold-time\":90}"
	t.Run("GET network-instance BGP neighbor hold-time", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor hold-time", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR holdtime cleared", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_keepalive_interval(t *testing.T) {
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

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/timers/config/keepalive-interval"
	urlInputBodyJSON = "{\"openconfig-network-instance:keepalive-interval\":30}"
	t.Run("PATCH network-instance BGP neighbor keepalive-interval", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":       "65200",
				"keepalive": "30",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR keepalive mapping", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:keepalive-interval\":30}"
	t.Run("GET network-instance BGP neighbor keepalive-interval", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor keepalive-interval", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR keepalive cleared", verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_neighbor_timers_config_container(t *testing.T) {
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

	// A request at the timers config container can carry any of the three.
	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/timers/config"
	urlInputBodyJSON = "{\"connect-retry\":45,\"hold-time\":90,\"keepalive-interval\":30}"
	t.Run("PATCH network-instance BGP neighbor timers config container",
		processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":        "65200",
				"conn_retry": "45",
				"holdtime":   "90",
				"keepalive":  "30",
			},
		},
	}
	t.Run("Verify all three timers from timers config container request",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:config\":{\"connect-retry\":45,\"hold-time\":90,\"keepalive-interval\":30}}"
	t.Run("GET network-instance BGP neighbor timers config container",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The container delete takes all three timers with it and leaves the rest
	// of the BGP_NEIGHBOR row alone.
	t.Run("DELETE network-instance BGP neighbor timers config container", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR timers cleared by timers config container DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
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

// The state container has no datastore of its own: it reports the same
// ROUTE_REDISTRIBUTE row the config container writes, and refuses writes.
func Test_openconfig_network_instance_table_connections_table_connection_state(t *testing.T) {
	var url string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": map[string]interface{}{},
		},
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"route_map": "RM-REDIST-1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections/table-connection[src-protocol=STATIC][dst-protocol=BGP][address-family=IPV4]/state"
	expectedGetJSON := `{"openconfig-network-instance:state":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","import-policy":["RM-REDIST-1"],` +
		`"src-protocol":"openconfig-policy-types:STATIC"}}`
	t.Run("GET network-instance table-connection state container",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance table-connection state import-policy",
		processGetRequest(url+"/import-policy", nil,
			`{"openconfig-network-instance:import-policy":["RM-REDIST-1"]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance table-connection state src-protocol",
		processGetRequest(url+"/src-protocol", nil,
			`{"openconfig-network-instance:src-protocol":"openconfig-policy-types:STATIC"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance table-connection state dst-protocol",
		processGetRequest(url+"/dst-protocol", nil,
			`{"openconfig-network-instance:dst-protocol":"openconfig-policy-types:BGP"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance table-connection state address-family",
		processGetRequest(url+"/address-family", nil,
			`{"openconfig-network-instance:address-family":"openconfig-types:IPV4"}`, false))
	time.Sleep(1 * time.Second)

	// A write aimed at state must be refused rather than land on config.
	expectedErr := tlerr.NotSupportedError{
		Format: "table-connection state is read-only; write to the config container instead",
	}
	t.Run("PATCH network-instance table-connection state rejected",
		processSetRequest(url+"/import-policy", "{\"openconfig-network-instance:import-policy\":[\"RM-REDIST-1\"]}", "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance table-connection state rejected",
		processDeleteRequest(url, true, expectedErr))
	time.Sleep(1 * time.Second)

	// The refused writes must leave the row that backs the state read alone.
	expectedMap := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"route_map": "RM-REDIST-1",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE unchanged after refused state writes",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

const niTableConnUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections"

// A table-connection with a route-map and one without, both created by a single
// container level request, then read back from the container and from the list.
func Test_openconfig_network_instance_table_connections_container_hierarchical_set(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4":    "",
			"default|connected|bgp|ipv6": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	urlInputBodyJSON := `{"table-connection":[` +
		`{"src-protocol":"openconfig-policy-types:STATIC","dst-protocol":"openconfig-policy-types:BGP",` +
		`"address-family":"openconfig-types:IPV4","config":{"src-protocol":"openconfig-policy-types:STATIC",` +
		`"dst-protocol":"openconfig-policy-types:BGP","address-family":"openconfig-types:IPV4",` +
		`"import-policy":["RM-REDIST-1"]}},` +
		`{"src-protocol":"openconfig-policy-types:DIRECTLY_CONNECTED","dst-protocol":"openconfig-policy-types:BGP",` +
		`"address-family":"openconfig-types:IPV6","config":{"src-protocol":"openconfig-policy-types:DIRECTLY_CONNECTED",` +
		`"dst-protocol":"openconfig-policy-types:BGP","address-family":"openconfig-types:IPV6"}}]}`
	t.Run("PATCH network-instance table-connections container",
		processSetRequest(niTableConnUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"route_map": "RM-REDIST-1",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE row with route_map from container payload",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A table-connection without an import-policy is a plain redistribution, so
	// the row carries only the placeholder field REDIS needs to hold it.
	expectedMap = map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|connected|bgp|ipv6": map[string]interface{}{
				"NULL": "NULL",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE row without route_map from container payload",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|connected|bgp|ipv6", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The state container reports the same row back, so it accompanies config
	// wherever a request reaches the list instance or above.
	v4EntryJSON := `{"address-family":"openconfig-types:IPV4","config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","import-policy":["RM-REDIST-1"],` +
		`"src-protocol":"openconfig-policy-types:STATIC"},"dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:STATIC",` +
		`"state":{"address-family":"openconfig-types:IPV4","dst-protocol":"openconfig-policy-types:BGP",` +
		`"import-policy":["RM-REDIST-1"],"src-protocol":"openconfig-policy-types:STATIC"}}`
	v6EntryJSON := `{"address-family":"openconfig-types:IPV6","config":{"address-family":"openconfig-types:IPV6",` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:DIRECTLY_CONNECTED"},` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:DIRECTLY_CONNECTED",` +
		`"state":{"address-family":"openconfig-types:IPV6","dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:DIRECTLY_CONNECTED"}}`

	expectedGetJSON := `{"openconfig-network-instance:table-connections":{"table-connection":[` +
		v4EntryJSON + `,` + v6EntryJSON + `]}}`
	t.Run("GET network-instance table-connections container",
		processGetRequest(niTableConnUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:table-connection":[` + v4EntryJSON + `,` + v6EntryJSON + `]}`
	t.Run("GET network-instance table-connection list",
		processGetRequest(niTableConnUrl+"/table-connection", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.NotSupportedError{
		Format: "DELETE is not supported at the table-connections container level; delete a table-connection instead",
	}
	t.Run("DELETE table-connections container rejected",
		processDeleteRequest(niTableConnUrl, true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify ROUTE_REDISTRIBUTE row kept after rejected container DELETE",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|connected|bgp|ipv6", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// The list instance level: a request anchored on the table-connection itself,
// including one that names only the keys and so asks for a redistribution with
// no route-map.
func Test_openconfig_network_instance_table_connection_list_instance(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"RM-REDIST-1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	instUrl := niTableConnUrl + "/table-connection[address-family=IPV4][dst-protocol=BGP][src-protocol=STATIC]"

	urlInputBodyJSON := `{"openconfig-network-instance:config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:STATIC",` +
		`"import-policy":["RM-REDIST-1"]}}`
	t.Run("PATCH network-instance table-connection list instance",
		processSetRequest(instUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"route_map": "RM-REDIST-1",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE row from list instance payload",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	entryJSON := `{"address-family":"openconfig-types:IPV4","config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","import-policy":["RM-REDIST-1"],` +
		`"src-protocol":"openconfig-policy-types:STATIC"},"dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:STATIC",` +
		`"state":{"address-family":"openconfig-types:IPV4","dst-protocol":"openconfig-policy-types:BGP",` +
		`"import-policy":["RM-REDIST-1"],"src-protocol":"openconfig-policy-types:STATIC"}}`
	expectedGetJSON := `{"openconfig-network-instance:table-connection":[` + entryJSON + `]}`
	t.Run("GET network-instance table-connection list instance",
		processGetRequest(instUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance table-connection src-protocol key leaf",
		processGetRequest(instUrl+"/src-protocol", nil,
			`{"openconfig-network-instance:src-protocol":"openconfig-policy-types:STATIC"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance table-connection address-family key leaf",
		processGetRequest(instUrl+"/address-family", nil,
			`{"openconfig-network-instance:address-family":"openconfig-types:IPV4"}`, false))
	time.Sleep(1 * time.Second)

	// A key is the row identity, so it cannot be cleared on its own.
	expectedErr := tlerr.NotSupportedError{
		Format: "DELETE is not supported for a table-connection list key; delete the table-connection instead",
	}
	t.Run("DELETE table-connection key leaf rejected",
		processDeleteRequest(instUrl+"/src-protocol", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify ROUTE_REDISTRIBUTE row kept after rejected key leaf DELETE",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance table-connection list instance", processDeleteRequest(instUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify ROUTE_REDISTRIBUTE row removed by list instance DELETE",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	// The keys alone are a complete request: they ask for a redistribution with
	// no route-map, which is what a REPLACE of the whole instance sends.
	urlInputBodyJSON = `{"openconfig-network-instance:address-family":"openconfig-types:IPV4",` +
		`"openconfig-network-instance:config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:STATIC"},` +
		`"openconfig-network-instance:dst-protocol":"openconfig-policy-types:BGP",` +
		`"openconfig-network-instance:src-protocol":"openconfig-policy-types:STATIC"}`
	t.Run("REPLACE network-instance table-connection list instance without import-policy",
		processSetRequest(instUrl, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"NULL": "NULL",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE row created without route_map",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	entryJSON = `{"address-family":"openconfig-types:IPV4","config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:STATIC"},` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:STATIC",` +
		`"state":{"address-family":"openconfig-types:IPV4","dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:STATIC"}}`
	expectedGetJSON = `{"openconfig-network-instance:table-connection":[` + entryJSON + `]}`
	t.Run("GET network-instance table-connection list instance without import-policy",
		processGetRequest(instUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// The config container level: the key leaves it repeats, and a payload that
// names no import-policy at all.
func Test_openconfig_network_instance_table_connection_config_keys_without_policy(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	cfgUrl := niTableConnUrl + "/table-connection[address-family=IPV4][dst-protocol=BGP][src-protocol=STATIC]/config"

	urlInputBodyJSON := `{"address-family":"openconfig-types:IPV4","dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:STATIC"}`
	t.Run("PATCH network-instance table-connection config without import-policy",
		processSetRequest(cfgUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_REDISTRIBUTE": map[string]interface{}{
			"default|static|bgp|ipv4": map[string]interface{}{
				"NULL": "NULL",
			},
		},
	}
	t.Run("Verify ROUTE_REDISTRIBUTE row created from config keys alone",
		verifyDbResult(rclient, "ROUTE_REDISTRIBUTE|default|static|bgp|ipv4", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:STATIC"}}`
	t.Run("GET network-instance table-connection config without import-policy",
		processGetRequest(cfgUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance table-connection config dst-protocol key leaf",
		processGetRequest(cfgUrl+"/dst-protocol", nil,
			`{"openconfig-network-instance:dst-protocol":"openconfig-policy-types:BGP"}`, false))
	time.Sleep(1 * time.Second)

	// default-import-policy has no CONFIG_DB mapping, so the whole request is
	// rejected instead of being reduced to the leaves that do map.
	urlInputBodyJSON = `{"src-protocol":"openconfig-policy-types:STATIC","default-import-policy":"REJECT_ROUTE"}`
	expectedErr := tlerr.NotSupportedError{
		Format: "table-connections payload carries leaves that have no CONFIG_DB mapping: " +
			"table-connections/table-connection[src-protocol=STATIC][dst-protocol=BGP][address-family=IPV4]" +
			"/config/default-import-policy",
	}
	t.Run("PATCH table-connection config rejects unmapped leaf",
		processSetRequest(cfgUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
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

// The levels above the neighbor's own annotated containers are served by the
// BGP container level handler rather than by the neighbor transformer, and
// that is a separate mapping which used to drop both of these buckets: a
// payload naming them was accepted and written nowhere, and a read reported
// them not at all. The peer-group timers beside them were always carried, so
// this keeps the two in step.
func Test_openconfig_network_instance_bgp_container_neighbor_timers_and_auth_password(t *testing.T) {
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

	urlInputBodyJSON := `{` +
		`"neighbors":{"neighbor":[{"neighbor-address":"20.0.0.1",` +
		`"config":{"neighbor-address":"20.0.0.1","auth-password":"Sonic123"},` +
		`"timers":{"config":{"connect-retry":"43","hold-time":"99","keepalive-interval":"33"}}}]}}`
	t.Run("PATCH network-instance BGP container with neighbor timers and auth-password",
		processSetRequest(niBgpProtoUrl+"/bgp", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":           "65200",
				"auth_password": "Sonic123",
				"conn_retry":    "43",
				"holdtime":      "99",
				"keepalive":     "33",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR timers and auth_password from container payload",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The timers container sits above the annotated config container, so this
	// read is served by the same handler the write above went through.
	url := niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]/timers"
	expectedGetJSON := "{\"openconfig-network-instance:timers\":{\"config\":{\"connect-retry\":43,\"hold-time\":99,\"keepalive-interval\":33}}}"
	t.Run("GET network-instance BGP neighbor timers container",
		processGetRequest(url, nil, expectedGetJSON, false))
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
		`"timers":{"config":{"minimum-advertisement-interval":"30"}}}]}}}`
	expectedErr := tlerr.NotSupportedError{
		Format: "BGP container payload carries leaves that have no CONFIG_DB mapping: bgp/neighbors/neighbor[20.0.0.1]/timers/config/minimum-advertisement-interval",
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

const niBgpNbrUrl = niBgpProtoUrl + "/bgp/neighbors/neighbor[neighbor-address=20.0.0.1]"

// The neighbor afi-safis hierarchy above the config containers: the container,
// the list instance and the key leaf the instance repeats. Until these levels
// were served, an address family activated through OpenConfig could not be
// removed through it.
func Test_openconfig_network_instance_bgp_neighbor_afi_safis_hierarchy(t *testing.T) {
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

	afisUrl := niBgpNbrUrl + "/afi-safis"
	v4Url := afisUrl + "/afi-safi[afi-safi-name=IPV4_UNICAST]"
	v6Url := afisUrl + "/afi-safi[afi-safi-name=IPV6_UNICAST]"

	// One request at the container writes a row per family it names.
	urlInputBodyJSON := `{"afi-safi":[` +
		`{"afi-safi-name":"IPV4_UNICAST","config":{"afi-safi-name":"IPV4_UNICAST","enabled":true}},` +
		`{"afi-safi-name":"IPV6_UNICAST","config":{"afi-safi-name":"IPV6_UNICAST","enabled":false}}]}`
	t.Run("PATCH network-instance BGP neighbor afi-safis container",
		processSetRequest(afisUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 row from afi-safis container payload",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"admin_status": "down",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 row from afi-safis container payload",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP neighbor afi-safi key leaf",
		processGetRequest(v4Url+"/afi-safi-name", nil,
			`{"openconfig-network-instance:afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST"}`, false))
	time.Sleep(1 * time.Second)

	v4EntryJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST",` +
		`"config":{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":true}}`
	v6EntryJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV6_UNICAST",` +
		`"config":{"afi-safi-name":"openconfig-bgp-types:IPV6_UNICAST","enabled":false}}`
	t.Run("GET network-instance BGP neighbor afi-safi list instance",
		processGetRequest(v4Url, nil,
			`{"openconfig-network-instance:afi-safi":[`+v4EntryJSON+`]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP neighbor afi-safi list instance ipv6",
		processGetRequest(v6Url, nil,
			`{"openconfig-network-instance:afi-safi":[`+v6EntryJSON+`]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP neighbor afi-safis container",
		processGetRequest(afisUrl, nil,
			`{"openconfig-network-instance:afi-safis":{"afi-safi":[`+v4EntryJSON+`,`+v6EntryJSON+`]}}`, false))
	time.Sleep(1 * time.Second)

	// A key is the row identity, so removing the instance is what removes it.
	t.Run("DELETE network-instance BGP neighbor afi-safi list instance",
		processDeleteRequest(v4Url, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_NEIGHBOR_AF ipv4 row removed by list instance DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"admin_status": "down",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv6 row kept by ipv4 list instance DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The instance level accepts a payload naming only its key, which asks for
	// the family to be activated with no other setting.
	urlInputBodyJSON = `{"openconfig-network-instance:afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST"}`
	t.Run("PATCH network-instance BGP neighbor afi-safi key leaf",
		processSetRequest(v4Url+"/afi-safi-name", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF ipv4 row activated by key leaf alone",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The container delete takes every family with it.
	t.Run("DELETE network-instance BGP neighbor afi-safis container",
		processDeleteRequest(afisUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_NEIGHBOR_AF ipv4 row removed by afi-safis container DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_NEIGHBOR_AF ipv6 row removed by afi-safis container DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	// The neighbor itself survives its address families being removed.
	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR kept after afi-safis container DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// Deleting the neighbor list instance takes its address-family rows with it, so
// that a neighbor created through OpenConfig can be removed through it rather
// than leaving an empty BGP_NEIGHBOR row behind.
func Test_openconfig_network_instance_bgp_neighbor_list_instance_delete(t *testing.T) {
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
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
			"default|20.0.0.1|ipv6_unicast": map[string]interface{}{
				"admin_status": "up",
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

	// The address is the row identity, so it cannot be cleared on its own.
	expectedErr := tlerr.NotSupportedError{
		Format: "DELETE is not supported for the neighbor-address key; delete the neighbor instead",
	}
	t.Run("DELETE network-instance BGP neighbor-address key leaf rejected",
		processDeleteRequest(niBgpNbrUrl+"/neighbor-address", true, expectedErr))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":          "65200",
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR kept after rejected key leaf DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedErr = tlerr.NotSupportedError{
		Format: "DELETE is not supported at the neighbors container level; delete a neighbor instead",
	}
	t.Run("DELETE network-instance BGP neighbors container rejected",
		processDeleteRequest(niBgpProtoUrl+"/bgp/neighbors", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor list instance",
		processDeleteRequest(niBgpNbrUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_NEIGHBOR row removed by list instance DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_NEIGHBOR_AF ipv4 row removed with its neighbor",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_NEIGHBOR_AF ipv6 row removed with its neighbor",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv6_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// Deleting the neighbor config container clears every leaf it maps, in both
// tables, while the list instance it hangs off survives.
func Test_openconfig_network_instance_bgp_neighbor_config_container_delete(t *testing.T) {
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
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{
				"asn":             "65200",
				"admin_status":    "up",
				"peer_group_name": "PG1",
			},
		},
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status":   "up",
				"send_community": "both",
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
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP neighbor config container",
		processDeleteRequest(niBgpNbrUrl+"/config", false))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_NEIGHBOR": map[string]interface{}{
			"default|20.0.0.1": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_NEIGHBOR fields cleared by config container DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR|default|20.0.0.1", expectedMap, false))
	time.Sleep(1 * time.Second)

	// send-community is the one leaf of the config container that lands in
	// BGP_NEIGHBOR_AF, so the delete reaches that row too without disturbing
	// the address family itself.
	expectedMap = map[string]interface{}{
		"BGP_NEIGHBOR_AF": map[string]interface{}{
			"default|20.0.0.1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_NEIGHBOR_AF send_community cleared by config container DELETE",
		verifyDbResult(rclient, "BGP_NEIGHBOR_AF|default|20.0.0.1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

const niBgpPgUrl = niBgpProtoUrl + "/bgp/peer-groups/peer-group[peer-group-name=PG1]"

// The peer-group afi-safis hierarchy above the config containers: the
// container, the list instance and the key leaf the instance repeats. Until
// these levels were served, an address family added to a peer-group through
// OpenConfig could not be removed through it, which in turn left the
// peer-group itself undeletable.
func Test_openconfig_network_instance_bgp_peer_group_afi_safis_hierarchy(t *testing.T) {
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
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": "",
			"default|PG1|ipv6_unicast": "",
		},
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	afisUrl := niBgpPgUrl + "/afi-safis"
	v4Url := afisUrl + "/afi-safi[afi-safi-name=IPV4_UNICAST]"
	v6Url := afisUrl + "/afi-safi[afi-safi-name=IPV6_UNICAST]"

	// One request at the container writes a row per family it names. Naming a
	// family is all it takes: the row carries admin_status so that frrcfgd
	// activates the peer-group for that address family.
	urlInputBodyJSON := `{"afi-safi":[{"afi-safi-name":"IPV4_UNICAST"},{"afi-safi-name":"IPV6_UNICAST"}]}`
	t.Run("PATCH network-instance BGP peer-group afi-safis container",
		processSetRequest(afisUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF ipv4 row from afi-safis container payload",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF ipv6 row from afi-safis container payload",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP peer-group afi-safi key leaf",
		processGetRequest(v4Url+"/afi-safi-name", nil,
			`{"openconfig-network-instance:afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST"}`, false))
	time.Sleep(1 * time.Second)

	v4EntryJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST",` +
		`"config":{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":true}}`
	v6EntryJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV6_UNICAST",` +
		`"config":{"afi-safi-name":"openconfig-bgp-types:IPV6_UNICAST","enabled":true}}`
	t.Run("GET network-instance BGP peer-group afi-safi list instance",
		processGetRequest(v4Url, nil,
			`{"openconfig-network-instance:afi-safi":[`+v4EntryJSON+`]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP peer-group afi-safi list instance ipv6",
		processGetRequest(v6Url, nil,
			`{"openconfig-network-instance:afi-safi":[`+v6EntryJSON+`]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP peer-group afi-safis container",
		processGetRequest(afisUrl, nil,
			`{"openconfig-network-instance:afi-safis":{"afi-safi":[`+v4EntryJSON+`,`+v6EntryJSON+`]}}`, false))
	time.Sleep(1 * time.Second)

	// A key is the row identity, so removing the instance is what removes it.
	t.Run("DELETE network-instance BGP peer-group afi-safi list instance",
		processDeleteRequest(v4Url, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_PEER_GROUP_AF ipv4 row removed by list instance DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF ipv6 row kept by ipv4 list instance DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The instance level accepts a payload naming only its key, which asks for
	// the family to be activated with no other setting.
	urlInputBodyJSON = `{"openconfig-network-instance:afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST"}`
	t.Run("PATCH network-instance BGP peer-group afi-safi key leaf",
		processSetRequest(v4Url+"/afi-safi-name", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF ipv4 row activated by key leaf alone",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A key naming one family while the path names another would land on the
	// row the path names, so it is refused instead.
	expectedErr := tlerr.InvalidArgsError{
		Format: "BGP peer-group afi-safi-name payload (ipv6_unicast) does not match key (ipv4_unicast)",
	}
	urlInputBodyJSON = `{"openconfig-network-instance:afi-safi-name":"openconfig-bgp-types:IPV6_UNICAST"}`
	t.Run("PATCH network-instance BGP peer-group afi-safi key leaf mismatch rejected",
		processSetRequest(v4Url+"/afi-safi-name", urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// The container delete takes every family with it.
	t.Run("DELETE network-instance BGP peer-group afi-safis container",
		processDeleteRequest(afisUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_PEER_GROUP_AF ipv4 row removed by afi-safis container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_PEER_GROUP_AF ipv6 row removed by afi-safis container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	// The peer-group itself survives its address families being removed.
	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP kept after afi-safis container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// A read at or above an afi-safi instance reports every field of the row,
// including the two that hang off containers of their own: max-prefixes below
// the family specific prefix-limit, and the apply-policy directions.
func Test_openconfig_network_instance_bgp_peer_group_afi_safi_instance_read(t *testing.T) {
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
				"admin_status":     "down",
				"max_prefix_limit": "500",
				"route_map_in@":    "RM_IN",
				"route_map_out@":   "RM_OUT",
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

	afisUrl := niBgpPgUrl + "/afi-safis"
	v4Url := afisUrl + "/afi-safi[afi-safi-name=IPV4_UNICAST]"

	v4EntryJSON := `{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST",` +
		`"apply-policy":{"config":{"export-policy":["RM_OUT"],"import-policy":["RM_IN"]}},` +
		`"config":{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":false},` +
		`"ipv4-unicast":{"prefix-limit":{"config":{"max-prefixes":500}}}}`
	t.Run("GET network-instance BGP peer-group afi-safi instance with every mapped field",
		processGetRequest(v4Url, nil,
			`{"openconfig-network-instance:afi-safi":[`+v4EntryJSON+`]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP peer-group afi-safis container with every mapped field",
		processGetRequest(afisUrl, nil,
			`{"openconfig-network-instance:afi-safis":{"afi-safi":[`+v4EntryJSON+`]}}`, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// Deleting the peer-group list instance takes its address-family rows with it,
// so that a peer-group created through OpenConfig can be removed through it
// however far its afi-safi subtree was filled in.
func Test_openconfig_network_instance_bgp_peer_group_list_instance_delete(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn":       "65200",
				"keepalive": "30",
			},
		},
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
			"default|PG1|ipv6_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": "",
			"default|PG1|ipv6_unicast": "",
		},
		"BGP_PEER_GROUP": map[string]interface{}{"default|PG1": ""},
		"BGP_GLOBALS":    map[string]interface{}{"default": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// The name below the list instance is the row identity, so it cannot be
	// cleared on its own.
	expectedErr := tlerr.NotSupportedError{
		Format: "DELETE is not supported for the peer-group-name key; delete the peer-group instead",
	}
	t.Run("DELETE network-instance BGP peer-group-name key leaf rejected",
		processDeleteRequest(niBgpPgUrl+"/peer-group-name", true, expectedErr))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn":       "65200",
				"keepalive": "30",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP kept after rejected key leaf DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedErr = tlerr.NotSupportedError{
		Format: "DELETE is not supported at the peer-groups container level; delete a peer-group instead",
	}
	t.Run("DELETE network-instance BGP peer-groups container rejected",
		processDeleteRequest(niBgpProtoUrl+"/bgp/peer-groups", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group list instance",
		processDeleteRequest(niBgpPgUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_PEER_GROUP row removed by list instance DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_PEER_GROUP_AF ipv4 row removed with its peer-group",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_PEER_GROUP_AF ipv6 row removed with its peer-group",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv6_unicast", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// Each config container below a peer-group clears the leaves it maps, and only
// those, while the row it hangs off survives. All four refused DELETE before
// the hierarchy was served.
func Test_openconfig_network_instance_bgp_peer_group_config_container_delete(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn":       "65200",
				"keepalive": "30",
				"holdtime":  "90",
			},
		},
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status":     "up",
				"max_prefix_limit": "500",
				"route_map_in@":    "RM_IN",
				"route_map_out@":   "RM_OUT",
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

	v4Url := niBgpPgUrl + "/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]"

	// The timers container holds both of its leaves in the peer-group row, so
	// the delete clears the two of them and leaves the rest of the row alone.
	t.Run("DELETE network-instance BGP peer-group timers config container",
		processDeleteRequest(niBgpPgUrl+"/timers/config", false))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP timers cleared by timers config container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	// peer-as is the one leaf of the peer-group config container that maps to a
	// field, since peer-group-name is the row key rather than data.
	t.Run("DELETE network-instance BGP peer-group config container",
		processDeleteRequest(niBgpPgUrl+"/config", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_PEER_GROUP asn cleared by config container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group ipv4 prefix-limit config container",
		processDeleteRequest(v4Url+"/ipv4-unicast/prefix-limit/config", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status":   "up",
				"route_map_in@":  "RM_IN",
				"route_map_out@": "RM_OUT",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF max_prefix_limit cleared by prefix-limit config container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The apply-policy container holds both directions, so its delete takes
	// the two route-maps and leaves the address family activated.
	t.Run("DELETE network-instance BGP peer-group ipv4 apply-policy config container",
		processDeleteRequest(v4Url+"/apply-policy/config", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF route-maps cleared by apply-policy config container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// The peer-group afi-safi config container and its enabled leaf, the paths a
// neighbor already served. Without them a peer-group address family could be
// named but never activated or deactivated on its own.
func Test_openconfig_network_instance_bgp_peer_group_afi_safi_config_enabled(t *testing.T) {
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
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": "",
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = niBgpPgUrl + "/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/config"
	urlInputBodyJSON = "{\"afi-safi-name\":\"IPV4_UNICAST\",\"enabled\":true}"
	t.Run("PATCH network-instance BGP peer-group AFI-SAFI config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// admin_status on the AF row is what makes frrcfgd emit
	// "neighbor <peer-group> activate" under the address-family.
	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF ipv4 admin_status mapping", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP peer-group AFI-SAFI config enabled",
		processGetRequest(url+"/enabled", nil, "{\"openconfig-network-instance:enabled\":true}", false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"openconfig-network-instance:enabled\":false}"
	t.Run("PATCH network-instance BGP peer-group AFI-SAFI config enabled false",
		processSetRequest(url+"/enabled", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status": "down",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF ipv4 admin_status down", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance BGP peer-group AFI-SAFI config enabled false",
		processGetRequest(url+"/enabled", nil, "{\"openconfig-network-instance:enabled\":false}", false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group AFI-SAFI config enabled", processDeleteRequest(url+"/enabled", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF ipv4 admin_status cleared", verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	// Re-activate through the config container, then take the container itself.
	urlInputBodyJSON = "{\"afi-safi-name\":\"IPV4_UNICAST\",\"enabled\":true}"
	t.Run("PATCH network-instance BGP peer-group AFI-SAFI config again", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance BGP peer-group AFI-SAFI config container", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify BGP_PEER_GROUP_AF ipv4 admin_status cleared by config container DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The peer-group itself is not touched by the address-family deletes.
	expectedMap = map[string]interface{}{
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": map[string]interface{}{
				"asn": "65200",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP survives AFI-SAFI config DELETE",
		verifyDbResult(rclient, "BGP_PEER_GROUP|default|PG1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_bgp_container_peer_group_afi_safi_enabled(t *testing.T) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{
				"local_asn": "65100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": "",
		},
		"BGP_PEER_GROUP": map[string]interface{}{
			"default|PG1": "",
		},
		"BGP_GLOBALS": map[string]interface{}{
			"default": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// A peer-group payload that names an AFI-SAFI activates it there, and the
	// import-policy under the same AFI-SAFI lands on the same row.
	urlInputBodyJSON := `{` +
		`"peer-groups":{"peer-group":[{"peer-group-name":"PG1",` +
		`"config":{"peer-as":65200},` +
		`"afi-safis":{"afi-safi":[{"afi-safi-name":"IPV4_UNICAST",` +
		`"config":{"afi-safi-name":"IPV4_UNICAST","enabled":true},` +
		`"apply-policy":{"config":{"import-policy":["RM-IN-V4"]}}}]}}]}}`
	t.Run("PATCH BGP container with peer-group AFI-SAFI enabled",
		processSetRequest(niBgpProtoUrl+"/bgp", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"BGP_PEER_GROUP_AF": map[string]interface{}{
			"default|PG1|ipv4_unicast": map[string]interface{}{
				"admin_status":  "up",
				"route_map_in@": "RM-IN-V4",
			},
		},
	}
	t.Run("Verify BGP_PEER_GROUP_AF row from container payload",
		verifyDbResult(rclient, "BGP_PEER_GROUP_AF|default|PG1|ipv4_unicast", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
