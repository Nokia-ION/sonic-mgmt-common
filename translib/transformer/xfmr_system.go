package transformer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	ygot "github.com/openconfig/ygot/ygot"
)

const (
	PATHZ_TBL = "PATHZ_TABLE"
	READS_GET = "get"
	READS_SUB = "subscribe"
	WRITES    = "set"
	GNXI_ID   = "gnxi"

	MEMORY_TBL  = "MEMORY_STATS"
	CPU_TBL     = "CPU_STATS"
	PROC_TBL    = "PROCESS_STATS"
	LOGGING_TBL = "SYSLOG_SERVER"
	systemKey   = "system"

	/** Credential Tables **/
	CREDENTIALS_TBL = "CREDENTIALS"
	CRED_PATHZ_TBL  = CREDENTIALS_TBL + "|" + CRED_PATHZ_PREFIX
	CRED_AUTHZ_TBL  = "CREDENTIALS|AUTHZ_POLICY"
	CERT_TBL        = CREDENTIALS_TBL + "|" + CRED_CERT_PREFIX
	AUTHZ_TBL       = "AUTHZ_TABLE"
	ACCEPTS         = "permitted"
	REJECTS         = "denied"
	cntResult       = "cntResult"
	tsResult        = "tsResult"

	/** First component of a CREDENTIALS key, naming the kind of credential
	  that the rest of the key identifies. **/
	CRED_CERT_PREFIX  = "CERT"
	CRED_PATHZ_PREFIX = "PATHZ_POLICY"

	/** Second component of the pathz CREDENTIALS key, which is a singleton. **/
	CRED_PATHZ_ACTIVE_KEY = "ACTIVE"

	/** System Root paths **/
	SYSTEM_ROOT = "/openconfig-system:system"

	/** Pathz paths **/
	GRPC_OC_SERVERS = SYSTEM_ROOT + "/openconfig-system-grpc:grpc-servers"
	GRPC_SERVERS    = SYSTEM_ROOT + "/grpc-servers"
	GRPC_SERVER     = GRPC_OC_SERVERS + "/grpc-server"

	/** Authz paths **/
	AUTHZ_POLICY_COUNTERS   = GRPC_SERVER + "/authz-policy-counters"
	ALL_AUTHZ               = AUTHZ_POLICY_COUNTERS + "/rpcs"
	SINGLE_AUTHZ            = ALL_AUTHZ + "/rpc"
	AUTHZ_STATE             = SINGLE_AUTHZ + "/state"
	AUTHZ_SUCCESS           = AUTHZ_STATE + "/access-accepts"
	AUTHZ_SUCCESS_TIMESTAMP = AUTHZ_STATE + "/last-access-accept"
	AUTHZ_FAILED            = AUTHZ_STATE + "/access-rejects"
	AUTHZ_FAILED_TIMESTAMP  = AUTHZ_STATE + "/last-access-reject"
	PATHZ_POLICY_COUNTERS   = GRPC_SERVER + "/gnsi-pathz:gnmi-pathz-policy-counters"
	ALL_PATHZ               = PATHZ_POLICY_COUNTERS + "/paths"
	SINGLE_PATHZ            = ALL_PATHZ + "/path"

	PATHZ_STATE  = SINGLE_PATHZ + "/state"
	PATHZ_READS  = PATHZ_STATE + "/reads"
	PATHZ_WRITES = PATHZ_STATE + "/writes"

	PATHZ_READ_SUCCESS            = PATHZ_READS + "/access-accepts"
	PATHZ_READ_SUCCESS_TIMESTAMP  = PATHZ_READS + "/last-access-accept"
	PATHZ_READ_FAILED             = PATHZ_READS + "/access-rejects"
	PATHZ_READ_FAILED_TIMESTAMP   = PATHZ_READS + "/last-access-reject"
	PATHZ_WRITE_SUCCESS           = PATHZ_WRITES + "/access-accepts"
	PATHZ_WRITE_SUCCESS_TIMESTAMP = PATHZ_WRITES + "/last-access-accept"
	PATHZ_WRITE_FAILED            = PATHZ_WRITES + "/access-rejects"
	PATHZ_WRITE_FAILED_TIMESTAMP  = PATHZ_WRITES + "/last-access-reject"
	ACCOUNT_TBL                   = "CREDENTIALS|SSH_ACCOUNT"
	CONSOLE_TBL                   = "CREDENTIALS|CONSOLE_ACCOUNT"
	SSH_TBL                       = "CREDENTIALS|SSH_HOST"
)

type SysMem struct {
	Total                  uint64
	Used                   uint64
	Free                   uint64
	TotalEccErrors         uint64
	CorrectableEccErrors   uint64
	UncorrectableEccErrors uint64
}

type timeStat struct {
	avg      uint8
	interval uint64
}

type Cpu struct {
	User   int64
	System int64
	Idle   int64
	Total  timeStat
}

type CpuState struct {
	user   uint8
	system uint8
	idle   uint8
}

type Proc struct {
	Cmd      string
	Start    uint64
	User     float64
	System   float64
	Mem      uint64
	Cputil   float64
	Memutil  float64
	MemLimit *uint64
}

type ProcessState struct {
	Args              []string
	CpuUsageSystem    uint64
	CpuUsageUser      uint64
	CpuUtilization    uint8
	MemoryLimit       *uint64
	MemoryUsage       uint64
	MemoryUtilization uint8
	Name              string
	Pid               uint64
	StartTime         uint64
}

type sshState struct {
	caKeys   certData
	hostCert certData
	hostKey  certData
	counters accessCounters
}

type accessCounters struct {
	accessRejects    uint64
	lastAccessReject uint64
	accessAccepts    uint64
	lastAccessAccept uint64
}

type certData struct {
	version string
	created uint64
}

// XfmrCache a sync.Map for storing path values that need to be cached
var XfmrCache sync.Map

var pathzOpers = [][]string{
	[]string{READS_GET, ACCEPTS},
	[]string{READS_GET, REJECTS},
	[]string{READS_SUB, ACCEPTS},
	[]string{READS_SUB, REJECTS},
	[]string{WRITES, ACCEPTS},
	[]string{WRITES, REJECTS}}

var pathzMap = &pathzCounters{
	mu:      sync.Mutex{},
	updated: make(map[string]time.Time),
	data:    make(map[string]map[string]map[string]*uint64),
}

func init() {
	XlateFuncBind("DbToYang_grpc_server_xfmr", DbToYang_grpc_server_xfmr)
	XlateFuncBind("YangToDb_grpc_server_xfmr", YangToDb_grpc_server_xfmr)
	XlateFuncBind("Subscribe_grpc_server_xfmr", Subscribe_grpc_server_xfmr)
	XlateFuncBind("DbToYang_grpc_server_key_xfmr", DbToYang_grpc_server_key_xfmr)
	XlateFuncBind("DbToYangPath_grpc_server_path_xfmr", DbToYangPath_grpc_server_path_xfmr)
	XlateFuncBind("DbToYang_ssh_server_state_xfmr", DbToYang_ssh_server_state_xfmr)
	XlateFuncBind("Subscribe_ssh_server_state_xfmr", Subscribe_ssh_server_state_xfmr)
	XlateFuncBind("DbToYang_authz_policy_xfmr", DbToYang_authz_policy_xfmr)
	XlateFuncBind("Subscribe_authz_policy_xfmr", Subscribe_authz_policy_xfmr)
	XlateFuncBind("DbToYang_pathz_policies_xfmr", DbToYang_pathz_policies_xfmr)
	XlateFuncBind("Subscribe_pathz_policies_xfmr", Subscribe_pathz_policies_xfmr)
	XlateFuncBind("DbToYang_pathz_policies_key_xfmr", DbToYang_pathz_policies_key_xfmr)
	XlateFuncBind("DbToYang_console_counters_xfmr", DbToYang_console_counters_xfmr)
	XlateFuncBind("Subscribe_console_counters_xfmr", Subscribe_console_counters_xfmr)
	XlateFuncBind("Subscribe_sys_memory_xfmr", Subscribe_sys_memory_xfmr)
	XlateFuncBind("DbToYang_sys_memory_xfmr", DbToYang_sys_memory_xfmr)
	XlateFuncBind("DbToYang_sys_cpus_xfmr", DbToYang_sys_cpus_xfmr)
	XlateFuncBind("Subscribe_sys_cpus_xfmr", Subscribe_sys_cpus_xfmr)
	XlateFuncBind("DbToYang_sys_procs_xfmr", DbToYang_sys_procs_xfmr)
	XlateFuncBind("Subscribe_sys_procs_xfmr", Subscribe_sys_procs_xfmr)
	XlateFuncBind("YangToDb_syslog_server_ip_fld_xfmr", YangToDb_syslog_server_ip_fld_xfmr)
	XlateFuncBind("DbToYang_syslog_server_ip_fld_xfmr", DbToYang_syslog_server_ip_fld_xfmr)
	XlateFuncBind("DbToYangPath_remote_server_path_xfmr", DbToYangPath_remote_server_path_xfmr)
	XlateFuncBind("YangToDb_sys_config_key_xfmr", YangToDb_sys_config_key_xfmr)
	XlateFuncBind("DbToYang_sys_config_key_xfmr", DbToYang_sys_config_key_xfmr)
	XlateFuncBind("YangToDb_sys_config_xfmr", YangToDb_sys_config_xfmr)
	XlateFuncBind("DbToYang_sys_config_xfmr", DbToYang_sys_config_xfmr)
	XlateFuncBind("Subscribe_sys_config_xfmr", Subscribe_sys_config_xfmr)
	XlateFuncBind("DbToYang_sys_state_xfmr", DbToYang_sys_state_xfmr)
	XlateFuncBind("Subscribe_sys_state_xfmr", Subscribe_sys_state_xfmr)
}

type grpcState struct {
	name           string
	certVersion    string
	certCreated    uint64
	caVersion      string
	caCreated      uint64
	crlVersion     string
	crlCreated     uint64
	authPolVersion string
	authPolCreated uint64
	profileId      string
	pathzVersion   string
	pathzCreated   uint64
}

type pathzCounters struct {
	mu      sync.Mutex
	updated map[string]time.Time
	data    map[string]map[string]map[string]*uint64
}

type policyState struct {
	instance ocbinds.E_OpenconfigSystem_System_GnmiPathzPolicies_Policies_Policy_State_Instance
	version  string
	created  uint64
}

var dbToYangPathzInstanceMap = map[string]ocbinds.E_OpenconfigSystem_System_GnmiPathzPolicies_Policies_Policy_State_Instance{
	"ACTIVE":  ocbinds.OpenconfigSystem_System_GnmiPathzPolicies_Policies_Policy_State_Instance_ACTIVE,
	"SANDBOX": ocbinds.OpenconfigSystem_System_GnmiPathzPolicies_Policies_Policy_State_Instance_SANDBOX,
}

func getAppRootObject(inParams XfmrParams) *ocbinds.OpenconfigSystem_System {
	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	return deviceObj.System
}

var YangToDb_sys_config_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	if inParams.oper == DELETE {
		switch {
		case strings.Contains(inParams.requestUri, "hostname"):
			return map[string]map[string]db.Value{
				"DEVICE_METADATA": {
					"localhost": db.Value{
						Field: map[string]string{"hostname": ""},
					},
				},
			}, nil
		case strings.Contains(inParams.requestUri, "domain-name"):
			return map[string]map[string]db.Value{
				"DEVICE_METADATA": {
					"localhost": db.Value{
						Field: map[string]string{"domain_name": ""},
					},
				},
			}, nil
		default:
			return nil, tlerr.InvalidArgs("SET Delete not supported at the subtree level for %v", inParams.requestUri)
		}
	}
	sysObj := getAppRootObject(inParams)
	if sysObj == nil {
		log.V(lvl.DEBUG).Info("YangToDb_sys_config_xfmr: Empty component.")
		return nil, tlerr.NotSupported("YangToDb_sys_config_xfmr: Empty component.")
	}
	if sysObj.Config == nil {
		return nil, nil
	}
	resMap := make(map[string]string)
	if sysObj.Config.Hostname != nil {
		resMap["hostname"] = *sysObj.Config.Hostname
	}
	if sysObj.Config.DomainName != nil {
		resMap["domain_name"] = *sysObj.Config.DomainName
	}
	if len(resMap) == 0 {
		return nil, nil
	}
	memMap := map[string]map[string]db.Value{
		"DEVICE_METADATA": {
			"localhost": db.Value{
				Field: resMap,
			},
		},
	}

	// In case of a Set Replace for the /system/config subtree, do an Update instead to
	// preserve system-written fields in DEVICE_METADATA|localhost table.
	if inParams.oper == REPLACE {
		updateSubOpDataMap(map[db.DBNum]map[string]map[string]db.Value{
			db.ConfigDB: memMap,
		}, UPDATE, inParams)
		return nil, nil
	}

	return memMap, nil
}

// getSystemHostAndDomainName reads hostname and domain_name from
// DEVICE_METADATA|localhost. Both /system/config and /system/state expose them.
func getSystemHostAndDomainName(inParams XfmrParams) (hostname string, domainName string, err error) {
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		cfgDb, err = db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			return "", "", tlerr.InvalidArgsError{Format: err.Error()}
		}
		defer cfgDb.DeleteDB()
	}

	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: "DEVICE_METADATA"}, db.Key{Comp: []string{"localhost"}})
	if err != nil {
		return "", "", err
	}

	return entry.Get("hostname"), entry.Get("domain_name"), nil
}

var DbToYang_sys_config_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	sysObj := getAppRootObject(inParams)

	hostname, domainName, err := getSystemHostAndDomainName(inParams)
	if err != nil {
		return err
	}
	if isUnsetSysNameLeafRequest(inParams.requestUri, hostname, domainName) {
		return nil
	}

	ygot.BuildEmptyTree(sysObj)
	sysObj.Config.Hostname = optionalString(hostname)
	sysObj.Config.DomainName = optionalString(domainName)

	return nil
}

// optionalString returns nil for an empty DB field, so an unset or deleted
// leaf is omitted rather than reported as "", which also violates the
// oc-inet:domain-name length 1..253 restriction.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isUnsetSysNameLeafRequest reports whether the request targets the hostname
// or domain-name leaf and that leaf has no value. The transformer must then
// leave the ygot tree untouched: translib only answers NotFound for a leaf
// when the tree is unchanged, and otherwise replies with "{}" as the leaf
// value, which clients cannot unmarshal into a string.
func isUnsetSysNameLeafRequest(requestUri, hostname, domainName string) bool {
	yangPath, err := getYangPathFromUri(requestUri)
	if err != nil {
		return false
	}
	leaf := yangPath[strings.LastIndex(yangPath, "/")+1:]
	if i := strings.LastIndex(leaf, ":"); i >= 0 {
		leaf = leaf[i+1:]
	}
	switch leaf {
	case "hostname":
		return hostname == ""
	case "domain-name":
		return domainName == ""
	}
	return false
}

var YangToDb_sys_config_key_xfmr KeyXfmrYangToDb = func(inParams XfmrParams) (string, error) {
	log.V(lvl.DEBUG).Info("YangToDb_sys_config_key_xfmr: ", inParams.uri)
	return "localhost", nil
}

var DbToYang_sys_config_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	rmap := make(map[string]interface{})
	log.V(lvl.DEBUG).Info("DbToYang_sys_config_key_xfmr root, uri: ", inParams.ygRoot, inParams.uri)
	return rmap, nil
}

var Subscribe_sys_config_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	result := XfmrSubscOutParams{
		dbDataMap: RedisDbSubscribeMap{
			db.ConfigDB: {"DEVICE_METADATA": {"localhost": {
				"hostname":    "Hostname",
				"domain_name": "DomainName",
			}}}},
		onChange: OnchangeDisable,
	}
	log.V(lvl.DEBUG).Infof("Subscribe_sys_config_xfmr:%s", inParams.requestURI)

	return result, nil
}

// getSystemBootAndUpTime returns OpenConfig system/state boot-time and up-time
// in nanoseconds. boot-time is cached so it stays stable across GETs.
func getSystemBootAndUpTime() (bootTime uint64, upTime uint64, err error) {
	sysinfo := syscall.Sysinfo_t{}
	if sys_err := syscall.Sysinfo(&sysinfo); sys_err != nil {
		log.V(lvl.WARNING).Infof("getSystemBootAndUpTime syscall error: %s", sys_err.Error())
		return 0, 0, sys_err
	}
	upTime = uint64(sysinfo.Uptime * 1_000_000_000)

	bt, ok := XfmrCache.Load("boot-time")
	if !ok {
		bt = uint64(time.Now().UnixNano() - int64(sysinfo.Uptime)*1_000_000_000)
		XfmrCache.Store("boot-time", bt)
	}
	bootTime = bt.(uint64)
	return bootTime, upTime, nil
}

var DbToYang_sys_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	sysObj := getAppRootObject(inParams)
	if sysObj == nil {
		return tlerr.NotSupported("DbToYang_sys_state_xfmr: Empty component.")
	}

	bootTime, upTime, err := getSystemBootAndUpTime()
	if err != nil {
		return err
	}

	hostname, domainName, err := getSystemHostAndDomainName(inParams)
	if err != nil {
		return err
	}
	if isUnsetSysNameLeafRequest(inParams.requestUri, hostname, domainName) {
		return nil
	}

	ygot.BuildEmptyTree(sysObj)
	sysObj.State.BootTime = &bootTime
	sysObj.State.UpTime = &upTime
	sysObj.State.Hostname = optionalString(hostname)
	sysObj.State.DomainName = optionalString(domainName)
	return nil
}

var Subscribe_sys_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	result := XfmrSubscOutParams{
		onChange: OnchangeDisable,
	}
	log.V(lvl.DEBUG).Infof("Subscribe_sys_state_xfmr:%s", inParams.requestURI)
	return result, nil
}

func getAllKeys(sdb *db.DB, tblName string) ([]string, error) {
	tbl, err := sdb.GetTable(&db.TableSpec{Name: tblName})
	if err != nil {
		return nil, fmt.Errorf("Can't get table: %v, err: %v", tblName, err)
	}
	log.V(3).Infof("tbl: %v", tbl)
	keys, err := tbl.GetKeys()
	if err != nil {
		return nil, fmt.Errorf("Can't get keys from %v, err: %v", tblName, err)
	}
	log.V(3).Infof("tbl keys: %v", keys)
	ret := []string{}
	for _, key := range keys {
		if len(key.Comp) != 3 {
			// This is a phantom key. Ignore it.
			continue
		}
		ret = append(ret, key.Comp[2])
	}
	log.V(3).Infof("keys: %v", ret)
	return ret, nil
}

var Subscribe_ssh_server_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	log.V(3).Infof("Subscribe_ssh_server_state_xfmr:%s", inParams.requestURI)

	return XfmrSubscOutParams{
		dbDataMap: RedisDbSubscribeMap{
			db.StateDB: {"CREDENTIALS": {"SSH_HOST": {}}}},
		onChange: OnchangeEnable,
		nOpts:    &notificationOpts{mInterval: 0, pType: OnChange},
	}, nil
}
var Subscribe_authz_policy_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	log.V(3).Infof("Subscribe_authz_policy_xfmr:%s", inParams.requestURI)
	return XfmrSubscOutParams{
		dbDataMap: RedisDbSubscribeMap{
			db.StateDB: {"CREDENTIALS": {"AUTHZ_POLICY|gnxi": {}}}},
		onChange: OnchangeEnable,
		nOpts:    &notificationOpts{mInterval: 0, pType: OnChange},
	}, nil
}

var DbToYang_ssh_server_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	var state sshState

	table, err := inParams.dbs[inParams.curDb].GetEntry(&db.TableSpec{Name: "CREDENTIALS"}, db.Key{Comp: []string{"SSH_HOST"}})
	if err != nil {
		log.V(3).Infof("Failed to read from StateDB: %v", inParams.table)
		return err
	}

	state.caKeys.version = table.Get("ca_keys_version")
	time := table.Get("ca_keys_created_on")
	if state.caKeys.created, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
		log.V(0).Infof("Couldn't find ca_keys_created_on: %v", err)
	}
	state.hostKey.version = table.Get("host_key_version")
	time = table.Get("host_key_created_on")
	if state.hostKey.created, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
		log.V(0).Infof("Couldn't find host_key_created_on: %v", err)
	}
	state.hostCert.version = table.Get("host_cert_version")
	time = table.Get("host_cert_created_on")
	if state.hostCert.created, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
		log.V(0).Infof("Couldn't find host_cert_created_on: %v", err)
	}
	accepts := table.Get("access_accepts")
	if state.counters.accessAccepts, err = strconv.ParseUint(accepts, 10, 64); err != nil && accepts != "" {
		log.V(0).Infof("Couldn't find access_accepts: %v", err)
	}
	lastAccept := table.Get("last_access_accept")
	if state.counters.lastAccessAccept, err = strconv.ParseUint(lastAccept, 10, 64); err != nil && lastAccept != "" {
		log.V(0).Infof("Couldn't find last_access_accept: %v", err)
	}
	rejects := table.Get("access_rejects")
	if state.counters.accessRejects, err = strconv.ParseUint(rejects, 10, 64); err != nil && rejects != "" {
		log.V(0).Infof("Couldn't find access_rejects: %v", err)
	}
	lastReject := table.Get("last_access_reject")
	if state.counters.lastAccessReject, err = strconv.ParseUint(lastReject, 10, 64); err != nil && lastReject != "" {
		log.V(0).Infof("Couldn't find last_access_reject: %v", err)
	}

	sysObj := getAppRootObject(inParams)
	ygot.BuildEmptyTree(sysObj.SshServer.State)

	sysObj.SshServer.State.ActiveTrustedUserCaKeysCreatedOn = &state.caKeys.created
	sysObj.SshServer.State.ActiveTrustedUserCaKeysVersion = &state.caKeys.version
	sysObj.SshServer.State.ActiveHostCertificateCreatedOn = &state.hostKey.created
	sysObj.SshServer.State.ActiveHostCertificateVersion = &state.hostKey.version
	sysObj.SshServer.State.ActiveHostKeyCreatedOn = &state.hostCert.created
	sysObj.SshServer.State.ActiveHostKeyVersion = &state.hostCert.version
	sysObj.SshServer.State.Counters.AccessAccepts = &state.counters.accessAccepts
	sysObj.SshServer.State.Counters.AccessRejects = &state.counters.accessRejects
	sysObj.SshServer.State.Counters.LastAccessAccept = &state.counters.lastAccessAccept
	sysObj.SshServer.State.Counters.LastAccessReject = &state.counters.lastAccessReject
	return nil
}
var DbToYang_authz_policy_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	var state certData

	table, err := inParams.dbs[inParams.curDb].GetEntry(&db.TableSpec{Name: CRED_AUTHZ_TBL}, db.Key{Comp: []string{GNXI_ID}})
	if err != nil {
		log.V(3).Infof("Failed to read from StateDB: %v", inParams.table)
		return err
	}

	state.version = table.Get("authz_version")
	time := table.Get("authz_created_on")
	if state.created, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
		log.V(3).Infof("Couldn't find authz_created_on: %v", err)
	}

	sysObj := getAppRootObject(inParams)
	ygot.BuildEmptyTree(sysObj.Aaa.Authorization.State)

	sysObj.Aaa.Authorization.State.GrpcAuthzPolicyCreatedOn = &state.created
	sysObj.Aaa.Authorization.State.GrpcAuthzPolicyVersion = &state.version

	return nil
}

func (m *pathzCounters) getCounters(pathzTables db.Table, xpath string) map[string]map[string]*uint64 {
	result := make(map[string]map[string]*uint64)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updated == nil || m.data == nil {
		m.updated = make(map[string]time.Time)
		m.data = make(map[string]map[string]map[string]*uint64)
	}

	// Update the map if necessary
	updateTime, ok := m.updated[xpath]
	if !ok {
		result = GetPathzPolicyCounter(pathzTables, xpath)
		if len(m.data) < 50 {
			m.data[xpath] = result
			m.updated[xpath] = time.Now()
		}
	} else if time.Now().After(updateTime.Add(30 * time.Second)) {
		m.data[xpath] = GetPathzPolicyCounter(pathzTables, xpath)
		m.updated[xpath] = time.Now()
	}

	// Fetch the result or return the previously calculated result
	if data, ok := m.data[xpath]; ok {
		result = data
	}
	return result
}

func GetPathzPolicyCounter(pathzTables db.Table, path string) map[string]map[string]*uint64 {
	cntMap := make(map[string]*uint64)
	tsMap := make(map[string]*uint64)

	for _, tmp := range pathzOpers {
		pattern := PatternGenerator(tmp, path)
		if pattern == "" {
			log.V(3).Infof("Invalid pathz counter key pattern.")
			continue
		}
		key := db.NewKey(tmp[0], path, tmp[1])

		// Sum the data collected
		value, err := pathzTables.GetEntry(*key)
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("Cannot get value from %v table for %v, err: %v", PATHZ_TBL, key, err)
			continue
		}

		c := value.Get("count")
		if c == "" {
			continue
		}
		dbCnt, err := strconv.ParseUint(c, 10, 64)
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("Failed to convert counters from DB for pathz, err: %v", err)
			continue
		}
		tsval := value.Get("timestamp")
		if tsval == "" {
			continue
		}
		dbTs, err := strconv.ParseUint(tsval, 10, 64)
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("Failed to convert timestamp for counters from DB for pathz, err: %v", err)
			continue
		}

		cnt, cntExists := cntMap[pattern]
		if cntExists && cnt != nil {
			cntUpdate, err := strconv.ParseUint(strconv.FormatUint((*cnt+dbCnt), 10), 10, 64)
			if err != nil {
				log.V(tlerr.ErrorSeverity(err)).Infof("Failed to convert counters for pathz, err: %v", err)
				continue
			}
			cntMap[pattern] = &cntUpdate
		} else {
			cntMap[pattern] = &dbCnt
		}

		ts, tsExists := tsMap[pattern]
		if !tsExists || ts == nil || *ts < dbTs {
			tsMap[pattern] = &dbTs
		}
	}
	return map[string]map[string]*uint64{cntResult: cntMap, tsResult: tsMap}
}

func getAllXpaths(pathzTables db.Table) ([]string, error) {
	var res []string
	check := make(map[string]bool)
	pathzTableKeys, err := pathzTables.GetKeys()
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("Cannot get all keys from %v table, err: %v", PATHZ_TBL, err)
		return []string{}, err
	}
	for _, pathzTableKey := range pathzTableKeys {
		if len(pathzTableKey.Comp) != 3 {
			log.V(3).Infof("invalid number of Comps for pathzTableKey %v.", pathzTableKey)
			continue
		}
		if pathzTableKey.Comp[1] != "" {
			key := pathzTableKey.Comp[1]
			if val, ok := check[key]; !ok || !val {
				res = append(res, key)
				check[key] = true
			}
		}
	}

	return res, nil
}

var pathToPatternKeysMap = map[string][]string{
	PATHZ_READ_SUCCESS:            []string{"reads", ACCEPTS},
	PATHZ_READ_SUCCESS_TIMESTAMP:  []string{"reads", ACCEPTS},
	PATHZ_READ_FAILED:             []string{"reads", REJECTS},
	PATHZ_READ_FAILED_TIMESTAMP:   []string{"reads", REJECTS},
	PATHZ_WRITE_SUCCESS:           []string{"writes", ACCEPTS},
	PATHZ_WRITE_SUCCESS_TIMESTAMP: []string{"writes", ACCEPTS},
	PATHZ_WRITE_FAILED:            []string{"writes", REJECTS},
	PATHZ_WRITE_FAILED_TIMESTAMP:  []string{"writes", REJECTS},
}

func PatternGenerator(params []string, xpath string) string {
	if len(params) != 2 {
		log.V(3).Infof("Invalid params for patternGenerator %#v", params)
		return ""
	}

	if params[0] == READS_GET || params[0] == READS_SUB || params[0] == "reads" {
		return "*|reads|" + xpath + "|" + params[1]
	}

	if params[0] == WRITES || params[0] == "writes" {
		return "*|writes|" + xpath + "|" + params[1]
	}

	log.V(3).Infof("Invalid operation %v", params[0])
	return ""
}

var Subscribe_grpc_server_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	pathInfo := NewPathInfo(inParams.uri)
	serverName := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return XfmrSubscOutParams{}, err
	}
	log.V(3).Infof("Subscribe_grpc_server_xfmr: targetUriPath: %s name: %s", targetUriPath, serverName)

	var result XfmrSubscOutParams
	// Both CREDENTIALS rows name the same grpc-server instance, so the framework
	// emits one notification per row it is given. Registering a row the request
	// is not asking about therefore repeats the same leaf, and a client that
	// rejects a repeated leaf, such as ygnmi, discards the whole response.
	// Register the one row the request reads: the counters come from the pathz
	// row, everything else from the certificate row. A request spanning both is
	// driven by the certificate row, since the counters are sampled rather than
	// reported on change.
	isCounters := strings.HasPrefix(targetUriPath, PATHZ_POLICY_COUNTERS)
	credRow := CRED_CERT_PREFIX + "|" + GNXI_ID
	if isCounters {
		credRow = CRED_PATHZ_PREFIX + "|" + CRED_PATHZ_ACTIVE_KEY
	}
	result.dbDataMap = RedisDbSubscribeMap{
		db.StateDB: map[string]map[string]map[string]string{
			CREDENTIALS_TBL: {credRow: {}},
		},
	}

	if !isCounters {
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{mInterval: 0, pType: OnChange}
	} else {

		// For counters, configure nOpts to enable sampling on path.
		result.onChange = OnchangeEnable
		result.nOpts = &notificationOpts{mInterval: 60, pType: Sample}
	}

	return result, nil
}
var DbToYang_pathz_policies_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	instances := []string{pathInfo.Var("instance")}
	targetUriPath, _ := getYangPathFromUri(pathInfo.Path)
	log.V(3).Infof("DbToYang_pathz_policies_xfmr: targetUriPath: %s instances: %v", targetUriPath, instances)

	stateDb := inParams.dbs[db.StateDB]
	if len(instances) == 0 || len(instances[0]) == 0 {
		var err error
		if instances, err = getAllKeys(stateDb, CRED_PATHZ_TBL); err != nil {
			return err
		}
	}
	sysObj := getAppRootObject(inParams)
	ygot.BuildEmptyTree(sysObj)
	ygot.BuildEmptyTree(sysObj.GnmiPathzPolicies)
	ygot.BuildEmptyTree(sysObj.GnmiPathzPolicies.Policies)

	for _, instance := range instances {
		log.V(3).Infof("instance: %v", instance)
		i, ok := dbToYangPathzInstanceMap[instance]
		if !ok {
			log.V(0).Infof("Pathz Policy Instance not found: %v", instance)
			continue
		}
		policyObj, ok := sysObj.GnmiPathzPolicies.Policies.Policy[i]
		if !ok {
			var err error
			policyObj, err = sysObj.GnmiPathzPolicies.Policies.NewPolicy(i)
			if err != nil {
				log.V(0).Infof("sysObj.GnmiPathzPolicies.Policies.NewPolicy failed: %v", err)
				continue
			}
		}
		table, err := stateDb.GetEntry(&db.TableSpec{Name: CRED_PATHZ_TBL}, db.Key{Comp: []string{instance}})
		if err != nil {
			log.V(0).Infof("Failed to read from StateDB %v, id: %v, err: %v", inParams.table, instance, err)
			return err
		}
		var state policyState

		state.instance = i
		state.version = table.Get("pathz_version")
		time := table.Get("pathz_created_on")
		if state.created, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
			return err
		}
		ygot.BuildEmptyTree(policyObj)
		policyObj.State.Instance = state.instance
		policyObj.State.CreatedOn = &state.created
		policyObj.State.Version = &state.version
	}
	return nil
}
var DbToYang_grpc_server_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	log.V(3).Info("DbToYang_grpc_server_key_xfmr root, uri: ", inParams.ygRoot, inParams.uri)

	return map[string]interface{}{"name": NewPathInfo(inParams.uri).Var("name")}, nil
}

// DbToYangPath_grpc_server_path_xfmr names the grpc-server instance a state
// database notification belongs to.
//
// A subscription may wildcard the server name, and the notification then has
// to say which server it describes. The key transformer cannot answer that: it
// only reflects the name the request already carried, which a wildcard does
// not have. Without this the subscribe path falls back to converting the db
// key directly, which CREDENTIALS has no sonic model for, and every
// notification is dropped. That is what a client discovering servers with a
// wildcard sees as an empty result.
//
// The server name is the third component of a CREDENTIALS|CERT|<name> key, the
// same source a Get of the list enumerates. The pathz policy is stored once
// under a key that names no server, and is reported against the default
// server, matching how the read path reports it.
var DbToYangPath_grpc_server_path_xfmr PathXfmrDbToYangFunc = func(params XfmrDbToYgPathParams) error {
	if params.tblName != CREDENTIALS_TBL {
		log.V(3).Infof("DbToYangPath_grpc_server_path_xfmr: unexpected table %v", params.tblName)
		return nil
	}

	var serverName string
	switch {
	case len(params.tblKeyComp) > 1 && params.tblKeyComp[0] == CRED_CERT_PREFIX:
		serverName = params.tblKeyComp[1]
	case len(params.tblKeyComp) > 0 && params.tblKeyComp[0] == CRED_PATHZ_PREFIX:
		serverName = GNXI_ID
	default:
		log.V(3).Infof("DbToYangPath_grpc_server_path_xfmr: no server name in key %v", params.tblKeyComp)
		return nil
	}

	params.ygPathKeys[GRPC_SERVER+"/name"] = serverName
	log.V(3).Infof("DbToYangPath_grpc_server_path_xfmr: ygPathKeys: %v", params.ygPathKeys)

	return nil
}

// GNMI_CFG_TBL is the CONFIG_DB table that holds the gNMI server runtime
// settings. The "gnmi" key carries port/dscp/certificate_id; certificate
// material itself lives under the separate "certs" key and is managed
// independently (not by this transformer).
const (
	GNMI_CFG_TBL = "GNMI"
	GNMI_CFG_KEY = "gnmi"
	GNMI_CERT_ID = "certificate_id"
)

// validateCertificateId rejects a binding to an SSL profile the device does not
// have. gNSI Certz publishes a CREDENTIALS|CERT|<profile> entry for every
// profile holding credentials, so that table is the list of bindable ids. The
// default gnxi profile always exists, even before any rotation has happened.
//
// Without this check a typo would be accepted and silently ignored, leaving the
// listener on its previous profile while the config claimed otherwise.
//
// The write path only hands transformers a CONFIG_DB handle, so StateDB is
// opened here rather than taken from inParams.dbs.
func validateCertificateId(certId string) error {
	if certId == "" || certId == GNXI_ID {
		return nil
	}
	stateDb, err := db.NewDB(getDBOptions(db.StateDB))
	if err != nil {
		return tlerr.New("cannot validate certificate-id %q: %v", certId, err)
	}
	defer stateDb.DeleteDB()

	entry, err := stateDb.GetEntry(&db.TableSpec{Name: CERT_TBL}, db.Key{Comp: []string{certId}})
	if err != nil || !entry.IsPopulated() {
		return tlerr.InvalidArgs("certificate-id %q: no such SSL profile", certId)
	}
	return nil
}

// YangToDb_grpc_server_xfmr maps the writable leaves of the OpenConfig
// grpc-server config container onto the SONiC "GNMI" CONFIG_DB table so that a
// controller can program the gNMI server through a single, vendor-neutral
// OpenConfig gNMI Set. Today it handles:
//
//	.../config/port           -> GNMI|gnmi:port
//	.../config/dscp           -> GNMI|gnmi:dscp  (openconfig-system-grpc-ext augment)
//	.../config/certificate-id -> GNMI|gnmi:certificate_id
//
// The DSCP leaf is the model-driven equivalent of the vendor "qos dscp" knob;
// telemetry.sh reads GNMI|gnmi:dscp at start-up and the gNMI server marks its
// served (observer) traffic with it. Changes take effect on the next gNMI
// service restart, mirroring how "port" is applied.
//
// certificate-id names the gNSI Certz SSL profile the listener presents. It is
// a binding, not certificate material: the cert/key/CA bytes are owned by Certz
// (and by the separate GNMI|certs provisioning), and this transformer only
// records which profile is selected. The gNMI server picks the new profile up
// on subsequent TLS handshakes, so the change applies without a restart.
var YangToDb_grpc_server_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	pathInfo := NewPathInfo(inParams.uri)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		return nil, err
	}
	log.V(3).Infof("YangToDb_grpc_server_xfmr: op=%v uri=%s target=%s", inParams.oper, inParams.uri, targetUriPath)

	// DELETE of an individual leaf clears just that field in GNMI|gnmi.
	// The last path token may carry a module prefix for augmented leaves
	// (e.g. .../config/openconfig-system-grpc-ext:dscp), so normalize it to the
	// bare leaf name before matching. A DELETE targeting the whole config
	// container clears both writable fields.
	if inParams.oper == DELETE {
		leaf := targetUriPath
		if i := strings.LastIndex(leaf, "/"); i >= 0 {
			leaf = leaf[i+1:]
		}
		if j := strings.LastIndex(leaf, ":"); j >= 0 {
			leaf = leaf[j+1:]
		}
		delFields := make(map[string]string)
		switch leaf {
		case "dscp":
			delFields["dscp"] = ""
		case "port":
			delFields["port"] = ""
		case "certificate-id":
			delFields[GNMI_CERT_ID] = ""
		case "config", "grpc-server":
			delFields["dscp"] = ""
			delFields["port"] = ""
			delFields[GNMI_CERT_ID] = ""
		default:
			return res, nil
		}
		res[GNMI_CFG_TBL] = map[string]db.Value{GNMI_CFG_KEY: {Field: delFields}}
		return res, nil
	}

	sysObj := getAppRootObject(inParams)
	if sysObj == nil || sysObj.GrpcServers == nil {
		return res, nil
	}

	fields := make(map[string]string)
	for serverName, serverObj := range sysObj.GrpcServers.GrpcServer {
		if serverObj == nil || serverObj.Config == nil {
			continue
		}
		cfg := serverObj.Config
		if cfg.Port != nil {
			fields["port"] = strconv.FormatUint(uint64(*cfg.Port), 10)
		}
		if cfg.Dscp != nil {
			fields["dscp"] = strconv.FormatUint(uint64(*cfg.Dscp), 10)
		}
		if cfg.CertificateId != nil {
			if err := validateCertificateId(*cfg.CertificateId); err != nil {
				return nil, err
			}
			fields[GNMI_CERT_ID] = *cfg.CertificateId
		}
		log.V(3).Infof("YangToDb_grpc_server_xfmr: server %s -> %v", serverName, fields)
	}

	if len(fields) == 0 {
		return res, nil
	}
	res[GNMI_CFG_TBL] = map[string]db.Value{GNMI_CFG_KEY: {Field: fields}}
	return res, nil
}

var DbToYang_grpc_server_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	serverNames := []string{pathInfo.Var("name")}
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		log.V(0).Infof("Error Parsing Uri Path, err: %v", err)
	}
	if log.V(3) {
		log.Info("SubtreeXfmrFunc - Uri SYS AUTH: ", inParams.uri)
		log.Info("TARGET URI PATH SYS AUTH:", targetUriPath)
		log.Info("names:", serverNames)
	}
	stateDb := inParams.dbs[db.StateDB]
	if stateDb == nil {
		return errors.New("DbToYang_grpc_server_xfmr stateDb is nil!")
	}
	// An unset key and an explicit "*" both mean "every gRPC server"; without
	// the wildcard case a Get on grpc-server[name=*] looks up a profile
	// literally named "*" and comes back empty.
	if len(serverNames) == 0 || len(serverNames[0]) == 0 || serverNames[0] == "*" {
		var err error
		if serverNames, err = getAllKeys(stateDb, CERT_TBL); err != nil {
			return err
		}
	}
	sysObj := getAppRootObject(inParams)
	ygot.BuildEmptyTree(sysObj)
	ygot.BuildEmptyTree(sysObj.GrpcServers)

	for _, serverName := range serverNames {
		log.V(3).Info("serverName: ", serverName)
		var state grpcState
		state.name = serverName

		// Read the writable gNMI server config (GNMI|gnmi) once: it carries
		// port and dscp. Certificate state is reported for the default gnxi
		// profile (certificates are managed separately, not by this feature).
		var gnmiCfg db.Value
		gnmiCfgOk := false
		if cfgDb := inParams.dbs[db.ConfigDB]; cfgDb != nil {
			if entry, cerr := cfgDb.GetEntry(&db.TableSpec{Name: GNMI_CFG_TBL}, db.Key{Comp: []string{GNMI_CFG_KEY}}); cerr == nil {
				gnmiCfg = entry
				gnmiCfgOk = true
			}
		}
		certzID := GNXI_ID
		certTable, err := stateDb.GetEntry(&db.TableSpec{Name: CERT_TBL}, db.Key{Comp: []string{certzID}})
		if err != nil {
			log.V(0).Infof("Failed to read from StateDB %v | %v err: %v", CERT_TBL, certzID, err)
		} else {
			state.certVersion = certTable.Get("certificate_version")
			state.caVersion = certTable.Get("ca_trust_bundle_version")
			state.crlVersion = certTable.Get("certificate_revocation_list_bundle_version")
			state.authPolVersion = certTable.Get("authentication_policy_version")
			state.profileId = certTable.Get("ssl_profile_id")
			time := certTable.Get("certificate_created_on")
			if state.certCreated, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
				log.V(0).Infof("Cannot convert `certificate_created_on` for %v, err: %v", certzID, err)
			}
			time = certTable.Get("ca_trust_bundle_created_on")
			if state.caCreated, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
				log.V(0).Infof("Cannot convert `ca_trust_bundle_created_on` for %v, err: %v", certzID, err)
			}
			time = certTable.Get("certificate_revocation_list_bundle_created_on")
			if state.crlCreated, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
				log.V(0).Infof("Cannot convert `certificate_revocation_list_bundle_created_on` for %v, err: %v", certzID, err)
			}
			time = certTable.Get("authentication_policy_created_on")
			if state.authPolCreated, err = strconv.ParseUint(time, 10, 64); err != nil && time != "" {
				log.V(0).Infof("Cannot convert `authentication_policy_created_on` for %v, err: %v", certzID, err)
			}
		}

		pathzTable, err := stateDb.GetEntry(&db.TableSpec{Name: CRED_PATHZ_TBL}, db.Key{Comp: []string{"ACTIVE"}})
		if err != nil {
			log.V(0).Infof("Failed to read from StateDB %v, err: %v", CRED_PATHZ_TBL, err)
		} else {
			state.pathzVersion = pathzTable.Get("pathz_version")
			if timeStr := pathzTable.Get("pathz_created_on"); timeStr != "" {
				if state.pathzCreated, err = strconv.ParseUint(timeStr, 10, 64); err != nil {
					log.V(0).Infof("Cannot convert `pathz_created_on` for %v, err: %v", serverName, err)
				}
			}
		}
		serverObj, ok := sysObj.GrpcServers.GrpcServer[serverName]
		if !ok {
			serverObj, err = sysObj.GrpcServers.NewGrpcServer(serverName)
			if err != nil {
				log.V(0).Infof("sysObj.GrpcServers.NewGrpcServer(%v) failed: %v", serverName, err)
				continue
			}
		}
		ygot.BuildEmptyTree(serverObj)
		serverObj.State.Name = &state.name
		serverObj.State.CaTrustBundleVersion = &state.caVersion
		serverObj.State.CaTrustBundleCreatedOn = &state.caCreated
		serverObj.State.CertificateVersion = &state.certVersion
		serverObj.State.CertificateCreatedOn = &state.certCreated
		serverObj.State.CertificateRevocationListBundleCreatedOn = &state.crlCreated
		serverObj.State.CertificateRevocationListBundleVersion = &state.crlVersion
		serverObj.State.AuthenticationPolicyVersion = &state.authPolVersion
		serverObj.State.SslProfileId = &state.profileId
		serverObj.State.AuthenticationPolicyCreatedOn = &state.authPolCreated
		serverObj.State.GnmiPathzPolicyCreatedOn = &state.pathzCreated
		serverObj.State.GnmiPathzPolicyVersion = &state.pathzVersion

		// Populate the writable config leaves (port, dscp, certificate-id)
		// from CONFIG_DB GNMI|gnmi so a GET of the grpc-server config subtree
		// reflects what YangToDb_grpc_server_xfmr programmed.
		if gnmiCfgOk {
			if portStr := gnmiCfg.Get("port"); portStr != "" {
				if p, perr := strconv.ParseUint(portStr, 10, 16); perr == nil {
					port := uint16(p)
					serverObj.Config.Port = &port
				}
			}
			if dscpStr := gnmiCfg.Get("dscp"); dscpStr != "" {
				if d, derr := strconv.ParseUint(dscpStr, 10, 8); derr == nil {
					cfgDscp := uint8(d)
					stDscp := cfgDscp
					serverObj.Config.Dscp = &cfgDscp
					serverObj.State.Dscp = &stDscp
				}
			}
		}
		// There is a single gRPC listener and a single binding for it in
		// GNMI|gnmi, but this list is keyed from CREDENTIALS|CERT, so every
		// Certz profile shows up as its own entry. Each entry is therefore a
		// view of the same listener: report the binding on all of them, so
		// that reading back any entry a controller just wrote to returns the
		// value it set.
		//
		// An unset binding means the listener serves the default profile, so
		// report that rather than leaving the leaf absent.
		cfgCertId := GNXI_ID
		if gnmiCfgOk {
			if id := gnmiCfg.Get(GNMI_CERT_ID); id != "" {
				cfgCertId = id
			}
		}
		stCertId := cfgCertId
		serverObj.Config.CertificateId = &cfgCertId
		serverObj.State.CertificateId = &stCertId

		// Authz counter
		authzTables, err := stateDb.GetTable(&db.TableSpec{Name: AUTHZ_TBL})
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("getAuthzPolicyCounter failed to get AUTHZ_TBL, err: %v", err)
			return err
		}

		rpcString := pathInfo.Var("name#2")
		rpcStrings := []string{rpcString}

		if rpcString == "" || rpcString == "*" {
			rpcStrings = []string{}
			rpcStrings, err = getAllRpcs(authzTables, serverName)
			if err != nil {
				log.V(tlerr.ErrorSeverity(err)).Infof("Failed get all authz rpcs, err: %v", err)
				return err
			}
		}

		ygot.BuildEmptyTree(serverObj.AuthzPolicyCounters)
		for _, rpcString := range rpcStrings {
			service, rpc, err := getServiceRpc(rpcString)
			if err != nil {
				log.V(0).Infof("invalid RPC method %s", rpcString)
				continue
			}

			authzPolicyData := getAuthzPolicyCounter(authzTables, serverName, rpcString)
			rpcObj, ok := serverObj.AuthzPolicyCounters.Rpcs.Rpc[rpcString]
			if !ok {
				rpcObj, err = serverObj.AuthzPolicyCounters.Rpcs.NewRpc(rpcString)
				if err != nil {
					log.V(0).Infof("serverObj.AuthzPolicyCounters.Rpcs.NewRpc(%v) failed: %v", rpcString, err)
					continue
				}
			}
			ygot.BuildEmptyTree(rpcObj)

			// If targetUriPath is a parent AUTHZ_STATE, i.e.root path, all counters and timestamps should be returned
			allAuthzCounter := strings.HasPrefix(AUTHZ_STATE, targetUriPath) || targetUriPath == GRPC_OC_SERVERS

			tmpCnt := make(map[string]*uint64)
			tmpTs := make(map[string]*uint64)
			if cnt, ok := authzPolicyData[cntResult]; ok {
				tmpCnt = cnt
			}
			if ts, ok := authzPolicyData[tsResult]; ok {
				tmpTs = ts
			}
			// Handle root paths here.
			if allAuthzCounter {
				ygot.BuildEmptyTree(rpcObj.State)
				rpcObj.State.AccessAccepts = tmpCnt["*|"+serverName+"|"+service+"|"+rpc+"|"+ACCEPTS]
				rpcObj.State.LastAccessAccept = tmpTs["*|"+serverName+"|"+service+"|"+rpc+"|"+ACCEPTS]
				rpcObj.State.AccessRejects = tmpCnt["*|"+serverName+"|"+service+"|"+rpc+"|"+REJECTS]
				rpcObj.State.LastAccessReject = tmpTs["*|"+serverName+"|"+service+"|"+rpc+"|"+REJECTS]

			} else {
				// Handle leaf paths here.
				switch targetUriPath {
				case AUTHZ_SUCCESS:
					rpcObj.State.AccessAccepts = tmpCnt["*|"+serverName+"|"+service+"|"+rpc+"|"+ACCEPTS]
				case AUTHZ_SUCCESS_TIMESTAMP:
					rpcObj.State.LastAccessAccept = tmpTs["*|"+serverName+"|"+service+"|"+rpc+"|"+ACCEPTS]
				case AUTHZ_FAILED:
					rpcObj.State.AccessRejects = tmpCnt["*|"+serverName+"|"+service+"|"+rpc+"|"+REJECTS]
				case AUTHZ_FAILED_TIMESTAMP:
					rpcObj.State.LastAccessReject = tmpTs["*|"+serverName+"|"+service+"|"+rpc+"|"+REJECTS]
				}
			}
		}
		// Pathz counter is for GNXI_ID only
		if serverName != GNXI_ID {
			continue
		}

		// Pathz counter
		pathzTables, err := stateDb.GetTable(&db.TableSpec{Name: PATHZ_TBL})
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("getPathzPolicyCounter failed to get PATHZ_TBL, err: %v", err)
		}

		xpath := pathInfo.Var("xpath")
		xpaths := []string{xpath}

		if xpath == "" || xpath == "*" {
			xpaths = []string{}
			xpaths, err = getAllXpaths(pathzTables)
			if err != nil {
				log.V(tlerr.ErrorSeverity(err)).Infof("Failed get all paths, err: %v", err)
			}
		}

		ygot.BuildEmptyTree(serverObj.GnmiPathzPolicyCounters)
		for _, xpath := range xpaths {
			// Processing these counters is hard on the CPU. We will only update these counters every 30 seconds.
			pathzPolicyData := pathzMap.getCounters(pathzTables, xpath)

			pathObj, ok := serverObj.GnmiPathzPolicyCounters.Paths.Path[xpath]
			if !ok {
				pathObj, err = serverObj.GnmiPathzPolicyCounters.Paths.NewPath(xpath)
				if err != nil {
					log.V(0).Infof("serverObj.GnmiPathzPolicyCounters.NewPath(%v) failed: %v", xpath, err)
					continue
				}
			}
			ygot.BuildEmptyTree(pathObj)

			// If targetUriPath is a parent PATHZ_STATE, i.e.root path, all counters and timestamps should be returned
			allPathzCounter := strings.HasPrefix(PATHZ_STATE, targetUriPath) || targetUriPath == GRPC_OC_SERVERS

			tmpCnt := make(map[string]*uint64)
			tmpTs := make(map[string]*uint64)
			if cnt, ok := pathzPolicyData[cntResult]; ok {
				tmpCnt = cnt
			}
			if ts, ok := pathzPolicyData[tsResult]; ok {
				tmpTs = ts
			}

			// Handle root paths here.
			if allPathzCounter || targetUriPath == PATHZ_READS || targetUriPath == PATHZ_WRITES {
				ygot.BuildEmptyTree(pathObj.State)
				if allPathzCounter || targetUriPath == PATHZ_READS {
					pathObj.State.Reads.AccessAccepts = tmpCnt[PatternGenerator(pathToPatternKeysMap[PATHZ_READ_SUCCESS], xpath)]
					pathObj.State.Reads.LastAccessAccept = tmpTs[PatternGenerator(pathToPatternKeysMap[PATHZ_READ_SUCCESS_TIMESTAMP], xpath)]
					pathObj.State.Reads.AccessRejects = tmpCnt[PatternGenerator(pathToPatternKeysMap[PATHZ_READ_FAILED], xpath)]
					pathObj.State.Reads.LastAccessReject = tmpTs[PatternGenerator(pathToPatternKeysMap[PATHZ_READ_FAILED_TIMESTAMP], xpath)]
				}
				if allPathzCounter || targetUriPath == PATHZ_WRITES {
					pathObj.State.Writes.AccessAccepts = tmpCnt[PatternGenerator(pathToPatternKeysMap[PATHZ_WRITE_SUCCESS], xpath)]
					pathObj.State.Writes.LastAccessAccept = tmpTs[PatternGenerator(pathToPatternKeysMap[PATHZ_WRITE_SUCCESS_TIMESTAMP], xpath)]
					pathObj.State.Writes.AccessRejects = tmpCnt[PatternGenerator(pathToPatternKeysMap[PATHZ_WRITE_FAILED], xpath)]
					pathObj.State.Writes.LastAccessReject = tmpTs[PatternGenerator(pathToPatternKeysMap[PATHZ_WRITE_FAILED_TIMESTAMP], xpath)]
				}
			} else {
				// Handle leaf paths here.
				patternKeys := pathToPatternKeysMap[targetUriPath]
				if patternKeys == nil {
					log.V(0).Infof("Invalid pathz table key: %#v", targetUriPath)
					continue
				}
				pattern := PatternGenerator([]string{patternKeys[0], patternKeys[1]}, xpath)

				switch targetUriPath {
				case PATHZ_READ_SUCCESS:
					pathObj.State.Reads.AccessAccepts = tmpCnt[pattern]
				case PATHZ_READ_SUCCESS_TIMESTAMP:
					pathObj.State.Reads.LastAccessAccept = tmpTs[pattern]
				case PATHZ_READ_FAILED:
					pathObj.State.Reads.AccessRejects = tmpCnt[pattern]
				case PATHZ_READ_FAILED_TIMESTAMP:
					pathObj.State.Reads.LastAccessReject = tmpTs[pattern]
				case PATHZ_WRITE_SUCCESS:
					pathObj.State.Writes.AccessAccepts = tmpCnt[pattern]
				case PATHZ_WRITE_SUCCESS_TIMESTAMP:
					pathObj.State.Writes.LastAccessAccept = tmpTs[pattern]
				case PATHZ_WRITE_FAILED:
					pathObj.State.Writes.AccessRejects = tmpCnt[pattern]
				case PATHZ_WRITE_FAILED_TIMESTAMP:
					pathObj.State.Writes.LastAccessReject = tmpTs[pattern]
				}
			}
		}
	}
	return nil
}
var DbToYang_pathz_policies_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	log.V(3).Info("DbToYang_pathz_policies_key_xfmr root, uri: ", inParams.ygRoot, inParams.uri)
	return map[string]interface{}{"instance": NewPathInfo(inParams.uri).Var("instance")}, nil
}

func getAuthzPolicyCounter(authzTables db.Table, server string, rpcString string) map[string]map[string]*uint64 {
	cntMap := make(map[string]*uint64)
	tsMap := make(map[string]*uint64)

	for _, oper := range []string{ACCEPTS, REJECTS} {
		var service string
		var rpc string
		service, rpc, err := getServiceRpc(rpcString)
		if err != nil {
			log.V(0).Infof("invalid RPC method %s", rpcString)
			continue
		}

		pattern := "*|" + server + "|" + service + "|" + rpc + "|" + oper
		key := db.NewKey(server, service, rpc, oper)

		// Sum the data collected
		value, err := authzTables.GetEntry(*key)
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("Cannot get value from %v table for %v, err: %v", AUTHZ_TBL, key, err)
			continue
		}

		c := value.Get("count")
		if c != "" {
			if dbCnt, err := strconv.ParseUint(c, 10, 64); err == nil {
				cntMap[pattern] = &dbCnt
			} else {
				log.V(tlerr.ErrorSeverity(err)).Infof("Failed to convert counters from DB for authz, err: %v", err)
			}
		}

		ts := value.Get("timestamp")
		if ts != "" {
			if dbTs, err := strconv.ParseUint(ts, 10, 64); err == nil {
				tsMap[pattern] = &dbTs
			} else {
				log.V(tlerr.ErrorSeverity(err)).Infof("Failed to convert timestamp for counters from DB for authz, err: %v", err)
			}
		}
	}
	return map[string]map[string]*uint64{cntResult: cntMap, tsResult: tsMap}
}

func getServiceRpc(rpcString string) (string, string, error) {
	strs := strings.Split(rpcString, "/")
	if len(strs) == 3 {
		return strs[1], strs[2], nil
	}

	return "", "", errors.New("invalid RPC method " + rpcString)
}

func getAllRpcs(authzTables db.Table, server string) ([]string, error) {
	var res []string
	check := make(map[string]bool)
	authzTableKeys, err := authzTables.GetKeys()
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("Cannot get all keys from %v table, err: %v", AUTHZ_TBL, err)
		return []string{}, err
	}
	for _, authzTableKey := range authzTableKeys {
		if len(authzTableKey.Comp) != 4 {
			log.V(3).Infof("invalid number of Comps for authzTableKey %v.", authzTableKey)
			continue
		}
		if authzTableKey.Comp[0] != server {
			continue
		}
		key := "/" + authzTableKey.Comp[1] + "/" + authzTableKey.Comp[2]
		if val, ok := check[key]; !ok || !val {
			res = append(res, key)
			check[key] = true
		}
	}

	return res, nil
}

var Subscribe_pathz_policies_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	pathInfo := NewPathInfo(inParams.uri)
	instance := pathInfo.Var("instance")
	if instance == "" {
		instance = "*"
	}
	targetUriPath, _ := getYangPathFromUri(pathInfo.Path)
	log.V(3).Infof("Subscribe_pathz_policies_xfmr: targetUriPath: %s instance: %s", targetUriPath, instance)

	key := strings.Join([]string{"PATHZ_POLICY", instance}, "|")
	return XfmrSubscOutParams{
		dbDataMap: RedisDbSubscribeMap{
			db.StateDB: {CREDENTIALS_TBL: {key: {}}}},
		onChange: OnchangeEnable,
		nOpts:    &notificationOpts{mInterval: 0, pType: OnChange},
	}, nil
}

var DbToYang_console_counters_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	var counters accessCounters

	table, err := inParams.dbs[inParams.curDb].GetEntry(&db.TableSpec{Name: "CREDENTIALS"}, db.Key{Comp: []string{"CONSOLE_METRICS"}})
	if err != nil {
		log.V(0).Infof("Failed to read from StateDB: %v", inParams.table)
		return err
	}

	accepts := table.Get("access_accepts")
	if counters.accessAccepts, err = strconv.ParseUint(accepts, 10, 64); err != nil && accepts != "" {
		log.V(0).Infof("Couldn't find access_accepts: %v", err)
	}
	lastAccept := table.Get("last_access_accept")
	if counters.lastAccessAccept, err = strconv.ParseUint(lastAccept, 10, 64); err != nil && lastAccept != "" {
		log.V(0).Infof("Couldn't find last_access_accept: %v", err)
	}
	rejects := table.Get("access_rejects")
	if counters.accessRejects, err = strconv.ParseUint(rejects, 10, 64); err != nil && rejects != "" {
		log.V(0).Infof("Couldn't find access_rejects: %v", err)
	}
	lastReject := table.Get("last_access_reject")
	if counters.lastAccessReject, err = strconv.ParseUint(lastReject, 10, 64); err != nil && lastReject != "" {
		log.V(0).Infof("Couldn't find last_access_reject: %v", err)
	}

	sysObj := getAppRootObject(inParams)
	ygot.BuildEmptyTree(sysObj)
	ygot.BuildEmptyTree(sysObj.Console)
	ygot.BuildEmptyTree(sysObj.Console.State)

	sysObj.Console.State.Counters.AccessAccepts = &counters.accessAccepts
	sysObj.Console.State.Counters.AccessRejects = &counters.accessRejects
	sysObj.Console.State.Counters.LastAccessAccept = &counters.lastAccessAccept
	sysObj.Console.State.Counters.LastAccessReject = &counters.lastAccessReject

	return nil
}

var Subscribe_console_counters_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	log.V(0).Infof("Subscribe_console_counters_xfmr:%s", inParams.requestURI)

	return XfmrSubscOutParams{
		dbDataMap: RedisDbSubscribeMap{
			db.StateDB: {"CREDENTIALS": {"CONSOLE_METRICS": {}}}},
		onChange: OnchangeEnable,
		nOpts:    &notificationOpts{mInterval: 0, pType: OnChange},
	}, nil
}

var Subscribe_sys_memory_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	result := XfmrSubscOutParams{
		dbDataMap: RedisDbSubscribeMap{
			db.StateDB: {"MEMORY_STATS": {"system": {}}}},
		onChange: OnchangeDisable,
	}
	log.V(lvl.DEBUG).Infof("Subscribe_sys_memory_xfmr: %s", inParams.requestURI)

	return result, nil
}

func getSysMemFromDb(d *db.DB) (*SysMem, error) {
	var err error

	memInfo := SysMem{}
	memEntry, err := d.GetEntry(&db.TableSpec{Name: MEMORY_TBL}, db.Key{Comp: []string{systemKey}})
	if err != nil {
		log.V(lvl.DEBUG).Infof("Can't get entry with key: %v err: %v", systemKey, err)
		return &memInfo, err
	}
	var errs []string
	if memInfo.Total, err = strconv.ParseUint(memEntry.Get("total"), 10, 64); err != nil {
		msg := fmt.Sprintf("total: %s", err.Error())
		log.V(lvl.DEBUG).Info(msg)
		errs = append(errs, msg)
	}
	if memInfo.Used, err = strconv.ParseUint(memEntry.Get("used"), 10, 64); err != nil {
		msg := fmt.Sprintf("used: %s", err.Error())
		log.V(lvl.DEBUG).Info(msg)
		errs = append(errs, msg)
	}
	if memInfo.Free, err = strconv.ParseUint(memEntry.Get("free"), 10, 64); err != nil {
		msg := fmt.Sprintf("free: %s", err.Error())
		log.V(lvl.DEBUG).Info(msg)
		errs = append(errs, msg)
	}
	if memInfo.TotalEccErrors, err = strconv.ParseUint(memEntry.Get("total-ecc-errors"), 10, 64); err != nil {
		msg := fmt.Sprintf("total-ecc-errors: %s", err.Error())
		log.V(lvl.DEBUG).Info(msg)
		errs = append(errs, msg)
	}
	if memInfo.CorrectableEccErrors, err = strconv.ParseUint(memEntry.Get("correctable-ecc-errors"), 10, 64); err != nil {
		msg := fmt.Sprintf("correctable-ecc-errors: %s", err.Error())
		log.V(lvl.DEBUG).Info(msg)
		errs = append(errs, msg)
	}
	if memInfo.UncorrectableEccErrors, err = strconv.ParseUint(memEntry.Get("uncorrectable-ecc-errors"), 10, 64); err != nil {
		msg := fmt.Sprintf("uncorrectable-ecc-errors: %s", err.Error())
		log.V(lvl.DEBUG).Info(msg)
		errs = append(errs, msg)
	}
	err = nil
	if len(errs) > 0 {
		err = fmt.Errorf(strings.Join(errs, "; "))
	}
	return &memInfo, err
}

func getSystemMemory(meminfo *SysMem, sysmem *ocbinds.OpenconfigSystem_System_Memory_State) {
	log.V(lvl.DEBUG).Infof("getSystemMemory Entry")
	sysmem.Physical = &meminfo.Total
	sysmem.Reserved = &meminfo.Used
	sysmem.Used = &meminfo.Used
	sysmem.Free = &meminfo.Free
	sysmem.Counters.TotalEccErrors = &meminfo.TotalEccErrors
	sysmem.Counters.CorrectableEccErrors = &meminfo.CorrectableEccErrors
	sysmem.Counters.UncorrectableEccErrors = &meminfo.UncorrectableEccErrors
}

var DbToYang_sys_memory_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	var err error
	log.V(lvl.DEBUG).Infof("DbToYang_sys_memory_xfmr called")
	sysObj := getAppRootObject(inParams)
	meminfo, err := getSysMemFromDb(inParams.dbs[db.StateDB])
	if err != nil {
		log.V(lvl.DEBUG).Infof("getSysMemFromDb failed")
		return err
	}
	ygot.BuildEmptyTree(sysObj)
	if sysObj.Memory == nil {
		ygot.BuildEmptyTree(sysObj.Memory)
	}
	sysObj.Memory.State = &ocbinds.OpenconfigSystem_System_Memory_State{}
	sysObj.Memory.State.Counters = &ocbinds.OpenconfigSystem_System_Memory_State_Counters{}
	getSystemMemory(meminfo, sysObj.Memory.State)
	return err
}

func getSystemCpu(idx int, cpuCnt int, cpu Cpu, syscpu *ocbinds.OpenconfigSystem_System_Cpus_Cpu) {
	log.V(lvl.DEBUG).Info("getSystemCpu Entry idx ", idx)

	sysinfo := syscall.Sysinfo_t{}
	sys_err := syscall.Sysinfo(&sysinfo)
	if sys_err != nil {
		log.V(lvl.DEBUG).Infof("syscall.Sysinfo failed.")
	}
	var cpucur CpuState
	if idx == 0 && cpuCnt > 0 {
		cpucur.user = uint8((cpu.User / int64(cpuCnt)) / int64(sysinfo.Uptime))
		cpucur.system = uint8((cpu.System / int64(cpuCnt)) / int64(sysinfo.Uptime))
		cpucur.idle = uint8((cpu.Idle / int64(cpuCnt)) / int64(sysinfo.Uptime))
	} else {
		cpucur.user = uint8(cpu.User / int64(sysinfo.Uptime))
		cpucur.system = uint8(cpu.System / int64(sysinfo.Uptime))
		cpucur.idle = uint8(cpu.Idle / int64(sysinfo.Uptime))
	}

	ygot.BuildEmptyTree(syscpu.State)
	syscpu.State.User.Instant = &cpucur.user
	syscpu.State.Kernel.Instant = &cpucur.system
	syscpu.State.Idle.Instant = &cpucur.idle
	syscpu.State.Total.Avg = &cpu.Total.avg
	syscpu.State.Total.Interval = &cpu.Total.interval
}

func getSystemCpus(cpus map[int]*Cpu, syscpus *ocbinds.OpenconfigSystem_System_Cpus) {
	log.V(lvl.DEBUG).Info("getSystemCpus Entry")

	sysinfo := syscall.Sysinfo_t{}
	sys_err := syscall.Sysinfo(&sysinfo)
	cpuCnt := len(cpus) - 1
	if sys_err != nil {
		log.V(lvl.DEBUG).Info("syscall.Sysinfo failed.")
	}

	for idx, cpu := range cpus {
		var index ocbinds.OpenconfigSystem_System_Cpus_Cpu_State_Index_Union_Uint32
		index.Uint32 = uint32(idx)
		syscpu, err := syscpus.NewCpu(&index)
		if err != nil {
			log.V(lvl.DEBUG).Info("syscpus.NewCpu failed")
			return
		}
		ygot.BuildEmptyTree(syscpu)
		syscpu.Index = &index
		getSystemCpu(idx, cpuCnt, *cpu, syscpu)
	}
}

func getCpusFromDb(d *db.DB) (map[int]*Cpu, error) {
	var err error

	cpuTbl, err := d.GetTable(&db.TableSpec{Name: CPU_TBL})
	if err != nil {
		log.V(lvl.DEBUG).Info("Can't get table: ", CPU_TBL)
		return nil, err
	}

	keys, err := cpuTbl.GetKeys()
	if err != nil {
		log.V(lvl.DEBUG).Info("Can't get CPU keys from table")
		return nil, err
	}

	cpus := make(map[int]*Cpu)
	for _, key := range keys {
		if len(key.Comp) == 0 {
			continue
		}
		idx, err := strconv.Atoi(key.Comp[0])
		if err != nil {
			log.V(lvl.DEBUG).Infof("Invalid CPU stat key: %v", key)
			continue
		}
		cpuEntry, err := cpuTbl.GetEntry(key)
		if err != nil {
			log.V(lvl.DEBUG).Infof("Can't get entry with key: %v", key)
			return nil, err
		}

		cpu := &Cpu{}
		interval, err := strconv.ParseFloat(cpuEntry.Get("total_interval"), 64)
		if err != nil {
			log.V(lvl.DEBUG).Infof("Invalid or empty Total.interval for cpu-%d: %v", idx, err)
		}
		cpu.Total.interval = uint64(interval)
		avg, err := strconv.ParseFloat(cpuEntry.Get("total_avg"), 64)
		if err != nil {
			log.V(lvl.DEBUG).Infof("Invalid or empty Total.avg for cpu-%d: %v", idx, err)
		}
		cpu.Total.avg = uint8(avg)
		cpus[idx] = cpu
	}

	return cpus, err
}

var DbToYang_sys_cpus_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	var err error

	sysObj := getAppRootObject(inParams)

	cpus, err := getCpusFromDb(inParams.dbs[db.StateDB])
	if err != nil {
		log.V(lvl.DEBUG).Info("getCpusFromDb failed")
		return err
	}
	if sysObj.Cpus == nil {
		ygot.BuildEmptyTree(sysObj)
	}

	path := NewPathInfo(inParams.uri)
	if val, ok := path.Vars["index"]; ok {
		idx, err := strconv.Atoi(val)
		if err != nil {
			log.V(lvl.DEBUG).Info("Invalid cpu index ", val)
			return err
		}
		totalCpu := len(cpus)
		if cpu, ok := cpus[idx]; ok {
			//Since key(a pointer) is unknown, there is no way to do a lookup. So looping through a map with only one entry
			for _, value := range sysObj.Cpus.Cpu {
				ygot.BuildEmptyTree(value)
				getSystemCpu(idx, totalCpu-1, *cpu, value)
			}
		} else {
			log.V(lvl.DEBUG).Info("Cpu id: ", cpu, "is invalid, max is ", totalCpu)
		}
	} else {
		getSystemCpus(cpus, sysObj.Cpus)
	}
	return err
}

var Subscribe_sys_cpus_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	key := NewPathInfo(inParams.uri).Var("index")
	log.V(lvl.DEBUG).Infof("+++ Subscribe_sys_cpus_xfmr (%v) +++", inParams.uri)
	if key == "" {
		if inParams.subscProc != TRANSLATE_SUBSCRIBE {
			/* no need to verify dB data if we are requesting ALL cpus */
			return XfmrSubscOutParams{isVirtualTbl: true}, nil
		}
		key = "*"
	}
	return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.StateDB: {CPU_TBL: {key: {}}}}}, nil
}

func getSystemProcess(proc *Proc, sysproc *ocbinds.OpenconfigSystem_System_Processes_Process, pid uint64) {

	var procstate ProcessState

	ygot.BuildEmptyTree(sysproc)
	procstate.CpuUsageUser = uint64(proc.User * 1000000000)     // ns
	procstate.CpuUsageSystem = uint64(proc.System * 1000000000) // ns
	procstate.MemoryLimit = proc.MemLimit                       // The memory available to the container the process is running in.
	procstate.MemoryUsage = proc.Mem
	procstate.MemoryUtilization = uint8(proc.Memutil)
	procstate.CpuUtilization = uint8(proc.Cputil)
	procstate.Name = proc.Cmd
	procstate.Pid = pid
	procstate.StartTime = proc.Start // ns

	sysproc.Pid = &procstate.Pid
	sysproc.State.CpuUsageSystem = &procstate.CpuUsageSystem
	sysproc.State.CpuUsageUser = &procstate.CpuUsageUser
	sysproc.State.CpuUtilization = &procstate.CpuUtilization
	// NOTE: cpn's openconfig-procmon has no process/state/memory-limit leaf, so it is not mapped here.
	sysproc.State.MemoryUsage = &procstate.MemoryUsage
	sysproc.State.MemoryUtilization = &procstate.MemoryUtilization
	sysproc.State.Name = &procstate.Name
	sysproc.State.Pid = &procstate.Pid
	sysproc.State.StartTime = &procstate.StartTime
}

func getSystemProcesses(procs *map[string]Proc, sysprocs *ocbinds.OpenconfigSystem_System_Processes, pid uint64) (err error) {
	log.V(lvl.DEBUG).Infof("getSystemProcesses Entry")

	if pid != 0 {
		proc := (*procs)[strconv.Itoa(int(pid))]
		sysproc, ok := sysprocs.Process[pid]
		if !ok || sysproc == nil {
			sysproc, err = sysprocs.NewProcess(pid)
			if err != nil {
				return errors.New("sysprocs.NewProcess failed: " + err.Error())
			}
		}

		getSystemProcess(&proc, sysproc, pid)
	} else {

		for pidstr, proc := range *procs {
			idx, _ := strconv.Atoi(pidstr)
			sysproc, err := sysprocs.NewProcess(uint64(idx))
			if err != nil {
				return errors.New("sysprocs.NewProcess failed: " + err.Error())
			}

			getSystemProcess(&proc, sysproc, uint64(idx))
		}
	}
	return nil
}

func getProcsFromDb(d *db.DB) (map[string]Proc, error) {
	var err error
	var procs map[string]Proc
	var curProc Proc

	procTbl, err := d.GetTable(&db.TableSpec{Name: PROC_TBL})
	if err != nil {
		log.V(lvl.INFO).Infof("Can't get %v table: %v", PROC_TBL, err)
		return procs, err
	}

	keys, err := procTbl.GetKeys()
	if err != nil {
		log.V(lvl.DEBUG).Info("Can't get proc keys from table")
		return procs, err
	}

	memEntry, err := d.GetEntry(&db.TableSpec{Name: MEMORY_TBL}, db.Key{Comp: []string{systemKey}})
	if err != nil {
		log.V(lvl.DEBUG).Info("Can't get entry with key: ", systemKey)
		return nil, err
	}
	totalMem, _ := strconv.ParseUint(memEntry.Get("total"), 10, 64)

	procs = make(map[string]Proc)
	for _, key := range keys {
		pidstr := key.Get(0)
		procEntry, err := procTbl.GetEntry(key)
		if err != nil {
			log.V(lvl.DEBUG).Info("Can't get entry with key: ", pidstr)
			return procs, err
		}

		curProc.Cmd = procEntry.Get("CMD")
		if curProc.Cmd == "" {
			log.V(lvl.DEBUG).Infof("CMD empty for pid=%s, ignoring entry", key)
			continue
		}
		if t, err := time.ParseInLocation("Jan 2 2006 15:04:05 MST", procEntry.Get("STIME"), time.Local); err == nil {
			// "The timeticks64 represents the time, modulo 2^64 in nanoseconds
			//  between two epochs. The leaf using this type must define
			//  the epochs that tests are relative to."
			// It is converted to local time-zone to be consistent with 'boot-time'.
			curProc.Start = uint64(t.Local().UnixNano())
		} else {
			log.V(lvl.DEBUG).Infof("Invalid or empty STIME for process - %s: %v", curProc.Cmd, err)
		}
		if curProc.User, err = strconv.ParseFloat(procEntry.Get("USER_TIME"), 64); err != nil {
			log.V(lvl.DEBUG).Infof("Invalid or empty User for process - %v: %v", curProc.Cmd, err)
		}
		if curProc.System, err = strconv.ParseFloat(procEntry.Get("SYS_TIME"), 64); err != nil {
			log.V(lvl.DEBUG).Infof("Invalid or empty System for process - %v: %v", curProc.Cmd, err)
		}
		/* For memory-usage, commenting out the lookup for attribute "VSZ" to align
		 * with SONiC implementation. Instead, using the %MEM attribute along with
		 * the total memory to calculate the memory-usage. */
		// curProc.Mem, _ = strconv.ParseUint(procEntry.Get("VSZ"), 10, 64) * 1024
		if curProc.Cputil, err = strconv.ParseFloat(procEntry.Get("%CPU"), 64); err != nil {
			log.V(lvl.DEBUG).Infof("Invalid or empty Cputil for process - %v: %v", curProc.Cmd, err)
		}
		if curProc.Memutil, err = strconv.ParseFloat(procEntry.Get("%MEM"), 64); err != nil {
			log.V(lvl.DEBUG).Infof("Invalid or empty Memutil for process - %v: %v", curProc.Cmd, err)
		}
		if memLimit, err := strconv.ParseUint(procEntry.Get("MEM_LIMIT"), 10, 64); err != nil {
			log.V(lvl.DEBUG).Infof("Invalid or empty MemLimit for process - %v: %v", curProc.Cmd, err)
			curProc.MemLimit = nil
		} else {
			curProc.MemLimit = &memLimit
		}
		log.V(lvl.DEBUG).Infof("curProc.MemLimit: %v", curProc.MemLimit)
		curProc.Mem = uint64((curProc.Memutil / 100) * float64(totalMem))
		procs[pidstr] = curProc
	}

	/* Delete the one non-pid key procdockerstatsd deamon uses to store the last
	 * update time in the PROCCESSSTATS table */
	delete(procs, "LastUpdateTime")

	return procs, err
}

var DbToYang_sys_procs_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	var err error

	sysObj := getAppRootObject(inParams)

	procs, err := getProcsFromDb(inParams.dbs[db.StateDB])
	if err != nil {
		log.V(lvl.DEBUG).Infof("getProcsFromDb failed")
		return err
	}

	ygot.BuildEmptyTree(sysObj)
	path := NewPathInfo(inParams.uri)
	val := path.Vars["pid"]
	pid := 0
	if len(val) != 0 {
		pid, _ = strconv.Atoi(val)
	}
	return getSystemProcesses(&procs, sysObj.Processes, uint64(pid))
}

var Subscribe_sys_procs_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	if key := NewPathInfo(inParams.uri).Var("pid"); key != "" {
		return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.StateDB: {PROC_TBL: {key: {}}}}}, nil
	}
	/* no need to verify dB data if we are requesting ALL processes */
	return XfmrSubscOutParams{isVirtualTbl: true}, nil
}

func YangToDb_syslog_server_ip_fld_xfmr(inParams XfmrParams) (map[string]string, error) {
	return make(map[string]string), nil
}

func DbToYang_syslog_server_ip_fld_xfmr(inParams XfmrParams) (map[string]interface{}, error) {
	log.V(lvl.DEBUG).Infof("DbToYang_syslog_server_ip_fld_xfmr: key=\"%s\"", inParams.key)
	result := make(map[string]interface{})
	if len((*inParams.dbDataMap)[inParams.curDb]) > 0 {
		result["host"] = inParams.key
	}
	return result, nil
}

var DbToYangPath_remote_server_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	rootPath := "/openconfig-system:system/logging/remote-servers/remote-server/host"

	log.V(lvl.DEBUG).Infof("DbToYangPath_remote_server_path_xfmr: inParams: %#v", inParams)
	if len(inParams.tblKeyComp) != 1 {
		return fmt.Errorf("Invalid tblKeyCom for DbToYangPath_remote_server_path_xmfr:%v", inParams.tblKeyComp)
	}
	if LOGGING_TBL != inParams.tblName {
		return nil
	}
	inParams.ygPathKeys[rootPath] = inParams.tblKeyComp[0]
	log.V(lvl.DEBUG).Info("DbToYangPath_remote_server_path_xfmr: returned params.ygPathKeys: ", inParams.ygPathKeys)
	return nil
}
