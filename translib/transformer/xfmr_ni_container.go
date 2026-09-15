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
	"regexp"
	"sort"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// The levels that enclose everything this module maps had no mapping of their
// own: the network-instances container, a network-instance list instance and
// its protocols container. A GET there read {} while every child answered on
// its own, a DELETE was accepted and did nothing, and a SET reached the BGP
// global parser, which refused it by naming global config for a path that is
// not about global config.
//
// A GET here answers with everything this module maps under a network
// instance: the BGP protocol instance, the static routes, the table
// connections and the AFTs. A SET and a DELETE are refused, since neither has
// one meaning at a level that spans four subtrees with different owners.
const (
	niContainerTargetRoot      = "network-instances-root"
	niContainerTargetInstance  = "network-instance-instance"
	niContainerTargetProtocols = "network-instance-protocols"
	niContainerTargetUnmapped  = "network-instance-unmapped"
)

const niContainerUriPfx = `^/openconfig-network-instance:network-instances`

var niContainerRootReqUriRe = regexp.MustCompile(niContainerUriPfx + `(?:/network-instance)?$`)
var niContainerInstanceReqUriRe = regexp.MustCompile(niContainerUriPfx + `/network-instance\[name=([^\]]+)\]$`)
var niContainerProtocolsReqUriRe = regexp.MustCompile(niContainerUriPfx + `/network-instance\[name=([^\]]+)\]/protocols(?:/protocol)?$`)
var niContainerAnyNiReqUriRe = regexp.MustCompile(niContainerUriPfx + `/network-instance\[name=([^\]]+)\]`)

func init() {
	XlateFuncBind("YangToDb_ni_container_xfmr", YangToDb_ni_container_xfmr)
	XlateFuncBind("DbToYang_ni_container_xfmr", DbToYang_ni_container_xfmr)
}

// parseNiContainerRequestUri classifies a request that reached this handler.
// The annotation binds the three enclosing levels, so every descendant of a
// network instance that no other transformer is annotated for arrives here
// too; those are reported as unmapped rather than guessed at.
func parseNiContainerRequestUri(uri string) (string, string) {
	if niContainerRootReqUriRe.MatchString(uri) {
		return niContainerTargetRoot, ""
	}
	if m := niContainerInstanceReqUriRe.FindStringSubmatch(uri); len(m) == 2 {
		return niContainerTargetInstance, m[1]
	}
	if m := niContainerProtocolsReqUriRe.FindStringSubmatch(uri); len(m) == 2 {
		return niContainerTargetProtocols, m[1]
	}
	if m := niContainerAnyNiReqUriRe.FindStringSubmatch(uri); len(m) == 2 {
		return niContainerTargetUnmapped, m[1]
	}
	return niContainerTargetUnmapped, ""
}

var YangToDb_ni_container_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	kind, _ := parseNiContainerRequestUri(inParams.requestUri)

	if kind == niContainerTargetUnmapped {
		return res, tlerr.NotSupportedError{
			Format: "unsupported network-instance path: " + inParams.requestUri,
			Path:   inParams.requestUri,
		}
	}

	// Refused rather than served. A network-instance level spans the BGP
	// protocol instance, the static routes, the table connections and the
	// AFTs, each owned by its own transformer, and CONFIG_DB has no row for
	// the network instance itself to create or remove.
	if inParams.oper == DELETE {
		return res, tlerr.NotSupportedError{
			Format: "DELETE is not supported at a network-instance container level; delete a mapped child path or a list instance instead",
			Path:   inParams.requestUri,
		}
	}
	return res, tlerr.NotSupportedError{
		Format: "SET is not supported at a network-instance container level; set a mapped child path instead",
		Path:   inParams.requestUri,
	}
}

var DbToYang_ni_container_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	kind, niName := parseNiContainerRequestUri(inParams.requestUri)
	if kind == niContainerTargetUnmapped {
		// This module maps nothing under the requested node. Reporting nothing
		// is the honest answer: an error here would fail a GET on a parent
		// level that does map, since translib walks into the children too.
		return nil
	}

	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance container mapping"}
	}

	wrappedRoot, err := wrapNiContainerRootAsDevice(inParams.ygRoot, niName)
	if err != nil {
		return err
	}

	if kind == niContainerTargetProtocols {
		return populateNiContainerProtocolsFromDb(inParams, wrappedRoot, cfgDb, niName)
	}

	niNames := []string{niName}
	if kind == niContainerTargetRoot {
		niNames = niContainerNamesFromDb(cfgDb)
	}
	for _, name := range niNames {
		if err := populateNiContainerProtocolsFromDb(inParams, wrappedRoot, cfgDb, name); err != nil {
			return err
		}
		if err := populateNiContainerNonProtocolFromDb(inParams, wrappedRoot, cfgDb, name); err != nil {
			return err
		}
	}
	return nil
}

// niContainerNamesFromDb lists the network instances this module holds data
// for. There is no table for the network instance itself, so the names are the
// ones its mapped tables are keyed by, and the default instance is always
// reported: it exists whether or not anything is configured under it.
func niContainerNamesFromDb(cfgDb *db.DB) []string {
	names := map[string]bool{niAftsDefaultNi: true}
	for _, tbl := range []string{niBgpGlobalsTbl, niTableConnTbl} {
		keys, err := cfgDb.GetKeys(&db.TableSpec{Name: tbl})
		if err != nil {
			continue
		}
		for _, k := range keys {
			if len(k.Comp) > 0 && k.Comp[0] != "" {
				names[k.Comp[0]] = true
			}
		}
	}
	// A static route of the default instance is keyed by its prefix alone, so
	// only a two-part key names an instance of its own.
	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niStaticRouteTbl}); err == nil {
		for _, k := range keys {
			if len(k.Comp) == 2 && k.Comp[0] != "" {
				names[k.Comp[0]] = true
			}
		}
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// populateNiContainerProtocolsFromDb fills the protocol instances of one
// network instance. Each subtree is populated by the transformer that owns it,
// replayed with the request URI that transformer expects, so the mapping lives
// in one place rather than being restated here.
func populateNiContainerProtocolsFromDb(inParams XfmrParams, ygRoot *ygot.GoStruct, cfgDb *db.DB, niName string) error {
	// Gated on the row, so a network instance with no BGP does not grow an
	// empty protocol instance in the response.
	if _, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsTbl}, db.Key{Comp: []string{niName}}); err == nil {
		uri := niContainerProtoUri(niName, "BGP", niBgpProtoNameLower)
		if err := niContainerReplay(inParams, ygRoot, uri, DbToYang_ni_bgp_global_cfg_xfmr); err != nil {
			return err
		}
	}
	if len(niStaticRoutePrefixesFromDb(cfgDb, niName)) > 0 {
		uri := niContainerProtoUri(niName, "STATIC", niStaticRouteProtoNameLower)
		if err := niContainerReplay(inParams, ygRoot, uri, DbToYang_ni_static_route_cfg_xfmr); err != nil {
			return err
		}
	}
	return nil
}

// populateNiContainerNonProtocolFromDb fills what a network instance holds
// beside its protocols: the table connections, and the AFTs of the instance
// that has them.
func populateNiContainerNonProtocolFromDb(inParams XfmrParams, ygRoot *ygot.GoStruct, cfgDb *db.DB, niName string) error {
	if rows, err := niTableConnDbRowsForNi(cfgDb, niName); err == nil && len(rows) > 0 {
		uri := niContainerNiUri(niName) + "/table-connections"
		if err := niContainerReplay(inParams, ygRoot, uri, DbToYang_ni_table_conn_cfg_xfmr); err != nil {
			return err
		}
	}
	if niAftsServesNi(niName) && inParams.dbs[db.ApplDB] != nil {
		uri := niContainerNiUri(niName) + "/afts"
		if err := niContainerReplay(inParams, ygRoot, uri, DbToYang_ni_afts_xfmr); err != nil {
			return err
		}
	}
	return nil
}

// niContainerReplay invokes one subtree transformer as if the request had been
// anchored at the node it is annotated for. A node that cannot be read is
// skipped rather than failing the whole response, the way the routing-policy
// container dispatch does, so one bad row does not empty a GET.
func niContainerReplay(inParams XfmrParams, ygRoot *ygot.GoStruct, uri string, handler SubTreeXfmrDbToYang) error {
	subParams := inParams
	subParams.uri = uri
	subParams.requestUri = uri
	subParams.ygRoot = ygRoot
	subParams.param = nil

	if err := handler(subParams); err != nil {
		xfmrLogInfo("network-instance container GET skipped node %v: %v", uri, err)
	}
	return nil
}

func niContainerNiUri(niName string) string {
	return "/openconfig-network-instance:network-instances/network-instance[name=" + niName + "]"
}

func niContainerProtoUri(niName, identifier, protoName string) string {
	return niContainerNiUri(niName) + "/protocols/protocol[identifier=" + identifier + "][name=" + protoName + "]"
}

// wrapNiContainerRootAsDevice presents the request root as a Device tree, the
// shape every transformer replayed above expects. The anchored object is
// reattached rather than copied, so what they populate lands in the tree
// translib owns and returns.
func wrapNiContainerRootAsDevice(s *ygot.GoStruct, niName string) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for network-instance mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}

	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	if devObj.NetworkInstances == nil {
		devObj.NetworkInstances = &ocbinds.OpenconfigNetworkInstance_NetworkInstances{}
	}

	switch obj := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances:
		devObj.NetworkInstances = obj
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance:
		if devObj.NetworkInstances.NetworkInstance == nil {
			devObj.NetworkInstances.NetworkInstance = make(map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance)
		}
		if obj.Name == nil {
			name := niName
			obj.Name = &name
		}
		devObj.NetworkInstances.NetworkInstance[niName] = obj
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols:
		niObj, err := devObj.NetworkInstances.NewNetworkInstance(niName)
		if err != nil || niObj == nil {
			return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for the container mapping"}
		}
		ygot.BuildEmptyTree(niObj)
		niObj.Protocols = obj
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for network-instance mapping"}
	}

	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}
