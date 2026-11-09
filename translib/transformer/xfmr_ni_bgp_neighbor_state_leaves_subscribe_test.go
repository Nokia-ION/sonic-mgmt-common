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

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

// allow-own-as state watches the allow_own_as field of every family row of
// the neighbor, on change.
func TestSubscribe_ni_bgp_nbr_as_path_options_state(t *testing.T) {
	for _, tc := range []struct{ name, nbr, key string }{
		{"a keyed neighbor", "11.0.0.155", "default|11.0.0.155|*"},
		{"a wildcard neighbor", "", "default|*|*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := niBgpNbrStateLeafSubscribe(&niBgpNbrStateLeafReq{niName: "default", neighborAddr: tc.nbr,
				container: "as-path-options/state", leaf: "allow-own-as"})
			if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
				t.Fatalf("onChange=%v nOpts=%v, want on change", out.onChange, out.nOpts)
			}
			flds := out.dbDataMap[db.StateDB][niBgpNbrAfStatsTbl][tc.key]
			if flds["allow_own_as"] != "allow-own-as" || len(out.dbDataMap[db.StateDB]) != 1 || len(out.dbDataMap) != 1 {
				t.Fatalf("dbDataMap %v, want BGP_NEIGH_AF_STATS %s allow_own_as only", out.dbDataMap, tc.key)
			}
		})
	}
}
