package transformer

import "testing"

func TestPlanStpVlanMemberDeleteLastMstMember(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode: STP_MODE_MST,
		current: map[string][]string{
			"Ethernet4": {"Vlan100"},
			"Ethernet0": {"Vlan100"},
		},
		deleted:    []string{"Vlan100|Ethernet4"},
		stpPorts:   map[string]bool{"Ethernet0": true, "Ethernet4": true},
		mstKeys:    []string{"0|Ethernet0", "0|Ethernet4"},
		vlanToInst: map[string]string{},
	})

	if _, ok := plan.deletePort["Ethernet4"]; !ok {
		t.Fatal("expected STP_PORT|Ethernet4 to be deleted")
	}
	if _, ok := plan.deleteMst["0|Ethernet4"]; !ok {
		t.Fatal("expected STP_MST_PORT|0|Ethernet4 to be deleted")
	}
	if _, ok := plan.deletePort["Ethernet0"]; ok {
		t.Fatal("Ethernet0 STP_PORT must stay")
	}
	if _, ok := plan.deleteMst["0|Ethernet0"]; ok {
		t.Fatal("Ethernet0 STP_MST_PORT must stay")
	}
	if len(plan.createPort) != 0 || len(plan.createMst) != 0 {
		t.Fatalf("delete must not create rows: %+v %+v", plan.createPort, plan.createMst)
	}
}

func TestPlanStpVlanMemberDeleteKeepsRowsWhileAnotherVlanRemains(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode:       STP_MODE_MST,
		current:    map[string][]string{"Ethernet4": {"Vlan100", "Vlan200"}},
		deleted:    []string{"Vlan100|Ethernet4"},
		stpPorts:   map[string]bool{"Ethernet4": true},
		mstKeys:    []string{"0|Ethernet4"},
		vlanToInst: map[string]string{},
	})
	if len(plan.deletePort) != 0 || len(plan.deleteMst) != 0 {
		t.Fatalf("CIST and STP_PORT must stay, got deletePort=%v deleteMst=%v", plan.deletePort, plan.deleteMst)
	}
}

func TestPlanStpVlanMemberDeleteDropsMappedInstance(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode:       STP_MODE_MST,
		current:    map[string][]string{"Ethernet4": {"Vlan100", "Vlan200"}},
		deleted:    []string{"Vlan200|Ethernet4"},
		stpPorts:   map[string]bool{"Ethernet4": true},
		mstKeys:    []string{"0|Ethernet4", "1|Ethernet4"},
		vlanToInst: map[string]string{"200": "1"},
	})
	if _, ok := plan.deleteMst["1|Ethernet4"]; !ok {
		t.Fatal("expected STP_MST_PORT|1|Ethernet4 to be deleted")
	}
	if _, ok := plan.deleteMst["0|Ethernet4"]; ok {
		t.Fatal("instance 0 must stay while the port is still a VLAN member")
	}
	if len(plan.deletePort) != 0 {
		t.Fatal("STP_PORT must stay")
	}
}

func TestPlanStpVlanMemberAddCreatesMissingMstRows(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode:       STP_MODE_MST,
		current:    map[string][]string{},
		added:      []string{"Vlan100|Ethernet4"},
		stpPorts:   map[string]bool{},
		mstKeys:    nil,
		vlanToInst: map[string]string{},
	})
	fields, ok := plan.createPort["Ethernet4"]
	if !ok {
		t.Fatal("expected STP_PORT|Ethernet4 to be created")
	}
	if fields["enabled"] != "true" || fields["link_type"] != "auto" || fields["priority"] != "128" {
		t.Fatalf("STP_PORT fields = %v", fields)
	}
	if _, ok := fields["path_cost"]; ok {
		t.Fatal("path_cost must be left unset so stpd derives it")
	}
	mst, ok := plan.createMst["0|Ethernet4"]
	if !ok || mst["priority"] != "128" {
		t.Fatalf("STP_MST_PORT create = %v", plan.createMst)
	}
}

func TestPlanStpVlanMemberAddDoesNotOverwrite(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode:       STP_MODE_MST,
		current:    map[string][]string{},
		added:      []string{"Vlan100|Ethernet4"},
		stpPorts:   map[string]bool{"Ethernet4": true},
		mstKeys:    []string{"0|Ethernet4"},
		vlanToInst: map[string]string{},
	})
	if len(plan.createPort) != 0 || len(plan.createMst) != 0 {
		t.Fatalf("existing rows must not be rewritten: %+v %+v", plan.createPort, plan.createMst)
	}
}

func TestPlanStpVlanMemberAddMappedInstance(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode:       STP_MODE_MST,
		current:    map[string][]string{"Ethernet4": {"Vlan100"}},
		added:      []string{"Vlan200|Ethernet4"},
		stpPorts:   map[string]bool{"Ethernet4": true},
		mstKeys:    []string{"0|Ethernet4"},
		vlanToInst: map[string]string{"200": "1"},
	})
	if _, ok := plan.createMst["1|Ethernet4"]; !ok {
		t.Fatalf("expected instance 1 row, got %v", plan.createMst)
	}
	if _, ok := plan.createMst["0|Ethernet4"]; ok {
		t.Fatal("existing CIST row must not be rewritten")
	}
	if len(plan.createPort) != 0 {
		t.Fatal("existing STP_PORT must not be rewritten")
	}
}

func TestPlanStpVlanMemberIgnoresNonRstpMode(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode:     "pvst",
		current:  map[string][]string{"Ethernet4": {"Vlan100"}},
		deleted:  []string{"Vlan100|Ethernet4"},
		added:    []string{"Vlan200|Ethernet4"},
		stpPorts: map[string]bool{"Ethernet4": true},
		mstKeys:  []string{"0|Ethernet4"},
	})
	if len(plan.deletePort) != 0 || len(plan.deleteMst) != 0 || len(plan.createPort) != 0 || len(plan.createMst) != 0 {
		t.Fatalf("non-RSTP mode must not change rows: %+v", plan)
	}
}

func TestPlanStpVlanMemberIgnoresDisabledStp(t *testing.T) {
	plan := planStpVlanMemberChange(stpVlanMemberState{
		mode:     "",
		current:  map[string][]string{"Ethernet4": {"Vlan100"}},
		deleted:  []string{"Vlan100|Ethernet4"},
		stpPorts: map[string]bool{"Ethernet4": true},
		mstKeys:  []string{"0|Ethernet4"},
	})
	if len(plan.deletePort) != 0 || len(plan.deleteMst) != 0 || len(plan.createPort) != 0 {
		t.Fatalf("disabled STP must not change rows: %+v", plan)
	}
}

func TestExpandVlanListTokens(t *testing.T) {
	got := expandVlanListTokens("100,200-202,300..301")
	want := []string{"100", "200", "201", "202", "300", "301"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
