package transformer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

// TACACS+ support maps the OpenConfig AAA server-groups subtree onto SONiC's
// flat TACPLUS_SERVER table.
//
// Semantic note: OpenConfig models TACACS servers under an arbitrarily named
// server-group whose type is TACACS. SONiC has no concept of named TACACS
// server-groups - there is a single implicit set of servers in TACPLUS_SERVER
// (plus a TACPLUS|global row). This transformer therefore treats every
// server-group of type TACACS (or of unset type, to allow partial updates on
// deeper paths) as contributing servers to the one TACPLUS_SERVER table; the
// group name is not persisted. On GET the servers are reported back under a
// single group (named from the request, or "TACACS" by default).
//
// The tacacs/config/source-address leaf is intentionally not mapped: SONiC has
// no per-server source address for TACACS+ (only a global TACPLUS|global:src_ip).
//
// The openconfig-system-ext source-interface and dscp leaves on the server-group
// config are mapped to the global TACPLUS|global:src_intf and TACPLUS|global:dscp
// fields (EOS 'ip tacacs source-interface' and 'tacacs-server qos dscp'); SONiC
// has neither per-group setting.
const (
	TACPLUS_SERVER_TABLE_NAME = "TACPLUS_SERVER"
	TACPLUS_GLOBAL_TABLE_NAME = "TACPLUS"
	TACPLUS_GLOBAL_KEY        = "global"
	TACACS_SERVER_GROUPS_PATH = "/openconfig-system:system/aaa/server-groups"
	TACACS_SERVER_GROUP_PATH  = "/openconfig-system:system/aaa/server-groups/server-group"
	TACACS_SERVERS_PATH       = "/openconfig-system:system/aaa/server-groups/server-group/servers"
	TACACS_SERVER_PATH        = "/openconfig-system:system/aaa/server-groups/server-group/servers/server"
	TACACS_DEFAULT_GROUP_NAME = "TACACS"
	TACACS_DEFAULT_TCP_PORT   = "49"
	TACACS_DEFAULT_PRIORITY   = "1"
	tacacsServerGroupKeyField = "name"
	tacacsServerKeyField      = "address"
	// Source interface and DSCP (openconfig-system-ext) map to the global TACPLUS row.
	tacacsSrcIntfField = "src_intf"
	tacacsDscpField    = "dscp"
)

func init() {
	XlateFuncBind("DbToYang_tacacs_server_groups_xfmr", DbToYang_tacacs_server_groups_xfmr)
	XlateFuncBind("YangToDb_tacacs_server_groups_xfmr", YangToDb_tacacs_server_groups_xfmr)
	XlateFuncBind("Subscribe_tacacs_server_groups_xfmr", Subscribe_tacacs_server_groups_xfmr)
}

// isTacacsGroup reports whether an OpenConfig server-group should be handled as
// a TACACS+ group. Groups explicitly typed as something other than TACACS
// (e.g. RADIUS/LDAP) are skipped; an unset type is accepted so that partial
// updates on deeper paths (which do not carry the group's config/type) still
// map to TACPLUS_SERVER.
func isTacacsGroup(grp *ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup) bool {
	if grp == nil || grp.Config == nil {
		return true
	}
	return grp.Config.Type == ocbinds.OpenconfigAaaTypes_AAA_SERVER_TYPE_TACACS ||
		grp.Config.Type == ocbinds.OpenconfigAaaTypes_AAA_SERVER_TYPE_UNSET
}

// buildTacacsServerFields translates a single OpenConfig server object into the
// TACPLUS_SERVER field map. When applyDefaults is true (fresh create or
// REPLACE) the required backend defaults (tcp_port, priority) are injected so
// the rendered entry is valid for hostcfgd.
func buildTacacsServerFields(srv *ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server, applyDefaults bool) map[string]string {
	fields := make(map[string]string)

	if srv != nil {
		if srv.Config != nil && srv.Config.Timeout != nil {
			fields["timeout"] = strconv.FormatUint(uint64(*srv.Config.Timeout), 10)
		}
		if srv.Tacacs != nil && srv.Tacacs.Config != nil {
			tc := srv.Tacacs.Config
			if tc.Port != nil {
				fields["tcp_port"] = strconv.FormatUint(uint64(*tc.Port), 10)
			}
			if tc.SecretKey != nil {
				fields["passkey"] = *tc.SecretKey
			}
		}
	}

	if applyDefaults {
		if _, ok := fields["tcp_port"]; !ok {
			fields["tcp_port"] = TACACS_DEFAULT_TCP_PORT
		}
		if _, ok := fields["priority"]; !ok {
			fields["priority"] = TACACS_DEFAULT_PRIORITY
		}
	}

	if len(fields) == 0 {
		fields[nullKey] = nullKey
	}
	return fields
}

// uriPathEndsWithLeaf reports whether the last node of a YANG path is the given
// leaf name, tolerating a module prefix on the final segment. Augmented leaves
// from a foreign module (e.g. openconfig-system-ext:source-interface) keep their
// module prefix in the path returned by getYangPathFromUri, so a plain
// strings.HasSuffix(path, "/source-interface") would not match.
func uriPathEndsWithLeaf(path, leaf string) bool {
	seg := path
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	if i := strings.LastIndex(seg, ":"); i >= 0 {
		seg = seg[i+1:]
	}
	return seg == leaf
}

// collectTacacsSourceIntf walks the OpenConfig payload and returns the
// source-interface (openconfig-system-ext) configured on any TACACS group, if
// present. It maps to the single global TACPLUS|global:src_intf field.
func collectTacacsSourceIntf(sysObj *ocbinds.OpenconfigSystem_System) (string, bool) {
	if sysObj == nil || sysObj.Aaa == nil || sysObj.Aaa.ServerGroups == nil {
		return "", false
	}
	for _, grp := range sysObj.Aaa.ServerGroups.ServerGroup {
		if !isTacacsGroup(grp) || grp.Config == nil || grp.Config.SourceInterface == nil {
			continue
		}
		return *grp.Config.SourceInterface, true
	}
	return "", false
}

// collectTacacsDscp walks the OpenConfig payload and returns the dscp
// (openconfig-system-ext) configured on any TACACS group, if present. It maps to
// the single global TACPLUS|global:dscp field.
func collectTacacsDscp(sysObj *ocbinds.OpenconfigSystem_System) (string, bool) {
	if sysObj == nil || sysObj.Aaa == nil || sysObj.Aaa.ServerGroups == nil {
		return "", false
	}
	for _, grp := range sysObj.Aaa.ServerGroups.ServerGroup {
		if !isTacacsGroup(grp) || grp.Config == nil || grp.Config.Dscp == nil {
			continue
		}
		return strconv.FormatUint(uint64(*grp.Config.Dscp), 10), true
	}
	return "", false
}

// collectTacacsPayloadServers walks the OpenConfig payload and returns a map of
// server address -> server object for every TACACS group present.
func collectTacacsPayloadServers(sysObj *ocbinds.OpenconfigSystem_System) map[string]*ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server {
	out := make(map[string]*ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server)
	if sysObj == nil || sysObj.Aaa == nil || sysObj.Aaa.ServerGroups == nil {
		return out
	}
	for _, grp := range sysObj.Aaa.ServerGroups.ServerGroup {
		if !isTacacsGroup(grp) || grp.Servers == nil {
			continue
		}
		for addr, srv := range grp.Servers.Server {
			out[addr] = srv
		}
	}
	return out
}

// YangToDb_tacacs_server_groups_xfmr handles gNMI SET/REPLACE/DELETE/UPDATE on
// the AAA server-groups subtree, mapping TACACS+ servers to TACPLUS_SERVER.
var YangToDb_tacacs_server_groups_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	var err error
	log.V(lvl.DEBUG).Infof("Inside YangToDb_tacacs_server_groups_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		cfgDb, err = db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("CONFIG_DB not found for URI %s: %v", inParams.uri, err)}
		}
		defer cfgDb.DeleteDB()
	}
	sysObj := getAppRootObject(inParams)

	tacTableOps := make(map[string]db.Value)
	dbMutations := map[string]map[string]db.Value{
		TACPLUS_SERVER_TABLE_NAME: tacTableOps,
	}

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		return nil, tlerr.InternalError{Format: fmt.Sprintf("Failed to get YANG path from URI %s: %v", inParams.uri, err)}
	}

	// requestUri is the original client path. For a nested leaf DELETE (e.g.
	// .../server-group[name=TACACS]/config/source-interface) the framework may
	// invoke this subtree with a higher-level URI (the server-groups subtree
	// root) rather than the leaf itself, so routing decisions that target a
	// specific leaf must be based on requestUri, not inParams.uri.
	reqUriPath, err := getYangPathFromUri(inParams.requestUri)
	if err != nil {
		return nil, tlerr.InternalError{Format: fmt.Sprintf("Failed to get YANG path from request URI %s: %v", inParams.requestUri, err)}
	}

	pathInfo := NewPathInfo(inParams.uri)
	serverKey := pathInfo.Var(tacacsServerKeyField)

	// Only server-list granularity (and above) is supported for DELETE/REPLACE.
	isServerListPath := strings.HasPrefix(targetUriPath, TACACS_SERVER_GROUPS_PATH)

	if !isServerListPath {
		return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Operation %v not supported on path %s", inParams.oper, targetUriPath)}
	}

	// Handle the openconfig-system-ext source-interface and dscp leaves, which map
	// to the global TACPLUS|global row (not the per-server TACPLUS_SERVER).
	// Detect them from the client's request path so a leaf DELETE is never
	// misrouted into the server-clearing logic below (which would otherwise wipe
	// all TACPLUS_SERVER entries).
	for _, gl := range []struct {
		leaf    string
		field   string
		collect func(*ocbinds.OpenconfigSystem_System) (string, bool)
	}{
		{"source-interface", tacacsSrcIntfField, collectTacacsSourceIntf},
		{"dscp", tacacsDscpField, collectTacacsDscp},
	} {
		if !uriPathEndsWithLeaf(targetUriPath, gl.leaf) && !uriPathEndsWithLeaf(reqUriPath, gl.leaf) {
			continue
		}
		if inParams.oper == DELETE {
			// Field-level delete of just this leaf, leaving the other
			// TACPLUS|global fields (e.g. src_ip) intact.
			return map[string]map[string]db.Value{
				TACPLUS_GLOBAL_TABLE_NAME: {TACPLUS_GLOBAL_KEY: db.Value{Field: map[string]string{gl.field: ""}}},
			}, nil
		}
		if val, ok := gl.collect(sysObj); ok {
			return map[string]map[string]db.Value{
				TACPLUS_GLOBAL_TABLE_NAME: {TACPLUS_GLOBAL_KEY: db.Value{Field: map[string]string{gl.field: val}}},
			}, nil
		}
		return map[string]map[string]db.Value{}, nil
	}

	// Handle DELETE
	if inParams.oper == DELETE {
		// A "clear all servers" delete is only valid when the client's request
		// targets the server-groups / server-group / servers scope. Basing this
		// on requestUri (not inParams.uri) prevents a deeper leaf DELETE that the
		// framework routes through this subtree from silently wiping every
		// TACPLUS_SERVER entry.
		reqClearAllServers := strings.HasSuffix(reqUriPath, "/server-groups") ||
			strings.HasSuffix(reqUriPath, "/server-group") ||
			strings.HasSuffix(reqUriPath, "/servers")
		if serverKey != "" {
			log.V(lvl.DEBUG).Infof("DELETE single TACACS server: %s", serverKey)
			tacTableOps[serverKey] = db.Value{}
		} else if reqClearAllServers {
			// DELETE at server-groups / server-group / servers granularity: clear all.
			log.V(lvl.DEBUG).Infof("DELETE ALL TACACS servers (reqPath=%s)", reqUriPath)
			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: TACPLUS_SERVER_TABLE_NAME})
			if err != nil {
				return nil, tlerr.InternalError{Format: fmt.Sprintf("failed to get keys for clear: %v", err)}
			}
			for _, key := range keys {
				tacTableOps[key.Comp[0]] = db.Value{}
			}
		} else {
			return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("DELETE not supported on path %s", reqUriPath)}
		}
		return dbMutations, nil
	}

	// Handle UPDATE/REPLACE
	if inParams.oper == UPDATE || inParams.oper == REPLACE {
		payloadServers := collectTacacsPayloadServers(sysObj)

		// A group/config-level SET may also carry the global source-interface
		// and/or dscp leaves.
		globalFields := make(map[string]string)
		if src, ok := collectTacacsSourceIntf(sysObj); ok {
			globalFields[tacacsSrcIntfField] = src
		}
		if dscp, ok := collectTacacsDscp(sysObj); ok {
			globalFields[tacacsDscpField] = dscp
		}
		if len(globalFields) > 0 {
			dbMutations[TACPLUS_GLOBAL_TABLE_NAME] = map[string]db.Value{
				TACPLUS_GLOBAL_KEY: {Field: globalFields},
			}
		}

		// REPLACE on a single server with no matching payload => delete it.
		if inParams.oper == REPLACE && serverKey != "" {
			if _, ok := payloadServers[serverKey]; !ok {
				log.V(lvl.DEBUG).Infof("REPLACE on single TACACS server %s with empty payload, deleting.", serverKey)
				tacTableOps[serverKey] = db.Value{}
				return dbMutations, nil
			}
		}

		for addr, srv := range payloadServers {
			applyDefaults := inParams.oper == REPLACE
			if !applyDefaults {
				// UPDATE: apply backend defaults only when the entry is new.
				if _, e := dbGetEntry(cfgDb, &db.TableSpec{Name: TACPLUS_SERVER_TABLE_NAME}, db.Key{Comp: []string{addr}}); e != nil {
					applyDefaults = true
				}
			}
			tacTableOps[addr] = db.Value{Field: buildTacacsServerFields(srv, applyDefaults)}
		}

		// REPLACE at the list level prunes servers not present in the payload.
		if inParams.oper == REPLACE && serverKey == "" {
			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: TACPLUS_SERVER_TABLE_NAME})
			if err != nil {
				return nil, err
			}
			keysToDelete := make(map[string]db.Value)
			for _, key := range keys {
				existing := key.Comp[0]
				if _, present := payloadServers[existing]; !present {
					keysToDelete[existing] = db.Value{}
				}
			}
			if len(keysToDelete) > 0 {
				subOpMap := map[db.DBNum]map[string]map[string]db.Value{
					db.ConfigDB: {
						TACPLUS_SERVER_TABLE_NAME: keysToDelete,
					},
				}
				updateSubOpDataMap(subOpMap, DELETE, inParams)
			}
		}

		return dbMutations, nil
	}

	return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Unsupported operation %v on %s", inParams.oper, inParams.uri)}
}

// DbToYang_tacacs_server_groups_xfmr handles GET requests, reporting the
// TACPLUS_SERVER entries under a single TACACS server-group.
var DbToYang_tacacs_server_groups_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_tacacs_server_groups_xfmr URI: %s, Key: %s", inParams.uri, inParams.key)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Aaa == nil {
		deviceObj.System.Aaa = &ocbinds.OpenconfigSystem_System_Aaa{}
	}
	if deviceObj.System.Aaa.ServerGroups == nil {
		deviceObj.System.Aaa.ServerGroups = &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups{}
	}
	serverGroupsObj := deviceObj.System.Aaa.ServerGroups
	if serverGroupsObj.ServerGroup == nil {
		serverGroupsObj.ServerGroup = make(map[string]*ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup)
	}

	pathInfo := NewPathInfo(inParams.uri)
	groupName := pathInfo.Var(tacacsServerGroupKeyField)
	if groupName == "" {
		groupName = TACACS_DEFAULT_GROUP_NAME
	}
	serverKey := pathInfo.Var(tacacsServerKeyField)

	groupObj := &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup{
		Name: ygot.String(groupName),
		Config: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Config{
			Name: ygot.String(groupName),
			Type: ocbinds.OpenconfigAaaTypes_AAA_SERVER_TYPE_TACACS,
		},
		State: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_State{
			Name: ygot.String(groupName),
			Type: ocbinds.OpenconfigAaaTypes_AAA_SERVER_TYPE_TACACS,
		},
		Servers: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers{
			Server: make(map[string]*ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server),
		},
	}
	serverGroupsObj.ServerGroup[groupName] = groupObj

	// Global source-interface and dscp (openconfig-system-ext) from TACPLUS|global.
	if gEntry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: TACPLUS_GLOBAL_TABLE_NAME}, db.Key{Comp: []string{TACPLUS_GLOBAL_KEY}}); err == nil {
		if v := gEntry.Get(tacacsSrcIntfField); v != "" {
			groupObj.Config.SourceInterface = ygot.String(v)
			groupObj.State.SourceInterface = ygot.String(v)
		}
		if v := gEntry.Get(tacacsDscpField); v != "" {
			// Skip a value the DB should not hold rather than reporting one
			// that openconfig-inet-types:dscp would reject.
			if num, e := strconv.ParseUint(v, 10, 8); e == nil && num <= 63 {
				groupObj.Config.Dscp = ygot.Uint8(uint8(num))
				groupObj.State.Dscp = ygot.Uint8(uint8(num))
			} else {
				log.Warningf("TACPLUS|global:%s is not a valid DSCP (%q), skipping", tacacsDscpField, v)
			}
		}
	}

	targetKeys := make(map[string]bool)
	if serverKey != "" {
		entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: TACPLUS_SERVER_TABLE_NAME}, db.Key{Comp: []string{serverKey}})
		if err == nil {
			targetKeys[serverKey] = true
			_ = entry
		} else {
			log.V(lvl.DEBUG).Infof("TACACS server %s not found in ConfigDB", serverKey)
			return tlerr.NotFoundError{Format: "Resource not found"}
		}
	} else {
		if keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: TACPLUS_SERVER_TABLE_NAME}); err == nil {
			for _, k := range keys {
				if len(k.Comp) > 0 {
					targetKeys[k.Comp[0]] = true
				}
			}
		}
	}

	for addr := range targetKeys {
		entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: TACPLUS_SERVER_TABLE_NAME}, db.Key{Comp: []string{addr}})
		if err != nil {
			continue
		}

		serverObj := &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server{
			Address: ygot.String(addr),
			Config: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_Config{
				Address: ygot.String(addr),
			},
			State: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_State{
				Address: ygot.String(addr),
			},
			Tacacs: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_Tacacs{
				Config: &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_Tacacs_Config{},
				State:  &ocbinds.OpenconfigSystem_System_Aaa_ServerGroups_ServerGroup_Servers_Server_Tacacs_State{},
			},
		}

		if val := entry.Get("timeout"); val != "" {
			if num, e := strconv.ParseUint(val, 10, 16); e == nil {
				serverObj.Config.Timeout = ygot.Uint16(uint16(num))
				serverObj.State.Timeout = ygot.Uint16(uint16(num))
			}
		}
		if val := entry.Get("tcp_port"); val != "" {
			if num, e := strconv.ParseUint(val, 10, 16); e == nil {
				serverObj.Tacacs.Config.Port = ygot.Uint16(uint16(num))
				serverObj.Tacacs.State.Port = ygot.Uint16(uint16(num))
			}
		}
		if val := entry.Get("passkey"); val != "" {
			serverObj.Tacacs.Config.SecretKey = ygot.String(val)
			serverObj.Tacacs.State.SecretKey = ygot.String(val)
		}

		groupObj.Servers.Server[addr] = serverObj
	}

	return nil
}

// Subscribe_tacacs_server_groups_xfmr handles subscriptions for the TACACS
// server-groups subtree.
var Subscribe_tacacs_server_groups_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)

	pathInfo := NewPathInfo(inParams.uri)
	keyName := pathInfo.Var(tacacsServerKeyField)
	if keyName == "" {
		keyName = "*"
	}
	groupName := pathInfo.Var(tacacsServerGroupKeyField)

	log.V(lvl.DEBUG).Infof("Subscribe_tacacs_server_groups_xfmr: path=%v group=%v key=%v proc=%v", inParams.uri, groupName, keyName, inParams.subscProc)

	// A specific server-group (identified by name) - and anything under it, such
	// as its config/source-interface (which maps to TACPLUS|global) or its
	// servers (the flat TACPLUS_SERVER table) - has no CONFIG_DB entry keyed by
	// the group name. Mark the subtree virtual so the framework's
	// parent-existence check (verifyParentTblSubtree) does not reject these
	// requests when no matching CONFIG_DB row exists; the transformer
	// validates/handles the backing tables itself. Only the whole-list URI
	// (no group name) maps the real tables below.
	//
	// Note: the framework invokes this callback with subscProc==TRANSLATE_SUBSCRIBE
	// for BOTH the CRUD parent-existence bypass (which needs isVirtualTbl so a
	// first-time SET to a not-yet-existing group succeeds) and for real
	// subscriptions, and the two are indistinguishable here. Returning concrete
	// keys for the named-group path would therefore reintroduce the first-time
	// SET failure, so ON_CHANGE is not offered for these paths; SAMPLE/periodic
	// telemetry works via the DbToYang callback.
	if groupName != "" || keyName != "*" {
		result.isVirtualTbl = true
		return result, nil
	}

	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		TACPLUS_SERVER_TABLE_NAME: {keyName: {}},
		TACPLUS_GLOBAL_TABLE_NAME: {TACPLUS_GLOBAL_KEY: {}},
	}

	return result, nil
}
