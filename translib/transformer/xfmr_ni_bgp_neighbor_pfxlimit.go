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
)

// A neighbor holds one maximum-prefix limit per address family. prefix-limit
// counts the prefixes accepted by inbound policy and prefix-limit-received
// every prefix received, which FRR expresses as the "force" keyword on the
// same command. Both containers therefore share the fields of the family's
// BGP_NEIGHBOR_AF row, max_prefix_force tells which of them is in effect, and
// the most recent write to either container wins.
const (
	niBgpNbrTargetPfxLimitCfg = "neighbor-af-prefix-limit-config"
	// The family container and the prefix-limit containers above config.
	niBgpNbrTargetAfFamily = "neighbor-af-family"

	niBgpNbrPfxLimitCont    = "prefix-limit"
	niBgpNbrPfxLimitRcvCont = "prefix-limit-received"

	niBgpNbrMaxPrefixesLeaf     = "max-prefixes"
	niBgpNbrPreventTeardownLeaf = "prevent-teardown"
	niBgpNbrWarnThresholdLeaf   = "warning-threshold-pct"

	niBgpNbrMaxPfxFld         = "max_prefix_limit"
	niBgpNbrMaxPfxWarnOnlyFld = "max_prefix_warning_only"
	niBgpNbrMaxPfxThreshFld   = "max_prefix_warning_threshold"
	niBgpNbrMaxPfxForceFld    = "max_prefix_force"
)

var niBgpNbrAfPfxLimitReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(ipv4-unicast|ipv6-unicast)/(prefix-limit|prefix-limit-received)/config(?:/(max-prefixes|prevent-teardown|warning-threshold-pct))?$`)
var niBgpNbrAfFamilyReqUriRe = regexp.MustCompile(niBgpNbrUriPfx + `/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(ipv4-unicast|ipv6-unicast)(?:/(prefix-limit|prefix-limit-received))?$`)

// niBgpNbrPfxLimitFlds are the fields either container writes, and so the
// ones a switch between them or a delete of either clears.
var niBgpNbrPfxLimitFlds = []string{niBgpNbrMaxPfxFld, niBgpNbrMaxPfxWarnOnlyFld, niBgpNbrMaxPfxThreshFld, niBgpNbrMaxPfxForceFld}

// matchNiBgpNbrPfxLimitUri matches the family container and everything below
// it that this transformer serves. It reports false for any other uri.
func matchNiBgpNbrPfxLimitUri(uri string) (*niBgpNbrReqInfo, bool, error) {
	target, leaf, pfxCont := "", "", ""
	m := niBgpNbrAfPfxLimitReqUriRe.FindStringSubmatch(uri)
	if len(m) == 8 {
		target, pfxCont, leaf = niBgpNbrTargetPfxLimitCfg, m[6], m[7]
	} else if m = niBgpNbrAfFamilyReqUriRe.FindStringSubmatch(uri); len(m) == 7 {
		target, pfxCont = niBgpNbrTargetAfFamily, m[6]
	} else {
		return nil, false, nil
	}
	afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
	if !ok {
		return nil, true, tlerr.InvalidArgsError{
			Format: "unsupported afi-safi-name for network-instance BGP neighbor prefix-limit mapping: " + m[4],
		}
	}
	if fam := strings.ReplaceAll(m[5], "-", "_"); fam != afiSafi {
		return nil, true, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("BGP neighbor %s container does not belong to afi-safi %s", m[5], m[4]),
		}
	}
	return &niBgpNbrReqInfo{
		niName:       m[1],
		protoName:    m[2],
		neighborAddr: m[3],
		afiSafi:      afiSafi,
		targetLeaf:   leaf,
		targetPath:   target,
		pfxCont:      pfxCont,
	}, true, nil
}

// niBgpNbrPfxLeaves points at the three leaves of one prefix-limit config
// container. The IPv4 and IPv6 containers are distinct types of one shape.
type niBgpNbrPfxLeaves struct {
	max      **uint32
	teardown **bool
	pct      **uint8
}

func niBgpNbrPfxCfgLeaves(afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi, afiSafi string, received bool) *niBgpNbrPfxLeaves {
	if afObj == nil {
		return nil
	}
	switch afiSafi {
	case niBgpNbrAfiSafiIPv4:
		if afObj.Ipv4Unicast == nil {
			return nil
		}
		if received {
			if c := afObj.Ipv4Unicast.PrefixLimitReceived; c != nil && c.Config != nil {
				return &niBgpNbrPfxLeaves{&c.Config.MaxPrefixes, &c.Config.PreventTeardown, &c.Config.WarningThresholdPct}
			}
		} else if c := afObj.Ipv4Unicast.PrefixLimit; c != nil && c.Config != nil {
			return &niBgpNbrPfxLeaves{&c.Config.MaxPrefixes, &c.Config.PreventTeardown, &c.Config.WarningThresholdPct}
		}
	case niBgpNbrAfiSafiIPv6:
		if afObj.Ipv6Unicast == nil {
			return nil
		}
		if received {
			if c := afObj.Ipv6Unicast.PrefixLimitReceived; c != nil && c.Config != nil {
				return &niBgpNbrPfxLeaves{&c.Config.MaxPrefixes, &c.Config.PreventTeardown, &c.Config.WarningThresholdPct}
			}
		} else if c := afObj.Ipv6Unicast.PrefixLimit; c != nil && c.Config != nil {
			return &niBgpNbrPfxLeaves{&c.Config.MaxPrefixes, &c.Config.PreventTeardown, &c.Config.WarningThresholdPct}
		}
	}
	return nil
}

// niBgpNbrPfxPayloadFields renders the leaves a payload carries, all of them
// or only leaf when one is named.
func niBgpNbrPfxPayloadFields(l *niBgpNbrPfxLeaves, leaf string) map[string]string {
	fields := map[string]string{}
	if l == nil {
		return fields
	}
	if (leaf == "" || leaf == niBgpNbrMaxPrefixesLeaf) && *l.max != nil {
		fields[niBgpNbrMaxPfxFld] = strconv.FormatUint(uint64(**l.max), 10)
	}
	if (leaf == "" || leaf == niBgpNbrPreventTeardownLeaf) && *l.teardown != nil {
		fields[niBgpNbrMaxPfxWarnOnlyFld] = strconv.FormatBool(**l.teardown)
	}
	if (leaf == "" || leaf == niBgpNbrWarnThresholdLeaf) && *l.pct != nil {
		fields[niBgpNbrMaxPfxThreshFld] = strconv.FormatUint(uint64(**l.pct), 10)
	}
	return fields
}

// niBgpNbrPfxIsReceived reports whether a BGP_NEIGHBOR_AF row holds the limit
// of prefix-limit-received rather than of prefix-limit.
func niBgpNbrPfxIsReceived(entry db.Value) bool {
	return strings.EqualFold(strings.TrimSpace(entry.Field[niBgpNbrMaxPfxForceFld]), "true")
}

// niBgpNbrPfxFamilyPayload reads the one prefix-limit container a payload at
// or above the family container carries. Only one limit can be in effect, so
// a payload carrying both is refused, and so is one for the other family.
func niBgpNbrPfxFamilyPayload(afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi, afiSafi string) (map[string]string, bool, error) {
	other := niBgpNbrAfiSafiIPv6
	if afiSafi == niBgpNbrAfiSafiIPv6 {
		other = niBgpNbrAfiSafiIPv4
	}
	for _, received := range []bool{false, true} {
		if len(niBgpNbrPfxPayloadFields(niBgpNbrPfxCfgLeaves(afObj, other, received), "")) > 0 {
			return nil, false, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("BGP neighbor %s-unicast prefix-limit does not belong to afi-safi %s", strings.SplitN(other, "_", 2)[0], afiSafi),
			}
		}
	}
	accepted := niBgpNbrPfxPayloadFields(niBgpNbrPfxCfgLeaves(afObj, afiSafi, false), "")
	received := niBgpNbrPfxPayloadFields(niBgpNbrPfxCfgLeaves(afObj, afiSafi, true), "")
	if len(accepted) > 0 && len(received) > 0 {
		return nil, false, tlerr.InvalidArgsError{
			Format: "BGP neighbor prefix-limit and prefix-limit-received cannot both be configured; only one limit is in effect per address family",
		}
	}
	if len(received) > 0 {
		return received, true, nil
	}
	return accepted, false, nil
}

// niBgpNbrPfxWithMode adds the field that names the container a write is for.
func niBgpNbrPfxWithMode(fields map[string]string, received bool) map[string]string {
	if len(fields) > 0 {
		fields[niBgpNbrMaxPfxForceFld] = strconv.FormatBool(received)
	}
	return fields
}

// niBgpNbrPfxClear lists the prefix-limit fields of existing that a write of
// fields leaves behind and must clear: every one it does not carry when it
// switches the limit to the other container, or when replace is set.
func niBgpNbrPfxClear(existing db.Value, fields map[string]string, received, replace bool) map[string]string {
	clear := map[string]string{}
	switching := len(fields) > 0 && existing.Has(niBgpNbrMaxPfxFld) && niBgpNbrPfxIsReceived(existing) != received
	if !switching && !replace {
		return clear
	}
	for _, fld := range niBgpNbrPfxLimitFlds {
		if _, ok := fields[fld]; !ok && existing.Has(fld) {
			clear[fld] = ""
		}
	}
	return clear
}

// niBgpNbrPfxStageWrite maps a write of one prefix-limit container. A write
// that has to clear fields stages its update beside the delete, the same way
// an apply-policy REPLACE does, as a REPLACE of a container that is not a
// terminal node would otherwise rewrite the whole row.
func niBgpNbrPfxStageWrite(inParams XfmrParams, reqInfo *niBgpNbrReqInfo, fields map[string]string, received, replace bool) (map[string]map[string]db.Value, error) {
	fields = niBgpNbrPfxWithMode(fields, received)
	existing := niBgpCfgDbEntry(inParams, niBgpNbrAfTbl, reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi)
	clear := niBgpNbrPfxClear(existing, fields, received, replace)
	res := ensureNiBgpNbrParentRow(makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, fields), reqInfo.niName, reqInfo.neighborAddr)
	if inParams.oper != REPLACE && len(clear) == 0 {
		return res, nil
	}
	if len(clear) > 0 {
		if err := niBgpStageSubOp(inParams, DELETE, makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, clear)); err != nil {
			return nil, err
		}
	}
	if len(fields) > 0 {
		if err := niBgpStageSubOp(inParams, UPDATE, res); err != nil {
			return nil, err
		}
	}
	return map[string]map[string]db.Value{}, nil
}

// niBgpNbrPfxLimitWrite maps a write at a prefix-limit config container or one
// of its leaves.
func niBgpNbrPfxLimitWrite(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (map[string]map[string]db.Value, error) {
	received := reqInfo.pfxCont == niBgpNbrPfxLimitRcvCont
	afObj := getNiBgpNbrAfObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, false)
	fields := niBgpNbrPfxPayloadFields(niBgpNbrPfxCfgLeaves(afObj, reqInfo.afiSafi, received), reqInfo.targetLeaf)
	if len(fields) == 0 {
		if reqInfo.targetLeaf != "" {
			return nil, tlerr.InvalidArgsError{Format: "BGP neighbor " + reqInfo.pfxCont + " " + reqInfo.targetLeaf + " is missing in request payload"}
		}
		if inParams.oper != REPLACE {
			return nil, tlerr.InvalidArgsError{Format: "BGP neighbor " + reqInfo.pfxCont + " config payload carries none of max-prefixes, prevent-teardown or warning-threshold-pct"}
		}
	}
	return niBgpNbrPfxStageWrite(inParams, reqInfo, fields, received, inParams.oper == REPLACE && reqInfo.targetLeaf == "")
}

// niBgpNbrAfFamilyWrite maps a write at the family container or at one of the
// prefix-limit containers above config.
func niBgpNbrAfFamilyWrite(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (map[string]map[string]db.Value, error) {
	wrappedRoot, err := wrapNiBgpNbrRootAsDevice(inParams.ygRoot, reqInfo)
	if err != nil {
		return nil, err
	}
	afObj := getNiBgpNbrAfObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, false)
	var fields map[string]string
	received := reqInfo.pfxCont == niBgpNbrPfxLimitRcvCont
	if reqInfo.pfxCont == "" {
		if fields, received, err = niBgpNbrPfxFamilyPayload(afObj, reqInfo.afiSafi); err != nil {
			return nil, err
		}
	} else {
		fields = niBgpNbrPfxPayloadFields(niBgpNbrPfxCfgLeaves(afObj, reqInfo.afiSafi, received), "")
	}
	if len(fields) == 0 && inParams.oper != REPLACE {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor " + reqInfo.afiSafi + " payload carries no prefix-limit leaf"}
	}
	return niBgpNbrPfxStageWrite(inParams, reqInfo, fields, received, inParams.oper == REPLACE)
}

// niBgpNbrPfxLimitDelete maps a delete at or below the family container. A
// prefix-limit container that is not the one in effect holds nothing, so a
// delete there leaves the limit of the other in place.
func niBgpNbrPfxLimitDelete(inParams XfmrParams, reqInfo *niBgpNbrReqInfo) (map[string]map[string]db.Value, error) {
	existing := niBgpCfgDbEntry(inParams, niBgpNbrAfTbl, reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi)
	if !existing.IsPopulated() {
		return map[string]map[string]db.Value{}, nil
	}
	if reqInfo.pfxCont != "" && niBgpNbrPfxIsReceived(existing) != (reqInfo.pfxCont == niBgpNbrPfxLimitRcvCont) {
		return map[string]map[string]db.Value{}, nil
	}
	var flds []string
	switch reqInfo.targetLeaf {
	case "", niBgpNbrMaxPrefixesLeaf:
		// The limit is what the other leaves qualify, so they go with it.
		flds = niBgpNbrPfxLimitFlds
	case niBgpNbrPreventTeardownLeaf:
		flds = []string{niBgpNbrMaxPfxWarnOnlyFld}
	case niBgpNbrWarnThresholdLeaf:
		flds = []string{niBgpNbrMaxPfxThreshFld}
	default:
		return nil, tlerr.NotSupportedError{Format: "unsupported leaf under neighbor " + reqInfo.pfxCont + " config", Path: inParams.requestUri}
	}
	clear := map[string]string{}
	for _, fld := range flds {
		if existing.Has(fld) {
			clear[fld] = ""
		}
	}
	if len(clear) == 0 {
		return map[string]map[string]db.Value{}, nil
	}
	return makeNiBgpNbrAfSingleResMap(reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi, clear), nil
}

// populateNiBgpNbrPfxFromEntry fills the prefix-limit config container that
// entry holds the limit of, or only its leaf when one is named. The other
// container reports nothing.
func populateNiBgpNbrPfxFromEntry(afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi, afiSafi string, entry db.Value, received bool, leaf string) error {
	if niBgpNbrPfxIsReceived(entry) != received {
		return nil
	}
	l := niBgpNbrPfxCfgLeaves(afObj, afiSafi, received)
	if l == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor prefix-limit config object in YANG tree"}
	}
	if raw := strings.TrimSpace(entry.Field[niBgpNbrMaxPfxFld]); raw != "" && (leaf == "" || leaf == niBgpNbrMaxPrefixesLeaf) {
		v, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR_AF.%s value %q", niBgpNbrMaxPfxFld, raw)}
		}
		max := uint32(v)
		*l.max = &max
	}
	if raw := strings.TrimSpace(entry.Field[niBgpNbrMaxPfxWarnOnlyFld]); raw != "" && (leaf == "" || leaf == niBgpNbrPreventTeardownLeaf) {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR_AF.%s value %q", niBgpNbrMaxPfxWarnOnlyFld, raw)}
		}
		*l.teardown = &v
	}
	if raw := strings.TrimSpace(entry.Field[niBgpNbrMaxPfxThreshFld]); raw != "" && (leaf == "" || leaf == niBgpNbrWarnThresholdLeaf) {
		v, err := strconv.ParseUint(raw, 10, 8)
		if err != nil {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR_AF.%s value %q", niBgpNbrMaxPfxThreshFld, raw)}
		}
		pct := uint8(v)
		*l.pct = &pct
	}
	return nil
}

// niBgpNbrPfxLimitRead serves a read at or below the family container.
func niBgpNbrPfxLimitRead(inParams XfmrParams, cfgDb *db.DB, reqInfo *niBgpNbrReqInfo) error {
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr, reqInfo.afiSafi}})
	if err != nil {
		return err
	}
	wrappedRoot, err := wrapNiBgpNbrRootAsDevice(inParams.ygRoot, reqInfo)
	if err != nil {
		return err
	}
	afObj := getNiBgpNbrAfObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.neighborAddr, reqInfo.afiSafi, true)
	if afObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor AFI-SAFI object in YANG tree"}
	}
	for _, received := range []bool{false, true} {
		cont := niBgpNbrPfxLimitCont
		if received {
			cont = niBgpNbrPfxLimitRcvCont
		}
		if reqInfo.pfxCont != "" && reqInfo.pfxCont != cont {
			continue
		}
		if err := populateNiBgpNbrPfxFromEntry(afObj, reqInfo.afiSafi, entry, received, reqInfo.targetLeaf); err != nil {
			return err
		}
	}
	return nil
}
