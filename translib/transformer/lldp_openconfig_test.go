////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                     //
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

//go:build testapp
// +build testapp

package transformer_test

import (
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const testIfName = "Ethernet0"

var (
	lldpGlobalCfgData = map[string]interface{}{
		"enabled":                      "true",
		"hello_time":                   "30",
		"system_name":                  "sonic-switch",
		"system_description":           "SONiC LLDP test",
		"supp_mgmt_address_tlv":        "true",
		"supp_system_capabilities_tlv": "false",
	}

	lldpLocChassisData = map[string]interface{}{
		"lldp_loc_chassis_id":         "00:11:22:33:44:55",
		"lldp_loc_chassis_id_subtype": "4",
		"lldp_loc_sys_name":           "loc-chassis-name",
		"lldp_loc_sys_desc":           "loc chassis desc",
	}

	lldpNeighborEntryData = map[string]interface{}{
		"lldp_rem_index":              "1",
		"lldp_rem_chassis_id":         "00:aa:00:00:00:01",
		"lldp_rem_chassis_id_subtype": "4",
		"lldp_rem_man_addr":           "172.17.2.1,fd00::1",
		"lldp_rem_port_id":            "Ethernet0",
		"lldp_rem_port_desc":          "Port 1/1",
		"lldp_rem_port_id_subtype":    "7",
		"lldp_rem_sys_cap_enabled":    "20 00",
		"lldp_rem_sys_cap_supported":  "20 00",
		"lldp_rem_sys_desc":           "SONiC simulator 1",
		"lldp_rem_sys_name":           "sonic1",
		"lldp_rem_time_mark":          "5000",
	}
)

func lldpGlobalCfgPrereq() map[string]interface{} {
	return map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": lldpGlobalCfgData,
		},
	}
}

func lldpLocChassisPrereq() map[string]interface{} {
	return map[string]interface{}{
		"LLDP_LOC_CHASSIS": map[string]interface{}{
			"": lldpLocChassisData,
		},
	}
}

func lldpNeighborPrereq() map[string]interface{} {
	return map[string]interface{}{
		"LLDP_ENTRY_TABLE": map[string]interface{}{
			testIfName: lldpNeighborEntryData,
		},
	}
}

func lldpPortPrereq(enabled string) map[string]interface{} {
	return map[string]interface{}{
		"LLDP_PORT": map[string]interface{}{
			testIfName: map[string]interface{}{
				"enabled": enabled,
			},
		},
	}
}

func lldpCleanupTables() map[string]interface{} {
	return map[string]interface{}{
		"LLDP":             map[string]interface{}{"GLOBAL": ""},
		"LLDP_PORT":        map[string]interface{}{testIfName: ""},
		"LLDP_ENTRY_TABLE": map[string]interface{}{testIfName: ""},
		"LLDP_LOC_CHASSIS": map[string]interface{}{"": ""},
	}
}

func loadLldpFixture(t *testing.T, withNeighbor, withPort bool, portEnabled string) {
	t.Helper()
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)

	loadDB(db.ConfigDB, lldpGlobalCfgPrereq())
	loadDB(db.ApplDB, lldpLocChassisPrereq())
	if withNeighbor {
		loadDB(db.ApplDB, lldpNeighborPrereq())
	}
	if withPort {
		loadDB(db.ConfigDB, lldpPortPrereq(portEnabled))
	}
	time.Sleep(1 * time.Second)
}

func unloadLldpFixture() {
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
}

func Test_openconfig_lldp_config(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ GET global LLDP config ++++++++++++")
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/config"
	expectedGetJSON := `{"openconfig-lldp:config":{"chassis-id":"00:11:22:33:44:55","chassis-id-type":"MAC_ADDRESS","enabled":true,"hello-timer":30,"suppress-tlv-advertisement":["openconfig-lldp-types:MANAGEMENT_ADDRESS"],"system-description":"SONiC LLDP test","system-name":"sonic-switch"}}`
	t.Run("GET global LLDP config", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET global LLDP config after DELETE ++++++++++++")
	url = "/openconfig-lldp:lldp/config/system-name"
	expectedGetJSON = `{"openconfig-lldp:system-name":"loc-chassis-name"}`
	t.Run("GET global system-name fallback from loc chassis", processGetRequest(url, nil, expectedGetJSON, false))
}
func Test_openconfig_lldp_state(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ GET global LLDP state ++++++++++++")
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/state"
	expectedGetJSON := `{"openconfig-lldp:state":{"chassis-id":"00:11:22:33:44:55","chassis-id-type":"MAC_ADDRESS","counters":{},"enabled":true,"hello-timer":30,"suppress-tlv-advertisement":["openconfig-lldp-types:MANAGEMENT_ADDRESS"],"system-description":"SONiC LLDP test","system-name":"sonic-switch"}}`
	t.Run("GET global LLDP state", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET global LLDP state counters ++++++++++++")
	url = "/openconfig-lldp:lldp/state/counters"
	expectedGetJSON = `{"openconfig-lldp:counters":{}}`
	t.Run("GET global LLDP state counters", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

func Test_openconfig_lldp_interface_config(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ GET per-interface LLDP config (default enabled) ++++++++++++")
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config"
	expectedGetJSON := `{"openconfig-lldp:config":{"enabled":true,"name":"Ethernet0"}}`
	t.Run("GET interface config default enabled", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

func Test_openconfig_lldp_interface_state(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ GET per-interface LLDP state ++++++++++++")
	loadLldpFixture(t, false, true, "false")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/state"
	expectedGetJSON := `{"openconfig-lldp:state":{"enabled":false,"name":"Ethernet0"}}`
	t.Run("GET interface state", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET per-interface LLDP state counters ++++++++++++")
	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/state/counters"
	expectedGetJSON = `{"openconfig-lldp:counters":{}}`
	t.Run("GET interface state counters", processGetRequest(url, nil, expectedGetJSON, false))
}
func Test_openconfig_lldp_neighbors(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ GET LLDP neighbor state ++++++++++++")
	loadLldpFixture(t, true, true, "true")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/state"
	expectedGetJSON := `{"openconfig-lldp:state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}`
	t.Run("GET neighbor state", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET LLDP neighbor capability ++++++++++++")
	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/capabilities/capability[name=MAC_BRIDGE]/state"
	expectedGetJSON = `{"openconfig-lldp:state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}`
	t.Run("GET neighbor MAC_BRIDGE capability", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET single LLDP interface with neighbor ++++++++++++")
	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]"
	expectedGetJSON = `{"openconfig-lldp:interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}`
	t.Run("GET single interface with neighbor", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET all LLDP interfaces ++++++++++++")
	url = "/openconfig-lldp:lldp/interfaces"
	expectedGetJSON = `{"openconfig-lldp:interfaces":{"interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}}`
	t.Run("GET all interfaces", processGetRequest(url, nil, expectedGetJSON, false))
}
func Test_openconfig_lldp_interface_from_port_only(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ GET interface listed from LLDP_PORT without neighbor ++++++++++++")
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	loadDB(db.ConfigDB, lldpPortPrereq("false"))
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config"
	expectedGetJSON := `{"openconfig-lldp:config":{"enabled":false,"name":"Ethernet0"}}`
	t.Run("GET interface from LLDP_PORT only", processGetRequest(url, nil, expectedGetJSON, false))
}
func Test_openconfig_lldp_interface_neighbor_without_port(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ GET keyed interface with neighbor but no LLDP_PORT ++++++++++++")
	loadLldpFixture(t, true, false, "")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]"
	expectedGetJSON := `{"openconfig-lldp:interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}`
	t.Run("GET keyed interface neighbor without LLDP_PORT", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET keyed interface config default without LLDP_PORT ++++++++++++")
	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config"
	expectedGetJSON = `{"openconfig-lldp:config":{"enabled":true,"name":"Ethernet0"}}`
	t.Run("GET keyed interface config without LLDP_PORT", processGetRequest(url, nil, expectedGetJSON, false))
}
func Test_openconfig_lldp_keyed_paths(t *testing.T) {
	var url string

	t.Log("\n\n+++++++++++++ Keyed GET paths (ygot pre-populates list keys) ++++++++++++")
	loadLldpFixture(t, true, true, "true")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]"
	expectedGetJSON := `{"openconfig-lldp:interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}`
	t.Run("GET keyed interface container", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config/enabled"
	expectedGetJSON = `{"openconfig-lldp:enabled":true}`
	t.Run("GET keyed interface config/enabled", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/state/enabled"
	expectedGetJSON = `{"openconfig-lldp:enabled":true}`
	t.Run("GET keyed interface state/enabled", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]"
	expectedGetJSON = `{"openconfig-lldp:neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]}`
	t.Run("GET keyed neighbor container", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/state"
	expectedGetJSON = `{"openconfig-lldp:state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}`
	t.Run("GET keyed neighbor state", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/capabilities/capability[name=MAC_BRIDGE]/state"
	expectedGetJSON = `{"openconfig-lldp:state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}`
	t.Run("GET keyed capability state", processGetRequest(url, nil, expectedGetJSON, false))
}
