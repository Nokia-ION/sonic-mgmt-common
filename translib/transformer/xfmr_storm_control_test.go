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

package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

func TestYangToDb_storm_control_xfmr_nonPhysicalDeleteIsNoop(t *testing.T) {
	uris := []string{
		"/openconfig-interfaces:interfaces/interface[name=PortChannel100]",
		"/openconfig-interfaces:interfaces/interface[name=PortChannel100]/openconfig-if-ethernet:ethernet/openconfig-storm-control-ext:storm-control",
		"/openconfig-interfaces:interfaces/interface[name=Vlan10]",
	}
	for _, uri := range uris {
		got, err := YangToDb_storm_control_xfmr(XfmrParams{uri: uri, oper: DELETE})
		if err != nil {
			t.Errorf("DELETE %s: unexpected error %v", uri, err)
		}
		if len(got) != 0 {
			t.Errorf("DELETE %s: expected empty map, got %v", uri, got)
		}
	}
}

func TestYangToDb_storm_control_xfmr_nonPhysicalSetRejected(t *testing.T) {
	uri := "/openconfig-interfaces:interfaces/interface[name=PortChannel100]/openconfig-if-ethernet:ethernet/openconfig-storm-control-ext:storm-control"
	_, err := YangToDb_storm_control_xfmr(XfmrParams{uri: uri, oper: UPDATE})
	if err == nil {
		t.Fatal("expected storm-control SET on PortChannel to fail")
	}
	if _, ok := err.(tlerr.NotSupportedError); !ok {
		t.Fatalf("expected NotSupportedError, got %T: %v", err, err)
	}
}

func TestYangToDb_storm_control_xfmr_ethernetDeleteStillMapped(t *testing.T) {
	uri := "/openconfig-interfaces:interfaces/interface[name=Ethernet0]/openconfig-if-ethernet:ethernet/openconfig-storm-control-ext:storm-control"
	got, err := YangToDb_storm_control_xfmr(XfmrParams{uri: uri, oper: DELETE})
	if err != nil {
		t.Fatalf("DELETE Ethernet0: unexpected error %v", err)
	}
	entries, ok := got[stormCtrlTbl]
	if !ok || len(entries) != len(stormCtrlTypeToDb) {
		t.Fatalf("DELETE Ethernet0: expected %d PORT_STORM_CONTROL keys, got %v", len(stormCtrlTypeToDb), got)
	}
}

func TestDbToYang_storm_control_xfmr_nonPhysicalGetIsNoop(t *testing.T) {
	uri := "/openconfig-interfaces:interfaces/interface[name=PortChannel100]"
	if err := DbToYang_storm_control_xfmr(XfmrParams{uri: uri}); err != nil {
		t.Fatalf("GET PortChannel100: unexpected error %v", err)
	}
}
