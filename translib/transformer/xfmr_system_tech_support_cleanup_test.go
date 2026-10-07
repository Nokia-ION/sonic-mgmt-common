package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const techSupportCleanupCfgUri = "/openconfig-system:system/openconfig-system-ext:tech-support-cleanup/config"

// newTechSupportCleanupDevice builds a *ocbinds.Device with
// /system/tech-support-cleanup/config populated with the optionally-set limit.
func newTechSupportCleanupDevice(setLimit bool, limit float64) *ocbinds.Device {
	d := &ocbinds.Device{}
	d.System = &ocbinds.OpenconfigSystem_System{
		TechSupportCleanup: &ocbinds.OpenconfigSystem_System_TechSupportCleanup{
			Config: &ocbinds.OpenconfigSystem_System_TechSupportCleanup_Config{},
		},
	}
	if setLimit {
		l := limit
		d.System.TechSupportCleanup.Config.MaxTechsupportLimit = &l
	}
	return d
}

func TestYangToDb_Tech_Support_Cleanup(t *testing.T) {
	tests := []struct {
		name      string
		op        Operation
		uri       string
		ygRoot    func() *ocbinds.Device
		wantMap   map[string]map[string]db.Value
		wantSubOp RedisDbMap
	}{
		{
			name:   "UPDATE max-techsupport-limit",
			op:     UPDATE,
			uri:    techSupportCleanupCfgUri + "/max-techsupport-limit",
			ygRoot: func() *ocbinds.Device { return newTechSupportCleanupDevice(true, 15) },
			wantMap: map[string]map[string]db.Value{
				AUTO_TECHSUPPORT_TABLE_NAME: {
					AUTO_TECHSUPPORT_GLOBAL_KEY: {Field: map[string]string{autoTsMaxTechsupportLimitField: "15.0"}},
				},
			},
		},
		{
			name:    "UPDATE with no max-techsupport-limit yields empty map",
			op:      UPDATE,
			uri:     techSupportCleanupCfgUri,
			ygRoot:  func() *ocbinds.Device { return newTechSupportCleanupDevice(false, 0) },
			wantMap: map[string]map[string]db.Value{},
		},
		{
			name:    "DELETE writes 0.0 instead of removing the field",
			op:      DELETE,
			uri:     techSupportCleanupCfgUri + "/max-techsupport-limit",
			ygRoot:  func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{},
			wantSubOp: RedisDbMap{
				db.ConfigDB: {
					AUTO_TECHSUPPORT_TABLE_NAME: {
						AUTO_TECHSUPPORT_GLOBAL_KEY: {Field: map[string]string{autoTsMaxTechsupportLimitField: "0.0"}},
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
			gotMap, err := YangToDb_tech_support_cleanup_xfmr(inParams)
			if err != nil {
				t.Fatalf("YangToDb_tech_support_cleanup_xfmr() error = %v", err)
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Errorf("YangToDb_tech_support_cleanup_xfmr() = %v, want %v", gotMap, tt.wantMap)
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

func TestDbToYang_Tech_Support_Cleanup(t *testing.T) {
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
			entry:       db.Value{Field: map[string]string{"max_techsupport_limit": "10.0", "max_core_limit": "5.0", "state": "enabled"}},
			wantLimit:   f(10),
			wantEnabled: b(true),
		},
		{
			name:        "disabled state",
			entry:       db.Value{Field: map[string]string{"max_techsupport_limit": "12.5", "state": "disabled"}},
			wantLimit:   f(12.5),
			wantEnabled: b(false),
		},
		{
			name:  "unparsable limit is omitted",
			entry: db.Value{Field: map[string]string{"max_techsupport_limit": "abc"}},
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
			params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: techSupportCleanupCfgUri}

			if err := DbToYang_tech_support_cleanup_xfmr(params); err != nil {
				t.Fatalf("DbToYang_tech_support_cleanup_xfmr() error = %v", err)
			}

			c := device.System.TechSupportCleanup
			if !reflect.DeepEqual(c.Config.MaxTechsupportLimit, tt.wantLimit) {
				t.Errorf("Config.MaxTechsupportLimit = %v, want %v", c.Config.MaxTechsupportLimit, tt.wantLimit)
			}
			if !reflect.DeepEqual(c.State.MaxTechsupportLimit, tt.wantLimit) {
				t.Errorf("State.MaxTechsupportLimit = %v, want %v", c.State.MaxTechsupportLimit, tt.wantLimit)
			}
			if !reflect.DeepEqual(c.State.Enabled, tt.wantEnabled) {
				t.Errorf("State.Enabled = %v, want %v", c.State.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestSubscribe_Tech_Support_Cleanup(t *testing.T) {
	out, err := Subscribe_tech_support_cleanup_xfmr(XfmrSubscInParams{uri: techSupportCleanupCfgUri})
	if err != nil {
		t.Fatalf("Subscribe_tech_support_cleanup_xfmr() error = %v", err)
	}
	if _, ok := out.dbDataMap[db.ConfigDB][AUTO_TECHSUPPORT_TABLE_NAME][AUTO_TECHSUPPORT_GLOBAL_KEY]; !ok {
		t.Errorf("expected ConfigDB subscription for %s|%s, got %v",
			AUTO_TECHSUPPORT_TABLE_NAME, AUTO_TECHSUPPORT_GLOBAL_KEY, out.dbDataMap)
	}
}
