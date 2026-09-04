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
	"context"
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const stpTestIfName = "Ethernet0"

func stpEnableRstpPrereq() map[string]interface{} {
	return map[string]interface{}{
		"VLAN_MEMBER": map[string]interface{}{
			"Vlan100|" + stpTestIfName: map[string]interface{}{},
		},
	}
}

func stpMstInstPrereq(priority string) map[string]interface{} {
	return map[string]interface{}{
		"STP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{"mode": "mst"},
		},
		"STP_MST_INST": map[string]interface{}{
			"0": map[string]interface{}{"bridge_priority": priority},
		},
		"STP_PORT": map[string]interface{}{
			stpTestIfName: map[string]interface{}{
				"enabled":   "true",
				"edge_port": "false",
				"link_type": "auto",
				"path_cost": "1",
				"priority":  "128",
			},
		},
		"STP_MST_PORT": map[string]interface{}{
			"0|" + stpTestIfName: map[string]interface{}{
				"path_cost": "1",
				"priority":  "128",
			},
		},
	}
}

func stpMstInstAppPrereq() map[string]interface{} {
	return map[string]interface{}{
		"STP_MST_INST_TABLE": map[string]interface{}{
			"0": map[string]interface{}{
				"bridge_address":     "8000.781f.7cfc.cf17",
				"bridge_priority":    "0",
				"root_address":       "0000.781f.7cfc.cf17",
				"root_path_cost":     "600",
				"regional_root_cost": "400",
				"root_port":          stpTestIfName,
				"hold_time":          "1",
			},
		},
		"STP_MST_PORT_TABLE": map[string]interface{}{
			"0:" + stpTestIfName: map[string]interface{}{
				"port_number":   "4",
				"priority":      "128",
				"path_cost":     "200",
				"desig_cost":    "400",
				"desig_port":    "32772",
				"port_state":    "FORWARDING",
				"role":          "root",
				"bpdu_sent":     "10",
				"bpdu_received": "5",
			},
		},
	}
}

func stpEnableRstpExpected() map[string]interface{} {
	return map[string]interface{}{
		"STP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{"mode": "mst"},
		},
		"STP_MST_INST": map[string]interface{}{
			"0": map[string]interface{}{"bridge_priority": "32768"},
		},
		"STP_PORT": map[string]interface{}{
			stpTestIfName: map[string]interface{}{
				"enabled":               "true",
				"bpdu_guard":            "false",
				"bpdu_guard_do_disable": "false",
				"root_guard":            "false",
				"priority":              "128",
				"edge_port":             "false",
				"link_type":             "auto",
			},
		},
		"STP_MST_PORT": map[string]interface{}{
			"0|" + stpTestIfName: map[string]interface{}{
				"priority": "128",
			},
		},
	}
}

func stpEnableRstpCleanup() map[string]interface{} {
	return map[string]interface{}{
		"STP":          map[string]interface{}{"GLOBAL": ""},
		"STP_MST_INST": map[string]interface{}{"0": ""},
		"STP_PORT":     map[string]interface{}{stpTestIfName: ""},
		"STP_MST_PORT": map[string]interface{}{"0|" + stpTestIfName: ""},
	}
}

func verifyStpMstPortCreatedOnEnable(t *testing.T) {
	t.Helper()
	expected := map[string]interface{}{
		"STP_MST_PORT": map[string]interface{}{
			"0|" + stpTestIfName: map[string]interface{}{"priority": "128"},
		},
	}
	verifyDbResult(rclient, "STP_MST_PORT|0|"+stpTestIfName, expected, false)(t)
}

func Test_set_rstp_enabled_protocol(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config"
	body := `{"openconfig-spanning-tree:config":{"enabled-protocol":["openconfig-spanning-tree-types:RSTP"]}}`
	expected := stpEnableRstpExpected()
	cleanup := stpEnableRstpCleanup()

	loadDB(db.ConfigDB, stpEnableRstpPrereq())
	time.Sleep(1 * time.Second)
	t.Run("enable RSTP via enabled-protocol", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify MST mode and MSTI 0 defaults", verifyDbResult(rclient, "STP|GLOBAL", expected, false))
	t.Run("verify STP_PORT link_type auto after enable", verifyDbResult(rclient, "STP_PORT|"+stpTestIfName, expected, false))
	t.Run("verify STP_MST_PORT created on enable", verifyStpMstPortCreatedOnEnable)
	unloadDB(db.ConfigDB, cleanup)
}

func Test_set_rstp_enabled_protocol_cold_start_update(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config"
	body := `{"openconfig-spanning-tree:config":{"enabled-protocol":["openconfig-spanning-tree-types:RSTP"]}}`
	expected := stpEnableRstpExpected()
	cleanup := stpEnableRstpCleanup()

	// Cold start: VLAN_MEMBER only, no STP|GLOBAL. PATCH uses translib.Update (gNMI UPDATE).
	loadDB(db.ConfigDB, stpEnableRstpPrereq())
	time.Sleep(1 * time.Second)
	t.Run("enable RSTP via UPDATE on cold start", processSetRequest(url, body, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify MST mode and MSTI 0 defaults after cold-start UPDATE", verifyDbResult(rclient, "STP|GLOBAL", expected, false))
	t.Run("verify STP_PORT link_type auto after enable", verifyDbResult(rclient, "STP_PORT|"+stpTestIfName, expected, false))
	t.Run("verify STP_MST_PORT created on enable", verifyStpMstPortCreatedOnEnable)
	unloadDB(db.ConfigDB, cleanup)
}

func Test_set_rstp_enabled_protocol_patch(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config"
	body := `{"openconfig-spanning-tree:config":{"enabled-protocol":["openconfig-spanning-tree-types:RSTP"]}}`
	expected := stpEnableRstpExpected()
	cleanup := stpEnableRstpCleanup()

	loadDB(db.ConfigDB, stpEnableRstpPrereq())
	time.Sleep(1 * time.Second)
	t.Run("enable RSTP via PATCH on global config", processSetRequest(url, body, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify MST mode and MSTI 0 defaults after PATCH", verifyDbResult(rclient, "STP|GLOBAL", expected, false))
	t.Run("verify STP_PORT link_type auto after enable", verifyDbResult(rclient, "STP_PORT|"+stpTestIfName, expected, false))
	t.Run("verify STP_MST_PORT created on enable", verifyStpMstPortCreatedOnEnable)
	unloadDB(db.ConfigDB, cleanup)
}

func Test_set_rstp_enabled_protocol_replace(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config"
	body := `{"openconfig-spanning-tree:config":{"enabled-protocol":["openconfig-spanning-tree-types:RSTP"]}}`
	cleanup := stpEnableRstpCleanup()

	loadDB(db.ConfigDB, stpEnableRstpPrereq())
	time.Sleep(1 * time.Second)
	t.Run("enable RSTP via PUT replace on global config", processSetRequest(url, body, "PUT", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify STP global mode after replace", verifyDbResult(rclient, "STP|GLOBAL", map[string]interface{}{
		"STP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{"mode": "mst"},
		},
	}, false))
	unloadDB(db.ConfigDB, cleanup)
}

func Test_set_rstp_enabled_protocol_leaf_patch(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config/enabled-protocol"
	body := `{"openconfig-spanning-tree:enabled-protocol":["openconfig-spanning-tree-types:RSTP"]}`
	cleanup := stpEnableRstpCleanup()

	loadDB(db.ConfigDB, stpEnableRstpPrereq())
	time.Sleep(1 * time.Second)
	t.Run("enable RSTP via PATCH on enabled-protocol leaf", processSetRequest(url, body, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify STP global mode after leaf PATCH", verifyDbResult(rclient, "STP|GLOBAL", map[string]interface{}{
		"STP": map[string]interface{}{
			"GLOBAL": map[string]interface{}{"mode": "mst"},
		},
	}, false))
	unloadDB(db.ConfigDB, cleanup)
}

func Test_set_rstp_bridge_priority(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/rstp/config/bridge-priority"
	body := `{"openconfig-spanning-tree:bridge-priority":40960}`
	expected := map[string]interface{}{
		"STP_MST_INST": map[string]interface{}{
			"0": map[string]interface{}{"bridge_priority": "40960"},
		},
	}
	cleanup := map[string]interface{}{
		"STP_MST_INST": map[string]interface{}{"0": ""},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set RSTP bridge priority on MSTI 0", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify MSTI 0 bridge priority", verifyDbResult(rclient, "STP_MST_INST|0", expected, false))
	unloadDB(db.ConfigDB, cleanup)
}

func Test_set_rstp_bridge_priority_zero(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/rstp/config/bridge-priority"
	body := `{"openconfig-spanning-tree:bridge-priority":0}`
	expected := map[string]interface{}{
		"STP_MST_INST": map[string]interface{}{
			"0": map[string]interface{}{"bridge_priority": "0"},
		},
	}
	cleanup := map[string]interface{}{
		"STP_MST_INST": map[string]interface{}{"0": ""},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set RSTP bridge priority 0 on MSTI 0", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify MSTI 0 bridge priority is 0", verifyDbResult(rclient, "STP_MST_INST|0", expected, false))
	unloadDB(db.ConfigDB, cleanup)
}

func Test_set_rstp_edge_port(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/interfaces/interface[name=" + stpTestIfName + "]/config/edge-port"
	body := `{"openconfig-spanning-tree:edge-port":"openconfig-spanning-tree-types:EDGE_ENABLE"}`
	expected := map[string]interface{}{
		"STP_PORT": map[string]interface{}{
			stpTestIfName: map[string]interface{}{"edge_port": "true"},
		},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set edge-port on interface", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify edge_port in STP_PORT", verifyDbResult(rclient, "STP_PORT|"+stpTestIfName, expected, false))
}

func Test_set_rstp_link_type(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/interfaces/interface[name=" + stpTestIfName + "]/config/link-type"
	body := `{"openconfig-spanning-tree:link-type":"P2P"}`
	expected := map[string]interface{}{
		"STP_PORT": map[string]interface{}{
			stpTestIfName: map[string]interface{}{"link_type": "point-to-point"},
		},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set link-type on interface", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify link_type in STP_PORT", verifyDbResult(rclient, "STP_PORT|"+stpTestIfName, expected, false))
}

func Test_set_rstp_link_type_auto(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/interfaces/interface[name=" + stpTestIfName + "]/config/link-type"
	body := `{"openconfig-spanning-tree:link-type":"AUTO"}`
	expected := map[string]interface{}{
		"STP_PORT": map[string]interface{}{
			stpTestIfName: map[string]interface{}{"link_type": "auto"},
		},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set link-type AUTO on interface", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify auto link_type in STP_PORT", verifyDbResult(rclient, "STP_PORT|"+stpTestIfName, expected, false))
	t.Run("get STP link-type AUTO", processGetRequest(url, nil, `{"openconfig-spanning-tree:link-type":"AUTO"}`, false))
}

func Test_set_rstp_port_priority(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config/port-priority"
	body := `{"openconfig-spanning-tree:port-priority":16}`
	expected := map[string]interface{}{
		"STP_MST_PORT": map[string]interface{}{
			"0|" + stpTestIfName: map[string]interface{}{
				"path_cost": "1",
				"priority":  "16",
			},
		},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set RSTP port-priority", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify STP_MST_PORT priority", verifyDbResult(rclient, "STP_MST_PORT|0|"+stpTestIfName, expected, false))
}

func Test_set_rstp_port_priority_zero(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config/port-priority"
	body := `{"openconfig-spanning-tree:port-priority":0}`
	expected := map[string]interface{}{
		"STP_MST_PORT": map[string]interface{}{
			"0|" + stpTestIfName: map[string]interface{}{
				"path_cost": "1",
				"priority":  "0",
			},
		},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set RSTP port-priority 0", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify STP_MST_PORT priority is 0", verifyDbResult(rclient, "STP_MST_PORT|0|"+stpTestIfName, expected, false))
}

func Test_set_rstp_port_cost(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config/cost"
	body := `{"openconfig-spanning-tree:cost":2000}`
	expected := map[string]interface{}{
		"STP_MST_PORT": map[string]interface{}{
			"0|" + stpTestIfName: map[string]interface{}{
				"path_cost": "2000",
				"priority":  "128",
			},
		},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set RSTP cost", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify STP_MST_PORT path_cost", verifyDbResult(rclient, "STP_MST_PORT|0|"+stpTestIfName, expected, false))
}

func Test_set_rstp_intf_config_container(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config"
	body := `{"openconfig-spanning-tree:config":{"port-priority":32,"cost":4000}}`
	expected := map[string]interface{}{
		"STP_MST_PORT": map[string]interface{}{
			"0|" + stpTestIfName: map[string]interface{}{
				"priority":  "32",
				"path_cost": "4000",
			},
		},
	}

	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	time.Sleep(1 * time.Second)
	t.Run("set RSTP interface config container", processSetRequest(url, body, "POST", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("verify STP_MST_PORT priority and path_cost", verifyDbResult(rclient, "STP_MST_PORT|0|"+stpTestIfName, expected, false))
}

func Test_get_rstp_state_msti0(t *testing.T) {
	loadDB(db.ConfigDB, stpMstInstPrereq("32768"))
	loadDB(db.ApplDB, stpMstInstAppPrereq())
	time.Sleep(1 * time.Second)

	url := "/openconfig-spanning-tree:stp/rstp/state"
	t.Run("get RSTP state from MSTI 0", processGetRequest(url, nil, "", false))

	url = "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/state"
	t.Run("get RSTP interface state from MSTI 0", processGetRequest(url, nil, "", false))
}

func Test_get_rstp_config(t *testing.T) {
	loadDB(db.ConfigDB, stpMstInstPrereq("40960"))
	time.Sleep(1 * time.Second)

	url := "/openconfig-spanning-tree:stp/global/config/enabled-protocol"
	t.Run("get RSTP enabled-protocol", processGetRequest(url, nil, "", false))

	url = "/openconfig-spanning-tree:stp/rstp/config/bridge-priority"
	t.Run("get RSTP bridge priority", processGetRequest(url, nil, "", false))
}

func Test_get_rstp_interface_config(t *testing.T) {
	prereq := stpMstInstPrereq("32768")
	prereq["STP_MST_PORT"].(map[string]interface{})["0|"+stpTestIfName] = map[string]interface{}{
		"path_cost": "2000",
		"priority":  "0",
	}
	loadDB(db.ConfigDB, prereq)
	time.Sleep(1 * time.Second)

	url := "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config/port-priority"
	t.Run("get RSTP port-priority from CONFIG_DB", processGetRequest(url, nil, `{"openconfig-spanning-tree:port-priority":0}`, false))

	url = "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config/cost"
	t.Run("get RSTP cost from CONFIG_DB", processGetRequest(url, nil, `{"openconfig-spanning-tree:cost":2000}`, false))

	url = "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config"
	t.Run("get RSTP interface config container from CONFIG_DB", processGetRequest(url, nil,
		`{"openconfig-spanning-tree:config":{"name":"Ethernet0","cost":2000,"port-priority":0}}`, false))
}

func Test_get_rstp_interface_config_cli_key(t *testing.T) {
	prereq := stpMstInstPrereq("32768")
	delete(prereq["STP_MST_PORT"].(map[string]interface{}), "0|"+stpTestIfName)
	prereq["STP_MST_PORT"].(map[string]interface{})["MST_INSTANCE|0|"+stpTestIfName] = map[string]interface{}{
		"path_cost": "1",
		"priority":  "128",
	}
	prereq["STP_PORT"].(map[string]interface{})[stpTestIfName] = map[string]interface{}{
		"enabled":   "true",
		"path_cost": "99",
		"priority":  "16",
	}
	loadDB(db.ConfigDB, prereq)
	time.Sleep(1 * time.Second)

	url := "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config/port-priority"
	t.Run("get RSTP port-priority from CLI STP_MST_PORT key", processGetRequest(url, nil, `{"openconfig-spanning-tree:port-priority":128}`, false))

	url = "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config/cost"
	t.Run("get RSTP cost from CLI STP_MST_PORT key", processGetRequest(url, nil, `{"openconfig-spanning-tree:cost":1}`, false))

	url = "/openconfig-spanning-tree:stp/rstp/interfaces/interface[name=" + stpTestIfName + "]/config"
	t.Run("get RSTP interface config from CLI STP_MST_PORT key", processGetRequest(url, nil,
		`{"openconfig-spanning-tree:config":{"name":"Ethernet0","cost":1,"port-priority":128}}`, false))
}

func Test_get_stp_interface_config(t *testing.T) {
	prereq := stpMstInstPrereq("32768")
	prereq["STP_PORT"].(map[string]interface{})[stpTestIfName] = map[string]interface{}{
		"enabled":   "true",
		"edge_port": "true",
		"link_type": "point-to-point",
		"path_cost": "1",
		"priority":  "128",
	}
	loadDB(db.ConfigDB, prereq)
	time.Sleep(1 * time.Second)

	url := "/openconfig-spanning-tree:stp/interfaces/interface[name=" + stpTestIfName + "]/config/edge-port"
	t.Run("get STP edge-port", processGetRequest(url, nil, "", false))

	url = "/openconfig-spanning-tree:stp/interfaces/interface[name=" + stpTestIfName + "]/config/link-type"
	t.Run("get STP link-type", processGetRequest(url, nil, "", false))
}

func stpRstpDisablePrereq() map[string]interface{} {
	prereq := stpMstInstPrereq("32768")
	prereq["VLAN_MEMBER"] = stpEnableRstpPrereq()["VLAN_MEMBER"]
	return prereq
}

func verifyStpMstDisabled(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"STP|GLOBAL",
		"STP_MST|GLOBAL",
		"STP_MST_INST|0",
		"STP_PORT|" + stpTestIfName,
		"STP_MST_PORT|0|" + stpTestIfName,
	} {
		key := key
		t.Run("verify "+key+" removed", func(t *testing.T) {
			exists, err := rclient.Exists(context.Background(), key).Result()
			if err != nil {
				t.Fatalf("redis EXISTS failed for %s: %v", key, err)
			}
			if exists != 0 {
				t.Fatalf("expected %s to be deleted", key)
			}
		})
	}
}

func Test_delete_rstp_global_config(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config"

	loadDB(db.ConfigDB, stpRstpDisablePrereq())
	time.Sleep(1 * time.Second)
	t.Run("disable RSTP via DELETE on global config", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)
	verifyStpMstDisabled(t)
}

func Test_delete_rstp_enabled_protocol_leaf(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config/enabled-protocol"

	loadDB(db.ConfigDB, stpRstpDisablePrereq())
	time.Sleep(1 * time.Second)
	t.Run("disable RSTP via DELETE on enabled-protocol leaf", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)
	verifyStpMstDisabled(t)
}

func Test_delete_rstp_with_link_type_configured(t *testing.T) {
	url := "/openconfig-spanning-tree:stp/global/config/enabled-protocol"
	prereq := stpRstpDisablePrereq()
	prereq["STP_PORT"].(map[string]interface{})[stpTestIfName] = map[string]interface{}{
		"enabled":   "true",
		"edge_port": "false",
		"link_type": "shared",
		"priority":  "128",
	}

	loadDB(db.ConfigDB, prereq)
	time.Sleep(1 * time.Second)
	t.Run("disable RSTP with link_type configured", processDeleteRequest(url, false))
	time.Sleep(1 * time.Second)
	verifyStpMstDisabled(t)
}

func Test_get_stp_full_tree(t *testing.T) {
	loadDB(db.ConfigDB, stpMstInstPrereq("40960"))
	loadDB(db.ApplDB, stpMstInstAppPrereq())
	time.Sleep(1 * time.Second)

	url := "/openconfig-spanning-tree:stp"
	t.Run("get full STP tree", processGetRequest(url, nil, "", false))
}

func Test_get_rstp_when_disabled(t *testing.T) {
	loadDB(db.ConfigDB, map[string]interface{}{})
	time.Sleep(1 * time.Second)

	notFound := tlerr.NotFoundError{Format: "Resource not found"}
	for _, url := range []string{
		"/openconfig-spanning-tree:stp/rstp",
		"/openconfig-spanning-tree:stp/rstp/config/bridge-priority",
		"/openconfig-spanning-tree:stp/rstp/state",
		"/openconfig-spanning-tree:stp/rstp/state/root-port",
	} {
		url := url
		t.Run(url, processGetRequest(url, nil, "", true, notFound))
	}

	t.Run("get /stp omits rstp", processGetRequest(
		"/openconfig-spanning-tree:stp",
		nil,
		`{"openconfig-spanning-tree:stp":{}}`,
		false,
	))
}
