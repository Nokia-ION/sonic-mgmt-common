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
	"sort"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/openconfig/ygot/ygot"
)

// populateNiBgpAggregateStateReplay merges operational BGP subtrees into a
// hierarchy GET after the CONFIG_DB mapping has run.
func populateNiBgpAggregateStateReplay(inParams XfmrParams, ygRoot *ygot.GoStruct, niName, protoName string) error {
	if err := populateNiBgpAggregateGlobalStateReplay(inParams, ygRoot, niName, protoName); err != nil {
		return err
	}
	if err := populateNiBgpAggregatePgStateReplay(inParams, ygRoot, niName, protoName); err != nil {
		return err
	}
	return populateNiBgpAggregateNeighborStateReplay(inParams, ygRoot, niName, protoName)
}

// populateNiBgpAggregateNeighborStateReplay merges the operational subtrees of
// every neighbor: its state, which carries the messages, its transport state,
// and the state and apply-policy state of each address family FRR reports.
func populateNiBgpAggregateNeighborStateReplay(inParams XfmrParams, ygRoot *ygot.GoStruct, niName, protoName string) error {
	stateDb := inParams.dbs[db.StateDB]
	if stateDb == nil {
		return nil
	}
	protoUri := niContainerProtoUri(niName, "BGP", protoName)
	afFamilies := niBgpNbrAfStatsFamilies(stateDb, niName)

	for _, nbrAddr := range niBgpNeighborAddrsForAggregate(inParams.dbs[db.ConfigDB], stateDb, niName) {
		nbrUri := protoUri + "/bgp/neighbors/neighbor[neighbor-address=" + nbrAddr + "]"
		for _, uri := range []string{nbrUri + "/state", nbrUri + "/transport/state"} {
			if !rpolNodeServesRequest(inParams.requestUri, uri) {
				continue
			}
			if err := niContainerReplay(inParams, ygRoot, uri, DbToYang_ni_bgp_nbr_state_xfmr); err != nil {
				return err
			}
		}
		if err := populateNiBgpNbrAfStateReplay(inParams, ygRoot, niName, protoName, nbrAddr, afFamilies[nbrAddr]); err != nil {
			return err
		}
	}
	return nil
}

// populateNiBgpAggregateGlobalStateReplay merges BGP global operational state.
func populateNiBgpAggregateGlobalStateReplay(inParams XfmrParams, ygRoot *ygot.GoStruct, niName, protoName string) error {
	if inParams.dbs[db.StateDB] == nil {
		return nil
	}
	protoUri := niContainerProtoUri(niName, "BGP", protoName)
	globalStateUri := protoUri + "/bgp/global/state"
	if !rpolNodeServesRequest(inParams.requestUri, globalStateUri) {
		return nil
	}
	return niContainerReplay(inParams, ygRoot, globalStateUri, DbToYang_ni_bgp_global_state_xfmr)
}

// niBgpNeighborAddrsForAggregate lists the neighbor addresses whose state
// container may be merged into a hierarchy GET. CONFIG_DB names every
// configured neighbor; STATE_DB may also hold applied rows for peers FRR knows
// about before configuration is written.
func niBgpNeighborAddrsForAggregate(cfgDb, stateDb *db.DB, niName string) []string {
	addrs := map[string]bool{}
	if cfgDb != nil {
		keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpNbrTbl})
		if err == nil {
			for _, k := range keys {
				if len(k.Comp) == 2 && k.Comp[0] == niName && k.Comp[1] != "" {
					addrs[k.Comp[1]] = true
				}
			}
		}
	}
	if stateDb != nil {
		keys, err := stateDb.GetKeys(&db.TableSpec{Name: niBgpNbrAppliedTbl})
		if err == nil {
			for _, k := range keys {
				if len(k.Comp) == 2 && k.Comp[0] == niName && k.Comp[1] != "" {
					addrs[k.Comp[1]] = true
				}
			}
		}
	}
	out := make([]string, 0, len(addrs))
	for addr := range addrs {
		out = append(out, addr)
	}
	sort.Strings(out)
	return out
}
