// Copyright 2026 Nokia.

package translib

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

func TestIsRootSetPath(t *testing.T) {
	for _, path := range []string{"", "/", "//"} {
		if !isRootSetPath(path) {
			t.Errorf("isRootSetPath(%q) = false, want true", path)
		}
	}
	if isRootSetPath("/openconfig-lldp:lldp") {
		t.Error("isRootSetPath(/openconfig-lldp:lldp) = true, want false")
	}
}

func TestResolveRootSetRequestPassthrough(t *testing.T) {
	payload := []byte(`{"config":{"enabled":true}}`)
	path, got, err := resolveRootSetRequest("/openconfig-lldp:lldp", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/openconfig-lldp:lldp" {
		t.Errorf("path = %s, want original", path)
	}
	if string(got) != string(payload) {
		t.Errorf("payload rewritten for non-root path: %s", got)
	}
}

func TestDeriveSetPathFromPayload(t *testing.T) {
	payload := []byte(`{"openconfig-lldp:lldp":{"config":{"enabled":true}}}`)
	path, inner, err := deriveSetPathFromPayload(payload)
	if err != nil {
		t.Fatalf("deriveSetPathFromPayload: %v", err)
	}
	if path != "/openconfig-lldp:lldp" {
		t.Errorf("path = %s, want /openconfig-lldp:lldp", path)
	}
	if !strings.Contains(string(inner), `"enabled"`) {
		t.Errorf("unwrapped payload missing enabled: %s", inner)
	}
	if strings.Contains(string(inner), "openconfig-lldp:lldp") {
		t.Errorf("payload was not unwrapped: %s", inner)
	}
}

func TestDeriveSetPathFromPayloadRejects(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    string
	}{
		{"empty", nil, "requires a JSON payload"},
		{"not object", []byte(`["x"]`), "must be a JSON object"},
		{"empty object", []byte(`{}`), "payload is empty"},
		{"unknown key", []byte(`{"not-a-yang-module:foo":{}}`), "cannot derive app"},
		{"mixed valid invalid", []byte(`{"openconfig-lldp:lldp":{},"not-a-yang-module:foo":{}}`), "cannot derive app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := deriveSetOpsFromPayload(tt.payload)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestDeriveSetOpsFromPayloadMultipleKeys(t *testing.T) {
	payload := []byte(`{
  "openconfig-interfaces:interfaces": {
    "interface": [{"name": "Ethernet46"}]
  },
  "openconfig-lacp:lacp": {
    "interfaces": {
      "interface": [{"name": "PortChannel10"}]
    }
  }
}`)
	ops, err := deriveSetOpsFromPayload(payload)
	if err != nil {
		t.Fatalf("deriveSetOpsFromPayload: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2", len(ops))
	}
	if ops[0].Path != "/openconfig-interfaces:interfaces" {
		t.Errorf("ops[0].Path = %s, want /openconfig-interfaces:interfaces", ops[0].Path)
	}
	if !strings.Contains(string(ops[0].Payload), "Ethernet46") {
		t.Errorf("interfaces payload missing Ethernet46: %s", ops[0].Payload)
	}
	if strings.Contains(string(ops[0].Payload), "openconfig-lacp:lacp") {
		t.Errorf("interfaces payload was not unwrapped: %s", ops[0].Payload)
	}
	if ops[1].Path != "/openconfig-lacp:lacp" {
		t.Errorf("ops[1].Path = %s, want /openconfig-lacp:lacp", ops[1].Path)
	}
	if !strings.Contains(string(ops[1].Payload), "PortChannel10") {
		t.Errorf("lacp payload missing PortChannel10: %s", ops[1].Payload)
	}
}

func TestExpandRootSetRequestMultipleKeys(t *testing.T) {
	payload := []byte(`{"openconfig-interfaces:interfaces":{"interface":[]},"openconfig-lacp:lacp":{"interfaces":{}}}`)
	ops, err := expandRootSetRequest("/", payload)
	if err != nil {
		t.Fatalf("expandRootSetRequest: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2", len(ops))
	}
}

func TestResolveRootSetRequestRejectsMultipleKeys(t *testing.T) {
	payload := []byte(`{"openconfig-interfaces:interfaces":{},"openconfig-lacp:lacp":{}}`)
	_, _, err := resolveRootSetRequest("/", payload)
	if err == nil {
		t.Fatal("expected error for multi-key resolveRootSetRequest")
	}
	if !strings.Contains(err.Error(), "multiple top-level keys") {
		t.Errorf("got error %v, want multiple top-level keys", err)
	}
}

func TestExpandBulkRootSetMultipleKeys(t *testing.T) {
	payload := []byte(`{"openconfig-interfaces:interfaces":{"interface":[]},"openconfig-lacp:lacp":{"interfaces":{}}}`)
	req := BulkRequest{
		Request: []BulkRequestEntry{
			{Entry: SetRequest{Path: "/", Payload: payload}, Operation: UPDATE},
			{Entry: SetRequest{Path: "/openconfig-lldp:lldp", Payload: []byte(`{"config":{"enabled":true}}`)}, Operation: UPDATE},
		},
	}
	got, origin, err := expandBulkRootSet(req)
	if err != nil {
		t.Fatalf("expandBulkRootSet: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	if !reflect.DeepEqual(origin, []int{0, 0, 1}) {
		t.Errorf("origin = %v, want [0 0 1]", origin)
	}
	if got[0].Entry.Path != "/openconfig-interfaces:interfaces" {
		t.Errorf("got[0].Path = %s", got[0].Entry.Path)
	}
	if got[1].Entry.Path != "/openconfig-lacp:lacp" {
		t.Errorf("got[1].Path = %s", got[1].Entry.Path)
	}
	if got[2].Entry.Path != "/openconfig-lldp:lldp" {
		t.Errorf("got[2].Path = %s", got[2].Entry.Path)
	}
}

func TestExpandSetOpsPortChannelListFromOrigin(t *testing.T) {
	payload := []byte(`{
  "openconfig-interfaces:interfaces": {
    "interface": [
      {
        "config": {
          "name": "PortChannel100",
          "type": "iana-if-type:ieee8023adLag"
        },
        "name": "PortChannel100",
        "openconfig-if-aggregate:aggregation": {
          "config": {
            "lag-type": "LACP"
          }
        }
      }
    ]
  }
}`)
	ops, needBulk, err := expandSetOps("/", payload, UPDATE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	if !needBulk {
		t.Fatal("expected Bulk for list-instance expansion")
	}
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	wantPath := "/openconfig-interfaces:interfaces/interface[name=PortChannel100]"
	if ops[0].Path != wantPath {
		t.Errorf("path = %s, want %s", ops[0].Path, wantPath)
	}
	if strings.Contains(string(ops[0].Payload), `"interface"`) {
		t.Errorf("instance payload still wrapped in interface list: %s", ops[0].Payload)
	}
	if !strings.Contains(string(ops[0].Payload), `"PortChannel100"`) {
		t.Errorf("instance payload missing PortChannel100: %s", ops[0].Payload)
	}
}

func TestExpandSetOpsMixedInterfaceList(t *testing.T) {
	payload := []byte(`{
  "interface": [
    {"name": "Ethernet46", "config": {"name": "Ethernet46", "type": "iana-if-type:ethernetCsmacd"}},
    {"name": "PortChannel100", "config": {"name": "PortChannel100", "type": "iana-if-type:ieee8023adLag"}}
  ]
}`)
	ops, changed, err := expandListInstances("/openconfig-interfaces:interfaces", payload)
	if err != nil {
		t.Fatalf("expandListInstances: %v", err)
	}
	if !changed {
		t.Fatal("expected list expansion")
	}
	if len(ops) != 2 {
		t.Fatalf("got %d ops, want 2", len(ops))
	}
	// PortChannels run first so members listed before them can still join.
	if ops[0].Path != "/openconfig-interfaces:interfaces/interface[name=PortChannel100]" {
		t.Errorf("ops[0].Path = %s", ops[0].Path)
	}
	if ops[1].Path != "/openconfig-interfaces:interfaces/interface[name=Ethernet46]" {
		t.Errorf("ops[1].Path = %s", ops[1].Path)
	}
}

func TestExpandSetOpsPortChannelsBeforeMembers(t *testing.T) {
	payload := []byte(`{
  "interface": [
    {"name": "Ethernet46", "openconfig-if-ethernet:ethernet": {"config": {"openconfig-if-aggregate:aggregate-id": "PortChannel49"}}},
    {"name": "Ethernet48", "config": {"name": "Ethernet48"}},
    {"name": "PortChannel49", "config": {"name": "PortChannel49", "enabled": true}},
    {"name": "Vlan10", "config": {"name": "Vlan10"}},
    {"name": "PortChannel2", "config": {"name": "PortChannel2"}}
  ]
}`)
	ops, _, err := expandSetOps("/openconfig-interfaces:interfaces", payload, UPDATE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	var got []string
	for _, op := range ops {
		got = append(got, strings.TrimPrefix(op.Path, "/openconfig-interfaces:interfaces/interface"))
	}
	want := []string{"[name=Vlan10]", "[name=PortChannel49]", "[name=PortChannel2]", "[name=Ethernet46]", "[name=Ethernet48]"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("op order = %v, want %v", got, want)
	}
}

func TestExpandSetOpsVlansBeforeSwitchedVlanMembers(t *testing.T) {
	payload := []byte(`{
  "interface": [
    {"name": "Ethernet40", "openconfig-if-ethernet:ethernet": {"openconfig-vlan:switched-vlan": {"config": {"access-vlan": 2010}}}},
    {"name": "PortChannel52", "openconfig-if-aggregate:aggregation": {"openconfig-vlan:switched-vlan": {"config": {"access-vlan": 2010}}}},
    {"name": "Vlan2010", "config": {"name": "Vlan2010"}},
    {"name": "Loopback0", "config": {"name": "Loopback0"}},
    {"name": "Vlan2011", "config": {"name": "Vlan2011"}}
  ]
}`)
	ops, _, err := expandSetOps("/openconfig-interfaces:interfaces", payload, UPDATE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	var got []string
	for _, op := range ops {
		got = append(got, strings.TrimPrefix(op.Path, "/openconfig-interfaces:interfaces/interface"))
	}
	want := []string{"[name=Vlan2010]", "[name=Vlan2011]", "[name=PortChannel52]", "[name=Ethernet40]", "[name=Loopback0]"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("op order = %v, want %v", got, want)
	}
}

func TestExpandSetOpsLacpListOrderUnchanged(t *testing.T) {
	payload := []byte(`{"interfaces":{"interface":[{"name":"PortChannel20"},{"name":"PortChannel10"}]}}`)
	ops, _, err := expandListInstances("/openconfig-lacp:lacp", payload)
	if err != nil {
		t.Fatalf("expandListInstances: %v", err)
	}
	if len(ops) != 2 || !strings.HasSuffix(ops[0].Path, "[name=PortChannel20]") {
		t.Errorf("non-interfaces lists must keep payload order, got %+v", ops)
	}
}

func TestExpandSetOpsLacpNestedList(t *testing.T) {
	payload := []byte(`{
  "interfaces": {
    "interface": [
      {"name": "PortChannel10", "config": {"name": "PortChannel10"}}
    ]
  }
}`)
	ops, changed, err := expandListInstances("/openconfig-lacp:lacp", payload)
	if err != nil {
		t.Fatalf("expandListInstances: %v", err)
	}
	if !changed {
		t.Fatal("expected nested list expansion")
	}
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	want := "/openconfig-lacp:lacp/interfaces/interface[name=PortChannel10]"
	if ops[0].Path != want {
		t.Errorf("path = %s, want %s", ops[0].Path, want)
	}
}

func TestExpandSetOpsKeyedEthernetWithSubinterfacesUnchanged(t *testing.T) {
	path := "/openconfig-interfaces:interfaces/interface[name=Ethernet52]"
	payload := []byte(`{
  "openconfig-interfaces:config": {
    "description": "dutate-2",
    "name": "Ethernet52",
    "type": "iana-if-type:ethernetCsmacd"
  },
  "openconfig-interfaces:name": "Ethernet52",
  "openconfig-interfaces:subinterfaces": {
    "subinterface": [
      {
        "config": {"index": 0},
        "index": 0,
        "openconfig-if-ip:ipv4": {
          "addresses": {
            "address": [
              {"config": {"ip": "10.1.2.253", "prefix-length": 30}, "ip": "10.1.2.253"}
            ]
          }
        }
      }
    ]
  }
}`)
	ops, needBulk, err := expandSetOps(path, payload, UPDATE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	if needBulk {
		t.Fatal("keyed interface SET must not Bulk-expand nested lists")
	}
	if len(ops) != 1 || ops[0].Path != path {
		t.Fatalf("ops = %+v, want passthrough of %s", ops, path)
	}
	if !strings.Contains(string(ops[0].Payload), `"subinterface"`) {
		t.Errorf("nested subinterface list was stripped: %s", ops[0].Payload)
	}
}

func TestExpandSetOpsKeyedSubinterfacesContainerUnchanged(t *testing.T) {
	path := "/openconfig-interfaces:interfaces/interface[name=Ethernet52]/subinterfaces"
	payload := []byte(`{
  "subinterface": [
    {"index": 0, "config": {"index": 0}}
  ]
}`)
	ops, changed, err := expandListInstances(path, payload)
	if err != nil {
		t.Fatalf("expandListInstances: %v", err)
	}
	if changed {
		t.Fatal("SET under a keyed parent must not expand nested lists")
	}
	if len(ops) != 1 || ops[0].Path != path {
		t.Fatalf("ops = %+v, want passthrough of %s", ops, path)
	}
}

func TestExpandSetOpsLldpUnchanged(t *testing.T) {
	payload := []byte(`{"config":{"enabled":true}}`)
	ops, needBulk, err := expandSetOps("/openconfig-lldp:lldp", payload, UPDATE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	if needBulk {
		t.Fatal("did not expect Bulk for leaf-only payload")
	}
	if len(ops) != 1 || ops[0].Path != "/openconfig-lldp:lldp" {
		t.Fatalf("ops = %+v", ops)
	}
}

func TestExpandSetOpsMissingListKey(t *testing.T) {
	payload := []byte(`{"interface":[{"config":{"name":"PortChannel100"}}]}`)
	_, _, err := expandListInstances("/openconfig-interfaces:interfaces", payload)
	if err == nil {
		t.Fatal("expected error for missing list key")
	}
	if !strings.Contains(err.Error(), "missing key leaf") {
		t.Errorf("error %q, want missing key leaf", err)
	}
}

func TestExpandSetOpsCreateSkipsListExpansion(t *testing.T) {
	payload := []byte(`{"openconfig-interfaces:interfaces":{"interface":[{"name":"PortChannel100"}]}}`)
	ops, needBulk, err := expandSetOps("/", payload, CREATE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	if needBulk {
		t.Fatal("CREATE should not Bulk-expand a single module key")
	}
	if len(ops) != 1 || ops[0].Path != "/openconfig-interfaces:interfaces" {
		t.Fatalf("CREATE should stay at the container, got %+v", ops)
	}
}

func TestExpandSetOpsReplaceSkipsListExpansion(t *testing.T) {
	payload := []byte(`{
  "interface": [
    {"name": "Ethernet46", "config": {"name": "Ethernet46"}},
    {"name": "PortChannel100", "config": {"name": "PortChannel100"}}
  ]
}`)
	ops, needBulk, err := expandSetOps("/openconfig-interfaces:interfaces", payload, REPLACE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	if needBulk {
		t.Fatal("REPLACE should not Bulk-expand list members")
	}
	if len(ops) != 1 || ops[0].Path != "/openconfig-interfaces:interfaces" {
		t.Fatalf("REPLACE should stay at the container, got %+v", ops)
	}
	if !strings.Contains(string(ops[0].Payload), `"Ethernet46"`) ||
		!strings.Contains(string(ops[0].Payload), `"PortChannel100"`) {
		t.Errorf("REPLACE payload lost list members: %s", ops[0].Payload)
	}

	updateOps, updateBulk, err := expandSetOps("/openconfig-interfaces:interfaces", payload, UPDATE)
	if err != nil {
		t.Fatalf("UPDATE expandSetOps: %v", err)
	}
	if !updateBulk || len(updateOps) != 2 {
		t.Fatalf("UPDATE should still expand to 2 keyed ops, got needBulk=%v ops=%+v", updateBulk, updateOps)
	}
}

func TestExpandSetOpsReplaceLoggingSkipsRemoteServers(t *testing.T) {
	payload := []byte(`{"remote-servers":{"remote-server":[
		{"host":"203.0.113.1","config":{"host":"203.0.113.1","remote-port":6514}},
		{"host":"203.0.113.2","config":{"host":"203.0.113.2","remote-port":6514}}
	]}}`)
	path := "/openconfig-system:system/logging"
	ops, needBulk, err := expandSetOps(path, payload, REPLACE)
	if err != nil {
		t.Fatalf("expandSetOps: %v", err)
	}
	if needBulk {
		t.Fatal("REPLACE /system/logging must not split remote-server list")
	}
	if len(ops) != 1 || ops[0].Path != path {
		t.Fatalf("REPLACE should stay at %s, got %+v", path, ops)
	}
}

func TestRootSetUnMarshallAfterDerive(t *testing.T) {
	payload := []byte(`{"openconfig-lldp:lldp":{"config":{"enabled":true}}}`)
	path, inner, err := resolveRootSetRequest("/", payload)
	if err != nil {
		t.Fatalf("resolveRootSetRequest: %v", err)
	}
	appRootType := reflect.TypeOf(ocbinds.Device{})
	_, workObj, err := getRequestBinder(&path, &inner, UPDATE, &appRootType).unMarshall()
	if err != nil {
		t.Fatalf("unMarshall after derive: %v", err)
	}
	lldp, ok := (*workObj).(*ocbinds.OpenconfigLldp_Lldp)
	if !ok {
		t.Fatalf("workObj type %T, want *ocbinds.OpenconfigLldp_Lldp", *workObj)
	}
	if lldp.Config == nil || lldp.Config.Enabled == nil || !*lldp.Config.Enabled {
		t.Errorf("lldp.config.enabled not set from payload: %+v", lldp.Config)
	}
}

// Responses to the parts of one expanded entry fold back onto that entry,
// keeping its first error; a transaction that stopped partway answers only
// the entries it reached.
func TestCollapseBulkResponse(t *testing.T) {
	errA := tlerr.InvalidArgs("part failed")
	sent := []BulkRequestEntry{{Operation: UPDATE}, {Operation: REPLACE}}
	origin := []int{0, 0, 1}
	part := func(err error) BulkResponseEntry {
		return BulkResponseEntry{Operation: UPDATE, Entry: SetResponse{Err: err}}
	}
	for _, tc := range []struct {
		name  string
		parts []BulkResponseEntry
		want  []BulkResponseEntry
	}{
		{"every part succeeds", []BulkResponseEntry{part(nil), part(nil), part(nil)},
			[]BulkResponseEntry{{Operation: UPDATE}, {Operation: REPLACE}}},
		{"the second part of the first entry fails",
			[]BulkResponseEntry{part(nil), part(errA)},
			[]BulkResponseEntry{{Operation: UPDATE, Entry: SetResponse{Err: errA}}}},
		{"the first part fails before the rest run",
			[]BulkResponseEntry{part(errA)},
			[]BulkResponseEntry{{Operation: UPDATE, Entry: SetResponse{Err: errA}}}},
		{"the second entry fails",
			[]BulkResponseEntry{part(nil), part(nil), part(errA)},
			[]BulkResponseEntry{{Operation: UPDATE}, {Operation: REPLACE, Entry: SetResponse{Err: errA}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := collapseBulkResponse(BulkResponse{Response: tc.parts}, sent, origin)
			if !reflect.DeepEqual(got.Response, tc.want) {
				t.Errorf("got %+v, want %+v", got.Response, tc.want)
			}
		})
	}
}

const rootSetBulkRpolPayload = `{"defined-sets":{"prefix-sets":{"prefix-set":[{"name":"R7-SET","config":{"name":"R7-SET","mode":"IPV4"},` +
	`"prefixes":{"prefix":[{"ip-prefix":"198.51.100.7/32","masklength-range":"exact","config":{"ip-prefix":"198.51.100.7/32","masklength-range":"exact"}}]}}]}},` +
	`"policy-definitions":{"policy-definition":[{"name":"R7-POL","config":{"name":"R7-POL"},"statements":{"statement":[{"name":"10","config":{"name":"10"},` +
	`"conditions":{"match-prefix-set":{"config":{"prefix-set":"R7-SET"}}},"actions":{"config":{"policy-result":"ACCEPT_ROUTE"}}}]}}]}}`

func rootSetBulkCleanup() {
	for _, p := range []string{
		"/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=R7-POL]",
		"/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=R7-SET]",
		"/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=R7-SET2]",
	} {
		Delete(SetRequest{Path: p})
	}
}

// A bulk SET whose first update translib expands into a prefix-set and a
// policy is still answered with one response per update sent.
func TestBulkAnswersEachUpdateSent(t *testing.T) {
	rootSetBulkCleanup()
	defer rootSetBulkCleanup()
	req := BulkRequest{Request: []BulkRequestEntry{
		{Entry: SetRequest{Path: "/openconfig-routing-policy:routing-policy", Payload: []byte(rootSetBulkRpolPayload)}, Operation: UPDATE},
		{Entry: SetRequest{Path: "/openconfig-routing-policy:routing-policy/defined-sets/prefix-sets/prefix-set[name=R7-SET2]/config",
			Payload: []byte(`{"name":"R7-SET2","mode":"IPV4"}`)}, Operation: UPDATE},
	}}
	resp, err := Bulk(req)
	if err != nil {
		t.Fatalf("Bulk: %v", err)
	}
	if len(resp.Response) != len(req.Request) {
		t.Fatalf("got %d responses to %d updates", len(resp.Response), len(req.Request))
	}
	for k, r := range resp.Response {
		if r.Entry.Err != nil || r.Operation != UPDATE {
			t.Errorf("response %d = %+v, want a successful UPDATE", k, r)
		}
	}
}

// A root SET writes routing-policy before the modules that reference it,
// keeping the payload's order for the rest.
func TestDeriveSetOpsFromPayloadRoutingPolicyFirst(t *testing.T) {
	payload := []byte(`{"openconfig-lldp:lldp":{"config":{"enabled":true}},` +
		`"openconfig-network-instance:network-instances":{"network-instance":[{"name":"default"}]},` +
		`"openconfig-routing-policy:routing-policy":{"policy-definitions":{}}}`)
	ops, err := deriveSetOpsFromPayload(payload)
	if err != nil {
		t.Fatalf("deriveSetOpsFromPayload: %v", err)
	}
	var got []string
	for _, op := range ops {
		got = append(got, op.Path)
	}
	want := []string{"/openconfig-routing-policy:routing-policy", "/openconfig-lldp:lldp", "/openconfig-network-instance:network-instances"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

const rootSetPgPolicyPayload = `{"openconfig-network-instance:network-instances":{"network-instance":[{"name":"default","config":{"name":"default"},` +
	`"protocols":{"protocol":[{"identifier":"openconfig-policy-types:BGP","name":"BGP","config":{"identifier":"openconfig-policy-types:BGP","name":"BGP"},` +
	`"bgp":{"global":{"config":{"as":65501,"router-id":"192.0.2.1"}},"peer-groups":{"peer-group":[{"peer-group-name":"BGP-PEER-GROUP1",` +
	`"config":{"peer-group-name":"BGP-PEER-GROUP1"},"afi-safis":{"afi-safi":[{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST",` +
	`"config":{"afi-safi-name":"openconfig-bgp-types:IPV4_UNICAST","enabled":true},` +
	`"apply-policy":{"config":{"import-policy":["PERMIT-ALL"],"export-policy":["PERMIT-ALL"]}}}]}}]}}}]}}]},` +
	`"openconfig-routing-policy:routing-policy":{"policy-definitions":{"policy-definition":[{"name":"PERMIT-ALL","config":{"name":"PERMIT-ALL"},` +
	`"statements":{"statement":[{"name":"20","config":{"name":"20"},"actions":{"config":{"policy-result":"ACCEPT_ROUTE"}}}]}}]}}}`

func rootSetPgPolicyCleanup() {
	Delete(SetRequest{Path: "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=BGP]"})
	Delete(SetRequest{Path: "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=PERMIT-ALL]"})
}

// A root SET naming network-instances before the routing-policy its
// peer-group applies is accepted: the policy is written first.
func TestRootSetPolicyAppliedBeforeItIsNamed(t *testing.T) {
	rootSetPgPolicyCleanup()
	defer rootSetPgPolicyCleanup()
	if _, err := Update(SetRequest{Path: "/", Payload: []byte(rootSetPgPolicyPayload)}); err != nil {
		t.Fatalf("root SET with network-instances first: %v", err)
	}
	d := getConfigDb()
	if d == nil {
		t.Fatal("no CONFIG_DB")
	}
	defer d.DeleteDB()
	if _, err := d.GetEntry(&db.TableSpec{Name: "ROUTE_MAP_SET"}, db.Key{Comp: []string{"PERMIT-ALL"}}); err != nil {
		t.Errorf("ROUTE_MAP_SET|PERMIT-ALL: %v", err)
	}
	if _, err := d.GetEntry(&db.TableSpec{Name: "BGP_PEER_GROUP_AF"}, db.Key{Comp: []string{"default", "BGP-PEER-GROUP1", "ipv4_unicast"}}); err != nil {
		t.Errorf("BGP_PEER_GROUP_AF|default|BGP-PEER-GROUP1|ipv4_unicast: %v", err)
	}
}
