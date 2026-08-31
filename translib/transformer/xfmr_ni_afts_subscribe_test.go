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
// table mapping rather than being refused. A refused path is not only unusable
// under STREAM: it is the difference between a client getting data and getting
// an error, and the failure is invisible to a GET test.
//
// ONCE and POLL do not consult the on-change preference at all - the gNMI layer
// serves both through the same sampling path as a GET - so a path that resolves
// here to sampling supports ONCE, SAMPLE and POLL, and refuses only ON_CHANGE.
// The unicast entry lists resolve to on-change instead, which they can because
// the prefix they are keyed on is the ROUTE_TABLE key and the path transformer
// recovers it from a notification.

package transformer

import (
	"strings"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

const niAftsSubscUrl = "/openconfig-network-instance:network-instances/network-instance[name=default]/afts"

func TestSubscribe_ni_afts_entries_paths(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		onChange bool
	}{
		// The IPv6 cases share the callback with IPv4, so they cannot fail on
		// the family today; they are here so that a later change giving one
		// family its own table or key cannot silently leave the other refused.
		{
			// The container is above the entry list, so a notification on it
			// names no prefix and it stays on sampling.
			name: "ipv6 unicast container",
			uri:  niAftsSubscUrl + "/ipv6-unicast",
		},
		{
			name:     "ipv6 entry list unkeyed",
			uri:      niAftsSubscUrl + "/ipv6-unicast/ipv6-entry",
			onChange: true,
		},
		{
			name:     "ipv6 entry list wildcard key",
			uri:      niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=*]",
			onChange: true,
		},
		{
			name:     "ipv6 entry keyed",
			uri:      niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]",
			onChange: true,
		},
		{
			name:     "ipv6 entry state container",
			uri:      niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]/state",
			onChange: true,
		},
		{
			name:     "ipv6 entry state next-hop-group leaf",
			uri:      niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]/state/next-hop-group",
			onChange: true,
		},
		{
			name:     "ipv4 entry keyed",
			uri:      niAftsSubscUrl + "/ipv4-unicast/ipv4-entry[prefix=20.50.0.0/16]",
			onChange: true,
		},
		{
			name:     "ipv4 entry prefix leaf",
			uri:      niAftsSubscUrl + "/ipv4-unicast/ipv4-entry[prefix=20.50.0.0/16]/prefix",
			onChange: true,
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
			if _, ok := out.dbDataMap[db.ApplDB][niAftsRouteTbl]["*"]; !ok {
				t.Fatalf("missing %s[*]: %v", niAftsRouteTbl, out.dbDataMap)
			}
		})
	}
}

// next-hop-group is the one entry leaf fed by a row field rather than by the
// key, so it is reported on change only if the field is mapped to it by name.
// Only a leaf directly below the anchor can be named, and the subscribe path
// anchors a leaf request on the container above it, so a request on the state
// container and one on a leaf under it both name the leaf plainly, while a
// request anchored on the entry list cannot name it at all. Getting this wrong
// does not fail a GET or a sample: the leaf simply never appears in a change
// notification, or a mapping is declared that never resolves.
func TestSubscribe_ni_afts_entries_next_hop_group_field_map(t *testing.T) {
	entry := niAftsSubscUrl + "/ipv4-unicast/ipv4-entry[prefix=20.50.0.0/16]"
	tests := []struct {
		name string
		// uri is what the subscribe path hands the callback, which is the
		// request path with a trailing leaf trimmed off.
		uri        string
		requestURI string
		// want is the leaf the nexthop_group field maps to, empty when the
		// anchor cannot name it.
		want string
	}{
		{
			name:       "anchored on the entry list, cannot name the leaf",
			uri:        niAftsSubscUrl + "/ipv4-unicast/ipv4-entry",
			requestURI: niAftsSubscUrl + "/ipv4-unicast/ipv4-entry",
			want:       "",
		},
		{
			name:       "anchored on a keyed entry, cannot name the leaf",
			uri:        entry,
			requestURI: entry,
			want:       "",
		},
		{
			name:       "anchored on the state container",
			uri:        entry + "/state",
			requestURI: entry + "/state",
			want:       niAftsNhGrpLeaf,
		},
		{
			name:       "request on the next-hop-group leaf, anchored on state",
			uri:        entry + "/state",
			requestURI: entry + "/state/next-hop-group",
			want:       niAftsNhGrpLeaf,
		},
		{
			name:       "request on the state/prefix leaf, anchored on state",
			uri:        entry + "/state",
			requestURI: entry + "/state/prefix",
			want:       niAftsNhGrpLeaf,
		},
		{
			name:       "ipv6 anchored on the state container",
			uri:        niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]/state",
			requestURI: niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]/state",
			want:       niAftsNhGrpLeaf,
		},
		{
			name:       "ipv6 anchored on the entry list, cannot name the leaf",
			uri:        niAftsSubscUrl + "/ipv6-unicast/ipv6-entry",
			requestURI: niAftsSubscUrl + "/ipv6-unicast/ipv6-entry",
			want:       "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_afts_entries_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.requestURI,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_afts_entries_xfmr failed: %v", err)
			}
			fields := out.dbDataMap[db.ApplDB][niAftsRouteTbl]["*"]
			got := fields[niAftsFldNhGrp]
			if got != tc.want {
				t.Errorf("%s maps to %q, want %q", niAftsFldNhGrp, got, tc.want)
			}
			// The prefix leaves are the list key, so nothing else should be
			// mapped: a mapping for a field the table does not carry would
			// report a change against a leaf that did not change.
			if len(fields) > 1 {
				t.Errorf("more than the one field is mapped: %v", fields)
			}
		})
	}
}

// The watch has to be registered as an all key pattern. A ROUTE_TABLE key for an
// IPv6 prefix is split into several components by the APPL_DB separator, and the
// notification machinery only lets such a key through a pattern that is the
// single component "*".
func TestSubscribe_ni_afts_entries_registers_all_key_pattern(t *testing.T) {
	for _, uri := range []string{
		niAftsSubscUrl + "/ipv4-unicast/ipv4-entry",
		niAftsSubscUrl + "/ipv6-unicast/ipv6-entry",
	} {
		out, err := Subscribe_ni_afts_entries_xfmr(XfmrSubscInParams{
			uri:        uri,
			requestURI: uri,
			subscProc:  TRANSLATE_SUBSCRIBE,
		})
		if err != nil {
			t.Fatalf("Subscribe_ni_afts_entries_xfmr failed for %s: %v", uri, err)
		}
		keys := out.dbDataMap[db.ApplDB][niAftsRouteTbl]
		if len(keys) != 1 {
			t.Fatalf("expected a single key pattern for %s, got %v", uri, keys)
		}
		if _, ok := keys["*"]; !ok {
			t.Errorf("key pattern for %s is not the all key pattern: %v", uri, keys)
		}
	}
}

// A request on a leaf reaches the callback with the leaf trimmed off uri, and
// reading the family and the list depth from uri rather than requestURI is what
// would silently downgrade such a request to sampling.
func TestSubscribe_ni_afts_entries_leaf_request_is_onchange(t *testing.T) {
	entry := niAftsSubscUrl + "/ipv4-unicast/ipv4-entry[prefix=20.50.0.0/16]"
	out, err := Subscribe_ni_afts_entries_xfmr(XfmrSubscInParams{
		uri:        entry + "/state",
		requestURI: entry + "/state/next-hop-group",
		subscProc:  TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ni_afts_entries_xfmr failed: %v", err)
	}
	if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
		t.Fatalf("onChange=%v nOpts=%v, want on-change", out.onChange, out.nOpts)
	}
}

// The CRUD parent-table check reaches the callback with requestURI unset, and
// falling back to uri has to keep the mapping usable rather than error.
func TestSubscribe_ni_afts_entries_without_requestURI(t *testing.T) {
	uri := niAftsSubscUrl + "/ipv6-unicast/ipv6-entry[prefix=fc00:61::/64]"
	out, err := Subscribe_ni_afts_entries_xfmr(XfmrSubscInParams{
		uri:       uri,
		subscProc: TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ni_afts_entries_xfmr failed: %v", err)
	}
	if out.onChange != OnchangeEnable || out.nOpts == nil || out.nOpts.pType != OnChange {
		t.Fatalf("onChange=%v nOpts=%v, want on-change", out.onChange, out.nOpts)
	}
	if _, ok := out.dbDataMap[db.ApplDB][niAftsRouteTbl]["*"]; !ok {
		t.Fatalf("missing %s[*]: %v", niAftsRouteTbl, out.dbDataMap)
	}
}

// The path transformer is what makes the on-change subscription above usable:
// it turns the ROUTE_TABLE key a keyspace notification carries into the prefix
// the entry list is keyed on. Every key shape fpmsyncd writes has to resolve,
// and a key the list cannot own has to be refused rather than mistranslated,
// because a refusal drops one notification while a wrong key reports a change
// against a prefix that did not change.
func TestDbToYangPath_ni_afts_entries(t *testing.T) {
	const niList = "/openconfig-network-instance:network-instances/network-instance"
	v4Entry := niList + "/afts/ipv4-unicast/ipv4-entry"
	v6Entry := niList + "/afts/ipv6-unicast/ipv6-entry"

	tests := []struct {
		name       string
		schemaPath string
		tblName    string
		keyComp    []string
		wantPrefix string // empty means no prefix key is expected
		wantErr    bool
	}{
		{
			name:       "ipv4 prefix",
			schemaPath: v4Entry,
			keyComp:    []string{"20.50.0.0/16"},
			wantPrefix: "20.50.0.0/16",
		},
		{
			// fpmsyncd writes a host route without a mask, which is not a valid
			// prefix, so the mask covering the single address has to be added.
			name:       "ipv4 host route gains /32",
			schemaPath: v4Entry,
			keyComp:    []string{"10.1.0.32"},
			wantPrefix: "10.1.0.32/32",
		},
		{
			// The APPL_DB separator is a colon, so an IPv6 prefix arrives split
			// across key components and has to be rejoined.
			name:       "ipv6 prefix rejoined from components",
			schemaPath: v6Entry,
			keyComp:    []string{"fc00", "61", "", "/64"},
			wantPrefix: "fc00:61::/64",
		},
		{
			name:       "ipv6 host route gains /128",
			schemaPath: v6Entry,
			keyComp:    []string{"fc00", "1", "", "32"},
			wantPrefix: "fc00:1::32/128",
		},
		{
			name:       "default route",
			schemaPath: v4Entry,
			keyComp:    []string{"0.0.0.0/0"},
			wantPrefix: "0.0.0.0/0",
		},
		{
			// A leaf under the list inherits the path transformer, so it is
			// called with the leaf on the schema path and still has to answer
			// with the key path of the list that holds the key.
			name:       "leaf under the entry list",
			schemaPath: v4Entry + "/state/next-hop-group",
			keyComp:    []string{"20.50.0.0/16"},
			wantPrefix: "20.50.0.0/16",
		},
		{
			// Both lists read one table, so every route event reaches both
			// subscriptions and the wrong family has to be refused.
			name:       "ipv6 key on the ipv4 list is refused",
			schemaPath: v4Entry,
			keyComp:    []string{"fc00", "61", "", "/64"},
			wantErr:    true,
		},
		{
			name:       "ipv4 key on the ipv6 list is refused",
			schemaPath: v6Entry,
			keyComp:    []string{"20.50.0.0/16"},
			wantErr:    true,
		},
		{
			// Only the default network instance can be attributed by key, and a
			// VRF scoped key rejoins into something that is not a prefix.
			name:       "vrf scoped key is refused",
			schemaPath: v4Entry,
			keyComp:    []string{"Vrf1", "20.50.0.0/16"},
			wantErr:    true,
		},
		{
			name:       "unparseable key is refused",
			schemaPath: v4Entry,
			keyComp:    []string{"not-a-prefix"},
			wantErr:    true,
		},
		{
			name:       "empty key is refused",
			schemaPath: v4Entry,
			keyComp:    []string{},
			wantErr:    true,
		},
		{
			name:       "notification from another table is refused",
			schemaPath: v4Entry,
			tblName:    niAftsNhGrpTbl,
			keyComp:    []string{"20.50.0.0/16"},
			wantErr:    true,
		},
		{
			// A container above the list names no prefix, so only the network
			// instance key can be answered.
			name:       "unicast container names no prefix",
			schemaPath: niList + "/afts/ipv4-unicast",
			keyComp:    []string{"20.50.0.0/16"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tblName := tc.tblName
			if tblName == "" {
				tblName = niAftsRouteTbl
			}
			keys := make(map[string]string)
			err := DbToYangPath_ni_afts_entries_path_xfmr(XfmrDbToYgPathParams{
				ygSchemaPath: tc.schemaPath,
				tblName:      tblName,
				tblKeyComp:   tc.keyComp,
				ygPathKeys:   keys,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got keys %v", keys)
				}
				return
			}
			if err != nil {
				t.Fatalf("DbToYangPath_ni_afts_entries_path_xfmr failed: %v", err)
			}
			if got := keys[niList+"/name"]; got != niAftsDefaultNi {
				t.Errorf("network instance key = %q, want %q", got, niAftsDefaultNi)
			}
			entryList := v4Entry
			if strings.Contains(tc.schemaPath, "ipv6") {
				entryList = v6Entry
			}
			got, ok := keys[entryList+"/prefix"]
			if tc.wantPrefix == "" {
				if ok {
					t.Errorf("prefix key = %q, want none", got)
				}
				return
			}
			if !ok {
				t.Fatalf("no prefix key under %q, got %v", entryList, keys)
			}
			if got != tc.wantPrefix {
				t.Errorf("prefix key = %q, want %q", got, tc.wantPrefix)
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

// The one parse serves both the retrieval and the subscription: the filter
// narrows what a GET reports, while the depth flags decide whether a
// subscription can be served on change. A path naming one group or one member
// that came back unkeyed would report the whole list instead.
func Test_parseNiAftsNextHopGroupsRequestUri(t *testing.T) {
	grps := niAftsSubscUrl + "/next-hop-groups"
	grp := grps + "/next-hop-group[id=901]"
	tests := []struct {
		name        string
		uri         string
		wantNi      string
		wantId      uint64
		wantKeyed   bool
		wantMember  uint64
		wantMbrKeyd bool
		atGroupList bool
		atMembers   bool
		wantErr     bool
	}{
		{
			name:   "next-hop-groups container",
			uri:    grps,
			wantNi: niAftsDefaultNi,
		},
		{
			name:        "group list unkeyed",
			uri:         grps + "/next-hop-group",
			wantNi:      niAftsDefaultNi,
			atGroupList: true,
		},
		{
			// A wildcard asks for the whole list, same as an absent predicate.
			name:        "group list wildcard key",
			uri:         grps + "/next-hop-group[id=*]",
			wantNi:      niAftsDefaultNi,
			atGroupList: true,
		},
		{
			name:        "group keyed",
			uri:         grp,
			wantNi:      niAftsDefaultNi,
			wantId:      901,
			wantKeyed:   true,
			atGroupList: true,
		},
		{
			name:        "leaf under the group state",
			uri:         grp + "/state/id",
			wantNi:      niAftsDefaultNi,
			wantId:      901,
			wantKeyed:   true,
			atGroupList: true,
		},
		{
			name:        "member container",
			uri:         grp + "/next-hops",
			wantNi:      niAftsDefaultNi,
			wantId:      901,
			wantKeyed:   true,
			atGroupList: true,
			atMembers:   true,
		},
		{
			name:        "member keyed",
			uri:         grp + "/next-hops/next-hop[index=202]",
			wantNi:      niAftsDefaultNi,
			wantId:      901,
			wantKeyed:   true,
			wantMember:  202,
			wantMbrKeyd: true,
			atGroupList: true,
			atMembers:   true,
		},
		{
			name:        "leaf under a keyed member",
			uri:         grp + "/next-hops/next-hop[index=202]/state/weight",
			wantNi:      niAftsDefaultNi,
			wantId:      901,
			wantKeyed:   true,
			wantMember:  202,
			wantMbrKeyd: true,
			atGroupList: true,
			atMembers:   true,
		},
		{
			name:        "member wildcard under a wildcard group",
			uri:         grps + "/next-hop-group[id=*]/next-hops/next-hop[index=*]/state/weight",
			wantNi:      niAftsDefaultNi,
			atGroupList: true,
			atMembers:   true,
		},
		{
			name:        "keyed member under a wildcard group",
			uri:         grps + "/next-hop-group[id=*]/next-hops/next-hop[index=202]",
			wantNi:      niAftsDefaultNi,
			wantMember:  202,
			wantMbrKeyd: true,
			atGroupList: true,
			atMembers:   true,
		},
		{
			// The network instance is read off the path, since a group is only
			// attributed to the default one.
			name:        "non default network instance",
			uri:         "/openconfig-network-instance:network-instances/network-instance[name=Vrf1]/afts/next-hop-groups/next-hop-group",
			wantNi:      "Vrf1",
			atGroupList: true,
		},
		{
			name:    "non numeric group id",
			uri:     grps + "/next-hop-group[id=not-an-id]",
			wantErr: true,
		},
		{
			name:    "non numeric member index",
			uri:     grp + "/next-hops/next-hop[index=not-an-id]",
			wantErr: true,
		},
		{
			name:    "path outside the subtree",
			uri:     niAftsSubscUrl + "/ipv4-unicast/ipv4-entry",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseNiAftsNextHopGroupsRequestUri(tc.uri)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseNiAftsNextHopGroupsRequestUri failed: %v", err)
			}
			if got.niName != tc.wantNi {
				t.Errorf("niName = %q, want %q", got.niName, tc.wantNi)
			}
			if got.filter.keyed != tc.wantKeyed || got.filter.id != tc.wantId {
				t.Errorf("group filter = {id:%d keyed:%v}, want {id:%d keyed:%v}",
					got.filter.id, got.filter.keyed, tc.wantId, tc.wantKeyed)
			}
			if got.filter.memberKeyed != tc.wantMbrKeyd || got.filter.member != tc.wantMember {
				t.Errorf("member filter = {member:%d keyed:%v}, want {member:%d keyed:%v}",
					got.filter.member, got.filter.memberKeyed, tc.wantMember, tc.wantMbrKeyd)
			}
			if got.atGroupList != tc.atGroupList {
				t.Errorf("atGroupList = %v, want %v", got.atGroupList, tc.atGroupList)
			}
			if got.atMembers != tc.atMembers {
				t.Errorf("atMembers = %v, want %v", got.atMembers, tc.atMembers)
			}
		})
	}
}

// The next-hop-group id is the NEXTHOP_GROUP_TABLE key, so a keyspace event
// names the group and the group level leaves can be served on change. The member
// list below is keyed on a next-hop id held inside a comma separated field value,
// which a notification cannot be decomposed into, so a request reaching the
// members has to keep sampling: accepting on-change there would report none of
// the member changes it was asked for.
func TestSubscribe_ni_afts_next_hop_groups_paths(t *testing.T) {
	grps := niAftsSubscUrl + "/next-hop-groups"
	grp := grps + "/next-hop-group[id=901]"
	tests := []struct {
		name     string
		uri      string
		onChange bool
	}{
		{
			// Above the group list, so a notification on it names no group.
			name: "next-hop-groups container",
			uri:  grps,
		},
		{
			name:     "group list unkeyed",
			uri:      grps + "/next-hop-group",
			onChange: true,
		},
		{
			name:     "group list wildcard key",
			uri:      grps + "/next-hop-group[id=*]",
			onChange: true,
		},
		{
			name:     "group keyed",
			uri:      grp,
			onChange: true,
		},
		{
			name:     "group id leaf",
			uri:      grp + "/id",
			onChange: true,
		},
		{
			name:     "group state container",
			uri:      grp + "/state",
			onChange: true,
		},
		{
			name:     "group state/id leaf",
			uri:      grp + "/state/id",
			onChange: true,
		},
		{
			name: "member container",
			uri:  grp + "/next-hops",
		},
		{
			name: "member keyed",
			uri:  grp + "/next-hops/next-hop[index=202]",
		},
		{
			name: "member state/weight leaf",
			uri:  grp + "/next-hops/next-hop[index=202]/state/weight",
		},
		{
			name: "member wildcard under a wildcard group",
			uri:  grps + "/next-hop-group[id=*]/next-hops/next-hop[index=*]/state/weight",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_afts_next_hop_groups_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.uri,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_afts_next_hop_groups_xfmr failed: %v", err)
			}
			if out.isVirtualTbl {
				t.Fatalf("expected a non-virtual subscribe mapping, got map %v", out.dbDataMap)
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
			keys := out.dbDataMap[db.ApplDB][niAftsNhGrpTbl]
			fields, ok := keys["*"]
			if !ok {
				t.Fatalf("missing %s[*]: %v", niAftsNhGrpTbl, out.dbDataMap)
			}
			// A NEXTHOP_GROUP_TABLE key is a bare id and so is one component,
			// but the watch is still registered as an all key pattern.
			if len(keys) != 1 {
				t.Errorf("expected a single key pattern, got %v", keys)
			}
			// id and state/id are both the list key, which the addition and
			// removal of the row carries on its own, and the fields the row does
			// hold describe members that are not served on change.
			if len(fields) != 0 {
				t.Errorf("expected no field mapped to a leaf, got %v", fields)
			}
		})
	}
}

// A request on a leaf reaches the callback with the leaf trimmed off uri, so
// reading the depth from uri rather than requestURI would put a group level leaf
// on sampling and, worse, give the member subtree an on-change subscription that
// reports nothing.
func TestSubscribe_ni_afts_next_hop_groups_leaf_request_depth(t *testing.T) {
	grp := niAftsSubscUrl + "/next-hop-groups/next-hop-group[id=901]"
	tests := []struct {
		name       string
		uri        string
		requestURI string
		onChange   bool
	}{
		{
			name:       "request on state/id, anchored on state",
			uri:        grp + "/state",
			requestURI: grp + "/state/id",
			onChange:   true,
		},
		{
			name:       "request on a member state/weight, anchored on its state",
			uri:        grp + "/next-hops/next-hop[index=202]/state",
			requestURI: grp + "/next-hops/next-hop[index=202]/state/weight",
		},
		{
			// The CRUD parent-table check arrives with requestURI unset.
			name:     "requestURI unset falls back to uri",
			uri:      grp,
			onChange: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Subscribe_ni_afts_next_hop_groups_xfmr(XfmrSubscInParams{
				uri:        tc.uri,
				requestURI: tc.requestURI,
				subscProc:  TRANSLATE_SUBSCRIBE,
			})
			if err != nil {
				t.Fatalf("Subscribe_ni_afts_next_hop_groups_xfmr failed: %v", err)
			}
			wantOnChange, wantPType := OnchangeDisable, Sample
			if tc.onChange {
				wantOnChange, wantPType = OnchangeEnable, OnChange
			}
			if out.onChange != wantOnChange || out.nOpts == nil || out.nOpts.pType != wantPType {
				t.Fatalf("onChange=%v nOpts=%v, want %v/%v",
					out.onChange, out.nOpts, wantOnChange, wantPType)
			}
		})
	}
}

// The path transformer is what makes the group level on-change subscription
// usable: it turns the NEXTHOP_GROUP_TABLE key into the id the group list is
// keyed on. A key it cannot own, and a path whose key it cannot supply, have to
// be refused rather than half answered, since a path left with an unresolved key
// would be reported against whichever group the infrastructure filled in.
func TestDbToYangPath_ni_afts_next_hop_groups(t *testing.T) {
	const niList = "/openconfig-network-instance:network-instances/network-instance"
	grpList := niList + "/afts/next-hop-groups/next-hop-group"
	memberList := grpList + "/next-hops/next-hop"

	tests := []struct {
		name       string
		schemaPath string
		tblName    string
		keyComp    []string
		wantId     string // empty means no id key is expected
		wantErr    bool
	}{
		{
			name:       "group id",
			schemaPath: grpList,
			keyComp:    []string{"901"},
			wantId:     "901",
		},
		{
			name:       "state under the group",
			schemaPath: grpList + "/state",
			keyComp:    []string{"901"},
			wantId:     "901",
		},
		{
			name:       "leaf under the group state",
			schemaPath: grpList + "/state/id",
			keyComp:    []string{"904"},
			wantId:     "904",
		},
		{
			// Above the group list, so there is no id key to fill, but the
			// network instance is still named.
			name:       "next-hop-groups container names no id",
			schemaPath: niList + "/afts/next-hop-groups",
			keyComp:    []string{"901"},
		},
		{
			// The key names the group a member belongs to, never the member.
			name:       "member list is refused",
			schemaPath: memberList,
			keyComp:    []string{"901"},
			wantErr:    true,
		},
		{
			name:       "leaf under a member is refused",
			schemaPath: memberList + "/state/weight",
			keyComp:    []string{"901"},
			wantErr:    true,
		},
		{
			name:       "non numeric key is refused",
			schemaPath: grpList,
			keyComp:    []string{"not-an-id"},
			wantErr:    true,
		},
		{
			name:       "empty key is refused",
			schemaPath: grpList,
			keyComp:    []string{},
			wantErr:    true,
		},
		{
			// NEXTHOP_GROUP_TABLE is not VRF scoped, so a key carrying more than
			// the id is not a group row.
			name:       "multi component key is refused",
			schemaPath: grpList,
			keyComp:    []string{"Vrf1", "901"},
			wantErr:    true,
		},
		{
			name:       "notification from another table is refused",
			schemaPath: grpList,
			tblName:    niAftsRouteTbl,
			keyComp:    []string{"901"},
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tblName := tc.tblName
			if tblName == "" {
				tblName = niAftsNhGrpTbl
			}
			keys := make(map[string]string)
			err := DbToYangPath_ni_afts_next_hop_groups_path_xfmr(XfmrDbToYgPathParams{
				ygSchemaPath: tc.schemaPath,
				tblName:      tblName,
				tblKeyComp:   tc.keyComp,
				ygPathKeys:   keys,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got keys %v", keys)
				}
				return
			}
			if err != nil {
				t.Fatalf("DbToYangPath_ni_afts_next_hop_groups_path_xfmr failed: %v", err)
			}
			if got := keys[niList+"/name"]; got != niAftsDefaultNi {
				t.Errorf("network instance key = %q, want %q", got, niAftsDefaultNi)
			}
			got, ok := keys[grpList+"/id"]
			if tc.wantId == "" {
				if ok {
					t.Errorf("id key = %q, want none", got)
				}
				return
			}
			if !ok {
				t.Fatalf("no id key under %q, got %v", grpList, keys)
			}
			if got != tc.wantId {
				t.Errorf("id key = %q, want %q", got, tc.wantId)
			}
		})
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
