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
	"errors"
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
	var urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{"NULL": "NULL"},
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

	stmtUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]"
	keyUrl := stmtUrl + "/name"
	cfgUrl := stmtUrl + "/config"
	cfgNameUrl := cfgUrl + "/name"

	// A statement carrying no other configuration still has to leave a row
	// behind, and SONiC keeps such a row alive with the NULL placeholder.
	presentRow := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{"NULL": "NULL"},
		},
	}
	absentRow := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{},
		},
	}

	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"10\"}"
	t.Run("PATCH routing-policy statement config name", processSetRequest(cfgNameUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify ROUTE_MAP statement row", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", presentRow, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-routing-policy:name\":\"10\"}"
	t.Run("GET routing-policy statement config name", processGetRequest(cfgNameUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = "{\"openconfig-routing-policy:config\":{\"name\":\"10\"}}"
	t.Run("GET routing-policy statement config", processGetRequest(cfgUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The key leaf and the config leaf answer with the same value.
	expectedGetJSON = "{\"openconfig-routing-policy:name\":\"10\"}"
	t.Run("GET routing-policy statement name", processGetRequest(keyUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "statement name payload (20) does not match key (10)",
	}
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"20\"}"
	t.Run("PATCH statement config name mismatch rejected", processSetRequest(cfgNameUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify mismatch left the statement row alone", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", presentRow, false))
	time.Sleep(1 * time.Second)

	// The same write addressed at the config container rather than the leaf.
	urlInputBodyJSON = "{\"name\":\"10\"}"
	t.Run("PATCH routing-policy statement config", processSetRequest(cfgUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify ROUTE_MAP statement row from config container", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", presentRow, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement config name", processDeleteRequest(cfgNameUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify ROUTE_MAP statement row deleted", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", absentRow, false))
	time.Sleep(1 * time.Second)

	// Removing the statement through the key leaf takes the row with it, the
	// same as removing it through config/name.
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"10\"}"
	t.Run("PATCH statement config name again", processSetRequest(cfgNameUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement name", processDeleteRequest(keyUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify ROUTE_MAP statement row deleted through the key leaf", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", absentRow, false))
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

	// match-set-options is not stored, so the read side reports the ANY default
	// alongside the prefix-set it applies to.
	expectedGetJSON := "{\"openconfig-routing-policy:config\":{\"match-set-options\":\"ANY\",\"prefix-set\":\"PS4\"}}"
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

func Test_openconfig_routing_policy_policy_definition_statement_conditions_match_prefix_set_family_switch(t *testing.T) {
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
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":  "permit",
				"match_prefix_set": "PS4",
				"match_med":        "20",
				"set_local_pref":   "150",
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

	v4Row := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":  "permit",
				"match_prefix_set": "PS4",
				"match_med":        "20",
				"set_local_pref":   "150",
			},
		},
	}
	v6Row := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":       "permit",
				"match_ipv6_prefix_set": "PS6",
				"match_med":             "20",
				"set_local_pref":        "150",
			},
		},
	}

	stmtUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]"
	mpsUrl := stmtUrl + "/conditions/match-prefix-set"

	// The container levels above config read the same row as the leaves do.
	expectedGetJSON := "{\"openconfig-routing-policy:match-prefix-set\":{\"config\":{\"match-set-options\":\"ANY\",\"prefix-set\":\"PS4\"}}}"
	t.Run("GET match-prefix-set container", processGetRequest(mpsUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = "{\"openconfig-routing-policy:conditions\":{\"openconfig-bgp-policy:bgp-conditions\":{\"config\":{\"med-eq\":20}},\"match-prefix-set\":{\"config\":{\"match-set-options\":\"ANY\",\"prefix-set\":\"PS4\"}}}}"
	t.Run("GET statement conditions container", processGetRequest(stmtUrl+"/conditions", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = "{\"openconfig-routing-policy:actions\":{\"openconfig-bgp-policy:bgp-actions\":{\"config\":{\"set-local-pref\":150}},\"config\":{\"policy-result\":\"ACCEPT_ROUTE\"}}}"
	t.Run("GET statement actions container", processGetRequest(stmtUrl+"/actions", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// A statement matches a single prefix-set, so naming one of the other
	// family drops the field of the old one.
	url = mpsUrl + "/config/prefix-set"
	urlInputBodyJSON = "{\"openconfig-routing-policy:prefix-set\":\"PS6\"}"
	t.Run("PATCH prefix-set leaf switches IPv4 to IPv6", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify PATCH leaf left only match_ipv6_prefix_set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", v6Row, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"openconfig-routing-policy:prefix-set\":\"PS4\"}"
	t.Run("PUT prefix-set leaf switches IPv6 to IPv4", processSetRequest(url, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify PUT leaf left only match_prefix_set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", v4Row, false))
	time.Sleep(1 * time.Second)

	url = mpsUrl + "/config"
	urlInputBodyJSON = "{\"prefix-set\":\"PS6\"}"
	t.Run("PUT match-prefix-set config switches IPv4 to IPv6", processSetRequest(url, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify PUT config left only match_ipv6_prefix_set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", v6Row, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"prefix-set\":\"PS4\"}"
	t.Run("PATCH match-prefix-set config switches IPv6 to IPv4", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify PATCH config left only match_prefix_set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", v4Row, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"config\":{\"prefix-set\":\"PS6\"}}"
	t.Run("PUT match-prefix-set container switches IPv4 to IPv6", processSetRequest(mpsUrl, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify PUT match-prefix-set kept the rest of the statement", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", v6Row, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"conditions\":{\"match-prefix-set\":{\"config\":{\"prefix-set\":\"PS4\"}}}}"
	t.Run("PATCH statement switches IPv6 to IPv4", processSetRequest(stmtUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify PATCH statement left only match_prefix_set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", v4Row, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_conditions_match_prefix_set_family_switch_sole_field(t *testing.T) {
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
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_ipv6_prefix_set": "PS6",
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

	// With the old family as the only field, dropping it must not leave a
	// NULL placeholder beside the new one.
	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions/match-prefix-set/config"
	urlInputBodyJSON = "{\"prefix-set\":\"PS4\"}"
	t.Run("PATCH match-prefix-set config over a lone IPv6 prefix-set", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_prefix_set": "PS4",
			},
		},
	}
	t.Run("Verify PATCH replaced the lone field", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	url += "/prefix-set"
	urlInputBodyJSON = "{\"openconfig-routing-policy:prefix-set\":\"PS6\"}"
	t.Run("PUT prefix-set leaf over a lone IPv4 prefix-set", processSetRequest(url, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_ipv6_prefix_set": "PS6",
			},
		},
	}
	t.Run("Verify PUT replaced the lone field", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_conditions_replace(t *testing.T) {
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
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":  "permit",
				"match_prefix_set": "PS4",
				"match_med":        "20",
				"set_local_pref":   "150",
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

	condUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions"

	// A conditions REPLACE owns only the match fields; the actions sharing
	// the statement row stay.
	urlInputBodyJSON = "{\"match-prefix-set\":{\"config\":{\"prefix-set\":\"PS6\"}}}"
	t.Run("PUT statement conditions with match-prefix-set only", processSetRequest(condUrl, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":       "permit",
				"match_ipv6_prefix_set": "PS6",
				"set_local_pref":        "150",
			},
		},
	}
	t.Run("Verify conditions REPLACE dropped match_med and kept actions", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"openconfig-bgp-policy:bgp-conditions\":{\"config\":{\"med-eq\":30}}}"
	t.Run("PUT statement conditions with bgp-conditions only", processSetRequest(condUrl, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "permit",
				"match_med":       "30",
				"set_local_pref":  "150",
			},
		},
	}
	t.Run("Verify conditions REPLACE dropped the prefix-set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	url = condUrl + "/match-prefix-set/config/prefix-set"
	urlInputBodyJSON = "{\"openconfig-routing-policy:prefix-set\":\"PS4\"}"
	t.Run("PATCH prefix-set leaf next to bgp-conditions", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// bgp-conditions owns its own fields only, so the prefix-set survives.
	url = condUrl + "/bgp-conditions"
	urlInputBodyJSON = "{\"config\":{\"med-eq\":40}}"
	t.Run("PUT bgp-conditions container", processSetRequest(url, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":  "permit",
				"match_prefix_set": "PS4",
				"match_med":        "40",
				"set_local_pref":   "150",
			},
		},
	}
	t.Run("Verify bgp-conditions REPLACE kept the prefix-set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_actions_replace(t *testing.T) {
	var url, urlInputBodyJSON string

	stmtRow := map[string]interface{}{
		"route_operation":   "permit",
		"match_prefix_set":  "PS4",
		"match_med":         "20",
		"set_local_pref":    "150",
		"set_med":           "5",
		"set_community_ref": "CS1",
		"set_asn":           "65001",
		"set_repeat_asn":    "2",
	}
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
		"PREFIX_SET": map[string]interface{}{
			"PS4": map[string]interface{}{
				"mode": "IPv4",
			},
		},
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": map[string]interface{}{
				"set_type":          "STANDARD",
				"match_action":      "ANY",
				"community_member@": "65000:9",
			},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": stmtRow,
		},
	}
	stmtRowReset := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
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
		},
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	reloadStmtRow := func() {
		unloadDB(db.ConfigDB, stmtRowReset)
		loadDB(db.ConfigDB, map[string]interface{}{"ROUTE_MAP": map[string]interface{}{"POL1|10": stmtRow}})
		time.Sleep(1 * time.Second)
	}
	actionsUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions"
	bgpActionsUrl := actionsUrl + "/bgp-actions"

	// An actions REPLACE owns the policy result and every set action; the
	// conditions sharing the statement row stay.
	urlInputBodyJSON = "{\"config\":{\"policy-result\":\"REJECT_ROUTE\"}}"
	t.Run("PUT statement actions container", processSetRequest(actionsUrl, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":  "deny",
				"match_prefix_set": "PS4",
				"match_med":        "20",
			},
		},
	}
	t.Run("Verify actions REPLACE kept the conditions", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	reloadStmtRow()
	urlInputBodyJSON = "{\"config\":{\"set-local-pref\":200}}"
	t.Run("PUT bgp-actions container", processSetRequest(bgpActionsUrl, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":  "permit",
				"match_prefix_set": "PS4",
				"match_med":        "20",
				"set_local_pref":   "200",
			},
		},
	}
	t.Run("Verify bgp-actions REPLACE kept the policy result and conditions", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	reloadStmtRow()
	url = bgpActionsUrl + "/set-community"
	urlInputBodyJSON = "{\"config\":{\"method\":\"INLINE\"},\"inline\":{\"config\":{\"communities\":[\"65000:1\"]}}}"
	t.Run("PUT set-community container switching to inline", processSetRequest(url, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":      "permit",
				"match_prefix_set":     "PS4",
				"match_med":            "20",
				"set_local_pref":       "150",
				"set_med":              "5",
				"set_community_inline": "65000:1",
				"set_asn":              "65001",
				"set_repeat_asn":       "2",
			},
		},
	}
	t.Run("Verify set-community REPLACE swapped only the community action", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	url = bgpActionsUrl + "/set-community/reference"
	urlInputBodyJSON = "{\"config\":{\"community-set-refs\":[\"CS1\"]}}"
	t.Run("PUT set-community reference container", processSetRequest(url, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": stmtRow,
		},
	}
	t.Run("Verify reference REPLACE dropped the inline communities", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The statement only references the set, so the set keeps its members.
	expectedMap = map[string]interface{}{
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": map[string]interface{}{
				"set_type":          "STANDARD",
				"match_action":      "ANY",
				"community_member@": "65000:9",
			},
		},
	}
	t.Run("Verify reference REPLACE left the referenced community-set alone", verifyDbResult(rclient, "COMMUNITY_SET|CS1", expectedMap, false))
	time.Sleep(1 * time.Second)

	url = bgpActionsUrl + "/set-as-path-prepend"
	urlInputBodyJSON = "{\"config\":{\"asn\":65002}}"
	t.Run("PUT set-as-path-prepend container", processSetRequest(url, urlInputBodyJSON, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation":   "permit",
				"match_prefix_set":  "PS4",
				"match_med":         "20",
				"set_local_pref":    "150",
				"set_med":           "5",
				"set_community_ref": "CS1",
				"set_asn":           "65002",
			},
		},
	}
	t.Run("Verify set-as-path-prepend REPLACE dropped only repeat-n", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_policy_definition_statement_conditions_match_prefix_set_match_set_options(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{},
		},
		"PREFIX_SET": map[string]interface{}{
			"PS4": map[string]interface{}{
				"mode": "IPv4",
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
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// ANY is the only modifier FRR can honour and it maps to no field, so the
	// container write still has to land the prefix-set on its own.
	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/conditions/match-prefix-set/config"
	urlInputBodyJSON = "{\"prefix-set\":\"PS4\",\"match-set-options\":\"ANY\"}"
	t.Run("PATCH match-prefix-set config with match-set-options ANY", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_prefix_set": "PS4",
			},
		},
	}
	t.Run("Verify match-set-options ANY adds no ROUTE_MAP field", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The modifier is not stored, so a GET reports the ANY default once a
	// prefix-set is configured.
	t.Run("GET match-prefix-set config match-set-options",
		processGetRequest(url+"/match-set-options", nil, `{"openconfig-routing-policy:match-set-options":"ANY"}`, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"openconfig-routing-policy:match-set-options\":\"ANY\"}"
	t.Run("PATCH match-prefix-set match-set-options leaf ANY", processSetRequest(url+"/match-set-options", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify leaf write of ANY left ROUTE_MAP unchanged", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	// INVERT has no CONFIG_DB representation, so it is refused rather than
	// silently ignored, at the leaf and at the container above it.
	expectedErr := tlerr.NotSupportedError{Format: "routing-policy match-prefix-set match-set-options supports only ANY"}
	urlInputBodyJSON = "{\"openconfig-routing-policy:match-set-options\":\"INVERT\"}"
	t.Run("PATCH match-prefix-set match-set-options INVERT rejected",
		processSetRequest(url+"/match-set-options", urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"prefix-set\":\"PS4\",\"match-set-options\":\"INVERT\"}"
	t.Run("PATCH match-prefix-set config with INVERT rejected",
		processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify refused INVERT left ROUTE_MAP unchanged", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	// Removing the unmapped modifier must not take the prefix-set with it.
	t.Run("DELETE match-prefix-set match-set-options leaf", processDeleteRequest(url+"/match-set-options", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify DELETE of match-set-options kept match_prefix_set", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
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
				// The two spellings are mutually exclusive, so writing the
				// reference empties the inline field rather than leaving a
				// value there for a GET to choose between.
				"set_community_inline": "",
				"set_community_ref":    "COMMSET1",
			},
		},
	}
	t.Run("Verify ROUTE_MAP set_community_ref mapping", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-bgp-policy:set-community\":{\"config\":{\"method\":\"REFERENCE\"},\"reference\":{\"config\":" +
		"{\"community-set-ref\":\"COMMSET1\",\"community-set-refs\":[\"COMMSET1\"]}}}}"
	t.Run("GET routing-policy statement actions bgp set-community", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy statement actions bgp set-community", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				// Both fields go, and a row left with none carries the
				// placeholder instead.
				"NULL": "NULL",
			},
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

	// Stored in the form FRR's parser accepts: space separated, and NO_EXPORT
	// written as the keyword FRR knows it by.
	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"set_community_inline": "65001:100 no-export",
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
			"POL1|10": map[string]interface{}{
				// The only field goes, and a row left with none carries the
				// placeholder instead.
				"NULL": "NULL",
			},
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

	// bgp-set-med-type carried a signed string member until openconfig-bgp-policy
	// 8.0.0 split the increment out into set-med-action, so a +N MED is now
	// refused while binding the request. The wording belongs to ygot and is not
	// asserted; that the stored MED is untouched is what shows the write was
	// refused rather than half applied.
	t.Run("PATCH routing-policy statement bgp-actions set-med as +10 rejected",
		processSetRequest(url+"/set-med", "{\"openconfig-bgp-policy:set-med\":\"+10\"}", "PATCH", true))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP kept the numeric MED after the refused increment",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
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

func Test_openconfig_routing_policy_policy_definition_statement_actions_bgp_actions_set_med_action(t *testing.T) {
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

	url := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/bgp-actions/config"

	// A MED written with an action lands in set_metric_action and set_metric,
	// the pair frrcfgd renders as "set metric +N", rather than in set_med,
	// which can only render a bare "set metric N".
	storedAction := func(action, value string) map[string]interface{} {
		return map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{
					"set_metric_action": action,
					"set_metric":        value,
				},
			},
		}
	}

	t.Run("PATCH bgp-actions config with set-med and set-med-action ADD",
		processSetRequest(url, "{\"set-med\":10,\"set-med-action\":\"ADD\"}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP holds the increment",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedAction("METRIC_ADD_VALUE", "10"), false))
	time.Sleep(1 * time.Second)
	t.Run("GET bgp-actions config reports the MED and the action",
		processGetRequest(url, nil, "{\"openconfig-bgp-policy:config\":{\"set-med\":10,\"set-med-action\":\"ADD\"}}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET bgp-actions set-med-action on its own",
		processGetRequest(url+"/set-med-action", nil, "{\"openconfig-bgp-policy:set-med-action\":\"ADD\"}", false))
	time.Sleep(1 * time.Second)
	t.Run("GET bgp-actions set-med reads the value out of set_metric",
		processGetRequest(url+"/set-med", nil, "{\"openconfig-bgp-policy:set-med\":10}", false))
	time.Sleep(1 * time.Second)

	// SUBTRACT is the case the old signed string could never reach: CVL refused
	// -10 outright, because set_med is a plain uint32.
	t.Run("PATCH bgp-actions set-med-action to SUBTRACT on its own",
		processSetRequest(url+"/set-med-action", "{\"openconfig-bgp-policy:set-med-action\":\"SUBTRACT\"}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP kept the value and took the decrement",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedAction("METRIC_SUBTRACT_VALUE", "10"), false))
	time.Sleep(1 * time.Second)

	// The value leaf on its own keeps the action already configured.
	t.Run("PATCH bgp-actions set-med on its own",
		processSetRequest(url+"/set-med", "{\"openconfig-bgp-policy:set-med\":25}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP took the new value under the same action",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedAction("METRIC_SUBTRACT_VALUE", "25"), false))
	time.Sleep(1 * time.Second)

	// SET is the action that means the same thing as a plain MED, and it still
	// goes through set_metric_action, since that is what the request asked for.
	t.Run("PATCH bgp-actions set-med-action to SET",
		processSetRequest(url+"/set-med-action", "{\"openconfig-bgp-policy:set-med-action\":\"SET\"}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP holds the plain set action",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedAction("METRIC_SET_VALUE", "25"), false))
	time.Sleep(1 * time.Second)

	// Dropping the action drops the value with it. frrcfgd reaches set_metric
	// only through set_metric_action, so a value left behind would quietly stop
	// being applied.
	t.Run("DELETE bgp-actions set-med-action", processDeleteRequest(url+"/set-med-action", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP has neither the action nor the value",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{"NULL": "NULL"},
			},
		}, false))
	time.Sleep(1 * time.Second)

	// An action with no value anywhere to apply it to has nothing to write.
	expectedErr := tlerr.InvalidArgsError{
		Format: "routing-policy statement actions bgp-actions config set-med-action needs a set-med value, which neither the request nor the statement carries",
	}
	t.Run("PATCH bgp-actions set-med-action with no value to act on rejected",
		processSetRequest(url+"/set-med-action", "{\"openconfig-bgp-policy:set-med-action\":\"ADD\"}", "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// Writing a plain MED clears the action spelling, since frrcfgd binds both
	// and would otherwise emit two competing set metric lines.
	t.Run("PATCH bgp-actions config with an action, to be displaced",
		processSetRequest(url, "{\"set-med\":30,\"set-med-action\":\"ADD\"}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("DELETE bgp-actions set-med", processDeleteRequest(url+"/set-med", false))
	time.Sleep(1 * time.Second)
	t.Run("PATCH bgp-actions set-med with no action in play",
		processSetRequest(url+"/set-med", "{\"openconfig-bgp-policy:set-med\":40}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP holds the plain MED alone",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{
					// The delete above took the row's last field with it, so
					// the placeholder SONiC leaves in an otherwise empty row is
					// still here alongside the MED written after it.
					"NULL":    "NULL",
					"set_med": "40",
				},
			},
		}, false))
	time.Sleep(1 * time.Second)
	t.Run("GET bgp-actions config reports the MED with no action",
		processGetRequest(url, nil, "{\"openconfig-bgp-policy:config\":{\"set-med\":40}}", false))
	time.Sleep(1 * time.Second)

	// And an action written over a plain MED takes that MED as its value.
	t.Run("PATCH bgp-actions set-med-action over a plain MED",
		processSetRequest(url+"/set-med-action", "{\"openconfig-bgp-policy:set-med-action\":\"ADD\"}", "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the plain MED became the increment value",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{
					"NULL":              "NULL",
					"set_metric_action": "METRIC_ADD_VALUE",
					"set_metric":        "40",
				},
			},
		}, false))
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
			"POL1": map[string]interface{}{"NULL": "NULL"},
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
		`"config":{"policy-result":"ACCEPT_ROUTE"}},"config":{"name":"10"},"name":"10","state":{"name":"10"}}`
	policyEntryJSON := `{"config":{"name":"POL1"},"name":"POL1","state":{"name":"POL1"},` +
		`"statements":{"statement":[` + statementEntryJSON + `]}}`
	definedSetsJSON := `{"openconfig-bgp-policy:bgp-defined-sets":{"community-sets":{"community-set":[` +
		`{"community-set-name":"COMM1","config":{"community-set-name":"COMM1"},"state":{"community-set-name":"COMM1"}}]}}}`

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

// A GET naming one list instance must answer with that instance alone. The
// container GET walks every mapped node in CONFIG_DB, so with siblings present
// an unfiltered walk answers a request for one statement with the whole list.
func Test_openconfig_routing_policy_container_get_list_instance(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{"NULL": "NULL"},
			"POL2": map[string]interface{}{"NULL": "NULL"},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"route_operation": "permit",
				"set_med":         "110",
			},
			"POL1|20": map[string]interface{}{
				"route_operation": "deny",
			},
			"POL2|10": map[string]interface{}{
				"route_operation": "permit",
			},
		},
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{"set_type": "STANDARD"},
			"COMM2": map[string]interface{}{"set_type": "STANDARD"},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
			"POL1|20": "",
			"POL2|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
			"POL2": "",
		},
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": "",
			"COMM2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	stmt10JSON := `{"actions":{"openconfig-bgp-policy:bgp-actions":{"config":{"set-med":110}},` +
		`"config":{"policy-result":"ACCEPT_ROUTE"}},"config":{"name":"10"},"name":"10","state":{"name":"10"}}`
	stmt20JSON := `{"actions":{"config":{"policy-result":"REJECT_ROUTE"}},"config":{"name":"20"},"name":"20","state":{"name":"20"}}`
	pol1JSON := `{"config":{"name":"POL1"},"name":"POL1","state":{"name":"POL1"},"statements":{"statement":[` +
		stmt10JSON + `,` + stmt20JSON + `]}}`
	pol2Stmt := `{"actions":{"config":{"policy-result":"ACCEPT_ROUTE"}},"config":{"name":"10"},"name":"10","state":{"name":"10"}}`
	pol2JSON := `{"config":{"name":"POL2"},"name":"POL2","state":{"name":"POL2"},"statements":{"statement":[` + pol2Stmt + `]}}`

	pol1Url := rpolContainerUrl + "/policy-definitions/policy-definition[name=POL1]"

	expectedGetJSON := `{"openconfig-routing-policy:statement":[` + stmt20JSON + `]}`
	t.Run("GET statement list instance returns only that statement",
		processGetRequest(pol1Url+"/statements/statement[name=20]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:statement":[` + stmt10JSON + `]}`
	t.Run("GET the other statement list instance",
		processGetRequest(pol1Url+"/statements/statement[name=10]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The list itself still reports every sibling.
	expectedGetJSON = `{"openconfig-routing-policy:statement":[` + stmt10JSON + `,` + stmt20JSON + `]}`
	t.Run("GET statement list returns both statements",
		processGetRequest(pol1Url+"/statements/statement", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:policy-definition":[` + pol2JSON + `]}`
	t.Run("GET policy-definition list instance returns only that policy",
		processGetRequest(rpolContainerUrl+"/policy-definitions/policy-definition[name=POL2]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:policy-definition":[` + pol1JSON + `,` + pol2JSON + `]}`
	t.Run("GET policy-definition list returns both policies",
		processGetRequest(rpolContainerUrl+"/policy-definitions/policy-definition", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-bgp-policy:community-set":[` +
		`{"community-set-name":"COMM2","config":{"community-set-name":"COMM2"},"state":{"community-set-name":"COMM2"}}]}`
	t.Run("GET community-set list instance returns only that set",
		processGetRequest(rpolContainerUrl+"/defined-sets/bgp-defined-sets/community-sets/community-set[community-set-name=COMM2]",
			nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// A node inside the named instance is answered from that instance.
	expectedGetJSON = `{"openconfig-routing-policy:config":{"policy-result":"REJECT_ROUTE"}}`
	t.Run("GET actions config under the named statement",
		processGetRequest(pol1Url+"/statements/statement[name=20]/actions/config", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// Filtering must not prune a request that asks for the whole subtree.
	definedSetsJSON := `{"openconfig-bgp-policy:bgp-defined-sets":{"community-sets":{"community-set":[` +
		`{"community-set-name":"COMM1","config":{"community-set-name":"COMM1"},"state":{"community-set-name":"COMM1"}},` +
		`{"community-set-name":"COMM2","config":{"community-set-name":"COMM2"},"state":{"community-set-name":"COMM2"}}]}}}`
	expectedGetJSON = `{"openconfig-routing-policy:routing-policy":{"defined-sets":` + definedSetsJSON +
		`,"policy-definitions":{"policy-definition":[` + pol1JSON + `,` + pol2JSON + `]}}}`
	t.Run("GET routing-policy container still reports everything",
		processGetRequest(rpolContainerUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// A mapped node stands for the row a handler reads, not for the node the
// response carries, and the two are not always the same branch: the prefix-set
// config URI is what fills the prefixes list. A GET aimed at prefixes must
// therefore still reach that node, while a GET naming one prefix-set must not
// answer with another.
func Test_openconfig_routing_policy_prefix_set_get_levels(t *testing.T) {
	preReqMap := map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{"mode": "IPv4"},
			"PS2": map[string]interface{}{"mode": "IPv4"},
		},
		"PREFIX": map[string]interface{}{
			"PS1|5|10.11.0.0/16|exact":   map[string]interface{}{"action": "permit"},
			"PS1|10|10.12.0.0/16|17..24": map[string]interface{}{"action": "permit"},
			"PS2|5|10.21.0.0/16|exact":   map[string]interface{}{"action": "permit"},
		},
	}
	cleanupTbl := map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.11.0.0/16|exact":   "",
			"PS1|10|10.12.0.0/16|17..24": "",
			"PS2|5|10.21.0.0/16|exact":   "",
		},
		"PREFIX_SET": map[string]interface{}{
			"PS1": "",
			"PS2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	pfxSetsUrl := "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets"
	ps1Url := pfxSetsUrl + "/prefix-set[name=PS1]"
	ps2Url := pfxSetsUrl + "/prefix-set[name=PS2]"

	ps1PrefixesJSON := `{"prefix":[` +
		`{"config":{"ip-prefix":"10.11.0.0/16","masklength-range":"exact"},` +
		`"ip-prefix":"10.11.0.0/16","masklength-range":"exact",` +
		`"state":{"ip-prefix":"10.11.0.0/16","masklength-range":"exact"}},` +
		`{"config":{"ip-prefix":"10.12.0.0/16","masklength-range":"17..24"},` +
		`"ip-prefix":"10.12.0.0/16","masklength-range":"17..24",` +
		`"state":{"ip-prefix":"10.12.0.0/16","masklength-range":"17..24"}}]}`
	ps2PrefixesJSON := `{"prefix":[` +
		`{"config":{"ip-prefix":"10.21.0.0/16","masklength-range":"exact"},` +
		`"ip-prefix":"10.21.0.0/16","masklength-range":"exact",` +
		`"state":{"ip-prefix":"10.21.0.0/16","masklength-range":"exact"}}]}`

	t.Run("GET the prefixes container under a named prefix-set",
		processGetRequest(ps1Url+"/prefixes", nil,
			`{"openconfig-routing-policy:prefixes":`+ps1PrefixesJSON+`}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET the prefixes container of the other prefix-set",
		processGetRequest(ps2Url+"/prefixes", nil,
			`{"openconfig-routing-policy:prefixes":`+ps2PrefixesJSON+`}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET the prefix-set list instance carries its own prefixes",
		processGetRequest(ps1Url, nil,
			`{"openconfig-routing-policy:prefix-set":[{"config":{"mode":"IPV4","name":"PS1"},`+
				`"name":"PS1","prefixes":`+ps1PrefixesJSON+`,"state":{"mode":"IPV4","name":"PS1"}}]}`, false))
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
		`"conditions":{"match-interface":{"config":{"interface":"Ethernet0","subinterface":1}},` +
		`"match-prefix-set":{"config":{"prefix-set":"PFX1"}}}}]}}]}}`
	expectedErr := tlerr.NotSupportedError{
		Format: "routing-policy container payload carries leaves that have no CONFIG_DB mapping: " +
			"routing-policy/policy-definitions/policy-definition[POL1]/statements/statement[10]/conditions/match-interface/config/subinterface",
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
		Format: "DELETE is not supported at a routing-policy container level; delete a list instance or a mapped child path instead",
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

// A payload anchored at the set-community reference node carries no method, so
// the set-community node above it has nothing to resolve and the referenced set
// has to be mapped from the reference node itself. The container walk used to
// skip the reference subtree, which reported the request applied and wrote
// nothing.
func Test_openconfig_routing_policy_set_community_reference_container_set(t *testing.T) {
	var urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{"NULL": "NULL"},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{"route_operation": "permit"},
		},
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": map[string]interface{}{"set_type": "STANDARD"},
			"CS2": map[string]interface{}{"set_type": "STANDARD"},
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
			"CS1": "",
			"CS2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	setCommUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]" +
		"/statements/statement[name=10]/actions/bgp-actions/set-community"
	refUrl := setCommUrl + "/reference"

	storedRef := func(name string) map[string]interface{} {
		return map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{
					"route_operation": "permit",
					// Writing a reference clears the inline spelling, which
					// leaves that field present and empty rather than absent.
					"set_community_inline": "",
					"set_community_ref":    name,
				},
			},
		}
	}

	urlInputBodyJSON = `{"config":{"community-set-refs":["CS1"]}}`
	t.Run("PATCH set-community reference container with the leaf-list",
		processSetRequest(refUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the reference container payload reached set_community_ref",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS1"), false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = `{"config":{"community-set-ref":"CS2"}}`
	t.Run("PATCH set-community reference container with the singular leaf",
		processSetRequest(refUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the singular leaf reached set_community_ref",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS2"), false))
	time.Sleep(1 * time.Second)

	// One level up, at set-community, the same subtree is refused rather than
	// dropped. That node resolves the method itself, and a payload naming
	// neither a method nor inline communities does not say which branch it
	// means.
	urlInputBodyJSON = `{"reference":{"config":{"community-set-refs":["CS1"]}}}`
	expectedErr := tlerr.InvalidArgsError{
		Format: "routing-policy statement actions bgp-actions set-community config is missing in request payload",
	}
	t.Run("PATCH set-community carrying a methodless reference subtree rejected",
		processSetRequest(setCommUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify the rejected payload left the stored reference alone",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS2"), false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// A DELETE naming a list instance has to remove that instance. Every
// leaf-level URI shape requires a child node, so an instance URI is classified
// as container level, where DELETE used to be refused by a message that told
// the caller to delete a list instance.
func Test_openconfig_routing_policy_list_instance_delete(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{"NULL": "NULL"},
			"POL2": map[string]interface{}{"NULL": "NULL"},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{"route_operation": "permit"},
			"POL1|20": map[string]interface{}{"route_operation": "deny"},
			"POL2|10": map[string]interface{}{"route_operation": "permit"},
		},
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": map[string]interface{}{"set_type": "STANDARD"},
		},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EC1": map[string]interface{}{"set_type": "STANDARD"},
		},
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{"mode": "IPv4"},
		},
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact":   map[string]interface{}{"action": "permit"},
			"PS1|10|10.2.0.0/16|17..24": map[string]interface{}{"action": "permit"},
		},
	}
	cleanupTbl := map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact":   "",
			"PS1|10|10.2.0.0/16|17..24": "",
		},
		"PREFIX_SET": map[string]interface{}{
			"PS1": "",
		},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EC1": "",
		},
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": "",
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": "",
			"POL1|20": "",
			"POL2|10": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": "",
			"POL2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	definedSetsUrl := rpolContainerUrl + "/defined-sets"
	policyUrl := rpolContainerUrl + "/policy-definitions/policy-definition[name=POL1]"
	gone := func(table, key string) map[string]interface{} {
		return map[string]interface{}{
			table: map[string]interface{}{
				key: map[string]interface{}{},
			},
		}
	}

	t.Run("DELETE routing-policy statement list instance",
		processDeleteRequest(policyUrl+"/statements/statement[name=20]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the named statement row is gone",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|20", gone("ROUTE_MAP", "POL1|20"), false))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{"route_operation": "permit"},
		},
	}
	t.Run("Verify the sibling statement survived",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A policy holds its statements, so removing the policy removes them too.
	// A ROUTE_MAP row left behind would sit under a policy that no longer
	// exists.
	t.Run("DELETE routing-policy policy-definition list instance",
		processDeleteRequest(policyUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the policy row is gone",
		verifyDbResult(rclient, "ROUTE_MAP_SET|POL1", gone("ROUTE_MAP_SET", "POL1"), false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the statement the policy held went with it",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", gone("ROUTE_MAP", "POL1|10"), false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL2|10": map[string]interface{}{"route_operation": "permit"},
		},
	}
	t.Run("Verify the other policy was left alone",
		verifyDbResult(rclient, "ROUTE_MAP|POL2|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy community-set list instance",
		processDeleteRequest(definedSetsUrl+"/bgp-defined-sets/community-sets/community-set[community-set-name=CS1]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the community-set row is gone",
		verifyDbResult(rclient, "COMMUNITY_SET|CS1", gone("COMMUNITY_SET", "CS1"), false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy ext-community-set list instance",
		processDeleteRequest(definedSetsUrl+"/bgp-defined-sets/ext-community-sets/ext-community-set[ext-community-set-name=EC1]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the ext-community-set row is gone",
		verifyDbResult(rclient, "EXTENDED_COMMUNITY_SET|EC1", gone("EXTENDED_COMMUNITY_SET", "EC1"), false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy prefix list instance",
		processDeleteRequest(definedSetsUrl+"/prefix-sets/prefix-set[name=PS1]"+
			"/prefixes/prefix[ip-prefix=10.2.0.0/16][masklength-range=17..24]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the named prefix row is gone",
		verifyDbResult(rclient, "PREFIX|PS1|10|10.2.0.0/16|17..24", gone("PREFIX", "PS1|10|10.2.0.0/16|17..24"), false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact": map[string]interface{}{"action": "permit"},
		},
	}
	t.Run("Verify the sibling prefix survived",
		verifyDbResult(rclient, "PREFIX|PS1|5|10.1.0.0/16|exact", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A prefix-set holds its prefixes the same way a policy holds statements.
	t.Run("DELETE routing-policy prefix-set list instance",
		processDeleteRequest(definedSetsUrl+"/prefix-sets/prefix-set[name=PS1]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the prefix-set row is gone",
		verifyDbResult(rclient, "PREFIX_SET|PS1", gone("PREFIX_SET", "PS1"), false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the prefix the set held went with it",
		verifyDbResult(rclient, "PREFIX|PS1|5|10.1.0.0/16|exact", gone("PREFIX", "PS1|5|10.1.0.0/16|exact"), false))
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

func Test_openconfig_routing_policy_defined_sets_prefix_set_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// Naming a set is enough to create it, and the mode CONFIG_DB defaults to
	// is seeded because frrcfgd drops a prefix-set that carries none.
	url = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]/config/name"
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"PS1\"}"
	t.Run("PATCH routing-policy prefix-set config name", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{
				"mode": "IPv4",
			},
		},
	}
	t.Run("Verify PREFIX_SET row seeded with the default mode", verifyDbResult(rclient, "PREFIX_SET|PS1", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix-set config name",
		processGetRequest(url, nil, "{\"openconfig-routing-policy:name\":\"PS1\"}", false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "prefix-set name payload (PS2) does not match key (PS1)",
	}
	urlInputBodyJSON = "{\"openconfig-routing-policy:name\":\"PS2\"}"
	t.Run("PATCH prefix-set name mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]/config/mode"
	urlInputBodyJSON = "{\"openconfig-routing-policy:mode\":\"IPV6\"}"
	t.Run("PATCH routing-policy prefix-set config mode", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{
				"mode": "IPv6",
			},
		},
	}
	t.Run("Verify PREFIX_SET mode", verifyDbResult(rclient, "PREFIX_SET|PS1", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix-set config mode",
		processGetRequest(url, nil, "{\"openconfig-routing-policy:mode\":\"IPV6\"}", false))
	time.Sleep(1 * time.Second)

	// PREFIX_SET holds one address family per set and FRR keeps its
	// prefix-lists the same way, so MIXED is refused rather than resolved to
	// one of them.
	unsupportedErr := tlerr.NotSupportedError{
		Format: "routing-policy prefix-set mode MIXED has no CONFIG_DB mapping; configure one prefix-set per address family",
	}
	urlInputBodyJSON = "{\"openconfig-routing-policy:mode\":\"MIXED\"}"
	t.Run("PATCH prefix-set config mode MIXED rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, unsupportedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify refused MIXED left PREFIX_SET unchanged", verifyDbResult(rclient, "PREFIX_SET|PS1", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A prefix-set always has an address family, so the mode leaf has no state
	// to be deleted back to.
	unsupportedErr = tlerr.NotSupportedError{
		Format: "DELETE is not supported for prefix-set config mode; a prefix-set always has an address family, so delete the prefix-set instead",
	}
	t.Run("DELETE prefix-set config mode rejected", processDeleteRequest(url, true, unsupportedErr))
	time.Sleep(1 * time.Second)

	url = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]/config"
	urlInputBodyJSON = "{\"name\":\"PS1\",\"mode\":\"IPV4\"}"
	t.Run("PATCH routing-policy prefix-set config container", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{
				"mode": "IPv4",
			},
		},
	}
	t.Run("Verify PREFIX_SET row from config container", verifyDbResult(rclient, "PREFIX_SET|PS1", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix-set config container",
		processGetRequest(url, nil, "{\"openconfig-routing-policy:config\":{\"mode\":\"IPV4\",\"name\":\"PS1\"}}", false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix-set name key leaf",
		processGetRequest("/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]/name",
			nil, "{\"openconfig-routing-policy:name\":\"PS1\"}", false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE routing-policy prefix-set config container", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{},
		},
	}
	t.Run("Verify PREFIX_SET row deleted", verifyDbResult(rclient, "PREFIX_SET|PS1", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_defined_sets_prefix_set_prefix_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{
				"mode": "IPv4",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact":   "",
			"PS1|10|10.2.0.0/16|17..24": "",
		},
		"PREFIX_SET": map[string]interface{}{
			"PS1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// OpenConfig keys a prefix by ip-prefix and masklength-range and has no
	// sequence-number leaf, so the sequence is allocated here, from 5 upwards
	// in steps of 5 the way FRR numbers entries it is given without one. Every
	// row is a permit because a prefix-set is purely a match set in
	// OpenConfig, and frrcfgd ignores a row that carries no action at all.
	url = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]" +
		"/prefixes/prefix[ip-prefix=10.1.0.0/16][masklength-range=exact]/config"
	urlInputBodyJSON = "{\"ip-prefix\":\"10.1.0.0/16\",\"masklength-range\":\"exact\"}"
	t.Run("PATCH routing-policy prefix config container", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact": map[string]interface{}{
				"action": "permit",
			},
		},
	}
	t.Run("Verify PREFIX row at the first allocated sequence",
		verifyDbResult(rclient, "PREFIX|PS1|5|10.1.0.0/16|exact", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix config container",
		processGetRequest(url, nil,
			"{\"openconfig-routing-policy:config\":{\"ip-prefix\":\"10.1.0.0/16\",\"masklength-range\":\"exact\"}}", false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix config ip-prefix",
		processGetRequest(url+"/ip-prefix", nil, "{\"openconfig-routing-policy:ip-prefix\":\"10.1.0.0/16\"}", false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix config masklength-range",
		processGetRequest(url+"/masklength-range", nil, "{\"openconfig-routing-policy:masklength-range\":\"exact\"}", false))
	time.Sleep(1 * time.Second)

	// Rewriting the same entry updates its row rather than adding a second
	// copy of the prefix at a fresh sequence number. The action already stored
	// is left alone, so the rewrite carries no field of its own and the row
	// picks up the placeholder a fieldless write leaves behind.
	t.Run("PATCH routing-policy prefix config container again", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	rewrittenMap := map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact": map[string]interface{}{
				"NULL":   "NULL",
				"action": "permit",
			},
		},
	}
	t.Run("Verify rewriting the entry kept its sequence",
		verifyDbResult(rclient, "PREFIX|PS1|5|10.1.0.0/16|exact", rewrittenMap, false))
	time.Sleep(1 * time.Second)

	// A second entry takes the next step, and a range renders as "ge N le M".
	url = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]" +
		"/prefixes/prefix[ip-prefix=10.2.0.0/16][masklength-range=17..24]/config/ip-prefix"
	urlInputBodyJSON = "{\"openconfig-routing-policy:ip-prefix\":\"10.2.0.0/16\"}"
	t.Run("PATCH routing-policy prefix config ip-prefix", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|10|10.2.0.0/16|17..24": map[string]interface{}{
				"action": "permit",
			},
		},
	}
	t.Run("Verify PREFIX row at the next allocated sequence",
		verifyDbResult(rclient, "PREFIX|PS1|10|10.2.0.0/16|17..24", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A range that does not widen the prefix it qualifies is refused, since
	// FRR would not accept the "ge" it renders to.
	badUrl := "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]" +
		"/prefixes/prefix[ip-prefix=10.3.0.0/16][masklength-range=8..24]/config"
	expectedErr := tlerr.InvalidArgsError{
		Format: "routing-policy prefix masklength-range 8..24 must satisfy 16 <= N <= M <= 32 for prefix 10.3.0.0/16",
	}
	urlInputBodyJSON = "{\"ip-prefix\":\"10.3.0.0/16\",\"masklength-range\":\"8..24\"}"
	t.Run("PATCH prefix with a range below the prefix length rejected",
		processSetRequest(badUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// A prefix of the wrong family for the set it is going into is refused
	// rather than stored where FRR would never render it.
	badUrl = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]" +
		"/prefixes/prefix[ip-prefix=2001:db8::/32][masklength-range=exact]/config"
	expectedErr = tlerr.InvalidArgsError{
		Format: "routing-policy prefix 2001:db8::/32 is IPv6 but prefix-set PS1 has mode IPv4",
	}
	urlInputBodyJSON = "{\"ip-prefix\":\"2001:db8::/32\",\"masklength-range\":\"exact\"}"
	t.Run("PATCH IPv6 prefix into an IPv4 prefix-set rejected",
		processSetRequest(badUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// A prefix carrying host bits would make a GET report a different list key
	// than the SET used, so it is refused instead of being masked silently.
	badUrl = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]" +
		"/prefixes/prefix[ip-prefix=10.4.1.1/16][masklength-range=exact]/config"
	expectedErr = tlerr.InvalidArgsError{
		Format: "routing-policy prefix ip-prefix 10.4.1.1/16 carries host bits; use its canonical form 10.4.0.0/16",
	}
	urlInputBodyJSON = "{\"ip-prefix\":\"10.4.1.1/16\",\"masklength-range\":\"exact\"}"
	t.Run("PATCH non-canonical prefix rejected", processSetRequest(badUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// ip-prefix is a list key, so removing it removes the entry it identifies
	// rather than clearing a field of it.
	url = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]" +
		"/prefixes/prefix[ip-prefix=10.2.0.0/16][masklength-range=17..24]/ip-prefix"
	t.Run("DELETE routing-policy prefix ip-prefix key leaf", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|10|10.2.0.0/16|17..24": map[string]interface{}{},
		},
	}
	t.Run("Verify PREFIX row removed", verifyDbResult(rclient, "PREFIX|PS1|10|10.2.0.0/16|17..24", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_defined_sets_prefix_sets_container_level(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact":   "",
			"PS1|10|10.2.0.0/16|17..24": "",
		},
		"PREFIX_SET": map[string]interface{}{
			"PS1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// A container level write has to create the set and every prefix under it
	// in one pass, allocating a distinct sequence number for each even though
	// CONFIG_DB holds none of them yet.
	url = "/openconfig-routing-policy:routing-policy"
	urlInputBodyJSON = `{"defined-sets":{"prefix-sets":{"prefix-set":[` +
		`{"name":"PS1","config":{"name":"PS1","mode":"IPV4"},"prefixes":{"prefix":[` +
		`{"ip-prefix":"10.1.0.0/16","masklength-range":"exact",` +
		`"config":{"ip-prefix":"10.1.0.0/16","masklength-range":"exact"}},` +
		`{"ip-prefix":"10.2.0.0/16","masklength-range":"17..24",` +
		`"config":{"ip-prefix":"10.2.0.0/16","masklength-range":"17..24"}}]}}]}}}`
	t.Run("PATCH routing-policy container with a whole prefix-set",
		processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{
				"mode": "IPv4",
			},
		},
	}
	t.Run("Verify PREFIX_SET row from the container write", verifyDbResult(rclient, "PREFIX_SET|PS1", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact": map[string]interface{}{
				"action": "permit",
			},
		},
	}
	t.Run("Verify first PREFIX row from the container write",
		verifyDbResult(rclient, "PREFIX|PS1|5|10.1.0.0/16|exact", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|10|10.2.0.0/16|17..24": map[string]interface{}{
				"action": "permit",
			},
		},
	}
	t.Run("Verify second PREFIX row got its own sequence",
		verifyDbResult(rclient, "PREFIX|PS1|10|10.2.0.0/16|17..24", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The same payload sent one level down maps the same way, so that a
	// request anchored at the list instance is not silently narrowed to the
	// set row with its prefixes dropped. A request that already names the
	// instance carries the instance's own children, not the list again.
	url = "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]"
	urlInputBodyJSON = `{"openconfig-routing-policy:config":{"name":"PS1","mode":"IPV4"},` +
		`"openconfig-routing-policy:prefixes":{"prefix":[` +
		`{"ip-prefix":"10.1.0.0/16","masklength-range":"exact",` +
		`"config":{"ip-prefix":"10.1.0.0/16","masklength-range":"exact"}}]}}`
	t.Run("PATCH routing-policy prefix-set list instance", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// The entry is rewritten rather than created, so its stored action stands
	// and the row carries the placeholder a fieldless write leaves behind.
	expectedMap = map[string]interface{}{
		"PREFIX": map[string]interface{}{
			"PS1|5|10.1.0.0/16|exact": map[string]interface{}{
				"NULL":   "NULL",
				"action": "permit",
			},
		},
	}
	t.Run("Verify the list instance write kept the entry at its sequence",
		verifyDbResult(rclient, "PREFIX|PS1|5|10.1.0.0/16|exact", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A leaf error inside a container payload has to be reported rather than
	// swallowed along with the rest of the request.
	url = "/openconfig-routing-policy:routing-policy"
	expectedErr := tlerr.NotSupportedError{
		Format: "routing-policy prefix-set mode MIXED has no CONFIG_DB mapping; configure one prefix-set per address family",
	}
	urlInputBodyJSON = `{"defined-sets":{"prefix-sets":{"prefix-set":[` +
		`{"name":"PS1","config":{"name":"PS1","mode":"MIXED"}}]}}}`
	t.Run("PATCH routing-policy container with MIXED mode rejected",
		processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_routing_policy_match_prefix_set_unknown_set(t *testing.T) {
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
		"PREFIX": map[string]interface{}{
			"PS6|5|2001:db8::/32|exact": "",
		},
		"PREFIX_SET": map[string]interface{}{
			"PS6": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// The condition only names a set, so the address family that decides which
	// ROUTE_MAP field carries it comes from the set. A set that does not exist
	// yet used to be created as IPv4 regardless of what it was meant to hold,
	// which silently drove the IPv4 field for an IPv6 set.
	url = "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]" +
		"/statements/statement[name=10]/conditions/match-prefix-set/config/prefix-set"
	expectedErr := tlerr.InvalidArgsError{
		Format: "routing-policy prefix-set PS6 is not configured; create it under defined-sets/prefix-sets with a mode before matching on it",
	}
	urlInputBodyJSON = "{\"openconfig-routing-policy:prefix-set\":\"PS6\"}"
	t.Run("PATCH match-prefix-set naming an unknown prefix-set rejected",
		processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// Once the set exists with its own mode, the condition resolves to the
	// field of that family.
	psUrl := "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS6]/config"
	urlInputBodyJSON = "{\"name\":\"PS6\",\"mode\":\"IPV6\"}"
	t.Run("PATCH routing-policy prefix-set PS6 as IPv6", processSetRequest(psUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = "{\"openconfig-routing-policy:prefix-set\":\"PS6\"}"
	t.Run("PATCH match-prefix-set naming the IPv6 prefix-set", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"match_ipv6_prefix_set": "PS6",
			},
		},
	}
	t.Run("Verify ROUTE_MAP took the IPv6 match field", verifyDbResult(rclient, "ROUTE_MAP|POL1|10", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// openconfig-bgp-policy 6.2.0 added the community-set-refs leaf-list and
// deprecated the singular community-set-ref, so that a statement could name more
// than one community set. ROUTE_MAP set_community_ref is a single leafref, so the
// list is served for exactly one reference: that is what this DB can express, and
// a longer list is refused rather than quietly stored as its first entry.
//
// The reference config also gains a mapping of its own here, so that a request
// can be anchored on it the way one can already be anchored on the inline config.
// Nothing in CONFIG_DB records which method is in use - a GET reports REFERENCE
// because set_community_ref is the field carrying a value - so a reference
// written at that anchor clears the inline spelling, just as a request at
// set-community naming method REFERENCE does.
func Test_openconfig_routing_policy_statement_actions_bgp_set_community_refs(t *testing.T) {
	var urlInputBodyJSON string

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
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": "",
			"CS2": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	setCommUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]" +
		"/statements/statement[name=10]/actions/bgp-actions/set-community"
	refCfgUrl := setCommUrl + "/reference/config"

	storedRef := func(name string) map[string]interface{} {
		return map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{
					// Writing a reference clears the inline spelling, which
					// leaves that field present and empty rather than absent.
					"set_community_inline": "",
					"set_community_ref":    name,
				},
			},
		}
	}

	// The leaf-list is accepted wherever the singular leaf is, starting with a
	// payload sent at set-community naming the REFERENCE method.
	urlInputBodyJSON = `{"config":{"method":"REFERENCE"},"reference":{"config":{"community-set-refs":["CS1"]}}}`
	t.Run("PATCH set-community with community-set-refs",
		processSetRequest(setCommUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the leaf-list reached set_community_ref",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS1"), false))
	time.Sleep(1 * time.Second)

	// Both spellings come back, so a client written against either revision of
	// the model finds the one it knows.
	t.Run("GET the reference config reports both spellings",
		processGetRequest(refCfgUrl, nil,
			`{"openconfig-bgp-policy:config":{"community-set-ref":"CS1","community-set-refs":["CS1"]}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET the community-set-refs leaf-list on its own",
		processGetRequest(refCfgUrl+"/community-set-refs", nil,
			`{"openconfig-bgp-policy:community-set-refs":["CS1"]}`, false))
	time.Sleep(1 * time.Second)

	// A request anchored on the reference config writes the same field, with no
	// method leaf needed alongside it.
	urlInputBodyJSON = `{"openconfig-bgp-policy:community-set-refs":["CS2"]}`
	t.Run("PATCH the reference config directly",
		processSetRequest(refCfgUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the direct write replaced the stored reference",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS2"), false))
	time.Sleep(1 * time.Second)

	// A second reference has nowhere to be stored.
	expectedErr := tlerr.NotSupportedError{
		Format: "routing-policy statement actions bgp-actions set-community reference config community-set-refs carries 2 references; ROUTE_MAP set_community_ref holds one community-set, so exactly one is supported",
	}
	urlInputBodyJSON = `{"openconfig-bgp-policy:community-set-refs":["CS1","CS2"]}`
	t.Run("PATCH two references rejected",
		processSetRequest(refCfgUrl, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify the refused list left the stored reference alone",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS2"), false))
	time.Sleep(1 * time.Second)

	// Both spellings map to the one field, so a payload naming a different set in
	// each has no answer that is not a guess.
	disagreeErr := tlerr.InvalidArgsError{
		Format: "routing-policy statement actions bgp-actions set-community reference config names CS1 in community-set-refs and CS2 in community-set-ref; both map to set_community_ref, so they have to agree",
	}
	urlInputBodyJSON = `{"openconfig-bgp-policy:community-set-refs":["CS1"],"openconfig-bgp-policy:community-set-ref":"CS2"}`
	t.Run("PATCH disagreeing spellings rejected",
		processSetRequest(refCfgUrl, urlInputBodyJSON, "PATCH", true, disagreeErr))
	time.Sleep(1 * time.Second)

	// Agreeing is accepted, since there is only one value to store.
	urlInputBodyJSON = `{"openconfig-bgp-policy:community-set-refs":["CS1"],"openconfig-bgp-policy:community-set-ref":"CS1"}`
	t.Run("PATCH agreeing spellings accepted",
		processSetRequest(refCfgUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify agreeing spellings stored the one reference",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS1"), false))
	time.Sleep(1 * time.Second)

	// The deprecated spelling still writes on its own, at the new anchor as well.
	urlInputBodyJSON = `{"openconfig-bgp-policy:community-set-ref":"CS2"}`
	t.Run("PATCH the deprecated singular leaf still works",
		processSetRequest(refCfgUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the deprecated spelling stored its reference",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedRef("CS2"), false))
	time.Sleep(1 * time.Second)

	// Both spellings are the one field, so deleting the reference config clears
	// it whichever spelling put it there.
	t.Run("DELETE the reference config", processDeleteRequest(refCfgUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the reference was cleared",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{
					"set_community_inline": "",
				},
			},
		}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// options at set-community/config is served for the two forms CONFIG_DB can
// carry. ROUTE_MAP has no field of its own for options: frrcfgd renders
// set_community_inline straight into "set community {}", so ADD is the additive
// marker on that value and REPLACE is its absence. A request that changes
// options alone therefore re-encodes the communities already stored. REMOVE is
// refused rather than approximated, because FRR removes communities only
// through "set comm-list <name> delete" and neither ROUTE_MAP nor frrcfgd has a
// field for that.
func Test_openconfig_routing_policy_statement_actions_bgp_set_community_options(t *testing.T) {
	var urlInputBodyJSON string

	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{"NULL": "NULL"},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"set_community_inline": "65001:100",
			},
			// A statement with no communities, to show there is nothing for the
			// marker to sit on.
			"POL1|20": map[string]interface{}{"NULL": "NULL"},
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
		"COMMUNITY_SET": map[string]interface{}{
			"CS1": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	setCommUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]" +
		"/statements/statement[name=10]/actions/bgp-actions/set-community"
	cfgUrl := setCommUrl + "/config"
	optionsUrl := cfgUrl + "/options"

	storedInline := func(value string) map[string]interface{} {
		return map[string]interface{}{
			"ROUTE_MAP": map[string]interface{}{
				"POL1|10": map[string]interface{}{
					"set_community_inline": value,
				},
			},
		}
	}

	// A stored value with no marker renders as "set community 65001:100", which
	// replaces the attribute, so it reads back as REPLACE rather than as no
	// options at all.
	t.Run("GET options reports REPLACE for an unmarked value",
		processGetRequest(optionsUrl, nil, `{"openconfig-bgp-policy:options":"REPLACE"}`, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = `{"openconfig-bgp-policy:options":"ADD"}`
	t.Run("PATCH options ADD", processSetRequest(optionsUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify ADD marked the communities already stored",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:100 additive"), false))
	time.Sleep(1 * time.Second)

	t.Run("GET options reports ADD",
		processGetRequest(optionsUrl, nil, `{"openconfig-bgp-policy:options":"ADD"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET the config container reports the method alongside options",
		processGetRequest(cfgUrl, nil,
			`{"openconfig-bgp-policy:config":{"method":"INLINE","options":"ADD"}}`, false))
	time.Sleep(1 * time.Second)

	urlInputBodyJSON = `{"openconfig-bgp-policy:options":"REPLACE"}`
	t.Run("PATCH options REPLACE", processSetRequest(optionsUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify REPLACE took the marker off",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:100"), false))
	time.Sleep(1 * time.Second)

	removeErr := tlerr.NotSupportedError{
		Format: `routing-policy statement actions bgp-actions set-community config options REMOVE is not supported; FRR removes communities only through "set comm-list <name> delete", which ROUTE_MAP has no field for`,
	}
	urlInputBodyJSON = `{"openconfig-bgp-policy:options":"REMOVE"}`
	t.Run("PATCH options REMOVE rejected",
		processSetRequest(optionsUrl, urlInputBodyJSON, "PATCH", true, removeErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify the refused REMOVE left the stored value alone",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:100"), false))
	time.Sleep(1 * time.Second)

	// options also arrives at the container, alongside the method and the
	// communities it applies to.
	urlInputBodyJSON = `{"config":{"method":"INLINE","options":"ADD"},"inline":{"config":{"communities":["65001:200"]}}}`
	t.Run("PATCH set-community with options ADD",
		processSetRequest(setCommUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the container write marked its own communities",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:200 additive"), false))
	time.Sleep(1 * time.Second)

	// The inline config carries no options of its own, so a write there keeps
	// the marker rather than silently turning an ADD into a REPLACE.
	urlInputBodyJSON = `{"communities":["65001:300"]}`
	t.Run("PATCH the inline config on its own",
		processSetRequest(setCommUrl+"/inline/config", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the inline write kept the marker",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:300 additive"), false))
	time.Sleep(1 * time.Second)

	// Dropping options keeps the communities and takes the marker off them,
	// which is the REPLACE form.
	t.Run("DELETE options", processDeleteRequest(optionsUrl, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify the delete left the communities unmarked",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:300"), false))
	time.Sleep(1 * time.Second)

	// The stored value is the single argument frrcfgd hands to
	// "set community {}", so several communities are space separated with one
	// trailing marker, and a well-known community is written as the keyword FRR
	// knows it by. A comma, or a marker repeated per value, is malformed to FRR
	// and leaves the route-map with no set clause at all.
	urlInputBodyJSON = `{"config":{"method":"INLINE","options":"ADD"},"inline":{"config":{"communities":["65001:100","NO_EXPORT"]}}}`
	t.Run("PATCH several communities including a well-known one",
		processSetRequest(setCommUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the stored value is the form FRR accepts",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:100 no-export additive"), false))
	time.Sleep(1 * time.Second)

	// The keyword goes back out as the identity name the model uses.
	t.Run("GET the inline config reports the identity name",
		processGetRequest(setCommUrl+"/inline/config", nil,
			`{"openconfig-bgp-policy:config":{"communities":["65001:100","NO_EXPORT"]}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET options reports ADD alongside several communities",
		processGetRequest(optionsUrl, nil, `{"openconfig-bgp-policy:options":"ADD"}`, false))
	time.Sleep(1 * time.Second)

	// A row written before the encoding was corrected holds a comma separated
	// list with the marker repeated on every value, and still reads back.
	loadDB(db.ConfigDB, map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{
				"set_community_inline": "65001:100 additive, no-export additive",
			},
		},
	})
	time.Sleep(1 * time.Second)

	t.Run("GET a row in the superseded encoding still reports its communities",
		processGetRequest(setCommUrl+"/inline/config", nil,
			`{"openconfig-bgp-policy:config":{"communities":["65001:100","NO_EXPORT"]}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET a row in the superseded encoding still reports ADD",
		processGetRequest(optionsUrl, nil, `{"openconfig-bgp-policy:options":"ADD"}`, false))
	time.Sleep(1 * time.Second)

	// Rewriting options normalises that row onto the current encoding.
	urlInputBodyJSON = `{"openconfig-bgp-policy:options":"REPLACE"}`
	t.Run("PATCH options REPLACE over the superseded encoding",
		processSetRequest(optionsUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify the rewrite normalised the stored value",
		verifyDbResult(rclient, "ROUTE_MAP|POL1|10", storedInline("65001:100 no-export"), false))
	time.Sleep(1 * time.Second)

	// method is inferred from whichever field holds a value, so a write at that
	// leaf has nothing to change.
	methodErr := tlerr.NotSupportedError{
		Format: "SET is not supported for set-community config method; ROUTE_MAP has no method field and a GET reports the method of whichever of set_community_inline and set_community_ref holds a value, so write the communities or the reference instead",
	}
	urlInputBodyJSON = `{"openconfig-bgp-policy:method":"INLINE"}`
	t.Run("PATCH the method leaf rejected",
		processSetRequest(cfgUrl+"/method", urlInputBodyJSON, "PATCH", true, methodErr))
	time.Sleep(1 * time.Second)

	emptyErr := tlerr.InvalidArgsError{
		Format: "routing-policy statement actions bgp-actions set-community config options is stored as a marker on the inline communities, so the communities have to be configured before options",
	}
	emptyOptionsUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]" +
		"/statements/statement[name=20]/actions/bgp-actions/set-community/config/options"
	urlInputBodyJSON = `{"openconfig-bgp-policy:options":"ADD"}`
	t.Run("PATCH options with no communities stored rejected",
		processSetRequest(emptyOptionsUrl, urlInputBodyJSON, "PATCH", true, emptyErr))
	time.Sleep(1 * time.Second)

	// Under REFERENCE there is no field to carry options at all, so it is
	// refused rather than accepted and dropped.
	urlInputBodyJSON = `{"config":{"method":"REFERENCE"},"reference":{"config":{"community-set-refs":["CS1"]}}}`
	t.Run("PATCH set-community naming a reference",
		processSetRequest(setCommUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	refErr := tlerr.NotSupportedError{
		Format: "routing-policy statement actions bgp-actions set-community config options is not supported with the REFERENCE method; ROUTE_MAP records the referenced community-set in set_community_ref and has no field to carry options alongside it",
	}
	urlInputBodyJSON = `{"openconfig-bgp-policy:options":"ADD"}`
	t.Run("PATCH options under the REFERENCE method rejected",
		processSetRequest(optionsUrl, urlInputBodyJSON, "PATCH", true, refErr))
	time.Sleep(1 * time.Second)

	t.Run("GET the config container reports no options under REFERENCE",
		processGetRequest(cfgUrl, nil, `{"openconfig-bgp-policy:config":{"method":"REFERENCE"}}`, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// The state container under a statement's actions has no datastore of its own:
// it reports the same ROUTE_MAP route_operation the config container writes. It
// is read-only, so a write naming it is refused, and it is reported at the
// levels above it as well as at its own leaf.
func Test_openconfig_routing_policy_policy_definition_statement_actions_state(t *testing.T) {
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP":     map[string]interface{}{"POLST|10": ""},
		"ROUTE_MAP_SET": map[string]interface{}{"POLST": ""},
	}
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{"POLST": map[string]interface{}{"NULL": "NULL"}},
		"ROUTE_MAP": map[string]interface{}{
			"POLST|10": map[string]interface{}{"route_operation": "deny"},
		},
	}
	// The applied row is what state reports, so the fixture writes what bgpmon
	// would have scraped out of FRR. It is deliberately the same value as the
	// configured one here; the divergence case is asserted further down.
	appliedTbl := map[string]interface{}{
		"ROUTE_MAP_APPLIED": map[string]interface{}{"POLST|10": ""},
	}
	appliedMap := map[string]interface{}{
		"ROUTE_MAP_APPLIED": map[string]interface{}{
			"POLST|10": map[string]interface{}{"route_operation": "deny"},
		},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
	loadDB(db.ConfigDB, preReqMap)
	loadDB(db.StateDB, appliedMap)
	time.Sleep(1 * time.Second)

	stmtUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POLST]" +
		"/statements/statement[name=10]"

	expectedGetJSON := `{"openconfig-routing-policy:policy-result":"REJECT_ROUTE"}`
	t.Run("GET the state policy-result leaf",
		processGetRequest(stmtUrl+"/actions/state/policy-result", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:state":{"policy-result":"REJECT_ROUTE"}}`
	t.Run("GET the state container",
		processGetRequest(stmtUrl+"/actions/state", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// A read of the enclosing statement reports both halves, which is the point
	// of serving state at all: the level above is not left half empty. A uri
	// terminating at `actions` is a separate, pre-existing gap: no parse pattern
	// matches it, so it answers empty at HEAD too and is left for the
	// container-level work.
	expectedGetJSON = `{"openconfig-routing-policy:statement":[{"actions":{"config":{"policy-result":"REJECT_ROUTE"},` +
		`"state":{"policy-result":"REJECT_ROUTE"}},"config":{"name":"10"},"name":"10","state":{"name":"10"}}]}`
	t.Run("GET the statement reports config and state",
		processGetRequest(stmtUrl, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// state is config false, so the request binder refuses a write to it before
	// any transformer is reached. That message, not ours, is what a client sees,
	// and it is the one worth pinning.
	expectedErr := tlerr.TranslibSyntaxValidationError{
		ErrorStr: errors.New("SET operation not allowed on the read-only object"),
	}
	t.Run("PATCH the state leaf is refused",
		processSetRequest(stmtUrl+"/actions/state/policy-result",
			`{"openconfig-routing-policy:policy-result":"ACCEPT_ROUTE"}`, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE the state container is refused",
		processDeleteRequest(stmtUrl+"/actions/state", true, expectedErr))
	time.Sleep(1 * time.Second)

	// A container payload carrying state is refused by the same check, which is
	// what makes the handler's own state guard unreachable rather than merely
	// redundant.
	t.Run("PATCH the actions container carrying state is refused",
		processSetRequest(stmtUrl+"/actions",
			`{"openconfig-routing-policy:state":{"policy-result":"ACCEPT_ROUTE"}}`,
			"PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// The refused writes must leave the row exactly as it was.
	verifyMap := map[string]interface{}{
		"ROUTE_MAP": map[string]interface{}{
			"POLST|10": map[string]interface{}{"route_operation": "deny"},
		},
	}
	t.Run("Verify the refused writes left route_operation alone",
		verifyDbResult(rclient, "ROUTE_MAP|POLST|10", verifyMap, false))
	time.Sleep(1 * time.Second)

	// Config and state are now two sources, so a config write alone does not
	// move state. That is the point of an applied view: it reports what FRR
	// took, and until the scrape catches up the two legitimately disagree.
	t.Run("PATCH the config leaf",
		processSetRequest(stmtUrl+"/actions/config/policy-result",
			`{"openconfig-routing-policy:policy-result":"ACCEPT_ROUTE"}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:policy-result":"ACCEPT_ROUTE"}`
	t.Run("GET the config leaf shows the write",
		processGetRequest(stmtUrl+"/actions/config/policy-result", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:policy-result":"REJECT_ROUTE"}`
	t.Run("GET the state leaf still reports what FRR applied",
		processGetRequest(stmtUrl+"/actions/state/policy-result", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// And when the scrape catches up, state follows it rather than config.
	loadDB(db.StateDB, map[string]interface{}{
		"ROUTE_MAP_APPLIED": map[string]interface{}{
			"POLST|10": map[string]interface{}{"route_operation": "permit"},
		},
	})
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-routing-policy:policy-result":"ACCEPT_ROUTE"}`
	t.Run("GET the state leaf follows the applied row",
		processGetRequest(stmtUrl+"/actions/state/policy-result", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// A statement FRR has not taken reports no state at all, rather than
	// echoing the configuration back and calling it applied.
	unloadDB(db.StateDB, appliedTbl)
	time.Sleep(1 * time.Second)
	t.Run("GET the state container reports nothing when FRR has not taken it",
		processGetRequest(stmtUrl+"/actions/state", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
}

// bgp-actions/state reports what FRR applied, decoded from the same field
// names CONFIG_DB uses, which is what lets the applied row be read by the same
// logic as the configured one. A statement FRR has not taken reports nothing.
func Test_openconfig_routing_policy_statement_bgp_actions_state(t *testing.T) {
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP":     map[string]interface{}{"POLBA|10": ""},
		"ROUTE_MAP_SET": map[string]interface{}{"POLBA": ""},
	}
	appliedTbl := map[string]interface{}{
		"ROUTE_MAP_APPLIED": map[string]interface{}{"POLBA|10": ""},
	}
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{"POLBA": map[string]interface{}{"NULL": "NULL"}},
		"ROUTE_MAP": map[string]interface{}{
			"POLBA|10": map[string]interface{}{"route_operation": "permit"},
		},
	}
	// What bgpmon would have scraped: FRR took a local-pref, an origin, a
	// next-hop and a MED for this statement.
	appliedMap := map[string]interface{}{
		"ROUTE_MAP_APPLIED": map[string]interface{}{
			"POLBA|10": map[string]interface{}{
				"route_operation": "permit",
				"set_local_pref":  "250",
				"set_origin":      "egp",
				"set_next_hop":    "10.9.9.9",
				"set_med":         "400",
			},
		},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
	loadDB(db.ConfigDB, preReqMap)
	loadDB(db.StateDB, appliedMap)
	time.Sleep(1 * time.Second)

	baUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POLBA]" +
		"/statements/statement[name=10]/actions/openconfig-bgp-policy:bgp-actions"

	t.Run("GET state/set-local-pref",
		processGetRequest(baUrl+"/state/set-local-pref", nil,
			`{"openconfig-bgp-policy:set-local-pref":250}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET state/set-route-origin",
		processGetRequest(baUrl+"/state/set-route-origin", nil,
			`{"openconfig-bgp-policy:set-route-origin":"EGP"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET state/set-next-hop",
		processGetRequest(baUrl+"/state/set-next-hop", nil,
			`{"openconfig-bgp-policy:set-next-hop":"10.9.9.9"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET state/set-med",
		processGetRequest(baUrl+"/state/set-med", nil,
			`{"openconfig-bgp-policy:set-med":400}`, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-bgp-policy:state":{"set-local-pref":250,"set-med":400,` +
		`"set-next-hop":"10.9.9.9","set-route-origin":"EGP"}}`
	t.Run("GET the whole state container",
		processGetRequest(baUrl+"/state", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The configured row says nothing but permit, so config and state differ
	// here exactly as an applied view should allow.
	t.Run("GET the config container carries none of it",
		processGetRequest(baUrl+"/config", nil, "{}", false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.TranslibSyntaxValidationError{
		ErrorStr: errors.New("SET operation not allowed on the read-only object"),
	}
	t.Run("PATCH state/set-local-pref is refused",
		processSetRequest(baUrl+"/state/set-local-pref",
			`{"openconfig-bgp-policy:set-local-pref":300}`, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// Nothing applied means nothing reported, rather than the configuration
	// echoed back under a state path.
	unloadDB(db.StateDB, appliedTbl)
	time.Sleep(1 * time.Second)
	t.Run("GET the state container reports nothing when FRR has not taken it",
		processGetRequest(baUrl+"/state", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
}

// The rest of the statement's state surface, all read from the applied row:
// the as-path prepend, the community settings and the match conditions. The
// configured row is deliberately left bare, so anything reported here came
// from what FRR applied rather than from CONFIG_DB.
func Test_openconfig_routing_policy_statement_state_from_applied(t *testing.T) {
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP":     map[string]interface{}{"POLAP|10": ""},
		"ROUTE_MAP_SET": map[string]interface{}{"POLAP": ""},
	}
	appliedTbl := map[string]interface{}{
		"ROUTE_MAP_APPLIED": map[string]interface{}{"POLAP|10": ""},
	}
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{"POLAP": map[string]interface{}{"NULL": "NULL"}},
		"ROUTE_MAP": map[string]interface{}{
			"POLAP|10": map[string]interface{}{"route_operation": "permit"},
		},
	}
	appliedMap := map[string]interface{}{
		"ROUTE_MAP_APPLIED": map[string]interface{}{
			"POLAP|10": map[string]interface{}{
				"route_operation":      "permit",
				"set_asn":              "65010",
				"set_repeat_asn":       "3",
				"set_community_inline": "65001:100",
				"match_prefix_set":     "PSETV4",
				"match_med":            "100",
				"match_local_pref":     "120",
			},
		},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
	loadDB(db.ConfigDB, preReqMap)
	loadDB(db.StateDB, appliedMap)
	time.Sleep(1 * time.Second)

	stmt := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POLAP]" +
		"/statements/statement[name=10]"
	bgpAct := stmt + "/actions/openconfig-bgp-policy:bgp-actions"

	t.Run("GET set-as-path-prepend state/asn",
		processGetRequest(bgpAct+"/set-as-path-prepend/state/asn", nil,
			`{"openconfig-bgp-policy:asn":65010}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET set-as-path-prepend state/repeat-n",
		processGetRequest(bgpAct+"/set-as-path-prepend/state/repeat-n", nil,
			`{"openconfig-bgp-policy:repeat-n":3}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET match-prefix-set state/prefix-set",
		processGetRequest(stmt+"/conditions/match-prefix-set/state/prefix-set", nil,
			`{"openconfig-routing-policy:prefix-set":"PSETV4"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET bgp-conditions state/med-eq",
		processGetRequest(stmt+"/conditions/openconfig-bgp-policy:bgp-conditions/state/med-eq", nil,
			`{"openconfig-bgp-policy:med-eq":100}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET bgp-conditions state/local-pref-eq",
		processGetRequest(stmt+"/conditions/openconfig-bgp-policy:bgp-conditions/state/local-pref-eq", nil,
			`{"openconfig-bgp-policy:local-pref-eq":120}`, false))
	time.Sleep(1 * time.Second)

	// Nothing applied, nothing reported.
	unloadDB(db.StateDB, appliedTbl)
	time.Sleep(1 * time.Second)
	t.Run("GET set-as-path-prepend state reports nothing once the row is gone",
		processGetRequest(bgpAct+"/set-as-path-prepend/state", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
}

// A statement sets one next-hop, which ROUTE_MAP spells by family: an IPv4
// address or self in set_next_hop, an IPv6 address in set_ipv6_next_hop_global.
// frrcfgd renders the first as "set ip next-hop", so an IPv6 address written
// there is rejected by FRR and never applied. A write of one family replaces
// the other, by update and by replace, and the rest of the row is kept.
func Test_openconfig_routing_policy_bgp_actions_set_next_hop_families(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{"POLNH": map[string]interface{}{}},
		"ROUTE_MAP": map[string]interface{}{
			"POLNH|10": map[string]interface{}{"route_operation": "permit", "set_local_pref": "200"},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP":     map[string]interface{}{"POLNH|10": ""},
		"ROUTE_MAP_SET": map[string]interface{}{"POLNH": ""},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POLNH]" +
		"/statements/statement[name=10]/actions/openconfig-bgp-policy:bgp-actions/config/set-next-hop"
	row := func(nhFld, nh string) map[string]interface{} {
		return map[string]interface{}{"ROUTE_MAP": map[string]interface{}{"POLNH|10": map[string]interface{}{
			"route_operation": "permit", "set_local_pref": "200", nhFld: nh}}}
	}
	body := func(v string) string { return `{"openconfig-bgp-policy:set-next-hop":"` + v + `"}` }

	t.Run("PATCH set-next-hop IPv6", processSetRequest(url, body("2001:db8::1"), "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify IPv6 next-hop in set_ipv6_next_hop_global",
		verifyDbResult(rclient, "ROUTE_MAP|POLNH|10", row("set_ipv6_next_hop_global", "2001:db8::1"), false))
	t.Run("GET set-next-hop IPv6", processGetRequest(url, nil, `{"openconfig-bgp-policy:set-next-hop":"2001:db8::1"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH set-next-hop IPv4 over IPv6", processSetRequest(url, body("10.10.10.1"), "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify IPv4 next-hop replaced the IPv6 one",
		verifyDbResult(rclient, "ROUTE_MAP|POLNH|10", row("set_next_hop", "10.10.10.1"), false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH set-next-hop SELF", processSetRequest(url, body("SELF"), "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify SELF stored as self", verifyDbResult(rclient, "ROUTE_MAP|POLNH|10", row("set_next_hop", "self"), false))
	t.Run("GET set-next-hop SELF", processGetRequest(url, nil, `{"openconfig-bgp-policy:set-next-hop":"SELF"}`, false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH set-next-hop IPv6 over SELF", processSetRequest(url, body("2001:db8::2"), "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("PUT set-next-hop IPv4 over IPv6", processSetRequest(url, body("10.10.10.2"), "PUT", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the replace dropped the IPv6 next-hop",
		verifyDbResult(rclient, "ROUTE_MAP|POLNH|10", row("set_next_hop", "10.10.10.2"), false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH set-next-hop that is not an address refused", processSetRequest(url, body("not-an-ip"), "PATCH", true))
	time.Sleep(1 * time.Second)
	t.Run("Verify the refused write left the row alone",
		verifyDbResult(rclient, "ROUTE_MAP|POLNH|10", row("set_next_hop", "10.10.10.2"), false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH set-next-hop IPv6 before delete", processSetRequest(url, body("2001:db8::3"), "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("DELETE set-next-hop", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)
	t.Run("Verify both next-hop fields cleared", verifyDbResult(rclient, "ROUTE_MAP|POLNH|10",
		map[string]interface{}{"ROUTE_MAP": map[string]interface{}{"POLNH|10": map[string]interface{}{
			"route_operation": "permit", "set_local_pref": "200"}}}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// Policy definitions, statements and the defined sets have no operational source
// of their own, so their state containers report what is configured.
func Test_openconfig_routing_policy_state_mirrors_config(t *testing.T) {
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET": map[string]interface{}{
			"POL1": map[string]interface{}{"NULL": "NULL"},
		},
		"ROUTE_MAP": map[string]interface{}{
			"POL1|10": map[string]interface{}{"route_operation": "permit"},
		},
		"PREFIX_SET": map[string]interface{}{
			"PS1": map[string]interface{}{"mode": "IPv4"},
		},
		"PREFIX": map[string]interface{}{
			"PS1|5|10.11.0.0/16|exact": map[string]interface{}{"action": "permit"},
		},
		"COMMUNITY_SET": map[string]interface{}{
			"COMM1": map[string]interface{}{
				"set_type":          "STANDARD",
				"match_action":      "ALL",
				"community_member@": "65100:100,no-export",
			},
		},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{
			"EXTCOMM1": map[string]interface{}{
				"set_type":          "STANDARD",
				"match_action":      "ANY",
				"community_member@": "route-target:65100:100",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP":              map[string]interface{}{"POL1|10": ""},
		"ROUTE_MAP_SET":          map[string]interface{}{"POL1": ""},
		"PREFIX":                 map[string]interface{}{"PS1|5|10.11.0.0/16|exact": ""},
		"PREFIX_SET":             map[string]interface{}{"PS1": ""},
		"COMMUNITY_SET":          map[string]interface{}{"COMM1": ""},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{"EXTCOMM1": ""},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	pdUrl := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=POL1]"
	stmtUrl := pdUrl + "/statements/statement[name=10]"
	psUrl := "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=PS1]"
	pfxUrl := psUrl + "/prefixes/prefix[ip-prefix=10.11.0.0/16][masklength-range=exact]"
	bgpSetsUrl := "/openconfig-routing-policy:routing-policy/defined-sets/bgp-defined-sets"
	csUrl := bgpSetsUrl + "/community-sets/community-set[community-set-name=COMM1]"
	ecsUrl := bgpSetsUrl + "/ext-community-sets/ext-community-set[ext-community-set-name=EXTCOMM1]"

	t.Run("GET routing-policy policy-definition state name",
		processGetRequest(pdUrl+"/state/name", nil, `{"openconfig-routing-policy:name":"POL1"}`, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy policy-definition state container",
		processGetRequest(pdUrl+"/state", nil, `{"openconfig-routing-policy:state":{"name":"POL1"}}`, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy statement state name",
		processGetRequest(stmtUrl+"/state/name", nil, `{"openconfig-routing-policy:name":"10"}`, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy prefix-set state container",
		processGetRequest(psUrl+"/state", nil, `{"openconfig-routing-policy:state":{"mode":"IPV4","name":"PS1"}}`, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy prefix state container",
		processGetRequest(pfxUrl+"/state", nil,
			`{"openconfig-routing-policy:state":{"ip-prefix":"10.11.0.0/16","masklength-range":"exact"}}`, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy community-set state container",
		processGetRequest(csUrl+"/state", nil,
			`{"openconfig-bgp-policy:state":{"community-member":["65100:100","openconfig-bgp-types:NO_EXPORT"],`+
				`"community-set-name":"COMM1","match-set-options":"ALL"}}`, false))
	time.Sleep(1 * time.Second)
	t.Run("GET routing-policy ext-community-set state ext-community-member",
		processGetRequest(ecsUrl+"/state/ext-community-member", nil,
			`{"openconfig-bgp-policy:ext-community-member":["route-target:65100:100"]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("GET routing-policy prefix-set list instance carries state at every level",
		processGetRequest(psUrl, nil,
			`{"openconfig-routing-policy:prefix-set":[{"config":{"mode":"IPV4","name":"PS1"},"name":"PS1",`+
				`"prefixes":{"prefix":[{"config":{"ip-prefix":"10.11.0.0/16","masklength-range":"exact"},`+
				`"ip-prefix":"10.11.0.0/16","masklength-range":"exact",`+
				`"state":{"ip-prefix":"10.11.0.0/16","masklength-range":"exact"}}]},`+
				`"state":{"mode":"IPV4","name":"PS1"}}]}`, false))
	time.Sleep(1 * time.Second)

	// A state leaf with no CONFIG_DB mapping is not found rather than empty.
	t.Run("GET routing-policy statement conditions state call-policy is not found",
		processGetRequest(stmtUrl+"/conditions/state/call-policy", nil, "", true, tlerr.NotFoundError{Format: "Resource not found"}))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// AS path sets, the statement conditions call-policy, install-protocol-eq,
// match-interface, match-as-path-set and the match-community-set containers,
// and set-ext-community, each in config and in the state FRR applies.
func Test_openconfig_routing_policy_as_path_sets_conditions_and_set_ext_community(t *testing.T) {
	cleanupTbl := map[string]interface{}{
		"ROUTE_MAP":              map[string]interface{}{"POLVI|10": ""},
		"ROUTE_MAP_SET":          map[string]interface{}{"POLVI": "", "POLVIC": ""},
		"AS_PATH_SET":            map[string]interface{}{"ASP1": ""},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{"ECVI": ""},
		"PORT":                   map[string]interface{}{"Ethernet120": ""},
	}
	appliedTbl := map[string]interface{}{"ROUTE_MAP_APPLIED": map[string]interface{}{"POLVI|10": ""}}
	preReqMap := map[string]interface{}{
		"ROUTE_MAP_SET":          map[string]interface{}{"POLVI": map[string]interface{}{"NULL": "NULL"}, "POLVIC": map[string]interface{}{"NULL": "NULL"}},
		"ROUTE_MAP":              map[string]interface{}{"POLVI|10": map[string]interface{}{"route_operation": "permit"}},
		"EXTENDED_COMMUNITY_SET": map[string]interface{}{"ECVI": map[string]interface{}{"set_type": "standard", "match_action": "any", "community_member@": "route-target:9:9"}},
		"PORT":                   map[string]interface{}{"Ethernet120": map[string]interface{}{"admin_status": "up"}},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	asp := rpolContainerUrl + "/defined-sets/openconfig-bgp-policy:bgp-defined-sets/openconfig-bgp-policy:as-path-sets/openconfig-bgp-policy:as-path-set[as-path-set-name=ASP1]"
	stmt := rpolContainerUrl + "/policy-definitions/policy-definition[name=POLVI]/statements/statement[name=10]"
	bgpCond := stmt + "/conditions/openconfig-bgp-policy:bgp-conditions"
	sec := stmt + "/actions/openconfig-bgp-policy:bgp-actions/openconfig-bgp-policy:set-ext-community"

	t.Run("PATCH as-path-set config", processSetRequest(asp+"/config",
		`{"as-path-set-name":"ASP1","as-path-set-member":["^65001_","_65002$"]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify AS_PATH_SET", verifyDbResult(rclient, "AS_PATH_SET|ASP1",
		map[string]interface{}{"AS_PATH_SET": map[string]interface{}{"ASP1": map[string]interface{}{"as_path_set_member@": "^65001_,_65002$"}}}, false))
	time.Sleep(1 * time.Second)
	t.Run("GET as-path-set", processGetRequest(asp, nil, `{"openconfig-bgp-policy:as-path-set":[{"as-path-set-name":"ASP1",`+
		`"config":{"as-path-set-member":["^65001_","_65002$"],"as-path-set-name":"ASP1"},`+
		`"state":{"as-path-set-member":["^65001_","_65002$"],"as-path-set-name":"ASP1"}}]}`, false))
	time.Sleep(1 * time.Second)

	for _, c := range []struct{ name, url, body string }{
		{"conditions config", stmt + "/conditions/config", `{"call-policy":"POLVIC","install-protocol-eq":"BGP"}`},
		{"match-interface config", stmt + "/conditions/match-interface/config", `{"interface":"Ethernet120"}`},
		{"match-as-path-set config", bgpCond + "/match-as-path-set/config", `{"as-path-set":"ASP1","match-set-options":"ANY"}`},
		{"match-ext-community-set config", bgpCond + "/match-ext-community-set/config", `{"ext-community-set":"ECVI"}`},
		{"set-ext-community inline", sec, `{"config":{"method":"INLINE","options":"REPLACE"},` +
			`"inline":{"config":{"communities":["route-target:2:2","route-origin:3:3"]}}}`},
	} {
		t.Run("PATCH "+c.name, processSetRequest(c.url, c.body, "PATCH", false, nil))
		time.Sleep(1 * time.Second)
	}
	t.Run("Verify ROUTE_MAP", verifyDbResult(rclient, "ROUTE_MAP|POLVI|10", map[string]interface{}{"ROUTE_MAP": map[string]interface{}{"POLVI|10": map[string]interface{}{
		"route_operation": "permit", "call_route_map": "POLVIC", "match_protocol": "bgp", "match_interface": "Ethernet120",
		"match_as_path": "ASP1", "match_ext_community": "ECVI", "set_ext_community_inline@": "route-target:2:2,route-origin:3:3"}}}, false))
	time.Sleep(1 * time.Second)

	for _, c := range []struct{ name, url, want string }{
		{"conditions config", stmt + "/conditions/config", `{"openconfig-routing-policy:config":{"call-policy":"POLVIC","install-protocol-eq":"openconfig-policy-types:BGP"}}`},
		{"match-interface config interface", stmt + "/conditions/match-interface/config/interface", `{"openconfig-routing-policy:interface":"Ethernet120"}`},
		{"match-as-path-set config", bgpCond + "/match-as-path-set/config", `{"openconfig-bgp-policy:config":{"as-path-set":"ASP1"}}`},
		{"set-ext-community config", sec + "/config", `{"openconfig-bgp-policy:config":{"method":"INLINE"}}`},
		{"set-ext-community inline config", sec + "/inline/config", `{"openconfig-bgp-policy:config":{"communities":["route-target:2:2","route-origin:3:3"]}}`},
	} {
		t.Run("GET "+c.name, processGetRequest(c.url, nil, c.want, false))
		time.Sleep(1 * time.Second)
	}

	t.Run("GET statement conditions container", processGetRequest(stmt+"/conditions", nil,
		`{"openconfig-routing-policy:conditions":{"config":{"call-policy":"POLVIC","install-protocol-eq":"openconfig-policy-types:BGP"},`+
			`"match-interface":{"config":{"interface":"Ethernet120"}},"openconfig-bgp-policy:bgp-conditions":{"config":{"ext-community-set":"ECVI"},`+
			`"match-as-path-set":{"config":{"as-path-set":"ASP1"}},"match-ext-community-set":{"config":{"ext-community-set":"ECVI"}}}}}`, false))
	time.Sleep(1 * time.Second)

	for _, c := range []struct{ name, url, body string }{
		{"match-set-options ALL", bgpCond + "/match-as-path-set/config", `{"as-path-set":"ASP1","match-set-options":"ALL"}`},
		{"install-protocol-eq OSPF", stmt + "/conditions/config", `{"install-protocol-eq":"OSPF"}`},
		{"match-interface subinterface", stmt + "/conditions/match-interface/config", `{"interface":"Ethernet120","subinterface":1}`},
		{"set-ext-community options ADD", sec + "/config", `{"options":"ADD"}`},
	} {
		t.Run("PATCH "+c.name+" is refused", processSetRequest(c.url, c.body, "PATCH", true))
		time.Sleep(1 * time.Second)
	}

	t.Run("PATCH set-ext-community reference", processSetRequest(sec+"/reference/config",
		`{"ext-community-set-refs":["ECVI"]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("GET set-ext-community config after the switch", processGetRequest(sec+"/config", nil, `{"openconfig-bgp-policy:config":{"method":"REFERENCE"}}`, false))
	time.Sleep(1 * time.Second)

	appliedMap := map[string]interface{}{"ROUTE_MAP_APPLIED": map[string]interface{}{"POLVI|10": map[string]interface{}{
		"route_operation": "permit", "call_route_map": "POLVIC", "match_protocol": "bgp", "match_interface": "Ethernet120",
		"match_as_path": "ASP1", "set_ext_community_inline": "route-target:9:9"}}}
	loadDB(db.StateDB, appliedMap)
	time.Sleep(1 * time.Second)
	for _, c := range []struct{ name, url, want string }{
		{"conditions state", stmt + "/conditions/state", `{"openconfig-routing-policy:state":{"call-policy":"POLVIC","install-protocol-eq":"openconfig-policy-types:BGP"}}`},
		{"match-as-path-set state", bgpCond + "/match-as-path-set/state", `{"openconfig-bgp-policy:state":{"as-path-set":"ASP1"}}`},
		{"match-interface state", stmt + "/conditions/match-interface/state", `{"openconfig-routing-policy:state":{"interface":"Ethernet120"}}`},
		// FRR holds the members the reference expands to.
		{"set-ext-community state", sec + "/state", `{"openconfig-bgp-policy:state":{"method":"INLINE"}}`},
		{"set-ext-community inline state", sec + "/inline/state", `{"openconfig-bgp-policy:state":{"communities":["route-target:9:9"]}}`},
	} {
		t.Run("GET "+c.name, processGetRequest(c.url, nil, c.want, false))
		time.Sleep(1 * time.Second)
	}

	t.Run("DELETE match-interface config", processDeleteRequest(stmt+"/conditions/match-interface/config", false))
	time.Sleep(1 * time.Second)
	t.Run("DELETE set-ext-community", processDeleteRequest(sec, false))
	time.Sleep(1 * time.Second)
	t.Run("DELETE conditions config call-policy", processDeleteRequest(stmt+"/conditions/config/call-policy", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify ROUTE_MAP after the deletes", verifyDbResult(rclient, "ROUTE_MAP|POLVI|10", map[string]interface{}{"ROUTE_MAP": map[string]interface{}{"POLVI|10": map[string]interface{}{
		"route_operation": "permit", "match_protocol": "bgp", "match_as_path": "ASP1", "match_ext_community": "ECVI"}}}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedTbl)
}
