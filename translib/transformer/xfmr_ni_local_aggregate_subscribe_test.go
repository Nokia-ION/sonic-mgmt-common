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

const niLaSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=LOCAL_AGGREGATE][name=bgp]"

// The configured aggregates and the ones FRR holds are each watched on-change,
// a row arriving or going away being the whole story.
func TestSubscribe_ni_local_aggregate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uri     string
		dbNum   db.DBNum
		table   string
		keyPatt string
	}{
		{"the aggregate list", niLaSubscUrl + "/local-aggregates/aggregate", db.ConfigDB, "BGP_GLOBALS_AF_AGGREGATE_ADDR", "default|*|*"},
		{"a wildcard prefix", niLaSubscUrl + "/local-aggregates/aggregate[prefix=*]", db.ConfigDB, "BGP_GLOBALS_AF_AGGREGATE_ADDR", "default|*|*"},
		{"a keyed IPv4 aggregate config", niLaSubscUrl + "/local-aggregates/aggregate[prefix=10.10.0.0/16]/config",
			db.ConfigDB, "BGP_GLOBALS_AF_AGGREGATE_ADDR", "default|ipv4_unicast|10.10.0.0/16"},
		{"a keyed IPv6 aggregate state", niLaSubscUrl + "/local-aggregates/aggregate[prefix=2001:db8::/32]/state",
			db.StateDB, "BGP_GLOBALS_AF_AGGREGATE_ADDR_APPLIED", "default|ipv6_unicast|2001:db8::/32"},
		{"state of every aggregate", niLaSubscUrl + "/local-aggregates/aggregate/state",
			db.StateDB, "BGP_GLOBALS_AF_AGGREGATE_ADDR_APPLIED", "default|*|*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_local_aggr_cfg_xfmr(XfmrSubscInParams{uri: tc.uri, requestURI: tc.uri, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("subscribe callback failed: %v", err)
			}
			if out.isVirtualTbl || out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
				t.Fatalf("isVirtualTbl=%v onChange=%v nOpts=%v, want an on-change mapping", out.isVirtualTbl, out.onChange, out.nOpts)
			}
			if _, ok := out.dbDataMap[tc.dbNum][tc.table][tc.keyPatt]; !ok {
				t.Fatalf("missing %s[%s]: %v", tc.table, tc.keyPatt, out.dbDataMap)
			}
		})
	}
}

func TestSubscribe_ni_local_aggregate_other_instance_name(t *testing.T) {
	uri := "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=LOCAL_AGGREGATE][name=agg]/local-aggregates/aggregate"
	out, err := Subscribe_ni_local_aggr_cfg_xfmr(XfmrSubscInParams{uri: uri, requestURI: uri, subscProc: TRANSLATE_SUBSCRIBE})
	if err != nil || !out.isVirtualTbl || out.dbDataMap != nil {
		t.Fatalf("err=%v isVirtualTbl=%v dbDataMap=%v, want no mapping", err, out.isVirtualTbl, out.dbDataMap)
	}
}

// A notification for a row a subscription left the key of open names the
// instance and the aggregate it belongs to.
func TestDbToYangPath_ni_local_aggregate(t *testing.T) {
	schema := "/openconfig-network-instance:network-instances/network-instance/protocols/protocol/local-aggregates/aggregate/state"
	for _, table := range []string{"BGP_GLOBALS_AF_AGGREGATE_ADDR", "BGP_GLOBALS_AF_AGGREGATE_ADDR_APPLIED"} {
		keys := map[string]string{}
		err := DbToYangPath_ni_local_aggr_path_xfmr(XfmrDbToYgPathParams{
			tblName:      table,
			tblKeyComp:   []string{"Vrf_red", "ipv6_unicast", "2001:db8::/32"},
			ygSchemaPath: schema,
			ygPathKeys:   keys,
		})
		if err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		niPath := "/openconfig-network-instance:network-instances/network-instance"
		if keys[niPath+"/name"] != "Vrf_red" || keys[niPath+"/protocols/protocol/local-aggregates/aggregate/prefix"] != "2001:db8::/32" {
			t.Fatalf("%s: keys %v", table, keys)
		}
	}
}
