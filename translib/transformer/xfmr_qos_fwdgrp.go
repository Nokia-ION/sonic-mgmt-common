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

const (
	TcToQueueKey     = "global_tc_to_queue_map"
	TcToUcMcQueueKey = "global_tc_to_uc_mc_queue_map"
	QosFwdGroup      = "QOS_FWD_GROUP"
)

func init() {
	XlateFuncBind("YangToDb_qos_fwdgrp_xfmr", YangToDb_qos_fwdgrp_xfmr)
	XlateFuncBind("DbToYang_qos_fwdgrp_xfmr", DbToYang_qos_fwdgrp_xfmr)
	XlateFuncBind("Subscribe_qos_fwdgrp_xfmr", Subscribe_qos_fwdgrp_xfmr)
	XlateFuncBind("DbToYangPath_qos_fwdgrp_path_xfmr", DbToYangPath_qos_fwdgrp_path_xfmr)
}

func get_fwd_group_priority(inParams XfmrParams, name string) string {
	dbSpec := &db.TableSpec{Name: QosFwdGroup}
	keys, err := inParams.d.GetKeys(dbSpec)

	if err != nil {
		log.V(lvl.DEBUG).Info("Forwarding group table not found")
		/* Return empty string, invalid priority */
		return ""
	}

	for _, key := range keys {
		log.V(lvl.DEBUG).Info("fwd group key: ", key)
		if len(key.Comp) > 0 && key.Comp[0] == name {
			fwd_map, err := inParams.d.GetEntry(dbSpec, key)
			if err != nil {
				log.V(lvl.DEBUG).Info("fwd group key not found ", key)
				return ""
			}
			return fwd_map.Field["priority"]
		}
	}

	return ""
}

func get_queue_from_tc(d *db.DB, tc string) string {
	comp_key := db.Key{Comp: []string{TcToQueueKey}}
	tc_map, err := d.GetEntry(&db.TableSpec{Name: "TC_TO_QUEUE_MAP"}, comp_key)
	if err != nil {
		log.V(lvl.DEBUG).Info("tc map entry not found ", comp_key)
		return ""
	}

	q, err := getOCQueueName(FRONT_PANEL, tc_map.Field[tc])
	if err != nil {
		return ""
	}
	return q
}

func get_mc_queue_from_tc(d *db.DB, tc string) string {
	return get_queue_from_uc_mc_helper(d, tc, "MULTICAST")
}

func get_uc_queue_from_tc(d *db.DB, tc string) string {
	return get_queue_from_uc_mc_helper(d, tc, "UNICAST")
}

func get_queue_from_uc_mc_helper(d *db.DB, tc, prefix string) string {
	comp_key := db.Key{Comp: []string{TcToUcMcQueueKey}}
	tc_map, err := d.GetEntry(&db.TableSpec{Name: "TC_TO_UC_MC_QUEUE_MAP"}, comp_key)
	if err != nil {
		log.V(lvl.DEBUG).Info("tc map entry not found ", comp_key)
		return ""
	}

	prefixtc := strings.Join([]string{prefix, tc}, ":")
	q, err := getOCQueueName(FRONT_PANEL, tc_map.Field[prefixtc])
	if err != nil {
		return ""
	}
	return q
}

func get_fwd_group_from_priority(inParams XfmrParams, priority string) string {
	dbSpec := &db.TableSpec{Name: QosFwdGroup}

	keys, err := inParams.d.GetKeys(dbSpec)
	if err != nil {
		log.V(lvl.DEBUG).Info("Fwd group table not found")
		return ""
	}

	for _, key := range keys {
		log.V(lvl.DEBUG).Info("fwd group key: ", key)
		fwd_map, err := inParams.d.GetEntry(dbSpec, key)

		if err != nil {
			log.V(lvl.DEBUG).Info("Fwd group key not found")
			return ""
		}

		if len(key.Comp) > 0 && fwd_map.Field["priority"] == priority {
			return key.Comp[0]
		}
	}

	return ""
}

var Subscribe_qos_fwdgrp_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	return Subscribe_qos_map_xfmr(inParams, QosFwdGroup)
}

var YangToDb_qos_fwdgrp_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	log.V(lvl.DEBUG).Info("YangToDb_qos_fwdgrp_xfmr: ", inParams.ygRoot, inParams.uri)
	log.V(lvl.DEBUG).Info("inParams: ", inParams)

	if inParams.oper == DELETE {
		return qos_map_delete_xfmr(inParams, QosFwdGroup)
	}

	res_map := make(map[string]map[string]db.Value)

	pathInfo := NewPathInfo(inParams.uri)

	// parse the inParams
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		return res_map, tlerr.InvalidArgs("QoS obj is nil")
	}

	if qosObj.ForwardingGroups == nil || qosObj.ForwardingGroups.ForwardingGroup == nil {
		// Forwarding group is not present, nothing further to process
		return res_map, nil
	}
	var names []string
	fgname := pathInfo.Var("name")
	if fgname != "" {
		names = append(names, fgname)
	} else {
		for n, _ := range qosObj.ForwardingGroups.ForwardingGroup {
			names = append(names, n)
		}
	}

	fgrp_entry := make(map[string]db.Value)

	tc_queue_map := make(map[string]db.Value)
	tc_queue_map[TcToQueueKey] = db.Value{Field: make(map[string]string)}
	tc_queue_dbval := tc_queue_map[TcToQueueKey]

	tc_uc_mc_queue_map := make(map[string]db.Value)
	tc_uc_mc_queue_map[TcToUcMcQueueKey] = db.Value{Field: make(map[string]string)}
	tc_uc_mc_queue_dbval := tc_uc_mc_queue_map[TcToUcMcQueueKey]

	if inParams.d == nil {
		log.V(lvl.ERROR).Info("unable to get configDB")
		return res_map, tlerr.New("Database not available")
	}

	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	for _, name := range names {
		fwdGrp, ok := qosObj.ForwardingGroups.ForwardingGroup[name]
		if !ok || fwdGrp.Config == nil {
			// Nothing to write for this group. Another group in the same
			// request may still carry configuration, so keep going rather than
			// discarding what has been built so far.
			continue
		}
		cfg := fwdGrp.Config

		priority, err := qosFwdGrpPriority(inParams, name, cfg)
		if err != nil {
			return res_map, err
		}

		fgrp_entry[name] = db.Value{Field: make(map[string]string)}
		fgrp_entry[name].Field["priority"] = priority
		if cfg.OutputQueue != nil && *cfg.OutputQueue != "" {
			fgrp_entry[name].Field["queue"] = *cfg.OutputQueue
			err := prepareTcQueueMapDbVal(TcToQueueKey, *cfg.OutputQueue, priority, tc_queue_dbval)
			if err != nil {
				return nil, err
			}
		}

		if cfg.UnicastOutputQueue != nil && *cfg.UnicastOutputQueue != "" {
			fgrp_entry[name].Field["ucast_queue"] = *cfg.UnicastOutputQueue
			err := prepareTcToUcMcQueueMapDbVal("UNICAST", *cfg.UnicastOutputQueue, priority, tc_uc_mc_queue_dbval)
			if err != nil {
				return nil, err
			}
		}

		if cfg.MulticastOutputQueue != nil && *cfg.MulticastOutputQueue != "" {
			fgrp_entry[name].Field["mcast_queue"] = *cfg.MulticastOutputQueue
			err := prepareTcToUcMcQueueMapDbVal("MULTICAST", *cfg.MulticastOutputQueue, priority, tc_uc_mc_queue_dbval)
			if err != nil {
				return nil, err
			}
		}
	}

	if len(fgrp_entry) == 0 {
		return res_map, nil
	}

	// Apply the appropriate

	map_to_apply := TcToQueueKey
	if len(tc_queue_dbval.Field) > 0 {
		res_map["TC_TO_QUEUE_MAP"] = tc_queue_map
	}
	if len(tc_uc_mc_queue_dbval.Field) > 0 {
		map_to_apply = TcToUcMcQueueKey
		res_map["TC_TO_UC_MC_QUEUE_MAP"] = tc_uc_mc_queue_map
	}
	res_map["PORT_QOS_MAP"] = assignMapsToInterfaces(qosObj, map_to_apply, inParams)
	res_map[QosFwdGroup] = fgrp_entry
	log.V(lvl.DEBUG).Infof("YangToDb_qos_fwdgrp_xfmr returned res_map: %v", res_map)
	return res_map, nil
}

func assignMapsToInterfaces(qosObj *ocbinds.OpenconfigQos_Qos, tableKey string, inParams XfmrParams) map[string]db.Value {
	qosTblMap := make(map[string]db.Value)

	qosIntfsObj := qosObj.Interfaces
	if qosIntfsObj == nil || qosIntfsObj.Interface == nil {
		log.V(lvl.WARNING).Infof("Interface root object not found")
		return qosTblMap
	}
	// These map references are mutually exclusive,
	// only one should ever be present
	map_ptr_field := "tc_to_queue_map"
	map_ptr_field_to_del := "tc_to_uc_mc_queue_map"
	if tableKey == TcToUcMcQueueKey {
		map_ptr_field = "tc_to_uc_mc_queue_map"
		map_ptr_field_to_del = "tc_to_queue_map"
	}

	// Build a set of PORT_QOS_MAP keys to inform whether delete is necessary
	pqmKeyMap := map[string]struct{}{}
	keys, err := inParams.d.GetKeys(&db.TableSpec{Name: "PORT_QOS_MAP"})
	if err == nil {
		for _, key := range keys {
			pqmKeyMap[key.Comp[0]] = struct{}{}
		}
	}

	addDelMap := false
	delMap := make(map[string]map[string]db.Value)
	delMap["PORT_QOS_MAP"] = make(map[string]db.Value)
	for ifName, _ := range qosIntfsObj.Interface {
		// Add the desired reference
		qosMaps := make(map[string]string)
		qosMaps[map_ptr_field] = tableKey
		qosTblMap[ifName] = db.Value{Field: qosMaps}

		// Delete the other reference, but only if the entry exists
		if _, ok := pqmKeyMap[ifName]; ok {
			addDelMap = true
			delMap["PORT_QOS_MAP"][ifName] = db.Value{Field: map[string]string{
				map_ptr_field_to_del: "deleteme"}}
		}
	}
	if addDelMap {
		subOpMap := make(map[db.DBNum]map[string]map[string]db.Value)
		subOpMap[db.ConfigDB] = delMap
		updateSubOpDataMap(subOpMap, DELETE, inParams)
	}
	return qosTblMap
}

// qosFwdGrpPriority resolves the traffic class of a forwarding group.
// fabric-priority is the class number and the key of every queue map entry
// written for the group, so it is needed even by a request that only changes a
// queue. It is an optional leaf, so a client may leave it out when updating a
// group that already has one, and then the recorded class stands.
func qosFwdGrpPriority(inParams XfmrParams, name string,
	cfg *ocbinds.OpenconfigQos_Qos_ForwardingGroups_ForwardingGroup_Config) (string, error) {

	if cfg.FabricPriority != nil {
		return strconv.Itoa(int(*cfg.FabricPriority)), nil
	}
	if priority := get_fwd_group_priority(inParams, name); priority != "" {
		return priority, nil
	}
	return "", tlerr.InvalidArgsError{
		Format: "fabric-priority is required for forwarding group " + name}
}

func prepareTcQueueMapDbVal(table_key, qname string, priority string, dbval db.Value) error {
	// Since ACLs are used for CPU queue assignment,
	// we only need to consider FP ports here
	dbQueue, ok := ocQueueHwQueueMap[FRONT_PANEL][qname]
	if !ok {
		log.V(lvl.ERROR).Info("Unknown queue ", qname)
		return tlerr.New("Unknown queue " + qname)
	}

	dbval.Field[priority] = dbQueue
	return nil
}

func prepareTcToUcMcQueueMapDbVal(prefix, qname string, priority string, dbval db.Value) error {
	ok := false
	dbQueue := ""
	for _, qMapType := range queueTypes {
		if dbQueue, ok = ocQueueHwQueueMap[qMapType][qname]; ok {
			break
		}

	}
	if !ok {
		log.V(lvl.ERROR).Info("Unknown queue ", qname)
		return tlerr.New("Unknown queue " + qname)
	}
	prefixtc := strings.Join([]string{prefix, priority}, ":")
	dbval.Field[prefixtc] = dbQueue
	return nil
}

func fill_fwd_grp_info_by_name(inParams XfmrParams, fgrps *ocbinds.OpenconfigQos_Qos_ForwardingGroups, name string) error {
	fg, ok := fgrps.ForwardingGroup[name]
	if !ok {
		fg, _ = fgrps.NewForwardingGroup(name)
		ygot.BuildEmptyTree(fg)
		fg.Name = &name
	}

	if fg.Config == nil {
		fg.Config = &ocbinds.OpenconfigQos_Qos_ForwardingGroups_ForwardingGroup_Config{}
	}

	if fg.State == nil {
		fg.State = &ocbinds.OpenconfigQos_Qos_ForwardingGroups_ForwardingGroup_State{}
	}

	key := db.Key{Comp: []string{name}}

	dbSpec := &db.TableSpec{Name: QosFwdGroup}
	mapCfg, err := inParams.d.GetEntry(dbSpec, key)

	if err != nil {
		log.V(lvl.DEBUG).Info("No fwd group found : ", name)
		return tlerr.NotFoundError{Format: "Fwd group not found"}
	}

	fg.Config.Name = &name
	fg.State.Name = &name

	// Handle priority
	prioStr, ok := mapCfg.Field["priority"]
	if !ok {
		return tlerr.NotFound("priority missing")
	}
	if prio, err := strconv.ParseUint(prioStr, 10, 8); err == nil {
		prio8 := uint8(prio)
		fg.Config.FabricPriority = &prio8
		fg.State.FabricPriority = &prio8
	}

	queueStr, ok := mapCfg.Field["queue"]
	if ok {
		output_queue := queueStr
		fg.Config.OutputQueue = &output_queue
	}
	uqueueStr, ok := mapCfg.Field["ucast_queue"]
	if ok {
		unicast_output_queue := uqueueStr
		fg.Config.UnicastOutputQueue = &unicast_output_queue
	}
	mqueueStr, ok := mapCfg.Field["mcast_queue"]
	if ok {
		multicast_output_queue := mqueueStr
		fg.Config.MulticastOutputQueue = &multicast_output_queue
	}

	if queueStr != "" {
		oq := get_queue_from_tc(inParams.dbs[db.ApplDB], prioStr)
		fg.State.OutputQueue = &oq
	}
	if uqueueStr != "" {
		uoq := get_uc_queue_from_tc(inParams.dbs[db.ApplDB], prioStr)
		fg.State.UnicastOutputQueue = &uoq
	}
	if mqueueStr != "" {
		moq := get_mc_queue_from_tc(inParams.dbs[db.ApplDB], prioStr)
		fg.State.MulticastOutputQueue = &moq
	}

	return nil
}

var DbToYang_qos_fwdgrp_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	pathInfo := NewPathInfo(inParams.uri)
	name := pathInfo.Var("name")

	log.V(lvl.DEBUG).Info("inParams: ", inParams)
	log.V(lvl.DEBUG).Info("name: ", name)
	qosObj := getQosRoot(inParams.ygRoot)

	if qosObj == nil {
		ygot.BuildEmptyTree(qosObj)
	}

	if qosObj.ForwardingGroups == nil {
		ygot.BuildEmptyTree(qosObj.ForwardingGroups)
	}

	dbSpec := &db.TableSpec{Name: QosFwdGroup}

	map_added := 0
	keyPattern := "*"
	if name != "" {
		keyPattern = name
	}

	keys, err := inParams.d.GetKeysByPattern(dbSpec, keyPattern)

	if err != nil {
		log.V(lvl.ERROR).Info("Could not find fwd group table in DB")
		return err
	}

	for _, key := range keys {
		log.V(lvl.DEBUG).Info("key: ", key)

		if len(key.Comp) == 0 {
			log.V(lvl.ERROR).Infof("Invalid fwd group key in DB.")
			return tlerr.NotFoundError{Format: "Invalid fwd group key in DB"}
		}
		map_added++
		fill_fwd_grp_info_by_name(inParams, qosObj.ForwardingGroups, key.Comp[0])
	}

	if name != "" && map_added == 0 {
		log.V(lvl.ERROR).Info("Resource not found.")
		return tlerr.NotFound("Resource not found")
	}

	return nil
}

var DbToYangPath_qos_fwdgrp_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	log.V(lvl.DEBUG).Infof("DbToYangPath_qos_fwdgrp_path_xfmr: yangPath %v tblKeyComp %v", inParams.yangPath, inParams.tblKeyComp)

	if len(inParams.tblKeyComp) != 1 {
		return fmt.Errorf("DbToYangPath_qos_fwdgrp_path_xfmr: Invalid tblKey %v or tblEntry %v", inParams.tblKeyComp, inParams.tblEntry)
	}

	inParams.ygPathKeys["/openconfig-qos:qos/forwarding-groups/forwarding-group/name"] = inParams.tblKeyComp[0]
	return nil
}
