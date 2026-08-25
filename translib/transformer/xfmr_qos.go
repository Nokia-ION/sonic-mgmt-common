package transformer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"maps"
	"strconv"
	"strings"
	"sync"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/Azure/sonic-mgmt-common/translib/utils"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	QUEUE_MAP_JSON                        = "/usr/share/sonic/hwsku/qos_queue_map.json"
	COUNTERS_QUEUE_NAME_MAP               = "COUNTERS_QUEUE_NAME_MAP"
	TRUNK_QUEUE_VIRT                      = "TRUNK_QUEUE_VIRT"
	QUEUE_NAME_TO_ID_MAP                  = "QUEUE_NAME_TO_ID_MAP"
	PG_NAME_TO_ID_MAP                     = "PG_NAME_TO_ID_MAP"
	PG_ID_KEY                             = "PG_ID_MAP"
	CPU                                   = "CPU"
	FRONT_PANEL                           = "FRONT_PANEL"
	BUFFER_PG                             = "BUFFER_PG"
	TXCACHE_QUEUE_MAP_PREFIX              = "queueMap_"
	TXCACHE_INTERFACE_TO_QUEUE_TO_OID_MAP = "interfaceToQueueToOidMap"
	TXCACHE_COUNTERS_QUEUE_TYPE_MAP       = "COUNTERS_QUEUE_TYPE_MAP"
	PORT_TN                               = "PORT"
)

var qMapStr map[string]interface{}
var queueTypes []string = []string{CPU, FRONT_PANEL}
var qCounterTblAttr []string = []string{"transmit-pkts", "transmit-octets", "dropped-pkts"}
var ocQueueHwQueueMap map[string]map[string]string
var queueMapMutex sync.RWMutex // protects ocQueueHwQueueMap

func TestingQosClearState() {
	//queueMapMutex.Lock()
	//defer queueMapMutex.Unlock()
	ocQueueHwQueueMap = nil
	buildQueueNameToIdMapFromDb(nil)
}

func parseQueueMapJSONFile() error {
	file, err := ioutil.ReadFile(QUEUE_MAP_JSON)
	if err != nil {
		log.V(lvl.DEBUG).Infof("Queue map file not present")
		return err
	}
	qMapStr = make(map[string]interface{})
	return json.Unmarshal([]byte(file), &qMapStr)
}

func init() {
	XlateFuncBind("qos_intf_table_xfmr", qos_intf_table_xfmr)
	XlateFuncBind("YangToDb_qos_intf_tbl_key_xfmr", YangToDb_qos_intf_tbl_key_xfmr)
	XlateFuncBind("DbToYang_qos_intf_tbl_key_xfmr", DbToYang_qos_intf_tbl_key_xfmr)
	XlateFuncBind("YangToDb_qos_intf_intf_id_fld_xfmr", YangToDb_qos_intf_intf_id_fld_xfmr)
	XlateFuncBind("DbToYang_qos_intf_intf_id_fld_xfmr", DbToYang_qos_intf_intf_id_fld_xfmr)

	XlateFuncBind("YangToDb_qos_get_one_intf_all_q_xfmr", YangToDb_qos_get_one_intf_all_q_xfmr)
	XlateFuncBind("DbToYang_qos_get_one_intf_all_q_xfmr", DbToYang_qos_get_one_intf_all_q_xfmr)
	XlateFuncBind("Subscribe_qos_get_one_intf_all_q_xfmr", Subscribe_qos_get_one_intf_all_q_xfmr)
	XlateFuncBind("DbToYangPath_qos_get_one_intf_all_q_path_xfmr", DbToYangPath_qos_get_one_intf_all_q_path_xfmr)

	XlateFuncBind("qos_post_xfmr", qos_post_xfmr)
	XlateFuncBind("qos_pre_xfmr", qos_pre_xfmr)
	// Parse queue JSON
	parseQueueMapJSONFile()
}

func getQosRoot(s *ygot.GoStruct) *ocbinds.OpenconfigQos_Qos {
	deviceObj := (*s).(*ocbinds.Device)
	return deviceObj.Qos
}

func getQosBufferAllocationProfilesRoot(s *ygot.GoStruct) *ocbinds.OpenconfigQos_Qos_BufferAllocationProfiles {
	if s != nil {
		if deviceObj, ok := (*s).(*ocbinds.Device); ok {
			return deviceObj.Qos.BufferAllocationProfiles
		}
	}
	return &ocbinds.OpenconfigQos_Qos_BufferAllocationProfiles{}
}

func getQosIntfRoot(s *ygot.GoStruct) *ocbinds.OpenconfigQos_Qos_Interfaces {
	deviceObj := (*s).(*ocbinds.Device)
	return deviceObj.Qos.Interfaces
}

func getIntfRootbyName(s *ygot.GoStruct, ifName string) *ocbinds.OpenconfigInterfaces_Interfaces_Interface {
	deviceObj := (*s).(*ocbinds.Device)
	if deviceObj.Interfaces == nil || deviceObj.Interfaces.Interface == nil || len(deviceObj.Interfaces.Interface) < 1 {
		log.V(lvl.DEBUG).Infof("getIntfbyNameRoot: deviceObj.Interfaces is nil")
		return nil
	}
	intfObj, ok := deviceObj.Interfaces.Interface[ifName]
	if !ok {
		log.V(lvl.DEBUG).Infof("getIntfbyNameRoot: deviceObj.Interfaces.Interface[ifName] does not exist.")
		return nil
	}
	return intfObj
}

var DbToYang_qos_name_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	var err error
	resMap := make(map[string]interface{})
	resMap["name"] = inParams.key
	return resMap, err
}

func getMembers(stateDb *db.DB, intfName string) ([]string, error) {
	if !strings.HasPrefix(intfName, PORTCHANNEL) {
		return []string{intfName}, nil
	}
	memKeys, err := stateDb.GetKeys(&db.TableSpec{Name: LAG_MEMBER_TABLE_TN + stateDb.Opts.KeySeparator + intfName})
	if err != nil {
		return []string{intfName}, fmt.Errorf("%w; Unable to retrieve member keys for %s", err, intfName)
	}
	var members []string
	for i := range memKeys {
		members = append(members, memKeys[i].Get(1))
	}
	return members, nil
}

func doGetIntfBufferTableEntries(d *db.DB, ifName, tableName string) []db.Key {
	if d == nil {
		log.V(lvl.ERROR).Infof("unable to get configDB")
		return nil
	}

	dbSpec := &db.TableSpec{Name: tableName}
	keys, err := d.GetKeysByPattern(dbSpec, ifName+"|*")
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("unable to get keys matching %v: %v", ifName, err)
		return nil
	}

	return keys
}

func doGetIntfBufferAllocationProfile(d *db.DB, isState bool, intfName string) string {
	if d == nil {
		log.V(lvl.ERROR).Infof("unable to get configDB")
		return ""
	}

	dbSpec := &db.TableSpec{Name: "BUFFER_QUEUE"}
	pattern := intfName + "|*"

	if isState {
		dbSpec = &db.TableSpec{Name: "BUFFER_QUEUE_TABLE"}
		pattern = intfName + ":*"
	}

	keys, err := d.GetKeysByPattern(dbSpec, pattern)
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("unable to get keys matching %v: %v", intfName, err)
		return ""
	}
	for _, key := range keys {
		qCfg, err := d.GetEntry(dbSpec, key)
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("doGetIntfBufferAllocationProfile: unable to get qCfg: %v: %v", key, err)
			return ""
		}

		// Profile on queue looks like "[BUFFER_PROFILE_TABLE:staggered_8queue.3]"
		// We need to return "staggered_8queue" as profile name.
		if bufferQueueProfile, ok := qCfg.Field["profile"]; ok {
			bufferQueueProfile = strings.Trim(bufferQueueProfile, "[]")
			if isState {
				bufferQueueProfile = strings.TrimPrefix(bufferQueueProfile, "BUFFER_PROFILE_TABLE:")
			} else {
				bufferQueueProfile = strings.TrimPrefix(bufferQueueProfile, "BUFFER_PROFILE|")
			}
			bufferAllocationProfile := strings.Split(bufferQueueProfile, ".")
			log.V(lvl.DEBUG).Infof("doGetIntfBufferAllocationProfile:  %v", bufferAllocationProfile)
			return bufferAllocationProfile[0]
		}
	}

	return ""
}

func doGetAllQueueOidMap(d *db.DB) (db.Value, error) {

	// COUNTERS_QUEUE_NAME_MAP
	dbSpec := &db.TableSpec{Name: "COUNTERS_QUEUE_NAME_MAP"}
	queueOidMap, err := d.GetMapAll(dbSpec)
	log.V(lvl.DEBUG).Infof("queueOidMap %v", queueOidMap)

	if err != nil {
		log.V(lvl.DEBUG).Infof("queueOidMap get failed: %v", err)
	}
	return queueOidMap, err
}

func doGetInterfaceToQueueToOidMap(d *db.DB) (map[string]map[string]string, error) {
	queueOidMap, err := doGetAllQueueOidMap(d)
	if err != nil {
		return nil, err
	}
	intfQueueOidMap := make(map[string]map[string]string)

	// The key will be in the form of Ethernet1/31/1:1
	for k, oid := range queueOidMap.Field {
		ks := strings.Split(k, ":")
		if len(ks) != 2 {
			continue
		}
		if _, ok := intfQueueOidMap[ks[0]]; !ok {
			intfQueueOidMap[ks[0]] = make(map[string]string)
		}
		intfQueueOidMap[ks[0]][ks[1]] = oid
	}
	return intfQueueOidMap, nil
}

func doGetAllQueueTypeMap(d *db.DB) (db.Value, error) {

	// COUNTERS_QUEUE_TYPE_MAP
	queueTs := &db.TableSpec{Name: "COUNTERS_QUEUE_TYPE_MAP"}
	queueTypeMap, err := d.GetMapAll(queueTs)
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("queueTypeMap get failed: %v", err)
	}

	return queueTypeMap, err
}

func getIntfQCountersTblKey(d *db.DB, ifQKey string) (string, error) {
	var oid string

	queueOidMap, err := doGetAllQueueOidMap(d)
	if err == nil && queueOidMap.IsPopulated() {
		_, ok := queueOidMap.Field[ifQKey]
		if !ok {
			err = errors.New("OID info not found from Counters DB for interface queue: " + ifQKey)
		} else {
			oid = queueOidMap.Field[ifQKey]
		}
	} else {
		err = fmt.Errorf("Get for OID info from all the interfaces queues from Counters DB failed! %w", err)
	}

	return oid, err
}

func getQosCounters(entry *db.Value, attr string, counter_val **uint64) error {

	var ok bool = false
	val, ok := entry.Field[attr]

	if ok && len(val) > 0 {
		v, _ := strconv.ParseUint(val, 10, 64)
		*counter_val = &v
		return nil
	} else {
		log.V(lvl.DEBUG).Infof("getQosCounters: Attr %v doesn't exist in table Map!", attr)
	}
	return nil
}

func getQosOffsetCounters(entry *db.Value, attr string) (uint64, error) {
	val1, ok := entry.Field[attr]
	if !ok {
		return 0, errors.New("Attr " + attr + "doesn't exist in table Map!")
	}

	if len(val1) == 0 {
		return 0, nil
	}

	val, err := strconv.ParseUint(val1, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w; Failed to convert %s to int", err, val1)
	}
	return val, nil
}

func getPersistentWatermark(d *db.DB, oid string, entry *db.Value) error {
	var err error
	ts := &db.TableSpec{Name: "PERSISTENT_WATERMARKS"}
	if *entry, err = d.GetEntry(ts, db.Key{Comp: []string{oid}}); err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("getPersistentWatermark: not able to find the oid entry in DB: %v", err)
		return err
	}

	return nil
}

func getQueueSpecificCounterAttr(targetUriPath string, entry *db.Value, counters *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues_Queue_State) (bool, error) {
	switch targetUriPath {

	case "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state/transmit-pkts":
		val, e := getQosOffsetCounters(entry, "SAI_QUEUE_STAT_PACKETS")
		val += *counters.TransmitPkts
		counters.TransmitPkts = &val
		return true, e

	case "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state/transmit-octets":
		val, e := getQosOffsetCounters(entry, "SAI_QUEUE_STAT_BYTES")
		val += *counters.TransmitOctets
		counters.TransmitOctets = &val
		return true, e

	case "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state/dropped-pkts":
		val, e := getQosOffsetCounters(entry, "SAI_QUEUE_STAT_DROPPED_PACKETS")
		val += *counters.DroppedPkts
		counters.DroppedPkts = &val
		return true, e

	case "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state/name":
		return false, nil

	default:
		log.V(lvl.WARNING).Infof("%v - Not an interface state counter attribute or unsupported", targetUriPath)
	}
	return false, nil
}

func getCounterAndBackupByOid(inParams XfmrParams, oid string, entry *db.Value) error {

	var dbErr error
	cntTs := &db.TableSpec{Name: "COUNTERS"}
	*entry, dbErr = inParams.dbs[inParams.curDb].GetEntry(cntTs, db.Key{Comp: []string{oid}})
	if dbErr != nil {
		log.V(tlerr.ErrorSeverity(dbErr)).Infof("getCounterAndBackupByOid : not able to find the oid entry in DB Counters table: %v", dbErr)
		return dbErr
	}

	return nil
}

type stringUint64LeafList struct {
	field string
	leaf  **uint64
}

func dbStringToUint64LeafMapping(entry db.Value, flList []stringUint64LeafList) error {
	for _, fl := range flList {
		if val, ok := entry.Field[fl.field]; ok {
			if v, err := strconv.ParseUint(val, 10, 64); err == nil {
				*fl.leaf = &v
			}
		}
	}
	return nil
}

func populateQCounters(inParams XfmrParams, targetUriPath string, oid string, counter *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues_Queue_State) error {

	var err error
	var entry db.Value
	var counterTs string

	switch targetUriPath {
	case "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state":
		dbErr := getCounterAndBackupByOid(inParams, oid, &entry)
		if dbErr != nil {
			log.V(lvl.DEBUG).Infof("populateQCounters : not able to find the oid entry in DB Counters table: %v", dbErr)
			return dbErr
		}

		for _, attr := range qCounterTblAttr {
			uri := targetUriPath + "/" + attr
			if ok, err := getQueueSpecificCounterAttr(uri, &entry, counter); !ok || err != nil {
				log.V(lvl.DEBUG).Infof("Get Counter URI failed: %v; for %v", uri, oid)
			}
		}
		counterTs = entry.Field["QUEUE_STAT_TIME_STAMP_USEC"]

		err = getPersistentWatermark(inParams.dbs[inParams.curDb], oid, &entry)
		if err == nil {
			getQosCounters(&entry, "SAI_QUEUE_STAT_SHARED_WATERMARK_BYTES", &counter.MaxQueueLen)
		}

	// persisten-watermark resides on separate DB table
	case "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state/max-queue-len":
		getPersistentWatermark(inParams.dbs[inParams.curDb], oid, &entry)
		err = getQosCounters(&entry, "SAI_QUEUE_STAT_SHARED_WATERMARK_BYTES", &counter.MaxQueueLen)

	default:
		log.V(lvl.DEBUG).Infof("Entering default branch")
		dbErr := getCounterAndBackupByOid(inParams, oid, &entry)
		if dbErr != nil {
			log.V(lvl.ERROR).Infof("populateQCounters : not able to find the oid entry in DB Counters table: %v", dbErr)
			return dbErr
		}
		_, err = getQueueSpecificCounterAttr(targetUriPath, &entry, counter)
		counterTs = entry.Field["QUEUE_STAT_TIME_STAMP_USEC"]
	}

	if err != nil || counterTs == "" {
		return err
	}

	if usec, err := strconv.ParseInt(counterTs, 10, 64); err == nil {
		utils.UpdateYGSTimestamp(*inParams.ygRoot, counter, usec*1000)
	} else {
		log.V(lvl.DEBUG).Infof("Invalid timestamp for queue %s, %v", oid, counterTs)
	}

	return nil
}

func getQType(queueTypeMap db.Value, oid string) string {

	q_type, ok := queueTypeMap.Field[oid]
	if !ok {
		log.V(lvl.DEBUG).Infof("Queue oid (%v) is not mapped in Queue-Type-Map", oid)
		return "AC"
	} else {
		if strings.Compare(q_type, "SAI_QUEUE_TYPE_MULTICAST") == 0 {
			return "MC"
		} else {
			if strings.Compare(q_type, "SAI_QUEUE_TYPE_UNICAST") == 0 {
				return "UC"
			} else {
				return "AC"
			}
		}
	}
}

/* Validate whether intf exists in DB */
func validateQosIntf(confd *db.DB, dbs [db.MaxDB]*db.DB, intfName string) error {

	log.V(lvl.DEBUG).Infof(" validateQosIntf - intfName %v", intfName)
	if intfName == "" {
		return nil
	}

	if intfName == "CPU" {
		return nil
	}
	var d *db.DB

	if confd != nil {
		log.V(lvl.DEBUG).Infof(" validateQosIntf - confd intfName %v", intfName)
		d = confd
	} else {
		log.V(lvl.DEBUG).Infof(" validateQosIntf - Read from dbs intfName %v", intfName)
		d = dbs[db.ConfigDB]
	}
	if d != nil {
		entry, err := d.GetEntry(&db.TableSpec{Name: "PORT"}, db.Key{Comp: []string{intfName}})
		if err != nil || !entry.IsPopulated() {
			entry, err := d.GetEntry(&db.TableSpec{Name: "PORTCHANNEL"}, db.Key{Comp: []string{intfName}})
			if err != nil || !entry.IsPopulated() {
				errStr := "Interface %v is not available; err: %w"
				log.V(tlerr.ErrorSeverity(err)).Infof(errStr, intfName, err)
				return tlerr.InvalidArgsError{Format: errStr, Args: []interface{}{intfName, err}}
			}
		}
	}
	log.V(lvl.DEBUG).Infof(" validateQosIntf - intfName %v success ", intfName)
	return nil
}

func getDbQueueName(queueName string) (string, error) {
	log.V(lvl.DEBUG).Infof(" getDbQueueName - queueName %v", queueName)

	if strings.Contains(queueName, ":") {
		queue := strings.Split(queueName, ":")
		dbQueueName := queue[0] + ":" + queue[1]
		log.V(lvl.DEBUG).Infof(" getDbQueueName - dbQueueName %v", dbQueueName)
		return dbQueueName, nil
	}
	errStr := "Invalid Queue: " + queueName
	log.V(lvl.ERROR).Infof(errStr)
	return queueName, tlerr.InvalidArgsError{Format: errStr}
}

func getQueueMapType(intfName string) string {
	qMap := FRONT_PANEL
	if intfName == CPU {
		qMap = CPU
	}
	return qMap
}

func getMappedDbQueueName(intfName, queueName string) (string, error) {
	log.V(lvl.DEBUG).Infof(" getDbQueueIdFromName - intfName  %v queueName %v", intfName, queueName)

	qMap := getQueueMapType(intfName)
	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	if qid, ok := ocQueueHwQueueMap[qMap]; ok {
		if qidStr, ok := qid[queueName]; ok {
			return intfName + ":" + qidStr, nil
		}
	}

	return queueName, tlerr.InvalidArgsError{Format: "Invalid Queue: " + queueName}
}

func GetFPQueueNameFromId(qid string) (string, error) {
	return getQueueNameFromIdByQueueType(FRONT_PANEL, qid)
}

func getQueueNameFromIdByQueueType(qType, qid string) (string, error) {
	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	qMap, ok := ocQueueHwQueueMap[qType]
	if !ok {
		return qid, tlerr.InvalidArgsError{Format: "queue map not valid type=" + qType}
	}
	for k, vStr := range qMap {
		if vStr == qid {
			log.V(lvl.DEBUG).Infof(" OCQueueName -  queueName  %v-%v", k, vStr)
			return k, nil
		}
	}
	return qid, tlerr.InvalidArgsError{Format: "Invalid Queue: " + qid}
}

// The "OCQueueName" queue name takes the form of AF1 or INBAND_PRIORITY_X;
// ie: where queueName is "3", the returned OC queue name would be "AF1"
func getOCQueueName(intfName, queueName string) (string, error) {
	qMapType := getQueueMapType(intfName)
	return getQueueNameFromIdByQueueType(qMapType, queueName)
}

func GetIdFromFPQueueName(queueName string) (string, error) {
	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	return getNativeQueueNameByQueueType(FRONT_PANEL, queueName)
}

func getNativeQueueNameByQueueType(qType, queueName string) (string, error) {
	qMap, ok := ocQueueHwQueueMap[qType]
	if !ok {
		return "", tlerr.InvalidArgsError{Format: "queue map not found for type:" + qType}
	}
	if nameStr, ok := qMap[queueName]; ok {
		return nameStr, nil
	}
	return "", tlerr.InvalidArgsError{Format: "Invalid queue name:" + queueName}
}

// The "native" queue name is a qid;
// ie: where queueName is "AF1", the returned qid would be "3"
func getNativeQueueName(intfName, queueName string) (string, error) {
	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	return getNativeQueueNameNoLock(intfName, queueName)
}

// getNativeQueueNameNoLock assumes that the queueMapMutex is already held by the caller.
// The "native" queue name is a qid.
func getNativeQueueNameNoLock(intfName, queueName string) (string, error) {
	if intfName == "*" {
		for _, qMap := range ocQueueHwQueueMap {
			if nameStr, ok := qMap[queueName]; ok {
				return nameStr, nil
			}
		}
		return "", tlerr.InvalidArgsError{Format: "Invalid queue name:" + queueName}
	}
	return getNativeQueueNameByQueueType(getQueueMapType(intfName), queueName)
}

/* Validate whether intf queues valid or not */
func validateQosIntfQueue(dbs [db.MaxDB]*db.DB, intfName string, queueName string) error {

	log.V(lvl.DEBUG).Infof(" validateQosIntfQueue -intfName %v queueName %v", intfName, queueName)

	if !strings.Contains(queueName, ":") {
		errStr := "Invalid Queue: " + queueName
		log.V(lvl.ERROR).Infof(errStr)
		return tlerr.InvalidArgsError{Format: errStr}
	}
	queue := strings.Split(queueName, ":")
	if intfName != queue[0] {
		errStr := "Invalid Queue: " + queueName + " on interface " + intfName
		log.V(lvl.ERROR).Infof(errStr)
		return tlerr.InvalidArgsError{Format: errStr}
	}

	if dbs[db.CountersDB] != nil {
		_, err := getIntfQCountersTblKey(dbs[db.CountersDB], queueName)
		if err != nil {
			errStr := "Invalid Queue: " + queueName + " on interface " + intfName
			log.V(lvl.ERROR).Infof(errStr)
			return tlerr.InvalidArgsError{Format: errStr}
		}
	}
	return nil
}

func validateQosQueue(dbs [db.MaxDB]*db.DB, queueName string) error {

	log.V(lvl.DEBUG).Infof(" validateQosQueue - queueName %v", queueName)
	if dbs[db.CountersDB] != nil {
		_, err := getIntfQCountersTblKey(dbs[db.CountersDB], queueName)
		if err != nil {
			errStr := "Invalid Queue:" + queueName
			log.V(lvl.ERROR).Infof(errStr)
			return tlerr.InvalidArgsError{Format: errStr}
		}
	}

	return nil
}

// queueToQidMapCacheByType will return a map of queues to their hardware queue IDs. For example, "AF1": "3".
// This map is cached in the XfmrParams to avoid the repeated processing required to create this map.
func queueToQidMapCacheByType(inParams XfmrParams, qMapType string) map[string]string {
	queueToQidMap := map[string]string{}
	cachedQueueMap, present := inParams.txCache.Load(TXCACHE_QUEUE_MAP_PREFIX + qMapType)
	if !present {
		queueMapMutex.RLock()
		defer queueMapMutex.RUnlock()
		maps.Copy(queueToQidMap, ocQueueHwQueueMap[qMapType])
		inParams.txCache.Store(TXCACHE_QUEUE_MAP_PREFIX+qMapType, queueToQidMap)
	} else {
		queueToQidMap = cachedQueueMap.(map[string]string)
	}
	return queueToQidMap
}

var DbToYang_qos_get_one_intf_one_q_counters_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	qosIntfsObj := getQosIntfRoot(inParams.ygRoot)
	pathInfo := NewPathInfo(inParams.uri)
	intfName := pathInfo.Var("interface-id")
	queueName := pathInfo.Var("name")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	queueNameList := []string{}
	if err != nil {
		log.V(lvl.DEBUG).Infof("DbToYang_qos_get_one_intf_one_q_counters_xfmr - Unable to get yang path from uri %v", targetUriPath)
		return err
	}

	// if queue name == *, go through all the queue name in the mapping table
	if queueName == "*" {
		qMapType := getQueueMapType(intfName)
		queueMapMutex.RLock()
		if qMap, ok := ocQueueHwQueueMap[qMapType]; ok {
			for qName := range qMap {
				queueNameList = append(queueNameList, qName)
			}
		}
		queueMapMutex.RUnlock()
	} else {
		queueNameList = append(queueNameList, queueName)
	}
	// Get list of members from interface
	members, err := getMembers(inParams.dbs[db.StateDB], intfName)
	if err != nil {
		return fmt.Errorf("%w; getMembers() for %s failed", err, intfName)
	}
	is_singleton := len(members) == 1

	/* Try getting Queue name from map */
	var dbQueueNames []string
	for _, member := range members {
		for _, qqName := range queueNameList {
			var dbQueueName string
			if dbQueueName, err = getMappedDbQueueName(member, qqName); err != nil {
				if dbQueueName, err = getDbQueueName(qqName); err != nil {
					log.V(lvl.ERROR).Infof("DbToYang_qos_get_one_intf_one_q_counters_xfmr - invalid queue %v", qqName)
					continue
				}
			}

			if err = validateQosIntfQueue(inParams.dbs, member, dbQueueName); err != nil {
				log.V(lvl.ERROR).Infof("DbToYang_qos_get_one_intf_one_q_counters_xfmr - invalid interface %v queue %v db qname %v", intfName, qqName, dbQueueName)
				continue
			}
			dbQueueNames = append(dbQueueNames, dbQueueName)
		}
	}
	if len(dbQueueNames) == 0 {
		return nil
	}

	var state *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues_Queue_State
	var cfg *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues_Queue_Config

	if qosIntfsObj != nil && qosIntfsObj.Interface != nil && len(qosIntfsObj.Interface) > 0 {
		queuesObj := qosIntfsObj.Interface[intfName].Output.Queues
		var queueObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues_Queue
		if queuesObj != nil {
			queueObj = queuesObj.Queue[queueName]
			ygot.BuildEmptyTree(queueObj)
		}
		if queueObj != nil {
			state = queueObj.State
			cfg = queueObj.Config
		}
	}

	queue_path := strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/queues/queue")
	state_path := strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state")
	config_path := strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/queues/queue/config")
	config_and_state_path := queue_path && !(config_path || state_path)

	var oid string
	if (config_and_state_path || config_path) && is_singleton {
		if cfg == nil {
			log.V(lvl.ERROR).Infof("DbToYang_qos_get_one_intf_one_q_counters_xfmr - cfg is nil")
			return err
		}
		cfg.Name = &queueName
		configDb := inParams.dbs[db.ConfigDB]
		if configDb == nil {
			err = errors.New("DbToYang_qos_get_one_intf_one_q_counters_xfmr - Unable to get config db")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		queueManagementProfileName := doGetIntfQueueManagementProfile(configDb, strings.Replace(dbQueueNames[0], ":", "|", 1))
		cfg.QueueManagementProfile = &queueManagementProfileName
		log.V(lvl.DEBUG).Infof("DbToYang_qos_get_one_intf_one_q_counters_xfmr: %v", targetUriPath)
	}

	if config_and_state_path || state_path {
		if state == nil {
			err = errors.New("DbToYang_qos_get_one_intf_one_q_counters_xfmr - state_counters is nil")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		state.Name = &queueName
		if !state_path {
			targetUriPath = targetUriPath + "/state"
		}
		countersDb := inParams.dbs[db.CountersDB]
		if countersDb == nil {
			err = errors.New("DbToYang_qos_get_one_intf_one_q_counters_xfmr - Unable to get counters db")
			log.V(lvl.ERROR).Info(err)
			return err
		}

		val := uint64(0)
		state.TransmitPkts = &val
		state.TransmitOctets = &val
		state.DroppedPkts = &val
		for _, dbQueueName := range dbQueueNames {
			if oid, err = getIntfQCountersTblKey(countersDb, dbQueueName); err != nil {
				continue
			}
			if err = populateQCounters(inParams, targetUriPath, oid, state); err != nil {
				log.V(lvl.ERROR).Info(err)
				continue
			}
		}
		if is_singleton {
			applStateDb := inParams.dbs[db.ApplDB]
			if applStateDb == nil {
				err = errors.New("DbToYang_qos_get_one_intf_one_q_counters_xfmr - Unable to appState db")
				log.V(lvl.ERROR).Info(err)
				return err
			}
			queueManagementProfileName := doGetIntfQueueManagementProfile(applStateDb, strings.Replace(dbQueueNames[0], ":", "|", 1))
			state.QueueManagementProfile = &queueManagementProfileName
		}
	}

	log.V(lvl.DEBUG).Infof("DbToYang_qos_get_one_intf_one_q_counters_xfmr - finished %v", state)
	return nil
}

func getKeyForQueueTable(ifName string, queueName string) (string, error) {
	qMapType := getQueueMapType(ifName)
	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	qMap, ok := ocQueueHwQueueMap[qMapType]
	if !ok {
		errStr := "getKeyForQueueTable - Hardware queue id to oc-queue map not available"
		log.V(lvl.ERROR).Infof(errStr)
		return "", tlerr.InvalidArgsError{Format: errStr}
	}
	qidStr, ok := qMap[queueName]
	if !ok {
		errStr := "getKeyForQueueTable - Hardware queue map entry for " + queueName + " not available"
		log.V(lvl.ERROR).Infof(errStr)
		return "", tlerr.InvalidArgsError{Format: errStr}
	}

	// Concatenate interface name with hardware queue id to get key in QUEUE table
	qidStr = ifName + "|" + qidStr
	return qidStr, nil
}

func applyBufferAllocationProfileToEgressInterfaceQueue(inParams XfmrParams, ifName string, configObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Config) (map[string]db.Value, error) {
	bufferQueueTblMap := make(map[string]db.Value)
	if configObj.BufferAllocationProfile == nil {
		return bufferQueueTblMap, nil
	}
	bufferAllocationProfileName := configObj.BufferAllocationProfile
	bufferAllocationProfilesObj := getQosBufferAllocationProfilesRoot(inParams.ygRoot)
	if bufferAllocationProfilesObj == nil {
		return bufferQueueTblMap, nil
	}
	var ok bool
	var err error
	if inParams.oper == DELETE {
		configDb, err := db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			log.V(lvl.ERROR).Infof("applyBufferAllocationProfileToEgressInterfaceQueue, unable to get configDB, error %v", err)
			return nil, err
		}
		defer configDb.DeleteDB()
		bufferQueuesTbl := &db.TableSpec{Name: "BUFFER_QUEUE"}
		for _, key := range doGetIntfBufferTableEntries(configDb, ifName, "BUFFER_QUEUE") {
			if err = configDb.DeleteEntry(bufferQueuesTbl, key); err != nil {
				log.V(lvl.ERROR).Infof("Error deleting entry (%v) from BUFFER_QUEUE ", key)
			}
		}
		return bufferQueueTblMap, nil
	}
	if bufferAllocationProfilesObj.BufferAllocationProfile == nil {
		errStr := "applyBufferAllocationProfileToEgressInterfaceQueue - BufferAllocationProfile object not found."
		return nil, tlerr.NotFoundError{Format: errStr}
	}
	bufferAllocationProfileObj, ok := bufferAllocationProfilesObj.BufferAllocationProfile[*bufferAllocationProfileName]
	if !ok {
		errStr := "applyBufferAllocationProfileToEgressInterfaceQueue - BufferAllocationProfile object not found for " + *bufferAllocationProfileName
		return nil, tlerr.NotFoundError{Format: errStr}
	}
	if bufferAllocationProfileObj.Queues == nil || bufferAllocationProfileObj.Queues.Queue == nil || len(bufferAllocationProfileObj.Queues.Queue) < 1 {
		errStr := "No queue in buffer allocation profile subtree object populated"
		return nil, tlerr.NotFoundError{Format: errStr}
	}

	intfName := FRONT_PANEL
	if strings.HasPrefix(*bufferAllocationProfileName, "cpu") {
		intfName = CPU
	}
	for queueName := range bufferAllocationProfileObj.Queues.Queue {
		var qidStr string
		if qidStr, err = getKeyForQueueTable(ifName, queueName); err != nil {
			log.V(lvl.ERROR).Infof("applyBufferAllocationProfileToEgressInterfaceQueue - Hardware queue id for oc-queue not found for %v; err: %v", queueName, err)
			continue
		}

		var nativeQidStr string
		nativeQidStr, err = getNativeQueueName(ifName, queueName)
		if err != nil {
			log.V(lvl.ERROR).Infof("applyBufferAllocationProfileToEgressInterfaceQueue - Hardware queue id for oc-queue not found for %v" + queueName)
			continue
		}
		bufferAllocationProfileNameStr := *bufferAllocationProfileName
		if intfName != CPU {
			bufferAllocationProfileNameStr = bufferAllocationProfileNameStr + "." + nativeQidStr
		}
		log.V(lvl.DEBUG).Infof("applyBufferAllocationProfileToEgressInterfaceQueue --> %v:%v", qidStr, bufferAllocationProfileNameStr)
		var bufferAllocationProfile db.Value
		if bufferAllocationProfile, ok = bufferQueueTblMap[qidStr]; !ok {
			bufferAllocationProfile = db.Value{Field: make(map[string]string)}
			bufferQueueTblMap[qidStr] = bufferAllocationProfile
		}
	}
	return bufferQueueTblMap, nil
}

func findTrunk(s *ygot.GoStruct, ifName string) string {
	intfObj := getIntfRootbyName(s, ifName)
	if intfObj == nil {
		log.V(lvl.DEBUG).Infof("Interface root object not found")
		return ""
	}
	if intfObj.Ethernet == nil || intfObj.Ethernet.Config == nil {
		log.V(lvl.DEBUG).Infof("Interface ethernet object not found")
		return ""
	}
	if intfObj.Ethernet.Config.AggregateId != nil {
		return *intfObj.Ethernet.Config.AggregateId
	}
	return ""
}

func trunkQueueTables(inParams XfmrParams, ifName string, intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface) (map[string]db.Value, error) {
	tblMap := make(map[string]db.Value)
	trunkStr := findTrunk(inParams.ygRoot, ifName)
	if trunkStr == "" {
		return tblMap, nil
	}
	for queueName := range intfObj.Output.Queues.Queue {
		var err error
		var qidStr string
		if qidStr, err = getKeyForQueueTable(ifName, queueName); err != nil {
			errStr := "trunkQueueTables - Queue object not found for " + queueName
			return tblMap, tlerr.InvalidArgsError{Format: errStr}
		}
		trunkQidStr := strings.Replace(qidStr, ifName, trunkStr, 1)
		tblMap[trunkQidStr] = db.Value{
			Field: map[string]string{
				"NULL": "NULL",
			},
		}
	}
	return tblMap, nil
}

func applyQueueManagementProfileToEgressInterfaceQueue(inParams XfmrParams, ifName string, intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface) (map[string]db.Value, error) {
	queueTblMap := make(map[string]db.Value)
	if intfObj.Output.Queues == nil || intfObj.Output.Queues.Queue == nil || len(intfObj.Output.Queues.Queue) < 1 {
		return queueTblMap, nil
	}
	for queueName := range intfObj.Output.Queues.Queue {
		queueObj, ok := intfObj.Output.Queues.Queue[queueName]
		if !ok {
			errStr := "applyQueueuManagementProfileToEgressInterfaceQueue - Queue object not found for " + queueName
			return queueTblMap, tlerr.InvalidArgsError{Format: errStr}
		}

		var err error
		var qidStr string
		if qidStr, err = getKeyForQueueTable(ifName, queueName); err != nil {
			errStr := "applyQueueManagementProfileToEgressInterfaceQueue - Queue object not found for " + queueName
			return queueTblMap, tlerr.InvalidArgsError{Format: errStr}
		}

		if inParams.oper == DELETE {
			qosEgressIntfQueueManagementProfileDelete(inParams, qidStr)
		}

		// Get the queue management profile name from OC object
		if queueObj == nil {
			errStr := "applyQueueManagementProfileToEgressInterfaceQueue - output queue object not found for " + ifName
			return queueTblMap, tlerr.InvalidArgsError{Format: errStr}
		}
		if queueObj.Config == nil {
			errStr := "applyQueueManagementProfileToEgressInterfaceQueue - output queue config object not found for " + ifName
			return queueTblMap, tlerr.InvalidArgsError{Format: errStr}
		}
		cfgObj := queueObj.Config
		if cfgObj.QueueManagementProfile == nil {
			errStr := "applyQueueManagementProfileToEgressInterfaceQueue - output queue management profile object not found for " + ifName
			return queueTblMap, tlerr.InvalidArgsError{Format: errStr}
		}
		queueManagementProfile := cfgObj.QueueManagementProfile

		prevQueueManagementProfileNameStr := doGetIntfQueueManagementProfile(inParams.d, qidStr)
		queueManagementProfileNameStr := *queueManagementProfile

		log.V(lvl.DEBUG).Infof("applyQueueManagementProfileToEgressInterfaceQueue --> key: %v, queueManagementProfileName: %v", qidStr, queueManagementProfileNameStr)
		mgmtProfile, ok := queueTblMap[qidStr]
		if !ok {
			mgmtProfile = db.Value{Field: make(map[string]string)}
			queueTblMap[qidStr] = mgmtProfile
		}
		mgmtProfile.Field["wred_profile"] = queueManagementProfileNameStr

		if strings.Compare(prevQueueManagementProfileNameStr, queueManagementProfileNameStr) != 0 {
			log.V(lvl.DEBUG).Infof("Modify Case, Prev queue management profile %v New Profile %v", prevQueueManagementProfileNameStr, queueManagementProfileNameStr)
			qosEgressIntfQueueManagementProfileDelete(inParams, ifName)
		}
	}
	return queueTblMap, nil
}

var YangToDb_qos_get_one_intf_all_q_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	log.V(lvl.DEBUG).Infof("YangToDb_qos_get_one_intf_all_q_xfmr: %v", inParams.uri)
	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("interface-id")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.DEBUG).Infoln("error parsing targetUriPath", err)
		return resMap, nil
	}
	// Filter out unnecessary calls to the transformer during SET at root.
	if targetUriPath != "/openconfig-qos:qos/interfaces/interface/output" && inParams.requestUri == "/openconfig-qos:qos" {
		return nil, nil
	}

	/* Check openconfig-qos:qos/interfaces/interface/output is present */
	qosIntfsObj := getQosIntfRoot(inParams.ygRoot)
	if qosIntfsObj == nil || qosIntfsObj.Interface == nil {
		log.V(lvl.WARNING).Infof("Interface root object not found")
		return resMap, nil
	}
	intfObj, ok := qosIntfsObj.Interface[ifName]
	if !ok {
		log.V(lvl.WARNING).Infof("Interface object not found for %v", ifName)
		return resMap, nil
	}
	if intfObj.Output == nil {
		log.V(lvl.WARNING).Infof("Interface output object not found for %v", ifName)
		return resMap, nil
	}

	// Apply queue-management-profile to egress interface queue
	queueTblMap, err := applyQueueManagementProfileToEgressInterfaceQueue(inParams, ifName, intfObj)
	if err == nil {
		resMap["QUEUE"] = queueTblMap
	}

	// If ifName is part of a PortChannel, write a corresponding UMF_TRUNK_QUEUE table
	trunkQueueTblMap, err := trunkQueueTables(inParams, ifName, intfObj)
	if err == nil && len(trunkQueueTblMap) > 0 {
		resMap["UMF_TRUNK_QUEUE"] = trunkQueueTblMap
	}

	// Apply buffer-allocation-profile to egress interface queue
	if intfObj.Output.Config != nil {
		bufferQueueTblMap, err := applyBufferAllocationProfileToEgressInterfaceQueue(inParams, ifName, intfObj.Output.Config)
		if err == nil {
			resMap["BUFFER_QUEUE"] = bufferQueueTblMap
		}
	}

	log.V(lvl.DEBUG).Infof("YangToDb_qos_get_one_intf_all_q_xfmr: End  %v %v", inParams.d, resMap)
	return resMap, nil
}

var DbToYang_qos_get_one_intf_all_q_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infof("DbToYang_qos_get_one_intf_all_q_xfmr - started ")
	qosIntfsObj := getQosIntfRoot(inParams.ygRoot)
	pathInfo := NewPathInfo(inParams.uri)
	intfName := pathInfo.Var("interface-id")
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Infof("DbToYang_qos_get_one_intf_all_q_xfmr - unable to get yang path from uri: %v", err)
		return err
	}

	if strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/queues/queue") {
		queueName := pathInfo.Var("name")
		if queueName != "" {
			log.V(lvl.DEBUG).Infof("DbToYang_qos_get_one_intf_all_q_xfmr - interface specific ")
			return DbToYang_qos_get_one_intf_one_q_counters_xfmr(inParams)
		}
	}

	var intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface
	if qosIntfsObj != nil && qosIntfsObj.Interface != nil && len(qosIntfsObj.Interface) > 0 {
		var ok bool = false
		if intfObj, ok = qosIntfsObj.Interface[intfName]; !ok {
			intfObj, _ = qosIntfsObj.NewInterface(intfName)
		}
		ygot.BuildEmptyTree(intfObj)
	} else {
		ygot.BuildEmptyTree(qosIntfsObj)
		intfObj, _ = qosIntfsObj.NewInterface(intfName)
		ygot.BuildEmptyTree(intfObj)
	}

	if intfObj == nil {
		errStr := "Unable to get intf object " + intfName
		log.V(lvl.ERROR).Infof(errStr)
		err = errors.New(errStr)
		return err
	}
	if intfObj.Output != nil {
		ygot.BuildEmptyTree(intfObj.Output)
	}
	populateEgressInterface(inParams, intfObj)
	populateSchedulerPolicyOnEgressInterface(inParams, intfObj)

	var queuesObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues
	if queuesObj = intfObj.Output.Queues; queuesObj != nil {
		ygot.BuildEmptyTree(queuesObj)
	}
	populateEgressInterfaceQueues(inParams, queuesObj)
	return nil
}

func populateSchedulerPolicyOnEgressInterface(inParams XfmrParams, intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface) error {
	ifName := intfObj.InterfaceId
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Infof("DbToYang_qos_get_one_intf_all_q_xfmr - unable to get yang path from uri: %v", err)
		return err
	}

	scheduler_config, scheduler_state := false, false
	switch {
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output") == 0:
		scheduler_config, scheduler_state = true, true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/scheduler-policy") == 0:
		scheduler_config, scheduler_state = true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/scheduler-policy/config"):
		scheduler_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/scheduler-policy/state"):
		scheduler_state = true
	default:
		return nil
	}

	if intfObj.Output.SchedulerPolicy == nil {
		ygot.BuildEmptyTree(intfObj.Output.SchedulerPolicy)
	}

	if scheduler_config {
		if intfObj.Output.SchedulerPolicy.Config == nil {
			ygot.BuildEmptyTree(intfObj.Output.SchedulerPolicy.Config)
		}
		cfg := intfObj.Output.SchedulerPolicy.Config
		configDb := inParams.dbs[db.ConfigDB]
		if configDb == nil {
			err := errors.New("populateSchedulerPolicyOnEgressInterface: Unable to get ConfigDb")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		schedulerPolicyName := doGetIntfSchedulerPolicy(configDb, *ifName)
		log.V(lvl.DEBUG).Infof("Scheduler policy name %v", schedulerPolicyName)
		cfg.Name = &schedulerPolicyName
	}

	if scheduler_state {
		if intfObj.Output.SchedulerPolicy.State == nil {
			ygot.BuildEmptyTree(intfObj.Output.SchedulerPolicy.State)
		}
		state := intfObj.Output.SchedulerPolicy.State
		applStateDb := inParams.dbs[db.ApplDB]
		if applStateDb == nil {
			err := errors.New("populateSchedulerPolicyOnEgressInterface: Unable to get appStateDb")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		schedulerPolicyName := doGetIntfSchedulerPolicy(applStateDb, *ifName)
		log.V(lvl.DEBUG).Infof("Scheduler policy name %v", schedulerPolicyName)
		state.Name = &schedulerPolicyName
	}

	return nil
}

func populateEgressInterface(inParams XfmrParams, intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface) error {
	var err error
	var targetUriPath string
	ifName := intfObj.InterfaceId
	if targetUriPath, err = getYangPathFromUri(inParams.uri); err != nil {
		log.V(lvl.ERROR).Infof("Unable to get Yang path from uri %v :%v", targetUriPath, err)
		return err
	}

	buffer_profile_config, buffer_profile_state := false, false
	switch {
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output") == 0:
		buffer_profile_config, buffer_profile_state = true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/config"):
		buffer_profile_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/interfaces/interface/output/state"):
		buffer_profile_state = true
	default:
		return nil
	}

	if buffer_profile_state {
		if intfObj.Output.State == nil {
			ygot.BuildEmptyTree(intfObj.Output.State)
		}
		state := intfObj.Output.State
		applStateDb := inParams.dbs[db.ApplDB]
		if applStateDb == nil {
			err = errors.New("populateSchedulerPolicyOnEgressInterface: Unable to get appStateDb")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		bufferAllocationProfileName := doGetIntfBufferAllocationProfile(applStateDb, true, *ifName)
		state.BufferAllocationProfile = &bufferAllocationProfileName
	}

	if buffer_profile_config {
		if intfObj.Output.Config == nil {
			ygot.BuildEmptyTree(intfObj.Output.Config)
		}
		cfg := intfObj.Output.Config
		configDb := inParams.dbs[db.ConfigDB]
		if configDb == nil {
			err = errors.New("populateSchedulerPolicyOnEgressInterface: Unable to get configDb")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		bufferAllocationProfileName := doGetIntfBufferAllocationProfile(configDb, false, *ifName)
		cfg.BufferAllocationProfile = &bufferAllocationProfileName
	}

	return nil
}

// queueTypeAbbrev maps a SAI queue type to the short label used by the
// "show queue counters" CLI (queuestat), so gNMI/OpenConfig queue names can
// match the CLI output (e.g. UC0, MC8).
func queueTypeAbbrev(saiQueueType string) string {
	switch saiQueueType {
	case "SAI_QUEUE_TYPE_UNICAST":
		return "UC"
	case "SAI_QUEUE_TYPE_MULTICAST":
		return "MC"
	case "SAI_QUEUE_TYPE_UNICAST_VOQ":
		return "VOQ"
	case "SAI_QUEUE_TYPE_ALL":
		return "ALL"
	default:
		return ""
	}
}

// populateEgressQueuesFromCounters enumerates an interface's egress queues
// directly from COUNTERS_QUEUE_NAME_MAP / COUNTERS_QUEUE_TYPE_MAP (the same
// source as the "show queue counters" CLI) and fills their OpenConfig state
// counters. It is used as a fallback when no OpenConfig queue-name-to-id map
// (CONFIG_DB QUEUE_NAME_TO_ID_MAP) has been provisioned, so queue-counter
// telemetry works out-of-the-box and mirrors the CLI/native COUNTERS_DB view.
func populateEgressQueuesFromCounters(inParams XfmrParams, queuesObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues, targetUriPath, intfName string, queueToOidMap map[string]string, queueTypeMap db.Value) error {
	stateUriPath := targetUriPath
	if stateUriPath == "/openconfig-qos:qos/interfaces/interface/output" ||
		stateUriPath == "/openconfig-qos:qos/interfaces/interface/output/queues" {
		stateUriPath = "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state"
	}

	for qid, oid := range queueToOidMap {
		// Name matches queuestat: "<type><index>", e.g. UC0, MC8.
		queueName := queueTypeAbbrev(queueTypeMap.Field[oid]) + qid

		queueObj, err := queuesObj.NewQueue(queueName)
		if err != nil {
			log.V(lvl.ERROR).Infof("populateEgressQueuesFromCounters - unable to allocate new queue %v: %v", queueName, err)
			return err
		}
		ygot.BuildEmptyTree(queueObj)
		queueObj.Name = &queueName
		ygot.BuildEmptyTree(queueObj.State)
		state := queueObj.State
		state.Name = &queueName
		if queueObj.Config != nil {
			queueObj.Config.Name = &queueName
		}

		val := uint64(0)
		state.TransmitPkts = &val
		state.TransmitOctets = &val
		state.DroppedPkts = &val

		if err = populateQCounters(inParams, stateUriPath, oid, state); err != nil {
			log.V(tlerr.ErrorSeverity(err)).Info(err)
			continue
		}
	}
	return nil
}

func populateEgressInterfaceQueues(inParams XfmrParams, queuesObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface_Output_Queues) error {
	var present bool

	pathInfo := NewPathInfo(inParams.uri)
	intfName := pathInfo.Var("interface-id")
	countersDb := inParams.dbs[db.CountersDB]
	if countersDb == nil {
		err := errors.New("populateEgressInterfaceQueues: Unable to get counters Db")
		log.V(lvl.ERROR).Info(err)
		return err
	}
	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Infof("populateEgressInterfaceQueues - Unable to get yang path from Uri %v :%v", targetUriPath, err)
		return err
	}

	interfaceToQueueToOidMap, present := inParams.txCache.Load(TXCACHE_INTERFACE_TO_QUEUE_TO_OID_MAP)
	if !present {
		interfaceToQueueToOidMap, err = doGetInterfaceToQueueToOidMap(countersDb)
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("populateEgressInterfaceQueues - unable to get queueOidPerInterfaceMap %v", err)
			return err
		}
		inParams.txCache.Store(TXCACHE_INTERFACE_TO_QUEUE_TO_OID_MAP, interfaceToQueueToOidMap)
	}
	intfQueueOidMap := interfaceToQueueToOidMap.(map[string]map[string]string)

	var queueTypeMap db.Value
	typeMap, present := inParams.txCache.Load(TXCACHE_COUNTERS_QUEUE_TYPE_MAP)
	if !present {
		queueTypeMap, _ = doGetAllQueueTypeMap(countersDb)
		inParams.txCache.Store(TXCACHE_COUNTERS_QUEUE_TYPE_MAP, queueTypeMap)
		log.V(lvl.DEBUG).Infof("Loading queueTypeMap")
	} else {
		queueTypeMap = typeMap.(db.Value)
		log.V(lvl.DEBUG).Infof("Reusing queueTypeMap")
	}

	queueToOidMap, ok := intfQueueOidMap[intfName]
	if !ok {
		log.V(lvl.DEBUG).Infof("populateEgressInterfaceQueues - key not found for %v in queueOidPerInterfaceMap", intfName)
		queueToOidMap = map[string]string{}
	}

	// Get the queues defined for this interface type.
	queueToQidMap := queueToQidMapCacheByType(inParams, getQueueMapType(intfName))

	// When no OpenConfig queue-name-to-id map has been provisioned (the UMF
	// CONFIG_DB QUEUE_NAME_TO_ID_MAP table is empty), fall back to enumerating
	// queues straight from COUNTERS_QUEUE_NAME_MAP so telemetry matches the
	// "show queue counters" CLI without requiring UMF-style QoS config.
	if len(queueToQidMap) == 0 {
		return populateEgressQueuesFromCounters(inParams, queuesObj, targetUriPath, intfName, queueToOidMap, queueTypeMap)
	}

	for queue, qid := range queueToQidMap {
		if queue == "" {
			continue
		}
		queueName := strings.Clone(queue)
		log.V(lvl.DEBUG).Infof("populateEgressInterfaceQueues: %v", queueName)

		queueObj, err := queuesObj.NewQueue(queueName)
		if err != nil {
			log.V(lvl.ERROR).Infof("populateEgressInterfaceQueues - unable to allocate new queue %v", err)
			return err
		}

		ygot.BuildEmptyTree(queueObj)
		queueObj.Name = &queueName
		if queueObj.State == nil {
			ygot.BuildEmptyTree(queueObj.State)
		}

		state := queueObj.State
		if state == nil {
			err = errors.New("populateEgressInterfaceQueues: state is nil")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		state.Name = &queueName

		cfg := queueObj.Config
		if cfg == nil {
			err = errors.New("populateEgressInterfaceQueues: cfg is nil")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		cfg.Name = &queueName

		if targetUriPath == "/openconfig-qos:qos/interfaces/interface/output" ||
			targetUriPath == "/openconfig-qos:qos/interfaces/interface/output/queues" {
			targetUriPath = "/openconfig-qos:qos/interfaces/interface/output/queues/queue/state"
		}

		var dbQueueName string
		if dbQueueName, err = getMappedDbQueueName(intfName, queueName); err != nil {
			if dbQueueName, err = getDbQueueName(queueName); err != nil {
				log.V(lvl.ERROR).Infof("populateEgressInterfaceQueues - invalid queue %v", queueName)
				return err
			}
		}

		applStateDb := inParams.dbs[db.ApplDB]
		if applStateDb == nil {
			err = errors.New("populateEgressInterfaceQueues: Unable to get appState Db")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		queueManagementProfileName := doGetIntfQueueManagementProfile(applStateDb, strings.Replace(dbQueueName, ":", "|", 1))
		state.QueueManagementProfile = &queueManagementProfileName

		configDb := inParams.dbs[db.ConfigDB]
		if configDb == nil {
			err = errors.New("populateEgressInterfaceQueues: Unable to get ConfigDb")
			log.V(lvl.ERROR).Info(err)
			return err
		}
		queueManagementProfileName = doGetIntfQueueManagementProfile(configDb, strings.Replace(dbQueueName, ":", "|", 1))
		cfg.QueueManagementProfile = &queueManagementProfileName

		val := uint64(0)
		state.TransmitPkts = &val
		state.TransmitOctets = &val
		state.DroppedPkts = &val
		oid, ok := queueToOidMap[qid]
		if !ok {
			continue
		}
		if err = populateQCounters(inParams, targetUriPath, oid, state); err != nil {
			log.V(tlerr.ErrorSeverity(err)).Info(err)
			continue
		}
	}

	return nil
}

func normalizeQueueNameKey(pathInfo *PathInfo, ifName string) (string, error) {
	queueName := pathInfo.Var("name")
	if queueName == "" || queueName == "*" {
		return "*", nil
	}
	if strings.Contains(queueName, ":") {
		// Expected format of queueName: <interface>:<qid>, e.g. Ethernet1/1/1:0
		queue := strings.Split(queueName, ":")
		if len(queue) < 2 {
			return "", fmt.Errorf("Invalid queue name in key (%v)", queueName)
		}
		return queue[1], nil
	}
	return getNativeQueueName(ifName, queueName)
}

var Subscribe_qos_get_one_intf_all_q_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	result := XfmrSubscOutParams{
		dbDataMap:    RedisDbSubscribeMap{},
		needCache:    true,
		onChange:     OnchangeDisable,
		nOpts:        &notificationOpts{mInterval: 0, pType: Sample},
		isVirtualTbl: false}

	log.V(lvl.DEBUG).Infof("Subscribe_qos_get_one_intf_all_q_xfmr: %+v", inParams)
	defer log.V(lvl.DEBUG).Infof("Subscribe_qos_get_one_intf_all_q_xfmr result: %+v", result)

	pathInfo := NewPathInfo(inParams.uri)
	ifName := pathInfo.Var("interface-id")
	isTrunk := false
	if ifName == "" {
		ifName = "*"
	} else if strings.HasPrefix(ifName, PORTCHANNEL) {
		isTrunk = true
	}

	queueName, err := normalizeQueueNameKey(pathInfo, ifName)
	if err != nil {
		return result, err
	}
	log.V(lvl.DEBUG).Infof("ifName=%v, queueName=%v", ifName, queueName)
	fieldMapCountersTbl := map[string]string{}

	entryKey := ifName + "|" + queueName
	if ifName == "*" {
		result.dbDataMap = RedisDbSubscribeMap{
			db.ConfigDB: {
				"QUEUE":           {entryKey: fieldMapCountersTbl},
				"UMF_TRUNK_QUEUE": {entryKey: fieldMapCountersTbl},
			}}
	} else if isTrunk {
		result.dbDataMap = RedisDbSubscribeMap{
			db.ConfigDB: {
				"UMF_TRUNK_QUEUE": {entryKey: fieldMapCountersTbl},
			}}
	} else {
		result.dbDataMap = RedisDbSubscribeMap{
			db.ConfigDB: {
				"QUEUE": {entryKey: fieldMapCountersTbl},
			}}
	}
	return result, nil
}

var qos_intf_table_xfmr TableXfmrFunc = func(inParams XfmrParams) ([]string, error) {
	var tblList []string
	var key string

	pathInfo := NewPathInfo(inParams.uri)
	targetUriPath, err := getYangPathFromUri(pathInfo.Path)
	if err != nil {
		log.V(lvl.ERROR).Infof("qos_intf_table_xfmr - Unable to get yang path from uri %v %v", targetUriPath, err)
		return tblList, err
	}

	ifName := pathInfo.Var("interface-id")
	log.V(lvl.DEBUG).Infof("qos_intf_table_xfmr - Uri: %v requestUri %v", inParams.uri, inParams.requestUri,
		" targetUriPath ", targetUriPath, " ifName ", ifName)
	if ifName == "" {
		ifName = "*"
	}

	key = ifName
	log.V(lvl.DEBUG).Infof("qos_intf_table_xfmr - intf_table_xfmr Intf key is present, curr DB %v", inParams.curDb)

	switch {
	case strings.HasPrefix(ifName, ETHERNET):
		tblList = []string{PORT_TN}
	case strings.HasPrefix(ifName, CPU):
		tblList = []string{"CPU_PORT"}
	case strings.HasPrefix(ifName, PORTCHANNEL):
		tblList = []string{PORTCHANNEL_TN}
	case ifName == "*" || ifName == "":
		tblList = []string{PORT_TN, "CPU_PORT", PORTCHANNEL_TN}
		return tblList, nil
	default:
		log.V(lvl.DEBUG).Infof("qos_intf_table_xfmr - Invalid interface type")
		return tblList, errors.New("Invalid interface type")
	}

	if inParams.dbDataMap != nil {
		for _, name := range tblList {
			if _, ok := (*inParams.dbDataMap)[db.ConfigDB][name]; !ok {
				(*inParams.dbDataMap)[db.ConfigDB][name] = make(map[string]db.Value)
			}
			if _, ok := (*inParams.dbDataMap)[db.ConfigDB][name][key]; !ok {
				(*inParams.dbDataMap)[db.ConfigDB][name][key] = db.Value{Field: make(map[string]string)}
				(*inParams.dbDataMap)[db.ConfigDB][name][key].Field["NULL"] = "NULL"
			}
		}
	}
	return tblList, nil
}
var YangToDb_qos_intf_tbl_key_xfmr KeyXfmrYangToDb = func(inParams XfmrParams) (string, error) {
	var err error
	var ifName string
	pathInfo := NewPathInfo(inParams.uri)
	ifName = pathInfo.Var("interface-id")
	log.V(lvl.DEBUG).Infof("Entering YangToDb_qos_intf_tbl_key_xfmr Uri %v interface %v", inParams.uri, ifName)
	return ifName, err
}

var DbToYang_qos_intf_tbl_key_xfmr KeyXfmrDbToYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	resMap := make(map[string]interface{})

	resMap["interface-id"] = inParams.key
	log.V(lvl.DEBUG).Infof("Entering DbToYang_qos_intf_tbl_key_xfmr - End %v resMap %v", inParams.uri, resMap)
	return resMap, nil
}

var YangToDb_qos_intf_intf_id_fld_xfmr FieldXfmrYangToDb = func(inParams XfmrParams) (map[string]string, error) {
	resMap := make(map[string]string)
	requestUriPath, _ := getYangPathFromUri(inParams.requestUri)
	if inParams.oper != GET && requestUriPath == "/openconfig-qos:qos/interfaces/interface/interface-id" {
		return resMap, tlerr.NotSupported("Operation Not Supported")
	}
	return resMap, nil
}

var DbToYang_qos_intf_intf_id_fld_xfmr FieldXfmrDbtoYang = func(inParams XfmrParams) (map[string]interface{}, error) {
	resMap := make(map[string]interface{})
	resMap["interface-id"] = inParams.key
	return resMap, nil
}

func qos_interface_post_xfmr(inParams XfmrParams, retDbDataMap map[string]map[string]db.Value, ifName string) error {

	qos_port_tbl := "PORT_QOS_MAP"
	if ifName == "CPU" {
		if _, ok := retDbDataMap[qos_port_tbl]; ok {
			log.V(lvl.DEBUG).Infof("qos_post_xfmr - Skip CPU delete")
			delete(retDbDataMap[qos_port_tbl], ifName)
		}
		return nil
	}
	if _, ok := (*inParams.dbDataMap)[db.ConfigDB][qos_port_tbl][ifName]; ok {
		retDbDataMap[qos_port_tbl][ifName] = db.Value{Field: make(map[string]string)}
	} else if strings.HasPrefix(ifName, "PortChannel") {
		log.V(lvl.DEBUG).Infof("qos_post_xfmr - Update PortChannel to delete, ifName %v", ifName)
		entry, err := inParams.d.GetEntry(&db.TableSpec{Name: qos_port_tbl}, db.Key{Comp: []string{ifName}})
		if err != nil || entry.IsPopulated() {
			if _, ok := retDbDataMap[qos_port_tbl]; !ok {
				retDbDataMap[qos_port_tbl] = make(map[string]db.Value)
			}
			retDbDataMap[qos_port_tbl][ifName] = db.Value{Field: make(map[string]string)}
		}
	}

	return nil
}

func qos_queue_post_xfmr(inParams XfmrParams, retDbDataMap map[string]map[string]db.Value, dbQKey db.Key) error {

	qCfg, _ := inParams.d.GetEntry(&db.TableSpec{Name: "QUEUE"}, dbQKey)
	var q_sched_present bool = false
	_, ok := qCfg.Field["scheduler"]
	if ok {
		q_sched_present = true
	}

	var qKey_str string = dbQKey.Get(0) + "|" + dbQKey.Get(1)
	if strings.Contains(qKey_str, "CPU") {
		delete(retDbDataMap["QUEUE"], qKey_str)
		log.V(lvl.DEBUG).Infof("qos_post_xfmr- Skip delete CPU QUEUE Settings - %v", qKey_str)
		return nil
	}

	log.V(lvl.DEBUG).Infof("qos_post_xfmr- Update QUEUE with wred_policy - %v", qKey_str)
	if _, ok := retDbDataMap["QUEUE"][qKey_str]; !ok {
		retDbDataMap["QUEUE"][qKey_str] = db.Value{Field: make(map[string]string)}
	}

	/* If only WRED field, then delete QUEUE entry else delete only wred_profile */
	if q_sched_present {
		retDbDataMap["QUEUE"][qKey_str].Field["wred_profile"] = ""
	} else {
		retDbDataMap["QUEUE"][qKey_str] = db.Value{Field: make(map[string]string)}
	}

	return nil
}

func findSchedNames(schedPolicyObj *ocbinds.OpenconfigQos_Qos_SchedulerPolicies) (cpuSchedName, fpSchedName string) {
	for sp := range schedPolicyObj.SchedulerPolicy {
		if strings.HasPrefix(sp, "cpu") {
			cpuSchedName = sp
			break
		}
	}
	for sp := range schedPolicyObj.SchedulerPolicy {
		if !strings.HasPrefix(sp, "cpu") {
			fpSchedName = sp
			break
		}
	}
	return cpuSchedName, fpSchedName
}

func getMaxSchedNum(sp map[uint32]*ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler) (maxSchedNum uint32) {
	for seqVal := range sp {
		maxSchedNum = seqVal
		break
	}
	for seqVal := range sp {
		if maxSchedNum < seqVal {
			maxSchedNum = seqVal
		}
	}
	return maxSchedNum
}

func newQueueNameToIdMap() map[string]map[string]string {
	qm := make(map[string]map[string]string)
	qm[CPU] = make(map[string]string)
	qm[FRONT_PANEL] = make(map[string]string)
	return qm
}

func commitToGlobalMap(m map[string]map[string]string) {
	queueMapMutex.Lock()
	defer queueMapMutex.Unlock()
	ocQueueHwQueueMap = newQueueNameToIdMap()
	for _, qtype := range []string{CPU, FRONT_PANEL} {
		for k, v := range m[qtype] {
			ocQueueHwQueueMap[qtype][k] = v
		}
	}
}

func isPriorityGroupBufferAllocationProfile(bapName string) bool {
	return (strings.Contains(bapName, "lossless") || strings.Contains(bapName, "ingress"))
}

func getQueueToTypeMap(qosObj *ocbinds.OpenconfigQos_Qos) (map[string]string, error) {
	qnameToTypeMap := make(map[string]string)
	if qosObj.BufferAllocationProfiles == nil || qosObj.BufferAllocationProfiles.BufferAllocationProfile == nil || len(qosObj.BufferAllocationProfiles.BufferAllocationProfile) < 1 {
		return nil, errors.New("No buffer-allocation-profiles subtree populated")
	}
	bapObjList := qosObj.BufferAllocationProfiles.BufferAllocationProfile
	for bapName, bapObj := range bapObjList {
		if isPriorityGroupBufferAllocationProfile(bapName) {
			continue
		}
		intType := FRONT_PANEL
		if strings.HasPrefix(bapName, "cpu") {
			intType = CPU
		}
		if bapObj.Queues == nil || bapObj.Queues.Queue == nil {
			return nil, errors.New("Queues missing from buffer-allocation-profiles subtree")
		}
		for queueName := range bapObj.Queues.Queue {
			qnameToTypeMap[queueName] = intType
		}
	}
	log.V(lvl.DEBUG).Infof("generated qnameToTypeMap: %v", qnameToTypeMap)
	return qnameToTypeMap, nil
}

func prepareQueueNameToIdMap(qosObj *ocbinds.OpenconfigQos_Qos) error {
	qm := newQueueNameToIdMap()
	qtm, err := getQueueToTypeMap(qosObj)
	if err == nil {
		idsFound := false
		if qosObj.Queues != nil && qosObj.Queues.Queue != nil {
			for queueName, queueObj := range qosObj.Queues.Queue {
				qconfigObj := queueObj.Config
				if qconfigObj == nil || qconfigObj.QueueId == nil {
					continue
				}
				idsFound = true
				if intfType, ok := qtm[queueName]; ok {
					qm[intfType][queueName] = strconv.Itoa(int(*qconfigObj.QueueId))
				} else {
					log.V(lvl.DEBUG).Infof("qtm miss! queue name = %v", queueName)
				}

			}
		}
		if idsFound {
			log.V(lvl.INFO).Infof("Skipping scheduler based derivation, since queue-id's provided")
			commitToGlobalMap(qm)
			return nil
		}
	}

	//
	// Fallback to the scheduler based derivation
	//
	if qosObj.SchedulerPolicies == nil || qosObj.SchedulerPolicies.SchedulerPolicy == nil || len(qosObj.SchedulerPolicies.SchedulerPolicy) < 1 {
		log.V(lvl.INFO).Infoln("No scheduler subtree populated")
		return nil
	}

	queueMapMutex.Lock()
	defer queueMapMutex.Unlock()
	ocQueueHwQueueMap = newQueueNameToIdMap()
	cpuSchedName, fpSchedName := findSchedNames(qosObj.SchedulerPolicies)
	if err := generateOCToHWQueueMap(cpuSchedName, CPU, qosObj.SchedulerPolicies.SchedulerPolicy); err != nil {
		return err
	}
	if err := generateOCToHWQueueMap(fpSchedName, FRONT_PANEL, qosObj.SchedulerPolicies.SchedulerPolicy); err != nil {
		return err
	}
	return nil
}

func generateOCToHWQueueMap(spName, qMapType string, sp map[string]*ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy) error {
	spObj, ok := sp[spName]
	if !ok {
		log.V(lvl.DEBUG).Infoln("YangToDb: No policy object for ", spName)
		return nil
	}
	if spObj.Schedulers == nil || spObj.Schedulers.Scheduler == nil || len(spObj.Schedulers.Scheduler) < 1 {
		return nil
	}

	maxSchedNum := getMaxSchedNum(spObj.Schedulers.Scheduler)
	for seqVal := range spObj.Schedulers.Scheduler {
		hwQueueID := maxSchedNum - seqVal
		log.V(lvl.DEBUG).Infof("YangToDb: hw queue id = %v, maxSchedNum = %v, seqVal = %v", hwQueueID, maxSchedNum, seqVal)

		schedObj, ok := spObj.Schedulers.Scheduler[uint32(seqVal)]
		if !ok {
			log.V(lvl.DEBUG).Infoln("YangToDb: No Scheduler obj: ", spName, " seqVal ", seqVal)
			return nil
		}
		if schedObj.Inputs == nil || schedObj.Inputs.Input == nil || len(schedObj.Inputs.Input) < 1 {
			log.V(lvl.DEBUG).Infoln("YangToDb: No Input subtree present for Scheduler obj: ", spName, " seqVal ", seqVal)
			continue
		}
		for idx := range schedObj.Inputs.Input {
			log.V(lvl.DEBUG).Infoln("YangToDb: Scheduler obj: ", spName, " seqVal ", idx)
			if idx == "" {
				log.V(lvl.DEBUG).Infoln("YangToDb: no input id specified")
				return errors.New("missing data, no input specified")
			}
			idxObj, ok := schedObj.Inputs.Input[idx]
			if !ok {
				log.V(lvl.DEBUG).Infoln("YangToDb: No input obj: ", spName, " sequence: ", seqVal, " ID: ", idx)
				return errors.New("missing data, no input object present")
			}
			if idxObj.Config != nil {
				if idxObj.Config.Id == nil {
					// id cannot be empty
					return errors.New("missing data, no input/config/id specified")
				}
				queueName := *idxObj.Config.Id
				if idxObj.Config.Queue != nil {
					queueName = *idxObj.Config.Queue
				}
				queue_id := strconv.Itoa(int(hwQueueID))
				qMap, ok := ocQueueHwQueueMap[qMapType]
				if !ok {
					return errors.New("Failed to get qos queue map: " + qMapType)
				}
				if hw_queue_id, ok := qMap[queueName]; !ok {
					qMap[queueName] = queue_id
					log.V(lvl.DEBUG).Infof("YangToDb: oc queue to hw queue mapping (%v) ", ocQueueHwQueueMap)
				} else if queue_id != hw_queue_id {
					return fmt.Errorf("Failed consistent config check for oc queue (%v) to hw queue mapping (%v) for queue (%v)", queue_id, hw_queue_id, queueName)
				}
			}
		}
	}
	return nil
}

var qos_pre_xfmr PreXfmrFunc = func(inParams XfmrParams) error {
	if inParams.oper == GET || inParams.oper == DELETE || inParams.oper == UPDATE {
		if ocQueueHwQueueMap != nil {
			return nil
		}
		return buildQueueNameToIdMapFromDb(inParams.d)
	}
	log.V(lvl.DEBUG).Infof("qos_pre_xfmr start : OCToHWQueueMap (%v)", ocQueueHwQueueMap)

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		log.V(lvl.DEBUG).Infoln("QoS tree is not populated")
		return nil
	}

	if err := prepareQueueNameToIdMap(qosObj); err != nil {
		return err
	}
	log.V(lvl.DEBUG).Infof("generated OCToHWQueueMap: %v", ocQueueHwQueueMap)
	return nil
}

// buildQueueNameToIdMapFromDb reads the queue name to id map. A caller running
// inside a request passes that request's ConfigDB handle: translib holds the
// ConfigDB lock for the duration of a request, so opening a second writable
// ConfigDB connection here would fail with a resource lock error.
func buildQueueNameToIdMapFromDb(d *db.DB) error {
	if d == nil {
		ownDb, err := db.NewDB(getDBOptions(db.ConfigDB))
		if err != nil {
			return err
		}
		defer ownDb.DeleteDB()
		d = ownDb
	}

	queueMapMutex.Lock()
	defer queueMapMutex.Unlock()

	// Only build this map if uninitialized.
	if ocQueueHwQueueMap != nil {
		return nil
	}

	ocQueueHwQueueMap = newQueueNameToIdMap()

	for _, qMapType := range queueTypes {
		entry, dbErr := d.GetEntry(&db.TableSpec{Name: QUEUE_NAME_TO_ID_MAP}, db.Key{Comp: []string{qMapType}})
		if dbErr != nil {
			log.V(tlerr.ErrorSeverity(dbErr)).Info("Failed to read entry from config DB, " + QUEUE_NAME_TO_ID_MAP + " " + qMapType)
			continue
		}
		for qName, qID := range entry.Field {
			ocQueueHwQueueMap[qMapType][qName] = qID
		}
	}
	return nil
}

var qos_post_xfmr PostXfmrFunc = func(inParams XfmrParams) error {

	log.V(lvl.DEBUG).Infof("qos_post_xfmr - inParams.uri: %v", inParams.uri)

	if inParams.dbDataMap != nil {
		requestUriPath, _ := getYangPathFromUri(inParams.requestUri)
		retDbDataMap := (*inParams.dbDataMap)[db.ConfigDB]
		if inParams.oper == DELETE {
			log.V(lvl.DEBUG).Infof(" qos_post_xfmr - Received retDbDataMap from xfmrs %v", retDbDataMap)

			if requestUriPath == "/openconfig-qos:qos/interfaces/interface" ||
				requestUriPath == "/openconfig-qos:qos/interfaces" ||
				requestUriPath == "/openconfig-qos:qos" {

				pathInfo := NewPathInfo(inParams.uri)
				ifName := pathInfo.Var("interface-id")

				log.V(lvl.DEBUG).Infof("qos_post_xfmr - Uri ifName: %v", ifName)

				/* DELETE at interface/interfaces/qos will trigger every sub trees.
				* Infra will not include direct fields like interface-maps to clear.
				* We need to include map fileds or delete fileds return from other subtrees.
				* Delete interface can remove PORT_QOS_MAP entry. Overwirte map entry with
				* empty fileds to clear everything.
				*
				*
				* To flush maps on Portchannel, add explicity port_qos_map entry
				* CPU is assigned with copp, do not allow delete.
				 */
				qos_port_tbl := "PORT_QOS_MAP"

				if len(ifName) != 0 {
					qos_interface_post_xfmr(inParams, retDbDataMap, ifName)
				} else {
					/* Delete all port_qos_map entries */
					mapIntfKeys, _ := inParams.d.GetKeys(&db.TableSpec{Name: qos_port_tbl})
					if len(mapIntfKeys) > 0 {
						for _, mapIntfKey := range mapIntfKeys {
							ifName := mapIntfKey.Get(0)
							key := ifName
							qos_interface_post_xfmr(inParams, retDbDataMap, key)
						}
					}
				}
			}
			/* QUEUE table has 2 fields scheduler and wred_policy.
			* Scheduler will be deleted from interface/schduelr-policy, should not delete entire QUEUE table
			* by request /openconfig-qos:qos/queues/queue or /openconfig-qos:qos/queues
			* Delete only wred_policy
			* From Xfmr we can get empty QUEUE map. Should avoid because it clears COPP configs
			 */
			if requestUriPath == "/openconfig-qos:qos/queues/queue" ||
				requestUriPath == "/openconfig-qos:qos/queues" ||
				requestUriPath == "/openconfig-qos:qos" {
				log.V(lvl.DEBUG).Infof("qos_post_xfmr - Delete Queues/queue, Updated retDbDataMap: %v", retDbDataMap)
				for table, keyInstance := range retDbDataMap {
					if table != "QUEUE" {
						continue
					}

					if requestUriPath == "/openconfig-qos:qos/queues/queue" {
						for dbKey := range keyInstance {
							if strings.Contains(dbKey, "CPU") {
								return tlerr.NotSupported("Operation Not Supported")
							} else {
								s := strings.Split(dbKey, "|")
								qKey := db.Key{Comp: []string{s[0], s[1]}}
								qos_queue_post_xfmr(inParams, retDbDataMap, qKey)
							}
						}
					} else {
						log.V(lvl.DEBUG).Infof("qos_post_xfmr - Delete all queues wred_policy")
						qKeys, _ := inParams.d.GetKeys(&db.TableSpec{Name: "QUEUE"})
						if len(qKeys) > 0 {
							for _, qKey := range qKeys {
								qos_queue_post_xfmr(inParams, retDbDataMap, qKey)
							}
						}
						/* To Avoid CPU copp scheduler cleanup on queue */
						if len(retDbDataMap["QUEUE"]) == 0 {
							log.V(lvl.DEBUG).Infof("qos_post_xfmr- Remove empty QUEUE table, to avoid COPP configs remove")
							delete(retDbDataMap, "QUEUE")
						}
					}
				}
			}

			if requestUriPath == "/openconfig-qos:qos" {
				log.V(lvl.DEBUG).Infof("qos_post_xfmr - Updates retDbDataMap while scheduler %v", retDbDataMap)
				log.V(lvl.DEBUG).Infof("qos_post_xfmr - Delete All schedulers except copp")
				sKeys, _ := inParams.d.GetKeys(&db.TableSpec{Name: "SCHEDULER"})
				if len(sKeys) > 0 {
					for _, sKey := range sKeys {
						var sKey_str string = sKey.Get(0)
						if strings.Contains(sKey_str, "copp-scheduler-policy") {
							log.V(lvl.DEBUG).Infof("qos_post_xfmr - Skip add CPU copp scheduler to delete - %v", sKey_str)
							continue
						}

						log.V(lvl.DEBUG).Infof("qos_post_xfmr: Update scheduler to delete - %v", sKey_str)
						if _, ok := retDbDataMap["SCHEDULER"]; !ok {
							retDbDataMap["SCHEDULER"] = make(map[string]db.Value)
						}

						if _, ok := retDbDataMap["SCHEDULER"][sKey_str]; !ok {
							retDbDataMap["SCHEDULER"][sKey_str] = db.Value{Field: make(map[string]string)}
						}
					}
				}
				/* Avoid Delete Copp scheduler policy */
				if _, ok := retDbDataMap["SCHEDULER"]; ok {
					if len(retDbDataMap["SCHEDULER"]) == 0 {
						log.V(lvl.DEBUG).Infof("qos_post_xfmr- Remove empty SCHEDULER table, to avoid COPP configs remove")
						delete(retDbDataMap, "SCHEDULER")
					}
				}
			}
		} else if inParams.oper == REPLACE {
			// Save updated queue name to id map to Config DB
			if requestUriPath == "/openconfig-qos:qos" {
				if ocQueueHwQueueMap != nil {
					retDbDataMap[QUEUE_NAME_TO_ID_MAP] = make(map[string]db.Value)
					retDbDataMap["UMF_QUEUE"] = make(map[string]db.Value)
					queueMapMutex.RLock()
					defer queueMapMutex.RUnlock()
					for _, qMapType := range queueTypes {
						retDbDataMap[QUEUE_NAME_TO_ID_MAP][qMapType] = db.Value{Field: make(map[string]string)}
						if ocQueueHwQueueMap[qMapType] == nil {
							continue
						}
						for qName, qID := range ocQueueHwQueueMap[qMapType] {
							retDbDataMap[QUEUE_NAME_TO_ID_MAP][qMapType].Field[qName] = qID
							if _, ok := retDbDataMap["UMF_QUEUE"][qName]; !ok {
								retDbDataMap["UMF_QUEUE"][qName] = db.Value{Field: make(map[string]string)}
								retDbDataMap["UMF_QUEUE"][qName].Field["NULL"] = "NULL"
							}
						}
					}
				}

				impliedTableDelete(inParams, "BUFFER_QUEUE")
				impliedTableDelete(inParams, BUFFER_PG)
				impliedTableDelete(inParams, QosFwdGroup)
			}

			// Cleanup unused BUFFER_PROFILEs
			if requestUriPath == "/openconfig-qos:qos" || requestUriPath == "/openconfig-qos:qos/buffer-allocation-profiles" {
				impliedTableDelete(inParams, "BUFFER_PROFILE")
			}
		}
		log.V(lvl.DEBUG).Infof("qos_post_xfmr- Return result : %v", retDbDataMap)
		return nil
	}
	return nil
}

// Compares the intent carried by inParams with the deployed state in ConfigDb
// Deployed tables not found in the intent will be staged for deletion
//
// This function should only be at the appropriate request scope for tableName
func impliedTableDelete(inParams XfmrParams, tableName string) {
	if inParams.dbDataMap == nil || inParams.oper != REPLACE {
		return
	}

	keys, err := inParams.d.GetKeys(&db.TableSpec{Name: tableName})
	if err != nil {
		log.V(lvl.ERROR).Infof("Error returned querying for %s keys : %v", tableName, err)
		return
	}

	intentConfigDbMap := (*inParams.dbDataMap)[db.ConfigDB]
	toDelete := map[string]db.Value{}
	for _, k := range keys {
		key := strings.Join(k.Comp, inParams.d.Opts.KeySeparator)
		if _, ok := intentConfigDbMap[tableName][key]; !ok {
			toDelete[key] = db.Value{}
		}
	}
	if len(toDelete) > 0 {
		log.V(lvl.DEBUG).Infof("%s cleanup, toDelete: %v", tableName, toDelete)
		subOpMap := map[db.DBNum]map[string]map[string]db.Value{
			db.ConfigDB: map[string]map[string]db.Value{
				tableName: toDelete,
			},
		}
		updateSubOpDataMap(subOpMap, DELETE, inParams)
	}
}

var DbToYangPath_qos_get_one_intf_all_q_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	rootPath := "/openconfig-qos:qos/interfaces/interface"

	log.V(lvl.DEBUG).Infof("DbToYangPath_qos_get_one_intf_all_q_path_xfmr: inParams: %v", inParams)

	if len(inParams.tblKeyComp) != 2 {
		return fmt.Errorf("Invalid tblKeyCom for path xmfr:%v", inParams.tblKeyComp)
	}

	ifName := inParams.tblKeyComp[0]
	inParams.ygPathKeys[rootPath+"/interface-id"] = ifName

	queueName, err := getOCQueueName(ifName, inParams.tblKeyComp[1])
	if err != nil {
		log.V(lvl.DEBUG).Infof("DbToYangPath_qos_get_one_intf_all_q_path_xfmr:- err: %v", err)
		return err
	}

	inParams.ygPathKeys[rootPath+"/output/queues/queue/name"] = queueName

	log.V(lvl.DEBUG).Infof("DbToYangPath_qos_get_one_intf_all_q_path_xfmr:- params.ygPathKeys: %v", inParams.ygPathKeys)

	return nil
}
