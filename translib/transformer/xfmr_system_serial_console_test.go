package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// newConsoleConfigDevice builds a *ocbinds.Device with /system/console/config
// populated with the optionally-set timeout (seconds) leaf.
func newConsoleConfigDevice(setTimeout bool, timeout uint16) *ocbinds.Device {
	d := &ocbinds.Device{}
	d.System = &ocbinds.OpenconfigSystem_System{
		Console: &ocbinds.OpenconfigSystem_System_Console{
			Config: &ocbinds.OpenconfigSystem_System_Console_Config{},
		},
	}
	if setTimeout {
		to := timeout
		d.System.Console.Config.Timeout = &to
	}
	return d
}

func TestYangToDb_Serial_Console_Config(t *testing.T) {
	tests := []struct {
		name    string
		op      Operation
		uri     string
		ygRoot  func() *ocbinds.Device
		wantMap map[string]map[string]db.Value
		wantErr bool
	}{
		{
			name:   "UPDATE timeout (600s -> 10min)",
			op:     UPDATE,
			uri:    "/openconfig-system:system/console/config",
			ygRoot: func() *ocbinds.Device { return newConsoleConfigDevice(true, 600) },
			wantMap: map[string]map[string]db.Value{
				SERIAL_CONSOLE_TABLE_NAME: {
					SERIAL_CONSOLE_POLICIES_KEY: {Field: map[string]string{serialInactivityTimeoutField: "10"}},
				},
			},
		},
		{
			name:    "UPDATE with no timeout yields empty map",
			op:      UPDATE,
			uri:     "/openconfig-system:system/console/config",
			ygRoot:  func() *ocbinds.Device { return newConsoleConfigDevice(false, 0) },
			wantMap: map[string]map[string]db.Value{},
		},
		{
			name:   "DELETE clears inactivity_timeout",
			op:     DELETE,
			uri:    "/openconfig-system:system/console/config/timeout",
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{
				SERIAL_CONSOLE_TABLE_NAME: {
					SERIAL_CONSOLE_POLICIES_KEY: {Field: map[string]string{serialInactivityTimeoutField: ""}},
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
			gotMap, err := YangToDb_serial_console_xfmr(inParams)
			if (err != nil) != tt.wantErr {
				t.Fatalf("YangToDb_serial_console_xfmr() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Errorf("YangToDb_serial_console_xfmr() = %v, want %v", gotMap, tt.wantMap)
			}
		})
	}
}

func TestDbToYang_Serial_Console_Config(t *testing.T) {
	origGetEntry := dbGetEntry
	defer func() { dbGetEntry = origGetEntry }()

	tests := []struct {
		name        string
		entry       db.Value
		entryErr    error
		wantTimeout bool
		timeout     uint16
	}{
		{
			name:        "timeout present (10min -> 600s)",
			entry:       db.Value{Field: map[string]string{"inactivity_timeout": "10"}},
			wantTimeout: true, timeout: 600,
		},
		{
			name:     "no SERIAL_CONSOLE|POLICIES row configured",
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
			params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/console/config"}

			if err := DbToYang_serial_console_xfmr(params); err != nil {
				t.Fatalf("DbToYang_serial_console_xfmr() error = %v", err)
			}

			cfg := device.System.Console.Config
			if tt.wantTimeout {
				if cfg.Timeout == nil || *cfg.Timeout != tt.timeout {
					t.Errorf("Timeout = %v, want %d", cfg.Timeout, tt.timeout)
				}
			} else if cfg.Timeout != nil {
				t.Errorf("Timeout = %d, want nil", *cfg.Timeout)
			}
		})
	}
}

func TestSubscribe_Serial_Console_Config(t *testing.T) {
	out, err := Subscribe_serial_console_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/console/config"})
	if err != nil {
		t.Fatalf("Subscribe_serial_console_xfmr() error = %v", err)
	}
	if _, ok := out.dbDataMap[db.ConfigDB][SERIAL_CONSOLE_TABLE_NAME][SERIAL_CONSOLE_POLICIES_KEY]; !ok {
		t.Errorf("expected ConfigDB subscription for %s|%s, got %v",
			SERIAL_CONSOLE_TABLE_NAME, SERIAL_CONSOLE_POLICIES_KEY, out.dbDataMap)
	}
}
