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

const niContainerDefaultURL = "/openconfig-network-instance:network-instances/network-instance[name=default]"
const niContainerBlueURL = "/openconfig-network-instance:network-instances/network-instance[name=blue]"

// The instance has no CONFIG_DB row. Name and type are derived from the list
// key: default and DEFAULT are DEFAULT_INSTANCE; every other name is L3VRF.
func Test_openconfig_network_instance_derived_config_type(t *testing.T) {
	loadDB(db.ConfigDB, map[string]interface{}{})
	time.Sleep(1 * time.Second)

	expectedConfigJSON := `{"openconfig-network-instance:config":{"name":"default","type":"openconfig-network-instance-types:DEFAULT_INSTANCE"}}`
	t.Run("GET the default instance config container",
		processGetRequest(niContainerDefaultURL+"/config", nil, expectedConfigJSON, false))
	time.Sleep(1 * time.Second)

	expectedTypeJSON := `"openconfig-network-instance-types:DEFAULT_INSTANCE"`
	t.Run("GET the default instance config type leaf",
		processGetRequest(niContainerDefaultURL+"/config/type", nil, expectedTypeJSON, false))
	time.Sleep(1 * time.Second)

	instanceJSON := `{"name":"default","config":{"name":"default","type":"openconfig-network-instance-types:DEFAULT_INSTANCE"}}`
	t.Run("PATCH the default instance naming the derived type is a no-op",
		processSetRequest(niContainerDefaultURL, instanceJSON, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("PATCH the default instance with a wrong type is refused",
		processSetRequest(niContainerDefaultURL,
			`{"name":"default","config":{"name":"default","type":"openconfig-network-instance-types:L3VRF"}}`,
			"PATCH", true, tlerr.InvalidArgsError{
				Format: `network-instance type "L3VRF" does not match the derived type "DEFAULT_INSTANCE" for instance "default"`,
			}))
	time.Sleep(1 * time.Second)

	t.Run("PATCH a payload naming config/enabled is still refused",
		processSetRequest("/openconfig-network-instance:network-instances",
			`{"network-instance":[{"name":"default","config":{"name":"default","enabled":true}}]}`,
			"PATCH", true, tlerr.NotSupportedError{
				Format: "network-instance payload carries nodes that have no CONFIG_DB mapping: config/enabled"}))
	time.Sleep(1 * time.Second)

	loadDB(db.ConfigDB, map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"blue": map[string]interface{}{
				"local_asn": "65001",
			},
		},
	})
	time.Sleep(1 * time.Second)

	expectedBlueConfigJSON := `{"openconfig-network-instance:config":{"name":"blue","type":"openconfig-network-instance-types:L3VRF"}}`
	t.Run("GET a non-default instance config reports L3VRF",
		processGetRequest(niContainerBlueURL+"/config", nil, expectedBlueConfigJSON, false))
	time.Sleep(1 * time.Second)

	t.Run("PATCH a non-default instance with L2VSI is refused",
		processSetRequest(niContainerBlueURL,
			`{"name":"blue","config":{"name":"blue","type":"openconfig-network-instance-types:L2VSI"}}`,
			"PATCH", true, tlerr.NotSupportedError{
				Format: `network-instance "blue" supports only L3VRF as a non-default type`,
			}))
	time.Sleep(1 * time.Second)

	unloadDB(db.ConfigDB, map[string]interface{}{
		"BGP_GLOBALS": map[string]interface{}{
			"blue": "",
		},
	})
}
