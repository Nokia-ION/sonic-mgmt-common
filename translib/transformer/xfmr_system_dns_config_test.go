package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

func dnsConfigDevice(search ...string) *ocbinds.Device {
	d := &ocbinds.Device{}
	d.System = &ocbinds.OpenconfigSystem_System{
		Dns: &ocbinds.OpenconfigSystem_System_Dns{
			Config: &ocbinds.OpenconfigSystem_System_Dns_Config{Search: search},
		},
	}
	return d
}

func TestYangToDb_DNS_Config(t *testing.T) {
	const configUri = "/openconfig-system:system/dns/config"
	const searchUri = "/openconfig-system:system/dns/config/search"

	tests := []struct {
		name       string
		op         Operation
		requestUri string
		device     *ocbinds.Device
		wantMap    map[string]map[string]db.Value
		wantSubOps map[Operation]map[string]map[string]db.Value
	}{
		{
			name:       "UPDATE writes the search list",
			op:         UPDATE,
			requestUri: searchUri,
			device:     dnsConfigDevice("net.google.com", "example.com"),
			wantMap: map[string]map[string]db.Value{
				DNS_OPTIONS_TABLE_NAME: {
					DNS_OPTIONS_GLOBAL_KEY: {Field: map[string]string{dnsSearchField: "net.google.com,example.com"}},
				},
			},
		},
		{
			name:       "UPDATE without search writes nothing",
			op:         UPDATE,
			requestUri: configUri,
			device:     dnsConfigDevice(),
			wantMap:    map[string]map[string]db.Value{},
		},
		{
			name:       "REPLACE swaps the list through an UPDATE sub-operation",
			op:         REPLACE,
			requestUri: configUri,
			device:     dnsConfigDevice("net.google.com"),
			wantMap:    map[string]map[string]db.Value{},
			wantSubOps: map[Operation]map[string]map[string]db.Value{
				UPDATE: {
					DNS_OPTIONS_TABLE_NAME: {
						DNS_OPTIONS_GLOBAL_KEY: {Field: map[string]string{dnsSearchField: "net.google.com"}},
					},
				},
			},
		},
		{
			name:       "REPLACE without search clears the list",
			op:         REPLACE,
			requestUri: configUri,
			device:     dnsConfigDevice(),
			wantMap:    map[string]map[string]db.Value{},
			wantSubOps: map[Operation]map[string]map[string]db.Value{
				DELETE: {
					DNS_OPTIONS_TABLE_NAME: {
						DNS_OPTIONS_GLOBAL_KEY: {Field: map[string]string{dnsSearchField: ""}},
					},
				},
			},
		},
		{
			name:       "DELETE of the leaf-list clears it",
			op:         DELETE,
			requestUri: searchUri,
			device:     &ocbinds.Device{},
			wantMap: map[string]map[string]db.Value{
				DNS_OPTIONS_TABLE_NAME: {
					DNS_OPTIONS_GLOBAL_KEY: {Field: map[string]string{dnsSearchField: ""}},
				},
			},
		},
		{
			name:       "DELETE of one instance removes only that domain",
			op:         DELETE,
			requestUri: searchUri + "[search=research.example.com]",
			device:     &ocbinds.Device{},
			wantMap: map[string]map[string]db.Value{
				DNS_OPTIONS_TABLE_NAME: {
					DNS_OPTIONS_GLOBAL_KEY: {Field: map[string]string{dnsSearchField: "research.example.com"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ygRoot := ygot.GoStruct(tt.device)
			inParams := XfmrParams{
				dbs:          [db.MaxDB]*db.DB{db.ConfigDB: &db.DB{}},
				ygRoot:       &ygRoot,
				oper:         tt.op,
				uri:          configUri,
				requestUri:   tt.requestUri,
				subOpDataMap: make(map[Operation]*map[db.DBNum]map[string]map[string]db.Value),
			}

			gotMap, err := YangToDb_dns_config_xfmr(inParams)
			if err != nil {
				t.Fatalf("YangToDb_dns_config_xfmr() error = %v", err)
			}
			if !reflect.DeepEqual(gotMap, tt.wantMap) {
				t.Errorf("YangToDb_dns_config_xfmr() gotMap = %v, want %v", gotMap, tt.wantMap)
			}
			for _, op := range []Operation{CREATE, UPDATE, REPLACE, DELETE} {
				var got map[string]map[string]db.Value
				if subOp := inParams.subOpDataMap[op]; subOp != nil {
					got = (*subOp)[db.ConfigDB]
				}
				if want := tt.wantSubOps[op]; !reflect.DeepEqual(got, want) {
					t.Errorf("sub-operation %v got = %v, want %v", op, got, want)
				}
			}
		})
	}
}

func TestDbToYang_DNS_Config_State(t *testing.T) {
	originalDbGetEntry := dbGetEntry
	defer func() { dbGetEntry = originalDbGetEntry }()

	var row *db.Value
	dbGetEntry = func(d *db.DB, table *db.TableSpec, key db.Key) (db.Value, error) {
		if table.Name != DNS_OPTIONS_TABLE_NAME || key.Comp[0] != DNS_OPTIONS_GLOBAL_KEY || row == nil {
			return db.Value{}, tlerr.NotFoundError{Format: "not found"}
		}
		return *row, nil
	}
	dbs := [db.MaxDB]*db.DB{db.ConfigDB: &db.DB{}}

	tests := []struct {
		name string
		row  *db.Value
		want []string
	}{
		{
			name: "search list configured",
			row:  &db.Value{Field: map[string]string{dnsSearchField: "net.google.com,example.com", "ndots": "2"}},
			want: []string{"net.google.com", "example.com"},
		},
		{
			name: "row without search",
			row:  &db.Value{Field: map[string]string{"ndots": "2"}},
		},
		{
			name: "no row",
		},
	}

	for _, tt := range tests {
		row = tt.row
		t.Run(tt.name+" config", func(t *testing.T) {
			device := &ocbinds.Device{}
			var ygRoot ygot.GoStruct = device
			params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/dns/config"}
			if err := DbToYang_dns_config_xfmr(params); err != nil {
				t.Fatalf("DbToYang_dns_config_xfmr() error = %v", err)
			}
			if got := device.System.Dns.Config.Search; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("config search = %v, want %v", got, tt.want)
			}
			if device.System.Dns.State != nil {
				t.Errorf("config GET populated state: %+v", device.System.Dns.State)
			}
		})
		t.Run(tt.name+" state", func(t *testing.T) {
			device := &ocbinds.Device{}
			var ygRoot ygot.GoStruct = device
			params := XfmrParams{dbs: dbs, ygRoot: &ygRoot, uri: "/openconfig-system:system/dns/state"}
			if err := DbToYang_dns_state_xfmr(params); err != nil {
				t.Fatalf("DbToYang_dns_state_xfmr() error = %v", err)
			}
			if got := device.System.Dns.State.Search; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("state search = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscribe_DNS_Config(t *testing.T) {
	out, err := Subscribe_dns_config_xfmr(XfmrSubscInParams{uri: "/openconfig-system:system/dns/config"})
	if err != nil {
		t.Fatalf("Subscribe_dns_config_xfmr() error = %v", err)
	}
	if _, ok := out.dbDataMap[db.ConfigDB][DNS_OPTIONS_TABLE_NAME][DNS_OPTIONS_GLOBAL_KEY]; !ok {
		t.Errorf("missing CONFIG_DB subscription for %s|%s, got %v", DNS_OPTIONS_TABLE_NAME, DNS_OPTIONS_GLOBAL_KEY, out.dbDataMap)
	}
}
