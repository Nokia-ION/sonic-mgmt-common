package transformer

import (
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
