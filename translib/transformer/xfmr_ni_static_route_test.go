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
		`{"config":{"index":"10.1.1.1","next-hop":"10.1.1.1"},"index":"10.1.1.1"}]},` +
		`"state":{"prefix":"172.16.1.0/24"}}`
	v6EntryJSON := `{"config":{"prefix":"2004::/64"},"prefix":"2004::/64","next-hops":{"next-hop":[` +
		`{"config":{"index":"2001::1","next-hop":"2001::1"},"index":"2001::1"}]},` +
		`"state":{"prefix":"2004::/64"}}`

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

	// metric has no CONFIG_DB mapping, FRR having no metric for a static
	// route at all, so the whole request is rejected.
	urlInputBodyJSON := `{"static":[{"prefix":"172.16.3.0/24","next-hops":{"next-hop":[` +
		`{"index":"60.0.0.1","config":{"index":"60.0.0.1","next-hop":"60.0.0.1","metric":5}}]}}]}`
	expectedErr := tlerr.NotSupportedError{
		Format: "static routes container payload carries leaves that have no CONFIG_DB mapping: " +
			"static-routes/static[172.16.3.0/24]/next-hops/next-hop[60.0.0.1]/config/metric",
	}
	t.Run("PATCH static-routes container rejects unmapped leaf",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes", urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE not created after rejected container payload",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.3.0/24", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	// The same payload at the route's own config container. set-tag is mapped
	// now, so what stops this one is the route having no next-hop to carry the
	// tag, which is the refusal that guards the fan-out.
	cfgUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.3.0/24]/config"
	cfgInputBodyJSON := `{"prefix":"172.16.3.0/24","set-tag":42}`
	cfgExpectedErr := tlerr.InvalidArgsError{
		Format: "static route 172.16.3.0/24 has no next-hop to carry a tag; " +
			"FRR takes the tag on each route line, so a next-hop has to exist first",
	}
	t.Run("PATCH config container tagging a route with no next-hop is refused",
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

// The protocol instance name is not a CONFIG_DB key component, so both served
// spellings reach the same rows. Every other name would be one more alias for
// those rows and is refused instead.
func Test_openconfig_network_instance_static_route_protocol_instance_name(t *testing.T) {
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24": map[string]interface{}{
				"nexthop": "10.1.1.9",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24":  "",
			"default|172.16.10.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	niProtoUrlPfx := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=STATIC][name="
	upperProtoUrl := niProtoUrlPfx + "STATIC]"
	unknownProtoUrl := niProtoUrlPfx + "foo]"

	entryJSON := `{"config":{"prefix":"172.16.9.0/24"},"prefix":"172.16.9.0/24","next-hops":{"next-hop":[` +
		`{"config":{"index":"10.1.1.9","next-hop":"10.1.1.9"},"index":"10.1.1.9"}]},` +
		`"state":{"prefix":"172.16.9.0/24"}}`
	expectedGetJSON := `{"openconfig-network-instance:static":[` + entryJSON + `]}`
	t.Run("GET static route list with upper case protocol instance name",
		processGetRequest(upperProtoUrl+"/static-routes/static", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url := upperProtoUrl + "/static-routes/static[prefix=172.16.10.0/24]/config/prefix"
	urlInputBodyJSON := "{\"openconfig-network-instance:prefix\":\"172.16.10.0/24\"}"
	t.Run("PATCH static route with upper case protocol instance name",
		processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.10.0/24": map[string]interface{}{},
		},
	}
	t.Run("Verify STATIC_ROUTE row created via upper case protocol instance name",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.10.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedErr := tlerr.NotSupportedError{
		Format: "unsupported STATIC protocol instance name \"foo\"; expected \"static\" or \"STATIC\"",
	}
	t.Run("GET static route list with unknown protocol instance name rejected",
		processGetRequest(unknownProtoUrl+"/static-routes/static", nil, "", true, expectedErr))
	time.Sleep(1 * time.Second)

	url = unknownProtoUrl + "/static-routes/static[prefix=172.16.11.0/24]/config/prefix"
	urlInputBodyJSON = "{\"openconfig-network-instance:prefix\":\"172.16.11.0/24\"}"
	t.Run("PATCH static route with unknown protocol instance name rejected",
		processSetRequest(url, urlInputBodyJSON, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify STATIC_ROUTE not created for unknown protocol instance name",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.11.0/24", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	// The identifier may arrive module qualified, which is how a client that
	// round-trips a GET payload spells it. The transformer accepts it, but
	// nothing above it does: ygot renders a list key as the bare STATIC and
	// compares path keys as plain strings, so the qualified spelling matches no
	// protocol instance and the read reports an empty list.
	qualifiedProtoUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=openconfig-policy-types:STATIC][name=static]"
	t.Run("GET static route list with module qualified identifier reads empty",
		processGetRequest(qualifiedProtoUrl+"/static-routes/static", nil, "{}", false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
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

func Test_openconfig_network_instance_static_route_config_set_tag(t *testing.T) {
	var url, urlInputBodyJSON string

	// Two next-hops, so that the fan-out has something to fan out to.
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24": map[string]interface{}{
				"nexthop": "30.0.0.1,30.0.0.2",
				"ifname":  ",",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	staticUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=STATIC][name=static]/static-routes/static[prefix=172.16.9.0/24]"
	url = staticUrl + "/config/set-tag"

	// The tag is a property of the route, so it lands on every next-hop.
	urlInputBodyJSON = "{\"openconfig-network-instance:set-tag\":100}"
	t.Run("PATCH static route config set-tag", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24": map[string]interface{}{
				"nexthop": "30.0.0.1,30.0.0.2",
				"ifname":  ",",
				"tag":     "100,100",
			},
		},
	}
	t.Run("Verify the tag landed once per next-hop", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.9.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := "{\"openconfig-network-instance:set-tag\":100}"
	t.Run("GET static route config set-tag", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The hex-string member of the union is converted rather than refused,
	// since CONFIG_DB and FRR both hold the decimal.
	urlInputBodyJSON = "{\"openconfig-network-instance:set-tag\":\"ff\"}"
	t.Run("PATCH static route config set-tag as a hex-string", processSetRequest(url, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24": map[string]interface{}{
				"nexthop": "30.0.0.1,30.0.0.2",
				"ifname":  ",",
				"tag":     "255,255",
			},
		},
	}
	t.Run("Verify the hex-string arrived as decimal", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.9.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	// FRR takes 1 through 4294967295, and 0 is how it spells no tag at all.
	urlInputBodyJSON = "{\"openconfig-network-instance:set-tag\":0}"
	t.Run("PATCH static route config set-tag zero is refused",
		processSetRequest(url, urlInputBodyJSON, "PATCH", true, tlerr.InvalidArgsError{
			Format: "static route set-tag accepts 1 through 4294967295, since that is the range FRR takes"}))
	time.Sleep(1 * time.Second)

	// A next-hop added afterwards has to inherit the tag. Were it padded with
	// an empty element, the tag list would be shorter than the next-hop list
	// and FRR would program nothing for the route at all.
	nhUrl := staticUrl + "/next-hops/next-hop[index=30.0.0.3]/config/next-hop"
	urlInputBodyJSON = "{\"openconfig-network-instance:next-hop\":\"30.0.0.3\"}"
	t.Run("PATCH a third next-hop onto the tagged route",
		processSetRequest(nhUrl, urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24": map[string]interface{}{
				"nexthop": "30.0.0.1,30.0.0.2,30.0.0.3",
				"ifname":  ",,",
				"tag":     "255,255,255",
			},
		},
	}
	t.Run("Verify the new next-hop inherited the tag", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.9.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A delete of the leaf takes the tag and leaves the route.
	t.Run("DELETE static route config set-tag", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.9.0/24": map[string]interface{}{
				"nexthop": "30.0.0.1,30.0.0.2,30.0.0.3",
				"ifname":  ",,",
			},
		},
	}
	t.Run("Verify the tag is gone and the route is not", verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.9.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_set_tag_without_next_hop(t *testing.T) {
	// A route with no next-hop has nowhere to carry a tag: FRR takes the tag
	// on each route line, and a tag list that does not match the next-hop
	// list in length programs nothing at all. A blackhole route does carry
	// one, so the case here is a route with no next-hop of any kind.
	url := "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=STATIC][name=static]/static-routes/static[prefix=172.16.7.0/24]/config/set-tag"
	t.Run("PATCH set-tag on a route with no next-hop is refused",
		processSetRequest(url, "{\"openconfig-network-instance:set-tag\":100}", "PATCH", true, tlerr.InvalidArgsError{
			Format: "static route 172.16.7.0/24 has no next-hop to carry a tag; FRR takes the tag on each route line, so a next-hop has to exist first"}))
	time.Sleep(1 * time.Second)
}

func Test_openconfig_network_instance_static_route_next_hop_preference(t *testing.T) {
	// Two next-hops, because a preference belongs to one next-hop rather than
	// to the route: FRR writes each next-hop as its own route line with its
	// own distance, and prefers the lowest.
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.11.0/24": map[string]interface{}{
				"nexthop": "40.0.0.1,40.0.0.2",
				"ifname":  ",",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.11.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	staticUrl := niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.11.0/24]"
	firstUrl := staticUrl + "/next-hops/next-hop[index=40.0.0.1]/config/preference"
	secondUrl := staticUrl + "/next-hops/next-hop[index=40.0.0.2]/config/preference"

	t.Run("PATCH static route next-hop preference",
		processSetRequest(firstUrl, `{"openconfig-network-instance:preference":10}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// The next-hop that was not named keeps the default, which the row spells
	// as 0. A distance list shorter than the next-hop list programs none of
	// the route at all.
	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.11.0/24": map[string]interface{}{
				"nexthop":  "40.0.0.1,40.0.0.2",
				"ifname":   ",",
				"distance": "10,0",
			},
		},
	}
	t.Run("Verify the preference landed on its own next-hop",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.11.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	t.Run("GET static route next-hop preference",
		processGetRequest(firstUrl, nil, `{"openconfig-network-instance:preference":10}`, false))
	time.Sleep(1 * time.Second)

	// Two next-hops at different preferences is the point of the leaf: the
	// second is a backup rather than a second path.
	t.Run("PATCH a different preference on the other next-hop",
		processSetRequest(secondUrl, `{"openconfig-network-instance:preference":200}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.11.0/24": map[string]interface{}{
				"nexthop":  "40.0.0.1,40.0.0.2",
				"ifname":   ",",
				"distance": "10,200",
			},
		},
	}
	t.Run("Verify each next-hop carries its own preference",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.11.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:config":{"index":"40.0.0.2","next-hop":"40.0.0.2","preference":200}}`
	t.Run("GET the next-hop config container with its preference",
		processGetRequest(staticUrl+"/next-hops/next-hop[index=40.0.0.2]/config", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// A delete takes the preference of the next-hop it names and leaves both
	// the next-hop and the other next-hop's preference alone.
	t.Run("DELETE static route next-hop preference", processDeleteRequest(firstUrl, false))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.11.0/24": map[string]interface{}{
				"nexthop":  "40.0.0.1,40.0.0.2",
				"ifname":   ",",
				"distance": "0,200",
			},
		},
	}
	t.Run("Verify only the named next-hop lost its preference",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.11.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	// The 0 left behind is the absence of a preference, so it is reported as
	// absent rather than as 0: FRR leaves the distance off that route line and
	// applies its own default, so 0 is never the distance the next-hop has.
	expectedGetJSON = `{"openconfig-network-instance:config":{"index":"40.0.0.1","next-hop":"40.0.0.1"}}`
	t.Run("GET the next-hop whose preference was deleted",
		processGetRequest(staticUrl+"/next-hops/next-hop[index=40.0.0.1]/config", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET the leaf itself once deleted", processGetRequest(firstUrl, nil, emptyJson, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hop_preference_container(t *testing.T) {
	preReqMap := map[string]interface{}{}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.12.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// One container level request carrying both next-hops and the preference
	// of each, so the whole route arrives at once.
	urlInputBodyJSON := `{"next-hop":[{"index":"41.0.0.1","config":{"index":"41.0.0.1","next-hop":"41.0.0.1","preference":20}},` +
		`{"index":"41.0.0.2","config":{"index":"41.0.0.2","next-hop":"41.0.0.2","preference":220}}]}`
	t.Run("PATCH the next-hops container with a preference on each",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=172.16.12.0/24]/next-hops",
			urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.12.0/24": map[string]interface{}{
				"nexthop":  "41.0.0.1,41.0.0.2",
				"distance": "20,220",
			},
		},
	}
	t.Run("Verify both preferences landed in next-hop order",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.12.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_next_hop_preference_rejected(t *testing.T) {
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.13.0/24": map[string]interface{}{
				"nexthop": "42.0.0.1",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.13.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	url := niStaticRouteProtoUrl + "/static-routes/static[prefix=172.16.13.0/24]/next-hops/next-hop[index=42.0.0.1]/config/preference"

	// FRR takes 1 through 255, and a 0 is how the row spells no preference at
	// all, so it is refused rather than written: a DELETE asks for that.
	t.Run("PATCH preference zero is refused",
		processSetRequest(url, `{"openconfig-network-instance:preference":0}`, "PATCH", true, tlerr.InvalidArgsError{
			Format: "static route next-hop preference 0 is out of range; FRR takes an administrative distance of 1 to 255, and a DELETE of the leaf restores the default"}))
	time.Sleep(1 * time.Second)

	// The OpenConfig leaf is a uint32, so a value FRR cannot take is refused
	// here rather than left to the pattern CVL enforces on the field.
	t.Run("PATCH preference above the FRR range is refused",
		processSetRequest(url, `{"openconfig-network-instance:preference":256}`, "PATCH", true, tlerr.InvalidArgsError{
			Format: "static route next-hop preference 256 is out of range; FRR takes an administrative distance of 1 to 255, and a DELETE of the leaf restores the default"}))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

func Test_openconfig_network_instance_static_route_set_tag_from_container(t *testing.T) {
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.15.0/24": map[string]interface{}{
				"nexthop": "50.0.0.3",
			},
		},
	}
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.14.0/24": "",
			"default|172.16.15.0/24": "",
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	// One request carrying the route, both its next-hops and its tag. The tag
	// is applied once the next-hops have been merged in, since a route with
	// none of them has nowhere to carry one.
	urlInputBodyJSON := `{"static":[{"prefix":"172.16.14.0/24","config":{"prefix":"172.16.14.0/24","set-tag":600},` +
		`"next-hops":{"next-hop":[{"index":"50.0.0.1","config":{"index":"50.0.0.1","next-hop":"50.0.0.1"}},` +
		`{"index":"50.0.0.2","config":{"index":"50.0.0.2","next-hop":"50.0.0.2"}}]}}]}`
	t.Run("PATCH the static-routes container with a tagged route",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes", urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.14.0/24": map[string]interface{}{
				"nexthop": "50.0.0.1,50.0.0.2",
				"tag":     "600,600",
			},
		},
	}
	t.Run("Verify the tag arrived with the route it came in with",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.14.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON := `{"openconfig-network-instance:set-tag":600}`
	t.Run("GET the tag that arrived through the container",
		processGetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=172.16.14.0/24]/config/set-tag",
			nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// The same, one level down, on a route that already has its next-hop.
	urlInputBodyJSON = `{"prefix":"172.16.15.0/24","config":{"prefix":"172.16.15.0/24","set-tag":601}}`
	t.Run("PATCH the route instance with a tag",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes/static[prefix=172.16.15.0/24]",
			urlInputBodyJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expectedMap = map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.15.0/24": map[string]interface{}{
				"nexthop": "50.0.0.3",
				"tag":     "601",
			},
		},
	}
	t.Run("Verify the route instance payload tagged the route",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.15.0/24", expectedMap, false))
	time.Sleep(1 * time.Second)

	// A route with no next-hop is refused here for the same reason as at the
	// route's own config container, rather than accepted and dropped.
	urlInputBodyJSON = `{"static":[{"prefix":"172.16.16.0/24","config":{"prefix":"172.16.16.0/24","set-tag":602}}]}`
	t.Run("PATCH a container payload tagging a route with no next-hop is refused",
		processSetRequest(niStaticRouteProtoUrl+"/static-routes", urlInputBodyJSON, "PATCH", true, tlerr.InvalidArgsError{
			Format: "static route 172.16.16.0/24 has no next-hop to carry a tag; FRR takes the tag on each route line, so a next-hop has to exist first"}))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// A static route's state container has no datastore of its own: it reports the
// same STATIC_ROUTE row the config container writes, prefix and the tag alike.
// It is read-only, and the request binder refuses a write to it before any
// transformer runs.
func Test_openconfig_network_instance_static_route_state(t *testing.T) {
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{"default|172.16.40.0/24": ""},
	}
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.40.0/24": map[string]interface{}{
				"nexthop": "10.1.1.40",
				"ifname":  "",
				"tag":     "77",
			},
		},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	rtUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=STATIC][name=static]/static-routes/static[prefix=172.16.40.0/24]"

	expectedGetJSON := `{"openconfig-network-instance:prefix":"172.16.40.0/24"}`
	t.Run("GET the state prefix leaf",
		processGetRequest(rtUrl+"/state/prefix", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:set-tag":77}`
	t.Run("GET the state set-tag leaf",
		processGetRequest(rtUrl+"/state/set-tag", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:state":{"prefix":"172.16.40.0/24","set-tag":77}}`
	t.Run("GET the state container",
		processGetRequest(rtUrl+"/state", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// state is config false, so the binder turns a write away before any
	// transformer is reached. That message is what a client sees.
	expectedErr := tlerr.TranslibSyntaxValidationError{
		ErrorStr: errors.New("SET operation not allowed on the read-only object"),
	}
	t.Run("PATCH the state prefix leaf is refused",
		processSetRequest(rtUrl+"/state/prefix",
			`{"openconfig-network-instance:prefix":"172.16.40.0/24"}`, "PATCH", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("DELETE the state container is refused",
		processDeleteRequest(rtUrl+"/state", true, expectedErr))
	time.Sleep(1 * time.Second)

	verifyMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.40.0/24": map[string]interface{}{
				"nexthop": "10.1.1.40",
				"ifname":  "",
				"tag":     "77",
			},
		},
	}
	t.Run("Verify the refused writes left the row alone",
		verifyDbResult(rclient, "STATIC_ROUTE|default|172.16.40.0/24", verifyMap, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// A wildcard on the static list's own key reads every route of the instance.
// The enclosing keys stay concrete: omitting the network-instance or protocol
// key is served nowhere in this repo, and a partial wildcard on the protocol
// pair is refused by the request binder before any transformer runs.
// A wildcard on the static list's own key is served at the list node: translib
// removes the key before dispatch, the node is the list itself, and every route
// of the instance is answered with its state alongside its config. A request
// that wildcards the key and then descends into a container is refused, because
// after the key is removed the path identifies no route and an answer would be
// silently empty. The enclosing keys stay concrete throughout: omitting the
// network-instance or protocol key is served nowhere in this repo.
func Test_openconfig_network_instance_static_route_state_wildcard(t *testing.T) {
	cleanupTbl := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.41.0/24": "",
			"default|172.16.42.0/24": "",
		},
	}
	preReqMap := map[string]interface{}{
		"STATIC_ROUTE": map[string]interface{}{
			"default|172.16.41.0/24": map[string]interface{}{
				"nexthop": "10.1.1.41", "ifname": "", "tag": "41",
			},
			"default|172.16.42.0/24": map[string]interface{}{
				"nexthop": "10.1.1.42", "ifname": "", "tag": "42",
			},
		},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	routesUrl := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=STATIC][name=static]/static-routes"

	wildcardListJSON := `{"openconfig-network-instance:static":[` +
		`{"config":{"prefix":"172.16.41.0/24","set-tag":41},"next-hops":{"next-hop":[{"config":{"index":"10.1.1.41","next-hop":"10.1.1.41"},"index":"10.1.1.41"}]},"prefix":"172.16.41.0/24","state":{"prefix":"172.16.41.0/24","set-tag":41}}` + `,` +
		`{"config":{"prefix":"172.16.42.0/24","set-tag":42},"next-hops":{"next-hop":[{"config":{"index":"10.1.1.42","next-hop":"10.1.1.42"},"index":"10.1.1.42"}]},"prefix":"172.16.42.0/24","state":{"prefix":"172.16.42.0/24","set-tag":42}}` + `]}`

	t.Run("GET the route list with a wildcard prefix",
		processGetRequest(routesUrl+"/static[prefix=*]", nil, wildcardListJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET the route list with the key omitted",
		processGetRequest(routesUrl+"/static", nil, wildcardListJSON, false))
	time.Sleep(1 * time.Second)

	// One level down the key is gone and the path names no route.
	expectedErr := tlerr.NotSupportedError{
		Format: "a wildcard or absent static route prefix is served at the route list; address static-routes/static rather than a container below it",
	}
	t.Run("GET the state container with a wildcard prefix is refused",
		processGetRequest(routesUrl+"/static[prefix=*]/state", nil, "", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("GET the state prefix leaf with a wildcard prefix is refused",
		processGetRequest(routesUrl+"/static[prefix=*]/state/prefix", nil, "", true, expectedErr))
	time.Sleep(1 * time.Second)

	t.Run("GET the config container with a wildcard prefix is refused",
		processGetRequest(routesUrl+"/static[prefix=*]/config", nil, "", true, expectedErr))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
