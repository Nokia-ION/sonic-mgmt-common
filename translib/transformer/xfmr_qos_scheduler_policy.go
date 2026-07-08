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

type SchedStruct struct {
	cbs                uint32
	pbs                uint32
	cir                uint64
	pir                uint64
	weight             uint64
	schedType          ocbinds.E_OpenconfigQosTypes_QOS_SCHEDULER_TYPE
	priority           ocbinds.E_OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority
	inputType          ocbinds.E_OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType
	isMeterTypePackets bool
	queue              string
}

func init() {
	XlateFuncBind("YangToDb_qos_scheduler_policy_xfmr", YangToDb_qos_scheduler_policy_xfmr)
	XlateFuncBind("DbToYang_qos_scheduler_policy_xfmr", DbToYang_qos_scheduler_policy_xfmr)
	XlateFuncBind("Subscribe_qos_scheduler_policy_xfmr", Subscribe_qos_scheduler_policy_xfmr)
	XlateFuncBind("DbToYangPath_qos_scheduler_policy_path_xfmr", DbToYangPath_qos_scheduler_policy_path_xfmr)
	parseQueueMapJSONFile()
}

const (
	SCHEDULER_PORT_SEQUENCE string = "255"
)

func isSchedulerOnQueue(intf string) bool {
	if intf == "CPU" && qMapStr[CPU_SCHEDULER_NODE] != nil {
		return qMapStr[CPU_SCHEDULER_NODE].(string) == SCHEDULER_NODE_QUEUE
	} else if qMapStr[FP_PORT_SCHEDULER_NODE] != nil {
		return qMapStr[FP_PORT_SCHEDULER_NODE].(string) == SCHEDULER_NODE_QUEUE
	}
	return false
}

func Subscribe_qos_scheduler_policy_xfmr(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	pathInfo := NewPathInfo(inParams.uri)
	seq := pathInfo.Var("sequence")
	if seq == "" {
		seq = "*"
	}
	name := pathInfo.Var("name")
	if name == "" {
		name = "*"
	}
	log.V(lvl.DEBUG).Infoln("XfmrSubscribe_qos_scheduler_policy_xfmr")
	return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {"SCHEDULER": {name + "@" + seq: {}}}}}, nil
}

var DbToYangPath_qos_scheduler_policy_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	log.V(lvl.DEBUG).Infof("DbToYangPath_qos_scheduler_policy_path_xfmr: yangPath %v tblKeyComp %v", inParams.yangPath, inParams.tblKeyComp)

	if len(inParams.tblKeyComp) != 1 {
		return fmt.Errorf("DbToYangPath_qos_scheduler_policy_path_xfmr: Invalid tblKey %v or tblEntry %v", inParams.tblKeyComp, inParams.tblEntry)
	}
	// The DB key will look like `scheduler_100gb@1` (<scheduler-policy>@<sequence>).
	keySplit := strings.Split(inParams.tblKeyComp[0], "@")
	if len(keySplit) != 2 {
		return fmt.Errorf("DbToYangPath_qos_scheduler_policy_path_xfmr: Invalid tblKey %v or tblEntry %v", inParams.tblKeyComp, inParams.tblEntry)
	}

	inParams.ygPathKeys["/openconfig-qos:qos/scheduler-policies/scheduler-policy/name"] = keySplit[0]
	inParams.ygPathKeys["/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/sequence"] = keySplit[1]
	return nil
}

func getIntfsBySchedulerPolicyName(spName string, inParams XfmrParams) []string {
	log.V(lvl.DEBUG).Infoln("spName ", spName)

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil || qosObj.Interfaces == nil || qosObj.Interfaces.Interface == nil {
		return []string{}
	}
	var s []string
	for intf := range qosObj.Interfaces.Interface {
		intfObj, ok := qosObj.Interfaces.Interface[intf]
		if !ok {
			log.V(lvl.DEBUG).Infoln("getIntfsBySchedulerPolicyName No interface object: ", spName)
			return []string{}
		}
		if intfObj.Output == nil {
			continue
		}
		if intfObj.Output.SchedulerPolicy == nil {
			continue
		}
		if intfObj.Output.SchedulerPolicy.Config == nil {
			continue
		}
		if intfObj.Output.SchedulerPolicy.Config.Name == nil {
			continue
		}
		if strings.Compare(spName, *intfObj.Output.SchedulerPolicy.Config.Name) != 0 {
			continue
		}
		s = append(s, intf)
	}
	return s
}

var YangToDb_qos_scheduler_policy_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	if inParams.oper == DELETE {
		return resMap, nil
	}
	log.V(lvl.DEBUG).Infoln("YangToDb_qos_scheduler_policy_xfmr: ", inParams.ygRoot, inParams.uri)
	log.V(lvl.DEBUG).Infoln("inParams: ", inParams)

	pathInfo := NewPathInfo(inParams.uri)
	spKey := pathInfo.Var("name")
	log.V(lvl.DEBUG).Infoln("YangToDb: policy name: ", spKey)

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.DEBUG).Infoln("error parsing targetUriPath", err)
		return resMap, nil
	}
	// Filter out unnecessary calls to the transformer during SET at root.
	if targetUriPath != "/openconfig-qos:qos/scheduler-policies/scheduler-policy" && inParams.requestUri == "/openconfig-qos:qos" {
		return nil, nil
	}

	log.V(lvl.DEBUG).Infoln("targetUriPath: ", targetUriPath)

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		return nil, errors.New("No qos tree populated")
	}

	if qosObj.SchedulerPolicies == nil || qosObj.SchedulerPolicies.SchedulerPolicy == nil || len(qosObj.SchedulerPolicies.SchedulerPolicy) < 1 {
		return nil, errors.New("No scheduler subtree populated")
	}

	var spList []string
	if spKey == "" {
		spList = make([]string, len(qosObj.SchedulerPolicies.SchedulerPolicy))
		i := 0
		for sp := range qosObj.SchedulerPolicies.SchedulerPolicy {
			spList[i] = sp
			i++
		}
	} else {
		spList = []string{spKey}
	}

	schedEntry := make(map[string]db.Value)
	/* update "Queue" table for newly created scheduler if the scheduler profile is used by intfs*/
	queueTblMap := make(map[string]db.Value)
	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	for _, spName := range spList {
		spObj, ok := qosObj.SchedulerPolicies.SchedulerPolicy[spName]
		if !ok {
			log.V(lvl.DEBUG).Infoln("YangToDb: No policy name: ", spName)
			return resMap, nil
		}

		if spObj.Schedulers == nil || spObj.Schedulers.Scheduler == nil || len(spObj.Schedulers.Scheduler) < 1 {
			return resMap, nil
		}

		qMapType := FRONT_PANEL
		if strings.HasPrefix(spName, "cpu") {
			qMapType = CPU
		}
		maxSchedNum := getMaxSchedNum(spObj.Schedulers.Scheduler)

		var seqVals []uint32
		if strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler") && pathInfo.Var("sequence") != "" {
			seq := pathInfo.Var("sequence")
			seqVal, err := strconv.ParseUint(seq, 10, 32)
			if err != nil {
				log.V(lvl.DEBUG).Infof("Failed to convert sequence to int: %v", err)
				return nil, nil
			}
			seqVals = []uint32{uint32(seqVal)}
		} else {
			seqVals = make([]uint32, len(spObj.Schedulers.Scheduler))
			i := 0
			for seq := range spObj.Schedulers.Scheduler {
				seqVals[i] = seq
				i++
			}
		}

		for _, seqVal := range seqVals {
			log.V(lvl.DEBUG).Infoln("YangToDb: Scheduler obj: ", spName, " seqVal ", seqVal)
			seq := strconv.Itoa(int(seqVal))

			if seq == "" {
				log.V(lvl.DEBUG).Infoln("YangToDb: no sequence specified")
				return resMap, fmt.Errorf("YangToDb: no sequence specified seq (%v) for sp (%v)", seqVal, spName)
			}

			schedObj, ok := spObj.Schedulers.Scheduler[uint32(seqVal)]
			if !ok {
				log.V(lvl.DEBUG).Infoln("YangToDb: No Scheduler obj: ", spName, " sequence: ", seq, " seqVal ", seqVal)
				return resMap, fmt.Errorf("YangToDb: No Scheduler obj seq (%v) for sp (%v)", seqVal, spName)
			}
			if schedObj.Inputs == nil || schedObj.Inputs.Input == nil || len(schedObj.Inputs.Input) < 1 {
				log.V(lvl.DEBUG).Infoln("YangToDb: No Input subtree present for Scheduler obj: ", spName, " seqVal ", seq)
				continue
			}

			sched_key := spName + "@" + seq
			schedVal := db.Value{Field: make(map[string]string)}

			if (inParams.oper == CREATE) || (inParams.oper == REPLACE) || (inParams.oper == UPDATE) {
				var cir, pir, prev_pir, prev_cir, prev_weight uint64 = 0, 0, 0, 0, 0
				var prev_pir_exist, prev_cir_exist, prev_weight_exist bool = false, false, false
				var val, queue_id string

				log.V(lvl.DEBUG).Infoln("key: ", sched_key)
				ts := &db.TableSpec{Name: "SCHEDULER"}
				prev_entry, entry_err := inParams.d.GetEntry(ts, db.Key{Comp: []string{sched_key}})
				if entry_err == nil {
					log.V(lvl.DEBUG).Infoln("current entry: ", prev_entry)
					if val, prev_cir_exist = prev_entry.Field["cir"]; prev_cir_exist {
						prev_cir, err = strconv.ParseUint(val, 10, 64)
						if err != nil {
							prev_cir_exist = false
							log.V(lvl.DEBUG).Infoln("error parsing previous cir value for scheduler ", spName, " sequence: ", seq)
						}
					}

					if val, prev_pir_exist = prev_entry.Field["pir"]; prev_pir_exist {
						prev_pir, err = strconv.ParseUint(val, 10, 64)
						if err != nil {
							prev_pir_exist = false
							log.V(lvl.DEBUG).Infoln("error parsing previous pir value for scheduler ", spName, " sequence: ", seq)
						}
					}
					if val, prev_weight_exist = prev_entry.Field["weight"]; prev_weight_exist {
						prev_weight, err = strconv.ParseUint(val, 10, 32)
						if err != nil {
							prev_weight_exist = false
							log.V(lvl.DEBUG).Infoln("error parsing previous weight value for scheduler ", spName, " sequence: ", seq)
						}
					}
				}
				for idx := range schedObj.Inputs.Input {
					log.V(lvl.DEBUG).Infoln("YangToDb: Scheduler obj: ", spName, " seqVal ", idx)
					if idx == "" {
						log.V(lvl.DEBUG).Infoln("YangToDb: no input id specified")
						return nil, errors.New("missing data, no input specified")
					}
					idxObj, ok := schedObj.Inputs.Input[idx]
					if !ok {
						log.V(lvl.DEBUG).Infoln("YangToDb: No input obj: ", spName, " sequence: ", seq, " ID: ", idx)
						return nil, errors.New("missing data, no input object present")
					}
					if idxObj.Config != nil {
						if idxObj.Config.Weight != nil {
							if schedObj.Config != nil && schedObj.Config.Priority == ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority_STRICT {
								log.V(lvl.ERROR).Infoln("STRICT priority scheduling cannot be configured with weight.")
								return nil, errors.New("STRICT priority scheduling cannot be configured with weight.")
							}
							schedVal.Field["weight"] = strconv.Itoa((int)(*idxObj.Config.Weight))
							prev_weight = (uint64)(*idxObj.Config.Weight)
						}
						if idxObj.Config.Id == nil {
							// id cannot be empty
							return nil, errors.New("missing data, no input/config/id specified")
						}

						schedVal.Field["id"] = *idxObj.Config.Id

						queueName := *idxObj.Config.Id
						if idxObj.Config.Queue != nil {
							queueName = *idxObj.Config.Queue
						}
						schedVal.Field["queue"] = queueName

						qMap, ok := ocQueueHwQueueMap[qMapType]
						if !ok {
							return nil, errors.New("Failed to get qos queue map: " + qMapType)
						}
						log.V(lvl.DEBUG).Infoln("YangToDb: Scheduler qMap: ", qMap, ", ocQueueHwQueueMap ", ocQueueHwQueueMap, ", qMapType ", qMapType)

						queue_id, ok = qMap[queueName]
						if !ok {
							return nil, fmt.Errorf("Mismatch in oc queue (%v) to hw queue mapping (%v) for queue (%v)", queue_id, qMap, queueName)
						} else if inParams.oper == REPLACE {
							hw_queue_id := strconv.Itoa(int(maxSchedNum - uint32(seqVal)))
							log.V(lvl.DEBUG).Infof("YangToDb: hw queue id = %v, maxSchedNum = %v, seqVal = %v", hw_queue_id, maxSchedNum, seqVal)
							if queue_id != hw_queue_id {
								return nil, fmt.Errorf("Failed consistent config check for oc queue (%v) to hw queue mapping (%v) for queue (%v), map (%v)", queue_id, hw_queue_id, queueName, qMap)
							}
						}

						if idxObj.Config.InputType == ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_QUEUE {
							schedVal.Field["input-type"] = "QUEUE"
						}
					}
					config_type_two_rate := false
					if schedObj.Config != nil {
						schedVal.Field["type"] = "DWRR"
						if schedObj.Config.Priority == ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority_STRICT {
							schedVal.Field["type"] = "STRICT"
							if prev_weight != 0 && inParams.oper == UPDATE {
								log.V(lvl.ERROR).Infoln("Strict priority scheduling can not be configured with weight")
								return nil, errors.New("Strict priority scheduling can not be configured with weight")
							}
						}
						if schedObj.Config.Type == ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_TWO_RATE_THREE_COLOR {
							config_type_two_rate = true
						}
					}
					if config_type_two_rate {
						if schedObj.TwoRateThreeColor != nil && schedObj.TwoRateThreeColor.Config != nil {

							if schedObj.TwoRateThreeColor.Config.Bc != nil {
								cbs := (int)(*schedObj.TwoRateThreeColor.Config.Bc)
								schedVal.Field["cbs"] = strconv.Itoa(cbs)
							}

							if schedObj.TwoRateThreeColor.Config.Be != nil {
								pbs := (int)(*schedObj.TwoRateThreeColor.Config.Be)
								schedVal.Field["pbs"] = strconv.Itoa(pbs)
							}

							if schedObj.TwoRateThreeColor.Config.Cir != nil {
								cir = (uint64)(*schedObj.TwoRateThreeColor.Config.Cir)
								if schedObj.TwoRateThreeColor.Config.Pir == nil {
									if prev_pir_exist && (cir > prev_pir) {
										log.V(lvl.ERROR).Infoln("PIR must be greater than or equal to CIR")
										return nil, errors.New("PIR must be greater than or equal to CIR")
									}
								}
								schedVal.Field["cir"] = strconv.FormatUint(cir/8, 10)
							}
							if schedObj.TwoRateThreeColor.Config.Pir != nil {
								pir = (uint64)(*schedObj.TwoRateThreeColor.Config.Pir)
								if schedObj.TwoRateThreeColor.Config.Cir == nil {
									if prev_cir_exist && (prev_cir > pir) {
										log.V(lvl.ERROR).Infoln("PIR must be greater than or equal to CIR")
										return nil, errors.New("PIR must be greater than or equal to CIR")
									}
								} else {
									if cir > pir {
										log.V(lvl.ERROR).Infoln("PIR must be greater than or equal to CIR")
										return nil, errors.New("PIR must be greater than or equal to CIR")
									}
								}
								schedVal.Field["pir"] = strconv.FormatUint(pir/8, 10)
							}

							intfs := getIntfsBySchedulerPolicyName(spName, inParams)
							if len(intfs) > 0 {
								dbSpec := &db.TableSpec{Name: "PORT"}
								for _, intf := range intfs {
									log.V(lvl.DEBUG).Infoln("intf: ", intf)
									speed, err := getCfgIntfSpeedMbps(inParams, intf)
									if err != nil {
										var ok bool
										portCfg, err := inParams.d.GetEntry(dbSpec, db.Key{Comp: []string{intf}})
										if err != nil {
											continue
										}
										if speed, ok = portCfg.Field["speed"]; !ok {
											continue
										}
									}
									speed_Mbps, err := strconv.ParseUint(speed, 10, 64)
									if err != nil {
										continue
									}
									speed_Bps := speed_Mbps * 1000 * 1000
									if cir > speed_Bps || pir > speed_Bps {
										errmsg := fmt.Sprintf("PIR(%d)/(%d) CIR must be less than or equal to port (%s) speed (%d)", pir, cir, intf, speed_Bps)
										log.V(lvl.DEBUG).Infoln(errmsg)
										/*
										 * TODO (b/194814696): Currently we have an issue where we are unable to
										 * access incoming config for Interfaces to get interface
										 * speed. Do not enforce this check till this capability is
										 * available.
										 */
										//return nil, errors.New(errmsg)
									}
								}
							}
						}
					} else {
						if schedObj.OneRateTwoColor == nil || schedObj.OneRateTwoColor.Config == nil {
							log.V(lvl.ERROR).Infoln("Type is oneRateTwoColor but the config subtree is empty")
							return nil, errors.New("Type is oneRateTwoColor but the config subtree is empty")
						}

						if schedObj.OneRateTwoColor.Config.Bc != nil {
							cbs := (int)(*schedObj.OneRateTwoColor.Config.Bc)
							schedVal.Field["cbs"] = strconv.Itoa(cbs)
						}

						if schedObj.OneRateTwoColor.Config.Cir != nil {
							cir = (uint64)(*schedObj.OneRateTwoColor.Config.Cir)
							schedVal.Field["cir"] = strconv.FormatUint(cir/8, 10)
						}
					}
				}

				schedEntry[sched_key] = schedVal
				log.V(lvl.DEBUG).Infoln("YangToDb_qos_scheduler_policy_xfmr - entry_key : ", sched_key)
				resMap["SCHEDULER"] = schedEntry

				if inParams.oper == CREATE || inParams.oper == REPLACE || inParams.oper == UPDATE {
					intfs := getIntfsBySchedulerPolicyName(spName, inParams)

					for _, if_name := range intfs {
						key := if_name + "|" + queue_id
						db_spName := spName + "@" + seq
						log.V(lvl.DEBUG).Infof("YangToDb_qos_scheduler_policy_xfmr --> key: %v, db_spName: %v", key, db_spName)

						pTbl := &queueTblMap
						if _, ok := (*pTbl)[key]; !ok {
							(*pTbl)[key] = db.Value{Field: make(map[string]string)}
						}
						(*pTbl)[key].Field["scheduler"] = db_spName
						if isSchedulerOnQueue(if_name) {
							(*pTbl)[key].Field[SONIC_SCHEDULER_NODE_KEY] = SCHEDULER_NODE_QUEUE
						}
					}
					resMap["QUEUE"] = queueTblMap
				}
			}
		}
	}
	log.V(lvl.DEBUG).Infof("resMap %v", resMap)
	return resMap, nil
}

func setIfPresentUInt32(field *uint32, value db.Value, name string) {
	if val, exist := value.Field[name]; exist {
		tmp, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			log.V(lvl.ERROR).Infof("Unable to parse the %v value", name)
		} else {
			*field = uint32(tmp)
		}
	}
}

func getSchedulerAttrFromDb(schedCfg db.Value, schedState db.Value) (SchedStruct, SchedStruct, error) {
	var cfg, state SchedStruct
	setIfPresentUInt32(&cfg.cbs, schedCfg, "cbs")
	setIfPresentUInt32(&state.cbs, schedState, "cbs")
	setIfPresentUInt32(&cfg.pbs, schedCfg, "pbs")
	setIfPresentUInt32(&state.pbs, schedState, "pbs")

	if val, exist := schedCfg.Field["cir"]; exist {
		cir, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			log.V(lvl.ERROR).Infoln("Unable to parse the cir value")
		} else {
			cfg.cir = uint64(cir)
			cfg.schedType = ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_ONE_RATE_TWO_COLOR
		}
	}
	if val, exist := schedState.Field["cir"]; exist {
		cir, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			log.V(lvl.ERROR).Infoln("Unable to parse the cir value")
		} else {
			state.cir = uint64(cir)
			state.schedType = ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_ONE_RATE_TWO_COLOR
		}
	}
	if val, exist := schedCfg.Field["pir"]; exist {
		pir, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			log.V(lvl.ERROR).Infoln("Unable to parse the pir value")
		} else {
			cfg.pir = uint64(pir)
			cfg.schedType = ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_TWO_RATE_THREE_COLOR
		}
	}
	if val, exist := schedState.Field["pir"]; exist {
		pir, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			log.V(lvl.ERROR).Infoln("Unable to parse the pir value")
		} else {
			state.pir = uint64(pir)
			state.schedType = ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_TWO_RATE_THREE_COLOR
		}
	}
	if val, exist := schedCfg.Field["type"]; exist {
		if val == "STRICT" {
			cfg.priority = ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority_STRICT
		}
	}
	if val, exist := schedState.Field["type"]; exist {
		if val == "STRICT" {
			state.priority = ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority_STRICT
		}
	}
	if val, exist := schedCfg.Field["queue"]; exist {
		cfg.queue = val
		state.queue = val
	}
	if val, exist := schedCfg.Field["input-type"]; exist {
		if val == "QUEUE" {
			cfg.inputType = ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_QUEUE
			state.inputType = ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_QUEUE
		}
	}
	if val, exist := schedCfg.Field["weight"]; exist {
		tmp, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			log.V(lvl.ERROR).Infoln("Unable to parse the weight value")
		} else {
			cfg.weight = tmp
		}
	}
	if val, exist := schedState.Field["weight"]; exist {
		tmp, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			log.V(lvl.ERROR).Infoln("Unable to parse the weight value")
		} else {
			state.weight = tmp
		}
	}

	cfg.isMeterTypePackets = false
	if val, exist := schedCfg.Field["meter_type"]; exist {
		if val == "packets" {
			cfg.isMeterTypePackets = true
		}
	}

	state.isMeterTypePackets = false
	if val, exist := schedState.Field["meter_type"]; exist {
		if val == "packets" {
			state.isMeterTypePackets = true
		}
	}
	return cfg, state, nil
}

var DbToYang_qos_scheduler_policy_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	log.V(lvl.DEBUG).Infoln("DbToYang_qos_scheduler_policy_xfmr - inParams.uri: ", inParams.uri)
	pathInfo := NewPathInfo(inParams.uri)
	spName := pathInfo.Var("name")
	sp_seq := pathInfo.Var("sequence")

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.ERROR).Infoln("error parsing targetUriPath")
		return err
	}
	log.V(lvl.DEBUG).Infoln("targetUriPath: ", targetUriPath)

	qosObj := getQosRoot(inParams.ygRoot)

	if qosObj == nil {
		ygot.BuildEmptyTree(qosObj)
	}
	var sp_config, sp_state, sched_config, sched_state, t23c_config, t23c_state, t12c_config, t12c_state, input_config, input_state bool
	switch {
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies") == 0 || strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy") == 0:
		sp_config, sp_state, sched_config, sched_state, t23c_config, t23c_state, t12c_config, t12c_state, input_config, input_state = true, true, true, true, true, true, true, true, true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/config"):
		sp_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/state"):
		sp_state = true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers") == 0:
		sched_config, sched_state, t23c_config, t23c_state, input_config, input_state = true, true, true, true, true, true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler") == 0:
		sched_config, sched_state, t23c_config, t23c_state, input_config, input_state = true, true, true, true, true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/config"):
		sched_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/state"):
		sched_state = true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/inputs") == 0:
		input_config, input_state = true, true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/inputs/input") == 0:
		input_config, input_state = true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/inputs/input/config"):
		input_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/inputs/input/state"):
		input_state = true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/two-rate-three-color") == 0:
		t23c_config, t23c_state = true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/two-rate-three-color/config"):
		t23c_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/two-rate-three-color/state"):
		t23c_state = true
	case strings.Compare(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/one-rate-two-color") == 0:
		t12c_config, t12c_state = true, true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/one-rate-two-color/config"):
		t12c_config = true
	case strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/one-rate-two-color/state"):
		t12c_state = true
	default:
		errStr := "Invalid URI"
		log.V(lvl.ERROR).Info(errStr)
		return tlerr.InvalidArgsError{Format: errStr}
	}

	// Scheduler
	var keyPattern string
	dbSpec := &db.TableSpec{Name: "SCHEDULER"}
	keyPattern = "*"
	if spName != "" {
		keyPattern = spName + "@*"
	}

	keys, err := inParams.dbs[db.ConfigDB].GetKeysByPattern(dbSpec, keyPattern)
	if err != nil {
		log.V(lvl.ERROR).Infoln("Unable to get the data from the CONFIG DB")
		return err
	}
	for _, key := range keys {
		log.V(lvl.DEBUG).Infoln("current key: ", key)
		if len(key.Comp) < 1 {
			continue
		}
		var spname, spseq string

		spname = key.Comp[0]
		spseq = "0"
		if strings.Contains(key.Comp[0], "@") {
			if s := strings.Split(key.Comp[0], "@"); len(s) > 1 {
				spname = s[0]
				spseq = s[1]
			}
		}

		if spName != "" && strings.Compare(spName, spname) != 0 {
			continue
		}

		if sp_seq != "" && strings.Compare(sp_seq, spseq) != 0 {
			continue
		}

		tmp, err := strconv.ParseUint(spseq, 10, 32)
		if err != nil {
			log.V(lvl.DEBUG).Infoln("Error parsing scheduler seq number for scheduler: spname %v", spname)
			continue
		}
		seq := (uint32)(tmp)

		log.V(lvl.DEBUG).Infoln("Fill scheduler policy in scheduler: spname %v spseq %v", spname, spseq)
		spObj, ok := qosObj.SchedulerPolicies.SchedulerPolicy[spname]
		if !ok || qosObj.SchedulerPolicies.SchedulerPolicy == nil {
			spObj, err = qosObj.SchedulerPolicies.NewSchedulerPolicy(spname)
			if err != nil {
				log.V(lvl.DEBUG).Infoln("Unable to create qos scheduler policy object")
				continue
			}
		}
		ygot.BuildEmptyTree(spObj)
		if spObj.Schedulers == nil {
			ygot.BuildEmptyTree(spObj.Schedulers)
		}

		if sp_config {
			spObj.Name = &spname
			spObj.Config.Name = &spname
		}
		if sp_state {
			spObj.Name = &spname
			spObj.State.Name = &spname
		}

		if !sched_config && !sched_state && !t23c_config && !t23c_state && !input_config && !input_config {
			continue
		}

		schedObj, ok := spObj.Schedulers.Scheduler[seq]
		if !ok {
			schedObj, err = spObj.Schedulers.NewScheduler(seq)
			if err != nil {
				log.V(lvl.DEBUG).Infoln("Unable to create qos scheduler policy object")
				continue
			}
			ygot.BuildEmptyTree(schedObj)
		} else if schedObj != nil {
			ygot.BuildEmptyTree(schedObj)
		}
		if schedObj.TwoRateThreeColor != nil {
			ygot.BuildEmptyTree(schedObj.TwoRateThreeColor)
		}

		if schedObj.OneRateTwoColor != nil {
			ygot.BuildEmptyTree(schedObj.OneRateTwoColor)
		}

		schedConfig, errCfg := inParams.dbs[db.ConfigDB].GetEntry(dbSpec, key)
		if errCfg != nil {
			log.V(lvl.DEBUG).Infoln("Unable to get the data from the CONFIG DB")
		}
		schedState, errState := inParams.dbs[db.ApplDB].GetEntry(dbSpec, key)
		if errState != nil {
			log.V(lvl.DEBUG).Infoln("Unable to get the data from the APPL STATE DB")
		}
		if errCfg != nil && errState != nil {
			continue
		}
		schedCfg, schedInfo, err := getSchedulerAttrFromDb(schedConfig, schedState)
		if err == nil {
			schedObj.Sequence = &seq
			if sched_config {
				schedObj.Config.Sequence = &seq
				schedObj.Config.Type = schedCfg.schedType
				schedObj.Config.Priority = schedCfg.priority
			}
			if sched_state {
				schedObj.State.Sequence = &seq
				schedObj.State.Type = schedInfo.schedType
				schedObj.State.Priority = schedInfo.priority
			}
			if schedCfg.schedType == ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_TWO_RATE_THREE_COLOR && t23c_config {
				schedObj.TwoRateThreeColor.Config.Bc = &schedCfg.cbs
				schedObj.TwoRateThreeColor.Config.Be = &schedCfg.pbs
				schedCfg.cir *= 8
				schedCfg.pir *= 8
				schedObj.TwoRateThreeColor.Config.Cir = &schedCfg.cir
				schedObj.TwoRateThreeColor.Config.Pir = &schedCfg.pir
			}
			if schedInfo.schedType == ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_TWO_RATE_THREE_COLOR && t23c_state {
				schedObj.TwoRateThreeColor.State.Bc = &schedInfo.cbs
				schedObj.TwoRateThreeColor.State.Be = &schedInfo.pbs
				schedInfo.cir *= 8
				schedInfo.pir *= 8
				schedObj.TwoRateThreeColor.State.Cir = &schedInfo.cir
				schedObj.TwoRateThreeColor.State.Pir = &schedInfo.pir
			}

			if schedCfg.schedType == ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_ONE_RATE_TWO_COLOR && t12c_config {
				schedObj.OneRateTwoColor.Config.Bc = &schedCfg.cbs
				schedCfg.cir *= 8
				schedObj.OneRateTwoColor.Config.Cir = &schedCfg.cir
			}
			if schedInfo.schedType == ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_ONE_RATE_TWO_COLOR && t12c_state {
				schedObj.OneRateTwoColor.State.Bc = &schedInfo.cbs
				schedInfo.cir *= 8
				schedObj.OneRateTwoColor.State.Cir = &schedInfo.cir
			}

			if val, exist := schedConfig.Field["id"]; exist {
				intfObj, ok := schedObj.Inputs.Input[val]
				if !ok {
					intfObj, err = schedObj.Inputs.NewInput(val)
					if err != nil {
						log.V(lvl.DEBUG).Infoln("Unable to create qos scheduler input object")
						continue
					}
					ygot.BuildEmptyTree(intfObj)
				} else if intfObj != nil {
					ygot.BuildEmptyTree(intfObj)
				}
				intfObj.Id = &val
				if input_config {
					intfObj.Config.Id = &val
					intfObj.Config.Queue = &schedCfg.queue
					intfObj.Config.InputType = schedCfg.inputType
					if sched_config && schedCfg.priority != ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority_STRICT {
						intfObj.Config.Weight = &schedCfg.weight
					}
				}
				if input_state {
					intfObj.State.Id = &val
					intfObj.State.Queue = &schedInfo.queue
					intfObj.State.InputType = schedInfo.inputType
					if sched_state && schedInfo.priority != ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority_STRICT {
						intfObj.State.Weight = &schedInfo.weight
					}
				}

			}
		}
	}
	log.V(lvl.DEBUG).Infoln("DbToYang_qos_scheduler_policy_xfmr - Done")
	return nil
}
