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
	return Subscribe_qos_map_xfmr(inParams, "DSCP_TO_TC_MAP")
}

var YangToDb_qos_dscp_classifier_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	map_type := "DSCP_TO_TC_MAP"

	log.V(lvl.DEBUG).Info("YangToDb_qos_dscp_classifier_xfmr: ", inParams.ygRoot, inParams.uri)
	log.V(lvl.DEBUG).Info("inParams: ", inParams)

	if inParams.oper == DELETE {
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

	map_type := "DSCP_TO_TC_MAP"
	if intfObj.Input != nil && intfObj.Input.Classifiers != nil {
		for _, classifier := range intfObj.Input.Classifiers.Classifier {
			if classifier.Config == nil {
				log.V(lvl.ERROR).Info("Classifier not supported ")
				return res_map, tlerr.NotFoundError{Format: "Classifier not supported"}
			}

			if !is_classifier_dscp(inParams, *classifier.Config.Name) {
				log.V(lvl.ERROR).Info("Classifier not supported ", *classifier.Config.Name)
				return res_map, tlerr.NotFoundError{Format: "Classifier not supported " + *classifier.Config.Name}
			}

			switch classifier.Config.Type {
			case ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_UNSET:
				fallthrough
			case ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV4:
				mapv4_name = *classifier.Config.Name
			case ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV6:
				mapv6_name = *classifier.Config.Name
			}
			map_type = "DSCP_TO_TC_MAP"
		}
	}

	port_qos_map_table := "PORT_QOS_MAP"
	if inParams.oper == DELETE {
		attr_name, ok := map_type_name_in_db[map_type]
		if !ok {
			log.V(lvl.ERROR).Info("map_type not implemented", map_type)
			return res_map, tlerr.InternalError{Format: "Not Implemented"}
		}
		portQosTbl[if_name].Field[attr_name] = ""
		res_map[port_qos_map_table] = portQosTbl
		return res_map, nil
	}

	map_name := mapv4_name
	if map_name == "" {
		map_name = mapv6_name
	}

	if map_name == "" {
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

	dbSpec := &db.TableSpec{Name: "PORT_QOS_MAP"}

	key := db.Key{Comp: []string{if_name}}
	mapCfg, err := inParams.d.GetEntry(dbSpec, key)
	if err != nil {
		log.V(lvl.DEBUG).Info("QoS Map not found for interface ", if_name)
		return nil
	}

	mapState, err := inParams.dbs[db.ApplDB].GetEntry(dbSpec, key)
	if err != nil {
		log.V(lvl.DEBUG).Info("QoS Map not found for interface ", if_name)
	}

	map_type := "DSCP_TO_TC_MAP"

	db_attr_name, ok := map_type_name_in_db[map_type]
	if !ok {
		log.V(lvl.DEBUG).Info("map_type not implemented", map_type)
		return nil
	}

	if value, ok := mapCfg.Field[db_attr_name]; ok {
		if intfObj.Input == nil {
			intfObj.Input = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input{}
		}

		if intfObj.Input.Classifiers == nil {
			intfObj.Input.Classifiers = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers{}
		}

		state_value, ok := mapState.Field[db_attr_name]
		if !ok {
			log.V(lvl.DEBUG).Info("QoS Map state not available for intf ", if_name)
		}

		classifier_name := value
		state_classifier_name := state_value
		classifierv4, classifierv6 := get_classifier_names(inParams.d, classifier_name)
		state_classifierv4, state_classifierv6 := get_classifier_names(inParams.d, state_classifier_name)

		/* Since a classifier in SONiC applies to both v4 and v6 traffic we will fill both v4 and v6 paths */
		/* V4 Classifier */
		classifier, err := intfObj.Input.Classifiers.NewClassifier(ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV4)
		if err != nil {
			log.V(lvl.ERROR).Info("Failed to initialize classifier ", classifier_name)
			return tlerr.NotFoundError{Format: "Failed to initialize classifier"}
		}

		if classifier.Config == nil {
			classifier.Config = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config{}
		}

		if classifier.State == nil {
			classifier.State = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_State{}
		}

		/* Update Config */
		classifier.Config.Name = &classifierv4
		classifier.Config.Type = ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV4
		/* Update State */
		classifier.State.Name = &state_classifierv4
		classifier.State.Type = ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV4

		/* V6 Classifier */
		classifier6, err := intfObj.Input.Classifiers.NewClassifier(ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV6)
		if err != nil {
			log.V(lvl.ERROR).Info("Failed to initialize classifier ", classifier_name)
			return tlerr.NotFoundError{Format: "Failed to initialize classifier"}
		}

		if classifier6.Config == nil {
			classifier6.Config = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config{}
		}

		if classifier6.State == nil {
			classifier6.State = &ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_State{}
		}

		/* Update Config */
		classifier6.Config.Name = &classifierv6
		classifier6.Config.Type = ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV6
		/* Update State */
		classifier6.State.Name = &state_classifierv6
		classifier6.State.Type = ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Input_Classifiers_Classifier_Config_Type_IPV6
	}
	return nil
}

var Subscribe_classifier_intf_qos_map_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	return Subscribe_qos_map_xfmr(inParams, "PORT_QOS_MAP")
}
