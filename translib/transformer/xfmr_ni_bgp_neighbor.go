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
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niBgpNbrTbl          = "BGP_NEIGHBOR"
	niBgpNbrPeerGroupFld = "peer_group_name"
	niBgpNbrAdminStatus  = "admin_status"
)

var niBgpNbrReqUriRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/protocols/protocol\[identifier=BGP\]\[name=bgp\]/bgp/neighbors/neighbor\[neighbor-address=([^\]]+)\]/config(?:/(peer-group|enabled))?$`)

type niBgpNbrReqInfo struct {
	niName       string
	neighborAddr string
	targetLeaf   string // "", "peer-group", "enabled"
}

func init() {
	XlateFuncBind("YangToDb_ni_bgp_nbr_cfg_xfmr", YangToDb_ni_bgp_nbr_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_bgp_nbr_cfg_xfmr", DbToYang_ni_bgp_nbr_cfg_xfmr)
}

func parseNiBgpNbrRequestUri(uri string) (*niBgpNbrReqInfo, error) {
	m := niBgpNbrReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &niBgpNbrReqInfo{
			niName:       m[1],
			neighborAddr: m[2],
			targetLeaf:   leaf,
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP neighbor config mapping: " + uri,
	}
}

func getNiBgpNbrCfgObj(s *ygot.GoStruct, niName, neighborAddr string, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config {
	if s == nil || *s == nil {
		return nil
	}

	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(deviceObj)

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
		Identifier: ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
		Name:       "bgp",
	}
	if pObj, ok := niObj.Protocols.Protocol[wantKey]; ok && pObj != nil {
		ygot.BuildEmptyTree(pObj)
		if nbrObj, ok := pObj.Bgp.Neighbors.Neighbor[neighborAddr]; ok && nbrObj != nil {
			ygot.BuildEmptyTree(nbrObj)
			return nbrObj.Config
		}
		if !createIfMissing {
			return nil
		}
		nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
		if err != nil || nbrObj == nil {
			return nil
		}
		ygot.BuildEmptyTree(nbrObj)
		return nbrObj.Config
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Neighbors == nil {
			continue
		}
		if !strings.Contains(fmt.Sprint(pKey.Identifier), "BGP") {
			continue
		}
		if pKey.Name != "bgp" {
			if pObj.Name == nil || *pObj.Name != "bgp" {
				continue
			}
		}
		ygot.BuildEmptyTree(pObj)
		if nbrObj, ok := pObj.Bgp.Neighbors.Neighbor[neighborAddr]; ok && nbrObj != nil {
			ygot.BuildEmptyTree(nbrObj)
			return nbrObj.Config
		}
		if !createIfMissing {
			return nil
		}
		nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
		if err != nil || nbrObj == nil {
			return nil
		}
		ygot.BuildEmptyTree(nbrObj)
		return nbrObj.Config
	}

	if !createIfMissing {
		return nil
	}
	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	nbrObj, err := pObj.Bgp.Neighbors.NewNeighbor(neighborAddr)
	if err != nil || nbrObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(nbrObj)
	return nbrObj.Config
}

func parseNiBgpNbrPeerGroupValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s != "" {
			return s, true
		}
	case *string:
		if val != nil {
			s := strings.TrimSpace(*val)
			if s != "" {
				return s, true
			}
		}
	}
	return "", false
}

func parseNiBgpNbrPeerGroupFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if pg, ok := parseNiBgpNbrPeerGroupValue(param); ok {
		return pg, true
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
		fv := rv.FieldByName("PeerGroup")
		if fv.IsValid() && fv.CanInterface() {
			if pg, ok := parseNiBgpNbrPeerGroupValue(fv.Interface()); ok {
				return pg, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "peer-group" || strings.HasSuffix(k, ":peer-group") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if pg, ok := parseNiBgpNbrPeerGroupValue(mv.Interface()); ok {
						return pg, true
					}
				}
			}
		}
	}
	return "", false
}

func parseNiBgpNbrPeerGroup(inParams XfmrParams) (string, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return "", err
	}
	if pg, ok := parseNiBgpNbrPeerGroupFromParam(inParams.param); ok {
		return pg, nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.PeerGroup == nil || strings.TrimSpace(*cfgObj.PeerGroup) == "" {
		return "", tlerr.InvalidArgsError{Format: "BGP neighbor peer-group is missing in request payload"}
	}
	return strings.TrimSpace(*cfgObj.PeerGroup), nil
}

func niBgpNbrAdminStatusToEnabled(status string) (*bool, bool) {
	s := strings.TrimSpace(strings.ToLower(status))
	switch s {
	case "up", "true":
		v := true
		return &v, true
	case "down", "false":
		v := false
		return &v, true
	default:
		return nil, false
	}
}

func niBgpNbrEnabledToAdminStatus(enabled bool) string {
	if enabled {
		return "up"
	}
	return "down"
}

func parseNiBgpNbrEnabledValue(v interface{}) (*bool, bool) {
	switch val := v.(type) {
	case bool:
		b := val
		return &b, true
	case *bool:
		if val != nil {
			return val, true
		}
	case string:
		s := strings.TrimSpace(val)
		if b, err := strconv.ParseBool(s); err == nil {
			return &b, true
		}
	case *string:
		if val != nil {
			s := strings.TrimSpace(*val)
			if b, err := strconv.ParseBool(s); err == nil {
				return &b, true
			}
		}
	}
	return nil, false
}

func parseNiBgpNbrEnabledFromParam(param interface{}) (*bool, bool) {
	if param == nil {
		return nil, false
	}
	if enabled, ok := parseNiBgpNbrEnabledValue(param); ok {
		return enabled, true
	}

	rv := reflect.ValueOf(param)
	for rv.IsValid() && (rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr) {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil, false
	}

	if rv.Kind() == reflect.Struct {
		fv := rv.FieldByName("Enabled")
		if fv.IsValid() && fv.CanInterface() {
			if enabled, ok := parseNiBgpNbrEnabledValue(fv.Interface()); ok {
				return enabled, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "enabled" || strings.HasSuffix(k, ":enabled") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if enabled, ok := parseNiBgpNbrEnabledValue(mv.Interface()); ok {
						return enabled, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseNiBgpNbrEnabled(inParams XfmrParams) (*bool, error) {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return nil, err
	}
	if enabled, ok := parseNiBgpNbrEnabledFromParam(inParams.param); ok {
		return enabled, nil
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, false)
	if cfgObj == nil || cfgObj.Enabled == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP neighbor enabled is missing in request payload"}
	}
	return cfgObj.Enabled, nil
}

func makeNiBgpNbrResMap(niName, neighborAddr string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpNbrTbl: {
			niName + "|" + neighborAddr: db.Value{Field: fields},
		},
	}
}

var YangToDb_ni_bgp_nbr_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		switch reqInfo.targetLeaf {
		case "peer-group":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrPeerGroupFld: ""}), nil
		case "enabled":
			return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, map[string]string{niBgpNbrAdminStatus: ""}), nil
		default:
			return res, tlerr.NotSupportedError{Format: "DELETE supported only for /neighbors/neighbor/config/peer-group or /neighbors/neighbor/config/enabled", Path: inParams.requestUri}
		}
	}

	fields := make(map[string]string)
	switch reqInfo.targetLeaf {
	case "peer-group":
		pg, err := parseNiBgpNbrPeerGroup(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpNbrPeerGroupFld] = pg
	case "enabled":
		enabled, err := parseNiBgpNbrEnabled(inParams)
		if err != nil {
			return res, err
		}
		fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
	default:
		if pg, ok := parseNiBgpNbrPeerGroupFromParam(inParams.param); ok {
			fields[niBgpNbrPeerGroupFld] = pg
		}
		if enabled, ok := parseNiBgpNbrEnabledFromParam(inParams.param); ok {
			fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*enabled)
		}
		if len(fields) == 0 {
			cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, false)
			if cfgObj != nil {
				if cfgObj.PeerGroup != nil && strings.TrimSpace(*cfgObj.PeerGroup) != "" {
					fields[niBgpNbrPeerGroupFld] = strings.TrimSpace(*cfgObj.PeerGroup)
				}
				if cfgObj.Enabled != nil {
					fields[niBgpNbrAdminStatus] = niBgpNbrEnabledToAdminStatus(*cfgObj.Enabled)
				}
			}
		}
		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "BGP neighbor config payload missing supported fields (peer-group/enabled)"}
		}
	}
	return makeNiBgpNbrResMap(reqInfo.niName, reqInfo.neighborAddr, fields), nil
}

var DbToYang_ni_bgp_nbr_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseNiBgpNbrRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP neighbor config"}
	}

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrTbl}, db.Key{Comp: []string{reqInfo.niName, reqInfo.neighborAddr}})
	if err != nil {
		return err
	}

	cfgObj := getNiBgpNbrCfgObj(inParams.ygRoot, reqInfo.niName, reqInfo.neighborAddr, true)
	if cfgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP neighbor config object in YANG tree"}
	}

	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "peer-group" {
		pg, ok := entry.Field[niBgpNbrPeerGroupFld]
		if ok && strings.TrimSpace(pg) != "" {
			pg = strings.TrimSpace(pg)
			cfgObj.PeerGroup = &pg
		}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == "enabled" {
		status, ok := entry.Field[niBgpNbrAdminStatus]
		if ok && strings.TrimSpace(status) != "" {
			if enabled, ok := niBgpNbrAdminStatusToEnabled(status); ok {
				cfgObj.Enabled = enabled
			}
		}
	}
	return nil
}
