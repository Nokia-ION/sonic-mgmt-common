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

func rpolStmtActionsStateUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/actions/state"
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

func rpolStmtBgpSetCommunityRefCfgUri(policyName, stmtName string) string {
	return rpolStmtUriPrefix(policyName, stmtName) + "/actions/bgp-actions/set-community/reference/config"
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
				case setCommunity.Reference != nil && setCommunity.Reference.Config != nil &&
					(setCommunity.Reference.Config.CommunitySetRef != nil ||
						len(setCommunity.Reference.Config.CommunitySetRefs) > 0):
					// A payload anchored at the reference node names no method,
					// so the set-community node above has nothing to resolve and
					// the referenced set has to be mapped from here.
					uris = append(uris, rpolStmtBgpSetCommunityRefCfgUri(policyName, stmtName))
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
		uris = append(uris, rpolStmtActionsStateUri(policyName, stmtName))
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
		// The caller writes what the nodes return as one row per statement, so
		// each node is mapped as the update it contributes to that row.
		if subParams.oper == REPLACE {
			subParams.oper = UPDATE
		}
		part, err := rpolYangToDbXfmr(subParams)
		if err != nil {
			return res, err
		}
		res = mergeDbResMapFields(res, part)
	}
	return res, nil
}

// rpolListInstanceDeleteResMap answers a DELETE whose target is a list instance
// itself rather than a node under it. Every leaf-level URI shape requires a
// child node, so an instance URI is classified as container level, which is the
// right answer for a GET or a SET: both are served by walking the whole
// instance. A DELETE has an exact meaning there, and it is the same row removal
// that the config and name spellings of that instance already perform.
func rpolListInstanceDeleteResMap(inParams XfmrParams) (map[string]map[string]db.Value, bool, error) {
	uri := inParams.requestUri
	if m := rpolStmtInstanceReqUriRe.FindStringSubmatch(uri); len(m) == 3 {
		return makeRpolRouteMapResMap(m[1], m[2], nil), true, nil
	}
	if m := rpolPolicyDefInstanceReqUriRe.FindStringSubmatch(uri); len(m) == 2 {
		return rpolDeletePolicyDefResMap(inParams.dbs[db.ConfigDB], m[1]), true, nil
	}
	if m := rpolCommunitySetInstanceReqUriRe.FindStringSubmatch(uri); len(m) == 2 {
		return makeRpolCommunitySetResMap(m[1], nil), true, nil
	}
	if m := rpolExtCommunitySetInstanceReqUriRe.FindStringSubmatch(uri); len(m) == 2 {
		return makeRpolExtCommunitySetResMap(m[1], nil), true, nil
	}
	if m := rpolPrefixSetInstanceReqUriRe.FindStringSubmatch(uri); len(m) == 2 {
		return rpolDeletePrefixSetResMap(inParams.dbs[db.ConfigDB], m[1]), true, nil
	}
	if m := rpolPrefixEntryInstanceReqUriRe.FindStringSubmatch(uri); len(m) == 4 {
		res, err := rpolDeletePrefixEntryResMap(inParams.dbs[db.ConfigDB], &rpolReqInfo{
			prefixSetName:   m[1],
			ipPrefix:        m[2],
			masklengthRange: m[3],
		})
		return res, true, err
	}
	return nil, false, nil
}

// rpolDeletePolicyDefResMap removes a policy together with the statements it
// holds. The ROUTE_MAP rows are keyed by the policy name and would otherwise be
// left behind under a policy that no longer exists.
func rpolDeletePolicyDefResMap(cfgDb *db.DB, policyName string) map[string]map[string]db.Value {
	res := makeRpolRouteMapSetResMap(policyName, nil)
	if cfgDb == nil {
		return res
	}
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolRouteMapTbl})
	if err != nil {
		return res
	}
	for _, k := range keys {
		if len(k.Comp) != 2 || k.Comp[0] != policyName {
			continue
		}
		res = mergeDbResMap(res, makeRpolRouteMapResMap(k.Comp[0], k.Comp[1], nil))
	}
	return res
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
		if !rpolNodeServesRequest(inParams.requestUri, uri) {
			continue
		}
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

// rpolNodeServesRequest reports whether a mapped node has to be populated for
// this request. The enumeration above walks every mapped node in the
// datastore, so without this test a request naming one list instance is
// answered with its siblings as well.
//
// A node URI names the row a handler reads, not the node the response carries,
// and the two need not be the same branch: the prefix-set config URI is what
// fills that set's prefixes list. Branches that diverge by name are therefore
// left to the pruning translib already does by path, and only a key naming a
// different instance of the same list rules a node out here.
func rpolNodeServesRequest(requestUri, nodeUri string) bool {
	reqElems := rpolNormalizedPathElems(requestUri)
	nodeElems := rpolNormalizedPathElems(nodeUri)
	depth := len(reqElems)
	if len(nodeElems) < depth {
		// The node sits above the requested one. It is still populated, so
		// that the list instances on the way down keep their key leaves.
		depth = len(nodeElems)
	}
	for i := 0; i < depth; i++ {
		if reqElems[i].name != nodeElems[i].name {
			return true
		}
		if rpolKeysConflict(reqElems[i], nodeElems[i]) {
			return false
		}
	}
	return true
}

// rpolKeysConflict reports whether two elements name the same list but
// different instances of it. Only the keys the request spells out are
// compared, so that a request naming a list with no key at all still covers
// every instance, and a key the node does not carry is not a disagreement.
func rpolKeysConflict(reqElem, nodeElem rpolPathElem) bool {
	for key, want := range reqElem.keys {
		if got, ok := nodeElem.keys[key]; ok && got != want {
			return true
		}
	}
	return false
}

type rpolPathElem struct {
	name string
	keys map[string]string
}

// rpolNormalizedPathElems splits a URI into path elements and drops the module
// prefix from every element name and key name, so that a request written with
// prefixes compares equal to the URIs this file builds without them.
func rpolNormalizedPathElems(uri string) []rpolPathElem {
	elems := SplitPath(uri)
	out := make([]rpolPathElem, 0, len(elems))
	for _, elem := range elems {
		if elem == "" {
			continue
		}
		out = append(out, rpolNormalizePathElem(elem))
	}
	return out
}

func rpolNormalizePathElem(elem string) rpolPathElem {
	name, keys := elem, ""
	if i := strings.IndexByte(elem, '['); i >= 0 {
		name, keys = elem[:i], elem[i:]
	}
	out := rpolPathElem{name: rpolStripModulePrefix(name)}
	for _, pred := range rpolKeyPredicates(keys) {
		eq := strings.IndexByte(pred, '=')
		if eq < 0 {
			continue
		}
		if out.keys == nil {
			out.keys = make(map[string]string, 2)
		}
		out.keys[rpolStripModulePrefix(pred[:eq])] = strings.Trim(pred[eq+1:], `"'`)
	}
	return out
}

func rpolStripModulePrefix(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// rpolKeyPredicates returns the contents of each key predicate of an element,
// so that a multi-key element such as a prefix entry compares key by key.
func rpolKeyPredicates(keys string) []string {
	var out []string
	depth, start := 0, -1
	for i := 0; i < len(keys); i++ {
		switch keys[i] {
		case '[':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
			if depth == 0 && start >= 0 {
				out = append(out, keys[start:i])
				start = -1
			}
		}
	}
	return out
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
