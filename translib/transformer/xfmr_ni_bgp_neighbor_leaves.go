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

// niBgpNbrLeafFlds maps the neighbor leaves that are plain BGP_NEIGHBOR fields,
// by the config container holding them below the neighbor. The config,
// timers and transport containers also hold leaves with mappings of their
// own, which the neighbor config transformer serves; ebgp-multihop,
// enable-bfd and graceful-restart are served here alone.
var niBgpNbrLeafFlds = map[string]map[string]string{
	"config": {
		"local-as":      "local_asn",
		"peer-type":     niBgpNbrPeerTypeFld,
		"description":   "name",
		"neighbor-port": "peer_port",
	},
	"timers/config": {
		"minimum-advertisement-interval": "min_adv_interval",
	},
	"transport/config": {
		"tcp-mss": "tcp_mss",
	},
	"ebgp-multihop/config": {
		"enabled":      "ebgp_multihop",
		"multihop-ttl": "ebgp_multihop_ttl",
	},
	"enable-bfd/config": {
		"enabled": "bfd",
	},
	"graceful-restart/config": {
		"enabled": "graceful_restart_enable",
	},
}

// niBgpNbrAfLeafFlds maps the neighbor afi-safi leaves that are plain
// BGP_NEIGHBOR_AF fields, by the config container holding them below the
// afi-safi. add-paths is mapped apart, its two leaves sharing two fields.
var niBgpNbrAfLeafFlds = map[string]map[string]string{
	"ipv4-unicast/config": {"send-default-route": "send_default_route"},
	"ipv6-unicast/config": {"send-default-route": "send_default_route"},
}

const (
	niBgpNbrPeerTypeFld       = "peer_type"
	niBgpNbrTxAddPathsFld     = "tx_add_paths"
	niBgpNbrTxAddPathCountFld = "tx_add_best_path_count"
	niBgpNbrTxAllPaths        = "tx_all_paths"
	niBgpNbrAddPathsCfg       = "add-paths/config"
	// niBgpClearFldPrefix marks a field a write has to remove rather than set,
	// so that one field map can carry both to the code staging the write.
	niBgpClearFldPrefix = "!"
)

// niBgpPeerKind describes one of the two lists of BGP peers, neighbors and
// peer-groups, which share their leaves and the fields behind them, so that
// one mapping serves both.
type niBgpPeerKind struct {
	scope      string // the list entry, as messages name it
	payloadPfx string // the list below bgp, as payload leaf paths name it
	tbl, afTbl string
	// The containers mapped here, as the tables above for neighbors. A
	// container in ownContainers is served here alone; the list's config
	// transformer serves the other leaves of the rest.
	leafFlds      map[string]map[string]string
	ownContainers map[string]bool
	afLeafFlds    map[string]map[string]string
	leafReqRe     *regexp.Regexp
	afLeafReqRe   *regexp.Regexp
	// afFamilyReqRe, when set, matches the family containers above the
	// afi-safi config containers, which then cover every container of
	// afLeafFlds below them.
	afFamilyReqRe *regexp.Regexp
	normalizeAf   func(string) (string, bool)
	obj           func(s *ygot.GoStruct, niName, protoName, peer string, create bool) reflect.Value
	afObj         func(s *ygot.GoStruct, niName, protoName, peer, afiSafi string, create bool) reflect.Value
	resMap        func(niName, peer string, fields map[string]string) map[string]map[string]db.Value
	afResMap      func(niName, peer, afiSafi string, fields map[string]string) map[string]map[string]db.Value
	parentRow     func(inParams XfmrParams, res map[string]map[string]db.Value, niName, peer string) map[string]map[string]db.Value
}

// niBgpPeerLeafContainers renders the container names of leafFlds as the
// alternatives of a request regexp.
func niBgpPeerLeafContainers(leafFlds map[string]map[string]string) string {
	return strings.Join(niBgpSortedKeys(leafFlds), "|")
}

var niBgpNbrKind = &niBgpPeerKind{
	scope:         "neighbor",
	payloadPfx:    "neighbors/neighbor",
	tbl:           niBgpNbrTbl,
	afTbl:         niBgpNbrAfTbl,
	leafFlds:      niBgpNbrLeafFlds,
	ownContainers: map[string]bool{"ebgp-multihop/config": true, "enable-bfd/config": true, "graceful-restart/config": true},
	afLeafFlds:    niBgpNbrAfLeafFlds,
	leafReqRe: regexp.MustCompile(niBgpNbrUriPfx +
		`/(` + niBgpPeerLeafContainers(niBgpNbrLeafFlds) + `)(?:/([a-z-]+))?$`),
	afLeafReqRe: regexp.MustCompile(niBgpNbrUriPfx +
		`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(` + niBgpPeerLeafContainers(niBgpNbrAfLeafFlds) + `|add-paths/config)(?:/([a-z-]+))?$`),
	normalizeAf: normalizeNiBgpNbrAfiSafiName,
	obj: func(s *ygot.GoStruct, niName, protoName, peer string, create bool) reflect.Value {
		return reflect.ValueOf(getNiBgpNbrObj(s, niName, protoName, peer, create))
	},
	afObj: func(s *ygot.GoStruct, niName, protoName, peer, afiSafi string, create bool) reflect.Value {
		return reflect.ValueOf(getNiBgpNbrAfObj(s, niName, protoName, peer, afiSafi, create))
	},
	resMap:    makeNiBgpNbrResMap,
	afResMap:  makeNiBgpNbrAfSingleResMap,
	parentRow: niBgpNbrParentRowIfMissing,
}

type niBgpPeerLeafReq struct {
	kind                    *niBgpPeerKind
	niName, protoName, peer string
	afiSafi                 string // CONFIG_DB spelling, for the afi-safi containers
	container, leaf         string
}

// owned reports a request this file serves alone: a container of its own, an
// afi-safi container, or a leaf of the kind's table in a shared container.
func (r *niBgpPeerLeafReq) owned() bool {
	if r.afiSafi != "" || r.kind.ownContainers[r.container] {
		return true
	}
	_, ok := r.kind.leafFlds[r.container][r.leaf]
	return r.leaf != "" && ok
}

func (k *niBgpPeerKind) matchLeafReq(uri string) (*niBgpPeerLeafReq, bool) {
	if m := k.leafReqRe.FindStringSubmatch(uri); len(m) == 6 {
		return &niBgpPeerLeafReq{kind: k, niName: m[1], protoName: m[2], peer: m[3], container: m[4], leaf: m[5]}, true
	}
	if m := k.afLeafReqRe.FindStringSubmatch(uri); len(m) == 7 {
		afiSafi, ok := k.normalizeAf(m[4])
		if !ok {
			afiSafi = m[4]
		}
		return &niBgpPeerLeafReq{kind: k, niName: m[1], protoName: m[2], peer: m[3], afiSafi: afiSafi, container: m[5], leaf: m[6]}, true
	}
	if k.afFamilyReqRe == nil {
		return nil, false
	}
	if m := k.afFamilyReqRe.FindStringSubmatch(uri); len(m) == 6 {
		afiSafi, ok := k.normalizeAf(m[4])
		if !ok {
			afiSafi = m[4]
		}
		return &niBgpPeerLeafReq{kind: k, niName: m[1], protoName: m[2], peer: m[3], afiSafi: afiSafi, container: m[5]}, true
	}
	return nil, false
}

// afContainersUnder lists the afi-safi containers of the kind's table at or
// below c.
func (k *niBgpPeerKind) afContainersUnder(c string) []string {
	var under []string
	for _, t := range niBgpSortedKeys(k.afLeafFlds) {
		if t == c || strings.HasPrefix(t, c+"/") {
			under = append(under, t)
		}
	}
	return under
}

// ocWalkPath returns the node path names below v, a pointer to a ygot struct,
// by path tags. With create set, missing containers on the way are allocated.
func ocWalkPath(v reflect.Value, path string, create bool) reflect.Value {
	for _, seg := range strings.Split(path, "/") {
		f := ocFieldByPath(v, seg)
		if !f.IsValid() || f.Kind() != reflect.Ptr {
			return reflect.Value{}
		}
		if f.IsNil() {
			if !create || !f.CanSet() {
				return reflect.Value{}
			}
			f.Set(reflect.New(f.Type().Elem()))
		}
		v = f
	}
	return v
}

func niBgpNbrLeafToDb(leaf string, f reflect.Value) (string, bool) {
	if leaf == "peer-type" {
		switch f.Interface().(ocbinds.E_OpenconfigBgp_PeerType) {
		case ocbinds.OpenconfigBgp_PeerType_INTERNAL:
			return "internal", true
		case ocbinds.OpenconfigBgp_PeerType_EXTERNAL:
			return "external", true
		}
		return "", false
	}
	return niBgpGlobalLeafToDb(f)
}

func niBgpNbrLeafFromDb(leaf string, f reflect.Value, raw string) error {
	if leaf == "peer-type" {
		switch strings.TrimSpace(raw) {
		case "internal":
			f.Set(reflect.ValueOf(ocbinds.OpenconfigBgp_PeerType_INTERNAL))
		case "external":
			f.Set(reflect.ValueOf(ocbinds.OpenconfigBgp_PeerType_EXTERNAL))
		}
		return nil
	}
	return niBgpGlobalLeafFromDb(f, raw)
}

// niBgpNbrLeafFields maps the leaves of leafFlds that cfg, a config struct,
// holds. With own set the container has no other mapping, so any other leaf
// it holds is returned apart, by name.
func niBgpNbrLeafFields(cfg reflect.Value, prefix string, leafFlds map[string]string, own bool) (map[string]string, []string) {
	fields := map[string]string{}
	var unmapped []string
	if !cfg.IsValid() || cfg.Kind() != reflect.Ptr || cfg.IsNil() {
		return fields, nil
	}
	v := cfg.Elem()
	for i := 0; i < v.NumField(); i++ {
		leaf := v.Type().Field(i).Tag.Get("path")
		fld, ok := leafFlds[leaf]
		val, set := niBgpNbrLeafToDb(leaf, v.Field(i))
		if !set {
			continue
		}
		if !ok {
			if own {
				unmapped = append(unmapped, prefix+"/"+leaf)
			}
			continue
		}
		fields[fld] = val
	}
	return fields, unmapped
}

// niBgpPeerAddPathsFields maps add-paths config, cfg, onto the two fields
// FRR's alternative addpath modes are stored in: send alone advertises all
// paths, send-max N the N best. Each sets one field and clears the other, and
// send false clears both.
func niBgpPeerAddPathsFields(scope string, cfg reflect.Value) (map[string]string, error) {
	fields := map[string]string{}
	if !cfg.IsValid() || cfg.IsNil() {
		return fields, nil
	}
	if !ocFieldByPath(cfg, "receive").IsNil() || !ocFieldByPath(cfg, "eligible-prefix-policy").IsNil() {
		return nil, tlerr.NotSupportedError{Format: "BGP " + scope + " add-paths receive and eligible-prefix-policy have no CONFIG_DB mapping"}
	}
	send, sendMax := ocFieldByPath(cfg, "send"), ocFieldByPath(cfg, "send-max")
	switch {
	case !sendMax.IsNil():
		if !send.IsNil() && !send.Elem().Bool() {
			return nil, tlerr.InvalidArgsError{Format: "BGP " + scope + " add-paths send-max needs send true"}
		}
		fields[niBgpNbrTxAddPathCountFld] = fmt.Sprintf("%d", sendMax.Elem().Uint())
		fields[niBgpClearFldPrefix+niBgpNbrTxAddPathsFld] = ""
	case !send.IsNil() && send.Elem().Bool():
		fields[niBgpNbrTxAddPathsFld] = niBgpNbrTxAllPaths
		fields[niBgpClearFldPrefix+niBgpNbrTxAddPathCountFld] = ""
	case !send.IsNil():
		fields[niBgpClearFldPrefix+niBgpNbrTxAddPathsFld] = ""
		fields[niBgpClearFldPrefix+niBgpNbrTxAddPathCountFld] = ""
	}
	return fields, nil
}

// niBgpSplitClearFlds separates the fields marked for removal from the rest.
func niBgpSplitClearFlds(fields map[string]string) (map[string]string, map[string]string) {
	set, clear := map[string]string{}, map[string]string{}
	for f, v := range fields {
		if strings.HasPrefix(f, niBgpClearFldPrefix) {
			clear[strings.TrimPrefix(f, niBgpClearFldPrefix)] = ""
			continue
		}
		set[f] = v
	}
	return set, clear
}

// niBgpPeerAfFamilyErr refuses a container of the other family's unicast
// container.
func niBgpPeerAfFamilyErr(scope, container, afiSafi string) error {
	if (strings.HasPrefix(container+"/", "ipv4-unicast/") && afiSafi != niBgpNbrAfiSafiIPv4) ||
		(strings.HasPrefix(container+"/", "ipv6-unicast/") && afiSafi != niBgpNbrAfiSafiIPv6) {
		return tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP %s %s does not belong to afi-safi %s", scope, strings.TrimSuffix(container, "/config"), afiSafi)}
	}
	return nil
}

// niBgpPeerFields maps every leaf of the kind's table that obj, a list entry,
// holds, with the unmapped leaves of the containers served here alone.
func niBgpPeerFields(k *niBgpPeerKind, obj reflect.Value) (map[string]string, []string) {
	fields := map[string]string{}
	var unmapped []string
	for _, c := range niBgpSortedKeys(k.leafFlds) {
		f, u := niBgpNbrLeafFields(ocWalkPath(obj, c, false), k.payloadPfx+"/"+c, k.leafFlds[c], k.ownContainers[c])
		for fld, v := range f {
			fields[fld] = v
		}
		unmapped = append(unmapped, u...)
	}
	return fields, unmapped
}

// niBgpPeerAfFields maps every afi-safi table and add-paths leaf afObj holds,
// clears included.
func niBgpPeerAfFields(k *niBgpPeerKind, afObj reflect.Value, afiSafi string) (map[string]string, error) {
	fields := map[string]string{}
	if !afObj.IsValid() || afObj.IsNil() {
		return fields, nil
	}
	for _, c := range niBgpSortedKeys(k.afLeafFlds) {
		f, unmapped := niBgpNbrLeafFields(ocWalkPath(afObj, c, false), k.payloadPfx+"/afi-safis/afi-safi/"+c, k.afLeafFlds[c], true)
		if len(unmapped) > 0 {
			return nil, tlerr.NotSupportedError{Format: "BGP " + k.scope + " payload carries leaves that have no CONFIG_DB mapping: " + strings.Join(unmapped, ", ")}
		}
		if len(f) > 0 {
			if err := niBgpPeerAfFamilyErr(k.scope, c, afiSafi); err != nil {
				return nil, err
			}
		}
		for fld, val := range f {
			fields[fld] = val
		}
	}
	f, err := niBgpPeerAddPathsFields(k.scope, ocWalkPath(afObj, niBgpNbrAddPathsCfg, false))
	if err != nil {
		return nil, err
	}
	for fld, val := range f {
		fields[fld] = val
	}
	return fields, nil
}

// niBgpHeldFields keeps the fields of rows that CONFIG_DB holds, so that a
// staged removal never names a field or a row that is not there.
func niBgpHeldFields(inParams XfmrParams, rows map[string]map[string]db.Value) map[string]map[string]db.Value {
	held := make(map[string]map[string]db.Value)
	for tbl, keys := range rows {
		for key, val := range keys {
			existing := niBgpCfgDbEntry(inParams, tbl, strings.Split(key, "|")...)
			fields := map[string]string{}
			for f := range val.Field {
				if _, ok := existing.Field[f]; ok {
					fields[f] = ""
				}
			}
			if len(fields) == 0 {
				continue
			}
			if held[tbl] == nil {
				held[tbl] = make(map[string]db.Value)
			}
			held[tbl][key] = db.Value{Field: fields}
		}
	}
	return held
}

func niBgpSortedKeys(m map[string]map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// niBgpPeerRemoteAsErr refuses a neighbor or peer-group holding both peer-as
// and peer-type: frrcfgd renders each as "neighbor X remote-as", so the two
// would contend. fields is the row a write produces, existing what CONFIG_DB
// holds.
func niBgpPeerRemoteAsErr(scope string, fields map[string]string, existing db.Value) error {
	effective := func(fld string) string {
		if v, ok := fields[fld]; ok {
			return v
		}
		return existing.Field[fld]
	}
	if strings.TrimSpace(effective(niBgpNbrAsnFld)) != "" && strings.TrimSpace(effective(niBgpNbrPeerTypeFld)) != "" {
		return tlerr.InvalidArgsError{Format: "BGP " + scope + " peer-as and peer-type are alternatives: FRR takes one of them as remote-as; delete one before setting the other"}
	}
	return nil
}

// niBgpPeerLeafYangToDb maps a SET or a DELETE of a niBgpPeerLeafReq. For a
// shared container at container level it maps only this file's leaves, and
// a payload carrying none of them maps nothing.
func niBgpPeerLeafYangToDb(inParams XfmrParams, r *niBgpPeerLeafReq) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if r.afiSafi != "" {
		return niBgpPeerAfLeafYangToDb(inParams, r)
	}
	k := r.kind
	leafFlds := k.leafFlds[r.container]
	own := k.ownContainers[r.container]
	if r.leaf != "" {
		if _, ok := leafFlds[r.leaf]; !ok {
			return res, tlerr.NotSupportedError{Format: fmt.Sprintf("BGP %s %s leaf %s has no CONFIG_DB mapping", k.scope, r.container, r.leaf), Path: inParams.requestUri}
		}
	}
	if inParams.oper == DELETE {
		fields := map[string]string{}
		for leaf, fld := range leafFlds {
			if r.leaf == "" || r.leaf == leaf {
				fields[fld] = ""
			}
		}
		return k.resMap(r.niName, r.peer, fields), nil
	}
	cfg := ocWalkPath(k.obj(inParams.ygRoot, r.niName, r.protoName, r.peer, false), r.container, false)
	fields, unmapped := niBgpNbrLeafFields(cfg, k.payloadPfx+"/"+r.container, leafFlds, own)
	if len(unmapped) > 0 {
		return res, tlerr.NotSupportedError{Format: "BGP " + k.scope + " payload carries leaves that have no CONFIG_DB mapping: " + strings.Join(unmapped, ", "), Path: inParams.requestUri}
	}
	if r.leaf != "" {
		val, ok := fields[leafFlds[r.leaf]]
		if !ok {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP %s %s %s is missing in request payload", k.scope, r.container, r.leaf)}
		}
		fields = map[string]string{leafFlds[r.leaf]: val}
	}
	if len(fields) == 0 {
		if own {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP %s %s payload carries no supported leaf", k.scope, r.container)}
		}
		return res, nil
	}
	return k.resMap(r.niName, r.peer, fields), nil
}

func niBgpPeerAfLeafYangToDb(inParams XfmrParams, r *niBgpPeerLeafReq) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	k := r.kind
	if err := niBgpPeerAfFamilyErr(k.scope, r.container, r.afiSafi); err != nil {
		return res, err
	}
	leafFlds := map[string]string{}
	if r.container == niBgpNbrAddPathsCfg {
		leafFlds = map[string]string{"send": niBgpNbrTxAddPathsFld, "send-max": niBgpNbrTxAddPathCountFld}
	} else {
		for _, c := range k.afContainersUnder(r.container) {
			for leaf, fld := range k.afLeafFlds[c] {
				leafFlds[leaf] = fld
			}
		}
	}
	if r.leaf != "" {
		if _, ok := leafFlds[r.leaf]; !ok {
			return res, tlerr.NotSupportedError{Format: fmt.Sprintf("BGP %s %s leaf %s has no CONFIG_DB mapping", k.scope, r.container, r.leaf), Path: inParams.requestUri}
		}
	}
	if inParams.oper == DELETE {
		fields := map[string]string{}
		for leaf, fld := range leafFlds {
			// Without send the count would still select a mode, so deleting
			// send removes both.
			if r.leaf == "" || r.leaf == leaf || (r.leaf == "send" && fld == niBgpNbrTxAddPathCountFld) {
				fields[fld] = ""
			}
		}
		return k.afResMap(r.niName, r.peer, r.afiSafi, fields), nil
	}
	cfg := ocWalkPath(k.afObj(inParams.ygRoot, r.niName, r.protoName, r.peer, r.afiSafi, false), r.container, false)
	var fields map[string]string
	if r.container == niBgpNbrAddPathsCfg {
		var err error
		if fields, err = niBgpPeerAddPathsFields(k.scope, cfg); err != nil {
			return res, err
		}
	} else {
		fields = map[string]string{}
		afObj := k.afObj(inParams.ygRoot, r.niName, r.protoName, r.peer, r.afiSafi, false)
		for _, c := range k.afContainersUnder(r.container) {
			f, unmapped := niBgpNbrLeafFields(ocWalkPath(afObj, c, false), k.payloadPfx+"/afi-safis/afi-safi/"+c, k.afLeafFlds[c], true)
			if len(unmapped) > 0 {
				return res, tlerr.NotSupportedError{Format: "BGP " + k.scope + " payload carries leaves that have no CONFIG_DB mapping: " + strings.Join(unmapped, ", "), Path: inParams.requestUri}
			}
			for fld, v := range f {
				fields[fld] = v
			}
		}
	}
	if r.leaf != "" && r.container != niBgpNbrAddPathsCfg {
		val, ok := fields[leafFlds[r.leaf]]
		if !ok {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP %s %s %s is missing in request payload", k.scope, r.container, r.leaf)}
		}
		fields = map[string]string{leafFlds[r.leaf]: val}
	}
	if len(fields) == 0 {
		return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP %s %s payload carries no supported leaf", k.scope, r.container)}
	}
	set, clear := niBgpSplitClearFlds(fields)
	existing := niBgpCfgDbEntry(inParams, k.afTbl, r.niName, r.peer, r.afiSafi)
	stale := map[string]string{}
	for f := range clear {
		if _, ok := existing.Field[f]; ok {
			stale[f] = ""
		}
	}
	if len(stale) > 0 {
		if err := niBgpStageSubOp(inParams, DELETE, k.afResMap(r.niName, r.peer, r.afiSafi, stale)); err != nil {
			return res, err
		}
	}
	if len(set) == 0 {
		return k.parentRow(inParams, res, r.niName, r.peer), nil
	}
	return k.parentRow(inParams, k.afResMap(r.niName, r.peer, r.afiSafi, set), r.niName, r.peer), nil
}

// niBgpPeerLeavesToYang fills the leaves of the kind's table for one list
// entry from its row, in container only when one is named.
func niBgpPeerLeavesToYang(k *niBgpPeerKind, ygRoot *ygot.GoStruct, niName, protoName, peer, container, targetLeaf string, entry db.Value) error {
	for _, c := range niBgpSortedKeys(k.leafFlds) {
		if container != "" && c != container {
			continue
		}
		var held []string
		for leaf, fld := range k.leafFlds[c] {
			if (targetLeaf == "" || targetLeaf == leaf) && strings.TrimSpace(entry.Field[fld]) != "" {
				held = append(held, leaf)
			}
		}
		if len(held) == 0 {
			continue
		}
		cfg := ocWalkPath(k.obj(ygRoot, niName, protoName, peer, true), c, true)
		if !cfg.IsValid() {
			continue
		}
		for _, leaf := range held {
			if err := niBgpNbrLeafFromDb(leaf, ocFieldByPath(cfg, leaf), entry.Field[k.leafFlds[c][leaf]]); err != nil {
				return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid %s value for %s %s: %v", k.tbl, c, leaf, err)}
			}
		}
	}
	return nil
}

// niBgpPeerAfLeavesToYang fills the afi-safi table and add-paths leaves of one
// list entry's afi-safi from its row, in container only when one is named.
func niBgpPeerAfLeavesToYang(k *niBgpPeerKind, ygRoot *ygot.GoStruct, niName, protoName, peer, afiSafi, container, targetLeaf string, entry db.Value) error {
	for _, c := range niBgpSortedKeys(k.afLeafFlds) {
		if (container != "" && c != container && !strings.HasPrefix(c, container+"/")) || niBgpPeerAfFamilyErr(k.scope, c, afiSafi) != nil {
			continue
		}
		for leaf, fld := range k.afLeafFlds[c] {
			if (targetLeaf != "" && targetLeaf != leaf) || strings.TrimSpace(entry.Field[fld]) == "" {
				continue
			}
			cfg := ocWalkPath(k.afObj(ygRoot, niName, protoName, peer, afiSafi, true), c, true)
			if err := niBgpNbrLeafFromDb(leaf, ocFieldByPath(cfg, leaf), entry.Field[fld]); err != nil {
				return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid %s value for %s %s: %v", k.afTbl, c, leaf, err)}
			}
		}
	}
	if container != "" && container != niBgpNbrAddPathsCfg {
		return nil
	}
	mode := strings.TrimSpace(entry.Field[niBgpNbrTxAddPathsFld])
	count := strings.TrimSpace(entry.Field[niBgpNbrTxAddPathCountFld])
	if mode == "" && count == "" {
		return nil
	}
	cfg := ocWalkPath(k.afObj(ygRoot, niName, protoName, peer, afiSafi, true), niBgpNbrAddPathsCfg, true)
	if !cfg.IsValid() {
		return nil
	}
	// Every addpath mode advertises more than the best path, so each reads as
	// send true; only the count of the best paths has a leaf of its own.
	if targetLeaf == "" || targetLeaf == "send" {
		if err := niBgpGlobalLeafFromDb(ocFieldByPath(cfg, "send"), "true"); err != nil {
			return err
		}
	}
	if count != "" && (targetLeaf == "" || targetLeaf == "send-max") {
		if err := niBgpGlobalLeafFromDb(ocFieldByPath(cfg, "send-max"), count); err != nil {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid %s tx_add_best_path_count %q: %v", k.afTbl, count, err)}
		}
	}
	return nil
}

// niBgpPeerLeafDbToYang serves a GET of a niBgpPeerLeafReq.
func niBgpPeerLeafDbToYang(inParams XfmrParams, r *niBgpPeerLeafReq) error {
	k := r.kind
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP " + k.scope + " config"}
	}
	if r.afiSafi != "" {
		if err := niBgpPeerAfFamilyErr(k.scope, r.container, r.afiSafi); err != nil {
			return err
		}
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: k.afTbl}, db.Key{Comp: []string{r.niName, r.peer, r.afiSafi}})
		if err != nil {
			return err
		}
		return niBgpPeerAfLeavesToYang(k, inParams.ygRoot, r.niName, r.protoName, r.peer, r.afiSafi, r.container, r.leaf, entry)
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: k.tbl}, db.Key{Comp: []string{r.niName, r.peer}})
	if err != nil {
		return err
	}
	return niBgpPeerLeavesToYang(k, inParams.ygRoot, r.niName, r.protoName, r.peer, r.container, r.leaf, entry)
}

// niBgpPeerPayloadHasLeaves reports a shared container payload carrying a leaf
// of the kind's table, which the list's config transformer then maps
// alongside its own.
func niBgpPeerPayloadHasLeaves(k *niBgpPeerKind, inParams XfmrParams, niName, protoName, peer, container string) bool {
	cfg := ocWalkPath(k.obj(inParams.ygRoot, niName, protoName, peer, false), container, false)
	fields, _ := niBgpNbrLeafFields(cfg, "", k.leafFlds[container], false)
	return len(fields) > 0
}

// niBgpPeerCfgYangToDb serves a write to a config container of the list, the
// leaves of the kind's table here and the rest by base, the list's own config
// transformer.
func niBgpPeerCfgYangToDb(k *niBgpPeerKind, inParams XfmrParams, base SubTreeXfmrYangToDb) (map[string]map[string]db.Value, error) {
	r, ok := k.matchLeafReq(niHandlerReqUri(inParams))
	if ok {
		if err := validateNiBgpProtoName(r.protoName); err != nil {
			return make(map[string]map[string]db.Value), err
		}
	}
	var res map[string]map[string]db.Value
	var err error
	if ok && r.owned() {
		res, err = niBgpPeerLeafYangToDb(inParams, r)
	} else {
		res, err = base(inParams)
		if err == nil && ok && r.leaf == "" {
			var extra map[string]map[string]db.Value
			if extra, err = niBgpPeerLeafYangToDb(inParams, r); err == nil {
				res = mergeDbResMapFields(res, extra)
			}
		}
	}
	if err != nil || !ok || inParams.oper == DELETE || r.container != "config" {
		return res, err
	}
	if row, held := res[k.tbl][r.niName+"|"+r.peer]; held {
		if rerr := niBgpPeerRemoteAsErr(k.scope, row.Field, niBgpCfgDbEntry(inParams, k.tbl, r.niName, r.peer)); rerr != nil {
			return make(map[string]map[string]db.Value), rerr
		}
	}
	return res, nil
}

// niBgpPeerCfgDbToYang serves a read of a config container of the list, the
// way niBgpPeerCfgYangToDb splits a write.
func niBgpPeerCfgDbToYang(k *niBgpPeerKind, inParams XfmrParams, base SubTreeXfmrDbToYang) error {
	r, ok := k.matchLeafReq(inParams.requestUri)
	if ok {
		if err := validateNiBgpProtoName(r.protoName); err != nil {
			return err
		}
		if r.owned() {
			return niBgpPeerLeafDbToYang(inParams, r)
		}
	}
	if err := base(inParams); err != nil {
		return err
	}
	if ok && r.leaf == "" {
		return niBgpPeerLeafDbToYang(inParams, r)
	}
	return nil
}

var YangToDb_ni_bgp_nbr_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	return niBgpPeerCfgYangToDb(niBgpNbrKind, inParams, yangToDbNiBgpNbrCfgBase)
}

var DbToYang_ni_bgp_nbr_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	return niBgpPeerCfgDbToYang(niBgpNbrKind, inParams, dbToYangNiBgpNbrCfgBase)
}

// registerSubsc describes the kind's containers to the subscribe mapping:
// cfgMaps holds the list's nodes, below the list, and listVirtual the
// containers above the config containers, below bgp. The payload allow list
// gains the leaves too.
func (k *niBgpPeerKind) registerSubsc(cfgMaps map[string]niBgpSubscMap, afVirtual map[string]bool, key, afKey niBgpSubscKey) {
	for c, leaves := range k.leafFlds {
		node := "/" + c
		m, ok := cfgMaps[node]
		if !ok {
			m = niBgpSubscMap{dbNum: db.ConfigDB, table: k.tbl, key: key, fields: map[string][]string{}}
			cfgMaps[node] = m
			niBgpGlobalSubscVirtual["/bgp/"+k.payloadPfx+"/"+strings.TrimSuffix(c, "/config")] = true
		}
		for leaf, fld := range leaves {
			m.fields[leaf] = []string{fld}
			niBgpMappedPayloadLeaves[k.payloadPfx+"/"+c+"/"+leaf] = true
		}
	}
	afNodes := map[string]map[string][]string{
		"/afi-safis/afi-safi/" + niBgpNbrAddPathsCfg: {"send": {niBgpNbrTxAddPathsFld, niBgpNbrTxAddPathCountFld}, "send-max": {niBgpNbrTxAddPathCountFld}},
	}
	for c, leaves := range k.afLeafFlds {
		afNodes["/afi-safis/afi-safi/"+c] = map[string][]string{}
		for leaf, fld := range leaves {
			afNodes["/afi-safis/afi-safi/"+c][leaf] = []string{fld}
		}
	}
	for node, flds := range afNodes {
		m, ok := cfgMaps[node]
		if !ok {
			m = niBgpSubscMap{dbNum: db.ConfigDB, table: k.afTbl, key: afKey, fields: map[string][]string{}}
			cfgMaps[node] = m
		}
		for leaf, f := range flds {
			m.fields[leaf] = f
			niBgpMappedPayloadLeaves[k.payloadPfx+node+"/"+leaf] = true
		}
		for parent := strings.TrimSuffix(node, "/config"); parent != "/afi-safis/afi-safi"; parent = parent[:strings.LastIndex(parent, "/")] {
			afVirtual[parent] = true
		}
	}
}

// afInstOwnedFlds lists the afi-safi row fields the kind's afi-safi
// containers map.
func (k *niBgpPeerKind) afInstOwnedFlds() []string {
	flds := []string{niBgpNbrTxAddPathsFld, niBgpNbrTxAddPathCountFld}
	for _, leaves := range k.afLeafFlds {
		for _, fld := range leaves {
			flds = append(flds, fld)
		}
	}
	return flds
}

func init() {
	niBgpNbrKind.registerSubsc(niBgpNbrCfgSubscMaps, niBgpNbrCfgSubscVirtual, niBgpSubscKeyNiNbr, niBgpSubscKeyNiNbrAf)
	for _, f := range niBgpNbrKind.afInstOwnedFlds() {
		niBgpNbrAfInstOwnedFlds[f] = true
	}
}
