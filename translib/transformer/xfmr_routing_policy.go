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
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	rpolRouteMapSetTbl                     = "ROUTE_MAP_SET"
	rpolRouteMapTbl                        = "ROUTE_MAP"
	rpolCommunitySetTbl                    = "COMMUNITY_SET"
	rpolExtCommunitySetTbl                 = "EXTENDED_COMMUNITY_SET"
	rpolPrefixSetTbl                       = "PREFIX_SET"
	rpolRouteOperationFld                  = "route_operation"
	rpolMatchPrefixSetFld                  = "match_prefix_set"
	rpolMatchIpv6PrefixSetFld              = "match_ipv6_prefix_set"
	rpolSetCommunityInlineFld              = "set_community_inline"
	rpolSetCommunityRefFld                 = "set_community_ref"
	rpolSetAsnFld                          = "set_asn"
	rpolSetRepeatAsnFld                    = "set_repeat_asn"
	rpolSetNextHopFld                      = "set_next_hop"
	rpolSetOriginFld                       = "set_origin"
	rpolSetLocalPrefFld                    = "set_local_pref"
	rpolSetMedFld                          = "set_med"
	rpolMatchMedFld                        = "match_med"
	rpolMatchOriginFld                     = "match_origin"
	rpolMatchLocalPrefFld                  = "match_local_pref"
	rpolMatchCommunityFld                  = "match_community"
	rpolMatchExtCommunityFld               = "match_ext_community"
	rpolRouteOpPermit                      = "permit"
	rpolRouteOpDeny                        = "deny"
	rpolTargetPolicyDefCfg                 = "policy-definition-config"
	rpolTargetStmtName                     = "statement-name"
	rpolTargetStmtActionsCfg               = "statement-actions-config"
	rpolTargetStmtCondMatchPfxCfg          = "statement-conditions-match-prefix-set-config"
	rpolTargetStmtBgpActionsCfg            = "statement-actions-bgp-actions-config"
	rpolTargetStmtBgpSetCommunity          = "statement-actions-bgp-set-community"
	rpolTargetStmtBgpSetCommunityInlineCfg = "statement-actions-bgp-set-community-inline-config"
	rpolTargetStmtBgpSetAsPathCfg          = "statement-actions-bgp-set-as-path-prepend-config"
	rpolTargetStmtBgpConditionsCfg         = "statement-conditions-bgp-conditions-config"
	rpolTargetCommunitySetName             = "community-set-name"
	rpolTargetRoot                         = "routing-policy-root"
	rpolPolicyDefNameLeaf                  = "name"
	rpolStmtNameLeaf                       = "name"
	rpolCommunitySetNameLeaf               = "community-set-name"
	rpolPolicyResultLeaf                   = "policy-result"
	rpolPrefixSetLeaf                      = "prefix-set"
	rpolAsnLeaf                            = "asn"
	rpolRepeatNLeaf                        = "repeat-n"
	rpolSetNextHopLeaf                     = "set-next-hop"
	rpolSetRouteOriginLeaf                 = "set-route-origin"
	rpolSetLocalPrefLeaf                   = "set-local-pref"
	rpolSetMedLeaf                         = "set-med"
	rpolMedEqLeaf                          = "med-eq"
	rpolOriginEqLeaf                       = "origin-eq"
	rpolLocalPrefEqLeaf                    = "local-pref-eq"
	rpolCommunitySetLeaf                   = "community-set"
	rpolExtCommunitySetLeaf                = "ext-community-set"
	rpolSetTypeFld                         = "set_type"
	rpolPrefixSetModeFld                   = "mode"
	rpolCommunityMemberFld                 = "community_member@"
	rpolMatchActionFld                     = "match_action"
	rpolSetTypeStandard                    = "STANDARD"
	rpolSetTypeExpanded                    = "EXPANDED"
	rpolMatchActionAny                     = "ANY"
	rpolMatchActionAll                     = "ALL"
	rpolMatchSetOptionsInvert              = "INVERT"
	rpolTargetExtCommunitySetName          = "ext-community-set-name"
	rpolExtCommunitySetNameLeaf            = "ext-community-set-name"
	rpolCommunityMemberLeaf                = "community-member"
	rpolExtCommunityMemberLeaf             = "ext-community-member"
	rpolMatchSetOptionsLeaf                = "match-set-options"
)

var rpolPolicyDefCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/config(?:/(name))?$`)
var rpolStmtNameReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/name$`)
var rpolStmtActionsCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/config(?:/(policy-result))?$`)
var rpolStmtCondMatchPfxCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions/match-prefix-set/config(?:/(prefix-set|match-set-options))?$`)
var rpolStmtBgpActionsCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(set-next-hop|set-route-origin|set-local-pref|set-med))?$`)
var rpolStmtBgpSetCommunityReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-community$`)
var rpolStmtBgpSetCommunityInlineCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-community/(?:[A-Za-z0-9_.-]+:)?inline/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(communities))?$`)
var rpolStmtBgpSetAsPathCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-as-path-prepend/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(asn|repeat-n))?$`)
var rpolStmtBgpConditionsCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions/(?:[A-Za-z0-9_.-]+:)?bgp-conditions/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(med-eq|origin-eq|local-pref-eq|community-set|ext-community-set))?$`)
var rpolContainerReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy(?:/.*)?$`)
var rpolCommunitySetReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?community-sets/(?:[A-Za-z0-9_.-]+:)?community-set\[(?:[A-Za-z0-9_.-]+:)?community-set-name=([^\]]+)\]/(?:(?:[A-Za-z0-9_.-]+:)?community-set-name|(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(community-set-name|community-member|match-set-options))?)$`)
var rpolExtCommunitySetReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?ext-community-sets/(?:[A-Za-z0-9_.-]+:)?ext-community-set\[(?:[A-Za-z0-9_.-]+:)?ext-community-set-name=([^\]]+)\]/(?:(?:[A-Za-z0-9_.-]+:)?ext-community-set-name|(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(ext-community-set-name|ext-community-member|match-set-options))?)$`)

type rpolReqInfo struct {
	policyName       string
	stmtName         string
	communitySetName string
	targetLeaf       string
	targetPath       string
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
	m = rpolStmtNameReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: rpolStmtNameLeaf,
			targetPath: rpolTargetStmtName,
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
	m = rpolStmtBgpActionsCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpActionsCfg,
		}, nil
	}
	m = rpolStmtBgpSetCommunityReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: "",
			targetPath: rpolTargetStmtBgpSetCommunity,
		}, nil
	}
	m = rpolStmtBgpSetCommunityInlineCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpSetCommunityInlineCfg,
		}, nil
	}
	m = rpolStmtBgpSetAsPathCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpSetAsPathCfg,
		}, nil
	}
	m = rpolStmtBgpConditionsCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpConditionsCfg,
		}, nil
	}
	m = rpolCommunitySetReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &rpolReqInfo{
			communitySetName: m[1],
			targetLeaf:       leaf,
			targetPath:       rpolTargetCommunitySetName,
		}, nil
	}
	m = rpolExtCommunitySetReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &rpolReqInfo{
			communitySetName: m[1],
			targetLeaf:       leaf,
			targetPath:       rpolTargetExtCommunitySetName,
		}, nil
	}
	// Any other node of the routing-policy subtree is served by the container
	// dispatch, which maps or populates every node the request touches in one
	// pass and lets translib return just the requested node. Without this,
	// intermediate containers such as /policy-definitions or a keyed
	// policy-definition would be rejected, since the annotation routes every
	// unannotated descendant of routing-policy here.
	if rpolContainerReqUriRe.MatchString(uri) {
		return &rpolReqInfo{targetPath: rpolTargetRoot}, nil
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

func getRpolPolicyStmtObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement {
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
	return stmtObj
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

func getRpolPolicyStmtBgpActionsCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config {
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
	return stmtObj.Actions.BgpActions.Config
}

func getRpolPolicyStmtBgpConditionsCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_Config {
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
	return stmtObj.Conditions.BgpConditions.Config
}

func getRpolCommunitySetObj(s *ygot.GoStruct, communitySetName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet {
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
	if deviceObj.RoutingPolicy.DefinedSets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets{}
	}
	if deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets{}
	}
	if deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.CommunitySets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.CommunitySets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets{}
	}

	if csObj, ok := deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.CommunitySets.CommunitySet[communitySetName]; ok && csObj != nil {
		ygot.BuildEmptyTree(csObj)
		return csObj
	}
	if !createIfMissing {
		return nil
	}
	csObj, err := deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.CommunitySets.NewCommunitySet(communitySetName)
	if err != nil || csObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(csObj)
	return csObj
}

func getRpolExtCommunitySetObj(s *ygot.GoStruct, communitySetName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_ExtCommunitySets_ExtCommunitySet {
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
	if deviceObj.RoutingPolicy.DefinedSets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets{}
	}
	if deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets{}
	}
	if deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.ExtCommunitySets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.ExtCommunitySets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_ExtCommunitySets{}
	}

	if ecObj, ok := deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.ExtCommunitySets.ExtCommunitySet[communitySetName]; ok && ecObj != nil {
		ygot.BuildEmptyTree(ecObj)
		return ecObj
	}
	if !createIfMissing {
		return nil
	}
	ecObj, err := deviceObj.RoutingPolicy.DefinedSets.BgpDefinedSets.ExtCommunitySets.NewExtCommunitySet(communitySetName)
	if err != nil || ecObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(ecObj)
	return ecObj
}

func getRpolPolicyStmtBgpSetAsPathPrependCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetAsPathPrepend_Config {
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
	return stmtObj.Actions.BgpActions.SetAsPathPrepend.Config
}

func getRpolPolicyStmtBgpSetCommunityCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config {
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
	if stmtObj.Actions.BgpActions.SetCommunity == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity{}
	}
	if stmtObj.Actions.BgpActions.SetCommunity.Config == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity.Config = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config{}
	}
	return stmtObj.Actions.BgpActions.SetCommunity.Config
}

func getRpolPolicyStmtBgpSetCommunityRefCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Reference_Config {
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
	if stmtObj.Actions.BgpActions.SetCommunity == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity{}
	}
	if stmtObj.Actions.BgpActions.SetCommunity.Reference == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity.Reference = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Reference{}
	}
	if stmtObj.Actions.BgpActions.SetCommunity.Reference.Config == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity.Reference.Config = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Reference_Config{}
	}
	return stmtObj.Actions.BgpActions.SetCommunity.Reference.Config
}

func getRpolPolicyStmtBgpSetCommunityInlineCfgObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config {
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
	if stmtObj.Actions.BgpActions.SetCommunity == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity{}
	}
	if stmtObj.Actions.BgpActions.SetCommunity.Inline == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity.Inline = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline{}
	}
	if stmtObj.Actions.BgpActions.SetCommunity.Inline.Config == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity.Inline.Config = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config{}
	}
	return stmtObj.Actions.BgpActions.SetCommunity.Inline.Config
}

func rpolSetCommunityInlineCommunitiesToSlice(cfgObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config) ([]string, error) {
	if cfgObj == nil || len(cfgObj.Communities) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(cfgObj.Communities))
	for _, c := range cfgObj.Communities {
		switch v := c.(type) {
		case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union_String:
			out = append(out, strings.TrimSpace(v.String))
		case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union_Uint32:
			out = append(out, strconv.FormatUint(uint64(v.Uint32), 10))
		case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union_E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY:
			if c, ok := rpolWellKnownStdCommunityToString(v.E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY); ok {
				out = append(out, c)
			} else {
				return nil, tlerr.InvalidArgsError{Format: "unsupported set-community inline well-known community in request payload"}
			}
		default:
			return nil, tlerr.InvalidArgsError{Format: "unsupported set-community inline communities value in request payload"}
		}
	}
	return out, nil
}

func rpolWellKnownStdCommunityToString(v ocbinds.E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY) (string, bool) {
	switch v {
	case ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NOPEER:
		return "NOPEER", true
	case ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_ADVERTISE:
		return "NO_ADVERTISE", true
	case ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_EXPORT:
		return "NO_EXPORT", true
	case ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_EXPORT_SUBCONFED:
		return "NO_EXPORT_SUBCONFED", true
	default:
		return "", false
	}
}

func rpolSetCommunityInlineEncode(communities []string, additive bool) string {
	if len(communities) == 0 {
		return ""
	}
	out := make([]string, 0, len(communities))
	for _, c := range communities {
		v := strings.TrimSpace(c)
		if v == "" {
			continue
		}
		if additive {
			v += " additive"
		}
		out = append(out, v)
	}
	return strings.Join(out, ", ")
}

func rpolSetCommunityInlineDecode(in string) ([]string, bool) {
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	additive := false
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if strings.HasSuffix(v, " additive") {
			additive = true
			v = strings.TrimSpace(strings.TrimSuffix(v, " additive"))
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out, additive
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

func parseRpolStmtNameValue(v interface{}) (string, bool) {
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

func parseRpolStmtNameFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if stmt, ok := parseRpolStmtNameValue(param); ok {
		return stmt, true
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
			if stmt, ok := parseRpolStmtNameValue(fv.Interface()); ok {
				return stmt, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "name" || strings.HasSuffix(k, ":name") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if stmt, ok := parseRpolStmtNameValue(mv.Interface()); ok {
						return stmt, true
					}
				}
			}
		}
	}
	return "", false
}

func parseRpolCommunitySetNameValue(v interface{}) (string, bool) {
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

func parseRpolCommunitySetNameFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if name, ok := parseRpolCommunitySetNameValue(param); ok {
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
		fv := rv.FieldByName("CommunitySetName")
		if fv.IsValid() && fv.CanInterface() {
			if name, ok := parseRpolCommunitySetNameValue(fv.Interface()); ok {
				return name, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "community-set-name" || strings.HasSuffix(k, ":community-set-name") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if name, ok := parseRpolCommunitySetNameValue(mv.Interface()); ok {
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

func parseRpolAsnValue(v interface{}) (*uint32, bool) {
	var out uint64
	switch val := v.(type) {
	case uint32:
		out = uint64(val)
	case uint64:
		out = val
	case int:
		if val < 0 {
			return nil, false
		}
		out = uint64(val)
	case float64:
		if val < 0 || val != float64(uint64(val)) {
			return nil, false
		}
		out = uint64(val)
	case string:
		p := strings.TrimSpace(val)
		if p == "" {
			return nil, false
		}
		u, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, false
		}
		out = u
	case *uint32:
		if val == nil {
			return nil, false
		}
		x := *val
		return &x, true
	default:
		return nil, false
	}
	if out > uint64(^uint32(0)) {
		return nil, false
	}
	x := uint32(out)
	return &x, true
}

func parseRpolAsnFromParam(param interface{}) (*uint32, bool) {
	if param == nil {
		return nil, false
	}
	if asn, ok := parseRpolAsnValue(param); ok {
		return asn, true
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
		fv := rv.FieldByName("Asn")
		if fv.IsValid() && fv.CanInterface() {
			if asn, ok := parseRpolAsnValue(fv.Interface()); ok {
				return asn, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "asn" || strings.HasSuffix(k, ":asn") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if asn, ok := parseRpolAsnValue(mv.Interface()); ok {
						return asn, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseRpolRepeatNValue(v interface{}) (*uint8, bool) {
	var out uint64
	switch val := v.(type) {
	case uint8:
		out = uint64(val)
	case uint16:
		out = uint64(val)
	case uint32:
		out = uint64(val)
	case uint64:
		out = val
	case int:
		if val < 0 {
			return nil, false
		}
		out = uint64(val)
	case float64:
		if val < 0 || val != float64(uint64(val)) {
			return nil, false
		}
		out = uint64(val)
	case string:
		p := strings.TrimSpace(val)
		if p == "" {
			return nil, false
		}
		u, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return nil, false
		}
		out = u
	case *uint8:
		if val == nil {
			return nil, false
		}
		x := *val
		return &x, true
	default:
		return nil, false
	}
	if out > uint64(^uint8(0)) {
		return nil, false
	}
	x := uint8(out)
	return &x, true
}

func parseRpolRepeatNFromParam(param interface{}) (*uint8, bool) {
	if param == nil {
		return nil, false
	}
	if rep, ok := parseRpolRepeatNValue(param); ok {
		return rep, true
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
		fv := rv.FieldByName("RepeatN")
		if fv.IsValid() && fv.CanInterface() {
			if rep, ok := parseRpolRepeatNValue(fv.Interface()); ok {
				return rep, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "repeat-n" || strings.HasSuffix(k, ":repeat-n") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if rep, ok := parseRpolRepeatNValue(mv.Interface()); ok {
						return rep, true
					}
				}
			}
		}
	}
	return nil, false
}

func parseRpolUint32Value(v interface{}) (*uint32, bool) {
	switch val := v.(type) {
	case uint32:
		out := val
		return &out, true
	case uint64:
		if val <= uint64(^uint32(0)) {
			out := uint32(val)
			return &out, true
		}
	case int:
		if val >= 0 && uint64(val) <= uint64(^uint32(0)) {
			out := uint32(val)
			return &out, true
		}
	case float64:
		if val >= 0 && val == float64(uint64(val)) && uint64(val) <= uint64(^uint32(0)) {
			out := uint32(val)
			return &out, true
		}
	case string:
		p := strings.TrimSpace(val)
		if p == "" {
			return nil, false
		}
		u, err := strconv.ParseUint(p, 10, 32)
		if err == nil {
			out := uint32(u)
			return &out, true
		}
	case *uint32:
		if val != nil {
			out := *val
			return &out, true
		}
	}
	return nil, false
}

func parseRpolStringValue(v interface{}) (string, bool) {
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

func parseRpolBgpSetNextHopValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_Union_String:
		return parseRpolStringValue(val.String)
	case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_Union_E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop:
		if val.E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_SELF {
			return "self", true
		}
	case ocbinds.E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop:
		if val == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_SELF {
			return "self", true
		}
	}
	return parseRpolStringValue(v)
}

func parseRpolBgpSetMedValue(v interface{}) (string, bool) {
	if u, ok := parseRpolUint32Value(v); ok {
		return strconv.FormatUint(uint64(*u), 10), true
	}
	switch val := v.(type) {
	case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_Uint32:
		return strconv.FormatUint(uint64(val.Uint32), 10), true
	case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_String:
		s := strings.TrimSpace(val.String)
		return s, s != ""
	case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed:
		if val.E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_IGP {
			return "igp", true
		}
	case ocbinds.E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed:
		if val == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_IGP {
			return "igp", true
		}
	}
	return "", false
}

func parseRpolBgpOriginValue(v interface{}) (ocbinds.E_OpenconfigBgpPolicy_BgpOriginAttrType, bool) {
	switch val := v.(type) {
	case ocbinds.E_OpenconfigBgpPolicy_BgpOriginAttrType:
		switch val {
		case ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_IGP,
			ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_EGP,
			ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_INCOMPLETE:
			return val, true
		}
	case string:
		switch strings.ToUpper(strings.TrimSpace(val)) {
		case "IGP":
			return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_IGP, true
		case "EGP":
			return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_EGP, true
		case "INCOMPLETE":
			return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_INCOMPLETE, true
		}
	}
	return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_UNSET, false
}

func rpolBgpOriginToDb(v ocbinds.E_OpenconfigBgpPolicy_BgpOriginAttrType) (string, bool) {
	switch v {
	case ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_IGP:
		return "igp", true
	case ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_EGP:
		return "egp", true
	case ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_INCOMPLETE:
		return "incomplete", true
	default:
		return "", false
	}
}

func rpolDbToBgpOrigin(v string) (ocbinds.E_OpenconfigBgpPolicy_BgpOriginAttrType, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "igp":
		return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_IGP, true
	case "egp":
		return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_EGP, true
	case "incomplete":
		return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_INCOMPLETE, true
	default:
		return ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_UNSET, false
	}
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

func makeRpolCommunitySetResMap(communitySetName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		rpolCommunitySetTbl: {
			communitySetName: db.Value{Field: fields},
		},
	}
}

func makeRpolExtCommunitySetResMap(communitySetName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		rpolExtCommunitySetTbl: {
			communitySetName: db.Value{Field: fields},
		},
	}
}

func makeRpolPrefixSetResMap(prefixSetName string, fields map[string]string) map[string]map[string]db.Value {
	return map[string]map[string]db.Value{
		rpolPrefixSetTbl: {
			prefixSetName: db.Value{Field: fields},
		},
	}
}

func mergeDbResMap(dst map[string]map[string]db.Value, src map[string]map[string]db.Value) map[string]map[string]db.Value {
	if dst == nil {
		dst = make(map[string]map[string]db.Value)
	}
	for tbl, entries := range src {
		if _, ok := dst[tbl]; !ok {
			dst[tbl] = make(map[string]db.Value)
		}
		for key, val := range entries {
			dst[tbl][key] = val
		}
	}
	return dst
}

// mergeDbResMapFields merges src into dst field by field. mergeDbResMap
// replaces the whole row value, which drops fields already collected for the
// same table and key when one request maps several leaves onto one row.
func mergeDbResMapFields(dst map[string]map[string]db.Value, src map[string]map[string]db.Value) map[string]map[string]db.Value {
	if dst == nil {
		dst = make(map[string]map[string]db.Value)
	}
	for tbl, entries := range src {
		if _, ok := dst[tbl]; !ok {
			dst[tbl] = make(map[string]db.Value)
		}
		for key, val := range entries {
			cur, ok := dst[tbl][key]
			if !ok || cur.Field == nil {
				fields := make(map[string]string, len(val.Field))
				for f, v := range val.Field {
					fields[f] = v
				}
				dst[tbl][key] = db.Value{Field: fields}
				continue
			}
			for f, v := range val.Field {
				cur.Field[f] = v
			}
			dst[tbl][key] = cur
		}
	}
	return dst
}

// rpolCommunitySetSeedFields returns the set_type and match_action fields to
// write alongside whatever a community-set request carries. frrcfgd only renders
// a community list once set_type, match_action and at least one member are all
// present in the row, so a request that omits one of them still seeds it: the
// members dictate the type, because FRR only accepts a regexp member on an
// expanded list, and match_action follows the YANG default of ANY. An already
// configured value is left untouched, so that naming a set neither turns an
// expanded list into a standard one nor resets a match modifier.
func rpolCommunitySetSeedFields(cfgDb *db.DB, table, setName, memberType string) map[string]string {
	seeds := map[string]string{}
	existing := db.Value{Field: map[string]string{}}
	if cfgDb != nil {
		if entry, err := cfgDb.GetEntry(&db.TableSpec{Name: table}, db.Key{Comp: []string{setName}}); err == nil {
			existing = entry
		}
	}
	if memberType != "" {
		seeds[rpolSetTypeFld] = memberType
	} else if strings.TrimSpace(existing.Field[rpolSetTypeFld]) == "" {
		seeds[rpolSetTypeFld] = rpolSetTypeStandard
	}
	if strings.TrimSpace(existing.Field[rpolMatchActionFld]) == "" {
		seeds[rpolMatchActionFld] = rpolMatchActionAny
	}
	return seeds
}

// rpolCommunitySetMembersToSlice converts a community-set member leaf-list into
// its CONFIG_DB spellings. frrcfgd feeds this field straight into
// "bgp community-list ... permit", so a well-known member is spelled with the
// keyword FRR parses rather than with the OpenConfig identity name.
func rpolCommunitySetMembersToSlice(cfgObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config) ([]string, error) {
	if cfgObj == nil || len(cfgObj.CommunityMember) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(cfgObj.CommunityMember))
	for _, m := range cfgObj.CommunityMember {
		switch v := m.(type) {
		case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config_CommunityMember_Union_String:
			if s := strings.TrimSpace(v.String); s != "" {
				out = append(out, s)
			}
		case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config_CommunityMember_Union_Uint32:
			out = append(out, fmt.Sprintf("%d:%d", v.Uint32>>16, v.Uint32&0xffff))
		case *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config_CommunityMember_Union_E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY:
			c, ok := rpolWellKnownStdCommunityToFrrString(v.E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY)
			if !ok {
				return nil, tlerr.InvalidArgsError{Format: "unsupported routing-policy community-set well-known community-member in request payload"}
			}
			out = append(out, c)
		default:
			return nil, tlerr.InvalidArgsError{Format: "unsupported routing-policy community-set community-member value in request payload"}
		}
	}
	return out, nil
}

func rpolExtCommunitySetMembersToSlice(cfgObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_ExtCommunitySets_ExtCommunitySet_Config) []string {
	if cfgObj == nil || len(cfgObj.ExtCommunityMember) == 0 {
		return nil
	}
	out := make([]string, 0, len(cfgObj.ExtCommunityMember))
	for _, m := range cfgObj.ExtCommunityMember {
		if v := strings.TrimSpace(m); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// rpolFrrWellKnownCommunities maps the well-known community identities onto the
// keywords FRR's community parser accepts.
var rpolFrrWellKnownCommunities = map[ocbinds.E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY]string{
	ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_EXPORT:           "no-export",
	ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_ADVERTISE:        "no-advertise",
	ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NO_EXPORT_SUBCONFED: "local-AS",
	ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_NOPEER:              "no-peer",
}

func rpolWellKnownStdCommunityToFrrString(v ocbinds.E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY) (string, bool) {
	c, ok := rpolFrrWellKnownCommunities[v]
	return c, ok
}

func rpolFrrStringToWellKnownStdCommunity(v string) (ocbinds.E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY, bool) {
	for enum, keyword := range rpolFrrWellKnownCommunities {
		if strings.EqualFold(keyword, v) {
			return enum, true
		}
	}
	return ocbinds.OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY_UNSET, false
}

// rpolStdCommunityMemberRe matches the members a standard community-list can
// hold, namely a plain community value in AA:NN or 32-bit form.
var rpolStdCommunityMemberRe = regexp.MustCompile(`^(?:[0-9]+:[0-9]+|[0-9]+)$`)

// rpolExtCommunityStdMarkers are the member markers frrcfgd renders as "rt" and
// "soo" on a standard extcommunity-list. It drops anything else.
var rpolExtCommunityStdMarkers = []string{"route-target:", "route-origin:"}

// rpolCommunitySetTypeFromMembers reports the community-list type FRR needs to
// hold the given members.
func rpolCommunitySetTypeFromMembers(members []string) string {
	for _, m := range members {
		if rpolStdCommunityMemberRe.MatchString(m) {
			continue
		}
		if _, ok := rpolFrrStringToWellKnownStdCommunity(m); ok {
			continue
		}
		if strings.EqualFold(m, "internet") || strings.EqualFold(m, "graceful-shutdown") {
			continue
		}
		return rpolSetTypeExpanded
	}
	return rpolSetTypeStandard
}

func rpolExtCommunitySetTypeFromMembers(members []string) string {
	for _, m := range members {
		marked := false
		for _, marker := range rpolExtCommunityStdMarkers {
			if strings.HasPrefix(m, marker) {
				marked = true
				break
			}
		}
		if !marked {
			return rpolSetTypeExpanded
		}
	}
	return rpolSetTypeStandard
}

// rpolMatchSetOptionsToMatchAction maps the OpenConfig match modifier onto the
// CONFIG_DB match_action, which decides whether FRR gets one permit line per
// member or a single line holding all of them.
func rpolMatchSetOptionsToMatchAction(v ocbinds.E_OpenconfigRoutingPolicy_MatchSetOptionsType) (string, error) {
	switch v {
	case ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_ANY:
		return rpolMatchActionAny, nil
	case ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_ALL:
		return rpolMatchActionAll, nil
	case ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_INVERT:
		return "", tlerr.NotSupportedError{Format: "routing-policy match-set-options INVERT has no CONFIG_DB mapping"}
	default:
		return "", tlerr.InvalidArgsError{Format: "unsupported routing-policy match-set-options value in request payload"}
	}
}

// parseRpolMatchSetOptionsRestrictedFromParam reads the restricted match
// modifier out of a leaf targeted request payload. The prefix-set condition uses
// the restricted type, which allows only ANY and INVERT.
func parseRpolMatchSetOptionsRestrictedFromParam(param interface{}) (ocbinds.E_OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType, bool) {
	switch v := param.(type) {
	case ocbinds.E_OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType:
		return v, true
	case *ocbinds.E_OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType:
		if v != nil {
			return *v, true
		}
	case string:
		switch strings.ToUpper(strings.TrimSpace(v)) {
		case rpolMatchActionAny:
			return ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_ANY, true
		case rpolMatchSetOptionsInvert:
			return ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_INVERT, true
		}
	}
	return ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_UNSET, false
}

// validateRpolMatchPfxSetOptions enforces the one match modifier a prefix-set
// condition can carry. FRR matches a prefix-list with ANY semantics and
// ROUTE_MAP has no field to record anything else, so ANY is accepted and
// mapped to nothing, while INVERT is refused rather than silently ignored.
func validateRpolMatchPfxSetOptions(inParams XfmrParams, reqInfo *rpolReqInfo) error {
	opts := ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_UNSET
	if reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
		if v, ok := parseRpolMatchSetOptionsRestrictedFromParam(inParams.param); ok {
			opts = v
		}
	}
	if opts == ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_UNSET {
		if cfgObj := getRpolPolicyStmtCondMatchPrefixSetCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false); cfgObj != nil {
			opts = cfgObj.MatchSetOptions
		}
	}
	switch opts {
	case ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_UNSET,
		ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_ANY:
		return nil
	}
	return tlerr.NotSupportedError{Format: "routing-policy match-prefix-set match-set-options supports only ANY"}
}

func rpolMatchActionToMatchSetOptions(v string) (ocbinds.E_OpenconfigRoutingPolicy_MatchSetOptionsType, bool) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case rpolMatchActionAny:
		return ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_ANY, true
	case rpolMatchActionAll:
		return ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_ALL, true
	}
	return ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET, false
}

func rpolEncodeCommunityMembers(members []string) string {
	return strings.Join(members, ",")
}

func rpolDecodeCommunityMembers(in string) []string {
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// The transformer callbacks are plain functions so that the container dispatch
// can replay them per node; a func-typed package variable that referred to
// itself through a helper would be an initialization cycle.
var YangToDb_routing_policy_xfmr SubTreeXfmrYangToDb = rpolYangToDbXfmr

func rpolYangToDbXfmr(inParams XfmrParams) (res map[string]map[string]db.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("routing-policy transformer panic in SET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			res = make(map[string]map[string]db.Value)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy transformer panic in SET path: %v", r)}
		}
	}()
	res = make(map[string]map[string]db.Value)

	reqInfo, err := parseRpolRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		if reqInfo.targetPath == rpolTargetRoot {
			return res, tlerr.NotSupportedError{Format: "DELETE is not supported at routing-policy container level; delete mapped child paths or list instances instead", Path: inParams.requestUri}
		}
		if reqInfo.targetPath == rpolTargetPolicyDefCfg {
			return makeRpolRouteMapSetResMap(reqInfo.policyName, nil), nil
		}
		switch reqInfo.targetPath {
		case rpolTargetCommunitySetName:
			switch reqInfo.targetLeaf {
			case rpolCommunityMemberLeaf:
				return makeRpolCommunitySetResMap(reqInfo.communitySetName, map[string]string{rpolCommunityMemberFld: ""}), nil
			case rpolMatchSetOptionsLeaf:
				return makeRpolCommunitySetResMap(reqInfo.communitySetName, map[string]string{rpolMatchActionFld: ""}), nil
			default:
				return makeRpolCommunitySetResMap(reqInfo.communitySetName, nil), nil
			}
		case rpolTargetExtCommunitySetName:
			switch reqInfo.targetLeaf {
			case rpolExtCommunityMemberLeaf:
				return makeRpolExtCommunitySetResMap(reqInfo.communitySetName, map[string]string{rpolCommunityMemberFld: ""}), nil
			case rpolMatchSetOptionsLeaf:
				return makeRpolExtCommunitySetResMap(reqInfo.communitySetName, map[string]string{rpolMatchActionFld: ""}), nil
			default:
				return makeRpolExtCommunitySetResMap(reqInfo.communitySetName, nil), nil
			}
		case rpolTargetStmtName:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, nil), nil
		case rpolTargetStmtActionsCfg:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{rpolRouteOperationFld: ""}), nil
		case rpolTargetStmtCondMatchPfxCfg:
			if reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
				// Nothing is stored for the match modifier, so its removal
				// must not take the prefix-set fields with it.
				return res, nil
			}
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolMatchPrefixSetFld:     "",
				rpolMatchIpv6PrefixSetFld: "",
			}), nil
		case rpolTargetStmtBgpActionsCfg:
			fields := map[string]string{}
			switch reqInfo.targetLeaf {
			case "":
				fields[rpolSetNextHopFld] = ""
				fields[rpolSetOriginFld] = ""
				fields[rpolSetLocalPrefFld] = ""
				fields[rpolSetMedFld] = ""
			case rpolSetNextHopLeaf:
				fields[rpolSetNextHopFld] = ""
			case rpolSetRouteOriginLeaf:
				fields[rpolSetOriginFld] = ""
			case rpolSetLocalPrefLeaf:
				fields[rpolSetLocalPrefFld] = ""
			case rpolSetMedLeaf:
				fields[rpolSetMedFld] = ""
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /actions/bgp-actions/config or mapped leaves under it", Path: inParams.requestUri}
			}
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
		case rpolTargetStmtBgpSetCommunity:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolSetCommunityInlineFld: "",
				rpolSetCommunityRefFld:    "",
			}), nil
		case rpolTargetStmtBgpSetCommunityInlineCfg:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolSetCommunityInlineFld: "",
			}), nil
		case rpolTargetStmtBgpSetAsPathCfg:
			fields := map[string]string{}
			switch reqInfo.targetLeaf {
			case "", rpolAsnLeaf:
				fields[rpolSetAsnFld] = ""
				fields[rpolSetRepeatAsnFld] = ""
			case rpolRepeatNLeaf:
				fields[rpolSetRepeatAsnFld] = ""
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /actions/bgp-actions/set-as-path-prepend/config, /actions/bgp-actions/set-as-path-prepend/config/asn, or /actions/bgp-actions/set-as-path-prepend/config/repeat-n", Path: inParams.requestUri}
			}
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
		case rpolTargetStmtBgpConditionsCfg:
			fields := map[string]string{}
			switch reqInfo.targetLeaf {
			case "":
				fields[rpolMatchMedFld] = ""
				fields[rpolMatchOriginFld] = ""
				fields[rpolMatchLocalPrefFld] = ""
				fields[rpolMatchCommunityFld] = ""
				fields[rpolMatchExtCommunityFld] = ""
			case rpolMedEqLeaf:
				fields[rpolMatchMedFld] = ""
			case rpolOriginEqLeaf:
				fields[rpolMatchOriginFld] = ""
			case rpolLocalPrefEqLeaf:
				fields[rpolMatchLocalPrefFld] = ""
			case rpolCommunitySetLeaf:
				fields[rpolMatchCommunityFld] = ""
			case rpolExtCommunitySetLeaf:
				fields[rpolMatchExtCommunityFld] = ""
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /conditions/bgp-conditions/config or mapped leaves under it", Path: inParams.requestUri}
			}
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
		}
		return res, tlerr.NotSupportedError{Format: "unsupported DELETE path under routing-policy mapping", Path: inParams.requestUri}
	}

	if reqInfo.targetPath == rpolTargetRoot {
		wrappedRoot, wrapErr := wrapRpolRootAsDevice(inParams.ygRoot)
		if wrapErr != nil {
			return res, wrapErr
		}
		if unmapped := findRpolUnmappedPayloadLeaves(wrappedRoot); len(unmapped) > 0 {
			return res, tlerr.NotSupportedError{
				Format: "routing-policy container payload carries leaves that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
				Path:   inParams.requestUri,
			}
		}
		res, err = buildRpolContainerResMap(inParams, wrappedRoot)
		if err != nil {
			return res, err
		}
		if len(res) == 0 {
			return res, tlerr.InvalidArgsError{Format: "routing-policy container payload does not include currently supported mapped fields"}
		}
		if inParams.invokeCRUSubtreeOnce != nil {
			// The whole subtree is mapped in this call, so translib must not
			// invoke this transformer again for the child nodes.
			*inParams.invokeCRUSubtreeOnce = true
		}
		return res, nil
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
	if reqInfo.targetPath == rpolTargetStmtName && reqInfo.targetLeaf == rpolStmtNameLeaf {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		if stmtName, ok := parseRpolStmtNameFromParam(inParams.param); ok && stmtName != reqInfo.stmtName {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("statement name payload (%s) does not match key (%s)", stmtName, reqInfo.stmtName)}
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{}), nil
	}
	if reqInfo.targetPath == rpolTargetCommunitySetName {
		if name, ok := parseRpolCommunitySetNameFromParam(inParams.param); ok && name != reqInfo.communitySetName {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("community-set-name payload (%s) does not match key (%s)", name, reqInfo.communitySetName)}
		}
		var cfgObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config
		if csObj := getRpolCommunitySetObj(inParams.ygRoot, reqInfo.communitySetName, false); csObj != nil {
			cfgObj = csObj.Config
		}
		if cfgObj != nil && cfgObj.CommunitySetName != nil {
			if name := strings.TrimSpace(*cfgObj.CommunitySetName); name != "" && name != reqInfo.communitySetName {
				return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("community-set-name payload (%s) does not match key (%s)", name, reqInfo.communitySetName)}
			}
		}

		fields := map[string]string{}
		memberType := ""
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunityMemberLeaf {
			members, err := rpolCommunitySetMembersToSlice(cfgObj)
			if err != nil {
				return res, err
			}
			if len(members) == 0 && reqInfo.targetLeaf == rpolCommunityMemberLeaf {
				return res, tlerr.InvalidArgsError{Format: "routing-policy community-set config community-member is missing in request payload"}
			}
			if len(members) > 0 {
				fields[rpolCommunityMemberFld] = rpolEncodeCommunityMembers(members)
				memberType = rpolCommunitySetTypeFromMembers(members)
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
			matchOpts := ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET
			if cfgObj != nil {
				matchOpts = cfgObj.MatchSetOptions
			}
			if matchOpts == ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET && reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
				return res, tlerr.InvalidArgsError{Format: "routing-policy community-set config match-set-options is missing in request payload"}
			}
			if matchOpts != ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET {
				matchAction, err := rpolMatchSetOptionsToMatchAction(matchOpts)
				if err != nil {
					return res, err
				}
				fields[rpolMatchActionFld] = matchAction
			}
		}
		for fld, val := range rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolCommunitySetTbl, reqInfo.communitySetName, memberType) {
			if _, ok := fields[fld]; !ok {
				fields[fld] = val
			}
		}
		return makeRpolCommunitySetResMap(reqInfo.communitySetName, fields), nil
	}
	if reqInfo.targetPath == rpolTargetExtCommunitySetName {
		var cfgObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_ExtCommunitySets_ExtCommunitySet_Config
		if ecObj := getRpolExtCommunitySetObj(inParams.ygRoot, reqInfo.communitySetName, false); ecObj != nil {
			cfgObj = ecObj.Config
		}
		if cfgObj != nil && cfgObj.ExtCommunitySetName != nil {
			if name := strings.TrimSpace(*cfgObj.ExtCommunitySetName); name != "" && name != reqInfo.communitySetName {
				return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("ext-community-set-name payload (%s) does not match key (%s)", name, reqInfo.communitySetName)}
			}
		}

		fields := map[string]string{}
		memberType := ""
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolExtCommunityMemberLeaf {
			members := rpolExtCommunitySetMembersToSlice(cfgObj)
			if len(members) == 0 && reqInfo.targetLeaf == rpolExtCommunityMemberLeaf {
				return res, tlerr.InvalidArgsError{Format: "routing-policy ext-community-set config ext-community-member is missing in request payload"}
			}
			if len(members) > 0 {
				fields[rpolCommunityMemberFld] = rpolEncodeCommunityMembers(members)
				memberType = rpolExtCommunitySetTypeFromMembers(members)
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
			matchOpts := ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET
			if cfgObj != nil {
				matchOpts = cfgObj.MatchSetOptions
			}
			if matchOpts == ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET && reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
				return res, tlerr.InvalidArgsError{Format: "routing-policy ext-community-set config match-set-options is missing in request payload"}
			}
			if matchOpts != ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsType_UNSET {
				matchAction, err := rpolMatchSetOptionsToMatchAction(matchOpts)
				if err != nil {
					return res, err
				}
				fields[rpolMatchActionFld] = matchAction
			}
		}
		for fld, val := range rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolExtCommunitySetTbl, reqInfo.communitySetName, memberType) {
			if _, ok := fields[fld]; !ok {
				fields[fld] = val
			}
		}
		return makeRpolExtCommunitySetResMap(reqInfo.communitySetName, fields), nil
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
	if reqInfo.targetPath == rpolTargetStmtCondMatchPfxCfg && (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetLeaf || reqInfo.targetLeaf == rpolMatchSetOptionsLeaf) {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		if err := validateRpolMatchPfxSetOptions(inParams, reqInfo); err != nil {
			return res, err
		}
		if reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
			// Accepted above and mapped to no field, so there is nothing to write.
			return res, nil
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
		pfxSetAutoCreate := false
		if err != nil {
			// If PREFIX_SET doesn't exist yet, auto-create a minimal IPv4 entry so
			// dependent match-prefix-set writes can proceed.
			mode = "ipv4"
			pfxSetAutoCreate = true
		}
		fields := map[string]string{}
		if mode == "ipv4" {
			fields[rpolMatchPrefixSetFld] = pfxSet
		} else if mode == "ipv6" {
			fields[rpolMatchIpv6PrefixSetFld] = pfxSet
		} else {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported PREFIX_SET mode %s for set %s", mode, pfxSet)}
		}
		res = makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields)
		if pfxSetAutoCreate {
			res = mergeDbResMap(res, makeRpolPrefixSetResMap(pfxSet, map[string]string{
				rpolPrefixSetModeFld: "IPv4",
			}))
		}
		return res, nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpActionsCfg {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		cfgObj := getRpolPolicyStmtBgpActionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
		fields := map[string]string{}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetNextHopLeaf {
			if nh, ok := parseRpolBgpSetNextHopValue(inParams.param); ok {
				fields[rpolSetNextHopFld] = nh
			} else if cfgObj != nil {
				if nh, ok := parseRpolBgpSetNextHopValue(cfgObj.SetNextHop); ok {
					fields[rpolSetNextHopFld] = nh
				}
			}
			if reqInfo.targetLeaf == rpolSetNextHopLeaf {
				if _, ok := fields[rpolSetNextHopFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-next-hop is missing in request payload"}
				}
			}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetRouteOriginLeaf {
			if origin, ok := parseRpolBgpOriginValue(inParams.param); ok {
				if originDb, conv := rpolBgpOriginToDb(origin); conv {
					fields[rpolSetOriginFld] = originDb
				}
			} else if cfgObj != nil {
				if originDb, conv := rpolBgpOriginToDb(cfgObj.SetRouteOrigin); conv {
					fields[rpolSetOriginFld] = originDb
				}
			}
			if reqInfo.targetLeaf == rpolSetRouteOriginLeaf {
				if _, ok := fields[rpolSetOriginFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-route-origin is missing in request payload"}
				}
			}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetLocalPrefLeaf {
			if lp, ok := parseRpolUint32Value(inParams.param); ok {
				fields[rpolSetLocalPrefFld] = strconv.FormatUint(uint64(*lp), 10)
			} else if cfgObj != nil && cfgObj.SetLocalPref != nil {
				fields[rpolSetLocalPrefFld] = strconv.FormatUint(uint64(*cfgObj.SetLocalPref), 10)
			}
			if reqInfo.targetLeaf == rpolSetLocalPrefLeaf {
				if _, ok := fields[rpolSetLocalPrefFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-local-pref is missing in request payload"}
				}
			}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetMedLeaf {
			if med, ok := parseRpolBgpSetMedValue(inParams.param); ok {
				fields[rpolSetMedFld] = med
			} else if cfgObj != nil {
				if med, ok := parseRpolBgpSetMedValue(cfgObj.SetMed); ok {
					fields[rpolSetMedFld] = med
				}
			}
			if reqInfo.targetLeaf == rpolSetMedLeaf {
				if _, ok := fields[rpolSetMedFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-med is missing in request payload"}
				}
			}
		}

		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config payload missing supported fields (set-next-hop/set-route-origin/set-local-pref/set-med)"}
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunity {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		cfgObj := getRpolPolicyStmtBgpSetCommunityCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
		if cfgObj == nil {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community config is missing in request payload"}
		}

		if cfgObj.Options != ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_UNSET &&
			cfgObj.Options != ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community config options only supports ADD"}
		}

		switch cfgObj.Method {
		case ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_REFERENCE:
			refCfgObj := getRpolPolicyStmtBgpSetCommunityRefCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			if refCfgObj == nil || refCfgObj.CommunitySetRef == nil || strings.TrimSpace(*refCfgObj.CommunitySetRef) == "" {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community reference config community-set-ref is missing in request payload"}
			}
			refName := strings.TrimSpace(*refCfgObj.CommunitySetRef)
			res = makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolSetCommunityInlineFld: "",
				rpolSetCommunityRefFld:    refName,
			})
			res = mergeDbResMap(res, makeRpolCommunitySetResMap(refName,
				rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolCommunitySetTbl, refName, "")))
			return res, nil
		case ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE:
			inlineCfgObj := getRpolPolicyStmtBgpSetCommunityInlineCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			communities, err := rpolSetCommunityInlineCommunitiesToSlice(inlineCfgObj)
			if err != nil {
				return res, err
			}
			if len(communities) == 0 {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community inline config communities is missing in request payload"}
			}
			inlineValue := rpolSetCommunityInlineEncode(communities, cfgObj.Options == ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD)
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolSetCommunityInlineFld: inlineValue,
			}), nil
		default:
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community config method is missing or unsupported"}
		}
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunityInlineCfg {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		inlineCfgObj := getRpolPolicyStmtBgpSetCommunityInlineCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
		communities, err := rpolSetCommunityInlineCommunitiesToSlice(inlineCfgObj)
		if err != nil {
			return res, err
		}
		if len(communities) == 0 {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community inline config communities is missing in request payload"}
		}
		inlineValue := rpolSetCommunityInlineEncode(communities, false)
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
			rpolSetCommunityInlineFld: inlineValue,
		}), nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetAsPathCfg && (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolAsnLeaf || reqInfo.targetLeaf == rpolRepeatNLeaf) {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}

		var asn *uint32
		var repeatN *uint8
		var asnFromPayload, repeatFromPayload bool

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolAsnLeaf {
			asn, asnFromPayload = parseRpolAsnFromParam(inParams.param)
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolRepeatNLeaf {
			repeatN, repeatFromPayload = parseRpolRepeatNFromParam(inParams.param)
		}
		if reqInfo.targetLeaf == "" {
			cfgObj := getRpolPolicyStmtBgpSetAsPathPrependCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			if cfgObj != nil {
				if !asnFromPayload && cfgObj.Asn != nil {
					asn = cfgObj.Asn
					asnFromPayload = true
				}
				if !repeatFromPayload && cfgObj.RepeatN != nil {
					repeatN = cfgObj.RepeatN
					repeatFromPayload = true
				}
			}
		}
		if reqInfo.targetLeaf == rpolAsnLeaf && !asnFromPayload {
			cfgObj := getRpolPolicyStmtBgpSetAsPathPrependCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			if cfgObj != nil && cfgObj.Asn != nil {
				asn = cfgObj.Asn
				asnFromPayload = true
			}
			if !asnFromPayload {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-as-path-prepend config asn is missing in request payload"}
			}
		}
		if reqInfo.targetLeaf == rpolRepeatNLeaf && !repeatFromPayload {
			cfgObj := getRpolPolicyStmtBgpSetAsPathPrependCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			if cfgObj != nil && cfgObj.RepeatN != nil {
				repeatN = cfgObj.RepeatN
				repeatFromPayload = true
			}
			if !repeatFromPayload {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-as-path-prepend config repeat-n is missing in request payload"}
			}
		}
		if !asnFromPayload && !repeatFromPayload {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-as-path-prepend config payload missing supported fields (asn/repeat-n)"}
		}
		if asnFromPayload && asn != nil && *asn == 0 {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-as-path-prepend config asn cannot be 0"}
		}

		if repeatFromPayload {
			hasAsn := asnFromPayload
			if !hasAsn {
				cfgDb := inParams.dbs[db.ConfigDB]
				if cfgDb == nil {
					return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for routing-policy mapping"}
				}
				entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{reqInfo.policyName, reqInfo.stmtName}})
				if err == nil {
					existingAsn, ok := entry.Field[rpolSetAsnFld]
					hasAsn = ok && strings.TrimSpace(existingAsn) != ""
				}
			}
			if !hasAsn {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-as-path-prepend config repeat-n requires asn to be configured"}
			}
		}

		fields := map[string]string{}
		if asnFromPayload && asn != nil {
			fields[rpolSetAsnFld] = strconv.FormatUint(uint64(*asn), 10)
		}
		if repeatFromPayload && repeatN != nil {
			fields[rpolSetRepeatAsnFld] = strconv.FormatUint(uint64(*repeatN), 10)
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpConditionsCfg {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		cfgObj := getRpolPolicyStmtBgpConditionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
		fields := map[string]string{}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolMedEqLeaf {
			if med, ok := parseRpolUint32Value(inParams.param); ok {
				fields[rpolMatchMedFld] = strconv.FormatUint(uint64(*med), 10)
			} else if cfgObj != nil && cfgObj.MedEq != nil {
				fields[rpolMatchMedFld] = strconv.FormatUint(uint64(*cfgObj.MedEq), 10)
			}
			if reqInfo.targetLeaf == rpolMedEqLeaf {
				if _, ok := fields[rpolMatchMedFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement conditions bgp-conditions config med-eq is missing in request payload"}
				}
			}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolOriginEqLeaf {
			if origin, ok := parseRpolBgpOriginValue(inParams.param); ok {
				if originDb, conv := rpolBgpOriginToDb(origin); conv {
					fields[rpolMatchOriginFld] = originDb
				}
			} else if cfgObj != nil {
				if originDb, conv := rpolBgpOriginToDb(cfgObj.OriginEq); conv {
					fields[rpolMatchOriginFld] = originDb
				}
			}
			if reqInfo.targetLeaf == rpolOriginEqLeaf {
				if _, ok := fields[rpolMatchOriginFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement conditions bgp-conditions config origin-eq is missing in request payload"}
				}
			}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolLocalPrefEqLeaf {
			if lp, ok := parseRpolUint32Value(inParams.param); ok {
				fields[rpolMatchLocalPrefFld] = strconv.FormatUint(uint64(*lp), 10)
			} else if cfgObj != nil && cfgObj.LocalPrefEq != nil {
				fields[rpolMatchLocalPrefFld] = strconv.FormatUint(uint64(*cfgObj.LocalPrefEq), 10)
			}
			if reqInfo.targetLeaf == rpolLocalPrefEqLeaf {
				if _, ok := fields[rpolMatchLocalPrefFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement conditions bgp-conditions config local-pref-eq is missing in request payload"}
				}
			}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunitySetLeaf {
			if community, ok := parseRpolStringValue(inParams.param); ok {
				fields[rpolMatchCommunityFld] = community
			} else if cfgObj != nil && cfgObj.CommunitySet != nil && strings.TrimSpace(*cfgObj.CommunitySet) != "" {
				fields[rpolMatchCommunityFld] = strings.TrimSpace(*cfgObj.CommunitySet)
			}
			if reqInfo.targetLeaf == rpolCommunitySetLeaf {
				if _, ok := fields[rpolMatchCommunityFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement conditions bgp-conditions config community-set is missing in request payload"}
				}
			}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolExtCommunitySetLeaf {
			if extCommunity, ok := parseRpolStringValue(inParams.param); ok {
				fields[rpolMatchExtCommunityFld] = extCommunity
			} else if cfgObj != nil && cfgObj.ExtCommunitySet != nil && strings.TrimSpace(*cfgObj.ExtCommunitySet) != "" {
				fields[rpolMatchExtCommunityFld] = strings.TrimSpace(*cfgObj.ExtCommunitySet)
			}
			if reqInfo.targetLeaf == rpolExtCommunitySetLeaf {
				if _, ok := fields[rpolMatchExtCommunityFld]; !ok {
					return res, tlerr.InvalidArgsError{Format: "routing-policy statement conditions bgp-conditions config ext-community-set is missing in request payload"}
				}
			}
		}

		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement conditions bgp-conditions config payload missing supported fields (med-eq/origin-eq/local-pref-eq/community-set/ext-community-set)"}
		}
		res = makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields)
		if cName, ok := fields[rpolMatchCommunityFld]; ok && strings.TrimSpace(cName) != "" {
			res = mergeDbResMap(res, makeRpolCommunitySetResMap(strings.TrimSpace(cName),
				rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolCommunitySetTbl, strings.TrimSpace(cName), "")))
		}
		if ecName, ok := fields[rpolMatchExtCommunityFld]; ok && strings.TrimSpace(ecName) != "" {
			res = mergeDbResMap(res, makeRpolExtCommunitySetResMap(strings.TrimSpace(ecName),
				rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolExtCommunitySetTbl, strings.TrimSpace(ecName), "")))
		}
		return res, nil
	}
	return res, tlerr.NotSupportedError{Format: "unsupported leaf under routing-policy mapping", Path: inParams.requestUri}
}

var DbToYang_routing_policy_xfmr SubTreeXfmrDbToYang = rpolDbToYangXfmr

func rpolDbToYangXfmr(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("routing-policy transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy transformer panic in GET path: %v", r)}
		}
	}()

	reqInfo, err := parseRpolRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for routing-policy mapping"}
	}

	if reqInfo.targetPath == rpolTargetRoot {
		wrappedRoot, wrapErr := wrapRpolRootAsDevice(inParams.ygRoot)
		if wrapErr != nil {
			return wrapErr
		}
		return populateRpolContainerFromDb(inParams, wrappedRoot)
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
	if reqInfo.targetPath == rpolTargetStmtName {
		_, err = parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return err
		}
		_, err = cfgDb.GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{reqInfo.policyName, reqInfo.stmtName}})
		if err != nil {
			return err
		}
		stmtObj := getRpolPolicyStmtObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if stmtObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement object in YANG tree"}
		}
		name := reqInfo.stmtName
		stmtObj.Name = &name
		return nil
	}
	if reqInfo.targetPath == rpolTargetCommunitySetName {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolCommunitySetTbl}, db.Key{Comp: []string{reqInfo.communitySetName}})
		if err != nil {
			return err
		}
		csObj := getRpolCommunitySetObj(inParams.ygRoot, reqInfo.communitySetName, true)
		if csObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy community-set object in YANG tree"}
		}
		name := reqInfo.communitySetName
		csObj.CommunitySetName = &name
		// Keep key leaf and config leaf in sync on GET so both
		// /community-set-name and /config/community-set-name return same value.
		if csObj.Config == nil {
			return nil
		}
		csObj.Config.CommunitySetName = &name
		if members := rpolDecodeCommunityMembers(entry.Field[rpolCommunityMemberFld]); len(members) > 0 {
			csObj.Config.CommunityMember = make([]ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config_CommunityMember_Union, 0, len(members))
			for _, member := range members {
				var memberVal interface{} = member
				if wellKnown, ok := rpolFrrStringToWellKnownStdCommunity(member); ok {
					memberVal = wellKnown
				}
				unionVal, convErr := csObj.Config.To_OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_BgpDefinedSets_CommunitySets_CommunitySet_Config_CommunityMember_Union(memberVal)
				if convErr == nil {
					csObj.Config.CommunityMember = append(csObj.Config.CommunityMember, unionVal)
				}
			}
		}
		if matchOpts, ok := rpolMatchActionToMatchSetOptions(entry.Field[rpolMatchActionFld]); ok {
			csObj.Config.MatchSetOptions = matchOpts
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetExtCommunitySetName {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolExtCommunitySetTbl}, db.Key{Comp: []string{reqInfo.communitySetName}})
		if err != nil {
			return err
		}
		ecObj := getRpolExtCommunitySetObj(inParams.ygRoot, reqInfo.communitySetName, true)
		if ecObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy ext-community-set object in YANG tree"}
		}
		name := reqInfo.communitySetName
		ecObj.ExtCommunitySetName = &name
		if ecObj.Config == nil {
			return nil
		}
		ecObj.Config.ExtCommunitySetName = &name
		if members := rpolDecodeCommunityMembers(entry.Field[rpolCommunityMemberFld]); len(members) > 0 {
			ecObj.Config.ExtCommunityMember = members
		}
		if matchOpts, ok := rpolMatchActionToMatchSetOptions(entry.Field[rpolMatchActionFld]); ok {
			ecObj.Config.MatchSetOptions = matchOpts
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
		matched := false
		if pfxSet, ok := entry.Field[rpolMatchPrefixSetFld]; ok && strings.TrimSpace(pfxSet) != "" {
			matched = true
			if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetLeaf {
				cfgObj.PrefixSet = ygot.String(strings.TrimSpace(pfxSet))
			}
		} else if pfxSet6, ok := entry.Field[rpolMatchIpv6PrefixSetFld]; ok && strings.TrimSpace(pfxSet6) != "" {
			matched = true
			if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetLeaf {
				cfgObj.PrefixSet = ygot.String(strings.TrimSpace(pfxSet6))
			}
		}
		if matched && (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolMatchSetOptionsLeaf) {
			// The modifier is not stored because the match is always ANY, which
			// is also the YANG default. Report it only once a prefix-set is
			// actually configured, so the answer describes a real condition.
			cfgObj.MatchSetOptions = ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_ANY
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpActionsCfg {
		cfgObj := getRpolPolicyStmtBgpActionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions config object in YANG tree"}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetNextHopLeaf {
			if nh, ok := entry.Field[rpolSetNextHopFld]; ok && strings.TrimSpace(nh) != "" {
				nextHop := strings.TrimSpace(nh)
				if strings.EqualFold(nextHop, "self") {
					cfgObj.SetNextHop = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_Union_E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop{
						E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop: ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_SELF,
					}
				} else {
					cfgObj.SetNextHop = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_Union_String{
						String: nextHop,
					}
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetRouteOriginLeaf {
			if originStr, ok := entry.Field[rpolSetOriginFld]; ok && strings.TrimSpace(originStr) != "" {
				if origin, conv := rpolDbToBgpOrigin(originStr); conv {
					cfgObj.SetRouteOrigin = origin
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetLocalPrefLeaf {
			if lpStr, ok := entry.Field[rpolSetLocalPrefFld]; ok && strings.TrimSpace(lpStr) != "" {
				if lp, convErr := strconv.ParseUint(strings.TrimSpace(lpStr), 10, 32); convErr == nil {
					cfgObj.SetLocalPref = ygot.Uint32(uint32(lp))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetMedLeaf {
			if medStr, ok := entry.Field[rpolSetMedFld]; ok && strings.TrimSpace(medStr) != "" {
				medVal := strings.TrimSpace(medStr)
				if med, convErr := strconv.ParseUint(medVal, 10, 32); convErr == nil {
					cfgObj.SetMed = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_Uint32{
						Uint32: uint32(med),
					}
				} else if strings.EqualFold(medVal, "igp") {
					cfgObj.SetMed = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed{
						E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed: ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_IGP,
					}
				} else {
					cfgObj.SetMed = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_String{
						String: medVal,
					}
				}
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunity {
		cfgObj := getRpolPolicyStmtBgpSetCommunityCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-community config object in YANG tree"}
		}
		if ref, ok := entry.Field[rpolSetCommunityRefFld]; ok && strings.TrimSpace(ref) != "" {
			cfgObj.Method = ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_REFERENCE
			refCfgObj := getRpolPolicyStmtBgpSetCommunityRefCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
			if refCfgObj != nil {
				refCfgObj.CommunitySetRef = ygot.String(strings.TrimSpace(ref))
			}
		} else if inline, ok := entry.Field[rpolSetCommunityInlineFld]; ok && strings.TrimSpace(inline) != "" {
			cfgObj.Method = ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE
			decoded, additive := rpolSetCommunityInlineDecode(inline)
			if additive {
				cfgObj.Options = ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD
			}
			inlineCfgObj := getRpolPolicyStmtBgpSetCommunityInlineCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
			if inlineCfgObj != nil {
				inlineCfgObj.Communities = make([]ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union, 0, len(decoded))
				for _, c := range decoded {
					unionVal, err := inlineCfgObj.To_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union(c)
					if err == nil {
						inlineCfgObj.Communities = append(inlineCfgObj.Communities, unionVal)
					}
				}
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunityInlineCfg {
		inlineCfgObj := getRpolPolicyStmtBgpSetCommunityInlineCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if inlineCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-community inline config object in YANG tree"}
		}
		inline, ok := entry.Field[rpolSetCommunityInlineFld]
		if !ok || strings.TrimSpace(inline) == "" {
			return nil
		}
		decoded, _ := rpolSetCommunityInlineDecode(inline)
		inlineCfgObj.Communities = make([]ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union, 0, len(decoded))
		for _, c := range decoded {
			unionVal, err := inlineCfgObj.To_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union(c)
			if err == nil {
				inlineCfgObj.Communities = append(inlineCfgObj.Communities, unionVal)
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetAsPathCfg {
		cfgObj := getRpolPolicyStmtBgpSetAsPathPrependCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-as-path-prepend config object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolAsnLeaf {
			if asnStr, ok := entry.Field[rpolSetAsnFld]; ok && strings.TrimSpace(asnStr) != "" {
				if asn, convErr := strconv.ParseUint(strings.TrimSpace(asnStr), 10, 32); convErr == nil {
					cfgObj.Asn = ygot.Uint32(uint32(asn))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolRepeatNLeaf {
			if repStr, ok := entry.Field[rpolSetRepeatAsnFld]; ok && strings.TrimSpace(repStr) != "" {
				if rep, convErr := strconv.ParseUint(strings.TrimSpace(repStr), 10, 8); convErr == nil {
					cfgObj.RepeatN = ygot.Uint8(uint8(rep))
				}
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpConditionsCfg {
		cfgObj := getRpolPolicyStmtBgpConditionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement conditions bgp-conditions config object in YANG tree"}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolMedEqLeaf {
			if medStr, ok := entry.Field[rpolMatchMedFld]; ok && strings.TrimSpace(medStr) != "" {
				if med, convErr := strconv.ParseUint(strings.TrimSpace(medStr), 10, 32); convErr == nil {
					cfgObj.MedEq = ygot.Uint32(uint32(med))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolOriginEqLeaf {
			if originStr, ok := entry.Field[rpolMatchOriginFld]; ok && strings.TrimSpace(originStr) != "" {
				if origin, conv := rpolDbToBgpOrigin(originStr); conv {
					cfgObj.OriginEq = origin
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolLocalPrefEqLeaf {
			if lpStr, ok := entry.Field[rpolMatchLocalPrefFld]; ok && strings.TrimSpace(lpStr) != "" {
				if lp, convErr := strconv.ParseUint(strings.TrimSpace(lpStr), 10, 32); convErr == nil {
					cfgObj.LocalPrefEq = ygot.Uint32(uint32(lp))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunitySetLeaf {
			if communitySet, ok := entry.Field[rpolMatchCommunityFld]; ok && strings.TrimSpace(communitySet) != "" {
				cfgObj.CommunitySet = ygot.String(strings.TrimSpace(communitySet))
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolExtCommunitySetLeaf {
			if extCommunitySet, ok := entry.Field[rpolMatchExtCommunityFld]; ok && strings.TrimSpace(extCommunitySet) != "" {
				cfgObj.ExtCommunitySet = ygot.String(strings.TrimSpace(extCommunitySet))
			}
		}
		return nil
	}
	return tlerr.NotSupportedError{Format: "unsupported leaf under routing-policy mapping", Path: inParams.requestUri}
}
