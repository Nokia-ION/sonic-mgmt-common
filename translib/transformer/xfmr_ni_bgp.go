////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                 //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
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
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	niBgpGlobalsTbl = "BGP_GLOBALS"
	niBgpGlobalsKey = "default"
	niBgpAsFld      = "local_asn"
	niBgpCfgUri     = "/openconfig-network-instance:network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/config"
	niBgpAsUri      = niBgpCfgUri + "/as"
)

func init() {
	XlateFuncBind("YangToDb_ni_bgp_global_cfg_xfmr", YangToDb_ni_bgp_global_cfg_xfmr)
	XlateFuncBind("DbToYang_ni_bgp_global_cfg_xfmr", DbToYang_ni_bgp_global_cfg_xfmr)
}

func validateNiBgpRequestUri(uri string) error {
	if uri == niBgpCfgUri || uri == niBgpAsUri {
		return nil
	}
	return tlerr.InvalidArgsError{
		Format: "unsupported path for network-instance BGP global AS mapping: " + uri,
	}
}

func getNiBgpGlobalCfgObj(s *ygot.GoStruct, createIfMissing bool) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config {
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

	niObj, ok := deviceObj.NetworkInstances.NetworkInstance[niBgpGlobalsKey]
	if !ok || niObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		niObj, err = deviceObj.NetworkInstances.NewNetworkInstance(niBgpGlobalsKey)
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
		return pObj.Bgp.Global.Config
	}

	for pKey, pObj := range niObj.Protocols.Protocol {
		if pObj == nil || pObj.Bgp == nil || pObj.Bgp.Global == nil {
			continue
		}
		if !strings.Contains(fmt.Sprint(pKey.Identifier), "BGP") {
			continue
		}
		// Name might be nil in some request bind flows; accept key match too.
		if pKey.Name != "bgp" {
			if pObj.Name == nil || *pObj.Name != "bgp" {
				continue
			}
		}
		ygot.BuildEmptyTree(pObj)
		return pObj.Bgp.Global.Config
	}

	if !createIfMissing {
		return nil
	}

	pObj, err := niObj.Protocols.NewProtocol(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, "bgp")
	if err != nil || pObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pObj)
	return pObj.Bgp.Global.Config
}

func parseNiBgpAsValue(v interface{}) (*uint32, bool) {
	switch val := v.(type) {
	case uint32:
		as := val
		return &as, true
	case *uint32:
		if val != nil {
			return val, true
		}
	case uint64:
		if val <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case uint:
		if uint64(val) <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case int:
		if val >= 0 && uint64(val) <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case int32:
		if val >= 0 {
			as := uint32(val)
			return &as, true
		}
	case int64:
		if val >= 0 && uint64(val) <= math.MaxUint32 {
			as := uint32(val)
			return &as, true
		}
	case float64:
		if val >= 0 && val <= float64(math.MaxUint32) && math.Trunc(val) == val {
			as := uint32(val)
			return &as, true
		}
	case float32:
		f := float64(val)
		if f >= 0 && f <= float64(math.MaxUint32) && math.Trunc(f) == f {
			as := uint32(f)
			return &as, true
		}
	case json.Number:
		if i64, err := val.Int64(); err == nil && i64 >= 0 && uint64(i64) <= math.MaxUint32 {
			as := uint32(i64)
			return &as, true
		}
	case string:
		if u64, err := strconv.ParseUint(val, 10, 32); err == nil {
			as := uint32(u64)
			return &as, true
		}
	case *string:
		if val != nil {
			if u64, err := strconv.ParseUint(*val, 10, 32); err == nil {
				as := uint32(u64)
				return &as, true
			}
		}
	}
	return nil, false
}

func parseNiBgpAsFromParam(param interface{}) (*uint32, bool) {
	if param == nil {
		return nil, false
	}
	if as, ok := parseNiBgpAsValue(param); ok {
		return as, true
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
		fv := rv.FieldByName("As")
		if fv.IsValid() && fv.CanInterface() {
			if as, ok := parseNiBgpAsValue(fv.Interface()); ok {
				return as, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "as" || strings.HasSuffix(k, ":as") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if as, ok := parseNiBgpAsValue(mv.Interface()); ok {
						return as, true
					}
				}
			}
		}
	}

	return nil, false
}

func parseNiBgpAs(inParams XfmrParams) (*uint32, error) {
	if asPtr, ok := parseNiBgpAsFromParam(inParams.param); ok {
		return asPtr, nil
	}

	cfgObj := getNiBgpGlobalCfgObj(inParams.ygRoot, false)
	if cfgObj == nil || cfgObj.As == nil {
		return nil, tlerr.InvalidArgsError{Format: "BGP global AS is missing in request payload"}
	}

	return cfgObj.As, nil
}

func makeNiBgpGlobalsResMap(asVal string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		niBgpGlobalsTbl: {
			niBgpGlobalsKey: db.Value{
				Field: map[string]string{
					niBgpAsFld: asVal,
				},
			},
		},
	}
}

var YangToDb_ni_bgp_global_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	if err := validateNiBgpRequestUri(inParams.requestUri); err != nil {
		return res, err
	}

	// DELETE: clear only the AS field in BGP_GLOBALS|default. Returning the
	// field under the DELETE operation removes that field from the entry.
	if inParams.oper == DELETE {
		if inParams.requestUri != niBgpAsUri {
			return res, tlerr.NotSupportedError{Format: "DELETE supported only for /config/as", Path: inParams.requestUri}
		}
		return makeNiBgpGlobalsResMap(""), nil
	}

	asPtr, err := parseNiBgpAs(inParams)
	if err != nil {
		return res, err
	}

	// Return the primary result map directly so translib commits the write for
	// both container (/config) and leaf (/config/as) targeted requests.
	return makeNiBgpGlobalsResMap(strconv.FormatUint(uint64(*asPtr), 10)), nil
}

var DbToYang_ni_bgp_global_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	if err := validateNiBgpRequestUri(inParams.requestUri); err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP global config"}
	}

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsTbl}, db.Key{Comp: []string{niBgpGlobalsKey}})
	if err != nil {
		return err
	}

	asStr, ok := entry.Field[niBgpAsFld]
	if !ok || asStr == "" {
		return nil
	}

	asVal, err := strconv.ParseUint(asStr, 10, 32)
	if err != nil {
		return err
	}

	cfgObj := getNiBgpGlobalCfgObj(inParams.ygRoot, true)
	if cfgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate BGP global config object in YANG tree"}
	}

	asU32 := uint32(asVal)
	cfgObj.As = &asU32
	return nil
}
