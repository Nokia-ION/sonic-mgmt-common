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
)

func TestRstpIntfNameFromDbKey(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{key: "0:Ethernet0", want: "Ethernet0"},
		{key: "0|Ethernet0", want: "Ethernet0"},
		{key: "Ethernet0", want: "Ethernet0"},
		{key: "0:PortChannel1", want: "PortChannel1"},
	}
	for _, tc := range cases {
		if got := rstpIntfNameFromDbKey(tc.key); got != tc.want {
			t.Errorf("rstpIntfNameFromDbKey(%q)=%q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestRstpIntfDbKey(t *testing.T) {
	ifName := "Ethernet0"
	cfgPath := STP_RSTP_INTF_CONFIG + "/port-priority"
	if got := rstpIntfDbKey(ifName, cfgPath); got != "0|Ethernet0" {
		t.Errorf("config key=%q, want 0|Ethernet0", got)
	}
	if got := rstpIntfDbKey(ifName, STP_RSTP_INTERFACE+"/state"); got != "0:Ethernet0" {
		t.Errorf("state key=%q, want 0:Ethernet0", got)
	}
	if got := rstpIntfDbKey("*", cfgPath); got != "0|*" {
		t.Errorf("config wildcard key=%q, want 0|*", got)
	}
	if got := rstpIntfDbKey("*", STP_RSTP_INTERFACE+"/state"); got != "0:*" {
		t.Errorf("state wildcard key=%q, want 0:*", got)
	}
}

func TestMstPortCliCfgKey(t *testing.T) {
	if got := mstPortCliCfgKey("Ethernet0"); got != "MST_INSTANCE|0|Ethernet0" {
		t.Errorf("mstPortCliCfgKey=%q, want MST_INSTANCE|0|Ethernet0", got)
	}
}

func TestShouldMapStpRstpIntfConfigLeaf(t *testing.T) {
	if !shouldMapStpRstpIntfConfigLeaf(STP_RSTP_INTF_CONFIG, "port-priority") {
		t.Fatal("container path should map port-priority")
	}
	if !shouldMapStpRstpIntfConfigLeaf(STP_RSTP_INTF_CONFIG, "cost") {
		t.Fatal("container path should map cost")
	}
	if !shouldMapStpRstpIntfConfigLeaf(STP_RSTP_INTF_CONFIG+"/port-priority", "port-priority") {
		t.Fatal("leaf path should map port-priority")
	}
	if shouldMapStpRstpIntfConfigLeaf(STP_RSTP_INTF_CONFIG+"/port-priority", "cost") {
		t.Fatal("port-priority leaf path should not map cost")
	}
}

func TestDecodeStpDesignatedPortId(t *testing.T) {
	cases := []struct {
		val      string
		ok       bool
		priority uint8
		portNum  uint16
	}{
		{val: "", ok: false},
		{val: "65535", ok: false},
		{val: "0", ok: true, priority: 0, portNum: 0},
		{val: "32768", ok: true, priority: 128, portNum: 0},
		{val: "32772", ok: true, priority: 128, portNum: 4},
		{val: "4", ok: true, priority: 0, portNum: 4},
	}
	for _, tc := range cases {
		prio, portNum, ok := decodeStpDesignatedPortId(tc.val)
		if ok != tc.ok || prio != tc.priority || portNum != tc.portNum {
			t.Errorf("decodeStpDesignatedPortId(%q)=(%d,%d,%v), want (%d,%d,%v)",
				tc.val, prio, portNum, ok, tc.priority, tc.portNum, tc.ok)
		}
	}
}

func TestRstpIntfConfigFieldsZeroPriority(t *testing.T) {
	prio := uint8(0)
	cost := uint32(2000)
	inParams := XfmrParams{param: &prio}
	fields := rstpIntfConfigFieldsFromRequest(inParams, "Ethernet0", STP_RSTP_INTF_CONFIG+"/port-priority")
	if fields["priority"] != "0" {
		t.Fatalf("priority=%q, want 0", fields["priority"])
	}
	inParams.param = &cost
	fields = rstpIntfConfigFieldsFromRequest(inParams, "Ethernet0", STP_RSTP_INTF_CONFIG+"/cost")
	if fields["path_cost"] != "2000" {
		t.Fatalf("path_cost=%q, want 2000", fields["path_cost"])
	}
}
