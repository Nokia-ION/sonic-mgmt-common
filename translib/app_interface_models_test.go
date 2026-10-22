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

package translib

import "testing"

// A module registered by an app and loaded by the transformer as well is
// listed once, with the registration that carries a version.
func TestAddModel_lists_a_module_once(t *testing.T) {
	saved := models
	defer func() { models = saved }()
	models = nil

	addModel(&ModelData{Name: "openconfig-x", Org: "OpenConfig working group"})
	addModel(&ModelData{Name: "openconfig-x", Org: "OpenConfig working group", Ver: "1.0.0"})
	addModel(&ModelData{Name: "openconfig-x", Org: "OpenConfig working group", Ver: "2.0.0"})
	addModel(&ModelData{Name: "openconfig-y", Org: "OpenConfig working group", Ver: "0.1.0"})

	if len(models) != 2 {
		t.Fatalf("models %v, want one entry per module", models)
	}
	if models[0].Name != "openconfig-x" || models[0].Ver != "1.0.0" {
		t.Fatalf("openconfig-x listed as %v, want the first registration with a version", models[0])
	}
}

// Every model translib reports carries a version.
func TestGetModels_every_model_has_a_version(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range getModels() {
		if seen[m.Name] {
			t.Errorf("%s is listed more than once", m.Name)
		}
		seen[m.Name] = true
		if m.Ver == "" {
			t.Errorf("%s has no version", m.Name)
		}
	}
	for _, name := range []string{"openconfig-bgp", "openconfig-bgp-policy", "openconfig-local-routing", "openconfig-aft"} {
		if !seen[name] {
			t.Errorf("%s is not listed", name)
		}
	}
}
