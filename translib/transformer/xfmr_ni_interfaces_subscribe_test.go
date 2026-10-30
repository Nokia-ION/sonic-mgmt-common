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

const niIntfSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=Vrf_red]/interfaces"

// The base rows are watched on-change: the one an id names, or those of every
// L3 interface table when the id is left open.
func TestSubscribe_ni_interfaces(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uri     string
		tables  []string
		keyPatt string
	}{
		{"the interface list", niIntfSubscUrl + "/interface", niIntfTables, "*"},
		{"a wildcard id", niIntfSubscUrl + "/interface[id=*]/state", niIntfTables, "*"},
		{"a keyed Ethernet interface config", niIntfSubscUrl + "/interface[id=Ethernet0]/config", []string{"INTERFACE"}, "Ethernet0"},
		{"a keyed subinterface state", niIntfSubscUrl + "/interface[id=Ethernet0.10]/state", []string{"VLAN_SUB_INTERFACE"}, "Ethernet0.10"},
		{"a keyed loopback id", niIntfSubscUrl + "/interface[id=Loopback0]/id", []string{"LOOPBACK_INTERFACE"}, "Loopback0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_intf_xfmr(XfmrSubscInParams{uri: tc.uri, requestURI: tc.uri, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("subscribe callback failed: %v", err)
			}
			if out.isVirtualTbl || out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
				t.Fatalf("isVirtualTbl=%v onChange=%v nOpts=%v, want an on-change mapping", out.isVirtualTbl, out.onChange, out.nOpts)
			}
			if len(out.dbDataMap[db.ConfigDB]) != len(tc.tables) {
				t.Fatalf("tables %v, want %v", out.dbDataMap[db.ConfigDB], tc.tables)
			}
			for _, table := range tc.tables {
				if _, ok := out.dbDataMap[db.ConfigDB][table][tc.keyPatt]; !ok {
					t.Fatalf("missing %s[%s]: %v", table, tc.keyPatt, out.dbDataMap)
				}
			}
		})
	}
}

func niIntfSubscPath(niName string) *gnmi.Path {
	return &gnmi.Path{Elem: []*gnmi.PathElem{
		{Name: "network-instances"},
		{Name: "network-instance", Key: map[string]string{"name": niName}},
		{Name: "interfaces"},
		{Name: "interface"},
	}}
}

// A notification for a base row names the instance its vrf_name binds it to,
// and the interface. An address row, and a row of another instance than the
// one subscribed to, are unrelated.
func TestDbToYangPath_ni_interfaces(t *testing.T) {
	schema := "/openconfig-network-instance:network-instances/network-instance/interfaces/interface/state"
	niPath := "/openconfig-network-instance:network-instances/network-instance"
	for _, tc := range []struct {
		name    string
		table   string
		key     []string
		entry   map[string]string
		subscNi string
		wantNi  string
		wantId  string
		ignored bool
	}{
		{"a VRF row under an open instance", "INTERFACE", []string{"Ethernet0"}, map[string]string{"vrf_name": "Vrf_red"}, "*", "Vrf_red", "Ethernet0", false},
		{"a default row under an open instance", "LOOPBACK_INTERFACE", []string{"Loopback0"}, map[string]string{"NULL": "NULL"}, "*", "default", "Loopback0", false},
		{"a default row under DEFAULT", "INTERFACE", []string{"Ethernet0"}, map[string]string{"NULL": "NULL"}, "DEFAULT", "DEFAULT", "Ethernet0", false},
		{"a row of another instance", "INTERFACE", []string{"Ethernet0"}, map[string]string{"vrf_name": "Vrf_blue"}, "Vrf_red", "", "", true},
		{"an address row", "INTERFACE", []string{"Ethernet0", "10.0.0.1/31"}, map[string]string{"NULL": "NULL"}, "*", "", "", true},
		{"a VNET row", "VLAN_INTERFACE", []string{"Vlan10"}, map[string]string{"vnet_name": "Vnet_1"}, "*", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := map[string]string{}
			entry := db.Value{Field: tc.entry}
			err := DbToYangPath_ni_intf_path_xfmr(XfmrDbToYgPathParams{
				subscribePath: niIntfSubscPath(tc.subscNi),
				tblName:       tc.table,
				tblKeyComp:    tc.key,
				tblEntry:      &entry,
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
			if keys[niPath+"/name"] != tc.wantNi || keys[niPath+"/interfaces/interface/id"] != tc.wantId {
				t.Fatalf("keys %v, want instance %s interface %s", keys, tc.wantNi, tc.wantId)
			}
		})
	}
}
