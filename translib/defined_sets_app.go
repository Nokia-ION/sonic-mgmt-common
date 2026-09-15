////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                     //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
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

package translib

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	OC_DEFINED_SETS_APP_MODULE_NAME = "/openconfig-defined-sets:defined-sets"

	// The only part of the model this app serves.
	OC_PORT_SETS_PATH = OC_DEFINED_SETS_APP_MODULE_NAME + "/port-sets"

	PORT_SET_TABLE      = "ACL_PORT_SET"
	RULE_PORT_SET_TABLE = "ACL_RULE_PORT_SET"

	PORT_SET_PORTS       = "PORTS"
	PORT_SET_DESCRIPTION = "DESCRIPTION"

	RULE_SRC_PORT_SET = "SRC_PORT_SET"
	RULE_DST_PORT_SET = "DST_PORT_SET"

	L4_SRC_PORT       = "L4_SRC_PORT"
	L4_SRC_PORT_RANGE = "L4_SRC_PORT_RANGE"
	L4_DST_PORT       = "L4_DST_PORT"
	L4_DST_PORT_RANGE = "L4_DST_PORT_RANGE"
)

// DefinedSetsApp serves the OpenConfig /defined-sets subtree. Only port sets
// are modelled, and a set has to resolve to the single L4 port match an
// ACL_RULE can carry; see openconfig-defined-sets.yang.
type DefinedSetsApp struct {
	pathInfo   *PathInfo
	ygotRoot   *ygot.GoStruct
	ygotTarget *interface{}
	appOptions

	portSetTs *db.TableSpec
	refTs     *db.TableSpec

	// port set name to its CONFIG_DB row, built from the request payload
	portSetMap map[string]db.Value
}

func init() {

	err := register(OC_DEFINED_SETS_APP_MODULE_NAME,
		&appInfo{appType: reflect.TypeOf(DefinedSetsApp{}),
			ygotRootType:  reflect.TypeOf(ocbinds.OpenconfigDefinedSets_DefinedSets{}),
			isNative:      false,
			tablesToWatch: []*db.TableSpec{{Name: PORT_SET_TABLE}}})

	if err != nil {
		log.Fatal("Register defined-sets app module with App Interface failed with error=", err)
	}

	err = addModel(&ModelData{Name: "openconfig-defined-sets",
		Org: "OpenConfig working group",
		Ver: "1.1.0"})
	if err != nil {
		log.Fatal("Adding model data to appinterface failed with error=", err)
	}
}

func (app *DefinedSetsApp) initialize(data appData) {
	log.Info("initialize:defined-sets:path =", data.path)
	*app = DefinedSetsApp{
		pathInfo:   NewPathInfo(data.path),
		ygotRoot:   data.ygotRoot,
		ygotTarget: data.ygotTarget,
		appOptions: data.appOptions,
	}

	app.portSetTs = &db.TableSpec{Name: PORT_SET_TABLE}
	app.refTs = &db.TableSpec{Name: RULE_PORT_SET_TABLE}
	app.portSetMap = make(map[string]db.Value)
}

func (app *DefinedSetsApp) getAppRootObject() *ocbinds.OpenconfigDefinedSets_DefinedSets {
	deviceObj := (*app.ygotRoot).(*ocbinds.Device)
	return deviceObj.DefinedSets
}

// checkSupportedPath turns away the parts of the model this app does not
// serve. openconfig-defined-sets also carries ipv4 and ipv6 prefix sets, and
// their list key is called "name" as well, so a prefix set request that got
// through would be read as a request for the port set of the same name.
func (app *DefinedSetsApp) checkSupportedPath() error {
	t := app.pathInfo.Template
	if isSubtreeRequest(t, OC_PORT_SETS_PATH) ||
		!isSubtreeRequest(t, OC_DEFINED_SETS_APP_MODULE_NAME+"/") {
		return nil
	}
	return tlerr.NotSupported("%s is not supported; only port sets are", t)
}

func (app *DefinedSetsApp) translateCreate(d *db.DB) ([]db.WatchKeys, error) {
	if err := app.checkSupportedPath(); err != nil {
		return nil, err
	}
	return nil, app.convertOCPortSetsToInternal()
}

func (app *DefinedSetsApp) translateUpdate(d *db.DB) ([]db.WatchKeys, error) {
	if err := app.checkSupportedPath(); err != nil {
		return nil, err
	}
	return nil, app.convertOCPortSetsToInternal()
}

func (app *DefinedSetsApp) translateReplace(d *db.DB) ([]db.WatchKeys, error) {
	if err := app.checkSupportedPath(); err != nil {
		return nil, err
	}
	return nil, app.convertOCPortSetsToInternal()
}

func (app *DefinedSetsApp) translateDelete(d *db.DB) ([]db.WatchKeys, error) {
	return nil, app.checkSupportedPath()
}

func (app *DefinedSetsApp) translateGet(dbs [db.MaxDB]*db.DB) error {
	return app.checkSupportedPath()
}

func (app *DefinedSetsApp) translateAction(dbs [db.MaxDB]*db.DB) error {
	return tlerr.NotSupported("unsupported")
}

func (app *DefinedSetsApp) translateSubscribe(req translateSubRequest) (translateSubResponse, error) {
	return emptySubscribeResponse(req.path)
}

func (app *DefinedSetsApp) processSubscribe(req processSubRequest) (processSubResponse, error) {
	return processSubResponse{}, tlerr.New("not implemented")
}

func (app *DefinedSetsApp) processCreate(d *db.DB) (SetResponse, error) {
	return app.setResponse(app.processWrite(d, CREATE))
}

func (app *DefinedSetsApp) processUpdate(d *db.DB) (SetResponse, error) {
	return app.setResponse(app.processWrite(d, UPDATE))
}

func (app *DefinedSetsApp) processReplace(d *db.DB) (SetResponse, error) {
	return app.setResponse(app.processWrite(d, REPLACE))
}

func (app *DefinedSetsApp) processDelete(d *db.DB) (SetResponse, error) {
	return app.setResponse(app.processDeletePortSets(d))
}

func (app *DefinedSetsApp) processAction(dbs [db.MaxDB]*db.DB) (ActionResponse, error) {
	return ActionResponse{}, tlerr.New("not implemented")
}

func (app *DefinedSetsApp) setResponse(err error) (SetResponse, error) {
	if err != nil {
		log.Error(err)
		return SetResponse{ErrSrc: AppErr}, err
	}
	return SetResponse{}, nil
}

func (app *DefinedSetsApp) processGet(dbs [db.MaxDB]*db.DB, fmtType TranslibFmtType) (GetResponse, error) {
	var payload []byte

	if err := app.processGetPortSets(dbs[db.ConfigDB]); err != nil {
		return GetResponse{Payload: payload, ErrSrc: AppErr}, err
	}

	if err := filterGetContent(app.pathInfo.Template, app.content, app.ygotRoot); err != nil {
		return GetResponse{Payload: payload, ErrSrc: AppErr}, err
	}

	return generateGetResponse(app.pathInfo.Path, app.ygotRoot, fmtType)
}

// convertOCPortSetsToInternal builds the CONFIG_DB rows for the port sets in the
// request payload. Everything a SONiC ACL rule cannot express is rejected here,
// before any row is written.
func (app *DefinedSetsApp) convertOCPortSetsToInternal() error {
	definedSets := app.getAppRootObject()
	if definedSets == nil || definedSets.PortSets == nil {
		return nil
	}

	for name, portSet := range definedSets.PortSets.PortSet {
		val := db.Value{Field: map[string]string{}}

		if portSet != nil && portSet.Config != nil {
			cfg := portSet.Config

			if cfg.Name != nil && *cfg.Name != name {
				return tlerr.InvalidArgs("Port set name %s does not match the list key %s", *cfg.Name, name)
			}

			if len(cfg.ExcludePort) > 0 {
				return tlerr.NotSupported(
					"exclude-port is not supported; a SONiC ACL rule cannot express a port exclusion")
			}

			if cfg.Description != nil && len(*cfg.Description) > 0 {
				val.Field[PORT_SET_DESCRIPTION] = *cfg.Description
			}

			if len(cfg.Port) > 1 {
				return tlerr.InvalidArgs(
					"Port set %s holds %d ports; an ACL rule can match only one port or one contiguous range",
					name, len(cfg.Port))
			}

			// An empty port leaf-list reaches us as no port at all, so a
			// request without ports leaves them alone on an update and clears
			// them on a replace, which is what the two operations mean anyway.
			for _, port := range cfg.Port {
				portVal, err := portNumRangeUnionToDb(port)
				if err != nil {
					return err
				}
				(&val).SetList(PORT_SET_PORTS, []string{portVal})
			}
		}

		app.portSetMap[name] = val
	}

	return nil
}

func (app *DefinedSetsApp) processWrite(d *db.DB, opcode int) error {
	log.Infof("processWrite:defined-sets:path = %s opcode = %d", app.pathInfo.Template, opcode)

	for name, val := range app.portSetMap {
		existing, _ := d.GetEntry(app.portSetTs, asKey(name))

		var err error
		switch {
		case !existing.IsPopulated():
			// CVL rejects an update against a key that does not exist yet.
			if opcode == UPDATE || opcode == REPLACE || opcode == CREATE {
				err = d.CreateEntry(app.portSetTs, asKey(name), val)
			}
		case opcode == CREATE:
			return tlerr.AlreadyExists("Port set %s already exists", name)
		case opcode == REPLACE:
			// A replace without ports empties the set, which leaves a referring
			// rule with a match that no longer has a source, the same as a
			// delete of the port leaf-list.
			if !val.Has(PORT_SET_PORTS+"@") && (&existing).Has(PORT_SET_PORTS+"@") {
				if err := app.checkPortSetUnused(d, name, "clear its ports"); err != nil {
					return err
				}
			}
			err = d.SetEntry(app.portSetTs, asKey(name), val)
		default:
			err = d.ModEntry(app.portSetTs, asKey(name), val)
		}
		if err != nil {
			return err
		}

		// A rule holds the port its set resolved to, not the set name, so the
		// rules matching on a set that just changed have to be rewritten.
		if existing.IsPopulated() {
			if err := reprogramPortSetUsers(d, name); err != nil {
				return err
			}
		}
	}

	return nil
}

func (app *DefinedSetsApp) processDeletePortSets(d *db.DB) error {
	log.Infof("processDeletePortSets:defined-sets:path = %s", app.pathInfo.Template)

	name := app.pathInfo.Var("name")
	if name == "" {
		// The whole defined-sets or port-sets subtree.
		keys, err := d.GetKeys(app.portSetTs)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := app.deletePortSet(d, key.Get(0)); err != nil {
				return err
			}
		}
		return nil
	}

	switch {
	case strings.HasSuffix(app.pathInfo.Template, "/description"):
		return app.deletePortSetFields(d, name, PORT_SET_DESCRIPTION)
	case strings.HasSuffix(app.pathInfo.Template, "/port"):
		// A referring rule would be left with a match that no longer has a
		// source.
		if err := app.checkPortSetUnused(d, name, "clear its ports"); err != nil {
			return err
		}
		return app.deletePortSetFields(d, name, PORT_SET_PORTS+"@")
	default:
		return app.deletePortSet(d, name)
	}
}

func (app *DefinedSetsApp) deletePortSet(d *db.DB, name string) error {
	if err := app.checkPortSetUnused(d, name, "delete it"); err != nil {
		return err
	}
	return d.DeleteEntry(app.portSetTs, asKey(name))
}

func (app *DefinedSetsApp) deletePortSetFields(d *db.DB, name string, fields ...string) error {
	val := db.Value{Field: map[string]string{}}
	for _, field := range fields {
		val.Field[field] = ""
	}
	return d.DeleteEntryFields(app.portSetTs, asKey(name), val)
}

// checkPortSetUnused fails when rules still match on the port set. Their match
// was resolved from the set, so it cannot be left pointing at nothing.
func (app *DefinedSetsApp) checkPortSetUnused(d *db.DB, name, action string) error {
	users, err := rulesUsingPortSet(d, name)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}

	rules := make([]string, 0, len(users))
	for _, key := range users {
		rules = append(rules, strings.Join(key.Comp, "|"))
	}

	return tlerr.InvalidArgs("Port set %s is in use by %s; stop matching on it before you %s",
		name, strings.Join(rules, ", "), action)
}

func (app *DefinedSetsApp) processGetPortSets(d *db.DB) error {
	definedSets := app.getAppRootObject()
	if definedSets == nil {
		return nil
	}
	ygot.BuildEmptyTree(definedSets)

	if name := app.pathInfo.Var("name"); name != "" {
		entry, err := d.GetEntry(app.portSetTs, asKey(name))
		if err != nil || !entry.IsPopulated() {
			return tlerr.NotFound("Port set %s not found", name)
		}
		return app.convertInternalToOCPortSet(definedSets, name, entry)
	}

	keys, err := d.GetKeys(app.portSetTs)
	if err != nil {
		return err
	}
	for _, key := range keys {
		entry, err := d.GetEntry(app.portSetTs, key)
		if err != nil {
			return err
		}
		if err := app.convertInternalToOCPortSet(definedSets, key.Get(0), entry); err != nil {
			return err
		}
	}

	return nil
}

func (app *DefinedSetsApp) convertInternalToOCPortSet(definedSets *ocbinds.OpenconfigDefinedSets_DefinedSets, name string, entry db.Value) error {
	portSet := definedSets.PortSets.PortSet[name]
	if portSet == nil {
		var err error
		if portSet, err = definedSets.PortSets.NewPortSet(name); err != nil {
			return err
		}
	}
	ygot.BuildEmptyTree(portSet)

	setName := name
	portSet.Name = &setName
	portSet.Config.Name = &setName
	portSet.State.Name = &setName

	if descr := (&entry).Get(PORT_SET_DESCRIPTION); len(descr) > 0 {
		portSet.Config.Description = &descr
		portSet.State.Description = &descr
	}

	for _, port := range (&entry).GetList(PORT_SET_PORTS) {
		ocPort := portNumRangeToOc(port)

		cfgPort, err := portSet.Config.To_OpenconfigDefinedSets_DefinedSets_PortSets_PortSet_Config_Port_Union(ocPort)
		if err != nil {
			return err
		}
		portSet.Config.Port = append(portSet.Config.Port, cfgPort)

		statePort, err := portSet.State.To_OpenconfigDefinedSets_DefinedSets_PortSets_PortSet_State_Port_Union(ocPort)
		if err != nil {
			return err
		}
		portSet.State.Port = append(portSet.State.Port, statePort)
	}

	return nil
}

// portNumRangeUnionToDb converts a port-num-range union value of a port set into
// the CONFIG_DB form.
func portNumRangeUnionToDb(port ocbinds.OpenconfigDefinedSets_DefinedSets_PortSets_PortSet_Config_Port_Union) (string, error) {
	switch v := port.(type) {
	case *ocbinds.OpenconfigDefinedSets_DefinedSets_PortSets_PortSet_Config_Port_Union_String:
		return portNumRangeToDb(v.String)
	case *ocbinds.OpenconfigDefinedSets_DefinedSets_PortSets_PortSet_Config_Port_Union_Uint16:
		return strconv.FormatUint(uint64(v.Uint16), 10), nil
	case *ocbinds.OpenconfigDefinedSets_DefinedSets_PortSets_PortSet_Config_Port_Union_E_OpenconfigDefinedSets_DefinedSets_PortSets_PortSet_Config_Port:
		return portNumRangeToDb("ANY")
	}

	return "", tlerr.InvalidArgs("Invalid port value %v", port)
}

// portNumRangeToOc renders a CONFIG_DB port match for the openconfig
// port-num-range union: a range keeps its "<lo>..<hi>" string form, a single
// port is a number.
func portNumRangeToOc(port string) interface{} {
	if strings.Contains(port, "-") {
		return portNumRangeFromDb(port)
	}
	if portNum, err := strconv.ParseUint(port, 10, 16); err == nil {
		return uint16(portNum)
	}
	return port
}

// portNumRangeToDb converts an openconfig port-num-range value into the form
// CONFIG_DB uses: a single port stays as it is, an inclusive range "<lo>..<hi>"
// becomes "<lo>-<hi>". ANY has no CONFIG_DB representation in a port set, since
// a set exists to narrow the match.
func portNumRangeToDb(port string) (string, error) {
	if port == "ANY" {
		return "", tlerr.InvalidArgs("ANY cannot be used in a port set; omit the port set to match any port")
	}

	lo, hi, isRange := strings.Cut(port, "..")
	if !isRange {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return "", tlerr.InvalidArgs("Invalid port %s", port)
		}
		return port, nil
	}

	loNum, err := strconv.ParseUint(lo, 10, 16)
	if err != nil {
		return "", tlerr.InvalidArgs("Invalid lower port %s in range %s", lo, port)
	}
	hiNum, err := strconv.ParseUint(hi, 10, 16)
	if err != nil {
		return "", tlerr.InvalidArgs("Invalid upper port %s in range %s", hi, port)
	}
	if loNum > hiNum {
		return "", tlerr.InvalidArgs("Invalid port range %s; lower port is greater than upper port", port)
	}

	return lo + "-" + hi, nil
}

// portNumRangeFromDb is the inverse of portNumRangeToDb.
func portNumRangeFromDb(port string) string {
	return strings.Replace(port, "-", "..", 1)
}

// portSetMatch returns the port match a port set resolves to, in CONFIG_DB
// form. An ACL_RULE carries a single L4 port match, so a set that does not hold
// exactly one port or range cannot be programmed.
func portSetMatch(d *db.DB, name string) (string, error) {
	entry, err := d.GetEntry(&db.TableSpec{Name: PORT_SET_TABLE}, asKey(name))
	if err != nil || !entry.IsPopulated() {
		return "", tlerr.NotFound("Port set %s not found", name)
	}

	ports := (&entry).GetList(PORT_SET_PORTS)
	switch len(ports) {
	case 1:
		return ports[0], nil
	case 0:
		return "", tlerr.InvalidArgs("Port set %s holds no port", name)
	default:
		return "", tlerr.InvalidArgs(
			"Port set %s holds %d ports; an ACL rule can match only one port or one contiguous range",
			name, len(ports))
	}
}

// setRulePortMatch writes an L4 port match into an ACL_RULE value. A range goes
// to its own field, and the plain port and the range are a yang choice, so the
// other one has to go.
func setRulePortMatch(ruleData db.Value, isSrc bool, portVal string) {
	port, portRange := rulePortFields(isSrc)

	(&ruleData).Remove(port)
	(&ruleData).Remove(portRange)

	if strings.Contains(portVal, "-") {
		ruleData.Field[portRange] = portVal
	} else {
		ruleData.Field[port] = portVal
	}
}

// rulePortFields returns the plain port and the range field of an ACL_RULE, which
// are the two cases of the src_port/dst_port yang choice.
func rulePortFields(isSrc bool) (string, string) {
	if isSrc {
		return L4_SRC_PORT, L4_SRC_PORT_RANGE
	}
	return L4_DST_PORT, L4_DST_PORT_RANGE
}

// writeRulePortMatch programs an L4 port match on a rule that already exists.
// The unused half of the choice is cleared in a request of its own: CVL
// validates a write against the merged view of the request and the current row,
// so clearing and setting the two halves at once looks to it like a rule
// matching on a port and on a range at the same time.
func writeRulePortMatch(d *db.DB, ruleTs *db.TableSpec, key db.Key, ruleData db.Value, isSrc bool, portVal string) error {
	port, portRange := rulePortFields(isSrc)
	field, unused := port, portRange
	if strings.Contains(portVal, "-") {
		field, unused = portRange, port
	}

	if len((&ruleData).Get(unused)) > 0 {
		clear := db.Value{Field: map[string]string{unused: ""}}
		if err := d.DeleteEntryFields(ruleTs, key, clear); err != nil {
			return err
		}
	}

	return d.ModEntry(ruleTs, key, db.Value{Field: map[string]string{field: portVal}})
}

// rulePortSetRefs returns the port set references recorded for a rule. An
// unpopulated value is returned when the rule matches on literal ports.
func rulePortSetRefs(d *db.DB, aclKey, ruleKey string) db.Value {
	entry, err := d.GetEntry(&db.TableSpec{Name: RULE_PORT_SET_TABLE},
		db.Key{Comp: []string{aclKey, ruleKey}})
	if err != nil {
		return db.Value{}
	}
	return entry
}

// rulesUsingPortSet returns the keys of the rules that match on the given port
// set, as <acl name>|<rule name>.
func rulesUsingPortSet(d *db.DB, name string) ([]db.Key, error) {
	refTs := &db.TableSpec{Name: RULE_PORT_SET_TABLE}

	keys, err := d.GetKeys(refTs)
	if err != nil {
		return nil, err
	}

	var users []db.Key
	for _, key := range keys {
		entry, err := d.GetEntry(refTs, key)
		if err != nil {
			continue
		}
		if (&entry).Get(RULE_SRC_PORT_SET) == name || (&entry).Get(RULE_DST_PORT_SET) == name {
			users = append(users, key)
		}
	}

	return users, nil
}

// reprogramPortSetUsers rewrites the L4 port match of every rule that matches on
// the given port set. It is needed when the contents of a set change, because
// the rules hold the resolved port rather than the set name.
func reprogramPortSetUsers(d *db.DB, name string) error {
	users, err := rulesUsingPortSet(d, name)
	if err != nil || len(users) == 0 {
		return err
	}

	portVal, err := portSetMatch(d, name)
	if err != nil {
		return err
	}

	ruleTs := &db.TableSpec{Name: RULE_TABLE}
	refTs := &db.TableSpec{Name: RULE_PORT_SET_TABLE}

	for _, key := range users {
		refs, err := d.GetEntry(refTs, key)
		if err != nil {
			return err
		}

		ruleData, err := d.GetEntry(ruleTs, key)
		if err != nil || !ruleData.IsPopulated() {
			// The reference outlived its rule. Drop it rather than fail the
			// port set update on stale data.
			log.Warningf("reprogramPortSetUsers: rule %v of port set %s is gone, removing its reference", key, name)
			if err := d.DeleteEntry(refTs, key); err != nil {
				return err
			}
			continue
		}

		if (&refs).Get(RULE_SRC_PORT_SET) == name {
			if err := writeRulePortMatch(d, ruleTs, key, ruleData, true, portVal); err != nil {
				return err
			}
		}
		if (&refs).Get(RULE_DST_PORT_SET) == name {
			if err := writeRulePortMatch(d, ruleTs, key, ruleData, false, portVal); err != nil {
				return err
			}
		}
	}

	return nil
}
