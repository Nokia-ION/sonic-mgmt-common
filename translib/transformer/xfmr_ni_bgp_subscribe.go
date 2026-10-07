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
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

func init() {
	XlateFuncBind("Subscribe_ni_bgp_global_cfg_xfmr", Subscribe_ni_bgp_global_cfg_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_global_state_xfmr", Subscribe_ni_bgp_global_state_xfmr)
	XlateFuncBind("Subscribe_ni_bgp_pg_cfg_xfmr", Subscribe_ni_bgp_pg_cfg_xfmr)
}

// niBgpUriKeysRe strips the list keys from a uri, so that a node is
// recognised by its shape whichever keys the request binds.
var niBgpUriKeysRe = regexp.MustCompile(`\[[^\]]*\]`)

// The key a mapped row is read under, built from the keys of the subscribed
// path. A key the path leaves open is a wildcard, which the list path
// transformer resolves when a notification arrives.
type niBgpSubscKey int

const (
	niBgpSubscKeyNi niBgpSubscKey = iota
	niBgpSubscKeyNiNbr
	niBgpSubscKeyNiNbrAf
	niBgpSubscKeyNiNbrAllAfs
	niBgpSubscKeyNiPg
	niBgpSubscKeyNiPgAf
	niBgpSubscKeyNiPgIPv4
	niBgpSubscKeyNiPgIPv6
	niBgpSubscKeyNiAf
)

// niBgpSubscMap describes one node a BGP transformer owns: the table and row
// it reads and the field behind each of its leaves, which are the fields its
// GET reads. A leaf in sampled is one that changes with traffic rather than
// with configuration; a request covering one is sampled as a whole.
type niBgpSubscMap struct {
	dbNum   db.DBNum
	table   string
	key     niBgpSubscKey
	fields  map[string][]string
	sampled map[string]bool
}

func niBgpSubscVar(pathInfo *PathInfo, name string) string {
	v := strings.TrimSpace(pathInfo.Var(name))
	if v == "" {
		return "*"
	}
	return v
}

func niBgpSubscAf(pathInfo *PathInfo) string {
	if normalized, ok := normalizeNiBgpNbrAfiSafiName(pathInfo.Var("afi-safi-name")); ok {
		return normalized
	}
	return "*"
}

func (k niBgpSubscKey) build(pathInfo *PathInfo) string {
	ni := niBgpSubscVar(pathInfo, "name")
	switch k {
	case niBgpSubscKeyNiNbr:
		return ni + "|" + niBgpSubscVar(pathInfo, "neighbor-address")
	case niBgpSubscKeyNiNbrAf:
		return ni + "|" + niBgpSubscVar(pathInfo, "neighbor-address") + "|" + niBgpSubscAf(pathInfo)
	case niBgpSubscKeyNiNbrAllAfs:
		return ni + "|" + niBgpSubscVar(pathInfo, "neighbor-address") + "|*"
	case niBgpSubscKeyNiPg:
		return ni + "|" + niBgpSubscVar(pathInfo, "peer-group-name")
	case niBgpSubscKeyNiPgAf:
		return ni + "|" + niBgpSubscVar(pathInfo, "peer-group-name") + "|" + niBgpSubscAf(pathInfo)
	case niBgpSubscKeyNiPgIPv4:
		return ni + "|" + niBgpSubscVar(pathInfo, "peer-group-name") + "|" + niBgpPgAfiSafiIPv4
	case niBgpSubscKeyNiPgIPv6:
		return ni + "|" + niBgpSubscVar(pathInfo, "peer-group-name") + "|" + niBgpPgAfiSafiIPv6
	case niBgpSubscKeyNiAf:
		return ni + "|" + niBgpSubscAf(pathInfo)
	}
	return ni
}

// niBgpSubscribe maps a subscription to one node of a BGP transformer from the
// node's description. A node that holds no field of its own is a virtual
// table, which lets translib reach its children's mappings on-change. A node
// the transformer owns but does not describe, and every call that is not a
// subscription, gets what translib assumes for a transformer that binds no
// subscribe callback. That includes the CRUD parent-table check, which calls
// with requestURI unset and must find no table of this transformer's to verify.
func niBgpSubscribe(inParams XfmrSubscInParams, anchor string, virtual map[string]bool,
	maps map[string]niBgpSubscMap, fieldFor func(node string, pathInfo *PathInfo, leaf string) []string) (XfmrSubscOutParams, error) {
	result := XfmrSubscOutParams{onChange: OnchangeDisable}
	if inParams.subscProc != TRANSLATE_SUBSCRIBE || inParams.requestURI == "" {
		return result, nil
	}
	nodeUri := strings.TrimSuffix(inParams.uri, "/")
	bareUri := niBgpUriKeysRe.ReplaceAllString(nodeUri, "")
	idx := strings.Index(bareUri, anchor)
	if idx < 0 || !niBgpUriIsBgpProtocol(nodeUri) {
		return result, nil
	}
	node := bareUri[idx+len(anchor):]
	if virtual[node] {
		result.isVirtualTbl = true
		result.onChange = OnchangeEnable
		return result, nil
	}
	subsc, ok := maps[node]
	if !ok {
		return result, nil
	}
	if err := validateNiBgpProtoNameInUri(inParams.uri); err != nil {
		return result, err
	}
	pathInfo := NewPathInfo(inParams.uri)

	// The subscribe path trims a trailing leaf off uri before calling this
	// callback, so which leaf was asked for survives only in requestURI, and
	// only when the request is at or below this node.
	targetLeaf := ""
	if reqUri := strings.TrimSuffix(inParams.requestURI, "/"); strings.HasPrefix(reqUri, nodeUri+"/") {
		targetLeaf = reqUri[len(nodeUri)+1:]
	}
	fields := map[string]string{}
	sampled := false
	for leaf, flds := range subsc.fields {
		if targetLeaf != "" && targetLeaf != leaf {
			continue
		}
		if fieldFor != nil {
			if picked := fieldFor(node, pathInfo, leaf); picked != nil {
				flds = picked
			}
		}
		for _, fld := range flds {
			fields[fld] = leaf
		}
		sampled = sampled || subsc.sampled[leaf]
	}
	// A leaf with no field, such as a key leaf or a default policy the write
	// side refuses, has nothing to watch and reports nothing, as a GET does.
	if len(fields) == 0 {
		result.isVirtualTbl = true
		result.onChange = OnchangeEnable
		return result, nil
	}

	result.dbDataMap = RedisDbSubscribeMap{subsc.dbNum: {subsc.table: {subsc.key.build(pathInfo): fields}}}
	if sampled {
		result.nOpts = &notificationOpts{pType: Sample}
		return result, nil
	}
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	return result, nil
}

// niBgpUriIsBgpProtocol reports a uri below the BGP protocol instance. The
// global transformer is annotated on the protocol list, which every protocol
// shares, so a node of another protocol is left alone.
func niBgpUriIsBgpProtocol(uri string) bool {
	pathInfo := NewPathInfo(uri)
	id := pathInfo.Var("identifier")
	if i := strings.LastIndex(id, ":"); i >= 0 {
		id = id[i+1:]
	}
	return id == "BGP"
}

const niBgpProtoSubscAnchor = "/protocols/protocol"

// The nodes the global transformer owns that hold no field of their own:
// containers and lists whose content is all children with mappings of their
// own, here or in the neighbor, peer-group and state transformers.
var niBgpGlobalSubscVirtual = map[string]bool{
	"/bgp":                                                   true,
	"/bgp/global":                                            true,
	"/bgp/global/route-selection-options":                    true,
	"/bgp/global/use-multiple-paths":                         true,
	"/bgp/global/use-multiple-paths/ebgp":                    true,
	"/bgp/global/use-multiple-paths/ibgp":                    true,
	"/bgp/global/afi-safis":                                  true,
	"/bgp/global/afi-safis/afi-safi":                         true,
	"/bgp/global/afi-safis/afi-safi/use-multiple-paths":      true,
	"/bgp/global/afi-safis/afi-safi/use-multiple-paths/ebgp": true,
	"/bgp/global/afi-safis/afi-safi/use-multiple-paths/ibgp": true,
	"/bgp/neighbors":                                         true,
	"/bgp/neighbors/neighbor":                                true,
	"/bgp/neighbors/neighbor/timers":                         true,
	"/bgp/neighbors/neighbor/as-path-options":                true,
	"/bgp/peer-groups":                                       true,
	"/bgp/peer-groups/peer-group":                            true,
	"/bgp/peer-groups/peer-group/timers":                     true,
}

var niBgpGlobalSubscMaps = map[string]niBgpSubscMap{
	"/bgp/global/config": {dbNum: db.ConfigDB, table: niBgpGlobalsTbl, key: niBgpSubscKeyNi, fields: map[string][]string{
		"as":        {niBgpAsFld},
		"router-id": {niBgpRouterIdFld},
	}},
	"/bgp/global/route-selection-options/config": {dbNum: db.ConfigDB, table: niBgpGlobalsTbl, key: niBgpSubscKeyNi, fields: map[string][]string{
		"always-compare-med": {niBgpAlwaysCompareMedFld},
	}},
	"/bgp/global/use-multiple-paths/ebgp/config": {dbNum: db.ConfigDB, table: niBgpGlobalsTbl, key: niBgpSubscKeyNi, fields: map[string][]string{
		"maximum-paths": {niBgpMaxEbgpPathsFld},
	}},
	"/bgp/global/use-multiple-paths/ibgp/config": {dbNum: db.ConfigDB, table: niBgpGlobalsTbl, key: niBgpSubscKeyNi, fields: map[string][]string{
		"maximum-paths": {niBgpMaxIbgpPathsFld},
	}},
	// Activating a family is the default unicast flag on the BGP_GLOBALS row,
	// one field per family, rather than a row of its own.
	"/bgp/global/afi-safis/afi-safi/config": {dbNum: db.ConfigDB, table: niBgpGlobalsTbl, key: niBgpSubscKeyNi, fields: map[string][]string{
		"enabled": {niBgpDefaultIpv4UnicastFld, niBgpDefaultIpv6UnicastFld},
	}},
	"/bgp/global/afi-safis/afi-safi/use-multiple-paths/ebgp/config": {dbNum: db.ConfigDB, table: niBgpGlobalsAfTbl, key: niBgpSubscKeyNiAf, fields: map[string][]string{
		"maximum-paths": {niBgpMaxEbgpPathsFld},
	}},
	"/bgp/global/afi-safis/afi-safi/use-multiple-paths/ibgp/config": {dbNum: db.ConfigDB, table: niBgpGlobalsAfTbl, key: niBgpSubscKeyNiAf, fields: map[string][]string{
		"maximum-paths": {niBgpMaxIbgpPathsFld},
	}},
}

// niBgpGlobalAfEnabledFld narrows the enabled leaf of a global afi-safi to the
// one family the path names.
func niBgpGlobalAfEnabledFld(node string, pathInfo *PathInfo, leaf string) []string {
	if node != "/bgp/global/afi-safis/afi-safi/config" || leaf != "enabled" {
		return nil
	}
	if fld, err := niBgpGlobalDefaultUnicastFldForAfi(niBgpSubscAf(pathInfo)); err == nil {
		return []string{fld}
	}
	return nil
}

// Subscribe_ni_bgp_global_cfg_xfmr maps a subscription to the BGP global
// config containers, on-change, and lets the container and list nodes the
// global transformer owns reach their children's mappings.
var Subscribe_ni_bgp_global_cfg_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	return niBgpSubscribe(inParams, niBgpProtoSubscAnchor, niBgpGlobalSubscVirtual, niBgpGlobalSubscMaps, niBgpGlobalAfEnabledFld)
}

// BGP global state is the BGP process as FRR holds it. The as and router-id
// change only with configuration, and bgpmon updates the row in place, so
// they are offered on-change; the totals move with the routes and are
// sampled, as every counter is.
var niBgpGlobalStateSubscMaps = map[string]niBgpSubscMap{
	"/bgp/global/state": {dbNum: db.StateDB, table: niBgpGlobalsAppliedTbl, key: niBgpSubscKeyNi,
		fields: map[string][]string{
			niBgpGlobalStateAsLeaf:            {niBgpGlobalsAppliedAsFld},
			niBgpGlobalStateRouterIdLeaf:      {niBgpGlobalsAppliedRouterIdFld},
			niBgpGlobalStateTotalPathsLeaf:    {niBgpGlobalsAppliedTotalPathsFld},
			niBgpGlobalStateTotalPrefixesLeaf: {niBgpGlobalsAppliedTotalPrefixesFld},
		},
		sampled: map[string]bool{niBgpGlobalStateTotalPathsLeaf: true, niBgpGlobalStateTotalPrefixesLeaf: true},
	},
}

var Subscribe_ni_bgp_global_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	result, err := niBgpSubscribe(inParams, niBgpProtoSubscAnchor, nil, niBgpGlobalStateSubscMaps, nil)
	if err == nil && inParams.subscProc != TRANSLATE_SUBSCRIBE {
		// Not a table this transformer writes, so there is nothing for the
		// CRUD and existence checks to verify.
		result.isVirtualTbl = true
	}
	return result, err
}

const niBgpPgSubscAnchor = "/bgp/peer-groups/peer-group"

var niBgpPgSubscVirtual = map[string]bool{
	"/afi-safis":                                    true,
	"/afi-safis/afi-safi":                           true,
	"/afi-safis/afi-safi/apply-policy":              true,
	"/afi-safis/afi-safi/ipv4-unicast":              true,
	"/afi-safis/afi-safi/ipv4-unicast/prefix-limit": true,
	"/afi-safis/afi-safi/ipv6-unicast":              true,
	"/afi-safis/afi-safi/ipv6-unicast/prefix-limit": true,
}

var niBgpPgSubscMaps = map[string]niBgpSubscMap{
	"/config": {dbNum: db.ConfigDB, table: niBgpPeerGroupTbl, key: niBgpSubscKeyNiPg, fields: map[string][]string{
		niBgpPgPeerAsLeaf: {niBgpPgAsnFld},
	}},
	"/timers/config": {dbNum: db.ConfigDB, table: niBgpPeerGroupTbl, key: niBgpSubscKeyNiPg, fields: map[string][]string{
		niBgpPgKeepaliveLeaf: {niBgpPgKeepaliveFld},
		niBgpPgHoldTimeLeaf:  {niBgpPgHoldTimeFld},
	}},
	"/afi-safis/afi-safi/config": {dbNum: db.ConfigDB, table: niBgpPeerGroupAfTbl, key: niBgpSubscKeyNiPgAf, fields: map[string][]string{
		niBgpPgEnabledLeaf:      {niBgpPgAdminStatusFld},
		niBgpPgSendCommTypeLeaf: {niBgpPgSendCommFld},
	}},
	"/afi-safis/afi-safi/apply-policy/config": {dbNum: db.ConfigDB, table: niBgpPeerGroupAfTbl, key: niBgpSubscKeyNiPgAf, fields: map[string][]string{
		niBgpPgImportPolicyLeaf: {niBgpPgRouteMapInFld},
		niBgpPgExportPolicyLeaf: {niBgpPgRouteMapOutFld},
	}},
	"/afi-safis/afi-safi/ipv4-unicast/prefix-limit/config": {dbNum: db.ConfigDB, table: niBgpPeerGroupAfTbl, key: niBgpSubscKeyNiPgIPv4, fields: map[string][]string{
		niBgpPgMaxPrefixesLeaf: {niBgpPgMaxPfxLimitFld},
	}},
	"/afi-safis/afi-safi/ipv6-unicast/prefix-limit/config": {dbNum: db.ConfigDB, table: niBgpPeerGroupAfTbl, key: niBgpSubscKeyNiPgIPv6, fields: map[string][]string{
		niBgpPgMaxPrefixesLeaf: {niBgpPgMaxPfxLimitFld},
	}},
}

// Subscribe_ni_bgp_pg_cfg_xfmr maps a subscription to the peer-group config
// containers, on-change.
var Subscribe_ni_bgp_pg_cfg_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	return niBgpSubscribe(inParams, niBgpPgSubscAnchor, niBgpPgSubscVirtual, niBgpPgSubscMaps, nil)
}
