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
)

// AAA authentication-method support maps the OpenConfig
// /system/aaa/authentication/config/authentication-method leaf-list onto SONiC's
// AAA|authentication:login field (a comma-separated, ordered method list).
//
// This is what actually activates a remote AAA method (e.g. TACACS+): hostcfgd
// wires PAM/nsswitch based on AAA|authentication:login containing "tacacs+".
// Configuring TACPLUS_SERVER alone is not sufficient.
//
// Method translation (OpenConfig <-> SONiC):
//
//	AAA_METHOD_TYPE LOCAL       <-> "local"
//	AAA_METHOD_TYPE TACACS_ALL  <-> "tacacs+"
//	AAA_METHOD_TYPE RADIUS_ALL  <-> "radius"
//	string "ldap"/"default"     <-> "ldap"/"default" (no matching identity)
const (
	AAA_TABLE_NAME         = "AAA"
	AAA_AUTHENTICATION_KEY = "authentication"
	AAA_LOGIN_FIELD        = "login"
)

func init() {
	XlateFuncBind("DbToYang_aaa_authentication_xfmr", DbToYang_aaa_authentication_xfmr)
	XlateFuncBind("YangToDb_aaa_authentication_xfmr", YangToDb_aaa_authentication_xfmr)
	XlateFuncBind("Subscribe_aaa_authentication_xfmr", Subscribe_aaa_authentication_xfmr)
}

// normalizeAaaMethodString maps a free-form authentication-method string to the
// SONiC login token. Unknown values are lower-cased and passed through so CVL's
// pattern check can reject genuinely invalid input.
func normalizeAaaMethodString(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "tacacs", "tacacs+", "tacacs_all", "tacacs-all":
		return "tacacs+"
	case "radius", "radius_all", "radius-all":
		return "radius"
	case "local":
		return "local"
	case "ldap":
		return "ldap"
	case "default":
		return "default"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

// aaaConfigMethodToSonic converts one OpenConfig config authentication-method
// union element to a SONiC login token ("" to skip).
func aaaConfigMethodToSonic(m ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union) string {
	switch v := m.(type) {
	case *ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE:
		switch v.E_OpenconfigAaaTypes_AAA_METHOD_TYPE {
		case ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_LOCAL:
			return "local"
		case ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_TACACS_ALL:
			return "tacacs+"
		case ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_RADIUS_ALL:
			return "radius"
		}
	case *ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_String:
		return normalizeAaaMethodString(v.String)
	}
	return ""
}

// sonicLoginToConfigUnion converts one SONiC login token to an OpenConfig config
// authentication-method union element.
func sonicLoginToConfigUnion(tok string) ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union {
	switch tok {
	case "local":
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE{E_OpenconfigAaaTypes_AAA_METHOD_TYPE: ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_LOCAL}
	case "tacacs+":
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE{E_OpenconfigAaaTypes_AAA_METHOD_TYPE: ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_TACACS_ALL}
	case "radius":
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE{E_OpenconfigAaaTypes_AAA_METHOD_TYPE: ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_RADIUS_ALL}
	case "":
		return nil
	default:
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union_String{String: tok}
	}
}

// sonicLoginToStateUnion is the state-tree counterpart of sonicLoginToConfigUnion.
func sonicLoginToStateUnion(tok string) ocbinds.OpenconfigSystem_System_Aaa_Authentication_State_AuthenticationMethod_Union {
	switch tok {
	case "local":
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_State_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE{E_OpenconfigAaaTypes_AAA_METHOD_TYPE: ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_LOCAL}
	case "tacacs+":
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_State_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE{E_OpenconfigAaaTypes_AAA_METHOD_TYPE: ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_TACACS_ALL}
	case "radius":
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_State_AuthenticationMethod_Union_E_OpenconfigAaaTypes_AAA_METHOD_TYPE{E_OpenconfigAaaTypes_AAA_METHOD_TYPE: ocbinds.OpenconfigAaaTypes_AAA_METHOD_TYPE_RADIUS_ALL}
	case "":
		return nil
	default:
		return &ocbinds.OpenconfigSystem_System_Aaa_Authentication_State_AuthenticationMethod_Union_String{String: tok}
	}
}

// YangToDb_aaa_authentication_xfmr handles SET/REPLACE/UPDATE/DELETE on the AAA
// authentication method list.
var YangToDb_aaa_authentication_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	log.V(lvl.DEBUG).Infof("Inside YangToDb_aaa_authentication_xfmr: op=%v, uri=%s", inParams.oper, inParams.uri)
	sysObj := getAppRootObject(inParams)

	aaaOps := make(map[string]db.Value)
	dbMutations := map[string]map[string]db.Value{
		AAA_TABLE_NAME: aaaOps,
	}

	// DELETE resets authentication to the default (local). Only the "login" field
	// is removed so that any other AAA fields (failthrough/fallback/debug/trace,
	// which are not reachable over this OpenConfig path) are preserved.
	if inParams.oper == DELETE {
		log.V(lvl.DEBUG).Infof("DELETE AAA authentication method")
		aaaOps[AAA_AUTHENTICATION_KEY] = db.Value{Field: map[string]string{AAA_LOGIN_FIELD: ""}}
		return dbMutations, nil
	}

	if inParams.oper == UPDATE || inParams.oper == REPLACE {
		var methods []ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config_AuthenticationMethod_Union
		if sysObj != nil && sysObj.Aaa != nil && sysObj.Aaa.Authentication != nil && sysObj.Aaa.Authentication.Config != nil {
			methods = sysObj.Aaa.Authentication.Config.AuthenticationMethod
		}

		tokens := make([]string, 0, len(methods))
		for _, m := range methods {
			if tok := aaaConfigMethodToSonic(m); tok != "" {
				tokens = append(tokens, tok)
			}
		}

		if len(tokens) == 0 {
			// REPLACE with empty payload clears the method (reset to default).
			if inParams.oper == REPLACE {
				aaaOps[AAA_AUTHENTICATION_KEY] = db.Value{}
			}
			return dbMutations, nil
		}

		aaaOps[AAA_AUTHENTICATION_KEY] = db.Value{Field: map[string]string{
			AAA_LOGIN_FIELD: strings.Join(tokens, ","),
		}}
		return dbMutations, nil
	}

	return nil, tlerr.NotSupportedError{Format: fmt.Sprintf("Unsupported operation %v on %s", inParams.oper, inParams.uri)}
}

// DbToYang_aaa_authentication_xfmr handles GET on the AAA authentication method.
var DbToYang_aaa_authentication_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("Inside DbToYang_aaa_authentication_xfmr URI: %s", inParams.uri)
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
	if deviceObj.System.Aaa.Authentication == nil {
		deviceObj.System.Aaa.Authentication = &ocbinds.OpenconfigSystem_System_Aaa_Authentication{}
	}
	authObj := deviceObj.System.Aaa.Authentication
	if authObj.Config == nil {
		authObj.Config = &ocbinds.OpenconfigSystem_System_Aaa_Authentication_Config{}
	}
	if authObj.State == nil {
		authObj.State = &ocbinds.OpenconfigSystem_System_Aaa_Authentication_State{}
	}

	login := ""
	if entry, err := dbGetEntry(cfgDb, &db.TableSpec{Name: AAA_TABLE_NAME}, db.Key{Comp: []string{AAA_AUTHENTICATION_KEY}}); err == nil {
		login = entry.Get(AAA_LOGIN_FIELD)
	}
	if login == "" {
		return nil
	}

	for _, tok := range strings.Split(login, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if u := sonicLoginToConfigUnion(tok); u != nil {
			authObj.Config.AuthenticationMethod = append(authObj.Config.AuthenticationMethod, u)
		}
		if u := sonicLoginToStateUnion(tok); u != nil {
			authObj.State.AuthenticationMethod = append(authObj.State.AuthenticationMethod, u)
		}
	}

	return nil
}

// Subscribe_aaa_authentication_xfmr maps the subtree to AAA|authentication.
var Subscribe_aaa_authentication_xfmr = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	var result XfmrSubscOutParams
	result.dbDataMap = make(RedisDbSubscribeMap)

	log.V(lvl.DEBUG).Infof("Subscribe_aaa_authentication_xfmr: path=%v proc=%v", inParams.uri, inParams.subscProc)

	result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{
		AAA_TABLE_NAME: {AAA_AUTHENTICATION_KEY: {}},
	}

	return result, nil
}
