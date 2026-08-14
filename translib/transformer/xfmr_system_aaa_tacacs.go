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
const (
	TACPLUS_SERVER_TABLE_NAME = "TACPLUS_SERVER"
	TACACS_SERVER_GROUPS_PATH = "/openconfig-system:system/aaa/server-groups"
	TACACS_SERVER_GROUP_PATH  = "/openconfig-system:system/aaa/server-groups/server-group"
	TACACS_SERVERS_PATH       = "/openconfig-system:system/aaa/server-groups/server-group/servers"
	TACACS_SERVER_PATH        = "/openconfig-system:system/aaa/server-groups/server-group/servers/server"
	TACACS_DEFAULT_GROUP_NAME = "TACACS"
	TACACS_DEFAULT_TCP_PORT   = "49"
	TACACS_DEFAULT_PRIORITY   = "1"
	tacacsServerGroupKeyField = "name"
	tacacsServerKeyField      = "address"
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

	pathInfo := NewPathInfo(inParams.uri)
	serverKey := pathInfo.Var(tacacsServerKeyField)

	// Only server-list granularity (and above) is supported for DELETE/REPLACE.
	isServerListPath := strings.HasPrefix(targetUriPath, TACACS_SERVER_GROUPS_PATH)
	isConfigOp := strings.HasSuffix(targetUriPath, "/config") ||
		strings.HasSuffix(targetUriPath, "/address") ||
		strings.Contains(targetUriPath, "/tacacs")

	if !isServerListPath {
		return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Operation %v not supported on path %s", inParams.oper, targetUriPath)}
	}

	// Handle DELETE
	if inParams.oper == DELETE {
		if serverKey != "" {
			log.V(lvl.DEBUG).Infof("DELETE single TACACS server: %s", serverKey)
			tacTableOps[serverKey] = db.Value{}
		} else if !isConfigOp || strings.HasSuffix(targetUriPath, "/servers") {
			// DELETE at server-groups / server-group / servers granularity: clear all.
			log.V(lvl.DEBUG).Infof("DELETE ALL TACACS servers (path=%s)", targetUriPath)
			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: TACPLUS_SERVER_TABLE_NAME})
			if err != nil {
				return nil, tlerr.InternalError{Format: fmt.Sprintf("failed to get keys for clear: %v", err)}
			}
			for _, key := range keys {
				tacTableOps[key.Comp[0]] = db.Value{}
			}
		} else {
			return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("DELETE not supported on path %s", targetUriPath)}
		}
		return dbMutations, nil
	}

	// Handle UPDATE/REPLACE
	if inParams.oper == UPDATE || inParams.oper == REPLACE {
		payloadServers := collectTacacsPayloadServers(sysObj)

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

	log.V(lvl.DEBUG).Infof("Subscribe_tacacs_server_groups_xfmr: path=%v key=%v proc=%v", inParams.uri, keyName, inParams.subscProc)

	// Keyed (single-server) requests: bypass the parent-existence check since
	// there is no named server-group entry in CONFIG_DB backing the group key.
	if keyName != "*" {
		result.isVirtualTbl = true
		return result, nil
	}

	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		TACPLUS_SERVER_TABLE_NAME: {keyName: {}},
	}

	return result, nil
}
