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

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	// bgpmon publishes the per-address-family prefix counters as
	// BGP_NEIGH_AF_STATS|<network-instance>|<neighbor-address>|<afi-safi>, from
	// the same "show bgp vrf all neighbors json" scrape that fills
	// BGP_NEIGH_APPLIED and BGP_NEIGH_STATS. The address family is part of the
	// key rather than folded into field names, which keys the row the way the
	// OpenConfig afi-safi list is keyed and lets one family be read without
	// parsing the other's fields.
	niBgpNbrAfStatsTbl = "BGP_NEIGH_AF_STATS"

	niBgpNbrAfStatsPfxRcvdFld = "prefixes_received"
	niBgpNbrAfStatsPfxSentFld = "prefixes_sent"

	niBgpNbrPfxRcvdLeaf          = "received"
	niBgpNbrPfxRcvdPrePolicyLeaf = "received-pre-policy"
	niBgpNbrPfxSentLeaf          = "sent"
)

// The afi-safi state container sits below the neighbor list, so it reuses the
// neighbor uri prefix and adds the afi-safi list key. Both the state container
// and the prefixes container are matched so that a request anchored at either is
// served the counters below it rather than falling back to the afi-safi list
// dispatch, which maps CONFIG_DB only.
var niBgpNbrAfStateReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/state(?:/prefixes(?:/(received|received-pre-policy|sent))?)?$`)

// niBgpNbrAfStatsFldForLeaf maps the prefix counter leaves onto their producer
// field. received-pre-policy is deliberately absent: FRR reports no pre-policy
// count, so the leaf is annotated to keep the path valid but is never populated.
var niBgpNbrAfStatsFldForLeaf = map[string]string{
	niBgpNbrPfxRcvdLeaf: niBgpNbrAfStatsPfxRcvdFld,
	niBgpNbrPfxSentLeaf: niBgpNbrAfStatsPfxSentFld,
}

type niBgpNbrAfStateReqInfo struct {
	niName       string
	neighborAddr string
	afiSafi      string                                     // CONFIG_DB spelling, as the row is keyed
	afiSafiEnum  ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE // as the YANG list is keyed
	targetLeaf   string                                     // "" for the whole container, else the leaf name
}

func init() {
	XlateFuncBind("DbToYang_ni_bgp_nbr_af_state_xfmr", DbToYang_ni_bgp_nbr_af_state_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_nbr_af_state_xfmr", Subscribe_ni_bgp_nbr_af_state_xfmr)
}

func parseNiBgpNbrAfStateRequestUri(uri string) (*niBgpNbrAfStateReqInfo, error) {
	m := niBgpNbrAfStateReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{
			Format: "unsupported network-instance BGP neighbor AFI-SAFI state path: " + uri,
		}
	}
	if m[2] != "bgp" {
		return nil, tlerr.NotSupportedError{
			Format: fmt.Sprintf("unsupported BGP protocol instance name %q for neighbor AFI-SAFI state; only \"bgp\" is mapped", m[2]),
		}
	}
	afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
	if !ok {
		return nil, tlerr.NotSupportedError{
			Format: "unsupported afi-safi-name for network-instance BGP neighbor AFI-SAFI state mapping: " + m[4],
		}
	}
	afiSafiEnum, ok := niBgpNbrAfiSafiEnumFromName(afiSafi)
	if !ok {
		return nil, tlerr.NotSupportedError{
			Format: "unsupported afi-safi-name for network-instance BGP neighbor AFI-SAFI state mapping: " + m[4],
		}
	}
	return &niBgpNbrAfStateReqInfo{
		niName:       m[1],
		neighborAddr: m[3],
		afiSafi:      afiSafi,
		afiSafiEnum:  afiSafiEnum,
		targetLeaf:   m[5],
	}, nil
}

// niBgpNbrAfStatsFresh returns the prefix counter row for one neighbor address
// family. It is gated on the same marker as the applied row and the session
// counters, because one scrape writes all three tables, so a marker that has
// gone stale has left all three behind.
func niBgpNbrAfStatsFresh(stateDb *db.DB, niName, neighborAddr, afiSafi string) (db.Value, bool, error) {
	if !niBgpNbrAppliedIsFresh(stateDb) {
		return db.Value{}, false, nil
	}
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfStatsTbl},
		db.Key{Comp: []string{niName, neighborAddr, afiSafi}})
	if err != nil {
		// A family the producer has never reported is absent rather than an
		// error, which is the normal state for a family that is not activated
		// on the neighbor and before the first scrape completes.
		if _, notPresent := err.(tlerr.TranslibRedisClientEntryNotExist); notPresent {
			return db.Value{}, false, nil
		}
		return db.Value{}, false, err
	}
	return entry, true, nil
}

// niBgpNbrAfStatsUint32 reads one prefix counter. An absent field means FRR
// reported no value, which leaves the leaf unset; a zero is a real count and is
// reported, since zero is what a session under RFC 8212 policy reports. A value
// that is not a number, or does not fit the leaf, is a producer fault and fails
// the request rather than being dropped.
func niBgpNbrAfStatsUint32(entry db.Value, field string, reqInfo *niBgpNbrAfStateReqInfo) (uint32, bool, error) {
	raw, ok := niBgpNbrAppliedStr(entry, field)
	if !ok {
		return 0, false, nil
	}
	val, convErr := strconv.ParseUint(raw, 10, 32)
	if convErr != nil {
		return 0, false, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("unrecognized %s.%s value %q for neighbor %s address family %s",
				niBgpNbrAfStatsTbl, field, raw, reqInfo.neighborAddr, reqInfo.afiSafi),
		}
	}
	return uint32(val), true, nil
}

// getNiBgpNbrAfPrefixesObj locates the prefixes container of one neighbor
// address family in the YANG tree, creating the ancestors when asked to.
func getNiBgpNbrAfPrefixesObj(s *ygot.GoStruct, reqInfo *niBgpNbrAfStateReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_State_Prefixes {
	nbrObj := getNiBgpNbrObj(s, reqInfo.niName, reqInfo.neighborAddr, createIfMissing)
	if nbrObj == nil {
		return nil
	}
	if nbrObj.AfiSafis == nil {
		if !createIfMissing {
			return nil
		}
		nbrObj.AfiSafis = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis{}
	}
	afObj, ok := nbrObj.AfiSafis.AfiSafi[reqInfo.afiSafiEnum]
	if !ok || afObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		afObj, err = nbrObj.AfiSafis.NewAfiSafi(reqInfo.afiSafiEnum)
		if err != nil || afObj == nil {
			return nil
		}
	}
	if afObj.State == nil {
		if !createIfMissing {
			return nil
		}
		afObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_State{}
	}
	if afObj.State.Prefixes == nil {
		if !createIfMissing {
			return nil
		}
		afObj.State.Prefixes = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_State_Prefixes{}
	}
	return afObj.State.Prefixes
}

var DbToYang_ni_bgp_nbr_af_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpNbrAfStateRequestUri(inParams.requestUri)
	if err != nil {
		if isNiBgpContainerLevelReqUri(inParams.requestUri) {
			// The request targets a node above this subtree, so the BGP
			// container dispatch owns it and has already populated what it
			// maps. Reporting an error here would fail that request.
			return nil
		}
		return err
	}

	stateDb := inParams.dbs[db.StateDB]
	if stateDb == nil {
		return tlerr.InvalidArgsError{Format: "StateDB nil for network-instance BGP neighbor AFI-SAFI state"}
	}
	return populateNiBgpNbrAfPrefixesFromDb(inParams, stateDb, reqInfo)
}

// populateNiBgpNbrAfPrefixesFromDb fills the prefix counters of one neighbor
// address family. The container is created only once a counter is known to be
// present, so a request for a counter FRR is not reporting is answered as absent
// rather than being filled with the zero value of its type.
func populateNiBgpNbrAfPrefixesFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpNbrAfStateReqInfo) error {
	// received-pre-policy counts the prefixes received before inbound policy is
	// applied. FRR reports no such counter in the neighbor scrape, and the
	// accepted count it does report is the post-policy one, so reporting that
	// here would be wrong under any inbound filtering. The leaf is annotated so
	// the path is valid and answers as absent, which is what "no value
	// available" looks like for every other leaf in this subtree.
	if reqInfo.targetLeaf == niBgpNbrPfxRcvdPrePolicyLeaf {
		return nil
	}

	entry, found, err := niBgpNbrAfStatsFresh(stateDb, reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	received, hasReceived, err := niBgpNbrAfStatsUint32(entry, niBgpNbrAfStatsPfxRcvdFld, reqInfo)
	if err != nil {
		return err
	}
	sent, hasSent, err := niBgpNbrAfStatsUint32(entry, niBgpNbrAfStatsPfxSentFld, reqInfo)
	if err != nil {
		return err
	}

	wantReceived := (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrPfxRcvdLeaf) && hasReceived
	wantSent := (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrPfxSentLeaf) && hasSent
	if !wantReceived && !wantSent {
		return nil
	}

	prefixesObj := getNiBgpNbrAfPrefixesObj(inParams.ygRoot, reqInfo, true)
	if prefixesObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor AFI-SAFI prefixes object in YANG tree"}
	}
	if wantReceived {
		count := received
		prefixesObj.Received = &count
	}
	if wantSent {
		count := sent
		prefixesObj.Sent = &count
	}
	return nil
}

var Subscribe_ni_bgp_nbr_af_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	pathInfo := NewPathInfo(inParams.uri)
	neighborAddr := pathInfo.Var("neighbor-address")
	if neighborAddr == "" {
		neighborAddr = "*"
	}
	niName := strings.TrimSpace(pathInfo.Var("name"))
	if niName == "" {
		niName = "*"
	}
	// An unresolvable address family is left as a wildcard rather than failing
	// the subscription, so a request spanning the afi-safi list is offered the
	// rows of every family it covers.
	afiSafi := "*"
	if raw := pathInfo.Var("afi-safi-name"); raw != "" {
		if normalized, ok := normalizeNiBgpNbrAfiSafiName(raw); ok {
			afiSafi = normalized
		}
	}

	result.isVirtualTbl = false
	result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrAfStatsTbl: {
		niName + "|" + neighborAddr + "|" + afiSafi: {}}}}

	// Offered sampling only, for the same reason as the applied leaves and the
	// session counters: the producer rewrites a changed row as a delete followed
	// by a set, so a field FRR stopped reporting disappears instead of going
	// stale, and on-change would turn each of those rewrites into a spurious
	// delete notification.
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}

	return result, nil
}
