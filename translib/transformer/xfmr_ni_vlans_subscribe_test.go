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
	"errors"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
	"github.com/openconfig/gnmi/proto/gnmi"
)

const niVlanSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/vlans"

// The config is watched on CONFIG_DB VLAN, the state on APPL_DB VLAN_TABLE and
// the members on VLAN_MEMBER_TABLE, all on change. Another instance maps
// nothing.
func TestSubscribe_ni_vlans(t *testing.T) {
	type tbl struct {
		dbNum db.DBNum
		table string
		key   string
	}
	for _, tc := range []struct {
		name   string
		uri    string
		tables []tbl
	}{
		{"the vlans container", niVlanSubscUrl, []tbl{{db.ConfigDB, "VLAN", "*"}, {db.ApplDB, "VLAN_TABLE", "*"}, {db.ApplDB, "VLAN_MEMBER_TABLE", "*"}}},
		{"a keyed VLAN", niVlanSubscUrl + "/vlan[vlan-id=3101]", []tbl{{db.ConfigDB, "VLAN", "Vlan3101"}, {db.ApplDB, "VLAN_TABLE", "Vlan3101"}, {db.ApplDB, "VLAN_MEMBER_TABLE", "Vlan3101:*"}}},
		{"a keyed VLAN config", niVlanSubscUrl + "/vlan[vlan-id=3101]/config", []tbl{{db.ConfigDB, "VLAN", "Vlan3101"}}},
		{"a wildcard VLAN state status", niVlanSubscUrl + "/vlan[vlan-id=*]/state/status", []tbl{{db.ApplDB, "VLAN_TABLE", "*"}}},
		{"a keyed VLAN members", niVlanSubscUrl + "/vlan[vlan-id=3101]/members", []tbl{{db.ApplDB, "VLAN_MEMBER_TABLE", "Vlan3101:*"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_vlans_xfmr(XfmrSubscInParams{uri: tc.uri, requestURI: tc.uri, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("subscribe callback failed: %v", err)
			}
			if out.isVirtualTbl || out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
				t.Fatalf("isVirtualTbl=%v onChange=%v nOpts=%v, want an on-change mapping", out.isVirtualTbl, out.onChange, out.nOpts)
			}
			n := 0
			for _, tables := range out.dbDataMap {
				n += len(tables)
			}
			if n != len(tc.tables) {
				t.Fatalf("tables %v, want %v", out.dbDataMap, tc.tables)
			}
			for _, want := range tc.tables {
				if _, ok := out.dbDataMap[want.dbNum][want.table][want.key]; !ok {
					t.Fatalf("missing %s[%s]: %v", want.table, want.key, out.dbDataMap)
				}
			}
		})
	}

	uri := "/openconfig-network-instance:network-instances/network-instance[name=Vrf_red]/vlans/vlan[vlan-id=3101]"
	out, err := Subscribe_ni_vlans_xfmr(XfmrSubscInParams{uri: uri, requestURI: uri, subscProc: TRANSLATE_SUBSCRIBE})
	if err != nil || !out.isVirtualTbl || len(out.dbDataMap) != 0 {
		t.Fatalf("another instance: err=%v isVirtualTbl=%v map=%v, want nothing mapped", err, out.isVirtualTbl, out.dbDataMap)
	}
}

func niVlanSubscPath(niName string) *gnmi.Path {
	return &gnmi.Path{Elem: []*gnmi.PathElem{
		{Name: "network-instances"},
		{Name: "network-instance", Key: map[string]string{"name": niName}},
		{Name: "vlans"},
		{Name: "vlan"},
	}}
}

// A notification names the VLAN its row belongs to, under the default
// instance. Another table, a key naming no VLAN, and a subscription to another
// instance are unrelated.
func TestDbToYangPath_ni_vlans(t *testing.T) {
	schema := "/openconfig-network-instance:network-instances/network-instance/vlans/vlan/state"
	niPath := "/openconfig-network-instance:network-instances/network-instance"
	for _, tc := range []struct {
		name    string
		table   string
		key     []string
		subscNi string
		wantNi  string
		wantId  string
		ignored bool
	}{
		{"a VLAN row under an open instance", "VLAN", []string{"Vlan3101"}, "*", "default", "3101", false},
		{"a VLAN_TABLE row under DEFAULT", "VLAN_TABLE", []string{"Vlan3101"}, "DEFAULT", "DEFAULT", "3101", false},
		{"a VLAN_MEMBER_TABLE row", "VLAN_MEMBER_TABLE", []string{"Vlan3101", "Ethernet0"}, "default", "default", "3101", false},
		{"a row of another table", "PORT", []string{"Ethernet0"}, "*", "", "", true},
		{"a key naming no VLAN", "VLAN", []string{"Ethernet0"}, "*", "", "", true},
		{"a subscription to another instance", "VLAN", []string{"Vlan3101"}, "Vrf_red", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := map[string]string{}
			err := DbToYangPath_ni_vlans_path_xfmr(XfmrDbToYgPathParams{
				subscribePath: niVlanSubscPath(tc.subscNi),
				tblName:       tc.table,
				tblKeyComp:    tc.key,
				ygSchemaPath:  schema,
				ygPathKeys:    keys,
			})
			if tc.ignored {
				if !errors.Is(err, apis.ErrUnrelatedEntry) {
					t.Fatalf("err=%v keys=%v, want an unrelated entry", err, keys)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v", err)
			}
			if keys[niPath+"/name"] != tc.wantNi || keys[niPath+"/vlans/vlan/vlan-id"] != tc.wantId {
				t.Fatalf("keys %v, want instance %s VLAN %s", keys, tc.wantNi, tc.wantId)
			}
		})
	}
}
