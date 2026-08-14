package transformer

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	DNS_NAMESERVER_TABLE_NAME = "DNS_NAMESERVER"
	DNS_DEFAULT_PORT          = 53
	DNS_SERVERS_PATH          = "/openconfig-system:system/dns/servers"
	DNS_SERVER_PATH           = "/openconfig-system:system/dns/servers/server"
	dnsServerKeyField         = "address"
)

func init() {
	XlateFuncBind("DbToYang_dns_servers_list_xfmr", DbToYang_dns_servers_list_xfmr)
	XlateFuncBind("YangToDb_dns_servers_list_xfmr", YangToDb_dns_servers_list_xfmr)
	XlateFuncBind("Subscribe_dns_servers_list_xfmr", Subscribe_dns_servers_list_xfmr)
}

// YangToDb_dns_servers_list_xfmr handles gNMI SET/REPLACE/DELETE/UPDATE on the DNS name-servers list.
// It maps /system/dns/servers/server[address] to the CONFIG_DB DNS_NAMESERVER table, whose entries are
// key-only (no fields). The server "port" leaf is fixed at 53 by the backend and is not persisted.
var YangToDb_dns_servers_list_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	var err error
	log.V(lvl.DEBUG).Infof("Inside YangToDb_dns_servers_list_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		cfgDb, err = db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("CONFIG_DB not found for URI %s: %v", inParams.uri, err)}
		}
		defer cfgDb.DeleteDB()
	}
	sysObj := getAppRootObject(inParams)

	// Initialize Result Maps
	dnsTableOps := make(map[string]db.Value)
	dbMutations := map[string]map[string]db.Value{
		DNS_NAMESERVER_TABLE_NAME: dnsTableOps,
	}

	// Determine Target (List vs Single Item) using YANG Path
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		return nil, tlerr.InternalError{Format: fmt.Sprintf("Failed to get YANG path from URI %s: %v", inParams.uri, err)}
	}

	targetKey := keyFromInParamsOrUri(inParams, dnsServerKeyField)

	isListOp := targetUriPath == DNS_SERVERS_PATH || (targetUriPath == DNS_SERVER_PATH && targetKey == "")
	isServerOp := targetUriPath == DNS_SERVER_PATH && targetKey != ""

	isConfigOp := strings.HasSuffix(targetUriPath, "/config") || strings.HasSuffix(targetUriPath, "/address")

	// Restrict operations on unsupported deeper paths for DELETE and REPLACE.
	if (inParams.oper == DELETE || inParams.oper == REPLACE) && !isListOp && !isServerOp && !isConfigOp {
		return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Operation %v not supported on path %s. Only %s and %s are supported", inParams.oper, targetUriPath, DNS_SERVERS_PATH, DNS_SERVER_PATH)}
	}

	// Handle DELETE
	if inParams.oper == DELETE {
		if isListOp {
			// DELETE List Root -> Clear All
			log.V(lvl.DEBUG).Infof("DELETE ALL DNS name-servers")
			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: DNS_NAMESERVER_TABLE_NAME})
			if err != nil {
				return nil, tlerr.InternalError{Format: fmt.Sprintf("failed to get keys for clear: %v", err)}
			}
			for _, key := range keys {
				dnsTableOps[key.Comp[0]] = db.Value{}
			}
		} else if targetKey != "" && isServerOp {
			// DELETE Single Item -> Delete One Key
			log.V(lvl.DEBUG).Infof("DELETE single DNS name-server: %s", targetKey)
			dnsTableOps[targetKey] = db.Value{}
		} else {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("Invalid DELETE request on %s, targetKey: %s", inParams.uri, targetKey)}
		}
		return dbMutations, nil
	}

	// Extract Payload
	var newServersMap map[string]*ocbinds.OpenconfigSystem_System_Dns_Servers_Server
	if sysObj != nil && sysObj.Dns != nil && sysObj.Dns.Servers != nil {
		newServersMap = sysObj.Dns.Servers.Server
	} else {
		newServersMap = make(map[string]*ocbinds.OpenconfigSystem_System_Dns_Servers_Server)
	}

	// Handle UPDATE/REPLACE
	if inParams.oper == UPDATE || inParams.oper == REPLACE {
		// Pass 1: Shared ADD/UPDATE logic for both UPDATE and REPLACE
		if isListOp {
			log.V(lvl.DEBUG).Infof("%s on DNS name-servers list", inParams.oper)
			for ip := range newServersMap {
				dnsTableOps[ip] = db.Value{Field: map[string]string{nullKey: nullKey}}
			}
		} else if targetKey != "" { // Handles isServerOp for REPLACE/UPDATE and deeper paths for UPDATE
			log.V(lvl.DEBUG).Infof("%s on DNS name-server path for key: %s", inParams.oper, targetKey)
			if inParams.oper == REPLACE && isServerOp && newServersMap[targetKey] == nil {
				log.V(lvl.DEBUG).Infof("REPLACE on single server %s with empty payload, deleting.", targetKey)
				dnsTableOps[targetKey] = db.Value{}
			} else {
				// Ensure the key exists in the DB for UPDATE on any server path or REPLACE on server path with payload.
				dnsTableOps[targetKey] = db.Value{Field: map[string]string{nullKey: nullKey}}
			}
		} else {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("Missing key for %v on %s", inParams.oper, inParams.uri)}
		}

		// Pass 2: Implicit DELETE only for REPLACE on the full list path
		if inParams.oper == REPLACE && isListOp {
			log.V(lvl.DEBUG).Infof("REPLACE: Pruning non-present DNS name-servers from DB")

			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: DNS_NAMESERVER_TABLE_NAME})
			if err != nil {
				return nil, err
			}

			keysToDelete := make(map[string]db.Value)
			for _, key := range keys {
				existingIp := key.Comp[0]
				if _, present := newServersMap[existingIp]; !present {
					log.V(lvl.DEBUG).Infof("REPLACE: Deleting %s", existingIp)
					keysToDelete[existingIp] = db.Value{}
				}
			}

			if len(keysToDelete) > 0 {
				subOpMap := map[db.DBNum]map[string]map[string]db.Value{
					db.ConfigDB: {
						DNS_NAMESERVER_TABLE_NAME: keysToDelete,
					},
				}
				updateSubOpDataMap(subOpMap, DELETE, inParams)
			}
		}

		return dbMutations, nil
	}

	return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Unsupported operation %v on %s", inParams.oper, inParams.uri)}
}

// DbToYang_dns_servers_list_xfmr handles GET requests for DNS name-server config and state.
var DbToYang_dns_servers_list_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_dns_servers_list_xfmr URI: %s, Key: %s", inParams.uri, inParams.key)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	// Initialize the Root Object
	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Dns == nil {
		deviceObj.System.Dns = &ocbinds.OpenconfigSystem_System_Dns{}
	}
	if deviceObj.System.Dns.Servers == nil {
		deviceObj.System.Dns.Servers = &ocbinds.OpenconfigSystem_System_Dns_Servers{}
	}
	serversObj := deviceObj.System.Dns.Servers

	targetKeys := make(map[string]bool)

	targetKey := keyFromInParamsOrUri(inParams, dnsServerKeyField)

	// Single Server GET
	if targetKey != "" {
		if _, err := dbGetEntry(cfgDb, &db.TableSpec{Name: DNS_NAMESERVER_TABLE_NAME}, db.Key{Comp: []string{targetKey}}); err == nil {
			targetKeys[targetKey] = true
		} else {
			log.V(lvl.DEBUG).Infof("DNS name-server %s not found in ConfigDB", targetKey)
			serversObj.Server = make(map[string]*ocbinds.OpenconfigSystem_System_Dns_Servers_Server)
			return tlerr.NotFoundError{Format: "Resource not found"}
		}
	} else {
		// List GET - collect keys from ConfigDB
		if keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: DNS_NAMESERVER_TABLE_NAME}); err == nil {
			for _, k := range keys {
				if len(k.Comp) > 0 {
					targetKeys[k.Comp[0]] = true
				}
			}
		}
	}

	newServerMap := make(map[string]*ocbinds.OpenconfigSystem_System_Dns_Servers_Server)

	for ipAddr := range targetKeys {
		serverObj := &ocbinds.OpenconfigSystem_System_Dns_Servers_Server{
			Address: ygot.String(ipAddr),
			Config:  &ocbinds.OpenconfigSystem_System_Dns_Servers_Server_Config{},
			State:   &ocbinds.OpenconfigSystem_System_Dns_Servers_Server_State{},
		}

		serverObj.Config.Address = ygot.String(ipAddr)
		serverObj.State.Address = ygot.String(ipAddr)
		serverObj.State.Port = ygot.Uint16(DNS_DEFAULT_PORT)

		newServerMap[ipAddr] = serverObj
	}

	serversObj.Server = newServerMap

	return nil
}

// Subscribe_dns_servers_list_xfmr: Handles subscriptions for the DNS name-server list.
var Subscribe_dns_servers_list_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)

	pathInfo := NewPathInfo(inParams.uri)
	keyName := pathInfo.Var("address")
	if keyName == "" {
		keyName = "*"
	}

	log.V(lvl.DEBUG).Infof("Subscribe_dns_servers_list_xfmr: path=%v key=%v proc=%v", inParams.uri, keyName, inParams.subscProc)

	// Single-server (keyed) requests: mark the subtree as a virtual table so the
	// framework's parent-existence check is bypassed. DNS_NAMESERVER entries are
	// config-only (key-only) and have no STATE_DB counterpart; the transformer
	// validates existence itself (DbToYang against CONFIG_DB for GET, YangToDb
	// for DELETE).
	if keyName != "*" {
		result.isVirtualTbl = true
		return result, nil
	}

	// Whole-list subscription: map CONFIG_DB (config only).
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		DNS_NAMESERVER_TABLE_NAME: {keyName: {}},
	}

	return result, nil
}
