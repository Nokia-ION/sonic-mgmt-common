// Subscribe and path transformer tests for the static routes. These live in
// the internal package, like the AFT and routing-policy ones, because
// XfmrSubscInParams cannot be built from outside it.
//
// Two separate things have to hold for a subscription to work, and each was a
// separate bug elsewhere in this repo. The callback has to map the path to a
// table, or translib treats the path as non-DB: collectNotificationPreferences
// then clears both on-change and wildcard support, and validateNonDbPath
// refuses a wildcard key outright. And the path transformer has to turn a row
// key back into yang keys, or a subscription whose key translib must supply is
// accepted and then reports nothing.

package transformer

import (
	"errors"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
)

const niSrSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
	"/protocols/protocol[identifier=STATIC][name=static]"

func TestSubscribe_ni_static_route(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		reqUri   string // empty means the same as uri
		onChange bool
		keyPatt  string
		wantTag  bool // the tag field mapped to the set-tag leaf
	}{
		{
			name:     "the route list",
			uri:      niSrSubscUrl + "/static-routes/static",
			onChange: true,
			keyPatt:  "default|*",
		},
		{
			name:     "a wildcard prefix",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=*]",
			onChange: true,
			keyPatt:  "default|*",
		},
		{
			name:     "a keyed route",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]",
			onChange: true,
			keyPatt:  "default|10.1.0.0/16",
		},
		{
			// The path the whole increment is for. A changed tag is reported
			// only because the field is named here; the prefix needs no
			// mapping, being the key the row itself carries.
			name:     "the state container of a keyed route",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/state",
			onChange: true,
			keyPatt:  "default|10.1.0.0/16",
			wantTag:  true,
		},
		{
			// What really arrives for a subscription on state/prefix: the
			// subscribe path trims the leaf, so the callback is anchored on
			// state while requestURI still names the leaf. Deciding from
			// requestURI refused this exact path on hardware while every
			// same-uri test above passed.
			name:     "state/prefix, anchored on state as the framework does it",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/state",
			reqUri:   niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/state/prefix",
			onChange: true,
			keyPatt:  "default|10.1.0.0/16",
			wantTag:  true,
		},
		{
			// And a caller that hands over the whole path untrimmed is asking
			// about the same node.
			name:     "state/prefix given whole",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/state/prefix",
			onChange: true,
			keyPatt:  "default|10.1.0.0/16",
			wantTag:  true,
		},
		{
			name:     "state/set-tag given whole",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/state/set-tag",
			onChange: true,
			keyPatt:  "default|10.1.0.0/16",
			wantTag:  true,
		},
		{
			name:     "the static-routes container",
			uri:      niSrSubscUrl + "/static-routes",
			onChange: true,
			keyPatt:  "default|*",
		},
		{
			// Next-hops are a list this table does not decompose, so the path
			// samples rather than being accepted and then reporting nothing.
			name:     "below the route, at the next-hops",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/next-hops",
			onChange: false,
			keyPatt:  "default|10.1.0.0/16",
		},
		{
			// Next-hop state is still one applied row per route, so a change to
			// any next-hop field is reported at route granularity.
			name:     "next-hop state container",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/next-hops/next-hop[index=10.1.1.1]/state",
			onChange: true,
			keyPatt:  "default|10.1.0.0/16",
		},
		{
			name:     "next-hop state/next-hop, anchored on state",
			uri:      niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/next-hops/next-hop[index=10.1.1.1]/state",
			reqUri:   niSrSubscUrl + "/static-routes/static[prefix=10.1.0.0/16]/next-hops/next-hop[index=10.1.1.1]/state/next-hop",
			onChange: true,
			keyPatt:  "default|10.1.0.0/16",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqUri := tc.reqUri
			if reqUri == "" {
				reqUri = tc.uri
			}
			out, err := Subscribe_ni_static_route_cfg_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: reqUri,
				subscProc:  TRANSLATE_SUBSCRIBE,
				dbDataMap:  make(RedisDbMap),
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_static_route_cfg_xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a table mapping, got a virtual table")
			}
			tbls, ok := out.dbDataMap[db.StateDB]
			if !ok {
				t.Fatalf("expected a STATE_DB mapping, got %v", out.dbDataMap)
			}
			keys, ok := tbls["STATIC_ROUTE_APPLIED"]
			if !ok {
				t.Fatalf("expected the applied table, got %v", tbls)
			}
			fields, ok := keys[tc.keyPatt]
			if !ok {
				t.Fatalf("expected key pattern %q, got %v", tc.keyPatt, keys)
			}
			if _, named := fields["tag"]; named != tc.wantTag {
				t.Errorf("tag field mapped = %v, want %v (fields %v)", named, tc.wantTag, fields)
			}
			if (out.onChange == OnchangeEnable) != tc.onChange {
				t.Errorf("onChange = %v, want %v", out.onChange, tc.onChange)
			}
		})
	}
}

// Everything outside the static routes keeps the behaviour it has today, so
// widening it later is a deliberate act rather than a side effect.
func TestSubscribe_ni_static_route_rejects_unsupported_protocol_name(t *testing.T) {
	const badUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]" +
		"/protocols/protocol[identifier=STATIC][name=DEFAULT]/static-routes/static"
	out, err := Subscribe_ni_static_route_cfg_xfmr(XfmrSubscInParams{
		uri:        badUrl,
		requestURI: badUrl,
		subscProc:  TRANSLATE_SUBSCRIBE,
		dbDataMap:  make(RedisDbMap),
	})
	if err != nil {
		t.Fatalf("Subscribe_ni_static_route_cfg_xfmr failed: %v", err)
	}
	if !out.isVirtualTbl {
		t.Errorf("expected no mapping for unsupported protocol instance name, got %v", out.dbDataMap)
	}
}

func TestSubscribe_ni_static_route_other_paths_report_no_mapping(t *testing.T) {
	for _, uri := range []string{
		"/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]",
		"/openconfig-network-instance:network-instances/network-instance[name=default]/table-connections",
		"/openconfig-network-instance:network-instances",
	} {
		t.Run(uri, func(t *testing.T) {
			out, err := Subscribe_ni_static_route_cfg_xfmr(XfmrSubscInParams{
				uri:        uri,
				requestURI: uri,
				subscProc:  TRANSLATE_SUBSCRIBE,
				dbDataMap:  make(RedisDbMap),
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_static_route_cfg_xfmr failed: %v", err)
			}
			if !out.isVirtualTbl {
				t.Errorf("expected no mapping, got %v", out.dbDataMap)
			}
		})
	}
}

func TestDbToYangPath_ni_static_route(t *testing.T) {
	const niList = "/openconfig-network-instance:network-instances/network-instance"
	routeList := niList + "/protocols/protocol/static-routes/static"

	tests := []struct {
		name          string
		schemaPath    string
		tblName       string
		keyComp       []string
		wantNi        string
		wantPrefix    string
		wantErr       bool
		wantUnrelated bool
	}{
		{
			name:       "a route names its instance and prefix",
			schemaPath: routeList,
			keyComp:    []string{"default", "10.1.0.0/16"},
			wantNi:     "default",
			wantPrefix: "10.1.0.0/16",
		},
		{
			name:       "a route of another instance",
			schemaPath: routeList,
			keyComp:    []string{"Vrf1", "10.2.0.0/16"},
			wantNi:     "Vrf1",
			wantPrefix: "10.2.0.0/16",
		},
		{
			// An IPv6 prefix carries colons, which must not be mistaken for a
			// key separator: the producer writes the prefix as one component.
			name:       "an ipv6 prefix stays one component",
			schemaPath: routeList,
			keyComp:    []string{"default", "fc00:50::/64"},
			wantNi:     "default",
			wantPrefix: "fc00:50::/64",
		},
		{
			// A node below the list inherits the path transformer, so it is
			// called with the leaf on the schema path and still has to answer
			// with the key paths of the lists holding the keys.
			name:       "the state leaf below the route inherits it",
			schemaPath: routeList + "/state/prefix",
			keyComp:    []string{"default", "10.1.0.0/16"},
			wantNi:     "default",
			wantPrefix: "10.1.0.0/16",
		},
		{
			name:       "a path above the route names only the instance",
			schemaPath: niList + "/protocols/protocol/static-routes",
			keyComp:    []string{"default", "10.1.0.0/16"},
			wantNi:     "default",
		},
		{
			name:          "another table is declined as unrelated",
			schemaPath:    routeList,
			tblName:       "STATIC_ROUTE",
			keyComp:       []string{"default", "10.1.0.0/16"},
			wantErr:       true,
			wantUnrelated: true,
		},
		{
			// A key missing its instance is not a row of this table, so it
			// stays a fault and keeps its warning.
			name:       "a key without the instance is a fault",
			schemaPath: routeList,
			keyComp:    []string{"10.1.0.0/16"},
			wantErr:    true,
		},
		{
			name:       "an empty prefix is a fault",
			schemaPath: routeList,
			keyComp:    []string{"default", ""},
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tblName := tc.tblName
			if tblName == "" {
				tblName = "STATIC_ROUTE_APPLIED"
			}
			keys := make(map[string]string)
			err := DbToYangPath_ni_static_route_path_xfmr(XfmrDbToYgPathParams{
				ygSchemaPath: tc.schemaPath,
				tblName:      tblName,
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
				t.Fatalf("DbToYangPath_ni_static_route_path_xfmr failed: %v", err)
			}
			if got := keys[niList+"/name"]; got != tc.wantNi {
				t.Errorf("instance key = %q, want %q; all keys %v", got, tc.wantNi, keys)
			}
			if got := keys[routeList+"/prefix"]; got != tc.wantPrefix {
				t.Errorf("prefix key = %q, want %q; all keys %v", got, tc.wantPrefix, keys)
			}
		})
	}
}

// Declaring a callback is not enough: translib reaches every transformer
// callback through the XlateFuncs registry, keyed by name, and an unbound one
// is dead code that the tests above would still pass.
func TestSubscribe_ni_static_route_callbacks_are_registered(t *testing.T) {
	for _, name := range []string{
		"YangToDb_ni_static_route_cfg_xfmr",
		"DbToYang_ni_static_route_cfg_xfmr",
		"Subscribe_ni_static_route_cfg_xfmr",
		"DbToYangPath_ni_static_route_path_xfmr",
	} {
		if !IsXlateFuncBinded(name) {
			t.Errorf("%s is not bound in XlateFuncs, so translib will never call it", name)
		}
	}
}
