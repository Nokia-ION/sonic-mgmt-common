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

func Test_openconfig_routing_policy_policy_definition_config_name(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/config/name"
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"POL1\"}"
	t.Run("PATCH routing-policy policy-definition config name", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP_SET policy row", verifyDbResult(rclient, "ROUTE_MAP_SET|POL1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-routing-policy:name\":\"POL1\"}"
	t.Run("GET routing-policy policy-definition config name", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "policy-definition name payload (POL2) does not match key (POL1)",
	}
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"POL2\"}"
	t.Run("PATCH policy-definition name mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy policy-definition config name", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP_SET policy row deleted", verifyDbResult(rclient, "ROUTE_MAP_SET|POL1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_actions_config_policy_result(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/config/policy-result"
	urlInputBodyJSON = "{\"openconfig-routing-policy:policy-result\":\"ACCEPT_ROUTE\"}"
	t.Run("PATCH routing-policy statement actions config policy-result", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "permit",
			},
		},
	}
	t.Run("Verify ROUTE_MAP route_operation mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-routing-policy:policy-result\":\"ACCEPT_ROUTE\"}"
	t.Run("GET routing-policy statement actions config policy-result", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement actions config policy-result", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP route_operation cleared", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_conditions_match_prefix_set_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
		"PREFIX_SET": map[string]interface{}{
			"PS4": map[string]interface{}{
				"mode": "IPv4",
			},
			"PS6": map[string]interface{}{
				"mode": "IPv6",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
		"PREFIX_SET": map[string]interface{}{
			"PS4": "",
			"PS6": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions/match-prefix-set/config"
	urlInputBodyJSON = "{\"openconfig-routing-policy:config\":{\"prefix-set\":\"PS4\"}}"
	t.Run("PATCH routing-policy statement conditions match-prefix-set config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_prefix_set": "PS4",
			},
		},
	}
	t.Run("Verify ROUTE_MAP match_prefix_set mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-routing-policy:config\":{\"prefix-set\":\"PS4\"}}"
	t.Run("GET routing-policy statement conditions match-prefix-set config", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions/match-prefix-set/config/prefix-set"
	urlInputBodyJSON = "{\"openconfig-routing-policy:prefix-set\":\"PS6\"}"
	t.Run("PATCH routing-policy statement conditions match-prefix-set leaf", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_ipv6_prefix_set": "PS6",
			},
		},
	}
	t.Run("Verify ROUTE_MAP match_ipv6_prefix_set mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions/match-prefix-set/config"
	t.Run("DELETE routing-policy statement conditions match-prefix-set config", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP match-prefix-set fields cleared", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_actions_bgp_actions_set_community(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/bgp-actions/set-community"
	urlInputBodyJSON = "{\"openconfig-bgp-policy:set-community\":{\"config\":{\"method\":\"REFERENCE\"},\"reference\":{\"config\":{\"community-set-ref\":\"COMMSET1\"}}}}"
	t.Run("PATCH routing-policy statement actions bgp set-community", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"set_community_ref": "COMMSET1",
			},
		},
	}
	t.Run("Verify ROUTE_MAP set_community_ref mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-bgp-policy:set-community\":{\"config\":{\"method\":\"REFERENCE\"},\"reference\":{\"config\":{\"community-set-ref\":\"COMMSET1\"}}}}"
	t.Run("GET routing-policy statement actions bgp set-community", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement actions bgp set-community", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP set-community fields cleared", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_actions_bgp_actions_set_community_inline_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/bgp-actions/set-community/inline/config"
	urlInputBodyJSON = "{\"openconfig-bgp-policy:config\":{\"communities\":[\"65001:100\",\"NO_EXPORT\"]}}"
	t.Run("PATCH routing-policy statement actions bgp set-community inline config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"set_community_inline": "65001:100, NO_EXPORT",
			},
		},
	}
	t.Run("Verify ROUTE_MAP set_community_inline mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-bgp-policy:config\":{\"communities\":[\"65001:100\",\"NO_EXPORT\"]}}"
	t.Run("GET routing-policy statement actions bgp set-community inline config", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement actions bgp set-community inline config", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP set-community inline field cleared", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_actions_bgp_actions_set_as_path_prepend_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
			"POL1|20": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/bgp-actions/set-as-path-prepend/config"
	urlInputBodyJSON = "{\"openconfig-bgp-policy:config\":{\"asn\":65000,\"repeat-n\":3}}"
	t.Run("PATCH routing-policy statement actions bgp set-as-path-prepend config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"set_asn":        "65000",
				"set_repeat_asn": "3",
			},
		},
	}
	t.Run("Verify ROUTE_MAP set_asn and set_repeat_asn mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-bgp-policy:config\":{\"asn\":65000,\"repeat-n\":3}}"
	t.Run("GET routing-policy statement actions bgp set-as-path-prepend config", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=20]/actions/bgp-actions/set-as-path-prepend/config/repeat-n"
	urlInputBodyJSON = "{\"openconfig-bgp-policy:repeat-n\":2}"
	expectedErr := tlerr.InvalidArgsError{
		Format: "routing-policy statement actions bgp-actions set-as-path-prepend config repeat-n requires asn to be configured",
	}
	t.Run("PATCH repeat-n without asn rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/bgp-actions/set-as-path-prepend/config"
	t.Run("DELETE routing-policy statement actions bgp set-as-path-prepend config", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP set-as-path-prepend fields cleared", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
