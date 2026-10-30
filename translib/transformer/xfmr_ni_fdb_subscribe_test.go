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

const niFdbSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/fdb"

// The MAC table is sampled from the ASIC_DB FDB entries; the FDB config and
// state, and the FDB of another instance, map nothing.
func TestSubscribe_ni_fdb(t *testing.T) {
	for _, tc := range []struct {
		name    string
		uri     string
		virtual bool
	}{
		{"the entry list", niFdbSubscUrl + "/mac-table/entries/entry", false},
		{"an entry state", niFdbSubscUrl + "/mac-table/entries/entry[mac-address=00:11:22:33:44:55][vlan=100]/state", false},
		{"the FDB container", niFdbSubscUrl, false},
		{"the FDB config", niFdbSubscUrl + "/config", true},
		{"another instance", "/openconfig-network-instance:network-instances/network-instance[name=Vrf_red]/fdb/mac-table", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_fdb_xfmr(XfmrSubscInParams{uri: tc.uri, requestURI: tc.uri, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("subscribe callback failed: %v", err)
			}
			if tc.virtual {
				if !out.isVirtualTbl || out.dbDataMap != nil {
					t.Fatalf("isVirtualTbl=%v dbDataMap=%v, want no mapping", out.isVirtualTbl, out.dbDataMap)
				}
				return
			}
			if out.isVirtualTbl || out.onChange != OnchangeDisable || out.nOpts == nil || out.nOpts.pType != Sample {
				t.Fatalf("isVirtualTbl=%v onChange=%v nOpts=%v, want a sampled mapping", out.isVirtualTbl, out.onChange, out.nOpts)
			}
			if _, ok := out.dbDataMap[db.AsicDB]["ASIC_STATE"]["SAI_OBJECT_TYPE_FDB_ENTRY:*"]; !ok {
				t.Fatalf("missing ASIC_STATE FDB entries: %v", out.dbDataMap)
			}
		})
	}
}

func niFdbSubscPath(mac, vlan string) *gnmi.Path {
	return &gnmi.Path{Elem: []*gnmi.PathElem{
		{Name: "network-instances"},
		{Name: "network-instance", Key: map[string]string{"name": "default"}},
		{Name: "fdb"},
		{Name: "mac-table"},
		{Name: "entries"},
		{Name: "entry", Key: map[string]string{"mac-address": mac, "vlan": vlan}},
	}}
}

// An FDB entry names the entry by its MAC and the VLAN id its VLAN object
// holds; an entry another subscription key names, or one on no VLAN, is
// unrelated.
func TestDbToYangPath_ni_fdb(t *testing.T) {
	asicDb, err := db.NewDB(getDBOptions(db.AsicDB))
	if err != nil {
		t.Fatalf("open ASIC_DB: %v", err)
	}
	defer asicDb.DeleteDB()
	vlanKey := db.Key{Comp: []string{"SAI_OBJECT_TYPE_VLAN", "oid:0x26000000000d01"}}
	ts := &db.TableSpec{Name: "ASIC_STATE", CompCt: 2}
	if err := asicDb.SetEntry(ts, vlanKey, db.Value{Field: map[string]string{"SAI_VLAN_ATTR_VLAN_ID": "200"}}); err != nil {
		t.Fatalf("load VLAN object: %v", err)
	}
	defer asicDb.DeleteEntry(ts, vlanKey)

	schema := "/openconfig-network-instance:network-instances/network-instance/fdb/mac-table/entries/entry/state"
	niPath := "/openconfig-network-instance:network-instances/network-instance"
	entryPath := niPath + "/fdb/mac-table/entries/entry"
	for _, tc := range []struct {
		name     string
		key      string
		mac      string
		vlan     string
		wantMac  string
		wantVlan string
		ignored  bool
	}{
		{"an entry by its VLAN object", `{"bvid":"oid:0x26000000000d01","mac":"00:AA:BB:CC:DD:01","switch_id":"oid:0x21000000000000"}`, "*", "*", "00:aa:bb:cc:dd:01", "200", false},
		{"an entry keyed by its VLAN id", `{"mac":"00:aa:bb:cc:dd:02","switch_id":"oid:0x21000000000000","vlan":"300"}`, "*", "*", "00:aa:bb:cc:dd:02", "300", false},
		{"an entry another key names", `{"mac":"00:aa:bb:cc:dd:02","switch_id":"oid:0x21000000000000","vlan":"300"}`, "00:aa:bb:cc:dd:03", "300", "", "", true},
		{"an entry on no VLAN", `{"bvid":"oid:0x26000000000d09","mac":"00:aa:bb:cc:dd:04","switch_id":"oid:0x21000000000000"}`, "*", "*", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := map[string]string{}
			err := DbToYangPath_ni_fdb_path_xfmr(XfmrDbToYgPathParams{
				subscribePath: niFdbSubscPath(tc.mac, tc.vlan),
				tblName:       "ASIC_STATE",
				tblKeyComp:    []string{"SAI_OBJECT_TYPE_FDB_ENTRY", tc.key},
				dbNum:         db.AsicDB,
				db:            asicDb,
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
			if keys[niPath+"/name"] != "default" || keys[entryPath+"/mac-address"] != tc.wantMac || keys[entryPath+"/vlan"] != tc.wantVlan {
				t.Fatalf("keys %v, want %s on VLAN %s", keys, tc.wantMac, tc.wantVlan)
			}
		})
	}
}
