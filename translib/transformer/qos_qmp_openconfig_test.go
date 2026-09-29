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

// Queue management profiles cover the OpenConfig paths:
//
//	/qos/queue-management-profiles/queue-management-profile[name]/wred/uniform/config
//	/qos/queue-management-profiles/queue-management-profile[name]/wred/uniform/state
//
// A profile becomes a WRED_PROFILE entry carrying only what orchagent takes:
// WRED on for every colour, ecn_all when enable-ecn is set, and thresholds no
// larger than an int. The OpenConfig values themselves, which read back, are
// kept in QOS_UMF_WRED_PROFILE.

import (
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const (
	qosQmpUrl  = "/openconfig-qos:qos/queue-management-profiles"
	qosQmpName = "QMP_UT_ECN"

	// The largest max-threshold OpenConfig allows, which featureprofiles uses
	// to mean no upper bound, and the most of it orchagent can take.
	qosQmpMaxTh     = "18446744073709551615"
	qosQmpMaxThWred = "2147483647"
)

func qosQmpJson(weight string) string {
	return `{"openconfig-qos:queue-management-profile":[{
		"name":"` + qosQmpName + `",
		"config":{"name":"` + qosQmpName + `"},
		"wred":{"uniform":{"config":{
			"enable-ecn":true,"drop":false,
			"min-threshold":"80000","max-threshold":"` + qosQmpMaxTh + `",
			"max-drop-probability-percent":1,"weight":` + weight + `}}}
	}]}`
}

// Test_qos_qmp_ecn covers an ECN profile whose max-threshold is 2^64-1, which
// has to reach orchagent in a form it takes and read back exactly, weight
// included.
func Test_qos_qmp_ecn(t *testing.T) {
	cleanuptbl := map[string]interface{}{
		"WRED_PROFILE":         map[string]interface{}{qosQmpName: ""},
		"QOS_UMF_WRED_PROFILE": map[string]interface{}{qosQmpName: ""},
	}
	unloadDB(db.ConfigDB, cleanuptbl)
	defer unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ An ECN profile with no upper threshold ++++++++++++")
	t.Run("Create profile",
		processSetRequest(qosQmpUrl, qosQmpJson("0"), "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	fields := map[string]interface{}{"ecn": "ecn_all"}
	for _, color := range []string{"green", "yellow", "red"} {
		fields["wred_"+color+"_enable"] = "true"
		fields[color+"_min_threshold"] = "80000"
		fields[color+"_max_threshold"] = qosQmpMaxThWred
		fields[color+"_drop_probability"] = "1"
	}
	expected_map := map[string]interface{}{"WRED_PROFILE": map[string]interface{}{qosQmpName: fields}}
	t.Run("Verify WRED_PROFILE",
		verifyDbResult(rclient, "WRED_PROFILE|"+qosQmpName, expected_map, false))

	expected_map = map[string]interface{}{"QOS_UMF_WRED_PROFILE": map[string]interface{}{qosQmpName: map[string]interface{}{
		"min_threshold": "80000", "max_threshold": qosQmpMaxTh, "max_drop_probability_percent": "1",
		"enable_ecn": "true", "drop": "false"}}}
	t.Run("Verify QOS_UMF_WRED_PROFILE",
		verifyDbResult(rclient, "QOS_UMF_WRED_PROFILE|"+qosQmpName, expected_map, false))

	t.Log("\n\n+++++++++++++ Read it back ++++++++++++")
	t.Run("Get uniform state",
		processGetRequest(qosQmpUrl+"/queue-management-profile[name="+qosQmpName+"]/wred/uniform/state", nil,
			`{"openconfig-qos:state":{"drop":false,"enable-ecn":true,"max-drop-probability-percent":1,
			"max-threshold":"`+qosQmpMaxTh+`","min-threshold":"80000","weight":0}}`, false))

	t.Log("\n\n+++++++++++++ Updating one leaf keeps the others ++++++++++++")
	t.Run("Update min-threshold",
		processSetRequest(qosQmpUrl+"/queue-management-profile[name="+qosQmpName+"]/wred/uniform/config",
			`{"openconfig-qos:config":{"min-threshold":"90000"}}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Get uniform state after update",
		processGetRequest(qosQmpUrl+"/queue-management-profile[name="+qosQmpName+"]/wred/uniform/state", nil,
			`{"openconfig-qos:state":{"drop":false,"enable-ecn":true,"max-drop-probability-percent":1,
			"max-threshold":"`+qosQmpMaxTh+`","min-threshold":"90000","weight":0}}`, false))

	t.Log("\n\n+++++++++++++ A WRED weight other than 0 is refused ++++++++++++")
	var expErr error = tlerr.NotSupported("Queue management profile %v: WRED weight %v is not supported, only 0",
		qosQmpName, uint32(3))
	t.Run("Create profile with weight",
		processSetRequest(qosQmpUrl, qosQmpJson("3"), "PATCH", true, expErr))

	gone := func(tbl string) map[string]interface{} {
		return map[string]interface{}{tbl: map[string]interface{}{qosQmpName: map[string]interface{}{}}}
	}
	t.Log("\n\n+++++++++++++ Deleting one leaf is refused ++++++++++++")
	expErr = tlerr.NotSupported("Queue management profile %v: deleting part of a profile is not supported, replace or delete the whole profile",
		qosQmpName)
	t.Run("Delete min-threshold",
		processDeleteRequest(qosQmpUrl+"/queue-management-profile[name="+qosQmpName+"]/wred/uniform/config/min-threshold", true, expErr))

	t.Log("\n\n+++++++++++++ Deleting the profile removes both entries ++++++++++++")
	t.Run("Delete profile",
		processDeleteRequest(qosQmpUrl+"/queue-management-profile[name="+qosQmpName+"]", false))
	time.Sleep(1 * time.Second)
	t.Run("Verify WRED_PROFILE deleted",
		verifyDbResult(rclient, "WRED_PROFILE|"+qosQmpName, gone("WRED_PROFILE"), false))
	t.Run("Verify QOS_UMF_WRED_PROFILE deleted",
		verifyDbResult(rclient, "QOS_UMF_WRED_PROFILE|"+qosQmpName, gone("QOS_UMF_WRED_PROFILE"), false))

	t.Log("\n\n+++++++++++++ Deleting all profiles removes an unused one ++++++++++++")
	t.Run("Create profile again",
		processSetRequest(qosQmpUrl, qosQmpJson("0"), "PATCH", false, nil))
	time.Sleep(1 * time.Second)
	t.Run("Delete all profiles", processDeleteRequest(qosQmpUrl, false))
	time.Sleep(1 * time.Second)
	t.Run("Verify WRED_PROFILE deleted by delete all",
		verifyDbResult(rclient, "WRED_PROFILE|"+qosQmpName, gone("WRED_PROFILE"), false))
	t.Run("Verify QOS_UMF_WRED_PROFILE deleted by delete all",
		verifyDbResult(rclient, "QOS_UMF_WRED_PROFILE|"+qosQmpName, gone("QOS_UMF_WRED_PROFILE"), false))

	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}
