////////////////////////////////////////////////////////////////////////////////
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

package translib

import (
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
	"github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Workiva/go-datastructures/queue"
	"github.com/openconfig/gnmi/proto/gnmi"
)

func dedupeTestPath(elems ...string) *gnmi.Path {
	p := &gnmi.Path{}
	for _, e := range elems {
		p.Elem = append(p.Elem, &gnmi.PathElem{Name: e})
	}
	return p
}

// dedupeTestNInfo returns a notificationInfo for a fixed container path that is
// mapped to the given db table and key; it is eligible for deduplication unless
// the test modifies it.
func dedupeTestNInfo(dbno db.DBNum, table string, key ...string) *notificationInfo {
	return &notificationInfo{
		dbno:  dbno,
		table: &db.TableSpec{Name: table},
		key:   &db.Key{Comp: key},
		path:  dedupeTestPath("openconfig-lldp:lldp", "state"),
	}
}

func TestInitialUpdateDedupKey(t *testing.T) {
	wantKey := path.String(dedupeTestPath("openconfig-lldp:lldp", "state"))

	tests := []struct {
		name   string
		modify func(n *notificationInfo)
		want   bool
	}{{
		name:   "fixed path, singleton key",
		modify: func(n *notificationInfo) {},
		want:   true,
	}, {
		name:   "fixed path, plain key",
		modify: func(n *notificationInfo) { n.key = &db.Key{Comp: []string{"GLOBAL"}} },
		want:   true,
	}, {
		name:   "fixed path, empty key components",
		modify: func(n *notificationInfo) { n.key = &db.Key{} },
		want:   true,
	}, {
		name:   "non-db path",
		modify: func(n *notificationInfo) { n.table, n.key = nil, nil },
		want:   true,
	}, {
		name:   "wildcard path",
		modify: func(n *notificationInfo) { n.flags.Set(niWildcardPath) },
		want:   false,
	}, {
		name:   "dynamic table",
		modify: func(n *notificationInfo) { n.flags.Set(niDynamic) },
		want:   false,
	}, {
		name:   "fixed path, glob pattern key",
		modify: func(n *notificationInfo) { n.key = &db.Key{Comp: []string{"*"}} },
		want:   true,
	}, {
		name:   "fixed path, glob pattern in one key component",
		modify: func(n *notificationInfo) { n.key = &db.Key{Comp: []string{"Ethernet0", "*"}} },
		want:   true,
	}, {
		name: "wildcard path, glob pattern key",
		modify: func(n *notificationInfo) {
			n.key = &db.Key{Comp: []string{"*"}}
			n.flags.Set(niWildcardPath)
		},
		want: false,
	}, {
		name:   "db table without a key",
		modify: func(n *notificationInfo) { n.key = nil },
		want:   false,
	}, {
		name: "app handler",
		modify: func(n *notificationInfo) {
			n.handler = func(*apis.NotificationContext, apis.NotificationSender) {}
		},
		want: false,
	}, {
		name:   "field scan",
		modify: func(n *notificationInfo) { n.fldScanPatt = "addr*" },
		want:   false,
	}, {
		name:   "key group",
		modify: func(n *notificationInfo) { n.keyGroup = []int{1} },
		want:   false,
	}, {
		name:   "nil path",
		modify: func(n *notificationInfo) { n.path = nil },
		want:   false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := dedupeTestNInfo(db.ApplDB, "LLDP_LOC_CHASSIS", "")
			tc.modify(n)
			key, ok := initialUpdateDedupKey(n)
			if ok != tc.want {
				t.Fatalf("initialUpdateDedupKey ok=%v, want %v", ok, tc.want)
			}
			if ok && key != wantKey {
				t.Fatalf("initialUpdateDedupKey key=%q, want %q", key, wantKey)
			}
		})
	}

	if _, ok := initialUpdateDedupKey(nil); ok {
		t.Fatal("initialUpdateDedupKey(nil) should not be deduplicated")
	}
}

// Mappings of one path to different tables must share a single dedup key, so
// that only the first one to send data produces the initial update.
func TestInitialUpdateDedupKey_sameForAllTablesOfPath(t *testing.T) {
	infos := []*notificationInfo{
		dedupeTestNInfo(db.ConfigDB, "LLDP", "GLOBAL"),
		dedupeTestNInfo(db.ApplDB, "LLDP_LOC_CHASSIS", ""),
		dedupeTestNInfo(db.CountersDB, "LLDP_STATISTICS", "GLOBAL"),
	}
	first, ok := initialUpdateDedupKey(infos[0])
	if !ok {
		t.Fatal("expected first mapping to be deduplicable")
	}
	for _, n := range infos[1:] {
		k, ok := initialUpdateDedupKey(n)
		if !ok || k != first {
			t.Fatalf("table %s: key=%q ok=%v, want %q", n.table.Name, k, ok, first)
		}
	}

	other := dedupeTestNInfo(db.CountersDB, "LLDP_STATISTICS", "GLOBAL")
	other.path = dedupeTestPath("openconfig-lldp:lldp", "state", "counters")
	if k, ok := initialUpdateDedupKey(other); !ok || k == first {
		t.Fatalf("different path must not share the dedup key; key=%q ok=%v", k, ok)
	}
}

func TestMarkInitialUpdateSent(t *testing.T) {
	sInfo := &subscribeInfo{}
	if sInfo.initSynced["a"] {
		t.Fatal("nothing should be recorded on a new subscribeInfo")
	}
	sInfo.markInitialUpdateSent("a")
	if !sInfo.initSynced["a"] || sInfo.initSynced["b"] {
		t.Fatalf("unexpected initSynced content: %v", sInfo.initSynced)
	}
}

// A path whose initial update was already sent must be skipped without
// touching the db or queueing anything, whichever table maps it.
func TestSendInitialUpdate_skipsPathAlreadySent(t *testing.T) {
	q := queue.NewPriorityQueue(1, false)
	defer q.Dispose()

	sInfo := &subscribeInfo{id: "dedupe-test", q: q}

	// A nil sInfo.dbs entry would make a real walk panic, so reaching the
	// db would fail the test loudly.
	for _, n := range []*notificationInfo{
		dedupeTestNInfo(db.ConfigDB, "LLDP", "GLOBAL"),
		dedupeTestNInfo(db.ApplDB, "LLDP_LOC_CHASSIS", ""),
		dedupeTestNInfo(db.CountersDB, "LLDP_STATISTICS", "GLOBAL"),
	} {
		key, _ := initialUpdateDedupKey(n)
		sInfo.markInitialUpdateSent(key)

		if err := sendInitialUpdate(sInfo, n); err != nil {
			t.Fatalf("sendInitialUpdate(%s) failed: %v", n.table.Name, err)
		}
	}

	if q.Len() != 0 {
		t.Fatalf("expected no responses, queue has %d", q.Len())
	}
}

// A fixed path mapped to glob db keys in more than one table, as the QoS
// interface output is to QUEUE and UMF_TRUNK_QUEUE, shares one dedup key and
// is skipped once sent.
func TestSendInitialUpdate_skipsFixedPathWithGlobKey(t *testing.T) {
	q := queue.NewPriorityQueue(1, false)
	defer q.Dispose()

	sInfo := &subscribeInfo{id: "dedupe-glob-test", q: q}
	outPath := dedupeTestPath("openconfig-qos:qos", "interfaces", "interface", "output", "scheduler-policy", "config", "name")

	var first string
	for i, n := range []*notificationInfo{
		dedupeTestNInfo(db.ConfigDB, "QUEUE", "Ethernet0", "*"),
		dedupeTestNInfo(db.ConfigDB, "UMF_TRUNK_QUEUE", "Ethernet0", "*"),
	} {
		n.path = outPath
		key, ok := initialUpdateDedupKey(n)
		if !ok {
			t.Fatalf("table %s: expected a fixed path with a glob key to be deduplicable", n.table.Name)
		}
		if i == 0 {
			first = key
			sInfo.markInitialUpdateSent(key)
		} else if key != first {
			t.Fatalf("table %s: key=%q, want %q", n.table.Name, key, first)
		}

		if err := sendInitialUpdate(sInfo, n); err != nil {
			t.Fatalf("sendInitialUpdate(%s) failed: %v", n.table.Name, err)
		}
	}

	if q.Len() != 0 {
		t.Fatalf("expected no responses, queue has %d", q.Len())
	}
}

// An instance a wildcard path resolves to is sent once in the initial sync,
// whichever row resolved it; after the sync every notification is sent.
func TestResolvedUpdateDedupKey(t *testing.T) {
	wild := dedupeTestNInfo(db.StateDB, "ROUTE_MAP_APPLIED", "*", "*")
	wild.path = dedupeTestPath("openconfig-routing-policy:routing-policy", "policy-definitions", "policy-definition")
	wild.flags.Set(niWildcardPath)
	fields := []*yangNodeInfo{new(yangNodeInfo)}
	inst := "/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition[name=P1]"

	sInfo := &subscribeInfo{}
	k1, ok := resolvedUpdateDedupKey(wild, sInfo, "", false, inst, fields)
	if !ok {
		t.Fatal("a wildcard path in the initial sync should be deduplicated")
	}
	set := dedupeTestNInfo(db.StateDB, "ROUTE_MAP_SET_APPLIED", "*")
	set.path, set.flags = wild.path, wild.flags
	if k2, ok := resolvedUpdateDedupKey(set, sInfo, "", false, inst, fields); !ok || k2 != k1 {
		t.Fatalf("rows of another table resolving to the same instance must share the key; key=%q ok=%v", k2, ok)
	}
	if k3, _ := resolvedUpdateDedupKey(wild, sInfo, "", false, inst+"[name=P2]", fields); k3 == k1 {
		t.Fatal("another instance must not share the key")
	}
	if k4, _ := resolvedUpdateDedupKey(wild, sInfo, "", false, inst, []*yangNodeInfo{{leafName: "state"}}); k4 == k1 {
		t.Fatal("other fields of the instance must not share the key")
	}

	fixed := dedupeTestNInfo(db.StateDB, "ROUTE_MAP_SET_APPLIED", "P1")
	if _, ok := resolvedUpdateDedupKey(fixed, sInfo, "", false, inst, fields); ok {
		t.Fatal("a fixed path is left to initialUpdateDedupKey")
	}
	if _, ok := resolvedUpdateDedupKey(fixed, sInfo, "", true, inst, fields); !ok {
		t.Fatal("a path resolved from a glob key in the initial sync should be deduplicated")
	}
	handler := dedupeTestNInfo(db.StateDB, "ROUTE_MAP_APPLIED", "*", "*")
	handler.flags = wild.flags
	handler.handler = func(*apis.NotificationContext, apis.NotificationSender) {}
	if _, ok := resolvedUpdateDedupKey(handler, sInfo, "", false, inst, fields); ok {
		t.Fatal("an app handler path is never deduplicated")
	}
	if _, ok := resolvedUpdateDedupKey(wild, &subscribeInfo{syncDone: true}, "", false, inst, fields); ok {
		t.Fatal("a notification after the initial sync is never deduplicated")
	}
}
