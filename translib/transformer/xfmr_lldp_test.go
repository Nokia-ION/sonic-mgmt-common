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
