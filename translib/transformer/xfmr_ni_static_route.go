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
	"strconv"
	"strings"

	log "github.com/golang/glog"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niStaticRouteTbl = "STATIC_ROUTE"

	// The two protocol instance names served for identifier=STATIC. The lower
	// case spelling is the one to synthesize when a request carries no name.
	niStaticRouteProtoNameLower = "static"
	niStaticRouteProtoNameUpper = "STATIC"

	niStaticRouteNexthopFld    = "nexthop"
	niStaticRouteIfnameFld     = "ifname"
	niStaticRouteBlackholeFld  = "blackhole"
	niStaticRouteNexthopVrfFld = "nexthop-vrf"
	niStaticRouteDistanceFld   = "distance"
	niStaticRouteAdvertiseFld  = "advertise"
	niStaticRouteTagFld        = "tag"
	// What FRR holds, published by bgpmon, keyed <vrf>|<prefix> exactly as the
	// configured table is keyed, so state decodes the way config does.
	niStaticRouteAppliedTbl = "STATIC_ROUTE_APPLIED"
	niStaticRouteNiListNode = "network-instance"
	niStaticRouteListNode   = "static"

	niStaticRouteTargetProtocolRoot = "static-protocol-root"
	niStaticRouteTargetRoutesRoot   = "static-routes-root"
	niStaticRouteTargetRouteInst    = "static-route-instance"
	niStaticRouteTargetRouteCfg     = "static-route-config"
	niStaticRouteTargetRouteState   = "static-route-state"
	niStaticRouteTargetNextHopsRoot = "static-route-next-hops-root"
	niStaticRouteTargetNextHopInst  = "static-route-next-hop-instance"
	niStaticRouteTargetNextHopCfg   = "static-route-next-hop-config"

	niStaticRoutePrefixLeaf  = "prefix"
	niStaticRouteSetTagLeaf  = "set-tag"
	niStaticRouteIndexLeaf   = "index"
	niStaticRouteNextHopLeaf = "next-hop"

	niStaticRoutePreferenceLeaf = "preference"
	// FRR takes an administrative distance of 1 to 255 on a route line, which
	// is the range the STATIC_ROUTE distance field holds too.
	niStaticRoutePreferenceMax = 255

	// Next-hop kinds as spelled by the OpenConfig next-hop leaf and, for the
	// list key, by the index of the next-hop.
	niStaticRouteDropToken      = "DROP"
	niStaticRouteLocalLinkToken = "LOCAL_LINK"
	// niStaticRouteIndexSep separates the next-hop from the interface in an
	// index such as "10.1.1.1+Ethernet0" or "LOCAL_LINK+Loopback0".
	niStaticRouteIndexSep = "+"
	// niStaticRouteNullIfname is the interface CONFIG_DB carries for a
	// blackhole next-hop.
	niStaticRouteNullIfname = "Null0"

	niStaticRouteZeroIpv4 = "0.0.0.0"
	niStaticRouteZeroIpv6 = "::"

	// niStaticRouteMaxTreeDepth bounds the container recursion done while
	// walking a static route YANG subtree.
	niStaticRouteMaxTreeDepth = 32
)

// The protocol instance name is not part of the CONFIG_DB key, so it is
// captured here and echoed back as sent, and the accepted spellings are
// restricted by validateNiStaticRouteProtoName. The identifier may arrive
// module qualified.
const niStaticRouteUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=(?:[-A-Za-z0-9_.]+:)?STATIC\]\[name=([^\]]+)\]`

var niStaticRouteProtoRootReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `$`)
var niStaticRouteProtoInstanceReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `(?:/.*)?$`)
var niStaticRoutesRootReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes$`)
var niStaticRouteInstReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]$`)
var niStaticRouteKeyLeafReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/prefix$`)
var niStaticRouteCfgReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/config(?:/(prefix|set-tag))?$`)
var niStaticRouteStateReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/state(?:/(prefix|set-tag))?$`)

// A wildcard key is removed by translib before the request reaches here, so
// these match what a `[prefix=*]` request arrives as: the list with no key.
var niStaticRouteWildcardCfgReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static/config(?:/(prefix|set-tag))?$`)
var niStaticRouteWildcardStateReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static/state(?:/(prefix|set-tag))?$`)
var niStaticRouteListReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static$`)
var niStaticRouteNhsRootReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/next-hops(?:/next-hop)?$`)
var niStaticRouteNhInstReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/next-hops/next-hop\[index=([^\]]+)\]$`)
var niStaticRouteNhKeyLeafReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/next-hops/next-hop\[index=([^\]]+)\]/index$`)
var niStaticRouteNhCfgReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/next-hops/next-hop\[index=([^\]]+)\]/config(?:/(index|next-hop|preference))?$`)

type niStaticRouteReqInfo struct {
	niName         string
	protoName      string
	prefix         string
	index          string
	targetLeaf     string
	targetPathKind string
}

func init() {
	XlateFuncBind("YangToDb_ni_static_route_cfg_xfmr", YangToDb_ni_static_route_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_static_route_cfg_xfmr", DbToYang_ni_static_route_cfg_xfmr)
	XlateFuncBind("Subscribe_ni_static_route_cfg_xfmr", Subscribe_ni_static_route_cfg_xfmr)
	XlateFuncBind("DbToYangPath_ni_static_route_path_xfmr", DbToYangPath_ni_static_route_path_xfmr)
}

// isNiStaticRouteReqUri reports whether uri targets the STATIC protocol
// instance or anything below it. The protocol list carries the BGP subtree
// transformer, so that transformer hands such a request over here instead of
// failing it as an unsupported BGP path.
func isNiStaticRouteReqUri(uri string) bool {
	return niStaticRouteProtoInstanceReqUriRe.MatchString(uri)
}

// isNiStaticRouteContainerLevelReqUri reports whether uri targets a node of the
// STATIC protocol instance. Such a request is mapped or populated in a single
// pass by the container dispatch, but translib still invokes the transformers
// annotated on the child nodes with the same requestUri; those invocations must
// stay silent rather than fail the request.
func isNiStaticRouteContainerLevelReqUri(uri string) bool {
	return isNiStaticRouteReqUri(uri)
}

// niStaticRouteUnescapeKey undoes the escaping a client may apply to the path
// separators of a prefix carried in a list key.
var niStaticRouteKeyUnescaper = strings.NewReplacer(`\\`, `\`, `\/`, `/`, `\[`, `[`, `\]`, `]`, `\=`, `=`)

func niStaticRouteUnescapeKey(v string) string {
	return niStaticRouteKeyUnescaper.Replace(strings.TrimSpace(v))
}

// validateNiStaticRoutePrefix enforces normalized CIDR input; the prefix is
// used verbatim as the CONFIG_DB key component and is not normalized here.
func validateNiStaticRoutePrefix(prefix string) error {
	ip, ipNet, err := net.ParseCIDR(prefix)
	if err != nil {
		return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid static route prefix %q", prefix)}
	}
	if !ip.Equal(ipNet.IP) {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("static route prefix %q is not in normalized CIDR form; expected %q", prefix, ipNet.String()),
		}
	}
	return nil
}

// niStaticRouteProtoNames lists the protocol instance names served for
// identifier=STATIC. Every name would otherwise address the same CONFIG_DB
// rows, so the two conventional spellings bound how many aliases one row has.
var niStaticRouteProtoNames = map[string]bool{
	niStaticRouteProtoNameLower: true,
	niStaticRouteProtoNameUpper: true,
}

func validateNiStaticRouteProtoName(name string) error {
	if niStaticRouteProtoNames[name] {
		return nil
	}
	return tlerr.NotSupportedError{
		Format: fmt.Sprintf("unsupported STATIC protocol instance name %q; expected %q or %q",
			name, niStaticRouteProtoNameLower, niStaticRouteProtoNameUpper),
	}
}

func parseNiStaticRouteRequestUri(uri string) (*niStaticRouteReqInfo, error) {
	reqInfo, err := matchNiStaticRouteRequestUri(uri)
	if err != nil {
		return nil, err
	}
	if err := validateNiStaticRouteProtoName(reqInfo.protoName); err != nil {
		return nil, err
	}
	return reqInfo, nil
}

func matchNiStaticRouteRequestUri(uri string) (*niStaticRouteReqInfo, error) {
	if m := niStaticRouteNhCfgReqUriRe.FindStringSubmatch(uri); len(m) >= 5 {
		leaf := ""
		if len(m) >= 6 {
			leaf = m[5]
		}
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			index:          niStaticRouteUnescapeKey(m[4]),
			targetLeaf:     leaf,
			targetPathKind: niStaticRouteTargetNextHopCfg,
		}, nil
	}
	if m := niStaticRouteNhKeyLeafReqUriRe.FindStringSubmatch(uri); len(m) >= 5 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			index:          niStaticRouteUnescapeKey(m[4]),
			targetLeaf:     niStaticRouteIndexLeaf,
			targetPathKind: niStaticRouteTargetNextHopInst,
		}, nil
	}
	if m := niStaticRouteNhInstReqUriRe.FindStringSubmatch(uri); len(m) >= 5 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			index:          niStaticRouteUnescapeKey(m[4]),
			targetPathKind: niStaticRouteTargetNextHopInst,
		}, nil
	}
	if m := niStaticRouteNhsRootReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			targetPathKind: niStaticRouteTargetNextHopsRoot,
		}, nil
	}
	if m := niStaticRouteCfgReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			targetLeaf:     leaf,
			targetPathKind: niStaticRouteTargetRouteCfg,
		}, nil
	}
	if m := niStaticRouteStateReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			targetLeaf:     leaf,
			targetPathKind: niStaticRouteTargetRouteState,
		}, nil
	}
	if m := niStaticRouteWildcardStateReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			targetLeaf:     leaf,
			targetPathKind: niStaticRouteTargetRouteState,
		}, nil
	}
	if m := niStaticRouteWildcardCfgReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			targetLeaf:     leaf,
			targetPathKind: niStaticRouteTargetRouteCfg,
		}, nil
	}
	if m := niStaticRouteKeyLeafReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			targetLeaf:     niStaticRoutePrefixLeaf,
			targetPathKind: niStaticRouteTargetRouteInst,
		}, nil
	}
	if m := niStaticRouteInstReqUriRe.FindStringSubmatch(uri); len(m) >= 4 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			prefix:         niStaticRouteUnescapeKey(m[3]),
			targetPathKind: niStaticRouteTargetRouteInst,
		}, nil
	}
	// A request that names the list but no instance covers every route, and is
	// mapped and populated the same way as one that names the container.
	if m := niStaticRouteListReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			targetPathKind: niStaticRouteTargetRoutesRoot,
		}, nil
	}
	if m := niStaticRoutesRootReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			targetPathKind: niStaticRouteTargetRoutesRoot,
		}, nil
	}
	if m := niStaticRouteProtoRootReqUriRe.FindStringSubmatch(uri); len(m) >= 3 {
		return &niStaticRouteReqInfo{
			niName:         m[1],
			protoName:      m[2],
			targetPathKind: niStaticRouteTargetProtocolRoot,
		}, nil
	}
	// Named after what this handler serves, the protocol instance, rather than
	// the static routes alone: every node of the instance is routed here, so a
	// path that is not one of them arrives here too.
	return nil, tlerr.NotSupportedError{
		Format: "unsupported path under the network-instance STATIC protocol instance: " + uri,
		Path:   uri,
	}
}

// niStaticRouteMappedPayloadLeaves lists every leaf, relative to the
// static-routes container, that a container-level request can carry. List keys
// are included because a payload repeats them alongside the data they identify.
var niStaticRouteMappedPayloadLeaves = map[string]bool{
	"static/prefix":                               true,
	"static/config/prefix":                        true,
	"static/config/set-tag":                       true,
	"static/next-hops/next-hop/index":             true,
	"static/next-hops/next-hop/config/index":      true,
	"static/next-hops/next-hop/config/next-hop":   true,
	"static/next-hops/next-hop/config/preference": true,
}

// STATIC_ROUTE stores one comma separated list per next-hop attribute, and the
// lists are read position by position. Every list the row carries must
// therefore hold one element per next-hop, which is what the padding defaults
// are for.
var niStaticRouteNhFieldOrder = []string{
	niStaticRouteNexthopFld,
	niStaticRouteIfnameFld,
	niStaticRouteBlackholeFld,
	niStaticRouteNexthopVrfFld,
	niStaticRouteDistanceFld,
	niStaticRouteAdvertiseFld,
	niStaticRouteTagFld,
}

// The padding of a next-hop that leaves an attribute unset is the default the
// STATIC_ROUTE schema gives that attribute. nexthop-vrf has no empty spelling,
// so a next-hop that leaks into no other VRF names its own.
var niStaticRouteNhFieldDefault = map[string]string{
	niStaticRouteIfnameFld:     "",
	niStaticRouteBlackholeFld:  "false",
	niStaticRouteNexthopVrfFld: "default",
	niStaticRouteDistanceFld:   "0",
	niStaticRouteAdvertiseFld:  "false",
}

// niStaticRouteIsIpv6Prefix reports the address family of a validated prefix.
func niStaticRouteIsIpv6Prefix(prefix string) bool {
	ip, _, err := net.ParseCIDR(prefix)
	if err != nil {
		return false
	}
	return ip.To4() == nil
}

func niStaticRouteZeroIp(isIpv6 bool) string {
	if isIpv6 {
		return niStaticRouteZeroIpv6
	}
	return niStaticRouteZeroIpv4
}

func niStaticRouteIsZeroIp(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	return parsed != nil && parsed.IsUnspecified()
}

// niStaticRouteDecodeNextHops splits the comma separated attributes of a
// STATIC_ROUTE row into one field map per next-hop.
func niStaticRouteDecodeNextHops(entry db.Value) []map[string]string {
	lists := make(map[string][]string)
	count := 0
	for _, fld := range niStaticRouteNhFieldOrder {
		v, ok := entry.Field[fld]
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		parts := strings.Split(v, ",")
		lists[fld] = parts
		if len(parts) > count {
			count = len(parts)
		}
	}
	entries := make([]map[string]string, count)
	for i := range entries {
		e := make(map[string]string)
		for fld, parts := range lists {
			if i < len(parts) {
				e[fld] = strings.TrimSpace(parts[i])
			}
		}
		entries[i] = e
	}
	return entries
}

// niStaticRoutePreferenceToDb renders a preference onto the distance the row
// holds for one next-hop. FRR takes a distance of 1 to 255 on a route line and
// spells "no distance" by leaving it off, which the row holds as 0. So 0 is not
// a preference to be set but the absence of one, and a DELETE of the leaf is
// what asks for it.
func niStaticRoutePreferenceToDb(preference uint32) (string, error) {
	if preference == 0 || preference > niStaticRoutePreferenceMax {
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf(
			"static route next-hop preference %d is out of range; FRR takes an administrative distance of 1 to %d, and a DELETE of the leaf restores the default",
			preference, niStaticRoutePreferenceMax)}
	}
	return strconv.FormatUint(uint64(preference), 10), nil
}

// niStaticRoutePreferenceOf reports the preference of one stored next-hop. A 0
// is reported as absent rather than as 0: it is how the row spells no
// preference, and FRR then leaves the distance off the route line and applies
// its own default, so 0 is never the distance the next-hop really has.
func niStaticRoutePreferenceOf(e map[string]string) (uint32, bool) {
	v := strings.TrimSpace(e[niStaticRouteDistanceFld])
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil || n == 0 || n > niStaticRoutePreferenceMax {
		return 0, false
	}
	return uint32(n), true
}

// niStaticRoutePreferenceFromPayload reads the preference a next-hop request
// carries, spelled as the distance field holds it.
func niStaticRoutePreferenceFromPayload(s *ygot.GoStruct, reqInfo *niStaticRouteReqInfo) (string, bool, error) {
	nhObj := getNiStaticRouteNextHopObj(s, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, reqInfo.index, false)
	if nhObj == nil || nhObj.Config == nil || nhObj.Config.Preference == nil {
		if reqInfo.targetLeaf == niStaticRoutePreferenceLeaf {
			return "", false, tlerr.InvalidArgsError{Format: "static route next-hop preference is missing in request payload"}
		}
		return "", false, nil
	}
	distance, err := niStaticRoutePreferenceToDb(*nhObj.Config.Preference)
	if err != nil {
		return "", false, err
	}
	return distance, true, nil
}

// niStaticRouteSetTagFields renders a set-tag request into the row fields that
// carry it. The tag describes the route, so it is written into every next-hop
// position: FRR keeps one tag per prefix, and it programs nothing at all when
// the tag list is a different length to the next-hop list.
func niStaticRouteSetTagFields(inParams XfmrParams, s *ygot.GoStruct, reqInfo *niStaticRouteReqInfo) (map[string]string, error) {
	// A subtree transformer is handed the struct that contains the leaf rather
	// than the leaf's value, so the tag is taken from whichever struct carries
	// it: the one passed as the param, or the one built from the request.
	var raw interface{}
	if cfg, ok := inParams.param.(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config); ok && cfg != nil && cfg.SetTag != nil {
		raw = cfg.SetTag
	} else if cfgObj := getNiStaticRouteCfgObj(s, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, false); cfgObj != nil && cfgObj.SetTag != nil {
		raw = cfgObj.SetTag
	}
	if raw == nil {
		if reqInfo.targetLeaf == niStaticRouteSetTagLeaf {
			return nil, tlerr.InvalidArgsError{Format: "static route set-tag is missing in request payload"}
		}
		return nil, nil
	}
	existing := niStaticRouteEntryOrEmpty(inParams.dbs[db.ConfigDB], reqInfo.niName, reqInfo.prefix)
	entries := niStaticRouteDecodeNextHops(existing)
	if err := niStaticRouteApplyTag(entries, reqInfo.prefix, raw); err != nil {
		return nil, err
	}
	return niStaticRouteEncodeNextHops(entries, existing, reqInfo.prefix), nil
}

// niStaticRouteApplyTag puts a route's tag on each of its next-hops. FRR keeps
// one tag per prefix and takes it on every route line, so a route with no
// next-hop has nowhere to carry one, and a tag list of a different length to
// the next-hop list programs none of the route at all.
func niStaticRouteApplyTag(entries []map[string]string, prefix string, raw interface{}) error {
	tag, err := niStaticRouteTagToDb(raw)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return tlerr.InvalidArgsError{
			Format: "static route " + prefix + " has no next-hop to carry a tag; FRR takes the tag on each route line, so a next-hop has to exist first",
		}
	}
	for _, e := range entries {
		e[niStaticRouteTagFld] = tag
	}
	return nil
}

// niStaticRouteSetTagToYang reports the route's tag, when its next-hops agree
// on one. The decimal the row holds is reported as the uint32 member of the
// union, which is the spelling FRR and CONFIG_DB use; a request may also send
// the hex-string member, and that is converted on the way in.
func niStaticRouteSetTagToYang(cfgObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config, entry db.Value) {
	tag, ok := niStaticRouteUniformTag(entry)
	if !ok {
		return
	}
	n, err := strconv.ParseUint(tag, 10, 32)
	if err != nil {
		return
	}
	cfgObj.SetTag = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config_SetTag_Union_Uint32{Uint32: uint32(n)}
}

// niStaticRouteTagOf reports the tag of a route: the one value its next-hops
// agree on, whether that comes from the next-hops being written or from the
// stored row. An empty answer means the route carries no tag.
func niStaticRouteTagOf(entries []map[string]string, existing db.Value) string {
	for _, e := range entries {
		if v := strings.TrimSpace(e[niStaticRouteTagFld]); v != "" {
			return v
		}
	}
	for _, v := range strings.Split(existing.Field[niStaticRouteTagFld], ",") {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// niStaticRouteUniformTag reports the tag every next-hop of a stored route
// carries, and whether they all carry it. A row whose positions disagree is
// not describable by the route-level leaf: FRR would have kept whichever value
// it applied last, so nothing is reported rather than one of them.
func niStaticRouteUniformTag(entry db.Value) (string, bool) {
	raw := strings.TrimSpace(entry.Field[niStaticRouteTagFld])
	if raw == "" {
		return "", false
	}
	first := ""
	for _, v := range strings.Split(raw, ",") {
		v = strings.TrimSpace(v)
		if v == "" || v == "0" {
			return "", false
		}
		if first == "" {
			first = v
		} else if v != first {
			return "", false
		}
	}
	return first, first != ""
}

// niStaticRouteTagToDb renders what a request carries for set-tag into the
// decimal FRR takes. The leaf is a union of uint32 and hex-string, so the hex
// spelling is converted rather than refused, and a GET reports the decimal.
func niStaticRouteTagToDb(v interface{}) (string, error) {
	unsupported := func() error {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("static route set-tag carries an unsupported value of type %T", v),
		}
	}
	var n uint64
	switch val := v.(type) {
	case nil:
		return "", unsupported()
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config_SetTag_Union_Uint32:
		if val == nil {
			return "", unsupported()
		}
		n = uint64(val.Uint32)
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config_SetTag_Union_String:
		if val == nil {
			return "", unsupported()
		}
		parsed, err := strconv.ParseUint(strings.ReplaceAll(strings.TrimSpace(val.String), ":", ""), 16, 32)
		if err != nil {
			return "", tlerr.InvalidArgsError{Format: "static route set-tag is not a valid hex-string: " + val.String}
		}
		n = parsed
	case uint32:
		n = uint64(val)
	case float64:
		if val < 0 || val != float64(uint64(val)) {
			return "", unsupported()
		}
		n = uint64(val)
	case string:
		s := strings.TrimSpace(val)
		parsed, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			hex, hexErr := strconv.ParseUint(strings.ReplaceAll(s, ":", ""), 16, 32)
			if hexErr != nil {
				return "", unsupported()
			}
			parsed = hex
		}
		n = parsed
	default:
		return "", unsupported()
	}
	if n == 0 || n > 4294967295 {
		return "", tlerr.InvalidArgsError{
			Format: "static route set-tag accepts 1 through 4294967295, since that is the range FRR takes",
		}
	}
	return strconv.FormatUint(n, 10), nil
}

// niStaticRouteEncodeNextHops renders the next-hops back into the comma
// separated row attributes, keeping every list aligned. A field that neither
// the next-hops nor the stored row carry is left out rather than introduced,
// but one the stored row carries is always rewritten so that no stale element
// is left behind at the position of a next-hop that changed.
func niStaticRouteEncodeNextHops(entries []map[string]string, existing db.Value, prefix string) map[string]string {
	fields := make(map[string]string)
	for _, fld := range niStaticRouteNhFieldOrder {
		carried := false
		for _, e := range entries {
			if strings.TrimSpace(e[fld]) != "" {
				carried = true
				break
			}
		}
		if !carried && strings.TrimSpace(existing.Field[fld]) == "" {
			continue
		}
		padding := niStaticRouteNhFieldDefault[fld]
		if fld == niStaticRouteTagFld {
			// A tag describes the route, and FRR programs nothing at all when
			// the tag list is shorter than the next-hop list, so a next-hop
			// that arrives without one inherits the route's.
			padding = niStaticRouteTagOf(entries, existing)
		}
		if fld == niStaticRouteNexthopFld {
			// An interface only next-hop is stored as the unspecified address
			// of the address family of the prefix.
			padding = niStaticRouteZeroIp(niStaticRouteIsIpv6Prefix(prefix))
		}
		vals := make([]string, len(entries))
		for i, e := range entries {
			v := strings.TrimSpace(e[fld])
			if v == "" {
				v = padding
			}
			vals[i] = v
		}
		fields[fld] = strings.Join(vals, ",")
	}
	return fields
}

// niStaticRouteNhIndex derives the OpenConfig next-hop list key of a stored
// next-hop: the IP, DROP for a blackhole, or LOCAL_LINK for an interface only
// next-hop, each suffixed with the interface when the next-hop names one.
func niStaticRouteNhIndex(e map[string]string) string {
	nexthop := strings.TrimSpace(e[niStaticRouteNexthopFld])
	ifname := strings.TrimSpace(e[niStaticRouteIfnameFld])
	if strings.EqualFold(strings.TrimSpace(e[niStaticRouteBlackholeFld]), "true") || ifname == niStaticRouteNullIfname {
		return niStaticRouteDropToken
	}
	if nexthop == "" || niStaticRouteIsZeroIp(nexthop) {
		if ifname == "" {
			return ""
		}
		return niStaticRouteLocalLinkToken + niStaticRouteIndexSep + ifname
	}
	if ifname == "" {
		return nexthop
	}
	return nexthop + niStaticRouteIndexSep + ifname
}

// niStaticRouteNhFromIndex builds the CONFIG_DB attributes of one next-hop from
// its list key. The key carries the whole next-hop, so it is also what a
// request that names only the key maps to.
func niStaticRouteNhFromIndex(index, prefix string) (map[string]string, error) {
	isIpv6 := niStaticRouteIsIpv6Prefix(prefix)
	idx := strings.TrimSpace(index)
	if idx == "" {
		return nil, tlerr.InvalidArgsError{Format: "static route next-hop index is empty"}
	}
	if idx == niStaticRouteDropToken {
		return map[string]string{
			niStaticRouteNexthopFld:   niStaticRouteZeroIp(isIpv6),
			niStaticRouteIfnameFld:    niStaticRouteNullIfname,
			niStaticRouteBlackholeFld: "true",
		}, nil
	}

	nhPart := idx
	ifname := ""
	if sep := strings.Index(idx, niStaticRouteIndexSep); sep >= 0 {
		nhPart = strings.TrimSpace(idx[:sep])
		ifname = strings.TrimSpace(idx[sep+len(niStaticRouteIndexSep):])
		if ifname == "" {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("static route next-hop index %q names no interface", index)}
		}
	}
	if nhPart == niStaticRouteLocalLinkToken {
		if ifname == "" {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("static route next-hop index %q must be %s%s<interface>", index, niStaticRouteLocalLinkToken, niStaticRouteIndexSep),
			}
		}
		return map[string]string{
			niStaticRouteNexthopFld: niStaticRouteZeroIp(isIpv6),
			niStaticRouteIfnameFld:  ifname,
		}, nil
	}
	if err := niStaticRouteValidateNhIp(nhPart, isIpv6); err != nil {
		return nil, err
	}
	return map[string]string{
		niStaticRouteNexthopFld: nhPart,
		niStaticRouteIfnameFld:  ifname,
	}, nil
}

func niStaticRouteValidateNhIp(value string, prefixIsIpv6 bool) error {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil {
		return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid static route next-hop address %q", value)}
	}
	if (ip.To4() == nil) != prefixIsIpv6 {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("static route next-hop address %q does not match the address family of the prefix", value),
		}
	}
	return nil
}

// niStaticRouteNextHopValue renders the OpenConfig next-hop leaf of a stored
// next-hop, either an IP or one of the locally defined next-hop identities.
func niStaticRouteNextHopValue(e map[string]string) string {
	switch idx := niStaticRouteNhIndex(e); {
	case idx == "":
		return ""
	case idx == niStaticRouteDropToken:
		return niStaticRouteDropToken
	case strings.HasPrefix(idx, niStaticRouteLocalLinkToken+niStaticRouteIndexSep):
		return niStaticRouteLocalLinkToken
	default:
		return strings.TrimSpace(e[niStaticRouteNexthopFld])
	}
}

// niStaticRouteCheckNextHopValue rejects a next-hop leaf that contradicts the
// next-hop the list key already identifies.
func niStaticRouteCheckNextHopValue(nextHop, index string) error {
	nextHop = strings.TrimSpace(nextHop)
	if nextHop == "" {
		return nil
	}
	if i := strings.LastIndex(nextHop, ":"); i >= 0 && !strings.Contains(nextHop, "::") && net.ParseIP(nextHop) == nil {
		// Strip a module qualified identity such as
		// openconfig-local-routing:DROP.
		nextHop = nextHop[i+1:]
	}
	want := ""
	switch {
	case nextHop == niStaticRouteDropToken:
		want = niStaticRouteDropToken
	case nextHop == niStaticRouteLocalLinkToken:
		want = niStaticRouteLocalLinkToken
	default:
		want = nextHop
	}

	idx := strings.TrimSpace(index)
	idxNh := idx
	if sep := strings.Index(idx, niStaticRouteIndexSep); sep >= 0 {
		idxNh = idx[:sep]
	}
	if idxNh == want {
		return nil
	}
	return tlerr.InvalidArgsError{
		Format: fmt.Sprintf("static route next-hop %q in payload does not match next-hop index %q", nextHop, index),
	}
}

// niStaticRouteUpsertNextHop merges one next-hop into the next-hops already
// stored for the route, matched by list key.
func niStaticRouteUpsertNextHop(entries []map[string]string, index string, nh map[string]string) []map[string]string {
	for i, e := range entries {
		if niStaticRouteNhIndex(e) != index {
			continue
		}
		merged := make(map[string]string, len(e))
		for k, v := range e {
			merged[k] = v
		}
		for k, v := range nh {
			merged[k] = v
		}
		if strings.TrimSpace(nh[niStaticRouteBlackholeFld]) == "" {
			delete(merged, niStaticRouteBlackholeFld)
		}
		entries[i] = merged
		return entries
	}
	return append(entries, nh)
}

func makeNiStaticRouteResMap(niName, prefix string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niStaticRouteTbl: {
			niName + "|" + prefix: db.Value{Field: fields},
		},
	}
}

func buildNiStaticRouteEmptySubtree(obj ygot.GoStruct) {
	v := reflect.ValueOf(obj)
	if !v.IsValid() || v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	buildNiStaticRouteEmptyContainers(v.Elem(), 0)
}

func buildNiStaticRouteEmptyContainers(v reflect.Value, depth int) {
	if depth >= niStaticRouteMaxTreeDepth || v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Ptr || f.Type().Elem().Kind() != reflect.Struct || !f.CanSet() {
			continue
		}
		if f.IsNil() {
			f.Set(reflect.New(f.Type().Elem()))
		}
		buildNiStaticRouteEmptyContainers(f.Elem(), depth+1)
	}
}

// wrapNiStaticRouteRootAsDevice normalises the bind root to a Device tree. The
// request binder anchors the root at the requested node, so a static route
// request can arrive rooted at the protocol instance, the static-routes
// container or one static list entry.
func wrapNiStaticRouteRootAsDevice(s *ygot.GoStruct, reqInfo *niStaticRouteReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for static route mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}

	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	if devObj.NetworkInstances == nil {
		devObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}
	niObj, err := devObj.NetworkInstances.NewNetworkInstance(reqInfo.niName)
	if err != nil || niObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for static route mapping"}
	}
	ygot.BuildEmptyTree(niObj)
	if niObj.Protocols == nil {
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC, reqInfo.protoName)
	if err != nil || pObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create protocol context for static route mapping"}
	}
	buildNiStaticRouteEmptySubtree(pObj)

	// Reuse the original subtree pointers so that DbToYang population updates
	// the caller owned objects regardless of the anchor path.
	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol:
		pObj = root
		niObj.Protocols.Protocol[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
			Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
			Name:       reqInfo.protoName,
		}] = pObj
		buildNiStaticRouteEmptySubtree(pObj)
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes:
		pObj.StaticRoutes = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static:
		if reqInfo.prefix == "" {
			return nil, tlerr.InvalidArgsError{Format: "static route prefix missing in request path"}
		}
		if pObj.StaticRoutes == nil {
			pObj.StaticRoutes = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes{}
		}
		if pObj.StaticRoutes.Static == nil {
			pObj.StaticRoutes.Static = make(map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static)
		}
		pObj.StaticRoutes.Static[reqInfo.prefix] = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops:
		rtObj := niStaticRouteEnsureRouteInProtocol(pObj, reqInfo.prefix)
		if rtObj == nil {
			return nil, tlerr.InvalidArgsError{Format: "unable to create static route context for static route mapping"}
		}
		rtObj.NextHops = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop:
		if reqInfo.index == "" {
			return nil, tlerr.InvalidArgsError{Format: "static route next-hop index missing in request path"}
		}
		rtObj := niStaticRouteEnsureRouteInProtocol(pObj, reqInfo.prefix)
		if rtObj == nil {
			return nil, tlerr.InvalidArgsError{Format: "unable to create static route context for static route mapping"}
		}
		if rtObj.NextHops == nil {
			rtObj.NextHops = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops{}
		}
		if rtObj.NextHops.NextHop == nil {
			rtObj.NextHops.NextHop = make(map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop)
		}
		rtObj.NextHops.NextHop[reqInfo.index] = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config:
		if reqInfo.index == "" {
			return nil, tlerr.InvalidArgsError{Format: "static route next-hop index missing in request path"}
		}
		rtObj := niStaticRouteEnsureRouteInProtocol(pObj, reqInfo.prefix)
		if rtObj == nil {
			return nil, tlerr.InvalidArgsError{Format: "unable to create static route context for static route mapping"}
		}
		if rtObj.NextHops == nil {
			rtObj.NextHops = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops{}
		}
		nhObj, err := rtObj.NextHops.NewNextHop(reqInfo.index)
		if err != nil || nhObj == nil {
			return nil, tlerr.InvalidArgsError{Format: "unable to create static route next-hop context for static route mapping"}
		}
		nhObj.Config = root
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for static route mapping"}
	}

	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}

func niStaticRouteEnsureRouteInProtocol(pObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol, prefix string) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static {
	if pObj == nil || prefix == "" {
		return nil
	}
	if pObj.StaticRoutes == nil {
		pObj.StaticRoutes = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes{}
	}
	if rtObj, ok := pObj.StaticRoutes.Static[prefix]; ok && rtObj != nil {
		return rtObj
	}
	rtObj, err := pObj.StaticRoutes.NewStatic(prefix)
	if err != nil {
		return nil
	}
	return rtObj
}

func getNiStaticRoutesObj(s *ygot.GoStruct, niName, protoName string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes {
	if s == nil || *s == nil {
		return nil
	}
	if srObj, ok := (*s).(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes); ok && srObj != nil {
		return srObj
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
	niObj, ok := deviceObj.NetworkInstances.NetworkInstance[niName]
	if !ok || niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(niName)
		if err != nil || niObj == nil {
			return nil
		}
	}
	if niObj.Protocols == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{}
	}
	if niObj.Protocols.Protocol == nil {
		if !createIfMissing {
			return nil
		}
		niObj.Protocols.Protocol = make(map[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol)
	}
	wantKey := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
		Name:       protoName,
	}
	pObj, ok := niObj.Protocols.Protocol[wantKey]
	if !ok || pObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pObj, err = niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC, protoName)
		if err != nil || pObj == nil {
			return nil
		}
	}
	if pObj.StaticRoutes == nil {
		if !createIfMissing {
			return nil
		}
		pObj.StaticRoutes = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes{}
	}
	return pObj.StaticRoutes
}

func getNiStaticRouteObj(s *ygot.GoStruct, niName, protoName, prefix string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static {
	if s != nil && *s != nil {
		if rtObj, ok := (*s).(*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static); ok && rtObj != nil {
			if createIfMissing {
				buildNiStaticRouteEmptySubtree(rtObj)
			}
			return rtObj
		}
	}
	srObj := getNiStaticRoutesObj(s, niName, protoName, createIfMissing)
	if srObj == nil {
		return nil
	}
	rtObj, ok := srObj.Static[prefix]
	if !ok || rtObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		rtObj, err = srObj.NewStatic(prefix)
		if err != nil || rtObj == nil {
			return nil
		}
	}
	if createIfMissing {
		buildNiStaticRouteEmptySubtree(rtObj)
	}
	return rtObj
}

func getNiStaticRouteCfgObj(s *ygot.GoStruct, niName, protoName, prefix string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config {
	rtObj := getNiStaticRouteObj(s, niName, protoName, prefix, createIfMissing)
	if rtObj == nil {
		return nil
	}
	if rtObj.Config == nil {
		if !createIfMissing {
			return nil
		}
		rtObj.Config = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_Config{}
	}
	return rtObj.Config
}

// getNiStaticRouteStateObj returns a static route's state container. It has no
// datastore of its own: it reports the same STATIC_ROUTE row config writes.
func getNiStaticRouteStateObj(s *ygot.GoStruct, niName, protoName, prefix string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_State {
	rtObj := getNiStaticRouteObj(s, niName, protoName, prefix, createIfMissing)
	if rtObj == nil {
		return nil
	}
	if rtObj.State == nil {
		if !createIfMissing {
			return nil
		}
		rtObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_State{}
	}
	return rtObj.State
}

// niStaticRouteSetTagToYangState fills the tag on the state container. Config
// and State are distinct generated types carrying the same leaf, so the decode
// is shared and only the assignment differs.
func niStaticRouteSetTagToYangState(stateObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_State, entry db.Value) {
	tag, ok := niStaticRouteUniformTag(entry)
	if !ok {
		return
	}
	n, err := strconv.ParseUint(tag, 10, 32)
	if err != nil {
		return
	}
	stateObj.SetTag = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_State_SetTag_Union_Uint32{Uint32: uint32(n)}
}

func getNiStaticRouteNextHopObj(s *ygot.GoStruct, niName, protoName, prefix, index string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop {
	rtObj := getNiStaticRouteObj(s, niName, protoName, prefix, createIfMissing)
	if rtObj == nil {
		return nil
	}
	if rtObj.NextHops == nil {
		if !createIfMissing {
			return nil
		}
		rtObj.NextHops = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops{}
	}
	nhObj, ok := rtObj.NextHops.NextHop[index]
	if !ok || nhObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		nhObj, err = rtObj.NextHops.NewNextHop(index)
		if err != nil || nhObj == nil {
			return nil
		}
	}
	if createIfMissing && nhObj.Config == nil {
		nhObj.Config = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config{}
	}
	return nhObj
}

// niStaticRouteNextHopFromUnion renders the OpenConfig next-hop union of a
// payload as the string spelling used by the list key.
func niStaticRouteNextHopFromUnion(v interface{}) (string, bool) {
	switch val := v.(type) {
	case nil:
		return "", false
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config_NextHop_Union_String:
		if val == nil {
			return "", false
		}
		s := strings.TrimSpace(val.String)
		return s, s != ""
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config_NextHop_Union_E_OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP:
		if val == nil {
			return "", false
		}
		return niStaticRouteLocalDefinedNextHopName(val.E_OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP)
	case ocbinds.E_OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP:
		return niStaticRouteLocalDefinedNextHopName(val)
	case string:
		s := strings.TrimSpace(val)
		return s, s != ""
	case *string:
		if val == nil {
			return "", false
		}
		s := strings.TrimSpace(*val)
		return s, s != ""
	}
	return "", false
}

func niStaticRouteLocalDefinedNextHopName(v ocbinds.E_OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP) (string, bool) {
	switch v {
	case ocbinds.OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP_DROP:
		return niStaticRouteDropToken, true
	case ocbinds.OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP_LOCAL_LINK:
		return niStaticRouteLocalLinkToken, true
	}
	return "", false
}

// niStaticRouteNextHopUnionValue converts a stored next-hop into the union the
// OpenConfig next-hop leaf holds.
func niStaticRouteNextHopUnionValue(cfgObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config, value string) error {
	if cfgObj == nil || value == "" {
		return nil
	}
	var err error
	switch value {
	case niStaticRouteDropToken:
		cfgObj.NextHop, err = cfgObj.To_OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config_NextHop_Union(
			ocbinds.OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP_DROP)
	case niStaticRouteLocalLinkToken:
		cfgObj.NextHop, err = cfgObj.To_OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config_NextHop_Union(
			ocbinds.OpenconfigLocalRouting_LOCAL_DEFINED_NEXT_HOP_LOCAL_LINK)
	default:
		cfgObj.NextHop, err = cfgObj.To_OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config_NextHop_Union(value)
	}
	return err
}

// niStaticRouteNextHopFromPayload reads the next-hop leaf of a request that
// targets the next-hop config container or that leaf.
func niStaticRouteNextHopFromPayload(inParams XfmrParams, ygRoot *ygot.GoStruct, reqInfo *niStaticRouteReqInfo) string {
	if nh, ok := niStaticRouteNextHopFromUnion(inParams.param); ok {
		return nh
	}
	if rv := reflect.ValueOf(inParams.param); rv.IsValid() && rv.Kind() == reflect.Ptr && !rv.IsNil() {
		if elem := rv.Elem(); elem.Kind() == reflect.Struct {
			if fv := elem.FieldByName("NextHop"); fv.IsValid() && fv.CanInterface() {
				if nh, ok := niStaticRouteNextHopFromUnion(fv.Interface()); ok {
					return nh
				}
			}
		}
	}
	nhObj := getNiStaticRouteNextHopObj(ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, reqInfo.index, false)
	if nhObj == nil || nhObj.Config == nil {
		return ""
	}
	if nh, ok := niStaticRouteNextHopFromUnion(nhObj.Config.NextHop); ok {
		return nh
	}
	return ""
}

// findNiStaticRouteUnmappedPayloadLeaves returns the payload leaves under the
// static-routes container that carry a value but have no CONFIG_DB mapping, so
// that a container-level request is rejected as a whole instead of applying
// part of it and dropping the rest silently.
func findNiStaticRouteUnmappedPayloadLeaves(s *ygot.GoStruct, niName, protoName string) []string {
	srObj := getNiStaticRoutesObj(s, niName, protoName, false)
	if srObj == nil {
		return nil
	}
	var unmapped []string
	collectOcUnmappedPayloadLeaves(reflect.ValueOf(srObj), niStaticRouteMappedPayloadLeaves, "", "static-routes", 0, &unmapped)
	sort.Strings(unmapped)
	return unmapped
}

func parseNiStaticRoutePrefixValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		return s, s != ""
	case *string:
		if val != nil {
			s := strings.TrimSpace(*val)
			return s, s != ""
		}
	}
	return "", false
}

func parseNiStaticRoutePrefixFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if p, ok := parseNiStaticRoutePrefixValue(param); ok {
		return p, true
	}

	rv := reflect.ValueOf(param)
	for rv.IsValid() && (rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr) {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return "", false
	}
	if rv.Kind() == reflect.Struct {
		fv := rv.FieldByName("Prefix")
		if fv.IsValid() && fv.CanInterface() {
			if p, ok := parseNiStaticRoutePrefixValue(fv.Interface()); ok {
				return p, true
			}
		}
	}
	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == niStaticRoutePrefixLeaf || strings.HasSuffix(k, ":"+niStaticRoutePrefixLeaf) {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if p, ok := parseNiStaticRoutePrefixValue(mv.Interface()); ok {
						return p, true
					}
				}
			}
		}
	}
	return "", false
}

// niStaticRouteCheckPayloadPrefix rejects a payload whose prefix contradicts
// the prefix in the request path; the CONFIG_DB key comes from the path.
func niStaticRouteCheckPayloadPrefix(inParams XfmrParams, reqInfo *niStaticRouteReqInfo) error {
	payloadPrefix, ok := parseNiStaticRoutePrefixFromParam(inParams.param)
	if !ok {
		cfgObj := getNiStaticRouteCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, false)
		if cfgObj == nil || cfgObj.Prefix == nil {
			return nil
		}
		payloadPrefix = strings.TrimSpace(*cfgObj.Prefix)
	}
	if payloadPrefix == "" || payloadPrefix == reqInfo.prefix {
		return nil
	}
	return tlerr.InvalidArgsError{
		Format: fmt.Sprintf("static route prefix %q in payload does not match prefix %q in request path", payloadPrefix, reqInfo.prefix),
	}
}

// buildNiStaticRouteHierResMap maps a whole static-routes payload, letting one
// container-level request carry several static routes and next-hops. Next-hops
// are merged into the ones already stored, because they share the comma
// separated attributes of a single CONFIG_DB row.
func buildNiStaticRouteHierResMap(cfgDb *db.DB, s *ygot.GoStruct, niName, protoName string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	srObj := getNiStaticRoutesObj(s, niName, protoName, false)
	if srObj == nil {
		return res, nil
	}
	for prefix, rtObj := range srObj.Static {
		if rtObj == nil {
			continue
		}
		if err := validateNiStaticRoutePrefix(prefix); err != nil {
			return res, err
		}
		if rtObj.Config != nil && rtObj.Config.Prefix != nil {
			if cfgPrefix := strings.TrimSpace(*rtObj.Config.Prefix); cfgPrefix != "" && cfgPrefix != prefix {
				return res, tlerr.InvalidArgsError{
					Format: fmt.Sprintf("static route prefix %q in payload does not match list key %q", cfgPrefix, prefix),
				}
			}
		}

		existing := niStaticRouteEntryOrEmpty(cfgDb, niName, prefix)
		entries := niStaticRouteDecodeNextHops(existing)
		changed := false
		if rtObj.NextHops != nil {
			// The next-hops of a route share one row, so a payload carrying
			// several of them must map to the same row whatever order the
			// list entries are held in.
			for _, index := range sortedNiStaticRouteNhKeys(rtObj.NextHops.NextHop) {
				nhObj := rtObj.NextHops.NextHop[index]
				if nhObj == nil {
					continue
				}
				if nhObj.Config != nil && nhObj.Config.Index != nil {
					if cfgIndex := strings.TrimSpace(*nhObj.Config.Index); cfgIndex != "" && cfgIndex != index {
						return res, tlerr.InvalidArgsError{
							Format: fmt.Sprintf("static route next-hop index %q in payload does not match list key %q", cfgIndex, index),
						}
					}
				}
				if nhObj.Config != nil {
					if nh, ok := niStaticRouteNextHopFromUnion(nhObj.Config.NextHop); ok {
						if err := niStaticRouteCheckNextHopValue(nh, index); err != nil {
							return res, err
						}
					}
				}
				nh, err := niStaticRouteNhFromIndex(index, prefix)
				if err != nil {
					return res, err
				}
				if nhObj.Config != nil && nhObj.Config.Preference != nil {
					distance, prefErr := niStaticRoutePreferenceToDb(*nhObj.Config.Preference)
					if prefErr != nil {
						return res, prefErr
					}
					nh[niStaticRouteDistanceFld] = distance
				}
				entries = niStaticRouteUpsertNextHop(entries, index, nh)
				changed = true
			}
		}

		// The tag arrives in the same payload as the next-hops that have to
		// carry it, so it is applied once those have been merged in. Without
		// this the request was accepted and the tag quietly dropped, since
		// this arm sets invokeCRUSubtreeOnce and nothing else revisits the
		// leaf.
		if rtObj.Config != nil && rtObj.Config.SetTag != nil {
			if err := niStaticRouteApplyTag(entries, prefix, rtObj.Config.SetTag); err != nil {
				return res, err
			}
			changed = true
		}

		fields := map[string]string{}
		if changed {
			fields = niStaticRouteEncodeNextHops(entries, existing, prefix)
		}
		if _, ok := res[niStaticRouteTbl]; !ok {
			res[niStaticRouteTbl] = make(map[string]db.Value)
		}
		res[niStaticRouteTbl][niName+"|"+prefix] = db.Value{Field: fields}
	}
	return res, nil
}

func sortedNiStaticRouteNhKeys(nhMap map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop) []string {
	keys := make([]string, 0, len(nhMap))
	for k := range nhMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func niStaticRouteEntryOrEmpty(cfgDb *db.DB, niName, prefix string) db.Value {
	if cfgDb != nil {
		if entry, err := niStaticRouteEntryFromDb(cfgDb, niName, prefix); err == nil {
			return entry
		}
	}
	return db.Value{Field: map[string]string{}}
}

// niStaticRoutePrefixesFromDb lists the prefixes configured for one VRF. A key
// without a VRF component belongs to the default VRF.
// niStaticRouteDeleteAllForNi names every static route row of the instance.
//
// The rows are named from the keys the database holds rather than rebuilt from
// the prefixes, because a route of the default instance may be keyed on the
// prefix alone, and a key rebuilt as <ni>|<prefix> would then name no row.
func niStaticRouteDeleteAllForNi(inParams XfmrParams, niName string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance static route mapping"}
	}

	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niStaticRouteTbl})
	if err != nil {
		// An instance holding no routes is already in the state the caller
		// asked for, so having nothing to delete is success.
		return res, nil
	}
	for _, k := range keys {
		switch len(k.Comp) {
		case 1:
			if niName != "default" {
				continue
			}
		case 2:
			if k.Comp[0] != niName {
				continue
			}
		default:
			continue
		}
		if _, ok := res[niStaticRouteTbl]; !ok {
			res[niStaticRouteTbl] = make(map[string]db.Value)
		}
		res[niStaticRouteTbl][strings.Join(k.Comp, "|")] = db.Value{Field: map[string]string{}}
	}
	return res, nil
}

func niStaticRoutePrefixesFromDb(cfgDb *db.DB, niName string) []string {
	var prefixes []string
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niStaticRouteTbl})
	if err != nil {
		return prefixes
	}
	for _, k := range keys {
		switch len(k.Comp) {
		case 1:
			if niName == "default" {
				prefixes = append(prefixes, k.Comp[0])
			}
		case 2:
			if k.Comp[0] == niName {
				prefixes = append(prefixes, k.Comp[1])
			}
		}
	}
	sort.Strings(prefixes)
	return prefixes
}

// niStaticRouteAppliedEntry reads what FRR holds for one route. There is no
// key without the VRF here, unlike the configured table: the producer reads
// FRR's own vrf-keyed dump, so every row names its instance.
func niStaticRouteAppliedEntry(inParams XfmrParams, niName, prefix string) (db.Value, bool) {
	stDb := inParams.dbs[db.StateDB]
	if stDb == nil {
		return db.Value{}, false
	}
	entry, err := stDb.GetEntry(&db.TableSpec{Name: niStaticRouteAppliedTbl},
		db.Key{Comp: []string{niName, prefix}})
	// A missing row is not always reported as an error: the answer can be a
	// value holding no fields. Reading that as a route FRR holds would make
	// state report a route FRR never accepted.
	if err != nil || len(entry.Field) == 0 {
		return db.Value{}, false
	}
	return entry, true
}

// niStaticRouteSchemaPathUpto truncates a schema path just after the named
// node, and reports whether the path names it at all. A yang key is looked up
// under the path of the list that holds it, so a path transformer has to
// answer with a prefix of the path it was given.
func niStaticRouteSchemaPathUpto(schemaPath string, node string) (string, bool) {
	var b strings.Builder
	for _, elem := range strings.Split(strings.TrimPrefix(schemaPath, "/"), "/") {
		b.WriteString("/")
		b.WriteString(elem)
		name := elem
		if i := strings.Index(name, ":"); i >= 0 {
			name = name[i+1:]
		}
		if name == node {
			return b.String(), true
		}
	}
	return "", false
}

var niStaticRouteSubscReqUriRe = regexp.MustCompile(niStaticRouteUriPfx +
	`/static-routes(?:/static(?:\[prefix=([^\]]+)\])?(?:/(.*))?)?$`)

// Subscribe_ni_static_route_cfg_xfmr maps the applied table behind the static
// routes, which is what makes the path DB backed. A path that is not gets
// refused twice over: on-change and wildcard support are both cleared for it,
// and a wildcard key is rejected outright.
var Subscribe_ni_static_route_cfg_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	// The anchor is uri: the subscribe path trims a trailing leaf off it, so a
	// request on state/prefix arrives anchored on state, which is the node the
	// notification is formed against. requestURI names the leaf the caller
	// asked for and would answer a different question. The CRUD parent-table
	// check arrives with uri unset.
	anchor := inParams.uri
	if anchor == "" {
		anchor = inParams.requestURI
	}

	m := niStaticRouteSubscReqUriRe.FindStringSubmatch(anchor)
	if m == nil {
		result.isVirtualTbl = true
		return result, nil
	}

	niName, prefix, below := m[1], m[3], m[4]
	// A caller that hands over the whole path rather than the trimmed one is
	// asking about the same node, so it is answered the same way.
	if below == "state/"+niStaticRoutePrefixLeaf || below == "state/"+niStaticRouteSetTagLeaf {
		below = "state"
	}
	keyPatt := niName + "|*"
	if prefix != "" && prefix != "*" {
		keyPatt = niName + "|" + prefix
	}

	// Only a leaf directly below the anchored node can be named, because the
	// framework forms the path by appending the name to the anchor. prefix
	// needs no mapping: it is the list key, which a row arriving or going away
	// carries on its own.
	fields := map[string]string{}
	if below == "state" {
		fields[niStaticRouteTagFld] = niStaticRouteSetTagLeaf
	}

	result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {
		niStaticRouteAppliedTbl: {keyPatt: fields},
	}}
	result.isVirtualTbl = false

	// On change where a row arriving or going away is the whole story: the
	// route list, a keyed route, and its state container. Deeper nodes describe
	// next-hops, which this table does not decompose, so they keep sampling
	// rather than being accepted and then reporting nothing.
	if below == "" || below == "state" {
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		return result, nil
	}

	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}
	return result, nil
}

// DbToYangPath_ni_static_route_path_xfmr names the instance and the route an
// applied row belongs to. A subscription needs this whenever translib has to
// supply a key rather than read one from the request: with a wildcard or an
// omitted key it holds a changed row and no way to say which route it is for.
var DbToYangPath_ni_static_route_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != niStaticRouteAppliedTbl {
		return fmt.Errorf("ni_static_route_path_xfmr: notification from table %q, expected %q: %w",
			params.tblName, niStaticRouteAppliedTbl, apis.ErrUnrelatedEntry)
	}
	if len(params.tblKeyComp) != 2 || params.tblKeyComp[0] == "" || params.tblKeyComp[1] == "" {
		return fmt.Errorf("ni_static_route_path_xfmr: %s key %v does not name an instance and a prefix",
			niStaticRouteAppliedTbl, params.tblKeyComp)
	}
	niName, prefix := params.tblKeyComp[0], params.tblKeyComp[1]

	niPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niStaticRouteNiListNode)
	if !ok {
		// Nothing of this row is on the path, so there is no key to answer
		// with. That is not a fault, the same way it is not on the AFT side.
		return nil
	}
	params.ygPathKeys[niPath+"/name"] = niName

	if routePath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niStaticRouteListNode); ok {
		params.ygPathKeys[routePath+"/"+niStaticRoutePrefixLeaf] = prefix
	}
	return nil
}

func niStaticRouteEntryFromDb(cfgDb *db.DB, niName, prefix string) (db.Value, error) {
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niStaticRouteTbl}, db.Key{Comp: []string{niName, prefix}})
	if err == nil {
		return entry, nil
	}
	if niName != "default" {
		return entry, err
	}
	// Routes configured without the VRF key component belong to the default VRF.
	return cfgDb.GetEntry(&db.TableSpec{Name: niStaticRouteTbl}, db.Key{Comp: []string{prefix}})
}

// populateNiStaticRouteHierToYangFromDb fills the static-routes subtree from
// CONFIG_DB. An empty prefix populates every route of the network-instance.
func populateNiStaticRouteHierToYangFromDb(cfgDb *db.DB, ygRoot *ygot.GoStruct, niName, protoName, prefix string) error {
	prefixes := []string{prefix}
	if prefix == "" {
		prefixes = niStaticRoutePrefixesFromDb(cfgDb, niName)
	}
	for _, p := range prefixes {
		entry, err := niStaticRouteEntryFromDb(cfgDb, niName, p)
		if err != nil {
			continue
		}
		rtObj := getNiStaticRouteObj(ygRoot, niName, protoName, p, true)
		if rtObj == nil {
			continue
		}
		routePrefix := p
		rtObj.Prefix = &routePrefix
		if rtObj.Config != nil {
			rtObj.Config.Prefix = &routePrefix
			niStaticRouteSetTagToYang(rtObj.Config, entry)
		}
		if rtObj.State != nil {
			rtObj.State.Prefix = &routePrefix
			niStaticRouteSetTagToYangState(rtObj.State, entry)
		}
		if err := populateNiStaticRouteNextHopsToYang(ygRoot, niName, protoName, p, entry, ""); err != nil {
			return err
		}
	}
	return nil
}

// populateNiStaticRouteNextHopsToYang fills the next-hop list of one route from
// its stored comma separated attributes. An empty index populates every
// next-hop of the route.
func populateNiStaticRouteNextHopsToYang(ygRoot *ygot.GoStruct, niName, protoName, prefix string, entry db.Value, index string) error {
	for _, e := range niStaticRouteDecodeNextHops(entry) {
		nhIndex := niStaticRouteNhIndex(e)
		if nhIndex == "" || (index != "" && nhIndex != index) {
			continue
		}
		nhObj := getNiStaticRouteNextHopObj(ygRoot, niName, protoName, prefix, nhIndex, true)
		if nhObj == nil {
			continue
		}
		nhKey := nhIndex
		nhObj.Index = &nhKey
		if nhObj.Config == nil {
			continue
		}
		nhObj.Config.Index = &nhKey
		if err := niStaticRouteNextHopUnionValue(nhObj.Config, niStaticRouteNextHopValue(e)); err != nil {
			return err
		}
		if preference, ok := niStaticRoutePreferenceOf(e); ok {
			nhObj.Config.Preference = &preference
		} else {
			// The leaf a GET targets is allocated before this runs, so an
			// absent preference is cleared rather than left at the zero value
			// it was handed: 0 is the absence of a preference, never one the
			// next-hop has.
			nhObj.Config.Preference = nil
		}
	}
	return nil
}

var YangToDb_ni_static_route_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (res map[string]map[string]db.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance static route transformer panic in SET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			res = make(map[string]map[string]db.Value)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance static route transformer panic in SET path: %v", r)}
		}
	}()
	res = make(map[string]map[string]db.Value)

	reqInfo, err := parseNiStaticRouteRequestUri(niHandlerReqUri(inParams))
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		switch reqInfo.targetPathKind {
		case niStaticRouteTargetProtocolRoot, niStaticRouteTargetRoutesRoot:
			// The same two spellings of one request as on the BGP side: the
			// protocol instance holds nothing but its routes, so removing the
			// instance and removing the static-routes container it holds are
			// the same thing.
			return niStaticRouteDeleteAllForNi(inParams, reqInfo.niName)
		case niStaticRouteTargetRouteCfg:
			if reqInfo.targetLeaf == niStaticRouteSetTagLeaf {
				// Only the tag, which lives in the row the route shares with its
				// next-hops rather than being the route's identity.
				return makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, map[string]string{niStaticRouteTagFld: ""}), nil
			}
			// prefix is the CONFIG_DB key, so clearing it removes the route.
			return makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, nil), nil
		case niStaticRouteTargetRouteInst:
			return makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, nil), nil
		case niStaticRouteTargetNextHopsRoot:
			return res, tlerr.NotSupportedError{
				Format: "DELETE is not supported at the next-hops container; delete one next-hop or the static route instead",
				Path:   inParams.requestUri,
			}
		case niStaticRouteTargetNextHopInst, niStaticRouteTargetNextHopCfg:
			if reqInfo.targetLeaf == niStaticRoutePreferenceLeaf {
				// Only the preference, which is one element of the distance
				// list the route's next-hops share, rather than the next-hop
				// that carries it.
				return res, niStaticRouteClearNextHopPreference(inParams, reqInfo)
			}
			return res, niStaticRouteDeleteNextHop(inParams, reqInfo)
		}
		return res, tlerr.NotSupportedError{Format: "unsupported DELETE path under static route mapping", Path: inParams.requestUri}
	}

	switch reqInfo.targetPathKind {
	case niStaticRouteTargetProtocolRoot, niStaticRouteTargetRoutesRoot, niStaticRouteTargetRouteInst, niStaticRouteTargetNextHopsRoot:
		wrappedRoot, wrapErr := wrapNiStaticRouteRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return res, wrapErr
		}
		if unmapped := findNiStaticRouteUnmappedPayloadLeaves(wrappedRoot, reqInfo.niName, reqInfo.protoName); len(unmapped) > 0 {
			return res, tlerr.NotSupportedError{
				Format: "static routes container payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
				Path:   inParams.requestUri,
			}
		}
		res, err = buildNiStaticRouteHierResMap(inParams.dbs[db.ConfigDB], wrappedRoot, reqInfo.niName, reqInfo.protoName)
		if err != nil {
			return make(map[string]map[string]db.Value), err
		}
		if len(res) == 0 {
			if reqInfo.prefix == "" {
				return res, tlerr.InvalidArgsError{Format: "static routes container payload does not include any static route"}
			}
			if err := validateNiStaticRoutePrefix(reqInfo.prefix); err != nil {
				return make(map[string]map[string]db.Value), err
			}
			res = makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, map[string]string{})
		}
		if inParams.invokeCRUSubtreeOnce != nil {
			// The whole subtree is mapped in this call, so translib must not
			// invoke this transformer again for the child nodes.
			*inParams.invokeCRUSubtreeOnce = true
		}
		return res, nil
	case niStaticRouteTargetRouteCfg:
		if err := validateNiStaticRoutePrefix(reqInfo.prefix); err != nil {
			return res, err
		}
		wrappedRoot, wrapErr := wrapNiStaticRouteRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return res, wrapErr
		}
		if unmapped := findNiStaticRouteUnmappedPayloadLeaves(wrappedRoot, reqInfo.niName, reqInfo.protoName); len(unmapped) > 0 {
			return res, tlerr.NotSupportedError{
				Format: "static route payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
				Path:   inParams.requestUri,
			}
		}
		if err := niStaticRouteCheckPayloadPrefix(inParams, reqInfo); err != nil {
			return res, err
		}
		fields, tagErr := niStaticRouteSetTagFields(inParams, wrappedRoot, reqInfo)
		if tagErr != nil {
			return res, tagErr
		}
		if fields == nil {
			// A payload naming the route without its tag still asks for the row.
			fields = map[string]string{}
		}
		return makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, fields), nil
	case niStaticRouteTargetNextHopInst, niStaticRouteTargetNextHopCfg:
		return niStaticRouteSetNextHop(inParams, reqInfo)
	}
	return res, tlerr.NotSupportedError{Format: "unsupported path under the network-instance STATIC protocol instance", Path: inParams.requestUri}
}

// niStaticRouteSetNextHop merges one next-hop into the comma separated
// attributes of the route it belongs to.
func niStaticRouteSetNextHop(inParams XfmrParams, reqInfo *niStaticRouteReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if err := validateNiStaticRoutePrefix(reqInfo.prefix); err != nil {
		return res, err
	}
	wrappedRoot, wrapErr := wrapNiStaticRouteRootAsDevice(inParams.ygRoot, reqInfo)
	if wrapErr != nil {
		return res, wrapErr
	}
	if unmapped := findNiStaticRouteUnmappedPayloadLeaves(wrappedRoot, reqInfo.niName, reqInfo.protoName); len(unmapped) > 0 {
		return res, tlerr.NotSupportedError{
			Format: "static route next-hop payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
			Path:   inParams.requestUri,
		}
	}
	if reqInfo.targetLeaf != niStaticRouteIndexLeaf {
		if nh := niStaticRouteNextHopFromPayload(inParams, wrappedRoot, reqInfo); nh != "" {
			if err := niStaticRouteCheckNextHopValue(nh, reqInfo.index); err != nil {
				return res, err
			}
		}
	}
	if nhObj := getNiStaticRouteNextHopObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, reqInfo.index, false); nhObj != nil &&
		nhObj.Config != nil && nhObj.Config.Index != nil {
		if cfgIndex := strings.TrimSpace(*nhObj.Config.Index); cfgIndex != "" && cfgIndex != reqInfo.index {
			return res, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("static route next-hop index %q in payload does not match index %q in request path", cfgIndex, reqInfo.index),
			}
		}
	}

	nh, err := niStaticRouteNhFromIndex(reqInfo.index, reqInfo.prefix)
	if err != nil {
		return res, err
	}
	distance, hasPreference, prefErr := niStaticRoutePreferenceFromPayload(wrappedRoot, reqInfo)
	if prefErr != nil {
		return res, prefErr
	}
	if hasPreference {
		nh[niStaticRouteDistanceFld] = distance
	}
	existing := niStaticRouteEntryOrEmpty(inParams.dbs[db.ConfigDB], reqInfo.niName, reqInfo.prefix)
	entries := niStaticRouteUpsertNextHop(niStaticRouteDecodeNextHops(existing), reqInfo.index, nh)
	return makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, niStaticRouteEncodeNextHops(entries, existing, reqInfo.prefix)), nil
}

// niStaticRouteDeleteNextHop removes one next-hop from the comma separated
// attributes of its route. The next-hops of a route share one CONFIG_DB row, so
// the remaining ones are rewritten as an update rather than deleted.
func niStaticRouteDeleteNextHop(inParams XfmrParams, reqInfo *niStaticRouteReqInfo) error {
	existing := niStaticRouteEntryOrEmpty(inParams.dbs[db.ConfigDB], reqInfo.niName, reqInfo.prefix)
	entries := niStaticRouteDecodeNextHops(existing)

	kept := make([]map[string]string, 0, len(entries))
	found := false
	for _, e := range entries {
		if niStaticRouteNhIndex(e) == reqInfo.index {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return tlerr.NotFoundError{Format: fmt.Sprintf("static route %s has no next-hop %s", reqInfo.prefix, reqInfo.index)}
	}
	if len(kept) == 0 {
		return tlerr.NotSupportedError{
			Format: "DELETE of the only next-hop is not supported; delete the static route instead",
			Path:   inParams.requestUri,
		}
	}

	fields := niStaticRouteEncodeNextHops(kept, existing, reqInfo.prefix)
	if inParams.subOpDataMap == nil {
		return tlerr.InvalidArgsError{Format: "unable to rewrite static route next-hops for DELETE"}
	}
	updMap := make(RedisDbMap)
	updMap[db.ConfigDB] = makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, fields)
	inParams.subOpDataMap[UPDATE] = &updMap
	return nil
}

// niStaticRouteClearNextHopPreference puts the preference of one next-hop back
// at the default. The next-hops of a route share one row, so the distance list
// is rewritten through an update sub-operation rather than the field deleted:
// deleting the field would drop the preference of every other next-hop too.
func niStaticRouteClearNextHopPreference(inParams XfmrParams, reqInfo *niStaticRouteReqInfo) error {
	existing := niStaticRouteEntryOrEmpty(inParams.dbs[db.ConfigDB], reqInfo.niName, reqInfo.prefix)
	entries := niStaticRouteDecodeNextHops(existing)
	found := false
	for _, e := range entries {
		if niStaticRouteNhIndex(e) != reqInfo.index {
			continue
		}
		found = true
		delete(e, niStaticRouteDistanceFld)
	}
	if !found {
		return tlerr.NotFoundError{Format: fmt.Sprintf("static route %s has no next-hop %s", reqInfo.prefix, reqInfo.index)}
	}
	if inParams.subOpDataMap == nil {
		return tlerr.InvalidArgsError{Format: "unable to rewrite static route next-hop preference for DELETE"}
	}
	updMap := make(RedisDbMap)
	updMap[db.ConfigDB] = makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix,
		niStaticRouteEncodeNextHops(entries, existing, reqInfo.prefix))
	inParams.subOpDataMap[UPDATE] = &updMap
	return nil
}

var DbToYang_ni_static_route_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance static route transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance static route transformer panic in GET path: %v", r)}
		}
	}()

	reqInfo, err := parseNiStaticRouteRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance static route config"}
	}

	switch reqInfo.targetPathKind {
	case niStaticRouteTargetProtocolRoot, niStaticRouteTargetRoutesRoot, niStaticRouteTargetRouteInst:
		wrappedRoot, wrapErr := wrapNiStaticRouteRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return wrapErr
		}
		if reqInfo.targetPathKind == niStaticRouteTargetRouteInst {
			if _, err := niStaticRouteEntryFromDb(cfgDb, reqInfo.niName, reqInfo.prefix); err != nil {
				return err
			}
		}
		return populateNiStaticRouteHierToYangFromDb(cfgDb, wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix)
	case niStaticRouteTargetRouteState:
		// translib removes a wildcard list key before dispatch, so the request
		// path no longer names a route. At the list node that is served, the node
		// being the list itself, but here it identifies nothing and a filled tree
		// would serialise to nothing, so say so rather than answer empty.
		if reqInfo.prefix == "" {
			return tlerr.NotSupportedError{Format: "a wildcard or absent static route prefix is served at the route list; address static-routes/static rather than a container below it"}
		}
		stateObj := getNiStaticRouteStateObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, true)
		if stateObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate static route state object in YANG tree"}
		}
		// State is what FRR holds rather than what was written. A route FRR has
		// not taken reports nothing, rather than echoing the configuration back
		// and calling it applied, so an absent applied row leaves the container
		// empty instead of failing.
		applied, held := niStaticRouteAppliedEntry(inParams, reqInfo.niName, reqInfo.prefix)
		if !held {
			return nil
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niStaticRoutePrefixLeaf {
			routePrefix := reqInfo.prefix
			stateObj.Prefix = &routePrefix
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niStaticRouteSetTagLeaf {
			niStaticRouteSetTagToYangState(stateObj, applied)
		}
		return nil
	case niStaticRouteTargetRouteCfg:
		if reqInfo.prefix == "" {
			return tlerr.NotSupportedError{Format: "a wildcard or absent static route prefix is served at the route list; address static-routes/static rather than a container below it"}
		}
		entry, err := niStaticRouteEntryFromDb(cfgDb, reqInfo.niName, reqInfo.prefix)
		if err != nil {
			return err
		}
		cfgObj := getNiStaticRouteCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate static route config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niStaticRoutePrefixLeaf {
			routePrefix := reqInfo.prefix
			cfgObj.Prefix = &routePrefix
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == niStaticRouteSetTagLeaf {
			niStaticRouteSetTagToYang(cfgObj, entry)
		}
		return nil
	case niStaticRouteTargetNextHopsRoot, niStaticRouteTargetNextHopInst, niStaticRouteTargetNextHopCfg:
		entry, err := niStaticRouteEntryFromDb(cfgDb, reqInfo.niName, reqInfo.prefix)
		if err != nil {
			return err
		}
		wrappedRoot, wrapErr := wrapNiStaticRouteRootAsDevice(inParams.ygRoot, reqInfo)
		if wrapErr != nil {
			return wrapErr
		}
		if err := populateNiStaticRouteNextHopsToYang(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, entry, reqInfo.index); err != nil {
			return err
		}
		if reqInfo.index == "" {
			return nil
		}
		nhObj := getNiStaticRouteNextHopObj(wrappedRoot, reqInfo.niName, reqInfo.protoName, reqInfo.prefix, reqInfo.index, false)
		if nhObj == nil {
			return tlerr.NotFoundError{Format: fmt.Sprintf("static route %s has no next-hop %s", reqInfo.prefix, reqInfo.index)}
		}
		if reqInfo.targetPathKind == niStaticRouteTargetNextHopCfg && nhObj.Config != nil {
			// A leaf targeted GET reports only the leaf it asked for.
			switch reqInfo.targetLeaf {
			case niStaticRouteIndexLeaf:
				nhObj.Config.NextHop = nil
				nhObj.Config.Preference = nil
			case niStaticRouteNextHopLeaf:
				nhObj.Config.Index = nil
				nhObj.Config.Preference = nil
			case niStaticRoutePreferenceLeaf:
				nhObj.Config.Index = nil
				nhObj.Config.NextHop = nil
			}
		}
		return nil
	}
	return nil
}

// niStaticRouteYangToDbDispatch handles a SET on the STATIC protocol instance
// on behalf of the transformer bound to the protocol list.
func niStaticRouteYangToDbDispatch(inParams XfmrParams) (bool, map[string]map[string]db.Value, error) {
	if !isNiStaticRouteReqUri(niHandlerReqUri(inParams)) {
		return false, nil, nil
	}
	res, err := YangToDb_ni_static_route_cfg_xfmr(inParams)
	return true, res, err
}

// niStaticRouteDbToYangDispatch handles a GET on the STATIC protocol instance
// on behalf of the transformer bound to the protocol list.
func niStaticRouteDbToYangDispatch(inParams XfmrParams) (bool, error) {
	if !isNiStaticRouteReqUri(inParams.requestUri) {
		return false, nil
	}
	return true, DbToYang_ni_static_route_cfg_xfmr(inParams)
}
