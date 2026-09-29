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

// Egress queue scheduling covers the OpenConfig paths:
//
//	/qos/scheduler-policies/scheduler-policy[name]/schedulers/scheduler[seq]/config/priority
//	/qos/scheduler-policies/scheduler-policy[name]/schedulers/scheduler[seq]/inputs/input[id]/config/weight
//	/qos/interfaces/interface[interface-id]/output/scheduler-policy/config/name
//
// A scheduler becomes a SCHEDULER entry keyed <policy>@<sequence>, where STRICT
// priority selects type STRICT and anything else DWRR. A sequence with several
// inputs becomes one entry per input, keyed <policy>@<sequence>@<input>, since
// SONiC attaches a scheduler to a single queue. SCHEDULER holds only what
// orchagent programs; the input itself, and the weight of a strict scheduler,
// are kept in QOS_UMF_SCHEDULER under the same key. Binding an interface is
// the scheduler name landing on that interface's QUEUE rows, which is what
// orchagent turns into a SAI scheduler on the queue.
//
// Without queue-ids in the request, a scheduler's queue index is derived from
// its sequence, counting down from the highest sequence in the policy, so in a
// two scheduler policy sequence 0 drains queue 1 and sequence 1 drains queue 0.

import (
	"strconv"
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const (
	qosSchedSp  = "SP_UT_SCHED"
	qosSchedSp2 = "SP_UT_SCHED2"

	// The strict priority scheduler is at sequence 0 and so drains the higher
	// queue; the weighted one is at sequence 1 and drains queue 0.
	qosSchedQueueHi   = "NC1"
	qosSchedQueueHiId = "1"
	qosSchedQueueLo   = "BE1"
	qosSchedQueueLoId = "0"

	qosSchedIntf  = "Ethernet0"
	qosSchedIntf2 = "Ethernet8"

	qosSchedPolicyUrl = "/openconfig-qos:qos/scheduler-policies"
)

// qosSchedEgressBindUrl is the egress scheduler-policy binding of one interface.
func qosSchedEgressBindUrl(ifName string) string {
	return "/openconfig-qos:qos/interfaces/interface[interface-id=" + ifName +
		"]/output/scheduler-policy"
}

// qosSchedPolicyListJson is a two scheduler egress policy: strict priority on
// the high queue and a weighted share on queue 0, each with a one-rate-two-color
// shaper.
func qosSchedPolicyListJson(name string) string {
	return `[{
		"name":"` + name + `",
		"config":{"name":"` + name + `"},
		"schedulers":{"scheduler":[
		{
			"sequence":0,
			"config":{"sequence":0,"priority":"STRICT","type":"ONE_RATE_TWO_COLOR"},
			"inputs":{"input":[{"id":"` + qosSchedQueueHi + `",
				"config":{"id":"` + qosSchedQueueHi + `","input-type":"QUEUE"}}]},
			"one-rate-two-color":{"config":{"cir":"800000000"}}
		},
		{
			"sequence":1,
			"config":{"sequence":1,"type":"ONE_RATE_TWO_COLOR"},
			"inputs":{"input":[{"id":"` + qosSchedQueueLo + `",
				"config":{"id":"` + qosSchedQueueLo + `","input-type":"QUEUE","weight":"10"}}]},
			"one-rate-two-color":{"config":{"cir":"400000000"}}
		}]}
	}]`
}

// qosSchedPolicyJson wraps the policy list as the body of a write to the
// scheduler-policies container.
func qosSchedPolicyJson(name string) string {
	return `{"openconfig-qos:scheduler-policy":` + qosSchedPolicyListJson(name) + `}`
}

// qosSchedCleanupTbl lists every table an egress scheduler policy and its
// interface bindings can touch.
func qosSchedCleanupTbl() map[string]interface{} {
	schedKeys := map[string]interface{}{}
	for _, sp := range []string{qosSchedSp, qosSchedSp2} {
		for _, suffix := range []string{"@0", "@1", "@0@AF4", "@0@NC1", "@1@AF1", "@1@BE0", "@1@BE1"} {
			schedKeys[sp+suffix] = ""
		}
	}
	umfKeys := map[string]interface{}{}
	for k := range schedKeys {
		umfKeys[k] = ""
	}
	queueKeys := map[string]interface{}{}
	for _, intf := range []string{qosSchedIntf, qosSchedIntf2} {
		for qid := 0; qid <= 6; qid++ {
			queueKeys[intf+"|"+strconv.Itoa(qid)] = ""
		}
	}
	return map[string]interface{}{
		"SCHEDULER":         schedKeys,
		"QOS_UMF_SCHEDULER": umfKeys,
		"QUEUE":             queueKeys,
	}
}

// qosSchedUnboundQueue is the expected state of a queue whose scheduler was
// cleared. Removing the last real field of a row leaves the NULL placeholder
// behind rather than dropping the row, so the assertion is that the scheduler
// field is gone rather than that the row is.
func qosSchedUnboundQueue() map[string]interface{} {
	return map[string]interface{}{"QUEUE": map[string]interface{}{
		"queue": map[string]interface{}{"NULL": "NULL"},
	}}
}

// qosSchedStrictRow is the SCHEDULER entry of the strict scheduler of
// qosSchedPolicyListJson. It carries SAI attributes only.
func qosSchedStrictRow(name string) map[string]interface{} {
	return map[string]interface{}{"SCHEDULER": map[string]interface{}{
		name + "@0": map[string]interface{}{
			"type": "STRICT",
			"cir":  "100000000",
		}}}
}

// qosSchedSetup clears leftovers and returns the teardown.
func qosSchedSetup() func() {
	cleanuptbl := qosSchedCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	return func() { unloadDB(db.ConfigDB, cleanuptbl) }
}

// qosSchedCreatePolicy installs a policy with REPLACE. A replace is what lets
// the queue name to index map be derived from the request, since the platform
// ships no queue map file and CONFIG_DB carries no QUEUE_NAME_TO_ID_MAP.
func qosSchedCreatePolicy(t *testing.T, name string) {
	t.Run("Create policy "+name,
		processSetRequest(qosSchedPolicyUrl, qosSchedPolicyJson(name), "PUT", false, nil))
	time.Sleep(1 * time.Second)
}

////////////////////////////////////////////////////////////////////////////////
// Scheduling priority
////////////////////////////////////////////////////////////////////////////////

// Test_qos_sched_priority_and_weight covers STRICT priority reaching the
// SCHEDULER type field, which orchagent turns into SAI_SCHEDULING_TYPE_STRICT,
// and a weight reaching the weight field on the DWRR scheduler beside it.
func Test_qos_sched_priority_and_weight(t *testing.T) {
	defer qosSchedSetup()()

	t.Log("\n\n+++++++++++++ STRICT priority becomes a STRICT scheduler ++++++++++++")
	qosSchedCreatePolicy(t, qosSchedSp)

	// The rate is divided by eight because SCHEDULER counts bytes while
	// OpenConfig counts bits.
	expected_map := qosSchedStrictRow(qosSchedSp)
	t.Run("Verify strict scheduler",
		verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@0", expected_map, false))

	expected_map = map[string]interface{}{"SCHEDULER": map[string]interface{}{
		qosSchedSp + "@1": map[string]interface{}{
			"type":   "DWRR",
			"weight": "10",
			"cir":    "50000000",
		}}}
	t.Run("Verify weighted scheduler",
		verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@1", expected_map, false))

	// The input is recorded beside the scheduler, out of orchagent's way.
	expected_map = map[string]interface{}{"QOS_UMF_SCHEDULER": map[string]interface{}{
		qosSchedSp + "@0": map[string]interface{}{
			"id":         qosSchedQueueHi,
			"queue":      qosSchedQueueHi,
			"input_type": "QUEUE",
		}}}
	t.Run("Verify strict scheduler input",
		verifyDbResult(rclient, "QOS_UMF_SCHEDULER|"+qosSchedSp+"@0", expected_map, false))

	expected_map = map[string]interface{}{"QOS_UMF_SCHEDULER": map[string]interface{}{
		qosSchedSp + "@1": map[string]interface{}{
			"id":         qosSchedQueueLo,
			"queue":      qosSchedQueueLo,
			"input_type": "QUEUE",
			"weight":     "10",
		}}}
	t.Run("Verify weighted scheduler input",
		verifyDbResult(rclient, "QOS_UMF_SCHEDULER|"+qosSchedSp+"@1", expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Egress interface binding
////////////////////////////////////////////////////////////////////////////////

// Test_qos_sched_egress_bind_standalone covers binding an interface to a policy
// that already exists, which is how a client that defines its policies once and
// attaches ports later behaves. Every scheduler in the policy has to reach the
// matching queue of that interface, else orchagent never programs it.
func Test_qos_sched_egress_bind_standalone(t *testing.T) {
	defer qosSchedSetup()()

	t.Log("\n\n+++++++++++++ Create the policy on its own ++++++++++++")
	qosSchedCreatePolicy(t, qosSchedSp)

	expected_map := qosSchedStrictRow(qosSchedSp)
	t.Run("Verify policy exists",
		verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@0", expected_map, false))

	t.Log("\n\n+++++++++++++ Bind an interface in a separate request ++++++++++++")
	url_body_json := `{"config":{"name":"` + qosSchedSp + `"}}`
	t.Run("Bind policy on egress",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf), url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map = map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp + "@0",
		}}}
	t.Run("Verify high queue carries the strict scheduler",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, expected_map, false))

	expected_map = map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueLoId: map[string]interface{}{
			"scheduler": qosSchedSp + "@1",
		}}}
	t.Run("Verify queue 0 carries the weighted scheduler",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueLoId, expected_map, false))

	t.Log("\n\n+++++++++++++ Read the binding back ++++++++++++")
	expected_get_json := `{"openconfig-qos:config":{"name":"` + qosSchedSp + `"}}`
	t.Run("Get binding",
		processGetRequest(qosSchedEgressBindUrl(qosSchedIntf)+"/config", nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_sched_egress_rebind covers moving an interface from one policy to
// another. Every queue has to end up naming the new policy's schedulers.
func Test_qos_sched_egress_rebind(t *testing.T) {
	defer qosSchedSetup()()

	qosSchedCreatePolicy(t, qosSchedSp)
	qosSchedCreatePolicy(t, qosSchedSp2)

	t.Log("\n\n+++++++++++++ Bind the first policy ++++++++++++")
	t.Run("Bind first policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp + "@0",
		}}}
	t.Run("Verify first policy bound",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, expected_map, false))

	t.Log("\n\n+++++++++++++ Rebind to the second policy ++++++++++++")
	t.Run("Bind second policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp2+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map = map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp2 + "@0",
		}}}
	t.Run("Verify high queue moved",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, expected_map, false))

	expected_map = map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueLoId: map[string]interface{}{
			"scheduler": qosSchedSp2 + "@1",
		}}}
	t.Run("Verify queue 0 moved",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueLoId, expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_sched_egress_bind_clears_stale_queue covers a queue that an earlier
// policy bound but the incoming one does not drain. Leaving it behind would keep
// the interface attached to a scheduler nothing references any more.
func Test_qos_sched_egress_bind_clears_stale_queue(t *testing.T) {
	cleanuptbl := qosSchedCleanupTbl()
	staleKey := qosSchedIntf + "|3"
	cleanuptbl["QUEUE"].(map[string]interface{})[staleKey] = ""
	unloadDB(db.ConfigDB, cleanuptbl)
	defer unloadDB(db.ConfigDB, cleanuptbl)

	loadDB(db.ConfigDB, map[string]interface{}{"QUEUE": map[string]interface{}{
		staleKey: map[string]interface{}{"scheduler": "SP_UT_OLD@0"},
	}})

	qosSchedCreatePolicy(t, qosSchedSp)

	t.Log("\n\n+++++++++++++ Binding clears a queue the policy does not drain ++++++++++++")
	t.Run("Bind policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp + "@0",
		}}}
	t.Run("Verify new binding applied",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, expected_map, false))

	t.Run("Verify stale queue cleared",
		verifyDbResult(rclient, "QUEUE|"+staleKey, qosSchedUnboundQueue(), false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_sched_egress_unbind covers removing the binding, which has to clear
// the scheduler from every queue of that interface.
func Test_qos_sched_egress_unbind(t *testing.T) {
	defer qosSchedSetup()()

	qosSchedCreatePolicy(t, qosSchedSp)
	t.Run("Bind policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Unbinding clears every queue ++++++++++++")
	t.Run("Unbind policy", processDeleteRequest(qosSchedEgressBindUrl(qosSchedIntf), false))
	time.Sleep(1 * time.Second)

	t.Run("Verify high queue cleared",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, qosSchedUnboundQueue(), false))
	t.Run("Verify queue 0 cleared",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueLoId, qosSchedUnboundQueue(), false))

	// The policy itself outlives the binding.
	expected_map := qosSchedStrictRow(qosSchedSp)
	t.Run("Verify policy survives",
		verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@0", expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_sched_egress_bind_other_interface covers unbinding one interface
// while a second stays bound to the same policy.
func Test_qos_sched_egress_bind_other_interface(t *testing.T) {
	defer qosSchedSetup()()

	qosSchedCreatePolicy(t, qosSchedSp)
	for _, intf := range []string{qosSchedIntf, qosSchedIntf2} {
		t.Run("Bind "+intf,
			processSetRequest(qosSchedEgressBindUrl(intf),
				`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	}
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Unbinding one leaves the other ++++++++++++")
	t.Run("Unbind first", processDeleteRequest(qosSchedEgressBindUrl(qosSchedIntf), false))
	time.Sleep(1 * time.Second)

	t.Run("Verify first cleared",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, qosSchedUnboundQueue(), false))

	expected_map := map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf2 + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp + "@0",
		}}}
	t.Run("Verify second still bound",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf2+"|"+qosSchedQueueHiId, expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_sched_egress_bind_same_request covers a policy and its interface
// binding arriving together. The scheduler-policy transformer already binds
// interfaces it finds in the request tree, so the interface side has to defer to
// it rather than reject a policy that CONFIG_DB has not seen yet.
func Test_qos_sched_egress_bind_same_request(t *testing.T) {
	defer qosSchedSetup()()

	t.Log("\n\n+++++++++++++ Create and bind in one request ++++++++++++")
	url_body_json := `{
		"openconfig-qos:scheduler-policies":{"scheduler-policy":` + qosSchedPolicyListJson(qosSchedSp) + `},
		"openconfig-qos:interfaces":{"interface":[{
			"interface-id":"` + qosSchedIntf + `",
			"config":{"interface-id":"` + qosSchedIntf + `"},
			"output":{"scheduler-policy":{"config":{"name":"` + qosSchedSp + `"}}}
		}]}}`
	t.Run("Create and bind together",
		processSetRequest("/openconfig-qos:qos", url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp + "@0",
		}}}
	t.Run("Verify high queue bound",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, expected_map, false))

	expected_map = map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueLoId: map[string]interface{}{
			"scheduler": qosSchedSp + "@1",
		}}}
	t.Run("Verify queue 0 bound",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueLoId, expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_sched_egress_rebind_same_request covers a rebind where the incoming
// policy is defined by the same request. The interface side cannot resolve that
// policy from CONFIG_DB, but the rows the previous policy owned still have to be
// cleared, or the interface stays attached to a policy it no longer names.
func Test_qos_sched_egress_rebind_same_request(t *testing.T) {
	cleanuptbl := qosSchedCleanupTbl()
	staleKey := qosSchedIntf + "|3"
	cleanuptbl["QUEUE"].(map[string]interface{})[staleKey] = ""
	unloadDB(db.ConfigDB, cleanuptbl)
	defer unloadDB(db.ConfigDB, cleanuptbl)

	// A queue the first policy drained that the second one does not.
	loadDB(db.ConfigDB, map[string]interface{}{"QUEUE": map[string]interface{}{
		staleKey: map[string]interface{}{"scheduler": qosSchedSp + "@2"},
	}})
	qosSchedCreatePolicy(t, qosSchedSp)
	t.Run("Bind first policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Rebind to a policy defined in the same request ++++++++++++")
	url_body_json := `{
		"openconfig-qos:scheduler-policies":{"scheduler-policy":` + qosSchedPolicyListJson(qosSchedSp2) + `},
		"openconfig-qos:interfaces":{"interface":[{
			"interface-id":"` + qosSchedIntf + `",
			"config":{"interface-id":"` + qosSchedIntf + `"},
			"output":{"scheduler-policy":{"config":{"name":"` + qosSchedSp2 + `"}}}
		}]}}`
	t.Run("Create and rebind together",
		processSetRequest("/openconfig-qos:qos", url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp2 + "@0",
		}}}
	t.Run("Verify high queue moved",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, expected_map, false))

	t.Run("Verify stale queue cleared",
		verifyDbResult(rclient, "QUEUE|"+staleKey, qosSchedUnboundQueue(), false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Sequences feeding several queues
////////////////////////////////////////////////////////////////////////////////

// qosSchedFpPolicyJson is the shape of policy featureprofiles programs: a
// weighted sequence feeding several queues without naming a scheduler type, and
// a strict sequence feeding two queues whose inputs still carry a weight.
func qosSchedFpPolicyJson(name string) string {
	return `{"openconfig-qos:scheduler-policy":[{
		"name":"` + name + `",
		"config":{"name":"` + name + `"},
		"schedulers":{"scheduler":[
		{
			"sequence":0,
			"config":{"sequence":0,"priority":"STRICT"},
			"inputs":{"input":[
				{"id":"AF4","config":{"id":"AF4","input-type":"QUEUE","queue":"AF4","weight":"99"}},
				{"id":"NC1","config":{"id":"NC1","input-type":"QUEUE","queue":"NC1","weight":"100"}}]}
		},
		{
			"sequence":1,
			"config":{"sequence":1},
			"inputs":{"input":[
				{"id":"BE1","config":{"id":"BE1","input-type":"QUEUE","queue":"BE1","weight":"1"}},
				{"id":"BE0","config":{"id":"BE0","input-type":"QUEUE","queue":"BE0","weight":"2"}},
				{"id":"AF1","config":{"id":"AF1","input-type":"QUEUE","queue":"AF1","weight":"4"}}]}
		}]}
	}]}`
}

// qosSchedPolicyInputUrl is one input of one scheduler of a policy.
func qosSchedPolicyInputUrl(name string, seq string, id string) string {
	return qosSchedPolicyUrl + "/scheduler-policy[name=" + name + "]/schedulers/scheduler[sequence=" + seq +
		"]/inputs/input[id=" + id + "]"
}

// Test_qos_sched_multi_input covers a sequence with several inputs, each of
// which needs a scheduler of its own with its own weight, a strict sequence
// whose inputs carry a weight SAI does not take, and a scheduler with no type,
// which gets no shaper. All of it has to read back as configured.
func Test_qos_sched_multi_input(t *testing.T) {
	defer qosSchedSetup()()

	t.Log("\n\n+++++++++++++ One scheduler per input ++++++++++++")
	t.Run("Create policy",
		processSetRequest(qosSchedPolicyUrl, qosSchedFpPolicyJson(qosSchedSp), "PUT", false, nil))
	time.Sleep(1 * time.Second)

	for _, id := range []string{"AF4", "NC1"} {
		expected_map := map[string]interface{}{"SCHEDULER": map[string]interface{}{
			qosSchedSp + "@0@" + id: map[string]interface{}{"type": "STRICT"}}}
		t.Run("Verify strict scheduler of "+id,
			verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@0@"+id, expected_map, false))
	}
	expected_map := map[string]interface{}{"SCHEDULER": map[string]interface{}{
		qosSchedSp + "@1@BE0": map[string]interface{}{"type": "DWRR", "weight": "2"}}}
	t.Run("Verify weighted scheduler of BE0",
		verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@1@BE0", expected_map, false))

	expected_map = map[string]interface{}{"QOS_UMF_SCHEDULER": map[string]interface{}{
		qosSchedSp + "@0@AF4": map[string]interface{}{
			"id": "AF4", "queue": "AF4", "input_type": "QUEUE", "weight": "99"}}}
	t.Run("Verify strict input keeps its weight",
		verifyDbResult(rclient, "QOS_UMF_SCHEDULER|"+qosSchedSp+"@0@AF4", expected_map, false))

	t.Run("Verify no scheduler for the whole sequence",
		verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@0", map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ Queues sharing a sequence keep distinct ids ++++++++++++")
	t.Run("Bind policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	for id, qid := range map[string]string{"BE1": "0", "BE0": "1", "AF1": "2", "AF4": "5", "NC1": "6"} {
		seq := "1"
		if id == "AF4" || id == "NC1" {
			seq = "0"
		}
		expected_map := map[string]interface{}{"QUEUE": map[string]interface{}{
			qosSchedIntf + "|" + qid: map[string]interface{}{"scheduler": qosSchedSp + "@" + seq + "@" + id}}}
		t.Run("Verify queue "+id+" on hardware queue "+qid,
			verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qid, expected_map, false))
	}

	t.Log("\n\n+++++++++++++ Read back ++++++++++++")
	t.Run("Get strict input state",
		processGetRequest(qosSchedPolicyInputUrl(qosSchedSp, "0", "AF4")+"/state", nil,
			`{"openconfig-qos:state":{"id":"AF4","input-type":"QUEUE","queue":"AF4","weight":"99"}}`, false))
	t.Run("Get weighted input state",
		processGetRequest(qosSchedPolicyInputUrl(qosSchedSp, "1", "BE0")+"/state", nil,
			`{"openconfig-qos:state":{"id":"BE0","input-type":"QUEUE","queue":"BE0","weight":"2"}}`, false))
	t.Run("Get strict scheduler state",
		processGetRequest(qosSchedPolicyUrl+"/scheduler-policy[name="+qosSchedSp+"]/schedulers/scheduler[sequence=0]/state", nil,
			`{"openconfig-qos:state":{"priority":"STRICT","sequence":0}}`, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_sched_split_sequence_rebinds_queue covers a single input sequence
// gaining a second input. Its scheduler is renamed to the per input form, so a
// queue already bound to the old name has to follow, or deleting the old
// scheduler fails while the queue still names it.
func Test_qos_sched_split_sequence_rebinds_queue(t *testing.T) {
	defer qosSchedSetup()()

	qosSchedCreatePolicy(t, qosSchedSp)
	t.Run("Bind policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Sequence 1 gains a second input ++++++++++++")
	url_body_json := `{"openconfig-qos:scheduler-policy":[{
		"name":"` + qosSchedSp + `",
		"config":{"name":"` + qosSchedSp + `"},
		"schedulers":{"scheduler":[
		{
			"sequence":0,
			"config":{"sequence":0,"priority":"STRICT"},
			"inputs":{"input":[{"id":"` + qosSchedQueueHi + `",
				"config":{"id":"` + qosSchedQueueHi + `","input-type":"QUEUE"}}]}
		},
		{
			"sequence":1,
			"config":{"sequence":1},
			"inputs":{"input":[
				{"id":"BE1","config":{"id":"BE1","input-type":"QUEUE","weight":"10"}},
				{"id":"BE0","config":{"id":"BE0","input-type":"QUEUE","weight":"20"}}]}
		}]}
	}]}`
	t.Run("Replace policy",
		processSetRequest(qosSchedPolicyUrl, url_body_json, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify old scheduler removed",
		verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@1", map[string]interface{}{}, false))

	expected_map := map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueLoId: map[string]interface{}{
			"scheduler": qosSchedSp + "@1@BE1",
		}}}
	t.Run("Verify queue follows its input",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueLoId, expected_map, false))

	expected_map = map[string]interface{}{"QUEUE": map[string]interface{}{
		qosSchedIntf + "|" + qosSchedQueueHiId: map[string]interface{}{
			"scheduler": qosSchedSp + "@0",
		}}}
	t.Run("Verify untouched sequence stays bound",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, expected_map, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_replace_empties_bound_policy covers a replace of /qos that keeps a
// bound interface but no longer gives its policy any scheduler, as a test
// starting over on a box configured by an earlier run does. The schedulers go,
// so the queues naming them have to be unbound in the same request.
func Test_qos_replace_empties_bound_policy(t *testing.T) {
	defer qosSchedSetup()()

	qosSchedCreatePolicy(t, qosSchedSp)
	t.Run("Bind policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"`+qosSchedSp+`"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Replace /qos with the policy emptied ++++++++++++")
	url_body_json := `{"openconfig-qos:qos":{
		"interfaces":{"interface":[{"interface-id":"` + qosSchedIntf + `",
			"config":{"interface-id":"` + qosSchedIntf + `"}}]},
		"scheduler-policies":{"scheduler-policy":[{"name":"` + qosSchedSp + `",
			"config":{"name":"` + qosSchedSp + `"}}]}}}`
	t.Run("Replace qos",
		processSetRequest("/openconfig-qos:qos", url_body_json, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	for _, seq := range []string{"0", "1"} {
		t.Run("Verify scheduler "+seq+" removed",
			verifyDbResult(rclient, "SCHEDULER|"+qosSchedSp+"@"+seq, map[string]interface{}{}, false))
	}
	for _, qid := range []string{qosSchedQueueHiId, qosSchedQueueLoId} {
		t.Run("Verify queue "+qid+" unbound",
			verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qid, qosSchedUnboundQueue(), false))
	}

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_intf_ref covers the interface-ref of a QoS interface. SONiC applies
// QoS to the port itself, so only a reference to that same port is accepted.
func Test_qos_intf_ref(t *testing.T) {
	intfJson := func(ref string) string {
		return `{"openconfig-qos:interfaces":{"interface":[{
			"interface-id":"` + qosSchedIntf + `",
			"config":{"interface-id":"` + qosSchedIntf + `"},
			"interface-ref":{"config":{"interface":"` + ref + `"}}
		}]}}`
	}

	t.Log("\n\n+++++++++++++ A reference to the interface itself is accepted ++++++++++++")
	t.Run("Reference self",
		processSetRequest("/openconfig-qos:qos/interfaces", intfJson(qosSchedIntf), "PATCH", false, nil))
	t.Run("Get reference config",
		processGetRequest("/openconfig-qos:qos/interfaces/interface[interface-id="+qosSchedIntf+"]/interface-ref/config",
			nil, `{"openconfig-qos:config":{"interface":"`+qosSchedIntf+`"}}`, false))

	t.Log("\n\n+++++++++++++ A reference to another interface is refused ++++++++++++")
	var expErr error = tlerr.NotSupported("QoS interface %v: interface-ref must name %v, not %v",
		qosSchedIntf, qosSchedIntf, qosSchedIntf2)
	t.Run("Reference other",
		processSetRequest("/openconfig-qos:qos/interfaces", intfJson(qosSchedIntf2), "PATCH", true, expErr))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Guards
////////////////////////////////////////////////////////////////////////////////

// Test_qos_sched_egress_bind_unknown_policy covers naming a policy that does not
// exist. Accepting it would leave the interface silently unscheduled.
func Test_qos_sched_egress_bind_unknown_policy(t *testing.T) {
	defer qosSchedSetup()()

	t.Log("\n\n+++++++++++++ Binding an unknown policy is refused ++++++++++++")
	expErr := tlerr.NotFoundError{Format: "Scheduler policy NO_SUCH_POLICY not found"}
	t.Run("Bind unknown policy",
		processSetRequest(qosSchedEgressBindUrl(qosSchedIntf),
			`{"config":{"name":"NO_SUCH_POLICY"}}`, "PATCH", true, expErr))

	empty := map[string]interface{}{}
	t.Run("Verify nothing bound",
		verifyDbResult(rclient, "QUEUE|"+qosSchedIntf+"|"+qosSchedQueueHiId, empty, false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}
