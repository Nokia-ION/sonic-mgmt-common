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
	"net"
	"reflect"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
	"github.com/openconfig/ygot/ytypes"
)

const (
	rpolRouteMapSetTbl = "ROUTE_MAP_SET"
	rpolRouteMapTbl    = "ROUTE_MAP"
	// What FRR has actually applied, scraped into STATE_DB by bgpmon. The
	// field names match the CONFIG_DB ones above on purpose, so state decodes
	// the same way config does.
	rpolRouteMapAppliedTbl                   = "ROUTE_MAP_APPLIED"
	rpolRouteMapSetAppliedTbl                = "ROUTE_MAP_SET_APPLIED"
	rpolMetricActionSet                      = "METRIC_SET_VALUE"
	rpolMetricActionAdd                      = "METRIC_ADD_VALUE"
	rpolMetricActionSubtract                 = "METRIC_SUBTRACT_VALUE"
	rpolCommunitySetTbl                      = "COMMUNITY_SET"
	rpolExtCommunitySetTbl                   = "EXTENDED_COMMUNITY_SET"
	rpolPrefixSetTbl                         = "PREFIX_SET"
	rpolRouteOperationFld                    = "route_operation"
	rpolMatchPrefixSetFld                    = "match_prefix_set"
	rpolMatchIpv6PrefixSetFld                = "match_ipv6_prefix_set"
	rpolSetCommunityInlineFld                = "set_community_inline"
	rpolSetCommunityRefFld                   = "set_community_ref"
	rpolSetAsnFld                            = "set_asn"
	rpolSetRepeatAsnFld                      = "set_repeat_asn"
	rpolSetNextHopFld                        = "set_next_hop"
	rpolSetIpv6NextHopGlobalFld              = "set_ipv6_next_hop_global"
	rpolSetOriginFld                         = "set_origin"
	rpolSetLocalPrefFld                      = "set_local_pref"
	rpolSetMedFld                            = "set_med"
	rpolSetMetricActionFld                   = "set_metric_action"
	rpolSetMetricFld                         = "set_metric"
	rpolMatchMedFld                          = "match_med"
	rpolMatchOriginFld                       = "match_origin"
	rpolMatchLocalPrefFld                    = "match_local_pref"
	rpolMatchCommunityFld                    = "match_community"
	rpolMatchExtCommunityFld                 = "match_ext_community"
	rpolRouteOpPermit                        = "permit"
	rpolRouteOpDeny                          = "deny"
	rpolTargetPolicyDefCfg                   = "policy-definition-config"
	rpolTargetStmtName                       = "statement-name"
	rpolTargetStmtActionsCfg                 = "statement-actions-config"
	rpolTargetStmtActionsState               = "statement-actions-state"
	rpolTargetStmtCondMatchPfxCfg            = "statement-conditions-match-prefix-set-config"
	rpolTargetStmtCondMatchPfxState          = "statement-conditions-match-prefix-set-state"
	rpolTargetStmtBgpActionsCfg              = "statement-actions-bgp-actions-config"
	rpolTargetStmtBgpActionsState            = "statement-actions-bgp-actions-state"
	rpolTargetStmtBgpSetCommunity            = "statement-actions-bgp-set-community"
	rpolTargetStmtBgpSetCommunityInlineCfg   = "statement-actions-bgp-set-community-inline-config"
	rpolTargetStmtBgpSetCommunityInlineState = "statement-actions-bgp-set-community-inline-state"
	rpolTargetStmtBgpSetCommunityRefCfg      = "statement-actions-bgp-set-community-reference-config"
	rpolTargetStmtBgpSetCommunityRefState    = "statement-actions-bgp-set-community-reference-state"
	rpolTargetStmtBgpSetAsPathCfg            = "statement-actions-bgp-set-as-path-prepend-config"
	rpolTargetStmtBgpSetAsPathState          = "statement-actions-bgp-set-as-path-prepend-state"
	rpolTargetStmtBgpConditionsCfg           = "statement-conditions-bgp-conditions-config"
	rpolTargetStmtBgpConditionsState         = "statement-conditions-bgp-conditions-state"
	rpolTargetCommunitySetName               = "community-set-name"
	rpolTargetRoot                           = "routing-policy-root"
	rpolPolicyDefNameLeaf                    = "name"
	rpolStmtNameLeaf                         = "name"
	rpolPolicyDefNode                        = "policy-definition"
	rpolStatementNode                        = "statement"
	rpolCommunitySetNameLeaf                 = "community-set-name"
	rpolCommunitySetRefLeaf                  = "community-set-ref"
	rpolCommunitySetRefsLeaf                 = "community-set-refs"
	rpolSetCommunityMethodLeaf               = "method"
	rpolSetCommunityOptionsLeaf              = "options"
	rpolPolicyResultLeaf                     = "policy-result"
	rpolPrefixSetLeaf                        = "prefix-set"
	rpolAsnLeaf                              = "asn"
	rpolRepeatNLeaf                          = "repeat-n"
	rpolSetNextHopLeaf                       = "set-next-hop"
	rpolSetRouteOriginLeaf                   = "set-route-origin"
	rpolSetLocalPrefLeaf                     = "set-local-pref"
	rpolSetMedLeaf                           = "set-med"
	rpolSetMedActionLeaf                     = "set-med-action"
	rpolMedEqLeaf                            = "med-eq"
	rpolOriginEqLeaf                         = "origin-eq"
	rpolLocalPrefEqLeaf                      = "local-pref-eq"
	rpolCommunitySetLeaf                     = "community-set"
	rpolExtCommunitySetLeaf                  = "ext-community-set"
	rpolSetTypeFld                           = "set_type"
	rpolPrefixSetModeFld                     = "mode"
	rpolCommunityMemberFld                   = "community_member@"
	rpolMatchActionFld                       = "match_action"
	rpolSetTypeStandard                      = "STANDARD"
	rpolSetTypeExpanded                      = "EXPANDED"
	rpolMatchActionAny                       = "ANY"
	rpolMatchActionAll                       = "ALL"
	rpolMatchSetOptionsInvert                = "INVERT"
	rpolTargetExtCommunitySetName            = "ext-community-set-name"
	rpolExtCommunitySetNameLeaf              = "ext-community-set-name"
	rpolCommunityMemberLeaf                  = "community-member"
	rpolExtCommunityMemberLeaf               = "ext-community-member"
	rpolMatchSetOptionsLeaf                  = "match-set-options"
	rpolTargetPrefixSet                      = "prefix-set-node"
	rpolTargetPrefixEntry                    = "prefix-entry-node"
	rpolPrefixSetNameLeaf                    = "name"
	rpolPrefixSetModeLeaf                    = "mode"
	rpolIpPrefixLeaf                         = "ip-prefix"
	rpolMasklengthRangeLeaf                  = "masklength-range"
)

var rpolPolicyDefCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/config(?:/(name))?$`)
var rpolStmtNameReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/(?:(?:[A-Za-z0-9_.-]+:)?name|(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?name)?)$`)
var rpolStmtActionsCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/config(?:/(policy-result))?$`)
var rpolStmtActionsStateReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/state(?:/(policy-result))?$`)
var rpolStmtCondMatchPfxCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions/match-prefix-set/config(?:/(prefix-set|match-set-options))?$`)
var rpolStmtCondMatchPfxStateReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions/match-prefix-set/state(?:/(prefix-set|match-set-options))?$`)
var rpolStmtBgpActionsCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(set-next-hop|set-route-origin|set-local-pref|set-med-action|set-med))?$`)
var rpolStmtBgpActionsStateReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?state(?:/(?:[A-Za-z0-9_.-]+:)?(set-next-hop|set-route-origin|set-local-pref|set-med-action|set-med))?$`)
var rpolStmtBgpSetCommunityReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-community(?:/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(method|options))?)?$`)
var rpolStmtBgpSetCommunityInlineCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-community/(?:[A-Za-z0-9_.-]+:)?inline/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(communities))?$`)
var rpolStmtBgpSetCommunityInlineStateReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-community/(?:[A-Za-z0-9_.-]+:)?inline/(?:[A-Za-z0-9_.-]+:)?state(?:/(?:[A-Za-z0-9_.-]+:)?(communities))?$`)
var rpolStmtBgpSetCommunityRefCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-community/(?:[A-Za-z0-9_.-]+:)?reference/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(community-set-refs|community-set-ref))?$`)
var rpolStmtBgpSetCommunityRefStateReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-community/(?:[A-Za-z0-9_.-]+:)?reference/(?:[A-Za-z0-9_.-]+:)?state(?:/(?:[A-Za-z0-9_.-]+:)?(community-set-refs|community-set-ref))?$`)
var rpolStmtBgpSetAsPathCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-as-path-prepend/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(asn|repeat-n))?$`)
var rpolStmtBgpSetAsPathStateReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions/(?:[A-Za-z0-9_.-]+:)?bgp-actions/(?:[A-Za-z0-9_.-]+:)?set-as-path-prepend/(?:[A-Za-z0-9_.-]+:)?state(?:/(?:[A-Za-z0-9_.-]+:)?(asn|repeat-n))?$`)
var rpolStmtBgpConditionsCfgReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions/(?:[A-Za-z0-9_.-]+:)?bgp-conditions/(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(med-eq|origin-eq|local-pref-eq|community-set|ext-community-set))?$`)
var rpolStmtBgpConditionsStateReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions/(?:[A-Za-z0-9_.-]+:)?bgp-conditions/(?:[A-Za-z0-9_.-]+:)?state(?:/(?:[A-Za-z0-9_.-]+:)?(med-eq|origin-eq|local-pref-eq|community-set|ext-community-set))?$`)
var rpolContainerReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy(?:/.*)?$`)
var rpolCommunitySetReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?community-sets/(?:[A-Za-z0-9_.-]+:)?community-set\[(?:[A-Za-z0-9_.-]+:)?community-set-name=([^\]]+)\]/(?:(?:[A-Za-z0-9_.-]+:)?community-set-name|(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(community-set-name|community-member|match-set-options))?)$`)
var rpolExtCommunitySetReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?ext-community-sets/(?:[A-Za-z0-9_.-]+:)?ext-community-set\[(?:[A-Za-z0-9_.-]+:)?ext-community-set-name=([^\]]+)\]/(?:(?:[A-Za-z0-9_.-]+:)?ext-community-set-name|(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(ext-community-set-name|ext-community-member|match-set-options))?)$`)

// The prefix-set lists sit under defined-sets in the routing-policy module
// itself rather than under bgp-defined-sets, and a prefix entry is keyed by
// ip-prefix and masklength-range together.
var rpolPrefixSetReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?prefix-sets/(?:[A-Za-z0-9_.-]+:)?prefix-set\[(?:[A-Za-z0-9_.-]+:)?name=([^\]]+)\]/(?:(?:[A-Za-z0-9_.-]+:)?name|(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(name|mode))?)$`)
var rpolPrefixEntryReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?prefix-sets/(?:[A-Za-z0-9_.-]+:)?prefix-set\[(?:[A-Za-z0-9_.-]+:)?name=([^\]]+)\]/(?:[A-Za-z0-9_.-]+:)?prefixes/(?:[A-Za-z0-9_.-]+:)?prefix\[(?:[A-Za-z0-9_.-]+:)?ip-prefix=([^\]]+)\]\[(?:[A-Za-z0-9_.-]+:)?masklength-range=([^\]]+)\]/(?:(?:[A-Za-z0-9_.-]+:)?ip-prefix|(?:[A-Za-z0-9_.-]+:)?masklength-range|(?:[A-Za-z0-9_.-]+:)?config(?:/(?:[A-Za-z0-9_.-]+:)?(ip-prefix|masklength-range))?)$`)

// A DELETE can name a list instance itself, carrying none of the child nodes
// the shapes above require. Such a URI is classified as container level, since
// a GET or a SET on one is answered by walking the whole instance, so the
// DELETE path recognises these shapes on their own.
var rpolPolicyDefInstanceReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]$`)

// A statement's conditions container, or one of the containers under it.
var rpolStmtCondReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/conditions(?:/((?:[A-Za-z0-9_.-]+:)?(?:match-prefix-set|bgp-conditions)))?$`)

// The policy-definitions container or any node below it.
var rpolPolicyDefsReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions(?:[/\[]|$)`)

// A statement's actions container, with the path below it in the third group.
var rpolStmtActionsReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]/actions((?:/[A-Za-z0-9_.:-]+)*)$`)
var rpolStmtInstanceReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/policy-definitions/policy-definition\[name=([^\]]+)\]/statements/statement\[name=([^\]]+)\]$`)
var rpolCommunitySetInstanceReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?community-sets/(?:[A-Za-z0-9_.-]+:)?community-set\[(?:[A-Za-z0-9_.-]+:)?community-set-name=([^\]]+)\]$`)
var rpolExtCommunitySetInstanceReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?ext-community-sets/(?:[A-Za-z0-9_.-]+:)?ext-community-set\[(?:[A-Za-z0-9_.-]+:)?ext-community-set-name=([^\]]+)\]$`)
var rpolPrefixSetInstanceReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?prefix-sets/(?:[A-Za-z0-9_.-]+:)?prefix-set\[(?:[A-Za-z0-9_.-]+:)?name=([^\]]+)\]$`)
var rpolPrefixEntryInstanceReqUriRe = regexp.MustCompile(`^/openconfig-routing-policy:routing-policy/defined-sets/(?:[A-Za-z0-9_.-]+:)?prefix-sets/(?:[A-Za-z0-9_.-]+:)?prefix-set\[(?:[A-Za-z0-9_.-]+:)?name=([^\]]+)\]/(?:[A-Za-z0-9_.-]+:)?prefixes/(?:[A-Za-z0-9_.-]+:)?prefix\[(?:[A-Za-z0-9_.-]+:)?ip-prefix=([^\]]+)\]\[(?:[A-Za-z0-9_.-]+:)?masklength-range=([^\]]+)\]$`)

// The state container of a policy definition, a statement, a defined set or a
// prefix, with an optional leaf below it in the second group. These objects have
// no operational source of their own, so their state reports what is configured
// and is served as its config twin.
var rpolMirroredStateReqUriRe = regexp.MustCompile(`^(/openconfig-routing-policy:routing-policy/(?:policy-definitions/policy-definition\[name=[^\]]+\](?:/statements/statement\[name=[^\]]+\])?|defined-sets/(?:[A-Za-z0-9_.-]+:)?(?:bgp-defined-sets/(?:[A-Za-z0-9_.-]+:)?(?:ext-)?community-sets/(?:[A-Za-z0-9_.-]+:)?(?:ext-)?community-set\[[^\]]+\]|prefix-sets/(?:[A-Za-z0-9_.-]+:)?prefix-set\[[^\]]+\](?:/(?:[A-Za-z0-9_.-]+:)?prefixes/(?:[A-Za-z0-9_.-]+:)?prefix\[[^\]]+\]\[[^\]]+\])?)))/(?:[A-Za-z0-9_.-]+:)?state(/(?:[A-Za-z0-9_.-]+:)?[A-Za-z0-9_.-]+)?$`)

type rpolReqInfo struct {
	policyName       string
	stmtName         string
	communitySetName string
	prefixSetName    string
	ipPrefix         string
	masklengthRange  string
	targetLeaf       string
	targetPath       string
}

func init() {
	XlateFuncBind("YangToDb_routing_policy_xfmr", YangToDb_routing_policy_xfmr)
	XlateFuncBind("DbToYang_routing_policy_xfmr", DbToYang_routing_policy_xfmr)
	XlateFuncBind("Subscribe_routing_policy_xfmr", Subscribe_routing_policy_xfmr)
	XlateFuncBind("DbToYangPath_routing_policy_path_xfmr", DbToYangPath_routing_policy_path_xfmr)
}

func parseRpolRequestUri(uri string) (*rpolReqInfo, error) {
	if m := rpolMirroredStateReqUriRe.FindStringSubmatch(uri); len(m) == 3 {
		return parseRpolRequestUri(m[1] + "/config" + m[2])
	}
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
	m = rpolStmtActionsStateReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtActionsState,
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
	m = rpolStmtCondMatchPfxStateReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtCondMatchPfxState,
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
	m = rpolStmtBgpActionsStateReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpActionsState,
		}, nil
	}
	m = rpolStmtBgpSetCommunityReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
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
	m = rpolStmtBgpSetCommunityInlineStateReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpSetCommunityInlineState,
		}, nil
	}
	m = rpolStmtBgpSetCommunityRefCfgReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpSetCommunityRefCfg,
		}, nil
	}
	m = rpolStmtBgpSetCommunityRefStateReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpSetCommunityRefState,
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
	m = rpolStmtBgpSetAsPathStateReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpSetAsPathState,
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
	m = rpolStmtBgpConditionsStateReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 3 {
		leaf := ""
		if len(m) >= 4 {
			leaf = m[3]
		}
		return &rpolReqInfo{
			policyName: m[1],
			stmtName:   m[2],
			targetLeaf: leaf,
			targetPath: rpolTargetStmtBgpConditionsState,
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
	m = rpolPrefixEntryReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 4 {
		leaf := ""
		if len(m) >= 5 {
			leaf = m[4]
		}
		return &rpolReqInfo{
			prefixSetName:   m[1],
			ipPrefix:        m[2],
			masklengthRange: m[3],
			targetLeaf:      leaf,
			targetPath:      rpolTargetPrefixEntry,
		}, nil
	}
	m = rpolPrefixSetReqUriRe.FindStringSubmatch(uri)
	if len(m) >= 2 {
		leaf := ""
		if len(m) >= 3 {
			leaf = m[2]
		}
		return &rpolReqInfo{
			prefixSetName: m[1],
			targetLeaf:    leaf,
			targetPath:    rpolTargetPrefixSet,
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

// ocMirrorConfigToState fills the state container of obj, a node holding
// config and state twins, with what its config container holds. The
// copy goes through JSON because the twins are distinct Go types, down to the
// union wrappers of the community members.
func ocMirrorConfigToState(obj interface{}) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return
	}
	cfg, st := v.Elem().FieldByName("Config"), v.Elem().FieldByName("State")
	if !cfg.IsValid() || !st.IsValid() || cfg.IsNil() {
		return
	}
	cfgObj, ok := cfg.Interface().(ygot.ValidatedGoStruct)
	if !ok {
		return
	}
	js, err := ygot.EmitJSON(cfgObj, &ygot.EmitJSONConfig{Format: ygot.RFC7951, SkipValidation: true})
	if err != nil {
		log.Warningf("state mirror: config emit failed: %v", err)
		return
	}
	if st.IsNil() {
		st.Set(reflect.New(st.Type().Elem()))
	}
	stObj, ok := st.Interface().(ygot.GoStruct)
	if !ok {
		return
	}
	if err := ocbinds.Unmarshal([]byte(js), stObj, &ytypes.IgnoreExtraFields{}); err != nil {
		log.Warningf("state mirror: state unmarshal failed: %v", err)
	}
}

func getRpolPolicyDefObj(s *ygot.GoStruct, policyName string) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition {
	if s == nil || *s == nil {
		return nil
	}
	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil || deviceObj.RoutingPolicy == nil || deviceObj.RoutingPolicy.PolicyDefinitions == nil {
		return nil
	}
	return deviceObj.RoutingPolicy.PolicyDefinitions.PolicyDefinition[policyName]
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
	return stmtObj.Actions.Config
}

// getRpolPolicyStmtActionsStateObj returns the state container of a statement's
// actions. It has no datastore of its own: it reports the same ROUTE_MAP row
// the config container writes.
// rpolAppliedStmtEntry reads the applied row for one statement. A miss is not
// an error: FRR may not have taken a statement the configuration holds, and
// reporting nothing is the honest answer for a state path.
func rpolAppliedStmtEntry(inParams XfmrParams, policyName, stmtName string) (db.Value, bool) {
	stateDb := inParams.dbs[db.StateDB]
	if stateDb == nil {
		return db.Value{}, false
	}
	entry, err := stateDb.GetEntry(&db.TableSpec{Name: rpolRouteMapAppliedTbl},
		db.Key{Comp: []string{policyName, stmtName}})
	if err != nil || len(entry.Field) == 0 {
		return db.Value{}, false
	}
	return entry, true
}

// getRpolPolicyStmtBgpActionsStateObj returns the state container of a
// statement's bgp-actions. Like its config twin it is created on demand, and
// like every state container here it is filled from the applied row.
func getRpolPolicyStmtBgpActionsStateObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_State {
	stmtObj := getRpolPolicyStmtObj(s, policyName, stmtName, createIfMissing)
	if stmtObj == nil || stmtObj.Actions == nil {
		return nil
	}
	ygot.BuildEmptyTree(stmtObj.Actions)
	if stmtObj.Actions.BgpActions == nil {
		return nil
	}
	if stmtObj.Actions.BgpActions.State == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.State = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_State{}
	}
	return stmtObj.Actions.BgpActions.State
}

func getRpolPolicyStmtActionsStateObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_State {
	stmtObj := getRpolPolicyStmtObj(s, policyName, stmtName, createIfMissing)
	if stmtObj == nil {
		return nil
	}
	rpolBuildStmtTree(stmtObj, createIfMissing)
	if stmtObj.Actions == nil {
		return nil
	}
	return stmtObj.Actions.State
}

// rpolBuildStmtTree fills in the containers below a statement, and with deep
// set, every container below those too. ygot.BuildEmptyTree creates only the
// children that are nil, so a container translib has already created for a
// GET keeps nil children of its own, and a getter reaching through it would
// dereference one of them. A write keeps the single level, since its handlers
// tell an absent payload container from an empty one.
func rpolBuildStmtTree(obj ygot.GoStruct, deep bool) {
	ygot.BuildEmptyTree(obj)
	if !deep {
		return
	}
	v := reflect.ValueOf(obj).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.Ptr || f.IsNil() || f.Elem().Kind() != reflect.Struct {
			continue
		}
		if child, ok := f.Interface().(ygot.GoStruct); ok {
			rpolBuildStmtTree(child, true)
		}
	}
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
	return stmtObj
}

func getRpolPolicyStmtCondMatchPrefixSetStateObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_MatchPrefixSet_State {
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
	return stmtObj.Conditions.MatchPrefixSet.State
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
	return stmtObj.Actions.BgpActions.Config
}

func getRpolPolicyStmtBgpConditionsStateObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Conditions_BgpConditions_State {
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
	return stmtObj.Conditions.BgpConditions.State
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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

func getRpolPolicyStmtBgpSetAsPathPrependStateObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetAsPathPrepend_State {
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
	return stmtObj.Actions.BgpActions.SetAsPathPrepend.State
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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

// rpolSetCommunityRefName resolves the community-set a set-community reference
// names. openconfig-bgp-policy 6.2.0 added the community-set-refs leaf-list and
// deprecated the singular community-set-ref without removing it, so both
// spellings are accepted here. ROUTE_MAP set_community_ref is a single leafref,
// so a list carrying more than one reference has nowhere to be stored and is
// refused rather than silently truncated to its first entry. Both spellings may
// be given together only when they name the same set, since they map to the one
// field and a disagreement has no answer that is not a guess.
func rpolSetCommunityRefName(refCfgObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Reference_Config) (string, error) {
	if refCfgObj == nil {
		return "", nil
	}

	refs := make([]string, 0, len(refCfgObj.CommunitySetRefs))
	for _, ref := range refCfgObj.CommunitySetRefs {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			refs = append(refs, trimmed)
		}
	}
	if len(refs) > 1 {
		return "", tlerr.NotSupportedError{Format: fmt.Sprintf(
			"routing-policy statement actions bgp-actions set-community reference config community-set-refs carries %d references; ROUTE_MAP set_community_ref holds one community-set, so exactly one is supported",
			len(refs))}
	}

	single := ""
	if refCfgObj.CommunitySetRef != nil {
		single = strings.TrimSpace(*refCfgObj.CommunitySetRef)
	}
	if len(refs) == 1 {
		if single != "" && single != refs[0] {
			return "", tlerr.InvalidArgsError{Format: fmt.Sprintf(
				"routing-policy statement actions bgp-actions set-community reference config names %s in community-set-refs and %s in community-set-ref; both map to set_community_ref, so they have to agree",
				refs[0], single)}
		}
		return refs[0], nil
	}
	return single, nil
}

func getRpolPolicyStmtBgpSetCommunityRefStateObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Reference_State {
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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
	if stmtObj.Actions.BgpActions.SetCommunity.Reference.State == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity.Reference.State = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Reference_State{}
	}
	return stmtObj.Actions.BgpActions.SetCommunity.Reference.State
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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

func getRpolPolicyStmtBgpSetCommunityInlineStateObj(s *ygot.GoStruct, policyName, stmtName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_State {
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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
	if stmtObj.Actions.BgpActions.SetCommunity.Inline.State == nil {
		if !createIfMissing {
			return nil
		}
		stmtObj.Actions.BgpActions.SetCommunity.Inline.State = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_State{}
	}
	return stmtObj.Actions.BgpActions.SetCommunity.Inline.State
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
	rpolBuildStmtTree(stmtObj, createIfMissing)
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
			// FRR's community parser has its own keywords: NO_EXPORT is
			// no-export to it, and NO_EXPORT_SUBCONFED is local-AS. Writing
			// the OpenConfig identity name instead makes FRR report the whole
			// attribute malformed, which leaves the route-map with no set
			// clause at all.
			if c, ok := rpolWellKnownStdCommunityToFrrString(v.E_OpenconfigBgpTypes_BGP_WELL_KNOWN_STD_COMMUNITY); ok {
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

// rpolSetCommunityInlineEncode renders the communities as the single argument
// frrcfgd hands to "set community {}". FRR wants them space separated with one
// trailing "additive", and refuses the attribute outright otherwise: both a
// comma separated list and "additive" repeated per value are reported as
// "Malformed communities attribute", which leaves the route-map with no set
// clause at all.
func rpolSetCommunityInlineEncode(communities []string, additive bool) string {
	out := make([]string, 0, len(communities)+1)
	for _, c := range communities {
		v := strings.TrimSpace(c)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return ""
	}
	if additive {
		out = append(out, "additive")
	}
	return strings.Join(out, " ")
}

// rpolSetCommunityInlineDecode reads that argument back. Commas are treated as
// separators as well as spaces, so a row written before the encoding was
// corrected still reads back: those hold a comma separated list with "additive"
// repeated on every value.
func rpolSetCommunityInlineDecode(in string) ([]string, bool) {
	additive := false
	out := make([]string, 0, 4)
	for _, v := range strings.Fields(strings.ReplaceAll(in, ",", " ")) {
		if strings.EqualFold(v, "additive") {
			additive = true
			continue
		}
		out = append(out, v)
	}
	return out, additive
}

// rpolSetCommunityInlineMemberToYang turns a stored member back into the
// spelling the model uses, since FRR's keywords are not the OpenConfig identity
// names. Anything unrecognised is passed through, which covers plain community
// values and rows that already hold an identity name.
func rpolSetCommunityInlineMemberToYang(v string) string {
	if enum, ok := rpolFrrStringToWellKnownStdCommunity(v); ok {
		if name, ok := rpolWellKnownStdCommunityToString(enum); ok {
			return name
		}
	}
	return v
}

// rpolSetCommunityAdditive maps the OpenConfig option onto the marker FRR
// understands: ADD renders "set community X additive" and REPLACE renders
// "set community X", which replaces the attribute. REMOVE has no spelling
// CONFIG_DB can carry, because FRR removes communities only through
// "set comm-list <name> delete" and neither ROUTE_MAP nor frrcfgd has a field
// for it, so it is refused rather than approximated.
func rpolSetCommunityAdditive(options ocbinds.E_OpenconfigBgpPolicy_BgpSetCommunityOptionType) (additive bool, given bool, err error) {
	switch options {
	case ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_UNSET:
		return false, false, nil
	case ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD:
		return true, true, nil
	case ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_REPLACE:
		return false, true, nil
	}
	return false, false, tlerr.NotSupportedError{Format: "routing-policy statement actions bgp-actions set-community config options REMOVE is not supported; FRR removes communities only through \"set comm-list <name> delete\", which ROUTE_MAP has no field for"}
}

// rpolSetCommunityStored reads back what a statement already carries. options
// has no field of its own: it is a marker on set_community_inline, so a request
// that changes options alone re-encodes the communities that are there.
func rpolSetCommunityStored(cfgDb *db.DB, policyName, stmtName string) (communities []string, additive bool, ref string) {
	if cfgDb == nil {
		return nil, false, ""
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{policyName, stmtName}})
	if err != nil {
		return nil, false, ""
	}
	communities, additive = rpolSetCommunityInlineDecode(entry.Field[rpolSetCommunityInlineFld])
	return communities, additive, strings.TrimSpace(entry.Field[rpolSetCommunityRefFld])
}

// rpolSetCommunityOptionsResMap re-encodes the stored communities with a new
// marker, which is how a change to options on its own reaches CONFIG_DB.
func rpolSetCommunityOptionsResMap(inParams XfmrParams, reqInfo *rpolReqInfo, additive bool) (map[string]map[string]db.Value, error) {
	stored, _, ref := rpolSetCommunityStored(inParams.dbs[db.ConfigDB], reqInfo.policyName, reqInfo.stmtName)
	if ref != "" {
		return nil, rpolSetCommunityOptionsRefErr(inParams)
	}
	if len(stored) == 0 {
		return nil, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community config options is stored as a marker on the inline communities, so the communities have to be configured before options"}
	}
	return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
		rpolSetCommunityInlineFld: rpolSetCommunityInlineEncode(stored, additive),
	}), nil
}

func rpolSetCommunityOptionsRefErr(inParams XfmrParams) error {
	return tlerr.NotSupportedError{Format: "routing-policy statement actions bgp-actions set-community config options is not supported with the REFERENCE method; ROUTE_MAP records the referenced community-set in set_community_ref and has no field to carry options alongside it", Path: inParams.requestUri}
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

// rpolSetNextHopFldFor picks the ROUTE_MAP field a next-hop is written to.
// frrcfgd renders set_next_hop as "set ip next-hop", self included, and
// set_ipv6_next_hop_global as "set ipv6 next-hop global", so an IPv6 address
// in the first would be rejected by FRR and silently not applied.
func rpolSetNextHopFldFor(nh string) (string, error) {
	if strings.EqualFold(nh, "self") {
		return rpolSetNextHopFld, nil
	}
	ip := net.ParseIP(nh)
	switch {
	case ip == nil:
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy bgp-actions set-next-hop %q is neither an IP address nor SELF", nh)}
	case ip.To4() != nil:
		return rpolSetNextHopFld, nil
	default:
		return rpolSetIpv6NextHopGlobalFld, nil
	}
}

// rpolBgpNextHopFromRow reports the next-hop a ROUTE_MAP row sets, from
// whichever of its two fields holds it.
func rpolBgpNextHopFromRow(entry db.Value) (string, bool) {
	for _, fld := range []string{rpolSetNextHopFld, rpolSetIpv6NextHopGlobalFld} {
		if nh := strings.TrimSpace(entry.Field[fld]); nh != "" {
			return nh, true
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

// rpolSetMedActionToDb maps the OpenConfig MED action onto the CONFIG_DB
// set_metric_action. SONiC also offers three round trip time actions, which
// have no OpenConfig spelling, so nothing here produces them.
func rpolSetMedActionToDb(v ocbinds.E_OpenconfigBgpPolicy_BgpSetMedAction) (string, error) {
	switch v {
	case ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_SET:
		return rpolMetricActionSet, nil
	case ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_ADD:
		return rpolMetricActionAdd, nil
	case ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_SUBTRACT:
		return rpolMetricActionSubtract, nil
	default:
		return "", tlerr.InvalidArgsError{Format: "unsupported routing-policy set-med-action value in request payload"}
	}
}

// rpolDbToSetMedAction reads back what the helper above writes. A row holding
// one of the round trip time actions reports no action, since OpenConfig has
// nothing to call it.
func rpolDbToSetMedAction(v string) (ocbinds.E_OpenconfigBgpPolicy_BgpSetMedAction, bool) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case rpolMetricActionSet:
		return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_SET, true
	case rpolMetricActionAdd:
		return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_ADD, true
	case rpolMetricActionSubtract:
		return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_SUBTRACT, true
	}
	return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET, false
}

func parseRpolSetMedActionValue(v interface{}) (ocbinds.E_OpenconfigBgpPolicy_BgpSetMedAction, bool) {
	switch val := v.(type) {
	case ocbinds.E_OpenconfigBgpPolicy_BgpSetMedAction:
		if val != ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET {
			return val, true
		}
	case *ocbinds.E_OpenconfigBgpPolicy_BgpSetMedAction:
		if val != nil && *val != ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET {
			return *val, true
		}
	case string:
		switch strings.ToUpper(strings.TrimSpace(val)) {
		case "SET":
			return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_SET, true
		case "ADD":
			return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_ADD, true
		case "SUBTRACT":
			return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_SUBTRACT, true
		}
	}
	return ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET, false
}

// rpolSetMedFields decides which of the two CONFIG_DB spellings of a MED
// setting a request lands in. set_metric_action paired with set_metric is what
// frrcfgd renders as "set metric +N" and "set metric -N", while the older
// set_med renders as a bare "set metric N" and cannot express an increment.
// frrcfgd binds both, so a row carrying values in both would have it emit two
// competing set metric lines for the one route-map entry. Whichever spelling a
// request settles on therefore clears the other, the same way a set-community
// reference clears the inline spelling.
//
// Either leaf may arrive on its own, so the one a request leaves out is taken
// from the row being updated: changing the value keeps the action already
// configured, and changing the action keeps the value.
func rpolSetMedFields(cfgDb *db.DB, reqInfo *rpolReqInfo, medVal string, medGiven bool,
	medAction ocbinds.E_OpenconfigBgpPolicy_BgpSetMedAction) (map[string]string, map[string]string, error) {
	stored := db.Value{Field: map[string]string{}}
	if cfgDb != nil {
		if entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolRouteMapTbl},
			db.Key{Comp: []string{reqInfo.policyName, reqInfo.stmtName}}); err == nil {
			stored = entry
		}
	}

	if medAction == ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET {
		if act, ok := rpolDbToSetMedAction(stored.Field[rpolSetMetricActionFld]); ok {
			medAction = act
		}
	}
	if !medGiven {
		for _, fld := range []string{rpolSetMetricFld, rpolSetMedFld} {
			if v := strings.TrimSpace(stored.Field[fld]); v != "" {
				medVal, medGiven = v, true
				break
			}
		}
	}

	// Only a field that actually holds something is named for deletion, so a
	// MED write does not carry a pointless delete for every row it touches.
	toClear := func(flds ...string) map[string]string {
		clear := map[string]string{}
		for _, fld := range flds {
			if strings.TrimSpace(stored.Field[fld]) != "" {
				clear[fld] = ""
			}
		}
		return clear
	}

	if medAction == ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET {
		// No action in the request or the row, so the plain spelling applies.
		// medGiven is necessarily set here, since this helper is only reached
		// when the request carries a value or an action.
		return map[string]string{rpolSetMedFld: medVal},
			toClear(rpolSetMetricActionFld, rpolSetMetricFld), nil
	}

	if !medGiven {
		return nil, nil, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-med-action needs a set-med value, which neither the request nor the statement carries"}
	}
	actDb, actErr := rpolSetMedActionToDb(medAction)
	if actErr != nil {
		return nil, nil, actErr
	}
	return map[string]string{
			rpolSetMetricActionFld: actDb,
			rpolSetMetricFld:       medVal,
		},
		toClear(rpolSetMedFld), nil
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
func rpolMatchSetOptionsToMatchAction(v ocbinds.E_OpenconfigBgpPolicy_MatchSetOptionsType) (string, error) {
	switch v {
	case ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_ANY:
		return rpolMatchActionAny, nil
	case ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_ALL:
		return rpolMatchActionAll, nil
	case ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_INVERT:
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

var rpolMatchPrefixSetFlds = []string{rpolMatchPrefixSetFld, rpolMatchIpv6PrefixSetFld}
var rpolBgpConditionsFlds = []string{rpolMatchMedFld, rpolMatchOriginFld, rpolMatchLocalPrefFld, rpolMatchCommunityFld, rpolMatchExtCommunityFld}
var rpolSetCommunityFlds = []string{rpolSetCommunityInlineFld, rpolSetCommunityRefFld}
var rpolSetAsPathPrependFlds = []string{rpolSetAsnFld, rpolSetRepeatAsnFld}
var rpolBgpActionsFlds = append(append([]string{rpolSetNextHopFld, rpolSetIpv6NextHopGlobalFld, rpolSetOriginFld, rpolSetLocalPrefFld, rpolSetMedFld, rpolSetMetricActionFld, rpolSetMetricFld},
	rpolSetCommunityFlds...), rpolSetAsPathPrependFlds...)

// rpolStageSubOp merges rows into the sub-operation of oper, so that several
// nodes replayed by one container request can each stage their own.
func rpolStageSubOp(inParams XfmrParams, oper Operation, rows map[string]map[string]db.Value) error {
	if inParams.subOpDataMap == nil {
		return tlerr.InternalError{Format: "routing-policy write has no sub-operation map to stage into"}
	}
	if inParams.subOpDataMap[oper] == nil {
		subOpMap := make(RedisDbMap)
		inParams.subOpDataMap[oper] = &subOpMap
	}
	subOpMap := *inParams.subOpDataMap[oper]
	subOpMap[db.ConfigDB] = mergeDbResMapFields(subOpMap[db.ConfigDB], rows)
	return nil
}

// rpolStageStaleMatchPrefixSet removes the prefix-set of the other address
// family when a write moves a statement to a set of this one. A statement
// matches one prefix-set, while ROUTE_MAP spells the two families as separate
// fields, so a write naming only the new one would leave both in place.
//
// An update deletes the stale field, or the whole row when that field is all
// it holds, since deleting the last field of a row leaves a NULL placeholder
// that the update then keeps. A REPLACE cannot carry that delete: translib
// drops a sub-operation DELETE for a row the REPLACE also names, yet writes a
// leaf-level REPLACE as a partial update. It is staged instead as a REPLACE of
// the whole row, and true tells the caller not to return the row itself.
func rpolStageStaleMatchPrefixSet(inParams XfmrParams, reqInfo *rpolReqInfo, fields map[string]string) (bool, error) {
	stale := rpolMatchIpv6PrefixSetFld
	if _, v6 := fields[rpolMatchIpv6PrefixSetFld]; v6 {
		stale = rpolMatchPrefixSetFld
	}
	return rpolStageStaleFamilyField(inParams, reqInfo, fields, stale, rpolMatchPrefixSetFld, rpolMatchIpv6PrefixSetFld)
}

// rpolStageStaleNextHop does the same for the next-hop a statement sets, which
// ROUTE_MAP spells as set_next_hop for an IPv4 address or self and as
// set_ipv6_next_hop_global for an IPv6 address.
func rpolStageStaleNextHop(inParams XfmrParams, reqInfo *rpolReqInfo, fields map[string]string) (bool, error) {
	stale := rpolSetIpv6NextHopGlobalFld
	if _, v6 := fields[rpolSetIpv6NextHopGlobalFld]; v6 {
		stale = rpolSetNextHopFld
	}
	return rpolStageStaleFamilyField(inParams, reqInfo, fields, stale, rpolSetNextHopFld, rpolSetIpv6NextHopGlobalFld)
}

func rpolStageStaleFamilyField(inParams XfmrParams, reqInfo *rpolReqInfo, fields map[string]string, stale string, pair ...string) (bool, error) {
	entry, err := inParams.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{reqInfo.policyName, reqInfo.stmtName}})
	if err != nil || strings.TrimSpace(entry.Get(stale)) == "" {
		return false, nil
	}
	if inParams.oper == REPLACE {
		row := niBgpReplaceRowFields(entry, fields, rpolOwnedFldSet(pair))
		rpolAddAuxField(inParams, reqInfo, stale)
		return true, rpolStageSubOp(inParams, REPLACE, makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, row))
	}
	for fld := range entry.Field {
		if fld != stale && fld != "NULL" {
			return false, rpolStageSubOp(inParams, DELETE, makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{stale: ""}))
		}
	}
	return false, rpolStageSubOp(inParams, DELETE, makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, nil))
}

// rpolAddAuxField lists a ROUTE_MAP field as one the REPLACE drops. A REPLACE
// at a terminal node whose row keeps fields it does not name is written as a
// partial update, which would leave that field in place.
func rpolAddAuxField(inParams XfmrParams, reqInfo *rpolReqInfo, fld string) {
	if inParams.yangAuxValMap == nil {
		return
	}
	key := reqInfo.policyName + "|" + reqInfo.stmtName
	if inParams.yangAuxValMap[rpolRouteMapTbl] == nil {
		inParams.yangAuxValMap[rpolRouteMapTbl] = make(map[string]db.Value)
	}
	entry := inParams.yangAuxValMap[rpolRouteMapTbl][key]
	if entry.Field == nil {
		entry.Field = make(map[string]string)
	}
	entry.Field[fld] = ""
	inParams.yangAuxValMap[rpolRouteMapTbl][key] = entry
}

// rpolStmtCondOwnedFlds names the ROUTE_MAP fields that a REPLACE anchored at a
// statement's conditions, or at a container under it, takes over. The
// statement is returned too, since the payload need not mention it.
func rpolStmtCondOwnedFlds(uri string) (string, string, map[string]bool) {
	m := rpolStmtCondReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return "", "", nil
	}
	var flds []string
	switch rpolStripModulePrefix(m[3]) {
	case "match-prefix-set":
		flds = rpolMatchPrefixSetFlds
	case "bgp-conditions":
		flds = append(append([]string{}, rpolBgpConditionsFlds...), rpolMatchAsPathFld)
	default:
		flds = append(append(append([]string{}, rpolMatchPrefixSetFlds...), rpolBgpConditionsFlds...), rpolCondLeafFlds...)
	}
	return m[1], m[2], rpolOwnedFldSet(flds)
}

// rpolStmtActionsOwnedFlds names the ROUTE_MAP fields that a REPLACE anchored
// at a statement's actions, or at a container under it, takes over. A
// statement sets communities by one method only, so inline and reference each
// own both spellings.
func rpolStmtActionsOwnedFlds(uri string) (string, string, map[string]bool) {
	m := rpolStmtActionsReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return "", "", nil
	}
	var segs []string
	for _, seg := range strings.Split(strings.TrimPrefix(m[3], "/"), "/") {
		if seg != "" {
			segs = append(segs, rpolStripModulePrefix(seg))
		}
	}
	var flds []string
	switch strings.Join(segs, "/") {
	case "":
		flds = append(append([]string{rpolRouteOperationFld}, rpolBgpActionsFlds...), rpolSetExtCommFlds...)
	case "bgp-actions":
		flds = append(append([]string{}, rpolBgpActionsFlds...), rpolSetExtCommFlds...)
	case "bgp-actions/set-community", "bgp-actions/set-community/inline", "bgp-actions/set-community/reference":
		flds = rpolSetCommunityFlds
	case "bgp-actions/set-as-path-prepend":
		flds = rpolSetAsPathPrependFlds
	default:
		return "", "", nil
	}
	return m[1], m[2], rpolOwnedFldSet(flds)
}

func rpolOwnedFldSet(flds []string) map[string]bool {
	owned := make(map[string]bool, len(flds))
	for _, f := range flds {
		owned[f] = true
	}
	return owned
}

// rpolReplaceStmtRow keeps a REPLACE of part of a statement to that part. A
// statement is one ROUTE_MAP row, and translib writes a REPLACE above the leaf
// level as the whole row, so the fields the replaced subtree does not own are
// carried over from CONFIG_DB rather than dropped with it.
func rpolReplaceStmtRow(inParams XfmrParams, res map[string]map[string]db.Value, policyName, stmtName string, owned map[string]bool) map[string]map[string]db.Value {
	existing, err := inParams.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{policyName, stmtName}})
	if err != nil {
		return res
	}
	key := policyName + "|" + stmtName
	var fields map[string]string
	if row, ok := res[rpolRouteMapTbl][key]; ok {
		fields = row.Field
	}
	row := niBgpReplaceRowFields(existing, fields, owned)
	// Some nodes clear a sibling field by naming it empty; the REPLACE has
	// already dropped every field it owns.
	for f, v := range row {
		if owned[f] && v == "" {
			delete(row, f)
		}
	}
	if len(row) == 0 {
		row["NULL"] = "NULL"
	}
	if res == nil {
		res = make(map[string]map[string]db.Value)
	}
	if res[rpolRouteMapTbl] == nil {
		res[rpolRouteMapTbl] = make(map[string]db.Value)
	}
	res[rpolRouteMapTbl][key] = db.Value{Field: row}
	return res
}

func rpolMatchActionToMatchSetOptions(v string) (ocbinds.E_OpenconfigBgpPolicy_MatchSetOptionsType, bool) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case rpolMatchActionAny:
		return ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_ANY, true
	case rpolMatchActionAll:
		return ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_ALL, true
	}
	return ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_UNSET, false
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
var YangToDb_routing_policy_xfmr SubTreeXfmrYangToDb = rpolYangToDbScopedXfmr

// rpolYangToDbScopedXfmr keeps a REPLACE to the subtree it is anchored at.
// translib writes a REPLACE above the leaf level as whole rows, which is right
// for a statement but not for a container inside one, nor for the
// community-sets a statement only references.
func rpolYangToDbScopedXfmr(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res, err := rpolYangToDbXfmr(inParams)
	if err != nil || inParams.oper != REPLACE {
		return res, err
	}
	policyName, stmtName, owned := rpolStmtCondOwnedFlds(inParams.requestUri)
	if owned == nil {
		policyName, stmtName, owned = rpolStmtActionsOwnedFlds(inParams.requestUri)
	}
	if r, ok := matchRpolStmtLeafReq(inParams.requestUri); ok {
		if flds, ferr := rpolStmtLeafDeleteFields(r); ferr == nil {
			policyName, stmtName, owned = r.policy, r.stmt, map[string]bool{}
			for f := range flds {
				owned[f] = true
			}
		}
	}
	if owned != nil {
		res = rpolReplaceStmtRow(inParams, res, policyName, stmtName, owned)
	}
	if rpolPolicyDefsReqUriRe.MatchString(inParams.requestUri) {
		if err := rpolStageReferencedSets(inParams, res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// rpolStageReferencedSets moves the community-set rows a statement seeds for
// the sets it references out of a REPLACE, so that they are merged into the
// sets rather than written over them. A set that already holds every seeded
// field gets no write, since an empty update would add a NULL placeholder.
func rpolStageReferencedSets(inParams XfmrParams, res map[string]map[string]db.Value) error {
	for _, tbl := range []string{rpolCommunitySetTbl, rpolExtCommunitySetTbl} {
		rows, ok := res[tbl]
		if !ok {
			continue
		}
		delete(res, tbl)
		seeds := make(map[string]db.Value, len(rows))
		for key, row := range rows {
			if len(row.Field) > 0 {
				seeds[key] = row
			}
		}
		if len(seeds) == 0 {
			continue
		}
		if err := rpolStageSubOp(inParams, UPDATE, map[string]map[string]db.Value{tbl: seeds}); err != nil {
			return err
		}
	}
	return nil
}

func rpolYangToDbXfmr(inParams XfmrParams) (res map[string]map[string]db.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("routing-policy transformer panic in SET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			res = make(map[string]map[string]db.Value)
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy transformer panic in SET path: %v", r)}
		}
	}()
	res = make(map[string]map[string]db.Value)

	if leafRes, handled, leafErr := rpolLeavesYangToDb(inParams); handled {
		return leafRes, leafErr
	}

	reqInfo, err := parseRpolRequestUri(inParams.requestUri)
	if err != nil {
		return res, err
	}

	if inParams.oper == DELETE {
		if reqInfo.targetPath == rpolTargetRoot {
			if instRes, handled, instErr := rpolListInstanceDeleteResMap(inParams); handled {
				return instRes, instErr
			}
			return res, tlerr.NotSupportedError{Format: "DELETE is not supported at a routing-policy container level; delete a list instance or a mapped child path instead", Path: inParams.requestUri}
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
		case rpolTargetPrefixSet:
			if reqInfo.targetLeaf == rpolPrefixSetModeLeaf {
				// frrcfgd drops a prefix-set that carries no mode, and every
				// prefix in it along with the set, and OpenConfig gives mode no
				// default to fall back to. So there is no state to return the
				// leaf to; removing a set means removing the set.
				return res, tlerr.NotSupportedError{Format: "DELETE is not supported for prefix-set config mode; a prefix-set always has an address family, so delete the prefix-set instead", Path: inParams.requestUri}
			}
			return rpolDeletePrefixSetResMap(inParams.dbs[db.ConfigDB], reqInfo.prefixSetName), nil
		case rpolTargetPrefixEntry:
			return rpolDeletePrefixEntryResMap(inParams.dbs[db.ConfigDB], reqInfo)
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
				fields[rpolSetIpv6NextHopGlobalFld] = ""
				fields[rpolSetOriginFld] = ""
				fields[rpolSetLocalPrefFld] = ""
				fields[rpolSetMedFld] = ""
				fields[rpolSetMetricActionFld] = ""
				fields[rpolSetMetricFld] = ""
			case rpolSetNextHopLeaf:
				fields[rpolSetNextHopFld] = ""
				fields[rpolSetIpv6NextHopGlobalFld] = ""
			case rpolSetRouteOriginLeaf:
				fields[rpolSetOriginFld] = ""
			case rpolSetLocalPrefLeaf:
				fields[rpolSetLocalPrefFld] = ""
			case rpolSetMedLeaf:
				// Both spellings go, since either may be the one holding the
				// value this leaf reports.
				fields[rpolSetMedFld] = ""
				fields[rpolSetMetricActionFld] = ""
				fields[rpolSetMetricFld] = ""
			case rpolSetMedActionLeaf:
				// CONFIG_DB cannot hold a value under an action it no longer
				// has: frrcfgd reaches set_metric only through
				// set_metric_action, so a set_metric left behind would stop
				// being applied without saying so. Dropping the action
				// therefore drops the MED setting with it, and a plain MED has
				// to be written back through set-med.
				fields[rpolSetMetricActionFld] = ""
				fields[rpolSetMetricFld] = ""
			default:
				return res, tlerr.NotSupportedError{Format: "DELETE supported only for /actions/bgp-actions/config or mapped leaves under it", Path: inParams.requestUri}
			}
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
		case rpolTargetStmtBgpSetCommunity:
			switch reqInfo.targetLeaf {
			case "":
				return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
					rpolSetCommunityInlineFld: "",
					rpolSetCommunityRefFld:    "",
				}), nil
			case rpolSetCommunityOptionsLeaf:
				// Dropping options keeps the communities and takes the marker
				// off them, which is the REPLACE form. The marker shares
				// set_community_inline with the communities, so what is left is
				// rewritten as an update rather than deleted.
				stored, _, ref := rpolSetCommunityStored(inParams.dbs[db.ConfigDB], reqInfo.policyName, reqInfo.stmtName)
				if ref != "" {
					return res, rpolSetCommunityOptionsRefErr(inParams)
				}
				if len(stored) == 0 {
					// No communities, so no marker to clear.
					return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
						rpolSetCommunityInlineFld: "",
					}), nil
				}
				if inParams.subOpDataMap == nil {
					return res, tlerr.InvalidArgsError{Format: "unable to rewrite set-community options for DELETE"}
				}
				updMap := make(RedisDbMap)
				updMap[db.ConfigDB] = makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
					rpolSetCommunityInlineFld: rpolSetCommunityInlineEncode(stored, false),
				})
				inParams.subOpDataMap[UPDATE] = &updMap
				return res, nil
			}
			return res, tlerr.NotSupportedError{Format: "DELETE supported only for /actions/bgp-actions/set-community, its config container, or /actions/bgp-actions/set-community/config/options", Path: inParams.requestUri}
		case rpolTargetStmtBgpSetCommunityInlineCfg:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolSetCommunityInlineFld: "",
			}), nil
		case rpolTargetStmtBgpSetCommunityRefCfg:
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolSetCommunityRefFld: "",
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
		// A config container payload carries the name in the tree rather than
		// in param, so the key check has to look in both places.
		if stmtObj := getRpolPolicyStmtObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false); stmtObj != nil &&
			stmtObj.Config != nil && stmtObj.Config.Name != nil {
			if name := strings.TrimSpace(*stmtObj.Config.Name); name != "" && name != reqInfo.stmtName {
				return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("statement config name payload (%s) does not match key (%s)", name, reqInfo.stmtName)}
			}
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{}), nil
	}
	if reqInfo.targetPath == rpolTargetPrefixSet {
		return rpolMapPrefixSetRequest(inParams, reqInfo)
	}
	if reqInfo.targetPath == rpolTargetPrefixEntry {
		return rpolMapPrefixEntryRequest(inParams, reqInfo)
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
			matchOpts := ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_UNSET
			if cfgObj != nil {
				matchOpts = cfgObj.MatchSetOptions
			}
			if matchOpts == ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_UNSET && reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
				return res, tlerr.InvalidArgsError{Format: "routing-policy community-set config match-set-options is missing in request payload"}
			}
			if matchOpts != ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_UNSET {
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
			matchOpts := ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_UNSET
			if cfgObj != nil {
				matchOpts = cfgObj.MatchSetOptions
			}
			if matchOpts == ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_UNSET && reqInfo.targetLeaf == rpolMatchSetOptionsLeaf {
				return res, tlerr.InvalidArgsError{Format: "routing-policy ext-community-set config match-set-options is missing in request payload"}
			}
			if matchOpts != ocbinds.OpenconfigBgpPolicy_MatchSetOptionsType_UNSET {
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
		if inParams.dbs[db.ConfigDB] == nil {
			return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for routing-policy mapping"}
		}
		// The condition names a set but says nothing about its address family,
		// and the family decides which ROUTE_MAP field carries it.
		mode, err := rpolResolvePrefixSetMode(inParams, pfxSet)
		if err != nil {
			return res, err
		}
		fields := map[string]string{}
		switch mode {
		case rpolPrefixSetModeIpv4:
			fields[rpolMatchPrefixSetFld] = pfxSet
		case rpolPrefixSetModeIpv6:
			fields[rpolMatchIpv6PrefixSetFld] = pfxSet
		default:
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported PREFIX_SET mode %s for set %s", mode, pfxSet)}
		}
		staged, err := rpolStageStaleMatchPrefixSet(inParams, reqInfo, fields)
		if err != nil {
			return res, err
		}
		if staged {
			return res, nil
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpActionsCfg {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		cfgObj := getRpolPolicyStmtBgpActionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
		fields := map[string]string{}

		nextHopGiven := false
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetNextHopLeaf {
			nh, ok := parseRpolBgpSetNextHopValue(inParams.param)
			if !ok && cfgObj != nil {
				nh, ok = parseRpolBgpSetNextHopValue(cfgObj.SetNextHop)
			}
			if ok {
				fld, fldErr := rpolSetNextHopFldFor(nh)
				if fldErr != nil {
					return res, fldErr
				}
				fields[fld] = nh
				nextHopGiven = true
			} else if reqInfo.targetLeaf == rpolSetNextHopLeaf {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-next-hop is missing in request payload"}
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

		medVal, medGiven := "", false
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetMedLeaf {
			if med, ok := parseRpolBgpSetMedValue(inParams.param); ok {
				medVal, medGiven = med, true
			} else if cfgObj != nil {
				if med, ok := parseRpolBgpSetMedValue(cfgObj.SetMed); ok {
					medVal, medGiven = med, true
				}
			}
			if reqInfo.targetLeaf == rpolSetMedLeaf && !medGiven {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-med is missing in request payload"}
			}
		}

		medAction := ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetMedActionLeaf {
			if act, ok := parseRpolSetMedActionValue(inParams.param); ok {
				medAction = act
			} else if cfgObj != nil {
				medAction = cfgObj.SetMedAction
			}
			if reqInfo.targetLeaf == rpolSetMedActionLeaf && medAction == ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config set-med-action is missing in request payload"}
			}
		}

		if medGiven || medAction != ocbinds.OpenconfigBgpPolicy_BgpSetMedAction_UNSET {
			medFields, medClear, medErr := rpolSetMedFields(inParams.dbs[db.ConfigDB], reqInfo, medVal, medGiven, medAction)
			if medErr != nil {
				return res, medErr
			}
			for fld, val := range medFields {
				fields[fld] = val
			}
			if len(medClear) > 0 {
				// The spelling being displaced cannot be cleared by writing ""
				// into it, the way the set-community pair is: CVL type checks
				// the value, and set_med, set_metric and set_metric_action are
				// a uint32, a uint32 and an enum. It has to be deleted, which a
				// DELETE sub-operation carries alongside this update.
				if err := rpolStageSubOp(inParams, DELETE, makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, medClear)); err != nil {
					return res, err
				}
			}
		}

		if len(fields) == 0 {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions config payload missing supported fields (set-next-hop/set-route-origin/set-local-pref/set-med/set-med-action)"}
		}
		if nextHopGiven {
			// A statement sets one next-hop, so a write naming the other
			// family's field replaces the one held, as the HLD requires.
			staged, err := rpolStageStaleNextHop(inParams, reqInfo, fields)
			if err != nil {
				return res, err
			}
			if staged {
				return res, nil
			}
		}
		return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, fields), nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunity {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		if reqInfo.targetLeaf == rpolSetCommunityMethodLeaf {
			return res, tlerr.NotSupportedError{Format: "SET is not supported for set-community config method; ROUTE_MAP has no method field and a GET reports the method of whichever of set_community_inline and set_community_ref holds a value, so write the communities or the reference instead", Path: inParams.requestUri}
		}
		cfgObj := getRpolPolicyStmtBgpSetCommunityCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
		if cfgObj == nil {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community config is missing in request payload"}
		}

		additive, optionsGiven, optErr := rpolSetCommunityAdditive(cfgObj.Options)
		if optErr != nil {
			return res, optErr
		}
		if reqInfo.targetLeaf == rpolSetCommunityOptionsLeaf && !optionsGiven {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community config options is missing in request payload"}
		}

		// options named without a method changes the marker on the communities
		// already stored: there is no field of its own for it to land in.
		if optionsGiven && cfgObj.Method == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_UNSET {
			return rpolSetCommunityOptionsResMap(inParams, reqInfo, additive)
		}

		switch cfgObj.Method {
		case ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_REFERENCE:
			if optionsGiven {
				return res, rpolSetCommunityOptionsRefErr(inParams)
			}
			refCfgObj := getRpolPolicyStmtBgpSetCommunityRefCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
			refName, refErr := rpolSetCommunityRefName(refCfgObj)
			if refErr != nil {
				return res, refErr
			}
			if refName == "" {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community reference config community-set-refs is missing in request payload"}
			}
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
			if len(communities) == 0 && optionsGiven {
				// options named alongside the method, with no communities of
				// its own, applies to the communities already stored.
				return rpolSetCommunityOptionsResMap(inParams, reqInfo, additive)
			}
			if len(communities) == 0 {
				return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community inline config communities is missing in request payload"}
			}
			inlineValue := rpolSetCommunityInlineEncode(communities, additive)
			return makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
				rpolSetCommunityInlineFld: inlineValue,
			}), nil
		default:
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community config method is missing or unsupported"}
		}
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunityRefCfg {
		_, err := parseRpolStmtNameToUint16(reqInfo.stmtName)
		if err != nil {
			return res, err
		}
		refCfgObj := getRpolPolicyStmtBgpSetCommunityRefCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, false)
		refName, refErr := rpolSetCommunityRefName(refCfgObj)
		if refErr != nil {
			return res, refErr
		}
		if refName == "" {
			return res, tlerr.InvalidArgsError{Format: "routing-policy statement actions bgp-actions set-community reference config community-set-refs is missing in request payload"}
		}
		// CONFIG_DB records no method of its own: a GET reports REFERENCE
		// because set_community_ref is the field that carries a value. So a
		// reference written here clears the inline spelling, exactly as a
		// request at set-community carrying method REFERENCE does, rather than
		// leaving both fields set for the GET to choose between.
		res = makeRpolRouteMapResMap(reqInfo.policyName, reqInfo.stmtName, map[string]string{
			rpolSetCommunityInlineFld: "",
			rpolSetCommunityRefFld:    refName,
		})
		res = mergeDbResMap(res, makeRpolCommunitySetResMap(refName,
			rpolCommunitySetSeedFields(inParams.dbs[db.ConfigDB], rpolCommunitySetTbl, refName, "")))
		return res, nil
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
		// options lives at set-community/config, outside this subtree, so a
		// write here keeps the marker the stored value carries rather than
		// silently turning an ADD into a REPLACE.
		_, storedAdditive, _ := rpolSetCommunityStored(inParams.dbs[db.ConfigDB], reqInfo.policyName, reqInfo.stmtName)
		inlineValue := rpolSetCommunityInlineEncode(communities, storedAdditive)
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

// rpolSubscPolicyDefsReqUriRe matches the policy definitions subtree, with the
// list key present, wildcarded or absent. A subscribe path keeps its wildcard,
// unlike a GET path, where translib removes it before dispatch.
var rpolSubscPolicyDefsReqUriRe = regexp.MustCompile(
	`^/openconfig-routing-policy:routing-policy/policy-definitions` +
		`(?:/policy-definition(?:\[name=([^\]]+)\])?(?:/(.*))?)?$`)

// Subscribe_routing_policy_xfmr reports the CONFIG_DB tables behind the policy
// definitions, which is what makes the path DB backed.
//
// Both refusals a routing-policy subscription used to get came from that one
// fact. collectNotificationPreferences clears IsOnChangeSupported and
// IsWildcardSupported for any path whose notificationAppInfo isNonDB, and
// validateNonDbPath refuses a wildcard key outright, so an on-change request was
// turned away as unsupported and a sampled one with a wildcard as a wildcard
// path. A path is non-DB precisely when no callback maps it to a table.
var Subscribe_routing_policy_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams

	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		result.isVirtualTbl = true
		return result, nil
	}

	// Which node was asked for, as opposed to which one the callback is anchored
	// on, survives only in requestURI: the subscribe path trims a trailing leaf
	// off uri. The CRUD parent-table check reaches here with requestURI unset.
	reqUri := inParams.requestURI
	if reqUri == "" {
		reqUri = inParams.uri
	}

	m := rpolSubscPolicyDefsReqUriRe.FindStringSubmatch(reqUri)
	if m == nil {
		// The rest of the subtree this transformer serves keeps the behaviour it
		// has today rather than gaining a mapping it has no tests for.
		result.isVirtualTbl = true
		return result, nil
	}

	// A definition is one ROUTE_MAP_SET row and the ROUTE_MAP rows of its
	// statements, the latter keyed <name>|<sequence>.
	name, below := m[1], m[2]
	setPatt, stmtPatt := "*", "*"
	if name != "" && name != "*" {
		setPatt, stmtPatt = name, name+"|*"
	}

	// No field is named. The only leaf directly below a policy definition is
	// name, which is the list key, and the addition or removal of the row
	// carries that on its own. A key is not a field, so nothing has to be
	// mapped for it to be reported.
	// The applied tables in STATE_DB, not the configured ones. A subscription on
	// a state path has to report what FRR took, and mapping these is also what
	// makes the path DB backed, which is what lifts the on-change and wildcard
	// refusals a non-DB path gets.
	result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {
		rpolRouteMapSetAppliedTbl: {setPatt: {}},
		rpolRouteMapAppliedTbl:    {stmtPatt: {}},
	}}
	result.isVirtualTbl = false

	// On change is offered where the row going away or arriving is the whole
	// story: the definitions container, the list, and a named definition. Deeper
	// nodes describe fields of a ROUTE_MAP row, and the notification mapping
	// reports a changed field only when it is mapped to a leaf directly below
	// the anchored node, so those keep sampling rather than being accepted and
	// then reporting nothing.
	if below == "" {
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{pType: OnChange}
		return result, nil
	}

	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}
	return result, nil
}

// rpolSchemaPathUpto truncates a schema path just after the named node, and
// reports whether the path names it at all. A yang key is looked up under the
// path of the list that holds it, so a path transformer has to answer with a
// prefix of the path it was given. Comparing on the node name alone lets a
// module qualified element match the node it names.
func rpolSchemaPathUpto(schemaPath string, node string) (string, bool) {
	var b strings.Builder
	for _, elem := range strings.Split(strings.TrimPrefix(schemaPath, "/"), "/") {
		b.WriteString("/")
		b.WriteString(elem)
		name := elem
		if i := strings.Index(name, ":"); i >= 0 {
			name = name[i+1:]
		}
		if name == node {
			return b.String(), true
		}
	}
	return "", false
}

// DbToYangPath_routing_policy_path_xfmr names the policy, and where the path
// reaches it the statement, that an applied row belongs to.
//
// A subscription needs this whenever translib has to supply a key rather than
// read one from the request: with a wildcard or an omitted key it holds a
// changed ROUTE_MAP_APPLIED row and no way to say which policy-definition
// instance the notification is for. Without this callback such a subscription
// is accepted and then reports nothing, which is how the AFT lists behaved
// before they were given one. A fully keyed subscription never needed it,
// since the path already carries the key.
var DbToYangPath_routing_policy_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	var policy, statement string

	switch params.tblName {
	case rpolRouteMapSetAppliedTbl:
		if len(params.tblKeyComp) != 1 || params.tblKeyComp[0] == "" {
			return fmt.Errorf("routing_policy_path_xfmr: %s key %v does not name a policy",
				params.tblName, params.tblKeyComp)
		}
		policy = params.tblKeyComp[0]
	case rpolRouteMapAppliedTbl:
		// <policy>|<sequence>, and the sequence is the statement name.
		if len(params.tblKeyComp) != 2 || params.tblKeyComp[0] == "" || params.tblKeyComp[1] == "" {
			return fmt.Errorf("routing_policy_path_xfmr: %s key %v does not name a policy and a statement",
				params.tblName, params.tblKeyComp)
		}
		policy, statement = params.tblKeyComp[0], params.tblKeyComp[1]
	default:
		// A row of a table these lists do not serve is not a fault, so it is
		// declined as unrelated rather than reported as an error.
		return fmt.Errorf("routing_policy_path_xfmr: notification from table %q: %w",
			params.tblName, apis.ErrUnrelatedEntry)
	}

	// A path above the policy definitions holds no key of this row to fill in,
	// so there is nothing to answer with. That is not a fault: the same thing
	// happens on the AFT side for a path anchored above its entry list.
	defPath, ok := rpolSchemaPathUpto(params.ygSchemaPath, rpolPolicyDefNode)
	if !ok {
		return nil
	}
	params.ygPathKeys[defPath+"/"+rpolPolicyDefNameLeaf] = policy

	// A path anchored at or above the policy names no statement, so there is no
	// statement key to answer with even when the row carries one.
	if statement != "" {
		if stmtPath, ok := rpolSchemaPathUpto(params.ygSchemaPath, rpolStatementNode); ok {
			params.ygPathKeys[stmtPath+"/"+rpolStmtNameLeaf] = statement
		}
	}

	return nil
}

func rpolDbToYangXfmr(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("routing-policy transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy transformer panic in GET path: %v", r)}
		}
	}()

	if handled, leafErr := rpolLeavesDbToYang(inParams); handled {
		return leafErr
	}

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
		ocMirrorConfigToState(getRpolPolicyDefObj(inParams.ygRoot, reqInfo.policyName))
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
		// Keep the key leaf and the config leaf in step on GET so that both
		// /name and /config/name answer with the same value.
		if stmtObj.Config != nil {
			stmtObj.Config.Name = &name
		}
		ocMirrorConfigToState(stmtObj)
		return nil
	}
	if reqInfo.targetPath == rpolTargetPrefixSet {
		if err := rpolReadPrefixSet(inParams, reqInfo); err != nil {
			return err
		}
		ocMirrorConfigToState(getRpolPrefixSetObj(inParams.ygRoot, reqInfo.prefixSetName, false))
		return nil
	}
	if reqInfo.targetPath == rpolTargetPrefixEntry {
		return rpolReadPrefixEntry(inParams, reqInfo)
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
		ocMirrorConfigToState(csObj)
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
		ocMirrorConfigToState(ecObj)
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

	if reqInfo.targetPath == rpolTargetStmtActionsState {
		stateObj := getRpolPolicyStmtActionsStateObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if stateObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions state object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPolicyResultLeaf {
			// The applied row, not the configured one: this is a state path, so
			// it reports what FRR took rather than what was asked for. The two
			// agree in the ordinary case and diverge exactly when it matters.
			applied, ok := rpolAppliedStmtEntry(inParams, reqInfo.policyName, reqInfo.stmtName)
			if ok {
				routeOp := applied.Field[rpolRouteOperationFld]
				if strings.TrimSpace(routeOp) != "" {
					if polRes, conv := rpolRouteOpToPolicyResult(routeOp); conv {
						stateObj.PolicyResult = polRes
					}
				}
			}
		}
		return nil
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
	if reqInfo.targetPath == rpolTargetStmtCondMatchPfxState {
		applied, appliedOk := rpolAppliedStmtEntry(inParams, reqInfo.policyName, reqInfo.stmtName)
		if !appliedOk {
			return nil
		}
		stateObj := getRpolPolicyStmtCondMatchPrefixSetStateObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if stateObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement conditions match-prefix-set state object in YANG tree"}
		}
		matched := false
		if pfxSet, ok := applied.Field[rpolMatchPrefixSetFld]; ok && strings.TrimSpace(pfxSet) != "" {
			matched = true
			if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetLeaf {
				stateObj.PrefixSet = ygot.String(strings.TrimSpace(pfxSet))
			}
		} else if pfxSet6, ok := applied.Field[rpolMatchIpv6PrefixSetFld]; ok && strings.TrimSpace(pfxSet6) != "" {
			matched = true
			if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetLeaf {
				stateObj.PrefixSet = ygot.String(strings.TrimSpace(pfxSet6))
			}
		}
		if matched && (reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolMatchSetOptionsLeaf) {
			// The modifier is not stored because the match is always ANY, which
			// is also the YANG default. Report it only once a prefix-set is
			// actually configured, so the answer describes a real condition.
			stateObj.MatchSetOptions = ocbinds.OpenconfigRoutingPolicy_MatchSetOptionsRestrictedType_ANY
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
	// The applied row, read the way the config branch below reads the
	// configured one. Derived from it deliberately: the two decode the same
	// fields into distinct generated types.
	if reqInfo.targetPath == rpolTargetStmtBgpActionsState {
		stateObj := getRpolPolicyStmtBgpActionsStateObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if stateObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions state object in YANG tree"}
		}
		applied, appliedOk := rpolAppliedStmtEntry(inParams, reqInfo.policyName, reqInfo.stmtName)
		if !appliedOk {
			return nil
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetNextHopLeaf {
			if nextHop, ok := rpolBgpNextHopFromRow(applied); ok {
				if strings.EqualFold(nextHop, "self") {
					stateObj.SetNextHop = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_State_SetNextHop_Union_E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop{
						E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop: ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetNextHop_SELF,
					}
				} else {
					stateObj.SetNextHop = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_State_SetNextHop_Union_String{
						String: nextHop,
					}
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetRouteOriginLeaf {
			if originStr, ok := applied.Field[rpolSetOriginFld]; ok && strings.TrimSpace(originStr) != "" {
				if origin, conv := rpolDbToBgpOrigin(originStr); conv {
					stateObj.SetRouteOrigin = origin
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetLocalPrefLeaf {
			if lpStr, ok := applied.Field[rpolSetLocalPrefFld]; ok && strings.TrimSpace(lpStr) != "" {
				if lp, convErr := strconv.ParseUint(strings.TrimSpace(lpStr), 10, 32); convErr == nil {
					stateObj.SetLocalPref = ygot.Uint32(uint32(lp))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetMedLeaf || reqInfo.targetLeaf == rpolSetMedActionLeaf {
			// The row carries the MED in one spelling or the other, never both,
			// and the action spelling is the one to read when it is present.
			medVal := strings.TrimSpace(applied.Field[rpolSetMedFld])
			medAction, haveAction := rpolDbToSetMedAction(applied.Field[rpolSetMetricActionFld])
			if haveAction {
				medVal = strings.TrimSpace(applied.Field[rpolSetMetricFld])
			}
			if haveAction && reqInfo.targetLeaf != rpolSetMedLeaf {
				stateObj.SetMedAction = medAction
			}
			if medVal != "" && reqInfo.targetLeaf != rpolSetMedActionLeaf {
				if med, convErr := strconv.ParseUint(medVal, 10, 32); convErr == nil {
					stateObj.SetMed = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_State_SetMed_Union_Uint32{
						Uint32: uint32(med),
					}
				} else if strings.EqualFold(medVal, "igp") {
					stateObj.SetMed = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_State_SetMed_Union_E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed{
						E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed: ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_IGP,
					}
				}
				// A row holding a signed value was written while the model
				// still carried a string member for it, which 8.0.0 removed.
				// Such a row is left out rather than reported as the bare
				// number, which would drop the sign and claim the opposite of
				// what is configured. set-med-action expresses it now.
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpActionsCfg {
		cfgObj := getRpolPolicyStmtBgpActionsCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions config object in YANG tree"}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetNextHopLeaf {
			if nextHop, ok := rpolBgpNextHopFromRow(entry); ok {
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
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetMedLeaf || reqInfo.targetLeaf == rpolSetMedActionLeaf {
			// The row carries the MED in one spelling or the other, never both,
			// and the action spelling is the one to read when it is present.
			medVal := strings.TrimSpace(entry.Field[rpolSetMedFld])
			medAction, haveAction := rpolDbToSetMedAction(entry.Field[rpolSetMetricActionFld])
			if haveAction {
				medVal = strings.TrimSpace(entry.Field[rpolSetMetricFld])
			}
			if haveAction && reqInfo.targetLeaf != rpolSetMedLeaf {
				cfgObj.SetMedAction = medAction
			}
			if medVal != "" && reqInfo.targetLeaf != rpolSetMedActionLeaf {
				if med, convErr := strconv.ParseUint(medVal, 10, 32); convErr == nil {
					cfgObj.SetMed = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_Uint32{
						Uint32: uint32(med),
					}
				} else if strings.EqualFold(medVal, "igp") {
					cfgObj.SetMed = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_Union_E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed{
						E_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed: ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_Config_SetMed_IGP,
					}
				}
				// A row holding a signed value was written while the model
				// still carried a string member for it, which 8.0.0 removed.
				// Such a row is left out rather than reported as the bare
				// number, which would drop the sign and claim the opposite of
				// what is configured. set-med-action expresses it now.
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunity {
		cfgObj := getRpolPolicyStmtBgpSetCommunityCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if cfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-community config object in YANG tree"}
		}
		reportMethod := reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetCommunityMethodLeaf
		reportOptions := reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolSetCommunityOptionsLeaf
		if ref, ok := entry.Field[rpolSetCommunityRefFld]; ok && strings.TrimSpace(ref) != "" {
			if reportMethod {
				cfgObj.Method = ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_REFERENCE
			}
			// options is not reported under REFERENCE: no field records it, so
			// there is nothing to report.
			refCfgObj := getRpolPolicyStmtBgpSetCommunityRefCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
			if refCfgObj != nil {
				// Both spellings are reported. 6.2.0 deprecated the singular
				// leaf rather than removing it, so a client written against
				// either revision of the model finds the spelling it knows.
				refName := strings.TrimSpace(ref)
				refCfgObj.CommunitySetRefs = []string{refName}
				refCfgObj.CommunitySetRef = ygot.String(refName)
			}
		} else if inline, ok := entry.Field[rpolSetCommunityInlineFld]; ok && strings.TrimSpace(inline) != "" {
			if reportMethod {
				cfgObj.Method = ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE
			}
			decoded, additive := rpolSetCommunityInlineDecode(inline)
			if reportOptions {
				// A stored value without the marker renders as
				// "set community X", which replaces the attribute, so it
				// reports REPLACE rather than leaving options unset.
				if additive {
					cfgObj.Options = ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD
				} else {
					cfgObj.Options = ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_REPLACE
				}
			}
			inlineCfgObj := getRpolPolicyStmtBgpSetCommunityInlineCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
			if inlineCfgObj != nil {
				inlineCfgObj.Communities = make([]ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union, 0, len(decoded))
				for _, c := range decoded {
					unionVal, err := inlineCfgObj.To_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union(rpolSetCommunityInlineMemberToYang(c))
					if err == nil {
						inlineCfgObj.Communities = append(inlineCfgObj.Communities, unionVal)
					}
				}
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunityRefState {
		applied, appliedOk := rpolAppliedStmtEntry(inParams, reqInfo.policyName, reqInfo.stmtName)
		if !appliedOk {
			return nil
		}
		ref, ok := applied.Field[rpolSetCommunityRefFld]
		if !ok || strings.TrimSpace(ref) == "" {
			return nil
		}
		refCfgObj := getRpolPolicyStmtBgpSetCommunityRefStateObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if refCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-community reference state object in YANG tree"}
		}
		refName := strings.TrimSpace(ref)
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunitySetRefsLeaf {
			refCfgObj.CommunitySetRefs = []string{refName}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunitySetRefLeaf {
			refCfgObj.CommunitySetRef = ygot.String(refName)
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunityRefCfg {
		ref, ok := entry.Field[rpolSetCommunityRefFld]
		if !ok || strings.TrimSpace(ref) == "" {
			return nil
		}
		refCfgObj := getRpolPolicyStmtBgpSetCommunityRefCfgObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if refCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-community reference config object in YANG tree"}
		}
		refName := strings.TrimSpace(ref)
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunitySetRefsLeaf {
			refCfgObj.CommunitySetRefs = []string{refName}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunitySetRefLeaf {
			refCfgObj.CommunitySetRef = ygot.String(refName)
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetCommunityInlineState {
		applied, appliedOk := rpolAppliedStmtEntry(inParams, reqInfo.policyName, reqInfo.stmtName)
		if !appliedOk {
			return nil
		}
		inlineCfgObj := getRpolPolicyStmtBgpSetCommunityInlineStateObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if inlineCfgObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-community inline state object in YANG tree"}
		}
		inline, ok := applied.Field[rpolSetCommunityInlineFld]
		if !ok || strings.TrimSpace(inline) == "" {
			return nil
		}
		decoded, _ := rpolSetCommunityInlineDecode(inline)
		inlineCfgObj.Communities = make([]ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_State_Communities_Union, 0, len(decoded))
		for _, c := range decoded {
			unionVal, err := inlineCfgObj.To_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_State_Communities_Union(rpolSetCommunityInlineMemberToYang(c))
			if err == nil {
				inlineCfgObj.Communities = append(inlineCfgObj.Communities, unionVal)
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
			unionVal, err := inlineCfgObj.To_OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union(rpolSetCommunityInlineMemberToYang(c))
			if err == nil {
				inlineCfgObj.Communities = append(inlineCfgObj.Communities, unionVal)
			}
		}
		return nil
	}
	if reqInfo.targetPath == rpolTargetStmtBgpSetAsPathState {
		applied, appliedOk := rpolAppliedStmtEntry(inParams, reqInfo.policyName, reqInfo.stmtName)
		if !appliedOk {
			return nil
		}
		stateObj := getRpolPolicyStmtBgpSetAsPathPrependStateObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if stateObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement actions bgp-actions set-as-path-prepend state object in YANG tree"}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolAsnLeaf {
			if asnStr, ok := applied.Field[rpolSetAsnFld]; ok && strings.TrimSpace(asnStr) != "" {
				if asn, convErr := strconv.ParseUint(strings.TrimSpace(asnStr), 10, 32); convErr == nil {
					stateObj.Asn = ygot.Uint32(uint32(asn))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolRepeatNLeaf {
			if repStr, ok := applied.Field[rpolSetRepeatAsnFld]; ok && strings.TrimSpace(repStr) != "" {
				if rep, convErr := strconv.ParseUint(strings.TrimSpace(repStr), 10, 8); convErr == nil {
					stateObj.RepeatN = ygot.Uint8(uint8(rep))
				}
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
	if reqInfo.targetPath == rpolTargetStmtBgpConditionsState {
		applied, appliedOk := rpolAppliedStmtEntry(inParams, reqInfo.policyName, reqInfo.stmtName)
		if !appliedOk {
			return nil
		}
		stateObj := getRpolPolicyStmtBgpConditionsStateObj(inParams.ygRoot, reqInfo.policyName, reqInfo.stmtName, true)
		if stateObj == nil {
			return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy statement conditions bgp-conditions state object in YANG tree"}
		}

		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolMedEqLeaf {
			if medStr, ok := applied.Field[rpolMatchMedFld]; ok && strings.TrimSpace(medStr) != "" {
				if med, convErr := strconv.ParseUint(strings.TrimSpace(medStr), 10, 32); convErr == nil {
					stateObj.MedEq = ygot.Uint32(uint32(med))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolOriginEqLeaf {
			if originStr, ok := applied.Field[rpolMatchOriginFld]; ok && strings.TrimSpace(originStr) != "" {
				if origin, conv := rpolDbToBgpOrigin(originStr); conv {
					stateObj.OriginEq = origin
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolLocalPrefEqLeaf {
			if lpStr, ok := applied.Field[rpolMatchLocalPrefFld]; ok && strings.TrimSpace(lpStr) != "" {
				if lp, convErr := strconv.ParseUint(strings.TrimSpace(lpStr), 10, 32); convErr == nil {
					stateObj.LocalPrefEq = ygot.Uint32(uint32(lp))
				}
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolCommunitySetLeaf {
			if communitySet, ok := applied.Field[rpolMatchCommunityFld]; ok && strings.TrimSpace(communitySet) != "" {
				stateObj.CommunitySet = ygot.String(strings.TrimSpace(communitySet))
			}
		}
		if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolExtCommunitySetLeaf {
			if extCommunitySet, ok := applied.Field[rpolMatchExtCommunityFld]; ok && strings.TrimSpace(extCommunitySet) != "" {
				stateObj.ExtCommunitySet = ygot.String(strings.TrimSpace(extCommunitySet))
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
