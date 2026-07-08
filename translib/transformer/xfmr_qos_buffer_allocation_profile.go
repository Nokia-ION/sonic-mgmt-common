package transformer

import (
	"errors"
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

type bufferAllocationProfileAttr struct {
	dedicatedBuffer           uint64
	useSharedBuffer           bool
	sharedBufferLimitType     string
	staticSharedBufferLimit   uint32
	dynamicLimitScalingFactor int32
	bufferPoolName            string
	xon                       uint64
	xoff                      uint64
	xonOffset                 uint64
}

func init() {
	XlateFuncBind("YangToDb_qos_buffer_allocation_profile_xfmr", YangToDb_qos_buffer_allocation_profile_xfmr)
	XlateFuncBind("DbToYang_qos_buffer_allocation_profile_xfmr", DbToYang_qos_buffer_allocation_profile_xfmr)
	XlateFuncBind("Subscribe_qos_buffer_allocation_profile_xfmr", Subscribe_qos_buffer_allocation_profile_xfmr)
	XlateFuncBind("DbToYangPath_qos_buffer_allocation_profile_path_xfmr", DbToYangPath_qos_buffer_allocation_profile_path_xfmr)
	parseQueueMapJSONFile()
}

const (
	QUEUE_TO_BUF_PROFILE_MAP = "QUEUE_TO_BUF_PROFILE_MAP"
)

func Subscribe_qos_buffer_allocation_profile_xfmr(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	pathInfo := NewPathInfo(inParams.uri)
	name := pathInfo.Var("name")
	name = name + "*"
	log.V(lvl.DEBUG).Info("XfmrSubscribe_qos_buffer_allocation_profile_xfmr")
	result := XfmrSubscOutParams{
		dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {"BUFFER_PROFILE": {name: {}}}}, // tablename & table-idx for the inParams.uri
		needCache: true,
		nOpts:     &notificationOpts{mInterval: 0, pType: OnChange},
	}
	log.V(lvl.DEBUG).Info("Returning Subscribe_qos_buffer_allocation_profile_xfmr")
	return result, nil
}

func GetIntfsByBufferAllocationProfileName(bufferAllocationProfileName string, inParams XfmrParams) []string {
	log.V(lvl.DEBUG).Info("bufferAllocationProfileName ", bufferAllocationProfileName)
	qos := getQosRoot(inParams.ygRoot)
	if qos == nil || qos.Interfaces == nil {
		return []string{}
	}
	var s []string
	for intfName, intf := range qos.Interfaces.Interface {
		if intf.Output == nil || intf.Output.Config == nil {
			continue
		}
		if strings.Compare(bufferAllocationProfileName, *intf.Output.Config.BufferAllocationProfile) != 0 {
			continue
		}
		s = append(s, intfName)
	}
	return s
}

func GetBufferAllocationProfilesByName(name string) []string {
	var bufferAllocationProfilesList []string
	log.V(lvl.DEBUG).Infof("GetBufferAllocationProfilesByName: ", name)

	d, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		log.V(lvl.ERROR).Infof("GetBufferAllocationProfilesByName, unable to get configDB, error %v", err)
		return bufferAllocationProfilesList
	}

	defer d.DeleteDB()
	ts := &db.TableSpec{Name: "BUFFER_PROFILE"}
	keyPattern := name
	if keyPattern == "" {
		keyPattern = "*"
	}

	keys, err := d.GetKeysByPattern(ts, keyPattern)
	log.V(lvl.DEBUG).Infof("GetBufferAllocationProfilesByName: looking for matching keys for ", keyPattern)
	if err != nil {
		log.V(lvl.ERROR).Infof("GetBufferAllocationProfilesByName: no matching keys for ", keyPattern)
		return bufferAllocationProfilesList
	}

	for _, key := range keys {
		bufferAllocationProfilesList = append(bufferAllocationProfilesList, key.Comp[0])
	}

	log.V(lvl.DEBUG).Info("matching buffer allocation profiles: ", bufferAllocationProfilesList)
	return bufferAllocationProfilesList
}

func GetQueuesByBufferAllocationProfileName(name string) []string {
	var s []string

	log.V(lvl.DEBUG).Info("name ", name)
	d, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		log.V(lvl.ERROR).Infof("GetQueuesByBufferAllocationProfileName, unable to get configDB, error %v", err)
		return s
	}
	defer d.DeleteDB()

	tblList := []string{"BUFFER_QUEUE"}
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
			bufferAllocationProfile, ok := qCfg.Field["profile"]
			if !ok {
				continue
			}
			bufferAllocationProfile = bufferAllocationProfile
			if key.Len() < 2 {
				continue
			}
			if bufferAllocationProfile == name {
				queueName, err := getOCQueueName(key.Get(0), key.Get(1))
				if err != nil {
					continue
				}
				s = append(s, queueName)
				log.V(lvl.DEBUG).Infof("GetQueuesByBufferAllocationProfileName: ", queueName)
			}
		}
	}

	return s
}

func isBufferAllocationProfileActive(name string) bool {
	// read queues refering to the buffer allocation profile
	if queues := GetQueuesByBufferAllocationProfileName(name); len(queues) == 0 {
		log.V(lvl.DEBUG).Info("No active user of the buffer allocation profile", name)
		return false
	}

	log.V(lvl.DEBUG).Info("buffer allocation profile in active use!")
	return true
}

func isBufferAllocationProfileField(bufferAllocationProfileKey string, attr string) bool {
	d, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		log.V(lvl.ERROR).Infof("isBufferAllocationProfileField, unable to get configDB, error %v", err)
		return false
	}

	defer d.DeleteDB()
	ts := &db.TableSpec{Name: "BUFFER_PROFILE"}
	entry, err := d.GetEntry(ts, db.Key{Comp: []string{bufferAllocationProfileKey}})
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Info("err in getting buffer allocation profile entry: ", bufferAllocationProfileKey)
		return false
	}

	if len(entry.Field) > 0 {
		_, ok := entry.Field[attr]
		return ok
	}

	return false
}

func QosBufferAllocationProfileDeleteXfmr(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Info("QosBufferAllocationProfileDeleteXfmr: ", inParams.ygRoot, inParams.uri)
	pathInfo := NewPathInfo(inParams.uri)
	name := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Info("Error getting yang path from uri: ", inParams.uri)
		return resMap, err
	}

	var bufferAllocationProfiles []string
	if name != "" {
		name = name + "*"
		bufferAllocationProfiles = GetBufferAllocationProfilesByName(name)
		if len(bufferAllocationProfiles) == 0 {
			log.V(lvl.DEBUG).Info("Buffer Allocation Profiles not found matching ", name)
			return resMap, tlerr.InternalError{Format: "Buffer Allocation Profiles Not found"}
		}
	}

	if !strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile") {
		log.V(lvl.DEBUG).Info("YangToDb: buffer allocation profile name unspecified, using delete by name")
		return QosBufferAllocationProfileDeleteByName(inParams, name)
	}

	/* update "BUFFER_PROFILE" table */
	bufferAllocationProfileEntry := make(map[string]db.Value)

	for _, bufferAllocationProfileKey := range bufferAllocationProfiles {
		bufferAllocationProfileEntry[bufferAllocationProfileKey] = db.Value{Field: make(map[string]string)}
		switch targetUriPath {
		case "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile/queues/queue/config/dedicated-buffer":
			if isBufferAllocationProfileActive(bufferAllocationProfileKey) &&
				isBufferAllocationProfileField(bufferAllocationProfileKey, "dedicated-buffer") {
				log.V(lvl.DEBUG).Info("Disallow to delete the last buffer allocation profile in an actively used policy: ", bufferAllocationProfileKey)
				return resMap, tlerr.InternalError{Format: "Last buffer allocation profile used by interface cannot be deleted"}
			}
			bufferAllocationProfileEntry[bufferAllocationProfileKey].Field["dedicated_buffer"] = "0"
		case "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile/queues/queue/config/static-shared-buffer-limit":
			if isBufferAllocationProfileActive(bufferAllocationProfileKey) &&
				isBufferAllocationProfileField(bufferAllocationProfileKey, "static-shared-buffer-limit") {
				log.V(lvl.DEBUG).Info("Disallow to delete the last buffer allocation profile in an actively used policy: ", bufferAllocationProfileKey)
				return resMap, tlerr.InternalError{Format: "Last buffer allocation profile used by interface cannot be deleted"}
			}
			bufferAllocationProfileEntry[bufferAllocationProfileKey].Field["static_th"] = "0"
		case "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile/queues/queue/config/dynamic-limit-scaling-factor":
			if isBufferAllocationProfileActive(bufferAllocationProfileKey) &&
				isBufferAllocationProfileField(bufferAllocationProfileKey, "dynamic-th") {
				log.V(lvl.DEBUG).Info("Disallow to delete the last buffer allocation profile in an actively used policy: ", bufferAllocationProfileKey)
				return resMap, tlerr.InternalError{Format: "Last buffer allocation profile used by interface cannot be deleted"}
			}
			bufferAllocationProfileEntry[bufferAllocationProfileKey].Field["dynamic_th"] = "0"
		default:
		}
		log.V(lvl.DEBUG).Info("QosBufferAllocationProfileDeleteXfmr - entry_key : ", bufferAllocationProfileKey)
	}

	resMap["BUFFER_PROFILE"] = bufferAllocationProfileEntry
	log.V(lvl.DEBUG).Infof("QosBufferAllocationProfileDeleteXfmr --> resMap %v", resMap)

	return resMap, nil
}

func QosBufferAllocationProfileDeleteAll(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("QosBufferAllocationProfileDeleteAll: ", inParams.ygRoot, inParams.uri)
	if _, err := getYangPathFromUri(inParams.uri); err != nil {
		log.V(lvl.ERROR).Info("Unable to get yang path from uri ", inParams.uri)
		return resMap, err
	}

	/* get all matching queue management profiles */
	bufferAllocationProfileKeys := GetBufferAllocationProfilesByName("")

	/* update "BUFFER_PROFILE" table */
	bufferAllocationProfileEntry := make(map[string]db.Value)
	buffer_allocation_profile_del := false
	for _, bufferAllocationProfileKey := range bufferAllocationProfileKeys {
		if isBufferAllocationProfileActive(bufferAllocationProfileKey) {
			continue
		}
		buffer_allocation_profile_del = true
		bufferAllocationProfileEntry[bufferAllocationProfileKey] = db.Value{Field: make(map[string]string)}
	}

	if buffer_allocation_profile_del {
		resMap["BUFFER_PROFILE"] = bufferAllocationProfileEntry
	}

	log.V(lvl.DEBUG).Info("QosBufferAllocationProfileDeleteAll ")
	return resMap, nil
}

func QosBufferAllocationProfileDeleteByName(inParams XfmrParams, name string) (map[string]map[string]db.Value, error) {
	var err error
	resMap := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("QosBufferAllocationProfileDeleteByName: ", inParams.ygRoot, inParams.uri)
	if name == "" {
		return QosBufferAllocationProfileDeleteAll(inParams)
	}

	if _, err = getYangPathFromUri(inParams.uri); err != nil {
		log.V(lvl.ERROR).Info("Unable to get yang path from uri ", inParams.uri)
		return resMap, err
	}

	// validation
	if isBufferAllocationProfileActive(name) {
		log.V(lvl.ERROR).Info("Deletion of an active buffer allocation profile is disallowed: ", name)
		return resMap, tlerr.InternalError{Format: "Deletion of an active buffer allocation profile is disallowed"}
	}

	/* get all matching buffer allocation profiles */
	bufferAllocationProfileKeys := GetBufferAllocationProfilesByName(name)
	bufferAllocationProfileEntry := make(map[string]db.Value)

	for _, bufferAllocationProfileKey := range bufferAllocationProfileKeys {
		bufferAllocationProfileEntry[bufferAllocationProfileKey] = db.Value{Field: make(map[string]string)}
	}

	resMap["BUFFER_PROFILE"] = bufferAllocationProfileEntry
	log.V(lvl.DEBUG).Info("QosBufferAllocationProfileDeleteByName - : ", name)
	return resMap, nil
}

var YangToDb_qos_buffer_allocation_profile_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	if inParams.oper == DELETE {
		return QosBufferAllocationProfileDeleteXfmr(inParams)
	}
	log.V(lvl.DEBUG).Info("YangToDb_qos_buffer_allocation_profile_xfmr: ", inParams.ygRoot, inParams.uri)

	pathInfo := NewPathInfo(inParams.uri)
	bufferAllocationProfileName := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Info("error parsing targetUriPath")
		return resMap, nil
	}

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		return nil, errors.New("No qos tree populated")
	}

	if qosObj.BufferAllocationProfiles == nil || qosObj.BufferAllocationProfiles.BufferAllocationProfile == nil || len(qosObj.BufferAllocationProfiles.BufferAllocationProfile) < 1 {
		return nil, errors.New("No buffer allocation profile subtree populated")
	}
	bufferAllocationProfileObj, ok := qosObj.BufferAllocationProfiles.BufferAllocationProfile[bufferAllocationProfileName]
	if !ok {
		log.V(lvl.DEBUG).Info("YangToDb: No buffer allocation profile name: ", bufferAllocationProfileName)
		return resMap, nil
	}

	if !strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile") {
		log.V(lvl.DEBUG).Info("YangToDb: buffer allocation profile unspecified, stop here")
		return resMap, nil
	}

	if (inParams.oper == CREATE) || (inParams.oper == REPLACE) || (inParams.oper == UPDATE) {
		bufferProfileTblMap := make(map[string]db.Value)
		bufferQueueTblMap := make(map[string]db.Value)
		queueToBufferProfileMap := make(map[string]db.Value)
		queueToBufferProfileMap[bufferAllocationProfileName] = db.Value{Field: make(map[string]string)}
		if bufferAllocationProfileObj.Queues == nil || bufferAllocationProfileObj.Queues.Queue == nil || len(bufferAllocationProfileObj.Queues.Queue) < 1 {
			return nil, errors.New("No queue in buffer allocation profile subtree populated")
		}
		for queueName := range bufferAllocationProfileObj.Queues.Queue {
			bufferProfileKey := bufferAllocationProfileName
			bufferProfileVal := db.Value{Field: make(map[string]string)}
			queueObj, ok := bufferAllocationProfileObj.Queues.Queue[queueName]
			if !ok || queueObj.Config == nil {
				continue
			}

			// Provide a default (from the default configuration)
			poolName := "default_egress"
			bufferProfileVal.Field["pool"] = poolName
			bufferProfileKey += "." + poolName

			if queueObj.Config.DedicatedBuffer != nil {
				dedicatedBufferSize := strconv.Itoa((int)(*queueObj.Config.DedicatedBuffer))
				bufferProfileVal.Field["size"] = dedicatedBufferSize
				bufferProfileKey += "." + dedicatedBufferSize
			}
			if queueObj.Config.UseSharedBuffer != nil && *queueObj.Config.UseSharedBuffer {
				if queueObj.Config.SharedBufferLimitType == ocbinds.OpenconfigQos_SHARED_BUFFER_LIMIT_TYPE_STATIC &&
					queueObj.Config.StaticSharedBufferLimit != nil {
					staticSharedBufferLimit := strconv.Itoa((int)(*queueObj.Config.StaticSharedBufferLimit))
					bufferProfileVal.Field["static_th"] = staticSharedBufferLimit
					bufferProfileKey += ".s" + staticSharedBufferLimit
				} else if queueObj.Config.SharedBufferLimitType == ocbinds.OpenconfigQos_SHARED_BUFFER_LIMIT_TYPE_DYNAMIC_BASED_ON_SCALING_FACTOR &&
					queueObj.Config.DynamicLimitScalingFactor != nil {
					dynamicLimitScalingFactor := strconv.Itoa((int)(*queueObj.Config.DynamicLimitScalingFactor))
					bufferProfileVal.Field["dynamic_th"] = dynamicLimitScalingFactor
					bufferProfileKey += ".d" + dynamicLimitScalingFactor
				}
			}
			// Only add this profile if it isn't already present
			if _, ok := bufferProfileTblMap[bufferProfileKey]; !ok {
				bufferProfileTblMap[bufferProfileKey] = bufferProfileVal
				log.V(lvl.DEBUG).Info("YangToDb_qos_buffer_allocation_profile_xfmr adding entry_key : ", bufferProfileKey)
			}
			queueToBufferProfileMap[bufferAllocationProfileName].Field[queueName] = bufferProfileKey
			log.V(lvl.DEBUG).Info("YangToDb_qos_buffer_allocation_profile_xfmr - entry_key : ", bufferProfileKey)

			// update "BUFFER_QUEUE" table for newly created buffer allocation profile, if the buffer allocation profile is used by intfs
			// Skip for ingress buffer allocation profile
			if !isPriorityGroupBufferAllocationProfile(bufferAllocationProfileName) {
				intfName := FRONT_PANEL
				if strings.HasPrefix(bufferAllocationProfileName, "cpu") {
					intfName = CPU
				}
				qidStr, err := getNativeQueueName(intfName, queueName)
				if err != nil {
					continue
				}
				intfs := GetIntfsByBufferAllocationProfileName(bufferAllocationProfileName, inParams)
				for _, if_name := range intfs {
					key := if_name + "|" + qidStr
					log.V(lvl.DEBUG).Infof("YangToDb_qos_buffer_allocation_profile_xfmr --> key: %v, db_bufferAllocationProfileName: %v", key, bufferAllocationProfileName)
					if _, ok := bufferQueueTblMap[key]; !ok {
						bufferQueueTblMap[key] = db.Value{Field: make(map[string]string)}
					}
					bufferQueueTblMap[key].Field["profile"] = bufferProfileKey
				}
			}
		}
		if len(bufferProfileTblMap) > 16 {
			return nil, errors.New("Only 16 Buffer Profiles are available, config contains " + strconv.Itoa(len(bufferProfileTblMap)))
		}
		resMap["BUFFER_PROFILE"] = bufferProfileTblMap
		resMap["BUFFER_QUEUE"] = bufferQueueTblMap
		resMap[QUEUE_TO_BUF_PROFILE_MAP] = queueToBufferProfileMap
	}
	return resMap, nil
}

func setIfPresentInt32(field *int32, value db.Value, name string) {
	val, exist := value.Field[name]
	if !exist {
		return
	}
	if tmp, err := strconv.ParseInt(val, 10, 32); err == nil {
		*field = int32(tmp)
	} else {
		log.V(lvl.ERROR).Info("Unable to parse the %v value", name)
	}
}

func getbufferAllocationProfileAttrFromDb(entry db.Value) (bufferAllocationProfileAttr, error) {
	var ret bufferAllocationProfileAttr

	ret.useSharedBuffer = true
	setIfPresentUInt64(&ret.dedicatedBuffer, entry, "size")
	setIfPresentUInt32(&ret.staticSharedBufferLimit, entry, "static_th")
	setIfPresentInt32(&ret.dynamicLimitScalingFactor, entry, "dynamic_th")
	setIfPresentUInt64(&ret.xon, entry, "xon")
	setIfPresentUInt64(&ret.xoff, entry, "xoff")
	setIfPresentUInt64(&ret.xonOffset, entry, "xon_offset")
	ret.sharedBufferLimitType = "DYNAMIC_BASED_ON_SCALING_FACTOR"
	if ret.staticSharedBufferLimit != 0 {
		ret.sharedBufferLimitType = "STATIC"
	}
	ret.bufferPoolName = entry.Field["pool"]
	return ret, nil
}

var DbToYang_qos_buffer_allocation_profile_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infoln("DbToYang_qos_buffer_allocation_profile_xfmr - inParams.uri: ", inParams.uri)

	pathInfo := NewPathInfo(inParams.uri)
	bufferAllocationProfileName := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Infoln("error parsing targetUriPath")
		return err
	}

	qos := getQosRoot(inParams.ygRoot)
	if qos == nil {
		ygot.BuildEmptyTree(qos)
	}

	var doConfig, doState bool
	switch {
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/buffer-allocation-profiles") == 0:
		doConfig, doState = true, true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile") == 0:
		doConfig, doState = true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile/config"):
		doConfig = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile/state"):
		doState = true
	default:
		errStr := "Invalid URI"
		log.V(lvl.ERROR).Info(errStr)
		return tlerr.InvalidArgsError{Format: errStr}
	}

	if bufferAllocationProfileName == "" {
		bufferAllocationProfileName = "*"
	}
	keys, err := inParams.dbs[db.ConfigDB].GetKeysByPattern(&db.TableSpec{Name: QUEUE_TO_BUF_PROFILE_MAP}, bufferAllocationProfileName)
	if err != nil {
		return tlerr.New("Unable to get QUEUE_TO_BUF_PROFILE_MAP")
	}

	for _, key := range keys {
		if len(key.Comp) < 1 {
			continue
		}
		bapName := key.Comp[0]
		log.V(lvl.DEBUG).Info("Fill buffer allocation profile ", bapName)
		qToBufProfMapEntry, errCfg := inParams.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: QUEUE_TO_BUF_PROFILE_MAP}, key)
		if errCfg != nil {
			log.V(lvl.ERROR).Infof("Unable to get QUEUE_TO_BUF_PROFILE_MAP entry for %v", key)
		}

		var bapObj *ocbinds.OpenconfigQos_Qos_BufferAllocationProfiles_BufferAllocationProfile
		if qos.BufferAllocationProfiles != nil && qos.BufferAllocationProfiles.BufferAllocationProfile != nil && len(qos.BufferAllocationProfiles.BufferAllocationProfile) > 0 {
			var ok bool = false
			if bapObj, ok = qos.BufferAllocationProfiles.BufferAllocationProfile[bapName]; !ok {
				bapObj, _ = qos.BufferAllocationProfiles.NewBufferAllocationProfile(bapName)
			}
			ygot.BuildEmptyTree(bapObj)
		} else {
			ygot.BuildEmptyTree(qos.BufferAllocationProfiles)
			bapObj, _ = qos.BufferAllocationProfiles.NewBufferAllocationProfile(bapName)
			ygot.BuildEmptyTree(bapObj)
		}
		bapObj.Name = &bapName
		bapObj.Config.Name = &bapName
		bapObj.State.Name = &bapName

		for qName, buffer_profile_name := range qToBufProfMapEntry.Field {
			queueName := qName
			log.V(lvl.DEBUG).Infof("Adding %s to %s", queueName, bapName)
			if bapObj.Queues == nil {
				ygot.BuildEmptyTree(bapObj.Queues)
			}
			queueObj, ok := bapObj.Queues.Queue[queueName]
			if !ok {
				queueObj, _ = bapObj.Queues.NewQueue(queueName)
			}
			ygot.BuildEmptyTree(queueObj)

			if doConfig {
				ygot.BuildEmptyTree(queueObj.Config)
				queueObj.Name = &queueName
				queueObj.Config.Name = &queueName
				entry, errCfg := inParams.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: "BUFFER_PROFILE"}, db.Key{Comp: []string{buffer_profile_name}})
				if errCfg != nil {
					log.V(lvl.DEBUG).Info("Unable to get the data from the CONFIG DB")
				}
				info, err := getbufferAllocationProfileAttrFromDb(entry)
				if err == nil {
					queueObj.Config.DedicatedBuffer = &info.dedicatedBuffer
					queueObj.Config.UseSharedBuffer = &info.useSharedBuffer
					queueObj.Config.SharedBufferLimitType = ocbinds.OpenconfigQos_SHARED_BUFFER_LIMIT_TYPE_DYNAMIC_BASED_ON_SCALING_FACTOR
					if info.sharedBufferLimitType == "STATIC" {
						queueObj.Config.SharedBufferLimitType = ocbinds.OpenconfigQos_SHARED_BUFFER_LIMIT_TYPE_STATIC
					}
					queueObj.Config.StaticSharedBufferLimit = &info.staticSharedBufferLimit
					queueObj.Config.DynamicLimitScalingFactor = &info.dynamicLimitScalingFactor
				}
			}
			if doState {
				ygot.BuildEmptyTree(queueObj.State)
				queueObj.Name = &queueName
				queueObj.State.Name = &queueName

				entry, errState := inParams.dbs[db.ApplDB].GetEntry(&db.TableSpec{Name: "BUFFER_PROFILE_TABLE"}, db.Key{Comp: []string{buffer_profile_name}})
				if errState != nil {
					log.V(lvl.DEBUG).Info("Unable to get the data from the APPL STATE DB")
				}
				info, err := getbufferAllocationProfileAttrFromDb(entry)
				if err == nil {
					queueObj.State.DedicatedBuffer = &info.dedicatedBuffer
					queueObj.State.UseSharedBuffer = &info.useSharedBuffer
					queueObj.State.SharedBufferLimitType = ocbinds.OpenconfigQos_SHARED_BUFFER_LIMIT_TYPE_DYNAMIC_BASED_ON_SCALING_FACTOR
					if info.sharedBufferLimitType == "STATIC" {
						queueObj.State.SharedBufferLimitType = ocbinds.OpenconfigQos_SHARED_BUFFER_LIMIT_TYPE_STATIC
					}
					queueObj.State.StaticSharedBufferLimit = &info.staticSharedBufferLimit
					queueObj.State.DynamicLimitScalingFactor = &info.dynamicLimitScalingFactor
				}
			}

		}
	}
	return nil
}

var DbToYangPath_qos_buffer_allocation_profile_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	log.V(lvl.DEBUG).Infof("DbToYangPath_qos_buffer_allocation_profile_path_xfmr: yangPath %v tblKeyComp %v", inParams.yangPath, inParams.tblKeyComp)

	if len(inParams.tblKeyComp) != 1 {
		return fmt.Errorf("DbToYangPath_qos_buffer_allocation_profile_path_xfmr: Invalid tblKey %v or tblEntry %v", inParams.tblKeyComp, inParams.tblEntry)
	}

	// The key will look like `staggered_8queue.egress_shared_pool.0.d-3` where `staggered_8queue` is the name of the buffer-allocation-profile.
	key := inParams.tblKeyComp[0]
	split := strings.Split(key, ".")
	if len(split) < 1 {
		return fmt.Errorf("DbToYangPath_qos_buffer_allocation_profile_path_xfmr: Invalid tblKey %v or tblEntry %v", inParams.tblKeyComp, inParams.tblEntry)
	}
	name := split[0]

	// QUEUE_TO_BUF_PROFILE_MAP contains the mapping between the DB key and the queue name/id.
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return fmt.Errorf("DbToYangPath_qos_buffer_allocation_profile_path_xfmr: ConfigDB is nil!")
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: QUEUE_TO_BUF_PROFILE_MAP}, db.Key{Comp: []string{name}})
	if err != nil {
		return fmt.Errorf("DbToYangPath_qos_buffer_allocation_profile_path_xfmr: Unable to get QUEUE_TO_BUF_PROFILE_MAP")
	}

	queue := ""
	for q, k := range entry.Field {
		if k == key {
			queue = q
			break
		}
	}
	if queue == "" {
		return fmt.Errorf("DbToYangPath_qos_buffer_allocation_profile_path_xfmr: %v not found in QUEUE_TO_BUF_PROFILE_MAP", key)
	}

	inParams.ygPathKeys["/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile/name"] = name
	inParams.ygPathKeys["/openconfig-qos:qos/buffer-allocation-profiles/buffer-allocation-profile/queues/queue/id"] = queue
	return nil
}
