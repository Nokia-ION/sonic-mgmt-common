// Subscribe callback tests for the routing-policy subtree. These live in the
// internal package, like the AFT ones, because XfmrSubscInParams cannot be
// built from outside it.
//
// What they pin down is that a policy-definition path is answered with a real
// table mapping rather than being refused. Before this callback existed the path
// was non-DB, and translib draws two conclusions from that in different places:
// collectNotificationPreferences clears IsOnChangeSupported and
// IsWildcardSupported, and validateNonDbPath refuses a wildcard key outright. So
// an on-change request was turned away as unsupported and a sampled one carrying
// a wildcard as a wildcard path, both for the one reason.
//
// ONCE and POLL do not consult the on-change preference: the gNMI layer serves
// them through the same sampling path as a GET. So a path resolving here to
// sampling still supports ONCE, SAMPLE and POLL, and refuses only ON_CHANGE.

package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

const rpolSubscUrl = "/openconfig-routing-policy:routing-policy"

// A policy definition is one ROUTE_MAP_SET_APPLIED row plus the
// ROUTE_MAP_APPLIED rows of its statements, both in STATE_DB, so both are
// watched, and a named definition narrows the
// patterns to that name rather than waking the subscription for every policy.
func TestSubscribe_routing_policy_policy_definitions(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		onChange bool
		setPatt  string
		stmtPatt string
	}{
		{
			name:     "policy-definitions container",
			uri:      rpolSubscUrl + "/policy-definitions",
			onChange: true,
			setPatt:  "*",
			stmtPatt: "*",
		},
		{
			name:     "policy-definition list with the key omitted",
			uri:      rpolSubscUrl + "/policy-definitions/policy-definition",
			onChange: true,
			setPatt:  "*",
			stmtPatt: "*",
		},
		{
			name:     "policy-definition list with a wildcard key",
			uri:      rpolSubscUrl + "/policy-definitions/policy-definition[name=*]",
			onChange: true,
			setPatt:  "*",
			stmtPatt: "*",
		},
		{
			name:     "policy-definition named",
			uri:      rpolSubscUrl + "/policy-definitions/policy-definition[name=POL1]",
			onChange: true,
			setPatt:  "POL1",
			stmtPatt: "POL1|*",
		},
		{
			// Deeper nodes describe fields of a ROUTE_MAP row. A changed field is
			// reported only when it is mapped to a leaf directly below the anchored
			// node, so these sample rather than being accepted and reporting nothing.
			name:     "statement actions config policy-result",
			uri:      rpolSubscUrl + "/policy-definitions/policy-definition[name=POL1]/statements/statement[name=10]/actions/config/policy-result",
			onChange: false,
			setPatt:  "POL1",
			stmtPatt: "POL1|*",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_routing_policy_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.uri,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_routing_policy_xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a real table mapping, got a virtual table: %v", out.dbDataMap)
			}

			wantOnChange, wantPType := OnchangeDisable, Sample
			if tc.onChange {
				wantOnChange, wantPType = OnchangeEnable, OnChange
			}
			if out.onChange != wantOnChange {
				t.Fatalf("onChange=%v, want %v", out.onChange, wantOnChange)
			}
			if out.nOpts == nil {
				t.Fatalf("nOpts not set, so the preferred type would default to on-change")
			}
			if out.nOpts.pType != wantPType {
				t.Fatalf("pType=%v, want %v", out.nOpts.pType, wantPType)
			}

			if _, ok := out.dbDataMap[db.StateDB][rpolRouteMapSetAppliedTbl][tc.setPatt]; !ok {
				t.Fatalf("missing %s[%s]: %v", rpolRouteMapSetAppliedTbl, tc.setPatt, out.dbDataMap)
			}
			if _, ok := out.dbDataMap[db.StateDB][rpolRouteMapAppliedTbl][tc.stmtPatt]; !ok {
				t.Fatalf("missing %s[%s]: %v", rpolRouteMapAppliedTbl, tc.stmtPatt, out.dbDataMap)
			}
		})
	}
}

// The rest of the subtree this transformer is annotated on keeps the behaviour
// it had before the callback existed: no Redis mapping, so sampling only and a
// wildcard still refused. Asserted so that widening the callback later is a
// deliberate act rather than a side effect.
func TestSubscribe_routing_policy_other_subtrees_report_no_mapping(t *testing.T) {
	uris := []string{
		rpolSubscUrl + "/defined-sets/prefix-sets/prefix-set[name=PSET1]",
		rpolSubscUrl + "/defined-sets/openconfig-bgp-policy:bgp-defined-sets/community-sets/community-set[community-set-name=COMM1]",
		rpolSubscUrl,
	}
	for _, uri := range uris {
		t.Run(uri, func(t *testing.T) {
			out, err := Subscribe_routing_policy_xfmr(XfmrSubscInParams{
				uri:        uri,
				requestURI: uri,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_routing_policy_xfmr failed: %v", err)
			}
			if !out.isVirtualTbl {
				t.Fatalf("expected no Redis mapping, got %v", out.dbDataMap)
			}
		})
	}
}

// The existence check, which is what a CRUD request runs through this callback,
// must report no mapping whatever the uri. The three process types are
// TRANSLATE_EXISTS, TRANSLATE_SUBSCRIBE and PROCESS_SUBSCRIBE, and only the
// middle one is asking about a subscription.
func TestSubscribe_routing_policy_exists_check_reports_no_mapping(t *testing.T) {
	out, err := Subscribe_routing_policy_xfmr(XfmrSubscInParams{
		uri:        rpolSubscUrl + "/policy-definitions/policy-definition[name=POL1]",
		requestURI: rpolSubscUrl + "/policy-definitions/policy-definition[name=POL1]",
		subscProc:  TRANSLATE_EXISTS,
	})
	if err != nil {
		t.Fatalf("Subscribe_routing_policy_xfmr failed: %v", err)
	}
	if !out.isVirtualTbl {
		t.Fatalf("expected no mapping for an existence check, got %v", out.dbDataMap)
	}
}
