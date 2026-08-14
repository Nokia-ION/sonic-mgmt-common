package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// buildTacacsPayload constructs a Device with a single TACACS server-group and
// one server (address) carrying the supplied timeout/port/secret leaves.
func buildTacacsPayload(address string, timeout *uint16, port *uint16, secret *string) *ocbinds.Device {
	d := &ocbinds.Device{}
	d.System = &ocbinds.OpenconfigSystem_System{
		Aaa: &ocbinds.OpenconfigSystem_System_Aaa{
			ServerGroups: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups{
				ServerGroup: map[string]*ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup{},
			},
		},
	}
	g, _ := d.System.Aaa.ServerGroups.NewServerGroup(TACACS_DEFAULT_GROUP_NAME)
	g.Config = &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Config{
		Name: ygot.String(TACACS_DEFAULT_GROUP_NAME),
		Type: ocbinds.OpenconfigAaaTypes_AAA_SERVER_TYPE_TACACS,
	}
	g.Servers = &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers{
		Server: map[string]*ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server{},
	}
	if address != "" {
		s, _ := g.Servers.NewServer(address)
		s.Config = &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_Config{
			Address: ygot.String(address),
			Timeout: timeout,
		}
		s.Tacacs = &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_Tacacs{
			Config: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_Tacacs_Config{
				Port:      port,
				SecretKey: secret,
			},
		}
	}
	return d
}

func TestYangToDb_TACACS_Servers(t *testing.T) {
	const testIP1 = "10.1.1.1"
	const testIP2 = "10.1.1.2"
	const groupPrefix = "/openconfig-system:system/aaa/server-groups/server-group[name=TACACS]"

	originalDbGetKeys := dbGetKeys
	originalDbGetEntry := dbGetEntry
	defer func() {
		dbGetKeys = originalDbGetKeys
		dbGetEntry = originalDbGetEntry
	}()

	tests := []struct {
		name       string
		op         Operation
		uri        string
		ygRoot     func() *ocbinds.Device
		dbKeys     []string        // existing TACPLUS_SERVER keys (GetKeys)
		dbExisting map[string]bool // addresses for which GetEntry succeeds
		wantMap    map[string]map[string]db.Value
		wantSubDel map[string]map[string]db.Value
		wantErr    bool
	}{
		{
			name: "List: UPDATE new server applies backend defaults",
			op:   UPDATE,
			uri:  "/openconfig-system:system/aaa/server-groups",
			ygRoot: func() *ocbinds.Device {
				return buildTacacsPayload(testIP1, ygot.Uint16(10), nil, ygot.String("secret"))
			},
			dbExisting: map[string]bool{},
			wantMap: map[string]map[string]db.Value{
				TACPLUS_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{
						"timeout":  "10",
						"passkey":  "secret",
						"tcp_port": TACACS_DEFAULT_TCP_PORT,
						"priority": TACACS_DEFAULT_PRIORITY,
					}},
				},
			},
		},
		{
			name: "List: UPDATE existing server merges without defaults",
			op:   UPDATE,
			uri:  "/openconfig-system:system/aaa/server-groups",
			ygRoot: func() *ocbinds.Device {
				return buildTacacsPayload(testIP1, ygot.Uint16(20), nil, nil)
			},
			dbExisting: map[string]bool{testIP1: true},
			wantMap: map[string]map[string]db.Value{
				TACPLUS_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{"timeout": "20"}},
				},
			},
		},
		{
			name: "List: REPLACE adds server and prunes missing",
			op:   REPLACE,
			uri:  "/openconfig-system:system/aaa/server-groups",
			ygRoot: func() *ocbinds.Device {
				return buildTacacsPayload(testIP1, nil, ygot.Uint16(49), ygot.String("secret"))
			},
			dbKeys: []string{testIP2},
			wantMap: map[string]map[string]db.Value{
				TACPLUS_SERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{
						"tcp_port": "49",
						"passkey":  "secret",
						"priority": TACACS_DEFAULT_PRIORITY,
					}},
				},
			},
			wantSubDel: map[string]map[string]db.Value{
				TACPLUS_SERVER_TABLE_NAME: {
					testIP2: {},
				},
			},
		},
		{
			name:   "Item: DELETE single server",
			op:     DELETE,
			uri:    groupPrefix + "/servers/server[address=" + testIP1 + "]",
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{
				TACPLUS_SERVER_TABLE_NAME: {
					testIP1: {},
				},
			},
		},
		{
			name:   "List: DELETE all servers",
			op:     DELETE,
			uri:    "/openconfig-system:system/aaa/server-groups",
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			dbKeys: []string{testIP1, testIP2},
			wantMap: map[string]map[string]db.Value{
				TACPLUS_SERVER_TABLE_NAME: {
					testIP1: {},
					testIP2: {},
				},
			},
		},
		{
			name: "Item: REPLACE single server empty payload deletes",
			op:   REPLACE,
			uri:  groupPrefix + "/servers/server[address=" + testIP1 + "]",
			ygRoot: func() *ocbinds.Device {
				return buildTacacsPayload("", nil, nil, nil) // group present, no server
			},
			wantMap: map[string]map[string]db.Value{
				TACPLUS_SERVER_TABLE_NAME: {
					testIP1: {},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbGetKeys = func(d *db.DB, table *db.TableSpec) ([]db.Key, error) {
				var keys []db.Key
				for _, k := range tt.dbKeys {
					keys = append(keys, db.Key{Comp: []string{k}})
				}
				return keys, nil
			}
			dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
				if tt.dbExisting != nil && tt.dbExisting[key.Comp[0]] {
					return db.Value{Field: map[string]string{"tcp_port": "49"}}, nil
				}
				return db.Value{}, tlerr.NotFoundError{Format: "not found"}
			}

			dbs := [db.MaxDB]*db.DB{db.ConfigDB: &db.DB{}}
			ygRoot := ygot.GoStruct(tt.ygRoot())
			inParams := XfmrParams{
				dbs:          dbs,
				ygRoot:       &ygRoot,
				oper:         tt.op,
				uri:          tt.uri,
				subOpDataMap: make(map[Operation]*map[db.DBNum]map[string]map[string]db.Value),
			}

			gotMap, err := YangToDb_tacacs_server_groups_xfmr(inParams)

			if (err != nil) != tt.wantErr {
				t.Errorf("YangToDb_tacacs_server_groups_xfmr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(gotMap, tt.wantMap) {
					t.Errorf("YangToDb_tacacs_server_groups_xfmr() gotMap = %v, wantMap %v", gotMap, tt.wantMap)
				}
				if tt.wantSubDel != nil {
					delOps := inParams.subOpDataMap[DELETE]
					if delOps == nil || (*delOps)[db.ConfigDB] == nil {
						t.Errorf("Expected DELETE sub operations for pruned keys, got none")
					} else if gotSubDel := (*delOps)[db.ConfigDB]; !reflect.DeepEqual(gotSubDel, tt.wantSubDel) {
						t.Errorf("SubOpMap DELETE got = %v, want %v", gotSubDel, tt.wantSubDel)
					}
				}
			}
		})
	}
}

func TestDbToYang_TACACS_Servers(t *testing.T) {
	entries := map[string]db.Value{
		"10.1.1.1": {Field: map[string]string{"tcp_port": "49", "timeout": "10", "passkey": "secret", "priority": "1"}},
		"10.1.1.2": {Field: map[string]string{"tcp_port": "49", "priority": "1"}},
	}

	mockCfgDb := &mockDb{
		getKeys: func(table *db.TableSpec) ([]db.Key, error) {
			keys := []db.Key{}
			for k := range entries {
				keys = append(keys, db.Key{Comp: []string{k}})
			}
			return keys, nil
		},
		getEntry: func(table *db.TableSpec, key db.Key) (db.Value, error) {
			if v, ok := entries[key.Comp[0]]; ok {
				return v, nil
			}
			return db.Value{}, tlerr.NotFoundError{Format: "not found"}
		},
	}

	originalDbGetKeys := dbGetKeys
	originalDbGetEntry := dbGetEntry
	defer func() {
		dbGetKeys = originalDbGetKeys
		dbGetEntry = originalDbGetEntry
	}()
	dbGetKeys = func(d *db.DB, table *db.TableSpec) ([]db.Key, error) { return mockCfgDb.getKeys(table) }
	dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
		return mockCfgDb.getEntry(table, key)
	}

	dbs := [db.MaxDB]*db.DB{db.ConfigDB: &mockCfgDb.DB}

	t.Run("Get All Servers", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/aaa/server-groups"}

		if err := DbToYang_tacacs_server_groups_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed: %v", err)
		}
		grp := device.System.Aaa.ServerGroups.ServerGroup[TACACS_DEFAULT_GROUP_NAME]
		if grp == nil {
			t.Fatalf("Expected default TACACS server-group")
		}
		if grp.Config.Type != ocbinds.OpenconfigAaaTypes_AAA_SERVER_TYPE_TACACS {
			t.Errorf("Expected group type TACACS")
		}
		if len(grp.Servers.Server) != 2 {
			t.Fatalf("Expected 2 servers, got %d", len(grp.Servers.Server))
		}
		s1 := grp.Servers.Server["10.1.1.1"]
		if s1 == nil || *s1.Config.Timeout != 10 || *s1.Tacacs.Config.Port != 49 || *s1.Tacacs.Config.SecretKey != "secret" {
			t.Errorf("Server 10.1.1.1 data mismatch: %+v", s1)
		}
	})

	t.Run("Get Single Server", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/aaa/server-groups/server-group[name=TACACS]/servers/server[address=10.1.1.1]"}

		if err := DbToYang_tacacs_server_groups_xfmr(params); err != nil {
			t.Fatalf("DbToYang single failed: %v", err)
		}
		grp := device.System.Aaa.ServerGroups.ServerGroup[TACACS_DEFAULT_GROUP_NAME]
		if len(grp.Servers.Server) != 1 {
			t.Errorf("Expected exactly 1 server, got %d", len(grp.Servers.Server))
		}
	})

	t.Run("Get Non-Existent Server", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/aaa/server-groups/server-group[name=TACACS]/servers/server[address=9.9.9.9]"}

		err := DbToYang_tacacs_server_groups_xfmr(params)
		if _, ok := err.(tlerr.NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got: %v", err)
		}
	})
}

func TestSubscribe_TACACS_Servers(t *testing.T) {
	t.Run("Subscribe Specific", func(t *testing.T) {
		out, err := Subscribe_tacacs_server_groups_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/aaa/server-groups/server-group[name=TACACS]/servers/server[address=10.1.1.1]"})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if !out.isVirtualTbl {
			t.Errorf("Expected virtual-table mode for keyed subscribe")
		}
	})

	t.Run("Subscribe All", func(t *testing.T) {
		out, err := Subscribe_tacacs_server_groups_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/aaa/server-groups"})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][TACPLUS_SERVER_TABLE_NAME]["*"]; !ok {
			t.Errorf("Missing ConfigDB subscription for wildcard key")
		}
	})
}
