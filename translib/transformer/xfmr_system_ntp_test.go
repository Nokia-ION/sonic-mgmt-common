package transformer

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

type mockDb struct {
	db.DB
	getEntry func(table *db.TableSpec, key db.Key) (db.Value, error)
	getKeys  func(table *db.TableSpec) ([]db.Key, error)
}

func TestYangToDb_NTP_Servers(t *testing.T) {
	const testIP1 = "10.1.1.1"
	const testIP2 = "10.1.1.2"

	originalDbGetKeys := dbGetKeys
	defer func() { dbGetKeys = originalDbGetKeys }()

	tests := []struct {
		name       string
		op         Operation
		uri        string
		key        string
		ygRoot     func() *ocbinds.Device
		dbState    map[string]db.Value // Initial state of DB keys
		dbErr      error
		wantMap    map[string]map[string]db.Value
		wantSubDel map[string]map[string]db.Value // Expected deletions in subOpDataMap
		wantErr    bool
	}{
		// --- LIST LEVEL TESTS ---
		{
			name: "List: REPLACE (Add New, Delete Missing)",
			op:   REPLACE,
			uri:  "/openconfig-system:system/ntp/servers",
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Ntp: &ocbinds.OpenconfigSystem_System_Ntp{
						Servers: &ocbinds.OpenconfigSystem_System_Ntp_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{},
						},
					},
				}
				// Input has IP1
				s1, _ := d.System.Ntp.Servers.NewServer(testIP1)
				s1.Config = &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server_Config{Address: ygot.String(testIP1)}
				return d
			},
			// DB has IP2
			dbState: map[string]db.Value{testIP2: {Field: map[string]string{nullKey: nullKey}}},
			// Result: Add IP1, Delete IP2
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
			wantSubDel: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP2: {}, // Deleted via subOpMap
				},
			},
		},
		{
			name: "List: REPLACE (Empty Payload = Clear All)",
			op:   REPLACE,
			uri:  "/openconfig-system:system/ntp/servers",
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Ntp: &ocbinds.OpenconfigSystem_System_Ntp{
						Servers: &ocbinds.OpenconfigSystem_System_Ntp_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{},
						},
					},
				}
				// Input is empty (No servers)
				return d
			},
			// DB has IP1 and IP2
			dbState: map[string]db.Value{
				testIP1: {Field: map[string]string{nullKey: nullKey}},
				testIP2: {Field: map[string]string{nullKey: nullKey}},
			},
			// Result: Delete both
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {},
			},
			wantSubDel: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {}, // Deleted via subOpMap
					testIP2: {}, // Deleted via subOpMap
				},
			},
		},
		{
			name:    "List: DELETE (Clear All)",
			op:      DELETE,
			uri:     "/openconfig-system:system/ntp/servers",
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			dbState: map[string]db.Value{testIP1: {Field: map[string]string{nullKey: nullKey}}},
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {}, // Delete
				},
			},
		},
		{
			name: "List: UPDATE (Add Only)",
			op:   UPDATE,
			uri:  "/openconfig-system:system/ntp/servers",
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Ntp: &ocbinds.OpenconfigSystem_System_Ntp{
						Servers: &ocbinds.OpenconfigSystem_System_Ntp_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{},
						},
					},
				}
				s1, _ := d.System.Ntp.Servers.NewServer(testIP1)
				s1.Config = &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server_Config{Address: ygot.String(testIP1)}
				return d
			},
			dbState: map[string]db.Value{testIP2: {Field: map[string]string{nullKey: nullKey}}},
			// Result: Add IP1, IP2 remains untouched
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
		},

		// --- SINGLE SERVER LEVEL TESTS ---
		{
			name: "Item: DELETE Single Server",
			op:   DELETE,
			uri:  "/openconfig-system:system/ntp/servers/server[address=" + testIP1 + "]",
			key:  testIP1,
			ygRoot: func() *ocbinds.Device {
				return &ocbinds.Device{}
			},
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {}, // Delete specific key
				},
			},
		},
		{
			name:    "Item: DELETE Single Server Sub-Element (Config pass-through)",
			op:      DELETE,
			uri:     "/openconfig-system:system/ntp/servers/server[address=" + testIP1 + "]/config/address",
			key:     testIP1,
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			wantErr: true, // Cannot delete a key leaf directly
		},
		{
			name: "Item: REPLACE Single Server",
			op:   REPLACE,
			uri:  "/openconfig-system:system/ntp/servers/server[address=" + testIP1 + "]",
			key:  testIP1,
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Ntp: &ocbinds.OpenconfigSystem_System_Ntp{
						Servers: &ocbinds.OpenconfigSystem_System_Ntp_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{},
						},
					},
				}
				s1, _ := d.System.Ntp.Servers.NewServer(testIP1)
				s1.Config = &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server_Config{Address: ygot.String(testIP1)}
				return d
			},
			// DB has IP2, should NOT be deleted
			dbState: map[string]db.Value{testIP2: {}},
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
		},
		{
			name: "Item: REPLACE Single Server Empty Payload (DELETE)",
			op:   REPLACE,
			uri:  "/openconfig-system:system/ntp/servers/server[address=" + testIP1 + "]",
			key:  testIP1,
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Ntp: &ocbinds.OpenconfigSystem_System_Ntp{
						Servers: &ocbinds.OpenconfigSystem_System_Ntp_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{},
							// IP1 not in payload
						},
					},
				}
				return d
			},
			dbState: map[string]db.Value{testIP1: {Field: map[string]string{nullKey: nullKey}}},
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {}, // Delete
				},
			},
		},
		{
			name:    "Item: REPLACE Single Server Sub-Element (Config pass-through)",
			op:      REPLACE,
			uri:     "/openconfig-system:system/ntp/servers/server[address=" + testIP1 + "]/config/address",
			key:     testIP1,
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			wantErr: false,
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
		},
		{
			name: "Item: UPDATE Single Server Config Leaf",
			op:   UPDATE,
			uri:  "/openconfig-system:system/ntp/servers/server[address=" + testIP1 + "]/config/address",
			key:  testIP1,
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Ntp: &ocbinds.OpenconfigSystem_System_Ntp{
						Servers: &ocbinds.OpenconfigSystem_System_Ntp_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{},
						},
					},
				}
				s1, _ := d.System.Ntp.Servers.NewServer(testIP1)
				s1.Config = &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server_Config{Address: ygot.String(testIP1)}
				return d
			},
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
		},
		{
			name:    "List: DELETE Item Missing Key (Acts as List)",
			op:      DELETE,
			uri:     "/openconfig-system:system/ntp/servers/server", // Missing key predicate
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			dbState: map[string]db.Value{testIP1: {Field: map[string]string{nullKey: nullKey}}},
			wantErr: false, // List Operation
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {}, // Delete
				},
			},
		},
		{
			name:    "List: REPLACE Item Missing Key (Acts as List)",
			op:      REPLACE,
			uri:     "/openconfig-system:system/ntp/servers/server", // Missing key predicate
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			dbState: map[string]db.Value{testIP1: {Field: map[string]string{nullKey: nullKey}}},
			wantErr: false, // List Operation
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {},
			},
			wantSubDel: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {}, // Deleted via subOpMap
				},
			},
		},
		{
			name: "List: UPDATE Item Missing Key (Acts as List)",
			op:   UPDATE,
			uri:  "/openconfig-system:system/ntp/servers/server", // Missing key predicate
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Ntp: &ocbinds.OpenconfigSystem_System_Ntp{
						Servers: &ocbinds.OpenconfigSystem_System_Ntp_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{},
						},
					},
				}
				s1, _ := d.System.Ntp.Servers.NewServer(testIP1)
				s1.Config = &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server_Config{Address: ygot.String(testIP1)}
				return d
			},
			wantErr: false, // List Operation
			wantMap: map[string]map[string]db.Value{
				NTP_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
		},
		{
			name:    "Error: DB Fail on List DELETE",
			op:      DELETE,
			uri:     "/openconfig-system:system/ntp/servers",
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			dbErr:   errors.New("db connection failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock DB keys
			dbGetKeys = func(d *db.DB, table *db.TableSpec) ([]db.Key, error) {
				if tt.dbErr != nil {
					return nil, tt.dbErr
				}
				var keys []db.Key
				for k := range tt.dbState {
					keys = append(keys, db.Key{Comp: []string{k}})
				}
				return keys, nil
			}

			dbs := [db.MaxDB]*db.DB{db.ConfigDB: &db.DB{}}
			ygRoot := ygot.GoStruct(tt.ygRoot())
			inParams := XfmrParams{
				dbs:          dbs,
				ygRoot:       &ygRoot,
				oper:         tt.op,
				uri:          tt.uri,
				key:          tt.key,
				subOpDataMap: make(map[Operation]*map[db.DBNum]map[string]map[string]db.Value),
			}

			gotMap, err := YangToDb_ntp_servers_list_xfmr(inParams)

			if (err != nil) != tt.wantErr {
				t.Errorf("YangToDb_ntp_servers_list_xfmr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(gotMap, tt.wantMap) {
					t.Errorf("YangToDb_ntp_servers_list_xfmr() gotMap = %v, wantMap %v", gotMap, tt.wantMap)
				}

				// Verify our new updateSubOpDataMap DELETE logic
				if tt.wantSubDel != nil {
					delOps := inParams.subOpDataMap[DELETE]
					if delOps == nil || (*delOps)[db.ConfigDB] == nil {
						t.Errorf("Expected DELETE sub operations for orphaned keys, got none")
					} else {
						gotSubDel := (*delOps)[db.ConfigDB]
						if !reflect.DeepEqual(gotSubDel, tt.wantSubDel) {
							t.Errorf("SubOpMap DELETE got = %v, want %v", gotSubDel, tt.wantSubDel)
						}
					}
				}
			}
		})
	}
}

func TestDbToYang_NTP_Servers(t *testing.T) {
	// CONFIG_DB only contains keys, no fields.
	cfgDbKeys := []string{"1.1.1.1", "3.3.3.3"}
	stateDbData := map[string]db.Value{
		"1.1.1.1": {Field: map[string]string{"stratum": "2", "reach": "377", "refId": "GPS"}},
		"2.2.2.2": {Field: map[string]string{"stratum": "3", "reach": "300", "refId": "PPS"}},
	}

	// Mock DB Implementations
	mockCfgDb := &mockDb{
		getKeys: func(table *db.TableSpec) ([]db.Key, error) {
			keys := []db.Key{}
			for _, k := range cfgDbKeys {
				keys = append(keys, db.Key{Comp: []string{k}})
			}
			return keys, nil
		},
		getEntry: func(table *db.TableSpec, key db.Key) (db.Value, error) {
			for _, k := range cfgDbKeys {
				if k == key.Comp[0] {
					return db.Value{Field: map[string]string{}}, nil // Entry exists, no fields
				}
			}
			return db.Value{}, tlerr.NotFoundError{Format: "not found in mockCfgDb"}
		},
	}

	mockStateDb := &mockDb{
		getKeys: func(table *db.TableSpec) ([]db.Key, error) {
			keys := []db.Key{}
			for k := range stateDbData {
				keys = append(keys, db.Key{Comp: []string{k}})
			}
			return keys, nil
		},
		getEntry: func(table *db.TableSpec, key db.Key) (db.Value, error) {
			if val, ok := stateDbData[key.Comp[0]]; ok {
				return val, nil
			}
			return db.Value{}, tlerr.NotFoundError{Format: "not found in mockStateDb"}
		},
	}

	originalDbGetKeys := dbGetKeys
	originalDbGetEntry := dbGetEntry
	defer func() {
		dbGetKeys = originalDbGetKeys
		dbGetEntry = originalDbGetEntry
	}()

	dbGetKeys = func(d *db.DB, table *db.TableSpec) ([]db.Key, error) {
		if d == &mockCfgDb.DB {
			return mockCfgDb.getKeys(table)
		}
		if d == &mockStateDb.DB {
			return mockStateDb.getKeys(table)
		}
		return nil, errors.New("Unknown DB pointer in GetKeys")
	}

	dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
		if d == &mockCfgDb.DB {
			return mockCfgDb.getEntry(table, key)
		}
		if d == &mockStateDb.DB {
			return mockStateDb.getEntry(table, key)
		}
		return db.Value{}, errors.New("Unknown DB pointer in GetEntry")
	}

	dbs := [db.MaxDB]*db.DB{db.ConfigDB: &mockCfgDb.DB, db.StateDB: &mockStateDb.DB}

	// Test GET on all servers
	t.Run("Get All Servers", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/ntp/servers", key: ""}

		if err := DbToYang_ntp_servers_list_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed for all servers: %v", err)
		}

		if device.System == nil || device.System.Ntp == nil || device.System.Ntp.Servers == nil {
			t.Fatalf("NTP server structure not populated")
		}
		servers := device.System.Ntp.Servers.Server
		if len(servers) != 3 {
			t.Fatalf("Expected 3 unique servers (1.1.1.1, 2.2.2.2, 3.3.3.3), got %d", len(servers))
		}

		// Server 1.1.1.1 (in ConfigDB & StateDB)
		s1 := servers["1.1.1.1"]
		if *s1.Config.Address != "1.1.1.1" {
			t.Errorf("Server 1.1.1.1 Config Address mismatch")
		}
		if *s1.State.Stratum != 2 {
			t.Errorf("Server 1.1.1.1 State mismatch")
		}

		// Server 2.2.2.2 (State Only)
		s2 := servers["2.2.2.2"]
		if *s2.Config.Address != "2.2.2.2" {
			t.Errorf("Server 2.2.2.2 Config Address mismatch")
		}
		if *s2.State.Stratum != 3 {
			t.Errorf("Server 2.2.2.2 State mismatch")
		}

		// Server 3.3.3.3 (Config Only)
		s3 := servers["3.3.3.3"]
		if *s3.Config.Address != "3.3.3.3" {
			t.Errorf("Server 3.3.3.3 Config Address mismatch")
		}
		if s3.State.Stratum != nil {
			t.Errorf("Server 3.3.3.3 should have no stratum")
		}
	})

	// Test GET on a single server
	t.Run("Get Single Server 1.1.1.1", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/ntp/servers/server[address=1.1.1.1]", key: "1.1.1.1"}

		if err := DbToYang_ntp_servers_list_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed for 1.1.1.1: %v", err)
		}
		// Should only return 1.1.1.1
		if len(device.System.Ntp.Servers.Server) != 1 {
			t.Errorf("Expected exactly 1 server, got %d", len(device.System.Ntp.Servers.Server))
		}
		s1 := device.System.Ntp.Servers.Server["1.1.1.1"]
		if s1 == nil || *s1.State.Stratum != 2 {
			t.Errorf("Server 1.1.1.1 single GET data mismatch")
		}
	})

	// Test keyed URI GET when inParams.key is empty.
	t.Run("Get Single Server From URI Key Only", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{
			dbs:    dbs,
			ygRoot: &ygRoot,
			uri:    "/openconfig-system:system/ntp/servers/server[address=1.1.1.1]",
			key:    "",
		}

		if err := DbToYang_ntp_servers_list_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed for URI keyed server: %v", err)
		}

		if len(device.System.Ntp.Servers.Server) != 1 {
			t.Errorf("Expected exactly 1 server from keyed URI, got %d", len(device.System.Ntp.Servers.Server))
		}
		if _, exists := device.System.Ntp.Servers.Server["1.1.1.1"]; !exists {
			t.Errorf("Expected server 1.1.1.1 in response")
		}
	})

	// Test GET on non-existent server
	t.Run("Get Non-Existent Server", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/ntp/servers/server[address=9.9.9.9]", key: "9.9.9.9"}

		err := DbToYang_ntp_servers_list_xfmr(params)
		if _, ok := err.(tlerr.NotFoundError); !ok {
			t.Fatalf("DbToYang should return NotFoundError for missing key, got: %v", err)
		}
		if _, exists := device.System.Ntp.Servers.Server["9.9.9.9"]; exists {
			t.Errorf("Server 9.9.9.9 should not exist in the response")
		}
	})
}

func TestSubscribe_NTP_Servers(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		keyName string
	}{
		{"Subscribe Specific", "/openconfig-system:system/ntp/servers/server[address=1.1.1.1]", "1.1.1.1"},
		{"Subscribe All", "/openconfig-system:system/ntp/servers", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inParams := XfmrSubscInParams{uri: tt.uri}

			out, err := Subscribe_ntp_servers_list_xfmr(inParams)

			if err != nil {
				t.Fatalf("Subscribe_ntp_servers_list_xfmr failed: %v", err)
			}

			if tt.keyName != "*" {
				if !out.isVirtualTbl {
					t.Errorf("Expected virtual-table mode for keyed subscribe")
				}
				if len(out.dbDataMap) != 0 {
					t.Errorf("Expected no direct DB subscriptions for keyed subscribe, got: %v", out.dbDataMap)
				}
				return
			}

			// Verify ConfigDB Subscription
			if _, ok := out.dbDataMap[db.ConfigDB][NTP_SERVER_TABLE_NAME][tt.keyName]; !ok {
				t.Errorf("Missing ConfigDB subscription for key %s", tt.keyName)
			}
			// Verify StateDB Subscription
			if _, ok := out.dbDataMap[db.StateDB][NTP_SERVER_TABLE_NAME][tt.keyName]; !ok {
				t.Errorf("Missing StateDB subscription for key %s", tt.keyName)
			}
		})
	}
}
