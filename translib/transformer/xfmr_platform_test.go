package transformer

import (
	"encoding/binary"
	"math"
	"reflect"
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
		{CompTypeCpu, "CompTypeCpu"},
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
	seedChassisInfoEntry(t, sdb, name, map[string]string{
		"firmware-version": "1.0",
		"hardware-version": "0",
		"manufacture_date": "03/04/2024 10:00:00",
		"part-no":          "3HE18723AARA01",
		"serial":           "LL2503K1024",
	})
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

	t.Run("pre-created leaves with no reading are cleared", func(t *testing.T) {
		xcvrState := newXcvrStateForTest(t)
		rx, tx, bias := 0.0, 0.0, 0.0
		xcvrState.InputPower.Instant = &rx
		xcvrState.OutputPower.Instant = &tx
		xcvrState.LaserBiasCurrent.Instant = &bias
		var xcvrInfo XcvrInfo
		xcvrInfo.Lanes[0] = XcvrLane{RxPowerLane: "-inf", TxPowerLane: "-inf", TxBiasLane: "N/A"}
		for i := 1; i < XCVR_LANE_LIMIT; i++ {
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

func seedTempInfoEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: TEMP_TBL}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed TEMPERATURE_INFO for %s: %v", name, err)
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

// Chassis mfg-date comes from CHASSIS_INFO manufacture_date, falls back to the
// EEPROM_INFO Manufacture Date, and is NotFound when neither has it.
func TestFillFirmwareInfoMfgDate(t *testing.T) {
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

	seedChassisInfoEntry(t, stdb, "chassis 10", map[string]string{"model": "3HE18723AARA01"})
	seedChassisInfoEntry(t, stdb, "chassis 11", map[string]string{
		"model":            "3HE18723AARA01",
		"manufacture_date": "03/04/2024 10:00:00",
	})
	eepromTs := &db.TableSpec{Name: EEPROM_TBL}
	eepromKey := db.Key{Comp: []string{"0x25"}}
	if err := stdb.SetEntry(eepromTs, eepromKey, db.Value{Field: map[string]string{
		"Len": "19", "Name": "Manufacture Date", "Value": "01/20/2025 15:53:08",
	}}); err != nil {
		t.Fatalf("failed to seed EEPROM_INFO: %v", err)
	}
	t.Cleanup(func() { stdb.DeleteEntry(eepromTs, eepromKey) })

	cases := []struct {
		name          string
		chassis       string
		pType         PathType
		targetUriPath string
		want          string
	}{
		{"eeprom whole component", "chassis 10", AllPaths, COMP, "2025-01-20"},
		{"eeprom targeted leaf", "chassis 10", SingularPath, COMP_STATE_MFG_DATE, "2025-01-20"},
		{"chassis_info wins", "chassis 11", SingularPath, COMP_STATE_MFG_DATE, "2024-03-04"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillFirmwareInfo(comp, c.chassis, c.pType, c.targetUriPath, stdb, cfgdb); err != nil {
				t.Fatalf("fillFirmwareInfo() unexpected error: %v", err)
			}
			assertStrLeaf(t, comp.State.MfgDate, c.want, "state/mfg-date")
		})
	}

	t.Run("no date anywhere", func(t *testing.T) {
		if err := stdb.DeleteEntry(eepromTs, eepromKey); err != nil {
			t.Fatalf("failed to delete EEPROM_INFO: %v", err)
		}
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		err := fillFirmwareInfo(comp, "chassis 10", SingularPath, COMP_STATE_MFG_DATE, stdb, cfgdb)
		if _, ok := err.(tlerr.NotFoundError); !ok {
			t.Fatalf("fillFirmwareInfo() returned %T (%v), want tlerr.NotFoundError", err, err)
		}
	})
}

// Chassis mfg-name comes from the EEPROM_INFO Manufacturer, falls back to the
// Vendor Name, and is NotFound when neither has it.
func TestFillFirmwareInfoMfgName(t *testing.T) {
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

	const name = "chassis 12"
	seedChassisInfoEntry(t, stdb, name, map[string]string{"model": "3HE18723AARA01"})
	eepromTs := &db.TableSpec{Name: EEPROM_TBL}
	mfrKey := db.Key{Comp: []string{"0x2b"}}
	vendorKey := db.Key{Comp: []string{"0x2d"}}
	seedEeprom := func(key db.Key, attr, value string) {
		t.Helper()
		if err := stdb.SetEntry(eepromTs, key, db.Value{Field: map[string]string{
			"Len": "5", "Name": attr, "Value": value,
		}}); err != nil {
			t.Fatalf("failed to seed EEPROM_INFO: %v", err)
		}
		t.Cleanup(func() { stdb.DeleteEntry(eepromTs, key) })
	}
	fill := func(pType PathType, uri string) (*ocbinds.OpenconfigPlatform_Components_Component, error) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		return comp, fillFirmwareInfo(comp, name, pType, uri, stdb, cfgdb)
	}

	seedEeprom(vendorKey, "Vendor Name", "Nokia")
	t.Run("vendor name", func(t *testing.T) {
		comp, err := fill(SingularPath, COMP_STATE_MFG_NAME)
		if err != nil {
			t.Fatalf("fillFirmwareInfo() unexpected error: %v", err)
		}
		assertStrLeaf(t, comp.State.MfgName, "Nokia", "state/mfg-name")
	})

	seedEeprom(mfrKey, "Manufacturer", "Nokia Solutions")
	for _, c := range []struct {
		name  string
		pType PathType
		uri   string
	}{
		{"manufacturer whole component", AllPaths, COMP},
		{"manufacturer state", StatePaths, COMP_ST},
		{"manufacturer targeted leaf", SingularPath, COMP_STATE_MFG_NAME},
	} {
		t.Run(c.name, func(t *testing.T) {
			comp, err := fill(c.pType, c.uri)
			if err != nil {
				t.Fatalf("fillFirmwareInfo() unexpected error: %v", err)
			}
			assertStrLeaf(t, comp.State.MfgName, "Nokia Solutions", "state/mfg-name")
		})
	}

	t.Run("no manufacturer anywhere", func(t *testing.T) {
		stdb.DeleteEntry(eepromTs, mfrKey)
		stdb.DeleteEntry(eepromTs, vendorKey)
		if _, err := fill(SingularPath, COMP_STATE_MFG_NAME); !isNotFound(err) {
			t.Fatalf("fillFirmwareInfo() returned %T (%v), want tlerr.NotFoundError", err, err)
		}
	})
}

// A targeted chassis leaf with no value is NotFound, even when translib has
// pre-created it with its zero value, and the state container omits it.
func TestFillFirmwareInfoMissingLeafNotFound(t *testing.T) {
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

	const name = "chassis 13"
	seedChassisInfoEntry(t, stdb, name, map[string]string{"name": name})

	for _, uri := range []string{
		COMP_STATE_DESCR, COMP_STATE_FIRM_VER, COMP_STATE_HW_VER, COMP_STATE_PART_NO,
		COMP_STATE_SERIAL_NO, COMP_STATE_MODEL_NAME, COMP_STATE_MFG_NAME,
		COMP_STATE_LOCATION, COMP_STATE_TEMP,
	} {
		t.Run(uri, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			empty := ""
			comp.State.FirmwareVersion = &empty
			comp.State.Location = &empty
			err := fillFirmwareInfo(comp, name, SingularPath, uri, stdb, cfgdb)
			if !isNotFound(err) {
				t.Fatalf("fillFirmwareInfo(%s) returned %T (%v), want tlerr.NotFoundError", uri, err, err)
			}
		})
	}

	t.Run("state container", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillFirmwareInfo(comp, name, StatePaths, COMP_ST, stdb, cfgdb); err != nil {
			t.Fatalf("fillFirmwareInfo() unexpected error: %v", err)
		}
		assertStrLeafUnset(t, comp.State.FirmwareVersion, "state/firmware-version")
		assertStrLeafUnset(t, comp.State.MfgName, "state/mfg-name")
	})
}

// A targeted PSU leaf with no value is NotFound, even when translib has
// pre-created it as "".
func TestFillPsuMissingLeafNotFound(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedPsuInfoEntry(t, d, "PSU10", map[string]string{"presence": "true", "status": "true"})

	for _, uri := range []string{
		COMP_STATE_SERIAL_NO, COMP_STATE_DESCR, COMP_STATE_MODEL_NAME, COMP_STATE_HW_VER,
		COMP_STATE_MFG_NAME, COMP_STATE_PART_NO, COMP_STATE_LOCATION,
		COMP_STATE_INSTALL_POSITION, COMP_STATE_INSTALL_COMPONENT,
	} {
		t.Run(uri, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			empty := ""
			comp.State.MfgName = &empty
			err := fillPsuInfo(comp, "PSU10", false, false, uri, d)
			if !isNotFound(err) {
				t.Fatalf("fillPsuInfo(%s) returned %T (%v), want tlerr.NotFoundError", uri, err, err)
			}
		})
	}
}

// A targeted fan or fan drawer leaf with no value is NotFound, even when
// translib has pre-created it with its zero value, and the state container
// omits it.
func TestFillFanMissingLeafNotFound(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedFanInfoEntry(t, d, "Fan11", map[string]string{"presence": "True", "status": "True"})
	seedFanDrawerInfoEntry(t, d, "drawer11", map[string]string{"presence": "True"})

	for _, c := range []struct {
		name  string
		cType componentType
	}{{"Fan11", CompTypeFan}, {"drawer11", CompTypeFanTray}} {
		for _, uri := range []string{
			COMP_STATE_DESCR, COMP_STATE_MFG_NAME, COMP_STATE_FIRM_VER, COMP_STATE_SW_VER,
			COMP_STATE_ID, COMP_STATE_TEMP, COMP_STATE_HW_VER, COMP_STATE_PART_NO,
			COMP_STATE_SERIAL_NO, COMP_STATE_MODEL_NAME, COMP_STATE_INSTALL_COMPONENT,
			COMP_STATE_INSTALL_POSITION,
		} {
			t.Run(c.name+uri, func(t *testing.T) {
				comp := &ocbinds.OpenconfigPlatform_Components_Component{}
				ygot.BuildEmptyTree(comp)
				empty := ""
				comp.State.Description = &empty
				err := fillFanInfo(comp, c.name, uri, d, c.cType)
				if !isNotFound(err) {
					t.Fatalf("fillFanInfo(%s) returned %T (%v), want tlerr.NotFoundError", uri, err, err)
				}
			})
		}

		t.Run(c.name+" state container", func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillFanInfo(comp, c.name, COMP_ST, d, c.cType); err != nil {
				t.Fatalf("fillFanInfo() unexpected error: %v", err)
			}
			assertStrLeafUnset(t, comp.State.HardwareVersion, "state/hardware-version")
			assertStrLeafUnset(t, comp.State.Description, "state/description")
		})
	}
}

// An N/A hardware_revision on a fan or fan drawer is served as stored.
func TestFillFanHardwareVersionNotAvailableServed(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedFanInfoEntry(t, d, "Fan12", map[string]string{"presence": "True", "hardware_revision": "N/A"})
	seedFanDrawerInfoEntry(t, d, "drawer12", map[string]string{"presence": "True", "hardware_revision": "N/A"})

	for _, c := range []struct {
		name  string
		cType componentType
	}{{"Fan12", CompTypeFan}, {"drawer12", CompTypeFanTray}} {
		for _, uri := range []string{COMP_ST, COMP_STATE_HW_VER} {
			t.Run(c.name+uri, func(t *testing.T) {
				comp := &ocbinds.OpenconfigPlatform_Components_Component{}
				ygot.BuildEmptyTree(comp)
				if err := fillFanInfo(comp, c.name, uri, d, c.cType); err != nil {
					t.Fatalf("fillFanInfo(%s) unexpected error: %v", uri, err)
				}
				assertStrLeaf(t, comp.State.HardwareVersion, "N/A", "state/hardware-version")
			})
		}
	}
}

// A fan or fan drawer description is its model, served as stored.
func TestFillFanDescriptionFromModel(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedFanInfoEntry(t, d, "Fan13", map[string]string{"presence": "True", "model": "3HE12345AA"})
	seedFanDrawerInfoEntry(t, d, "drawer13", map[string]string{"presence": "True", "model": "N/A"})

	for _, c := range []struct {
		name  string
		cType componentType
		want  string
	}{{"Fan13", CompTypeFan, "3HE12345AA"}, {"drawer13", CompTypeFanTray, "N/A"}} {
		for _, uri := range []string{COMP_ST, COMP_STATE_DESCR} {
			t.Run(c.name+uri, func(t *testing.T) {
				comp := &ocbinds.OpenconfigPlatform_Components_Component{}
				ygot.BuildEmptyTree(comp)
				if err := fillFanInfo(comp, c.name, uri, d, c.cType); err != nil {
					t.Fatalf("fillFanInfo(%s) unexpected error: %v", uri, err)
				}
				assertStrLeaf(t, comp.State.Description, c.want, "state/description")
			})
		}
	}
}

// An integrated circuit's oper-status is ACTIVE.
func TestFillICInfoOperStatus(t *testing.T) {
	var dbs [db.MaxDB]*db.DB
	for _, n := range []db.DBNum{db.ConfigDB, db.CountersDB, db.AsicDB} {
		d, err := db.NewDB(getDBOptions(n))
		if err != nil {
			t.Fatalf("NewDB(%v) failed: %v", n, err)
		}
		defer d.DeleteDB()
		dbs[n] = d
	}
	for _, uri := range []string{COMP_ST, COMP_STATE_OPER_STATUS} {
		t.Run(uri, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillICInfo(comp, "integrated_circuit0", uri, dbs, nil); err != nil {
				t.Fatalf("fillICInfo(%s) unexpected error: %v", uri, err)
			}
			if comp.State.OperStatus != ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE {
				t.Errorf("state/oper-status == %v, want ACTIVE", comp.State.OperStatus)
			}
		})
	}
}

func isNotFound(err error) bool {
	_, ok := err.(tlerr.NotFoundError)
	return ok
}

// Chassis oper-status is ACTIVE. CHASSIS_INFO does not carry it.
func TestFillFirmwareInfoOperStatus(t *testing.T) {
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

	seedChassisInfoEntry(t, stdb, "chassis 1", map[string]string{
		"serial": "LL2503K1024", "model": "3HE18723AARA01", "revision": "0",
	})

	fill := func(name string, pType PathType, uri string) *ocbinds.OpenconfigPlatform_Components_Component {
		t.Helper()
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillFirmwareInfo(comp, name, pType, uri, stdb, cfgdb); err != nil {
			t.Fatalf("fillFirmwareInfo() unexpected error: %v", err)
		}
		return comp
	}
	active := ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE

	if got := fill("chassis 1", SingularPath, COMP_STATE_OPER_STATUS).State.OperStatus; got != active {
		t.Errorf("targeted oper-status = %v, want ACTIVE", got)
	}
	if got := fill("chassis 1", StatePaths, COMP_ST).State.OperStatus; got != active {
		t.Errorf("state oper-status = %v, want ACTIVE", got)
	}
}

// A temperature sensor that is present reports oper-status ACTIVE.
func TestFillTempSensorOperStatus(t *testing.T) {
	stdb, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer stdb.DeleteDB()

	name := "exhaust_sensor_1"
	seedTempInfoEntry(t, stdb, name, map[string]string{"temperature": "42.5"})
	active := ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE

	for _, c := range []struct {
		name  string
		pType PathType
		uri   string
	}{
		{"targeted leaf", SingularPath, COMP_STATE_OPER_STATUS},
		{"state container", StatePaths, COMP_ST},
	} {
		t.Run(c.name, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillTempSensorInfo(comp, name, c.pType, c.uri, stdb); err != nil {
				t.Fatalf("fillTempSensorInfo() unexpected error: %v", err)
			}
			if comp.State.OperStatus != active {
				t.Errorf("oper-status = %v, want ACTIVE", comp.State.OperStatus)
			}
		})
	}
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

// A transceiver serves its module temperature on the state/temperature
// container as well as on the instant leaf below it.
func TestFillXcvrTemperature(t *testing.T) {
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

	const name = "Ethernet72"
	seedXcvrInfoEntry(t, d, name, map[string]string{
		"type": "QSFP-DD Double Density 8X Pluggable Transceiver",
	})
	seedXcvrDomEntry(t, d, name, map[string]string{"temperature": "24.004"})

	for _, targetUriPath := range []string{COMP_STATE_TEMP_CTR, COMP_STATE_TEMP} {
		t.Run(targetUriPath, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			if err := fillXcvrInfo(comp, name, false, "", targetUriPath, dbs); err != nil {
				t.Fatalf("fillXcvrInfo() unexpected error: %v", err)
			}
			got := comp.State.Temperature.Instant
			if got == nil {
				t.Fatalf("state/temperature/instant is nil, want 24.004")
			}
			if *got != 24.004 {
				t.Errorf("state/temperature/instant = %v, want 24.004", *got)
			}
		})
	}
}

// A targeted leaf that an SFF module has no value for is NotFound, even when
// translib has pre-created it with its zero value.
func TestFillXcvrMissingLeafNotFound(t *testing.T) {
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

	const name = "Ethernet76"
	seedXcvrInfoEntry(t, d, name, map[string]string{
		"type":       "SFP/SFP+/SFP28",
		"vendor_rev": "A   ",
	})
	seedXcvrDomEntry(t, d, name, map[string]string{
		"rx1power": "-inf", "rx2power": "N/A", "rx3power": "N/A", "rx4power": "N/A",
	})

	for _, targetUriPath := range []string{
		COMP_STATE_FIRM_VER, COMP_STATE_HW_VER, XCVR_INPUT_POWER_INSTANT,
		COMP_STATE_MODEL_NAME, COMP_STATE_SW_VER, COMP_STATE_LOCATION,
	} {
		t.Run(targetUriPath, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			ygot.BuildEmptyTree(comp.Transceiver)
			ygot.BuildEmptyTree(comp.Transceiver.State)
			empty, zero := "", 0.0
			comp.State.FirmwareVersion = &empty
			comp.State.HardwareVersion = &empty
			comp.State.ModelName = &empty
			comp.State.SoftwareVersion = &empty
			comp.State.Location = &empty
			comp.Transceiver.State.InputPower.Instant = &zero

			err := fillXcvrInfo(comp, name, false, "", targetUriPath, dbs)
			if _, ok := err.(tlerr.NotFoundError); !ok {
				t.Fatalf("fillXcvrInfo(%s) returned %T (%v), want tlerr.NotFoundError", targetUriPath, err, err)
			}
		})
	}
}

// vendor-rev carries TRANSCEIVER_INFO vendor_rev with its padding trimmed, and
// hardware-version carries hardware_rev only.
func TestFillXcvrVendorRev(t *testing.T) {
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

	const cmisName = "Ethernet80"
	seedXcvrInfoEntry(t, d, cmisName, map[string]string{
		"type":         "QSFP-DD Double Density 8X Pluggable Transceiver",
		"hardware_rev": "2.12",
		"vendor_rev":   "1A",
	})
	seedXcvrDomEntry(t, d, cmisName, map[string]string{"temperature": "35.0"})

	const sffName = "Ethernet84"
	seedXcvrInfoEntry(t, d, sffName, map[string]string{
		"type":       "SFP/SFP+/SFP28",
		"vendor_rev": "A   ",
	})
	seedXcvrDomEntry(t, d, sffName, map[string]string{"temperature": "30.0"})

	t.Run("cmis module", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillXcvrInfo(comp, cmisName, true, "", COMP, dbs); err != nil {
			t.Fatalf("fillXcvrInfo() unexpected error: %v", err)
		}
		assertStrLeaf(t, comp.State.HardwareVersion, "2.12", "state/hardware-version")
		assertStrLeaf(t, comp.Transceiver.State.VendorRev, "1A", "transceiver/state/vendor-rev")
	})

	t.Run("sff module", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillXcvrInfo(comp, sffName, true, "", COMP, dbs); err != nil {
			t.Fatalf("fillXcvrInfo() unexpected error: %v", err)
		}
		assertStrLeafUnset(t, comp.State.HardwareVersion, "state/hardware-version")
		assertStrLeaf(t, comp.Transceiver.State.VendorRev, "A", "transceiver/state/vendor-rev")
	})

	t.Run("targeted vendor-rev", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillXcvrInfo(comp, sffName, false, "", XCVR_VENDOR_REV, dbs); err != nil {
			t.Fatalf("fillXcvrInfo() unexpected error: %v", err)
		}
		assertStrLeaf(t, comp.Transceiver.State.VendorRev, "A", "transceiver/state/vendor-rev")
	})
}

// An N/A stored for a transceiver string leaf is served as stored.
func TestFillXcvrNotAvailableServed(t *testing.T) {
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

	const name = "Ethernet88"
	seedXcvrInfoEntry(t, d, name, map[string]string{
		"type":         "N/A",
		"hardware_rev": "N/A",
		"vendor_rev":   "N/A",
	})
	seedXcvrFirmwareEntry(t, d, name, map[string]string{"active_firmware": "N/A"})
	seedXcvrDomEntry(t, d, name, map[string]string{"temperature": "30.0"})

	check := func(t *testing.T, comp *ocbinds.OpenconfigPlatform_Components_Component, targetUriPath string) {
		switch targetUriPath {
		case COMP, COMP_STATE_HW_VER:
			assertStrLeaf(t, comp.State.HardwareVersion, "N/A", "state/hardware-version")
		}
		switch targetUriPath {
		case COMP, COMP_STATE_FIRM_VER:
			assertStrLeaf(t, comp.State.FirmwareVersion, "N/A", "state/firmware-version")
		}
		switch targetUriPath {
		case COMP, COMP_STATE_DESCR:
			assertStrLeaf(t, comp.State.Description, "N/A", "state/description")
		}
		switch targetUriPath {
		case COMP, XCVR_VENDOR_REV:
			assertStrLeaf(t, comp.Transceiver.State.VendorRev, "N/A", "transceiver/state/vendor-rev")
		}
	}

	for _, targetUriPath := range []string{
		COMP, COMP_STATE_HW_VER, COMP_STATE_FIRM_VER, COMP_STATE_DESCR, XCVR_VENDOR_REV,
	} {
		t.Run(targetUriPath, func(t *testing.T) {
			comp := &ocbinds.OpenconfigPlatform_Components_Component{}
			ygot.BuildEmptyTree(comp)
			all := targetUriPath == COMP
			if err := fillXcvrInfo(comp, name, all, "", targetUriPath, dbs); err != nil {
				t.Fatalf("fillXcvrInfo(%s) unexpected error: %v", targetUriPath, err)
			}
			check(t, comp, targetUriPath)
		})
	}
}

// PSU_INFO.revision carries the PSU hardware revision on both the
// whole-component and the targeted leaf path, and an N/A revision is served
// as stored.
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

	t.Run("N/A revision", func(t *testing.T) {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		if err := fillPsuInfo(comp, "PSU8", true, false, COMP, d); err != nil {
			t.Fatalf("fillPsuInfo() unexpected error: %v", err)
		}
		assertStrLeaf(t, comp.State.HardwareVersion, "N/A", "state/hardware-version")
	})
}

// A targeted mfg-date on a PSU, fan or fan drawer with no date is NotFound,
// even when translib has pre-created the leaf as "".
func TestGetSysComponentsMfgDateNotFound(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedPsuInfoEntry(t, d, "PSU9", map[string]string{"presence": "true", "status": "true"})
	seedFanInfoEntry(t, d, "Fan9", map[string]string{"presence": "True", "status": "True"})
	seedFanDrawerInfoEntry(t, d, "drawer9", map[string]string{"presence": "True", "mfg_date": "N/A"})
	defer d.DeleteEntry(&db.TableSpec{Name: PSU_TBL}, db.Key{Comp: []string{"PSU9"}})
	defer d.DeleteEntry(&db.TableSpec{Name: FAN_TBL}, db.Key{Comp: []string{"Fan9"}})
	defer d.DeleteEntry(&db.TableSpec{Name: FAN_TRAY_TBL}, db.Key{Comp: []string{"drawer9"}})

	for _, name := range []string{"PSU9", "Fan9", "drawer9"} {
		t.Run(name, func(t *testing.T) {
			device := &ocbinds.Device{}
			ygot.BuildEmptyTree(device)
			comp, err := device.Components.NewComponent(name)
			if err != nil {
				t.Fatalf("creating the component failed: %v", err)
			}
			ygot.BuildEmptyTree(comp)
			empty := ""
			comp.State.MfgDate = &empty

			var inParams XfmrParams
			inParams.dbs[db.StateDB] = d
			err = getSysComponents(device.Components, COMP_STATE_MFG_DATE, inParams, name, "")
			if _, ok := err.(tlerr.NotFoundError); !ok {
				t.Fatalf("getSysComponents(%q) returned %T (%v), want tlerr.NotFoundError", name, err, err)
			}
		})
	}
}

// A fan mfg-date in MM/DD/YYYY form is served as YYYY-MM-DD on the targeted leaf.
func TestFillFanMfgDate(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedFanInfoEntry(t, d, "Fan10", map[string]string{"presence": "True", "mfg_date": "01/20/2025 15:53:08"})

	comp := &ocbinds.OpenconfigPlatform_Components_Component{}
	ygot.BuildEmptyTree(comp)
	if err := fillFanInfo(comp, "Fan10", COMP_STATE_MFG_DATE, d, CompTypeFan); err != nil {
		t.Fatalf("fillFanInfo() unexpected error: %v", err)
	}
	assertStrLeaf(t, comp.State.MfgDate, "2025-01-20", "state/mfg-date")
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

func seedStorageInfoEntry(t *testing.T, d *db.DB, name string, fields map[string]string) {
	t.Helper()
	if err := d.SetEntry(&db.TableSpec{Name: STORAGE_INFO}, db.Key{Comp: []string{name}}, db.Value{Field: fields}); err != nil {
		t.Fatalf("failed to seed STORAGE_INFO for %s: %v", name, err)
	}
}

func assertUint8Leaf(t *testing.T, got *uint8, want uint8, leaf string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %d", leaf, want)
	}
	if *got != want {
		t.Errorf("%s = %d, want %d", leaf, *got, want)
	}
}

// stormond's STORAGE_INFO rows are served as STORAGE components under
// "chassis 1", and a targeted leaf with no value is NotFound.
func TestFillStorageInfo(t *testing.T) {
	d, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		t.Fatal("NewDB failed")
	}
	defer d.DeleteDB()
	seedStorageInfoEntry(t, d, "sdz", map[string]string{
		"device_model": "M.2 (S42) 3IE7",
		"serial":       "YCA12404080220032",
		"firmware":     "S23C27i",
		"health":       "97.4",
		"temperature":  "28",
	})
	seedStorageInfoEntry(t, d, "mmcblk9", map[string]string{
		"device_model": "G1M15M",
		"serial":       "0x1ba2a988",
		"health":       "100.0",
		"temperature":  "N/A",
	})
	seedStorageInfoEntry(t, d, STORAGE_FSSTATS_KEY, map[string]string{"successful_sync_time": "2026-10-07 03:20:45"})
	defer d.DeleteEntry(&db.TableSpec{Name: STORAGE_INFO}, db.Key{Comp: []string{"sdz"}})
	defer d.DeleteEntry(&db.TableSpec{Name: STORAGE_INFO}, db.Key{Comp: []string{"mmcblk9"}})
	defer d.DeleteEntry(&db.TableSpec{Name: STORAGE_INFO}, db.Key{Comp: []string{STORAGE_FSSTATS_KEY}})

	newComp := func() *ocbinds.OpenconfigPlatform_Components_Component {
		comp := &ocbinds.OpenconfigPlatform_Components_Component{}
		ygot.BuildEmptyTree(comp)
		return comp
	}

	t.Run("whole component", func(t *testing.T) {
		comp := newComp()
		if err := fillStorageInfo(comp, "sdz", AllPaths, COMP, d); err != nil {
			t.Fatalf("fillStorageInfo() unexpected error: %v", err)
		}
		assertStrLeaf(t, comp.State.Name, "sdz", "state/name")
		want, _ := comp.State.To_OpenconfigPlatform_Components_Component_State_Type_Union(
			ocbinds.OpenconfigPlatformTypes_OPENCONFIG_HARDWARE_COMPONENT_STORAGE)
		if !reflect.DeepEqual(comp.State.Type, want) {
			t.Errorf("state/type == %v, want %v", comp.State.Type, want)
		}
		assertStrLeaf(t, comp.State.Parent, CHASSIS_COMP_NAME, "state/parent")
		assertStrLeaf(t, comp.State.SerialNo, "YCA12404080220032", "state/serial-no")
		assertStrLeaf(t, comp.State.ModelName, "M.2 (S42) 3IE7", "state/model-name")
		assertStrLeaf(t, comp.State.FirmwareVersion, "S23C27i", "state/firmware-version")
		if comp.State.Temperature.Instant == nil || *comp.State.Temperature.Instant != 28 {
			t.Errorf("state/temperature/instant = %v, want 28", comp.State.Temperature.Instant)
		}
		assertUint8Leaf(t, comp.Storage.State.Counters.LifeLeft, 97, "storage/state/counters/life-left")
		if comp.State.OperStatus != ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE {
			t.Errorf("state/oper-status == %v, want ACTIVE", comp.State.OperStatus)
		}
	})

	t.Run("targeted oper-status", func(t *testing.T) {
		comp := newComp()
		if err := fillStorageInfo(comp, "mmcblk9", SingularPath, COMP_STATE_OPER_STATUS, d); err != nil {
			t.Fatalf("fillStorageInfo() unexpected error: %v", err)
		}
		if comp.State.OperStatus != ocbinds.OpenconfigPlatformTypes_COMPONENT_OPER_STATUS_ACTIVE {
			t.Errorf("state/oper-status == %v, want ACTIVE", comp.State.OperStatus)
		}
	})

	t.Run("missing and N/A values", func(t *testing.T) {
		comp := newComp()
		if err := fillStorageInfo(comp, "mmcblk9", AllPaths, COMP, d); err != nil {
			t.Fatalf("fillStorageInfo() unexpected error: %v", err)
		}
		assertStrLeafUnset(t, comp.State.FirmwareVersion, "state/firmware-version")
		if comp.State.Temperature.Instant != nil {
			t.Errorf("state/temperature/instant = %v, want unset", *comp.State.Temperature.Instant)
		}
		assertUint8Leaf(t, comp.Storage.State.Counters.LifeLeft, 100, "storage/state/counters/life-left")
	})

	t.Run("targeted life-left", func(t *testing.T) {
		comp := newComp()
		if err := fillStorageInfo(comp, "sdz", SingularPath, COMP_STORAGE_LIFE_LEFT, d); err != nil {
			t.Fatalf("fillStorageInfo() unexpected error: %v", err)
		}
		assertUint8Leaf(t, comp.Storage.State.Counters.LifeLeft, 97, "storage/state/counters/life-left")
	})

	for _, uri := range []string{COMP_STATE_FIRM_VER, COMP_STATE_TEMP, COMP_STATE_MFG_NAME, COMP_STATE_PART_NO} {
		t.Run("targeted "+uri, func(t *testing.T) {
			if err := fillStorageInfo(newComp(), "mmcblk9", SingularPath, uri, d); !isNotFound(err) {
				t.Fatalf("fillStorageInfo(%s) returned %T (%v), want tlerr.NotFoundError", uri, err, err)
			}
		})
	}

	t.Run("sync bookkeeping row", func(t *testing.T) {
		if err := fillStorageInfo(newComp(), STORAGE_FSSTATS_KEY, AllPaths, COMP, d); !isNotFound(err) {
			t.Fatalf("fillStorageInfo(%s) returned %T (%v), want tlerr.NotFoundError", STORAGE_FSSTATS_KEY, err, err)
		}
	})

	t.Run("enumeration", func(t *testing.T) {
		device := &ocbinds.Device{}
		ygot.BuildEmptyTree(device)
		var inParams XfmrParams
		inParams.dbs[db.StateDB] = d
		createCompAndFuncCall(device.Components, COMP, CompTypeStorage, inParams, STORAGE_INFO, STORAGE_NAME_PREFIX)
		for _, name := range []string{"sdz", "mmcblk9"} {
			if device.Components.Component[name] == nil {
				t.Errorf("component %s not created", name)
			}
		}
		if device.Components.Component[STORAGE_FSSTATS_KEY] != nil {
			t.Errorf("component %s created, want skipped", STORAGE_FSSTATS_KEY)
		}
	})
}
