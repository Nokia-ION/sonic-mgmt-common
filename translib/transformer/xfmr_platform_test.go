package transformer

import (
	"math"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/ygot/ygot"
)

func TestPathTypeString(t *testing.T) {
	cases := []struct {
		in   PathType
		want string
	}{
		{AllPaths, "AllPaths"},
		{AllCompPaths, "AllComponentPaths"},
		{ConfigPaths, "ConfigPaths"},
		{StatePaths, "StatePaths"},
		{SingularPath, "SingularPath"},
		{RailPaths, "RailPaths"},
	}
	for _, c := range cases {
		got := c.in.String()
		if got != c.want {
			t.Errorf("PathType.String() == %q, want %q", got, c.want)
		}
	}
}

func TestComponentTypeString(t *testing.T) {
	cases := []struct {
		in   componentType
		want string
	}{
		{CompTypeInvalid, "CompTypeInvalid"},
		{CompTypePsu, "CompTypePsu"},
		{CompTypeFan, "CompTypeFan"},
		{CompTypeFanTray, "CompTypeFanTray"},
		{CompTypeStorage, "CompTypeStorage"},
		{CompTypeXcvr, "CompTypeXcvr"},
		{CompTypeTemp, "CompTypeTemp"},
		{CompTypeIC, "CompTypeIC"},
		{CompTypeChassis, "CompTypeChassis"},
		{CompTypeNWStack, "CompTypeNWStack"},
		{CompTypeOS, "CompTypeOS"},
		{CompTypeBootLoader, "CompTypeBootLoader"},
		{CompTypePort, "CompTypePort"},
		{CompTypePcie, "CompTypePcie"},
		{CompTypePowerSupplyVR, "CompTypePowerSupplyVR"},
		{CompTypePowerSupplyPB, "CompTypePowerSupplyPB"},
		{CompTypePowerSupplyPH, "CompTypePowerSupplyPH"},
		{CompTypePowerSupplyPSEQ, "CompTypePowerSupplyPSEQ"},
		{CompTypeControllerCard, "CompTypeControllerCard"},
		{CompTypeLineCard, "CompTypeLineCard"},
		{CompTypeDiskCarrier, "CompTypeDiskCarrier"},
	}
	for _, c := range cases {
		got := c.in.String()
		if got != c.want {
			t.Errorf("componentType.String() == %q, want %q", got, c.want)
		}
	}
}

func TestEthPmdTypeFromString(t *testing.T) {
	cases := []struct {
		in   string
		want ocbinds.E_OpenconfigTransportTypes_ETHERNET_PMD_TYPE
	}{
		{"", ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_UNDEFINED},
		{"JunkValue", ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_UNDEFINED},
		{"PMD_UNKNOWN", ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_UNDEFINED},
		{"100G_PSM4", ocbinds.OpenconfigTransportTypes_ETHERNET_PMD_TYPE_ETH_100GBASE_PSM4},
	}
	for _, c := range cases {
		got := ethPmdTypeFromString(c.in)
		if got != c.want {
			t.Errorf("ethPmdTypeFromString(%s)=%s; want %s", c.in, got, c.want)
		}
	}
}

func TestGetPfmRootObject(t *testing.T) {
	if r := getPfmRootObject(nil); r != nil {
		t.Errorf("Calling getPfmRootObject with nil didn't return nil, %v", r)
	}
}

func testFetchAllPortsFromParentPortWithLanes(t *testing.T) {
	if _, err := fetchAllPortsFromParentPortWithLanes("bad port name", nil, 8); err == nil {
		t.Errorf("Calling fetchAllPortsFromParentPortWithLanes with a bad port name didn't return an error")
	}
}

func TestFillFirmwareInfo(t *testing.T) {
	var err error
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	ygot.BuildEmptyTree(comp.Chassis)
	ygot.BuildEmptyTree(comp.Chassis.State)
	ygot.BuildEmptyTree(comp.Chassis.Config)
	name := "chassis"
	sdb, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer sdb.DeleteDB()
	cdb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer cdb.DeleteDB()
	singlePathURIs := []string{
		COMP_STATE_FIRM_VER, COMP_STATE_HW_VER, COMP_STATE_MFG_DATE,
		COMP_CONFIG_NAME, COMP_STATE_PART_NO,
		COMP_STATE_SERIAL_NO}
	for _, uri := range singlePathURIs {
		t.Run(uri, func(t *testing.T) {
			if err = fillFirmwareInfo(comp, name, SingularPath, uri, sdb, cdb); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVariousDbMisses(t *testing.T) {
	name := "NONEXISTENTKEY"
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	empty := PhysicalEntityInfo{}
	if pie := entityInfo(name, d); pie != empty {
		t.Fatalf("Expected empty response when PhysicalEntityInfo undefined")
	}
}

func TestGetSysComponentsWithUnknownComponentType(t *testing.T) {
	var inParams XfmrParams
	key := "IDONTEXIST"

	dbNum := db.StateDB
	d, err := db.NewDB(getDBOptions(dbNum))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	inParams.dbs[dbNum] = d
	for _, targetUriPath := range []string{COMP, COMP_ST, COMP_CFG} {
		if err := getSysComponents(nil, targetUriPath, inParams, key, ""); err != nil {
			t.Fatal("getSysComponents returned an error with an unknown component type")
		}
	}
}

// TestParseXcvrDomValue covers the per-lane DOM value parser, including the
// "N/A" sentinel that xcvrd writes for unused/unsupported lanes.
func TestParseXcvrDomValue(t *testing.T) {
	cases := []struct {
		in     string
		want   float64
		wantOk bool
	}{
		{"", 0, false},
		{"N/A", 0, false},
		{"n/a", 0, false},
		{"junk", 0, false},
		{"-4.145", -4.145, true},
		{"0", 0, true},
		{"6.834", 6.834, true},
	}
	for _, c := range cases {
		got, ok := parseXcvrDomValue(c.in)
		if ok != c.wantOk {
			t.Errorf("parseXcvrDomValue(%q) ok=%v, want %v", c.in, ok, c.wantOk)
			continue
		}
		if ok && math.Abs(got-c.want) > 1e-9 {
			t.Errorf("parseXcvrDomValue(%q)=%v, want %v", c.in, got, c.want)
		}
	}
}

func TestTrimXcvrEepromString(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"ATOP            ", "ATOP"},
		{"LL2547G0238     ", "LL2547G0238"},
		{"APSP85B33IDL03NG", "APSP85B33IDL03NG"},
	}
	for _, c := range cases {
		if got := trimXcvrEepromString(c.in); got != c.want {
			t.Errorf("trimXcvrEepromString(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrimXcvrMfgDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"2025-11-28   ", "2025-11-28"},
		{"2025-11-28 00", "2025-11-28"},
		{"2025-11-28", "2025-11-28"},
	}
	for _, c := range cases {
		if got := trimXcvrMfgDate(c.in); got != c.want {
			t.Errorf("trimXcvrMfgDate(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

// newXcvrStateForTest returns a transceiver State object with the optical-power
// sub-containers built, ready for fillXcvrAggregatePower.
func newXcvrStateForTest(t *testing.T) *ocbinds.OpenconfigPlatform_Components_Component_Transceiver_State {
	t.Helper()
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	ygot.BuildEmptyTree(comp.Transceiver)
	ygot.BuildEmptyTree(comp.Transceiver.State)
	return comp.Transceiver.State
}

// TestFillXcvrAggregatePower verifies the OpenConfig "sum of physical channels"
// aggregation for transceiver/state input-power, output-power and
// laser-bias-current.
func TestFillXcvrAggregatePower(t *testing.T) {
	const eps = 1e-6

	t.Run("single active lane", func(t *testing.T) {
		xcvrState := newXcvrStateForTest(t)
		var xcvrInfo XcvrInfo
		xcvrInfo.Lanes[0] = XcvrLane{RxPowerLane: "-4.145", TxPowerLane: "-3.969", TxBiasLane: "6.834"}
		for i := 1; i < XCVR_LANE_LIMIT; i++ {
			xcvrInfo.Lanes[i] = XcvrLane{RxPowerLane: "N/A", TxPowerLane: "N/A", TxBiasLane: "N/A"}
		}

		fillXcvrAggregatePower(xcvrState, xcvrInfo)

		if xcvrState.InputPower.Instant == nil || math.Abs(*xcvrState.InputPower.Instant-(-4.145)) > eps {
			t.Errorf("InputPower.Instant = %v, want -4.145", xcvrState.InputPower.Instant)
		}
		if xcvrState.OutputPower.Instant == nil || math.Abs(*xcvrState.OutputPower.Instant-(-3.969)) > eps {
			t.Errorf("OutputPower.Instant = %v, want -3.969", xcvrState.OutputPower.Instant)
		}
		if xcvrState.LaserBiasCurrent.Instant == nil || math.Abs(*xcvrState.LaserBiasCurrent.Instant-6.834) > eps {
			t.Errorf("LaserBiasCurrent.Instant = %v, want 6.834", xcvrState.LaserBiasCurrent.Instant)
		}
	})

	t.Run("multi lane sum", func(t *testing.T) {
		xcvrState := newXcvrStateForTest(t)
		var xcvrInfo XcvrInfo
		// Two lanes at 0 dBm (1 mW each) -> 2 mW aggregate -> 10*log10(2) dBm.
		xcvrInfo.Lanes[0] = XcvrLane{RxPowerLane: "0", TxPowerLane: "0", TxBiasLane: "1.0"}
		xcvrInfo.Lanes[1] = XcvrLane{RxPowerLane: "0", TxPowerLane: "0", TxBiasLane: "2.0"}
		for i := 2; i < XCVR_LANE_LIMIT; i++ {
			xcvrInfo.Lanes[i] = XcvrLane{RxPowerLane: "N/A", TxPowerLane: "N/A", TxBiasLane: "N/A"}
		}

		fillXcvrAggregatePower(xcvrState, xcvrInfo)

		wantDbm := 10 * math.Log10(2)
		if xcvrState.InputPower.Instant == nil || math.Abs(*xcvrState.InputPower.Instant-wantDbm) > eps {
			t.Errorf("InputPower.Instant = %v, want %v", xcvrState.InputPower.Instant, wantDbm)
		}
		if xcvrState.OutputPower.Instant == nil || math.Abs(*xcvrState.OutputPower.Instant-wantDbm) > eps {
			t.Errorf("OutputPower.Instant = %v, want %v", xcvrState.OutputPower.Instant, wantDbm)
		}
		if xcvrState.LaserBiasCurrent.Instant == nil || math.Abs(*xcvrState.LaserBiasCurrent.Instant-3.0) > eps {
			t.Errorf("LaserBiasCurrent.Instant = %v, want 3.0", xcvrState.LaserBiasCurrent.Instant)
		}
	})

	t.Run("all lanes N/A leaves aggregate unset", func(t *testing.T) {
		xcvrState := newXcvrStateForTest(t)
		var xcvrInfo XcvrInfo
		for i := 0; i < XCVR_LANE_LIMIT; i++ {
			xcvrInfo.Lanes[i] = XcvrLane{RxPowerLane: "N/A", TxPowerLane: "N/A", TxBiasLane: "N/A"}
		}

		fillXcvrAggregatePower(xcvrState, xcvrInfo)

		if xcvrState.InputPower.Instant != nil {
			t.Errorf("InputPower.Instant = %v, want nil", *xcvrState.InputPower.Instant)
		}
		if xcvrState.OutputPower.Instant != nil {
			t.Errorf("OutputPower.Instant = %v, want nil", *xcvrState.OutputPower.Instant)
		}
		if xcvrState.LaserBiasCurrent.Instant != nil {
			t.Errorf("LaserBiasCurrent.Instant = %v, want nil", *xcvrState.LaserBiasCurrent.Instant)
		}
	})
}

func TestResolveTransceiverPresence(t *testing.T) {
	cases := []struct {
		name        string
		swStatus    string
		hwStatus    string
		infoPresent bool
		want        bool
		wantErr     bool
	}{
		{name: "sw inserted", swStatus: SFP_STATUS_INSERTED, want: true},
		{name: "sw removed", swStatus: SFP_STATUS_REMOVED, want: false},
		{name: "legacy hw inserted", hwStatus: SFP_STATUS_INSERTED, want: true},
		{name: "legacy hw removed", hwStatus: SFP_STATUS_REMOVED, want: false},
		{name: "sw wins over legacy hw", swStatus: SFP_STATUS_INSERTED, hwStatus: SFP_STATUS_REMOVED, want: true},
		{name: "hw-only row with info present", infoPresent: true, want: true},
		{name: "hw-only row without info", want: false},
		{name: "unknown status", swStatus: "bad", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveTransceiverPresence(c.swStatus, c.hwStatus, c.infoPresent)
			if c.wantErr {
				if err == nil {
					t.Fatalf("resolveTransceiverPresence() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveTransceiverPresence() unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("resolveTransceiverPresence() = %v, want %v", got, c.want)
			}
		})
	}
}

func seedFanInfoEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: FAN_TBL}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed FAN_INFO for %s: %v", name, err)
	}
}

func assertFanSpeed(t *testing.T, comp *ocbinds.OpenconfigPlatform_Components_Component, want uint32) {
	t.Helper()
	if comp.Fan == nil || comp.Fan.State == nil || comp.Fan.State.Speed == nil {
		t.Fatalf("Fan.State.Speed is nil, want %d", want)
	}
	if *comp.Fan.State.Speed != want {
		t.Fatalf("Fan.State.Speed = %d, want %d", *comp.Fan.State.Speed, want)
	}
}

// TestFillFanSpeed verifies FAN_INFO.speed from STATE_DB is mapped to
// /components/component/fan/state/openconfig-platform-fan:speed.
func TestFillFanSpeed(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	seedFanInfoEntry(t, d, "Fan1", map[string]string{
		"presence":    "true",
		"status":      "true",
		"speed":       "46",
		"drawer_name": "drawer1",
	})
	seedFanInfoEntry(t, d, "Fan2", map[string]string{
		"presence":    "true",
		"status":      "true",
		"speed":       "51",
		"drawer_name": "drawer2",
	})

	cases := []struct {
		name          string
		fanName       string
		targetUriPath string
		wantSpeed     uint32
	}{
		{name: "speed leaf", fanName: "Fan1", targetUriPath: COMP_FAN_SPEED, wantSpeed: 46},
		{name: "fan state container", fanName: "Fan1", targetUriPath: COMP_FAN_ST, wantSpeed: 46},
		{name: "fan container", fanName: "Fan1", targetUriPath: COMP_FAN, wantSpeed: 46},
		{name: "full component path", fanName: "Fan1", targetUriPath: COMP, wantSpeed: 46},
		{name: "second fan", fanName: "Fan2", targetUriPath: COMP_FAN_SPEED, wantSpeed: 51},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillFanInfo(comp, c.fanName, c.targetUriPath, d, CompTypeFan); err != nil {
				t.Fatalf("fillFanInfo() unexpected error: %v", err)
			}
			assertFanSpeed(t, comp, c.wantSpeed)
		})
	}

	t.Run("invalid speed leaves speed unset", func(t *testing.T) {
		seedFanInfoEntry(t, d, "Fan3", map[string]string{
			"presence": "true",
			"speed":    "bad",
		})
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillFanInfo(comp, "Fan3", COMP_FAN_SPEED, d, CompTypeFan); err != nil {
			t.Fatalf("fillFanInfo() unexpected error: %v", err)
		}
		if comp.Fan != nil && comp.Fan.State != nil && comp.Fan.State.Speed != nil {
			t.Fatalf("Fan.State.Speed = %d, want nil for invalid speed", *comp.Fan.State.Speed)
		}
	})

	t.Run("missing speed leaves speed unset", func(t *testing.T) {
		seedFanInfoEntry(t, d, "Fan4", map[string]string{
			"presence": "true",
		})
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillFanInfo(comp, "Fan4", COMP_FAN_SPEED, d, CompTypeFan); err != nil {
			t.Fatalf("fillFanInfo() unexpected error: %v", err)
		}
		if comp.Fan != nil && comp.Fan.State != nil && comp.Fan.State.Speed != nil {
			t.Fatalf("Fan.State.Speed = %d, want nil when speed field is absent", *comp.Fan.State.Speed)
		}
	})
}

// TestGetSysComponentsFanWildcard exercises the wildcard (no component name)
// GET on the fan sub-container, e.g. /components/component/fan, which must
// enumerate fan components instead of returning an empty container.
func TestGetSysComponentsFanWildcard(t *testing.T) {
	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)

	var inParams XfmrParams
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	inParams.dbs[db.StateDB] = d

	for _, targetUriPath := range []string{COMP_FAN, COMP_FAN_ST, COMP_FAN_SPEED} {
		t.Run(targetUriPath, func(t *testing.T) {
			if err := getSysComponents(device.Components, targetUriPath, inParams, "", ""); err != nil {
				t.Fatalf("getSysComponents(%q, wildcard) returned error: %v", targetUriPath, err)
			}
		})
	}
}
