package transformer

import (
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

func init() {
	XlateFuncBind("YangToDb_qos_dscp_classifier_xfmr", YangToDb_qos_dscp_classifier_xfmr)
	XlateFuncBind("DbToYang_qos_dscp_classifier_xfmr", DbToYang_qos_dscp_classifier_xfmr)
	XlateFuncBind("Subscribe_qos_dscp_classifier_xfmr", Subscribe_qos_dscp_classifier_xfmr)

	XlateFuncBind("YangToDb_classifier_intf_qos_map_fld_xfmr", YangToDb_classifier_intf_qos_map_xfmr)
	XlateFuncBind("DbToYang_classifier_intf_qos_map_fld_xfmr", DbToYang_classifier_intf_qos_map_xfmr)
	XlateFuncBind("Subscribe_classifier_intf_qos_map_fld_xfmr", Subscribe_classifier_intf_qos_map_xfmr)
	XlateFuncBind("DbToYang_classifier_intf_qos_map_key_xfmr", DbToYang_classifier_intf_qos_map_key_xfmr)
}

var Subscribe_qos_dscp_classifier_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	/*
	 * The RFC parent table check resolves a keyed classifier against the table
	 * reported here and fails the request when that key is absent. A policed
	 * classifier lives in ACL_TABLE rather than DSCP_TO_TC_MAP, so report the
	 * table it is actually stored in, else GET and DELETE report it missing.
	 */
	if name := NewPathInfo(inParams.uri).Var("name"); name != "" {
		if qosPolicedClassifierInDb(inParams.dbs[db.ConfigDB], name) {
			return XfmrSubscOutParams{
				dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {qosAclTableTbl: {name: {}}}},
			}, nil
		}
	}
	return Subscribe_qos_map_xfmr(inParams, "DSCP_TO_TC_MAP")
}

var YangToDb_qos_dscp_classifier_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	map_type := "DSCP_TO_TC_MAP"

	log.V(lvl.DEBUG).Info("YangToDb_qos_dscp_classifier_xfmr: ", inParams.ygRoot, inParams.uri)
	log.V(lvl.DEBUG).Info("inParams: ", inParams)

	if inParams.oper == DELETE {
		/*
		 * A policed classifier lives in ACL_RULE rather than DSCP_TO_TC_MAP, so
		 * the map delete below would leave its ACL objects behind.
		 */
		delPathInfo := NewPathInfo(inParams.uri)
		if name := delPathInfo.Var("name"); name != "" && qosClassifierIsPolicing(inParams, name) {
			return qosClassifierAclDelete(inParams, name, delPathInfo.Var("id"))
		}
		if res_map, err := qos_map_delete_xfmr(inParams, "QOS_UMF_CLASSIFIER"); err != nil {
			return res_map, err
		}
		return qos_map_delete_xfmr(inParams, map_type)
	}

	res_map := make(map[string]map[string]db.Value)

	pathInfo := NewPathInfo(inParams.uri)
	name := pathInfo.Var("name")
	keyid := pathInfo.Var("id")
	targetUriPath, err := getYangPathFromUri(inParams.uri)

	if err != nil {
		log.V(lvl.ERROR).Info("Could not get target URI path")
		return res_map, err
	}

	/* parse the inParams */
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		log.V(lvl.ERROR).Info("nil qos obj")
		return res_map, tlerr.InvalidArgsError{Format: "nil qos obj"}
	}

	if _, ok := qosObj.Classifiers.Classifier[name]; !ok {
		/* URI path does not have a classifier instance, hence nothing more to process */
		return res_map, nil
	}

	if inParams.d == nil {
		log.V(lvl.ERROR).Info("unable to get configDB")
		return res_map, tlerr.InternalError{Format: "Database not available"}
	}

	/*
	 * A classifier whose terms match on more than a bare DSCP value cannot be
	 * held in DSCP_TO_TC_MAP. Those are realized as an ingress ACL carrying a
	 * policer instead; see xfmr_qos_policer.go.
	 */
	if qosClassifierIsPolicing(inParams, name) {
		if err := qosClassifierToAcl(inParams, name, qosObj.Classifiers.Classifier[name], res_map); err != nil {
			return res_map, err
		}
		return res_map, nil
	}

	/* Fill in the UMF table for bookkeeping classfier type */
	umf_map_entry := make(map[string]db.Value)
	umf_map_key := name
	umf_map_entry[umf_map_key] = db.Value{Field: make(map[string]string)}

	switch qosObj.Classifiers.Classifier[name].Config.Type {
	case ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV6:
		umf_map_entry[umf_map_key].Field["type"] = "ipv6"
	default:
		umf_map_entry[umf_map_key].Field["type"] = "ipv4"
	}
	res_map["QOS_UMF_CLASSIFIER"] = umf_map_entry

	/* Fill SONiC DSCP_TO_TC Table */
	map_entry := make(map[string]db.Value)
	map_key := name
	map_entry[map_key] = db.Value{Field: make(map[string]string)}

	if !strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/classifiers/classifier") {
		log.V(lvl.ERROR).Info("YangToDb: map entry unspecified, return the map")
		res_map[map_type] = map_entry
		return res_map, tlerr.NotSupportedError{Format: "Unsupported path"}
	}

	term, ok := qosObj.Classifiers.Classifier[name].Terms.Term[keyid]
	if !ok {
		/* URI path does not have a Term, hence nothing further to process */
		res_map[map_type] = map_entry
		return res_map, nil
	}

	var entry_key string
	if term.Conditions.Ipv4 != nil && term.Conditions.Ipv4.Config != nil {
		entry_key = strconv.Itoa(int(*term.Conditions.Ipv4.Config.Dscp))
	}

	/* If v4 DSCP is not specified lets check the v6 DSCP */
	if entry_key == "" {
		if term.Conditions.Ipv6 != nil && term.Conditions.Ipv6.Config != nil {
			entry_key = strconv.Itoa(int(*term.Conditions.Ipv6.Config.Dscp))
		}
		if entry_key == "" {
			return res_map, tlerr.InvalidArgsError{Format: "Invalid DSCP"}
		}
	}

	log.V(lvl.DEBUG).Info("entry_key : ", entry_key, " operation: ", inParams.oper, " CREATE: ", CREATE,
		" REPLACE: ", REPLACE, " UPDATE: ", UPDATE)

	val := *(term.Actions.Config.TargetGroup)

	if qosObj.ForwardingGroups == nil {
		map_entry[map_key].Field[entry_key] = get_fwd_group_priority(inParams, val)
	} else {
		fwdGrp, ok := qosObj.ForwardingGroups.ForwardingGroup[val]
		if ok {
			map_entry[map_key].Field[entry_key] = strconv.Itoa(int(*fwdGrp.Config.FabricPriority))
		} else {
			map_entry[map_key].Field[entry_key] = get_fwd_group_priority(inParams, val)
		}
	}

	log.V(lvl.DEBUG).Info("map key : ", map_key, " entry_key: ", entry_key)
	res_map[map_type] = map_entry

	return res_map, nil

}

func fill_classifier_info_by_name(inParams XfmrParams, classifiers *ocbinds.OpenconfigQos_Qos_Classifiers, name string, fillActionState bool) error {
	mapObj, ok := classifiers.Classifier[name]
	if !ok {
		mapObj, _ = classifiers.NewClassifier(name)
		ygot.BuildEmptyTree(mapObj)
		mapObj.Name = &name
	}

	if mapObj.Terms == nil {
		mapObj.Terms = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms{}
	}

	if mapObj.Config == nil {
		mapObj.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config{}
	}

	if mapObj.State == nil {
		mapObj.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_State{}
	}

	key := db.Key{Comp: []string{name}}

	map_type := "DSCP_TO_TC_MAP"
	dbSpec := &db.TableSpec{Name: map_type}
	mapCfg, err := inParams.d.GetEntry(dbSpec, key)

	if fillActionState {
		mapCfg, err = inParams.dbs[db.ApplDB].GetEntry(dbSpec, key)
	}

	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Info("No map with a name of : ", name)
		return nil
	}

	mapObj.Config.Name = &name
	mapObj.State.Name = &name

	/* Get classifier type from UMF table */
	dbSpec = &db.TableSpec{Name: "QOS_UMF_CLASSIFIER"}
	classifierCfg, err := inParams.d.GetEntry(dbSpec, key)
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Info("No umf map with a name of : ", name)
		return nil
	}

	switch classifierCfg.Field["type"] {
	case "ipv6":
		mapObj.Config.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV6
		mapObj.State.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV6
	default:
		mapObj.Config.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV4
		mapObj.State.Type = ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Config_Type_IPV4
	}

	for k, v := range mapCfg.Field {
		log.V(lvl.DEBUG).Info("Table key-value ", k, " ", v)

		tmp, err := strconv.ParseUint(k, 10, 8)
		if err != nil {
			log.V(lvl.ERROR).Info("Could not parse DSCP key")
			return err
		}

		key := uint8(tmp)
		entryObj, ok := mapObj.Terms.Term[strconv.Itoa(int(key))]
		if !ok {
			entryObj, err = mapObj.Terms.NewTerm(strconv.Itoa(int(key)))
			if err != nil {
				log.V(lvl.ERROR).Info("Could not create new term")
				return err
			}
			ygot.BuildEmptyTree(entryObj)
			ygot.BuildEmptyTree(entryObj.Config)
			ygot.BuildEmptyTree(entryObj.State)
		}

		if entryObj.Config == nil {
			entryObj.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Config{}
		}

		key_str := strconv.Itoa(int(key))
		entryObj.Config.Id = &key_str

		if entryObj.State == nil {
			entryObj.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_State{}
		}
		entryObj.State.Id = &key_str

		if entryObj.Conditions == nil {
			entryObj.Conditions = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions{}
		}

		if entryObj.Conditions.Ipv4 == nil {
			entryObj.Conditions.Ipv4 = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4{}
		}

		if entryObj.Conditions.Ipv4.Config == nil {
			entryObj.Conditions.Ipv4.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_Config{}
		}

		if entryObj.Conditions.Ipv4.State == nil {
			entryObj.Conditions.Ipv4.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv4_State{}
		}

		if entryObj.Conditions.Ipv6 == nil {
			entryObj.Conditions.Ipv6 = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6{}
		}

		if entryObj.Conditions.Ipv6.Config == nil {
			entryObj.Conditions.Ipv6.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_Config{}
		}

		if entryObj.Conditions.Ipv6.State == nil {
			entryObj.Conditions.Ipv6.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Conditions_Ipv6_State{}
		}

		if entryObj.Actions == nil {
			entryObj.Actions = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Actions{}
		}

		if entryObj.Actions.Config == nil {
			entryObj.Actions.Config = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Actions_Config{}
		}

		if entryObj.Actions.State == nil {
			entryObj.Actions.State = &ocbinds.OpenconfigQos_Qos_Classifiers_Classifier_Terms_Term_Actions_State{}
		}

		// Get the forwarding group name from value
		fwd_group := get_fwd_group_from_priority(inParams, v)
		if !fillActionState {
			switch classifierCfg.Field["type"] {
			case "ipv6":
				entryObj.Conditions.Ipv6.Config.Dscp = &key
				entryObj.Conditions.Ipv6.State.Dscp = &key
			default:
				entryObj.Conditions.Ipv4.Config.Dscp = &key
				entryObj.Conditions.Ipv4.State.Dscp = &key
			}

			entryObj.Actions.Config.TargetGroup = &fwd_group
		} else {
			entryObj.Actions.State.TargetGroup = &fwd_group
		}
		log.V(lvl.DEBUG).Infof("Added entry: %v ", entryObj)
	}

	return nil
}

var DbToYang_qos_dscp_classifier_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	name := pathInfo.Var("name")

	log.V(lvl.DEBUG).Info("inParams: ", inParams)
	log.V(lvl.DEBUG).Info("name: ", name)
	qosObj := getQosRoot(inParams.ygRoot)

	if qosObj == nil {
		ygot.BuildEmptyTree(qosObj)
	}

	if qosObj.Classifiers == nil {
		ygot.BuildEmptyTree(qosObj.Classifiers)
	}

	dbSpec := &db.TableSpec{Name: "DSCP_TO_TC_MAP"}

	map_added := 0
	keyPattern := "*"
	if name != "" {
		keyPattern = name
	}

	keys, err := inParams.d.GetKeysByPattern(dbSpec, keyPattern)

	if err != nil {
		log.V(lvl.ERROR).Info("Could not find classifier table in DB")
		return err
	}

	for _, key := range keys {
		log.V(lvl.DEBUG).Info("key: ", key)

		if len(key.Comp) == 0 {
			err := tlerr.NotFoundError{Format: "Invalid Key in DB"}
			log.V(lvl.ERROR).Info("Invalid Key in DB")
			return err
		}
		map_name := key.Comp[0]

		map_added++

		if err := fill_classifier_info_by_name(inParams, qosObj.Classifiers, map_name, false); err != nil {
			return err
		}

		/* Fill the Action State */
		if err := fill_classifier_info_by_name(inParams, qosObj.Classifiers, map_name, true); err != nil {
			return err
		}
	}

	/*
	 * Policed classifiers are held in ACL_RULE rather than DSCP_TO_TC_MAP, so
	 * they have to be gathered separately. Without this a GET reports them as
	 * missing, which also makes translib refuse to delete them.
	 */
	acl_added, err := qosFillClassifiersFromAcl(inParams, qosObj.Classifiers, name)
	if err != nil {
		return err
	}
	map_added += acl_added

	if name != "" && map_added == 0 {
		log.V(lvl.ERROR).Infof("Resource not found: %v", name)
		return tlerr.NotFoundError{Format: "Resource not found"}
	}

	return nil
}

func is_classifier_dscp(inParams XfmrParams, key string) bool {
	/* parse the inParams */
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj != nil && qosObj.Classifiers != nil {
		if classifier, ok := qosObj.Classifiers.Classifier[key]; ok && classifier.Terms != nil {
			for _, term := range classifier.Terms.Term {
				if term.Conditions == nil {
					continue
				}
				if term.Conditions.Ipv4 != nil &&
					term.Conditions.Ipv4.Config != nil &&
					term.Conditions.Ipv4.Config.Dscp != nil {
					return true
				}
				if term.Conditions.Ipv6 != nil &&
					term.Conditions.Ipv6.Config != nil &&
					term.Conditions.Ipv6.Config.Dscp != nil {
					return true
				}
			}
		}
	}

	/* Check in DB */
	dbSpec := &db.TableSpec{Name: "DSCP_TO_TC_MAP"}
	_, err := inParams.d.GetKeysByPattern(dbSpec, key)

	return err == nil
}

var DbToYang_classifier_intf_qos_map_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	keyMap := map[string]interface{}{}
	ifName := NewPathInfo(inParams.uri).Var("interface-id")
	dbSpec := &db.TableSpec{Name: "PORT_QOS_MAP"}
	key := db.Key{Comp: []string{ifName}}

	mapCfg, err := inParams.d.GetEntry(dbSpec, key)
	if err != nil {
		return nil, tlerr.NotFoundError{Format: fmt.Sprintf("PORT_QOS_MAP not found for %v", ifName)}
	}
	db_attr_name, ok := map_type_name_in_db["DSCP_TO_TC_MAP"]
	if !ok {
		return nil, tlerr.NotFoundError{Format: "Map type name not found for DSCP_TO_TC_MAP"}
	}

	if name, ok := mapCfg.Field[db_attr_name]; ok {
		keyMap["name"] = name
	}
	return keyMap, nil
}

// qosIntfHasDscpMap reports whether an interface names a QoS map in the given
// PORT_QOS_MAP field.
func qosIntfHasDscpMap(inParams XfmrParams, ifName, attrName string) bool {
	if inParams.d == nil {
		return false
	}
	entry, err := inParams.d.GetEntry(&db.TableSpec{Name: "PORT_QOS_MAP"},
		db.Key{Comp: []string{ifName}})
	return err == nil && entry.Field[attrName] != ""
}

func YangToDb_classifier_intf_qos_map_xfmr(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	res_map := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("inParams: ", inParams)

	pathInfo := NewPathInfo(inParams.uri)

	if_name := pathInfo.Var("interface-id")

	if if_name == "" {
		return res_map, nil
	}

	qosIntfsObj := getQosIntfRoot(inParams.ygRoot)
	if qosIntfsObj == nil {
		return res_map, nil
	}

	intfObj, ok := qosIntfsObj.Interface[if_name]
	if !ok {
		return res_map, nil
	}

	portQosTbl := make(map[string]db.Value)
	portQosTbl[if_name] = db.Value{Field: make(map[string]string)}

	var mapv4_name string
	var mapv6_name string
	aclNames := []string{}

	map_type := "DSCP_TO_TC_MAP"
	if intfObj.Input != nil && intfObj.Input.Classifiers != nil {
		for _, classifier := range intfObj.Input.Classifiers.Classifier {
			if classifier.Config == nil || classifier.Config.Name == nil {
				/*
				 * A delete addressing one list instance carries the type key
				 * and nothing else, which names no classifier. That is not an
				 * error; the bindings to undo come from the tables below.
				 */
				if inParams.oper == DELETE {
					continue
				}
				log.V(lvl.ERROR).Info("Classifier not supported ")
				return res_map, tlerr.NotFoundError{Format: "Classifier not supported"}
			}
			clsName := *classifier.Config.Name

			if qosClassifierIsPolicing(inParams, clsName) {
				/*
				 * A policed classifier reaches an interface through the
				 * scheduler-policy that polices it, which owns the port list
				 * this path would write. Binding it here would put the
				 * interface in a list only that policy may take it out of, so
				 * say so rather than leave a binding nothing can undo.
				 */
				if qosPolicerForClassifier(inParams, clsName) != "" {
					return res_map, tlerr.NotSupportedError{
						Format: "Classifier " + clsName + " is policed and is bound " +
							"through its scheduler-policy, not as an interface classifier",
					}
				}
				aclNames = append(aclNames, clsName)
				continue
			}

			if !is_classifier_dscp(inParams, clsName) {
				log.V(lvl.ERROR).Info("Classifier not supported ", clsName)
				return res_map, tlerr.NotFoundError{Format: "Classifier not supported " + clsName}
			}

			switch classifier.Config.Type {
			case ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_UNSET:
				fallthrough
			case ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV4:
				mapv4_name = clsName
			case ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV6:
				mapv6_name = clsName
			}
			map_type = "DSCP_TO_TC_MAP"
		}
	}

	port_qos_map_table := "PORT_QOS_MAP"

	/*
	 * SONiC has no per interface classifier binding, so an ACL backed
	 * classifier is bound by adding the interface to its table's ports. Name
	 * only this interface, never the whole list: translib appends the members
	 * named here on an update and removes just those on a delete, so any other
	 * interface bound to the same classifier stays bound.
	 */
	bindAcl := func(names []string) {
		for _, cls := range names {
			qosMergeTable(res_map, qosAclTableTbl,
				map[string]db.Value{cls: {Field: map[string]string{"ports@": if_name}}})
		}
	}

	if inParams.oper == DELETE {
		/*
		 * A delete carries the list key but not the classifier name, so the
		 * bindings to undo come from the tables. Only the ones this path made
		 * are listed; a policed classifier is unbound through its
		 * scheduler-policy instead.
		 */
		if len(aclNames) == 0 {
			// A delete of one list instance names a family and must leave the
			// other one bound.
			family := ""
			switch pathInfo.Var("type") {
			case "IPV4":
				family = "ipv4"
			case "IPV6":
				family = "ipv6"
			}
			for _, cls := range qosAclClassifiersOnIntf(inParams, if_name) {
				if family == "" || qosAclClassifierFamily(inParams, cls) == family {
					aclNames = append(aclNames, cls)
				}
			}
		}
		bindAcl(aclNames)

		attr_name, ok := map_type_name_in_db[map_type]
		if !ok {
			log.V(lvl.ERROR).Info("map_type not implemented", map_type)
			return res_map, tlerr.InternalError{Format: "Not Implemented"}
		}

		/*
		 * Having unbound an ACL, leave PORT_QOS_MAP alone unless the interface
		 * really carries a map, rather than clearing one the operator never
		 * configured. A map that is there is reported under this same list key
		 * and so is part of what the delete addresses.
		 */
		if mapv4_name == "" && mapv6_name == "" && len(aclNames) > 0 &&
			!qosIntfHasDscpMap(inParams, if_name, attr_name) {
			return res_map, nil
		}
		portQosTbl[if_name].Field[attr_name] = ""
		res_map[port_qos_map_table] = portQosTbl
		return res_map, nil
	}

	bindAcl(aclNames)

	map_name := mapv4_name
	if map_name == "" {
		map_name = mapv6_name
	}

	if map_name == "" {
		if len(aclNames) > 0 {
			return res_map, nil
		}
		log.V(lvl.ERROR).Info("map name is missing")
		return res_map, tlerr.InternalError{Format: "map name missing "}
	}

	if attr_name, ok := map_type_name_in_db[map_type]; !ok {
		log.V(lvl.DEBUG).Info("map_type not implemented", map_type)
	} else {
		/*
		 * We could update/delete existing keys by
		 * '|'ing the interface names if value is same,
		 * however, this approach is complex and bug prone.
		 * Hence we will manage unique entries per interface.
		 */
		portQosTbl[if_name].Field[attr_name] = map_name
	}

	res_map[port_qos_map_table] = portQosTbl
	return res_map, nil
}

/*
 * Given a classfier name, function returns the v4 and v6 classfier names
 * based on classifier type v4 or v6, and correspondingly generate the
 * classifier name of other type which is needed for OC completeness.
 * Ex1: Classifier name: "dscp_ipv4", type ipv4, returns "dscp_ipv4", "dscp_ipv6"
 * Ex1: Classifier name: "dscp_ipv6", type ipv6 returns "dscp_ipv4", "dscp_ipv6"
 * Ex3: Classifier name: "dscp", type ipv4, returns "dscp", "dscp_ipv6"
 * Ex4: Classifier name: "dscp", type ipv6, returns "dscp_ipv4", "dscp"
 */
func get_classifier_names(d *db.DB, classifier_name string) (string, string) {
	/*  Get entry from DB */
	dbSpec := &db.TableSpec{Name: "QOS_UMF_CLASSIFIER"}
	key := db.Key{Comp: []string{classifier_name}}
	classifierCfg, err := d.GetEntry(dbSpec, key)
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Info("Could not find classifier ", classifier_name)
	}

	switch classifierCfg.Field["type"] {
	case "ipv6":
		if strings.HasSuffix(classifier_name, "_ipv6") {
			return classifier_name[:len(classifier_name)-5] + "_ipv4", classifier_name
		}
		return classifier_name + "_ipv4", classifier_name
	default:
		if strings.HasSuffix(classifier_name, "_ipv4") {
			return classifier_name, classifier_name[:len(classifier_name)-5] + "_ipv6"
		}
		return classifier_name, classifier_name + "_ipv6"
	}

	return classifier_name, classifier_name + "_ipv6"
}

type qosIntfClsType = ocbinds.E_OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type

const (
	qosIntfClsAny  = ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_UNSET
	qosIntfClsIPv4 = ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV4
	qosIntfClsIPv6 = ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV6
)

var DbToYang_classifier_intf_qos_map_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	if_name := pathInfo.Var("interface-id")
	qosIntfsObj := getQosIntfRoot(inParams.ygRoot)
	if qosIntfsObj == nil {
		return nil
	}

	intfObj, ok := qosIntfsObj.Interface[if_name]
	if !ok {
		return nil
	}

	/*
	 * A request addressing one list instance asks for that type alone, and
	 * the tree already holds an entry for it built from the URI key.
	 */
	keyType := pathInfo.Var("type")
	wantType := qosIntfClsAny
	switch keyType {
	case "":
	case "IPV4":
		wantType = qosIntfClsIPv4
	case "IPV6":
		wantType = qosIntfClsIPv6
	default:
		return tlerr.NotFoundError{Format: "No " + keyType + " classifier on " + if_name}
	}

	filled := map[qosIntfClsType]bool{}

	if err := fill_intf_dscp_classifiers(inParams, intfObj, if_name, wantType, filled); err != nil {
		return err
	}
	if err := fill_intf_acl_classifiers(inParams, intfObj, if_name, wantType, filled); err != nil {
		return err
	}

	if wantType != qosIntfClsAny && !filled[wantType] {
		return tlerr.NotFoundError{Format: "No " + keyType + " classifier on " + if_name}
	}
	return nil
}

// fill_intf_classifier sets one entry of an interface's classifier list,
// reusing the entry a request addressing that instance has already created.
func fill_intf_classifier(intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface,
	clsType qosIntfClsType, name string) error {

	if intfObj.Input == nil {
		intfObj.Input = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input{}
	}
	if intfObj.Input.Classifiers == nil {
		intfObj.Input.Classifiers = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers{}
	}

	classifier, ok := intfObj.Input.Classifiers.Classifier[clsType]
	if !ok {
		var err error
		if classifier, err = intfObj.Input.Classifiers.NewClassifier(clsType); err != nil {
			return tlerr.InternalError{Format: "Failed to initialize classifier " + name}
		}
	}

	cfgName, stateName := name, name
	classifier.Config = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config{
		Name: &cfgName, Type: clsType,
	}
	classifier.State = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_State{
		Name: &stateName, Type: clsType,
	}
	return nil
}

/*
 * fill_intf_dscp_classifiers reports the DSCP map bound to an interface. SONiC
 * applies the map to both families, so it is reported under both list keys.
 * qosorch takes PORT_QOS_MAP from CONFIG_DB and no applied copy is kept in
 * APPL_DB, so the state is what CONFIG_DB holds.
 */
func fill_intf_dscp_classifiers(inParams XfmrParams,
	intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface, if_name string,
	wantType qosIntfClsType, filled map[qosIntfClsType]bool) error {

	db_attr_name, ok := map_type_name_in_db["DSCP_TO_TC_MAP"]
	if !ok {
		log.V(lvl.DEBUG).Info("map_type not implemented DSCP_TO_TC_MAP")
		return nil
	}

	mapCfg, err := inParams.d.GetEntry(&db.TableSpec{Name: "PORT_QOS_MAP"}, db.Key{Comp: []string{if_name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("QoS Map not found for interface ", if_name)
		return nil
	}

	classifier_name := mapCfg.Field[db_attr_name]
	if classifier_name == "" {
		return nil
	}

	classifierv4, classifierv6 := get_classifier_names(inParams.d, classifier_name)
	for clsType, name := range map[qosIntfClsType]string{
		qosIntfClsIPv4: classifierv4,
		qosIntfClsIPv6: classifierv6,
	} {
		if wantType != qosIntfClsAny && clsType != wantType {
			continue
		}
		if err := fill_intf_classifier(intfObj, clsType, name); err != nil {
			return err
		}
		filled[clsType] = true
	}
	return nil
}

/*
 * fill_intf_acl_classifiers reports the ACL backed classifiers bound to an
 * interface. Unlike a DSCP map, which SONiC applies to both families and which
 * is therefore reported under both list keys, an ACL backed classifier is built
 * for one family and is reported under that key alone.
 */
func fill_intf_acl_classifiers(inParams XfmrParams,
	intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface, if_name string,
	wantType qosIntfClsType, filled map[qosIntfClsType]bool) error {

	for _, name := range qosAclClassifiersOnIntf(inParams, if_name) {
		clsType := qosIntfClsIPv4
		if qosAclClassifierFamily(inParams, name) == "ipv6" {
			clsType = qosIntfClsIPv6
		}
		if wantType != qosIntfClsAny && clsType != wantType {
			continue
		}

		// A DSCP map on the same interface already claimed both list keys. It
		// is what PORT_QOS_MAP says is applied, so let it stand.
		if filled[clsType] {
			log.V(lvl.DEBUG).Info("Classifier list already holds type for intf ", if_name, " ", name)
			continue
		}

		if err := fill_intf_classifier(intfObj, clsType, name); err != nil {
			return err
		}
		filled[clsType] = true
	}
	return nil
}

var Subscribe_classifier_intf_qos_map_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	/*
	 * The RFC parent table check resolves a keyed binding against the table
	 * reported here and fails the request when that key is absent. An ACL
	 * backed classifier is bound by the interface being in its table's ports,
	 * not through PORT_QOS_MAP, so report the table the binding is really in,
	 * else a DELETE of the list instance reports it missing.
	 */
	pathInfo := NewPathInfo(inParams.uri)
	family := ""
	switch pathInfo.Var("type") {
	case "IPV4":
		family = "ipv4"
	case "IPV6":
		family = "ipv6"
	}
	cls := qosAclClassifierBoundToIntf(inParams.dbs[db.ConfigDB],
		pathInfo.Var("interface-id"), family)
	if cls != "" {
		return XfmrSubscOutParams{
			dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {qosAclTableTbl: {cls: {}}}},
		}, nil
	}
	return Subscribe_qos_map_xfmr(inParams, "PORT_QOS_MAP")
}
