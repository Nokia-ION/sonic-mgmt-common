package transformer

import (
	"encoding/binary"
	"math"
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
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
		{CompTypeFilesystem, "CompTypeFilesystem"},
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
// "N/A" sentinel that xcvrd writes for unused/unsupported lanes and the "-inf"
// it writes for a lane receiving 0 mW.
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
		{"-inf", 0, false},
		{"-Inf", 0, false},
		{"inf", 0, false},
		{"-infinity", 0, false},
		{"nan", 0, false},
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

// newXcvrChannelForTest returns a physical channel with its State sub-containers
// built, ready for convAndFillDBValues.
func newXcvrChannelForTest(t *testing.T) *ocbinds.OpenconfigPlatform_Components_Component_Transceiver_PhysicalChannels_Channel {
	t.Helper()
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	ygot.BuildEmptyTree(comp.Transceiver)
	channel, err := comp.Transceiver.PhysicalChannels.NewChannel(0)
	if err != nil {
		t.Fatalf("NewChannel(0) returned an error: %v", err)
	}
	ygot.BuildEmptyTree(channel)
	ygot.BuildEmptyTree(channel.State)
	return channel
}

// TestConvAndFillDBValues covers the per-channel DOM leaves, in particular that
// the "-inf" xcvrd writes for a lane receiving 0 mW is omitted rather than
// reported as a non-finite decimal64.
func TestConvAndFillDBValues(t *testing.T) {
	const eps = 1e-6

	t.Run("finite readings are reported", func(t *testing.T) {
		channel := newXcvrChannelForTest(t)
		convAndFillDBValues("-4.145", "-3.969", "6.834", "False", channel)

		if channel.State.InputPower.Instant == nil || math.Abs(*channel.State.InputPower.Instant-(-4.145)) > eps {
			t.Errorf("InputPower.Instant = %v, want -4.145", channel.State.InputPower.Instant)
		}
		if channel.State.OutputPower.Instant == nil || math.Abs(*channel.State.OutputPower.Instant-(-3.969)) > eps {
			t.Errorf("OutputPower.Instant = %v, want -3.969", channel.State.OutputPower.Instant)
		}
		if channel.State.LaserBiasCurrent.Instant == nil || math.Abs(*channel.State.LaserBiasCurrent.Instant-6.834) > eps {
			t.Errorf("LaserBiasCurrent.Instant = %v, want 6.834", channel.State.LaserBiasCurrent.Instant)
		}
		if channel.State.TxLaser == nil || !*channel.State.TxLaser {
			t.Errorf("TxLaser = %v, want true", channel.State.TxLaser)
		}
	})

	t.Run("dark lane omits optical power", func(t *testing.T) {
		channel := newXcvrChannelForTest(t)
		convAndFillDBValues("-inf", "-inf", "0.0", "True", channel)

		if channel.State.InputPower.Instant != nil {
			t.Errorf("InputPower.Instant = %v, want nil", *channel.State.InputPower.Instant)
		}
		if channel.State.OutputPower.Instant != nil {
			t.Errorf("OutputPower.Instant = %v, want nil", *channel.State.OutputPower.Instant)
		}
		if channel.State.LaserBiasCurrent.Instant == nil || *channel.State.LaserBiasCurrent.Instant != 0 {
			t.Errorf("LaserBiasCurrent.Instant = %v, want 0", channel.State.LaserBiasCurrent.Instant)
		}
		if channel.State.TxLaser == nil || *channel.State.TxLaser {
			t.Errorf("TxLaser = %v, want false", channel.State.TxLaser)
		}
	})

	t.Run("unused lane omits all readings", func(t *testing.T) {
		channel := newXcvrChannelForTest(t)
		convAndFillDBValues("N/A", "N/A", "N/A", "", channel)

		if channel.State.InputPower.Instant != nil {
			t.Errorf("InputPower.Instant = %v, want nil", *channel.State.InputPower.Instant)
		}
		if channel.State.OutputPower.Instant != nil {
			t.Errorf("OutputPower.Instant = %v, want nil", *channel.State.OutputPower.Instant)
		}
		if channel.State.LaserBiasCurrent.Instant != nil {
			t.Errorf("LaserBiasCurrent.Instant = %v, want nil", *channel.State.LaserBiasCurrent.Instant)
		}
	})
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

	t.Run("dark lanes are excluded from the aggregate", func(t *testing.T) {
		xcvrState := newXcvrStateForTest(t)
		var xcvrInfo XcvrInfo
		// A lane receiving no light is written by xcvrd as "-inf" dBm; it must
		// contribute nothing rather than poison the sum.
		xcvrInfo.Lanes[0] = XcvrLane{RxPowerLane: "0", TxPowerLane: "0", TxBiasLane: "1.0"}
		xcvrInfo.Lanes[1] = XcvrLane{RxPowerLane: "-inf", TxPowerLane: "-inf", TxBiasLane: "-inf"}
		for i := 2; i < XCVR_LANE_LIMIT; i++ {
			xcvrInfo.Lanes[i] = XcvrLane{RxPowerLane: "N/A", TxPowerLane: "N/A", TxBiasLane: "N/A"}
		}

		fillXcvrAggregatePower(xcvrState, xcvrInfo)

		if xcvrState.InputPower.Instant == nil || math.Abs(*xcvrState.InputPower.Instant) > eps {
			t.Errorf("InputPower.Instant = %v, want 0", xcvrState.InputPower.Instant)
		}
		if xcvrState.OutputPower.Instant == nil || math.Abs(*xcvrState.OutputPower.Instant) > eps {
			t.Errorf("OutputPower.Instant = %v, want 0", xcvrState.OutputPower.Instant)
		}
		if xcvrState.LaserBiasCurrent.Instant == nil || math.Abs(*xcvrState.LaserBiasCurrent.Instant-1.0) > eps {
			t.Errorf("LaserBiasCurrent.Instant = %v, want 1.0", xcvrState.LaserBiasCurrent.Instant)
		}
	})

	t.Run("all lanes dark leaves aggregate unset", func(t *testing.T) {
		xcvrState := newXcvrStateForTest(t)
		var xcvrInfo XcvrInfo
		for i := 0; i < XCVR_LANE_LIMIT; i++ {
			xcvrInfo.Lanes[i] = XcvrLane{RxPowerLane: "-inf", TxPowerLane: "-inf", TxBiasLane: "-inf"}
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

// A targeted GET on state/type takes a different path through getSysComponents
// than a whole-component GET, and must report the same identity on both.
func TestGetSysComponentsFanTrayStateType(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedFanDrawerInfoEntry(t, d, "drawer1", map[string]string{"presence": "True", "status": "True"})
	seedFanInfoEntry(t, d, "Fan1", map[string]string{"presence": "True", "status": "True"})

	cases := []struct {
		name string
		want interface{}
	}{
		{"drawer1", ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_FAN_TRAY},
		{"Fan1", ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_FAN},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			device := &ocbinds.Device{}
			ygot.BuildEmptyTree(device)
			if _, err := device.Components.NewComponent(c.name); err != nil {
				t.Fatalf("creating the component failed: %v", err)
			}

			var inParams XfmrParams
			inParams.dbs[db.StateDB] = d
			if err := getSysComponents(device.Components, COMP_STATE_TYPE, inParams, c.name, ""); err != nil {
				t.Fatalf("getSysComponents(%q) returned error: %v", c.name, err)
			}

			comp := device.Components.Component[c.name]
			if comp == nil || comp.State == nil || comp.State.Type == nil {
				t.Fatalf("state/type not populated for %s", c.name)
			}
			want, err := comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(c.want)
			if err != nil {
				t.Fatalf("building the expected type union: %v", err)
			}
			if !reflect.DeepEqual(comp.State.Type, want) {
				t.Errorf("state/type == %v, want %v", comp.State.Type, want)
			}
		})
	}
}

func ieeeFloat32FromBinary(b ocbinds.Binary) (float32, bool) {
	if len(b) != 4 {
		return 0, false
	}
	return math.Float32frombits(binary.BigEndian.Uint32(b)), true
}

func assertIEEEFloat32(t *testing.T, got ocbinds.Binary, want float32) {
	t.Helper()
	v, ok := ieeeFloat32FromBinary(got)
	if !ok {
		t.Fatalf("expected 4-byte ieeefloat32, got %v", got)
	}
	if math.Abs(float64(v-want)) > 1e-5 {
		t.Fatalf("got %v, want %v", v, want)
	}
}

func TestExtractFloat32StrWithFallback(t *testing.T) {
	t.Run("primary preferred over fallback", func(t *testing.T) {
		entry := &db.Value{Field: map[string]string{
			"voltage":        "12.5",
			"output_voltage": "11.0",
		}}
		assertIEEEFloat32(t, extractFloat32StrWithFallback("voltage", "output_voltage", entry), 12.5)
	})
	t.Run("fallback when primary missing", func(t *testing.T) {
		entry := &db.Value{Field: map[string]string{"output_voltage": "11.0"}}
		assertIEEEFloat32(t, extractFloat32StrWithFallback("voltage", "output_voltage", entry), 11.0)
	})
	t.Run("fallback when primary is N/A", func(t *testing.T) {
		entry := &db.Value{Field: map[string]string{
			"voltage":        "N/A",
			"output_voltage": "11.0",
		}}
		assertIEEEFloat32(t, extractFloat32StrWithFallback("voltage", "output_voltage", entry), 11.0)
	})
	t.Run("omit when both unavailable", func(t *testing.T) {
		entry := &db.Value{Field: map[string]string{"voltage": "N/A"}}
		if got := extractFloat32StrWithFallback("voltage", "output_voltage", entry); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
	t.Run("no fallback field", func(t *testing.T) {
		entry := &db.Value{Field: map[string]string{"max_power": "750"}}
		assertIEEEFloat32(t, extractFloat32StrWithFallback("max_power", "", entry), 750)
		if got := extractFloat32StrWithFallback("missing", "", entry); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}

func TestSetPsuElectricalInfoSonicFields(t *testing.T) {
	entry := &db.Value{Field: map[string]string{
		"voltage":       "12.00875",
		"current":       "3.1",
		"power":         "37.227125",
		"input_voltage": "N/A",
		"input_current": "N/A",
		"max_power":     "N/A",
	}}
	var info PSUInfo
	setPsuElectricalInfo(&info, entry)

	assertIEEEFloat32(t, info.Output_Voltage, 12.00875)
	assertIEEEFloat32(t, info.Output_Current, 3.1)
	assertIEEEFloat32(t, info.Output_Power, 37.227125)
	if info.Input_Voltage != nil || info.Input_Current != nil || info.Capacity != nil {
		t.Fatalf("expected N/A electrical leaves to be omitted, got inV=%v inC=%v cap=%v",
			info.Input_Voltage, info.Input_Current, info.Capacity)
	}
}

func TestSetPsuElectricalInfoLegacyFallback(t *testing.T) {
	entry := &db.Value{Field: map[string]string{
		"output_voltage": "12.0",
		"output_current": "4.0",
		"output_power":   "48.0",
		"max_power":      "750.5",
		"input_voltage":  "220.0",
		"input_current":  "1.5",
	}}
	var info PSUInfo
	setPsuElectricalInfo(&info, entry)

	assertIEEEFloat32(t, info.Output_Voltage, 12.0)
	assertIEEEFloat32(t, info.Output_Current, 4.0)
	assertIEEEFloat32(t, info.Output_Power, 48.0)
	assertIEEEFloat32(t, info.Capacity, 750.5)
	assertIEEEFloat32(t, info.Input_Voltage, 220.0)
	assertIEEEFloat32(t, info.Input_Current, 1.5)
}

func TestApplyPsuPowerSupplyState(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	info := PSUInfo{
		Output_Voltage: mustIEEEFloat32(t, "12.00875"),
		Output_Current: mustIEEEFloat32(t, "3.1"),
		Output_Power:   mustIEEEFloat32(t, "37.227125"),
	}
	applyPsuPowerSupplyState(comp, info)

	if comp.PowerSupply == nil || comp.PowerSupply.State == nil {
		t.Fatal("expected power-supply/state to be populated")
	}
	st := comp.PowerSupply.State
	assertIEEEFloat32(t, st.OutputVoltage, 12.00875)
	assertIEEEFloat32(t, st.OutputCurrent, 3.1)
	assertIEEEFloat32(t, st.OutputPower, 37.227125)
	if st.InputVoltage != nil || st.Capacity != nil {
		t.Fatalf("expected nil input-voltage/capacity, got %v / %v", st.InputVoltage, st.Capacity)
	}
}

func mustIEEEFloat32(t *testing.T, s string) ocbinds.Binary {
	t.Helper()
	b, err := float32StrTo4Bytes(s)
	if err != nil {
		t.Fatalf("float32StrTo4Bytes(%q): %v", s, err)
	}
	return b
}

func TestFillPsuInfoPowerStatsPath(t *testing.T) {
	// Exercise apply path used for /power-supply and /power-supply/state without
	// requiring a live Redis PSU_INFO row: map fields then apply state.
	entry := &db.Value{Field: map[string]string{
		"voltage": "12.0",
		"current": "2.0",
		"power":   "24.0",
	}}
	var info PSUInfo
	setPsuElectricalInfo(&info, entry)

	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	applyPsuPowerSupplyState(comp, info)

	st := comp.PowerSupply.State
	assertIEEEFloat32(t, st.OutputVoltage, 12.0)
	assertIEEEFloat32(t, st.OutputCurrent, 2.0)
	assertIEEEFloat32(t, st.OutputPower, 24.0)
}

func TestEnsurePsuSingularLeafAssignment(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ensurePsuPowerSupplyState(comp)
	want := mustIEEEFloat32(t, "12.5")
	comp.PowerSupply.State.OutputVoltage = want
	assertIEEEFloat32(t, comp.PowerSupply.State.OutputVoltage, 12.5)
}

func TestGetCompTypeByNameFilesystem(t *testing.T) {
	cases := []struct {
		in   string
		want componentType
	}{
		{"/", CompTypeFilesystem},
		{"/var/log", CompTypeFilesystem},
		{"/sys/kernel/debug/tracing", CompTypeFilesystem},
		// A name with no leading slash must not be claimed as a filesystem.
		{"var/log", CompTypeInvalid},
		{"", CompTypeInvalid},
	}
	for _, c := range cases {
		got, err := getCompTypeByName(c.in)
		if got != c.want {
			t.Errorf("getCompTypeByName(%q) == %v, want %v", c.in, got, c.want)
		}
		if (err == nil) != (c.want != CompTypeInvalid) {
			t.Errorf("getCompTypeByName(%q) returned err %v, inconsistent with type %v", c.in, err, got)
		}
	}
}

// Pins the two derivations a mix-up would leave plausible-looking: used is total
// minus free, not total minus available, and byte counts scale by Frsize.
func TestFsPropFields(t *testing.T) {
	st := syscall.Statfs_t{
		Blocks: 1011640,
		Bfree:  915617,
		Bavail: 859093,
		Frsize: 4096,
		// Deliberately unequal to Frsize so a Bsize mix-up cannot pass.
		Bsize: 8192,
	}
	want := map[string]uint64{
		"total_blocks":     1011640,
		"free_blocks":      915617,
		"available_blocks": 859093,
		"total_bytes":      1011640 * 4096,
		"free_bytes":       915617 * 4096,
		"available_bytes":  859093 * 4096,
		"used_blocks":      96023,
		"used_bytes":       96023 * 4096,
	}
	if len(fsPropFields) != len(want) {
		t.Fatalf("fsPropFields has %d entries, want %d", len(fsPropFields), len(want))
	}
	for name, field := range fsPropFields {
		expected, ok := want[name]
		if !ok {
			t.Errorf("unexpected property %q in fsPropFields", name)
			continue
		}
		if got := field(&st); got != expected {
			t.Errorf("fsPropFields[%q] == %d, want %d", name, got, expected)
		}
	}
}

func TestHostPath(t *testing.T) {
	// The prefix is applied only when /mnt/host exists, so assert against
	// whichever branch this machine is in.  The last two cases must not escape.
	_, err := os.Stat("/mnt/host")
	cases := []struct {
		in       string
		bare     string
		prefixed string
	}{
		{"/", "/", "/mnt/host"},
		{"/var/log", "/var/log", "/mnt/host/var/log"},
		{"/dev/shm", "/dev/shm", "/mnt/host/dev/shm"},
		{"/../etc", "/etc", "/mnt/host/etc"},
		{"/var/log/../..", "/", "/mnt/host"},
	}
	for _, c := range cases {
		want := c.bare
		if err == nil {
			want = c.prefixed
		}
		if got := hostPath(c.in); got != want {
			t.Errorf("hostPath(%q) == %q, want %q", c.in, got, want)
		}
	}
}

// A filesystem name that does not resolve must yield no component at all, not a
// typed one with no properties.
func TestFillFilesystemInfoUnresolvedName(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillFilesystemInfo(comp, "/no/such/mount/point", "")
	if err == nil {
		t.Fatal("fillFilesystemInfo on a nonexistent path returned nil error")
	}
	if comp.State != nil || comp.Properties != nil {
		t.Errorf("fillFilesystemInfo populated the component despite failing: %v", comp)
	}
}

// A property name that is not one of the eight must be reported as not found,
// not answered with an empty component the caller cannot interpret.
func TestFillFilesystemInfoUnknownProperty(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	err := fillFilesystemInfo(comp, "/", "bogus")
	if _, ok := err.(tlerr.NotFoundError); !ok {
		t.Fatalf("fillFilesystemInfo with an unknown property returned %T (%v), want tlerr.NotFoundError", err, err)
	}
	if comp.State != nil || comp.Properties != nil {
		t.Errorf("fillFilesystemInfo populated the component despite failing: %v", comp)
	}
}

func TestFillFilesystemInfo(t *testing.T) {
	// "/" resolves everywhere this test can run, including a build container.
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := fillFilesystemInfo(comp, "/", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if comp.State == nil || comp.State.Name == nil || *comp.State.Name != "/" {
		t.Errorf("state/name not set to the component name: %v", comp.State)
	}
	// The union members are pointers to generated structs, so compare with
	// DeepEqual rather than ==, which would compare the pointers.
	wantType, err := comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
		ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_STORAGE)
	if err != nil {
		t.Fatalf("unexpected error building the expected type union: %v", err)
	}
	if !reflect.DeepEqual(comp.State.Type, wantType) {
		t.Errorf("state/type == %v, want %v", comp.State.Type, wantType)
	}

	if got := len(comp.Properties.Property); got != len(fsPropFields) {
		t.Errorf("emitted %d properties, want %d", got, len(fsPropFields))
	}
	for name := range fsPropFields {
		prop, ok := comp.Properties.Property[name]
		if !ok {
			t.Errorf("property %q missing", name)
			continue
		}
		if prop.State.Name == nil || *prop.State.Name != name {
			t.Errorf("property %q has state/name %v", name, prop.State.Name)
		}
		if prop.State.Value == nil {
			t.Errorf("property %q has no state/value", name)
		}
	}

	// Checks the value reaches the leaf unchanged.  The Frsize scaling itself is
	// pinned by TestFsPropFields, which uses an unequal Bsize; this cannot, since
	// the two are equal on any filesystem this test runs against.
	var stat syscall.Statfs_t
	if err := syscall.Statfs(hostPath("/"), &stat); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	totalBytes := comp.Properties.Property["total_bytes"].State
	wantBytes, err := totalBytes.
		To_OpenconfigPlatform_Components_Component_Properties_Property_State_Value_Union(stat.Blocks * uint64(stat.Frsize))
	if err != nil {
		t.Fatalf("unexpected error building the expected value union: %v", err)
	}
	if !reflect.DeepEqual(totalBytes.Value, wantBytes) {
		t.Errorf("total_bytes == %v, want %v", totalBytes.Value, wantBytes)
	}
}

// A property name in the request must select exactly that one property.
func TestFillFilesystemInfoSingleProperty(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	if err := fillFilesystemInfo(comp, "/", "used_bytes"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(comp.Properties.Property); got != 1 {
		t.Fatalf("emitted %d properties, want 1: %v", got, comp.Properties.Property)
	}
	if _, ok := comp.Properties.Property["used_bytes"]; !ok {
		t.Errorf("used_bytes missing: %v", comp.Properties.Property)
	}
}

// The framework pre-creates the list entries named in the request URI, so the
// fill must populate an existing property rather than skipping it.
func TestFillFilesystemInfoPreCreatedProperty(t *testing.T) {
	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if _, err := comp.Properties.NewProperty("free_bytes"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := fillFilesystemInfo(comp, "/", "free_bytes"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prop := comp.Properties.Property["free_bytes"]
	if prop == nil || prop.State == nil || prop.State.Value == nil {
		t.Errorf("pre-created property was not populated: %v", prop)
	}
}

// A filesystem subscribe must claim a virtual table, so the framework skips DB
// key validation, and must disable on-change, which needs an explicit
// assignment because the zero value is OnchangeDefault.
func TestSubscribe_pfm_components_filesystem(t *testing.T) {
	for _, uri := range []string{
		"/openconfig-platform:components/component[name=/var/log]",
		"/openconfig-platform:components/component[name=/var/log]/properties/property[name=used_bytes]/state/value",
	} {
		t.Run(uri, func(t *testing.T) {
			out, err := Subscribe_pfm_components_xfmr(
				XfmrSubscInParams{uri: uri, requestURI: uri, subscProc: TRANSLATE_SUBSCRIBE})
			if err != nil {
				t.Fatalf("Subscribe_pfm_components_xfmr failed: %v", err)
			}
			if !out.isVirtualTbl {
				t.Error("isVirtualTbl is false; a filesystem has no DB row to validate")
			}
			if out.onChange != OnchangeDisable {
				t.Errorf("onChange == %v, want OnchangeDisable", out.onChange)
			}
			if len(out.dbDataMap) != 0 {
				t.Errorf("expected no DB subscriptions, got %v", out.dbDataMap)
			}
		})
	}
}

func seedPsuInfoEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: PSU_TBL}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed PSU_INFO for %s: %v", name, err)
	}
}

func seedFanDrawerInfoEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: FAN_TRAY_TBL}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed FAN_DRAWER_INFO for %s: %v", name, err)
	}
}

// A fan drawer is named either drawerN or in one of the FanTray forms.  All of
// them must type as a fan tray, while a name that merely begins with "drawer"
// must not.
func TestValidFanTrayName(t *testing.T) {
	cases := map[string]bool{
		"drawer1": true, "drawer4": true, "drawer10": true,
		"FanTray1": true, "1_fan_drawer": true,
		"drawer": false, "drawer0": false, "drawer1a": false,
		"drawer_disk_carrier": false, "Fan1": false, "": false,
	}
	for in, want := range cases {
		if got := validFanTrayName(in); got != want {
			t.Errorf("validFanTrayName(%q) == %v, want %v", in, got, want)
		}
	}
}

// A fan drawer must resolve to CompTypeFanTray, or every leaf on it is served as
// an unknown component.
func TestGetCompTypeByNameFanDrawer(t *testing.T) {
	for _, name := range []string{"drawer1", "drawer4", "FanTray1"} {
		got, err := getCompTypeByName(name)
		if err != nil {
			t.Errorf("getCompTypeByName(%q) failed: %v", name, err)
			continue
		}
		if got != CompTypeFanTray {
			t.Errorf("getCompTypeByName(%q) == %v, want CompTypeFanTray", name, got)
		}
	}
}

func seedChassisInfoEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: CHASSIS_TBL}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed CHASSIS_INFO for %s: %v", name, err)
	}
}

func seedXcvrInfoEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: TRANSCEIVER_TBL}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed TRANSCEIVER_INFO for %s: %v", name, err)
	}
}

func seedXcvrFirmwareEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: TRANSCEIVER_FIRMWARE}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed TRANSCEIVER_FIRMWARE_INFO for %s: %v", name, err)
	}
}

func seedXcvrDomEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: TRANSCEIVER_DOM}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed TRANSCEIVER_DOM_SENSOR for %s: %v", name, err)
	}
}

func seedPcieDeviceEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: PCIE_TBL}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed PCIE_DEVICE for %s: %v", name, err)
	}
}

func assertStrLeaf(t *testing.T, got *string, want, leaf string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %q", leaf, want)
	}
	if *got != want {
		t.Errorf("%s = %q, want %q", leaf, *got, want)
	}
}

func assertStrLeafUnset(t *testing.T, got *string, leaf string) {
	t.Helper()
	if got != nil {
		t.Errorf("%s = %q, want unset", leaf, *got)
	}
}

func assertUint64Leaf(t *testing.T, got *uint64, want uint64, leaf string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %d", leaf, want)
	}
	if *got != want {
		t.Errorf("%s = %d, want %d", leaf, *got, want)
	}
}

// The route resources are named V4Routes and V6Routes, used and free are the
// CRM counts, and max-limit is their sum. high-watermark has no count and stays
// unset.
func TestFillICUtilization(t *testing.T) {
	stats := db.Value{Field: map[string]string{
		"crm_stats_ipv4_route_used":      "66",
		"crm_stats_ipv4_route_available": "1048510",
		"crm_stats_ipv6_route_used":      "3",
		"crm_stats_ipv6_route_available": "1048573",
	}}

	util := &ocbinds.OpenconfigPlatform_Components_Component_IntegratedCircuit_Utilization{}
	if err := fillICUtilization(util, stats); err != nil {
		t.Fatalf("fillICUtilization() unexpected error: %v", err)
	}

	want := map[string]struct {
		used, free, maxLimit uint64
	}{
		"V4Routes": {66, 1048510, 1048576},
		"V6Routes": {3, 1048573, 1048576},
	}
	if len(util.Resources.Resource) != len(want) {
		t.Fatalf("resources = %d, want %d", len(util.Resources.Resource), len(want))
	}
	for name, w := range want {
		res, ok := util.Resources.Resource[name]
		if !ok {
			t.Fatalf("resource %q is absent", name)
		}
		assertStrLeaf(t, res.Name, name, "resource/name")
		assertUint64Leaf(t, res.State.Used, w.used, name+" state/used")
		assertUint64Leaf(t, res.State.Free, w.free, name+" state/free")
		assertUint64Leaf(t, res.State.MaxLimit, w.maxLimit, name+" state/max-limit")
		if res.State.HighWatermark != nil {
			t.Errorf("%s state/high-watermark = %d, want unset", name, *res.State.HighWatermark)
		}
	}
}

// A count that CRM does not carry is not found, so a targeted get reports that
// rather than a zero.
func TestFillICUtilizationMissingCount(t *testing.T) {
	stats := db.Value{Field: map[string]string{"crm_stats_ipv4_route_used": "66"}}

	util := &ocbinds.OpenconfigPlatform_Components_Component_IntegratedCircuit_Utilization{}
	err := fillICUtilization(util, stats)
	if _, ok := err.(tlerr.NotFoundError); !ok {
		t.Fatalf("fillICUtilization() returned %T (%v), want tlerr.NotFoundError", err, err)
	}
}

// firstAvailable skips empty values and the not-available sentinels the
// platform writes, and yields "" when nothing qualifies.
func TestFirstAvailable(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"first value wins", []string{"a", "b"}, "a"},
		{"skips empty", []string{"", "b"}, "b"},
		{"skips N/A", []string{"N/A", "b"}, "b"},
		{"skips lowercase n/a", []string{"n/a", "b"}, "b"},
		{"nothing available", []string{"", "N/A", "n/a"}, ""},
		{"no values", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstAvailable(c.in...); got != c.want {
				t.Errorf("firstAvailable(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// CHASSIS_INFO names its inventory fields differently from the OpenConfig
// leaves, so description, part-no and hardware-version come from model and
// revision when the OpenConfig spellings are absent.
func TestFillFirmwareInfoFieldFallbacks(t *testing.T) {
	stdb, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer stdb.DeleteDB()
	cfgdb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer cfgdb.DeleteDB()

	name := "chassis 9"
	seedChassisInfoEntry(t, stdb, name, map[string]string{
		"model":    "3HE20596AA0101",
		"revision": "0",
		"serial":   "NS1234567890",
	})

	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if err := fillFirmwareInfo(comp, name, AllPaths, COMP, stdb, cfgdb); err != nil {
		t.Fatalf("fillFirmwareInfo() unexpected error: %v", err)
	}
	assertStrLeaf(t, comp.State.Description, "3HE20596AA0101", "state/description")
	assertStrLeaf(t, comp.State.PartNo, "3HE20596AA0101", "state/part-no")
	assertStrLeaf(t, comp.State.HardwareVersion, "0", "state/hardware-version")
}

// A CMIS module reports its running firmware in TRANSCEIVER_FIRMWARE_INFO, and
// its type string doubles as the component description. A module with no
// firmware row still serves the rest of its inventory.
func TestFillXcvrFirmwareVersionAndDescription(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	cfgdb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer cfgdb.DeleteDB()
	var dbs [db.MaxDB]*db.DB
	dbs[db.StateDB] = d
	dbs[db.ConfigDB] = cfgdb

	const cmisName = "Ethernet64"
	const cmisType = "QSFP-DD Double Density 8X Pluggable Transceiver"
	seedXcvrInfoEntry(t, d, cmisName, map[string]string{
		"type":         cmisType,
		"hardware_rev": "2.12",
		"model":        "1AB123456AA",
		"serial":       "ABC1234567",
	})
	seedXcvrFirmwareEntry(t, d, cmisName, map[string]string{"active_firmware": "180.139"})
	seedXcvrDomEntry(t, d, cmisName, map[string]string{"temperature": "35.0"})

	const sffName = "Ethernet68"
	seedXcvrInfoEntry(t, d, sffName, map[string]string{
		"type":   "SFP/SFP+/SFP28",
		"model":  "1AB000000AA",
		"serial": "XYZ7654321",
	})
	seedXcvrDomEntry(t, d, sffName, map[string]string{"temperature": "30.0"})

	t.Run("cmis module", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillXcvrInfo(comp, cmisName, true, "", COMP, dbs); err != nil {
			t.Fatalf("fillXcvrInfo() unexpected error: %v", err)
		}
		assertStrLeaf(t, comp.State.FirmwareVersion, "180.139", "state/firmware-version")
		assertStrLeaf(t, comp.State.HardwareVersion, "2.12", "state/hardware-version")
		assertStrLeaf(t, comp.State.Description, cmisType, "state/description")
	})

	t.Run("module without a firmware row", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillXcvrInfo(comp, sffName, true, "", COMP, dbs); err != nil {
			t.Fatalf("fillXcvrInfo() unexpected error: %v", err)
		}
		assertStrLeafUnset(t, comp.State.FirmwareVersion, "state/firmware-version")
		assertStrLeaf(t, comp.State.Description, "SFP/SFP+/SFP28", "state/description")
		assertStrLeaf(t, comp.State.SerialNo, "XYZ7654321", "state/serial-no")
	})

	singular := []struct {
		targetUriPath string
		get           func(*ocbinds.OpenconfigPlatform_Components_Component) *string
		want          string
	}{
		{COMP_STATE_FIRM_VER, func(c *ocbinds.OpenconfigPlatform_Components_Component) *string {
			return c.State.FirmwareVersion
		}, "180.139"},
		{COMP_STATE_DESCR, func(c *ocbinds.OpenconfigPlatform_Components_Component) *string {
			return c.State.Description
		}, cmisType},
	}
	for _, c := range singular {
		t.Run("targeted "+c.targetUriPath, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillXcvrInfo(comp, cmisName, false, "", c.targetUriPath, dbs); err != nil {
				t.Fatalf("fillXcvrInfo() unexpected error: %v", err)
			}
			assertStrLeaf(t, c.get(comp), c.want, c.targetUriPath)
		})
	}
}

// PSU_INFO.revision carries the PSU hardware revision on both the
// whole-component and the targeted leaf path, and the not-available sentinel
// leaves the leaf unset.
func TestFillPsuHardwareVersion(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	seedPsuInfoEntry(t, d, "PSU7", map[string]string{
		"presence": "true",
		"status":   "true",
		"model":    "3HE12345AA",
		"revision": "D1",
		"serial":   "PS1234567",
	})
	seedPsuInfoEntry(t, d, "PSU8", map[string]string{
		"presence": "true",
		"status":   "true",
		"revision": "N/A",
	})

	cases := []struct {
		name          string
		psuName       string
		all           bool
		targetUriPath string
		want          string
	}{
		{"whole component", "PSU7", true, COMP, "D1"},
		{"targeted leaf", "PSU7", false, COMP_STATE_HW_VER, "D1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillPsuInfo(comp, c.psuName, c.all, false, c.targetUriPath, d); err != nil {
				t.Fatalf("fillPsuInfo() unexpected error: %v", err)
			}
			assertStrLeaf(t, comp.State.HardwareVersion, c.want, "state/hardware-version")
		})
	}

	t.Run("sentinel revision", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillPsuInfo(comp, "PSU8", true, false, COMP, d); err != nil {
			t.Fatalf("fillPsuInfo() unexpected error: %v", err)
		}
		assertStrLeafUnset(t, comp.State.HardwareVersion, "state/hardware-version")
	})
}

// A PCIe component reports its own name, which a targeted GET of state/name
// reaches through a different branch than a whole-component GET.
func TestFillPcieName(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()

	name := "ff:1f.7"
	seedPcieDeviceEntry(t, d, name, map[string]string{"fatal|TOTAL_ERR_FATAL": "0"})

	cases := []struct {
		targetUriPath string
		pType         PathType
	}{
		{COMP, AllPaths},
		{COMP_ST, StatePaths},
		{COMP_STATE_NAME, SingularPath},
	}
	for _, c := range cases {
		t.Run(c.targetUriPath, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillPcieInfo(comp, name, c.pType, c.targetUriPath, d); err != nil {
				t.Fatalf("fillPcieInfo() unexpected error: %v", err)
			}
			assertStrLeaf(t, comp.State.Name, name, "state/name")
		})
	}
}
