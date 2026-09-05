package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
)

func Test_fillGlobalStateLeaves_locChassisFallback_withPrecreatedEmptyLeaf(t *testing.T) {
	empty := ""
	state := &ocbinds.OpenconfigLldp_Lldp_State{
		SystemName:        &empty,
		SystemDescription: &empty,
	}
	locChassis := db.Value{Field: map[string]string{
		LLDP_LOC_SYS_NAME: "sonic",
		LLDP_LOC_SYS_DESC: "loc chassis desc",
	}}

	fillGlobalStateLeaves(state, db.Value{}, locChassis, false)

	if state.SystemName == nil || *state.SystemName != "sonic" {
		t.Fatalf("expected system-name sonic, got %#v", state.SystemName)
	}
	if state.SystemDescription == nil || *state.SystemDescription != "loc chassis desc" {
		t.Fatalf("expected system-description from loc chassis, got %#v", state.SystemDescription)
	}
}

func Test_fillGlobalConfigLeaves_locChassisFallback_withPrecreatedEmptyLeaf(t *testing.T) {
	empty := ""
	cfg := &ocbinds.OpenconfigLldp_Lldp_Config{
		SystemName:        &empty,
		SystemDescription: &empty,
	}
	locChassis := db.Value{Field: map[string]string{
		LLDP_LOC_SYS_NAME: "sonic",
		LLDP_LOC_SYS_DESC: "loc chassis desc",
	}}

	fillGlobalConfigLeaves(cfg, db.Value{}, locChassis)

	if cfg.SystemName == nil || *cfg.SystemName != "sonic" {
		t.Fatalf("expected system-name sonic, got %#v", cfg.SystemName)
	}
	if cfg.SystemDescription == nil || *cfg.SystemDescription != "loc chassis desc" {
		t.Fatalf("expected system-description from loc chassis, got %#v", cfg.SystemDescription)
	}
}

func Test_fillGlobalStateLeaves_defaults_without_global(t *testing.T) {
	state := &ocbinds.OpenconfigLldp_Lldp_State{}
	fillGlobalStateLeaves(state, db.Value{}, db.Value{}, false)

	if state.Enabled == nil || *state.Enabled != true {
		t.Fatalf("expected enabled default true, got %#v", state.Enabled)
	}
	if state.HelloTimer == nil || *state.HelloTimer != lldpDefaultHelloTimer {
		t.Fatalf("expected hello-timer default %d, got %#v", lldpDefaultHelloTimer, state.HelloTimer)
	}
}

func Test_fillGlobalStateLeaves_defaults_with_precreated_zero_values(t *testing.T) {
	falseVal := false
	zero := uint64(0)
	state := &ocbinds.OpenconfigLldp_Lldp_State{
		Enabled:    &falseVal,
		HelloTimer: &zero,
	}
	fillGlobalStateLeaves(state, db.Value{}, db.Value{}, false)

	if state.Enabled == nil || *state.Enabled != true {
		t.Fatalf("expected enabled default true, got %#v", state.Enabled)
	}
	if state.HelloTimer == nil || *state.HelloTimer != lldpDefaultHelloTimer {
		t.Fatalf("expected hello-timer default %d, got %#v", lldpDefaultHelloTimer, state.HelloTimer)
	}
}

func Test_fillGlobalStateLeaves_respects_configured_global_values(t *testing.T) {
	state := &ocbinds.OpenconfigLldp_Lldp_State{}
	globalEntry := db.Value{Field: map[string]string{
		LLDP_CFG_ENABLED:    "false",
		LLDP_CFG_HELLO_TIME: "45",
	}}
	fillGlobalStateLeaves(state, globalEntry, db.Value{}, false)

	if state.Enabled == nil || *state.Enabled != false {
		t.Fatalf("expected enabled false from GLOBAL, got %#v", state.Enabled)
	}
	if state.HelloTimer == nil || *state.HelloTimer != 45 {
		t.Fatalf("expected hello-timer 45 from GLOBAL, got %#v", state.HelloTimer)
	}
}

func TestSubscribe_lldp_global_maps_loc_chassis_singleton(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   SubTreeXfmrSubscribe
		uri  string
	}{
		{"config", Subscribe_lldp_config_xfmr, "/openconfig-lldp:lldp/config"},
		{"config/system-description", Subscribe_lldp_config_xfmr, "/openconfig-lldp:lldp/config/system-description"},
		{"config/chassis-id", Subscribe_lldp_config_xfmr, "/openconfig-lldp:lldp/config/chassis-id"},
		{"state", Subscribe_lldp_state_xfmr, "/openconfig-lldp:lldp/state"},
		{"state/system-description", Subscribe_lldp_state_xfmr, "/openconfig-lldp:lldp/state/system-description"},
		{"state/chassis-id", Subscribe_lldp_state_xfmr, "/openconfig-lldp:lldp/state/chassis-id"},
		{"state/system-name", Subscribe_lldp_state_xfmr, "/openconfig-lldp:lldp/state/system-name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.fn(XfmrSubscInParams{uri: tc.uri, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("subscribe xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected non-virtual subscribe mapping")
			}
			if _, ok := out.dbDataMap[db.ConfigDB][LLDP_CFG_TABLE][LLDP_GLOBAL_KEY]; !ok {
				t.Fatalf("missing CONFIG_DB LLDP|GLOBAL mapping: %v", out.dbDataMap)
			}
			if _, ok := out.dbDataMap[db.ApplDB][LLDP_LOC_CHASSIS_TABLE][""]; !ok {
				t.Fatalf("missing APPL_DB LLDP_LOC_CHASSIS singleton mapping: %v", out.dbDataMap)
			}
		})
	}
}

func TestSubscribe_lldp_global_translate_exists(t *testing.T) {
	for _, fn := range []SubTreeXfmrSubscribe{Subscribe_lldp_config_xfmr, Subscribe_lldp_state_xfmr} {
		out, err := fn(XfmrSubscInParams{uri: "/openconfig-lldp:lldp/state", subscProc: TRANSLATE_EXISTS})
		if err != nil {
			t.Fatalf("subscribe xfmr failed: %v", err)
		}
		if len(out.dbDataMap) != 0 {
			t.Fatalf("TRANSLATE_EXISTS should not emit DB maps, got %v", out.dbDataMap)
		}
	}
}

func TestSubscribe_lldp_interfaces_paths(t *testing.T) {
	tests := []struct {
		name        string
		uri         string
		virtual     bool
		wantApplKey string
		wantCfgKey  string
		exists      bool
	}{
		{name: "neighbors wildcard", uri: "/openconfig-lldp:lldp/interfaces/interface/neighbors", wantApplKey: "*"},
		{name: "neighbors keyed", uri: "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/neighbors", wantApplKey: "Ethernet0"},
		{name: "interface config", uri: "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config", virtual: true},
		{name: "interface state", uri: "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/state", virtual: true},
		{name: "interfaces list", uri: "/openconfig-lldp:lldp/interfaces", wantApplKey: "*", wantCfgKey: "*"},
		{name: "interface list", uri: "/openconfig-lldp:lldp/interfaces/interface", wantApplKey: "*", wantCfgKey: "*"},
		{name: "keyed interface", uri: "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]", virtual: true},
		{name: "keyed config enabled", uri: "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/config/enabled", virtual: true},
		{name: "translate exists", uri: "/openconfig-lldp:lldp/interfaces", exists: true, virtual: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := XfmrSubscInParams{uri: tc.uri, subscProc: TRANSLATE_SUBSCRIBE}
			if tc.exists {
				in.subscProc = TRANSLATE_EXISTS
			}
			out, err := Subscribe_lldp_interfaces_xfmr(in)
			if err != nil {
				t.Fatalf("Subscribe_lldp_interfaces_xfmr failed: %v", err)
			}
			if out.isVirtualTbl != tc.virtual {
				t.Fatalf("isVirtualTbl=%v, want %v (map=%v)", out.isVirtualTbl, tc.virtual, out.dbDataMap)
			}
			if tc.virtual {
				if len(out.dbDataMap) != 0 {
					t.Fatalf("expected no DB map for virtual path, got %v", out.dbDataMap)
				}
				return
			}
			if tc.wantApplKey != "" {
				if _, ok := out.dbDataMap[db.ApplDB][LLDP_ENTRY_TABLE][tc.wantApplKey]; !ok {
					t.Fatalf("missing APPL_DB LLDP_ENTRY_TABLE[%s]: %v", tc.wantApplKey, out.dbDataMap)
				}
			}
			if tc.wantCfgKey != "" {
				if _, ok := out.dbDataMap[db.ConfigDB][LLDP_PORT_TABLE][tc.wantCfgKey]; !ok {
					t.Fatalf("missing CONFIG_DB LLDP_PORT[%s]: %v", tc.wantCfgKey, out.dbDataMap)
				}
			}
		})
	}
}

func Test_rejectUnsupportedLldpGlobalConfig(t *testing.T) {
	err := rejectUnsupportedLldpGlobalConfig("/openconfig-lldp:lldp/config/ttl", nil)
	if err == nil {
		t.Fatal("expected NotSupported for ttl")
	}
	err = rejectUnsupportedLldpGlobalConfig("/openconfig-lldp:lldp/config/management-interface", nil)
	if err != nil {
		t.Fatal("management-interface should be allowed")
	}
	if err := rejectUnsupportedLldpGlobalConfig("/openconfig-lldp:lldp/config/hello-timer", nil); err != nil {
		t.Fatalf("hello-timer should be allowed: %v", err)
	}
	if err := rejectUnsupportedLldpGlobalConfig("/openconfig-lldp:lldp/config", nil); err != nil {
		t.Fatalf("empty config container should be allowed: %v", err)
	}
}

func Test_rejectUnsupportedLldpInterfaceConfig(t *testing.T) {
	err := rejectUnsupportedLldpInterfaceConfig("/openconfig-lldp:lldp/interfaces/interface/config/port-description", nil)
	if err == nil {
		t.Fatal("expected NotSupported for port-description")
	}
	if err := rejectUnsupportedLldpInterfaceConfig("/openconfig-lldp:lldp/interfaces/interface/config/enabled", nil); err != nil {
		t.Fatalf("enabled should be allowed: %v", err)
	}
}

func TestSubscribe_lldp_mgmt_addresses_maps_loc_chassis(t *testing.T) {
	out, err := Subscribe_lldp_mgmt_addresses_xfmr(XfmrSubscInParams{
		uri:       "/openconfig-lldp:lldp/mgmt-addresses",
		subscProc: TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("subscribe xfmr failed: %v", err)
	}
	if out.isVirtualTbl {
		t.Fatalf("expected non-virtual subscribe for mgmt-addresses")
	}
	if _, ok := out.dbDataMap[db.ApplDB][LLDP_LOC_CHASSIS_TABLE][""]; !ok {
		t.Fatalf("missing APPL_DB LLDP_LOC_CHASSIS mapping: %v", out.dbDataMap)
	}
}

func assertLldpCountersSubscribe(t *testing.T, out XfmrSubscOutParams, key string) {
	t.Helper()
	if out.isVirtualTbl {
		t.Fatalf("expected non-virtual counters subscribe")
	}
	if out.onChange != OnchangeEnable {
		t.Fatalf("onChange=%v, want OnchangeEnable so COUNTERS_DB ON_CHANGE is allowed", out.onChange)
	}
	if !out.needCache {
		t.Fatalf("expected needCache for SAMPLE counters")
	}
	if _, ok := out.dbDataMap[db.CountersDB][LLDP_STATISTICS_TABLE][key]; !ok {
		t.Fatalf("missing COUNTERS_DB %s[%s]: %v", LLDP_STATISTICS_TABLE, key, out.dbDataMap)
	}
}

func assertLldpCountersSample(t *testing.T, out XfmrSubscOutParams, key string) {
	t.Helper()
	assertLldpCountersSubscribe(t, out, key)
	if out.nOpts == nil || out.nOpts.pType != Sample || out.nOpts.mInterval != 30 {
		t.Fatalf("nOpts=%#v, want SAMPLE interval 30", out.nOpts)
	}
}

func TestSubscribe_lldp_state_counters_maps_statistics(t *testing.T) {
	out, err := Subscribe_lldp_state_xfmr(XfmrSubscInParams{
		uri:       "/openconfig-lldp:lldp/state/counters",
		subscProc: TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("subscribe xfmr failed: %v", err)
	}
	assertLldpCountersSample(t, out, LLDP_STATISTICS_GLOBAL_KEY)
	if _, ok := out.dbDataMap[db.ConfigDB]; ok {
		t.Fatalf("counters-only path should not watch CONFIG_DB: %v", out.dbDataMap)
	}
}

func TestSubscribe_lldp_state_container_maps_statistics(t *testing.T) {
	out, err := Subscribe_lldp_state_xfmr(XfmrSubscInParams{
		uri:       "/openconfig-lldp:lldp/state",
		subscProc: TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("subscribe xfmr failed: %v", err)
	}
	assertLldpCountersSubscribe(t, out, LLDP_STATISTICS_GLOBAL_KEY)
	if out.nOpts == nil || out.nOpts.pType != OnChange {
		t.Fatalf("nOpts=%#v, want ON_CHANGE for mixed /lldp/state (chassis + counters)", out.nOpts)
	}
	if _, ok := out.dbDataMap[db.ConfigDB][LLDP_CFG_TABLE][LLDP_GLOBAL_KEY]; !ok {
		t.Fatalf("missing CONFIG_DB LLDP|GLOBAL mapping: %v", out.dbDataMap)
	}
	if _, ok := out.dbDataMap[db.ApplDB][LLDP_LOC_CHASSIS_TABLE][""]; !ok {
		t.Fatalf("missing APPL_DB LLDP_LOC_CHASSIS mapping: %v", out.dbDataMap)
	}
}

func TestSubscribe_lldp_interface_counters_maps_statistics(t *testing.T) {
	out, err := Subscribe_lldp_interfaces_xfmr(XfmrSubscInParams{
		uri:       "/openconfig-lldp:lldp/interfaces/interface[name=Ethernet0]/state/counters",
		subscProc: TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("subscribe xfmr failed: %v", err)
	}
	assertLldpCountersSample(t, out, "Ethernet0")
}

func TestSubscribe_lldp_interface_counters_wildcard(t *testing.T) {
	out, err := Subscribe_lldp_interfaces_xfmr(XfmrSubscInParams{
		uri:       "/openconfig-lldp:lldp/interfaces/interface/state/counters",
		subscProc: TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("subscribe xfmr failed: %v", err)
	}
	assertLldpCountersSample(t, out, "*")
}

func Test_splitLldpMgmtAddresses(t *testing.T) {
	got := splitLldpMgmtAddresses("172.17.2.1, fd00::1")
	if len(got) != 2 || got[0] != "172.17.2.1" || got[1] != "fd00::1" {
		t.Fatalf("unexpected split: %v", got)
	}
	if splitLldpMgmtAddresses("") != nil && len(splitLldpMgmtAddresses("")) != 0 {
		t.Fatalf("empty string should yield no addresses")
	}
}

func Test_fillNeighborExtendedState(t *testing.T) {
	ifInfo := &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface{}
	entry := db.Value{Field: map[string]string{
		LLDP_REMOTE_SYS_NAME:       "peer",
		LLDP_REMOTE_REM_ID:         "1",
		LLDP_REMOTE_MAN_ADDR:       "10.0.0.1,fd00::1",
		LLDP_REMOTE_TTL:            "90",
		LLDP_REMOTE_MAX_FRAME_SIZE: "1514",
		LLDP_REMOTE_PORT_VLAN_ID:   "146",
		LLDP_REMOTE_MED_INV_SERIAL: "SN-1234",
		LLDP_REMOTE_AGG_PORT_ID:    "42",
		LLDP_REMOTE_CUSTOM_TLVS:    `[{"type":127,"oui":"00:90:69","oui-subtype":"1","value":"435530323133353130363530"}]`,
	}}
	if err := populateLldpNeighbor(ifInfo, "Ethernet0", entry, "", "/openconfig-lldp:lldp/interfaces/interface/neighbors"); err != nil {
		t.Fatalf("populateLldpNeighbor: %v", err)
	}
	ng := ifInfo.Neighbors.Neighbor["Ethernet0"]
	if ng.State.Ttl == nil || *ng.State.Ttl != 90 {
		t.Fatalf("ttl: %#v", ng.State.Ttl)
	}
	if ng.State.MaxFrameSize == nil || *ng.State.MaxFrameSize != 1514 {
		t.Fatalf("max-frame-size: %#v", ng.State.MaxFrameSize)
	}
	if ng.State.PortVlanId == nil || *ng.State.PortVlanId != 146 {
		t.Fatalf("port-vlan-id: %#v", ng.State.PortVlanId)
	}
	if ng.State.MedInventorySerialNumber == nil || *ng.State.MedInventorySerialNumber != "SN-1234" {
		t.Fatalf("med serial: %#v", ng.State.MedInventorySerialNumber)
	}
	if ng.State.ManagementAddress == nil || *ng.State.ManagementAddress != "10.0.0.1,fd00::1" {
		t.Fatalf("deprecated management-address: %#v", ng.State.ManagementAddress)
	}
	if ng.MgmtAddresses == nil || len(ng.MgmtAddresses.MgmtAddress) != 2 {
		t.Fatalf("mgmt-addresses: %#v", ng.MgmtAddresses)
	}
	if ng.LinkAggregation == nil || ng.LinkAggregation.State == nil || ng.LinkAggregation.State.PortId == nil || *ng.LinkAggregation.State.PortId != 42 {
		t.Fatalf("link-aggregation: %#v", ng.LinkAggregation)
	}
	if ng.CustomTlvs == nil || len(ng.CustomTlvs.Tlv) != 1 {
		t.Fatalf("custom-tlvs: %#v", ng.CustomTlvs)
	}
}

func Test_fillLldpRootMgmtAddresses(t *testing.T) {
	lldpObj := &ocbinds.OpenconfigLldp_Lldp{}
	fillLldpRootMgmtAddresses(lldpObj, "10.1.0.32,fc00:1::32")
	if lldpObj.MgmtAddresses == nil || len(lldpObj.MgmtAddresses.MgmtAddress) != 2 {
		t.Fatalf("expected 2 local mgmt addresses, got %#v", lldpObj.MgmtAddresses)
	}
	fillLldpRootMgmtAddresses(lldpObj, "")
	if lldpObj.MgmtAddresses != nil {
		t.Fatalf("empty joined string should clear mgmt-addresses")
	}
}

func Test_fillLldpGlobalCounters(t *testing.T) {
	state := &ocbinds.OpenconfigLldp_Lldp_State{}
	fillLldpGlobalCounters(state, db.Value{}, true)
	if state.Counters == nil {
		t.Fatalf("expected empty counters container when includeCounters is set")
	}

	entry := db.Value{Field: map[string]string{
		LLDP_STAT_FRAME_IN:         "220",
		LLDP_STAT_FRAME_OUT:        "110",
		LLDP_STAT_FRAME_DISCARD:    "7",
		LLDP_STAT_TLV_UNKNOWN:      "9",
		LLDP_STAT_TLV_ACCEPTED:     "13",
		LLDP_STAT_ENTRIES_AGED_OUT: "11",
	}}
	fillLldpGlobalCounters(state, entry, false)
	if state.Counters.FrameIn == nil || *state.Counters.FrameIn != 220 {
		t.Fatalf("frame-in: %#v", state.Counters.FrameIn)
	}
	if state.Counters.FrameOut == nil || *state.Counters.FrameOut != 110 {
		t.Fatalf("frame-out: %#v", state.Counters.FrameOut)
	}
	if state.Counters.TlvAccepted == nil || *state.Counters.TlvAccepted != 13 {
		t.Fatalf("tlv-accepted: %#v", state.Counters.TlvAccepted)
	}
	if state.Counters.EntriesAgedOut == nil || *state.Counters.EntriesAgedOut != 11 {
		t.Fatalf("entries-aged-out: %#v", state.Counters.EntriesAgedOut)
	}
	if state.Counters.LastClear != nil {
		t.Fatalf("last-clear must stay unset: %#v", state.Counters.LastClear)
	}
	if state.Counters.FrameErrorIn != nil {
		t.Fatalf("frame-error-in must stay unset: %#v", state.Counters.FrameErrorIn)
	}
}

func Test_fillLldpInterfaceCounters(t *testing.T) {
	name := "Ethernet0"
	ifInfo := &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface{
		Name:  &name,
		State: &ocbinds.OpenconfigLldp_Lldp_Interfaces_Interface_State{},
	}
	entry := db.Value{Field: map[string]string{
		LLDP_STAT_FRAME_IN:      "200",
		LLDP_STAT_FRAME_OUT:     "100",
		LLDP_STAT_FRAME_DISCARD: "6",
		LLDP_STAT_TLV_UNKNOWN:   "7",
		LLDP_STAT_TLV_ACCEPTED:  "9",
	}}
	fillLldpInterfaceCounters(ifInfo, entry, false)
	c := ifInfo.State.Counters
	if c == nil || c.FrameIn == nil || *c.FrameIn != 200 || c.FrameOut == nil || *c.FrameOut != 100 {
		t.Fatalf("interface counters: %#v", c)
	}
	if c.TlvUnknown == nil || *c.TlvUnknown != 7 {
		t.Fatalf("tlv-unknown: %#v", c.TlvUnknown)
	}
	if c.FrameErrorOut != nil || c.LastClear != nil {
		t.Fatalf("unsupported interface counter leaves must stay unset: %#v", c)
	}
}

func Test_parseLldpCapBitmap_ieee_extra_bits(t *testing.T) {
	states := map[ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY]bool{}
	parseLldpCapBitmap("28 00", states, true)
	if !states[ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_MAC_BRIDGE] {
		t.Fatal("expected MAC_BRIDGE from 0x2800")
	}
	if !states[ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_ROUTER] {
		t.Fatal("expected ROUTER from 0x2800")
	}

	extra := map[ocbinds.E_OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY]bool{}
	parseLldpCapBitmap("00 a0", extra, true)
	if !extra[ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_C_VLAN] {
		t.Fatal("expected C_VLAN from bit 8 (0x0080)")
	}
	if !extra[ocbinds.OpenconfigLldpTypes_LLDP_SYSTEM_CAPABILITY_TWO_PORT_MAC_RELAY] {
		t.Fatal("expected TWO_PORT_MAC_RELAY from bit 10 (0x0020)")
	}
}

func Test_suppressTlvsToConfig_rejects_unsupported_identity(t *testing.T) {
	_, err := suppressTlvsToConfig([]ocbinds.E_OpenconfigLldpTypes_LLDP_TLV{
		ocbinds.OpenconfigLldpTypes_LLDP_TLV_CHASSIS_ID,
	})
	if err == nil {
		t.Fatal("expected NotSupported for CHASSIS_ID suppress")
	}
	fields, err := suppressTlvsToConfig([]ocbinds.E_OpenconfigLldpTypes_LLDP_TLV{
		ocbinds.OpenconfigLldpTypes_LLDP_TLV_MANAGEMENT_ADDRESS,
	})
	if err != nil {
		t.Fatalf("management-address should map: %v", err)
	}
	if fields[LLDP_CFG_SUPP_MGMT_ADDR_TLV] != "true" {
		t.Fatalf("unexpected fields: %#v", fields)
	}
}

func Test_normalizeLldpOui(t *testing.T) {
	got, err := normalizeLldpOui("00:90:69")
	if err != nil || got != "00:90:69" {
		t.Fatalf("colon oui: %q %v", got, err)
	}
	got, err = normalizeLldpOui("00,90,69")
	if err != nil || got != "00:90:69" {
		t.Fatalf("comma oui: %q %v", got, err)
	}
	if _, err := normalizeLldpOui("0090"); err == nil {
		t.Fatal("expected error for short OUI")
	}
}

func Test_validateLldpManagementInterface(t *testing.T) {
	if err := validateLldpManagementInterface(nil, "eth0"); err != nil {
		t.Fatalf("eth0 syntax: %v", err)
	}
	if err := validateLldpManagementInterface(nil, "Ethernet0"); err != nil {
		t.Fatalf("Ethernet0 syntax: %v", err)
	}
	if err := validateLldpManagementInterface(nil, ""); err != nil {
		t.Fatalf("empty should be allowed: %v", err)
	}
	for _, bad := range []string{"?", "*", "eth0,*", " eth0", "eth 0"} {
		if err := validateLldpManagementInterface(nil, bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func Test_fillGlobalConfigLeaves_management_interface(t *testing.T) {
	cfg := &ocbinds.OpenconfigLldp_Lldp_Config{}
	fillGlobalConfigLeaves(cfg, db.Value{Field: map[string]string{
		LLDP_CFG_MGMT_INTERFACE: "eth0",
	}}, db.Value{})
	if cfg.ManagementInterface == nil || *cfg.ManagementInterface != "eth0" {
		t.Fatalf("management-interface: %#v", cfg.ManagementInterface)
	}
}
