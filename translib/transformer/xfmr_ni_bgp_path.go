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

package transformer

import (
	"strings"
)

func init() {
	XlateFuncBind("DbToYangPath_ni_bgp_list_path_xfmr", DbToYangPath_ni_bgp_list_path_xfmr)
}

// niBgpPathKeyTables describes, for each table a BGP neighbor or peer-group
// path maps, which list key each component of its Redis key carries. Every
// one of them is keyed <network-instance>|<neighbor or peer-group>[|<afi-safi>]
// except NEIGH_STATE_TABLE, which bgpmon keys by neighbor address alone and
// only for the default instance.
var niBgpPathKeyTables = map[string][]string{
	niBgpNbrTbl:         {"network-instance", "neighbor"},
	niBgpNbrAfTbl:       {"network-instance", "neighbor", "afi-safi"},
	niBgpNbrAppliedTbl:  {"network-instance", "neighbor"},
	niBgpNbrStatsTbl:    {"network-instance", "neighbor"},
	niBgpNbrAfStatsTbl:  {"network-instance", "neighbor", "afi-safi"},
	niBgpNbrStateTbl:    {"neighbor"},
	niBgpPeerGroupTbl:   {"network-instance", "peer-group"},
	niBgpPeerGroupAfTbl: {"network-instance", "peer-group", "afi-safi"},
	niBgpPgAppliedTbl:   {"network-instance", "peer-group"},
	niBgpPgAfAppliedTbl: {"network-instance", "peer-group", "afi-safi"},
}

// The key leaf of each list a BGP neighbor or peer-group path passes through.
var niBgpPathListKeyLeaf = map[string]string{
	"network-instance": "name",
	"neighbor":         "neighbor-address",
	"peer-group":       "peer-group-name",
	"afi-safi":         "afi-safi-name",
}

// DbToYangPath_ni_bgp_list_path_xfmr resolves the list keys of a BGP neighbor
// or peer-group notification path from the Redis key of the row that changed.
// The lists are virtual tables, so translib cannot convert the key itself, and
// a subscription leaving a key open, as one at the afi-safis or neighbors
// container does, otherwise cannot be answered at all.
//
// It is annotated on the neighbor and peer-group lists and inherited by every
// node below them, so translib calls it for keyed paths as well. A table it
// does not describe is left to translib rather than failed, which would drop
// the notification of a path with no open key.
var DbToYangPath_ni_bgp_list_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	roles, ok := niBgpPathKeyTables[params.tblName]
	if !ok || len(params.tblKeyComp) != len(roles) {
		return nil
	}
	values := map[string]string{"network-instance": "default"}
	for i, role := range roles {
		values[role] = params.tblKeyComp[i]
	}
	if afiSafi, ok := values["afi-safi"]; ok {
		values["afi-safi"] = strings.ToUpper(afiSafi)
	}

	if params.yangPath == nil {
		return nil
	}
	// translib looks the keys up under the element names exactly as the path
	// spells them, so a module prefix is dropped only to recognise the list.
	schemaPath := ""
	for _, elem := range params.yangPath.Elem {
		schemaPath += "/" + elem.Name
		listName := elem.Name
		if idx := strings.Index(listName, ":"); idx >= 0 {
			listName = listName[idx+1:]
		}
		switch listName {
		case "protocol":
			// Only the BGP protocol instance reaches this transformer, and
			// its keys are not in any of these Redis keys.
			params.ygPathKeys[schemaPath+"/identifier"] = "BGP"
			params.ygPathKeys[schemaPath+"/name"] = "bgp"
			continue
		}
		keyLeaf, isList := niBgpPathListKeyLeaf[listName]
		if !isList {
			continue
		}
		if v, ok := values[listName]; ok {
			params.ygPathKeys[schemaPath+"/"+keyLeaf] = v
		}
	}
	return nil
}
