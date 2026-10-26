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
// own, which the neighbor config transformer serves; ebgp-multihop and
// enable-bfd are served here alone.
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

// The containers served here alone, as opposed to those whose other leaves
// the neighbor config transformer maps.
var niBgpNbrLeafOwnContainers = map[string]bool{"ebgp-multihop/config": true, "enable-bfd/config": true}

var niBgpNbrLeafReqUriRe = regexp.MustCompile(niBgpNbrUriPfx +
	`/(config|timers/config|transport/config|ebgp-multihop/config|enable-bfd/config)(?:/([a-z-]+))?$`)
var niBgpNbrAfLeafReqUriRe = regexp.MustCompile(niBgpNbrUriPfx +
	`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(ipv4-unicast/config|ipv6-unicast/config|add-paths/config)(?:/([a-z-]+))?$`)

type niBgpNbrLeafReq struct {
	niName, protoName, neighborAddr string
	afiSafi                         string // CONFIG_DB spelling, for the afi-safi containers
	container, leaf                 string
}

// owned reports a request this file serves alone: a container of its own, an
// afi-safi container, or a leaf of niBgpNbrLeafFlds in a shared container.
func (r *niBgpNbrLeafReq) owned() bool {
	if r.afiSafi != "" || niBgpNbrLeafOwnContainers[r.container] {
		return true
	}
	_, ok := niBgpNbrLeafFlds[r.container][r.leaf]
	return r.leaf != "" && ok
}

func matchNiBgpNbrLeafReq(uri string) (*niBgpNbrLeafReq, bool) {
	if m := niBgpNbrLeafReqUriRe.FindStringSubmatch(uri); len(m) == 6 {
		return &niBgpNbrLeafReq{niName: m[1], protoName: m[2], neighborAddr: m[3], container: m[4], leaf: m[5]}, true
	}
	if m := niBgpNbrAfLeafReqUriRe.FindStringSubmatch(uri); len(m) == 7 {
		afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
		if !ok {
			afiSafi = m[4]
		}
		return &niBgpNbrLeafReq{niName: m[1], protoName: m[2], neighborAddr: m[3], afiSafi: afiSafi, container: m[5], leaf: m[6]}, true
	}
	return nil, false
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

// niBgpNbrAddPathsFields maps add-paths config onto the two fields FRR's
// alternative addpath modes are stored in: send alone advertises all paths,
// send-max N the N best. Each sets one field and clears the other, and send
// false clears both.
func niBgpNbrAddPathsFields(cfg *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_AddPaths_Config) (map[string]string, error) {
	fields := map[string]string{}
	if cfg == nil {
		return fields, nil
	}
	if cfg.Receive != nil || cfg.EligiblePrefixPolicy != nil {
		return nil, tlerr.NotSupportedError{Format: "BGP neighbor add-paths receive and eligible-prefix-policy have no CONFIG_DB mapping"}
	}
	switch {
	case cfg.SendMax != nil:
		if cfg.Send != nil && !*cfg.Send {
			return nil, tlerr.InvalidArgsError{Format: "BGP neighbor add-paths send-max needs send true"}
		}
		fields[niBgpNbrTxAddPathCountFld] = fmt.Sprintf("%d", *cfg.SendMax)
		fields[niBgpClearFldPrefix+niBgpNbrTxAddPathsFld] = ""
	case cfg.Send != nil && *cfg.Send:
		fields[niBgpNbrTxAddPathsFld] = niBgpNbrTxAllPaths
		fields[niBgpClearFldPrefix+niBgpNbrTxAddPathCountFld] = ""
	case cfg.Send != nil:
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

// niBgpNbrAfLeafFamilyErr refuses the unicast container of the other family.
func niBgpNbrAfLeafFamilyErr(container, afiSafi string) error {
	if (container == "ipv4-unicast/config" && afiSafi != niBgpNbrAfiSafiIPv4) ||
		(container == "ipv6-unicast/config" && afiSafi != niBgpNbrAfiSafiIPv6) {
		return tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP neighbor %s does not belong to afi-safi %s", strings.TrimSuffix(container, "/config"), afiSafi)}
	}
	return nil
}

// niBgpNbrNeighborFields maps every niBgpNbrLeafFlds leaf nbrObj holds, with
// the unmapped leaves of the containers served here alone.
func niBgpNbrNeighborFields(nbrObj reflect.Value) (map[string]string, []string) {
	fields := map[string]string{}
	var unmapped []string
	for _, c := range niBgpSortedKeys(niBgpNbrLeafFlds) {
		f, u := niBgpNbrLeafFields(ocWalkPath(nbrObj, c, false), "neighbors/neighbor/"+c, niBgpNbrLeafFlds[c], niBgpNbrLeafOwnContainers[c])
		for k, v := range f {
			fields[k] = v
		}
		unmapped = append(unmapped, u...)
	}
	return fields, unmapped
}

// niBgpNbrAfFields maps every niBgpNbrAfLeafFlds and add-paths leaf afObj
// holds, clears included.
func niBgpNbrAfFields(afObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi, afiSafi string) (map[string]string, error) {
	fields := map[string]string{}
	if afObj == nil {
		return fields, nil
	}
	v := reflect.ValueOf(afObj)
	for _, c := range niBgpSortedKeys(niBgpNbrAfLeafFlds) {
		f, unmapped := niBgpNbrLeafFields(ocWalkPath(v, c, false), "neighbors/neighbor/afi-safis/afi-safi/"+c, niBgpNbrAfLeafFlds[c], true)
		if len(unmapped) > 0 {
			return nil, tlerr.NotSupportedError{Format: "BGP neighbor payload carries leaves that have no CONFIG_DB mapping: " + strings.Join(unmapped, ", ")}
		}
		if len(f) > 0 {
			if err := niBgpNbrAfLeafFamilyErr(c, afiSafi); err != nil {
				return nil, err
			}
		}
		for k, val := range f {
			fields[k] = val
		}
	}
	if afObj.AddPaths != nil {
		f, err := niBgpNbrAddPathsFields(afObj.AddPaths.Config)
		if err != nil {
			return nil, err
		}
		for k, val := range f {
			fields[k] = val
		}
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

// niBgpNbrRemoteAsErr refuses a neighbor holding both peer-as and peer-type:
// frrcfgd renders each as "neighbor X remote-as", so the two would contend.
// fields is the row a write produces, existing what CONFIG_DB holds.
func niBgpNbrRemoteAsErr(fields map[string]string, existing db.Value) error {
	effective := func(fld string) string {
		if v, ok := fields[fld]; ok {
			return v
		}
		return existing.Field[fld]
	}
	if strings.TrimSpace(effective(niBgpNbrAsnFld)) != "" && strings.TrimSpace(effective(niBgpNbrPeerTypeFld)) != "" {
		return tlerr.InvalidArgsError{Format: "BGP neighbor peer-as and peer-type are alternatives: FRR takes one of them as remote-as; delete one before setting the other"}
	}
	return nil
}

// niBgpNbrLeafYangToDb maps a SET or a DELETE of a niBgpNbrLeafReq. For a
// shared container at container level it maps only this file's leaves, and
// a payload carrying none of them maps nothing.
func niBgpNbrLeafYangToDb(inParams XfmrParams, r *niBgpNbrLeafReq) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if r.afiSafi != "" {
		return niBgpNbrAfLeafYangToDb(inParams, r)
	}
	leafFlds := niBgpNbrLeafFlds[r.container]
	own := niBgpNbrLeafOwnContainers[r.container]
	if r.leaf != "" {
		if _, ok := leafFlds[r.leaf]; !ok {
			return res, tlerr.NotSupportedError{Format: fmt.Sprintf("BGP neighbor %s leaf %s has no CONFIG_DB mapping", r.container, r.leaf), Path: inParams.requestUri}
		}
	}
	if inParams.oper == DELETE {
		fields := map[string]string{}
		for leaf, fld := range leafFlds {
			if r.leaf == "" || r.leaf == leaf {
				fields[fld] = ""
			}
		}
		return makeNiBgpNbrResMap(r.niName, r.neighborAddr, fields), nil
	}
	nbrObj := reflect.ValueOf(getNiBgpNbrObj(inParams.ygRoot, r.niName, r.protoName, r.neighborAddr, false))
	cfg := ocWalkPath(nbrObj, r.container, false)
	fields, unmapped := niBgpNbrLeafFields(cfg, "neighbors/neighbor/"+r.container, leafFlds, own)
	if len(unmapped) > 0 {
		return res, tlerr.NotSupportedError{Format: "BGP neighbor payload carries leaves that have no CONFIG_DB mapping: " + strings.Join(unmapped, ", "), Path: inParams.requestUri}
	}
	if r.leaf != "" {
		val, ok := fields[leafFlds[r.leaf]]
		if !ok {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP neighbor %s %s is missing in request payload", r.container, r.leaf)}
		}
		fields = map[string]string{leafFlds[r.leaf]: val}
	}
	if len(fields) == 0 {
		if own {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP neighbor %s payload carries no supported leaf", r.container)}
		}
		return res, nil
	}
	return makeNiBgpNbrResMap(r.niName, r.neighborAddr, fields), nil
}

func niBgpNbrAfLeafYangToDb(inParams XfmrParams, r *niBgpNbrLeafReq) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if err := niBgpNbrAfLeafFamilyErr(r.container, r.afiSafi); err != nil {
		return res, err
	}
	var leafFlds map[string]string
	if r.container == niBgpNbrAddPathsCfg {
		leafFlds = map[string]string{"send": niBgpNbrTxAddPathsFld, "send-max": niBgpNbrTxAddPathCountFld}
	} else {
		leafFlds = niBgpNbrAfLeafFlds[r.container]
	}
	if r.leaf != "" {
		if _, ok := leafFlds[r.leaf]; !ok {
			return res, tlerr.NotSupportedError{Format: fmt.Sprintf("BGP neighbor %s leaf %s has no CONFIG_DB mapping", r.container, r.leaf), Path: inParams.requestUri}
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
		return makeNiBgpNbrAfSingleResMap(r.niName, r.neighborAddr, r.afiSafi, fields), nil
	}
	afObj := getNiBgpNbrAfObj(inParams.ygRoot, r.niName, r.protoName, r.neighborAddr, r.afiSafi, false)
	var fields map[string]string
	if r.container == niBgpNbrAddPathsCfg {
		var cfg *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_AddPaths_Config
		if afObj != nil && afObj.AddPaths != nil {
			cfg = afObj.AddPaths.Config
		}
		var err error
		if fields, err = niBgpNbrAddPathsFields(cfg); err != nil {
			return res, err
		}
	} else {
		var unmapped []string
		fields, unmapped = niBgpNbrLeafFields(ocWalkPath(reflect.ValueOf(afObj), r.container, false), "neighbors/neighbor/afi-safis/afi-safi/"+r.container, leafFlds, true)
		if len(unmapped) > 0 {
			return res, tlerr.NotSupportedError{Format: "BGP neighbor payload carries leaves that have no CONFIG_DB mapping: " + strings.Join(unmapped, ", "), Path: inParams.requestUri}
		}
	}
	if len(fields) == 0 {
		return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP neighbor %s payload carries no supported leaf", r.container)}
	}
	set, clear := niBgpSplitClearFlds(fields)
	existing := niBgpCfgDbEntry(inParams, niBgpNbrAfTbl, r.niName, r.neighborAddr, r.afiSafi)
	stale := map[string]string{}
	for f := range clear {
		if _, ok := existing.Field[f]; ok {
			stale[f] = ""
		}
	}
	if len(stale) > 0 {
		if err := niBgpStageSubOp(inParams, DELETE, makeNiBgpNbrAfSingleResMap(r.niName, r.neighborAddr, r.afiSafi, stale)); err != nil {
			return res, err
		}
	}
	if len(set) == 0 {
		return niBgpNbrParentRowIfMissing(inParams, res, r.niName, r.neighborAddr), nil
	}
	return niBgpNbrParentRowIfMissing(inParams, makeNiBgpNbrAfSingleResMap(r.niName, r.neighborAddr, r.afiSafi, set), r.niName, r.neighborAddr), nil
}

// niBgpNbrLeavesToYang fills the niBgpNbrLeafFlds leaves of a neighbor from
// its BGP_NEIGHBOR row, in container only when one is named.
func niBgpNbrLeavesToYang(ygRoot *ygot.GoStruct, niName, protoName, neighborAddr, container, targetLeaf string, entry db.Value) error {
	for _, c := range niBgpSortedKeys(niBgpNbrLeafFlds) {
		if container != "" && c != container {
			continue
		}
		var held []string
		for leaf, fld := range niBgpNbrLeafFlds[c] {
			if (targetLeaf == "" || targetLeaf == leaf) && strings.TrimSpace(entry.Field[fld]) != "" {
				held = append(held, leaf)
			}
		}
		if len(held) == 0 {
			continue
		}
		cfg := ocWalkPath(reflect.ValueOf(getNiBgpNbrObj(ygRoot, niName, protoName, neighborAddr, true)), c, true)
		if !cfg.IsValid() {
			continue
		}
		for _, leaf := range held {
			if err := niBgpNbrLeafFromDb(leaf, ocFieldByPath(cfg, leaf), entry.Field[niBgpNbrLeafFlds[c][leaf]]); err != nil {
				return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR value for %s %s: %v", c, leaf, err)}
			}
		}
	}
	return nil
}

// niBgpNbrAfLeavesToYang fills the niBgpNbrAfLeafFlds and add-paths leaves of
// a neighbor afi-safi from its BGP_NEIGHBOR_AF row, in container only when
// one is named.
func niBgpNbrAfLeavesToYang(ygRoot *ygot.GoStruct, niName, protoName, neighborAddr, afiSafi, container, targetLeaf string, entry db.Value) error {
	for _, c := range niBgpSortedKeys(niBgpNbrAfLeafFlds) {
		if (container != "" && c != container) || niBgpNbrAfLeafFamilyErr(c, afiSafi) != nil {
			continue
		}
		for leaf, fld := range niBgpNbrAfLeafFlds[c] {
			if (targetLeaf != "" && targetLeaf != leaf) || strings.TrimSpace(entry.Field[fld]) == "" {
				continue
			}
			cfg := ocWalkPath(reflect.ValueOf(getNiBgpNbrAfObj(ygRoot, niName, protoName, neighborAddr, afiSafi, true)), c, true)
			if err := niBgpNbrLeafFromDb(leaf, ocFieldByPath(cfg, leaf), entry.Field[fld]); err != nil {
				return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR_AF value for %s %s: %v", c, leaf, err)}
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
	cfg := ocWalkPath(reflect.ValueOf(getNiBgpNbrAfObj(ygRoot, niName, protoName, neighborAddr, afiSafi, true)), niBgpNbrAddPathsCfg, true)
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
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_NEIGHBOR_AF tx_add_best_path_count %q: %v", count, err)}
		}
	}
	return nil
}

// niBgpNbrLeafDbToYang serves a GET of a niBgpNbrLeafReq.
func niBgpNbrLeafDbToYang(inParams XfmrParams, r *niBgpNbrLeafReq) error {
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP neighbor config"}
	}
	if r.afiSafi != "" {
		if err := niBgpNbrAfLeafFamilyErr(r.container, r.afiSafi); err != nil {
			return err
		}
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{r.niName, r.neighborAddr, r.afiSafi}})
		if err != nil {
			return err
		}
		return niBgpNbrAfLeavesToYang(inParams.ygRoot, r.niName, r.protoName, r.neighborAddr, r.afiSafi, r.container, r.leaf, entry)
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrTbl}, db.Key{Comp: []string{r.niName, r.neighborAddr}})
	if err != nil {
		return err
	}
	return niBgpNbrLeavesToYang(inParams.ygRoot, r.niName, r.protoName, r.neighborAddr, r.container, r.leaf, entry)
}

// niBgpNbrPayloadHasLeaves reports a shared container payload carrying a
// niBgpNbrLeafFlds leaf, which the neighbor config transformer then maps
// alongside its own.
func niBgpNbrPayloadHasLeaves(inParams XfmrParams, niName, protoName, neighborAddr, container string) bool {
	nbrObj := reflect.ValueOf(getNiBgpNbrObj(inParams.ygRoot, niName, protoName, neighborAddr, false))
	fields, _ := niBgpNbrLeafFields(ocWalkPath(nbrObj, container, false), "", niBgpNbrLeafFlds[container], false)
	return len(fields) > 0
}

var YangToDb_ni_bgp_nbr_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	r, ok := matchNiBgpNbrLeafReq(niHandlerReqUri(inParams))
	if ok {
		if err := validateNiBgpProtoName(r.protoName); err != nil {
			return make(map[string]map[string]db.Value), err
		}
	}
	var res map[string]map[string]db.Value
	var err error
	if ok && r.owned() {
		res, err = niBgpNbrLeafYangToDb(inParams, r)
	} else {
		res, err = yangToDbNiBgpNbrCfgBase(inParams)
		if err == nil && ok && r.leaf == "" {
			var extra map[string]map[string]db.Value
			if extra, err = niBgpNbrLeafYangToDb(inParams, r); err == nil {
				res = mergeDbResMapFields(res, extra)
			}
		}
	}
	if err != nil || !ok || inParams.oper == DELETE || r.container != "config" {
		return res, err
	}
	if row, held := res[niBgpNbrTbl][r.niName+"|"+r.neighborAddr]; held {
		if rerr := niBgpNbrRemoteAsErr(row.Field, niBgpCfgDbEntry(inParams, niBgpNbrTbl, r.niName, r.neighborAddr)); rerr != nil {
			return make(map[string]map[string]db.Value), rerr
		}
	}
	return res, nil
}

var DbToYang_ni_bgp_nbr_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	r, ok := matchNiBgpNbrLeafReq(inParams.requestUri)
	if ok {
		if err := validateNiBgpProtoName(r.protoName); err != nil {
			return err
		}
		if r.owned() {
			return niBgpNbrLeafDbToYang(inParams, r)
		}
	}
	if err := dbToYangNiBgpNbrCfgBase(inParams); err != nil {
		return err
	}
	if ok && r.leaf == "" {
		return niBgpNbrLeafDbToYang(inParams, r)
	}
	return nil
}

func init() {
	for c, leaves := range niBgpNbrLeafFlds {
		node := "/" + c
		m, ok := niBgpNbrCfgSubscMaps[node]
		if !ok {
			m = niBgpSubscMap{dbNum: db.ConfigDB, table: niBgpNbrTbl, key: niBgpSubscKeyNiNbr, fields: map[string][]string{}}
			niBgpNbrCfgSubscMaps[node] = m
			niBgpGlobalSubscVirtual["/bgp/neighbors/neighbor/"+strings.TrimSuffix(c, "/config")] = true
		}
		for leaf, fld := range leaves {
			m.fields[leaf] = []string{fld}
			niBgpMappedPayloadLeaves["neighbors/neighbor/"+c+"/"+leaf] = true
		}
	}
	afNodes := map[string]map[string][]string{
		"/afi-safis/afi-safi/" + niBgpNbrAddPathsCfg: {"send": {niBgpNbrTxAddPathsFld, niBgpNbrTxAddPathCountFld}, "send-max": {niBgpNbrTxAddPathCountFld}},
	}
	for c, leaves := range niBgpNbrAfLeafFlds {
		afNodes["/afi-safis/afi-safi/"+c] = map[string][]string{}
		for leaf, fld := range leaves {
			afNodes["/afi-safis/afi-safi/"+c][leaf] = []string{fld}
		}
	}
	for node, flds := range afNodes {
		niBgpNbrCfgSubscMaps[node] = niBgpSubscMap{dbNum: db.ConfigDB, table: niBgpNbrAfTbl, key: niBgpSubscKeyNiNbrAf, fields: flds}
		for leaf := range flds {
			niBgpMappedPayloadLeaves["neighbors/neighbor"+node+"/"+leaf] = true
		}
	}
	niBgpNbrCfgSubscVirtual["/afi-safis/afi-safi/add-paths"] = true
	for _, f := range []string{"send_default_route", niBgpNbrTxAddPathsFld, niBgpNbrTxAddPathCountFld} {
		niBgpNbrAfInstOwnedFlds[f] = true
	}
}
