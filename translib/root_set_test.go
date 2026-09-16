// Copyright 2026 Nokia.

package translib

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
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
	got, err := expandBulkRootSet(req)
	if err != nil {
		t.Fatalf("expandBulkRootSet: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
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
