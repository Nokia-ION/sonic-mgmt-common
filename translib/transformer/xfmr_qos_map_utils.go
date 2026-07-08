package transformer

import (
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
)

func get_map_entry_by_map_name(d *db.DB, map_type string, map_name string) (db.Value, error) {

	ts := &db.TableSpec{Name: map_type}
	if log.V(lvl.DEBUG) {
		keys, _ := d.GetKeys(ts)

		log.V(lvl.DEBUG).Info("keys: ", keys)
	}
	entry, err := d.GetEntry(ts, db.Key{Comp: []string{map_name}})
	if err != nil {
		log.V(lvl.DEBUG).Info("not able to find the map entry in DB ", map_name)
		return entry, err
	}

	return entry, nil
}

var qos_map_oc_yang_key_map = map[string]string{
	"DSCP_TO_TC_MAP":            "dscp",
	"DOT1P_TO_TC_MAP":           "dot1p",
	"TC_TO_QUEUE_MAP":           "fwd-group",
	"TC_TO_PRIORITY_GROUP_MAP":  "fwd-group",
	"MAP_PFC_PRIORITY_TO_QUEUE": "dot1p",
	"TC_TO_DOT1P_MAP":           "fwd-group",
	"TC_TO_DSCP_MAP":            "fwd-group",
	"QOS_FWD_GROUP":             "name",
}

func targetUriPathContainsMapName(uri string, map_type string) bool {
	if map_type == "DSCP_TO_TC_MAP" &&
		strings.HasPrefix(uri, "/openconfig-qos:qos/dscp-maps/dscp-map") {
		return true
	}

	if map_type == "DOT1P_TO_TC_MAP" &&
		strings.HasPrefix(uri, "/openconfig-qos:qos/dot1p-maps/dot1p-map") {
		return true
	}

	if map_type == "TC_TO_QUEUE_MAP" &&
		strings.HasPrefix(uri, "/openconfig-qos:qos/forwarding-group-queue-maps/forwarding-group-queue-map") {
		return true
	}

	if map_type == "TC_TO_PRIORITY_GROUP_MAP" &&
		strings.HasPrefix(uri, "/openconfig-qos:qos/forwarding-group-priority-group-maps/forwarding-group-priority-group-map") {
		return true
	}

	if map_type == "MAP_PFC_PRIORITY_TO_QUEUE" &&
		strings.HasPrefix(uri, "/openconfig-qos:qos/pfc-priority-queue-maps/pfc-priority-queue-map") {
		return true
	}

	if map_type == "TC_TO_DOT1P_MAP" &&
		strings.HasPrefix(uri, "/openconfig-qos:qos/forwarding-group-dot1p-maps/forwarding-group-dot1p-map") {
		return true
	}

	if map_type == "TC_TO_DSCP_MAP" &&
		strings.HasPrefix(uri, "/openconfig-qos:qos/forwarding-group-dscp-maps/forwarding-group-dscp-map") {
		return true
	}

	return false

}

func targetUriPathIsAllMapEntry(uri string, map_type string) bool {
	if map_type == "DSCP_TO_TC_MAP" &&
		strings.HasSuffix(uri, "dscp-map-entries") {
		return true
	}

	if map_type == "DOT1P_TO_TC_MAP" &&
		strings.HasSuffix(uri, "dot1p-map-entries") {
		return true
	}

	if map_type == "TC_TO_QUEUE_MAP" &&
		strings.HasSuffix(uri, "forwarding-group-queue-map-entries") {
		return true
	}

	if map_type == "TC_TO_PRIORITY_GROUP_MAP" &&
		strings.HasSuffix(uri, "forwarding-group-priority-group-map-entries") {
		return true
	}

	if map_type == "MAP_PFC_PRIORITY_TO_QUEUE" &&
		strings.HasSuffix(uri, "pfc-priority-queue-map-entries") {
		return true
	}

	if map_type == "TC_TO_DOT1P_MAP" &&
		strings.HasSuffix(uri, "forwarding-group-dot1p-map-entries") {
		return true
	}

	if map_type == "TC_TO_DSCP_MAP" &&
		strings.HasSuffix(uri, "forwarding-group-dscp-map-entries") {
		return true
	}

	return false
}

func qos_map_delete_xfmr(inParams XfmrParams, map_type string) (map[string]map[string]db.Value, error) {
	var err error
	res_map := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("qos_map_delete_xfmr: ", inParams.ygRoot, inParams.uri)
	log.V(lvl.DEBUG).Info("inParams: ", inParams)

	pathInfo := NewPathInfo(inParams.uri)
	map_name := pathInfo.Var("name")
	log.V(lvl.DEBUG).Info("YangToDb: map name: ", map_name)

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	log.V(lvl.DEBUG).Info("targetUriPath: ", targetUriPath)

	var map_entry db.Value

	if map_name != "" {
		map_entry, err = get_map_entry_by_map_name(inParams.d, map_type, map_name)
		if err != nil {
			err = tlerr.NotFoundError{Format: "Resource not found"}
			log.V(lvl.ERROR).Info("map name not found.")
			return res_map, err
		}
	}

	if !targetUriPathContainsMapName(targetUriPath, map_type) {
		log.V(lvl.DEBUG).Info("YangToDb: map name unspecified, using delete_by_map_name")
		return qos_map_delete_by_map_name(inParams, map_type, map_name)
	}

	entry_key := pathInfo.Var(qos_map_oc_yang_key_map[map_type])
	if entry_key == "" {

		if targetUriPathIsAllMapEntry(targetUriPath, map_type) {
			// delete all map entries
			map_del := make(map[string]map[string]db.Value)
			map_del[map_type] = make(map[string]db.Value)
			key := map_name
			var value db.Value
			map_del[map_type][key] = value

			subOpMap := make(map[db.DBNum]map[string]map[string]db.Value)
			subOpMap[db.ConfigDB] = map_del
			log.V(lvl.DEBUG).Info("subOpMap: ", subOpMap)
			inParams.subOpDataMap[REPLACE] = &subOpMap
			return res_map, err
		}

		log.V(lvl.DEBUG).Info("YangToDb: map key field unspecified, using delete_by_map_name")
		return qos_map_delete_by_map_name(inParams, map_type, map_name)
	} else {
		_, exist := map_entry.Field[entry_key]
		if !exist {
			err = tlerr.NotFoundError{Format: "Resource not found"}
			log.V(lvl.ERROR).Info("Field Name value not found.", entry_key)
			return res_map, err
		}
	}

	/* update "map" table field only */
	rtTblMap := make(map[string]db.Value)
	rtTblMap[map_name] = db.Value{Field: make(map[string]string)}
	rtTblMap[map_name].Field[entry_key] = ""

	res_map[map_type] = rtTblMap

	return res_map, err

}

func qos_map_delete_all_map(inParams XfmrParams, map_type string) (map[string]map[string]db.Value, error) {
	var err error
	res_map := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("qos_map_delete_all_map: ", inParams.ygRoot, inParams.uri)
	log.V(lvl.DEBUG).Info("inParams: ", inParams)

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	log.V(lvl.DEBUG).Info("targetUriPath: ", targetUriPath)

	ts := &db.TableSpec{Name: map_type}
	keys, _ := inParams.d.GetKeys(ts)

	log.V(lvl.DEBUG).Info("keys: ", keys)
	/* update "map" table */
	rtTblMap := make(map[string]db.Value)

	for _, key := range keys {
		// validation: skip in-used map

		map_name := key.Comp[0]

		rtTblMap[map_name] = db.Value{Field: make(map[string]string)}
	}

	log.V(lvl.DEBUG).Info("qos_map_delete_all_map ")
	res_map[map_type] = rtTblMap

	return res_map, err
}

func qos_map_delete_by_map_name(inParams XfmrParams, map_type string, map_name string) (map[string]map[string]db.Value, error) {
	var err error
	res_map := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("qos_map_delete_by_map_name: ", inParams.ygRoot, inParams.uri)
	log.V(lvl.DEBUG).Info("inParams: ", inParams)
	log.V(lvl.DEBUG).Info("map_name: ", map_name)

	if map_name == "" {
		return qos_map_delete_all_map(inParams, map_type)
	}

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	log.V(lvl.DEBUG).Info("targetUriPath: ", targetUriPath)

	/* update "map" table */
	rtTblMap := make(map[string]db.Value)
	rtTblMap[map_name] = db.Value{Field: make(map[string]string)}

	log.V(lvl.DEBUG).Info("qos_map_delete_by_map_name - : ", map_type, map_name)
	res_map[map_type] = rtTblMap

	return res_map, err
}

var map_type_name_in_db = map[string]string{
	"DSCP_TO_TC_MAP":            "dscp_to_tc_map",
	"DOT1P_TO_TC_MAP":           "dot1p_to_tc_map",
	"TC_TO_QUEUE_MAP":           "tc_to_queue_map",
	"TC_TO_PRIORITY_GROUP_MAP":  "tc_to_pg_map",
	"MAP_PFC_PRIORITY_TO_QUEUE": "pfc_to_queue_map",
	"TC_TO_DOT1P_MAP":           "tc_to_dot1p_map",
	"TC_TO_DSCP_MAP":            "tc_to_dscp_map",
}

func Subscribe_qos_map_xfmr(inParams XfmrSubscInParams, map_type string) (XfmrSubscOutParams, error) {
	name := NewPathInfo(inParams.uri).Var("name")
	if name == "" {
		name = "*"
	}
	log.V(lvl.DEBUG).Info("XfmrSubscribe_qos_map_xfmr map_type (DB name): ", map_type, " name (key): ", name)
	return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {map_type: {name: {}}}}}, nil
}
