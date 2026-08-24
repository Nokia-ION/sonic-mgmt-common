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

func Test_openconfig_routing_policy_policy_definition_statement_name(t *testing.T) {
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

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/name"
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"10\"}"
	t.Run("PATCH routing-policy statement name", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP statement row", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-routing-policy:name\":\"10\"}"
	t.Run("GET routing-policy statement name", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "statement name payload (20) does not match key (10)",
	}
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"20\"}"
	t.Run("PATCH statement name mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement name", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP statement row deleted", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_defined_sets_bgp_defined_sets_community_set_name(t *testing.T) {
	var url, prefixedUrl, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/defined-sets/bgp-defined-sets/community-sets/community-set[community-set-name=COMM1]/community-set-name"
	urlInputBodyJSON = "{\"openconfig-bgp-policy:community-set-name\":\"COMM1\"}"
	t.Run("PATCH routing-policy community-set-name", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	configNameURL := "/openconfig-routing-policy:routing-policy/defined-sets/bgp-defined-sets/community-sets/community-set[community-set-name=COMM1]/config/community-set-name"
	t.Run("PATCH routing-policy community-set config/community-set-name", processSetRequest(configNameURL, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type":     "STANDARD",
				"match_action": "ANY",
			},
		},
	}
	t.Run("Verify COMMUNITY_SET row", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-bgp-policy:community-set-name\":\"COMM1\"}"
	t.Run("GET routing-policy community-set-name", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy community-set config/community-set-name", processGetRequest(configNameURL, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	prefixedUrl = "/openconfig-routing-policy:routing-policy/defined-sets/openconfig-bgp-policy:bgp-defined-sets/openconfig-bgp-policy:community-sets/openconfig-bgp-policy:community-set[openconfig-bgp-policy:community-set-name=COMM1]/openconfig-bgp-policy:community-set-name"
	t.Run("PATCH routing-policy community-set-name with module-prefixed augment segments", processSetRequest(prefixedUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy community-set-name with module-prefixed augment segments", processGetRequest(prefixedUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "community-set-name payload (COMM2) does not match key (COMM1)",
	}
	urlInputBodyJSON = "{\"openconfig-bgp-policy:community-set-name\":\"COMM2\"}"
	t.Run("PATCH community-set-name mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy community-set-name", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{},
		},
	}
	t.Run("Verify COMMUNITY_SET row deleted", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
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
	urlInputBodyJSON = "{\"prefix-set\":\"PS4\"}"
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
	urlInputBodyJSON = "{\"config\":{\"method\":\"REFERENCE\"},\"reference\":{\"config\":{\"community-set-ref\":\"COMMSET1\"}}}"
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
	urlInputBodyJSON = "{\"communities\":[\"65001:100\",\"NO_EXPORT\"]}"
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
	urlInputBodyJSON = "{\"asn\":65000,\"repeat-n\":3}"
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

func Test_openconfig_routing_policy_policy_definition_statement_actions_bgp_actions_config(t *testing.T) {
	var url, prefixedUrl, urlInputBodyJSON string

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

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/bgp-actions/config"
	urlInputBodyJSON = "{\"set-next-hop\":\"10.10.10.1\",\"set-route-origin\":\"IGP\",\"set-local-pref\":200,\"set-med\":100}"
	t.Run("PATCH routing-policy statement actions bgp-actions config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"set_next_hop":   "10.10.10.1",
				"set_origin":     "igp",
				"set_local_pref": "200",
				"set_med":        "100",
			},
		},
	}
	t.Run("Verify ROUTE_MAP bgp-actions config fields mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy statement bgp-actions set-next-hop", processGetRequest(url+"/set-next-hop", nil, "{\"openconfig-bgp-policy:set-next-hop\":\"10.10.10.1\"}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-actions set-route-origin", processGetRequest(url+"/set-route-origin", nil, "{\"openconfig-bgp-policy:set-route-origin\":\"IGP\"}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-actions set-local-pref", processGetRequest(url+"/set-local-pref", nil, "{\"openconfig-bgp-policy:set-local-pref\":200}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-actions set-med", processGetRequest(url+"/set-med", nil, "{\"openconfig-bgp-policy:set-med\":100}", false))
	time.Sleep(1 * time.Second)

	prefixedUrl = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/openconfig-bgp-policy:bgp-actions/openconfig-bgp-policy:config"
	t.Run("PATCH routing-policy statement bgp-actions config with module-prefixed augment segments", processSetRequest(prefixedUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-actions set-local-pref with module-prefixed augment segments", processGetRequest(prefixedUrl+"/openconfig-bgp-policy:set-local-pref", nil, "{\"openconfig-bgp-policy:set-local-pref\":200}", false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement actions bgp-actions config", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP bgp-actions config fields cleared", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_conditions_bgp_conditions_config(t *testing.T) {
	var url, prefixedUrl, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{},
		},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EXTCOMM1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": "",
		},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EXTCOMM1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions/bgp-conditions/config"
	urlInputBodyJSON = "{\"med-eq\":100,\"origin-eq\":\"EGP\",\"local-pref-eq\":300,\"community-set\":\"COMM1\",\"ext-community-set\":\"EXTCOMM1\"}"
	t.Run("PATCH routing-policy statement conditions bgp-conditions config", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_med":           "100",
				"match_origin":        "egp",
				"match_local_pref":    "300",
				"match_community":     "COMM1",
				"match_ext_community": "EXTCOMM1",
			},
		},
	}
	t.Run("Verify ROUTE_MAP bgp-conditions config fields mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy statement bgp-conditions med-eq", processGetRequest(url+"/med-eq", nil, "{\"openconfig-bgp-policy:med-eq\":100}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-conditions origin-eq", processGetRequest(url+"/origin-eq", nil, "{\"openconfig-bgp-policy:origin-eq\":\"EGP\"}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-conditions local-pref-eq", processGetRequest(url+"/local-pref-eq", nil, "{\"openconfig-bgp-policy:local-pref-eq\":300}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-conditions community-set", processGetRequest(url+"/community-set", nil, "{\"openconfig-bgp-policy:community-set\":\"COMM1\"}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-conditions ext-community-set", processGetRequest(url+"/ext-community-set", nil, "{\"openconfig-bgp-policy:ext-community-set\":\"EXTCOMM1\"}", false))
	time.Sleep(1 * time.Second)

	prefixedUrl = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions/openconfig-bgp-policy:bgp-conditions/openconfig-bgp-policy:config"
	t.Run("PATCH routing-policy statement bgp-conditions config with module-prefixed augment segments", processSetRequest(prefixedUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement bgp-conditions med-eq with module-prefixed augment segments", processGetRequest(prefixedUrl+"/openconfig-bgp-policy:med-eq", nil, "{\"openconfig-bgp-policy:med-eq\":100}", false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement conditions bgp-conditions config", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP bgp-conditions config fields cleared", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

const rpolContainerUrl = "/openconfig-routing-policy:routing-policy"

func Test_openconfig_routing_policy_container_hierarchical_set(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": "",
			"COMM2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	urlInputBodyJSON := `{` +
		`"policy-definitions":{"policy-definition":[{"name":"POL1","config":{"name":"POL1"},` +
		`"statements":{"statement":[{"name":"10","config":{"name":"10"},` +
		`"actions":{"config":{"policy-result":"ACCEPT_ROUTE"},` +
		`"openconfig-bgp-policy:bgp-actions":{"config":{"set-next-hop":"10.10.10.1","set-local-pref":200},` +
		`"set-as-path-prepend":{"config":{"asn":65001,"repeat-n":3}}}},` +
		`"conditions":{"openconfig-bgp-policy:bgp-conditions":{"config":{"med-eq":100,"community-set":"COMM1"}}}` +
		`}]}}]},` +
		`"defined-sets":{"openconfig-bgp-policy:bgp-defined-sets":{"community-sets":{"community-set":[` +
		`{"community-set-name":"COMM2","config":{"community-set-name":"COMM2"}}]}}}}`
	t.Run("PATCH routing-policy container with nested policy, statement and community-set payload",
		processSetRequest(rpolContainerUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP_SET row from container payload", verifyDbResult(rclient, "ROUTE_MAP_SET|POL1", expectedMap, false))
	time.Sleep(1 * time.Second)

	// Every mapped node of the statement contributes to the same ROUTE_MAP row,
	// so all of the fields must survive the merge.
	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "permit",
				"set_next_hop":    "10.10.10.1",
				"set_local_pref":  "200",
				"set_asn":         "65001",
				"set_repeat_asn":  "3",
				"match_med":       "100",
				"match_community": "COMM1",
			},
		},
	}
	t.Run("Verify ROUTE_MAP statement row from container payload", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A referenced community set is created by the condition mapping, and a
	// named community-set instance is created by the defined-sets payload.
	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type":     "STANDARD",
				"match_action": "ANY",
			},
		},
	}
	t.Run("Verify referenced COMMUNITY_SET row", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM2": map[string]interface{}{
				"set_type":     "STANDARD",
				"match_action": "ANY",
			},
		},
	}
	t.Run("Verify defined-sets COMMUNITY_SET row", verifyDbResult(rclient, "COMMUNITY_SET|COMM2", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The same payload shape must apply at an intermediate container too.
	urlInputBodyJSON = `{"policy-definition":[{"name":"POL1",` +
		`"statements":{"statement":[{"name":"10","actions":{"config":{"policy-result":"REJECT_ROUTE"}}}]}}]}`
	t.Run("PATCH routing-policy policy-definitions container",
		processSetRequest(rpolContainerUrl+"/policy-definitions", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "deny",
				"set_next_hop":    "10.10.10.1",
				"set_local_pref":  "200",
				"set_asn":         "65001",
				"set_repeat_asn":  "3",
				"match_med":       "100",
				"match_community": "COMM1",
			},
		},
	}
	t.Run("Verify ROUTE_MAP update from policy-definitions container payload",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_container_key_only_payload(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL2|20": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// Naming a list instance without any other data is a request for that
	// instance to exist.
	urlInputBodyJSON := `{"policy-definitions":{"policy-definition":[` +
		`{"name":"POL2","statements":{"statement":[{"name":"20"}]}}]}}`
	t.Run("PATCH routing-policy container with key only payload",
		processSetRequest(rpolContainerUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL2": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP_SET row created from key only payload", verifyDbResult(rclient, "ROUTE_MAP_SET|POL2", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL2|20": map[string]interface{}{},
		},
	}
	t.Run("Verify ROUTE_MAP row created from key only payload", verifyDbResult(rclient, "ROUTE_MAP|POL2|20", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_container_get(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "permit",
				"set_local_pref":  "200",
			},
		},
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type": "STANDARD",
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
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	statementEntryJSON := `{"actions":{"openconfig-bgp-policy:bgp-actions":{"config":{"set-local-pref":200}},` +
		`"config":{"policy-result":"ACCEPT_ROUTE"}},"name":"10"}`
	policyEntryJSON := `{"config":{"name":"POL1"},"name":"POL1","statements":{"statement":[` + statementEntryJSON + `]}}`
	definedSetsJSON := `{"openconfig-bgp-policy:bgp-defined-sets":{"community-sets":{"community-set":[` +
		`{"community-set-name":"COMM1","config":{"community-set-name":"COMM1"}}]}}}`

	expectedGetJSON := `{"openconfig-routing-policy:routing-policy":{"defined-sets":` + definedSetsJSON +
		`,"policy-definitions":{"policy-definition":[` + policyEntryJSON + `]}}}`
	t.Run("GET routing-policy container", processGetRequest(rpolContainerUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:policy-definitions":{"policy-definition":[` + policyEntryJSON + `]}}`
	t.Run("GET routing-policy policy-definitions container",
		processGetRequest(rpolContainerUrl+"/policy-definitions", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The whole list must come from ROUTE_MAP_SET only. A generically expanded
	// list reports one synthetic entry keyed by the inherited parent key.
	expectedGetJSON = `{"openconfig-routing-policy:policy-definition":[` + policyEntryJSON + `]}`
	t.Run("GET routing-policy policy-definition list",
		processGetRequest(rpolContainerUrl+"/policy-definitions/policy-definition", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy policy-definition list instance",
		processGetRequest(rpolContainerUrl+"/policy-definitions/policy-definition[name=POL1]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:statement":[` + statementEntryJSON + `]}`
	t.Run("GET routing-policy statement list",
		processGetRequest(rpolContainerUrl+"/policy-definitions/policy-definition[name=POL1]/statements/statement", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:defined-sets":` + definedSetsJSON + `}`
	t.Run("GET routing-policy defined-sets container",
		processGetRequest(rpolContainerUrl+"/defined-sets", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_container_unmapped_payload_rejected(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	urlInputBodyJSON := `{"policy-definitions":{"policy-definition":[` +
		`{"name":"POL1","statements":{"statement":[{"name":"10",` +
		`"actions":{"config":{"policy-result":"ACCEPT_ROUTE"}},` +
		`"conditions":{"match-prefix-set":{"config":{"prefix-set":"PFX1","match-set-options":"ANY"}}}}]}}]}}`
	expectedErr := tlerr.NotSupportedError{
		Format: "routing-policy container payload carries leaves that have no CONFIG_DB mapping: " +
			"routing-policy/policy-definitions/policy-definition[POL1]/statements/statement[10]/conditions/match-prefix-set/config/match-set-options",
	}
	t.Run("PATCH routing-policy container rejects unmapped leaf",
		processSetRequest(rpolContainerUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// The whole request is rejected, so the mapped leaves must not be applied.
	t.Run("Verify ROUTE_MAP not created after rejected container payload",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_container_delete_not_supported(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "permit",
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
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.NotSupportedError{
		Format: "DELETE is not supported at routing-policy container level; delete mapped child paths or list instances instead",
	}
	t.Run("DELETE routing-policy container rejected", processDeleteRequest(rpolContainerUrl, true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy policy-definitions container rejected",
		processDeleteRequest(rpolContainerUrl+"/policy-definitions", true))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "permit",
			},
		},
	}
	t.Run("Verify ROUTE_MAP retained after rejected container delete",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_defined_sets_bgp_defined_sets_community_set_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/defined-sets/bgp-defined-sets/community-sets/community-set[community-set-name=COMM1]/config"
	urlInputBodyJSON = `{"community-set-name":"COMM1",` +
		`"community-member":["65100:100","65100:200"],"match-set-options":"ALL"}`
	t.Run("PATCH routing-policy community-set config container", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// frrcfgd renders a community list only once the row holds the list type,
	// the match modifier and at least one member.
	expectedMap := map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type":          "STANDARD",
				"match_action":      "ALL",
				"community_member@": "65100:100,65100:200",
			},
		},
	}
	t.Run("Verify COMMUNITY_SET row from config container", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-bgp-policy:community-member":["65100:100","65100:200"]}`
	t.Run("GET routing-policy community-set config community-member", processGetRequest(url+"/community-member", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy community-set config match-set-options",
		processGetRequest(url+"/match-set-options", nil, `{"openconfig-bgp-policy:match-set-options":"ALL"}`, false))
	time.Sleep(1 * time.Second)

	// A member that no standard list can hold moves the set to an expanded one.
	urlInputBodyJSON = `{"openconfig-bgp-policy:community-member":["6510[0-9]:100"]}`
	t.Run("PATCH routing-policy community-set config community-member with a regexp member",
		processSetRequest(url+"/community-member", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type":          "EXPANDED",
				"match_action":      "ALL",
				"community_member@": "6510[0-9]:100",
			},
		},
	}
	t.Run("Verify COMMUNITY_SET row holds an expanded list", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A well-known member is stored with the keyword FRR parses.
	urlInputBodyJSON = `{"openconfig-bgp-policy:community-member":["NO_EXPORT"]}`
	t.Run("PATCH routing-policy community-set config community-member with a well-known community",
		processSetRequest(url+"/community-member", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type":          "STANDARD",
				"match_action":      "ALL",
				"community_member@": "no-export",
			},
		},
	}
	t.Run("Verify COMMUNITY_SET row holds the FRR keyword", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy community-set config community-member", processDeleteRequest(url+"/community-member", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type":     "STANDARD",
				"match_action": "ALL",
			},
		},
	}
	t.Run("Verify COMMUNITY_SET community_member cleared", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy community-set config match-set-options", processDeleteRequest(url+"/match-set-options", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type": "STANDARD",
			},
		},
	}
	t.Run("Verify COMMUNITY_SET match_action cleared", verifyDbResult(rclient, "COMMUNITY_SET|COMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_defined_sets_bgp_defined_sets_ext_community_set_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EXTCOMM1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/defined-sets/bgp-defined-sets/ext-community-sets/ext-community-set[ext-community-set-name=EXTCOMM1]/config"
	urlInputBodyJSON = `{"ext-community-set-name":"EXTCOMM1",` +
		`"ext-community-member":["route-target:65100:100"],"match-set-options":"ANY"}`
	t.Run("PATCH routing-policy ext-community-set config container", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// frrcfgd only renders a marked member on a standard extcommunity list.
	expectedMap := map[string]interface{}{
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EXTCOMM1": map[string]interface{}{
				"set_type":          "STANDARD",
				"match_action":      "ANY",
				"community_member@": "route-target:65100:100",
			},
		},
	}
	t.Run("Verify EXTENDED_COMMUNITY_SET row from config container",
		verifyDbResult(rclient, "EXTENDED_COMMUNITY_SET|EXTCOMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-bgp-policy:ext-community-member":["route-target:65100:100"]}`
	t.Run("GET routing-policy ext-community-set config ext-community-member",
		processGetRequest(url+"/ext-community-member", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy ext-community-set config match-set-options",
		processGetRequest(url+"/match-set-options", nil, `{"openconfig-bgp-policy:match-set-options":"ANY"}`, false))
	time.Sleep(1 * time.Second)

	// An unmarked member has no standard spelling in FRR, so the set becomes
	// an expanded one.
	urlInputBodyJSON = `{"openconfig-bgp-policy:ext-community-member":["65100:2[0-9]0"]}`
	t.Run("PATCH routing-policy ext-community-set config ext-community-member with an unmarked member",
		processSetRequest(url+"/ext-community-member", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EXTCOMM1": map[string]interface{}{
				"set_type":          "EXPANDED",
				"match_action":      "ANY",
				"community_member@": "65100:2[0-9]0",
			},
		},
	}
	t.Run("Verify EXTENDED_COMMUNITY_SET row holds an expanded list",
		verifyDbResult(rclient, "EXTENDED_COMMUNITY_SET|EXTCOMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy ext-community-set config ext-community-member",
		processDeleteRequest(url+"/ext-community-member", false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EXTCOMM1": map[string]interface{}{
				"set_type":     "EXPANDED",
				"match_action": "ANY",
			},
		},
	}
	t.Run("Verify EXTENDED_COMMUNITY_SET community_member cleared",
		verifyDbResult(rclient, "EXTENDED_COMMUNITY_SET|EXTCOMM1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
