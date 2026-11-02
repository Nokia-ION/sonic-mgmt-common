package translib

import (
	"encoding/json"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

func TestNormalizeIcmpv4IdentityPayload_leafType(t *testing.T) {
	payload := []byte(`"openconfig-icmpv4-types:ECHO"`)
	uri := "/openconfig-acl:acl/acl-sets/acl-set[name=ACL-12-IPV4][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=32820]/ipv4/icmpv4/config/type"
	if err := normalizeIcmpv4IdentityPayload(uri, &payload); err != nil {
		t.Fatal(err)
	}
	var n json.Number
	if err := json.Unmarshal(payload, &n); err != nil {
		t.Fatalf("expected numeric payload, got %q: %v", string(payload), err)
	}
	if n.String() != "8" {
		t.Fatalf("ECHO => 8, got %s", n.String())
	}
}

func TestNormalizeIcmpv4IdentityPayload_nestedConfig(t *testing.T) {
	payload := []byte(`{"openconfig-acl-ext:icmpv4":{"config":{"type":"openconfig-icmpv4-types:DST_UNREACHABLE","code":"openconfig-icmpv4-types:DST_UNREACHABLE_NET"}}}`)
	uri := "/openconfig-acl:acl/acl-sets/acl-set[name=x][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=1]"
	if err := normalizeIcmpv4IdentityPayload(uri, &payload); err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatal(err)
	}
	icmpv4 := root["openconfig-acl-ext:icmpv4"].(map[string]interface{})
	cfg := icmpv4["config"].(map[string]interface{})
	if cfg["type"].(float64) != 3 {
		t.Fatalf("type want 3 got %v", cfg["type"])
	}
	if cfg["code"].(float64) != 0 {
		t.Fatalf("code want 0 got %v", cfg["code"])
	}
}

func TestOcAclSetNameFromTable(t *testing.T) {
	entry := db.Value{Field: map[string]string{ACL_OPENCONFIG_SET_NAME: "ACL-NAMED-V4"}}
	if got := ocAclSetNameFromTable(entry, "ACL_NAMED_V4_ACL_IPV4", SONIC_ACL_TYPE_IPV4); got != "ACL-NAMED-V4" {
		t.Fatalf("got %q", got)
	}
}
