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

func Test_openconfig_network_instance_static_route_config_prefix_ipv6(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|2004::/64": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url = niStaticRouteProtoUrl + "/static-routes/static[prefix=2004::/64]/config/prefix"
	urlInputBodyJSON = "{\"openconfig-network-instance:prefix\":\"2004::/64\"}"
	t.Run("PATCH network-instance static route config prefix IPv6", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|2004::/64": map[string]interface{}{},
		},
	}
	t.Run("Verify IPv6 STATIC_ROUTE row created", verifyDbResult(rclient, "STATIC_ROUTE|default|2004::/64", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:prefix\":\"2004::/64\"}"
	t.Run("GET network-instance static route config prefix IPv6", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.InvalidArgsError{
		Format: "static route prefix \"2005::/64\" in payload does not match prefix \"2004::/64\" in request path",
	}
	urlInputBodyJSON = "{\"openconfig-network-instance:prefix\":\"2005::/64\"}"
	t.Run("PATCH static route prefix IPv6 mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance static route config prefix IPv6", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	t.Run("Verify IPv6 STATIC_ROUTE row deleted", verifyDbResult(rclient, "STATIC_ROUTE|default|2004::/64", map[string]interface{}{}, false))
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

	v4EntryJSON := `{"config":{"prefix":"172.16.1.0/24"},"prefix":"172.16.1.0/24","next-hops":{"next-hop":[` +
		`{"config":{"index":"10.1.1.1","next-hop":"10.1.1.1"},"index":"10.1.1.1"}]}}`
	v6EntryJSON := `{"config":{"prefix":"2004::/64"},"prefix":"2004::/64","next-hops":{"next-hop":[` +
		`{"config":{"index":"2001::1","next-hop":"2001::1"},"index":"2001::1"}]}}`

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

	cfgUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.3.0/24]/config"
	cfgInputBodyJSON := `{"prefix":"172.16.3.0/24","set-tag":42}`
	cfgExpectedErr := tlerr.NotSupportedError{
		Format: "static route payload carries leaves that have no CONFIG_DB mapping: " +
			"static-routes/static[172.16.3.0/24]/config/set-tag",
	}
	t.Run("PATCH config container rejects unmapped leaf",
		processSetRequest(cfgUrl, cfgInputBodyJSON, "PATCH", true, cfgExpectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE not created after rejected config payload",
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

func Test_openconfig_network_instance_static_route_next_hop_config(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	nhUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.1.0/24]/next-hops/next-hop[index=10.1.1.1]"

	url = nhUrl + "/config/next-hop"
	urlInputBodyJSON = "{\"openconfig-network-instance:next-hop\":\"10.1.1.1\"}"
	t.Run("PATCH network-instance static route next-hop", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.1.0/24": map[string]interface{}{
				"nexthop": "10.1.1.1",
			},
		},
	}
	t.Run("Verify STATIC_ROUTE next-hop written", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:next-hop\":\"10.1.1.1\"}"
	t.Run("GET network-instance static route next-hop", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = "{\"openconfig-network-instance:index\":\"10.1.1.1\"}"
	t.Run("GET network-instance static route next-hop index", processGetRequest(nhUrl+"/config/index", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = "{\"openconfig-network-instance:config\":{\"index\":\"10.1.1.1\",\"next-hop\":\"10.1.1.1\"}}"
	t.Run("GET network-instance static route next-hop config container",
		processGetRequest(nhUrl+"/config", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The next-hop in the payload has to agree with the next-hop the list key
	// already identifies.
	expectedErr := tlerr.InvalidArgsError{
		Format: "static route next-hop \"10.1.1.2\" in payload does not match next-hop index \"10.1.1.1\"",
	}
	urlInputBodyJSON = "{\"openconfig-network-instance:next-hop\":\"10.1.1.2\"}"
	t.Run("PATCH static route next-hop mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// The route keeps the only next-hop it has; the route itself has to be
	// deleted instead.
	expectedErr2 := tlerr.NotSupportedError{
		Format: "DELETE of the only next-hop is not supported; delete the static route instead",
	}
	t.Run("DELETE only next-hop rejected", processDeleteRequest(nhUrl, true, expectedErr2))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE next-hop kept after rejected DELETE",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hop_config_ipv6(t *testing.T) {
	var url, urlInputBodyJSON string

	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|2004::/64": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	nhUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=2004::/64]/next-hops/next-hop[index=2001::1]"

	url = nhUrl + "/config/next-hop"
	urlInputBodyJSON = "{\"openconfig-network-instance:next-hop\":\"2001::1\"}"
	t.Run("PATCH network-instance static route IPv6 next-hop", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|2004::/64": map[string]interface{}{
				"nexthop": "2001::1",
			},
		},
	}
	t.Run("Verify IPv6 STATIC_ROUTE next-hop written", verifyDbResult(rclient, "STATIC_ROUTE|default|2004::/64", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:next-hop\":\"2001::1\"}"
	t.Run("GET network-instance static route IPv6 next-hop", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = "{\"openconfig-network-instance:index\":\"2001::1\"}"
	t.Run("GET network-instance static route IPv6 next-hop index", processGetRequest(nhUrl+"/config/index", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = "{\"openconfig-network-instance:config\":{\"index\":\"2001::1\",\"next-hop\":\"2001::1\"}}"
	t.Run("GET network-instance static route IPv6 next-hop config container",
		processGetRequest(nhUrl+"/config", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The next-hop in the payload has to agree with the next-hop the list key
	// already identifies.
	expectedErr := tlerr.InvalidArgsError{
		Format: "static route next-hop \"2001::2\" in payload does not match next-hop index \"2001::1\"",
	}
	urlInputBodyJSON = "{\"openconfig-network-instance:next-hop\":\"2001::2\"}"
	t.Run("PATCH static route IPv6 next-hop mismatch rejected", processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	// The route keeps the only next-hop it has; the route itself has to be
	// deleted instead.
	expectedErr2 := tlerr.NotSupportedError{
		Format: "DELETE of the only next-hop is not supported; delete the static route instead",
	}
	t.Run("DELETE only IPv6 next-hop rejected", processDeleteRequest(nhUrl, true, expectedErr2))
	time.Sleep(1 * time.Second)

	t.Run("Verify IPv6 STATIC_ROUTE next-hop kept after rejected DELETE",
		verifyDbResult(rclient, "STATIC_ROUTE|default|2004::/64", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hops_container(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.20.0.0/16": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// One container level request carrying an IP next-hop and a blackhole
	// next-hop for the same route.
	urlInputBodyJSON := `{"next-hop":[{"index":"10.1.1.1","config":{"index":"10.1.1.1","next-hop":"10.1.1.1"}},` +
		`{"index":"DROP","config":{"index":"DROP","next-hop":"openconfig-local-routing:DROP"}}]}`
	t.Run("PATCH network-instance static route next-hops container",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=10.20.0.0/16]/next-hops",
			urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// STATIC_ROUTE keeps one comma separated list per attribute, read position
	// by position, so every list holds one element per next-hop.
	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.20.0.0/16": map[string]interface{}{
				"nexthop":   "10.1.1.1,0.0.0.0",
				"ifname":    ",Null0",
				"blackhole": "false,true",
			},
		},
	}
	t.Run("Verify STATIC_ROUTE row with two next-hops",
		verifyDbResult(rclient, "STATIC_ROUTE|default|10.20.0.0/16", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:next-hops":{"next-hop":[` +
		`{"config":{"index":"10.1.1.1","next-hop":"10.1.1.1"},"index":"10.1.1.1"},` +
		`{"config":{"index":"DROP","next-hop":"openconfig-local-routing:DROP"},"index":"DROP"}]}}`
	t.Run("GET network-instance static route next-hops container",
		processGetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=10.20.0.0/16]/next-hops", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:next-hop":[` +
		`{"config":{"index":"DROP","next-hop":"openconfig-local-routing:DROP"},"index":"DROP"}]}`
	t.Run("GET network-instance static route next-hop list instance",
		processGetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=10.20.0.0/16]/next-hops/next-hop[index=DROP]",
			nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hop_local_link(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.40.0.0/16": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// An interface only next-hop is stored as the unspecified address plus the
	// interface it leaves through.
	nhUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=10.40.0.0/16]/next-hops/next-hop[index=LOCAL_LINK+Ethernet0]"
	urlInputBodyJSON := `{"index":"LOCAL_LINK+Ethernet0","next-hop":"openconfig-local-routing:LOCAL_LINK"}`
	t.Run("PATCH network-instance static route LOCAL_LINK next-hop",
		processSetRequest(nhUrl+"/config", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.40.0.0/16": map[string]interface{}{
				"nexthop": "0.0.0.0",
				"ifname":  "Ethernet0",
			},
		},
	}
	t.Run("Verify STATIC_ROUTE row with LOCAL_LINK next-hop",
		verifyDbResult(rclient, "STATIC_ROUTE|default|10.40.0.0/16", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:config":{"index":"LOCAL_LINK+Ethernet0",` +
		`"next-hop":"openconfig-local-routing:LOCAL_LINK"}}`
	t.Run("GET network-instance static route LOCAL_LINK next-hop config",
		processGetRequest(nhUrl+"/config", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hop_merge(t *testing.T) {
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.50.0.0/16": map[string]interface{}{
				"nexthop": "10.1.1.1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.50.0.0/16": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// A next-hop added to a route that already has one is merged into the
	// comma separated attributes, and every list the row ends up with holds
	// one element per next-hop.
	nhUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=10.50.0.0/16]/next-hops/next-hop[index=LOCAL_LINK+Ethernet4]"
	t.Run("PATCH second static route next-hop",
		processSetRequest(nhUrl+"/config/next-hop", `{"openconfig-network-instance:next-hop":"openconfig-local-routing:LOCAL_LINK"}`,
			"PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.50.0.0/16": map[string]interface{}{
				"nexthop": "10.1.1.1,0.0.0.0",
				"ifname":  ",Ethernet4",
			},
		},
	}
	t.Run("Verify merged static route next-hops",
		verifyDbResult(rclient, "STATIC_ROUTE|default|10.50.0.0/16", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:next-hops":{"next-hop":[` +
		`{"config":{"index":"10.1.1.1","next-hop":"10.1.1.1"},"index":"10.1.1.1"},` +
		`{"config":{"index":"LOCAL_LINK+Ethernet4","next-hop":"openconfig-local-routing:LOCAL_LINK"},` +
		`"index":"LOCAL_LINK+Ethernet4"}]}}`
	t.Run("GET merged static route next-hops",
		processGetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=10.50.0.0/16]/next-hops", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hop_delete(t *testing.T) {
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.30.0.0/16": map[string]interface{}{
				"nexthop": "10.1.1.1,10.1.1.2",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.30.0.0/16": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// The next-hops of a route share one row, so removing one rewrites the
	// comma separated attributes with the ones that remain.
	t.Run("DELETE one static route next-hop",
		processDeleteRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=10.30.0.0/16]/next-hops/next-hop[index=10.1.1.2]", false))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|10.30.0.0/16": map[string]interface{}{
				"nexthop": "10.1.1.1",
			},
		},
	}
	t.Run("Verify remaining static route next-hop",
		verifyDbResult(rclient, "STATIC_ROUTE|default|10.30.0.0/16", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.NotFoundError{Format: "static route 10.30.0.0/16 has no next-hop 10.1.1.2"}
	t.Run("DELETE unknown static route next-hop rejected",
		processDeleteRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=10.30.0.0/16]/next-hops/next-hop[index=10.1.1.2]",
			true, expectedErr))
	time.Sleep(1 * time.Second)

	expectedErr2 := tlerr.NotSupportedError{
		Format: "DELETE is not supported at the next-hops container; delete one next-hop or the static route instead",
	}
	t.Run("DELETE static route next-hops container rejected",
		processDeleteRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=10.30.0.0/16]/next-hops", true, expectedErr2))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hop_rejected_payloads(t *testing.T) {
	preReqMap := map[string]interface{}{}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// metric has no CONFIG_DB mapping, so the whole request is rejected.
	nhUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.1.0/24]/next-hops/next-hop[index=10.1.1.1]"
	urlInputBodyJSON := `{"index":"10.1.1.1","next-hop":"10.1.1.1","metric":10}`
	expectedErr := tlerr.NotSupportedError{
		Format: "static route next-hop payload carries leaves that have no CONFIG_DB mapping: " +
			"static-routes/static[172.16.1.0/24]/next-hops/next-hop[10.1.1.1]/config/metric",
	}
	t.Run("PATCH static route next-hop rejects unmapped leaf",
		processSetRequest(nhUrl+"/config", urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE not created after rejected next-hop payload",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.1.0/24", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	// The next-hop address has to belong to the address family of the prefix.
	v6NhUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=2004::/64]/next-hops/next-hop[index=10.1.1.1]"
	expectedErr2 := tlerr.InvalidArgsError{
		Format: "static route next-hop address \"10.1.1.1\" does not match the address family of the prefix",
	}
	t.Run("PATCH static route next-hop rejects address family mismatch",
		processSetRequest(v6NhUrl+"/config/next-hop", "{\"openconfig-network-instance:next-hop\":\"10.1.1.1\"}",
			"PATCH", true, expectedErr2))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE not created after address family mismatch",
		verifyDbResult(rclient, "STATIC_ROUTE|default|2004::/64", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)
}
