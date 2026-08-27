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
)

// Tests for the OpenConfig AFT state mapping over the two APPL_DB tables
// fpmsyncd writes, ROUTE_TABLE and NEXTHOP_GROUP_TABLE.
//
// Every assertion below names a list key. That is deliberate, for two reasons.
// The tables are read whole rather than by key, so a whole list assertion would
// also see any row another test or the surrounding environment left behind, and
// separately the infrastructure materializes a placeholder list entry keyed on
// the enclosing network instance name whenever the list key is a string, which
// puts a spurious {"prefix":"default"} in every unkeyed ipv4-entry response.
// That placeholder is not produced by this mapping - the pre-existing
// static-routes list shows the same entry - and it is being tracked separately.
// Keying each request keeps these tests about the mapping rather than about
// either of those.

const niAftsUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/afts"
const niAftsNhGrpsUrl = niAftsUrl + "/next-hop-groups"
const niAftsV4Url = niAftsUrl + "/ipv4-unicast"

// loadNiAftsNhGrp writes one NEXTHOP_GROUP_TABLE row the way fpmsyncd does: the
// members are parallel comma separated lists, one element per member, in the
// same order across every field.
func loadNiAftsNhGrp(groupId string, fields map[string]interface{}) {
	loadDB(db.ApplDB, map[string]interface{}{
		"NEXTHOP_GROUP_TABLE": map[string]interface{}{groupId: fields},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiAftsNhGrp(groupId string) {
	unloadDB(db.ApplDB, map[string]interface{}{
		"NEXTHOP_GROUP_TABLE": map[string]interface{}{groupId: ""},
	})
}

// loadNiAftsRoute writes one ROUTE_TABLE row. The key is the prefix in the
// default VRF and carries a VRF component otherwise.
func loadNiAftsRoute(key string, fields map[string]interface{}) {
	loadDB(db.ApplDB, map[string]interface{}{
		"ROUTE_TABLE": map[string]interface{}{key: fields},
	})
	time.Sleep(1 * time.Second)
}

func unloadNiAftsRoute(key string) {
	unloadDB(db.ApplDB, map[string]interface{}{
		"ROUTE_TABLE": map[string]interface{}{key: ""},
	})
}

// A group row does not imply multipath. FRR encodes a protocol owned entry as a
// group whatever its size, so a row holding a single member is ordinary and has
// to be reported as a group of one rather than skipped or flattened.
func Test_openconfig_network_instance_afts_next_hop_group_of_one(t *testing.T) {
	loadNiAftsNhGrp("900", map[string]interface{}{
		"nexthop":    "10.9.9.1",
		"ifname":     "Ethernet1",
		"nexthop_id": "201",
		"weight":     "1",
	})

	url := niAftsNhGrpsUrl + "/next-hop-group[id=900]"
	expectedGetJSON := `{"openconfig-network-instance:next-hop-group":[{"id":"900",` +
		`"next-hops":{"next-hop":[{"index":"201","state":{"index":"201","weight":"1"}}]},` +
		`"state":{"id":"900"}}]}`
	t.Run("GET AFT next-hop-group holding a single member",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsNhGrp("900")
}

func Test_openconfig_network_instance_afts_next_hop_group_members(t *testing.T) {
	loadNiAftsNhGrp("901", map[string]interface{}{
		"nexthop":    "10.9.9.1,10.9.9.2",
		"ifname":     "Ethernet1,Ethernet2",
		"nexthop_id": "202,203",
		"weight":     "1,3",
	})

	url := niAftsNhGrpsUrl + "/next-hop-group[id=901]"
	expectedGetJSON := `{"openconfig-network-instance:next-hop-group":[{"id":"901",` +
		`"next-hops":{"next-hop":[` +
		`{"index":"202","state":{"index":"202","weight":"1"}},` +
		`{"index":"203","state":{"index":"203","weight":"3"}}]},` +
		`"state":{"id":"901"}}]}`
	t.Run("GET AFT next-hop-group with weighted members",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// Members are keyed on the next-hop id, so naming one must report that
	// member alone rather than every member of its group.
	url = niAftsNhGrpsUrl + "/next-hop-group[id=901]/next-hops/next-hop[index=203]"
	expectedGetJSON = `{"openconfig-network-instance:next-hop":[` +
		`{"index":"203","state":{"index":"203","weight":"3"}}]}`
	t.Run("GET one AFT next-hop-group member reports that member alone",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsNhGrp("901")
}

// fpmsyncd offsets the kernel weight, which counts from zero, so the lowest it
// ever writes is 1 and it omits the field when FRR gave the group no weights. A
// zero or absent weight therefore means the group is unweighted rather than
// weighted at zero, and the leaf has to stay unset.
func Test_openconfig_network_instance_afts_next_hop_group_unweighted(t *testing.T) {
	loadNiAftsNhGrp("902", map[string]interface{}{
		"nexthop":    "10.9.9.1,10.9.9.2",
		"ifname":     "Ethernet1,Ethernet2",
		"nexthop_id": "204,205",
		"weight":     "0,0",
	})

	url := niAftsNhGrpsUrl + "/next-hop-group[id=902]"
	expectedGetJSON := `{"openconfig-network-instance:next-hop-group":[{"id":"902",` +
		`"next-hops":{"next-hop":[` +
		`{"index":"204","state":{"index":"204"}},` +
		`{"index":"205","state":{"index":"205"}}]},` +
		`"state":{"id":"902"}}]}`
	t.Run("GET AFT next-hop-group with zero weights omits the weight leaf",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsNhGrp("902")

	loadNiAftsNhGrp("903", map[string]interface{}{
		"nexthop":    "10.9.9.3",
		"ifname":     "Ethernet3",
		"nexthop_id": "206",
	})

	url = niAftsNhGrpsUrl + "/next-hop-group[id=903]"
	expectedGetJSON = `{"openconfig-network-instance:next-hop-group":[{"id":"903",` +
		`"next-hops":{"next-hop":[{"index":"206","state":{"index":"206"}}]},` +
		`"state":{"id":"903"}}]}`
	t.Run("GET AFT next-hop-group with no weight field omits the weight leaf",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsNhGrp("903")
}

func Test_openconfig_network_instance_afts_next_hop_group_leaves(t *testing.T) {
	loadNiAftsNhGrp("904", map[string]interface{}{
		"nexthop":    "10.9.9.4",
		"ifname":     "Ethernet4",
		"nexthop_id": "207",
		"weight":     "2",
	})

	grpUrl := niAftsNhGrpsUrl + "/next-hop-group[id=904]"

	expectedGetJSON := `{"openconfig-network-instance:id":"904"}`
	t.Run("GET AFT next-hop-group id list key",
		processGetRequest(grpUrl+"/id", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET AFT next-hop-group state/id",
		processGetRequest(grpUrl+"/state/id", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:state":{"id":"904"}}`
	t.Run("GET AFT next-hop-group state container",
		processGetRequest(grpUrl+"/state", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	memberUrl := grpUrl + "/next-hops/next-hop[index=207]"

	expectedGetJSON = `{"openconfig-network-instance:index":"207"}`
	t.Run("GET AFT next-hop-group member index list key",
		processGetRequest(memberUrl+"/index", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("GET AFT next-hop-group member state/index",
		processGetRequest(memberUrl+"/state/index", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	expectedGetJSON = `{"openconfig-network-instance:weight":"2"}`
	t.Run("GET AFT next-hop-group member state/weight",
		processGetRequest(memberUrl+"/state/weight", nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsNhGrp("904")
}

// The next-hop list is the leafref target for every group member, and it is the
// union of both tables because a next-hop reaches APPL_DB either inline on a
// route or as a member of a group. FRR content-hashes its entries, so the same
// next-hop carries one id in both places and the union has to report it once.
func Test_openconfig_network_instance_afts_next_hops_union_dedups(t *testing.T) {
	loadNiAftsNhGrp("905", map[string]interface{}{
		"nexthop":    "10.9.9.5,0.0.0.0",
		"ifname":     "Ethernet5,Loopback9",
		"nexthop_id": "208,209",
		"weight":     "1,1",
	})
	// The same next-hop id, inline on a route this time. It must not produce a
	// second list entry.
	loadNiAftsRoute("20.60.0.0/16", map[string]interface{}{
		"nexthop":    "10.9.9.5",
		"ifname":     "Ethernet5",
		"nexthop_id": "208",
		"protocol":   "0xc4",
	})

	url := niAftsUrl + "/next-hops/next-hop[index=208]"
	expectedGetJSON := `{"openconfig-network-instance:next-hop":[{"index":"208",` +
		`"interface-ref":{"state":{"interface":"Ethernet5"}},` +
		`"state":{"index":"208","ip-address":"10.9.9.5"}}]}`
	t.Run("GET AFT next-hop present both inline and as a group member",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	// fpmsyncd writes the unspecified address when a next-hop has no gateway,
	// which is how a directly connected next-hop is published. Reporting
	// 0.0.0.0 would be misleading, so the address leaf stays unset and the
	// interface carries the meaning on its own.
	url = niAftsUrl + "/next-hops/next-hop[index=209]/state"
	expectedGetJSON = `{"openconfig-network-instance:state":{"index":"209"}}`
	t.Run("GET AFT next-hop state omits an unspecified ip-address",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsRoute("20.60.0.0/16")
	unloadNiAftsNhGrp("905")
}

// state/next-hop-group is set only for a route FRR resolved through a group. A
// route holding its next-hop inline must report no group rather than one
// synthesized on its behalf, so the leaf is absent exactly when no group exists.
func Test_openconfig_network_instance_afts_ipv4_entry_next_hop_group(t *testing.T) {
	loadNiAftsRoute("20.40.0.0/16", map[string]interface{}{
		"nexthop":    "10.9.9.1",
		"ifname":     "Ethernet1",
		"nexthop_id": "210",
		"protocol":   "0xc4",
	})
	loadNiAftsRoute("20.50.0.0/16", map[string]interface{}{
		"nexthop_group": "906",
		"protocol":      "0xc4",
	})

	url := niAftsV4Url + "/ipv4-entry[prefix=20.40.0.0/16]/state"
	expectedGetJSON := `{"openconfig-network-instance:state":{"prefix":"20.40.0.0/16"}}`
	t.Run("GET AFT ipv4 entry state omits next-hop-group for an inline route",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = niAftsV4Url + "/ipv4-entry[prefix=20.50.0.0/16]/state"
	expectedGetJSON = `{"openconfig-network-instance:state":{"next-hop-group":"906","prefix":"20.50.0.0/16"}}`
	t.Run("GET AFT ipv4 entry state reports next-hop-group for a grouped route",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = niAftsV4Url + "/ipv4-entry[prefix=20.50.0.0/16]/state/next-hop-group"
	expectedGetJSON = `{"openconfig-network-instance:next-hop-group":"906"}`
	t.Run("GET AFT ipv4 entry state/next-hop-group leaf",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsRoute("20.40.0.0/16")
	unloadNiAftsRoute("20.50.0.0/16")
}

// fpmsyncd keys a host route by the bare address with no mask length, which is
// not a valid prefix. The key has to be completed to the mask covering the one
// address it names, otherwise the list key is not a value the model accepts.
func Test_openconfig_network_instance_afts_ipv4_entry_host_route_mask_completed(t *testing.T) {
	loadNiAftsRoute("20.1.0.32", map[string]interface{}{
		"nexthop":    "0.0.0.0",
		"ifname":     "Loopback0",
		"nexthop_id": "211",
		"protocol":   "kernel",
	})

	url := niAftsV4Url + "/ipv4-entry[prefix=20.1.0.32/32]/prefix"
	expectedGetJSON := `{"openconfig-network-instance:prefix":"20.1.0.32/32"}`
	t.Run("GET AFT ipv4 host route reports a full length prefix",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = niAftsV4Url + "/ipv4-entry[prefix=20.1.0.32/32]/state"
	expectedGetJSON = `{"openconfig-network-instance:state":{"prefix":"20.1.0.32/32"}}`
	t.Run("GET AFT ipv4 host route state mirrors the completed prefix",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsRoute("20.1.0.32")
}

// ROUTE_TABLE holds both address families, so the entry mapping filters on the
// family of the prefix. An IPv6 route must not surface in the IPv4 list.
func Test_openconfig_network_instance_afts_ipv4_entry_address_family_filtered(t *testing.T) {
	loadNiAftsRoute("fc00:9::32", map[string]interface{}{
		"nexthop":    "::",
		"ifname":     "Loopback0",
		"nexthop_id": "212",
		"protocol":   "kernel",
	})

	url := niAftsV4Url + "/ipv4-entry[prefix=fc00:9::32/128]/state"
	t.Run("GET AFT ipv4 entry does not report an IPv6 route",
		processGetRequest(url, nil, emptyJson, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsRoute("fc00:9::32")
}

// A prefix with no route behind it is an empty result rather than an error, so
// that a sample subscription taken out on a route that has gone away is left
// reporting nothing instead of being torn down.
func Test_openconfig_network_instance_afts_ipv4_entry_unknown_prefix(t *testing.T) {
	url := niAftsV4Url + "/ipv4-entry[prefix=203.0.113.0/24]/state"
	t.Run("GET AFT ipv4 entry for a prefix with no route",
		processGetRequest(url, nil, emptyJson, false))
	time.Sleep(1 * time.Second)
}

// NEXTHOP_GROUP_TABLE is not VRF scoped at all, because FRR allocates its ids
// from one global space, and ROUTE_TABLE keys carry a VRF component outside the
// default VRF. Only the default network instance can therefore be attributed,
// and any other one reports an empty list rather than failing.
func Test_openconfig_network_instance_afts_non_default_network_instance_empty(t *testing.T) {
	loadNiAftsNhGrp("907", map[string]interface{}{
		"nexthop":    "10.9.9.7",
		"ifname":     "Ethernet7",
		"nexthop_id": "213",
		"weight":     "1",
	})

	url := "/openconfig-network-instance:network-instances/network-instance[name=Vrf1]/afts/next-hop-groups"
	t.Run("GET AFT next-hop-groups on a non-default network instance",
		processGetRequest(url, nil, emptyJson, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsNhGrp("907")
}

// A VRF scoped ROUTE_TABLE key rejoins into something that is not a prefix,
// which is how it is told apart from an IPv6 prefix, whose components rejoin
// into one. The row belongs to a network instance this mapping cannot attribute
// and has to be left out of the default one.
func Test_openconfig_network_instance_afts_vrf_scoped_route_excluded(t *testing.T) {
	loadNiAftsRoute("Vrf1:20.70.0.0/16", map[string]interface{}{
		"nexthop":    "10.9.9.8",
		"ifname":     "Ethernet8",
		"nexthop_id": "214",
		"protocol":   "0xc4",
	})

	url := niAftsV4Url + "/ipv4-entry[prefix=20.70.0.0/16]/state"
	t.Run("GET AFT ipv4 entry excludes a VRF scoped ROUTE_TABLE row",
		processGetRequest(url, nil, emptyJson, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsRoute("Vrf1:20.70.0.0/16")
}

// A group id with no row behind it is an empty result rather than an error, for
// the same reason an unknown prefix is. The infrastructure still materializes
// the entry named by the request, so the id it was asked for comes back with
// nothing under it.
func Test_openconfig_network_instance_afts_next_hop_group_unknown_id(t *testing.T) {
	url := niAftsNhGrpsUrl + "/next-hop-group[id=999999]"
	expectedGetJSON := `{"openconfig-network-instance:next-hop-group":[{"id":"999999"}]}`
	t.Run("GET AFT next-hop-group for an id with no row",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)
}

// The afts container carries a mapping of its own because the infrastructure
// stops descending once a subtree mapping claims a node, so the container has to
// reach its mapped children itself. Only the group list is asserted here: it is
// keyed on an integer, so unlike the entry list it is not joined by the
// placeholder entry described at the top of this file.
func Test_openconfig_network_instance_afts_container_dispatch(t *testing.T) {
	loadNiAftsNhGrp("908", map[string]interface{}{
		"nexthop":    "10.9.9.9",
		"ifname":     "Ethernet9",
		"nexthop_id": "215",
		"weight":     "1",
	})

	url := niAftsUrl + "/next-hop-groups/next-hop-group[id=908]/state/id"
	expectedGetJSON := `{"openconfig-network-instance:id":"908"}`
	t.Run("GET through the afts container reaches the next-hop-group list",
		processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	unloadNiAftsNhGrp("908")
}
