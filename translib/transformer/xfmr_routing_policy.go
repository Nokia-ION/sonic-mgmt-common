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
	rpolRouteMapSetTbl            = "ROUTE_MAP_SET"
	rpolRouteMapTbl               = "ROUTE_MAP"
	rpolRouteOperationFld         = "route_operation"
	rpolMatchPrefixSetFld         = "match_prefix_set"
	rpolMatchIpv6PrefixSetFld     = "match_ipv6_prefix_set"
	rpolRouteOpPermit             = "permit"
	rpolRouteOpDeny               = "deny"
	rpolTargetPolicyDefCfg        = "policy-definition-config"
	rpolTargetStmtActionsCfg      = "statement-actions-config"
	rpolTargetStmtCondMatchPfxCfg = "statement-conditions-match-prefix-set-config"
	rpolPolicyDefNameLeaf         = "name"
	rpolPolicyResultLeaf          = "policy-result"
	rpolPrefixSetLeaf             = "prefix-set"
)

var rpolPolicyDefCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/config(?:/(name))?$`)
var rpolStmtActionsCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/config(?:/(policy-result))?$`)
var rpolStmtCondMatchPfxCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions/match-prefix-set/config(?:/(prefix-set))?$`)

type rpolReqInfo struct {
	policyName string
	stmtName   string
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
	m = rpolStmtActionsCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtActionsCfg,
		}, nil
	}
	m = rpolStmtCondMatchPfxCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtCondMatchPfxCfg,
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

func getRpolPolicyStmtActionsCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_Config {
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

	pdObj, ok := deviceObj.RoutingPolicy.PolicyDefinitions.PolicyDefinition[policyName]
	if !ok || pdObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pdObj, err = deviceObj.RoutingPolicy.PolicyDefinitions.NewPolicyDefinition(policyName)
		if err != nil || pdObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(pdObj)

	stmtObj, ok := pdObj.Statements.Statement[stmtName]
	if !ok || stmtObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		stmtObj, err = pdObj.Statements.NewStatement(stmtName)
		if err != nil || stmtObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(stmtObj)
	return stmtObj.Actions.Config
}

func getRpolPolicyStmtCondMatchPrefixSetCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchPrefixSet_Config {
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

	pdObj, ok := deviceObj.RoutingPolicy.PolicyDefinitions.PolicyDefinition[policyName]
	if !ok || pdObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		pdObj, err = deviceObj.RoutingPolicy.PolicyDefinitions.NewPolicyDefinition(policyName)
		if err != nil || pdObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(pdObj)

	stmtObj, ok := pdObj.Statements.Statement[stmtName]
	if !ok || stmtObj == nil {
		if !createIfMissing {
			return nil
		}
		var err error
		stmtObj, err = pdObj.Statements.NewStatement(stmtName)
		if err != nil || stmtObj == nil {
			return nil
		}
	}
	ygot.BuildEmptyTree(stmtObj)
	return stmtObj.Conditions.MatchPrefixSet.Config
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

func parseRpolPolicyResultValue(v interface{}) (ocbinds.E_OpenconfigRoutingPolicy_PolicyResultType, bool) {
	switch val := v.(type) {
	case ocbinds.E_OpenconfigRoutingPolicy_PolicyResultType:
		switch val {
		case ocbinds.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE,
			ocbinds.OpenconfigRoutingPolicy_PolicyResultType_REJECT_ROUTE:
			return val, true
		}
	case string:
		s := strings.ToUpper(strings.TrimSpace(val))
		switch s {
		case "ACCEPT_ROUTE":
			return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE, true
		case "REJECT_ROUTE":
			return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_REJECT_ROUTE, true
		}
	}
	return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_UNSET, false
}

func parseRpolPolicyResultFromParam(param interface{}) (ocbinds.E_OpenconfigRoutingPolicy_PolicyResultType, bool) {
	if param == nil {
		return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_UNSET, false
	}
	if p, ok := parseRpolPolicyResultValue(param); ok {
		return p, true
	}

	rv := reflect.ValueOf(param)
	for rv.IsValid() && (rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr) {
		if rv.IsNil() {
			return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_UNSET, false
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_UNSET, false
	}

	if rv.Kind() == reflect.Struct {
		fv := rv.FieldByName("PolicyResult")
		if fv.IsValid() && fv.CanInterface() {
			if p, ok := parseRpolPolicyResultValue(fv.Interface()); ok {
				return p, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "policy-result" || strings.HasSuffix(k, ":policy-result") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if p, ok := parseRpolPolicyResultValue(mv.Interface()); ok {
						return p, true
					}
				}
			}
		}
	}
	return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_UNSET, false
}

func parseRpolPrefixSetValue(v interface{}) (string, bool) {
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

func parseRpolPrefixSetFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if pfxSet, ok := parseRpolPrefixSetValue(param); ok {
		return pfxSet, true
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
		fv := rv.FieldByName("PrefixSet")
		if fv.IsValid() && fv.CanInterface() {
			if pfxSet, ok := parseRpolPrefixSetValue(fv.Interface()); ok {
				return pfxSet, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "prefix-set" || strings.HasSuffix(k, ":prefix-set") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if pfxSet, ok := parseRpolPrefixSetValue(mv.Interface()); ok {
						return pfxSet, true
					}
				}
			}
		}
	}
	return "", false
}

func rpolPolicyResultToRouteOp(v ocbinds.E_OpenconfigRoutingPolicy_PolicyResultType) (string, bool) {
	switch v {
	case ocbinds.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE:
		return rpolRouteOpPermit, true
	case ocbinds.OpenconfigRoutingPolicy_PolicyResultType_REJECT_ROUTE:
		return rpolRouteOpDeny, true
	default:
		return "", false
	}
}

func rpolRouteOpToPolicyResult(v string) (ocbinds.E_OpenconfigRoutingPolicy_PolicyResultType, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case rpolRouteOpPermit:
		return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE, true
	case rpolRouteOpDeny:
		return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_REJECT_ROUTE, true
	default:
		return ocbinds.OpenconfigRoutingPolicy_PolicyResultType_UNSET, false
	}
}

func parseRpolStmtNameToUint16(stmtName string) (uint16, error) {
	s := strings.TrimSpace(stmtName)
	if s == "" {
		return 0, tlerr.InvalidArgsError{Format: "routing-policy statement name is empty"}
	}
	u64, err := strconv.ParseUint(s, 10, 16)
	if err != nil || u64 == 0 {
		return 0, tlerr.InvalidArgsError{Format: "routing-policy statement name must be an integer in range 1..65535"}
	}
	return uint16(u64), nil
}

func getRpolPrefixSetMode(cfgDb *db.DB, pfxSet string) (string, error) {
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: "PREFIX_SET"}, db.Key{Comp: []string{pfxSet}})
	if err != nil {
		return "", err
	}
	mode, ok := entry.Field["mode"]
	if !ok || strings.TrimSpace(mode) == "" {
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("PREFIX_SET %s missing mode field", pfxSet)}
	}
	return strings.ToLower(strings.TrimSpace(mode)), nil
}

func makeRpolRouteMapSetResMap(policyName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		rpolRouteMapSetTbl: {
			policyName: db.Value{Field: fields},
		},
	}
}

func makeRpolRouteMapResMap(policyName, stmtName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		rpolRouteMapTbl: {
			policyName + "|" + stmtName: db.Value{Field: fields},
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
		if reqInfo.targetPath == rpolTargetPolicyDefCfg {
			return makeRpolRouteMapSetResMap(reqInfo.policyName, nil), nil
		}
		switch reqInfo.targetPath {
		case rpolTargetStmtActionsCfg:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{rpolRouteOperationFld: ""}), nil
		case rpolTargetStmtCondMatchPfxCfg:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolMatchPrefixSetFld:     "",
				rpolMatchIpv6PrefixSetFld: "",
			}), nil
		}
		return res, tlerr.NotSupportedError{Format: "unsupported DELETE path under routing-policy mapping", Path: inParams.requestUri}
	}

	if reqInfo.targetPath == rpolTargetPolicyDefCfg && (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPolicyDefNameLeaf) {
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
	if reqInfo.targetPath == rpolTargetStmtActionsCfg && (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPolicyResultLeaf) {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		polRes, ok := parseRpolPolicyResultFromParam(inParams.param)
		if !ok {
			cfgObj := getRpolPolicyStmtActionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			if cfgObj == nil {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions config policy-result is missing in request payload"}
			}
			polRes = cfgObj.PolicyResult
		}
		routeOp, ok := rpolPolicyResultToRouteOp(polRes)
		if !ok {
			return res, tlerr.InvalidArgsError{Format: "unsupported routing-policy policy-result value"}
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{rpolRouteOperationFld: routeOp}), nil
	}
	if reqInfo.targetPath == rpolTargetStmtCondMatchPfxCfg && (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetLeaf) {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		pfxSet, ok := parseRpolPrefixSetFromParam(inParams.param)
		if !ok {
			cfgObj := getRpolPolicyStmtCondMatchPrefixSetCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			if cfgObj == nil || cfgObj.PrefixSet == nil || strings.TrimSpace(*cfgObj.PrefixSet) == "" {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement conditions match-prefix-set config prefix-set is missing in request payload"}
			}
			pfxSet = strings.TrimSpace(*cfgObj.PrefixSet)
		}
		cfgDb := inParams.dbs[db.ConfigDB]
		if cfgDb == nil {
			return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for routing-policy mapping"}
		}
		mode, err := getRpolPrefixSetMode(cfgDb, pfxSet)
		if err != nil {
			return res, err
		}
		fields := map[string]string{
			rpolMatchPrefixSetFld:     "",
			rpolMatchIpv6PrefixSetFld: "",
		}
		if mode == "ipv4" {
			fields[rpolMatchPrefixSetFld] = pfxSet
		} else if mode == "ipv6" {
			fields[rpolMatchIpv6PrefixSetFld] = pfxSet
		} else {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported PREFIX_SET mode %s for set %s", mode, pfxSet)}
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
	}
	return res, tlerr.NotSupportedError{Format: "unsupported leaf under routing-policy mapping", Path: inParams.requestUri}
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

	if reqInfo.targetPath == rpolTargetPolicyDefCfg {
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

	_, err = parseRpolStmtNameToUint16(reqInfo.stmtName)
	if err != nil {
		return err
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{reqInfo.policyName, reqInfo.stmtName}})
	if err != nil {
		return err
	}

	if reqInfo.targetPath == rpolTargetStmtActionsCfg {
		cfgObj := getRpolPolicyStmtActionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPolicyResultLeaf {
			routeOp, ok := entry.Field[rpolRouteOperationFld]
			if ok && strings.TrimSpace(routeOp) != "" {
				if polRes, conv := rpolRouteOpToPolicyResult(routeOp); conv {
					cfgObj.PolicyResult = polRes
				}
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtCondMatchPfxCfg {
		cfgObj := getRpolPolicyStmtCondMatchPrefixSetCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement conditions match-prefix-set config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetLeaf {
			if pfxSet, ok := entry.Field[rpolMatchPrefixSetFld]; ok && strings.TrimSpace(pfxSet) != "" {
				cfgObj.PrefixSet = ygot.String(strings.TrimSpace(pfxSet))
			} else if pfxSet6, ok := entry.Field[rpolMatchIpv6PrefixSetFld]; ok && strings.TrimSpace(pfxSet6) != "" {
				cfgObj.PrefixSet = ygot.String(strings.TrimSpace(pfxSet6))
			}
		}
		return nil
	}
	return tlerr.NotSupportedError{Format: "unsupported leaf under routing-policy mapping", Path: inParams.requestUri}
}
