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

const niStaticRouteProtoUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=STATIC][name=static]"

func Test_openconfig_network_instance_static_route_config_prefix(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.1.0/24]/config/prefix"
	urlInputBodyJSON = "{\"openconfig-network-instance:prefix\":\"172.16.1.0/24\"}"
	t.Run("PATCH network-instance static route config prefix", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// The prefix is the CONFIG_DB key, so a route with no next-hop yet is a
	// key only row.
	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": map[string]interface{}{},
		},
	}
	t.Run("Verify STATIC_ROUTE row created", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:prefix\":\"172.16.1.0/24\"}"
	t.Run("GET network-instance static route config prefix", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "static route prefix \"172.16.2.0/24\" in payload does not match prefix \"172.16.1.0/24\" in request path",
	}
	urlInputBodyJSON = "{\"openconfig-network-instance:prefix\":\"172.16.2.0/24\"}"
	t.Run("PATCH static route prefix mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance static route config prefix", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE row deleted", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_config_container(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|2004::/64": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = niStaticRouteProtoUrl + "/static-routes/static[prefix=2004::/64]/config"
	urlInputBodyJSON = "{\"prefix\":\"2004::/64\"}"
	t.Run("PATCH network-instance static route config container", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|2004::/64": map[string]interface{}{},
		},
	}
	t.Run("Verify IPv6 STATIC_ROUTE row created", verifyDbResult(rclient, "STATIC_ROUTE|default|2004::/64", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:config\":{\"prefix\":\"2004::/64\"}}"
	t.Run("GET network-instance static route config container", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance static route config container", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify IPv6 STATIC_ROUTE row deleted", verifyDbResult(rclient, "STATIC_ROUTE|default|2004::/64", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_container_set(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": "",
			"default|172.16.2.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// One container level request carrying two static routes.
	urlInputBodyJSON := `{"static":[{"prefix":"172.16.1.0/24","config":{"prefix":"172.16.1.0/24"}},` +
		`{"prefix":"172.16.2.0/24","config":{"prefix":"172.16.2.0/24"}}]}`
	t.Run("PATCH network-instance static-routes container",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": map[string]interface{}{},
		},
	}
	t.Run("Verify first STATIC_ROUTE row", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.2.0/24": map[string]interface{}{},
		},
	}
	t.Run("Verify second STATIC_ROUTE row", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.2.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The same payload anchored at the protocol instance.
	urlInputBodyJSON = `{"static-routes":{"static":[{"prefix":"172.16.1.0/24","config":{"prefix":"172.16.1.0/24"}}]}}`
	t.Run("PATCH network-instance STATIC protocol container",
		processSetRequest(niStaticRouteProtoUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE row from protocol container payload",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_container_get(t *testing.T) {
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": map[string]interface{}{
				"nexthop": "10.1.1.1",
			},
			"default|2004::/64": map[string]interface{}{
				"nexthop": "2001::1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": "",
			"default|2004::/64":     "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	v4EntryJSON := `{"config":{"prefix":"172.16.1.0/24"},"prefix":"172.16.1.0/24"}`
	v6EntryJSON := `{"config":{"prefix":"2004::/64"},"prefix":"2004::/64"}`

	expectedGetJSON := `{"openconfig-network-instance:static-routes":{"static":[` + v4EntryJSON + `,` + v6EntryJSON + `]}}`
	t.Run("GET network-instance static-routes container",
		processGetRequest(niStaticRouteProtoUrl+"/static-routes", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The whole list must come from STATIC_ROUTE only. A generically expanded
	// list reports one synthetic entry keyed by the inherited parent key.
	expectedGetJSON = `{"openconfig-network-instance:static":[` + v4EntryJSON + `,` + v6EntryJSON + `]}`
	t.Run("GET network-instance static route list",
		processGetRequest(niStaticRouteProtoUrl+"/static-routes/static", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:static":[` + v4EntryJSON + `]}`
	t.Run("GET network-instance static route list instance",
		processGetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=172.16.1.0/24]", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_unmapped_payload_rejected(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.3.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// set-tag has no CONFIG_DB mapping, so the whole request is rejected.
	urlInputBodyJSON := `{"static":[{"prefix":"172.16.3.0/24","config":{"prefix":"172.16.3.0/24","set-tag":42}}]}`
	expectedErr := tlerr.NotSupportedError{
		Format: "static routes container payload carries leaves that have no CONFIG_DB mapping: " +
			"static-routes/static[172.16.3.0/24]/config/set-tag",
	}
	t.Run("PATCH static-routes container rejects unmapped leaf",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes", urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE not created after rejected container payload",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.3.0/24", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_invalid_prefix_rejected(t *testing.T) {
	preReqMap := map[string]interface{}{}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url := niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.1.1/24]/config"
	urlInputBodyJSON := "{\"prefix\":\"172.16.1.1/24\"}"
	expectedErr := tlerr.InvalidArgsError{
		Format: "static route prefix \"172.16.1.1/24\" is not in normalized CIDR form; expected \"172.16.1.0/24\"",
	}
	t.Run("PATCH static route with host address rejected",
		processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE not created for non normalized prefix",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.1/24", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)
}

func Test_openconfig_network_instance_static_route_container_delete_not_supported(t *testing.T) {
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": map[string]interface{}{
				"nexthop": "10.1.1.1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.NotSupportedError{
		Format: "DELETE is not supported at static routes container level; delete a static route instance instead",
	}
	t.Run("DELETE static-routes container rejected",
		processDeleteRequest(niStaticRouteProtoUrl+"/static-routes", true, expectedErr))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": map[string]interface{}{
				"nexthop": "10.1.1.1",
			},
		},
	}
	t.Run("Verify STATIC_ROUTE row kept after rejected container DELETE",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE static route list instance", processDeleteRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=172.16.1.0/24]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE row deleted with list instance",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
