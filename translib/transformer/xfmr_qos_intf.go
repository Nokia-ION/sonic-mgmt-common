package transformer

import (
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
)

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
