////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//     http://www.apache.org/licenses/LICENSE-2.0                             //
//                                                                            //
//  Unless required by applicable law or agreed to in writing, software       //
//  distributed under the License is distributed on an "AS IS" BASIS,         //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  //
//  See the License for the specific language governing permissions and       //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

// Subscribe callback tests for the AFT subtrees. These live in the internal
// package, unlike the GET tests in xfmr_ni_afts_test.go, because
// XfmrSubscInParams cannot be built from outside it.
//
// What these pin down is that every mapped AFT path is answered with a real
// table mapping and a sampling preference, rather than being refused. A refused
// path is not only unusable under STREAM: it is the difference between a client
// getting data and getting an error, and the failure is invisible to a GET test.
// ONCE and POLL do not consult the on-change preference at all - the gNMI layer
// serves both through the same sampling path as a GET - so a path that resolves
// here to sampling supports ONCE, SAMPLE and POLL, and refuses only ON_CHANGE.

package transformer

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

const niAftsSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/afts"

func TestSubscribe_ni_afts_entries_paths(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		// The IPv6 cases are the ones being added. They share the callback with
		// IPv4, so they cannot fail on the family today; they are here so that a
		// later change giving one family its own table or key cannot silently
		// leave the other refused.
		{
			name: "ipv6 unicast container",
			uri:  niAftsSubscUrl + "/ipv6-unicast",
		},
		{
			name: "ipv6 entry list unkeyed",
			uri:  niAftsSubscUrl + "/ipv6-unicast/ipv6-entry",
		},
		{
			name: "ipv6 entry keyed",
			uri:  niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]",
		},
		{
			name: "ipv6 entry state container",
			uri:  niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]/state",
		},
		{
			name: "ipv4 entry keyed",
			uri:  niAftsSubscUrl + "/ipv4-unicast/ipv4-entry[prefix=20.50.0.0/16]",
		},
		{
			name: "ipv4 unicast container",
			uri:  niAftsSubscUrl + "/ipv4-unicast",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_afts_entries_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.uri,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_afts_entries_xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a non-virtual subscribe mapping, got map %v", out.dbDataMap)
			}
			if out.onChange != OnchangeDisable {
				t.Fatalf("onChange=%v, want %v", out.onChange, OnchangeDisable)
			}
			if out.nOpts == nil {
				t.Fatalf("nOpts not set, so the preferred type would default to on-change")
			}
			if out.nOpts.pType != Sample {
				t.Fatalf("pType=%v, want %v", out.nOpts.pType, Sample)
			}
			if _, ok := out.dbDataMap[db.ApplDB][niAftsRouteTbl]["*"]; !ok {
				t.Fatalf("missing %s[*]: %v", niAftsRouteTbl, out.dbDataMap)
			}
		})
	}
}

// The afts container fans out to every list below it on a GET, so its
// subscription has to register every table those lists read. Registering only
// one would leave a container subscription blind to changes in the other.
func TestSubscribe_ni_afts_container_registers_both_tables(t *testing.T) {
	out, err := Subscribe_ni_afts_xfmr(XfmrSubscInParams{
		uri:        niAftsSubscUrl,
		requestURI: niAftsSubscUrl,
		subscProc:  TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ni_afts_xfmr failed: %v", err)
	}
	if out.onChange != OnchangeDisable || out.nOpts == nil || out.nOpts.pType != Sample {
		t.Fatalf("afts container must be sampled, got onChange=%v nOpts=%v", out.onChange, out.nOpts)
	}
	for _, tbl := range []string{niAftsRouteTbl, niAftsNhGrpTbl} {
		if _, ok := out.dbDataMap[db.ApplDB][tbl]["*"]; !ok {
			t.Fatalf("missing %s[*]: %v", tbl, out.dbDataMap)
		}
	}
}

// Only the subscribe translation maps these tables. The other processing modes
// share the callback and must be told the subtree owns no table of its own.
func TestSubscribe_ni_afts_entries_non_translate_is_virtual(t *testing.T) {
	for _, proc := range []SubscProcType{TRANSLATE_EXISTS, PROCESS_SUBSCRIBE} {
		out, err := Subscribe_ni_afts_entries_xfmr(XfmrSubscInParams{
			uri:        niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]",
			requestURI: niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]",
			subscProc:  proc,
		})
		if err != nil {
			t.Fatalf("Subscribe_ni_afts_entries_xfmr failed for %v: %v", proc, err)
		}
		if !out.isVirtualTbl {
			t.Fatalf("expected a virtual table for %v, got map %v", proc, out.dbDataMap)
		}
		if len(out.dbDataMap) != 0 {
			t.Fatalf("expected no DB map for %v, got %v", proc, out.dbDataMap)
		}
	}
}
