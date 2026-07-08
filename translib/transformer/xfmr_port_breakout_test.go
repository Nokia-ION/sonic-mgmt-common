package transformer

import (
	"testing"

	db "github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/platform"
	"github.com/openconfig/ygot/ygot"
)

func TestPopulateChannelGroupsForSameSpeed(t *testing.T) {
	if _, err := populateChannelGroupsForSameSpeed("bad port name", "1x400G"); err == nil {
		t.Errorf("Calling populateChannelGroupsForSameSpeed with a bad port name didn't return an error")
	}
}

func TestPopulateBrkoutMode(t *testing.T) {
	if _, err := populateBrkoutMode(nil, "somePort", "bad IF name"); err == nil {
		t.Errorf("Calling populateBrkoutMode with a bad IF name didn't return an error")
	}
}

func TestGenerateDeployedPortsListWithInvalidMode(t *testing.T) {
	var device ygot.GoStruct = &ocbinds.Device{Interfaces: &ocbinds.OpenconfigInterfaces_Interfaces{}}
	_, err := generatePortInConfigPortsList("Ethernet1/1/1", "1x100G", (device).(*ocbinds.Device).Interfaces, nil, REPLACE)
	if err == nil {
		t.Errorf("Expect error with invalid to_mode")
	}

}

func TestHaveLaneSetsChangedWithInvalidMode(t *testing.T) {
	_, err := haveLaneSetsChanged("Ethernet1/1/1", "1x100G", "2X200G")
	if err == nil {
		t.Errorf("Expect error with invalid to_mode")
	}

	_, err = haveLaneSetsChanged("Ethernet1/1/1", "2x200G", "1X100G")
	if err == nil {
		t.Errorf("Expect error with invalid from_mode")
	}

}

func TestSanitizeMode(t *testing.T) {
	to_mode_per_platform, _ := sanitizeMode("2x100G4_1x200G4")
	if to_mode_per_platform != "2x100G(4)+1x200G(4)" {
		t.Errorf("Unexpected to_mode_per_platform %s", to_mode_per_platform)
	}

	to_mode_per_sonic, _ := sanitizeMode("2x100G(4)+1x200G(4)")
	if to_mode_per_sonic != "2x100G4_1x200G4" {
		t.Errorf("Unexpected to_mode_per_sonic %s", to_mode_per_sonic)
	}
}

func TestGetRefCount(t *testing.T) {
	configDb, _ := db.NewDB(db.Options{
		DBNo:               db.ConfigDB,
		InitIndicator:      "CONFIG_DB_INITIALIZED",
		TableNameSeparator: "|",
		KeySeparator:       "|",
	})
	defer configDb.DeleteDB()
	stateDb, _ := db.NewDB(db.Options{
		DBNo:               db.StateDB,
		InitIndicator:      "STATE_DB_INITIALIZED",
		TableNameSeparator: "|",
		KeySeparator:       "|",
	})

	// Test case 1: Port not found in Config DB
	refCount, err := getRefCount(configDb, stateDb, "Ethernet1/0/1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if refCount != 0 {
		t.Errorf("Expected refCount to be 0, got %d", refCount)
	}

	// Test case 2: Port found in Config DB but not in State DB
	configDb.SetEntry(&db.TableSpec{Name: "PORT"}, db.Key{Comp: []string{"Ethernet1/0/1"}}, db.Value{Field: map[string]string{"alias": "eth0"}})
	refCount, err = getRefCount(configDb, stateDb, "Ethernet1/0/1")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if refCount != -1 {
		t.Errorf("Expected refCount to be -1, got %d", refCount)
	}

	// Test case 3: Port found in both Config DB and State DB
	stateDb.SetEntry(&db.TableSpec{Name: "PORT_TABLE"}, db.Key{Comp: []string{"Ethernet1/0/1"}}, db.Value{Field: map[string]string{"ref_count": "5"}})
	refCount, err = getRefCount(configDb, stateDb, "Ethernet1/0/1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if refCount != 5 {
		t.Errorf("Expected refCount to be 5, got %d", refCount)
	}

	// Test case 4: Invalid ref_count value in State DB
	stateDb.SetEntry(&db.TableSpec{Name: "PORT_TABLE"}, db.Key{Comp: []string{"Ethernet1/0/1"}}, db.Value{Field: map[string]string{"ref_count": "abc"}})
	refCount, err = getRefCount(configDb, stateDb, "Ethernet1/0/1")
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
	if refCount != -1 {
		t.Errorf("Expected refCount to be -1, got %d", refCount)
	}
}

func TestFoundFlowProgramming(t *testing.T) {
	configDb, _ := db.NewDB(db.Options{
		DBNo:               db.ConfigDB,
		InitIndicator:      "CONFIG_DB_INITIALIZED",
		TableNameSeparator: "|",
		KeySeparator:       "|",
	})
	defer configDb.DeleteDB()
	appStateDb, _ := db.NewDB(db.Options{
		DBNo:               db.ApplDB,
		InitIndicator:      "APPL_DB_INITIALIZED",
		TableNameSeparator: "|",
		KeySeparator:       "|",
	})
	stateDb, _ := db.NewDB(db.Options{
		DBNo:               db.StateDB,
		InitIndicator:      "STATE_DB_INITIALIZED",
		TableNameSeparator: "|",
		KeySeparator:       "|",
	})

	ports := []platform.InterfaceProperties{
		{Name: "Ethernet1/0/1"},
	}

	// ref_count not found
	if err := foundFlowProgramming(configDb, appStateDb, stateDb, ports); err == nil {
		t.Errorf("Expected error, got nil")
	}

	// BUFFER_PG table entry found with different ref_count
	stateDb.SetEntry(&db.TableSpec{Name: "PORT_TABLE"}, db.Key{Comp: []string{"Ethernet1/0/1"}}, db.Value{Field: map[string]string{"ref_count": "2"}})
	if err := foundFlowProgramming(configDb, appStateDb, stateDb, ports); err == nil {
		t.Errorf("Expected error, got nil")
	}

	// BUFFER_PG table entry found with same ref_count
	appStateDb.SetEntry(&db.TableSpec{Name: "BUFFER_PG_TABLE"}, db.Key{Comp: []string{"Ethernet1/0/1", "0"}}, db.Value{Field: map[string]string{"NULL": "NULL"}})
	stateDb.SetEntry(&db.TableSpec{Name: "PORT_TABLE"}, db.Key{Comp: []string{"Ethernet1/0/1"}}, db.Value{Field: map[string]string{"ref_count": "1"}})
	if err := foundFlowProgramming(configDb, appStateDb, stateDb, ports); err != nil {
		t.Errorf("Expected nil, got error = %v", err)
	}
}

func TestRemovePorts(t *testing.T) {
	configDb, _ := db.NewDB(db.Options{
		DBNo:               db.ConfigDB,
		InitIndicator:      "CONFIG_DB_INITIALIZED",
		TableNameSeparator: "|",
		KeySeparator:       "|",
	})
	defer configDb.DeleteDB()
	appStateDb, _ := db.NewDB(db.Options{
		DBNo:               db.ApplDB,
		InitIndicator:      "APPL_DB_INITIALIZED",
		TableNameSeparator: "|",
		KeySeparator:       "|",
	})

	ports_i := []platform.InterfaceProperties{
		{Name: "Ethernet1/0/5"},
	}
	ports_n := []platform.InterfaceProperties{
		{Name: "Ethernet1/0/1"},
	}

	configDb.SetEntry(&db.TableSpec{Name: "PORT_QOS_MAP"}, db.Key{Comp: []string{"Ethernet1/0/5"}}, db.Value{Field: map[string]string{"NULL": "NULL"}})
	configDb.SetEntry(&db.TableSpec{Name: "QUEUE"}, db.Key{Comp: []string{"Ethernet1/0/5", "0"}}, db.Value{Field: map[string]string{"NULL": "NULL"}})
	configDb.SetEntry(&db.TableSpec{Name: "BUFFER_QUEUE"}, db.Key{Comp: []string{"Ethernet1/0/5", "0"}}, db.Value{Field: map[string]string{"NULL": "NULL"}})
	configDb.SetEntry(&db.TableSpec{Name: "BUFFER_PG"}, db.Key{Comp: []string{"Ethernet1/0/5", "0"}}, db.Value{Field: map[string]string{"NULL": "NULL"}})

	delMap, err := removePorts(configDb, appStateDb, ports_i, ports_n)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	configDBMap := delMap[db.ConfigDB]
	if _, ok := configDBMap["PORT_QOS_MAP"]["Ethernet1/0/5"]; !ok {
		t.Errorf("Expected port qos map Ethernet1/0/5 to be in delMap")
	}
	if _, ok := configDBMap["QUEUE"]["Ethernet1/0/5|0"]; !ok {
		t.Errorf("Expected queue Ethernet1/0/5|0 to be in delMap")
	}
	if _, ok := configDBMap["BUFFER_QUEUE"]["Ethernet1/0/5|0"]; !ok {
		t.Errorf("Expected buffer queue Ethernet1/0/5|0 to be in delMap")
	}
	if _, ok := configDBMap["BUFFER_PG"]["Ethernet1/0/5|0"]; !ok {
		t.Errorf("Expected buffer pg Ethernet1/0/5|0 to be in delMap")
	}
}
