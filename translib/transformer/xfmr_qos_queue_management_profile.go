package transformer

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

type QueueManagementProfileAttr struct {
	minThreshold              uint64
	maxThreshold              uint64
	enableECN                 bool
	drop                      bool
	maxDropProbabilityPercent uint8
}

func init() {
	XlateFuncBind("YangToDb_qos_queue_management_profile_xfmr", YangToDb_qos_queue_management_profile_xfmr)
	XlateFuncBind("DbToYang_qos_queue_management_profile_xfmr", DbToYang_qos_queue_management_profile_xfmr)
	XlateFuncBind("Subscribe_qos_queue_management_profile_xfmr", Subscribe_qos_queue_management_profile_xfmr)
	parseQueueMapJSONFile()
}

func Subscribe_qos_queue_management_profile_xfmr(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	name := NewPathInfo(inParams.uri).Var("name")
	if name == "" {
		name = "*"
	}
	log.V(lvl.DEBUG).Info("Subscribe_qos_queue_management_profile_xfmr name (key): ", name)
	return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {"WRED_PROFILE": {name: {}}}}}, nil
}

func getIntfsByQueueManagementProfileName(queueManagementProfileName string, inParams XfmrParams) []string {
	log.V(lvl.DEBUG).Info("queueManagementProfileName ", queueManagementProfileName)
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil || qosObj.Interfaces == nil || qosObj.Interfaces.Interface == nil {
		return []string{}
	}
	var s []string
	for intfName := range qosObj.Interfaces.Interface {
		intfObj, ok := qosObj.Interfaces.Interface[intfName]
		if !ok {
			log.V(lvl.DEBUG).Info("getIntfsByQueueManagementProfileName No interface object: ", queueManagementProfileName)
			continue
		}
		if intfObj.Output == nil {
			continue
		}
		if intfObj.Output.Queues == nil {
			continue
		}
		if intfObj.Output.Queues.Queue == nil {
			continue
		}
		for queueName := range intfObj.Output.Queues.Queue {
			queueObj, ok := intfObj.Output.Queues.Queue[queueName]
			if !ok {
				continue
			}
			if queueObj.Config == nil {
				continue
			}
			if queueObj.Config.Name == nil {
				continue
			}
			if strings.Compare(queueManagementProfileName, *queueObj.Config.Name) != 0 {
				continue
			}
			s = append(s, intfName)
		}
	}
	return s
}

func GetQueueManagementProfilesByName(name string) []string {
	var queueManagementProfilesList []string

	d, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		log.V(lvl.DEBUG).Infof("GetQueueManagementProfilesByName, unable to get configDB, error %v", err)
		return queueManagementProfilesList
	}

	defer d.DeleteDB()
	ts := &db.TableSpec{Name: "WRED_PROFILE"}
	keyPattern := name
	if name == "" {
		keyPattern = "*"
	}

	keys, err := d.GetKeysByPattern(ts, keyPattern)
	if err != nil {
		log.V(lvl.DEBUG).Infof("GetQueueManagementProfilesByName: no matching keys for ", keyPattern)
		return queueManagementProfilesList
	}

	for _, key := range keys {
		if name == "" || key.Comp[0] == name {
			queueManagementProfilesList = append(queueManagementProfilesList, key.Comp[0])
		}
	}

	log.V(lvl.DEBUG).Info("matching queue management profiles: ", queueManagementProfilesList)
	return queueManagementProfilesList
}

func getQueuesByQueueManagementProfileName(name string) []string {
	var s []string

	log.V(lvl.DEBUG).Info("name ", name)
	d, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		log.V(lvl.ERROR).Infof("getQueuesByQueueManagementProfileName, unable to get configDB, error %v", err)
		return s
	}
	defer d.DeleteDB()

	tblList := []string{"QUEUE"}
	for _, tblName := range tblList {
		dbSpec := &db.TableSpec{Name: tblName}
		keys, err := d.GetKeys(dbSpec)
		if err != nil {
			continue
		}
		for _, key := range keys {
			qCfg, err := d.GetEntry(dbSpec, key)
			if err != nil {
				continue
			}
			wredProfile, ok := qCfg.Field["wred-profile"]
			if !ok {
				continue
			}
			wredProfile = wredProfile
			if wredProfile == name {
				queueName := key.Get(0)
				s = append(s, queueName)
			}
		}
	}

	return s
}

func isQueueManagementProfileActive(name string) bool {
	// read queues refering to the scheduler profile
	queues := getQueuesByQueueManagementProfileName(name)
	if len(queues) == 0 {
		log.V(lvl.DEBUG).Info("No active user of the queue management profile", name)
		return false
	}

	log.V(lvl.DEBUG).Info("queue management profile in active use!")
	return true
}

func isQueueManagementProfileField(queueManagementProfileKey string, attr string) bool {
	d, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		log.V(lvl.ERROR).Infof("isQueueManagementProfileField, unable to get configDB, error %v", err)
		return false
	}

	defer d.DeleteDB()
	ts := &db.TableSpec{Name: "WRED_PROFILE"}
	entry, err := d.GetEntry(ts, db.Key{Comp: []string{queueManagementProfileKey}})
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Info("err in getting queue management profile entry: ", queueManagementProfileKey)
		return false
	}

	if len(entry.Field) == 1 {
		_, ok := entry.Field[attr]
		return ok
	}

	return false
}

func qos_queue_management_profile_delete_xfmr(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	var err error
	resMap := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_xfmr: ", inParams.ygRoot, inParams.uri)
	pathInfo := NewPathInfo(inParams.uri)
	name := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Info("Error getting yang path from uri: ", inParams.uri)
		return resMap, err
	}

	var queueManagementProfiles []string
	if name != "" {
		queueManagementProfiles = GetQueueManagementProfilesByName(name)
		if len(queueManagementProfiles) == 0 {
			err = tlerr.InternalError{Format: "Instance Not found"}
			log.V(lvl.ERROR).Info("Queue Management Profiles not found.")
			return resMap, err
		}
	}

	if !strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/queue-management-profiles/queue-management-profile") {
		log.V(lvl.DEBUG).Info("YangToDb: queue management profile name unspecified, using delete_by_name")
		return qos_queue_management_profile_delete_by_name(inParams, name)
	}

	/* update "WRED" table */
	queueManagementProfileEntry := make(map[string]db.Value)
	queueManagementProfileKey := name
	queueManagementProfileEntry[queueManagementProfileKey] = db.Value{Field: make(map[string]string)}

	if targetUriPath == "/openconfig-qos:qos/queue-management-profiles/queue-management-profile/wred/uniform/config/min-threshold" {
		log.V(lvl.DEBUG).Info("Handling No min-threshold")
		if isQueueManagementProfileActive(queueManagementProfileKey) &&
			isQueueManagementProfileField(queueManagementProfileKey, "min-threshold") {
			err = tlerr.InternalError{Format: "Last queue management profile used by interface cannot be deleted"}
			log.V(lvl.DEBUG).Info("Not allow the last field to be deleted")
			log.V(lvl.DEBUG).Info("Disallow to delete the last queue management profile in an actively used policy: ", queueManagementProfileKey)
			return resMap, err
		}
		queueManagementProfileEntry[queueManagementProfileKey].Field["min-threshold"] = "0"
	}

	if targetUriPath == "/openconfig-qos:qos/queue-management-profiles/queue-management-profile/wred/uniform/config/max-threshold" {
		log.V(lvl.DEBUG).Info("Handling No max-threshold")
		if isQueueManagementProfileActive(queueManagementProfileKey) &&
			isQueueManagementProfileField(queueManagementProfileKey, "max-threshold") {
			err = tlerr.InternalError{Format: "Last queue management profile used by interface cannot be deleted"}
			log.V(lvl.DEBUG).Info("Not allow the last field to be deleted")
			log.V(lvl.DEBUG).Info("Disallow to delete the last queue management profile in an actively used policy: ", queueManagementProfileKey)
			return resMap, err
		}
		queueManagementProfileEntry[queueManagementProfileKey].Field["max-threshold"] = "0"
	}

	if targetUriPath == "/openconfig-qos:qos/queue-management-profiles/queue-management-profile/wred/uniform/config/max-drop-probability-percent" {
		log.V(lvl.DEBUG).Info("Handling No max-drop-probability-percent")
		if isQueueManagementProfileActive(queueManagementProfileKey) &&
			isQueueManagementProfileField(queueManagementProfileKey, "max-drop-probability-percent") {
			err = tlerr.InternalError{Format: "Last queue management profile used by interface cannot be deleted"}
			log.V(lvl.DEBUG).Info("Not allow the last field to be deleted")
			log.V(lvl.DEBUG).Info("Disallow to delete the last queue management profile in an actively used policy: ", queueManagementProfileKey)
			return resMap, err
		}
		queueManagementProfileEntry[queueManagementProfileKey].Field["max-drop-probability-percent"] = "0"
	}

	if targetUriPath == "/openconfig-qos:qos/queue-management-profiles/queue-management-profile/wred/uniform/config/enable-ecn" {
		log.V(lvl.DEBUG).Info("Handling No enable-ecn")
		if isQueueManagementProfileActive(queueManagementProfileKey) &&
			isQueueManagementProfileField(queueManagementProfileKey, "enable-ecn") {
			err = tlerr.InternalError{Format: "Last queue management profile used by interface cannot be deleted"}
			log.V(lvl.DEBUG).Info("Not allow the last field to be deleted")
			log.V(lvl.DEBUG).Info("Disallow to delete the last queue management profile in an actively used policy: ", queueManagementProfileKey)
			return resMap, err
		}
		queueManagementProfileEntry[queueManagementProfileKey].Field["enable-ecn"] = "false"
	}

	if targetUriPath == "/openconfig-qos:qos/queue-management-profiles/queue-management-profile/wred/uniform/config/drop" {
		log.V(lvl.DEBUG).Info("Handling No drop")
		if isQueueManagementProfileActive(queueManagementProfileKey) &&
			isQueueManagementProfileField(queueManagementProfileKey, "drop") {
			err = tlerr.InternalError{Format: "Last queue management profile used by interface cannot be deleted"}
			log.V(lvl.DEBUG).Info("Not allow the last field to be deleted")
			log.V(lvl.DEBUG).Info("Disallow to delete the last queue management profile in an actively used policy: ", queueManagementProfileKey)
			return resMap, err
		}
		queueManagementProfileEntry[queueManagementProfileKey].Field["drop"] = "false"
	}

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_xfmr - entry_key : ", queueManagementProfileKey)
	resMap["WRED_PROFILE"] = queueManagementProfileEntry

	/* update "QUEUE" table for to-be-deleted queue management profile if it is used */
	rtTblMap := make(map[string]db.Value)

	if targetUriPath == "/openconfig-qos:qos/queue-management-profiles/queue-management-profile" {

		// read queues referring to the queue management profile in db
		keys := getQueuesByQueueManagementProfileName(queueManagementProfileKey)
		for _, key := range keys {
			log.V(lvl.DEBUG).Infof("qos_queue_management_profile_delete_xfmr: key: %v, profile_name: %v", key, queueManagementProfileKey)
			_, ok := rtTblMap[key]
			if !ok {
				rtTblMap[key] = db.Value{Field: make(map[string]string)}
			}
			rtTblMap[key].Field["wred-profile"] = ""
		}

		if len(keys) != 0 {
			resMap["QUEUE"] = rtTblMap
		}
	}

	log.V(lvl.DEBUG).Infof("qos_queue_management_profile_delete_xfmr --> resMap %v", resMap)
	return resMap, err
}

func qos_queue_management_profile_delete_all(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	var err error
	resMap := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_all: ", inParams.ygRoot, inParams.uri)
	if _, err = getYangPathFromUri(inParams.uri); err != nil {
		log.V(lvl.ERROR).Info("Unable to get yang path from uri ", inParams.uri)
		return resMap, err
	}

	/* get all matching queue management profiles */
	queueManagementProfileKeys := GetQueueManagementProfilesByName("")

	/* update "WRED_PROFILE" table */
	queueManagementProfileEntry := make(map[string]db.Value)
	var queue_management_profile_del bool = false
	for _, queueManagementProfileKey := range queueManagementProfileKeys {
		if isQueueManagementProfileActive(queueManagementProfileKey) {
			continue
		}
		queue_management_profile_del = true
		queueManagementProfileEntry[queueManagementProfileKey] = db.Value{Field: make(map[string]string)}
	}

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_all ")
	if queue_management_profile_del {
		resMap["WRED_PROFILE"] = queueManagementProfileEntry
	}

	// no need to clean Queue DB as only unused queue management profile is allowed to be deleted
	return resMap, err
}

func qos_queue_management_profile_delete_by_name(inParams XfmrParams, name string) (map[string]map[string]db.Value, error) {
	var err error
	resMap := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_by_name: ", inParams.ygRoot, inParams.uri)
	if name == "" {
		return qos_queue_management_profile_delete_all(inParams)
	}

	if _, err = getYangPathFromUri(inParams.uri); err != nil {
		log.V(lvl.ERROR).Info("Unable to get yang path from uri ", inParams.uri)
		return resMap, err
	}

	// validation
	if isQueueManagementProfileActive(name) {
		err = tlerr.InternalError{Format: "Disallow to delete an active queue management profile"}
		log.V(lvl.DEBUG).Info("Disallow to delete an active queue management profile: ", name)
		return resMap, err
	}

	/* get all matching queue management profiles */
	queueManagementProfileKeys := GetQueueManagementProfilesByName(name)

	/* update "WRED_PROFILE" table */
	queueManagementProfileEntry := make(map[string]db.Value)

	for _, queueManagementProfileKey := range queueManagementProfileKeys {
		queueManagementProfileEntry[queueManagementProfileKey] = db.Value{Field: make(map[string]string)}
	}

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_by_name - : ", name)
	resMap["WRED_PROFILE"] = queueManagementProfileEntry

	// no need to clean Queue DB as only unused queue management profile is allowed to be deleted
	return resMap, err
}

var YangToDb_qos_queue_management_profile_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	if inParams.oper == DELETE {
		return qos_queue_management_profile_delete_xfmr(inParams)
	}
	log.V(lvl.DEBUG).Info("YangToDb_qos_queue_management_profile_xfmr: ", inParams.ygRoot, inParams.uri)

	pathInfo := NewPathInfo(inParams.uri)
	queueManagementProfileName := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Info("error parsing targetUriPath")
		return resMap, nil
	}

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		return nil, errors.New("No qos tree populated")
	}

	if qosObj.QueueManagementProfiles == nil || qosObj.QueueManagementProfiles.QueueManagementProfile == nil || len(qosObj.QueueManagementProfiles.QueueManagementProfile) < 1 {
		return nil, errors.New("No queue management profile subtree populated")
	}
	queueManagementProfileObj, ok := qosObj.QueueManagementProfiles.QueueManagementProfile[queueManagementProfileName]
	if !ok {
		log.V(lvl.DEBUG).Info("YangToDb: No queue management profile name: ", queueManagementProfileName)
		return resMap, nil
	}

	if !strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/queue-management-profiles/queue-management-profile") {
		log.V(lvl.DEBUG).Info("YangToDb: queue management profile unspecified, stop here")
		return resMap, nil
	}

	queueManagementProfileKey := queueManagementProfileName
	queueManagementProfileVal := db.Value{Field: make(map[string]string)}

	if (inParams.oper == CREATE) || (inParams.oper == REPLACE) || (inParams.oper == UPDATE) {
		var queue_id string
		if queueManagementProfileObj.Wred.Uniform.Config.Drop != nil && ((bool)(*queueManagementProfileObj.Wred.Uniform.Config.Drop)) {
			queueManagementProfileVal.Field["ecn"] = "ecn_none"
			queueManagementProfileVal.Field["yellow_drop_probability"] = strconv.Itoa((int)(*queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent))
			queueManagementProfileVal.Field["red_drop_probability"] = strconv.Itoa((int)(*queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent))
			queueManagementProfileVal.Field["green_drop_probability"] = strconv.Itoa((int)(*queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent))
			queueManagementProfileVal.Field["yellow_min_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MinThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["yellow_max_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["red_min_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MinThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["red_max_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["green_min_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MinThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["green_max_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["wred_yellow_enable"] = "true"
			queueManagementProfileVal.Field["wred_green_enable"] = "true"
			queueManagementProfileVal.Field["wred_red_enable"] = "true"
		}
		if queueManagementProfileObj.Wred.Uniform.Config.EnableEcn != nil && ((bool)(*queueManagementProfileObj.Wred.Uniform.Config.EnableEcn)) {
			queueManagementProfileVal.Field["ecn"] = "ecn_all"
			queueManagementProfileVal.Field["yellow_ecn_mark_probability"] = strconv.Itoa((int)(*queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent))
			queueManagementProfileVal.Field["red_ecn_mark_probability"] = strconv.Itoa((int)(*queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent))
			queueManagementProfileVal.Field["green_ecn_mark_probability"] = strconv.Itoa((int)(*queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent))
			queueManagementProfileVal.Field["yellow_ecn_mark_min_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MinThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["yellow_ecn_mark_max_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["red_ecn_mark_min_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MinThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["red_ecn_mark_max_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["green_ecn_mark_min_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MinThreshold), 'f', -1, 64)
			queueManagementProfileVal.Field["green_ecn_mark_max_threshold"] = strconv.FormatFloat((float64)(*queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold), 'f', -1, 64)
		}
		queueManagementProfileEntry := make(map[string]db.Value)
		queueManagementProfileEntry[queueManagementProfileKey] = queueManagementProfileVal
		log.V(lvl.DEBUG).Info("YangToDb_qos_queue_management_profile_xfmr - entry_key : ", queueManagementProfileKey)
		resMap["WRED_PROFILE"] = queueManagementProfileEntry

		/* update "Queue" table for newly created queue management profile if the queue management profile is used by intfs*/
		queueTblMap := make(map[string]db.Value)

		if inParams.oper == CREATE || inParams.oper == REPLACE || inParams.oper == UPDATE {
			intfs := getIntfsByQueueManagementProfileName(queueManagementProfileName, inParams)
			for _, if_name := range intfs {
				key := if_name + "|" + queue_id
				db_queueManagementProfileName := queueManagementProfileName
				log.V(lvl.DEBUG).Info("YangToDb_qos_queue_management_profile_xfmr --> key: %v, db_queueManagementProfileName: %v", key, db_queueManagementProfileName)
				if _, ok := queueTblMap[key]; !ok {
					queueTblMap[key] = db.Value{Field: make(map[string]string)}
				}
				queueTblMap[key].Field["wred_profile"] = db_queueManagementProfileName
			}
			resMap["QUEUE"] = queueTblMap
		}
	}
	return resMap, nil
}

func setIfPresentUInt64(field *uint64, value db.Value, name string) {
	if val, exist := value.Field[name]; exist {
		tmp, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			log.V(lvl.ERROR).Info("Unable to parse the %v value", name)
		} else {
			*field = uint64(tmp)
		}
	}
}

func setIfPresentUInt8(field *uint8, value db.Value, name string) {
	if val, exist := value.Field[name]; exist {
		tmp, err := strconv.ParseUint(val, 10, 8)
		if err != nil {
			log.V(lvl.ERROR).Info("Unable to parse the %v value", name)
		} else {
			*field = uint8(tmp)
		}
	}
}

func getQueueManagementProfileAttrFromDb(queueManagementProfileCfg db.Value, queueManagementProfileState db.Value) (QueueManagementProfileAttr, QueueManagementProfileAttr, error) {
	var cfg, state QueueManagementProfileAttr

	cfg.enableECN = false
	cfg.drop = true
	setIfPresentUInt64(&cfg.minThreshold, queueManagementProfileCfg, "yellow_min_threshold")
	setIfPresentUInt64(&cfg.maxThreshold, queueManagementProfileCfg, "yellow_max_threshold")
	setIfPresentUInt8(&cfg.maxDropProbabilityPercent, queueManagementProfileCfg, "yellow_drop_probability")

	state.enableECN = false
	state.drop = true
	setIfPresentUInt64(&state.minThreshold, queueManagementProfileState, "yellow_min_threshold")
	setIfPresentUInt64(&state.maxThreshold, queueManagementProfileState, "yellow_max_threshold")
	setIfPresentUInt8(&state.maxDropProbabilityPercent, queueManagementProfileState, "yellow_drop_probability")
	log.V(lvl.DEBUG).Infoln("max drop probability percent ", state.maxDropProbabilityPercent)

	if ecn, exist := queueManagementProfileCfg.Field["ecn"]; exist {
		if ecn == "ecn_all" {
			cfg.enableECN = true
			cfg.drop = false
			setIfPresentUInt64(&cfg.minThreshold, queueManagementProfileCfg, "yellow_ecn_mark_min_threshold")
			setIfPresentUInt64(&cfg.maxThreshold, queueManagementProfileCfg, "yellow_ecn_mark_max_threshold")
			setIfPresentUInt8(&cfg.maxDropProbabilityPercent, queueManagementProfileCfg, "yellow_ecn_mark_probability")
		}
	}
	if ecn, exist := queueManagementProfileState.Field["ecn"]; exist {
		if ecn == "ecn_all" {
			state.enableECN = true
			state.drop = false
			setIfPresentUInt64(&state.minThreshold, queueManagementProfileState, "yellow_ecn_mark_min_threshold")
			setIfPresentUInt64(&state.maxThreshold, queueManagementProfileState, "yellow_ecn_mark_max_threshold")
			setIfPresentUInt8(&state.maxDropProbabilityPercent, queueManagementProfileState, "yellow_ecn_mark_probability")
			log.V(lvl.DEBUG).Infoln("max drop probability percent ", state.maxDropProbabilityPercent)
		}
	}
	return cfg, state, nil
}

var DbToYang_qos_queue_management_profile_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infoln("DbToYang_qos_queue_management_profile_xfmr - inParams.uri: ", inParams.uri)
	pathInfo := NewPathInfo(inParams.uri)
	queueManagementProfileName := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Infoln("error parsing targetUriPath")
		return err
	}
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		ygot.BuildEmptyTree(qosObj)
	}
	var queue_management_profile_config, queue_management_profile_state bool
	switch {
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/queue-management-profiles") == 0:
		queue_management_profile_config, queue_management_profile_state = true, true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/queue-management-profiles/queue-management-profile") == 0:
		queue_management_profile_config, queue_management_profile_state = true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/queue-management-profiles/queue-management-profile/config"):
		queue_management_profile_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/queue-management-profiles/queue-management-profile/state"):
		queue_management_profile_state = true
	default:
		errStr := "Invalid URI"
		log.V(lvl.ERROR).Info(errStr)
		return tlerr.InvalidArgsError{Format: errStr}
	}

	var keyPattern string
	dbSpec := &db.TableSpec{Name: "WRED_PROFILE"}
	keyPattern = "*"
	if queueManagementProfileName != "" {
		keyPattern = queueManagementProfileName
	}

	keys, err := inParams.dbs[db.ConfigDB].GetKeysByPattern(dbSpec, keyPattern)
	if err != nil {
		log.V(lvl.ERROR).Info("Unable to get the data from the CONFIG DB")
		return err
	}
	for _, key := range keys {
		log.V(lvl.DEBUG).Info("current key: ", key)
		if len(key.Comp) < 1 {
			continue
		}
		queue_management_profile_name := key.Comp[0]
		if queueManagementProfileName != "" && strings.Compare(queueManagementProfileName, queue_management_profile_name) != 0 {
			continue
		}
		log.V(lvl.DEBUG).Info("Fill queue management profile ", queue_management_profile_name)
		var queueManagementProfileObj *ocbinds.OpenconfigQos_Qos_QueueManagementProfiles_QueueManagementProfile
		var ok bool
		if qosObj.QueueManagementProfiles.QueueManagementProfile == nil {
			queueManagementProfileObj, err = qosObj.QueueManagementProfiles.NewQueueManagementProfile(queue_management_profile_name)
			if err != nil {
				log.V(lvl.DEBUG).Info("Unable to create qos queue management profile object for ", queue_management_profile_name)
				continue
			}
		}
		queueManagementProfileObj, ok = qosObj.QueueManagementProfiles.QueueManagementProfile[queue_management_profile_name]
		if !ok {
			queueManagementProfileObj, err = qosObj.QueueManagementProfiles.NewQueueManagementProfile(queue_management_profile_name)
			if err != nil {
				log.V(lvl.DEBUG).Info("Unable to create qos queue management profile object for ", queue_management_profile_name)
				continue
			}
		}
		ygot.BuildEmptyTree(queueManagementProfileObj)
		if queueManagementProfileObj.Wred == nil {
			ygot.BuildEmptyTree(queueManagementProfileObj.Wred)
		}
		if queueManagementProfileObj.Wred.Uniform == nil {
			ygot.BuildEmptyTree(queueManagementProfileObj.Wred.Uniform)
		}
		if queue_management_profile_config {
			ygot.BuildEmptyTree(queueManagementProfileObj.Config)
			queueManagementProfileObj.Name = &queue_management_profile_name
			queueManagementProfileObj.Config.Name = &queue_management_profile_name
		}
		if queue_management_profile_state {
			ygot.BuildEmptyTree(queueManagementProfileObj.State)
			queueManagementProfileObj.Name = &queue_management_profile_name
			queueManagementProfileObj.State.Name = &queue_management_profile_name
		}

		queueManagementProfileConfig, errCfg := inParams.dbs[db.ConfigDB].GetEntry(dbSpec, key)
		if errCfg != nil {
			log.V(lvl.DEBUG).Info("Unable to get the data from the CONFIG DB")
		}
		queueManagementProfileState, errState := inParams.dbs[db.ApplDB].GetEntry(dbSpec, key)
		if errState != nil {
			log.V(lvl.DEBUG).Info("Unable to get the data from the APPL STATE DB")
		}
		if errCfg != nil && errState != nil {
			continue
		}
		queueManagementProfileCfg, queueManagementProfileInfo, err := getQueueManagementProfileAttrFromDb(queueManagementProfileConfig, queueManagementProfileState)
		if err == nil {
			if queue_management_profile_config {
				queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent = &queueManagementProfileCfg.maxDropProbabilityPercent
				queueManagementProfileObj.Wred.Uniform.Config.MinThreshold = &queueManagementProfileCfg.minThreshold
				queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold = &queueManagementProfileCfg.maxThreshold
				queueManagementProfileObj.Wred.Uniform.Config.EnableEcn = &queueManagementProfileCfg.enableECN
				queueManagementProfileObj.Wred.Uniform.Config.Drop = &queueManagementProfileCfg.drop
			}
			if queue_management_profile_state {
				queueManagementProfileObj.Wred.Uniform.State.MaxDropProbabilityPercent = &queueManagementProfileInfo.maxDropProbabilityPercent
				queueManagementProfileObj.Wred.Uniform.State.MinThreshold = &queueManagementProfileInfo.minThreshold
				queueManagementProfileObj.Wred.Uniform.State.MaxThreshold = &queueManagementProfileInfo.maxThreshold
				queueManagementProfileObj.Wred.Uniform.State.EnableEcn = &queueManagementProfileInfo.enableECN
				queueManagementProfileObj.Wred.Uniform.State.Drop = &queueManagementProfileInfo.drop
			}
		}
	}
	return nil
}
