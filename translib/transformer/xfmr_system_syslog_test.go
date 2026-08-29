package transformer

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	syslogTestHost1   = "10.1.1.1"
	syslogTestHost2   = "10.1.1.2"
	syslogTestHost3   = "100.120.198.176"
	syslogTestProfile = "siem-east"
)

func newSyslogRemoteServer(host string, transportSecurity *bool, profileID *string, remotePort *uint16) *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer {
	server := &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer{
		Host: ygot.String(host),
		Config: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config{
			Host: ygot.String(host),
		},
	}
	if transportSecurity != nil {
		server.Config.TransportSecurity = transportSecurity
	}
	if profileID != nil {
		server.Config.TlsProfileId = profileID
	}
	if remotePort != nil {
		server.Config.RemotePort = remotePort
	}
	return server
}

func newSyslogDevice(servers map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer) *ocbinds.Device {
	device := &ocbinds.Device{}
	device.System = &ocbinds.OpenconfigSystem_System{
		Logging: &ocbinds.OpenconfigSystem_System_Logging{
			RemoteServers: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers{
				RemoteServer: servers,
			},
		},
	}
	return device
}

func setupSyslogDbMocks(serverState map[string]db.Value) func() {
	originalDbGetKeys := dbGetKeys
	originalDbGetEntry := dbGetEntry

	dbGetKeys = func(d *db.DB, table *db.TableSpec) ([]db.Key, error) {
		if table.Name == SYSLOG_SERVER_TABLE_NAME {
			keys := make([]db.Key, 0, len(serverState))
			for host := range serverState {
				keys = append(keys, db.Key{Comp: []string{host}})
			}
			return keys, nil
		}
		return nil, errors.New("unexpected table in dbGetKeys")
	}

	dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
		if len(key.Comp) == 0 {
			return db.Value{}, tlerr.NotFoundError{Format: "empty key"}
		}
		if table.Name == SYSLOG_SERVER_TABLE_NAME {
			if entry, ok := serverState[key.Comp[0]]; ok {
				return entry, nil
			}
		}
		return db.Value{}, tlerr.NotFoundError{Format: "not found"}
	}

	return func() {
		dbGetKeys = originalDbGetKeys
		dbGetEntry = originalDbGetEntry
	}
}

func syslogTlsServerFields(port, protocol, transportSecurity, profileID string) map[string]string {
	return map[string]string{
		nullKey:                     nullKey,
		SYSLOG_FIELD_PORT:           port,
		SYSLOG_FIELD_PROTOCOL:       protocol,
		SYSLOG_FIELD_TRANSPORT_SEC:  transportSecurity,
		SYSLOG_FIELD_TLS_PROFILE_ID: profileID,
	}
}

func TestSyslogServerConfigToDbFields(t *testing.T) {
	tsFalse := false
	tsTrue := true
	port200 := uint16(200)

	tests := []struct {
		name string
		cfg  *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config
		want map[string]string
	}{
		{
			name: "Plain syslog defaults",
			cfg: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config{
				Host:              ygot.String(syslogTestHost1),
				TransportSecurity: &tsFalse,
			},
			want: map[string]string{
				nullKey:                    nullKey,
				SYSLOG_FIELD_PORT:          "514",
				SYSLOG_FIELD_PROTOCOL:      "udp",
				SYSLOG_FIELD_TRANSPORT_SEC: "false",
			},
		},
		{
			name: "TLS syslog defaults",
			cfg: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config{
				Host:              ygot.String(syslogTestHost3),
				TransportSecurity: &tsTrue,
				TlsProfileId:      ygot.String(syslogTestProfile),
			},
			want: syslogTlsServerFields("6514", "tls", "true", syslogTestProfile),
		},
		{
			name: "TLS explicit port",
			cfg: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config{
				Host:              ygot.String(syslogTestHost3),
				TransportSecurity: &tsTrue,
				TlsProfileId:      ygot.String(syslogTestProfile),
				RemotePort:        &port200,
			},
			want: syslogTlsServerFields("200", "tls", "true", syslogTestProfile),
		},
		{
			name: "Host only uses plain defaults",
			cfg: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config{
				Host: ygot.String(syslogTestHost1),
			},
			want: map[string]string{
				nullKey:               nullKey,
				SYSLOG_FIELD_PORT:     "514",
				SYSLOG_FIELD_PROTOCOL: "udp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := syslogServerConfigToDbFields(tt.cfg)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("syslogServerConfigToDbFields() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateSyslogTlsProfile(t *testing.T) {
	profileID := syslogTestProfile
	gnxi := "gnxi"
	badID := "bad/id"

	tests := []struct {
		name              string
		transportSecurity bool
		profileID         *string
		wantErr           bool
		errContains       string
	}{
		{
			name:              "TLS requires profile id",
			transportSecurity: true,
			profileID:         nil,
			wantErr:           true,
			errContains:       "tls-profile-id is required",
		},
		{
			name:              "TLS rejects gnxi",
			transportSecurity: true,
			profileID:         &gnxi,
			wantErr:           true,
			errContains:       "not a valid syslog TLS profile",
		},
		{
			name:              "TLS rejects bad id",
			transportSecurity: true,
			profileID:         &badID,
			wantErr:           true,
			errContains:       "not a valid syslog TLS profile",
		},
		{
			name:              "TLS accepts Certz profile id",
			transportSecurity: true,
			profileID:         &profileID,
			wantErr:           false,
		},
		{
			name:              "Plain syslog without profile",
			transportSecurity: false,
			profileID:         nil,
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSyslogTlsProfile(tt.transportSecurity, tt.profileID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSyslogTlsProfile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("validateSyslogTlsProfile() error = %q, want substring %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestYangToDb_Syslog_Servers(t *testing.T) {
	tsTrue := true

	tests := []struct {
		name        string
		op          Operation
		uri         string
		key         string
		ygRoot      func() *ocbinds.Device
		serverState map[string]db.Value
		wantMap     map[string]map[string]db.Value
		wantSubDel  map[string]map[string]db.Value
		wantErr     bool
	}{
		{
			name: "List: REPLACE deletes missing servers",
			op:   REPLACE,
			uri:  SYSLOG_SERVERS_PATH,
			ygRoot: func() *ocbinds.Device {
				return newSyslogDevice(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer{
					syslogTestHost3: newSyslogRemoteServer(syslogTestHost3, &tsTrue, ygot.String(syslogTestProfile), nil),
				})
			},
			serverState: map[string]db.Value{
				syslogTestHost1: {Field: map[string]string{nullKey: nullKey}},
				syslogTestHost2: {Field: map[string]string{nullKey: nullKey}},
			},
			wantMap: map[string]map[string]db.Value{
				SYSLOG_SERVER_TABLE_NAME: {
					syslogTestHost3: {Field: syslogTlsServerFields("6514", "tls", "true", syslogTestProfile)},
				},
			},
			wantSubDel: map[string]map[string]db.Value{
				SYSLOG_SERVER_TABLE_NAME: {
					syslogTestHost1: {},
					syslogTestHost2: {},
				},
			},
		},
		{
			name: "List: REPLACE empty payload clears all",
			op:   REPLACE,
			uri:  SYSLOG_SERVERS_PATH,
			ygRoot: func() *ocbinds.Device {
				return newSyslogDevice(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer{})
			},
			serverState: map[string]db.Value{
				syslogTestHost1: {Field: map[string]string{nullKey: nullKey}},
				syslogTestHost2: {Field: map[string]string{nullKey: nullKey}},
			},
			wantMap: map[string]map[string]db.Value{
				SYSLOG_SERVER_TABLE_NAME: {},
			},
			wantSubDel: map[string]map[string]db.Value{
				SYSLOG_SERVER_TABLE_NAME: {
					syslogTestHost1: {},
					syslogTestHost2: {},
				},
			},
		},
		{
			name: "List: UPDATE adds without deleting existing",
			op:   UPDATE,
			uri:  SYSLOG_SERVERS_PATH,
			ygRoot: func() *ocbinds.Device {
				return newSyslogDevice(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer{
					syslogTestHost1: newSyslogRemoteServer(syslogTestHost1, nil, nil, nil),
				})
			},
			serverState: map[string]db.Value{
				syslogTestHost2: {Field: map[string]string{nullKey: nullKey}},
			},
			wantMap: map[string]map[string]db.Value{
				SYSLOG_SERVER_TABLE_NAME: {
					syslogTestHost1: {Field: map[string]string{
						nullKey:               nullKey,
						SYSLOG_FIELD_PORT:     "514",
						SYSLOG_FIELD_PROTOCOL: "udp",
					}},
				},
			},
		},
		{
			name: "List: REPLACE rejects TLS without profile id",
			op:   REPLACE,
			uri:  SYSLOG_SERVERS_PATH,
			ygRoot: func() *ocbinds.Device {
				return newSyslogDevice(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer{
					syslogTestHost3: newSyslogRemoteServer(syslogTestHost3, &tsTrue, nil, nil),
				})
			},
			wantErr: true,
		},
		{
			name: "List: REPLACE rejects gnxi profile",
			op:   REPLACE,
			uri:  SYSLOG_SERVERS_PATH,
			ygRoot: func() *ocbinds.Device {
				return newSyslogDevice(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer{
					syslogTestHost3: newSyslogRemoteServer(syslogTestHost3, &tsTrue, ygot.String("gnxi"), nil),
				})
			},
			wantErr: true,
		},
		{
			name:   "List: DELETE clears all",
			op:     DELETE,
			uri:    SYSLOG_SERVERS_PATH,
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			serverState: map[string]db.Value{
				syslogTestHost1: {Field: map[string]string{nullKey: nullKey}},
			},
			wantMap: map[string]map[string]db.Value{
				SYSLOG_SERVER_TABLE_NAME: {
					syslogTestHost1: {},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := setupSyslogDbMocks(tt.serverState)
			defer restore()

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

			gotMap, err := YangToDb_remote_servers_list_xfmr(inParams)
			if (err != nil) != tt.wantErr {
				t.Fatalf("YangToDb_remote_servers_list_xfmr() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Fatalf("YangToDb_remote_servers_list_xfmr() gotMap = %v, wantMap %v", gotMap, tt.wantMap)
			}
			if tt.wantSubDel != nil {
				delOps := inParams.subOpDataMap[DELETE]
				if delOps == nil || (*delOps)[db.ConfigDB] == nil {
					t.Fatalf("Expected DELETE sub operations, got none")
				}
				gotSubDel := (*delOps)[db.ConfigDB]
				if !reflect.DeepEqual(gotSubDel, tt.wantSubDel) {
					t.Fatalf("SubOpMap DELETE got = %v, want %v", gotSubDel, tt.wantSubDel)
				}
			}
		})
	}
}
