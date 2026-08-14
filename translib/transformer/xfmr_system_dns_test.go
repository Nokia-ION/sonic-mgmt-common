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

func TestYangToDb_DNS_Servers(t *testing.T) {
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
		dbState    map[string]db.Value
		dbErr      error
		wantMap    map[string]map[string]db.Value
		wantSubDel map[string]map[string]db.Value
		wantErr    bool
	}{
		{
			name: "List: REPLACE (Add New, Delete Missing)",
			op:   REPLACE,
			uri:  "/openconfig-system:system/dns/servers",
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Dns: &ocbinds.OpenconfigSystem_System_Dns{
						Servers: &ocbinds.OpenconfigSystem_System_Dns_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Dns_Servers_Server{},
						},
					},
				}
				s1, _ := d.System.Dns.Servers.NewServer(testIP1)
				s1.Config = &ocbinds.OpenconfigSystem_System_Dns_Servers_Server_Config{Address: ygot.String(testIP1)}
				return d
			},
			dbState: map[string]db.Value{testIP2: {Field: map[string]string{nullKey: nullKey}}},
			wantMap: map[string]map[string]db.Value{
				DNS_NAMESERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
			wantSubDel: map[string]map[string]db.Value{
				DNS_NAMESERVER_TABLE_NAME: {
					testIP2: {},
				},
			},
		},
		{
			name:    "List: DELETE (Clear All)",
			op:      DELETE,
			uri:     "/openconfig-system:system/dns/servers",
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			dbState: map[string]db.Value{testIP1: {Field: map[string]string{nullKey: nullKey}}},
			wantMap: map[string]map[string]db.Value{
				DNS_NAMESERVER_TABLE_NAME: {
					testIP1: {},
				},
			},
		},
		{
			name: "List: UPDATE (Add Only)",
			op:   UPDATE,
			uri:  "/openconfig-system:system/dns/servers",
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Dns: &ocbinds.OpenconfigSystem_System_Dns{
						Servers: &ocbinds.OpenconfigSystem_System_Dns_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Dns_Servers_Server{},
						},
					},
				}
				s1, _ := d.System.Dns.Servers.NewServer(testIP1)
				s1.Config = &ocbinds.OpenconfigSystem_System_Dns_Servers_Server_Config{Address: ygot.String(testIP1)}
				return d
			},
			dbState: map[string]db.Value{testIP2: {Field: map[string]string{nullKey: nullKey}}},
			wantMap: map[string]map[string]db.Value{
				DNS_NAMESERVER_TABLE_NAME: {
					testIP1: {Field: map[string]string{nullKey: nullKey}},
				},
			},
		},
		{
			name:   "Item: DELETE Single Server",
			op:     DELETE,
			uri:    "/openconfig-system:system/dns/servers/server[address=" + testIP1 + "]",
			key:    testIP1,
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{
				DNS_NAMESERVER_TABLE_NAME: {
					testIP1: {},
				},
			},
		},
		{
			name: "Item: REPLACE Single Server Empty Payload (DELETE)",
			op:   REPLACE,
			uri:  "/openconfig-system:system/dns/servers/server[address=" + testIP1 + "]",
			key:  testIP1,
			ygRoot: func() *ocbinds.Device {
				d := &ocbinds.Device{}
				d.System = &ocbinds.OpenconfigSystem_System{
					Dns: &ocbinds.OpenconfigSystem_System_Dns{
						Servers: &ocbinds.OpenconfigSystem_System_Dns_Servers{
							Server: map[string]*ocbinds.OpenconfigSystem_System_Dns_Servers_Server{},
						},
					},
				}
				return d
			},
			dbState: map[string]db.Value{testIP1: {Field: map[string]string{nullKey: nullKey}}},
			wantMap: map[string]map[string]db.Value{
				DNS_NAMESERVER_TABLE_NAME: {
					testIP1: {},
				},
			},
		},
		{
			name:    "Error: DB Fail on List DELETE",
			op:      DELETE,
			uri:     "/openconfig-system:system/dns/servers",
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			dbErr:   errors.New("db connection failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			gotMap, err := YangToDb_dns_servers_list_xfmr(inParams)

			if (err != nil) != tt.wantErr {
				t.Errorf("YangToDb_dns_servers_list_xfmr() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(gotMap, tt.wantMap) {
					t.Errorf("YangToDb_dns_servers_list_xfmr() gotMap = %v, wantMap %v", gotMap, tt.wantMap)
				}
				if tt.wantSubDel != nil {
					delOps := inParams.subOpDataMap[DELETE]
					if delOps == nil || (*delOps)[db.ConfigDB] == nil {
						t.Errorf("Expected DELETE sub operations for orphaned keys, got none")
					} else if gotSubDel := (*delOps)[db.ConfigDB]; !reflect.DeepEqual(gotSubDel, tt.wantSubDel) {
						t.Errorf("SubOpMap DELETE got = %v, want %v", gotSubDel, tt.wantSubDel)
					}
				}
			}
		})
	}
}

func TestDbToYang_DNS_Servers(t *testing.T) {
	cfgDbKeys := []string{"1.1.1.1", "2001:db8::1"}

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
					return db.Value{Field: map[string]string{}}, nil
				}
			}
			return db.Value{}, tlerr.NotFoundError{Format: "not found in mockCfgDb"}
		},
	}

	originalDbGetKeys := dbGetKeys
	originalDbGetEntry := dbGetEntry
	defer func() {
		dbGetKeys = originalDbGetKeys
		dbGetEntry = originalDbGetEntry
	}()

	dbGetKeys = func(d *db.DB, table *db.TableSpec) ([]db.Key, error) {
		return mockCfgDb.getKeys(table)
	}
	dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
		return mockCfgDb.getEntry(table, key)
	}

	dbs := [db.MaxDB]*db.DB{db.ConfigDB: &mockCfgDb.DB}

	t.Run("Get All Servers", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/dns/servers", key: ""}

		if err := DbToYang_dns_servers_list_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed for all servers: %v", err)
		}
		servers := device.System.Dns.Servers.Server
		if len(servers) != 2 {
			t.Fatalf("Expected 2 servers, got %d", len(servers))
		}
		s1 := servers["1.1.1.1"]
		if s1 == nil || *s1.Config.Address != "1.1.1.1" || *s1.State.Port != DNS_DEFAULT_PORT {
			t.Errorf("Server 1.1.1.1 data mismatch: %+v", s1)
		}
	})

	t.Run("Get Single Server", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/dns/servers/server[address=1.1.1.1]", key: "1.1.1.1"}

		if err := DbToYang_dns_servers_list_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed for single server: %v", err)
		}
		if len(device.System.Dns.Servers.Server) != 1 {
			t.Errorf("Expected exactly 1 server, got %d", len(device.System.Dns.Servers.Server))
		}
	})

	t.Run("Get Non-Existent Server", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/dns/servers/server[address=9.9.9.9]", key: "9.9.9.9"}

		err := DbToYang_dns_servers_list_xfmr(params)
		if _, ok := err.(tlerr.NotFoundError); !ok {
			t.Fatalf("DbToYang should return NotFoundError for missing key, got: %v", err)
		}
	})
}

func TestSubscribe_DNS_Servers(t *testing.T) {
	t.Run("Subscribe Specific", func(t *testing.T) {
		out, err := Subscribe_dns_servers_list_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/dns/servers/server[address=1.1.1.1]"})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if !out.isVirtualTbl {
			t.Errorf("Expected virtual-table mode for keyed subscribe")
		}
		if len(out.dbDataMap) != 0 {
			t.Errorf("Expected no direct DB subscriptions for keyed subscribe, got: %v", out.dbDataMap)
		}
	})

	t.Run("Subscribe All", func(t *testing.T) {
		out, err := Subscribe_dns_servers_list_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/dns/servers"})
		if err != nil {
			t.Fatalf("Subscribe failed: %v", err)
		}
		if _, ok := out.dbDataMap[db.ConfigDB][DNS_NAMESERVER_TABLE_NAME]["*"]; !ok {
			t.Errorf("Missing ConfigDB subscription for wildcard key")
		}
		if _, ok := out.dbDataMap[db.StateDB]; ok {
			t.Errorf("DNS servers are config-only; did not expect a StateDB subscription")
		}
	})
}
