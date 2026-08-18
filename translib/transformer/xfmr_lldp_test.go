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
