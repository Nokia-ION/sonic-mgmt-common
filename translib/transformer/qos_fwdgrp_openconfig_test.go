////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                     //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//     http://www.apache.org/licenses/LICENSE-2.0                             //
//                                                                            //
//  Unless required by applicable law or agreed to in writing, software        //
//  distributed under the License is distributed on an "AS IS" BASIS,         //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or           //
//  implied.                                                                  //
//  See the License for the specific language governing permissions and       //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

//go:build testapp
// +build testapp

package transformer_test

// Forwarding classes cover the OpenConfig paths:
//
//	/qos/forwarding-groups/forwarding-group[name]/config/fabric-priority
//	/qos/forwarding-groups/forwarding-group[name]/config/output-queue
//	/qos/classifiers/classifier[name]/terms/term[id]/actions/config/target-group
//
// A forwarding group is a named traffic class: fabric-priority is the class
// number and output-queue the queue that class drains to, which together become
// a QOS_FWD_GROUP entry and a TC_TO_QUEUE_MAP entry. Pointing a classifier term
// at one through target-group is what assigns traffic to the class, so the pair
// is the model's equivalent of naming a forwarding class and setting a traffic
// class on matched traffic.

import (
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const (
	qosFgUrl = "/openconfig-qos:qos/forwarding-groups"

	// The queue names come from the scheduler policy below, which is what
	// populates the OpenConfig queue name to queue id map.
	qosFgQueue   = "NC1"
	qosFgQueueId = "1"
	qosFgPrio    = "4"

	qosFgName  = "FG_UT_NC1"
	qosFgName2 = "FG_UT_BE1"
	qosFgCls   = "FG_UT_CLS"
)

func qosFgCleanupTbl() map[string]interface{} {
	return map[string]interface{}{
		"QOS_FWD_GROUP": map[string]interface{}{
			qosFgName:  "",
			qosFgName2: "",
		},
		"TC_TO_QUEUE_MAP":         map[string]interface{}{"global_tc_to_queue_map": ""},
		"DSCP_TO_TC_MAP":          map[string]interface{}{qosFgCls: ""},
		"QOS_UMF_CLASSIFIER":      map[string]interface{}{qosFgCls: ""},
		"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{qosFgCls + "|1": ""},
		"SCHEDULER": map[string]interface{}{
			qosSchedSp + "@0": "",
			qosSchedSp + "@1": "",
		},
	}
}

// qosFgSetup clears leftovers and installs the scheduler policy that teaches the
// queue name to id map about NC1, without which an output-queue cannot resolve.
func qosFgSetup(t *testing.T) func() {
	cleanuptbl := qosFgCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	qosSchedCreatePolicy(t, qosSchedSp)
	return func() { unloadDB(db.ConfigDB, cleanuptbl) }
}

// Test_qos_fwdgrp_create covers naming a forwarding class and giving it a
// traffic class number and a queue.
func Test_qos_fwdgrp_create(t *testing.T) {
	defer qosFgSetup(t)()

	t.Log("\n\n+++++++++++++ A forwarding group is a named traffic class ++++++++++++")
	url_body_json := `{"openconfig-qos:forwarding-group":[{
		"name":"` + qosFgName + `",
		"config":{"name":"` + qosFgName + `","fabric-priority":` + qosFgPrio + `,
			"output-queue":"` + qosFgQueue + `"}
	}]}`
	t.Run("Create forwarding group",
		processSetRequest(qosFgUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"QOS_FWD_GROUP": map[string]interface{}{
		qosFgName: map[string]interface{}{
			"priority": qosFgPrio,
			"queue":    qosFgQueue,
		}}}
	t.Run("Verify QOS_FWD_GROUP",
		verifyDbResult(rclient, "QOS_FWD_GROUP|"+qosFgName, expected_map, false))

	// The traffic class drains to the hardware queue the name resolves to.
	expected_map = map[string]interface{}{"TC_TO_QUEUE_MAP": map[string]interface{}{
		"global_tc_to_queue_map": map[string]interface{}{qosFgPrio: qosFgQueueId}}}
	t.Run("Verify TC_TO_QUEUE_MAP",
		verifyDbResult(rclient, "TC_TO_QUEUE_MAP|global_tc_to_queue_map", expected_map, false))

	t.Log("\n\n+++++++++++++ Read it back ++++++++++++")
	expected_get_json := `{"openconfig-qos:config":{"name":"` + qosFgName +
		`","fabric-priority":` + qosFgPrio + `,"output-queue":"` + qosFgQueue + `"}}`
	t.Run("Get forwarding group",
		processGetRequest(qosFgUrl+"/forwarding-group["+"name="+qosFgName+"]/config",
			nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_fwdgrp_no_fabric_priority covers creating a forwarding group without
// a traffic class number. fabric-priority is an optional leaf, so a client can
// send only a queue, and since the class is the key of every queue map entry the
// group would write, it has to be refused rather than dereferenced.
func Test_qos_fwdgrp_no_fabric_priority(t *testing.T) {
	defer qosFgSetup(t)()

	t.Log("\n\n+++++++++++++ A new group with no fabric-priority is refused ++++++++++++")
	url_body_json := `{"openconfig-qos:forwarding-group":[{
		"name":"` + qosFgName + `",
		"config":{"name":"` + qosFgName + `","output-queue":"` + qosFgQueue + `"}
	}]}`
	expErr := tlerr.InvalidArgsError{
		Format: "fabric-priority is required for forwarding group " + qosFgName}
	t.Run("Create forwarding group without priority",
		processSetRequest(qosFgUrl, url_body_json, "PATCH", true, expErr))
	time.Sleep(1 * time.Second)

	t.Run("Verify nothing written",
		verifyDbResult(rclient, "QOS_FWD_GROUP|"+qosFgName, map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_fwdgrp_update_queue_only covers changing the queue of an existing
// forwarding class without restating its class number, which a client updating
// one leaf will do. The recorded class has to stand rather than the update being
// refused or the queue map losing its key.
func Test_qos_fwdgrp_update_queue_only(t *testing.T) {
	defer qosFgSetup(t)()

	url_body_json := `{"openconfig-qos:forwarding-group":[{
		"name":"` + qosFgName + `",
		"config":{"name":"` + qosFgName + `","fabric-priority":` + qosFgPrio + `,
			"output-queue":"` + qosFgQueue + `"}
	}]}`
	t.Run("Create forwarding group",
		processSetRequest(qosFgUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Change only the queue ++++++++++++")
	url_body_json = `{"openconfig-qos:config":{"output-queue":"` + qosSchedQueueLo + `"}}`
	t.Run("Update queue only",
		processSetRequest(qosFgUrl+"/forwarding-group["+"name="+qosFgName+"]",
			url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"QOS_FWD_GROUP": map[string]interface{}{
		qosFgName: map[string]interface{}{
			"priority": qosFgPrio,
			"queue":    qosSchedQueueLo,
		}}}
	t.Run("Verify class kept and queue changed",
		verifyDbResult(rclient, "QOS_FWD_GROUP|"+qosFgName, expected_map, false))

	// The queue map still has to be keyed by the class the group kept.
	expected_map = map[string]interface{}{"TC_TO_QUEUE_MAP": map[string]interface{}{
		"global_tc_to_queue_map": map[string]interface{}{qosFgPrio: qosSchedQueueLoId}}}
	t.Run("Verify queue map re-keyed",
		verifyDbResult(rclient, "TC_TO_QUEUE_MAP|global_tc_to_queue_map", expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_fwdgrp_traffic_class_action covers assigning matched traffic to a
// forwarding class, which is what a policy-map setting a traffic class does. The
// classifier stores the class number rather than the group name, so the group
// has to exist for the term to resolve.
func Test_qos_fwdgrp_traffic_class_action(t *testing.T) {
	defer qosFgSetup(t)()

	url_body_json := `{"openconfig-qos:forwarding-group":[{
		"name":"` + qosFgName + `",
		"config":{"name":"` + qosFgName + `","fabric-priority":` + qosFgPrio + `,
			"output-queue":"` + qosFgQueue + `"}
	}]}`
	t.Run("Create forwarding group",
		processSetRequest(qosFgUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Point a classifier at the forwarding class ++++++++++++")
	url_body_json = `{"openconfig-qos:classifier":[{
		"name":"` + qosFgCls + `",
		"config":{"name":"` + qosFgCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"dscp":32}}},
			"actions":{"config":{"target-group":"` + qosFgName + `"}}
		}]}
	}]}`
	t.Run("Set classifier",
		processSetRequest("/openconfig-qos:qos/classifiers", url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// The map carries the traffic class of the group, not its name.
	expected_map := map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosFgCls: map[string]interface{}{"32": qosFgPrio}}}
	t.Run("Verify DSCP maps to the class",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosFgCls, expected_map, false))

	t.Log("\n\n+++++++++++++ Read the action back ++++++++++++")
	expected_get_json := `{"openconfig-qos:config":{"target-group":"` + qosFgName + `"}}`
	t.Run("Get classifier action",
		processGetRequest("/openconfig-qos:qos/classifiers/classifier["+"name="+qosFgCls+
			"]/terms/term[id=1]/actions/config", nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_fwdgrp_multiple covers writing several forwarding classes at once,
// which is how the full class list arrives.
func Test_qos_fwdgrp_multiple(t *testing.T) {
	defer qosFgSetup(t)()

	t.Log("\n\n+++++++++++++ Two classes in one request ++++++++++++")
	url_body_json := `{"openconfig-qos:forwarding-group":[
	{
		"name":"` + qosFgName + `",
		"config":{"name":"` + qosFgName + `","fabric-priority":` + qosFgPrio + `,
			"output-queue":"` + qosFgQueue + `"}
	},
	{
		"name":"` + qosFgName2 + `",
		"config":{"name":"` + qosFgName2 + `","fabric-priority":0,
			"output-queue":"` + qosSchedQueueLo + `"}
	}]}`
	t.Run("Create both groups",
		processSetRequest(qosFgUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"QOS_FWD_GROUP": map[string]interface{}{
		qosFgName: map[string]interface{}{"priority": qosFgPrio, "queue": qosFgQueue}}}
	t.Run("Verify first group",
		verifyDbResult(rclient, "QOS_FWD_GROUP|"+qosFgName, expected_map, false))

	expected_map = map[string]interface{}{"QOS_FWD_GROUP": map[string]interface{}{
		qosFgName2: map[string]interface{}{"priority": "0", "queue": qosSchedQueueLo}}}
	t.Run("Verify second group",
		verifyDbResult(rclient, "QOS_FWD_GROUP|"+qosFgName2, expected_map, false))

	// Both classes have to reach the queue map, not just the last one written.
	expected_map = map[string]interface{}{"TC_TO_QUEUE_MAP": map[string]interface{}{
		"global_tc_to_queue_map": map[string]interface{}{
			qosFgPrio: qosFgQueueId,
			"0":       qosSchedQueueLoId,
		}}}
	t.Run("Verify both classes in the queue map",
		verifyDbResult(rclient, "TC_TO_QUEUE_MAP|global_tc_to_queue_map", expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_fwdgrp_delete covers removing a forwarding class.
func Test_qos_fwdgrp_delete(t *testing.T) {
	defer qosFgSetup(t)()

	url_body_json := `{"openconfig-qos:forwarding-group":[{
		"name":"` + qosFgName + `",
		"config":{"name":"` + qosFgName + `","fabric-priority":` + qosFgPrio + `,
			"output-queue":"` + qosFgQueue + `"}
	}]}`
	t.Run("Create forwarding group",
		processSetRequest(qosFgUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Delete the class ++++++++++++")
	t.Run("Delete forwarding group",
		processDeleteRequest(qosFgUrl+"/forwarding-group["+"name="+qosFgName+"]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify group gone",
		verifyDbResult(rclient, "QOS_FWD_GROUP|"+qosFgName, map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}
