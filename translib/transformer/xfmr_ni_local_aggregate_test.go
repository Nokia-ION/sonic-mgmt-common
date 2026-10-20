////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");          //
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

const niLocalAggrProtoUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=LOCAL_AGGREGATE][name=bgp]"

const niLocalAggrTcUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections"

func niLocalAggrPreReq() (map[string]interface{}, map[string]interface{}) {
	preReqMap := map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"default": map[string]interface{}{"local_asn": "65100"},
		},
		"ROUTE_MAP_SET": map[string]interface{}{
			"AGG-V4":  map[string]interface{}{"NULL": "NULL"},
			"AGG-V4B": map[string]interface{}{"NULL": "NULL"},
			"AGG-V6":  map[string]interface{}{"NULL": "NULL"},
		},
	}
	cleanupTbl := map[string]interface{}{
		"BGP_GLOBALS_AF_AGGREGATE_ADDR": map[string]interface{}{
			"default|ipv4_unicast|10.10.0.0/16":  "",
			"default|ipv4_unicast|10.20.0.0/16":  "",
			"default|ipv4_unicast|10.30.0.0/16":  "",
			"default|ipv6_unicast|2001:db8::/32": "",
		},
		"ROUTE_MAP_SET": map[string]interface{}{"AGG-V4": "", "AGG-V4B": "", "AGG-V6": ""},
		"BGP_GLOBALS":   map[string]interface{}{"default": ""},
	}
	return preReqMap, cleanupTbl
}

func niLocalAggrRow(key string, fields map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"BGP_GLOBALS_AF_AGGREGATE_ADDR": map[string]interface{}{key: fields}}
}

// Each aggregate is one BGP_GLOBALS_AF_AGGREGATE_ADDR row keyed by the family of
// its prefix, written from any level of the LOCAL_AGGREGATE protocol instance.
func Test_openconfig_network_instance_local_aggregate_config(t *testing.T) {
	preReqMap, cleanupTbl := niLocalAggrPreReq()
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	v4Url := niLocalAggrProtoUrl + "/local-aggregates/aggregate[prefix=10.10.0.0/16]"
	t.Run("PATCH local aggregate config prefix",
		processSetRequest(v4Url+"/config/prefix", `{"openconfig-network-instance:prefix":"10.10.0.0/16"}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the IPv4 aggregate row",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.10.0.0/16",
			niLocalAggrRow("default|ipv4_unicast|10.10.0.0/16", map[string]interface{}{"NULL": "NULL"}), false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH local-aggregates container with an IPv6 aggregate",
		processSetRequest(niLocalAggrProtoUrl+"/local-aggregates",
			`{"aggregate":[{"prefix":"2001:db8::/32","config":{"prefix":"2001:db8::/32","discard":true}}]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the IPv6 aggregate row",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv6_unicast|2001:db8::/32",
			niLocalAggrRow("default|ipv6_unicast|2001:db8::/32", map[string]interface{}{"NULL": "NULL"}), false))
	time.Sleep(1 * time.Second)

	t.Run("GET local aggregate config",
		processGetRequest(v4Url+"/config", nil, `{"openconfig-network-instance:config":{"discard":true,"prefix":"10.10.0.0/16"}}`, false))
	t.Run("GET local aggregate config discard leaf",
		processGetRequest(v4Url+"/config/discard", nil, `{"openconfig-network-instance:discard":true}`, false))
	t.Run("GET local-aggregates container",
		processGetRequest(niLocalAggrProtoUrl+"/local-aggregates", nil,
			`{"openconfig-network-instance:local-aggregates":{"aggregate":[`+
				`{"config":{"discard":true,"prefix":"10.10.0.0/16"},"prefix":"10.10.0.0/16"},`+
				`{"config":{"discard":true,"prefix":"2001:db8::/32"},"prefix":"2001:db8::/32"}]}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH local aggregate discard true accepted",
		processSetRequest(v4Url+"/config/discard", `{"openconfig-network-instance:discard":true}`, "PATCH", false, nil))
	t.Run("PATCH local aggregate discard false rejected",
		processSetRequest(v4Url+"/config/discard", `{"openconfig-network-instance:discard":false}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "local aggregate discard false is not supported: FRR installs every BGP aggregate with a discard next-hop"}))
	t.Run("PATCH local aggregate description rejected",
		processSetRequest(v4Url+"/config/description", `{"openconfig-network-instance:description":"x"}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "local aggregate description has no CONFIG_DB mapping"}))
	t.Run("PATCH local aggregate config carrying set-tag rejected",
		processSetRequest(v4Url+"/config", `{"prefix":"10.10.0.0/16","set-tag":5}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "local aggregate payload carries leaves that have no CONFIG_DB mapping: local-aggregates/aggregate[10.10.0.0/16]/config/set-tag"}))
	t.Run("PATCH local aggregate with a prefix not in normalized form rejected",
		processSetRequest(niLocalAggrProtoUrl+"/local-aggregates/aggregate[prefix=10.10.1.0/16]/config/prefix",
			`{"openconfig-network-instance:prefix":"10.10.1.0/16"}`, "PATCH", true,
			tlerr.InvalidArgsError{Format: "local aggregate prefix \"10.10.1.0/16\" is not in normalized CIDR form; expected \"10.10.0.0/16\""}))
	t.Run("PATCH local aggregate under another protocol instance name rejected",
		processSetRequest("/openconfig-network-instance:network-instances/network-instance[name=default]"+
			"/protocols/protocol[identifier=LOCAL_AGGREGATE][name=agg]/local-aggregates/aggregate[prefix=10.10.0.0/16]/config/prefix",
			`{"openconfig-network-instance:prefix":"10.10.0.0/16"}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "unsupported LOCAL_AGGREGATE protocol instance name \"agg\"; expected \"bgp\" or \"BGP\""}))
	time.Sleep(1 * time.Second)

	t.Run("DELETE local aggregate instance",
		processDeleteRequest(v4Url, false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the IPv4 aggregate row removed",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.10.0.0/16", map[string]interface{}{}, false))
	t.Run("Verify the IPv6 aggregate row kept",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv6_unicast|2001:db8::/32",
			niLocalAggrRow("default|ipv6_unicast|2001:db8::/32", map[string]interface{}{"NULL": "NULL"}), false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE local-aggregates container",
		processDeleteRequest(niLocalAggrProtoUrl+"/local-aggregates", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the IPv6 aggregate row removed",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv6_unicast|2001:db8::/32", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// The attributes of the aggregates of a family are set by the import-policy of
// the LOCAL_AGGREGATE to BGP table-connection, which is derived from them.
func Test_openconfig_network_instance_local_aggregate_table_connection(t *testing.T) {
	preReqMap, cleanupTbl := niLocalAggrPreReq()
	preReqMap["BGP_GLOBALS_AF_AGGREGATE_ADDR"] = map[string]interface{}{
		"default|ipv4_unicast|10.10.0.0/16":  map[string]interface{}{"NULL": "NULL"},
		"default|ipv4_unicast|10.20.0.0/16":  map[string]interface{}{"summary_only": "true"},
		"default|ipv6_unicast|2001:db8::/32": map[string]interface{}{"NULL": "NULL"},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	tcV4 := niLocalAggrTcUrl + "/table-connection[src-protocol=LOCAL_AGGREGATE][dst-protocol=BGP][address-family=IPV4]"
	t.Run("PATCH LOCAL_AGGREGATE table-connection import-policy",
		processSetRequest(tcV4+"/config/import-policy", `{"openconfig-network-instance:import-policy":["AGG-V4"]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the policy on the first IPv4 aggregate",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.10.0.0/16",
			niLocalAggrRow("default|ipv4_unicast|10.10.0.0/16", map[string]interface{}{"NULL": "NULL", "policy": "AGG-V4"}), false))
	t.Run("Verify the policy beside the other fields of the second IPv4 aggregate",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.20.0.0/16",
			niLocalAggrRow("default|ipv4_unicast|10.20.0.0/16", map[string]interface{}{"summary_only": "true", "policy": "AGG-V4"}), false))
	t.Run("Verify the IPv6 aggregate untouched",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv6_unicast|2001:db8::/32",
			niLocalAggrRow("default|ipv6_unicast|2001:db8::/32", map[string]interface{}{"NULL": "NULL"}), false))
	time.Sleep(1 * time.Second)

	v4EntryJSON := `{"address-family":"openconfig-types:IPV4","config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","import-policy":["AGG-V4"],` +
		`"src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE"},"dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE",` +
		`"state":{"address-family":"openconfig-types:IPV4","dst-protocol":"openconfig-policy-types:BGP",` +
		`"import-policy":["AGG-V4"],"src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE"}}`
	t.Run("GET LOCAL_AGGREGATE table-connection instance",
		processGetRequest(tcV4, nil, `{"openconfig-network-instance:table-connection":[`+v4EntryJSON+`]}`, false))
	t.Run("GET LOCAL_AGGREGATE table-connection import-policy",
		processGetRequest(tcV4+"/config/import-policy", nil, `{"openconfig-network-instance:import-policy":["AGG-V4"]}`, false))
	time.Sleep(1 * time.Second)

	// A new aggregate is announced with the policy of its family.
	t.Run("PATCH a new IPv4 aggregate",
		processSetRequest(niLocalAggrProtoUrl+"/local-aggregates/aggregate[prefix=10.30.0.0/16]/config/prefix",
			`{"openconfig-network-instance:prefix":"10.30.0.0/16"}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the new aggregate takes the family policy",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.30.0.0/16",
			niLocalAggrRow("default|ipv4_unicast|10.30.0.0/16", map[string]interface{}{"policy": "AGG-V4"}), false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH LOCAL_AGGREGATE table-connection import-policy with another policy",
		processSetRequest(tcV4+"/config/import-policy", `{"openconfig-network-instance:import-policy":["AGG-V4B"]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify every IPv4 aggregate takes the new policy",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.30.0.0/16",
			niLocalAggrRow("default|ipv4_unicast|10.30.0.0/16", map[string]interface{}{"policy": "AGG-V4B"}), false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE LOCAL_AGGREGATE table-connection import-policy",
		processDeleteRequest(tcV4+"/config/import-policy", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the policy cleared and the aggregate kept",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.20.0.0/16",
			niLocalAggrRow("default|ipv4_unicast|10.20.0.0/16", map[string]interface{}{"summary_only": "true"}), false))
	time.Sleep(1 * time.Second)

	tcV6 := niLocalAggrTcUrl + "/table-connection[src-protocol=LOCAL_AGGREGATE][dst-protocol=BGP][address-family=IPV6]"
	t.Run("PATCH IPv6 LOCAL_AGGREGATE table-connection config container",
		processSetRequest(tcV6+"/config", `{"import-policy":["AGG-V6"]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the IPv6 aggregate takes the policy",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv6_unicast|2001:db8::/32",
			niLocalAggrRow("default|ipv6_unicast|2001:db8::/32", map[string]interface{}{"NULL": "NULL", "policy": "AGG-V6"}), false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH LOCAL_AGGREGATE table-connection towards STATIC rejected",
		processSetRequest(niLocalAggrTcUrl+"/table-connection[src-protocol=LOCAL_AGGREGATE][dst-protocol=STATIC][address-family=IPV4]/config/import-policy",
			`{"openconfig-network-instance:import-policy":["AGG-V4"]}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "a LOCAL_AGGREGATE table-connection is served only towards BGP"}))
	time.Sleep(1 * time.Second)

	t.Run("DELETE local-aggregates container",
		processDeleteRequest(niLocalAggrProtoUrl+"/local-aggregates", false))
	time.Sleep(1 * time.Second)
	t.Run("PATCH LOCAL_AGGREGATE table-connection with no aggregate rejected",
		processSetRequest(tcV4+"/config/import-policy", `{"openconfig-network-instance:import-policy":["AGG-V4"]}`, "PATCH", true,
			tlerr.InvalidArgsError{Format: "network-instance \"default\" has no ipv4 local aggregate; the LOCAL_AGGREGATE table-connection of a family exists while one is configured"}))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// State reports the aggregates FRR holds, which bgpmon publishes.
func Test_openconfig_network_instance_local_aggregate_state(t *testing.T) {
	preReqMap, cleanupTbl := niLocalAggrPreReq()
	preReqMap["BGP_GLOBALS_AF_AGGREGATE_ADDR"] = map[string]interface{}{
		"default|ipv4_unicast|10.10.0.0/16":  map[string]interface{}{"NULL": "NULL"},
		"default|ipv6_unicast|2001:db8::/32": map[string]interface{}{"NULL": "NULL"},
	}
	appliedMap := map[string]interface{}{
		"BGP_GLOBALS_AF_AGGREGATE_ADDR_APPLIED": map[string]interface{}{
			"default|ipv4_unicast|10.10.0.0/16": map[string]interface{}{"as_set": "false", "summary_only": "false"},
		},
	}
	appliedCleanup := map[string]interface{}{
		"BGP_GLOBALS_AF_AGGREGATE_ADDR_APPLIED": map[string]interface{}{"default|ipv4_unicast|10.10.0.0/16": ""},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	unloadDB(db.StateDB, appliedCleanup)
	loadDB(db.ConfigDB, preReqMap)
	loadDB(db.StateDB, appliedMap)
	time.Sleep(1 * time.Second)

	v4Url := niLocalAggrProtoUrl + "/local-aggregates/aggregate[prefix=10.10.0.0/16]"
	t.Run("GET local aggregate state",
		processGetRequest(v4Url+"/state", nil, `{"openconfig-network-instance:state":{"discard":true,"prefix":"10.10.0.0/16"}}`, false))
	t.Run("GET local aggregate state prefix leaf",
		processGetRequest(v4Url+"/state/prefix", nil, `{"openconfig-network-instance:prefix":"10.10.0.0/16"}`, false))
	t.Run("GET local aggregate instance with its state",
		processGetRequest(v4Url, nil, `{"openconfig-network-instance:aggregate":[{"config":{"discard":true,"prefix":"10.10.0.0/16"},`+
			`"prefix":"10.10.0.0/16","state":{"discard":true,"prefix":"10.10.0.0/16"}}]}`, false))
	// FRR does not hold the IPv6 aggregate, so it has no state.
	t.Run("GET local aggregate instance FRR does not hold",
		processGetRequest(niLocalAggrProtoUrl+"/local-aggregates/aggregate[prefix=2001:db8::/32]", nil,
			`{"openconfig-network-instance:aggregate":[{"config":{"discard":true,"prefix":"2001:db8::/32"},"prefix":"2001:db8::/32"}]}`, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.StateDB, appliedCleanup)
	unloadDB(db.ConfigDB, cleanupTbl)
}

// The levels above local-aggregates report the aggregates too: the protocol
// instance, the protocols container of the network instance, and the derived
// table-connections. A DELETE of the protocol instance removes every aggregate.
func Test_openconfig_network_instance_local_aggregate_hierarchy(t *testing.T) {
	preReqMap, cleanupTbl := niLocalAggrPreReq()
	preReqMap["BGP_GLOBALS_AF_AGGREGATE_ADDR"] = map[string]interface{}{
		"default|ipv4_unicast|10.10.0.0/16":  map[string]interface{}{"policy": "AGG-V4"},
		"default|ipv6_unicast|2001:db8::/32": map[string]interface{}{"NULL": "NULL"},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	aggrsJSON := `{"aggregate":[{"config":{"discard":true,"prefix":"10.10.0.0/16"},"prefix":"10.10.0.0/16"},` +
		`{"config":{"discard":true,"prefix":"2001:db8::/32"},"prefix":"2001:db8::/32"}]}`
	t.Run("GET LOCAL_AGGREGATE protocol instance",
		processGetRequest(niLocalAggrProtoUrl, nil,
			`{"openconfig-network-instance:protocol":[{"identifier":"openconfig-policy-types:LOCAL_AGGREGATE","name":"bgp",`+
				`"local-aggregates":`+aggrsJSON+`}]}`, false))
	t.Run("GET LOCAL_AGGREGATE protocol instance spelled in upper case",
		processGetRequest("/openconfig-network-instance:network-instances/network-instance[name=default]"+
			"/protocols/protocol[identifier=LOCAL_AGGREGATE][name=BGP]/local-aggregates", nil,
			`{"openconfig-network-instance:local-aggregates":`+aggrsJSON+`}`, false))
	time.Sleep(1 * time.Second)

	v4EntryJSON := `{"address-family":"openconfig-types:IPV4","config":{"address-family":"openconfig-types:IPV4",` +
		`"dst-protocol":"openconfig-policy-types:BGP","import-policy":["AGG-V4"],` +
		`"src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE"},"dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE",` +
		`"state":{"address-family":"openconfig-types:IPV4","dst-protocol":"openconfig-policy-types:BGP",` +
		`"import-policy":["AGG-V4"],"src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE"}}`
	v6EntryJSON := `{"address-family":"openconfig-types:IPV6","config":{"address-family":"openconfig-types:IPV6",` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE"},` +
		`"dst-protocol":"openconfig-policy-types:BGP","src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE",` +
		`"state":{"address-family":"openconfig-types:IPV6","dst-protocol":"openconfig-policy-types:BGP",` +
		`"src-protocol":"openconfig-policy-types:LOCAL_AGGREGATE"}}`
	t.Run("GET table-connections container with the derived LOCAL_AGGREGATE ones",
		processGetRequest(niLocalAggrTcUrl, nil,
			`{"openconfig-network-instance:table-connections":{"table-connection":[`+v4EntryJSON+`,`+v6EntryJSON+`]}}`, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE LOCAL_AGGREGATE protocol instance",
		processDeleteRequest(niLocalAggrProtoUrl, false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the IPv4 aggregate row removed",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv4_unicast|10.10.0.0/16", map[string]interface{}{}, false))
	t.Run("Verify the IPv6 aggregate row removed",
		verifyDbResult(rclient, "BGP_GLOBALS_AF_AGGREGATE_ADDR|default|ipv6_unicast|2001:db8::/32", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
