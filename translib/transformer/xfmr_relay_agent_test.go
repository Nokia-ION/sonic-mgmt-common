////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                     //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//  http://www.apache.org/licenses/LICENSE-2.0                                //
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
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/ygot/ygot"
)

func TestValidateDhcpv6RelayIfId(t *testing.T) {
	if err := validateDhcpv6RelayIfId("Vlan100"); err != nil {
		t.Fatalf("expected Vlan100 to be valid: %v", err)
	}
	if err := validateDhcpv6RelayIfId("Ethernet0"); err == nil {
		t.Fatalf("expected Ethernet0 to be rejected")
	}
	if err := validateDhcpv6RelayIfId(""); err == nil {
		t.Fatalf("expected empty id to be rejected")
	}
}

func TestValidateDhcpv6HelperAddress(t *testing.T) {
	if err := validateDhcpv6HelperAddress("2001:db8::1"); err != nil {
		t.Fatalf("expected IPv6 helper to be valid: %v", err)
	}
	if err := validateDhcpv6HelperAddress("10.0.0.1"); err == nil {
		t.Fatalf("expected IPv4 helper to be rejected")
	}
	if err := validateDhcpv6HelperAddress("not-an-ip"); err == nil {
		t.Fatalf("expected invalid helper to be rejected")
	}
}

func TestHelperAddressInstFromUri(t *testing.T) {
	uri := "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config/helper-address[.='2001:db8::1']"
	got := helperAddressInstFromUri(uri)
	if got != "2001:db8::1" {
		t.Fatalf("got %q, want 2001:db8::1", got)
	}

	uri = "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface[id=Vlan100]/config/helper-address[.=\"10.0.0.53\"]"
	got = helperAddressInstFromUri(uri)
	if got != "10.0.0.53" {
		t.Fatalf("got %q, want 10.0.0.53", got)
	}
}

func TestDhcpRelayLeafListDeleteMap(t *testing.T) {
	t.Run("vlan instance", func(t *testing.T) {
		got := dhcpRelayLeafListDeleteMap(VLAN_TN, vlanDhcpServersField, "Vlan100", []string{"10.0.0.53"}, false)
		val := got[VLAN_TN]["Vlan100"]
		if val.GetList(vlanDhcpServersField)[0] != "10.0.0.53" {
			t.Fatalf("expected instance delete of 10.0.0.53, got %v", val.Field)
		}
		if _, ok := val.Field["vlanid"]; ok {
			t.Fatalf("must not delete other VLAN fields: %v", val.Field)
		}
	})
	t.Run("vlan clear all", func(t *testing.T) {
		got := dhcpRelayLeafListDeleteMap(VLAN_TN, vlanDhcpServersField, "Vlan100", nil, false)
		if got[VLAN_TN]["Vlan100"].Field[vlanDhcpServersField+"@"] != "" {
			t.Fatalf("expected empty dhcp_servers@ to clear leaf-list, got %v", got[VLAN_TN]["Vlan100"].Field)
		}
	})
	t.Run("dhcpv4 relay whole key", func(t *testing.T) {
		got := dhcpRelayLeafListDeleteMap(dhcpv4RelayTable, dhcpv4ServersField, "Vlan100", nil, true)
		if len(got[dhcpv4RelayTable]["Vlan100"].Field) != 0 {
			t.Fatalf("expected whole-key delete, got %v", got[dhcpv4RelayTable]["Vlan100"].Field)
		}
	})
}

func TestDhcpRelayValueWithServers(t *testing.T) {
	existing := db.Value{Field: map[string]string{"rfc6939_support": "true"}}
	val := dhcpRelayValueWithServers(existing, []string{"2001:db8::1", "2001:db8::2"})
	got := val.GetList(dhcpv6ServersField)
	if len(got) != 2 || got[0] != "2001:db8::1" || got[1] != "2001:db8::2" {
		t.Fatalf("unexpected servers %v", got)
	}
	if val.Field["rfc6939_support"] != "true" {
		t.Fatalf("expected rfc6939_support to be preserved")
	}
}

func TestYangToDb_Dhcpv6RelayHelperAddress(t *testing.T) {
	device := &ocbinds.Device{}
	var yg ygot.GoStruct = device
	root := getDhcpv6RelayRoot(&yg)
	intf, err := root.Dhcpv6.Interfaces.NewInterface("Vlan100")
	if err != nil {
		t.Fatalf("NewInterface: %v", err)
	}
	enabled := true
	id := "Vlan100"
	intf.Config = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface_Config{
		Id:            &id,
		Enable:        &enabled,
		HelperAddress: []string{"2001:db8::53"},
	}

	inParams := XfmrParams{
		ygRoot:     &yg,
		uri:        "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config",
		requestUri: "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config",
		oper:       UPDATE,
	}

	got, err := YangToDb_dhcpv6_relay_agent_xfmr(inParams)
	if err != nil {
		t.Fatalf("YangToDb: %v", err)
	}
	entry, ok := got[dhcpRelayTable]["Vlan100"]
	if !ok {
		t.Fatalf("missing DHCP_RELAY|Vlan100 in %v", got)
	}
	servers := entry.GetList(dhcpv6ServersField)
	if len(servers) != 1 || servers[0] != "2001:db8::53" {
		t.Fatalf("unexpected dhcpv6_servers %v", servers)
	}
}

func TestYangToDb_Dhcpv6RelayRejectIpv4Helper(t *testing.T) {
	device := &ocbinds.Device{}
	var yg ygot.GoStruct = device
	root := getDhcpv6RelayRoot(&yg)
	intf, err := root.Dhcpv6.Interfaces.NewInterface("Vlan100")
	if err != nil {
		t.Fatalf("NewInterface: %v", err)
	}
	id := "Vlan100"
	intf.Config = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface_Config{
		Id:            &id,
		HelperAddress: []string{"10.0.0.1"},
	}

	inParams := XfmrParams{
		ygRoot:     &yg,
		uri:        "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config/helper-address",
		requestUri: "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config/helper-address",
		oper:       UPDATE,
	}

	_, err = YangToDb_dhcpv6_relay_agent_xfmr(inParams)
	if err == nil {
		t.Fatalf("expected IPv4 helper-address to fail")
	}
}

func TestYangToDb_Dhcpv6RelayEnableRequiresHelper(t *testing.T) {
	device := &ocbinds.Device{}
	var yg ygot.GoStruct = device
	root := getDhcpv6RelayRoot(&yg)
	intf, err := root.Dhcpv6.Interfaces.NewInterface("Vlan100")
	if err != nil {
		t.Fatalf("NewInterface: %v", err)
	}
	enabled := true
	id := "Vlan100"
	intf.Config = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcpv6_Interfaces_Interface_Config{
		Id:     &id,
		Enable: &enabled,
	}

	inParams := XfmrParams{
		ygRoot:     &yg,
		uri:        "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config/enable",
		requestUri: "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config/enable",
		oper:       UPDATE,
	}

	_, err = YangToDb_dhcpv6_relay_agent_xfmr(inParams)
	if err == nil {
		t.Fatalf("expected enable=true without helper-address to fail")
	}
}

func TestYangToDb_Dhcpv6RelayDeleteInterface(t *testing.T) {
	device := &ocbinds.Device{}
	var yg ygot.GoStruct = device
	inParams := XfmrParams{
		ygRoot:     &yg,
		uri:        "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]",
		requestUri: "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]",
		oper:       DELETE,
	}

	got, err := YangToDb_dhcpv6_relay_agent_xfmr(inParams)
	if err != nil {
		t.Fatalf("YangToDb delete: %v", err)
	}
	if _, ok := got[dhcpRelayTable]["Vlan100"]; !ok {
		t.Fatalf("expected DHCP_RELAY|Vlan100 delete in %v", got)
	}
}

func TestValidateDhcpv4RelayIfId(t *testing.T) {
	if err := validateDhcpv4RelayIfId("Vlan100"); err != nil {
		t.Fatalf("expected Vlan100 to be valid: %v", err)
	}
	if err := validateDhcpv4RelayIfId("Ethernet0"); err == nil {
		t.Fatalf("expected Ethernet0 to be rejected")
	}
}

func TestValidateDhcpv4HelperAddress(t *testing.T) {
	if err := validateDhcpv4HelperAddress("10.0.0.1"); err != nil {
		t.Fatalf("expected IPv4 helper to be valid: %v", err)
	}
	if err := validateDhcpv4HelperAddress("2001:db8::1"); err == nil {
		t.Fatalf("expected IPv6 helper to be rejected")
	}
}

func TestDhcpv4RelayValueWithServers(t *testing.T) {
	existing := db.Value{Field: map[string]string{"vlanid": "100", "admin_status": "up"}}
	val := dhcpv4RelayValueWithServers(existing, []string{"10.0.0.1"}, vlanDhcpServersField)
	got := val.GetList(vlanDhcpServersField)
	if len(got) != 1 || got[0] != "10.0.0.1" {
		t.Fatalf("unexpected servers %v", got)
	}
	if val.Field["vlanid"] != "100" {
		t.Fatalf("expected vlanid to be preserved")
	}
}

func TestYangToDb_Dhcpv4RelayHelperAddress(t *testing.T) {
	device := &ocbinds.Device{}
	var yg ygot.GoStruct = device
	root := getDhcpv4RelayRoot(&yg)
	intf, err := root.Dhcp.Interfaces.NewInterface("Vlan100")
	if err != nil {
		t.Fatalf("NewInterface: %v", err)
	}
	enabled := true
	id := "Vlan100"
	intf.Config = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_Config{
		Id:            &id,
		Enable:        &enabled,
		HelperAddress: []string{"10.0.0.53"},
	}

	inParams := XfmrParams{
		ygRoot:     &yg,
		uri:        "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface[id=Vlan100]/config",
		requestUri: "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface[id=Vlan100]/config",
		oper:       UPDATE,
	}

	got, err := YangToDb_dhcpv4_relay_agent_xfmr(inParams)
	if err != nil {
		t.Fatalf("YangToDb: %v", err)
	}
	entry, ok := got[VLAN_TN]["Vlan100"]
	if !ok {
		t.Fatalf("missing VLAN|Vlan100 in %v", got)
	}
	servers := entry.GetList(vlanDhcpServersField)
	if len(servers) != 1 || servers[0] != "10.0.0.53" {
		t.Fatalf("unexpected dhcp_servers %v", servers)
	}
}

func TestYangToDb_Dhcpv4RelayRejectIpv6Helper(t *testing.T) {
	device := &ocbinds.Device{}
	var yg ygot.GoStruct = device
	root := getDhcpv4RelayRoot(&yg)
	intf, err := root.Dhcp.Interfaces.NewInterface("Vlan100")
	if err != nil {
		t.Fatalf("NewInterface: %v", err)
	}
	id := "Vlan100"
	intf.Config = &ocbinds.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_Config{
		Id:            &id,
		HelperAddress: []string{"2001:db8::1"},
	}

	inParams := XfmrParams{
		ygRoot:     &yg,
		uri:        "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface[id=Vlan100]/config/helper-address",
		requestUri: "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface[id=Vlan100]/config/helper-address",
		oper:       UPDATE,
	}

	_, err = YangToDb_dhcpv4_relay_agent_xfmr(inParams)
	if err == nil {
		t.Fatalf("expected IPv6 helper-address to fail")
	}
}

func TestSubscribe_dhcpv6RelayAgent(t *testing.T) {
	t.Run("translate exists", func(t *testing.T) {
		out, err := Subscribe_dhcpv6_relay_agent_xfmr(XfmrSubscInParams{
			uri:       "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces",
			subscProc: TRANSLATE_EXISTS,
		})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if !out.isVirtualTbl {
			t.Fatalf("expected virtual table for TRANSLATE_EXISTS")
		}
		if len(out.dbDataMap) != 0 {
			t.Fatalf("expected no DB map for TRANSLATE_EXISTS, got %v", out.dbDataMap)
		}
	})

	t.Run("subscribe all interfaces", func(t *testing.T) {
		out, err := Subscribe_dhcpv6_relay_agent_xfmr(XfmrSubscInParams{
			uri:       "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces",
			subscProc: TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][dhcpRelayTable]["*"]; !ok {
			t.Fatalf("missing CONFIG_DB DHCP_RELAY wildcard subscription: %v", out.dbDataMap)
		}
		if out.onChange != OnchangeEnable {
			t.Fatalf("expected on-change subscription")
		}
	})

	t.Run("subscribe keyed interface", func(t *testing.T) {
		out, err := Subscribe_dhcpv6_relay_agent_xfmr(XfmrSubscInParams{
			uri:       "/openconfig-relay-agent:relay-agent/dhcpv6/interfaces/interface[id=Vlan100]/config",
			subscProc: TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][VLAN_TN]["Vlan100"]; !ok {
			t.Fatalf("missing CONFIG_DB VLAN|Vlan100 parent subscription: %v", out.dbDataMap)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][dhcpRelayTable]; ok {
			t.Fatalf("did not expect DHCP_RELAY parent subscription: %v", out.dbDataMap)
		}
	})
}

func TestSubscribe_dhcpv4RelayAgent(t *testing.T) {
	t.Run("translate exists", func(t *testing.T) {
		out, err := Subscribe_dhcpv4_relay_agent_xfmr(XfmrSubscInParams{
			uri:       "/openconfig-relay-agent:relay-agent/dhcp/interfaces",
			subscProc: TRANSLATE_EXISTS,
		})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if !out.isVirtualTbl {
			t.Fatalf("expected virtual table for TRANSLATE_EXISTS")
		}
	})

	t.Run("subscribe all interfaces", func(t *testing.T) {
		out, err := Subscribe_dhcpv4_relay_agent_xfmr(XfmrSubscInParams{
			uri:       "/openconfig-relay-agent:relay-agent/dhcp/interfaces",
			subscProc: TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][VLAN_TN]["*"]; !ok {
			t.Fatalf("missing CONFIG_DB VLAN wildcard subscription: %v", out.dbDataMap)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][dhcpv4RelayTable]; ok {
			t.Fatalf("did not expect DHCPV4_RELAY parent subscription: %v", out.dbDataMap)
		}
	})

	t.Run("subscribe keyed interface", func(t *testing.T) {
		out, err := Subscribe_dhcpv4_relay_agent_xfmr(XfmrSubscInParams{
			uri:       "/openconfig-relay-agent:relay-agent/dhcp/interfaces/interface[id=Vlan100]/config",
			subscProc: TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][VLAN_TN]["Vlan100"]; !ok {
			t.Fatalf("missing CONFIG_DB VLAN|Vlan100 subscription: %v", out.dbDataMap)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][dhcpv4RelayTable]; ok {
			t.Fatalf("did not expect DHCPV4_RELAY parent subscription: %v", out.dbDataMap)
		}
	})
}

func TestDbToYangPath_dhcpRelayAgent(t *testing.T) {
	t.Run("dhcpv6", func(t *testing.T) {
		params := XfmrDbToYgPathParams{
			tblName:    dhcpRelayTable,
			tblKeyComp: []string{"Vlan100"},
			ygPathKeys: map[string]string{},
		}
		if err := DbToYangPath_dhcpv6_relay_agent_intf_path_xfmr(params); err != nil {
			t.Fatalf("path xfmr failed: %v", err)
		}
		want := ocRelayDhcpv6IntfPath + "/id"
		if params.ygPathKeys[want] != "Vlan100" {
			t.Fatalf("got %q, want Vlan100", params.ygPathKeys[want])
		}
	})

	t.Run("dhcpv6 vlan parent", func(t *testing.T) {
		params := XfmrDbToYgPathParams{
			tblName:    VLAN_TN,
			tblKeyComp: []string{"Vlan100"},
			ygPathKeys: map[string]string{},
		}
		if err := DbToYangPath_dhcpv6_relay_agent_intf_path_xfmr(params); err != nil {
			t.Fatalf("path xfmr failed: %v", err)
		}
		want := ocRelayDhcpv6IntfPath + "/id"
		if params.ygPathKeys[want] != "Vlan100" {
			t.Fatalf("got %q, want Vlan100", params.ygPathKeys[want])
		}
	})

	t.Run("dhcpv4 vlan", func(t *testing.T) {
		params := XfmrDbToYgPathParams{
			tblName:    VLAN_TN,
			tblKeyComp: []string{"Vlan100"},
			ygPathKeys: map[string]string{},
		}
		if err := DbToYangPath_dhcpv4_relay_agent_intf_path_xfmr(params); err != nil {
			t.Fatalf("path xfmr failed: %v", err)
		}
		want := ocRelayDhcpIntfPath + "/id"
		if params.ygPathKeys[want] != "Vlan100" {
			t.Fatalf("got %q, want Vlan100", params.ygPathKeys[want])
		}
	})

	t.Run("dhcpv4 sonic table", func(t *testing.T) {
		params := XfmrDbToYgPathParams{
			tblName:    dhcpv4RelayTable,
			tblKeyComp: []string{"Vlan200"},
			ygPathKeys: map[string]string{},
		}
		if err := DbToYangPath_dhcpv4_relay_agent_intf_path_xfmr(params); err != nil {
			t.Fatalf("path xfmr failed: %v", err)
		}
		want := ocRelayDhcpIntfPath + "/id"
		if params.ygPathKeys[want] != "Vlan200" {
			t.Fatalf("got %q, want Vlan200", params.ygPathKeys[want])
		}
	})
}
