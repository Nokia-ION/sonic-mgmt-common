package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
)

// TestHealthzStatusFromFlags verifies the FAULT_INFO-empty fallback: a present
// component reporting status=false is UNHEALTHY, everything else is HEALTHY.
func TestHealthzStatusFromFlags(t *testing.T) {
	healthy := ocbinds.OpenconfigPlatform_Components_Component_Healthz_State_Status_HEALTHY
	unhealthy := ocbinds.OpenconfigPlatform_Components_Component_Healthz_State_Status_UNHEALTHY

	cases := []struct {
		name     string
		presence string
		status   string
		want     ocbinds.E_OpenconfigPlatform_Components_Component_Healthz_State_Status
	}{
		{"present and failed (PSU1)", "true", "false", unhealthy},
		{"present and ok (PSU2)", "true", "true", healthy},
		{"present and failed capitalized", "True", "False", unhealthy},
		{"present and ok capitalized", "True", "True", healthy},
		{"absent", "false", "false", healthy},
		{"presence unknown", "", "false", healthy},
		{"present, status unknown", "true", "", unhealthy},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := healthzStatusFromFlags(c.presence, c.status); got != c.want {
				t.Errorf("healthzStatusFromFlags(%q, %q)=%v, want %v", c.presence, c.status, got, c.want)
			}
		})
	}
}

func TestIsDbHealthFlagTrue(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"True", true},
		{"false", false},
		{"False", false},
		{"", false},
		{"1", false},
	}
	for _, c := range cases {
		if got := isDbHealthFlagTrue(c.in); got != c.want {
			t.Errorf("isDbHealthFlagTrue(%q)=%v, want %v", c.in, got, c.want)
		}
	}
}
