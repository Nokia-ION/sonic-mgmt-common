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
	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/openconfig/ygot/ygot"
)

// populateNiStaticRouteAggregateStateReplay merges route and next-hop operational
// data into a hierarchy GET after CONFIG_DB mapping has run. Applied next-hop
// indices are taken from STATIC_ROUTE_APPLIED rather than from the configured
// list, because FRR may publish an interface-qualified index the config row does
// not carry.
func populateNiStaticRouteAggregateStateReplay(inParams XfmrParams, ygRoot *ygot.GoStruct, niName, protoName, prefix string) error {
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return nil
	}
	prefixes := []string{prefix}
	if prefix == "" {
		prefixes = niStaticRoutePrefixesFromDb(cfgDb, niName)
	}
	protoUri := niContainerProtoUri(niName, "STATIC", protoName)
	for _, p := range prefixes {
		routeInstUri := protoUri + "/static-routes/static[prefix=" + p + "]"
		if !rpolNodeServesRequest(inParams.requestUri, routeInstUri) &&
			!rpolNodeServesRequest(inParams.requestUri, routeInstUri+"/state") &&
			!rpolNodeServesRequest(inParams.requestUri, routeInstUri+"/next-hops") {
			continue
		}
		routeStateUri := routeInstUri + "/state"
		if rpolNodeServesRequest(inParams.requestUri, routeStateUri) {
			if err := niContainerReplay(inParams, ygRoot, routeStateUri, DbToYang_ni_static_route_cfg_xfmr); err != nil {
				return err
			}
		}
		applied, held := niStaticRouteAppliedEntry(inParams, niName, p)
		if !held {
			continue
		}
		if rpolNodeServesRequest(inParams.requestUri, routeInstUri) ||
			rpolNodeServesRequest(inParams.requestUri, routeInstUri+"/next-hops") {
			if err := populateNiStaticRouteNextHopsStateToYang(ygRoot, niName, protoName, p, applied, ""); err != nil {
				return err
			}
		}
	}
	return nil
}
