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

// The AFT next-hop-group list maps NEXTHOP_GROUP_TABLE directly: the group id
// FRR assigned is the Redis key, and the members are the next-hop ids the row
// lists, which are the keys of the top level next-hop list they leafref to.
// A member predicate is picked out as well as the group one, so that a request
// naming one member reports that member alone rather than every member of its
// group.
// The part of the path below next-hop-groups is captured whole as well, so that
// how deep a request reaches can be told apart from which group it names: the
// group id is the Redis key and can be recovered from a notification, while a
// member index cannot, so the two depths are not served the same way.
var niAftsNextHopGroupsUriRe = regexp.MustCompile(
	`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]` +
		`/afts/next-hop-groups` +
		`((?:/next-hop-group(?:\[id=([^\]]+)\])?` +
		`(?:/next-hops/next-hop\[index=([^\]]+)\](?:/.*)?|/.*)?)?)$`)

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

	// atGroupList records whether the path names the next-hop-group list, and
	// atMembers whether it reaches the member list under it.
	atGroupList bool
	atMembers   bool
}

func init() {
	XlateFuncBind("DbToYang_ni_afts_next_hop_groups_xfmr", DbToYang_ni_afts_next_hop_groups_xfmr)
	XlateFuncBind("Subscribe_ni_afts_next_hop_groups_xfmr", Subscribe_ni_afts_next_hop_groups_xfmr)
	XlateFuncBind("DbToYangPath_ni_afts_next_hop_groups_path_xfmr", DbToYangPath_ni_afts_next_hop_groups_path_xfmr)
}

func parseNiAftsNextHopGroupsRequestUri(uri string) (*niAftsNextHopGroupsReqInfo, error) {
	m := niAftsNextHopGroupsUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance AFT next-hop-groups path: " + uri}
	}

	reqInfo := &niAftsNextHopGroupsReqInfo{
		niName:      m[1],
		atGroupList: strings.HasPrefix(m[2], "/"+niAftsNhGrpNode),
		atMembers:   strings.Contains(m[2], "/"+niAftsNhGrpMembersNode),
	}
	// An absent predicate asks for the whole list, which is how it is retrieved;
	// a wildcard is normalized the same way for the subscription paths that
	// carry one.
	if m[3] != "" && m[3] != "*" {
		id, ok := niAftsParseId(m[3])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("invalid AFT next-hop-group id %q; expected an unsigned integer", m[3]),
			}
		}
		reqInfo.filter.id = id
		reqInfo.filter.keyed = true
	}
	if m[4] != "" && m[4] != "*" {
		member, ok := niAftsParseId(m[4])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("invalid AFT next-hop-group member index %q; expected an unsigned integer", m[4]),
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

// DbToYangPath_ni_afts_next_hop_groups_path_xfmr turns the NEXTHOP_GROUP_TABLE
// key a keyspace notification carries into the path of the group it changed.
// The key is the group id FRR assigned, which is the list key, so the group is
// named by the notification itself and there is no model lookup to make.
//
// NEXTHOP_GROUP_TABLE is not VRF scoped, since FRR allocates group ids from one
// global space, so every row belongs to the default network instance.
var DbToYangPath_ni_afts_next_hop_groups_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != niAftsNhGrpTbl {
		return fmt.Errorf("AFT next-hop-group path transformer called for table %q, expected %q",
			params.tblName, niAftsNhGrpTbl)
	}

	// The id is the whole key. A key of any other shape is not a group row, so
	// it is refused rather than guessed at.
	if len(params.tblKeyComp) != 1 {
		return fmt.Errorf("AFT next-hop-group notification key %v does not name a group id",
			params.tblKeyComp)
	}
	id := strings.TrimSpace(params.tblKeyComp[0])
	if _, ok := niAftsParseId(id); !ok {
		return fmt.Errorf("AFT next-hop-group notification key %q is not a group id", id)
	}

	// A member is keyed on a next-hop id held inside a comma separated field
	// value, so the key names the group a member belongs to but not the member.
	// The member index is left unresolved: translib then reads the group and
	// reports each member it holds at its own path.

	niPath, ok := niAftsSchemaPathUpto(params.ygSchemaPath, niAftsNiListNode)
	if !ok {
		return fmt.Errorf("AFT next-hop-group path %q does not name the network instance list",
			params.ygSchemaPath)
	}
	params.ygPathKeys[niPath+"/name"] = niAftsDefaultNi

	if grpPath, ok := niAftsSchemaPathUpto(params.ygSchemaPath, niAftsNhGrpNode); ok {
		params.ygPathKeys[grpPath+"/id"] = id
	}

	return nil
}

// niAftsMemberFields maps the NEXTHOP_GROUP_TABLE fields that hold the members
// to the leaves directly below anchor, a node at or below the member list.
func niAftsMemberFields(anchor string) map[string]string {
	bare := niBgpUriKeysRe.ReplaceAllString(strings.TrimSuffix(anchor, "/"), "")
	switch {
	case strings.HasSuffix(bare, "/"+niAftsNhGrpMembersNode+"/"+niAftsNhGrpMemberNode):
		return map[string]string{niAftsFldNhId: "index"}
	case strings.HasSuffix(bare, "/"+niAftsNhGrpMembersNode+"/"+niAftsNhGrpMemberNode+"/state"):
		return map[string]string{niAftsFldNhId: "index", niAftsFldWeight: "weight"}
	}
	return map[string]string{}
}

var Subscribe_ni_afts_next_hop_groups_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	// No field is mapped to a leaf: id and state/id are both the list key, which
	// the addition and removal of the row carries on its own. The fields the row
	// does hold describe the members, and those are not served on change.
	result.dbDataMap = RedisDbSubscribeMap{db.ApplDB: {
		niAftsNhGrpTbl: {"*": {}},
	}}
	result.isVirtualTbl = false

	// Which node was asked for, as opposed to which one the callback is anchored
	// on, survives only in requestURI: the subscribe path trims a trailing leaf
	// off uri. The CRUD parent-table check reaches this callback with requestURI
	// unset and an untrimmed uri, hence the fallback.
	reqUri := inParams.requestURI
	if reqUri == "" {
		reqUri = inParams.uri
	}

	// The group id is the Redis key, so a keyspace event names the group and the
	// group level leaves can be served on change.
	//
	// The member list below is keyed on a next-hop id carried inside a comma
	// separated field value. The path transformer names the group and leaves the
	// member unresolved, and translib then reads the group and reports each of
	// its members, so a request reaching the members is served on change too.
	// The fields holding them are mapped to a leaf directly below the anchor,
	// the only place the framework can name one, so that a group rewritten in
	// place is reported as well as one added or removed.
	if reqInfo, err := parseNiAftsNextHopGroupsRequestUri(reqUri); err == nil && reqInfo.atGroupList {
		if reqInfo.atMembers {
			result.dbDataMap[db.ApplDB][niAftsNhGrpTbl]["*"] = niAftsMemberFields(inParams.uri)
		}
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		return result, nil
	}

	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
