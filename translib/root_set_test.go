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
		{"multiple keys", []byte(`{"openconfig-lldp:lldp":{},"openconfig-acl:acl":{}}`), "multiple top-level keys"},
		{"unknown key", []byte(`{"not-a-yang-module:foo":{}}`), "cannot derive app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := deriveSetPathFromPayload(tt.payload)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q, want substring %q", err, tt.want)
			}
		})
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
