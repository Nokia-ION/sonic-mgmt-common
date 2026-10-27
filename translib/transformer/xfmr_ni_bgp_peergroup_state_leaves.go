package transformer

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// FRR reports a peer-group's remote AS, its type and its activated families
// and nothing else, so the rest of its state is what is configured: these
// state containers mirror their config twins, by the container holding them
// below the peer-group ("" for the peer-group's own state). A nil list
// mirrors every leaf; a list names the leaves mirrored beside those FRR
// reports.
var niBgpPgStateMirror = map[string][]string{
	"":                 {"local-as", "auth-password", "description"},
	"timers":           nil,
	"transport":        nil,
	"ebgp-multihop":    nil,
	"enable-bfd":       nil,
	"graceful-restart": nil,
}

// niBgpPgAfStateMirror is niBgpPgStateMirror below a peer-group afi-safi.
var niBgpPgAfStateMirror = map[string][]string{
	"":                          {"send-community-type"},
	"apply-policy":              nil,
	"ipv4-unicast":              nil,
	"ipv6-unicast":              nil,
	"ipv4-unicast/prefix-limit": nil,
	"ipv6-unicast/prefix-limit": nil,
	"add-paths":                 nil,
}

var niBgpPgStateMirrorReqUriRe = regexp.MustCompile(niBgpPgUriPfx +
	`/(?:(timers|transport|ebgp-multihop|enable-bfd|graceful-restart)/)?state(?:/([a-z-]+))?$`)
var niBgpPgAfStateMirrorReqUriRe = regexp.MustCompile(niBgpPgUriPfx +
	`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(?:(apply-policy|ipv4-unicast/prefix-limit|ipv6-unicast/prefix-limit|ipv4-unicast|ipv6-unicast|add-paths)/)?state(?:/([a-z-]+))?$`)

type niBgpPgStateMirrorReq struct {
	niName, protoName, pg string
	afiSafi               string // CONFIG_DB spelling; set for the afi-safi containers
	af                    bool
	container, leaf       string
	uri                   string // the state container, up to and including "/state"
}

func matchNiBgpPgStateMirrorReq(uri string) (*niBgpPgStateMirrorReq, bool) {
	if m := niBgpPgStateMirrorReqUriRe.FindStringSubmatch(uri); len(m) == 6 {
		return &niBgpPgStateMirrorReq{niName: m[1], protoName: m[2], pg: m[3], container: m[4], leaf: m[5],
			uri: strings.TrimSuffix(strings.TrimSuffix(uri, m[5]), "/")}, true
	}
	if m := niBgpPgAfStateMirrorReqUriRe.FindStringSubmatch(uri); len(m) == 7 {
		afiSafi, ok := normalizeNiBgpPgAfiSafiName(m[4])
		if !ok {
			return nil, false
		}
		return &niBgpPgStateMirrorReq{niName: m[1], protoName: m[2], pg: m[3], afiSafi: afiSafi, af: true, container: m[5], leaf: m[6],
			uri: strings.TrimSuffix(strings.TrimSuffix(uri, m[6]), "/")}, true
	}
	return nil, false
}

// mirrored lists the leaves of the request's container mirrored from config,
// nil for all of them, and reports whether the request names any.
func (r *niBgpPgStateMirrorReq) mirrored() ([]string, bool) {
	spec := niBgpPgStateMirror
	if r.af {
		spec = niBgpPgAfStateMirror
	}
	leaves := spec[r.container]
	if r.leaf == "" || leaves == nil {
		return leaves, true
	}
	for _, l := range leaves {
		if l == r.leaf {
			return leaves, true
		}
	}
	return leaves, false
}

// frrReported reports a request that FRR's view of the peer-group serves: the
// peer-group or afi-safi state container itself, or one of its leaves that
// is not mirrored.
func (r *niBgpPgStateMirrorReq) frrReported() bool {
	if r.container != "" {
		return false
	}
	_, mirrored := r.mirrored()
	return r.leaf == "" || !mirrored
}

// node returns the request's container in the tree below root.
func (r *niBgpPgStateMirrorReq) node(root *ygot.GoStruct, create bool) reflect.Value {
	var obj reflect.Value
	if r.af {
		obj = niBgpPgKind.afObj(root, r.niName, r.protoName, r.pg, r.afiSafi, create)
	} else {
		obj = niBgpPgKind.obj(root, r.niName, r.protoName, r.pg, create)
	}
	if r.container == "" || !obj.IsValid() {
		return obj
	}
	return ocWalkPath(obj, r.container, create)
}

// niBgpCopyCfgLeaves copies the leaves of cfg, a config container, into st,
// the state container field of the same node, allocating it on the first leaf
// copied. leaves, when not nil, and targetLeaf, when set, restrict the copy.
func niBgpCopyCfgLeaves(cfg, st reflect.Value, leaves []string, targetLeaf string) {
	if !cfg.IsValid() || cfg.Kind() != reflect.Ptr || cfg.IsNil() || !st.IsValid() || !st.CanSet() {
		return
	}
	stType := st.Type().Elem()
	for i := 0; i < stType.NumField(); i++ {
		leaf := stType.Field(i).Tag.Get("path")
		if targetLeaf != "" && leaf != targetLeaf {
			continue
		}
		if leaves != nil {
			found := false
			for _, l := range leaves {
				found = found || l == leaf
			}
			if !found {
				continue
			}
		}
		cf := ocFieldByPath(cfg, leaf)
		if !cf.IsValid() || cf.IsZero() || cf.Type() != stType.Field(i).Type {
			continue
		}
		if st.IsNil() {
			st.Set(reflect.New(stType))
		}
		st.Elem().Field(i).Set(cf)
	}
}

// niBgpPgMirrorStateInTree fills the mirrored state of a peer-group, pgObj, and
// of its afi-safis from the config already in the tree, which is what a read
// above the state containers reports.
func niBgpPgMirrorStateInTree(pgObj reflect.Value) {
	if !pgObj.IsValid() || pgObj.IsNil() {
		return
	}
	for c, leaves := range niBgpPgStateMirror {
		node := pgObj
		if c != "" {
			node = ocWalkPath(pgObj, c, false)
		}
		niBgpCopyCfgLeaves(ocFieldByPath(node, "config"), ocFieldByPath(node, "state"), leaves, "")
	}
	afis := ocWalkPath(pgObj, "afi-safis", false)
	if !afis.IsValid() {
		return
	}
	list := ocFieldByPath(afis, "afi-safi")
	if !list.IsValid() || list.Kind() != reflect.Map {
		return
	}
	for _, k := range list.MapKeys() {
		niBgpPgMirrorAfStateInTree(list.MapIndex(k))
	}
}

// niBgpPgMirrorAfStateInTree is niBgpPgMirrorStateInTree for one afi-safi.
func niBgpPgMirrorAfStateInTree(afObj reflect.Value) {
	if !afObj.IsValid() || afObj.IsNil() {
		return
	}
	for c, leaves := range niBgpPgAfStateMirror {
		node := afObj
		if c != "" {
			node = ocWalkPath(afObj, c, false)
		}
		niBgpCopyCfgLeaves(ocFieldByPath(node, "config"), ocFieldByPath(node, "state"), leaves, "")
	}
}

// niBgpPgStateMirrorToYang fills the mirrored leaves of a state request from
// its config twin, read through the peer-group config transformer into a tree
// of its own so that the config it reads stays out of the response.
func niBgpPgStateMirrorToYang(inParams XfmrParams, r *niBgpPgStateMirrorReq) error {
	leaves, _ := r.mirrored()
	twin := strings.TrimSuffix(r.uri, "/state") + "/config"
	var scratch ygot.GoStruct = &ocbinds.Device{}
	cfgParams := inParams
	cfgParams.ygRoot, cfgParams.requestUri, cfgParams.uri = &scratch, twin, twin
	if err := niBgpPeerCfgDbToYang(niBgpPgKind, cfgParams, dbToYangNiBgpPgCfgBase); err != nil {
		if _, missing := err.(tlerr.TranslibRedisClientEntryNotExist); missing {
			return nil
		}
		return err
	}
	cfg := ocFieldByPath(r.node(&scratch, false), "config")
	if !cfg.IsValid() || cfg.IsNil() {
		return nil
	}
	niBgpCopyCfgLeaves(cfg, ocFieldByPath(r.node(inParams.ygRoot, true), "state"), leaves, r.leaf)
	return nil
}

var DbToYang_ni_bgp_pg_state_xfmr SubTreeXfmrDbToYang = dbToYangNiBgpPgState

// dbToYangNiBgpPgState serves peer-group state, the mirrored leaves from
// config and the rest from FRR.
func dbToYangNiBgpPgState(inParams XfmrParams) error {
	r, ok := matchNiBgpPgStateMirrorReq(inParams.requestUri)
	if !ok {
		// A read anchored above the state container reaches this callback
		// with uri naming the container it is called for.
		if r, ok = matchNiBgpPgStateMirrorReq(inParams.uri); ok && r.leaf != "" {
			ok = false
		}
	}
	if !ok {
		return dbToYangNiBgpPgStateBase(inParams)
	}
	if err := validateNiBgpProtoName(r.protoName); err != nil {
		return err
	}
	if _, mirrored := r.mirrored(); mirrored {
		if err := niBgpPgStateMirrorToYang(inParams, r); err != nil {
			return err
		}
	}
	if r.frrReported() {
		return dbToYangNiBgpPgStateBase(inParams)
	}
	return nil
}

// Subscribe_ni_bgp_pg_state_xfmr maps a subscription to a mirrored state
// container, or a mirrored leaf of the state containers FRR reports, onto the
// CONFIG_DB fields its config twin watches; everything else is FRR's.
var Subscribe_ni_bgp_pg_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	if inParams.subscProc != TRANSLATE_SUBSCRIBE || inParams.requestURI == "" {
		return subscribeNiBgpPgStateBase(inParams)
	}
	r, ok := matchNiBgpPgStateMirrorReq(inParams.requestURI)
	if !ok {
		if r, ok = matchNiBgpPgStateMirrorReq(strings.TrimSuffix(inParams.uri, "/")); ok && r.leaf != "" {
			ok = false
		}
	}
	if !ok || r.frrReported() {
		return subscribeNiBgpPgStateBase(inParams)
	}
	node := "/state"
	if r.container != "" {
		node = "/" + r.container + "/state"
	}
	cfgNode := strings.TrimSuffix(node, "/state") + "/config"
	if r.af {
		node, cfgNode = "/afi-safis/afi-safi"+node, "/afi-safis/afi-safi"+cfgNode
	}
	m, held := niBgpPgSubscMaps[cfgNode]
	if !held {
		return subscribeNiBgpPgStateBase(inParams)
	}
	leaves, _ := r.mirrored()
	fields := map[string][]string{}
	for leaf, flds := range m.fields {
		for _, l := range leaves {
			if l == leaf {
				fields[leaf] = flds
			}
		}
		if leaves == nil {
			fields[leaf] = flds
		}
	}
	m.fields = fields
	return niBgpSubscribe(inParams, niBgpPgSubscAnchor, nil, map[string]niBgpSubscMap{node: m}, nil)
}
