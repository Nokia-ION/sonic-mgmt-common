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
		"LLDP_CUSTOM_TLV":  map[string]interface{}{"vendor-serial": ""},
		"MGMT_PORT":        map[string]interface{}{"eth0": ""},
		"LLDP_ENTRY_TABLE": map[string]interface{}{testIfName: ""},
		"LLDP_LOC_CHASSIS": map[string]interface{}{"": ""},
	}
}

func lldpCountersCleanup() map[string]interface{} {
	return map[string]interface{}{
		"LLDP_STATISTICS": map[string]interface{}{
			"GLOBAL":   "",
			testIfName: "",
		},
	}
}

func loadLldpFixture(t *testing.T, withNeighbor, withPort bool, portEnabled string) {
	t.Helper()
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	unloadDB(db.CountersDB, lldpCountersCleanup())

	loadDB(db.ConfigDB, lldpGlobalCfgPrereq())
	loadDB(db.ConfigDB, map[string]interface{}{
		"MGMT_PORT": map[string]interface{}{
			"eth0": map[string]interface{}{"admin_status": "up"},
		},
	})
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
	unloadDB(db.CountersDB, lldpCountersCleanup())
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

	t.Log("\n\n+++++++++++++ GET top-level LLDP container ++++++++++++")
	url = "/openconfig-lldp:lldp"
	expectedGetJSON = `{"openconfig-lldp:lldp":{"config":{"chassis-id":"00:11:22:33:44:55","chassis-id-type":"MAC_ADDRESS","enabled":true,"hello-timer":30,"suppress-tlv-advertisement":["openconfig-lldp-types:MANAGEMENT_ADDRESS"],"system-description":"SONiC LLDP test","system-name":"sonic-switch"},"state":{"chassis-id":"00:11:22:33:44:55","chassis-id-type":"MAC_ADDRESS","counters":{},"enabled":true,"hello-timer":30,"suppress-tlv-advertisement":["openconfig-lldp-types:MANAGEMENT_ADDRESS"],"system-description":"SONiC LLDP test","system-name":"sonic-switch"}}}`
	t.Run("GET top-level lldp container", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

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

}

func Test_openconfig_lldp_interface_config(t *testing.T) {
	var url string
	t.Log("\n\n+++++++++++++ GET per-interface LLDP config (default enabled) ++++++++++++")
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config"
	expectedGetJSON := `{"openconfig-lldp:config":{"enabled":true,"name":"Ethernet0"}}`
	t.Run("GET interface config default enabled", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

}

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
	expectedGetJSON = `{"openconfig-lldp:interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","mgmt-addresses":{"mgmt-address":[{"address":"172.17.2.1","state":{"address":"172.17.2.1"}},{"address":"fd00::1","state":{"address":"fd00::1"}}]},"state":{"age":5000},"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}`
	t.Run("GET single interface with neighbor", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ GET all LLDP interfaces ++++++++++++")
	url = "/openconfig-lldp:lldp/interfaces"
	expectedGetJSON = `{"openconfig-lldp:interfaces":{"interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","mgmt-addresses":{"mgmt-address":[{"address":"172.17.2.1","state":{"address":"172.17.2.1"}},{"address":"fd00::1","state":{"address":"fd00::1"}}]},"state":{"age":5000},"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}}`
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
	expectedGetJSON := `{"openconfig-lldp:interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","mgmt-addresses":{"mgmt-address":[{"address":"172.17.2.1","state":{"address":"172.17.2.1"}},{"address":"fd00::1","state":{"address":"fd00::1"}}]},"state":{"age":5000},"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}`
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
	expectedGetJSON := `{"openconfig-lldp:interface":[{"config":{"enabled":true,"name":"Ethernet0"},"name":"Ethernet0","neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","mgmt-addresses":{"mgmt-address":[{"address":"172.17.2.1","state":{"address":"172.17.2.1"}},{"address":"fd00::1","state":{"address":"fd00::1"}}]},"state":{"age":5000},"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]},"state":{"enabled":true,"name":"Ethernet0"}}]}`
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
	expectedGetJSON = `{"openconfig-lldp:neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","mgmt-addresses":{"mgmt-address":[{"address":"172.17.2.1","state":{"address":"172.17.2.1"}},{"address":"fd00::1","state":{"address":"fd00::1"}}]},"state":{"age":5000},"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]}`
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

func Test_openconfig_lldp_config_without_global(t *testing.T) {
	t.Log("\n\n+++++++++++++ GET global LLDP config without LLDP|GLOBAL ++++++++++++")
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	loadDB(db.ApplDB, lldpLocChassisPrereq())
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	url := "/openconfig-lldp:lldp/config"
	expectedGetJSON := `{"openconfig-lldp:config":{"chassis-id":"00:11:22:33:44:55","chassis-id-type":"MAC_ADDRESS","enabled":true,"hello-timer":30,"system-description":"loc chassis desc","system-name":"loc-chassis-name"}}`
	t.Run("GET global config without GLOBAL row", processGetRequest(url, nil, expectedGetJSON, false))
}

func Test_openconfig_lldp_state_without_global(t *testing.T) {
	t.Log("\n\n+++++++++++++ GET global LLDP state without LLDP|GLOBAL ++++++++++++")
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	loadDB(db.ApplDB, lldpLocChassisPrereq())
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	url := "/openconfig-lldp:lldp/state"
	expectedGetJSON := `{"openconfig-lldp:state":{"chassis-id":"00:11:22:33:44:55","chassis-id-type":"MAC_ADDRESS","counters":{},"enabled":true,"hello-timer":30,"system-description":"loc chassis desc","system-name":"loc-chassis-name"}}`
	t.Run("GET global state without GLOBAL row", processGetRequest(url, nil, expectedGetJSON, false))
}

func Test_openconfig_lldp_system_info_leaf_gets_without_global(t *testing.T) {
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	loadDB(db.ApplDB, lldpLocChassisPrereq())
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"GET config/system-name without GLOBAL", "/openconfig-lldp:lldp/config/system-name", `{"openconfig-lldp:system-name":"loc-chassis-name"}`},
		{"GET config/system-description without GLOBAL", "/openconfig-lldp:lldp/config/system-description", `{"openconfig-lldp:system-description":"loc chassis desc"}`},
		{"GET config/chassis-id without GLOBAL", "/openconfig-lldp:lldp/config/chassis-id", `{"openconfig-lldp:chassis-id":"00:11:22:33:44:55"}`},
		{"GET config/chassis-id-type without GLOBAL", "/openconfig-lldp:lldp/config/chassis-id-type", `{"openconfig-lldp:chassis-id-type":"MAC_ADDRESS"}`},
		{"GET config/enabled without GLOBAL", "/openconfig-lldp:lldp/config/enabled", `{"openconfig-lldp:enabled":true}`},
		{"GET config/hello-timer without GLOBAL", "/openconfig-lldp:lldp/config/hello-timer", `{"openconfig-lldp:hello-timer":30}`},
		{"GET state/system-name without GLOBAL", "/openconfig-lldp:lldp/state/system-name", `{"openconfig-lldp:system-name":"loc-chassis-name"}`},
		{"GET state/system-description without GLOBAL", "/openconfig-lldp:lldp/state/system-description", `{"openconfig-lldp:system-description":"loc chassis desc"}`},
		{"GET state/chassis-id without GLOBAL", "/openconfig-lldp:lldp/state/chassis-id", `{"openconfig-lldp:chassis-id":"00:11:22:33:44:55"}`},
		{"GET state/chassis-id-type without GLOBAL", "/openconfig-lldp:lldp/state/chassis-id-type", `{"openconfig-lldp:chassis-id-type":"MAC_ADDRESS"}`},
		{"GET state/enabled without GLOBAL", "/openconfig-lldp:lldp/state/enabled", `{"openconfig-lldp:enabled":true}`},
		{"GET state/hello-timer without GLOBAL", "/openconfig-lldp:lldp/state/hello-timer", `{"openconfig-lldp:hello-timer":30}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, processGetRequest(tc.url, nil, tc.expected, false))
		time.Sleep(500 * time.Millisecond)
	}
}

func Test_openconfig_lldp_config_leaf_gets(t *testing.T) {
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"GET config/enabled", "/openconfig-lldp:lldp/config/enabled", `{"openconfig-lldp:enabled":true}`},
		{"GET config/hello-timer", "/openconfig-lldp:lldp/config/hello-timer", `{"openconfig-lldp:hello-timer":30}`},
		{"GET config/system-name", "/openconfig-lldp:lldp/config/system-name", `{"openconfig-lldp:system-name":"sonic-switch"}`},
		{"GET config/system-description", "/openconfig-lldp:lldp/config/system-description", `{"openconfig-lldp:system-description":"SONiC LLDP test"}`},
		{"GET config/chassis-id", "/openconfig-lldp:lldp/config/chassis-id", `{"openconfig-lldp:chassis-id":"00:11:22:33:44:55"}`},
		{"GET config/chassis-id-type", "/openconfig-lldp:lldp/config/chassis-id-type", `{"openconfig-lldp:chassis-id-type":"MAC_ADDRESS"}`},
		{"GET config/suppress-tlv-advertisement", "/openconfig-lldp:lldp/config/suppress-tlv-advertisement", `{"openconfig-lldp:suppress-tlv-advertisement":["openconfig-lldp-types:MANAGEMENT_ADDRESS"]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, processGetRequest(tc.url, nil, tc.expected, false))
		time.Sleep(500 * time.Millisecond)
	}
}

func Test_openconfig_lldp_state_leaf_gets(t *testing.T) {
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()

	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"GET state/enabled", "/openconfig-lldp:lldp/state/enabled", `{"openconfig-lldp:enabled":true}`},
		{"GET state/hello-timer", "/openconfig-lldp:lldp/state/hello-timer", `{"openconfig-lldp:hello-timer":30}`},
		{"GET state/system-name", "/openconfig-lldp:lldp/state/system-name", `{"openconfig-lldp:system-name":"sonic-switch"}`},
		{"GET state/chassis-id", "/openconfig-lldp:lldp/state/chassis-id", `{"openconfig-lldp:chassis-id":"00:11:22:33:44:55"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, processGetRequest(tc.url, nil, tc.expected, false))
		time.Sleep(500 * time.Millisecond)
	}
}

func Test_openconfig_lldp_neighbor_list_paths(t *testing.T) {
	loadLldpFixture(t, true, true, "true")
	defer unloadLldpFixture()

	url := "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors"
	expectedGetJSON := `{"openconfig-lldp:neighbors":{"neighbor":[{"capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]},"id":"Ethernet0","mgmt-addresses":{"mgmt-address":[{"address":"172.17.2.1","state":{"address":"172.17.2.1"}},{"address":"fd00::1","state":{"address":"fd00::1"}}]},"state":{"age":5000},"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","system-description":"SONiC simulator 1","system-name":"sonic1"}}]}}`
	t.Run("GET interface neighbors list", processGetRequest(url, nil, expectedGetJSON, false))
	time.Sleep(1 * time.Second)

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/capabilities"
	expectedGetJSON = `{"openconfig-lldp:capabilities":{"capability":[{"name":"openconfig-lldp-types:MAC_BRIDGE","state":{"enabled":true,"name":"openconfig-lldp-types:MAC_BRIDGE"}}]}}`
	t.Run("GET neighbor capabilities list", processGetRequest(url, nil, expectedGetJSON, false))
}

func Test_openconfig_lldp_set_global_config(t *testing.T) {
	var url, body string
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	loadDB(db.ApplDB, lldpLocChassisPrereq())
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ SET global LLDP enabled (create GLOBAL row) ++++++++++++")
	url = "/openconfig-lldp:lldp/config/enabled"
	body = `{"openconfig-lldp:enabled":false}`
	expectedMap := map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{"enabled": "false"},
		},
	}
	t.Run("PATCH global enabled leaf", processSetRequest(url, body, "PATCH", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify global enabled in CONFIG_DB", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))

	t.Log("\n\n+++++++++++++ SET global LLDP hello-timer ++++++++++++")
	url = "/openconfig-lldp:lldp/config/hello-timer"
	body = `{"openconfig-lldp:hello-timer":45}`
	expectedMap = map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{"enabled": "false", "hello_time": "45"},
		},
	}
	t.Run("PATCH global hello-timer leaf", processSetRequest(url, body, "PATCH", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify global hello-timer in CONFIG_DB", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))

	t.Log("\n\n+++++++++++++ SET global LLDP system-name and description ++++++++++++")
	url = "/openconfig-lldp:lldp/config"
	body = `{"openconfig-lldp:system-name":"new-name","openconfig-lldp:system-description":"new description"}`
	expectedMap = map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"enabled":            "false",
				"hello_time":         "45",
				"system_name":        "new-name",
				"system_description": "new description",
			},
		},
	}
	t.Run("PATCH global system info", processSetRequest(url, body, "PATCH", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify global system info in CONFIG_DB", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))

	t.Log("\n\n+++++++++++++ SET global suppress-tlv-advertisement ++++++++++++")
	url = "/openconfig-lldp:lldp/config/suppress-tlv-advertisement"
	body = `{"openconfig-lldp:suppress-tlv-advertisement":["openconfig-lldp-types:MANAGEMENT_ADDRESS","openconfig-lldp-types:SYSTEM_CAPABILITIES"]}`
	expectedMap = map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"enabled":                      "false",
				"hello_time":                   "45",
				"system_name":                  "new-name",
				"system_description":           "new description",
				"supp_mgmt_address_tlv":        "true",
				"supp_system_capabilities_tlv": "true",
			},
		},
	}
	t.Run("PUT global suppress-tlv-advertisement", processSetRequest(url, body, "PUT", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify global suppress TLVs in CONFIG_DB", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))

	t.Log("\n\n+++++++++++++ PATCH enabled only must not reset suppress TLVs ++++++++++++")
	url = "/openconfig-lldp:lldp/config/enabled"
	body = `{"openconfig-lldp:enabled":true}`
	t.Run("PATCH global enabled true", processSetRequest(url, body, "PATCH", false))
	time.Sleep(1 * time.Second)
	expectedMap = map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"enabled":                      "true",
				"hello_time":                   "45",
				"system_name":                  "new-name",
				"system_description":           "new description",
				"supp_mgmt_address_tlv":        "true",
				"supp_system_capabilities_tlv": "true",
			},
		},
	}
	t.Run("Verify suppress TLVs unchanged after enabled-only PATCH", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))
}

func Test_openconfig_lldp_set_enabled_only_preserves_suppress_tlv(t *testing.T) {
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	loadDB(db.ConfigDB, map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"enabled":                      "true",
				"supp_mgmt_address_tlv":        "true",
				"supp_system_capabilities_tlv": "false",
			},
		},
	})
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	url := "/openconfig-lldp:lldp/config/enabled"
	body := `{"openconfig-lldp:enabled":false}`
	t.Run("PATCH enabled leaf only", processSetRequest(url, body, "PATCH", false))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"enabled":                      "false",
				"supp_mgmt_address_tlv":        "true",
				"supp_system_capabilities_tlv": "false",
			},
		},
	}
	t.Run("Verify suppress TLV fields unchanged", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))
}

func Test_openconfig_lldp_delete_global_config(t *testing.T) {
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ DELETE global LLDP config leaves ++++++++++++")
	url := "/openconfig-lldp:lldp/config/enabled"
	t.Run("DELETE global enabled", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"hello_time":                   "30",
				"system_name":                  "sonic-switch",
				"system_description":           "SONiC LLDP test",
				"supp_mgmt_address_tlv":        "true",
				"supp_system_capabilities_tlv": "false",
			},
		},
	}
	t.Run("Verify enabled deleted from CONFIG_DB", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))

	url = "/openconfig-lldp:lldp/config/suppress-tlv-advertisement"
	t.Run("DELETE global suppress-tlv-advertisement", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)
	expectedMap = map[string]interface{}{
		"LLDP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"hello_time":         "30",
				"system_name":        "sonic-switch",
				"system_description": "SONiC LLDP test",
			},
		},
	}
	t.Run("Verify suppress TLV fields deleted from CONFIG_DB", verifyDbResult(rclient, "LLDP|GLOBAL", expectedMap, false))
}

func Test_openconfig_lldp_set_interface_config(t *testing.T) {
	cleanup := lldpCleanupTables()
	unloadDB(db.ConfigDB, cleanup)
	unloadDB(db.ApplDB, cleanup)
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ SET per-interface LLDP enabled ++++++++++++")
	url := "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config"
	body := `{"openconfig-lldp:config":{"enabled":false,"name":"Ethernet0"}}`
	expectedMap := map[string]interface{}{
		"LLDP_PORT": map[string]interface{}{
			testIfName: map[string]interface{}{"enabled": "false"},
		},
	}
	t.Run("PATCH interface config enabled false", processSetRequest(url, body, "PATCH", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify interface enabled in CONFIG_DB", verifyDbResult(rclient, "LLDP_PORT|Ethernet0", expectedMap, false))

	t.Log("\n\n+++++++++++++ SET per-interface LLDP enabled via leaf ++++++++++++")
	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config/enabled"
	body = `{"openconfig-lldp:enabled":true}`
	expectedMap = map[string]interface{}{
		"LLDP_PORT": map[string]interface{}{
			testIfName: map[string]interface{}{"enabled": "true"},
		},
	}
	t.Run("PATCH interface config/enabled leaf", processSetRequest(url, body, "PATCH", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify interface enabled leaf update in CONFIG_DB", verifyDbResult(rclient, "LLDP_PORT|Ethernet0", expectedMap, false))
}

func Test_openconfig_lldp_delete_interface_config(t *testing.T) {
	loadDB(db.ConfigDB, lldpPortPrereq("false"))
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ DELETE per-interface LLDP config leaves ++++++++++++")
	url := "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config/enabled"
	t.Run("DELETE interface config/enabled", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)

	expectedMap := map[string]interface{}{
		"LLDP_PORT": map[string]interface{}{
			testIfName: map[string]interface{}{},
		},
	}
	t.Run("Verify interface enabled deleted from CONFIG_DB", verifyDbResult(rclient, "LLDP_PORT|Ethernet0", expectedMap, false))

	url = "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config"
	t.Run("DELETE interface config container", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)
	deleteExpected := make(map[string]interface{})
	t.Run("Verify interface row deleted from CONFIG_DB", verifyDbResult(rclient, "LLDP_PORT|Ethernet0", deleteExpected, false))
}

func Test_openconfig_lldp_readonly_reject(t *testing.T) {
	loadLldpFixture(t, true, true, "true")
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	stateErr := tlerr.NotSupportedError{Format: "LLDP operational state is read-only"}
	neighborErr := tlerr.NotSupportedError{Format: "LLDP neighbor data is read-only"}

	t.Run("PATCH global state rejected", processSetRequest(
		"/openconfig-lldp:lldp/state",
		`{"openconfig-lldp:enabled":false}`,
		"PATCH", true, stateErr))
	t.Run("PATCH neighbor state rejected", processSetRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/state",
		`{"openconfig-lldp:system-name":"nope"}`,
		"PATCH", true, neighborErr))
	t.Run("DELETE neighbor rejected", processDeleteRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]",
		true, neighborErr))

	unsupportedDeleteErr := tlerr.NotSupportedError{Format: "DELETE not supported on /openconfig-lldp:lldp/config/chassis-id"}
	t.Run("DELETE read-only chassis-id rejected", processDeleteRequest(
		"/openconfig-lldp:lldp/config/chassis-id",
		true, unsupportedDeleteErr))
}

func Test_openconfig_lldp_unsupported_1_2_0_config(t *testing.T) {
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	ttlErr := tlerr.NotSupportedError{Format: "LLDP configuration not supported on /openconfig-lldp:lldp/config/ttl"}
	portDescErr := tlerr.NotSupportedError{Format: "LLDP configuration not supported on /openconfig-lldp:lldp/interfaces/interface/config/port-description"}
	mgmtAddrErr := tlerr.NotSupportedError{Format: "LLDP management addresses are read-only"}

	t.Run("PATCH global ttl rejected", processSetRequest(
		"/openconfig-lldp:lldp/config/ttl",
		`{"openconfig-lldp:ttl":120}`,
		"PATCH", true, ttlErr))
	t.Run("PATCH ttl inside config container rejected", processSetRequest(
		"/openconfig-lldp:lldp/config",
		`{"openconfig-lldp:ttl":120}`,
		"PATCH", true, ttlErr))
	t.Run("PATCH interface port-description rejected", processSetRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config/port-description",
		`{"openconfig-lldp:port-description":"override"}`,
		"PATCH", true, portDescErr))
	t.Run("PATCH mgmt-addresses rejected", processSetRequest(
		"/openconfig-lldp:lldp/mgmt-addresses",
		`{"openconfig-lldp:mgmt-addresses":{}}`,
		"PATCH", true, mgmtAddrErr))
	t.Run("DELETE global ttl rejected", processDeleteRequest(
		"/openconfig-lldp:lldp/config/ttl",
		true, ttlErr))
}

func Test_openconfig_lldp_extended_state(t *testing.T) {
	loadLldpFixture(t, true, true, "true")
	defer unloadLldpFixture()

	loadDB(db.ApplDB, map[string]interface{}{
		"LLDP_LOC_CHASSIS": map[string]interface{}{
			"": map[string]interface{}{
				"lldp_loc_man_addr": "10.1.0.32,fc00:1::32",
				"lldp_loc_ttl":      "120",
			},
		},
		"LLDP_ENTRY_TABLE": map[string]interface{}{
			testIfName: map[string]interface{}{
				"lldp_rem_ttl":            "90",
				"lldp_rem_max_frame_size": "1514",
				"lldp_rem_port_vlan_id":   "146",
				"lldp_rem_med_inv_serial": "SN-1234",
				"lldp_rem_agg_port_id":    "42",
				"lldp_rem_custom_tlvs":    `[{"type":127,"oui":"00:90:69","oui-subtype":"1","value":"435530323133353130363530"}]`,
			},
		},
	})
	time.Sleep(1 * time.Second)

	t.Run("GET local mgmt-addresses", processGetRequest(
		"/openconfig-lldp:lldp/mgmt-addresses",
		nil,
		`{"openconfig-lldp:mgmt-addresses":{"mgmt-address":[{"address":"10.1.0.32","state":{"address":"10.1.0.32"}},{"address":"fc00:1::32","state":{"address":"fc00:1::32"}}]}}`,
		false))

	t.Run("GET global state ttl from loc chassis", processGetRequest(
		"/openconfig-lldp:lldp/state/ttl",
		nil,
		`{"openconfig-lldp:ttl":120}`,
		false))

	t.Run("GET neighbor ttl pvid mfs med-serial", processGetRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/state",
		nil,
		`{"openconfig-lldp:state":{"age":5000,"chassis-id":"00:aa:00:00:00:01","chassis-id-type":"MAC_ADDRESS","id":"1","management-address":"172.17.2.1,fd00::1","management-address-type":"ipv4","max-frame-size":1514,"med-inventory-serial-number":"SN-1234","port-description":"Port 1/1","port-id":"Ethernet0","port-id-type":"LOCAL","port-vlan-id":146,"system-description":"SONiC simulator 1","system-name":"sonic1","ttl":90}}`,
		false))

	t.Run("GET neighbor mgmt-addresses list", processGetRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/mgmt-addresses",
		nil,
		`{"openconfig-lldp:mgmt-addresses":{"mgmt-address":[{"address":"172.17.2.1","state":{"address":"172.17.2.1"}},{"address":"fd00::1","state":{"address":"fd00::1"}}]}}`,
		false))

	t.Run("GET neighbor link-aggregation", processGetRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/link-aggregation/state",
		nil,
		`{"openconfig-lldp:state":{"capable":true,"enabled":true,"port-id":42}}`,
		false))

	t.Run("GET neighbor custom-tlvs", processGetRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors/neighbor[id=Ethernet0]/custom-tlvs",
		nil,
		`{"openconfig-lldp:custom-tlvs":{"tlv":[{"oui":"00:90:69","oui-subtype":"1","state":{"oui":"00:90:69","oui-subtype":"1","type":127,"value":"Q1UwMjEzNTEwNjUw"},"type":127}]}}`,
		false))
}

func Test_openconfig_lldp_counters(t *testing.T) {
	loadLldpFixture(t, true, true, "true")
	defer unloadLldpFixture()

	loadDB(db.CountersDB, map[string]interface{}{
		"LLDP_STATISTICS": map[string]interface{}{
			"GLOBAL": map[string]interface{}{
				"frame_in":         "220",
				"frame_out":        "110",
				"frame_discard":    "7",
				"tlv_unknown":      "9",
				"tlv_accepted":     "13",
				"entries_aged_out": "11",
			},
			testIfName: map[string]interface{}{
				"frame_in":         "200",
				"frame_out":        "100",
				"frame_discard":    "6",
				"tlv_unknown":      "7",
				"tlv_accepted":     "9",
				"entries_aged_out": "8",
			},
		},
	})
	time.Sleep(1 * time.Second)

	t.Run("GET global state counters from COUNTERS_DB", processGetRequest(
		"/openconfig-lldp:lldp/state/counters",
		nil,
		`{"openconfig-lldp:counters":{"entries-aged-out":11,"frame-discard":7,"frame-in":220,"frame-out":110,"tlv-accepted":13,"tlv-unknown":9}}`,
		false))

	t.Run("GET interface state counters from COUNTERS_DB", processGetRequest(
		"/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/state/counters",
		nil,
		`{"openconfig-lldp:counters":{"frame-discard":6,"frame-in":200,"frame-out":100,"tlv-unknown":7}}`,
		false))
}

func Test_openconfig_lldp_management_interface_and_custom_tlvs(t *testing.T) {
	loadLldpFixture(t, false, false, "")
	defer unloadLldpFixture()
	time.Sleep(1 * time.Second)

	t.Run("PATCH global management-interface", processSetRequest(
		"/openconfig-lldp:lldp/config/management-interface",
		`{"openconfig-lldp:management-interface":"eth0"}`,
		"PATCH", false))
	t.Run("GET global management-interface", processGetRequest(
		"/openconfig-lldp:lldp/config/management-interface",
		nil,
		`{"openconfig-lldp:management-interface":"eth0"}`,
		false))

	t.Run("PATCH invalid management-interface rejected", processSetRequest(
		"/openconfig-lldp:lldp/config/management-interface",
		`{"openconfig-lldp:management-interface":"?"}`,
		"PATCH", true,
		tlerr.InvalidArgs("Invalid LLDP management-interface '%s'", "?")))

	t.Run("PUT custom-tlv", processSetRequest(
		"/openconfig-lldp:lldp/custom-tlvs/tlv[name=vendor-serial]/config",
		`{"openconfig-lldp:config":{"name":"vendor-serial","oui":"00:90:69","oui-subtype":"1","type":127,"value":"Q1UwMjEzNTEwNjUw"}}`,
		"PUT", false))
	t.Run("GET custom-tlvs", processGetRequest(
		"/openconfig-lldp:lldp/custom-tlvs",
		nil,
		`{"openconfig-lldp:custom-tlvs":{"tlv":[{"config":{"name":"vendor-serial","oui":"00:90:69","oui-subtype":"1","type":127,"value":"Q1UwMjEzNTEwNjUw"},"name":"vendor-serial","state":{"name":"vendor-serial","oui":"00:90:69","oui-subtype":"1","type":127,"value":"Q1UwMjEzNTEwNjUw"}}]}}`,
		false))

	t.Run("PATCH suppress chassis-id rejected", processSetRequest(
		"/openconfig-lldp:lldp/config/suppress-tlv-advertisement",
		`{"openconfig-lldp:suppress-tlv-advertisement":["openconfig-lldp-types:CHASSIS_ID"]}`,
		"PATCH", true,
		tlerr.NotSupportedError{Format: "LLDP suppress-tlv-advertisement identity not supported by lldpd"}))
}
