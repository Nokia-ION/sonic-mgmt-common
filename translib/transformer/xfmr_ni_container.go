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
	"sort"
	"strings"

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
// connections and the AFTs. A SET is served by the handler that owns each
// subtree, which translib calls for its own node; this level refuses a payload
// naming anything with no mapping. A DELETE is still refused.
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
	kind, niName := parseNiContainerRequestUri(inParams.requestUri)

	if kind == niContainerTargetUnmapped {
		return res, tlerr.NotSupportedError{
			Format: "unsupported network-instance path: " + inParams.requestUri,
			Path:   inParams.requestUri,
		}
	}

	if inParams.oper == DELETE {
		return niContainerDelete(inParams, kind, niName)
	}

	// The rows themselves are written by the handler that owns each subtree,
	// which translib calls for its own node with this level as the request.
	// What is left here is the part only this level can do: refusing a payload
	// that names something with no mapping, since a caller told its request
	// succeeded must not find half of it applied.
	wrappedRoot, wrapErr := wrapNiContainerRootAsDevice(inParams.ygRoot, niName)
	if wrapErr != nil {
		return res, wrapErr
	}
	niMap := niContainerInstancesFromPayload(wrappedRoot)
	if len(niMap) == 0 {
		return res, tlerr.InvalidArgsError{Format: "network-instance payload does not name any network instance"}
	}
	for _, name := range niContainerSortedNames(niMap) {
		if kind != niContainerTargetRoot && name != niName {
			return res, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("network-instance payload names %q, which is not the network instance %q in the request path", name, niName),
			}
		}
		if niMap[name] == nil {
			continue
		}
		if err := niContainerCheckPayloadShape(niMap[name], name, kind); err != nil {
			return res, err
		}
	}
	return res, nil
}

// niContainerInstancesFromPayload reads the network instances a payload
// carries, whatever level it arrived at.
func niContainerInstancesFromPayload(s *ygot.GoStruct) map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance {
	devObj, ok := (*s).(*ocbinds.Device)
	if !ok || devObj == nil || devObj.NetworkInstances == nil {
		return nil
	}
	return devObj.NetworkInstances.NetworkInstance
}

func niContainerSortedNames(niMap map[string]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance) []string {
	names := make([]string, 0, len(niMap))
	for name := range niMap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// niContainerCheckPayloadShape refuses what this module does not map. A
// payload naming one of these used to be accepted and dropped.
func niContainerCheckPayloadShape(niObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance, niName string, kind string) error {
	if niObj.Name != nil && strings.TrimSpace(*niObj.Name) != "" && *niObj.Name != niName {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("network-instance name %q in payload does not match list key %q", *niObj.Name, niName),
		}
	}
	if kind == niContainerTargetProtocols && (niObj.TableConnections != nil || niObj.Afts != nil) {
		return tlerr.NotSupportedError{Format: "a payload at the protocols container carries nodes from outside it"}
	}
	if niObj.Afts != nil {
		return tlerr.NotSupportedError{
			Format: "the AFTs of a network instance report forwarding state and cannot be written",
		}
	}
	var unmapped []string
	for _, part := range []struct {
		name  string
		carry bool
	}{
		{"connection-points", niObj.ConnectionPoints != nil},
		{"encapsulation", niObj.Encapsulation != nil},
		{"fdb", niObj.Fdb != nil},
		{"inter-instance-policies", niObj.InterInstancePolicies != nil},
		{"interfaces", niObj.Interfaces != nil},
		{"mpls", niObj.Mpls != nil},
		{"policy-forwarding", niObj.PolicyForwarding != nil},
		{"route-limits", niObj.RouteLimits != nil},
		{"segment-routing", niObj.SegmentRouting != nil},
		{"state", niObj.State != nil},
		{"tables", niObj.Tables != nil},
		{"vlans", niObj.Vlans != nil},
	} {
		if part.carry {
			unmapped = append(unmapped, part.name)
		}
	}
	if niObj.Config != nil {
		// The instance has no CONFIG_DB row of its own, so nothing under its
		// config container is mapped. The name is tolerated, a payload
		// repeating the key it is addressed by.
		if niObj.Config.Name != nil && *niObj.Config.Name != niName {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("network-instance config/name %q does not match list key %q", *niObj.Config.Name, niName),
			}
		}
		unmapped = append(unmapped, niContainerConfigLeavesBesideName(niObj.Config)...)
	}
	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		return tlerr.NotSupportedError{
			Format: "network-instance payload carries nodes that have no CONFIG_DB mapping: " + formatOcLeafPathList(unmapped),
		}
	}
	return nil
}

// niContainerConfigLeavesBesideName names anything set under the instance's own
// config container other than the key it repeats.
func niContainerConfigLeavesBesideName(cfg *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Config) []string {
	var set []string
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Tag.Get("path")
		if name == "" || name == "name" {
			continue
		}
		f := v.Field(i)
		switch f.Kind() {
		case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice:
			if f.IsNil() {
				continue
			}
		case reflect.Int64:
			if f.Int() == 0 {
				continue
			}
		default:
			continue
		}
		set = append(set, "config/"+name)
	}
	return set
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

// niHandlerReqUri returns the URI a subtree handler under a network instance
// should read as its request.
//
// translib calls each annotated subtree transformer for its own node, and
// leaves requestUri at the level the caller asked for. Its
// invokeCRUSubtreeOnce is recorded per transformer function, so it stops a
// transformer being invoked again for its own children and says nothing about
// children owned by someone else: a request made at a network-instance level
// reaches every handler below it with a requestUri none of them recognise.
// When that happens the node's own uri is what describes what the handler has
// been handed.
//
// Only a request at one of the three network-instance levels is treated this
// way. A request at or below a protocol instance keeps reading requestUri, so
// the levels that already worked behave exactly as they did.
func niHandlerReqUri(inParams XfmrParams) string {
	if inParams.uri == "" || inParams.uri == inParams.requestUri {
		return inParams.requestUri
	}
	if !strings.HasPrefix(inParams.uri, inParams.requestUri) {
		return inParams.requestUri
	}
	if kind, _ := parseNiContainerRequestUri(inParams.requestUri); kind == niContainerTargetUnmapped {
		return inParams.requestUri
	}
	return inParams.uri
}

// niIsUnmappedProtocolNodeUri reports a node on the way to a protocol instance
// that this repo maps nothing at: the protocol list itself, and the identity
// leaves a payload repeats under the instance. translib walks each of these as
// well as the instance, so a handler reached at one of them maps nothing rather
// than refusing a path it cannot read.
var niUnmappedProtocolNodeRe = regexp.MustCompile(`^/openconfig-network-instance:network-instances/network-instance\[name=[^\]]+\]/protocols/protocol` +
	`(?:/?$|\[[^\]]+\]\[[^\]]+\]/(?:config|state)(?:/(?:identifier|name))?/?$|\[[^\]]+\]\[[^\]]+\]/(?:identifier|name)/?$)`)

func niIsUnmappedProtocolNodeUri(uri string) bool {
	return niUnmappedProtocolNodeRe.MatchString(uri)
}

// niContainerDelete removes what this module maps under the instances in
// scope. Nothing cascades on its own: translib walks the children of a SET but
// not of a DELETE, so each subtree is removed here, through the delete its own
// list instance already serves.
func niContainerDelete(inParams XfmrParams, kind, niName string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance container mapping"}
	}

	names := []string{niName}
	if kind == niContainerTargetRoot {
		names = niContainerNamesFromDb(cfgDb)
	}
	for _, name := range names {
		if err := niContainerDeleteInstance(inParams, cfgDb, name, kind, res); err != nil {
			return make(map[string]map[string]db.Value), err
		}
	}
	// An instance holding nothing this module maps is already in the state the
	// caller asked for, so an empty answer is success rather than an error.
	return res, nil
}

func niContainerDeleteInstance(inParams XfmrParams, cfgDb *db.DB, niName, kind string,
	res map[string]map[string]db.Value) error {
	bgpUri := niContainerProtoUri(niName, "BGP", niBgpProtoNameLower)

	// The neighbours and peer groups first, each by the delete its own list
	// instance serves, which is what clears their address-family rows too.
	// The whole BGP subtree of the instance, every row named by the handler
	// that owns those tables.
	if err := niContainerReplayDelete(inParams, bgpUri, YangToDb_ni_bgp_global_cfg_xfmr, res); err != nil {
		return err
	}

	staticUri := niContainerProtoUri(niName, "STATIC", niStaticRouteProtoNameLower)
	for _, prefix := range niStaticRoutePrefixesFromDb(cfgDb, niName) {
		uri := staticUri + "/static-routes/static[prefix=" + prefix + "]"
		if err := niContainerReplayDelete(inParams, uri, YangToDb_ni_static_route_cfg_xfmr, res); err != nil {
			return err
		}
	}

	if kind == niContainerTargetProtocols {
		// The protocols container holds the protocol instances and nothing
		// else, so the table connections and the AFTs are not its to remove.
		return nil
	}

	if rows, err := niTableConnDbRowsForNi(cfgDb, niName); err == nil {
		for _, k := range rows {
			if len(k.Comp) != 4 {
				continue
			}
			src, srcErr := niTableConnProtocolDbToEnum(k.Comp[1])
			dst, dstErr := niTableConnProtocolDbToEnum(k.Comp[2])
			af, afErr := niTableConnAddressFamilyDbToEnum(k.Comp[3])
			if srcErr != nil || dstErr != nil || afErr != nil {
				// A row spelled in a way the model cannot name is left alone
				// rather than guessed at.
				xfmrLogInfo("network-instance delete skipped table-connection row %v", k.Comp)
				continue
			}
			uri := niContainerNiUri(niName) + "/table-connections/table-connection[src-protocol=" +
				niTableConnProtocolEnumToOcName(src) + "][dst-protocol=" + niTableConnProtocolEnumToOcName(dst) +
				"][address-family=" + niTableConnAddressFamilyEnumToOcName(af) + "]"
			if err := niContainerReplayDelete(inParams, uri, YangToDb_ni_table_conn_cfg_xfmr, res); err != nil {
				return err
			}
		}
	}

	// The AFTs report the forwarding state fpmsyncd publishes, so there is
	// nothing under them for a delete to remove.
	return nil
}

func niContainerReplayDelete(inParams XfmrParams, uri string, handler SubTreeXfmrYangToDb,
	res map[string]map[string]db.Value) error {
	subParams := inParams
	subParams.uri = uri
	subParams.param = nil

	childRes, err := handler(subParams)
	if err != nil {
		return err
	}
	return niContainerMergeResMap(res, childRes, uri)
}

// niContainerMergeResMap folds one transformer's rows into the answer. Two
// owners naming one field of one row is not expected among the mapped
// subtrees, so a disagreement is reported rather than resolved by whichever
// ran last.
func niContainerMergeResMap(dst, src map[string]map[string]db.Value, uri string) error {
	for table, rows := range src {
		if _, ok := dst[table]; !ok {
			dst[table] = make(map[string]db.Value)
		}
		for key, val := range rows {
			existing, seen := dst[table][key]
			if !seen {
				dst[table][key] = val
				continue
			}
			if len(existing.Field) == 0 || len(val.Field) == 0 {
				// One of them removes the whole row, which covers the other.
				dst[table][key] = db.Value{Field: map[string]string{}}
				continue
			}
			for fld, v := range val.Field {
				if have, dup := existing.Field[fld]; dup && have != v {
					return tlerr.InvalidArgsError{Format: fmt.Sprintf(
						"network-instance delete maps %s field %s of row %s to both %q and %q, from %s",
						table, fld, key, have, v, uri)}
				}
				existing.Field[fld] = v
			}
			dst[table][key] = existing
		}
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
