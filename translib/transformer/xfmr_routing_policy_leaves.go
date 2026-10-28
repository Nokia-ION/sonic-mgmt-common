package transformer

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// rpolStmtLeafFlds maps the statement leaves that are plain ROUTE_MAP fields,
// by the config container holding them below the statement. A leaf mapped to
// "" holds no field: match-set-options is accepted as ANY, the only option a
// route-map match offers, and refused otherwise.
var rpolStmtLeafFlds = map[string]map[string]string{
	"conditions/config": {
		"call-policy":         rpolCallRouteMapFld,
		"install-protocol-eq": rpolMatchProtocolFld,
	},
	"conditions/match-interface/config": {
		"interface": rpolMatchInterfaceFld,
	},
	"conditions/bgp-conditions/match-as-path-set/config": {
		"as-path-set":       rpolMatchAsPathFld,
		"match-set-options": "",
	},
	"conditions/bgp-conditions/match-community-set/config": {
		"community-set":     rpolMatchCommunityFld,
		"match-set-options": "",
	},
	"conditions/bgp-conditions/match-ext-community-set/config": {
		"ext-community-set": rpolMatchExtCommunityFld,
		"match-set-options": "",
	},
}

const (
	rpolCallRouteMapFld         = "call_route_map"
	rpolMatchProtocolFld        = "match_protocol"
	rpolMatchInterfaceFld       = "match_interface"
	rpolMatchAsPathFld          = "match_as_path"
	rpolSetExtCommInlineFld     = "set_ext_community_inline@"
	rpolSetExtCommRefFld        = "set_ext_community_ref"
	rpolAppliedExtCommInlineFld = "set_ext_community_inline"
	rpolSetExtCommNode          = "actions/bgp-actions/set-ext-community"
	rpolAsPathSetTbl            = "AS_PATH_SET"
	rpolAsPathSetMemberFld      = "as_path_set_member@"
)

// The ROUTE_MAP fields the leaves above and set-ext-community own, by the
// container a REPLACE or a DELETE of it covers.
var rpolCondLeafFlds = []string{rpolCallRouteMapFld, rpolMatchProtocolFld, rpolMatchInterfaceFld, rpolMatchAsPathFld}
var rpolSetExtCommFlds = []string{rpolSetExtCommInlineFld, rpolSetExtCommRefFld}

// The install protocols ROUTE_MAP match_protocol takes, as FRR spells them.
var rpolInstallProtocols = map[ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE]string{
	ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP:                "bgp",
	ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED: "connected",
	ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC:             "static",
}

// rpolModPfxRe strips the module prefix of a path element.
var rpolModPfxRe = regexp.MustCompile(`(^|/)[A-Za-z0-9_.-]+:`)

var rpolStmtLeafReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/(.+)$`)

// The nodes below a statement this file serves: the containers of
// rpolStmtLeafFlds and set-ext-community, at or below them.
var rpolStmtLeafNodes = []string{"conditions/config", "conditions/state", "conditions/match-interface",
	"conditions/bgp-conditions/match-as-path-set", "conditions/bgp-conditions/match-community-set",
	"conditions/bgp-conditions/match-ext-community-set", rpolSetExtCommNode}

type rpolStmtLeafReq struct {
	policy, stmt string
	node         string // below the statement, without module prefixes
	leaf         string // the leaf named, when the node is a config or state container
}

func matchRpolStmtLeafReq(uri string) (*rpolStmtLeafReq, bool) {
	m := rpolStmtLeafReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, false
	}
	rest := rpolModPfxRe.ReplaceAllString(m[3], "$1")
	for _, n := range rpolStmtLeafNodes {
		if rest != n && !strings.HasPrefix(rest, n+"/") {
			continue
		}
		r := &rpolStmtLeafReq{policy: m[1], stmt: m[2], node: rest}
		if i := strings.LastIndex(rest, "/"); i > 0 {
			if parent := rest[:i]; strings.HasSuffix(parent, "/config") || strings.HasSuffix(parent, "/state") || parent == "conditions/config" || parent == "conditions/state" {
				r.node, r.leaf = parent, rest[i+1:]
			}
		}
		return r, true
	}
	return nil, false
}

// state reports a request for a state container or below one.
func (r *rpolStmtLeafReq) state() bool {
	return strings.HasSuffix(r.node, "/state") || strings.Contains(r.node+"/", "/state/")
}

// cfgContainers lists the containers of rpolStmtLeafFlds the request covers.
func (r *rpolStmtLeafReq) cfgContainers() []string {
	node := strings.TrimSuffix(strings.TrimSuffix(r.node, "/state"), "/config")
	if r.node == "conditions/config" || r.node == "conditions/state" {
		return []string{"conditions/config"}
	}
	var out []string
	for _, c := range niBgpSortedKeys(rpolStmtLeafFlds) {
		if c != "conditions/config" && (c == node+"/config" || strings.HasPrefix(c, node+"/")) {
			out = append(out, c)
		}
	}
	return out
}

func rpolStmtLeafToDb(container, leaf string, f reflect.Value) (string, bool, error) {
	switch leaf {
	case "install-protocol-eq":
		v := f.Interface().(ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE)
		if v == ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_UNSET {
			return "", false, nil
		}
		p, ok := rpolInstallProtocols[v]
		if !ok {
			return "", false, tlerr.NotSupportedError{Format: "routing-policy statement conditions install-protocol-eq supports BGP, DIRECTLY_CONNECTED and STATIC only"}
		}
		return p, true, nil
	case "match-set-options":
		v := f.Interface().(ocbinds.E_OpenconfigRoutingPolicy_MatchSetOptionsType)
		if v != ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET && v != ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_ANY {
			return "", false, tlerr.NotSupportedError{Format: fmt.Sprintf("routing-policy statement %s match-set-options supports ANY only", strings.TrimSuffix(container, "/config"))}
		}
		return "", false, nil
	}
	val, ok := niBgpGlobalLeafToDb(f)
	return strings.TrimSpace(val), ok, nil
}

func rpolStmtLeafFromDb(leaf string, f reflect.Value, raw string) {
	if leaf == "install-protocol-eq" {
		for e, p := range rpolInstallProtocols {
			if p == strings.TrimSpace(raw) {
				f.Set(reflect.ValueOf(e))
			}
		}
		return
	}
	_ = niBgpGlobalLeafFromDb(f, raw)
}

// rpolStmtLeafFields maps the leaves of container that cfg holds, refusing any
// other leaf set in it.
func rpolStmtLeafFields(container string, cfg reflect.Value) (map[string]string, error) {
	fields := map[string]string{}
	if !cfg.IsValid() || cfg.IsNil() {
		return fields, nil
	}
	v := cfg.Elem()
	for i := 0; i < v.NumField(); i++ {
		leaf := v.Type().Field(i).Tag.Get("path")
		fld, mapped := rpolStmtLeafFlds[container][leaf]
		if !mapped {
			if !v.Field(i).IsZero() {
				return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("routing-policy statement %s %s has no CONFIG_DB mapping", container, leaf)}
			}
			continue
		}
		val, set, err := rpolStmtLeafToDb(container, leaf, v.Field(i))
		if err != nil {
			return nil, err
		}
		if set && fld != "" {
			fields[fld] = val
		}
	}
	return fields, nil
}

// rpolSetExtCommFields maps a set-ext-community node, sec, onto the inline or
// the reference field, each clearing the other.
func rpolSetExtCommFields(sec *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetExtCommunity) (map[string]string, error) {
	fields := map[string]string{}
	if sec == nil {
		return fields, nil
	}
	const pfx = "routing-policy statement actions bgp-actions set-ext-community"
	method := ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_UNSET
	if sec.Config != nil {
		method = sec.Config.Method
		if o := sec.Config.Options; o != ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_UNSET && o != ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_REPLACE {
			return nil, tlerr.NotSupportedError{Format: pfx + " options supports REPLACE only"}
		}
	}
	var inline []string
	if sec.Inline != nil && sec.Inline.Config != nil {
		for _, c := range sec.Inline.Config.Communities {
			s, ok := c.(*ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetExtCommunity_Inline_Config_Communities_Union_String)
			if !ok {
				return nil, tlerr.NotSupportedError{Format: pfx + " inline communities supports route-target and route-origin communities only"}
			}
			if v := strings.TrimSpace(s.String); v != "" {
				if strings.Contains(v, ",") {
					return nil, tlerr.InvalidArgsError{Format: pfx + " inline community " + v + " cannot hold a comma"}
				}
				inline = append(inline, v)
			}
		}
	}
	ref := ""
	if sec.Reference != nil && sec.Reference.Config != nil {
		refs := map[string]bool{}
		for _, r := range sec.Reference.Config.ExtCommunitySetRefs {
			if v := strings.TrimSpace(r); v != "" {
				refs[v] = true
			}
		}
		if sec.Reference.Config.ExtCommunitySetRef != nil && strings.TrimSpace(*sec.Reference.Config.ExtCommunitySetRef) != "" {
			refs[strings.TrimSpace(*sec.Reference.Config.ExtCommunitySetRef)] = true
		}
		if len(refs) > 1 {
			return nil, tlerr.NotSupportedError{Format: pfx + " reference holds one ext-community-set; ROUTE_MAP set_ext_community_ref names a single set"}
		}
		for r := range refs {
			ref = r
		}
	}
	isInline := method == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE
	isRef := method == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_REFERENCE
	switch {
	case len(inline) > 0 && ref != "":
		return nil, tlerr.InvalidArgsError{Format: pfx + " carries both inline communities and a reference; a statement sets them by one method"}
	case len(inline) > 0 && isRef, ref != "" && isInline:
		return nil, tlerr.InvalidArgsError{Format: pfx + " method does not match the communities the payload carries"}
	case len(inline) > 0:
		fields[rpolSetExtCommInlineFld] = strings.Join(inline, ",")
		fields[niBgpClearFldPrefix+rpolSetExtCommRefFld] = ""
	case ref != "":
		fields[rpolSetExtCommRefFld] = ref
		fields[niBgpClearFldPrefix+rpolSetExtCommInlineFld] = ""
	}
	return fields, nil
}

// rpolStmtLeafWriteFields maps every node of the request the statement obj in
// the payload holds.
func rpolStmtLeafWriteFields(r *rpolStmtLeafReq, stmtObj reflect.Value) (map[string]string, error) {
	fields := map[string]string{}
	for _, c := range r.cfgContainers() {
		f, err := rpolStmtLeafFields(c, ocWalkPath(stmtObj, c, false))
		if err != nil {
			return nil, err
		}
		for k, v := range f {
			fields[k] = v
		}
	}
	if strings.HasPrefix(r.node, rpolSetExtCommNode) {
		sec, _ := ocWalkPath(stmtObj, rpolSetExtCommNode, false).Interface().(*ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetExtCommunity)
		f, err := rpolSetExtCommFields(sec)
		if err != nil {
			return nil, err
		}
		for k, v := range f {
			fields[k] = v
		}
	}
	if r.leaf != "" {
		narrowed := map[string]string{}
		for _, c := range r.cfgContainers() {
			if fld := rpolStmtLeafFlds[c][r.leaf]; fld != "" {
				if v, ok := fields[fld]; ok {
					narrowed[fld] = v
				}
			}
		}
		if !strings.HasPrefix(r.node, rpolSetExtCommNode) {
			fields = narrowed
		}
	}
	return fields, nil
}

// rpolStmtLeafDeleteFields lists the fields a DELETE of the request removes.
func rpolStmtLeafDeleteFields(r *rpolStmtLeafReq) (map[string]string, error) {
	fields := map[string]string{}
	for _, c := range r.cfgContainers() {
		for leaf, fld := range rpolStmtLeafFlds[c] {
			if fld != "" && (r.leaf == "" || r.leaf == leaf) {
				fields[fld] = ""
			}
		}
	}
	switch {
	case strings.HasPrefix(r.node, rpolSetExtCommNode+"/inline"):
		fields[rpolSetExtCommInlineFld] = ""
	case strings.HasPrefix(r.node, rpolSetExtCommNode+"/reference"):
		fields[rpolSetExtCommRefFld] = ""
	case strings.HasPrefix(r.node, rpolSetExtCommNode):
		fields[rpolSetExtCommInlineFld], fields[rpolSetExtCommRefFld] = "", ""
	}
	if r.leaf != "" && len(fields) == 0 {
		return nil, tlerr.NotSupportedError{Format: "DELETE of routing-policy statement " + r.node + " " + r.leaf + " is not supported; delete its container"}
	}
	return fields, nil
}

func rpolStmtLeafYangToDb(inParams XfmrParams, r *rpolStmtLeafReq) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if _, err := parseRpolStmtNameToUint16(r.stmt); err != nil {
		return res, err
	}
	if r.state() {
		return res, tlerr.NotSupportedError{Format: "routing-policy state is read-only", Path: inParams.requestUri}
	}
	if inParams.oper == DELETE {
		fields, err := rpolStmtLeafDeleteFields(r)
		if err != nil {
			return res, err
		}
		return makeRpolRouteMapResMap(r.policy, r.stmt, fields), nil
	}
	fields, err := rpolStmtLeafWriteFields(r, reflect.ValueOf(getRpolPolicyStmtObj(inParams.ygRoot, r.policy, r.stmt, false)))
	if err != nil {
		return res, err
	}
	set, clear := niBgpSplitClearFlds(fields)
	if r.leaf != "" && len(set) == 0 {
		return res, tlerr.InvalidArgsError{Format: "routing-policy statement " + r.node + " " + r.leaf + " is missing in request payload"}
	}
	if held := niBgpHeldFields(inParams, makeRpolRouteMapResMap(r.policy, r.stmt, clear)); len(held) > 0 && inParams.oper != REPLACE {
		if err := rpolStageSubOp(inParams, DELETE, held); err != nil {
			return res, err
		}
	}
	res = makeRpolRouteMapResMap(r.policy, r.stmt, set)
	if name := set[rpolMatchCommunityFld]; name != "" {
		res = mergeDbResMap(res, makeRpolCommunitySetResMap(name, rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolCommunitySetTbl, name, "")))
	}
	if name := set[rpolMatchExtCommunityFld]; name != "" {
		res = mergeDbResMap(res, makeRpolExtCommunitySetResMap(name, rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolExtCommunitySetTbl, name, "")))
	}
	return res, nil
}

// rpolStmtLeavesToYang fills the nodes of the request below stmtObj from a
// ROUTE_MAP row, entry, or from its applied row, the state containers being
// filled from FRR's view. cfgSide names the container FRR's view or the
// configuration lands in: "config" or "state".
func rpolStmtLeavesToYang(stmtObj reflect.Value, r *rpolStmtLeafReq, entry db.Value, cfgSide string) {
	for _, c := range r.cfgContainers() {
		target := strings.TrimSuffix(c, "config") + cfgSide
		for leaf, fld := range rpolStmtLeafFlds[c] {
			if fld == "" || (r.leaf != "" && r.leaf != leaf) || strings.TrimSpace(entry.Field[fld]) == "" {
				continue
			}
			rpolStmtLeafFromDb(leaf, ocFieldByPath(ocWalkPath(stmtObj, target, true), leaf), entry.Field[fld])
		}
	}
	if !strings.HasPrefix(r.node, rpolSetExtCommNode) {
		return
	}
	sec := ocWalkPath(stmtObj, rpolSetExtCommNode, true)
	inlineFld, refFld := rpolSetExtCommInlineFld, rpolSetExtCommRefFld
	if cfgSide == "state" {
		// FRR holds the members a reference expands to, and so reports every
		// set extcommunity as inline.
		inlineFld, refFld = rpolAppliedExtCommInlineFld, ""
	}
	var members []string
	for _, m := range strings.Split(entry.Field[inlineFld], ",") {
		if v := strings.TrimSpace(m); v != "" {
			members = append(members, v)
		}
	}
	ref := ""
	if refFld != "" {
		ref = strings.TrimSpace(entry.Field[refFld])
	}
	if len(members) == 0 && ref == "" {
		return
	}
	node := strings.TrimPrefix(strings.TrimPrefix(r.node, rpolSetExtCommNode), "/")
	want := func(n string) bool {
		return node == "" || node == n || strings.HasPrefix(node, n+"/") || strings.HasPrefix(n, node)
	}
	if want(cfgSide) && (r.leaf == "" || r.leaf == "method") {
		method := ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE
		if ref != "" {
			method = ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_REFERENCE
		}
		ocFieldByPath(ocWalkPath(sec, cfgSide, true), "method").Set(reflect.ValueOf(method))
	}
	if len(members) > 0 && want("inline/"+cfgSide) {
		list := ocFieldByPath(ocWalkPath(sec, "inline/"+cfgSide, true), "communities")
		union := list.Type().Elem()
		for _, m := range members {
			u := reflect.New(rpolExtCommUnionStringType(union))
			u.Elem().Field(0).SetString(m)
			list.Set(reflect.Append(list, u))
		}
	}
	if ref != "" && want("reference/"+cfgSide) {
		refNode := ocWalkPath(sec, "reference/"+cfgSide, true)
		ocFieldByPath(refNode, "ext-community-set-refs").Set(reflect.ValueOf([]string{ref}))
		ocFieldByPath(refNode, "ext-community-set-ref").Set(reflect.ValueOf(&ref))
	}
}

// rpolExtCommUnionStringType returns the string member type of the inline
// communities union of the config or the state container, union being that
// union's interface type.
func rpolExtCommUnionStringType(union reflect.Type) reflect.Type {
	if strings.Contains(union.Name(), "_State_") {
		return reflect.TypeOf(ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetExtCommunity_Inline_State_Communities_Union_String{})
	}
	return reflect.TypeOf(ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetExtCommunity_Inline_Config_Communities_Union_String{})
}

func rpolStmtLeafDbToYang(inParams XfmrParams, r *rpolStmtLeafReq) error {
	if _, err := parseRpolStmtNameToUint16(r.stmt); err != nil {
		return err
	}
	stmtObj := reflect.ValueOf(getRpolPolicyStmtObj(inParams.ygRoot, r.policy, r.stmt, true))
	if !stmtObj.IsValid() || stmtObj.IsNil() {
		return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement object in YANG tree"}
	}
	stateOnly := r.state()
	cfgOnly := !stateOnly && (strings.HasSuffix(r.node, "/config") || r.leaf != "" || strings.Contains(r.node+"/", "/config/"))
	if !stateOnly {
		entry, err := inParams.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{r.policy, r.stmt}})
		if err != nil {
			return err
		}
		rpolStmtLeavesToYang(stmtObj, r, entry, "config")
	}
	if !cfgOnly {
		if applied, ok := rpolAppliedStmtEntry(inParams, r.policy, r.stmt); ok {
			rpolStmtLeavesToYang(stmtObj, r, applied, "state")
		}
	}
	return nil
}

// rpolStmtLeafSetUris names the nodes of this file a statement payload sets,
// for a container-level write to replay.
func rpolStmtLeafSetUris(policyName, stmtName string, stmtObj reflect.Value) []string {
	var uris []string
	for _, c := range niBgpSortedKeys(rpolStmtLeafFlds) {
		if cfg := ocWalkPath(stmtObj, c, false); cfg.IsValid() && !cfg.Elem().IsZero() {
			uris = append(uris, rpolStmtUriPrefix(policyName, stmtName)+"/"+c)
		}
	}
	if sec := ocWalkPath(stmtObj, rpolSetExtCommNode, false); sec.IsValid() && !sec.Elem().IsZero() {
		uris = append(uris, rpolStmtUriPrefix(policyName, stmtName)+"/"+rpolSetExtCommNode)
	}
	return uris
}

// rpolStmtLeafGetUris names the nodes of this file a statement row holds data
// for, for a container-level read to replay.
func rpolStmtLeafGetUris(policyName, stmtName string, entry db.Value) []string {
	var uris []string
	for _, c := range niBgpSortedKeys(rpolStmtLeafFlds) {
		for _, fld := range rpolStmtLeafFlds[c] {
			if fld == "" || strings.TrimSpace(entry.Field[fld]) == "" {
				continue
			}
			if c == "conditions/config" {
				// The conditions container itself is the routing-policy
				// transformer's, so its config and state are named apart.
				uris = append(uris, rpolStmtUriPrefix(policyName, stmtName)+"/conditions/config",
					rpolStmtUriPrefix(policyName, stmtName)+"/conditions/state")
			} else {
				uris = append(uris, rpolStmtUriPrefix(policyName, stmtName)+"/"+strings.TrimSuffix(c, "/config"))
			}
			break
		}
	}
	for _, fld := range rpolSetExtCommFlds {
		if strings.TrimSpace(entry.Field[fld]) != "" {
			uris = append(uris, rpolStmtUriPrefix(policyName, stmtName)+"/"+rpolSetExtCommNode)
			break
		}
	}
	return uris
}

// AS path sets: AS_PATH_SET|<name> holding the members. frrcfgd renders every
// member as a permit entry of the as-path access-list, so a set has no other
// field.
var rpolAsPathSetReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?as-path-sets(?:/(?:[A-Za-z0-9_.-]+:)?as-path-set(?:\[(?:[A-Za-z0-9_.-]+:)?as-path-set-name=([^\]]+)\](?:/(?:[A-Za-z0-9_.-]+:)?(as-path-set-name|config|state)(?:/(?:[A-Za-z0-9_.-]+:)?(as-path-set-name|as-path-set-member))?)?)?)?$`)

type rpolAsPathSetReq struct {
	name, node, leaf string // node: "" for the set or the list, else as-path-set-name, config or state
}

func matchRpolAsPathSetReq(uri string) (*rpolAsPathSetReq, bool) {
	m := rpolAsPathSetReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, false
	}
	return &rpolAsPathSetReq{name: m[1], node: m[2], leaf: m[3]}, true
}

func rpolAsPathSetUri(name string) string {
	return "/openconfig-routing-policy:routing-policy/defined-sets/openconfig-bgp-policy:bgp-defined-sets/openconfig-bgp-policy:as-path-sets/openconfig-bgp-policy:as-path-set[as-path-set-name=" + name + "]"
}

func getRpolAsPathSetObj(s *ygot.GoStruct, name string, create bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_AsPathSets_AsPathSet {
	rp := lookupRpolObj(s)
	if rp == nil {
		if !create {
			return nil
		}
		if d, ok := (*s).(*ocbinds.Device); ok {
			ygot.BuildEmptyTree(d)
			rp = d.RoutingPolicy
		}
		if rp == nil {
			return nil
		}
	}
	sets := ocWalkPath(reflect.ValueOf(rp), "defined-sets/bgp-defined-sets/as-path-sets", create)
	if !sets.IsValid() {
		return nil
	}
	setsObj := sets.Interface().(*ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_AsPathSets)
	if obj, ok := setsObj.AsPathSet[name]; ok {
		return obj
	}
	if !create {
		return nil
	}
	obj, err := setsObj.NewAsPathSet(name)
	if err != nil {
		return nil
	}
	return obj
}

func rpolAsPathSetYangToDb(inParams XfmrParams, r *rpolAsPathSetReq) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if r.name == "" {
		if inParams.oper == DELETE {
			return res, tlerr.NotSupportedError{Format: "DELETE of the as-path-sets container is not supported; delete each as-path-set", Path: inParams.requestUri}
		}
		rp := lookupRpolObj(inParams.ygRoot)
		sets := ocWalkPath(reflect.ValueOf(rp), "defined-sets/bgp-defined-sets/as-path-sets", false)
		if !sets.IsValid() {
			return res, nil
		}
		for name := range sets.Interface().(*ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_AsPathSets).AsPathSet {
			part, err := rpolAsPathSetYangToDb(inParams, &rpolAsPathSetReq{name: name})
			if err != nil {
				return res, err
			}
			res = mergeDbResMap(res, part)
		}
		return res, nil
	}
	if r.node == "state" {
		return res, tlerr.NotSupportedError{Format: "routing-policy state is read-only", Path: inParams.requestUri}
	}
	if inParams.oper == DELETE {
		if r.leaf == "as-path-set-member" {
			return res, tlerr.NotSupportedError{Format: "DELETE of as-path-set-member is not supported; delete the as-path-set", Path: inParams.requestUri}
		}
		return map[string]map[string]db.Value{rpolAsPathSetTbl: {r.name: db.Value{Field: map[string]string{}}}}, nil
	}
	obj := getRpolAsPathSetObj(inParams.ygRoot, r.name, false)
	fields := map[string]string{}
	if obj != nil && obj.Config != nil {
		if n := obj.Config.AsPathSetName; n != nil && strings.TrimSpace(*n) != "" && strings.TrimSpace(*n) != r.name {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("as-path-set-name payload (%s) does not match key (%s)", strings.TrimSpace(*n), r.name)}
		}
		var members []string
		for _, m := range obj.Config.AsPathSetMember {
			if v := strings.TrimSpace(m); v != "" {
				if strings.Contains(v, ",") {
					return res, tlerr.InvalidArgsError{Format: "as-path-set-member " + v + " cannot hold a comma"}
				}
				members = append(members, v)
			}
		}
		if len(members) > 0 {
			fields[rpolAsPathSetMemberFld] = strings.Join(members, ",")
		}
	}
	if r.leaf == "as-path-set-member" && len(fields) == 0 {
		return res, tlerr.InvalidArgsError{Format: "as-path-set-member is missing in request payload"}
	}
	return map[string]map[string]db.Value{rpolAsPathSetTbl: {r.name: db.Value{Field: fields}}}, nil
}

func rpolAsPathSetDbToYang(inParams XfmrParams, r *rpolAsPathSetReq) error {
	cfgDb := inParams.dbs[db.ConfigDB]
	names := []string{r.name}
	if r.name == "" {
		names = rpolAsPathSetNamesFromDb(cfgDb)
	}
	for _, name := range names {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolAsPathSetTbl}, db.Key{Comp: []string{name}})
		if err != nil {
			if r.name == "" {
				continue
			}
			return err
		}
		obj := getRpolAsPathSetObj(inParams.ygRoot, name, true)
		if obj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy as-path-set object in YANG tree"}
		}
		ygot.BuildEmptyTree(obj)
		n := name
		obj.Config.AsPathSetName = &n
		var members []string
		for _, m := range strings.Split(entry.Field[rpolAsPathSetMemberFld], ",") {
			if v := strings.TrimSpace(m); v != "" {
				members = append(members, v)
			}
		}
		obj.Config.AsPathSetMember = members
		switch r.leaf {
		case "as-path-set-name":
			obj.Config.AsPathSetMember = nil
		case "as-path-set-member":
			obj.Config.AsPathSetName = nil
		}
		ocMirrorConfigToState(obj)
		if r.node == "state" {
			obj.Config = nil
		} else if r.node == "config" {
			obj.State = nil
		}
	}
	return nil
}

func rpolAsPathSetNamesFromDb(cfgDb *db.DB) []string {
	var names []string
	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolAsPathSetTbl}); err == nil {
		for _, k := range keys {
			if len(k.Comp) == 1 && strings.TrimSpace(k.Comp[0]) != "" {
				names = append(names, k.Comp[0])
			}
		}
	}
	sort.Strings(names)
	return names
}

// rpolLeavesYangToDb serves a write this file maps, reporting whether it did.
func rpolLeavesYangToDb(inParams XfmrParams) (map[string]map[string]db.Value, bool, error) {
	if r, ok := matchRpolStmtLeafReq(inParams.requestUri); ok {
		res, err := rpolStmtLeafYangToDb(inParams, r)
		return res, true, err
	}
	if r, ok := matchRpolAsPathSetReq(inParams.requestUri); ok {
		res, err := rpolAsPathSetYangToDb(inParams, r)
		return res, true, err
	}
	return nil, false, nil
}

// rpolLeavesDbToYang serves a read this file maps, reporting whether it did.
func rpolLeavesDbToYang(inParams XfmrParams) (bool, error) {
	if r, ok := matchRpolStmtLeafReq(inParams.requestUri); ok {
		return true, rpolStmtLeafDbToYang(inParams, r)
	}
	if r, ok := matchRpolAsPathSetReq(inParams.requestUri); ok {
		return true, rpolAsPathSetDbToYang(inParams, r)
	}
	return false, nil
}

func init() {
	for c, leaves := range rpolStmtLeafFlds {
		for leaf := range leaves {
			rpolMappedPayloadLeaves["policy-definitions/policy-definition/statements/statement/"+c+"/"+leaf] = true
		}
	}
	for _, p := range []string{"config/method", "config/options", "inline/config/communities", "reference/config/ext-community-set-refs", "reference/config/ext-community-set-ref"} {
		rpolMappedPayloadLeaves["policy-definitions/policy-definition/statements/statement/"+rpolSetExtCommNode+"/"+p] = true
	}
	for _, p := range []string{"as-path-set-name", "config/as-path-set-name", "config/as-path-set-member"} {
		rpolMappedPayloadLeaves["defined-sets/bgp-defined-sets/as-path-sets/as-path-set/"+p] = true
	}
}
