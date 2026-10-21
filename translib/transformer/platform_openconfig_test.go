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
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib"
	"github.com/Azure/sonic-mgmt-common/translib/db"
)

func platformComponentUrl(name string) string {
	return "/openconfig-platform:components/component[name=" + name + "]"
}

// Writes the given STATE_DB rows, keyed by table then key, for the duration of the test.
func loadPlatformStateRows(t *testing.T, rows map[string]interface{}) {
	t.Helper()
	unloadDB(db.StateDB, rows)
	loadDB(db.StateDB, rows)
	t.Cleanup(func() { unloadDB(db.StateDB, rows) })
}

// A get on a named component succeeds when the component has no row in the
// other tables it is read from.
func Test_openconfig_platform_state_name_without_other_rows(t *testing.T) {
	loadPlatformStateRows(t, map[string]interface{}{
		"TRANSCEIVER_STATUS":     map[string]interface{}{"Ethernet96": map[string]interface{}{"status": "1"}},
		"TRANSCEIVER_INFO":       map[string]interface{}{"Ethernet96": map[string]interface{}{"manufacturer": "x"}},
		"TRANSCEIVER_DOM_SENSOR": map[string]interface{}{"Ethernet96": map[string]interface{}{"temperature": "30"}},
		"STORAGE_INFO":           map[string]interface{}{"sdz": map[string]interface{}{"device_model": "x"}},
	})
	for _, name := range []string{"Ethernet96", "sdz"} {
		url := platformComponentUrl(name) + "/state/name"
		t.Run(url, processGetRequest(url, nil, `{"openconfig-platform:name":"`+name+`"}`, false))
	}
}

// A get on a component, its config and its config/name each carries config/name.
func Test_openconfig_platform_config_name(t *testing.T) {
	loadPlatformStateRows(t, map[string]interface{}{
		"FAN_INFO":         map[string]interface{}{"Fan9": map[string]interface{}{"presence": "true"}},
		"FAN_DRAWER_INFO":  map[string]interface{}{"drawer9": map[string]interface{}{"presence": "true"}},
		"PSU_INFO":         map[string]interface{}{"PSU 9": map[string]interface{}{"presence": "true"}},
		"STORAGE_INFO":     map[string]interface{}{"sdy": map[string]interface{}{"device_model": "x"}},
		"TEMPERATURE_INFO": map[string]interface{}{"Test Sensor9": map[string]interface{}{"temperature": "30"}},
		"CHASSIS_INFO":     map[string]interface{}{"chassis 9": map[string]interface{}{"serial": "x"}},
		"CPU_INFO":         map[string]interface{}{"host-cpu": map[string]interface{}{"NULL": "NULL"}},
	})
	for _, name := range []string{"Fan9", "drawer9", "PSU 9", "sdy", "Test Sensor9", "host-cpu", "chassis 9"} {
		comp := platformComponentUrl(name)
		t.Run(comp, func(t *testing.T) {
			resp, err := translib.Get(translib.GetRequest{Path: comp, User: translib.UserRoles{Name: "admin", Roles: []string{"admin"}}})
			if err != nil {
				t.Fatalf("GET %s failed: %v", comp, err)
			}
			var got struct {
				Component []struct {
					Config map[string]interface{} `json:"config"`
				} `json:"openconfig-platform:component"`
			}
			if err := json.Unmarshal(resp.Payload, &got); err != nil {
				t.Fatalf("GET %s returned %s: %v", comp, resp.Payload, err)
			}
			want := map[string]interface{}{"name": name}
			if len(got.Component) != 1 || !reflect.DeepEqual(got.Component[0].Config, want) {
				t.Fatalf("GET %s returned %s, want config %v", comp, resp.Payload, want)
			}
		})
		t.Run(comp+"/config", processGetRequest(comp+"/config", nil, `{"openconfig-platform:config":{"name":"`+name+`"}}`, false))
		t.Run(comp+"/config/name", processGetRequest(comp+"/config/name", nil, `{"openconfig-platform:name":"`+name+`"}`, false))
	}
}
