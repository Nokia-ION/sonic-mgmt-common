package transformer

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// niBgpGlobalLeafFlds maps the containers of the BGP global instance whose
// leaves are plain fields of the BGP_GLOBALS row, by their path below
// bgp/global, to the field of each leaf. A leaf-list field carries the @
// suffix CONFIG_DB lists are stored under. Each container has a config and a
// state twin; FRR applies these settings as given, so state reports CONFIG_DB.
var niBgpGlobalLeafFlds = map[string]map[string]string{
	"route-selection-options": {
		"always-compare-med":         niBgpAlwaysCompareMedFld,
		"ignore-as-path-length":      "ignore_as_path_length",
		"external-compare-router-id": "external_compare_router_id",
	},
	"use-multiple-paths/ebgp": {
		"maximum-paths":     niBgpMaxEbgpPathsFld,
		"allow-multiple-as": "load_balance_mp_relax",
	},
	"use-multiple-paths/ibgp": {
		"maximum-paths": niBgpMaxIbgpPathsFld,
	},
	"graceful-restart": {
		"enabled":           "graceful_restart_enable",
		"restart-time":      "gr_restart_time",
		"stale-routes-time": "gr_stale_routes_time",
	},
	"confederation": {
		"identifier": "confed_id",
		"member-as":  "confed_peers@",
	},
}

const niBgpTargetGlobalLeaves = "global-leaves"

var niBgpGlobalLeafReqUriRe = regexp.MustCompile(niBgpProtoUriPfx +
	`/bgp/global/(route-selection-options|use-multiple-paths/ebgp|use-multiple-paths/ibgp|graceful-restart|confederation)/(config|state)(?:/([a-z-]+))?$`)

// niBgpGlobalLeafContainers lists the containers in a fixed order, so that
// the maps built from them and the errors naming their leaves are stable.
func niBgpGlobalLeafContainers() []string {
	names := make([]string, 0, len(niBgpGlobalLeafFlds))
	for name := range niBgpGlobalLeafFlds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ocFieldByPath returns the field of struct v whose path tag is name.
func ocFieldByPath(v reflect.Value, name string) reflect.Value {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	for i := 0; i < v.NumField(); i++ {
		if v.Type().Field(i).Tag.Get("path") == name {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}

// niBgpGlobalLeafNode returns the container of the global instance that path
// names, below bgp/global. With create set, missing containers on the way are
// allocated.
func niBgpGlobalLeafNode(s *ygot.GoStruct, niName, protoName, path string, create bool) reflect.Value {
	bgpObj := getNiBgpProtoObj(s, niName, protoName, create)
	if bgpObj == nil {
		return reflect.Value{}
	}
	v := reflect.ValueOf(bgpObj)
	for _, seg := range append([]string{"global"}, strings.Split(path, "/")...) {
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

// niBgpGlobalLeafToDb renders one leaf value as its CONFIG_DB field value.
func niBgpGlobalLeafToDb(f reflect.Value) (string, bool) {
	switch f.Kind() {
	case reflect.Ptr:
		if f.IsNil() {
			return "", false
		}
		e := f.Elem()
		switch e.Kind() {
		case reflect.Bool:
			return strconv.FormatBool(e.Bool()), true
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return strconv.FormatUint(e.Uint(), 10), true
		}
	case reflect.Slice:
		if f.Len() == 0 {
			return "", false
		}
		vals := make([]string, 0, f.Len())
		for i := 0; i < f.Len(); i++ {
			vals = append(vals, strconv.FormatUint(f.Index(i).Uint(), 10))
		}
		return strings.Join(vals, ","), true
	}
	return "", false
}

// niBgpGlobalLeafFromDb sets one leaf from its CONFIG_DB field value.
func niBgpGlobalLeafFromDb(f reflect.Value, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || !f.CanSet() {
		return nil
	}
	switch f.Kind() {
	case reflect.Ptr:
		t := f.Type().Elem()
		nv := reflect.New(t)
		switch t.Kind() {
		case reflect.Bool:
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return err
			}
			nv.Elem().SetBool(b)
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n, err := strconv.ParseUint(raw, 10, t.Bits())
			if err != nil {
				return err
			}
			nv.Elem().SetUint(n)
		default:
			return nil
		}
		f.Set(nv)
	case reflect.Slice:
		parts := strings.Split(raw, ",")
		sl := reflect.MakeSlice(f.Type(), 0, len(parts))
		for _, p := range parts {
			n, err := strconv.ParseUint(strings.TrimSpace(p), 10, f.Type().Elem().Bits())
			if err != nil {
				return err
			}
			sl = reflect.Append(sl, reflect.ValueOf(n).Convert(f.Type().Elem()))
		}
		f.Set(sl)
	}
	return nil
}

// niBgpGlobalLeafFields maps the config struct cfg of container to BGP_GLOBALS
// fields. Leaves it holds that have no field are returned apart, by name.
func niBgpGlobalLeafFields(cfg reflect.Value, container string) (map[string]string, []string) {
	fields := map[string]string{}
	var unmapped []string
	if !cfg.IsValid() || (cfg.Kind() == reflect.Ptr && cfg.IsNil()) {
		return fields, nil
	}
	v := cfg.Elem()
	for i := 0; i < v.NumField(); i++ {
		leaf := v.Type().Field(i).Tag.Get("path")
		val, set := niBgpGlobalLeafToDb(v.Field(i))
		if !set {
			continue
		}
		fld, ok := niBgpGlobalLeafFlds[container][leaf]
		if !ok {
			unmapped = append(unmapped, "global/"+container+"/config/"+leaf)
			continue
		}
		fields[fld] = val
	}
	return fields, unmapped
}

// niBgpGlobalLeafYangToDb maps a SET or a DELETE of one of the containers in
// niBgpGlobalLeafFlds, or of a leaf in one.
func niBgpGlobalLeafYangToDb(inParams XfmrParams, reqInfo *niBgpReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	leafFlds := niBgpGlobalLeafFlds[reqInfo.container]
	if reqInfo.targetLeaf != "" {
		if _, ok := leafFlds[reqInfo.targetLeaf]; !ok {
			return res, tlerr.NotSupportedError{
				Format: fmt.Sprintf("BGP global %s leaf %s has no CONFIG_DB mapping", reqInfo.container, reqInfo.targetLeaf),
				Path:   inParams.requestUri,
			}
		}
	}
	if inParams.oper == DELETE {
		fields := map[string]string{}
		for leaf, fld := range leafFlds {
			if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == leaf {
				fields[fld] = ""
			}
		}
		return makeNiBgpGlobalsResMap(reqInfo.niName, fields), nil
	}

	cfg := ocFieldByPath(niBgpGlobalLeafNode(inParams.ygRoot, reqInfo.niName, reqInfo.protoName, reqInfo.container, false), "config")
	fields, unmapped := niBgpGlobalLeafFields(cfg, reqInfo.container)
	if len(unmapped) > 0 {
		return res, tlerr.NotSupportedError{
			Format: "BGP global payload carries leaves that have no CONFIG_DB mapping: " + strings.Join(unmapped, ", "),
			Path:   inParams.requestUri,
		}
	}
	if reqInfo.targetLeaf != "" {
		fld := leafFlds[reqInfo.targetLeaf]
		val, ok := fields[fld]
		if !ok {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP global %s %s is missing in request payload", reqInfo.container, reqInfo.targetLeaf)}
		}
		fields = map[string]string{fld: val}
	}
	if len(fields) == 0 {
		return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP global %s config payload carries no supported leaf", reqInfo.container)}
	}
	return makeNiBgpGlobalsResMap(reqInfo.niName, fields), nil
}

// niBgpGlobalLeavesToYang fills the config container of container from the
// BGP_GLOBALS row, only targetLeaf when one is named, and its state twin with
// the same values.
func niBgpGlobalLeavesToYang(ygRoot *ygot.GoStruct, niName, protoName, container, targetLeaf string, entry db.Value) error {
	var held []string
	for leaf, fld := range niBgpGlobalLeafFlds[container] {
		if (targetLeaf == "" || targetLeaf == leaf) && strings.TrimSpace(entry.Field[fld]) != "" {
			held = append(held, leaf)
		}
	}
	if len(held) == 0 {
		return nil
	}
	node := niBgpGlobalLeafNode(ygRoot, niName, protoName, container, true)
	cfg := ocFieldByPath(node, "config")
	if !cfg.IsValid() || cfg.Kind() != reflect.Ptr {
		return nil
	}
	if cfg.IsNil() {
		cfg.Set(reflect.New(cfg.Type().Elem()))
	}
	for _, leaf := range held {
		if err := niBgpGlobalLeafFromDb(ocFieldByPath(cfg, leaf), entry.Field[niBgpGlobalLeafFlds[container][leaf]]); err != nil {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid BGP_GLOBALS value for %s %s: %v", container, leaf, err)}
		}
	}
	ocMirrorConfigToState(node.Interface())
	return nil
}

// niBgpGlobalLeavesHierResMap maps the containers in niBgpGlobalLeafFlds that
// a container-level payload carries.
func niBgpGlobalLeavesHierResMap(s *ygot.GoStruct, niName, protoName string) map[string]string {
	fields := map[string]string{}
	for _, container := range niBgpGlobalLeafContainers() {
		cfg := ocFieldByPath(niBgpGlobalLeafNode(s, niName, protoName, container, false), "config")
		flds, _ := niBgpGlobalLeafFields(cfg, container)
		for k, v := range flds {
			fields[k] = v
		}
	}
	return fields
}

const niBgpTargetProtoCfg = "protocol-config"

var niBgpProtoCfgReqUriRe = regexp.MustCompile(niBgpProtoUriPfx + `/(?:config|state)(?:/(identifier|name|enabled))?$`)

func niBgpProtoInstanceObj(ygRoot *ygot.GoStruct, niName, protoName string) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol {
	if ygRoot == nil || *ygRoot == nil {
		return nil
	}
	devObj, ok := (*ygRoot).(*ocbinds.Device)
	if !ok || devObj.NetworkInstances == nil {
		return nil
	}
	niObj := devObj.NetworkInstances.NetworkInstance[niName]
	if niObj == nil || niObj.Protocols == nil {
		return nil
	}
	return niObj.Protocols.Protocol[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key{
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       protoName,
	}]
}

// niBgpProtoInstanceToYang fills config and state of the BGP protocol
// instance: its identity from the key, and enabled true, since the instance
// exists only while its BGP_GLOBALS row does.
func niBgpProtoInstanceToYang(ygRoot *ygot.GoStruct, niName, protoName string) {
	pObj := niBgpProtoInstanceObj(ygRoot, niName, protoName)
	if pObj == nil {
		return
	}
	if pObj.Config == nil {
		pObj.Config = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Config{}
	}
	name := protoName
	enabled := true
	pObj.Config.Identifier = ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP
	pObj.Config.Name = &name
	pObj.Config.Enabled = &enabled
	ocMirrorConfigToState(pObj)
}

// niBgpProtoInstanceYangToDb maps a write of the BGP protocol instance's
// config. Its identity restates the key and enabled true is what the instance
// already is, so neither holds a field; a disabled BGP instance has no FRR
// form other than its removal, which is a delete of the instance.
func niBgpProtoInstanceYangToDb(inParams XfmrParams, reqInfo *niBgpReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if inParams.oper == DELETE {
		return res, tlerr.NotSupportedError{
			Format: "DELETE is not supported for the BGP protocol instance config; delete the protocol instance instead",
			Path:   inParams.requestUri,
		}
	}
	if pObj := niBgpProtoInstanceObj(inParams.ygRoot, reqInfo.niName, reqInfo.protoName); pObj != nil && pObj.Config != nil &&
		pObj.Config.Enabled != nil && !*pObj.Config.Enabled {
		return res, tlerr.NotSupportedError{
			Format: "BGP protocol instance config enabled false is not supported; delete the protocol instance instead",
			Path:   inParams.requestUri,
		}
	}
	return res, nil
}

// niBgpGlobalAfMirror fills the state twins of a global afi-safi instance, its
// own and those of its use-multiple-paths containers, from their config. The
// instance takes its afi-safi-name from the key.
func niBgpGlobalAfMirror(ygRoot *ygot.GoStruct, niName, protoName, afiSafi string) {
	afObj := getNiBgpGlobalAfObj(ygRoot, niName, protoName, afiSafi, false)
	if afObj == nil {
		return
	}
	if afObj.Config != nil {
		if name, ok := niBgpNbrAfiSafiEnumFromName(afiSafi); ok {
			afObj.Config.AfiSafiName = name
		}
	}
	ocMirrorConfigToState(afObj)
	if mp := afObj.UseMultiplePaths; mp != nil {
		if mp.Ebgp != nil {
			ocMirrorConfigToState(mp.Ebgp)
		}
		if mp.Ibgp != nil {
			ocMirrorConfigToState(mp.Ibgp)
		}
	}
}

func init() {
	for container, leaves := range niBgpGlobalLeafFlds {
		node := "/bgp/global/" + container
		niBgpGlobalSubscVirtual[node] = true
		subscFlds := map[string][]string{}
		for leaf, fld := range leaves {
			subscFlds[leaf] = []string{fld}
		}
		for _, twin := range []string{"config", "state"} {
			niBgpGlobalSubscMaps[node+"/"+twin] = niBgpSubscMap{dbNum: db.ConfigDB, table: niBgpGlobalsTbl, key: niBgpSubscKeyNi, fields: subscFlds}
		}
		for leaf := range leaves {
			niBgpMappedPayloadLeaves["global/"+container+"/config/"+leaf] = true
		}
	}
}
