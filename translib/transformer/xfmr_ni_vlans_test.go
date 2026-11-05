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
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const niVlanDefaultUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/vlans"
const niVlanRedUrl = "/openconfig-network-instance:network-instances/network-instance[name=Vrf_red]/vlans"

func niVlanPath(vlanID int, extra string) string {
	return niVlanDefaultUrl + "/vlan[vlan-id=" + strconv.Itoa(vlanID) + "]" + extra
}

func niVlanDb(vlanID int, admin string) map[string]interface{} {
	name := "Vlan" + strconv.Itoa(vlanID)
	return map[string]interface{}{
		"VLAN": map[string]interface{}{name: map[string]interface{}{"vlanid": strconv.Itoa(vlanID), "admin_status": admin}},
	}
}

func cleanupNiVlans(t *testing.T, vlanIDs ...int) {
	t.Helper()
	for _, id := range vlanIDs {
		processDeleteRequest(niVlanPath(id, ""), true)(t)
	}
	time.Sleep(1 * time.Second)
}

var niVlanOtherNiErr = tlerr.NotSupportedError{
	Format: "VLANs exist only in the default network instance; SONiC has no per-instance VLAN, so network-instance \"Vrf_red\" holds none",
}

// The default instance writes the VLAN rows top-level /vlans maps, by the same
// rules: vlan-id is vlanid, the name is derived, status is admin_status. No
// other instance holds a VLAN.
func Test_openconfig_network_instance_vlans_config(t *testing.T) {
	ids := []int{3101, 3102}
	cleanupNiVlans(t, ids...)
	defer cleanupNiVlans(t, ids...)

	t.Run("PATCH network-instance VLAN config",
		processSetRequest(niVlanPath(3101, "/config"), `{"vlan-id":3101,"name":"Vlan3101","status":"ACTIVE"}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify VLAN 3101 in CONFIG_DB", verifyDbResult(rclient, "VLAN|Vlan3101", niVlanDb(3101, "up"), false))
	t.Run("GET network-instance VLAN config",
		processGetRequest(niVlanPath(3101, "/config"), nil, `{"openconfig-network-instance:config":{"name":"Vlan3101","status":"ACTIVE","vlan-id":3101}}`, false))
	t.Run("GET network-instance VLAN config vlan-id leaf",
		processGetRequest(niVlanPath(3101, "/config/vlan-id"), nil, `{"openconfig-network-instance:vlan-id":3101}`, false))
	t.Run("GET network-instance VLAN config name leaf",
		processGetRequest(niVlanPath(3101, "/config/name"), nil, `{"openconfig-network-instance:name":"Vlan3101"}`, false))
	t.Run("GET the VLAN through top-level vlans",
		processGetRequest("/openconfig-vlan:vlans/vlan[vlan-id=3101]/config", nil, `{"openconfig-vlan:config":{"name":"Vlan3101","status":"ACTIVE","vlan-id":3101}}`, false))

	t.Run("PATCH network-instance VLAN config status leaf SUSPENDED",
		processSetRequest(niVlanPath(3101, "/config/status"), `{"openconfig-network-instance:status":"SUSPENDED"}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify VLAN 3101 suspended", verifyDbResult(rclient, "VLAN|Vlan3101", niVlanDb(3101, "down"), false))
	t.Run("PATCH network-instance VLAN config without status",
		processSetRequest(niVlanPath(3101, "/config"), `{"vlan-id":3101}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify VLAN 3101 still suspended", verifyDbResult(rclient, "VLAN|Vlan3101", niVlanDb(3101, "down"), false))
	t.Run("GET network-instance VLAN config status leaf",
		processGetRequest(niVlanPath(3101, "/config/status"), nil, `{"openconfig-network-instance:status":"SUSPENDED"}`, false))
	t.Run("DELETE network-instance VLAN config status leaf",
		processDeleteRequest(niVlanPath(3101, "/config/status"), false))
	time.Sleep(1 * time.Second)
	t.Run("Verify VLAN 3101 back up", verifyDbResult(rclient, "VLAN|Vlan3101", niVlanDb(3101, "up"), false))

	t.Run("PATCH network-instance vlans container",
		processSetRequest(niVlanDefaultUrl, `{"vlan":[{"vlan-id":3102,"config":{"vlan-id":3102,"status":"SUSPENDED"}}]}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify VLAN 3102 in CONFIG_DB", verifyDbResult(rclient, "VLAN|Vlan3102", niVlanDb(3102, "down"), false))
	t.Run("PUT network-instance VLAN instance without status",
		processSetRequest(niVlanPath(3102, ""), `{"vlan-id":3102,"config":{"vlan-id":3102}}`, "PUT", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Verify VLAN 3102 replaced up", verifyDbResult(rclient, "VLAN|Vlan3102", niVlanDb(3102, "up"), false))

	t.Run("PATCH mismatched VLAN name rejected",
		processSetRequest(niVlanPath(3101, "/config"), `{"name":"engineering"}`, "PATCH", true, tlerr.InvalidArgsError{Format: "VLAN name must be Vlan3101"}))
	t.Run("PATCH mismatched VLAN id rejected",
		processSetRequest(niVlanPath(3101, "/config"), `{"vlan-id":3102}`, "PATCH", true, tlerr.InvalidArgsError{Format: "VLAN id in payload does not match 3101"}))
	t.Run("PATCH VLAN id 1 rejected",
		processSetRequest(niVlanPath(1, "/config"), `{"vlan-id":1}`, "PATCH", true, tlerr.InvalidArgsError{Format: "Invalid VLAN id 1: SONiC supports VLAN ids 2-4094"}))
	// state is config false, so the binder turns a write away before the
	// transformer is reached.
	t.Run("PATCH network-instance VLAN state rejected",
		processSetRequest(niVlanPath(3101, "/state"), `{"vlan-id":3101}`, "PATCH", true,
			tlerr.TranslibSyntaxValidationError{ErrorStr: errors.New("SET operation not allowed on the read-only object")}))
	t.Run("DELETE network-instance VLAN config name rejected",
		processDeleteRequest(niVlanPath(3101, "/config/name"), true,
			tlerr.NotSupportedError{Format: "network-instance VLAN config name cannot be deleted on its own; delete the VLAN instead"}))
	t.Run("PATCH a VLAN under another instance rejected",
		processSetRequest(niVlanRedUrl+"/vlan[vlan-id=3103]/config", `{"vlan-id":3103}`, "PATCH", true, niVlanOtherNiErr))
	t.Run("DELETE the vlans of another instance rejected",
		processDeleteRequest(niVlanRedUrl, true, niVlanOtherNiErr))
	time.Sleep(1 * time.Second)
	t.Run("Verify no VLAN 3103 in CONFIG_DB", verifyDbResult(rclient, "VLAN|Vlan3103", map[string]interface{}{}, false))

	member := map[string]interface{}{"VLAN_MEMBER": map[string]interface{}{"Vlan3101|Ethernet0": map[string]interface{}{"tagging_mode": "untagged"}}}
	loadDB(db.ConfigDB, member)
	time.Sleep(1 * time.Second)
	t.Run("DELETE network-instance VLAN with members rejected",
		processDeleteRequest(niVlanPath(3101, ""), true,
			tlerr.InvalidArgsError{Format: "VLAN ID 3101 can not be removed. First remove all members assigned to this VLAN."}))
	unloadDB(db.ConfigDB, member)
	time.Sleep(1 * time.Second)
	t.Run("DELETE network-instance VLAN", processDeleteRequest(niVlanPath(3101, ""), false))
	time.Sleep(1 * time.Second)
	t.Run("Verify VLAN 3101 deleted", verifyDbResult(rclient, "VLAN|Vlan3101", map[string]interface{}{}, false))
	t.Run("Verify VLAN 3102 kept", verifyDbResult(rclient, "VLAN|Vlan3102", niVlanDb(3102, "up"), false))
}

// Config is read from CONFIG_DB, state and members from what vlanmgrd applied
// to APPL_DB. Another instance reports no VLAN.
func Test_openconfig_network_instance_vlans_get(t *testing.T) {
	cfg := map[string]interface{}{
		"VLAN": map[string]interface{}{
			"Vlan3101": map[string]interface{}{"vlanid": "3101", "admin_status": "up"},
			"Vlan3102": map[string]interface{}{"vlanid": "3102", "admin_status": "down"},
			"Vlan3103": map[string]interface{}{"vlanid": "3103"},
		},
	}
	appl := map[string]interface{}{
		"VLAN_TABLE": map[string]interface{}{
			"Vlan3101": map[string]interface{}{"admin_status": "up", "mtu": "9100"},
			"Vlan3102": map[string]interface{}{"admin_status": "down", "mtu": "9100"},
		},
		"VLAN_MEMBER_TABLE": map[string]interface{}{
			"Vlan3101:Ethernet0": map[string]interface{}{"tagging_mode": "untagged"},
			"Vlan3101:Ethernet4": map[string]interface{}{"tagging_mode": "tagged"},
		},
	}
	unloadDB(db.ConfigDB, cfg)
	unloadDB(db.ApplDB, appl)
	loadDB(db.ConfigDB, cfg)
	loadDB(db.ApplDB, appl)
	time.Sleep(1 * time.Second)
	defer func() {
		unloadDB(db.ApplDB, appl)
		unloadDB(db.ConfigDB, cfg)
	}()

	v3101 := `{"config":{"name":"Vlan3101","status":"ACTIVE","vlan-id":3101},` +
		`"members":{"member":[{"state":{"interface":"Ethernet0"}},{"state":{"interface":"Ethernet4"}}]},` +
		`"state":{"name":"Vlan3101","status":"ACTIVE","vlan-id":3101},"vlan-id":3101}`
	t.Run("GET network-instance VLAN instance",
		processGetRequest(niVlanPath(3101, ""), nil, `{"openconfig-network-instance:vlan":[`+v3101+`]}`, false))
	t.Run("GET network-instance VLAN state",
		processGetRequest(niVlanPath(3102, "/state"), nil, `{"openconfig-network-instance:state":{"name":"Vlan3102","status":"SUSPENDED","vlan-id":3102}}`, false))
	t.Run("GET network-instance VLAN state status leaf",
		processGetRequest(niVlanPath(3102, "/state/status"), nil, `{"openconfig-network-instance:status":"SUSPENDED"}`, false))
	t.Run("GET network-instance VLAN members",
		processGetRequest(niVlanPath(3101, "/members"), nil,
			`{"openconfig-network-instance:members":{"member":[{"state":{"interface":"Ethernet0"}},{"state":{"interface":"Ethernet4"}}]}}`, false))
	t.Run("GET network-instance VLAN not applied reports config only",
		processGetRequest(niVlanPath(3103, ""), nil,
			`{"openconfig-network-instance:vlan":[{"config":{"name":"Vlan3103","status":"ACTIVE","vlan-id":3103},"vlan-id":3103}]}`, false))
	t.Run("GET network-instance VLAN state not applied is not found",
		processGetRequest(niVlanPath(3103, "/state"), nil, "", true, tlerr.NotFoundError{Format: "network-instance default has no VLAN 3103"}))
	t.Run("GET missing network-instance VLAN",
		processGetRequest(niVlanPath(3200, ""), nil, "", true, tlerr.NotFoundError{Format: "network-instance default has no VLAN 3200"}))
	t.Run("GET a VLAN under another instance is not found",
		processGetRequest(niVlanRedUrl+"/vlan[vlan-id=3101]", nil, "", true,
			tlerr.NotFoundError{Format: "network-instance Vrf_red holds no VLAN; VLANs exist only in the default network instance"}))
}
