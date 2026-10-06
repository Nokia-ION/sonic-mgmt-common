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

	// The route-maps FRR applies to the family, from the same scrape. They are
	// the effective policies, so a neighbor inheriting one from its peer-group
	// reports it here while its own BGP_NEIGHBOR_AF row is empty.
	niBgpNbrAfStatsImportPolicyFld = "import_policy"
	niBgpNbrAfStatsExportPolicyFld = "export_policy"

	niBgpNbrPfxRcvdLeaf          = "received"
	niBgpNbrPfxRcvdPrePolicyLeaf = "received-pre-policy"
	niBgpNbrPfxSentLeaf          = "sent"

	niBgpNbrImportPolicyLeaf = "import-policy"
	niBgpNbrExportPolicyLeaf = "export-policy"

	// The two subtrees this transformer serves below an afi-safi instance.
	niBgpNbrAfStateSubtree       = "state"
	niBgpNbrAfApplyPolicySubtree = "apply-policy"
)

// The afi-safi state container sits below the neighbor list, so it reuses the
// neighbor uri prefix and adds the afi-safi list key. Both the state container
// and the prefixes container are matched so that a request anchored at either is
// served the counters below it rather than falling back to the afi-safi list
// dispatch, which maps CONFIG_DB only.
var niBgpNbrAfStateReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/state(?:/prefixes(?:/(received|received-pre-policy|sent))?)?$`)

// apply-policy/state is matched apart from the state container above, since
// the same leaf names hang off apply-policy/config, which this transformer
// does not own.
var niBgpNbrAfApplyPolicyStateReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/apply-policy/state(?:/(import-policy|export-policy|default-import-policy|default-export-policy))?$`)

// niBgpNbrAfApplyPolicyFldForLeaf maps the apply-policy state leaves onto their
// producer field. The default policies are deliberately absent: FRR has no
// such setting to report, and the config side refuses them for the same
// reason, so the leaves are annotated to keep the paths valid but never
// populated.
var niBgpNbrAfApplyPolicyFldForLeaf = map[string]string{
	niBgpNbrImportPolicyLeaf: niBgpNbrAfStatsImportPolicyFld,
	niBgpNbrExportPolicyLeaf: niBgpNbrAfStatsExportPolicyFld,
}

// niBgpNbrAfStatsFldForLeaf maps the prefix counter leaves onto their producer
// field. received-pre-policy is deliberately absent: FRR reports no pre-policy
// count, so the leaf is annotated to keep the path valid but is never populated.
var niBgpNbrAfStatsFldForLeaf = map[string]string{
	niBgpNbrPfxRcvdLeaf: niBgpNbrAfStatsPfxRcvdFld,
	niBgpNbrPfxSentLeaf: niBgpNbrAfStatsPfxSentFld,
}

type niBgpNbrAfStateReqInfo struct {
	niName       string
	protoName    string
	neighborAddr string
	afiSafi      string                                     // CONFIG_DB spelling, as the row is keyed
	afiSafiEnum  ocbinds.E_OpenconfigBgpTypes_AFI_SAFI_TYPE // as the YANG list is keyed
	targetLeaf   string                                     // "" for the whole container, else the leaf name
	subtree      string                                     // niBgpNbrAfStateSubtree or niBgpNbrAfApplyPolicySubtree
}

func init() {
	XlateFuncBind("DbToYang_ni_bgp_nbr_af_state_xfmr", DbToYang_ni_bgp_nbr_af_state_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_nbr_af_state_xfmr", Subscribe_ni_bgp_nbr_af_state_xfmr)
}

func parseNiBgpNbrAfStateRequestUri(uri string) (*niBgpNbrAfStateReqInfo, error) {
	subtree := niBgpNbrAfStateSubtree
	m := niBgpNbrAfStateReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		subtree = niBgpNbrAfApplyPolicySubtree
		m = niBgpNbrAfApplyPolicyStateReqUriRe.FindStringSubmatch(uri)
	}
	if m == nil {
		return nil, tlerr.NotSupportedError{
			Format: "unsupported network-instance BGP neighbor AFI-SAFI state path: " + uri,
		}
	}
	if err := validateNiBgpProtoName(m[2]); err != nil {
		return nil, err
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
		protoName:    m[2],
		neighborAddr: m[3],
		afiSafi:      afiSafi,
		afiSafiEnum:  afiSafiEnum,
		targetLeaf:   m[5],
		subtree:      subtree,
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

// getNiBgpNbrAfStateAfObj locates one neighbor address family in the YANG
// tree, creating the ancestors when asked to. A request anchored on the
// afi-safis container or on an afi-safi instance hands every transformer below
// it that node as the root rather than a device tree, so the root is
// normalised the way the config transformer does for those anchors.
func getNiBgpNbrAfStateAfObj(s *ygot.GoStruct, reqInfo *niBgpNbrAfStateReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi {
	root, err := wrapNiBgpNbrRootAsDevice(s, &niBgpNbrReqInfo{
		niName:       reqInfo.niName,
		protoName:    reqInfo.protoName,
		neighborAddr: reqInfo.neighborAddr,
		afiSafi:      reqInfo.afiSafi,
	})
	if err != nil {
		return nil
	}
	nbrObj := getNiBgpNbrObj(root, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, createIfMissing)
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
	return afObj
}

// getNiBgpNbrAfPrefixesObj locates the prefixes container of one neighbor
// address family in the YANG tree, creating the ancestors when asked to.
func getNiBgpNbrAfPrefixesObj(s *ygot.GoStruct, reqInfo *niBgpNbrAfStateReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_State_Prefixes {
	afObj := getNiBgpNbrAfStateAfObj(s, reqInfo, createIfMissing)
	if afObj == nil {
		return nil
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

// getNiBgpNbrAfApplyPolicyStateObj locates the apply-policy state container of
// one neighbor address family, creating the ancestors when asked to.
func getNiBgpNbrAfApplyPolicyStateObj(s *ygot.GoStruct, reqInfo *niBgpNbrAfStateReqInfo, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy_State {
	afObj := getNiBgpNbrAfStateAfObj(s, reqInfo, createIfMissing)
	if afObj == nil {
		return nil
	}
	if afObj.ApplyPolicy == nil {
		if !createIfMissing {
			return nil
		}
		afObj.ApplyPolicy = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy{}
	}
	if afObj.ApplyPolicy.State == nil {
		if !createIfMissing {
			return nil
		}
		afObj.ApplyPolicy.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_ApplyPolicy_State{}
	}
	return afObj.ApplyPolicy.State
}

var DbToYang_ni_bgp_nbr_af_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpNbrAfStateRequestUri(inParams.requestUri)
	if err != nil {
		// A request anchored above apply-policy/state, from the apply-policy
		// container up to the neighbor, reaches this callback with uri naming
		// the state container while requestUri still names the anchor.
		if nodeInfo, nodeErr := parseNiBgpNbrAfStateRequestUri(inParams.uri); nodeErr == nil &&
			nodeInfo.subtree == niBgpNbrAfApplyPolicySubtree && nodeInfo.targetLeaf == "" {
			reqInfo, err = nodeInfo, nil
		}
	}
	if err != nil {
		fallback, nameErr := niBgpContainerLevelFallback(inParams.requestUri)
		if nameErr != nil {
			return nameErr
		}
		if fallback {
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
	if reqInfo.subtree == niBgpNbrAfApplyPolicySubtree {
		return populateNiBgpNbrAfApplyPolicyStateFromDb(inParams, stateDb, reqInfo)
	}
	return populateNiBgpNbrAfPrefixesFromDb(inParams, stateDb, reqInfo)
}

// populateNiBgpNbrAfApplyPolicyStateFromDb fills the policies FRR applies to
// one neighbor address family. FRR applies at most one route-map per
// direction, so each is reported as a single element list, and a direction
// with none is left absent rather than reported as an empty list.
func populateNiBgpNbrAfApplyPolicyStateFromDb(inParams XfmrParams, stateDb *db.DB, reqInfo *niBgpNbrAfStateReqInfo) error {
	if reqInfo.targetLeaf != "" {
		if _, mapped := niBgpNbrAfApplyPolicyFldForLeaf[reqInfo.targetLeaf]; !mapped {
			return nil
		}
	}

	entry, found, err := niBgpNbrAfStatsFresh(stateDb, reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	importPolicy, hasImport := niBgpNbrAppliedStr(entry, niBgpNbrAfStatsImportPolicyFld)
	exportPolicy, hasExport := niBgpNbrAppliedStr(entry, niBgpNbrAfStatsExportPolicyFld)
	wantImport := (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrImportPolicyLeaf) && hasImport
	wantExport := (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niBgpNbrExportPolicyLeaf) && hasExport
	if !wantImport && !wantExport {
		return nil
	}

	stateObj := getNiBgpNbrAfApplyPolicyStateObj(inParams.ygRoot, reqInfo, true)
	if stateObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor AFI-SAFI apply-policy state object in YANG tree"}
	}
	if wantImport {
		stateObj.ImportPolicy = []string{importPolicy}
	}
	if wantExport {
		stateObj.ExportPolicy = []string{exportPolicy}
	}
	return nil
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

	// The protocol instance name is part of no watched key, so without this an
	// unserved spelling would be given a working watch on a path that a GET
	// refuses.
	if err := validateNiBgpProtoNameInUri(inParams.uri); err != nil {
		return result, err
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

	if strings.HasSuffix(strings.TrimSuffix(inParams.uri, "/"), "/"+niBgpNbrAfApplyPolicySubtree+"/"+niBgpNbrAfStateSubtree) {
		return subscribeNiBgpNbrAfApplyPolicyState(inParams, niName+"|"+neighborAddr+"|"+afiSafi,
			niName != "*" && neighborAddr != "*" && afiSafi != "*")
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

// subscribeNiBgpNbrAfApplyPolicyState maps a subscription to apply-policy/state.
// The policies share their row with the prefix counters, so only the policy
// fields are registered: translib notifies on-change per mapped field, and a
// counter moving on the same row must not wake a policy subscriber.
//
// bgpmon updates a changed row in place and names the fields that went away,
// so a policy swapped or removed reaches a subscriber as an update or a leaf
// delete rather than as the row being rebuilt. That is what makes these
// leaves on-change capable where the counters beside them are not.
func subscribeNiBgpNbrAfApplyPolicyState(inParams XfmrSubscInParams, afKey string, keyed bool) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	// The subscribe path trims a trailing leaf off uri before calling this
	// callback, so which leaf was asked for survives only in requestURI.
	stateUri := strings.TrimSuffix(inParams.uri, "/") + "/"
	targetLeaf := ""
	if reqUri := strings.TrimSuffix(inParams.requestURI, "/"); strings.HasPrefix(reqUri, stateUri) {
		targetLeaf = reqUri[len(stateUri):]
	}

	fields := map[string]string{}
	for leaf, fld := range niBgpNbrAfApplyPolicyFldForLeaf {
		if targetLeaf == "" || targetLeaf == leaf {
			fields[fld] = leaf
		}
	}

	// A default policy leaf has no producer field, so there is nothing to
	// watch. It is answered as a virtual table, which accepts the subscription
	// and reports nothing, the way a GET of the leaf does.
	if len(fields) == 0 {
		result.isVirtualTbl = true
		result.onChange = OnchangeEnable
		return result, nil
	}

	result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrAfStatsTbl: {afKey: fields}}}

	// BGP_NEIGH_APPLIED_META is not registered, for the reason the transport
	// leaves give: its marker is rewritten every scrape. An unkeyed request is
	// sampled, because its notification key would have to be resolved back
	// from the Redis key, which needs a path transformer this table has no
	// SONiC YANG model to support.
	if keyed {
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		return result, nil
	}
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}
	return result, nil
}
