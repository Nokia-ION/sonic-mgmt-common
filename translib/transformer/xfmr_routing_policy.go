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
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	rpolRouteMapSetTbl     = "ROUTE_MAP_SET"
	rpolTargetPolicyDefCfg = "policy-definition-config"
	rpolPolicyDefNameLeaf  = "name"
)

var rpolPolicyDefCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/config(?:/(name))?$`)

type rpolReqInfo struct {
	policyName string
	targetLeaf string
	targetPath string
}

func init() {
	XlateFuncBind("YangToDb_routing_policy_xfmr", YangToDb_routing_policy_xfmr)
	XlateFuncBind("DbToYang_routing_policy_xfmr", DbToYang_routing_policy_xfmr)
}

func parseRpolRequestUri(uri string) (*rpolReqInfo, error) {
	m := rpolPolicyDefCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &rpolReqInfo{
			policyName: m[1],
			targetLeaf: leaf,
			targetPath: rpolTargetPolicyDefCfg,
		}, nil
	}
	return nil, tlerr.InvalidArgsError{
		Format: "unsupported path for routing-policy mapping: " + uri,
	}
}

func getRpolPolicyDefCfgObj(s *ygot.GoStruct, policyName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Config {
	if s == nil || *s == nil {
		return nil
	}

	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(deviceObj)

	if deviceObj.RoutingPolicy == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy{}
	}
	if deviceObj.RoutingPolicy.PolicyDefinitions == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.PolicyDefinitions = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions{}
	}

	if pdObj, ok := deviceObj.RoutingPolicy.PolicyDefinitions.PolicyDefinition[policyName]; ok && pdObj != nil {
		ygot.BuildEmptyTree(pdObj)
		return pdObj.Config
	}
	if !createIfMissing {
		return nil
	}
	pdObj, err := deviceObj.RoutingPolicy.PolicyDefinitions.NewPolicyDefinition(policyName)
	if err != nil || pdObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pdObj)
	return pdObj.Config
}

func parseRpolPolicyNameValue(v interface{}) (string, bool) {
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

func parseRpolPolicyNameFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if name, ok := parseRpolPolicyNameValue(param); ok {
		return name, true
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
		fv := rv.FieldByName("Name")
		if fv.IsValid() && fv.CanInterface() {
			if name, ok := parseRpolPolicyNameValue(fv.Interface()); ok {
				return name, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "name" || strings.HasSuffix(k, ":name") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if name, ok := parseRpolPolicyNameValue(mv.Interface()); ok {
						return name, true
					}
				}
			}
		}
	}
	return "", false
}

func makeRpolRouteMapSetResMap(policyName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		rpolRouteMapSetTbl: {
			policyName: db.Value{Field: fields},
		},
	}
}

var YangToDb_routing_policy_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	reqInfo, err := parseRpolRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		return makeRpolRouteMapSetResMap(reqInfo.policyName, nil), nil
	}

	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPolicyDefNameLeaf {
		if name, ok := parseRpolPolicyNameFromParam(inParams.param); ok && name != reqInfo.policyName {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("policy-definition name payload (%s) does not match key (%s)", name, reqInfo.policyName)}
		}
		if reqInfo.targetLeaf == "" {
			cfgObj := getRpolPolicyDefCfgObj(inParams.ygRoot, reqInfo.policyName, false)
			if cfgObj != nil && cfgObj.Name != nil && strings.TrimSpace(*cfgObj.Name) != "" && strings.TrimSpace(*cfgObj.Name) != reqInfo.policyName {
				return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("policy-definition config name payload (%s) does not match key (%s)", strings.TrimSpace(*cfgObj.Name), reqInfo.policyName)}
			}
		}
		return makeRpolRouteMapSetResMap(reqInfo.policyName, map[string]string{}), nil
	}

	return res, tlerr.NotSupportedError{Format: "unsupported leaf under routing-policy policy-definition config", Path: inParams.requestUri}
}

var DbToYang_routing_policy_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	reqInfo, err := parseRpolRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for routing-policy mapping"}
	}

	_, err = cfgDb.GetEntry(&db.TableSpec{Name: rpolRouteMapSetTbl}, db.Key{Comp: []string{reqInfo.policyName}})
	if err != nil {
		return err
	}

	cfgObj := getRpolPolicyDefCfgObj(inParams.ygRoot, reqInfo.policyName, true)
	if cfgObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy policy-definition config object in YANG tree"}
	}
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPolicyDefNameLeaf {
		name := reqInfo.policyName
		cfgObj.Name = &name
	}

	return nil
}
