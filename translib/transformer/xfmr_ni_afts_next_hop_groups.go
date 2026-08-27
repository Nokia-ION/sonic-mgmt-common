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

// The AFT next-hop-group list maps NEXTHOP_GROUP_TABLE directly: the group id
// FRR assigned is the Redis key, and the members are the next-hop ids the row
// lists, which are the keys of the top level next-hop list they leafref to.
// A member predicate is picked out as well as the group one, so that a request
// naming one member reports that member alone rather than every member of its
// group.
var niAftsNextHopGroupsUriRe = regexp.MustCompile(
	`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]` +
		`/afts/next-hop-groups` +
		`(?:/next-hop-group(?:\[id=([^\]]+)\])?` +
		`(?:/next-hops/next-hop\[index=([^\]]+)\](?:/.*)?|/.*)?)?$`)

// niAftsGroupFilter narrows what is reported to the group, and the member,
// named by a request. A zero value asks for everything.
type niAftsGroupFilter struct {
	id    uint64
	keyed bool

	member      uint64
	memberKeyed bool
}

type niAftsNextHopGroupsReqInfo struct {
	niName string
	filter niAftsGroupFilter
}

func init() {
	XlateFuncBind("DbToYang_ni_afts_next_hop_groups_xfmr", DbToYang_ni_afts_next_hop_groups_xfmr)
	XlateFuncBind("Subscribe_ni_afts_next_hop_groups_xfmr", Subscribe_ni_afts_next_hop_groups_xfmr)
}

func parseNiAftsNextHopGroupsRequestUri(uri string) (*niAftsNextHopGroupsReqInfo, error) {
	m := niAftsNextHopGroupsUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance AFT next-hop-groups path: " + uri}
	}

	reqInfo := &niAftsNextHopGroupsReqInfo{niName: m[1]}
	// An absent predicate asks for the whole list, which is how it is retrieved;
	// a wildcard is normalized the same way for the subscription paths that
	// carry one.
	if m[2] != "" && m[2] != "*" {
		id, ok := niAftsParseId(m[2])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("invalid AFT next-hop-group id %q; expected an unsigned integer", m[2]),
			}
		}
		reqInfo.filter.id = id
		reqInfo.filter.keyed = true
	}
	if m[3] != "" && m[3] != "*" {
		member, ok := niAftsParseId(m[3])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("invalid AFT next-hop-group member index %q; expected an unsigned integer", m[3]),
			}
		}
		reqInfo.filter.member = member
		reqInfo.filter.memberKeyed = true
	}
	return reqInfo, nil
}

var DbToYang_ni_afts_next_hop_groups_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiAftsNextHopGroupsRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	appDb := inParams.dbs[db.ApplDB]
	if appDb == nil {
		return tlerr.InvalidArgsError{Format: "ApplDB nil for network-instance AFT next-hop-groups"}
	}
	return niAftsPopulateNextHopGroups(inParams.ygRoot, reqInfo.niName, appDb, reqInfo.filter)
}

// niAftsPopulateNextHopGroups fills the next-hop-group list, either whole or as
// narrowed by the filter. It is also called by the afts container mapping.
func niAftsPopulateNextHopGroups(ygRoot *ygot.GoStruct, niName string, d *db.DB, filter niAftsGroupFilter) error {
	if !niAftsServesNi(niName) {
		return nil
	}

	groups, err := niAftsCollectGroups(d)
	if err != nil {
		return err
	}
	if filter.keyed {
		members, ok := groups[filter.id]
		if !ok {
			// An unknown id is an empty result rather than an error, so a
			// sample subscription on a group that has gone away stays alive.
			return nil
		}
		groups = map[uint64][]niAftsGroupMember{filter.id: members}
	}
	if filter.memberKeyed {
		for id, members := range groups {
			kept := make([]niAftsGroupMember, 0, 1)
			for _, member := range members {
				if member.id == filter.member {
					kept = append(kept, member)
				}
			}
			groups[id] = kept
		}
	}
	if len(groups) == 0 {
		return nil
	}

	aftsObj := getNiAftsObj(ygRoot, niName, true)
	if aftsObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate AFT object in YANG tree"}
	}
	if aftsObj.NextHopGroups == nil {
		aftsObj.NextHopGroups = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts_NextHopGroups{}
	}

	for gid, members := range groups {
		groupId := gid

		grpObj, ok := aftsObj.NextHopGroups.NextHopGroup[groupId]
		if !ok || grpObj == nil {
			grpObj, err = aftsObj.NextHopGroups.NewNextHopGroup(groupId)
			if err != nil || grpObj == nil {
				return tlerr.InvalidArgsError{
					Format: fmt.Sprintf("Unable to add AFT next-hop-group %d to YANG tree", groupId),
				}
			}
		}
		ygot.BuildEmptyTree(grpObj)

		grpObj.Id = &groupId
		if grpObj.State != nil {
			grpObj.State.Id = &groupId
		}
		if grpObj.NextHops == nil {
			continue
		}

		for _, member := range members {
			index := member.id

			memberObj, ok := grpObj.NextHops.NextHop[index]
			if !ok || memberObj == nil {
				memberObj, err = grpObj.NextHops.NewNextHop(index)
				if err != nil || memberObj == nil {
					return tlerr.InvalidArgsError{
						Format: fmt.Sprintf("Unable to add member %d of AFT next-hop-group %d to YANG tree", index, groupId),
					}
				}
			}
			ygot.BuildEmptyTree(memberObj)

			memberObj.Index = &index
			if memberObj.State != nil {
				memberObj.State.Index = &index
				if member.weight != 0 {
					weight := member.weight
					memberObj.State.Weight = &weight
				}
			}
		}
	}

	return nil
}

var Subscribe_ni_afts_next_hop_groups_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	result.dbDataMap = RedisDbSubscribeMap{db.ApplDB: {
		niAftsNhGrpTbl: {"*": {}},
	}}
	result.isVirtualTbl = false

	// The group id is the Redis key, but the member list below it is keyed on a
	// next-hop id carried in a field value, and the table has no SONiC YANG
	// model for the infrastructure to convert a notification key back into a
	// YANG path. Offer sampling rather than an on-change subscription that
	// would only fail on its first notification.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
