package transformer

import (
	"errors"
	"fmt"
	"regexp"
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
	SYSLOG_SERVER_TABLE_NAME    = "SYSLOG_SERVER"
	SYSLOG_SERVERS_PATH         = "/openconfig-system:system/logging/remote-servers"
	SYSLOG_SERVER_PATH          = "/openconfig-system:system/logging/remote-servers/remote-server"
	syslogServerKeyField        = "host"
	SYSLOG_FIELD_PORT           = "port"
	SYSLOG_FIELD_PROTOCOL       = "protocol"
	SYSLOG_FIELD_TRANSPORT_SEC  = "transport_security"
	SYSLOG_FIELD_TLS_PROFILE_ID = "tls_profile_id"
	SYSLOG_FIELD_SOURCE         = "source"
	SYSLOG_FIELD_VRF            = "vrf"
	SYSLOG_FIELD_SEVERITY       = "severity"
	SYSLOG_DEFAULT_PORT         = 514
	SYSLOG_DEFAULT_TLS_PORT     = 6514
	SYSLOG_DEFAULT_PROFILE      = "gnxi"
)

var syslogTlsProfileIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

var ocSyslogSeverityToDb = map[ocbinds.E_OpenconfigSystemLogging_SyslogSeverity]string{
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_EMERGENCY:     "crit",
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_ALERT:         "crit",
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_CRITICAL:      "crit",
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_ERROR:         "error",
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_WARNING:       "warn",
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_NOTICE:        "notice",
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_INFORMATIONAL: "info",
	ocbinds.OpenconfigSystemLogging_SyslogSeverity_DEBUG:         "debug",
}

var ocSeverityNameToDb = map[string]string{
	"EMERGENCY":     "crit",
	"ALERT":         "crit",
	"CRITICAL":      "crit",
	"ERROR":         "error",
	"WARNING":       "warn",
	"NOTICE":        "notice",
	"INFORMATIONAL": "info",
	"DEBUG":         "debug",
}

var dbSyslogSeverityToOc = map[string]ocbinds.E_OpenconfigSystemLogging_SyslogSeverity{
	"crit":    ocbinds.OpenconfigSystemLogging_SyslogSeverity_CRITICAL,
	"error":   ocbinds.OpenconfigSystemLogging_SyslogSeverity_ERROR,
	"err":     ocbinds.OpenconfigSystemLogging_SyslogSeverity_ERROR,
	"warn":    ocbinds.OpenconfigSystemLogging_SyslogSeverity_WARNING,
	"warning": ocbinds.OpenconfigSystemLogging_SyslogSeverity_WARNING,
	"notice":  ocbinds.OpenconfigSystemLogging_SyslogSeverity_NOTICE,
	"info":    ocbinds.OpenconfigSystemLogging_SyslogSeverity_INFORMATIONAL,
	"debug":   ocbinds.OpenconfigSystemLogging_SyslogSeverity_DEBUG,
}

func init() {
	XlateFuncBind("YangToDb_remote_servers_list_xfmr", YangToDb_remote_servers_list_xfmr)
	XlateFuncBind("DbToYang_remote_servers_list_xfmr", DbToYang_remote_servers_list_xfmr)
	XlateFuncBind("Subscribe_remote_servers_list_xfmr", Subscribe_remote_servers_list_xfmr)
	XlateFuncBind("YangToDb_syslog_selector_key_xfmr", YangToDb_syslog_selector_key_xfmr)
	XlateFuncBind("DbToYang_syslog_selector_key_xfmr", DbToYang_syslog_selector_key_xfmr)
	XlateFuncBind("YangToDb_syslog_selector_severity_fld_xfmr", YangToDb_syslog_selector_severity_fld_xfmr)
	XlateFuncBind("DbToYang_syslog_selector_severity_fld_xfmr", DbToYang_syslog_selector_severity_fld_xfmr)
	XlateFuncBind("YangToDb_syslog_selector_facility_fld_xfmr", YangToDb_syslog_selector_facility_fld_xfmr)
}

func validateSyslogTlsProfile(transportSecurity bool, profileId *string) error {
	if transportSecurity {
		if profileId == nil || *profileId == "" {
			return tlerr.InvalidArgsError{
				Format: "tls-profile-id is required when transport-security is true",
			}
		}
	}
	if profileId != nil && *profileId != "" {
		if *profileId == SYSLOG_DEFAULT_PROFILE || *profileId == "." || *profileId == ".." ||
			!syslogTlsProfileIDRe.MatchString(*profileId) {
			return tlerr.InvalidArgsError{
				Format: fmt.Sprintf("tls-profile-id %s is not a valid syslog TLS profile", *profileId),
			}
		}
		if !transportSecurity {
			return tlerr.InvalidArgsError{
				Format: "transport-security is required when tls-profile-id is set",
			}
		}
	}
	return nil
}

func validateSyslogServerDbFields(fields map[string]string) error {
	transportSecurity := false
	if ts, ok := fields[SYSLOG_FIELD_TRANSPORT_SEC]; ok {
		if b, err := strconv.ParseBool(ts); err == nil {
			transportSecurity = b
		}
	}
	if fields[SYSLOG_FIELD_PROTOCOL] == "tls" {
		transportSecurity = true
	}
	var profileId *string
	if pid, ok := fields[SYSLOG_FIELD_TLS_PROFILE_ID]; ok && pid != "" {
		profileId = &pid
	}
	return validateSyslogTlsProfile(transportSecurity, profileId)
}

func ocSeverityToDb(severity ocbinds.E_OpenconfigSystemLogging_SyslogSeverity) (string, error) {
	if severity == ocbinds.OpenconfigSystemLogging_SyslogSeverity_UNSET {
		return "", nil
	}
	if dbSev, ok := ocSyslogSeverityToDb[severity]; ok {
		return dbSev, nil
	}
	return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported syslog severity %v", severity)}
}

func dbSeverityToOc(severity string) (ocbinds.E_OpenconfigSystemLogging_SyslogSeverity, bool) {
	ocSev, ok := dbSyslogSeverityToOc[strings.ToLower(severity)]
	return ocSev, ok
}

func validateSyslogFacilityFromURI(uri string) error {
	pathInfo := NewPathInfo(uri)
	if facility := pathInfo.Var("facility"); facility != "" && strings.ToUpper(facility) != "ALL" {
		return tlerr.NotSupportedError{
			Format: fmt.Sprintf("only ALL facility is supported for remote-server selectors, got %s", facility),
		}
	}
	return nil
}

func syslogSeverityValueFromParam(param interface{}) (string, error) {
	if param == nil {
		return "", tlerr.InvalidArgsError{Format: "missing syslog severity value"}
	}
	switch v := param.(type) {
	case ocbinds.E_OpenconfigSystemLogging_SyslogSeverity:
		return ocSeverityToDb(v)
	case string:
		if dbSev, ok := ocSeverityNameToDb[strings.ToUpper(v)]; ok {
			return dbSev, nil
		}
	case *string:
		if v != nil {
			if dbSev, ok := ocSeverityNameToDb[strings.ToUpper(*v)]; ok {
				return dbSev, nil
			}
		}
	}
	return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported syslog severity value %v", param)}
}

func validateSyslogFacilityFromParam(param interface{}) error {
	switch v := param.(type) {
	case ocbinds.E_OpenconfigSystemLogging_SYSLOG_FACILITY:
		if v == ocbinds.OpenconfigSystemLogging_SYSLOG_FACILITY_ALL {
			return nil
		}
	case string:
		if strings.ToUpper(v) == "ALL" {
			return nil
		}
	case *string:
		if v != nil && strings.ToUpper(*v) == "ALL" {
			return nil
		}
	}
	return tlerr.NotSupportedError{
		Format: "only ALL facility is supported for remote-server selectors",
	}
}

func syslogSeverityFromURI(uri string) (string, error) {
	if err := validateSyslogFacilityFromURI(uri); err != nil {
		return "", err
	}
	pathInfo := NewPathInfo(uri)
	severityName := strings.ToUpper(pathInfo.Var("severity"))
	if severityName == "" {
		return "", nil
	}
	if dbSev, ok := ocSeverityNameToDb[severityName]; ok {
		return dbSev, nil
	}
	return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("unsupported syslog severity %s", severityName)}
}

func applySelectorSeverityToFields(fields map[string]string, server *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer, uri string) error {
	severity := ""
	var err error
	if server != nil && server.Selectors != nil {
		severity, err = syslogSeverityFromSelectors(server.Selectors)
		if err != nil {
			return err
		}
	}
	if severity == "" {
		severity, err = syslogSeverityFromURI(uri)
		if err != nil {
			return err
		}
	}
	if severity != "" {
		fields[SYSLOG_FIELD_SEVERITY] = severity
	}
	return nil
}

func syslogSeverityFromSelectors(selectors *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Selectors) (string, error) {
	if selectors == nil || len(selectors.Selector) == 0 {
		return "", nil
	}
	if len(selectors.Selector) > 1 {
		return "", tlerr.NotSupportedError{
			Format: "only one selector per remote syslog server is supported",
		}
	}
	for _, selector := range selectors.Selector {
		if selector == nil || selector.Config == nil {
			continue
		}
		if selector.Config.Facility != ocbinds.OpenconfigSystemLogging_SYSLOG_FACILITY_UNSET &&
			selector.Config.Facility != ocbinds.OpenconfigSystemLogging_SYSLOG_FACILITY_ALL {
			return "", tlerr.NotSupportedError{
				Format: "only ALL facility is supported for remote-server selectors",
			}
		}
		return ocSeverityToDb(selector.Config.Severity)
	}
	return "", nil
}

func populateSyslogServerSelectors(serverObj *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer, dbSeverity string) error {
	ocSeverity, ok := dbSeverityToOc(dbSeverity)
	if !ok {
		return nil
	}
	if serverObj.Selectors == nil {
		serverObj.Selectors = &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Selectors{}
	}
	selectorKey := ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Selectors_Selector_Key{
		Facility: ocbinds.OpenconfigSystemLogging_SYSLOG_FACILITY_ALL,
		Severity: ocSeverity,
	}
	selector, err := serverObj.Selectors.NewSelector(selectorKey.Facility, selectorKey.Severity)
	if err != nil {
		if existing, ok := serverObj.Selectors.Selector[selectorKey]; ok {
			selector = existing
		} else {
			return err
		}
	}
	ygot.BuildEmptyTree(selector)
	if selector.Config == nil {
		selector.Config = &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Selectors_Selector_Config{}
	}
	if selector.State == nil {
		selector.State = &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Selectors_Selector_State{}
	}
	selector.Config.Facility = ocbinds.OpenconfigSystemLogging_SYSLOG_FACILITY_ALL
	selector.Config.Severity = ocSeverity
	selector.State.Facility = ocbinds.OpenconfigSystemLogging_SYSLOG_FACILITY_ALL
	selector.State.Severity = ocSeverity
	return nil
}

// MODIFIED: added source-address and network-instance field mapping (Group A).
func syslogServerConfigToDbFields(cfg *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config) map[string]string {
	fields := map[string]string{nullKey: nullKey}
	if cfg == nil {
		return fields
	}
	tls := cfg.TransportSecurity != nil && *cfg.TransportSecurity
	if cfg.RemotePort != nil {
		fields[SYSLOG_FIELD_PORT] = strconv.Itoa(int(*cfg.RemotePort))
	} else if tls {
		fields[SYSLOG_FIELD_PORT] = strconv.Itoa(SYSLOG_DEFAULT_TLS_PORT)
	} else {
		fields[SYSLOG_FIELD_PORT] = strconv.Itoa(SYSLOG_DEFAULT_PORT)
	}
	if cfg.TransportSecurity != nil {
		fields[SYSLOG_FIELD_TRANSPORT_SEC] = strconv.FormatBool(*cfg.TransportSecurity)
		if tls {
			fields[SYSLOG_FIELD_PROTOCOL] = "tls"
		} else {
			fields[SYSLOG_FIELD_PROTOCOL] = "udp"
		}
	} else {
		fields[SYSLOG_FIELD_PROTOCOL] = "udp"
	}
	if cfg.TlsProfileId != nil {
		fields[SYSLOG_FIELD_TLS_PROFILE_ID] = *cfg.TlsProfileId
	}
	if cfg.SourceAddress != nil {
		fields[SYSLOG_FIELD_SOURCE] = *cfg.SourceAddress
	}
	if cfg.NetworkInstance != nil {
		fields[SYSLOG_FIELD_VRF] = *cfg.NetworkInstance
	}
	return fields
}

// MODIFIED: added uri parameter, selector severity handling, and source/vrf validation (Group A/B).
func syslogServerDbValueFromYang(cfgDb *db.DB, host string, server *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer, mergeExisting bool, uri string) (db.Value, error) {
	fields := syslogServerConfigToDbFields(nil)
	if mergeExisting && cfgDb != nil {
		if entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: SYSLOG_SERVER_TABLE_NAME}, db.Key{Comp: []string{host}}); err == nil {
			for k, v := range entry.Field {
				fields[k] = v
			}
		}
	}
	if server != nil && server.Config != nil {
		for k, v := range syslogServerConfigToDbFields(server.Config) {
			fields[k] = v
		}
	}
	if err := applySelectorSeverityToFields(fields, server, uri); err != nil {
		return db.Value{}, err
	}
	if err := validateSyslogServerDbFields(fields); err != nil {
		return db.Value{}, err
	}
	if err := validateSyslogServerFields(cfgDb, host, fields); err != nil {
		return db.Value{}, err
	}
	return db.Value{Field: fields}, nil
}

// MODIFIED: added source-address, network-instance, and selector severity population (Group A/B).
func populateSyslogServerFromDbEntry(host string, entry db.Value) *ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer {
	serverObj := &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer{
		Host: ygot.String(host),
		Config: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_Config{
			Host: ygot.String(host),
		},
		State: &ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer_State{
			Host: ygot.String(host),
		},
	}
	if portStr, ok := entry.Field[SYSLOG_FIELD_PORT]; ok {
		if port, err := strconv.ParseUint(portStr, 10, 16); err == nil {
			p := uint16(port)
			serverObj.Config.RemotePort = &p
			serverObj.State.RemotePort = &p
		}
	}
	if proto, ok := entry.Field[SYSLOG_FIELD_PROTOCOL]; ok && proto == "tls" {
		ts := true
		serverObj.Config.TransportSecurity = &ts
		serverObj.State.TransportSecurity = &ts
	} else if tsStr, ok := entry.Field[SYSLOG_FIELD_TRANSPORT_SEC]; ok {
		if ts, err := strconv.ParseBool(tsStr); err == nil {
			serverObj.Config.TransportSecurity = &ts
			serverObj.State.TransportSecurity = &ts
		}
	}
	if profile, ok := entry.Field[SYSLOG_FIELD_TLS_PROFILE_ID]; ok {
		serverObj.Config.TlsProfileId = ygot.String(profile)
		serverObj.State.TlsProfileId = ygot.String(profile)
	}
	if source, ok := entry.Field[SYSLOG_FIELD_SOURCE]; ok {
		serverObj.Config.SourceAddress = ygot.String(source)
		serverObj.State.SourceAddress = ygot.String(source)
	}
	if vrf, ok := entry.Field[SYSLOG_FIELD_VRF]; ok {
		serverObj.Config.NetworkInstance = ygot.String(vrf)
		serverObj.State.NetworkInstance = ygot.String(vrf)
	}
	if severity, ok := entry.Field[SYSLOG_FIELD_SEVERITY]; ok {
		_ = populateSyslogServerSelectors(serverObj, severity)
	}
	return serverObj
}

var DbToYang_remote_servers_list_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_remote_servers_list_xfmr URI: %s, Key: %s", inParams.uri, inParams.key)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return errors.New("CONFIG_DB not found")
	}

	deviceObj := (*inParams.ygRoot).(*ocbinds.Device)
	if deviceObj.System == nil {
		deviceObj.System = &ocbinds.OpenconfigSystem_System{}
	}
	if deviceObj.System.Logging == nil {
		deviceObj.System.Logging = &ocbinds.OpenconfigSystem_System_Logging{}
	}
	if deviceObj.System.Logging.RemoteServers == nil {
		deviceObj.System.Logging.RemoteServers = &ocbinds.OpenconfigSystem_System_Logging_RemoteServers{}
	}
	remoteServersObj := deviceObj.System.Logging.RemoteServers

	targetKeys := make(map[string]bool)
	targetKey := keyFromInParamsOrUri(inParams, syslogServerKeyField)
	if targetKey != "" {
		if _, err := dbGetEntry(cfgDb, &db.TableSpec{Name: SYSLOG_SERVER_TABLE_NAME}, db.Key{Comp: []string{targetKey}}); err != nil {
			remoteServersObj.RemoteServer = make(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer)
			return tlerr.NotFoundError{Format: "Resource not found"}
		}
		targetKeys[targetKey] = true
	} else {
		keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: SYSLOG_SERVER_TABLE_NAME})
		if err != nil {
			return err
		}
		for _, key := range keys {
			if len(key.Comp) > 0 {
				targetKeys[key.Comp[0]] = true
			}
		}
	}

	newServerMap := make(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer)
	for host := range targetKeys {
		entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: SYSLOG_SERVER_TABLE_NAME}, db.Key{Comp: []string{host}})
		if err != nil {
			log.V(lvl.DEBUG).Infof("No CONFIG_DB entry for syslog server %s: %v", host, err)
			continue
		}
		newServerMap[host] = populateSyslogServerFromDbEntry(host, entry)
	}
	remoteServersObj.RemoteServer = newServerMap
	return nil
}

var Subscribe_remote_servers_list_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)
	pathInfo := NewPathInfo(inParams.uri)
	host := pathInfo.Var(syslogServerKeyField)
	if host == "" {
		host = "*"
	}
	log.V(lvl.DEBUG).Infof("Subscribe_remote_servers_list_xfmr: path=%v host=%v proc=%v",
		inParams.uri, host, inParams.subscProc)
	if host != "*" {
		result.isVirtualTbl = true
		return result, nil
	}
	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		SYSLOG_SERVER_TABLE_NAME: {host: {}},
	}
	return result, nil
}

// MODIFIED: added selector-path handling, invokeCRUSubtreeOnce, and uri forwarding (Group B).
var YangToDb_remote_servers_list_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	var err error
	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}
	log.V(lvl.DEBUG).Infof("Inside YangToDb_remote_servers_list_xfmr: op=%v, uri=%s",
		inParams.oper, inParams.uri)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		cfgDb, err = db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("CONFIG_DB not found for URI %s: %v", inParams.uri, err),
			}
		}
		defer cfgDb.DeleteDB()
	}
	sysObj := getAppRootObject(inParams)
	syslogTableOps := make(map[string]db.Value)
	dbMutations := map[string]map[string]db.Value{
		SYSLOG_SERVER_TABLE_NAME: syslogTableOps,
	}
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		return nil, tlerr.InternalError{
			Format: fmt.Sprintf("Failed to get YANG path from URI %s: %v", inParams.uri, err),
		}
	}
	targetKey := keyFromInParamsOrUri(inParams, syslogServerKeyField)
	isListOp := targetUriPath == SYSLOG_SERVERS_PATH ||
		(targetUriPath == SYSLOG_SERVER_PATH && targetKey == "")
	isServerOp := targetUriPath == SYSLOG_SERVER_PATH && targetKey != ""
	isSelectorOp := strings.Contains(targetUriPath, "/selectors/")
	isConfigOp := strings.HasSuffix(targetUriPath, "/config") ||
		strings.HasSuffix(targetUriPath, "/host")
	if inParams.oper == DELETE {
		if isListOp {
			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: SYSLOG_SERVER_TABLE_NAME})
			if err != nil {
				return nil, tlerr.InternalError{
					Format: fmt.Sprintf("failed to get keys for clear: %v", err),
				}
			}
			for _, key := range keys {
				syslogTableOps[key.Comp[0]] = db.Value{}
			}
		} else if targetKey != "" && isSelectorOp {
			entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: SYSLOG_SERVER_TABLE_NAME}, db.Key{Comp: []string{targetKey}})
			if err != nil {
				return nil, tlerr.NotFoundError{Format: "Resource not found"}
			}
			fields := map[string]string{}
			for k, v := range entry.Field {
				if k == SYSLOG_FIELD_SEVERITY {
					continue
				}
				fields[k] = v
			}
			if len(fields) == 0 {
				fields[nullKey] = nullKey
			}
			syslogTableOps[targetKey] = db.Value{Field: fields}
		} else if targetKey != "" && (isServerOp || isConfigOp) {
			syslogTableOps[targetKey] = db.Value{}
		} else {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("Invalid DELETE on %s, targetKey: %s", inParams.uri, targetKey),
			}
		}
		return dbMutations, nil
	}
	var newServersMap map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer
	if sysObj != nil && sysObj.Logging != nil && sysObj.Logging.RemoteServers != nil {
		newServersMap = sysObj.Logging.RemoteServers.RemoteServer
	} else {
		newServersMap = make(map[string]*ocbinds.OpenconfigSystem_System_Logging_RemoteServers_RemoteServer)
	}
	if inParams.oper == UPDATE || inParams.oper == REPLACE {
		mergeExisting := inParams.oper == UPDATE && !isListOp
		if isListOp {
			for host, server := range newServersMap {
				value, err := syslogServerDbValueFromYang(cfgDb, host, server, false, inParams.uri)
				if err != nil {
					return nil, err
				}
				syslogTableOps[host] = value
			}
		} else if targetKey != "" {
			if inParams.oper == REPLACE && isServerOp && newServersMap[targetKey] == nil {
				syslogTableOps[targetKey] = db.Value{}
			} else {
				server := newServersMap[targetKey]
				value, err := syslogServerDbValueFromYang(cfgDb, targetKey, server, mergeExisting, inParams.uri)
				if err != nil {
					return nil, err
				}
				syslogTableOps[targetKey] = value
			}
		} else {
			return nil, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("Missing host key for %v on %s", inParams.oper, inParams.uri),
			}
		}
		if inParams.oper == REPLACE && isListOp {
			keys, err := dbGetKeys(cfgDb, &db.TableSpec{Name: SYSLOG_SERVER_TABLE_NAME})
			if err != nil {
				return nil, err
			}
			keysToDelete := make(map[string]db.Value)
			for _, key := range keys {
				existingHost := key.Comp[0]
				if _, present := newServersMap[existingHost]; !present {
					keysToDelete[existingHost] = db.Value{}
				}
			}
			if len(keysToDelete) > 0 {
				subOpMap := map[db.DBNum]map[string]map[string]db.Value{
					db.ConfigDB: {
						SYSLOG_SERVER_TABLE_NAME: keysToDelete,
					},
				}
				updateSubOpDataMap(subOpMap, DELETE, inParams)
			}
		}
		return dbMutations, nil
	}
	return nil, tlerr.NotSupportedError{
		Format: fmt.Sprintf("Unsupported operation %v on %s", inParams.oper, inParams.uri),
	}
}

var YangToDb_syslog_selector_key_xfmr KeyXfmrYangToDb = func(inParams XfmrParams) (string, error) {
	return keyFromInParamsOrUri(inParams, syslogServerKeyField), nil
}

var DbToYang_syslog_selector_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	return make(map[string]interface{}), nil
}

var YangToDb_syslog_selector_severity_fld_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	if err := validateSyslogFacilityFromURI(inParams.uri); err != nil {
		return nil, err
	}
	dbSev, err := syslogSeverityValueFromParam(inParams.param)
	if err != nil {
		return nil, err
	}
	return map[string]string{SYSLOG_FIELD_SEVERITY: dbSev}, nil
}

var DbToYang_syslog_selector_severity_fld_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	data := (*inParams.dbDataMap)[inParams.curDb]
	serverData, ok := data[SYSLOG_SERVER_TABLE_NAME]
	if !ok {
		return result, nil
	}
	entry, ok := serverData[inParams.key]
	if !ok {
		return result, nil
	}
	severity, ok := entry.Field[SYSLOG_FIELD_SEVERITY]
	if !ok {
		return result, nil
	}
	ocSev, ok := dbSeverityToOc(severity)
	if !ok {
		return result, nil
	}
	result["severity"] = ocSev
	return result, nil
}

var YangToDb_syslog_selector_facility_fld_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	if err := validateSyslogFacilityFromURI(inParams.uri); err != nil {
		return nil, err
	}
	if err := validateSyslogFacilityFromParam(inParams.param); err != nil {
		return nil, err
	}
	return map[string]string{}, nil
}
