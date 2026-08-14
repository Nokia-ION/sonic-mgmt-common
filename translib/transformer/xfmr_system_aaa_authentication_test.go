package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// buildAuthMethodPayload constructs a Device whose authentication config carries
// the supplied ordered authentication-method leaf-list (identity values).
func buildAuthMethodPayload(methods ...ocbinds.E_OpenconfigAaaTypes_AAA_METHOD_TYPE) *ocbinds.Device {
	d := &ocbinds.Device{}
	cfg := &ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config{}
	for _, m := range methods {
		cfg.AuthenticationMethod = append(cfg.AuthenticationMethod,
			&ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE{E_OpenconfigAaaTypes_AAA_METHOD_TYPE: m})
	}
	d.System = &ocbinds.OpenconfigSystem_System{
		Aaa: &ocbinds.OpenconfigSystem_System_Aaa{
			Authentication: &ocbinds.OpenconfigSystem_System_Aaa_Authentication{
				Config: cfg,
			},
		},
	}
	return d
}

func TestYangToDb_AAA_Authentication(t *testing.T) {
	const uri = "/openconfig-system:system/aaa/authentication"

	tests := []struct {
		name    string
		op      Operation
		ygRoot  func() *ocbinds.Device
		wantMap map[string]map[string]db.Value
		wantErr bool
	}{
		{
			name: "UPDATE tacacs+ then local",
			op:   UPDATE,
			ygRoot: func() *ocbinds.Device {
				return buildAuthMethodPayload(ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_TACACS_ALL, ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_LOCAL)
			},
			wantMap: map[string]map[string]db.Value{
				AAA_TABLE_NAME: {
					AAA_AUTHENTICATION_KEY: {Field: map[string]string{AAA_LOGIN_FIELD: "tacacs+,local"}},
				},
			},
		},
		{
			name: "REPLACE local only",
			op:   REPLACE,
			ygRoot: func() *ocbinds.Device {
				return buildAuthMethodPayload(ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_LOCAL)
			},
			wantMap: map[string]map[string]db.Value{
				AAA_TABLE_NAME: {
					AAA_AUTHENTICATION_KEY: {Field: map[string]string{AAA_LOGIN_FIELD: "local"}},
				},
			},
		},
		{
			name:   "REPLACE empty clears entry",
			op:     REPLACE,
			ygRoot: func() *ocbinds.Device { return buildAuthMethodPayload() },
			wantMap: map[string]map[string]db.Value{
				AAA_TABLE_NAME: {
					AAA_AUTHENTICATION_KEY: {},
				},
			},
		},
		{
			name:   "DELETE clears only the login field",
			op:     DELETE,
			ygRoot: func() *ocbinds.Device { return &ocbinds.Device{} },
			wantMap: map[string]map[string]db.Value{
				AAA_TABLE_NAME: {
					AAA_AUTHENTICATION_KEY: {Field: map[string]string{AAA_LOGIN_FIELD: ""}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbs := [db.MaxDB]*db.DB{db.ConfigDB: &db.DB{}}
			ygRoot := ygot.GoStruct(tt.ygRoot())
			inParams := XfmrParams{
				dbs:    dbs,
				ygRoot: &ygRoot,
				oper:   tt.op,
				uri:    uri,
			}

			gotMap, err := YangToDb_aaa_authentication_xfmr(inParams)
			if (err != nil) != tt.wantErr {
				t.Fatalf("YangToDb_aaa_authentication_xfmr() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Errorf("YangToDb_aaa_authentication_xfmr() gotMap = %v, wantMap %v", gotMap, tt.wantMap)
			}
		})
	}
}

func TestDbToYang_AAA_Authentication(t *testing.T) {
	entries := map[string]db.Value{
		AAA_AUTHENTICATION_KEY: {Field: map[string]string{AAA_LOGIN_FIELD: "tacacs+,local"}},
	}

	originalDbGetEntry := dbGetEntry
	defer func() { dbGetEntry = originalDbGetEntry }()
	dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
		if v, ok := entries[key.Comp[0]]; ok {
			return v, nil
		}
		return db.Value{}, tlerr.NotFoundError{Format: "not found"}
	}

	dbs := [db.MaxDB]*db.DB{db.ConfigDB: &db.DB{}}

	t.Run("Get authentication-method", func(t *testing.T) {
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/aaa/authentication"}

		if err := DbToYang_aaa_authentication_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed: %v", err)
		}
		cfg := device.System.Aaa.Authentication.Config
		if len(cfg.AuthenticationMethod) != 2 {
			t.Fatalf("Expected 2 config methods, got %d", len(cfg.AuthenticationMethod))
		}
		m0, ok := cfg.AuthenticationMethod[0].(*ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE)
		if !ok || m0.E_OpenconfigAaaTypes_AAA_METHOD_TYPE != ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_TACACS_ALL {
			t.Errorf("Expected first method TACACS_ALL, got %+v", cfg.AuthenticationMethod[0])
		}
		m1, ok := cfg.AuthenticationMethod[1].(*ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE)
		if !ok || m1.E_OpenconfigAaaTypes_AAA_METHOD_TYPE != ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_LOCAL {
			t.Errorf("Expected second method LOCAL, got %+v", cfg.AuthenticationMethod[1])
		}
		st := device.System.Aaa.Authentication.State
		if st == nil || len(st.AuthenticationMethod) != 2 {
			t.Errorf("Expected 2 state methods, got %+v", st)
		}
	})

	t.Run("Get with no AAA entry", func(t *testing.T) {
		empty := map[string]db.Value{}
		dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
			if v, ok := empty[key.Comp[0]]; ok {
				return v, nil
			}
			return db.Value{}, tlerr.NotFoundError{Format: "not found"}
		}
		device := &ocbinds.Device{}
		var ygRoot ygot.GoStruct = device
		params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/aaa/authentication"}

		if err := DbToYang_aaa_authentication_xfmr(params); err != nil {
			t.Fatalf("DbToYang failed: %v", err)
		}
		if len(device.System.Aaa.Authentication.Config.AuthenticationMethod) != 0 {
			t.Errorf("Expected no methods when AAA entry absent")
		}
	})
}

func TestSubscribe_AAA_Authentication(t *testing.T) {
	out, err := Subscribe_aaa_authentication_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/aaa/authentication"})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	if _, ok := out.dbDataMap[db.ConfigDB][AAA_TABLE_NAME][AAA_AUTHENTICATION_KEY]; !ok {
		t.Errorf("Missing ConfigDB subscription for AAA|authentication")
	}
}
