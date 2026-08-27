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
		`/afts/(ipv4|ipv6)-unicast(?:/(?:ipv4|ipv6)-entry(?:\[prefix=([^\]]+)\])?(?:/.*)?)?$`)

type niAftsEntriesReqInfo struct {
	niName string
	isV4   bool
	prefix string
	keyed  bool
}

func init() {
	XlateFuncBind("DbToYang_ni_afts_entries_xfmr", DbToYang_ni_afts_entries_xfmr)
	XlateFuncBind("Subscribe_ni_afts_entries_xfmr", Subscribe_ni_afts_entries_xfmr)
}

func parseNiAftsEntriesRequestUri(uri string) (*niAftsEntriesReqInfo, error) {
	m := niAftsEntriesUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance AFT unicast entry path: " + uri}
	}

	reqInfo := &niAftsEntriesReqInfo{niName: m[1], isV4: m[2] == "ipv4"}
	// An absent predicate asks for the whole list, which is how it is retrieved;
	// a wildcard is normalized the same way for the subscription paths that
	// carry one.
	if m[3] != "" && m[3] != "*" {
		prefix, _, ok := niAftsNormalizePrefix(m[3])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("invalid AFT entry prefix %q", m[3]),
			}
		}
		reqInfo.prefix = prefix
		reqInfo.keyed = true
	}
	return reqInfo, nil
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

var Subscribe_ni_afts_entries_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	result.dbDataMap = RedisDbSubscribeMap{db.ApplDB: {
		niAftsRouteTbl: {"*": {}},
	}}
	result.isVirtualTbl = false

	// The prefix is the Redis key, but the table has no SONiC YANG model for the
	// infrastructure to convert a notification key back into a YANG path, and a
	// host route is keyed without the mask length the list needs. Offer sampling
	// rather than an on-change subscription that would only fail on its first
	// notification.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
