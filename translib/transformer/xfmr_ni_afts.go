////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");          //
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

package transformer

import (
	"net"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// Shared plumbing for the OpenConfig AFT state mapping. fpmsyncd publishes the
// FRR forwarding state to APPL_DB: ROUTE_TABLE carries one row per prefix and
// NEXTHOP_GROUP_TABLE one row per next-hop group. Both carry the next-hop ids
// FRR assigned, which is what the AFT next-hop lists are keyed on.
const (
	niAftsRouteTbl = "ROUTE_TABLE"
	niAftsNhGrpTbl = "NEXTHOP_GROUP_TABLE"

	niAftsFldNexthop = "nexthop"
	niAftsFldIfname  = "ifname"
	niAftsFldNhId    = "nexthop_id"
	niAftsFldWeight  = "weight"
	niAftsFldNhGrp   = "nexthop_group"

	// ROUTE_TABLE keys are the prefix alone in the default VRF and carry a VRF
	// component otherwise, so the default network instance is the only one
	// whose rows can be attributed by key. NEXTHOP_GROUP_TABLE is not VRF
	// scoped at all: FRR allocates next-hop and group ids from one global
	// space. Mapping a non-default network instance therefore needs more than
	// a key match and is out of scope here.
	niAftsDefaultNi = "default"

	// fpmsyncd flattens the members of a group into parallel comma separated
	// lists, one element per member, in the same order across every field.
	niAftsListDelim = ","
)

// niAftsNextHop is one entry of the top level AFT next-hop list.
type niAftsNextHop struct {
	ipAddress string
	ifName    string
}

// getNiAftsObj locates the afts container of a network instance in the YANG
// tree, creating the ancestors when asked to.
func getNiAftsObj(s *ygot.GoStruct, niName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts {
	if s == nil || *s == nil {
		return nil
	}

	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(deviceObj)

	if deviceObj.NetworkInstances == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}

	niObj, ok := deviceObj.NetworkInstances.NetworkInstance[niName]
	if !ok || niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(niName)
		if err != nil || niObj == nil {
			return nil
		}
	}

	if niObj.Afts == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Afts = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts{}
	}
	return niObj.Afts
}

// niAftsSplitList splits one of the parallel member lists fpmsyncd writes.
func niAftsSplitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, niAftsListDelim)
}

// niAftsElem returns element i of a member list, or "" when the list is shorter
// than the one being iterated. The lists are meant to be the same length, but a
// row is read without holding a lock against the writer.
func niAftsElem(list []string, i int) string {
	if i >= len(list) {
		return ""
	}
	return strings.TrimSpace(list[i])
}

// niAftsParseId parses a next-hop or group id.
func niAftsParseId(s string) (uint64, bool) {
	id, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// niAftsReportableAddr drops the addresses that carry no information. fpmsyncd
// writes the unspecified address when a next-hop has no gateway, which is how a
// directly connected next-hop is published; there is no address to report in
// that case and the interface carries the meaning instead.
func niAftsReportableAddr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.IsUnspecified() {
		return ""
	}
	return s
}

// niAftsRouteKeyPrefix recovers the prefix from a ROUTE_TABLE key and reports
// whether the row belongs to the default VRF. The APPL_DB key separator is a
// colon, so an IPv6 prefix splits across several key components and has to be
// rejoined; a VRF scoped key rejoins into something that is not a prefix, which
// is how the two are told apart.
func niAftsRouteKeyPrefix(k db.Key) (string, bool) {
	joined := strings.Join(k.Comp, ":")
	if joined == "" {
		return "", false
	}
	if _, _, err := net.ParseCIDR(joined); err == nil {
		return joined, true
	}
	if ip := net.ParseIP(joined); ip != nil {
		return joined, true
	}
	return "", false
}

// niAftsForEachRow walks a table, skipping rows that keep rejects and rows that
// disappear between listing the keys and reading them.
func niAftsForEachRow(d *db.DB, table string, keep func(db.Key) bool, fn func(db.Key, db.Value)) error {
	ts := &db.TableSpec{Name: table}
	keys, err := d.GetKeys(ts)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if keep != nil && !keep(k) {
			continue
		}
		v, err := d.GetEntry(ts, k)
		if err != nil {
			if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); notPresent {
				continue
			}
			return err
		}
		fn(k, v)
	}
	return nil
}

// niAftsAddRowNextHops folds the next-hops published by one row into out.
func niAftsAddRowNextHops(out map[uint64]niAftsNextHop, v db.Value) {
	ids := niAftsSplitList(v.Field[niAftsFldNhId])
	if len(ids) == 0 {
		return
	}
	addrs := niAftsSplitList(v.Field[niAftsFldNexthop])
	ifNames := niAftsSplitList(v.Field[niAftsFldIfname])

	for i, rawId := range ids {
		id, ok := niAftsParseId(rawId)
		if !ok {
			continue
		}
		nh := niAftsNextHop{
			ipAddress: niAftsReportableAddr(niAftsElem(addrs, i)),
			ifName:    niAftsElem(ifNames, i),
		}
		// FRR keeps one entry per distinct next-hop, so an id seen twice
		// describes the same next-hop. Keep what is already known and let a
		// later row only fill in what was missing.
		if prev, seen := out[id]; seen {
			if prev.ipAddress == "" {
				prev.ipAddress = nh.ipAddress
			}
			if prev.ifName == "" {
				prev.ifName = nh.ifName
			}
			out[id] = prev
			continue
		}
		out[id] = nh
	}
}

// niAftsCollectNextHops gathers every next-hop fpmsyncd has published, keyed by
// its FRR next-hop id. Both tables are read because a next-hop reaches APPL_DB
// either inline on a route that references it directly or as a member of a
// group, and the same next-hop can appear in both forms under one id.
func niAftsCollectNextHops(d *db.DB) (map[uint64]niAftsNextHop, error) {
	out := make(map[uint64]niAftsNextHop)

	if err := niAftsForEachRow(d, niAftsNhGrpTbl, nil, func(_ db.Key, v db.Value) {
		niAftsAddRowNextHops(out, v)
	}); err != nil {
		return nil, err
	}

	keepDefaultVrf := func(k db.Key) bool {
		_, ok := niAftsRouteKeyPrefix(k)
		return ok
	}
	if err := niAftsForEachRow(d, niAftsRouteTbl, keepDefaultVrf, func(_ db.Key, v db.Value) {
		niAftsAddRowNextHops(out, v)
	}); err != nil {
		return nil, err
	}

	return out, nil
}
