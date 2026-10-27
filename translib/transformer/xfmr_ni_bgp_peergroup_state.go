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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	// bgpmon publishes each peer-group as FRR holds it in
	// BGP_PEER_GROUP_APPLIED|<network-instance>|<peer-group>, and each family
	// FRR has activated on it in
	// BGP_PEER_GROUP_AF_APPLIED|<network-instance>|<peer-group>|<afi-safi>,
	// keyed the way BGP_PEER_GROUP and BGP_PEER_GROUP_AF are. FRR reports a
	// peer-group's remote AS, its type and its activated families and nothing
	// else, so those are the only state leaves served.
	niBgpPgAppliedTbl   = "BGP_PEER_GROUP_APPLIED"
	niBgpPgAfAppliedTbl = "BGP_PEER_GROUP_AF_APPLIED"

	niBgpPgAppliedNameFld    = "peer_group_name"
	niBgpPgAppliedPeerAsFld  = "peer_as"
	niBgpPgAppliedTypeFld    = "peer_type"
	niBgpPgAfAppliedEnbldFld = "enabled"

	// The peer-groups come from their own vtysh calls, so they carry their own
	// freshness marker rather than sharing the neighbor one.
	niBgpPgAppliedMetaTbl     = "BGP_PEER_GROUP_APPLIED_META"
	niBgpPgAppliedMetaKey     = "global"
	niBgpPgAppliedLastSyncFld = "last_sync"
	niBgpPgAppliedMaxAge      = 60 * time.Second

	niBgpPgPeerTypeLeaf = "peer-type"

	niBgpPgStateSubtree   = "state"
	niBgpPgAfStateSubtree = "afi-safi-state"
)

var niBgpPgStateReqUriRe = regexp.MustCompile(niBgpPgUriPfx +
	`/state(?:/(peer-group-name|peer-as|peer-type))?$`)

var niBgpPgAfStateReqUriRe = regexp.MustCompile(niBgpPgUriPfx +
	`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/state(?:/(afi-safi-name|enabled))?$`)

// niBgpPgStateFldForLeaf maps the peer-group state leaves onto their producer
// field.
var niBgpPgStateFldForLeaf = map[string]string{
	niBgpPgPeerGroupNameLeaf: niBgpPgAppliedNameFld,
	niBgpPgPeerAsLeaf:        niBgpPgAppliedPeerAsFld,
	niBgpPgPeerTypeLeaf:      niBgpPgAppliedTypeFld,
}

type niBgpPgStateReqInfo struct {
	niName        string
	protoName     string
	peerGroupName string
	afiSafi       string // CONFIG_DB spelling, for the afi-safi state subtree
	targetLeaf    string // "" for the whole container, else the leaf name
	subtree       string // niBgpPgStateSubtree or niBgpPgAfStateSubtree
}

func init() {
	XlateFuncBind("DbToYang_ni_bgp_pg_state_xfmr", DbToYang_ni_bgp_pg_state_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_pg_state_xfmr", Subscribe_ni_bgp_pg_state_xfmr)
}

func parseNiBgpPgStateRequestUri(uri string) (*niBgpPgStateReqInfo, error) {
	if m := niBgpPgStateReqUriRe.FindStringSubmatch(uri); m != nil {
		if err := validateNiBgpProtoName(m[2]); err != nil {
			return nil, err
		}
		return &niBgpPgStateReqInfo{niName: m[1], protoName: m[2], peerGroupName: m[3],
			targetLeaf: m[4], subtree: niBgpPgStateSubtree}, nil
	}
	if m := niBgpPgAfStateReqUriRe.FindStringSubmatch(uri); m != nil {
		if err := validateNiBgpProtoName(m[2]); err != nil {
			return nil, err
		}
		afiSafi, ok := normalizeNiBgpPgAfiSafiName(m[4])
		if !ok {
			return nil, tlerr.NotSupportedError{
				Format: "unsupported afi-safi-name for network-instance BGP peer-group AFI-SAFI state mapping: " + m[4],
			}
		}
		return &niBgpPgStateReqInfo{niName: m[1], protoName: m[2], peerGroupName: m[3],
			afiSafi: afiSafi, targetLeaf: m[5], subtree: niBgpPgAfStateSubtree}, nil
	}
	return nil, tlerr.NotSupportedError{Format: "unsupported network-instance BGP peer-group state path: " + uri}
}

func niBgpPgAppliedIsFresh(stateDb *db.DB) bool {
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpPgAppliedMetaTbl}, db.Key{Comp: []string{niBgpPgAppliedMetaKey}})
	if err != nil {
		return false
	}
	epoch, convErr := strconv.ParseInt(strings.TrimSpace(entry.Field[niBgpPgAppliedLastSyncFld]), 10, 64)
	if convErr != nil {
		return false
	}
	// A marker dated in the future means the clock was stepped, not that the
	// producer is behind.
	age := time.Since(time.Unix(epoch, 0))
	return age < 0 || age <= niBgpPgAppliedMaxAge
}

// niBgpPgAppliedEntryFresh reads one applied row, reporting it absent behind a
// stale marker.
func niBgpPgAppliedEntryFresh(stateDb *db.DB, table string, key ...string) (db.Value, bool, error) {
	if !niBgpPgAppliedIsFresh(stateDb) {
		return db.Value{}, false, nil
	}
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: key})
	if err != nil {
		if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); notPresent {
			return db.Value{}, false, nil
		}
		return db.Value{}, false, err
	}
	return entry, true, nil
}

// niBgpPgAppliedGroups lists, per network instance, the peer-groups FRR
// reports, each with the families it has activated, while the marker is
// fresh.
func niBgpPgAppliedGroups(stateDb *db.DB, niName string) map[string][]string {
	groups := map[string][]string{}
	if stateDb == nil || !niBgpPgAppliedIsFresh(stateDb) {
		return groups
	}
	if keys, err := stateDb.GetKeys(&db.TableSpec{Name: niBgpPgAppliedTbl}); err == nil {
		for _, k := range keys {
			if len(k.Comp) == 2 && k.Comp[0] == niName && k.Comp[1] != "" {
				if _, seen := groups[k.Comp[1]]; !seen {
					groups[k.Comp[1]] = nil
				}
			}
		}
	}
	if keys, err := stateDb.GetKeys(&db.TableSpec{Name: niBgpPgAfAppliedTbl}); err == nil {
		for _, k := range keys {
			if len(k.Comp) != 3 || k.Comp[0] != niName {
				continue
			}
			if _, ok := niBgpPgAfiSafiEnumFromName(k.Comp[2]); ok {
				groups[k.Comp[1]] = append(groups[k.Comp[1]], k.Comp[2])
			}
		}
	}
	for _, afiSafis := range groups {
		sort.Strings(afiSafis)
	}
	return groups
}

// niBgpPgStateUris names the subtrees this transformer serves below one
// peer-group: its state and the state of each listed family.
func niBgpPgStateUris(niName, protoName, peerGroupName string, afiSafis []string) []string {
	pgUri := niContainerProtoUri(niName, "BGP", protoName) +
		"/bgp/peer-groups/peer-group[peer-group-name=" + peerGroupName + "]"
	uris := []string{pgUri + "/" + niBgpPgStateSubtree}
	for _, afiSafi := range afiSafis {
		uris = append(uris, pgUri+"/afi-safis/afi-safi[afi-safi-name="+strings.ToUpper(afiSafi)+"]/state")
	}
	return uris
}

// populateNiBgpPgStateReplay merges peer-group state into a read anchored
// above it, which translib does not descend into itself: neither the
// peer-group list nor its afi-safi list maps a table, so a read naming a list
// rather than one instance stops at what the config transformers fill.
func populateNiBgpPgStateReplay(inParams XfmrParams, ygRoot *ygot.GoStruct, niName, protoName, peerGroupName string, afiSafis []string, withPgState bool) error {
	for i, uri := range niBgpPgStateUris(niName, protoName, peerGroupName, afiSafis) {
		if i == 0 && !withPgState {
			continue
		}
		if !rpolNodeServesRequest(inParams.requestUri, uri) {
			continue
		}
		if err := niContainerReplay(inParams, ygRoot, uri, dbToYangNiBgpPgState); err != nil {
			return err
		}
	}
	return nil
}

// populateNiBgpAggregatePgStateReplay merges the state of every peer-group FRR
// reports into a read at or above the peer-groups container.
func populateNiBgpAggregatePgStateReplay(inParams XfmrParams, ygRoot *ygot.GoStruct, niName, protoName string) error {
	groups := niBgpPgAppliedGroups(inParams.dbs[db.StateDB], niName)
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := populateNiBgpPgStateReplay(inParams, ygRoot, niName, protoName, name, groups[name], true); err != nil {
			return err
		}
	}
	return nil
}

// dbToYangNiBgpPgStateBase serves the peer-group state FRR reports.
func dbToYangNiBgpPgStateBase(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpPgStateRequestUri(inParams.requestUri)
	if err != nil {
		// A read at the peer-group or afi-safi instance reaches this callback
		// once per subtree, with uri naming the state container while
		// requestUri still names the anchor.
		if nodeInfo, nodeErr := parseNiBgpPgStateRequestUri(inParams.uri); nodeErr == nil && nodeInfo.targetLeaf == "" {
			reqInfo, err = nodeInfo, nil
		}
	}
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
		return tlerr.InvalidArgsError{Format: "StateDB nil for network-instance BGP peer-group state"}
	}
	if reqInfo.subtree == niBgpPgAfStateSubtree {
		return populateNiBgpPgAfStateFromDb(inParams, stateDb, reqInfo)
	}
	return populateNiBgpPgStateFromDb(inParams, stateDb, reqInfo)
}

func populateNiBgpPgStateFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpPgStateReqInfo) error {
	entry, found, err := niBgpPgAppliedEntryFresh(stateDb, niBgpPgAppliedTbl, reqInfo.niName, reqInfo.peerGroupName)
	if err != nil || !found {
		return err
	}
	want := func(leaf string) bool { return reqInfo.targetLeaf == "" || reqInfo.targetLeaf == leaf }

	var name *string
	if v, ok := niBgpNbrAppliedStr(entry, niBgpPgAppliedNameFld); ok && want(niBgpPgPeerGroupNameLeaf) {
		name = &v
	}
	var peerAs *uint32
	if v, ok := niBgpNbrAppliedStr(entry, niBgpPgAppliedPeerAsFld); ok && want(niBgpPgPeerAsLeaf) {
		asn, convErr := strconv.ParseUint(v, 10, 32)
		if convErr != nil {
			return tlerr.InvalidArgsError{Format: "unrecognized " + niBgpPgAppliedTbl + "." + niBgpPgAppliedPeerAsFld +
				" value " + strconv.Quote(v) + " for peer-group " + reqInfo.peerGroupName}
		}
		asn32 := uint32(asn)
		peerAs = &asn32
	}
	peerType := ocbinds.OpenconfigBgp_PeerType_UNSET
	if v, ok := niBgpNbrAppliedStr(entry, niBgpPgAppliedTypeFld); ok && want(niBgpPgPeerTypeLeaf) {
		switch v {
		case "internal":
			peerType = ocbinds.OpenconfigBgp_PeerType_INTERNAL
		case "external":
			peerType = ocbinds.OpenconfigBgp_PeerType_EXTERNAL
		}
	}
	if name == nil && peerAs == nil && peerType == ocbinds.OpenconfigBgp_PeerType_UNSET {
		return nil
	}

	pgObj := getNiBgpPeerGroupObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, true)
	if pgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group object in YANG tree"}
	}
	if pgObj.State == nil {
		pgObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_State{}
	}
	pgObj.State.PeerGroupName = name
	pgObj.State.PeerAs = peerAs
	pgObj.State.PeerType = peerType
	return nil
}

// populateNiBgpPgAfStateFromDb reports a family FRR has activated on the
// peer-group. FRR lists only the activated families, so a family it does not
// list has no state rather than an enabled of false.
func populateNiBgpPgAfStateFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpPgStateReqInfo) error {
	entry, found, err := niBgpPgAppliedEntryFresh(stateDb, niBgpPgAfAppliedTbl, reqInfo.niName, reqInfo.peerGroupName, reqInfo.afiSafi)
	if err != nil || !found {
		return err
	}
	afiKey, ok := niBgpPgAfiSafiEnumFromName(reqInfo.afiSafi)
	if !ok {
		return nil
	}
	var enabled *bool
	if v, ok := niBgpNbrAppliedStr(entry, niBgpPgAfAppliedEnbldFld); ok &&
		(reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgEnabledLeaf) {
		b := v == "true"
		enabled = &b
	}
	wantName := reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpPgAfiSafiNameLeaf
	if enabled == nil && !wantName {
		return nil
	}

	pgReq := &niBgpPgReqInfo{niName: reqInfo.niName, protoName: reqInfo.protoName,
		peerGroupName: reqInfo.peerGroupName, afiSafi: reqInfo.afiSafi}
	root, err := wrapNiBgpPgRootAsDevice(inParams.ygRoot, pgReq)
	if err != nil {
		return err
	}
	afObj := getNiBgpPeerGroupAfObj(root, reqInfo.niName, reqInfo.protoName, reqInfo.peerGroupName, afiKey, true)
	if afObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP peer-group AFI-SAFI object in YANG tree"}
	}
	if afObj.State == nil {
		afObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_PeerGroups_PeerGroup_AfiSafis_AfiSafi_State{}
	}
	if wantName {
		afObj.State.AfiSafiName = afiKey
	}
	afObj.State.Enabled = enabled
	return nil
}

// subscribeNiBgpPgStateBase maps a subscription to peer-group state. FRR
// changes these values only when the peer-group is reconfigured, and bgpmon
// updates a changed row in place, so the paths are offered on-change; the
// marker is not watched, being rewritten every scrape. A request leaving a key
// open is resolved by the list path transformer.
func subscribeNiBgpPgStateBase(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}
	if err := validateNiBgpProtoNameInUri(inParams.uri); err != nil {
		return result, err
	}

	pathInfo := NewPathInfo(inParams.uri)
	niName := strings.TrimSpace(pathInfo.Var("name"))
	pgName := pathInfo.Var("peer-group-name")
	afiSafi := pathInfo.Var("afi-safi-name")
	if niName == "" {
		niName = "*"
	}
	if pgName == "" {
		pgName = "*"
	}

	// The subscribe path trims a trailing leaf off uri before calling this
	// callback, so the leaf asked for survives only in requestURI, and only
	// when the request is at or below this node.
	nodeUri := strings.TrimSuffix(inParams.uri, "/") + "/"
	targetLeaf := ""
	if reqUri := strings.TrimSuffix(inParams.requestURI, "/"); strings.HasPrefix(reqUri, nodeUri) {
		targetLeaf = reqUri[len(nodeUri):]
	}

	var table, key string
	fields := map[string]string{}
	if strings.Contains(inParams.uri, "/afi-safis/") {
		normalized, ok := normalizeNiBgpPgAfiSafiName(afiSafi)
		if !ok {
			normalized = "*"
		}
		table, key = niBgpPgAfAppliedTbl, niName+"|"+pgName+"|"+normalized
		// afi-safi-name is the key and has no field; a request for it watches
		// the field that comes and goes with the row.
		leaf := niBgpPgEnabledLeaf
		if targetLeaf == niBgpPgAfiSafiNameLeaf {
			leaf = niBgpPgAfiSafiNameLeaf
		}
		fields[niBgpPgAfAppliedEnbldFld] = leaf
	} else {
		table, key = niBgpPgAppliedTbl, niName+"|"+pgName
		for leaf, fld := range niBgpPgStateFldForLeaf {
			if targetLeaf == "" || targetLeaf == leaf {
				fields[fld] = leaf
			}
		}
	}

	result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {table: {key: fields}}}
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	return result, nil
}
