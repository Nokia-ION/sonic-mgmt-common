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
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niStaticRouteTbl = "STATIC_ROUTE"

	niStaticRouteNexthopFld    = "nexthop"
	niStaticRouteIfnameFld     = "ifname"
	niStaticRouteBlackholeFld  = "blackhole"
	niStaticRouteNexthopVrfFld = "nexthop-vrf"
	niStaticRouteDistanceFld   = "distance"
	niStaticRouteTrackFld      = "track"
	niStaticRouteTagFld        = "tag"

	niStaticRouteTargetProtocolRoot = "static-protocol-root"
	niStaticRouteTargetRoutesRoot   = "static-routes-root"
	niStaticRouteTargetRouteInst    = "static-route-instance"
	niStaticRouteTargetRouteCfg     = "static-route-config"

	niStaticRoutePrefixLeaf = "prefix"

	// niStaticRouteMaxTreeDepth bounds the container recursion done while
	// walking a static route YANG subtree.
	niStaticRouteMaxTreeDepth = 32
)

// The protocol instance name is not part of the CONFIG_DB key, so any name is
// accepted for identifier=STATIC. The identifier may arrive module qualified.
const niStaticRouteUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=(?:[-A-Za-z0-9_.]+:)?STATIC\]\[name=([^\]]+)\]`

var niStaticRouteProtoRootReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `$`)
var niStaticRouteProtoInstanceReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `(?:/.*)?$`)
var niStaticRoutesRootReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes$`)
var niStaticRouteInstReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]$`)
var niStaticRouteKeyLeafReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/prefix$`)
var niStaticRouteCfgReqUriRe = regexp.MustCompile(niStaticRouteUriPfx + `/static-routes/static\[prefix=([^\]]+)\]/config(?:/(prefix))?$`)

type niStaticRouteReqInfo struct {
	niName         string
	protoName      string
	prefix         string
	targetLeaf     string
	targetPathKind string
}

func init() {
	XlateFuncBind("YangToDb_ni_static_route_cfg_xfmr", YangToDb_ni_static_route_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_static_route_cfg_xfmr", DbToYang_ni_static_route_cfg_xfmr)
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

func parseNiStaticRouteRequestUri(uri string) (*niStaticRouteReqInfo, error) {
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
	return nil, tlerr.NotSupportedError{
		Format: "unsupported path for network-instance static route mapping: " + uri,
		Path:   uri,
	}
}

// niStaticRouteMappedPayloadLeaves lists every leaf, relative to the
// static-routes container, that a container-level request can carry. List keys
// are included because a payload repeats them alongside the data they identify.
var niStaticRouteMappedPayloadLeaves = map[string]bool{
	"static/prefix":        true,
	"static/config/prefix": true,
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
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for static route mapping"}
	}

	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
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
// container-level request carry several static routes.
func buildNiStaticRouteHierResMap(s *ygot.GoStruct, niName, protoName string) (map[string]map[string]db.Value, error) {
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
		if _, ok := res[niStaticRouteTbl]; !ok {
			res[niStaticRouteTbl] = make(map[string]db.Value)
		}
		res[niStaticRouteTbl][niName+"|"+prefix] = db.Value{Field: map[string]string{}}
	}
	return res, nil
}

// niStaticRoutePrefixesFromDb lists the prefixes configured for one VRF. A key
// without a VRF component belongs to the default VRF.
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
		if _, err := niStaticRouteEntryFromDb(cfgDb, niName, p); err != nil {
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

	reqInfo, err := parseNiStaticRouteRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		switch reqInfo.targetPathKind {
		case niStaticRouteTargetProtocolRoot, niStaticRouteTargetRoutesRoot:
			return res, tlerr.NotSupportedError{Format: "DELETE is not supported at static routes container level; delete a static route instance instead", Path: inParams.requestUri}
		case niStaticRouteTargetRouteInst, niStaticRouteTargetRouteCfg:
			// prefix is the CONFIG_DB key, so clearing it removes the route.
			return makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, nil), nil
		}
		return res, tlerr.NotSupportedError{Format: "unsupported DELETE path under static route mapping", Path: inParams.requestUri}
	}

	switch reqInfo.targetPathKind {
	case niStaticRouteTargetProtocolRoot, niStaticRouteTargetRoutesRoot, niStaticRouteTargetRouteInst:
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
		res, err = buildNiStaticRouteHierResMap(wrappedRoot, reqInfo.niName, reqInfo.protoName)
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
		if err := niStaticRouteCheckPayloadPrefix(inParams, reqInfo); err != nil {
			return res, err
		}
		return makeNiStaticRouteResMap(reqInfo.niName, reqInfo.prefix, map[string]string{}), nil
	}
	return res, tlerr.NotSupportedError{Format: "unsupported path for network-instance static route mapping", Path: inParams.requestUri}
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
	case niStaticRouteTargetRouteCfg:
		if _, err := niStaticRouteEntryFromDb(cfgDb, reqInfo.niName, reqInfo.prefix); err != nil {
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
		return nil
	}
	return nil
}

// niStaticRouteYangToDbDispatch handles a SET on the STATIC protocol instance
// on behalf of the transformer bound to the protocol list.
func niStaticRouteYangToDbDispatch(inParams XfmrParams) (bool, map[string]map[string]db.Value, error) {
	if !isNiStaticRouteReqUri(inParams.requestUri) {
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
