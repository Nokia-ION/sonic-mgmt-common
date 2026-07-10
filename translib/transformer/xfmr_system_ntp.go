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

const (
	NTP_SERVER_TABLE_NAME = "NTP_SERVER"
	NTP_DEFAULT_PORT      = 123
	NTP_SERVERS_PATH      = "/openconfig-system:system/ntp/servers"
	NTP_SERVER_PATH       = "/openconfig-system:system/ntp/servers/server"
	nullKey               = "NULL"
	ntpServerKeyField     = "address"
)

// Function variables for mocking in unit tests
var (
	dbGetKeys  = (*db.DB).GetKeys
	dbGetEntry = (*db.DB).GetEntry
)

func init() {
	XlateFuncBind("DbToYang_ntp_servers_list_xfmr", DbToYang_ntp_servers_list_xfmr)
	XlateFuncBind("YangToDb_ntp_servers_list_xfmr", YangToDb_ntp_servers_list_xfmr)
	XlateFuncBind("Subscribe_ntp_servers_list_xfmr", Subscribe_ntp_servers_list_xfmr)
}

// Helper to populate operational state from STATE_DB entry.
// Note: refId/reach may be present in STATE_DB but are not exposed here because
// the (standard) openconfig-system NTP server state schema has no refid/reach
// leaves. Only schema-backed leaves are populated.
func populateServer(entry db.Value, serverObj *ocbinds.OpenconfigSystem_System_Ntp_Servers_Server) {
	if val := entry.Get("stratum"); val != "" {
		if num, err := strconv.ParseUint(val, 10, 8); err == nil {
			serverObj.State.Stratum = ygot.Uint8(uint8(num))
		}
	}
	if val := entry.Get("poll"); val != "" {
		if num, err := strconv.ParseUint(val, 10, 32); err == nil {
			serverObj.State.PollInterval = ygot.Uint32(uint32(num))
		}
	}

	// NTPQ returns milliseconds. OpenConfig expects nanoseconds.
	convertMsToNs := func(field string) *int64 {
		if val := entry.Get(field); val != "" {
			if num, err := strconv.ParseFloat(val, 64); err == nil {
				// multiply by 1,000,000 to convert ms to ns
				return ygot.Int64(int64(num * 1e6))
			}
		}
		return nil
	}

	serverObj.State.RootDelay = convertMsToNs("delay")
	serverObj.State.Offset = convertMsToNs("offset")
	serverObj.State.RootDispersion = convertMsToNs("jitter") // Map jitter to dispersion
}

// YangToDb_ntp_servers_list_xfmr handles gNMI SET/REPLACE/DELETE/UPDATE on the NTP servers list.
// It handles both the full list path and specific server instance paths.
var YangToDb_ntp_servers_list_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	var err error
	log.V(lvl.DEBUG).Infof("Inside YangToDb_ntp_servers_list_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		cfgDb, err = db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("CONFIG_DB not found for URI %s: %v", inParams.uri, err)}
		}
		defer cfgDb.DeleteDB()
	}
	deviceObj := getAppRootObject(inParams)

	// Initialize Result Maps
	ntpTableOps := make(map[string]db.Value)
	dbMutations := map[string]map[string]db.Value{
		NTP_SERVER_TABLE_NAME: ntpTableOps,
	}

	// Determine Target (List vs Single Item) using YANG Path
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		return nil, tlerr.InternalError{Format: fmt.Sprintf("Failed to get YANG path from URI %s: %v", inParams.uri, err)}
	}

	targetKey := keyFromInParamsOrUri(inParams, ntpServerKeyField)

	isListOp := targetUriPath == NTP_SERVERS_PATH || (targetUriPath == NTP_SERVER_PATH && targetKey == "")
	isServerOp := targetUriPath == NTP_SERVER_PATH && targetKey != ""

	isConfigOp := strings.HasSuffix(targetUriPath, "/config") || strings.HasSuffix(targetUriPath, "/address")

	// Restrict operations on unsupported deeper paths for DELETE and REPLACE.
	if (inParams.oper == DELETE || inParams.oper == REPLACE) && !isListOp && !isServerOp && !isConfigOp {
		return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Operation %v not supported on path %s. Only %s and %s are supported", inParams.oper, targetUriPath, NTP_SERVERS_PATH, NTP_SERVER_PATH)}
	}

	// Handle DELETE
	if inParams.oper == DELETE {
		if isListOp {
			// DELETE List Root -> Clear All
			log.V(lvl.DEBUG).Infof("DELETE ALL NTP servers")
			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: NTP_SERVER_TABLE_NAME})
			if err != nil {
				return nil, tlerr.InternalError{Format: fmt.Sprintf("failed to get keys for clear: %v", err)}
			}
			for _, key := range keys {
				ntpTableOps[key.Comp[0]] = db.Value{}
			}
		} else if targetKey != "" && isServerOp {
			// DELETE Single Item -> Delete One Key
			log.V(lvl.DEBUG).Infof("DELETE single NTP server: %s", targetKey)
			ntpTableOps[targetKey] = db.Value{}
		} else {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("Invalid DELETE request on %s, targetKey: %s", inParams.uri, targetKey)}
		}
		return dbMutations, nil
	}

	// Extract Payload
	var newServersMap map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server
	if deviceObj != nil && deviceObj.Ntp != nil && deviceObj.Ntp.Servers != nil {
		newServersMap = deviceObj.Ntp.Servers.Server
	} else {
		newServersMap = make(map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server)
	}

	// Handle REPLACE
	if inParams.oper == UPDATE || inParams.oper == REPLACE {
		// Pass 1: Shared ADD/UPDATE logic for both UPDATE and REPLACE
		if isListOp {
			log.V(lvl.DEBUG).Infof("%s on NTP servers list", inParams.oper)
			for ip := range newServersMap {
				ntpTableOps[ip] = db.Value{Field: map[string]string{nullKey: nullKey}}
			}
		} else if targetKey != "" { // Handles isServerOp for REPLACE/UPDATE and deeper paths for UPDATE
			log.V(lvl.DEBUG).Infof("%s on NTP server path for key: %s", inParams.oper, targetKey)
			if inParams.oper == REPLACE && isServerOp && newServersMap[targetKey] == nil {
				log.V(lvl.DEBUG).Infof("REPLACE on single server %s with empty payload, deleting.", targetKey)
				ntpTableOps[targetKey] = db.Value{}
			} else {
				// Ensure the key exists in the DB for UPDATE on any server path or REPLACE on server path with payload.
				ntpTableOps[targetKey] = db.Value{Field: map[string]string{nullKey: nullKey}}
			}
		} else {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("Missing key for %v on %s", inParams.oper, inParams.uri)}
		}

		// Pass 2: Implicit DELETE only for REPLACE on the full list path
		if inParams.oper == REPLACE && isListOp {
			log.V(lvl.DEBUG).Infof("REPLACE: Pruning non-present servers from DB")

			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: NTP_SERVER_TABLE_NAME})
			if err != nil {
				return nil, err
			}

			// Build a map of keys to delete
			keysToDelete := make(map[string]db.Value)
			for _, key := range keys {
				existingIp := key.Comp[0]
				if _, present := newServersMap[existingIp]; !present {
					log.V(lvl.DEBUG).Infof("REPLACE: Deleting %s", existingIp)
					keysToDelete[existingIp] = db.Value{}
				}
			}

			// Inject orphaned keys into the transaction as a DELETE operation
			if len(keysToDelete) > 0 {
				subOpMap := map[db.DBNum]map[string]map[string]db.Value{
					db.ConfigDB: {
						NTP_SERVER_TABLE_NAME: keysToDelete,
					},
				}
				// Merge the deletions into main transaction builder
				updateSubOpDataMap(subOpMap, DELETE, inParams)
			}
		}

		return dbMutations, nil
	}

	return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Unsupported operation %v on %s", inParams.oper, inParams.uri)}
}

// DbToYang_ntp_servers_list_xfmr handles GET requests for NTP server config and state.
var DbToYang_ntp_servers_list_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_ntp_servers_list_xfmr URI: %s, Key: %s", inParams.uri, inParams.key)
	cfgDb := inParams.dbs[db.ConfigDB]
	stateDb := inParams.dbs[db.StateDB] // may be nil if Translib didn't open STATE_DB for this path

	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	// Initialize the Root Object
	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Ntp == nil {
		deviceObj.System.Ntp = &ocbinds.OpenconfigSystem_System_Ntp{}
	}
	if deviceObj.System.Ntp.Servers == nil {
		deviceObj.System.Ntp.Servers = &ocbinds.OpenconfigSystem_System_Ntp_Servers{}
	}
	serversObj := deviceObj.System.Ntp.Servers

	targetKeys := make(map[string]bool)

	targetKey := keyFromInParamsOrUri(inParams, ntpServerKeyField)

	// Single Server GET
	if targetKey != "" {
		exists := false
		if _, err := dbGetEntry(cfgDb, &db.TableSpec{Name: NTP_SERVER_TABLE_NAME}, db.Key{Comp: []string{targetKey}}); err == nil {
			exists = true
		}
		if !exists && stateDb != nil {
			if _, err := dbGetEntry(stateDb, &db.TableSpec{Name: NTP_SERVER_TABLE_NAME}, db.Key{Comp: []string{targetKey}}); err == nil {
				exists = true
			}
		}
		if exists {
			targetKeys[targetKey] = true
		} else {
			log.V(lvl.DEBUG).Infof("NTP server %s not found in ConfigDB or StateDB", targetKey)
			serversObj.Server = make(map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server)
			return tlerr.NotFoundError{Format: "Resource not found"}
		}
	} else {
		// List GET - collect keys from ConfigDB, and StateDB if available
		for _, targetDb := range []*db.DB{cfgDb, stateDb} {
			if targetDb == nil {
				continue
			}
			if keys, err := dbGetKeys(targetDb, &db.TableSpec{Name: NTP_SERVER_TABLE_NAME}); err == nil {
				for _, k := range keys {
					if len(k.Comp) > 0 {
						targetKeys[k.Comp[0]] = true
					}
				}
			}
		}
	}

	newServerMap := make(map[string]*ocbinds.OpenconfigSystem_System_Ntp_Servers_Server)

	// Process Keys
	for ipAddr := range targetKeys {
		// Create a new Server object for the map.
		serverObj := &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server{
			Address: ygot.String(ipAddr),
			Config:  &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server_Config{},
			State:   &ocbinds.OpenconfigSystem_System_Ntp_Servers_Server_State{},
		}

		// Populate Config and base State
		serverObj.Config.Address = ygot.String(ipAddr)
		serverObj.State.Address = ygot.String(ipAddr)
		serverObj.State.Port = ygot.Uint16(NTP_DEFAULT_PORT)

		// Populate dynamic operational state from StateDB (skip if Translib didn't open it)
		if stateDb != nil {
			if entry, err := dbGetEntry(stateDb, &db.TableSpec{Name: NTP_SERVER_TABLE_NAME}, db.Key{Comp: []string{ipAddr}}); err == nil {
				populateServer(entry, serverObj)
			} else {
				log.V(lvl.DEBUG).Infof("No StateDB entry for NTP server %s: %v", ipAddr, err)
			}
		}
		newServerMap[ipAddr] = serverObj
	}

	serversObj.Server = newServerMap

	return nil
}

// Subscribe_ntp_servers_list_xfmr: Handles subscriptions for NTP server list
var Subscribe_ntp_servers_list_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)

	pathInfo := NewPathInfo(inParams.uri)
	keyName := pathInfo.Var("address")
	if keyName == "" {
		keyName = "*"
	}

	log.V(lvl.DEBUG).Infof("Subscribe_ntp_servers_list_xfmr: path=%v key=%v proc=%v", inParams.uri, keyName, inParams.subscProc)

	// Single-server (keyed) requests: mark the subtree as a virtual table so the
	// framework's parent-existence check (verifyParentTblSubtree) is bypassed.
	// That check requires every returned {db,table,key} tuple to exist, but an
	// NTP server is config-only and has no STATE_DB entry, so requiring the
	// STATE_DB key yields a spurious "Resource not found" on single-server
	// GET and DELETE. The transformer validates existence itself (DbToYang
	// against CONFIG_DB/STATE_DB for GET, YangToDb for DELETE). The whole-list
	// URI never reaches this path (the caller short-circuits it as exists=true).
	if keyName != "*" {
		result.isVirtualTbl = true
		return result, nil
	}

	// Whole-list subscription: map CONFIG_DB (config) and STATE_DB (operational state).
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		NTP_SERVER_TABLE_NAME: {keyName: {}},
	}

	result.dbDataMap[db.StateDB] = map[string]map[string]map[string]string{
		NTP_SERVER_TABLE_NAME: {keyName: {}},
	}

	return result, nil
}
