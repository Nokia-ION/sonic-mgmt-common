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
	"encoding/json"
	"fmt"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	log "github.com/golang/glog"
	"github.com/openconfig/gnmi/proto/gnmi"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/internal/apis"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// The FDB of the default network instance is the MAC table the ASIC holds:
// one ASIC_DB FDB entry each, keyed by its MAC and the SAI VLAN object it was
// learned on, pointing at a bridge port. The VLAN id is read from the VLAN
// object and the interface from the port the bridge port wraps, named through
// the COUNTERS_DB port and LAG name maps, the way fdbshow does it.
//
// SONiC has no configuration for MAC learning, aging or static MAC entries,
// so the FDB is read-only: every write is refused. An entry on a VLAN object
// with no VLAN id is the default VLAN of untagged traffic and is not reported,
// and an entry learned behind a remote endpoint has no local interface.
const (
	niFdbAsicTbl     = "ASIC_STATE"
	niFdbEntryType   = "SAI_OBJECT_TYPE_FDB_ENTRY"
	niFdbBridgePort  = "SAI_OBJECT_TYPE_BRIDGE_PORT"
	niFdbVlanType    = "SAI_OBJECT_TYPE_VLAN"
	niFdbPortNameMap = "COUNTERS_PORT_NAME_MAP"
	niFdbLagNameMap  = "COUNTERS_LAG_NAME_MAP"

	niFdbNiListNode    = "network-instance"
	niFdbEntryListNode = "entry"

	niFdbTargetRoot  = "fdb-root"
	niFdbTargetCfg   = "fdb-config"
	niFdbTargetState = "fdb-state"
	niFdbTargetEntry = "fdb-entry"
)

const niFdbUriPfx = `^/openconfig-network-instance:network-instances/network-instance\[name=([^\]]+)\]/fdb`

var niFdbReqUriRe = regexp.MustCompile(niFdbUriPfx + `(/.*)?$`)
var niFdbEntryUriRe = regexp.MustCompile(`^/mac-table/entries/entry((?:\[[^\]]+\])*)(/.*)?$`)
var niFdbEntryKeyRe = regexp.MustCompile(`\[(mac-address|vlan)=([^\]]+)\]`)

type niFdbReqInfo struct {
	niName         string
	targetPathKind string
	mac            string
	vlan           string
}

type niFdbEntry struct {
	mac    string
	vlan   uint16
	ifName string
	static bool
}

func init() {
	XlateFuncBind("YangToDb_ni_fdb_xfmr", YangToDb_ni_fdb_xfmr)
	XlateFuncBind("DbToYang_ni_fdb_xfmr", DbToYang_ni_fdb_xfmr)
	XlateFuncBind("Subscribe_ni_fdb_xfmr", Subscribe_ni_fdb_xfmr)
	XlateFuncBind("DbToYangPath_ni_fdb_path_xfmr", DbToYangPath_ni_fdb_path_xfmr)
}

func parseNiFdbRequestUri(uri string) (*niFdbReqInfo, error) {
	m := niFdbReqUriRe.FindStringSubmatch(uri)
	if m == nil {
		return nil, tlerr.NotSupportedError{Format: "unsupported path under the network-instance FDB: " + uri, Path: uri}
	}
	r := &niFdbReqInfo{niName: m[1]}
	rest := m[2]
	switch {
	case rest == "/config" || strings.HasPrefix(rest, "/config/"):
		r.targetPathKind = niFdbTargetCfg
	case rest == "/state" || strings.HasPrefix(rest, "/state/"):
		r.targetPathKind = niFdbTargetState
	case rest == "" || rest == "/mac-table" || rest == "/mac-table/entries":
		r.targetPathKind = niFdbTargetRoot
	default:
		em := niFdbEntryUriRe.FindStringSubmatch(rest)
		if em == nil {
			return nil, tlerr.NotSupportedError{Format: "unsupported path under the network-instance FDB: " + uri, Path: uri}
		}
		r.targetPathKind = niFdbTargetEntry
		for _, k := range niFdbEntryKeyRe.FindAllStringSubmatch(em[1], -1) {
			if k[1] == "mac-address" {
				r.mac = strings.ToLower(k[2])
			} else {
				r.vlan = k[2]
			}
		}
	}
	return r, nil
}

func niFdbServesNi(niName string) bool {
	return niContainerIsDefaultInstanceName(niName)
}

// niFdbOidMap reads a COUNTERS_DB name map, OID to name.
func niFdbOidMap(ctrDb *db.DB, table string, out map[string]string) {
	entry, err := ctrDb.GetMapAll(&db.TableSpec{Name: table})
	if err != nil {
		return
	}
	for name, oid := range entry.Field {
		out[oid] = name
	}
}

func niFdbAsicAttr(asicDb *db.DB, objType, oid, attr string) string {
	entry, err := asicDb.GetEntry(&db.TableSpec{Name: niFdbAsicTbl, CompCt: 2}, db.Key{Comp: []string{objType, oid}})
	if err != nil {
		return ""
	}
	return entry.Field[attr]
}

// niFdbKeyParts reads the MAC and the VLAN object, or the VLAN id an older
// key carries, from the JSON key of an FDB entry.
func niFdbKeyParts(key string) (string, string, string, bool) {
	var k map[string]string
	if err := json.Unmarshal([]byte(key), &k); err != nil || k["mac"] == "" {
		return "", "", "", false
	}
	return strings.ToLower(k["mac"]), k["bvid"], k["vlan"], true
}

// niFdbVlanOf resolves the VLAN id of an entry, caching it per VLAN object.
func niFdbVlanOf(asicDb *db.DB, bvid, vlan string, cache map[string]string) (uint16, bool) {
	if vlan == "" && bvid != "" && asicDb != nil {
		v, seen := cache[bvid]
		if !seen {
			v = niFdbAsicAttr(asicDb, niFdbVlanType, bvid, "SAI_VLAN_ATTR_VLAN_ID")
			cache[bvid] = v
		}
		vlan = v
	}
	id, err := strconv.ParseUint(vlan, 10, 16)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint16(id), true
}

// niFdbEntriesFromDb lists the MAC table, sorted by VLAN then MAC.
func niFdbEntriesFromDb(asicDb, ctrDb *db.DB) []niFdbEntry {
	if asicDb == nil {
		return nil
	}
	keys, err := asicDb.GetKeysPattern(&db.TableSpec{Name: niFdbAsicTbl, CompCt: 2}, db.Key{Comp: []string{niFdbEntryType, "*"}})
	if err != nil {
		return nil
	}
	names := map[string]string{}
	if ctrDb != nil {
		niFdbOidMap(ctrDb, niFdbPortNameMap, names)
		niFdbOidMap(ctrDb, niFdbLagNameMap, names)
	}
	vlans := map[string]string{}
	ports := map[string]string{}
	var out []niFdbEntry
	for _, k := range keys {
		if len(k.Comp) != 2 {
			continue
		}
		mac, bvid, vlan, ok := niFdbKeyParts(k.Comp[1])
		if !ok {
			continue
		}
		vid, ok := niFdbVlanOf(asicDb, bvid, vlan, vlans)
		if !ok {
			continue
		}
		attrs, err := asicDb.GetEntry(&db.TableSpec{Name: niFdbAsicTbl, CompCt: 2}, k)
		if err != nil {
			continue
		}
		e := niFdbEntry{mac: mac, vlan: vid, static: attrs.Field["SAI_FDB_ENTRY_ATTR_TYPE"] == "SAI_FDB_ENTRY_TYPE_STATIC"}
		if attrs.Field["SAI_FDB_ENTRY_ATTR_ENDPOINT_IP"] == "" {
			if br := attrs.Field["SAI_FDB_ENTRY_ATTR_BRIDGE_PORT_ID"]; br != "" {
				port, seen := ports[br]
				if !seen {
					port = niFdbAsicAttr(asicDb, niFdbBridgePort, br, "SAI_BRIDGE_PORT_ATTR_PORT_ID")
					ports[br] = port
				}
				e.ifName = names[port]
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].vlan != out[j].vlan {
			return out[i].vlan < out[j].vlan
		}
		return out[i].mac < out[j].mac
	})
	return out
}

func niFdbEntryMatches(e niFdbEntry, reqInfo *niFdbReqInfo) bool {
	if reqInfo.mac != "" && reqInfo.mac != "*" && reqInfo.mac != e.mac {
		return false
	}
	if reqInfo.vlan != "" && reqInfo.vlan != "*" && reqInfo.vlan != strconv.Itoa(int(e.vlan)) {
		return false
	}
	return true
}

// wrapNiFdbRootAsDevice normalises the bind root to a Device tree. The request
// binder anchors the root at the requested node.
func wrapNiFdbRootAsDevice(s *ygot.GoStruct, reqInfo *niFdbReqInfo) (*ygot.GoStruct, error) {
	if s == nil || *s == nil {
		return nil, tlerr.InvalidArgsError{Format: "empty YANG root for network-instance FDB mapping"}
	}
	if _, ok := (*s).(*ocbinds.Device); ok {
		return s, nil
	}
	devObj := &ocbinds.Device{}
	ygot.BuildEmptyTree(devObj)
	niObj, err := devObj.NetworkInstances.NewNetworkInstance(reqInfo.niName)
	if err != nil || niObj == nil {
		return nil, tlerr.InvalidArgsError{Format: "unable to create network-instance context for FDB mapping"}
	}
	ygot.BuildEmptyTree(niObj)
	switch root := (*s).(type) {
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb:
		niObj.Fdb = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable:
		niObj.Fdb.MacTable = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries:
		ygot.BuildEmptyTree(niObj.Fdb)
		niObj.Fdb.MacTable.Entries = root
	case *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry:
		vlan, err := strconv.ParseUint(reqInfo.vlan, 10, 16)
		if err != nil || reqInfo.mac == "" {
			return nil, tlerr.InvalidArgsError{Format: "FDB entry keys missing in request path"}
		}
		ygot.BuildEmptyTree(niObj.Fdb)
		ygot.BuildEmptyTree(niObj.Fdb.MacTable)
		key := ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_Key{MacAddress: reqInfo.mac, Vlan: uint16(vlan)}
		niObj.Fdb.MacTable.Entries.Entry = map[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_Key]*ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry{key: root}
	default:
		return nil, tlerr.InvalidArgsError{Format: "unsupported YANG root type for network-instance FDB mapping"}
	}
	var wrappedRoot ygot.GoStruct = devObj
	return &wrappedRoot, nil
}

func getNiFdbEntriesObj(s *ygot.GoStruct, niName string) *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries {
	niObj, err := niContainerGetOrCreateNi(s, niName)
	if err != nil || niObj == nil {
		return nil
	}
	if niObj.Fdb == nil {
		niObj.Fdb = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb{}
	}
	if niObj.Fdb.MacTable == nil {
		niObj.Fdb.MacTable = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable{}
	}
	if niObj.Fdb.MacTable.Entries == nil {
		niObj.Fdb.MacTable.Entries = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries{}
	}
	return niObj.Fdb.MacTable.Entries
}

// populateNiFdbEntry fills the state of one entry and the interface it was
// learned on.
func populateNiFdbEntry(esObj *ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries, e niFdbEntry) error {
	eObj, ok := esObj.Entry[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_Key{MacAddress: e.mac, Vlan: e.vlan}]
	if !ok || eObj == nil {
		var err error
		if eObj, err = esObj.NewEntry(e.mac, e.vlan); err != nil {
			return err
		}
	}
	mac, vlan := e.mac, e.vlan
	eObj.State = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_State{
		MacAddress: &mac,
		Vlan:       &vlan,
		EntryType:  ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_State_EntryType_DYNAMIC,
	}
	if e.static {
		eObj.State.EntryType = ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_State_EntryType_STATIC
	}
	if e.ifName != "" {
		ifName := e.ifName
		eObj.Interface = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_Interface{
			InterfaceRef: &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_Interface_InterfaceRef{
				State: &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Fdb_MacTable_Entries_Entry_Interface_InterfaceRef_State{
					Interface: &ifName,
				},
			},
		}
	}
	return nil
}

var YangToDb_ni_fdb_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	reqInfo, err := parseNiFdbRequestUri(niHandlerReqUri(inParams))
	if err != nil {
		return res, err
	}
	if reqInfo.targetPathKind == niFdbTargetCfg {
		return res, tlerr.NotSupportedError{
			Format: "the FDB of a network instance is read-only: SONiC has no configuration for MAC learning, aging or the size of the MAC table",
			Path:   inParams.requestUri,
		}
	}
	return res, tlerr.NotSupportedError{
		Format: "the FDB of a network instance reports the MAC table the ASIC holds and cannot be written: SONiC has no configuration for static MAC entries",
		Path:   inParams.requestUri,
	}
}

var DbToYang_ni_fdb_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("network-instance FDB transformer panic in GET path for %s: %v\n%s", inParams.requestUri, r, debug.Stack())
			err = tlerr.InvalidArgsError{Format: fmt.Sprintf("network-instance FDB transformer panic in GET path: %v", r)}
		}
	}()

	reqInfo, err := parseNiFdbRequestUri(inParams.requestUri)
	if err != nil {
		return err
	}
	keyed := reqInfo.targetPathKind == niFdbTargetEntry && reqInfo.mac != "" && reqInfo.vlan != ""
	if !niFdbServesNi(reqInfo.niName) {
		if keyed {
			return tlerr.NotFoundError{Format: fmt.Sprintf("network-instance %s has no FDB; the MAC table is served under the default instance", reqInfo.niName)}
		}
		return nil
	}
	if reqInfo.targetPathKind == niFdbTargetCfg || reqInfo.targetPathKind == niFdbTargetState {
		// No SONiC mapping, so nothing to report.
		return nil
	}

	wrappedRoot, err := wrapNiFdbRootAsDevice(inParams.ygRoot, reqInfo)
	if err != nil {
		return err
	}
	esObj := getNiFdbEntriesObj(wrappedRoot, reqInfo.niName)
	if esObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate network-instance FDB entries in YANG tree"}
	}
	found := false
	for _, e := range niFdbEntriesFromDb(inParams.dbs[db.AsicDB], inParams.dbs[db.CountersDB]) {
		if !niFdbEntryMatches(e, reqInfo) {
			continue
		}
		if err := populateNiFdbEntry(esObj, e); err != nil {
			return err
		}
		found = true
	}
	if keyed && !found {
		return tlerr.NotFoundError{Format: fmt.Sprintf("FDB entry %s on VLAN %s not found", reqInfo.mac, reqInfo.vlan)}
	}
	return nil
}

// Subscribe_ni_fdb_xfmr maps the MAC table onto the ASIC_DB FDB entries. The
// ASIC learns and ages entries all the time, so the table is sampled.
var Subscribe_ni_fdb_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.isVirtualTbl = true
	if inParams.subscProc != TRANSLATE_SUBSCRIBE {
		return result, nil
	}
	anchor := inParams.uri
	if anchor == "" {
		anchor = inParams.requestURI
	}
	reqInfo, err := parseNiFdbRequestUri(anchor)
	if err != nil || !niFdbServesNi(reqInfo.niName) ||
		reqInfo.targetPathKind == niFdbTargetCfg || reqInfo.targetPathKind == niFdbTargetState {
		return result, nil
	}
	result.dbDataMap = RedisDbSubscribeMap{db.AsicDB: {
		niFdbAsicTbl: {niFdbEntryType + ":*": {KEY_COMP_CNT: "2"}},
	}}
	result.isVirtualTbl = false
	result.onChange = OnchangeDisable
	result.nOpts = &notificationOpts{pType: Sample}
	return result, nil
}

// niFdbSubscribedKeys names the entry keys a subscription path gives, "" for
// one it leaves open.
func niFdbSubscribedKeys(p *gnmi.Path) (string, string, string) {
	niName, mac, vlan := "", "", ""
	if p == nil {
		return niName, mac, vlan
	}
	open := func(v string) string {
		if v == "*" {
			return ""
		}
		return v
	}
	for _, e := range p.GetElem() {
		switch e.GetName() {
		case niFdbNiListNode:
			niName = open(e.GetKey()["name"])
		case niFdbEntryListNode:
			mac = strings.ToLower(open(e.GetKey()["mac-address"]))
			vlan = open(e.GetKey()["vlan"])
		}
	}
	return niName, mac, vlan
}

// DbToYangPath_ni_fdb_path_xfmr names the entry an ASIC_DB FDB entry is, for a
// subscription that left the key open.
var DbToYangPath_ni_fdb_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != niFdbAsicTbl || len(params.tblKeyComp) != 2 || params.tblKeyComp[0] != niFdbEntryType {
		return fmt.Errorf("ni_fdb_path_xfmr: %s key %v is not an FDB entry: %w", params.tblName, params.tblKeyComp, apis.ErrUnrelatedEntry)
	}
	mac, bvid, vlan, ok := niFdbKeyParts(params.tblKeyComp[1])
	asicDb := params.dbs[db.AsicDB]
	if asicDb == nil && params.dbNum == db.AsicDB {
		asicDb = params.db
	}
	if !ok {
		return fmt.Errorf("ni_fdb_path_xfmr: FDB entry %s cannot be read: %w", params.tblKeyComp[1], apis.ErrUnrelatedEntry)
	}
	vid, ok := niFdbVlanOf(asicDb, bvid, vlan, map[string]string{})
	if !ok {
		return fmt.Errorf("ni_fdb_path_xfmr: FDB entry %s is on no VLAN: %w", params.tblKeyComp[1], apis.ErrUnrelatedEntry)
	}
	niName, wantMac, wantVlan := niFdbSubscribedKeys(params.subscribePath)
	if (wantMac != "" && wantMac != mac) || (wantVlan != "" && wantVlan != strconv.Itoa(int(vid))) {
		return fmt.Errorf("ni_fdb_path_xfmr: FDB entry %s on VLAN %d is not the one subscribed to: %w", mac, vid, apis.ErrUnrelatedEntry)
	}
	if niName == "" {
		niName = niContainerDefaultNameLower
	}
	niPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niFdbNiListNode)
	if !ok {
		return nil
	}
	params.ygPathKeys[niPath+"/name"] = niName
	if entryPath, ok := niStaticRouteSchemaPathUpto(params.ygSchemaPath, niFdbEntryListNode); ok {
		params.ygPathKeys[entryPath+"/mac-address"] = mac
		params.ygPathKeys[entryPath+"/vlan"] = strconv.Itoa(int(vid))
	}
	return nil
}
