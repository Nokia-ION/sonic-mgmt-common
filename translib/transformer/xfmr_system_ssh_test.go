package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// newSshConfigDevice builds a *ocbinds.Device with /system/ssh-server/config
// populated with the optionally-set timeout (seconds) and dscp leaves.
func newSshConfigDevice(setTimeout bool, timeout uint16, setDscp bool, dscp uint8) *ocbinds.Device {
	d := &ocbinds.Device{}
	d.System = &ocbinds.OpenconfigSystem_System{
		SshServer: &ocbinds.OpenconfigSystem_System_SshServer{
			Config: &ocbinds.OpenconfigSystem_System_SshServer_Config{},
		},
	}
	if setTimeout {
		to := timeout
		d.System.SshServer.Config.Timeout = &to
	}
	if setDscp {
		dv := dscp
		d.System.SshServer.Config.Dscp = &dv
	}
	return d
}

func TestYangToDb_SSH_Server_Config(t *testing.T) {
	tests := []struct {
		name    string
		op      Operation
		uri     string
		ygRoot  func() *ocbinds.Device
		wantMap map[string]map[string]db.Value
		wantErr bool
	}{
		{
			name:   "UPDATE timeout only (1800s -> 30min)",
			op:     UPDATE,
			uri:    "/openconfig-system:system/ssh-server/config",
			ygRoot: func() *ocbinds.Device { return newSshConfigDevice(true, 1800, false, 0) },
			wantMap: map[string]map[string]db.Value{
				SSH_SERVER_TABLE_NAME: {
					SSH_SERVER_POLICIES_KEY: {Field: map[string]string{sshInactivityTimeoutField: "30"}},
				},
			},
		},
		{
			name:   "UPDATE dscp only",
			op:     UPDATE,
			uri:    "/openconfig-system:system/ssh-server/config",
			ygRoot: func() *ocbinds.Device { return newSshConfigDevice(false, 0, true, 32) },
			wantMap: map[string]map[string]db.Value{
				SSH_SERVER_TABLE_NAME: {
					SSH_SERVER_POLICIES_KEY: {Field: map[string]string{sshDscpField: "32"}},
				},
			},
		},
		{
			name:   "REPLACE timeout + dscp (3600s -> 60min)",
			op:     REPLACE,
			uri:    "/openconfig-system:system/ssh-server/config",
			ygRoot: func() *ocbinds.Device { return newSshConfigDevice(true, 3600, true, 10) },
			wantMap: map[string]map[string]db.Value{
				SSH_SERVER_TABLE_NAME: {
					SSH_SERVER_POLICIES_KEY: {Field: map[string]string{sshInactivityTimeoutField: "60", sshDscpField: "10"}},
				},
			},
		},
		{
			name:    "UPDATE with no config leaves yields empty map",
			op:      UPDATE,
			uri:     "/openconfig-system:system/ssh-server/config",
			ygRoot:  func() *ocbinds.Device { return newSshConfigDevice(false, 0, false, 0) },
			wantMap: map[string]map[string]db.Value{},
		},
		{
			name:   "DELETE timeout leaf clears inactivity_timeout",
			op:     DELETE,
			uri:    "/openconfig-system:system/ssh-server/config/timeout",
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{
				SSH_SERVER_TABLE_NAME: {
					SSH_SERVER_POLICIES_KEY: {Field: map[string]string{sshInactivityTimeoutField: ""}},
				},
			},
		},
		{
			name:   "DELETE dscp leaf clears dscp",
			op:     DELETE,
			uri:    "/openconfig-system:system/ssh-server/config/dscp",
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{
				SSH_SERVER_TABLE_NAME: {
					SSH_SERVER_POLICIES_KEY: {Field: map[string]string{sshDscpField: ""}},
				},
			},
		},
		{
			name:   "DELETE whole config clears both owned fields",
			op:     DELETE,
			uri:    "/openconfig-system:system/ssh-server/config",
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{
				SSH_SERVER_TABLE_NAME: {
					SSH_SERVER_POLICIES_KEY: {Field: map[string]string{sshInactivityTimeoutField: "", sshDscpField: ""}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ygRoot := ygot.GoStruct(tt.ygRoot())
			inParams := XfmrParams{
				ygRoot: &ygRoot,
				oper:   tt.op,
				uri:    tt.uri,
			}
			gotMap, err := YangToDb_ssh_server_config_xfmr(inParams)
			if (err != nil) != tt.wantErr {
				t.Fatalf("YangToDb_ssh_server_config_xfmr() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Errorf("YangToDb_ssh_server_config_xfmr() = %v, want %v", gotMap, tt.wantMap)
			}
		})
	}
}

func TestDbToYang_SSH_Server_Config(t *testing.T) {
	origGetEntry := dbGetEntry
	defer func() { dbGetEntry = origGetEntry }()

	tests := []struct {
		name        string
		entry       db.Value
		entryErr    error
		wantTimeout bool
		timeout     uint16
		wantDscp    bool
		dscp        uint8
	}{
		{
			name:        "timeout + dscp present (30min -> 1800s)",
			entry:       db.Value{Field: map[string]string{"inactivity_timeout": "30", "dscp": "32"}},
			wantTimeout: true, timeout: 1800,
			wantDscp: true, dscp: 32,
		},
		{
			name:        "timeout only (5min -> 300s)",
			entry:       db.Value{Field: map[string]string{"inactivity_timeout": "5"}},
			wantTimeout: true, timeout: 300,
		},
		{
			name:     "no SSH_SERVER|POLICIES row configured",
			entryErr: tlerr.NotFoundError{Format: "not found"},
		},
	}

	dbs := [db.MaxDB]*db.DB{db.ConfigDB: &db.DB{}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
				return tt.entry, tt.entryErr
			}
			device := &ocbinds.Device{}
			var ygRoot ygot.GoStruct = device
			params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/ssh-server/config"}

			if err := DbToYang_ssh_server_config_xfmr(params); err != nil {
				t.Fatalf("DbToYang_ssh_server_config_xfmr() error = %v", err)
			}

			cfg := device.System.SshServer.Config
			if tt.wantTimeout {
				if cfg.Timeout == nil || *cfg.Timeout != tt.timeout {
					t.Errorf("Timeout = %v, want %d", cfg.Timeout, tt.timeout)
				}
			} else if cfg.Timeout != nil {
				t.Errorf("Timeout = %d, want nil", *cfg.Timeout)
			}
			if tt.wantDscp {
				if cfg.Dscp == nil || *cfg.Dscp != tt.dscp {
					t.Errorf("Dscp = %v, want %d", cfg.Dscp, tt.dscp)
				}
			} else if cfg.Dscp != nil {
				t.Errorf("Dscp = %d, want nil", *cfg.Dscp)
			}
		})
	}
}

func TestSubscribe_SSH_Server_Config(t *testing.T) {
	out, err := Subscribe_ssh_server_config_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/ssh-server/config"})
	if err != nil {
		t.Fatalf("Subscribe_ssh_server_config_xfmr() error = %v", err)
	}
	if _, ok := out.dbDataMap[db.ConfigDB][SSH_SERVER_TABLE_NAME][SSH_SERVER_POLICIES_KEY]; !ok {
		t.Errorf("expected ConfigDB subscription for %s|%s, got %v",
			SSH_SERVER_TABLE_NAME, SSH_SERVER_POLICIES_KEY, out.dbDataMap)
	}
}
