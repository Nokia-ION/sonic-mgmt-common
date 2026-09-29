package transformer

import (
	"errors"
	"math"
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
	qosUmfWredTbl = "QOS_UMF_WRED_PROFILE"

	// WRED_MAX_THRESHOLD in qos_queue_map.json is the largest WRED threshold,
	// in bytes, the ASIC takes. Broadcom counts thresholds in buffer cells, so
	// its limit is the cell size times the largest cell count.
	WRED_MAX_THRESHOLD = "wred_max_threshold"
)

// qosWredMaxThreshold is the largest threshold WRED_PROFILE can carry: what
// the platform declares, and never more than orchagent, which parses the
// thresholds as int, can read.
func qosWredMaxThreshold() uint64 {
	if v, ok := qMapStr[WRED_MAX_THRESHOLD].(float64); ok && v >= 1 && v <= math.MaxInt32 {
		return uint64(v)
	}
	return math.MaxInt32
}

// qosWredThreshold is the most of an OpenConfig threshold the platform can
// take. featureprofiles sends 2^64-1 to mean no upper bound, which the largest
// threshold the ASIC has stands for.
func qosWredThreshold(th string) string {
	limit := qosWredMaxThreshold()
	v, err := strconv.ParseUint(th, 10, 64)
	if err != nil || v > limit {
		v = limit
	}
	return strconv.FormatUint(v, 10)
}

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

// The profile lookups run inside a request, which holds the ConfigDB lock, so
// they read through the request's own ConfigDB handle rather than opening a
// second connection, which would fail and make every profile look absent.

func GetQueueManagementProfilesByName(d *db.DB, name string) []string {
	var queueManagementProfilesList []string

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

// getQueuesByQueueManagementProfileName lists the QUEUE keys, "<ifname>|<qid>",
// whose wred_profile is name.
func getQueuesByQueueManagementProfileName(d *db.DB, name string) []string {
	var s []string

	log.V(lvl.DEBUG).Info("name ", name)
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
			if wredProfile, ok := qCfg.Field["wred_profile"]; ok && wredProfile == name && key.Len() == 2 {
				s = append(s, key.Get(0)+"|"+key.Get(1))
			}
		}
	}

	return s
}

func isQueueManagementProfileActive(d *db.DB, name string) bool {
	// read queues refering to the scheduler profile
	queues := getQueuesByQueueManagementProfileName(d, name)
	if len(queues) == 0 {
		log.V(lvl.DEBUG).Info("No active user of the queue management profile", name)
		return false
	}

	log.V(lvl.DEBUG).Info("queue management profile in active use!")
	return true
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
		queueManagementProfiles = GetQueueManagementProfilesByName(inParams.d, name)
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

	if name == "" {
		return qos_queue_management_profile_delete_by_name(inParams, name)
	}
	/*
	 * A leaf of a profile has no value to fall back to in WRED_PROFILE, which
	 * needs every threshold, so only the whole profile can be deleted.
	 */
	if targetUriPath != "/openconfig-qos:qos/queue-management-profiles/queue-management-profile" {
		return resMap, tlerr.NotSupported("Queue management profile %v: deleting part of a profile is not supported, replace or delete the whole profile", name)
	}

	queueManagementProfileKey := name
	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_xfmr - entry_key : ", queueManagementProfileKey)
	resMap["WRED_PROFILE"] = map[string]db.Value{queueManagementProfileKey: {Field: make(map[string]string)}}
	qosUmfWredDelete(inParams, resMap, []string{queueManagementProfileKey})

	/* update "QUEUE" table for to-be-deleted queue management profile if it is used */
	rtTblMap := make(map[string]db.Value)
	keys := getQueuesByQueueManagementProfileName(inParams.d, queueManagementProfileKey)
	for _, key := range keys {
		log.V(lvl.DEBUG).Infof("qos_queue_management_profile_delete_xfmr: key: %v, profile_name: %v", key, queueManagementProfileKey)
		rtTblMap[key] = db.Value{Field: map[string]string{"wred_profile": ""}}
	}
	if len(keys) != 0 {
		resMap["QUEUE"] = rtTblMap
	}

	log.V(lvl.DEBUG).Infof("qos_queue_management_profile_delete_xfmr --> resMap %v", resMap)
	return resMap, err
}

// qosUmfWredDelete removes the QOS_UMF_WRED_PROFILE entries of the profiles
// being deleted, where there are any.
func qosUmfWredDelete(inParams XfmrParams, resMap map[string]map[string]db.Value, names []string) {
	ts := &db.TableSpec{Name: qosUmfWredTbl}
	for _, name := range names {
		if _, err := inParams.d.GetEntry(ts, db.Key{Comp: []string{name}}); err != nil {
			continue
		}
		if resMap[qosUmfWredTbl] == nil {
			resMap[qosUmfWredTbl] = make(map[string]db.Value)
		}
		resMap[qosUmfWredTbl][name] = db.Value{Field: make(map[string]string)}
	}
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
	queueManagementProfileKeys := GetQueueManagementProfilesByName(inParams.d, "")

	// A DELETE of /qos removes the front panel queues too, so only a CPU queue keeps a profile in use.
	reqPath, _ := getYangPathFromUri(inParams.requestUri)
	wholeQos := reqPath == "/openconfig-qos:qos"

	/* update "WRED_PROFILE" table */
	queueManagementProfileEntry := make(map[string]db.Value)
	var queue_management_profile_del bool = false
	for _, queueManagementProfileKey := range queueManagementProfileKeys {
		inUse := false
		for _, q := range getQueuesByQueueManagementProfileName(inParams.d, queueManagementProfileKey) {
			if !wholeQos || strings.HasPrefix(q, CPU+"|") {
				inUse = true
				break
			}
		}
		if inUse {
			continue
		}
		queue_management_profile_del = true
		queueManagementProfileEntry[queueManagementProfileKey] = db.Value{Field: make(map[string]string)}
	}

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_all ")
	if queue_management_profile_del {
		resMap["WRED_PROFILE"] = queueManagementProfileEntry
		deleted := make([]string, 0, len(queueManagementProfileEntry))
		for key := range queueManagementProfileEntry {
			deleted = append(deleted, key)
		}
		qosUmfWredDelete(inParams, resMap, deleted)
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
	if isQueueManagementProfileActive(inParams.d, name) {
		err = tlerr.InternalError{Format: "Disallow to delete an active queue management profile"}
		log.V(lvl.DEBUG).Info("Disallow to delete an active queue management profile: ", name)
		return resMap, err
	}

	/* get all matching queue management profiles */
	queueManagementProfileKeys := GetQueueManagementProfilesByName(inParams.d, name)

	/* update "WRED_PROFILE" table */
	queueManagementProfileEntry := make(map[string]db.Value)

	for _, queueManagementProfileKey := range queueManagementProfileKeys {
		queueManagementProfileEntry[queueManagementProfileKey] = db.Value{Field: make(map[string]string)}
	}

	log.V(lvl.DEBUG).Info("qos_queue_management_profile_delete_by_name - : ", name)
	resMap["WRED_PROFILE"] = queueManagementProfileEntry
	qosUmfWredDelete(inParams, resMap, queueManagementProfileKeys)

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
		if queueManagementProfileObj.Wred == nil || queueManagementProfileObj.Wred.Uniform == nil ||
			queueManagementProfileObj.Wred.Uniform.Config == nil {
			return nil, tlerr.InvalidArgs("Queue management profile %v has no wred/uniform/config", queueManagementProfileName)
		}
		uniform := queueManagementProfileObj.Wred.Uniform.Config
		/*
		 * WRED_PROFILE has no averaging weight; the ASIC's default of 0, the
		 * instantaneous queue depth, is what applies.
		 */
		if uniform.Weight != nil && *uniform.Weight != 0 {
			return nil, tlerr.NotSupported("Queue management profile %v: WRED weight %v is not supported, only 0",
				queueManagementProfileName, *uniform.Weight)
		}
		/*
		 * A write that names one leaf, or updates the profile, keeps the
		 * leaves it leaves out.
		 */
		umfVal := db.Value{Field: make(map[string]string)}
		if inParams.oper != REPLACE || strings.Contains(inParams.requestUri, "/uniform/config/") {
			if prev, err := inParams.d.GetEntry(&db.TableSpec{Name: qosUmfWredTbl},
				db.Key{Comp: []string{queueManagementProfileKey}}); err == nil {
				for field, val := range prev.Field {
					if field != "NULL" {
						umfVal.Field[field] = val
					}
				}
			}
		}
		if uniform.MinThreshold != nil {
			umfVal.Field["min_threshold"] = strconv.FormatUint(*uniform.MinThreshold, 10)
		}
		if uniform.MaxThreshold != nil {
			umfVal.Field["max_threshold"] = strconv.FormatUint(*uniform.MaxThreshold, 10)
		}
		if uniform.MaxDropProbabilityPercent != nil {
			umfVal.Field["max_drop_probability_percent"] = strconv.Itoa(int(*uniform.MaxDropProbabilityPercent))
		}
		if uniform.EnableEcn != nil {
			umfVal.Field["enable_ecn"] = strconv.FormatBool(*uniform.EnableEcn)
		}
		if uniform.Drop != nil {
			umfVal.Field["drop"] = strconv.FormatBool(*uniform.Drop)
		}

		/*
		 * SAI marks an ECN capable packet where the WRED of its colour would
		 * drop it, so marking needs WRED on as much as dropping does.
		 */
		enableEcn := umfVal.Field["enable_ecn"] == "true"
		queueManagementProfileVal.Field["ecn"] = "ecn_none"
		if enableEcn {
			queueManagementProfileVal.Field["ecn"] = "ecn_all"
		}
		wredOn := strconv.FormatBool(enableEcn || umfVal.Field["drop"] == "true")
		for _, color := range []string{"green", "yellow", "red"} {
			queueManagementProfileVal.Field["wred_"+color+"_enable"] = wredOn
			if th, ok := umfVal.Field["min_threshold"]; ok {
				queueManagementProfileVal.Field[color+"_min_threshold"] = qosWredThreshold(th)
			}
			if th, ok := umfVal.Field["max_threshold"]; ok {
				queueManagementProfileVal.Field[color+"_max_threshold"] = qosWredThreshold(th)
			}
			if prob, ok := umfVal.Field["max_drop_probability_percent"]; ok {
				queueManagementProfileVal.Field[color+"_drop_probability"] = prob
			}
		}
		/*
		 * Earlier images put ECN settings in ecn_mark fields, which orchagent
		 * rejects the whole profile for. A replace drops them with the rest of
		 * the old entry; an update has to remove them itself.
		 */
		if inParams.oper != REPLACE {
			if prev, err := inParams.d.GetEntry(&db.TableSpec{Name: "WRED_PROFILE"},
				db.Key{Comp: []string{queueManagementProfileKey}}); err == nil {
				stale := db.Value{Field: make(map[string]string)}
				for field := range prev.Field {
					if strings.Contains(field, "_ecn_mark_") {
						stale.Field[field] = ""
					}
				}
				if len(stale.Field) > 0 {
					qosStageDelete(inParams, "WRED_PROFILE", queueManagementProfileKey, stale)
				}
			}
		}
		queueManagementProfileEntry := make(map[string]db.Value)
		queueManagementProfileEntry[queueManagementProfileKey] = queueManagementProfileVal
		log.V(lvl.DEBUG).Info("YangToDb_qos_queue_management_profile_xfmr - entry_key : ", queueManagementProfileKey)
		resMap["WRED_PROFILE"] = queueManagementProfileEntry
		resMap[qosUmfWredTbl] = map[string]db.Value{queueManagementProfileKey: umfVal}

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

// qosWredAttr reads the uniform settings out of a WRED_PROFILE entry, which is
// all there is of a profile configured outside OpenConfig.
func qosWredAttr(entry db.Value) QueueManagementProfileAttr {
	var attr QueueManagementProfileAttr
	for _, color := range []string{"red", "yellow", "green"} {
		setIfPresentUInt64(&attr.minThreshold, entry, color+"_min_threshold")
		setIfPresentUInt64(&attr.maxThreshold, entry, color+"_max_threshold")
		setIfPresentUInt8(&attr.maxDropProbabilityPercent, entry, color+"_drop_probability")
	}
	ecn := entry.Field["ecn"]
	attr.enableECN = ecn != "" && ecn != "ecn_none"
	attr.drop = !attr.enableECN && (entry.Field["wred_green_enable"] == "true" ||
		entry.Field["wred_yellow_enable"] == "true" || entry.Field["wred_red_enable"] == "true")
	return attr
}

// qosOverlayUmfWred puts back what WRED_PROFILE cannot hold: a threshold
// beyond what orchagent takes, and drop next to enable-ecn.
func qosOverlayUmfWred(attr *QueueManagementProfileAttr, umf db.Value) {
	setIfPresentUInt64(&attr.minThreshold, umf, "min_threshold")
	setIfPresentUInt64(&attr.maxThreshold, umf, "max_threshold")
	setIfPresentUInt8(&attr.maxDropProbabilityPercent, umf, "max_drop_probability_percent")
	if val, ok := umf.Field["enable_ecn"]; ok {
		attr.enableECN = val == "true"
	}
	if val, ok := umf.Field["drop"]; ok {
		attr.drop = val == "true"
	}
}

func getQueueManagementProfileAttrFromDb(queueManagementProfileCfg db.Value, queueManagementProfileState db.Value, umf db.Value) (QueueManagementProfileAttr, QueueManagementProfileAttr, error) {
	cfg := qosWredAttr(queueManagementProfileCfg)
	state := qosWredAttr(queueManagementProfileState)
	qosOverlayUmfWred(&cfg, umf)
	qosOverlayUmfWred(&state, umf)
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
	const qmpPath = "/openconfig-qos:qos/queue-management-profiles/queue-management-profile"
	var queue_management_profile_config, queue_management_profile_state bool
	var uniform_config, uniform_state bool
	switch {
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/queue-management-profiles") == 0,
		strings.Compare(targetUriPath, qmpPath) == 0:
		queue_management_profile_config, queue_management_profile_state = true, true
		uniform_config, uniform_state = true, true
	case strings.HasPrefix(targetUriPath, qmpPath+"/config"):
		queue_management_profile_config = true
	case strings.HasPrefix(targetUriPath, qmpPath+"/state"):
		queue_management_profile_state = true
	case targetUriPath == qmpPath+"/wred" || targetUriPath == qmpPath+"/wred/uniform":
		uniform_config, uniform_state = true, true
	case strings.HasPrefix(targetUriPath, qmpPath+"/wred/uniform/config"):
		uniform_config = true
	case strings.HasPrefix(targetUriPath, qmpPath+"/wred/uniform/state"):
		uniform_state = true
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
		/*
		 * A platform whose qosorch consumes CONFIG_DB directly keeps no APPL_DB
		 * copy of the profile, so the configured one is what is in effect.
		 */
		queueManagementProfileState, errState := queueManagementProfileConfig, errCfg
		if applDb := inParams.dbs[db.ApplDB]; applDb != nil {
			if entry, err := applDb.GetEntry(dbSpec, key); err == nil {
				queueManagementProfileState, errState = entry, nil
			}
		}
		if errCfg != nil && errState != nil {
			continue
		}
		umf, _ := inParams.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: qosUmfWredTbl}, key)
		queueManagementProfileCfg, queueManagementProfileInfo, err := getQueueManagementProfileAttrFromDb(queueManagementProfileConfig, queueManagementProfileState, umf)
		if err == nil {
			var weight uint32
			if uniform_config {
				ygot.BuildEmptyTree(queueManagementProfileObj.Wred.Uniform)
				queueManagementProfileObj.Wred.Uniform.Config.MaxDropProbabilityPercent = &queueManagementProfileCfg.maxDropProbabilityPercent
				queueManagementProfileObj.Wred.Uniform.Config.MinThreshold = &queueManagementProfileCfg.minThreshold
				queueManagementProfileObj.Wred.Uniform.Config.MaxThreshold = &queueManagementProfileCfg.maxThreshold
				queueManagementProfileObj.Wred.Uniform.Config.EnableEcn = &queueManagementProfileCfg.enableECN
				queueManagementProfileObj.Wred.Uniform.Config.Drop = &queueManagementProfileCfg.drop
				queueManagementProfileObj.Wred.Uniform.Config.Weight = &weight
			}
			if uniform_state {
				ygot.BuildEmptyTree(queueManagementProfileObj.Wred.Uniform)
				queueManagementProfileObj.Wred.Uniform.State.MaxDropProbabilityPercent = &queueManagementProfileInfo.maxDropProbabilityPercent
				queueManagementProfileObj.Wred.Uniform.State.MinThreshold = &queueManagementProfileInfo.minThreshold
				queueManagementProfileObj.Wred.Uniform.State.MaxThreshold = &queueManagementProfileInfo.maxThreshold
				queueManagementProfileObj.Wred.Uniform.State.EnableEcn = &queueManagementProfileInfo.enableECN
				queueManagementProfileObj.Wred.Uniform.State.Drop = &queueManagementProfileInfo.drop
				queueManagementProfileObj.Wred.Uniform.State.Weight = &weight
			}
		}
	}
	return nil
}
