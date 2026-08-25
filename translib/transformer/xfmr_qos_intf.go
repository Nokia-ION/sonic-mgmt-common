package transformer

import (
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
)

const qosQueueTbl = "QUEUE"

func init() {
	parseQueueMapJSONFile()
}

// return the first matching INTF Queue entry with a valid Queue Management Profile
func doGetIntfQueueManagementProfile(d *db.DB, qidStr string) string {
	log.V(lvl.DEBUG).Info("doGetIntfQueueuManagementProfile: ", qidStr, d)
	if d == nil {
		log.V(lvl.ERROR).Infof("ConfigDB pointer is nil")
		return ""
	}

	// qidStr will look like Ethernet1/14/5|11.
	qCfg, err := d.GetEntry(&db.TableSpec{Name: "QUEUE"}, *db.NewKey(qidStr))
	if err != nil {
		log.V(tlerr.ErrorSeverity(err)).Infof("Unable to read QUEUE|%v entry from ConfigDB; error=%v", qidStr, err)
		return ""
	}
	if queueManagementProfile, ok := qCfg.Field["wred_profile"]; ok {
		return queueManagementProfile
	}

	log.V(lvl.DEBUG).Infof("doGetIntfQueueManagementProfile: null")
	return ""
}

func qosEgressIntfQueueManagementProfileDelete(inParams XfmrParams, qidStr string) (map[string]map[string]db.Value, error) {
	var err error
	resMap := make(map[string]map[string]db.Value)

	log.V(lvl.DEBUG).Info("qosEgressIntfQueueManagementProfileDelete: ", inParams.ygRoot, inParams.uri)
	queueTblMap := make(map[string]db.Value)
	d := inParams.d
	if d == nil {
		log.V(lvl.DEBUG).Infof("unable to get configDB")
		return resMap, err
	}

	dbSpec := &db.TableSpec{Name: "QUEUE"}
	keys, err := d.GetKeysByPattern(dbSpec, qidStr)
	if err != nil {
		log.V(lvl.DEBUG).Infof("unable to get keys matching ", qidStr)
		return resMap, err
	}

	for _, key := range keys {
		if len(key.Comp) < 1 {
			continue
		}
		qCfg, err := d.GetEntry(dbSpec, key)
		if err != nil {
			log.V(lvl.DEBUG).Infof("unable to get qCfg: ", key)
			continue
		}
		if _, ok := qCfg.Field["wred_profile"]; ok {
			new_key := key.Comp[0]
			new_key = new_key + "|" + key.Comp[1]
			if _, ok := queueTblMap[new_key]; !ok {
				queueTblMap[new_key] = db.Value{Field: make(map[string]string)}
			}
			queueTblMap[new_key].Field["wred_profile"] = ""
		}
	}

	resMap["QUEUE"] = queueTblMap
	log.V(lvl.DEBUG).Info("qosEgressIntfQueueManagementProfileDelete: End resMap ", resMap)
	return resMap, err
}

// return the first matching QUEUE or PORT_QOS_MAP entry with a schedule policy containing an '@',
// else the last observed policy without an `@`, or an empty string
func doGetIntfSchedulerPolicy(d *db.DB, if_name string) string {
	log.V(lvl.DEBUG).Info("doGetIntfSchedulerPolicy: if_name ", if_name)
	if d == nil {
		log.V(lvl.DEBUG).Infof("unable to get DB")
		return ""
	}

	// QUEUE or PORT_QOS_MAP
	var keyPattern string
	lastPolicy := ""
	tbl_list := []string{"QUEUE", "PORT_QOS_MAP"}
	for _, tbl_name := range tbl_list {
		dbSpec := &db.TableSpec{Name: tbl_name}
		if tbl_name == "PORT_QOS_MAP" {
			keyPattern = if_name
		} else {
			keyPattern = if_name + "|*"
		}
		log.V(lvl.DEBUG).Infof("doGetIntfSchedulerPolicy: d=%v", d)
		log.V(lvl.DEBUG).Infof("doGetIntfSchedulerPolicy: keyPattern=%v", keyPattern)
		keys, _ := d.GetKeysByPattern(dbSpec, keyPattern)
		for _, key := range keys {
			if len(key.Comp) < 1 {
				continue
			}
			log.V(lvl.DEBUG).Infof("doGetIntfSchedulerPolicy key[0]=%v", key.Comp[0])
			s := strings.Split(key.Comp[0], "|")
			if strings.HasPrefix(if_name, s[0]) {
				qCfg, err := d.GetEntry(dbSpec, key)
				if err != nil {
					log.V(lvl.DEBUG).Infof("unable to get qCfg: ", key)
					continue
				}
				log.V(lvl.DEBUG).Info("current entry: ", qCfg)
				sched, ok := qCfg.Field["scheduler"]
				if ok {
					log.V(lvl.DEBUG).Info("sched: ", sched)
					sched = sched
					sp := strings.Split(sched, "@")
					log.V(lvl.DEBUG).Info("sp[0]: ", sp[0])
					log.V(lvl.DEBUG).Info("Scheduler policy: ", sp[0])
					if len(sp) > 1 {
						return sp[0]
					}
					lastPolicy = sp[0]
				}
			}
		}
	}

	return lastPolicy
}

// qosSchedPolicySchedulers lists the SCHEDULER keys belonging to one policy.
// The keys are <policy>@<sequence>, so a prefix match alone would also collect
// a policy whose name merely starts with this one.
func qosSchedPolicySchedulers(d *db.DB, spName string) []string {
	if d == nil || spName == "" {
		return nil
	}
	keys, err := d.GetKeysByPattern(&db.TableSpec{Name: "SCHEDULER"}, spName+"@*")
	if err != nil {
		log.V(lvl.DEBUG).Infof("qosSchedPolicySchedulers: no keys for %v: %v", spName, err)
		return nil
	}
	var schedKeys []string
	for _, key := range keys {
		if len(key.Comp) < 1 {
			continue
		}
		parts := strings.Split(key.Comp[0], "@")
		if len(parts) != 2 || parts[0] != spName {
			continue
		}
		schedKeys = append(schedKeys, key.Comp[0])
	}
	return schedKeys
}

// qosBindEgressSchedPolicy resolves every scheduler of spName to the queue it
// drains on ifName and returns the QUEUE rows carrying the binding.
func qosBindEgressSchedPolicy(inParams XfmrParams, ifName string, spName string) (map[string]db.Value, error) {
	queueTblMap := make(map[string]db.Value)
	schedKeys := qosSchedPolicySchedulers(inParams.d, spName)
	if len(schedKeys) == 0 {
		return nil, tlerr.NotFoundError{Format: "Scheduler policy " + spName + " not found"}
	}

	ts := &db.TableSpec{Name: "SCHEDULER"}
	onQueue := isSchedulerOnQueue(ifName)
	for _, schedKey := range schedKeys {
		/*
		 * The key came from the table a moment ago, so a failure here is not a
		 * missing scheduler. Skipping it would schedule some queues of the
		 * interface and silently leave the rest unscheduled.
		 */
		schedCfg, err := inParams.d.GetEntry(ts, db.Key{Comp: []string{schedKey}})
		if err != nil {
			log.V(tlerr.ErrorSeverity(err)).Infof("qosBindEgressSchedPolicy: unable to read SCHEDULER|%v: %v", schedKey, err)
			return nil, err
		}
		/*
		 * A policing scheduler meters a classifier rather than draining a
		 * queue, so it names no queue and has no egress binding.
		 */
		queueName, ok := schedCfg.Field["queue"]
		if !ok {
			continue
		}
		qid, err := getNativeQueueName(ifName, queueName)
		if err != nil {
			return nil, err
		}
		key := ifName + "|" + qid
		if _, ok := queueTblMap[key]; !ok {
			queueTblMap[key] = db.Value{Field: make(map[string]string)}
		}
		queueTblMap[key].Field["scheduler"] = schedKey
		if onQueue {
			queueTblMap[key].Field[SONIC_SCHEDULER_NODE_KEY] = SCHEDULER_NODE_QUEUE
		}
	}
	if len(queueTblMap) == 0 {
		return nil, tlerr.InvalidArgsError{Format: "Scheduler policy " + spName + " drains no queue"}
	}
	return queueTblMap, nil
}

// qosStaleEgressQueues lists the QUEUE keys of ifName carrying a scheduler that
// the binding of spName will not account for, which is what a previous policy
// left behind. A key in keep is being written by this request, and a queue
// already naming spName is staying where it is. Passing an empty spName treats
// every bound queue as stale, which is what an unbind wants.
func qosStaleEgressQueues(d *db.DB, ifName string, spName string, keep map[string]db.Value) []string {
	if d == nil {
		return nil
	}
	ts := &db.TableSpec{Name: qosQueueTbl}
	keys, err := d.GetKeysByPattern(ts, ifName+"|*")
	if err != nil {
		log.V(lvl.DEBUG).Infof("qosStaleEgressQueues: no queues for %v: %v", ifName, err)
		return nil
	}
	var stale []string
	for _, key := range keys {
		if len(key.Comp) < 2 {
			continue
		}
		qCfg, err := d.GetEntry(ts, key)
		if err != nil {
			continue
		}
		sched, ok := qCfg.Field["scheduler"]
		if !ok {
			continue
		}
		if spName != "" && strings.SplitN(sched, "@", 2)[0] == spName {
			continue
		}
		dbKey := key.Comp[0] + "|" + key.Comp[1]
		if _, ok := keep[dbKey]; ok {
			continue
		}
		stale = append(stale, dbKey)
	}
	return stale
}

// qosStageStaleEgressQueues stages the removal of the bindings a previous policy
// owned on ifName, so a rebind does not leave the interface attached to both.
func qosStageStaleEgressQueues(inParams XfmrParams, ifName string, spName string, keep map[string]db.Value) {
	for _, dbKey := range qosStaleEgressQueues(inParams.d, ifName, spName, keep) {
		qosStageDelete(inParams, qosQueueTbl, dbKey, qosEgressSchedUnbindVal(ifName))
	}
}

// qosEgressSchedUnbindVal is the delete payload for one queue, naming every
// field the bind owns. It mirrors qosBindEgressSchedPolicy so a platform that
// schedules on the queue does not keep the node type after an unbind.
func qosEgressSchedUnbindVal(ifName string) db.Value {
	val := db.Value{Field: map[string]string{"scheduler": ""}}
	if isSchedulerOnQueue(ifName) {
		val.Field[SONIC_SCHEDULER_NODE_KEY] = ""
	}
	return val
}

// qosSchedPolicyInRequest reports whether a scheduler policy is being written by
// the request in flight.
func qosSchedPolicyInRequest(inParams XfmrParams, spName string) bool {
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil || qosObj.SchedulerPolicies == nil {
		return false
	}
	_, ok := qosObj.SchedulerPolicies.SchedulerPolicy[spName]
	return ok
}

// qosRequestPolicyQueues returns the QUEUE keys of ifName that the
// scheduler-policy transformer will write from the policy carried by this
// request, resolving each scheduler input the way that transformer does. It lets
// a rebind tell the rows about to be rewritten from those a previous policy
// owned, so the ones being rewritten are not cleared and rewritten in turn.
func qosRequestPolicyQueues(inParams XfmrParams, ifName string, spName string) map[string]db.Value {
	keep := make(map[string]db.Value)
	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil || qosObj.SchedulerPolicies == nil {
		return keep
	}
	spObj, ok := qosObj.SchedulerPolicies.SchedulerPolicy[spName]
	if !ok || spObj.Schedulers == nil {
		return keep
	}
	for _, sched := range spObj.Schedulers.Scheduler {
		if sched == nil || sched.Inputs == nil {
			continue
		}
		for _, input := range sched.Inputs.Input {
			if input == nil || input.Config == nil || input.Config.Id == nil {
				continue
			}
			qid, err := getNativeQueueName(ifName, *input.Config.Id)
			if err != nil {
				continue
			}
			keep[ifName+"|"+qid] = db.Value{}
		}
	}
	return keep
}

// qosApplyEgressSchedPolicy binds an interface to an egress scheduler policy.
// The scheduler-policy transformer only writes the QUEUE rows for interfaces
// named in the same request, so binding a port to a policy that already exists
// is handled here. Rows the previous policy owned are staged for removal so a
// rebind does not leave the interface attached to both.
func qosApplyEgressSchedPolicy(inParams XfmrParams, ifName string,
	intfObj *ocbinds.OpenconfigQos_Qos_Interfaces_Interface,
	resMap map[string]map[string]db.Value) error {

	if intfObj.Output == nil || intfObj.Output.SchedulerPolicy == nil ||
		intfObj.Output.SchedulerPolicy.Config == nil ||
		intfObj.Output.SchedulerPolicy.Config.Name == nil {
		return nil
	}
	spName := *intfObj.Output.SchedulerPolicy.Config.Name
	if spName == "" {
		return nil
	}

	/*
	 * A policy carried by this same request is bound by the scheduler-policy
	 * transformer, which already knows the queue of every scheduler it writes.
	 * Refusing it here for not being in CONFIG_DB yet would reject a combined
	 * create and bind. The rows a previous policy owned still have to go, and
	 * they are recognisable without resolving the incoming policy at all: they
	 * name a different one.
	 */
	if qosSchedPolicyInRequest(inParams, spName) {
		qosStageStaleEgressQueues(inParams, ifName, spName,
			qosRequestPolicyQueues(inParams, ifName, spName))
		return nil
	}

	queueTblMap, err := qosBindEgressSchedPolicy(inParams, ifName, spName)
	if err != nil {
		return err
	}
	qosStageStaleEgressQueues(inParams, ifName, spName, queueTblMap)
	qosMergeTable(resMap, qosQueueTbl, queueTblMap)
	return nil
}

// qosEgressSchedPolicyDelete unbinds an interface from its egress scheduler
// policy by clearing the scheduler field from every queue that carries one.
func qosEgressSchedPolicyDelete(inParams XfmrParams, ifName string) map[string]db.Value {
	queueTblMap := make(map[string]db.Value)
	for _, dbKey := range qosStaleEgressQueues(inParams.d, ifName, "", nil) {
		queueTblMap[dbKey] = qosEgressSchedUnbindVal(ifName)
	}
	return queueTblMap
}

func getCfgIntfSpeedMbps(inParams XfmrParams, intf string) (string, error) {
	intfsObj := getIntfsRoot(inParams.ygRoot)
	if intfsObj == nil {
		return "", tlerr.InternalError{Format: "configuration for interfaces not available"}
	}

	intfObj, ok := intfsObj.Interface[intf]

	if !ok {
		return "", tlerr.InternalError{Format: "interface unavailable in config"}
	}

	if intfObj == nil || intfObj.Ethernet == nil || intfObj.Ethernet.Config == nil || intfObj.Ethernet.Config.PortSpeed == 0 {
		return "", tlerr.InternalError{Format: "port speed unavailable in config"}
	}
	portSpeed := intfObj.Ethernet.Config.PortSpeed
	val, ok := intfOCToSpeedMap[portSpeed]

	if !ok {
		return "", tlerr.InternalError{Format: "invalid speed in config"}
	}

	return val, nil
}

/* Given a scheduler name, (no sequence), check its MAX cir or pir against the port speed */
func check_port_speed_and_scheduler(inParams XfmrParams, sp_name string, intf string) bool {
	/* CPU port not there in PORT table, Copp scheduler can not be removed */
	if intf == "CPU" {
		return true
	}

	dbSpec := &db.TableSpec{Name: "PORT"}

	speed, err := getCfgIntfSpeedMbps(inParams, intf)
	if err != nil {
		var ok bool
		portCfg, err := inParams.d.GetEntry(dbSpec, db.Key{Comp: []string{intf}})
		if err != nil {
			return false
		}
		if speed, ok = portCfg.Field["speed"]; !ok {
			return false
		}
	}

	speed_Mbps, _ := strconv.ParseUint(speed, 10, 32)
	speed_Bps := speed_Mbps * 1000 * 1000 / 8

	// Scheduler
	dbSpec = &db.TableSpec{Name: "SCHEDULER"}

	keyPattern := sp_name + "*"
	keys, _ := inParams.d.GetKeysByPattern(dbSpec, keyPattern)
	for _, key := range keys {
		if len(key.Comp) < 1 {
			continue
		}
		var spname string

		if strings.Contains(key.Comp[0], "@") {
			s := strings.Split(key.Comp[0], "@")
			spname = s[0]
		} else {
			spname = key.Comp[0]
		}

		if strings.Compare(sp_name, spname) != 0 {
			continue
		}

		schedCfg, _ := inParams.d.GetEntry(dbSpec, key)
		if val, exist := schedCfg.Field["pir"]; exist {
			pir, _ := strconv.ParseUint(val, 10, 64)
			log.V(lvl.DEBUG).Info("pir :", pir, " speed_Bps: ", speed_Bps)
			if pir > speed_Bps {
				return false
			}
		}

		if val, exist := schedCfg.Field["cir"]; exist {
			cir, _ := strconv.ParseUint(val, 10, 64)
			if cir > speed_Bps {
				return false
			}
		}
	}

	return true
}
