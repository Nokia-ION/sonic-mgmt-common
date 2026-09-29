package transformer

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
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
	name := pathInfo.Var("name")

	/*
	 * The RFC parent table check resolves a keyed policy against the table
	 * reported here and fails the request when that key is absent. A policing
	 * policy lives in POLICER rather than SCHEDULER, so report the table it is
	 * actually stored in, else GET and DELETE report it missing.
	 */
	if name != "" {
		if policers := qosPolicerKeysForPolicyInDb(inParams.dbs[db.ConfigDB], name, seq); len(policers) > 0 {
			keys := make(map[string]map[string]string, len(policers))
			for _, policer := range policers {
				keys[policer] = map[string]string{}
			}
			return XfmrSubscOutParams{
				dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {qosPolicerTbl: keys}},
			}, nil
		}
	}

	key := qosSchedSubscKey(inParams.dbs[db.ConfigDB], name, seq, pathInfo.Var("id"))
	log.V(lvl.DEBUG).Infoln("XfmrSubscribe_qos_scheduler_policy_xfmr ", key)
	return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {"SCHEDULER": {key: {}}}}}, nil
}

// qosSchedSubscKey is the SCHEDULER key, or key pattern, backing a scheduler
// path. A sequence with several inputs has a row per input and none of its own.
func qosSchedSubscKey(d *db.DB, name string, seq string, inputId string) string {
	if name == "" {
		name = "*"
	}
	if seq == "" {
		return name + "@*"
	}
	key := name + "@" + seq
	if name == "*" || d == nil {
		return key
	}
	if _, err := d.GetEntry(&db.TableSpec{Name: "SCHEDULER"}, db.Key{Comp: []string{key}}); err == nil {
		return key
	}
	if inputId != "" {
		return key + "@" + inputId
	}
	return key + "@*"
}

var DbToYangPath_qos_scheduler_policy_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	log.V(lvl.DEBUG).Infof("DbToYangPath_qos_scheduler_policy_path_xfmr: yangPath %v tblKeyComp %v", inParams.yangPath, inParams.tblKeyComp)

	if len(inParams.tblKeyComp) != 1 {
		return fmt.Errorf("DbToYangPath_qos_scheduler_policy_path_xfmr: Invalid tblKey %v or tblEntry %v", inParams.tblKeyComp, inParams.tblEntry)
	}
	// The DB key looks like `scheduler_100gb@1` (<scheduler-policy>@<sequence>),
	// or `scheduler_100gb@1@BE1` for one input of a sequence with several.
	spName, seq, inputId, ok := qosParseSchedulerKey(inParams.tblKeyComp[0])
	if !ok {
		return fmt.Errorf("DbToYangPath_qos_scheduler_policy_path_xfmr: Invalid tblKey %v or tblEntry %v", inParams.tblKeyComp, inParams.tblEntry)
	}

	inParams.ygPathKeys["/openconfig-qos:qos/scheduler-policies/scheduler-policy/name"] = spName
	inParams.ygPathKeys["/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/sequence"] = seq
	if inputId != "" && strings.Contains(inParams.ygSchemaPath, "/inputs/input") {
		inParams.ygPathKeys["/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler/inputs/input/id"] = inputId
	}
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

const qosUmfSchedulerTbl = "QOS_UMF_SCHEDULER"

// An input id becomes part of a SCHEDULER key when its sequence has several
// inputs, so it is held to the characters that key allows.
var qosSchedInputIdRe = regexp.MustCompile(`^[-a-zA-Z0-9_]{1,64}$`)

// SCHEDULER fields shared by every input of a sequence.
var qosSchedSeqFields = []string{"type", "meter_type", "cir", "pir", "cbs", "pbs"}

// qosSchedulerKey names the SCHEDULER of one input. SONiC attaches a scheduler
// to a single queue, so a sequence feeding several queues needs one scheduler
// per queue, each carrying that queue's weight.
func qosSchedulerKey(spName string, seq string, inputId string, nInputs int) string {
	if nInputs > 1 {
		return spName + "@" + seq + "@" + inputId
	}
	return spName + "@" + seq
}

// qosParseSchedulerKey splits a SCHEDULER key into its policy, its sequence
// and, for a sequence with several inputs, the input id.
func qosParseSchedulerKey(key string) (spName string, seq string, inputId string, ok bool) {
	parts := strings.Split(key, "@")
	switch len(parts) {
	case 2:
		return parts[0], parts[1], "", true
	case 3:
		return parts[0], parts[1], parts[2], true
	}
	return "", "", "", false
}

// qosSchedRow is one scheduler input as held in CONFIG_DB: the SCHEDULER row
// orchagent programs, which takes only SAI scheduler attributes, and the
// QOS_UMF_SCHEDULER row recording the OpenConfig input it came from. Rows
// written before that split carry the input in SCHEDULER itself.
type qosSchedRow struct {
	key   string
	sched db.Value
	umf   db.Value
}

func (r qosSchedRow) get(field string) string {
	if v, ok := r.umf.Field[field]; ok {
		return v
	}
	if field == "input_type" {
		field = "input-type"
	}
	return r.sched.Field[field]
}

func (r qosSchedRow) inputId() string {
	if id := r.get("id"); id != "" {
		return id
	}
	_, _, id, _ := qosParseSchedulerKey(r.key)
	return id
}

func qosReadSchedRow(d *db.DB, key string) (qosSchedRow, error) {
	row := qosSchedRow{key: key}
	var err error
	if row.sched, err = d.GetEntry(&db.TableSpec{Name: "SCHEDULER"}, db.Key{Comp: []string{key}}); err != nil {
		return row, err
	}
	row.umf, _ = d.GetEntry(&db.TableSpec{Name: qosUmfSchedulerTbl}, db.Key{Comp: []string{key}})
	return row, nil
}

// qosQueueSchedulerRefs indexes the QUEUE rows by the scheduler they name.
func qosQueueSchedulerRefs(d *db.DB) map[string][]string {
	refs := make(map[string][]string)
	ts := &db.TableSpec{Name: qosQueueTbl}
	keys, err := d.GetKeys(ts)
	if err != nil {
		return refs
	}
	for _, key := range keys {
		if len(key.Comp) != 2 {
			continue
		}
		qCfg, err := d.GetEntry(ts, key)
		if err != nil {
			continue
		}
		if sched := qCfg.Field["scheduler"]; sched != "" {
			refs[sched] = append(refs[sched], key.Comp[0]+"|"+key.Comp[1])
		}
	}
	return refs
}

// qosQueueIdInRequest reports whether the request numbers a queue itself.
func qosQueueIdInRequest(qosObj *ocbinds.OpenconfigQos_Qos, name string) bool {
	if qosObj == nil || qosObj.Queues == nil {
		return false
	}
	q, ok := qosObj.Queues.Queue[name]
	return ok && q != nil && q.Config != nil && q.Config.QueueId != nil
}

var qosSchedInputTypeName = map[ocbinds.E_OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType]string{
	ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_QUEUE:       "QUEUE",
	ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_IN_PROFILE:  "IN_PROFILE",
	ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType_OUT_PROFILE: "OUT_PROFILE",
}

// qosSchedSeqValues derives the SCHEDULER fields a sequence sets on each of its
// inputs. prev is any row the sequence already has, which an update is merged
// into.
func qosSchedSeqValues(inParams XfmrParams, spName string,
	schedObj *ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler,
	prev *qosSchedRow) (map[string]string, error) {

	fields := make(map[string]string)
	var prevCir, prevPir uint64
	var prevCirOk, prevPirOk bool
	if prev != nil && inParams.oper != REPLACE {
		if v, err := strconv.ParseUint(prev.sched.Field["cir"], 10, 64); err == nil {
			prevCir, prevCirOk = v, true
		}
		if v, err := strconv.ParseUint(prev.sched.Field["pir"], 10, 64); err == nil {
			prevPir, prevPirOk = v, true
		}
	}

	/*
	 * Every row carries a type: orchagent refuses a SCHEDULER field it does not
	 * know, the NULL placeholder of an empty row included.
	 */
	schedType := ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_UNSET
	strict := false
	if schedObj.Config != nil {
		schedType = schedObj.Config.Type
		strict = schedObj.Config.Priority == ocbinds.OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Config_Priority_STRICT
	}
	switch {
	case strict:
		fields["type"] = "STRICT"
	case inParams.oper == UPDATE && prev != nil && prev.sched.Field["type"] != "":
		/*
		 * An update that leaves priority out keeps the one in force, rather
		 * than turning a strict scheduler into a weighted one.
		 */
	default:
		fields["type"] = "DWRR"
	}

	switch {
	case schedType == ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_TWO_RATE_THREE_COLOR:
		if schedObj.TwoRateThreeColor == nil || schedObj.TwoRateThreeColor.Config == nil {
			break
		}
		t23c := schedObj.TwoRateThreeColor.Config
		if t23c.Bc != nil {
			fields["cbs"] = strconv.FormatUint(uint64(*t23c.Bc), 10)
		}
		if t23c.Be != nil {
			fields["pbs"] = strconv.FormatUint(uint64(*t23c.Be), 10)
		}
		var cir, pir uint64
		if t23c.Cir != nil {
			cir = *t23c.Cir
			if t23c.Pir == nil && prevPirOk && cir/8 > prevPir {
				return nil, errors.New("PIR must be greater than or equal to CIR")
			}
			fields["cir"] = strconv.FormatUint(cir/8, 10)
		}
		if t23c.Pir != nil {
			pir = *t23c.Pir
			if (t23c.Cir == nil && prevCirOk && prevCir > pir/8) || (t23c.Cir != nil && cir > pir) {
				return nil, errors.New("PIR must be greater than or equal to CIR")
			}
			fields["pir"] = strconv.FormatUint(pir/8, 10)
		}
	case schedObj.OneRateTwoColor != nil && schedObj.OneRateTwoColor.Config != nil:
		t12c := schedObj.OneRateTwoColor.Config
		if t12c.Bc != nil {
			fields["cbs"] = strconv.FormatUint(uint64(*t12c.Bc), 10)
		}
		if t12c.Cir != nil {
			fields["cir"] = strconv.FormatUint(*t12c.Cir/8, 10)
		}
	case schedType == ocbinds.OpenconfigQosTypes_QOS_SCHEDULER_TYPE_ONE_RATE_TWO_COLOR:
		return nil, errors.New("Type is oneRateTwoColor but the config subtree is empty")
	}
	log.V(lvl.DEBUG).Infof("qosSchedSeqValues: %v: %v", spName, fields)
	return fields, nil
}

var YangToDb_qos_scheduler_policy_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	resMap := make(map[string]map[string]db.Value)
	if inParams.oper == DELETE {
		/*
		 * A policing policy is realized as a POLICER attached to the backing
		 * classifier's rules rather than a SCHEDULER entry, so the default
		 * no-op below would leave it programmed.
		 */
		delPathInfo := NewPathInfo(inParams.uri)
		if name := delPathInfo.Var("name"); name != "" {
			return qosSchedPolicyPolicerDelete(inParams, name, delPathInfo.Var("sequence"))
		}
		return resMap, nil
	}
	log.V(lvl.DEBUG).Infoln("YangToDb_qos_scheduler_policy_xfmr: ", inParams.ygRoot, inParams.uri)

	pathInfo := NewPathInfo(inParams.uri)
	spKey := pathInfo.Var("name")

	targetUriPath, err := getYangPathFromUri(inParams.uri)
	if err != nil {
		log.V(lvl.DEBUG).Infoln("error parsing targetUriPath", err)
		return resMap, nil
	}
	// Filter out unnecessary calls to the transformer during SET at root.
	if targetUriPath != "/openconfig-qos:qos/scheduler-policies/scheduler-policy" && inParams.requestUri == "/openconfig-qos:qos" {
		return nil, nil
	}

	qosObj := getQosRoot(inParams.ygRoot)
	if qosObj == nil {
		return nil, errors.New("No qos tree populated")
	}

	if qosObj.SchedulerPolicies == nil || qosObj.SchedulerPolicies.SchedulerPolicy == nil || len(qosObj.SchedulerPolicies.SchedulerPolicy) < 1 {
		return nil, errors.New("No scheduler subtree populated")
	}

	var spList []string
	if spKey == "" {
		for sp := range qosObj.SchedulerPolicies.SchedulerPolicy {
			spList = append(spList, sp)
		}
	} else {
		spList = []string{spKey}
	}

	/*
	 * A write below the input list names only some inputs, so it is merged
	 * with the ones already there however it is sent.
	 */
	mergeInputs := inParams.oper != REPLACE || strings.Contains(inParams.requestUri, "/inputs")
	seqInPath := strings.HasPrefix(targetUriPath, "/openconfig-qos:qos/scheduler-policies/scheduler-policy/schedulers/scheduler") && pathInfo.Var("sequence") != ""
	// A replace of /qos drops every interface it leaves out, bindings and all.
	rebindDbQueues := !(inParams.oper == REPLACE && inParams.requestUri == "/openconfig-qos:qos")
	var queueRefs map[string][]string

	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	for _, spName := range spList {
		if strings.Contains(spName, "@") {
			return nil, tlerr.InvalidArgs("Scheduler policy name %q may not contain '@', which separates the parts of a SCHEDULER key", spName)
		}
		spObj, ok := qosObj.SchedulerPolicies.SchedulerPolicy[spName]
		if !ok || spObj.Schedulers == nil || len(spObj.Schedulers.Scheduler) < 1 {
			log.V(lvl.DEBUG).Infoln("YangToDb: no schedulers in policy ", spName)
			continue
		}

		qMapType := FRONT_PANEL
		if strings.HasPrefix(spName, "cpu") {
			qMapType = CPU
		}
		maxSchedNum := getMaxSchedNum(spObj.Schedulers.Scheduler)

		var seqVals []uint32
		if seqInPath {
			seqVal, err := strconv.ParseUint(pathInfo.Var("sequence"), 10, 32)
			if err != nil {
				log.V(lvl.DEBUG).Infof("Failed to convert sequence to int: %v", err)
				return nil, nil
			}
			seqVals = []uint32{uint32(seqVal)}
		} else {
			for seq := range spObj.Schedulers.Scheduler {
				seqVals = append(seqVals, seq)
			}
		}

		dbRows := make(map[string]qosSchedRow)
		for _, key := range qosSchedPolicySchedulers(inParams.d, spName) {
			if row, err := qosReadSchedRow(inParams.d, key); err == nil {
				dbRows[key] = row
			}
		}
		intfs := getIntfsBySchedulerPolicyName(spName, inParams)
		intfInRequest := make(map[string]bool, len(intfs))
		for _, ifName := range intfs {
			intfInRequest[ifName] = true
		}
		newKeys := make(map[string]bool)
		newKeyOfInput := make(map[string]string)
		seqDecided := make(map[string]bool)

		for _, seqVal := range seqVals {
			seq := strconv.Itoa(int(seqVal))
			schedObj, ok := spObj.Schedulers.Scheduler[seqVal]
			if !ok {
				log.V(lvl.DEBUG).Infoln("YangToDb: No Scheduler obj: ", spName, " sequence: ", seq)
				return resMap, fmt.Errorf("YangToDb: No Scheduler obj seq (%v) for sp (%v)", seqVal, spName)
			}

			/*
			 * A scheduler whose input names a policed classifier is an ingress
			 * policer rather than an egress queue scheduler, so it belongs in
			 * POLICER instead of SCHEDULER. See xfmr_qos_policer.go.
			 */
			if clsName := qosSchedulerPolicerClassifier(inParams, schedObj); clsName != "" {
				if err := qosSchedulerToPolicer(inParams, spName, seqVal, clsName, schedObj, resMap); err != nil {
					return resMap, err
				}
				seqDecided[seq] = true
				continue
			}

			seqRows := make(map[string]qosSchedRow)
			var seqPrev *qosSchedRow
			for key, row := range dbRows {
				if _, kseq, _, _ := qosParseSchedulerKey(key); kseq != seq {
					continue
				}
				if id := row.inputId(); id != "" {
					seqRows[id] = row
				}
				if seqPrev == nil || key < seqPrev.key {
					r := row
					seqPrev = &r
				}
			}

			ids := make(map[string]bool)
			if schedObj.Inputs != nil {
				for id := range schedObj.Inputs.Input {
					ids[id] = true
				}
			}
			if mergeInputs {
				for id := range seqRows {
					ids[id] = true
				}
			}
			if len(ids) == 0 {
				log.V(lvl.DEBUG).Infoln("YangToDb: No Input subtree present for Scheduler obj: ", spName, " seqVal ", seq)
				continue
			}
			seqDecided[seq] = true
			sortedIds := make([]string, 0, len(ids))
			for id := range ids {
				if id == "" {
					return nil, errors.New("missing data, no input specified")
				}
				if len(ids) > 1 && !qosSchedInputIdRe.MatchString(id) {
					return nil, tlerr.InvalidArgs("Scheduler policy %v sequence %v: input id %q may only use letters, digits, '-' and '_'",
						spName, seq, id)
				}
				sortedIds = append(sortedIds, id)
			}
			sort.Strings(sortedIds)

			seqFields, err := qosSchedSeqValues(inParams, spName, schedObj, seqPrev)
			if err != nil {
				log.V(lvl.ERROR).Info(err)
				return nil, err
			}

			qMap, ok := ocQueueHwQueueMap[qMapType]
			if !ok {
				return nil, errors.New("Failed to get qos queue map: " + qMapType)
			}

			for _, id := range sortedIds {
				key := qosSchedulerKey(spName, seq, id, len(sortedIds))
				schedVal := db.Value{Field: make(map[string]string)}
				umfVal := db.Value{Field: make(map[string]string)}

				prev, hasPrev := seqRows[id]
				if mergeInputs {
					/*
					 * An input moving to a key of its own when its sequence
					 * gains or loses a sibling keeps what the request leaves out,
					 * and one joining a sequence takes the sequence's settings.
					 */
					src := seqPrev
					if hasPrev {
						src = &prev
						for _, f := range []string{"id", "queue", "input_type", "weight"} {
							if v := prev.get(f); v != "" {
								umfVal.Field[f] = v
							}
						}
					}
					if src != nil {
						for _, f := range qosSchedSeqFields {
							if v, ok := src.sched.Field[f]; ok {
								schedVal.Field[f] = v
							}
						}
					}
				}
				for f, v := range seqFields {
					schedVal.Field[f] = v
				}

				umfVal.Field["id"] = id
				if schedObj.Inputs != nil {
					if in := schedObj.Inputs.Input[id]; in != nil && in.Config != nil {
						if in.Config.Queue != nil {
							umfVal.Field["queue"] = *in.Config.Queue
						}
						if in.Config.Weight != nil {
							umfVal.Field["weight"] = strconv.FormatUint(*in.Config.Weight, 10)
						}
						if name, ok := qosSchedInputTypeName[in.Config.InputType]; ok {
							umfVal.Field["input_type"] = name
						}
					}
				}
				if umfVal.Field["queue"] == "" {
					umfVal.Field["queue"] = id
				}

				/*
				 * SAI takes a weight on a weighted scheduler only, so a strict
				 * one keeps it in QOS_UMF_SCHEDULER alone.
				 */
				strict := schedVal.Field["type"] == "STRICT"
				delete(schedVal.Field, "weight")
				if w, ok := umfVal.Field["weight"]; ok && !strict {
					if wv, err := strconv.ParseUint(w, 10, 64); err != nil || wv > 255 {
						return nil, tlerr.InvalidArgs("Scheduler policy %v sequence %v input %v: weight %v is out of range 0..255",
							spName, seq, id, w)
					}
					schedVal.Field["weight"] = w
				}
				if hasPrev && prev.key == key && inParams.oper != REPLACE {
					/*
					 * An update merges into the row, so what orchagent must not
					 * see has to be removed from it: a weight on a scheduler
					 * turned strict, and the input fields rows kept before
					 * QOS_UMF_SCHEDULER.
					 */
					stale := db.Value{Field: make(map[string]string)}
					for _, f := range []string{"id", "queue", "input-type"} {
						if _, ok := prev.sched.Field[f]; ok {
							stale.Field[f] = ""
						}
					}
					if _, ok := prev.sched.Field["weight"]; ok && strict {
						stale.Field["weight"] = ""
					}
					if len(stale.Field) > 0 {
						qosStageDelete(inParams, "SCHEDULER", key, stale)
					}
				}

				queueName := umfVal.Field["queue"]
				queueId, ok := qMap[queueName]
				if !ok {
					return nil, fmt.Errorf("Mismatch in oc queue (%v) to hw queue mapping (%v) for queue (%v)", queueId, qMap, queueName)
				}
				if inParams.oper == REPLACE && len(sortedIds) == 1 && !qosQueueIdInRequest(qosObj, queueName) {
					hwQueueId := strconv.Itoa(int(maxSchedNum - seqVal))
					if queueId != hwQueueId {
						return nil, fmt.Errorf("Failed consistent config check for oc queue (%v) to hw queue mapping (%v) for queue (%v), map (%v)", queueId, hwQueueId, queueName, qMap)
					}
				}

				qosMergeTable(resMap, "SCHEDULER", map[string]db.Value{key: schedVal})
				qosMergeTable(resMap, qosUmfSchedulerTbl, map[string]db.Value{key: umfVal})
				newKeys[key] = true
				newKeyOfInput[seq+"@"+id] = key

				for _, ifName := range intfs {
					qVal := db.Value{Field: map[string]string{"scheduler": key}}
					if isSchedulerOnQueue(ifName) {
						qVal.Field[SONIC_SCHEDULER_NODE_KEY] = SCHEDULER_NODE_QUEUE
					}
					qosMergeTable(resMap, qosQueueTbl, map[string]db.Value{ifName + "|" + queueId: qVal})
				}
			}
		}

		/*
		 * Rows of the sequences this request decides that it no longer writes
		 * are removed, with the queues naming them. A queue of an interface
		 * the request does not bind is moved to the row its input now has. It
		 * rides with the rows written here, as a sub-operation would reach the
		 * queue before the row it names exists, and a replace of a queue has
		 * to restate the fields the queue already has.
		 */
		allSeqs := inParams.oper == REPLACE && !seqInPath
		for key, row := range dbRows {
			_, kseq, _, _ := qosParseSchedulerKey(key)
			if newKeys[key] || (!allSeqs && !seqDecided[kseq]) {
				continue
			}
			qosStageDelete(inParams, "SCHEDULER", key, db.Value{})
			qosStageDelete(inParams, qosUmfSchedulerTbl, key, db.Value{})
			if queueRefs == nil {
				queueRefs = qosQueueSchedulerRefs(inParams.d)
			}
			newKey := newKeyOfInput[kseq+"@"+row.inputId()]
			for _, qKey := range queueRefs[key] {
				ifName := strings.SplitN(qKey, "|", 2)[0]
				qosStageDelete(inParams, qosQueueTbl, qKey, qosEgressSchedUnbindVal(ifName))
				if newKey == "" || !rebindDbQueues || intfInRequest[ifName] {
					continue
				}
				qVal := db.Value{Field: make(map[string]string)}
				if inParams.oper == REPLACE {
					if cur, err := inParams.d.GetEntry(&db.TableSpec{Name: qosQueueTbl}, db.Key{Comp: strings.SplitN(qKey, "|", 2)}); err == nil {
						for f, v := range cur.Field {
							qVal.Field[f] = v
						}
					}
				}
				qVal.Field["scheduler"] = newKey
				if isSchedulerOnQueue(ifName) {
					qVal.Field[SONIC_SCHEDULER_NODE_KEY] = SCHEDULER_NODE_QUEUE
				}
				qosMergeTable(resMap, qosQueueTbl, map[string]db.Value{qKey: qVal})
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
	inputFilter := pathInfo.Var("id")
	for _, key := range keys {
		log.V(lvl.DEBUG).Infoln("current key: ", key)
		if len(key.Comp) < 1 {
			continue
		}
		var spname, spseq string

		spname = key.Comp[0]
		spseq = "0"
		if strings.Contains(key.Comp[0], "@") {
			var ok bool
			if spname, spseq, _, ok = qosParseSchedulerKey(key.Comp[0]); !ok {
				continue
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

		if !sched_config && !sched_state && !t23c_config && !t23c_state && !t12c_config && !t12c_state && !input_config && !input_state {
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
			continue
		}
		/*
		 * A platform whose qosorch consumes CONFIG_DB directly keeps no APPL_DB
		 * copy of the scheduler, so the configured one is what is in effect.
		 */
		schedState := schedConfig
		if applDb := inParams.dbs[db.ApplDB]; applDb != nil {
			if entry, err := applDb.GetEntry(dbSpec, key); err == nil {
				schedState = entry
			}
		}
		row := qosSchedRow{key: key.Comp[0], sched: schedConfig}
		row.umf, _ = inParams.dbs[db.ConfigDB].GetEntry(&db.TableSpec{Name: qosUmfSchedulerTbl}, key)
		inputId := row.inputId()
		if inputFilter != "" && inputId != inputFilter {
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

			if val := inputId; val != "" && (input_config || input_state) {
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
				queue := row.get("queue")
				var inputType ocbinds.E_OpenconfigQos_Qos_SchedulerPolicies_SchedulerPolicy_Schedulers_Scheduler_Inputs_Input_Config_InputType
				for t, name := range qosSchedInputTypeName {
					if row.get("input_type") == name {
						inputType = t
					}
				}
				/*
				 * The weight lives in QOS_UMF_SCHEDULER, which also holds the one
				 * a strict scheduler carries and SAI does not take.
				 */
				var weight *uint64
				if w, err := strconv.ParseUint(row.get("weight"), 10, 64); err == nil {
					weight = &w
				}
				if input_config {
					intfObj.Config.Id = &val
					intfObj.Config.Queue = &queue
					intfObj.Config.InputType = inputType
					intfObj.Config.Weight = weight
				}
				if input_state {
					stateWeight := weight
					if _, ok := row.umf.Field["weight"]; !ok {
						if w, err := strconv.ParseUint(schedState.Field["weight"], 10, 64); err == nil {
							stateWeight = &w
						}
					}
					intfObj.State.Id = &val
					intfObj.State.Queue = &queue
					intfObj.State.InputType = inputType
					intfObj.State.Weight = stateWeight
				}
			}
		}
	}

	/*
	 * Policing policies are held in POLICER rather than SCHEDULER, so they have
	 * to be gathered separately. Without this a GET reports them as missing,
	 * which also makes translib refuse to delete them.
	 */
	if qosObj.SchedulerPolicies == nil {
		qosObj.SchedulerPolicies = &ocbinds.OpenconfigQos_Qos_SchedulerPolicies{}
	}
	if _, err := qosFillSchedPoliciesFromPolicer(inParams, qosObj.SchedulerPolicies, spName, sp_seq,
		qosPolicerReadFlags{
			policyConfig:  sp_config,
			policyState:   sp_state,
			schedConfig:   sched_config,
			schedState:    sched_state,
			oneRateConfig: t12c_config,
			oneRateState:  t12c_state,
			twoRateConfig: t23c_config,
			twoRateState:  t23c_state,
			inputConfig:   input_config,
			inputState:    input_state,
		}); err != nil {
		return err
	}

	log.V(lvl.DEBUG).Infoln("DbToYang_qos_scheduler_policy_xfmr - Done")
	return nil
}
