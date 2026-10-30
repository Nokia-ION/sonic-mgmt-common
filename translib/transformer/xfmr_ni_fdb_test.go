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

package transformer_test

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const niFdbUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/fdb"

const niFdbVlan100 = "oid:0x26000000000a01"
const niFdbVlanDefault = "oid:0x26000000000a00"

func niFdbKey(bvid, mac string) string {
	return `{"bvid":"` + bvid + `","mac":"` + mac + `","switch_id":"oid:0x21000000000000"}`
}

// The ASIC_DB objects of a MAC table: VLAN 100 and the default VLAN, a bridge
// port on Ethernet120 and one on PortChannel0120, an entry learned on each, one
// behind a remote endpoint and one on the default VLAN.
func niFdbAsicMap() map[string]interface{} {
	return map[string]interface{}{
		"ASIC_STATE": map[string]interface{}{
			"SAI_OBJECT_TYPE_VLAN:" + niFdbVlan100:                                         map[string]interface{}{"SAI_VLAN_ATTR_VLAN_ID": "100"},
			"SAI_OBJECT_TYPE_VLAN:" + niFdbVlanDefault:                                     map[string]interface{}{"NULL": "NULL"},
			"SAI_OBJECT_TYPE_BRIDGE_PORT:oid:0x3a000000000b01":                             map[string]interface{}{"SAI_BRIDGE_PORT_ATTR_PORT_ID": "oid:0x1000000000c01", "SAI_BRIDGE_PORT_ATTR_TYPE": "SAI_BRIDGE_PORT_TYPE_PORT"},
			"SAI_OBJECT_TYPE_BRIDGE_PORT:oid:0x3a000000000b02":                             map[string]interface{}{"SAI_BRIDGE_PORT_ATTR_PORT_ID": "oid:0x2000000000c02", "SAI_BRIDGE_PORT_ATTR_TYPE": "SAI_BRIDGE_PORT_TYPE_PORT"},
			"SAI_OBJECT_TYPE_FDB_ENTRY:" + niFdbKey(niFdbVlan100, "00:11:22:33:44:55"):     map[string]interface{}{"SAI_FDB_ENTRY_ATTR_TYPE": "SAI_FDB_ENTRY_TYPE_DYNAMIC", "SAI_FDB_ENTRY_ATTR_BRIDGE_PORT_ID": "oid:0x3a000000000b01"},
			"SAI_OBJECT_TYPE_FDB_ENTRY:" + niFdbKey(niFdbVlan100, "00:AA:BB:CC:DD:EE"):     map[string]interface{}{"SAI_FDB_ENTRY_ATTR_TYPE": "SAI_FDB_ENTRY_TYPE_STATIC", "SAI_FDB_ENTRY_ATTR_BRIDGE_PORT_ID": "oid:0x3a000000000b02"},
			"SAI_OBJECT_TYPE_FDB_ENTRY:" + niFdbKey(niFdbVlan100, "00:11:22:33:44:66"):     map[string]interface{}{"SAI_FDB_ENTRY_ATTR_TYPE": "SAI_FDB_ENTRY_TYPE_DYNAMIC", "SAI_FDB_ENTRY_ATTR_ENDPOINT_IP": "10.0.0.9"},
			"SAI_OBJECT_TYPE_FDB_ENTRY:" + niFdbKey(niFdbVlanDefault, "00:11:22:33:44:77"): map[string]interface{}{"SAI_FDB_ENTRY_ATTR_TYPE": "SAI_FDB_ENTRY_TYPE_DYNAMIC", "SAI_FDB_ENTRY_ATTR_BRIDGE_PORT_ID": "oid:0x3a000000000b01"},
		},
	}
}

func niFdbLoad(t *testing.T) func() {
	asic := getDbClient(int(db.AsicDB))
	ctx := context.Background()
	for _, sub := range niFdbAsicMap() {
		for key, fv := range sub.(map[string]interface{}) {
			if err := asic.HSet(ctx, "ASIC_STATE:"+key, fv.(map[string]interface{})).Err(); err != nil {
				t.Fatalf("load ASIC_STATE:%s: %v", key, err)
			}
		}
	}
	ctr := rclientDBNum[db.CountersDB]
	ctr.HSet(ctx, "COUNTERS_PORT_NAME_MAP", "Ethernet120", "oid:0x1000000000c01")
	ctr.HSet(ctx, "COUNTERS_LAG_NAME_MAP", "PortChannel0120", "oid:0x2000000000c02")
	return func() {
		for _, sub := range niFdbAsicMap() {
			for key := range sub.(map[string]interface{}) {
				asic.Del(ctx, "ASIC_STATE:"+key)
			}
		}
		ctr.HDel(ctx, "COUNTERS_PORT_NAME_MAP", "Ethernet120")
		ctr.HDel(ctx, "COUNTERS_LAG_NAME_MAP", "PortChannel0120")
		asic.Close()
	}
}

// The MAC table reports the ASIC_DB FDB entries of every VLAN with an id, by
// MAC and VLAN, with the interface each was learned on; it is read-only.
func Test_openconfig_network_instance_fdb_mac_table(t *testing.T) {
	cleanup := niFdbLoad(t)
	defer cleanup()
	time.Sleep(1 * time.Second)

	e55 := `{"interface":{"interface-ref":{"state":{"interface":"Ethernet120"}}},"mac-address":"00:11:22:33:44:55",` +
		`"state":{"entry-type":"DYNAMIC","mac-address":"00:11:22:33:44:55","vlan":100},"vlan":100}`
	e66 := `{"mac-address":"00:11:22:33:44:66","state":{"entry-type":"DYNAMIC","mac-address":"00:11:22:33:44:66","vlan":100},"vlan":100}`
	eEE := `{"interface":{"interface-ref":{"state":{"interface":"PortChannel0120"}}},"mac-address":"00:aa:bb:cc:dd:ee",` +
		`"state":{"entry-type":"STATIC","mac-address":"00:aa:bb:cc:dd:ee","vlan":100},"vlan":100}`
	entries := `{"entry":[` + e55 + `,` + e66 + `,` + eEE + `]}`

	t.Run("GET FDB mac-table entries",
		processGetRequest(niFdbUrl+"/mac-table/entries", nil, `{"openconfig-network-instance:entries":`+entries+`}`, false))
	t.Run("GET FDB container",
		processGetRequest(niFdbUrl, nil, `{"openconfig-network-instance:fdb":{"mac-table":{"entries":`+entries+`}}}`, false))
	e55Url := niFdbUrl + "/mac-table/entries/entry[mac-address=00:11:22:33:44:55][vlan=100]"
	t.Run("GET FDB entry",
		processGetRequest(e55Url, nil, `{"openconfig-network-instance:entry":[`+e55+`]}`, false))
	t.Run("GET FDB entry state",
		processGetRequest(e55Url+"/state", nil,
			`{"openconfig-network-instance:state":{"entry-type":"DYNAMIC","mac-address":"00:11:22:33:44:55","vlan":100}}`, false))
	t.Run("GET FDB entry state entry-type leaf",
		processGetRequest(e55Url+"/state/entry-type", nil, `{"openconfig-network-instance:entry-type":"DYNAMIC"}`, false))
	t.Run("GET FDB entry interface-ref state interface leaf",
		processGetRequest(e55Url+"/interface/interface-ref/state/interface", nil, `{"openconfig-network-instance:interface":"Ethernet120"}`, false))
	t.Run("GET FDB entry not in the table",
		processGetRequest(niFdbUrl+"/mac-table/entries/entry[mac-address=00:11:22:33:44:77][vlan=100]", nil, "", true,
			tlerr.NotFoundError{Format: "FDB entry 00:11:22:33:44:77 on VLAN 100 not found"}))
	t.Run("GET FDB entry of another instance",
		processGetRequest("/openconfig-network-instance:network-instances/network-instance[name=Vrf_red]/fdb/mac-table/entries/entry[mac-address=00:11:22:33:44:55][vlan=100]", nil, "", true,
			tlerr.NotFoundError{Format: "network-instance Vrf_red has no FDB; the MAC table is served under the default instance"}))
	time.Sleep(1 * time.Second)

	t.Run("PATCH FDB config mac-aging-time rejected",
		processSetRequest(niFdbUrl+"/config/mac-aging-time", `{"openconfig-network-instance:mac-aging-time":300}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "the FDB of a network instance is read-only: SONiC has no configuration for MAC learning, aging or the size of the MAC table"}))
	t.Run("PATCH FDB entry config rejected",
		processSetRequest(niFdbUrl+"/mac-table/entries/entry[mac-address=00:11:22:33:44:88][vlan=100]/config",
			`{"mac-address":"00:11:22:33:44:88","vlan":100}`, "PATCH", true,
			tlerr.NotSupportedError{Format: "the FDB of a network instance reports the MAC table the ASIC holds and cannot be written: SONiC has no configuration for static MAC entries"}))
	t.Run("DELETE FDB entry rejected",
		processDeleteRequest(e55Url, true,
			tlerr.NotSupportedError{Format: "the FDB of a network instance reports the MAC table the ASIC holds and cannot be written: SONiC has no configuration for static MAC entries"}))
	time.Sleep(1 * time.Second)
}
