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

// The top level AFT next-hop list is the leafref target for the members of
// every next-hop group, so it is keyed on the same next-hop id FRR assigned and
// fpmsyncd publishes. It is deliberately independent of the group mapping: a
// next-hop reaches APPL_DB either inline on a route or as a group member, and
// this list is the union of both.
var niAftsNextHopsUriRe = regexp.MustCompile(
	`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]` +
		`/afts/next-hops(?:/next-hop(?:\[index=([^\]]+)\])?(?:/.*)?)?$`)

type niAftsNextHopsReqInfo struct {
	niName string
	index  uint64
	keyed  bool
}

func init() {
	XlateFuncBind("DbToYang_ni_afts_next_hops_xfmr", DbToYang_ni_afts_next_hops_xfmr)
	XlateFuncBind("Subscribe_ni_afts_next_hops_xfmr", Subscribe_ni_afts_next_hops_xfmr)
}

func parseNiAftsNextHopsRequestUri(uri string) (*niAftsNextHopsReqInfo, error) {
	m := niAftsNextHopsUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance AFT next-hops path: " + uri}
	}

	reqInfo := &niAftsNextHopsReqInfo{niName: m[1]}
	// An absent predicate asks for the whole list. A wildcard is normalized the
	// same way for the subscription paths that carry one; a wildcard GET never
	// reaches here, because the infrastructure cannot turn it into the uint64
	// the list is keyed on and refuses the request first.
	if m[2] != "" && m[2] != "*" {
		index, ok := niAftsParseId(m[2])
		if !ok {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("invalid AFT next-hop index %q; expected an unsigned integer", m[2]),
			}
		}
		reqInfo.index = index
		reqInfo.keyed = true
	}
	return reqInfo, nil
}

var DbToYang_ni_afts_next_hops_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiAftsNextHopsRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	if reqInfo.niName != niAftsDefaultNi {
		// Nothing in APPL_DB can be attributed to another network instance, so
		// report an empty list rather than failing the request.
		return nil
	}

	appDb := inParams.dbs[db.ApplDB]
	if appDb == nil {
		return tlerr.InvalidArgsError{Format: "ApplDB nil for network-instance AFT next-hops"}
	}

	nextHops, err := niAftsCollectNextHops(appDb)
	if err != nil {
		return err
	}
	if reqInfo.keyed {
		nh, ok := nextHops[reqInfo.index]
		if !ok {
			// An unknown index is an empty result rather than an error, so that
			// a sample subscription on a next-hop that has gone away stays
			// alive instead of failing.
			return nil
		}
		nextHops = map[uint64]niAftsNextHop{reqInfo.index: nh}
	}
	if len(nextHops) == 0 {
		return nil
	}

	aftsObj := getNiAftsObj(inParams.ygRoot, reqInfo.niName, true)
	if aftsObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate AFT object in YANG tree"}
	}
	if aftsObj.NextHops == nil {
		aftsObj.NextHops = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Afts_NextHops{}
	}

	for id, nh := range nextHops {
		index := id

		nhObj, ok := aftsObj.NextHops.NextHop[index]
		if !ok || nhObj == nil {
			nhObj, err = aftsObj.NextHops.NewNextHop(index)
			if err != nil || nhObj == nil {
				return tlerr.InvalidArgsError{
					Format: fmt.Sprintf("Unable to add AFT next-hop %d to YANG tree", index),
				}
			}
		}
		ygot.BuildEmptyTree(nhObj)

		nhObj.Index = &index
		if nhObj.State != nil {
			nhObj.State.Index = &index
			if nh.ipAddress != "" {
				ipAddress := nh.ipAddress
				nhObj.State.IpAddress = &ipAddress
			}
		}
		if nh.ifName != "" && nhObj.InterfaceRef != nil && nhObj.InterfaceRef.State != nil {
			ifName := nh.ifName
			nhObj.InterfaceRef.State.Interface = &ifName
		}
	}

	return nil
}

var Subscribe_ni_afts_next_hops_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	result.dbDataMap = RedisDbSubscribeMap{db.ApplDB: {
		niAftsNhGrpTbl: {"*": {}},
		niAftsRouteTbl: {"*": {}},
	}}
	result.isVirtualTbl = false

	// This list is keyed on a next-hop id that fpmsyncd carries in a field
	// value, not in the Redis key, and it is assembled from two tables. A
	// notification therefore cannot be mapped back to a list entry without a
	// path transformer, so offer sampling rather than an on-change
	// subscription that would only fail on its first notification.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
