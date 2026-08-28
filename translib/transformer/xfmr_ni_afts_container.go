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
	"regexp"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

// A request on the afts container itself is answered by fanning out to each
// mapped subtree, because the subtrees below it are served by their own
// transformers that the infrastructure does not reach once this one claims the
// container.
var niAftsContainerUriRe = regexp.MustCompile(
	`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/afts/?$`)

func init() {
	XlateFuncBind("DbToYang_ni_afts_xfmr", DbToYang_ni_afts_xfmr)
	XlateFuncBind("Subscribe_ni_afts_xfmr", Subscribe_ni_afts_xfmr)
}

var DbToYang_ni_afts_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	m := niAftsContainerUriRe.FindStringSubmatch(inParams.requestUri)
	if m == nil {
		return tlerr.NotSupportedError{Format: "unsupported network-instance AFT path: " + inParams.requestUri}
	}
	niName := m[1]

	appDb := inParams.dbs[db.ApplDB]
	if appDb == nil {
		return tlerr.InvalidArgsError{Format: "ApplDB nil for network-instance AFT"}
	}

	if err := niAftsPopulateNextHops(inParams.ygRoot, niName, appDb, false, 0); err != nil {
		return err
	}
	if err := niAftsPopulateNextHopGroups(inParams.ygRoot, niName, appDb, niAftsGroupFilter{}); err != nil {
		return err
	}
	// Both entry lists are filled, because the container covers every family it
	// has children for. Asking for one only would drop every IPv6 route from a
	// request on the container while a request anchored on the ipv6-unicast list
	// itself still reported them.
	if err := niAftsPopulateEntries(inParams.ygRoot, niName, appDb, true, false, ""); err != nil {
		return err
	}
	return niAftsPopulateEntries(inParams.ygRoot, niName, appDb, false, false, "")
}

var Subscribe_ni_afts_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
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

	// Every list under this container is keyed on something the infrastructure
	// cannot recover from a notification key alone, so the container inherits
	// the sampling-only behaviour of the subtrees it covers.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
