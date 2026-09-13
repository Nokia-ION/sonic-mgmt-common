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
	"sort"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	// rpolMaxTreeDepth bounds the recursion done while walking a routing-policy
	// payload subtree.
	rpolMaxTreeDepth = 32
	rpolUriRoot      = "/openconfig-routing-policy:routing-policy"
)

// rpolMappedPayloadLeaves lists every leaf, relative to the routing-policy
// container, that a container-level request can carry. List keys are included
// because a payload repeats them alongside the data they identify.
var rpolMappedPayloadLeaves = map[string]bool{
	"policy-definitions/policy-definition/name":                                                                                       true,
	"policy-definitions/policy-definition/config/name":                                                                                true,
	"policy-definitions/policy-definition/statements/statement/name":                                                                  true,
	"policy-definitions/policy-definition/statements/statement/config/name":                                                           true,
	"policy-definitions/policy-definition/statements/statement/actions/config/policy-result":                                          true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/config/set-next-hop":                               true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/config/set-route-origin":                           true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/config/set-local-pref":                             true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/config/set-med":                                    true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/config/set-med-action":                             true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/set-community/config/method":                       true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/set-community/config/options":                      true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/set-community/inline/config/communities":           true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/set-community/reference/config/community-set-ref":  true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/set-community/reference/config/community-set-refs": true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/set-as-path-prepend/config/asn":                    true,
	"policy-definitions/policy-definition/statements/statement/actions/bgp-actions/set-as-path-prepend/config/repeat-n":               true,
	"policy-definitions/policy-definition/statements/statement/conditions/match-prefix-set/config/prefix-set":                         true,
	"policy-definitions/policy-definition/statements/statement/conditions/match-prefix-set/config/match-set-options":                  true,
	"policy-definitions/policy-definition/statements/statement/conditions/bgp-conditions/config/med-eq":                               true,
	"policy-definitions/policy-definition/statements/statement/conditions/bgp-conditions/config/origin-eq":                            true,
	"policy-definitions/policy-definition/statements/statement/conditions/bgp-conditions/config/local-pref-eq":                        true,
	"policy-definitions/policy-definition/statements/statement/conditions/bgp-conditions/config/community-set":                        true,
	"policy-definitions/policy-definition/statements/statement/conditions/bgp-conditions/config/ext-community-set":                    true,
	"defined-sets/bgp-defined-sets/community-sets/community-set/community-set-name":                                                   true,
	"defined-sets/bgp-defined-sets/community-sets/community-set/config/community-set-name":                                            true,
	"defined-sets/bgp-defined-sets/community-sets/community-set/config/community-member":                                              true,
	"defined-sets/bgp-defined-sets/community-sets/community-set/config/match-set-options":                                             true,
	"defined-sets/prefix-sets/prefix-set/name":                                                                                        true,
	"defined-sets/prefix-sets/prefix-set/config/name":                                                                                 true,
	"defined-sets/prefix-sets/prefix-set/config/mode":                                                                                 true,
	"defined-sets/prefix-sets/prefix-set/prefixes/prefix/ip-prefix":                                                                   true,
	"defined-sets/prefix-sets/prefix-set/prefixes/prefix/masklength-range":                                                            true,
	"defined-sets/prefix-sets/prefix-set/prefixes/prefix/config/ip-prefix":                                                            true,
	"defined-sets/prefix-sets/prefix-set/prefixes/prefix/config/masklength-range":                                                     true,
	"defined-sets/bgp-defined-sets/ext-community-sets/ext-community-set/ext-community-set-name":                                       true,
	"defined-sets/bgp-defined-sets/ext-community-sets/ext-community-set/config/ext-community-set-name":                                true,
	"defined-sets/bgp-defined-sets/ext-community-sets/ext-community-set/config/ext-community-member":                                  true,
	"defined-sets/bgp-defined-sets/ext-community-sets/ext-community-set/config/match-set-options":                                     true,
}

// Per-node request URIs. A container-level request is served by replaying the
// leaf-level handlers, one per mapped node the request touches, so that both
// levels apply the identical CONFIG_DB mapping.
func rpolPolicyDefCfgUri(policyName string) string {
	return fmt.Sprintf("%s/policy-definitions/policy-definition[name=%s]/config", rpolUriRoot, policyName)
}

func rpolStmtUriPrefix(policyName, stmtName string) string {
	return fmt.Sprintf("%s/policy-definitions/policy-definition[name=%s]/statements/statement[name=%s]",
		rpolUriRoot, policyName, stmtName)
}

func rpolStmtNameUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/name"
}

func rpolStmtActionsCfgUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/actions/config"
}

func rpolStmtBgpActionsCfgUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/actions/bgp-actions/config"
}

func rpolStmtBgpSetCommunityUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/actions/bgp-actions/set-community"
}

func rpolStmtBgpSetCommunityInlineCfgUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/actions/bgp-actions/set-community/inline/config"
}

func rpolStmtBgpSetAsPathCfgUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/actions/bgp-actions/set-as-path-prepend/config"
}

func rpolStmtCondMatchPfxCfgUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/conditions/match-prefix-set/config"
}

func rpolStmtBgpConditionsCfgUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/conditions/bgp-conditions/config"
}

// rpolCommunitySetCfgUri targets the config container so that one replay maps
// every mapped leaf of the set rather than just its name.
func rpolCommunitySetCfgUri(communitySetName string) string {
	return fmt.Sprintf("%s/defined-sets/bgp-defined-sets/community-sets/community-set[community-set-name=%s]/config",
		rpolUriRoot, communitySetName)
}

func rpolExtCommunitySetCfgUri(communitySetName string) string {
	return fmt.Sprintf("%s/defined-sets/bgp-defined-sets/ext-community-sets/ext-community-set[ext-community-set-name=%s]/config",
		rpolUriRoot, communitySetName)
}

// rpolPrefixSetCfgUri targets the config container of a prefix-set. The
// prefix-set node maps and populates the prefixes it holds as well, so one
// replay covers the whole set and the sequence numbers of the prefixes a single
// request adds are allocated together.
func rpolPrefixSetCfgUri(prefixSetName string) string {
	return fmt.Sprintf("%s/defined-sets/prefix-sets/prefix-set[name=%s]/config", rpolUriRoot, prefixSetName)
}

// lookupRpolObj returns the routing-policy container of the request without
// creating anything, so that a payload can be inspected as it was sent.
func lookupRpolObj(s *ygot.GoStruct) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy {
	if s == nil || *s == nil {
		return nil
	}
	if rpObj, ok := (*s).(*ocbinds.OpenconfigRoutingPolicy_RoutingPolicy); ok {
		return rpObj
	}
	if deviceObj, ok := (*s).(*ocbinds.Device); ok && deviceObj != nil {
		return deviceObj.RoutingPolicy
	}
	return nil
}

// wrapRpolRootAsDevice returns a Device rooted tree for the request. A request
// anchored at the routing-policy container can arrive with that container as
// the bind root, while every object helper in this package navigates from
// Device. The original container pointer is reused so that population through
// the wrapper updates the caller-owned object.
func wrapRpolRootAsDevice(s *ygot.GoStruct) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for routing-policy mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}
	rpObj, ok := (*s).(*ocbinds.OpenconfigRoutingPolicy_RoutingPolicy)
	if !ok || rpObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for routing-policy mapping"}
	}

	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	devObj.RoutingPolicy = rpObj

	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}

// findRpolUnmappedPayloadLeaves returns the payload leaves under the
// routing-policy container that carry a value but have no CONFIG_DB mapping, so
// that a container-level request is rejected as a whole instead of applying
// part of it and dropping the rest silently.
func findRpolUnmappedPayloadLeaves(s *ygot.GoStruct) []string {
	rpObj := lookupRpolObj(s)
	if rpObj == nil {
		return nil
	}
	var unmapped []string
	collectRpolUnmappedLeaves(reflect.ValueOf(rpObj), "", "routing-policy", 0, &unmapped)
	sort.Strings(unmapped)
	return unmapped
}

func collectRpolUnmappedLeaves(v reflect.Value, schemaPath, displayPath string, depth int, unmapped *[]string) {
	if depth >= rpolMaxTreeDepth {
		return
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Tag.Get("path")
		if name == "" {
			continue
		}
		f := v.Field(i)
		childSchema := name
		if schemaPath != "" {
			childSchema = schemaPath + "/" + name
		}
		childDisplay := displayPath + "/" + name

		switch f.Kind() {
		case reflect.Ptr:
			if f.IsNil() {
				continue
			}
			if f.Type().Elem().Kind() == reflect.Struct {
				collectRpolUnmappedLeaves(f, childSchema, childDisplay, depth+1, unmapped)
				continue
			}
		case reflect.Map:
			iter := f.MapRange()
			for iter.Next() {
				keyed := fmt.Sprintf("%s[%v]", childDisplay, iter.Key().Interface())
				collectRpolUnmappedLeaves(iter.Value(), childSchema, keyed, depth+1, unmapped)
			}
			continue
		case reflect.Slice:
			if f.Len() == 0 {
				continue
			}
		case reflect.Interface:
			if f.IsNil() {
				continue
			}
		case reflect.Int64:
			// Generated enum leaf; the zero value means unset.
			if f.Int() == 0 {
				continue
			}
		default:
			continue
		}

		if !rpolMappedPayloadLeaves[childSchema] {
			*unmapped = append(*unmapped, childDisplay)
		}
	}
}

// rpolContainerSetTargetUris returns the request URI of every mapped node the
// payload sets, in an order that creates a policy before its statements.
func rpolContainerSetTargetUris(s *ygot.GoStruct) ([]string, error) {
	rpObj := lookupRpolObj(s)
	if rpObj == nil {
		return nil, nil
	}

	var uris []string
	if rpObj.PolicyDefinitions != nil {
		policyNames := make([]string, 0, len(rpObj.PolicyDefinitions.PolicyDefinition))
		for policyName := range rpObj.PolicyDefinitions.PolicyDefinition {
			policyNames = append(policyNames, policyName)
		}
		sort.Strings(policyNames)

		for _, policyName := range policyNames {
			pdObj := rpObj.PolicyDefinitions.PolicyDefinition[policyName]
			if pdObj == nil {
				continue
			}
			if pdObj.Config != nil && pdObj.Config.Name != nil {
				if name := strings.TrimSpace(*pdObj.Config.Name); name != "" && name != policyName {
					return nil, tlerr.InvalidArgsError{
						Format: fmt.Sprintf("policy-definition config name payload (%s) does not match key (%s)", name, policyName),
					}
				}
			}
			// Naming the policy is itself a request for the row to exist.
			uris = append(uris, rpolPolicyDefCfgUri(policyName))

			if pdObj.Statements == nil {
				continue
			}
			stmtNames := make([]string, 0, len(pdObj.Statements.Statement))
			for stmtName := range pdObj.Statements.Statement {
				stmtNames = append(stmtNames, stmtName)
			}
			sort.Strings(stmtNames)

			for _, stmtName := range stmtNames {
				stmtObj := pdObj.Statements.Statement[stmtName]
				if stmtObj == nil {
					continue
				}
				if stmtObj.Config != nil && stmtObj.Config.Name != nil {
					if name := strings.TrimSpace(*stmtObj.Config.Name); name != "" && name != stmtName {
						return nil, tlerr.InvalidArgsError{
							Format: fmt.Sprintf("statement config name payload (%s) does not match key (%s)", name, stmtName),
						}
					}
				}
				uris = append(uris, rpolStmtNameUri(policyName, stmtName))
				uris = append(uris, rpolStmtSetTargetUris(policyName, stmtName, stmtObj)...)
			}
		}
	}

	if rpObj.DefinedSets != nil && rpObj.DefinedSets.BgpDefinedSets != nil && rpObj.DefinedSets.BgpDefinedSets.CommunitySets != nil {
		setNames := make([]string, 0, len(rpObj.DefinedSets.BgpDefinedSets.CommunitySets.CommunitySet))
		for setName := range rpObj.DefinedSets.BgpDefinedSets.CommunitySets.CommunitySet {
			setNames = append(setNames, setName)
		}
		sort.Strings(setNames)
		for _, setName := range setNames {
			csObj := rpObj.DefinedSets.BgpDefinedSets.CommunitySets.CommunitySet[setName]
			if csObj == nil {
				continue
			}
			if csObj.Config != nil && csObj.Config.CommunitySetName != nil {
				if name := strings.TrimSpace(*csObj.Config.CommunitySetName); name != "" && name != setName {
					return nil, tlerr.InvalidArgsError{
						Format: fmt.Sprintf("community-set-name payload (%s) does not match key (%s)", name, setName),
					}
				}
			}
			uris = append(uris, rpolCommunitySetCfgUri(setName))
		}
	}

	if rpObj.DefinedSets != nil && rpObj.DefinedSets.BgpDefinedSets != nil && rpObj.DefinedSets.BgpDefinedSets.ExtCommunitySets != nil {
		setNames := make([]string, 0, len(rpObj.DefinedSets.BgpDefinedSets.ExtCommunitySets.ExtCommunitySet))
		for setName := range rpObj.DefinedSets.BgpDefinedSets.ExtCommunitySets.ExtCommunitySet {
			setNames = append(setNames, setName)
		}
		sort.Strings(setNames)
		for _, setName := range setNames {
			ecObj := rpObj.DefinedSets.BgpDefinedSets.ExtCommunitySets.ExtCommunitySet[setName]
			if ecObj == nil {
				continue
			}
			if ecObj.Config != nil && ecObj.Config.ExtCommunitySetName != nil {
				if name := strings.TrimSpace(*ecObj.Config.ExtCommunitySetName); name != "" && name != setName {
					return nil, tlerr.InvalidArgsError{
						Format: fmt.Sprintf("ext-community-set-name payload (%s) does not match key (%s)", name, setName),
					}
				}
			}
			uris = append(uris, rpolExtCommunitySetCfgUri(setName))
		}
	}

	if rpObj.DefinedSets != nil && rpObj.DefinedSets.PrefixSets != nil {
		setNames := make([]string, 0, len(rpObj.DefinedSets.PrefixSets.PrefixSet))
		for setName := range rpObj.DefinedSets.PrefixSets.PrefixSet {
			setNames = append(setNames, setName)
		}
		sort.Strings(setNames)
		for _, setName := range setNames {
			psObj := rpObj.DefinedSets.PrefixSets.PrefixSet[setName]
			if psObj == nil {
				continue
			}
			if psObj.Config != nil && psObj.Config.Name != nil {
				if name := strings.TrimSpace(*psObj.Config.Name); name != "" && name != setName {
					return nil, tlerr.InvalidArgsError{
						Format: fmt.Sprintf("prefix-set config name payload (%s) does not match key (%s)", name, setName),
					}
				}
			}
			uris = append(uris, rpolPrefixSetCfgUri(setName))
		}
	}

	return uris, nil
}

func rpolStmtSetTargetUris(policyName, stmtName string, stmtObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement) []string {
	var uris []string
	if actions := stmtObj.Actions; actions != nil {
		if actions.Config != nil && actions.Config.PolicyResult != ocbinds.OpenconfigRoutingPolicy_PolicyResultType_UNSET {
			uris = append(uris, rpolStmtActionsCfgUri(policyName, stmtName))
		}
		if bgpActions := actions.BgpActions; bgpActions != nil {
			if cfg := bgpActions.Config; cfg != nil {
				if cfg.SetNextHop != nil || cfg.SetMed != nil || cfg.SetLocalPref != nil ||
					cfg.SetRouteOrigin != ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_UNSET {
					uris = append(uris, rpolStmtBgpActionsCfgUri(policyName, stmtName))
				}
			}
			if setCommunity := bgpActions.SetCommunity; setCommunity != nil {
				switch {
				case setCommunity.Config != nil &&
					(setCommunity.Config.Method != ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_UNSET ||
						setCommunity.Config.Options != ocbinds.OpenconfigBgpPolicy_BgpSetCommunityOptionType_UNSET):
					// The set-community node resolves the inline or reference
					// method itself, so it covers both of its child configs.
					uris = append(uris, rpolStmtBgpSetCommunityUri(policyName, stmtName))
				case setCommunity.Inline != nil && setCommunity.Inline.Config != nil && len(setCommunity.Inline.Config.Communities) > 0:
					uris = append(uris, rpolStmtBgpSetCommunityInlineCfgUri(policyName, stmtName))
				}
			}
			if setAsPath := bgpActions.SetAsPathPrepend; setAsPath != nil && setAsPath.Config != nil {
				if setAsPath.Config.Asn != nil || setAsPath.Config.RepeatN != nil {
					uris = append(uris, rpolStmtBgpSetAsPathCfgUri(policyName, stmtName))
				}
			}
		}
	}
	if conditions := stmtObj.Conditions; conditions != nil {
		if matchPfx := conditions.MatchPrefixSet; matchPfx != nil && matchPfx.Config != nil && matchPfx.Config.PrefixSet != nil {
			uris = append(uris, rpolStmtCondMatchPfxCfgUri(policyName, stmtName))
		}
		if bgpConditions := conditions.BgpConditions; bgpConditions != nil {
			if cfg := bgpConditions.Config; cfg != nil {
				if cfg.MedEq != nil || cfg.LocalPrefEq != nil || cfg.CommunitySet != nil || cfg.ExtCommunitySet != nil ||
					cfg.OriginEq != ocbinds.OpenconfigBgpPolicy_BgpOriginAttrType_UNSET {
					uris = append(uris, rpolStmtBgpConditionsCfgUri(policyName, stmtName))
				}
			}
		}
	}
	return uris
}

// rpolContainerGetTargetUris returns the request URI of every mapped node that
// CONFIG_DB currently holds data for.
func rpolContainerGetTargetUris(cfgDb *db.DB) []string {
	var uris []string

	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolRouteMapSetTbl}); err == nil {
		policyNames := make([]string, 0, len(keys))
		for _, k := range keys {
			if len(k.Comp) != 1 || strings.TrimSpace(k.Comp[0]) == "" {
				continue
			}
			policyNames = append(policyNames, k.Comp[0])
		}
		sort.Strings(policyNames)
		for _, policyName := range policyNames {
			uris = append(uris, rpolPolicyDefCfgUri(policyName))
		}
	}

	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolRouteMapTbl}); err == nil {
		stmtKeys := make([][2]string, 0, len(keys))
		for _, k := range keys {
			if len(k.Comp) != 2 {
				continue
			}
			stmtKeys = append(stmtKeys, [2]string{k.Comp[0], k.Comp[1]})
		}
		sort.Slice(stmtKeys, func(i, j int) bool {
			if stmtKeys[i][0] != stmtKeys[j][0] {
				return stmtKeys[i][0] < stmtKeys[j][0]
			}
			return stmtKeys[i][1] < stmtKeys[j][1]
		})
		for _, sk := range stmtKeys {
			policyName, stmtName := sk[0], sk[1]
			entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolRouteMapTbl}, db.Key{Comp: []string{policyName, stmtName}})
			if err != nil {
				continue
			}
			uris = append(uris, rpolStmtNameUri(policyName, stmtName))
			uris = append(uris, rpolStmtGetTargetUris(policyName, stmtName, entry)...)
		}
	}

	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolCommunitySetTbl}); err == nil {
		setNames := make([]string, 0, len(keys))
		for _, k := range keys {
			if len(k.Comp) != 1 || strings.TrimSpace(k.Comp[0]) == "" {
				continue
			}
			setNames = append(setNames, k.Comp[0])
		}
		sort.Strings(setNames)
		for _, setName := range setNames {
			uris = append(uris, rpolCommunitySetCfgUri(setName))
		}
	}

	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolExtCommunitySetTbl}); err == nil {
		setNames := make([]string, 0, len(keys))
		for _, k := range keys {
			if len(k.Comp) != 1 || strings.TrimSpace(k.Comp[0]) == "" {
				continue
			}
			setNames = append(setNames, k.Comp[0])
		}
		sort.Strings(setNames)
		for _, setName := range setNames {
			uris = append(uris, rpolExtCommunitySetCfgUri(setName))
		}
	}

	for _, setName := range rpolPrefixSetNamesFromDb(cfgDb) {
		uris = append(uris, rpolPrefixSetCfgUri(setName))
	}

	return uris
}

// rpolStmtGetTargetUris keeps the populated subtree limited to the nodes the
// statement row actually holds, so a container GET does not report empty
// containers for every OC node the model allows.
func rpolStmtGetTargetUris(policyName, stmtName string, entry db.Value) []string {
	hasField := func(flds ...string) bool {
		for _, f := range flds {
			if v, ok := entry.Field[f]; ok && strings.TrimSpace(v) != "" {
				return true
			}
		}
		return false
	}

	var uris []string
	if hasField(rpolRouteOperationFld) {
		uris = append(uris, rpolStmtActionsCfgUri(policyName, stmtName))
	}
	if hasField(rpolMatchPrefixSetFld, rpolMatchIpv6PrefixSetFld) {
		uris = append(uris, rpolStmtCondMatchPfxCfgUri(policyName, stmtName))
	}
	if hasField(rpolSetNextHopFld, rpolSetOriginFld, rpolSetLocalPrefFld, rpolSetMedFld) {
		uris = append(uris, rpolStmtBgpActionsCfgUri(policyName, stmtName))
	}
	if hasField(rpolSetCommunityInlineFld, rpolSetCommunityRefFld) {
		uris = append(uris, rpolStmtBgpSetCommunityUri(policyName, stmtName))
	}
	if hasField(rpolSetAsnFld, rpolSetRepeatAsnFld) {
		uris = append(uris, rpolStmtBgpSetAsPathCfgUri(policyName, stmtName))
	}
	if hasField(rpolMatchMedFld, rpolMatchOriginFld, rpolMatchLocalPrefFld, rpolMatchCommunityFld, rpolMatchExtCommunityFld) {
		uris = append(uris, rpolStmtBgpConditionsCfgUri(policyName, stmtName))
	}
	return uris
}

// buildRpolContainerResMap maps a container-level payload by replaying the
// leaf-level mapping for each node the payload sets.
func buildRpolContainerResMap(inParams XfmrParams, ygRoot *ygot.GoStruct) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)

	uris, err := rpolContainerSetTargetUris(ygRoot)
	if err != nil {
		return res, err
	}
	for _, uri := range uris {
		subParams, err := rpolNodeParams(inParams, ygRoot, uri)
		if err != nil {
			return res, err
		}
		part, err := rpolYangToDbXfmr(subParams)
		if err != nil {
			return res, err
		}
		res = mergeDbResMapFields(res, part)
	}
	return res, nil
}

// populateRpolContainerFromDb fills the routing-policy subtree by replaying the
// leaf-level population for every node CONFIG_DB holds data for. A node that
// cannot be read is skipped so that one bad row does not empty the response.
func populateRpolContainerFromDb(inParams XfmrParams, ygRoot *ygot.GoStruct) error {
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for routing-policy mapping"}
	}
	for _, uri := range rpolContainerGetTargetUris(cfgDb) {
		subParams, err := rpolNodeParams(inParams, ygRoot, uri)
		if err != nil {
			return err
		}
		if err := rpolDbToYangXfmr(subParams); err != nil {
			xfmrLogInfo("routing-policy container GET skipped node %v: %v", uri, err)
		}
	}
	return nil
}

// rpolNodeParams returns inParams retargeted at a single mapped node. param is
// cleared so the node handler reads the value from the payload tree rather than
// from the container-level parameter, and the node URI is checked to be a leaf
// level path so that the container dispatch cannot re-enter itself.
func rpolNodeParams(inParams XfmrParams, ygRoot *ygot.GoStruct, uri string) (XfmrParams, error) {
	subParams := inParams
	subParams.requestUri = uri
	subParams.uri = uri
	subParams.ygRoot = ygRoot
	subParams.param = nil

	reqInfo, err := parseRpolRequestUri(uri)
	if err != nil {
		return subParams, err
	}
	if reqInfo.targetPath == rpolTargetRoot {
		return subParams, tlerr.InvalidArgsError{
			Format: "routing-policy container dispatch built a request path it cannot map: " + uri,
		}
	}
	return subParams, nil
}
