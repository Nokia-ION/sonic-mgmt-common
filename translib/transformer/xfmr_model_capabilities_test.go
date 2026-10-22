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

package transformer

import "testing"

// The model capabilities carry the version each module declares, including the
// modules served under another one without an annotation file of their own.
func TestModelCapabilities_versions(t *testing.T) {
	for name, ver := range map[string]string{
		"openconfig-network-instance": "0.12.0",
		"openconfig-routing-policy":   "3.4.2",
		"openconfig-bgp":              "9.6.0",
		"openconfig-bgp-policy":       "8.2.0",
		"openconfig-local-routing":    "2.0.1",
		"openconfig-aft":              "0.3.2",
	} {
		info, ok := AddModelCpbltInfo()[name]
		if !ok || info == nil {
			t.Errorf("%s is not in the model capabilities", name)
			continue
		}
		if info.Ver != ver {
			t.Errorf("%s version %q, want %q", name, info.Ver, ver)
		}
		if info.Org != "OpenConfig working group" {
			t.Errorf("%s organization %q", name, info.Org)
		}
	}
}

func TestModelCapabilities_every_module_has_a_version(t *testing.T) {
	for name, info := range AddModelCpbltInfo() {
		if info == nil || info.Ver == "" {
			t.Errorf("%s has no version in the model capabilities", name)
		}
	}
}
