package transformer

import (
	"fmt"
	"strconv"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	lvl "github.com/Azure/sonic-mgmt-common/translib/log"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

const (
	CPU_SCHEDULER_NODE             = "cpu_port_scheduler_node"
	FP_PORT_SCHEDULER_NODE         = "front_panel_port_scheduler_node"
	SCHEDULER_NODE_QUEUE           = "queue"
	SCHEDULER_NODE_SCHEDULER_GROUP = "scheduler_group"
	SONIC_SCHEDULER_NODE_KEY       = "scheduler_node"
)

func init() {
	XlateFuncBind("YangToDb_qos_queue_xfmr", YangToDb_qos_queue_xfmr)
	XlateFuncBind("DbToYang_qos_queue_xfmr", DbToYang_qos_queue_xfmr)
	XlateFuncBind("Subscribe_qos_queue_xfmr", Subscribe_qos_queue_xfmr)
	//XlateFuncBind("DbToYang_qos_queue_key_xfmr", DbToYang_qos_queue_key_xfmr)
	XlateFuncBind("DbToYangPath_qos_queue_path_xfmr", DbToYangPath_qos_queue_path_xfmr)
	parseQueueMapJSONFile()
}

func getQosQueuesRoot(s *ygot.GoStruct) *ocbinds.OpenconfigQos_Qos_Queues {
	deviceObj := (*s).(*ocbinds.Device)
	return deviceObj.Qos.Queues
}

var YangToDb_qos_queue_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	// Writing this subtree to redis and the local cache is handled by qos_pre_xfmr/qos_post_xfmr
	resMap := make(map[string]map[string]db.Value)
	return resMap, nil
}

// The caller is required to acquire the lock before calling handleSingleQueuePopulate.
func handleSingleQueuePopulate(qName string, queuesObj *ocbinds.OpenconfigQos_Qos_Queues) error {
	var ok bool
	var queueObj *ocbinds.OpenconfigQos_Qos_Queues_Queue
	if queuesObj == nil {
		return fmt.Errorf("handleSingleQueuePopulate; nil queuesObj passed")
	}
	if queueObj, ok = queuesObj.Queue[qName]; !ok {
		queueObj, _ = queuesObj.NewQueue(qName)
	}
	ygot.BuildEmptyTree(queueObj)

	queueObj.Config.Name = &qName
	queueObj.State.Name = &qName

	qId, err := getNativeQueueNameNoLock("*", qName)
	if err != nil {
		return fmt.Errorf("handleSingleQueuePopulate; getNativeQueueName returned: %w", err)
	}
	iqId, err := strconv.Atoi(qId)
	if err != nil {
		return fmt.Errorf("handleSingleQueuePopulate; failed to convert qId=%s to int: %w", err)
	}
	u8qId := uint8(iqId)

	queueObj.Config.QueueId = &u8qId
	queueObj.State.QueueId = &u8qId
	return nil
}

var DbToYang_qos_queue_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	queuesObj := getQosQueuesRoot(inParams.ygRoot)
	pathInfo := NewPathInfo(inParams.uri)
	qName := pathInfo.Var("name")
	log.V(lvl.DEBUG).Infof("DbToYang_qos_queue_xfmr entered")
	queueMapMutex.RLock()
	defer queueMapMutex.RUnlock()
	if qName == "*" || qName == "" {
		for _, qMap := range ocQueueHwQueueMap {
			for qn, _ := range qMap {
				if err := handleSingleQueuePopulate(qn, queuesObj); err != nil {
					return err
				}
			}
		}
	} else {
		if err := handleSingleQueuePopulate(qName, queuesObj); err != nil {
			return err
		}
	}
	return nil
}

var Subscribe_qos_queue_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	key := NewPathInfo(inParams.uri).Var("name")
	log.V(lvl.DEBUG).Infof("+++ Subscribe_qos_queue_xfmr (%v) +++", inParams.uri)
	if key == "" {
		if inParams.subscProc != TRANSLATE_SUBSCRIBE {
			/* no need to verify dB data if we are requesting ALL queues */
			log.V(lvl.DEBUG).Infof("+++ Subscribe_qos_queue_xfmr end with virtual table")
			return XfmrSubscOutParams{isVirtualTbl: true}, nil
		}
		key = "*"
	}
	log.V(lvl.DEBUG).Infof("+++ Subscribe_qos_queue_xfmr end")
	return XfmrSubscOutParams{dbDataMap: RedisDbSubscribeMap{db.ConfigDB: {"UMF_QUEUE": {key: {}}}}}, nil
}

var DbToYangPath_qos_queue_path_xfmr PathXfmrDbToYangFunc = func(inParams XfmrDbToYgPathParams) error {
	rootPath := "/openconfig-qos:qos/queues/queue"

	log.V(lvl.DEBUG).Info("DbToYangPath_qos_queue_path_xfmr: inParams: ", inParams)

	switch len(inParams.tblKeyComp) {
	case 1:
		inParams.ygPathKeys[rootPath+"/name"] = inParams.tblKeyComp[0]
	default:
		return fmt.Errorf("Invalid tblKeyCom for intf path xmfr:%v", inParams.tblKeyComp)
	}

	log.V(lvl.DEBUG).Info("DbToYangPath_qos_queue_path_xfmr:- params.ygPathKeys: ", inParams.ygPathKeys)

	return nil
}
