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
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// The AFT unicast entry lists map ROUTE_TABLE, one entry per prefix. Both
// address families are served by one mapping because the two differ only in the
// generated types they fill.
var niAftsEntriesUriRe = regexp.MustCompile(
	`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]` +
		`/afts/(ipv4|ipv6)-unicast((?:/(?:ipv4|ipv6)-entry(?:\[prefix=([^\]]+)\])?(?:/.*)?)?)$`)

type niAftsEntriesReqInfo struct {
	niName string
	isV4   bool
	prefix string
	keyed  bool
	// atEntryList records whether the path reaches the entry list, as opposed to
	// stopping at the unicast container above it. Only a path that reaches the
	// list names a prefix, and only such a path can be served on change.
	atEntryList bool
}

func init() {
	XlateFuncBind("DbToYang_ni_afts_entries_xfmr", DbToYang_ni_afts_entries_xfmr)
	XlateFuncBind("Subscribe_ni_afts_entries_xfmr", Subscribe_ni_afts_entries_xfmr)
	XlateFuncBind("DbToYangPath_ni_afts_entries_path_xfmr", DbToYangPath_ni_afts_entries_path_xfmr)
}

func parseNiAftsEntriesRequestUri(uri string) (*niAftsEntriesReqInfo, error) {
	m := niAftsEntriesUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance AFT unicast entry path: " + uri}
	}

	reqInfo := &niAftsEntriesReqInfo{niName: m[1], isV4: m[2] == "ipv4", atEntryList: m[3] != ""}
	// An absent predicate asks for the whole list, which is how it is retrieved;
	// a wildcard is normalized the same way for the subscription paths that
	// carry one.
	if m[4] != "" && m[4] != "*" {
		prefix, _, ok := niAftsNormalizePrefix(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("invalid AFT entry prefix %q", m[4]),
			}
		}
		reqInfo.prefix = prefix
		reqInfo.keyed = true
	}
	return reqInfo, nil
}

// niAftsUriAtState reports whether a subscribe uri is anchored on the state
// container of an entry rather than on the entry list above it. Every leaf below
// state is a leaf, and the subscribe path trims a trailing leaf off the uri, so
// state is as deep as a uri reaching this callback can be anchored.
func niAftsUriAtState(uri string) bool {
	return strings.HasSuffix(strings.TrimSuffix(strings.TrimSpace(uri), "/"), "/"+niAftsStateNode)
}

var DbToYang_ni_afts_entries_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiAftsEntriesRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	appDb := inParams.dbs[db.ApplDB]
	if appDb == nil {
		return tlerr.InvalidArgsError{Format: "ApplDB nil for network-instance AFT unicast entries"}
	}
	return niAftsPopulateEntries(inParams.ygRoot, reqInfo.niName, appDb, reqInfo.isV4, reqInfo.keyed, reqInfo.prefix)
}

// niAftsPopulateEntries fills one address family's unicast entry list, either
// whole or for one prefix. It is also called by the afts container mapping.
func niAftsPopulateEntries(ygRoot *ygot.GoStruct, niName string, d *db.DB, isV4 bool, keyed bool, wantPrefix string) error {
	if !niAftsServesNi(niName) {
		return nil
	}

	entries, err := niAftsCollectEntries(d, isV4)
	if err != nil {
		return err
	}
	if keyed {
		entry, ok := entries[wantPrefix]
		if !ok {
			// An unknown prefix is an empty result rather than an error, so a
			// sample subscription on a route that has gone away stays alive.
			return nil
		}
		entries = map[string]niAftsEntry{wantPrefix: entry}
	}
	if len(entries) == 0 {
		return nil
	}

	aftsObj := getNiAftsObj(ygRoot, niName, true)
	if aftsObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate AFT object in YANG tree"}
	}
	if isV4 {
		return niAftsFillIpv4Entries(aftsObj, entries)
	}
	return niAftsFillIpv6Entries(aftsObj, entries)
}

func niAftsFillIpv4Entries(aftsObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts, entries map[string]niAftsEntry) error {
	if aftsObj.Ipv4Unicast == nil {
		aftsObj.Ipv4Unicast = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts_Ipv4Unicast{}
	}

	for p, entry := range entries {
		prefix := p

		entryObj, ok := aftsObj.Ipv4Unicast.Ipv4Entry[prefix]
		if !ok || entryObj == nil {
			var err error
			entryObj, err = aftsObj.Ipv4Unicast.NewIpv4Entry(prefix)
			if err != nil || entryObj == nil {
				return tlerr.InvalidArgsError{
					Format: fmt.Sprintf("Unable to add AFT ipv4 entry %s to YANG tree", prefix),
				}
			}
		}
		ygot.BuildEmptyTree(entryObj)

		entryObj.Prefix = &prefix
		if entryObj.State != nil {
			entryObj.State.Prefix = &prefix
			if entry.hasNhGrp {
				nhGrp := entry.nhGrp
				entryObj.State.NextHopGroup = &nhGrp
			}
		}
	}
	return nil
}

func niAftsFillIpv6Entries(aftsObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts, entries map[string]niAftsEntry) error {
	if aftsObj.Ipv6Unicast == nil {
		aftsObj.Ipv6Unicast = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts_Ipv6Unicast{}
	}

	for p, entry := range entries {
		prefix := p

		entryObj, ok := aftsObj.Ipv6Unicast.Ipv6Entry[prefix]
		if !ok || entryObj == nil {
			var err error
			entryObj, err = aftsObj.Ipv6Unicast.NewIpv6Entry(prefix)
			if err != nil || entryObj == nil {
				return tlerr.InvalidArgsError{
					Format: fmt.Sprintf("Unable to add AFT ipv6 entry %s to YANG tree", prefix),
				}
			}
		}
		ygot.BuildEmptyTree(entryObj)

		entryObj.Prefix = &prefix
		if entryObj.State != nil {
			entryObj.State.Prefix = &prefix
			if entry.hasNhGrp {
				nhGrp := entry.nhGrp
				entryObj.State.NextHopGroup = &nhGrp
			}
		}
	}
	return nil
}

// DbToYangPath_ni_afts_entries_path_xfmr resolves the wildcard keys of a
// notification path from the ROUTE_TABLE key that raised it. ROUTE_TABLE has no
// SONiC YANG model for the infrastructure to convert a key back into a path
// with, so an on-change subscription on the entry lists needs this to turn a
// keyspace event into the path of the entry it changed.
var DbToYangPath_ni_afts_entries_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != niAftsRouteTbl {
		return fmt.Errorf("ni_afts_entries_path_xfmr: notification from table %q, expected %q",
			params.tblName, niAftsRouteTbl)
	}

	// Only the default VRF is mapped, and a VRF scoped key rejoins into
	// something that is not a prefix, which is how the two are told apart.
	rawPrefix, ok := niAftsRouteKeyPrefix(db.Key{Comp: params.tblKeyComp})
	if !ok {
		return fmt.Errorf("ni_afts_entries_path_xfmr: ROUTE_TABLE key %v does not name a prefix in the"+
			" default network instance", params.tblKeyComp)
	}
	// fpmsyncd keys a host route on the bare address, so the mask the list is
	// keyed on has to be supplied, exactly as the get path supplies it.
	prefix, isV4, ok := niAftsNormalizePrefix(rawPrefix)
	if !ok {
		return fmt.Errorf("ni_afts_entries_path_xfmr: ROUTE_TABLE key %v does not normalize to a prefix",
			params.tblKeyComp)
	}

	niPath, ok := niAftsSchemaPathUpto(params.ygSchemaPath, niAftsNiListNode)
	if !ok {
		return fmt.Errorf("ni_afts_entries_path_xfmr: path %q does not name the network instance list",
			params.ygSchemaPath)
	}

	// One table holds both families, so every route event reaches the
	// subscriptions of both entry lists. Which list this notification is being
	// translated for decides which family it may carry.
	_, wantV4 := niAftsSchemaPathUpto(params.ygSchemaPath, niAftsIpv4UnicastNode)
	_, wantV6 := niAftsSchemaPathUpto(params.ygSchemaPath, niAftsIpv6UnicastNode)
	if wantV4 == wantV6 {
		return fmt.Errorf("ni_afts_entries_path_xfmr: path %q names neither unicast address family",
			params.ygSchemaPath)
	}
	if isV4 != wantV4 {
		return fmt.Errorf("ni_afts_entries_path_xfmr: prefix %s is not of the address family the path"+
			" %q asks for", prefix, params.ygSchemaPath)
	}

	params.ygPathKeys[niPath+"/name"] = niAftsDefaultNi

	// A path anchored on the unicast container names no prefix, so there is no
	// entry key to answer with.
	entryNode := niAftsIpv6EntryNode
	if isV4 {
		entryNode = niAftsIpv4EntryNode
	}
	if entryPath, ok := niAftsSchemaPathUpto(params.ygSchemaPath, entryNode); ok {
		params.ygPathKeys[entryPath+"/prefix"] = prefix
	}

	return nil
}

var Subscribe_ni_afts_entries_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	// A changed row field is only reported if it is mapped to a yang node, and
	// the mapping is by name, so the one entry leaf backed by a field has to be
	// spelled out: the field is nexthop_group and the leaf is next-hop-group.
	// prefix and state/prefix need no mapping because they are the list key,
	// which the addition and removal of the row carries on its own.
	//
	// Only a leaf directly below the node the request is anchored on can be
	// named, because the framework forms the path by appending the name to the
	// anchor. next-hop-group sits under state, so it is nameable when the request
	// is anchored on state, which covers a request on the state container and one
	// on a leaf under it alike: the subscribe path trims a trailing leaf off the
	// uri. A request anchored on the entry list cannot reach it, and declaring it
	// there would be a mapping that never resolves, so such a request reports the
	// addition and removal of entries only.
	fields := map[string]string{}
	if niAftsUriAtState(inParams.uri) {
		fields[niAftsFldNhGrp] = niAftsNhGrpLeaf
	}

	result.dbDataMap = RedisDbSubscribeMap{db.ApplDB: {
		niAftsRouteTbl: {"*": fields},
	}}
	result.isVirtualTbl = false

	// Which node was asked for, as opposed to which one the callback is anchored
	// on, survives only in requestURI. The CRUD parent-table check reaches this
	// callback with requestURI unset and an untrimmed uri, hence the fallback.
	leafUri := inParams.requestURI
	if leafUri == "" {
		leafUri = inParams.uri
	}

	// The prefix the list is keyed on is the ROUTE_TABLE key, and
	// DbToYangPath_ni_afts_entries_path_xfmr recovers it from a notification,
	// so a request that reaches the entry list can be served on change. A
	// request on the unicast container names no entry and stays on sampling.
	if reqInfo, err := parseNiAftsEntriesRequestUri(leafUri); err == nil && reqInfo.atEntryList {
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		return result, nil
	}

	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
