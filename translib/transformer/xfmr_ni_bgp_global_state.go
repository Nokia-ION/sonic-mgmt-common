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
	"strconv"
	"strings"
	"time"

	log "github.com/golang/glog"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	// bgpmon publishes the BGP process as FRR holds it, keyed by network
	// instance, in BGP_GLOBALS_APPLIED|<vrf>. The field names follow CONFIG_DB
	// BGP_GLOBALS where they overlap so operational as and router-id are not
	// confused with what was merely requested.
	niBgpGlobalsAppliedTbl = "BGP_GLOBALS_APPLIED"

	niBgpGlobalsAppliedAsFld            = "local_asn"
	niBgpGlobalsAppliedRouterIdFld      = "router_id"
	niBgpGlobalsAppliedTotalPathsFld    = "total_paths"
	niBgpGlobalsAppliedTotalPrefixesFld = "total_prefixes"

	niBgpGlobalsAppliedMetaTbl     = "BGP_GLOBALS_APPLIED_META"
	niBgpGlobalsAppliedMetaKey     = "global"
	niBgpGlobalsAppliedLastSyncFld = "last_sync"
	niBgpGlobalsAppliedMaxAge      = 60 * time.Second

	niBgpGlobalStateAsLeaf            = "as"
	niBgpGlobalStateRouterIdLeaf      = "router-id"
	niBgpGlobalStateTotalPathsLeaf    = "total-paths"
	niBgpGlobalStateTotalPrefixesLeaf = "total-prefixes"

	niBgpTargetGlobalState = "global-state"
)

var niBgpGlobalStateReqUriRe = regexp.MustCompile(niBgpProtoUriPfx +
	`/bgp/global/state(?:/(as|router-id|total-paths|total-prefixes))?$`)

type niBgpGlobalStateReqInfo struct {
	niName         string
	protoName      string
	targetLeaf     string
	targetPathKind string
}

func init() {
	XlateFuncBind("DbToYang_ni_bgp_global_state_xfmr", DbToYang_ni_bgp_global_state_xfmr)
}

func parseNiBgpGlobalStateRequestUri(uri string) (*niBgpGlobalStateReqInfo, error) {
	m := niBgpGlobalStateReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported network-instance BGP global state path: " + uri}
	}
	if err := validateNiBgpProtoName(m[2]); err != nil {
		return nil, err
	}
	leaf := ""
	if len(m) >= 4 {
		leaf = m[3]
	}
	return &niBgpGlobalStateReqInfo{
		niName:         m[1],
		protoName:      m[2],
		targetLeaf:     leaf,
		targetPathKind: niBgpTargetGlobalState,
	}, nil
}

func getNiBgpGlobalStateObj(s *ygot.GoStruct, niName, protoName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_State {
	if getNiBgpGlobalCfgObj(s, niName, protoName, createIfMissing) == nil {
		return nil
	}
	if s == nil || *s == nil {
		return nil
	}
	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	niObj := deviceObj.NetworkInstances.NetworkInstance[niName]
	if niObj == nil || niObj.Protocols == nil || niObj.Protocols.Protocol == nil {
		return nil
	}
	wantKey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       protoName,
	}
	pObj, ok := niObj.Protocols.Protocol[wantKey]
	if !ok || pObj == nil || pObj.Bgp == nil || pObj.Bgp.Global == nil {
		return nil
	}
	if pObj.Bgp.Global.State == nil && createIfMissing {
		pObj.Bgp.Global.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_State{}
	}
	return pObj.Bgp.Global.State
}

func niBgpGlobalsAppliedMetaIsFresh(stateDb *db.DB) bool {
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsAppliedMetaTbl}, db.Key{Comp: []string{niBgpGlobalsAppliedMetaKey}})
	if err != nil {
		return false
	}
	raw := strings.TrimSpace(entry.Field[niBgpGlobalsAppliedLastSyncFld])
	if raw == "" {
		return false
	}
	epoch, convErr := strconv.ParseInt(raw, 10, 64)
	if convErr != nil {
		return false
	}
	age := time.Since(time.Unix(epoch, 0))
	if age < 0 {
		return true
	}
	return age <= niBgpGlobalsAppliedMaxAge
}

func niBgpGlobalsAppliedEntry(stateDb *db.DB, niName string) (db.Value, bool, error) {
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsAppliedTbl}, db.Key{Comp: []string{niName}})
	if err != nil {
		if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); notPresent {
			return db.Value{}, false, nil
		}
		return db.Value{}, false, err
	}
	if len(entry.Field) == 0 {
		return db.Value{}, false, nil
	}
	return entry, true, nil
}

func niBgpGlobalsAppliedEntryFresh(stateDb *db.DB, niName string) (db.Value, bool, error) {
	if !niBgpGlobalsAppliedMetaIsFresh(stateDb) {
		return db.Value{}, false, nil
	}
	return niBgpGlobalsAppliedEntry(stateDb, niName)
}

func populateNiBgpGlobalStateFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpGlobalStateReqInfo) error {
	entry, found, err := niBgpGlobalsAppliedEntryFresh(stateDb, reqInfo.niName)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	stateObj := getNiBgpGlobalStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, true)
	if stateObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP global state object in YANG tree"}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpGlobalStateAsLeaf {
		if asStr, ok := entry.Field[niBgpGlobalsAppliedAsFld]; ok && strings.TrimSpace(asStr) != "" {
			if asVal, convErr := strconv.ParseUint(strings.TrimSpace(asStr), 10, 32); convErr == nil {
				asU32 := uint32(asVal)
				stateObj.As = &asU32
			}
		}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpGlobalStateRouterIdLeaf {
		if rid, ok := entry.Field[niBgpGlobalsAppliedRouterIdFld]; ok && strings.TrimSpace(rid) != "" {
			rid = strings.TrimSpace(rid)
			stateObj.RouterId = &rid
		}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpGlobalStateTotalPathsLeaf {
		if tp, ok := niBgpGlobalsAppliedUint32(entry, niBgpGlobalsAppliedTotalPathsFld); ok {
			stateObj.TotalPaths = &tp
		}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpGlobalStateTotalPrefixesLeaf {
		if tp, ok := niBgpGlobalsAppliedUint32(entry, niBgpGlobalsAppliedTotalPrefixesFld); ok {
			stateObj.TotalPrefixes = &tp
		}
	}
	return nil
}

func niBgpGlobalsAppliedUint32(entry db.Value, fld string) (uint32, bool) {
	raw, ok := entry.Field[fld]
	if !ok || strings.TrimSpace(raw) == "" {
		return 0, false
	}
	val, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(val), true
}

var DbToYang_ni_bgp_global_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance BGP global state transformer panic in GET path for %s: %v", inParams.requestUri, r)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance BGP global state transformer panic in GET path: %v", r)}
		}
	}()

	reqInfo, err := parseNiBgpGlobalStateRequestUri(inParams.requestUri)
	if err != nil {
		fallback, nameErr := niBgpContainerLevelFallback(inParams.requestUri)
		if nameErr != nil {
			return nameErr
		}
		if fallback {
			return nil
		}
		return err
	}
	stateDb := inParams.dbs[db.StateDB]
	if stateDb == nil {
		return tlerr.InvalidArgsError{Format: "StateDB nil for network-instance BGP global state"}
	}
	return populateNiBgpGlobalStateFromDb(inParams, stateDb, reqInfo)
}
