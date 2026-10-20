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
	"net"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"

	log "github.com/golang/glog"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// The local aggregates of the LOCAL_AGGREGATE protocol instance are the BGP
// aggregate addresses of the network instance, one
// BGP_GLOBALS_AF_AGGREGATE_ADDR|<vrf>|<afi-safi>|<prefix> row each, the address
// family following from the prefix. The policy that sets the attributes of the
// aggregates of a family is the import-policy of the LOCAL_AGGREGATE to BGP
// table-connection, held in the policy field of every row of that family.
const (
	niLocalAggrTbl        = "BGP_GLOBALS_AF_AGGREGATE_ADDR"
	niLocalAggrAppliedTbl = "BGP_GLOBALS_AF_AGGREGATE_ADDR_APPLIED"
	niLocalAggrPolicyFld  = "policy"

	niLocalAggrProtoNameLower = "bgp"
	niLocalAggrProtoNameUpper = "BGP"

	niLocalAggrNiListNode = "network-instance"
	niLocalAggrListNode   = "aggregate"

	niLocalAggrTargetProtocolRoot = "local-aggregate-protocol-root"
	niLocalAggrTargetRoot         = "local-aggregates-root"
	niLocalAggrTargetInst         = "local-aggregate-instance"
	niLocalAggrTargetCfg          = "local-aggregate-config"
	niLocalAggrTargetState        = "local-aggregate-state"

	niLocalAggrPrefixLeaf  = "prefix"
	niLocalAggrDiscardLeaf = "discard"
)

const niLocalAggrUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=(?:[-A-Za-z0-9_.]+:)?LOCAL_AGGREGATE\]\[name=([^\]]+)\]`

// The leaves of the config and state containers. Only prefix and discard are
// served; the others are matched so that a request naming one is answered for
// what it is rather than as an unknown path.
const niLocalAggrLeaves = `(prefix|discard|set-tag|description|metric|preference)`

var niLocalAggrProtoInstanceReqUriRe = regexp.MustCompile(niLocalAggrUriPfx + `(?:/.*)?$`)
var niLocalAggrProtoRootReqUriRe = regexp.MustCompile(niLocalAggrUriPfx + `$`)
var niLocalAggrRootReqUriRe = regexp.MustCompile(niLocalAggrUriPfx + `/local-aggregates(?:/aggregate)?$`)
var niLocalAggrInstReqUriRe = regexp.MustCompile(niLocalAggrUriPfx + `/local-aggregates/aggregate\[prefix=([^\]]+)\](?:/(prefix))?$`)
var niLocalAggrCfgReqUriRe = regexp.MustCompile(niLocalAggrUriPfx + `/local-aggregates/aggregate\[prefix=([^\]]+)\]/config(?:/` + niLocalAggrLeaves + `)?$`)
var niLocalAggrStateReqUriRe = regexp.MustCompile(niLocalAggrUriPfx + `/local-aggregates/aggregate\[prefix=([^\]]+)\]/state(?:/` + niLocalAggrLeaves + `)?$`)

// A wildcard key is removed by translib before the request reaches here.
var niLocalAggrWildcardReqUriRe = regexp.MustCompile(niLocalAggrUriPfx + `/local-aggregates/aggregate/(config|state)(?:/` + niLocalAggrLeaves + `)?$`)

// The leaves a write may carry, relative to the local-aggregates container.
var niLocalAggrMappedPayloadLeaves = map[string]bool{
	"aggregate/prefix":         true,
	"aggregate/config/prefix":  true,
	"aggregate/config/discard": true,
}

type niLocalAggrReqInfo struct {
	niName         string
	protoName      string
	prefix         string
	targetLeaf     string
	targetPathKind string
}

func init() {
	XlateFuncBind("YangToDb_ni_local_aggr_cfg_xfmr", YangToDb_ni_local_aggr_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_local_aggr_cfg_xfmr", DbToYang_ni_local_aggr_cfg_xfmr)
	XlateFuncBind("Subscribe_ni_local_aggr_cfg_xfmr", Subscribe_ni_local_aggr_cfg_xfmr)
	XlateFuncBind("DbToYangPath_ni_local_aggr_path_xfmr", DbToYangPath_ni_local_aggr_path_xfmr)
}

// isNiLocalAggrReqUri reports whether uri targets the LOCAL_AGGREGATE protocol
// instance or anything below it. The protocol list carries the BGP subtree
// transformer, which hands such a request over here.
func isNiLocalAggrReqUri(uri string) bool {
	return niLocalAggrProtoInstanceReqUriRe.MatchString(uri)
}

// The protocol instance name is not part of the CONFIG_DB key, so the two
// spellings of the one BGP instance whose aggregates these are bound how many
// aliases a row has.
func validateNiLocalAggrProtoName(name string) error {
	if name == niLocalAggrProtoNameLower || name == niLocalAggrProtoNameUpper {
		return nil
	}
	return tlerr.NotSupportedError{
		Format: fmt.Sprintf("unsupported LOCAL_AGGREGATE protocol instance name %q; expected %q or %q",
			name, niLocalAggrProtoNameLower, niLocalAggrProtoNameUpper),
	}
}

// niLocalAggrAfiSafi validates an aggregate prefix and names the address family
// its row is keyed by. The prefix is the key component verbatim, so it must
// already be in normalized CIDR form.
func niLocalAggrAfiSafi(prefix string) (string, error) {
	ip, ipNet, err := net.ParseCIDR(prefix)
	if err != nil {
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid local aggregate prefix %q", prefix)}
	}
	if !ip.Equal(ipNet.IP) {
		return "", tlerr.InvalidArgsError{
			Format: fmt.Sprintf("local aggregate prefix %q is not in normalized CIDR form; expected %q", prefix, ipNet.String()),
		}
	}
	if ip.To4() != nil {
		return niBgpAfiSafiIPv4, nil
	}
	return niBgpAfiSafiIPv6, nil
}

func parseNiLocalAggrRequestUri(uri string) (*niLocalAggrReqInfo, error) {
	reqInfo, err := matchNiLocalAggrRequestUri(uri)
	if err != nil {
		return nil, err
	}
	if err := validateNiLocalAggrProtoName(reqInfo.protoName); err != nil {
		return nil, err
	}
	return reqInfo, nil
}

func matchNiLocalAggrRequestUri(uri string) (*niLocalAggrReqInfo, error) {
	mk := func(m []string, kind string, prefixIdx, leafIdx int) *niLocalAggrReqInfo {
		r := &niLocalAggrReqInfo{niName: m[1], protoName: m[2], targetPathKind: kind}
		if prefixIdx > 0 && len(m) > prefixIdx {
			r.prefix = niStaticRouteUnescapeKey(m[prefixIdx])
		}
		if leafIdx > 0 && len(m) > leafIdx {
			r.targetLeaf = m[leafIdx]
		}
		return r
	}
	if m := niLocalAggrCfgReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niLocalAggrTargetCfg, 3, 4), nil
	}
	if m := niLocalAggrStateReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niLocalAggrTargetState, 3, 4), nil
	}
	if m := niLocalAggrInstReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niLocalAggrTargetInst, 3, 4), nil
	}
	if m := niLocalAggrWildcardReqUriRe.FindStringSubmatch(uri); m != nil {
		kind := niLocalAggrTargetCfg
		if m[3] == "state" {
			kind = niLocalAggrTargetState
		}
		return mk(m, kind, 0, 4), nil
	}
	if m := niLocalAggrRootReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niLocalAggrTargetRoot, 0, 0), nil
	}
	if m := niLocalAggrProtoRootReqUriRe.FindStringSubmatch(uri); m != nil {
		return mk(m, niLocalAggrTargetProtocolRoot, 0, 0), nil
	}
	return nil, tlerr.NotSupportedError{
		Format: "unsupported path under the network-instance LOCAL_AGGREGATE protocol instance: " + uri,
		Path:   uri,
	}
}

func niLocalAggrRowKey(niName, prefix string) (string, error) {
	afiSafi, err := niLocalAggrAfiSafi(prefix)
	if err != nil {
		return "", err
	}
	return niName + "|" + afiSafi + "|" + prefix, nil
}

// niLocalAggrRows lists the aggregate rows of one network instance, prefix to
// key, optionally of one address family only.
func niLocalAggrRows(cfgDb *db.DB, niName, afiSafi string) map[string]db.Key {
	rows := map[string]db.Key{}
	if cfgDb == nil {
		return rows
	}
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niLocalAggrTbl})
	if err != nil {
		return rows
	}
	for _, k := range keys {
		if len(k.Comp) != 3 || k.Comp[0] != niName || (afiSafi != "" && k.Comp[1] != afiSafi) {
			continue
		}
		rows[k.Comp[2]] = k
	}
	return rows
}

func niLocalAggrSortedPrefixes(rows map[string]db.Key) []string {
	prefixes := make([]string, 0, len(rows))
	for p := range rows {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	return prefixes
}

// niLocalAggrFamilyPolicy reports the policy the aggregates of a family share.
// A family whose rows disagree, which only a write outside this mapping can
// produce, reports none.
func niLocalAggrFamilyPolicy(cfgDb *db.DB, rows map[string]db.Key) string {
	policy, first := "", true
	for _, k := range rows {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niLocalAggrTbl}, k)
		if err != nil {
			continue
		}
		p := strings.TrimSpace(entry.Field[niLocalAggrPolicyFld])
		if first {
			policy, first = p, false
		} else if p != policy {
			return ""
		}
	}
	return policy
}

func niLocalAggrCheckBgp(cfgDb *db.DB, niName string) error {
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance local aggregate mapping"}
	}
	if _, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsTbl}, db.Key{Comp: []string{niName}}); err != nil {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("local aggregates of network-instance %q need BGP configured there first", niName),
		}
	}
	return nil
}

// niLocalAggrNewRowFields renders a new aggregate row. A row that already
// exists is left as it is; a new one takes the policy the table-connection of
// its family has set on the others.
func niLocalAggrNewRowFields(cfgDb *db.DB, niName, prefix string) (map[string]string, error) {
	afiSafi, err := niLocalAggrAfiSafi(prefix)
	if err != nil {
		return nil, err
	}
	rows := niLocalAggrRows(cfgDb, niName, afiSafi)
	if _, exists := rows[prefix]; exists {
		return map[string]string{}, nil
	}
	if policy := niLocalAggrFamilyPolicy(cfgDb, rows); policy != "" {
		return map[string]string{niLocalAggrPolicyFld: policy}, nil
	}
	return map[string]string{"NULL": "NULL"}, nil
}

func buildNiLocalAggrEmptySubtree(obj ygot.GoStruct) {
	v := reflect.ValueOf(obj)
	if !v.IsValid() || v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	buildNiStaticRouteEmptyContainers(v.Elem(), 0)
}

// wrapNiLocalAggrRootAsDevice normalises the bind root to a Device tree. The
// request binder anchors the root at the requested node.
func wrapNiLocalAggrRootAsDevice(s *ygot.GoStruct, reqInfo *niLocalAggrReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for local aggregate mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}
	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	niObj, err := devObj.NetworkInstances.NewNetworkInstance(reqInfo.niName)
	if err != nil || niObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for local aggregate mapping"}
	}
	ygot.BuildEmptyTree(niObj)
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE, reqInfo.protoName)
	if err != nil || pObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create protocol context for local aggregate mapping"}
	}
	buildNiLocalAggrEmptySubtree(pObj)

	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol:
		niObj.Protocols.Protocol[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
			Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE,
			Name:       reqInfo.protoName,
		}] = root
		buildNiLocalAggrEmptySubtree(root)
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates:
		pObj.LocalAggregates = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate:
		if reqInfo.prefix == "" {
			return nil, tlerr.InvalidArgsError{Format: "local aggregate prefix missing in request path"}
		}
		if pObj.LocalAggregates.Aggregate == nil {
			pObj.LocalAggregates.Aggregate = map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate{}
		}
		pObj.LocalAggregates.Aggregate[reqInfo.prefix] = root
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for local aggregate mapping"}
	}
	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}

func getNiLocalAggrsObj(s *ygot.GoStruct, niName, protoName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates {
	if s == nil || *s == nil {
		return nil
	}
	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	if deviceObj.NetworkInstances == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}
	niObj := deviceObj.NetworkInstances.NetworkInstance[niName]
	if niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		if niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(niName); err != nil {
			return nil
		}
	}
	if niObj.Protocols == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	key := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE,
		Name:       protoName,
	}
	pObj := niObj.Protocols.Protocol[key]
	if pObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		if pObj, err = niObj.Protocols.NewProtocol(key.Identifier, key.Name); err != nil {
			return nil
		}
	}
	if pObj.LocalAggregates == nil {
		if !createIfMissing {
			return nil
		}
		pObj.LocalAggregates = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates{}
	}
	return pObj.LocalAggregates
}

func getNiLocalAggrObj(s *ygot.GoStruct, niName, protoName, prefix string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate {
	if s != nil && *s != nil {
		if aObj, ok := (*s).(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate); ok && aObj != nil {
			if createIfMissing {
				buildNiLocalAggrEmptySubtree(aObj)
			}
			return aObj
		}
	}
	lasObj := getNiLocalAggrsObj(s, niName, protoName, createIfMissing)
	if lasObj == nil {
		return nil
	}
	aObj := lasObj.Aggregate[prefix]
	if aObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		if aObj, err = lasObj.NewAggregate(prefix); err != nil {
			return nil
		}
	}
	if createIfMissing {
		buildNiLocalAggrEmptySubtree(aObj)
	}
	return aObj
}

// niLocalAggrCheckPayload refuses what the payload of a write carries beyond
// the prefix, and a discard of false: FRR installs every BGP aggregate with a
// discard next-hop, so that is the only behaviour there is to configure.
func niLocalAggrCheckPayload(s *ygot.GoStruct, reqInfo *niLocalAggrReqInfo) error {
	lasObj := getNiLocalAggrsObj(s, reqInfo.niName, reqInfo.protoName, false)
	if lasObj == nil {
		return nil
	}
	var unmapped []string
	collectOcUnmappedPayloadLeaves(reflect.ValueOf(lasObj), niLocalAggrMappedPayloadLeaves, "", "local-aggregates", 0, &unmapped)
	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		return tlerr.NotSupportedError{
			Format: "local aggregate payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
		}
	}
	for prefix, aObj := range lasObj.Aggregate {
		if aObj == nil || aObj.Config == nil {
			continue
		}
		if aObj.Config.Prefix != nil && strings.TrimSpace(*aObj.Config.Prefix) != "" && strings.TrimSpace(*aObj.Config.Prefix) != prefix {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("local aggregate prefix %q in payload does not match list key %q", *aObj.Config.Prefix, prefix),
			}
		}
		if aObj.Config.Discard != nil && !*aObj.Config.Discard {
			return niLocalAggrDiscardErr()
		}
	}
	return nil
}

func niLocalAggrDiscardErr() error {
	return tlerr.NotSupportedError{
		Format: "local aggregate discard false is not supported: FRR installs every BGP aggregate with a discard next-hop",
	}
}

// buildNiLocalAggrResMap maps every aggregate a payload carries.
func buildNiLocalAggrResMap(cfgDb *db.DB, s *ygot.GoStruct, reqInfo *niLocalAggrReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	lasObj := getNiLocalAggrsObj(s, reqInfo.niName, reqInfo.protoName, false)
	if lasObj == nil {
		return res, nil
	}
	for prefix, aObj := range lasObj.Aggregate {
		if aObj == nil {
			continue
		}
		key, err := niLocalAggrRowKey(reqInfo.niName, prefix)
		if err != nil {
			return nil, err
		}
		fields, err := niLocalAggrNewRowFields(cfgDb, reqInfo.niName, prefix)
		if err != nil {
			return nil, err
		}
		if _, ok := res[niLocalAggrTbl]; !ok {
			res[niLocalAggrTbl] = make(map[string]db.Value)
		}
		res[niLocalAggrTbl][key] = db.Value{Field: fields}
	}
	return res, nil
}

func niLocalAggrDeleteAllForNi(cfgDb *db.DB, niName string) map[string]map[string]db.Value {
	res := make(map[string]map[string]db.Value)
	for _, k := range niLocalAggrRows(cfgDb, niName, "") {
		if _, ok := res[niLocalAggrTbl]; !ok {
			res[niLocalAggrTbl] = make(map[string]db.Value)
		}
		res[niLocalAggrTbl][strings.Join(k.Comp, "|")] = db.Value{Field: map[string]string{}}
	}
	return res
}

var YangToDb_ni_local_aggr_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (res map[string]map[string]db.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance local aggregate transformer panic in SET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			res = make(map[string]map[string]db.Value)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance local aggregate transformer panic in SET path: %v", r)}
		}
	}()
	res = make(map[string]map[string]db.Value)

	reqInfo, err := parseNiLocalAggrRequestUri(niHandlerReqUri(inParams))
	if err != nil {
		return res, err
	}
	cfgDb := inParams.dbs[db.ConfigDB]

	if reqInfo.targetPathKind == niLocalAggrTargetState {
		return res, tlerr.NotSupportedError{
			Format: "local aggregate state is read-only; write to the config container instead",
			Path:   inParams.requestUri,
		}
	}

	if inParams.oper == DELETE {
		switch reqInfo.targetPathKind {
		case niLocalAggrTargetProtocolRoot, niLocalAggrTargetRoot:
			return niLocalAggrDeleteAllForNi(cfgDb, reqInfo.niName), nil
		case niLocalAggrTargetInst, niLocalAggrTargetCfg:
			switch reqInfo.targetLeaf {
			case "", niLocalAggrPrefixLeaf:
				// The prefix is the row identity, so clearing it removes the
				// aggregate.
				key, keyErr := niLocalAggrRowKey(reqInfo.niName, reqInfo.prefix)
				if keyErr != nil {
					return res, keyErr
				}
				return map[string]map[string]db.Value{niLocalAggrTbl: {key: db.Value{Field: map[string]string{}}}}, nil
			case niLocalAggrDiscardLeaf:
				return res, tlerr.NotSupportedError{
					Format: "local aggregate discard cannot be cleared: FRR installs every BGP aggregate with a discard next-hop",
					Path:   inParams.requestUri,
				}
			default:
				return res, tlerr.NotSupportedError{
					Format: "local aggregate " + reqInfo.targetLeaf + " has no CONFIG_DB mapping",
					Path:   inParams.requestUri,
				}
			}
		}
		return res, tlerr.NotSupportedError{Format: "unsupported DELETE path under local aggregate mapping", Path: inParams.requestUri}
	}

	if err := niLocalAggrCheckBgp(cfgDb, reqInfo.niName); err != nil {
		return res, err
	}
	switch reqInfo.targetLeaf {
	case "", niLocalAggrPrefixLeaf, niLocalAggrDiscardLeaf:
	default:
		return res, tlerr.NotSupportedError{
			Format: "local aggregate " + reqInfo.targetLeaf + " has no CONFIG_DB mapping",
			Path:   inParams.requestUri,
		}
	}
	wrappedRoot, wrapErr := wrapNiLocalAggrRootAsDevice(inParams.ygRoot, reqInfo)
	if wrapErr != nil {
		return res, wrapErr
	}
	if err := niLocalAggrCheckPayload(wrappedRoot, reqInfo); err != nil {
		return res, err
	}

	switch reqInfo.targetPathKind {
	case niLocalAggrTargetProtocolRoot, niLocalAggrTargetRoot:
		res, err = buildNiLocalAggrResMap(cfgDb, wrappedRoot, reqInfo)
		if err != nil {
			return make(map[string]map[string]db.Value), err
		}
		if len(res) == 0 {
			return res, tlerr.InvalidArgsError{Format: "local-aggregates payload does not include any aggregate"}
		}
		if inParams.oper == REPLACE {
			// The container is replaced as a whole, so an aggregate the payload
			// leaves out goes.
			clear := map[string]db.Value{}
			for _, k := range niLocalAggrRows(cfgDb, reqInfo.niName, "") {
				if key := strings.Join(k.Comp, "|"); res[niLocalAggrTbl][key].Field == nil {
					clear[key] = db.Value{Field: map[string]string{}}
				}
			}
			if len(clear) > 0 {
				if err := niBgpStageSubOp(inParams, DELETE, map[string]map[string]db.Value{niLocalAggrTbl: clear}); err != nil {
					return make(map[string]map[string]db.Value), err
				}
			}
		}
		if inParams.invokeCRUSubtreeOnce != nil {
			*inParams.invokeCRUSubtreeOnce = true
		}
		return res, nil
	case niLocalAggrTargetInst, niLocalAggrTargetCfg:
		if reqInfo.targetLeaf == niLocalAggrDiscardLeaf {
			if discard, ok := inParams.param.(*bool); ok && discard != nil && !*discard {
				return res, niLocalAggrDiscardErr()
			}
			if aObj := getNiLocalAggrObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, false); aObj != nil &&
				aObj.Config != nil && aObj.Config.Discard != nil && !*aObj.Config.Discard {
				return res, niLocalAggrDiscardErr()
			}
		}
		key, keyErr := niLocalAggrRowKey(reqInfo.niName, reqInfo.prefix)
		if keyErr != nil {
			return res, keyErr
		}
		fields, fldErr := niLocalAggrNewRowFields(cfgDb, reqInfo.niName, reqInfo.prefix)
		if fldErr != nil {
			return res, fldErr
		}
		if reqInfo.targetPathKind == niLocalAggrTargetInst && inParams.invokeCRUSubtreeOnce != nil {
			*inParams.invokeCRUSubtreeOnce = true
		}
		return map[string]map[string]db.Value{niLocalAggrTbl: {key: db.Value{Field: fields}}}, nil
	}
	return res, tlerr.NotSupportedError{Format: "unsupported path under the network-instance LOCAL_AGGREGATE protocol instance", Path: inParams.requestUri}
}

func niLocalAggrAppliedEntry(inParams XfmrParams, niName, prefix string) (db.Value, bool) {
	stDb := inParams.dbs[db.StateDB]
	afiSafi, err := niLocalAggrAfiSafi(prefix)
	if stDb == nil || err != nil {
		return db.Value{}, false
	}
	entry, err := stDb.GetEntry(&db.TableSpec{Name: niLocalAggrAppliedTbl}, db.Key{Comp: []string{niName, afiSafi, prefix}})
	if err != nil || len(entry.Field) == 0 {
		return db.Value{}, false
	}
	return entry, true
}

// populateNiLocalAggrConfig fills the config of one aggregate. discard reports
// what FRR does with every BGP aggregate.
func populateNiLocalAggrConfig(aObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate, prefix, leaf string) {
	p, discard := prefix, true
	aObj.Prefix = &p
	if aObj.Config == nil {
		aObj.Config = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate_Config{}
	}
	if leaf == "" || leaf == niLocalAggrPrefixLeaf {
		aObj.Config.Prefix = &p
	}
	if leaf == "" || leaf == niLocalAggrDiscardLeaf {
		aObj.Config.Discard = &discard
	}
}

// populateNiLocalAggrState fills the state of one aggregate from what FRR
// holds, and reports whether FRR holds it.
func populateNiLocalAggrState(inParams XfmrParams, aObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate, niName, prefix, leaf string) bool {
	if _, held := niLocalAggrAppliedEntry(inParams, niName, prefix); !held {
		return false
	}
	p, discard := prefix, true
	if aObj.State == nil {
		aObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_LocalAggregates_Aggregate_State{}
	}
	if leaf == "" || leaf == niLocalAggrPrefixLeaf {
		aObj.State.Prefix = &p
	}
	if leaf == "" || leaf == niLocalAggrDiscardLeaf {
		aObj.State.Discard = &discard
	}
	return true
}

var DbToYang_ni_local_aggr_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance local aggregate transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance local aggregate transformer panic in GET path: %v", r)}
		}
	}()

	reqInfo, err := parseNiLocalAggrRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance local aggregate mapping"}
	}
	switch reqInfo.targetLeaf {
	case "", niLocalAggrPrefixLeaf, niLocalAggrDiscardLeaf:
	default:
		// No CONFIG_DB mapping, so nothing to report.
		return nil
	}

	switch reqInfo.targetPathKind {
	case niLocalAggrTargetProtocolRoot, niLocalAggrTargetRoot, niLocalAggrTargetInst:
		rows := niLocalAggrRows(cfgDb, reqInfo.niName, "")
		prefixes := niLocalAggrSortedPrefixes(rows)
		if reqInfo.targetPathKind == niLocalAggrTargetInst {
			if _, ok := rows[reqInfo.prefix]; !ok {
				return tlerr.NotFoundError{Format: fmt.Sprintf("local aggregate %s not found", reqInfo.prefix)}
			}
			prefixes = []string{reqInfo.prefix}
		}
		wrappedRoot, wrapErr := wrapNiLocalAggrRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return wrapErr
		}
		for _, p := range prefixes {
			aObj := getNiLocalAggrObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, p, true)
			if aObj == nil {
				continue
			}
			populateNiLocalAggrConfig(aObj, p, "")
			populateNiLocalAggrState(inParams, aObj, reqInfo.niName, p, "")
		}
		return nil
	case niLocalAggrTargetCfg, niLocalAggrTargetState:
		if reqInfo.prefix == "" {
			return tlerr.NotSupportedError{Format: "a wildcard or absent local aggregate prefix is served at the aggregate list; address local-aggregates/aggregate rather than a container below it"}
		}
		if _, ok := niLocalAggrRows(cfgDb, reqInfo.niName, "")[reqInfo.prefix]; !ok && reqInfo.targetPathKind == niLocalAggrTargetCfg {
			return tlerr.NotFoundError{Format: fmt.Sprintf("local aggregate %s not found", reqInfo.prefix)}
		}
		aObj := getNiLocalAggrObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, true)
		if aObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate local aggregate object in YANG tree"}
		}
		if reqInfo.targetPathKind == niLocalAggrTargetCfg {
			populateNiLocalAggrConfig(aObj, reqInfo.prefix, reqInfo.targetLeaf)
			return nil
		}
		populateNiLocalAggrState(inParams, aObj, reqInfo.niName, reqInfo.prefix, reqInfo.targetLeaf)
		return nil
	}
	return nil
}

var niLocalAggrSubscReqUriRe = regexp.MustCompile(niLocalAggrUriPfx +
	`/local-aggregates(?:/aggregate(?:\[prefix=([^\]]+)\])?(?:/(.*))?)?$`)

// Subscribe_ni_local_aggr_cfg_xfmr maps the configured aggregates and, for
// state, what FRR holds. Both change only with the configuration, so every
// node is on-change; a row arriving or going away is the whole story, the
// prefix being the key and discard constant.
var Subscribe_ni_local_aggr_cfg_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}
	anchor := inParams.uri
	if anchor == "" {
		anchor = inParams.requestURI
	}
	m := niLocalAggrSubscReqUriRe.FindStringSubmatch(anchor)
	if m == nil || validateNiLocalAggrProtoName(m[2]) != nil {
		result.isVirtualTbl = true
		return result, nil
	}
	niName, prefix, below := m[1], niStaticRouteUnescapeKey(m[3]), m[4]

	// A pattern matches only a key of as many components, and the address
	// family is one of them.
	keyPatt := niName + "|*|*"
	if prefix != "" && prefix != "*" {
		key, err := niLocalAggrRowKey(niName, prefix)
		if err != nil {
			return result, err
		}
		keyPatt = key
	}
	dbNum, table := db.ConfigDB, niLocalAggrTbl
	if below == "state" || strings.HasPrefix(below, "state/") {
		dbNum, table = db.StateDB, niLocalAggrAppliedTbl
	}
	result.dbDataMap = RedisDbSubscribeMap{dbNum: {table: {keyPatt: map[string]string{}}}}
	result.isVirtualTbl = false
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	return result, nil
}

// DbToYangPath_ni_local_aggr_path_xfmr names the instance and the aggregate a
// configured or applied row belongs to, for a subscription that left the key
// open.
var DbToYangPath_ni_local_aggr_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != niLocalAggrTbl && params.tblName != niLocalAggrAppliedTbl {
		return fmt.Errorf("ni_local_aggr_path_xfmr: notification from table %q: %w", params.tblName, apis.ErrUnrelatedEntry)
	}
	if len(params.tblKeyComp) != 3 || params.tblKeyComp[0] == "" || params.tblKeyComp[2] == "" {
		return fmt.Errorf("ni_local_aggr_path_xfmr: %s key %v does not name an instance, a family and a prefix",
			params.tblName, params.tblKeyComp)
	}
	niPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niLocalAggrNiListNode)
	if !ok {
		return nil
	}
	params.ygPathKeys[niPath+"/name"] = params.tblKeyComp[0]
	if aggrPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niLocalAggrListNode); ok {
		params.ygPathKeys[aggrPath+"/"+niLocalAggrPrefixLeaf] = params.tblKeyComp[2]
	}
	return nil
}

// niLocalAggrYangToDbDispatch handles a SET on the LOCAL_AGGREGATE protocol
// instance on behalf of the transformer bound to the protocol list.
func niLocalAggrYangToDbDispatch(inParams XfmrParams) (bool, map[string]map[string]db.Value, error) {
	if !isNiLocalAggrReqUri(niHandlerReqUri(inParams)) {
		return false, nil, nil
	}
	res, err := YangToDb_ni_local_aggr_cfg_xfmr(inParams)
	return true, res, err
}

func niLocalAggrDbToYangDispatch(inParams XfmrParams) (bool, error) {
	if !isNiLocalAggrReqUri(inParams.requestUri) {
		return false, nil
	}
	return true, DbToYang_ni_local_aggr_cfg_xfmr(inParams)
}

// The LOCAL_AGGREGATE to BGP table-connection of a family has no row of its
// own. FRR originates every aggregate in BGP, so the table-connection exists
// while the family has an aggregate, and its import-policy is the route-map
// the aggregates of the family are announced with.
func niTableConnIsLocalAggr(proto string) bool {
	return niTableConnNormalizeToken(proto) == "LOCAL_AGGREGATE"
}

func niLocalAggrTableConnFamily(dstProtocol, addressFamily string) (string, error) {
	if niTableConnNormalizeToken(dstProtocol) != "BGP" {
		return "", tlerr.NotSupportedError{Format: "a LOCAL_AGGREGATE table-connection is served only towards BGP"}
	}
	switch niTableConnNormalizeToken(addressFamily) {
	case "IPV4":
		return niBgpAfiSafiIPv4, nil
	case "IPV6":
		return niBgpAfiSafiIPv6, nil
	}
	return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported table-connection address-family: %s", addressFamily)}
}

func niLocalAggrNoAggregateErr(niName, afiSafi string) error {
	return tlerr.InvalidArgsError{
		Format: fmt.Sprintf("network-instance %q has no %s local aggregate; the LOCAL_AGGREGATE table-connection of a family exists while one is configured",
			niName, strings.SplitN(afiSafi, "_", 2)[0]),
	}
}

// niLocalAggrPolicyRows sets, or with an empty policy clears, the policy of
// every aggregate in rows. A clear names only the rows that hold one.
func niLocalAggrPolicyRows(cfgDb *db.DB, rows map[string]db.Key, policy string) map[string]map[string]db.Value {
	out := map[string]db.Value{}
	for _, k := range rows {
		if policy == "" {
			entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niLocalAggrTbl}, k)
			if err != nil || !entry.Has(niLocalAggrPolicyFld) {
				continue
			}
		}
		out[strings.Join(k.Comp, "|")] = db.Value{Field: map[string]string{niLocalAggrPolicyFld: policy}}
	}
	if len(out) == 0 {
		return map[string]map[string]db.Value{}
	}
	return map[string]map[string]db.Value{niLocalAggrTbl: out}
}

// niLocalAggrTableConnYangToDb maps a write at or below one LOCAL_AGGREGATE
// table-connection instance onto the aggregates of its family.
func niLocalAggrTableConnYangToDb(inParams XfmrParams, reqInfo *niTableConnReqInfo, root *ygot.GoStruct) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	afiSafi, err := niLocalAggrTableConnFamily(reqInfo.dstProtocol, reqInfo.addressFamily)
	if err != nil {
		return res, err
	}
	cfgDb := inParams.dbs[db.ConfigDB]
	rows := niLocalAggrRows(cfgDb, reqInfo.niName, afiSafi)

	switch reqInfo.targetLeaf {
	case "", niTableConnImportPolicyLeaf:
	default:
		if inParams.oper == DELETE {
			return res, tlerr.NotSupportedError{
				Format: "DELETE is not supported for a table-connection list key; delete the table-connection instead",
				Path:   inParams.requestUri,
			}
		}
		// A key leaf repeats the key in the path.
		if len(rows) == 0 {
			return res, niLocalAggrNoAggregateErr(reqInfo.niName, afiSafi)
		}
		return res, nil
	}

	if inParams.oper == DELETE {
		// The aggregates stay; what goes is the policy the table-connection
		// set on them.
		return niLocalAggrPolicyRows(cfgDb, rows, ""), nil
	}
	if len(rows) == 0 {
		return res, niLocalAggrNoAggregateErr(reqInfo.niName, afiSafi)
	}
	policy, found, err := niTableConnImportPolicyFromReq(inParams, root, reqInfo)
	if err != nil {
		return res, err
	}
	if reqInfo.targetPath == niTableConnTargetInst && inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}
	if found {
		return niLocalAggrPolicyRows(cfgDb, rows, policy), nil
	}
	if reqInfo.targetLeaf == niTableConnImportPolicyLeaf {
		return res, tlerr.InvalidArgsError{Format: "table-connection import-policy is missing in request payload"}
	}
	if inParams.oper == REPLACE {
		if err := niBgpStageSubOp(inParams, DELETE, niLocalAggrPolicyRows(cfgDb, rows, "")); err != nil {
			return res, err
		}
	}
	return res, nil
}

// niLocalAggrTableConnHierRows maps the LOCAL_AGGREGATE table-connections a
// table-connections payload carries.
func niLocalAggrTableConnHierRows(cfgDb *db.DB, s *ygot.GoStruct, niName string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	tcsObj := getNiTableConnsObj(s, niName, false)
	if tcsObj == nil {
		return res, nil
	}
	for key, tcObj := range tcsObj.TableConnection {
		if tcObj == nil || key.SrcProtocol != ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE {
			continue
		}
		afiSafi, err := niLocalAggrTableConnFamily(niTableConnProtocolEnumToOcName(key.DstProtocol), niTableConnAddressFamilyEnumToOcName(key.AddressFamily))
		if err != nil {
			return nil, err
		}
		rows := niLocalAggrRows(cfgDb, niName, afiSafi)
		if len(rows) == 0 {
			return nil, niLocalAggrNoAggregateErr(niName, afiSafi)
		}
		if tcObj.Config == nil || len(tcObj.Config.ImportPolicy) == 0 {
			continue
		}
		policy, err := parseNiTableConnSingleImportPolicy(tcObj.Config.ImportPolicy)
		if err != nil {
			return nil, err
		}
		if err := niContainerMergeResMap(res, niLocalAggrPolicyRows(cfgDb, rows, policy), "table-connections"); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// niLocalAggrTableConnEntry renders the derived table-connection of a family as
// the ROUTE_REDISTRIBUTE row it would be, so it is read the way the others are.
func niLocalAggrTableConnEntry(cfgDb *db.DB, niName, afiSafi string) (db.Value, bool) {
	rows := niLocalAggrRows(cfgDb, niName, afiSafi)
	if len(rows) == 0 {
		return db.Value{}, false
	}
	entry := db.Value{Field: map[string]string{}}
	if policy := niLocalAggrFamilyPolicy(cfgDb, rows); policy != "" {
		entry.Field[niTableConnRouteMapFld] = policy
	}
	return entry, true
}

// niLocalAggrTableConnEntries lists the derived table-connections of a network
// instance, by address family.
func niLocalAggrTableConnEntries(cfgDb *db.DB, niName string) map[ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY]db.Value {
	out := map[ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY]db.Value{}
	for af, afiSafi := range map[ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY]string{
		ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4: niBgpAfiSafiIPv4,
		ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6: niBgpAfiSafiIPv6,
	} {
		if entry, ok := niLocalAggrTableConnEntry(cfgDb, niName, afiSafi); ok {
			out[af] = entry
		}
	}
	return out
}

// niLocalAggrTableConnInPayload reports whether a table-connections payload
// names a LOCAL_AGGREGATE table-connection, which maps nothing when it carries
// no policy and is still a valid request.
func niLocalAggrTableConnInPayload(s *ygot.GoStruct, niName string) bool {
	tcsObj := getNiTableConnsObj(s, niName, false)
	if tcsObj == nil {
		return false
	}
	for key := range tcsObj.TableConnection {
		if key.SrcProtocol == ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE {
			return true
		}
	}
	return false
}
