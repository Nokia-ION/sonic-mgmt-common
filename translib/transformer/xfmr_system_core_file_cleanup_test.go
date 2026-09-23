package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const coreFileCleanupCfgUri = "/openconfig-system:system/openconfig-system-ext:core-file-cleanup/config"

// newCoreFileCleanupDevice builds a *ocbinds.Device with
// /system/core-file-cleanup/config populated with the optionally-set limit.
func newCoreFileCleanupDevice(setLimit bool, limit float64) *ocbinds.Device {
	d := &ocbinds.Device{}
	d.System = &ocbinds.OpenconfigSystem_System{
		CoreFileCleanup: &ocbinds.OpenconfigSystem_System_CoreFileCleanup{
			Config: &ocbinds.OpenconfigSystem_System_CoreFileCleanup_Config{},
		},
	}
	if setLimit {
		l := limit
		d.System.CoreFileCleanup.Config.MaxCoreLimit = &l
	}
	return d
}

func TestFormatMaxCoreLimit(t *testing.T) {
	for in, want := range map[float64]string{0: "0.0", 5: "5.0", 8.25: "8.25", 99.99: "99.99"} {
		if got := formatMaxCoreLimit(in); got != want {
			t.Errorf("formatMaxCoreLimit(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestYangToDb_Core_File_Cleanup(t *testing.T) {
	tests := []struct {
		name      string
		op        Operation
		uri       string
		ygRoot    func() *ocbinds.Device
		wantMap   map[string]map[string]db.Value
		wantSubOp RedisDbMap
	}{
		{
			name:   "UPDATE max-core-limit",
			op:     UPDATE,
			uri:    coreFileCleanupCfgUri + "/max-core-limit",
			ygRoot: func() *ocbinds.Device { return newCoreFileCleanupDevice(true, 8) },
			wantMap: map[string]map[string]db.Value{
				AUTO_TECHSUPPORT_TABLE_NAME: {
					AUTO_TECHSUPPORT_GLOBAL_KEY: {Field: map[string]string{autoTsMaxCoreLimitField: "8.0"}},
				},
			},
		},
		{
			name:    "UPDATE with no max-core-limit yields empty map",
			op:      UPDATE,
			uri:     coreFileCleanupCfgUri,
			ygRoot:  func() *ocbinds.Device { return newCoreFileCleanupDevice(false, 0) },
			wantMap: map[string]map[string]db.Value{},
		},
		{
			name:    "DELETE writes 0.0 instead of removing the field",
			op:      DELETE,
			uri:     coreFileCleanupCfgUri + "/max-core-limit",
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{},
			wantSubOp: RedisDbMap{
				db.ConfigDB: {
					AUTO_TECHSUPPORT_TABLE_NAME: {
						AUTO_TECHSUPPORT_GLOBAL_KEY: {Field: map[string]string{autoTsMaxCoreLimitField: "0.0"}},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ygRoot := ygot.GoStruct(tt.ygRoot())
			inParams := XfmrParams{
				ygRoot:       &ygRoot,
				oper:         tt.op,
				uri:          tt.uri,
				requestUri:   tt.uri,
				subOpDataMap: map[Operation]*RedisDbMap{},
			}
			gotMap, err := YangToDb_core_file_cleanup_xfmr(inParams)
			if err != nil {
				t.Fatalf("YangToDb_core_file_cleanup_xfmr() error = %v", err)
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Errorf("YangToDb_core_file_cleanup_xfmr() = %v, want %v", gotMap, tt.wantMap)
			}
			gotSubOp := inParams.subOpDataMap[UPDATE]
			if tt.wantSubOp == nil {
				if gotSubOp != nil {
					t.Errorf("subOpDataMap[UPDATE] = %v, want nil", *gotSubOp)
				}
				return
			}
			if gotSubOp == nil || !reflect.DeepEqual(*gotSubOp, tt.wantSubOp) {
				t.Errorf("subOpDataMap[UPDATE] = %v, want %v", gotSubOp, tt.wantSubOp)
			}
		})
	}
}

func TestDbToYang_Core_File_Cleanup(t *testing.T) {
	origGetEntry := dbGetEntry
	defer func() { dbGetEntry = origGetEntry }()

	f := func(v float64) *float64 { return &v }
	b := func(v bool) *bool { return &v }

	tests := []struct {
		name        string
		entry       db.Value
		entryErr    error
		wantLimit   *float64
		wantEnabled *bool
	}{
		{
			name:        "limit and enabled state",
			entry:       db.Value{Field: map[string]string{"max_core_limit": "5.0", "state": "enabled"}},
			wantLimit:   f(5),
			wantEnabled: b(true),
		},
		{
			name:        "disabled state",
			entry:       db.Value{Field: map[string]string{"max_core_limit": "8.25", "state": "disabled"}},
			wantLimit:   f(8.25),
			wantEnabled: b(false),
		},
		{
			name:  "unparsable limit is omitted",
			entry: db.Value{Field: map[string]string{"max_core_limit": "abc"}},
		},
		{
			name:     "no AUTO_TECHSUPPORT|GLOBAL row configured",
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
			params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: coreFileCleanupCfgUri}

			if err := DbToYang_core_file_cleanup_xfmr(params); err != nil {
				t.Fatalf("DbToYang_core_file_cleanup_xfmr() error = %v", err)
			}

			c := device.System.CoreFileCleanup
			if !reflect.DeepEqual(c.Config.MaxCoreLimit, tt.wantLimit) {
				t.Errorf("Config.MaxCoreLimit = %v, want %v", c.Config.MaxCoreLimit, tt.wantLimit)
			}
			if !reflect.DeepEqual(c.State.MaxCoreLimit, tt.wantLimit) {
				t.Errorf("State.MaxCoreLimit = %v, want %v", c.State.MaxCoreLimit, tt.wantLimit)
			}
			if !reflect.DeepEqual(c.State.Enabled, tt.wantEnabled) {
				t.Errorf("State.Enabled = %v, want %v", c.State.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestSubscribe_Core_File_Cleanup(t *testing.T) {
	out, err := Subscribe_core_file_cleanup_xfmr(XfmrSubscInParams{uri: coreFileCleanupCfgUri})
	if err != nil {
		t.Fatalf("Subscribe_core_file_cleanup_xfmr() error = %v", err)
	}
	if _, ok := out.dbDataMap[db.ConfigDB][AUTO_TECHSUPPORT_TABLE_NAME][AUTO_TECHSUPPORT_GLOBAL_KEY]; !ok {
		t.Errorf("expected ConfigDB subscription for %s|%s, got %v",
			AUTO_TECHSUPPORT_TABLE_NAME, AUTO_TECHSUPPORT_GLOBAL_KEY, out.dbDataMap)
	}
}
