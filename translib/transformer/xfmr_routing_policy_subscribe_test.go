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
	"errors"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
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

// Declaring the callback is not enough on its own: translib reaches every
// transformer callback through the XlateFuncs registry, keyed by the name the
// annotation gives the subtree. Without the bind the callback is dead code, the
// path stays non-DB, and both refusals above come back even though the tests
// that call the callback directly still pass. So assert the registration
// alongside the behaviour.
func TestSubscribe_routing_policy_callback_is_registered(t *testing.T) {
	for _, name := range []string{
		"YangToDb_routing_policy_xfmr",
		"DbToYang_routing_policy_xfmr",
		"Subscribe_routing_policy_xfmr",
	} {
		if !IsXlateFuncBinded(name) {
			t.Errorf("%s is not bound in XlateFuncs, so translib will never call it", name)
		}
	}
}

// The path transformer is what lets a subscription report a row whose key
// translib has to supply rather than read from the request. A wildcard or
// omitted key is exactly that case, which is why it was accepted and then
// reported nothing until this existed.
func TestDbToYangPath_routing_policy(t *testing.T) {
	const polList = rpolSubscUrl + "/policy-definitions/policy-definition"
	stmtList := polList + "/statements/statement"

	tests := []struct {
		name       string
		schemaPath string
		tblName    string
		keyComp    []string
		wantPolicy string // empty means no policy key is expected
		wantStmt   string // empty means no statement key is expected
		wantErr    bool
		// A row of a table these lists do not serve has to be declined with
		// apis.ErrUnrelatedEntry, so the infrastructure drops the notification
		// without logging a warning for it.
		wantUnrelated bool
	}{
		{
			name:       "policy row names the policy",
			schemaPath: polList,
			tblName:    rpolRouteMapSetAppliedTbl,
			keyComp:    []string{"POL1"},
			wantPolicy: "POL1",
		},
		{
			// The statement rows feed a subscription anchored on the policy
			// too, and there the statement is not on the path to be named.
			name:       "statement row on the policy path names only the policy",
			schemaPath: polList,
			tblName:    rpolRouteMapAppliedTbl,
			keyComp:    []string{"POL1", "10"},
			wantPolicy: "POL1",
		},
		{
			name:       "statement row on the statement path names both",
			schemaPath: stmtList,
			tblName:    rpolRouteMapAppliedTbl,
			keyComp:    []string{"POL1", "10"},
			wantPolicy: "POL1",
			wantStmt:   "10",
		},
		{
			// A node below the list inherits the path transformer, so it is
			// called with the leaf on the schema path and still has to answer
			// with the key paths of the lists that hold the keys.
			name:       "leaf below the statement inherits it",
			schemaPath: stmtList + "/actions/state/policy-result",
			tblName:    rpolRouteMapAppliedTbl,
			keyComp:    []string{"POL1", "10"},
			wantPolicy: "POL1",
			wantStmt:   "10",
		},
		{
			// The key is a policy name, and a name may hold characters that
			// mean something to a path, so it is passed through as it stands.
			name:       "policy name is not interpreted",
			schemaPath: polList,
			tblName:    rpolRouteMapSetAppliedTbl,
			keyComp:    []string{"POL-1.2_X"},
			wantPolicy: "POL-1.2_X",
		},
		{
			name:       "a path above the list has no key to answer with",
			schemaPath: rpolSubscUrl + "/policy-definitions",
			tblName:    rpolRouteMapSetAppliedTbl,
			keyComp:    []string{"POL1"},
		},
		{
			name:          "another table is declined as unrelated",
			schemaPath:    polList,
			tblName:       "ROUTE_MAP",
			keyComp:       []string{"POL1"},
			wantErr:       true,
			wantUnrelated: true,
		},
		{
			// No key at all is not a row of the table, so it stays a fault and
			// keeps its warning rather than being dropped quietly.
			name:       "empty policy key is a fault",
			schemaPath: polList,
			tblName:    rpolRouteMapSetAppliedTbl,
			keyComp:    []string{},
			wantErr:    true,
		},
		{
			name:       "statement row missing its sequence is a fault",
			schemaPath: stmtList,
			tblName:    rpolRouteMapAppliedTbl,
			keyComp:    []string{"POL1"},
			wantErr:    true,
		},
		{
			name:       "statement row with an empty sequence is a fault",
			schemaPath: stmtList,
			tblName:    rpolRouteMapAppliedTbl,
			keyComp:    []string{"POL1", ""},
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys := make(map[string]string)
			err := DbToYangPath_routing_policy_path_xfmr(XfmrDbToYgPathParams{
				ygSchemaPath: tc.schemaPath,
				tblName:      tc.tblName,
				tblKeyComp:   tc.keyComp,
				ygPathKeys:   keys,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got keys %v", keys)
				}
				if got := errors.Is(err, apis.ErrUnrelatedEntry); got != tc.wantUnrelated {
					t.Errorf("errors.Is(err, apis.ErrUnrelatedEntry) = %v, want %v; err: %v",
						got, tc.wantUnrelated, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DbToYangPath_routing_policy_path_xfmr failed: %v", err)
			}

			polKey := keys[polList+"/name"]
			if polKey != tc.wantPolicy {
				t.Errorf("policy key = %q, want %q; all keys %v", polKey, tc.wantPolicy, keys)
			}
			stmtKey := keys[stmtList+"/name"]
			if stmtKey != tc.wantStmt {
				t.Errorf("statement key = %q, want %q; all keys %v", stmtKey, tc.wantStmt, keys)
			}
		})
	}
}

// The path transformer reaches translib the same way the subscribe callback
// does, by name through the registry, so it needs its own bind.
func TestDbToYangPath_routing_policy_is_registered(t *testing.T) {
	if !IsXlateFuncBinded("DbToYangPath_routing_policy_path_xfmr") {
		t.Error("DbToYangPath_routing_policy_path_xfmr is not bound in XlateFuncs, so translib will never call it")
	}
}
