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

const niIntfNiPfx = "/openconfig-network-instance:network-instances/network-instance"
const niIntfRedUrl = niIntfNiPfx + "[name=Vrf_red]/interfaces"
const niIntfBlueUrl = niIntfNiPfx + "[name=Vrf_blue]/interfaces"
const niIntfDefaultUrl = niIntfNiPfx + "[name=default]/interfaces"

func niIntfPreReq() (map[string]interface{}, map[string]interface{}) {
	preReqMap := map[string]interface{}{
		"PORT": map[string]interface{}{
			"Ethernet120": map[string]interface{}{"admin_status": "up"},
			"Ethernet124": map[string]interface{}{"admin_status": "up"},
		},
		"VRF": map[string]interface{}{
			"Vrf_red":  map[string]interface{}{"NULL": "NULL"},
			"Vrf_blue": map[string]interface{}{"NULL": "NULL"},
		},
	}
	cleanupTbl := map[string]interface{}{
		"INTERFACE":          map[string]interface{}{"Ethernet120": "", "Ethernet124": "", "Ethernet120|10.0.0.1/31": ""},
		"LOOPBACK_INTERFACE": map[string]interface{}{"Loopback7": ""},
		"VLAN_SUB_INTERFACE": map[string]interface{}{"Ethernet124.10": ""},
		"VRF":                map[string]interface{}{"Vrf_red": "", "Vrf_blue": ""},
		"PORT":               map[string]interface{}{"Ethernet120": "", "Ethernet124": ""},
	}
	return preReqMap, cleanupTbl
}

func niIntfRow(table, key string, fields map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{table: map[string]interface{}{key: fields}}
}

// Binding an interface writes vrf_name on its base row, the default instance
// none; config and state are derived from the id, at every level.
func Test_openconfig_network_instance_interfaces_config(t *testing.T) {
	preReqMap, cleanupTbl := niIntfPreReq()
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	e120 := niIntfRedUrl + "/interface[id=Ethernet120]"
	t.Run("PATCH network-instance interface config",
		processSetRequest(e120+"/config", `{"id":"Ethernet120","interface":"Ethernet120"}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the interface is bound to the VRF",
		verifyDbResult(rclient, "INTERFACE|Ethernet120",
			niIntfRow("INTERFACE", "Ethernet120", map[string]interface{}{"vrf_name": "Vrf_red"}), false))
	time.Sleep(1 * time.Second)

	e120JSON := `{"config":{"id":"Ethernet120","interface":"Ethernet120"},"id":"Ethernet120",` +
		`"state":{"id":"Ethernet120","interface":"Ethernet120"}}`
	t.Run("GET network-instance interface instance",
		processGetRequest(e120, nil, `{"openconfig-network-instance:interface":[`+e120JSON+`]}`, false))
	t.Run("GET network-instance interface config interface leaf",
		processGetRequest(e120+"/config/interface", nil, `{"openconfig-network-instance:interface":"Ethernet120"}`, false))
	t.Run("GET network-instance interface state",
		processGetRequest(e120+"/state", nil, `{"openconfig-network-instance:state":{"id":"Ethernet120","interface":"Ethernet120"}}`, false))
	t.Run("GET network-instance interface state id leaf",
		processGetRequest(e120+"/state/id", nil, `{"openconfig-network-instance:id":"Ethernet120"}`, false))
	t.Run("GET network-instance interfaces container",
		processGetRequest(niIntfRedUrl, nil, `{"openconfig-network-instance:interfaces":{"interface":[`+e120JSON+`]}}`, false))
	t.Run("GET network-instance interface bound elsewhere is not found",
		processGetRequest(niIntfBlueUrl+"/interface[id=Ethernet120]", nil, "", true, tlerr.NotFoundError{Format: "network-instance Vrf_blue has no interface Ethernet120"}))
	time.Sleep(1 * time.Second)

	t.Run("PATCH network-instance interface config interface leaf again",
		processSetRequest(e120+"/config/interface", `{"openconfig-network-instance:interface":"Ethernet120"}`, "PATCH", false, nil))
	t.Run("PATCH binding the interface to another VRF rejected",
		processSetRequest(niIntfBlueUrl+"/interface[id=Ethernet120]/config", `{"id":"Ethernet120"}`, "PATCH", true,
			tlerr.InvalidArgsError{Format: "interface \"Ethernet120\" is bound to network-instance \"Vrf_red\"; remove it from that network instance first"}))
	t.Run("PATCH binding the interface to the default instance rejected",
		processSetRequest(niIntfDefaultUrl+"/interface[id=Ethernet120]/config/id", `{"openconfig-network-instance:id":"Ethernet120"}`, "PATCH", true,
			tlerr.InvalidArgsError{Format: "interface \"Ethernet120\" is bound to network-instance \"Vrf_red\"; remove it from that network instance first"}))
	time.Sleep(1 * time.Second)

	t.Run("PATCH default instance interfaces container with a loopback",
		processSetRequest(niIntfDefaultUrl, `{"interface":[{"id":"Loopback7","config":{"id":"Loopback7","interface":"Loopback7"}}]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the loopback is in the default instance",
		verifyDbResult(rclient, "LOOPBACK_INTERFACE|Loopback7",
			niIntfRow("LOOPBACK_INTERFACE", "Loopback7", map[string]interface{}{"NULL": "NULL"}), false))
	t.Run("GET default instance interface config",
		processGetRequest(niIntfDefaultUrl+"/interface[id=Loopback7]/config", nil,
			`{"openconfig-network-instance:config":{"id":"Loopback7","interface":"Loopback7"}}`, false))
	time.Sleep(1 * time.Second)

	e124 := niIntfRedUrl + "/interface[id=Ethernet124]"
	t.Run("PATCH interface not matching the id rejected",
		processSetRequest(e124+"/config", `{"id":"Ethernet124","interface":"Ethernet120"}`, "PATCH", true,
			tlerr.InvalidArgsError{Format: "interface \"Ethernet120\" does not match the interface \"Ethernet124\" that id \"Ethernet124\" names"}))
	t.Run("PATCH subinterface 0 rejected",
		processSetRequest(e124+"/config", `{"id":"Ethernet124","interface":"Ethernet124","subinterface":0}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "subinterface 0 is not supported; interface \"Ethernet124\" is bound by its id alone"}))
	t.Run("PATCH associated-address-families rejected",
		processSetRequest(e124+"/config", `{"id":"Ethernet124","associated-address-families":["openconfig-types:IPV4"]}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "network-instance interface payload carries leaves that have no CONFIG_DB mapping: interfaces/interface[Ethernet124]/config/associated-address-families"}))
	t.Run("PATCH an instance with no VRF rejected",
		processSetRequest(niIntfNiPfx+"[name=Vrf_green]/interfaces/interface[id=Ethernet124]/config", `{"id":"Ethernet124"}`, "PATCH", true,
			tlerr.InvalidArgsError{Format: "network-instance \"Vrf_green\" has no VRF in CONFIG_DB; create the VRF before binding interfaces to it"}))
	t.Run("PATCH an interface type that has no L3 table rejected",
		processSetRequest(niIntfRedUrl+"/interface[id=eth0]/config", `{"id":"eth0"}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "interface \"eth0\" cannot be bound to a network instance: only Ethernet, PortChannel, Vlan and Loopback interfaces and subinterfaces can"}))
	time.Sleep(1 * time.Second)

	sub := niIntfRedUrl + "/interface[id=Ethernet124.10]"
	t.Run("PATCH network-instance subinterface config",
		processSetRequest(sub+"/config", `{"id":"Ethernet124.10","interface":"Ethernet124","subinterface":10}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the subinterface is bound to the VRF",
		verifyDbResult(rclient, "VLAN_SUB_INTERFACE|Ethernet124.10",
			niIntfRow("VLAN_SUB_INTERFACE", "Ethernet124.10", map[string]interface{}{"vrf_name": "Vrf_red"}), false))
	t.Run("GET network-instance subinterface state",
		processGetRequest(sub+"/state", nil,
			`{"openconfig-network-instance:state":{"id":"Ethernet124.10","interface":"Ethernet124","subinterface":10}}`, false))
	t.Run("PATCH subinterface index not matching the id rejected",
		processSetRequest(sub+"/config/subinterface", `{"openconfig-network-instance:subinterface":20}`, "PATCH", true,
			tlerr.InvalidArgsError{Format: "subinterface 20 does not match the index 10 that id \"Ethernet124.10\" names"}))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance subinterface",
		processDeleteRequest(sub, false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the subinterface row removed",
		verifyDbResult(rclient, "VLAN_SUB_INTERFACE|Ethernet124.10", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// Removing an interface deletes its base row; it is refused while the
// interface has an address. A REPLACE of the list removes what it leaves out.
func Test_openconfig_network_instance_interfaces_delete(t *testing.T) {
	preReqMap, cleanupTbl := niIntfPreReq()
	preReqMap["INTERFACE"] = map[string]interface{}{
		"Ethernet120":             map[string]interface{}{"vrf_name": "Vrf_red"},
		"Ethernet120|10.0.0.1/31": map[string]interface{}{"NULL": "NULL"},
	}
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	e120 := niIntfRedUrl + "/interface[id=Ethernet120]"
	t.Run("DELETE network-instance interface with an address rejected",
		processDeleteRequest(e120, true,
			tlerr.InvalidArgsError{Format: "interface \"Ethernet120\" still has IP addresses; remove them before removing it from network-instance \"Vrf_red\""}))
	time.Sleep(1 * time.Second)
	unloadDB(db.ConfigDB, map[string]interface{}{"INTERFACE": map[string]interface{}{"Ethernet120|10.0.0.1/31": ""}})
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance interface from another instance is a no-op",
		processDeleteRequest(niIntfBlueUrl+"/interface[id=Ethernet120]", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the interface stays bound",
		verifyDbResult(rclient, "INTERFACE|Ethernet120",
			niIntfRow("INTERFACE", "Ethernet120", map[string]interface{}{"vrf_name": "Vrf_red"}), false))
	t.Run("DELETE network-instance interface config id leaf",
		processDeleteRequest(e120+"/config/id", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the base row removed",
		verifyDbResult(rclient, "INTERFACE|Ethernet120", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH the interface into another VRF once removed",
		processSetRequest(niIntfBlueUrl+"/interface[id=Ethernet120]", `{"id":"Ethernet120","config":{"id":"Ethernet120"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the interface is bound to the other VRF",
		verifyDbResult(rclient, "INTERFACE|Ethernet120",
			niIntfRow("INTERFACE", "Ethernet120", map[string]interface{}{"vrf_name": "Vrf_blue"}), false))
	time.Sleep(1 * time.Second)

	t.Run("PUT network-instance interfaces container",
		processSetRequest(niIntfBlueUrl, `{"interface":[{"id":"Ethernet124","config":{"id":"Ethernet124"}}]}`, "PUT", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the interface left out is removed",
		verifyDbResult(rclient, "INTERFACE|Ethernet120", map[string]interface{}{}, false))
	t.Run("Verify the interface in the payload is bound",
		verifyDbResult(rclient, "INTERFACE|Ethernet124",
			niIntfRow("INTERFACE", "Ethernet124", map[string]interface{}{"vrf_name": "Vrf_blue"}), false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance interfaces container",
		processDeleteRequest(niIntfBlueUrl, false))
	time.Sleep(1 * time.Second)
	t.Run("Verify every interface of the instance removed",
		verifyDbResult(rclient, "INTERFACE|Ethernet124", map[string]interface{}{}, false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}

// The network-instance level reports the interfaces bound to it and takes them
// in its payload. A DELETE of the instance leaves them bound.
func Test_openconfig_network_instance_interfaces_hierarchy(t *testing.T) {
	preReqMap, cleanupTbl := niIntfPreReq()
	unloadDB(db.ConfigDB, cleanupTbl)
	loadDB(db.ConfigDB, preReqMap)
	time.Sleep(1 * time.Second)

	t.Run("PATCH network-instance with interfaces",
		processSetRequest(niIntfNiPfx+"[name=Vrf_red]",
			`{"name":"Vrf_red","interfaces":{"interface":[{"id":"Ethernet120","config":{"id":"Ethernet120"}}]}}`,
			"PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify the interface is bound to the VRF",
		verifyDbResult(rclient, "INTERFACE|Ethernet120",
			niIntfRow("INTERFACE", "Ethernet120", map[string]interface{}{"vrf_name": "Vrf_red"}), false))
	time.Sleep(1 * time.Second)

	t.Run("GET network-instance with its interfaces",
		processGetRequest(niIntfNiPfx+"[name=Vrf_red]", nil,
			`{"openconfig-network-instance:network-instance":[{"config":{"name":"Vrf_red","type":"openconfig-network-instance-types:L3VRF"},`+
				`"interfaces":{"interface":[{"config":{"id":"Ethernet120","interface":"Ethernet120"},"id":"Ethernet120",`+
				`"state":{"id":"Ethernet120","interface":"Ethernet120"}}]},"name":"Vrf_red",`+
				`"state":{"name":"Vrf_red","type":"openconfig-network-instance-types:L3VRF"}}]}`, false))
	time.Sleep(1 * time.Second)

	t.Run("DELETE network-instance",
		processDeleteRequest(niIntfNiPfx+"[name=Vrf_red]", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify the interface stays bound",
		verifyDbResult(rclient, "INTERFACE|Ethernet120",
			niIntfRow("INTERFACE", "Ethernet120", map[string]interface{}{"vrf_name": "Vrf_red"}), false))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, cleanupTbl)
}
