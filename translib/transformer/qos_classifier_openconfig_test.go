////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia                                                      //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//  You may obtain a copy of the License at                                   //
//                                                                            //
//     http://www.apache.org/licenses/LICENSE-2.0                             //
//                                                                            //
//  Unless required by applicable law or agreed to in writing, software       //
//  distributed under the License is distributed on an "AS IS" BASIS,         //
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.  //
//  See the License for the specific language governing permissions and       //
//  limitations under the License.                                            //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

//go:build testapp
// +build testapp

package transformer_test

// Tests for DSCP classifiers, the ones held in DSCP_TO_TC_MAP:
//
//	/qos/classifiers/classifier[name]/terms/term[id]/conditions/ipv4/config/dscp
//	/qos/classifiers/classifier[name]/terms/term[id]/conditions/ipv4/config/dscp-set
//
// DSCP_TO_TC_MAP is keyed by DSCP value, so it keeps neither the term id nor
// whether the client gave dscp or dscp-set. QOS_UMF_CLASSIFIER_TERM records
// both, so a term reads back as it was written.

import (
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const qosClsDscp = "CLS_UT_DSCP_SET"

func qosClsCleanupTbl() map[string]interface{} {
	return map[string]interface{}{
		"DSCP_TO_TC_MAP":     map[string]interface{}{qosClsDscp: ""},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{qosClsDscp: ""},
		"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{
			qosClsDscp + "|af4": "", qosClsDscp + "|ef": "", qosClsDscp + "|1": "",
		},
		"QOS_FWD_GROUP": map[string]interface{}{qosRwFg: "", qosRwFg2: ""},
	}
}

func qosClsTermUrl(id string) string {
	return qosRwClassifierUrl(qosClsDscp) + "/terms/term[id=" + id + "]"
}

// Test_qos_classifier_dscp_set covers a term matching several DSCP values, as
// the featureprofiles QoS tests configure them.
func Test_qos_classifier_dscp_set(t *testing.T) {
	cleanuptbl := qosClsCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosRwPreloadFwdGroup())
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Set a dscp-set term and a dscp term ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosClsDscp + `",
		"config":{"name":"` + qosClsDscp + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"af4",
			"config":{"id":"af4"},
			"conditions":{"ipv4":{"config":{"dscp-set":[32,34]}}},
			"actions":{"config":{"target-group":"` + qosRwFg + `"}}
		},{
			"id":"ef",
			"config":{"id":"ef"},
			"conditions":{"ipv4":{"config":{"dscp":46}}},
			"actions":{"config":{"target-group":"` + qosRwFg2 + `"}}
		}]}
	}]}`
	t.Run("Set classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosClsDscp: map[string]interface{}{"32": qosRwFgTc, "34": qosRwFgTc, "46": qosRwFg2Tc}}}
	t.Run("Verify every value is mapped",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, expected_map, false))
	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{
		qosClsDscp + "|af4": map[string]interface{}{"dscp_set@": "32,34"}}}
	t.Run("Verify dscp-set term recorded",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|af4", expected_map, false))
	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{
		qosClsDscp + "|ef": map[string]interface{}{"dscp": "46"}}}
	t.Run("Verify dscp term recorded",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|ef", expected_map, false))

	t.Log("\n\n+++++++++++++ Read the terms back under their ids ++++++++++++")
	expected_get_json := `{"openconfig-qos:config":{"dscp-set":[32,34]}}`
	t.Run("Get dscp-set", processGetRequest(qosClsTermUrl("af4")+"/conditions/ipv4/config",
		nil, expected_get_json, false))
	expected_get_json = `{"openconfig-qos:config":{"target-group":"` + qosRwFg + `"}}`
	t.Run("Get dscp-set action", processGetRequest(qosClsTermUrl("af4")+"/actions/config",
		nil, expected_get_json, false))
	expected_get_json = `{"openconfig-qos:config":{"dscp":46}}`
	t.Run("Get dscp", processGetRequest(qosClsTermUrl("ef")+"/conditions/ipv4/config",
		nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Narrow the set ++++++++++++")
	url_body_json = `{"openconfig-qos:term":[{
		"id":"af4",
		"config":{"id":"af4"},
		"conditions":{"ipv4":{"config":{"dscp-set":[32]}}},
		"actions":{"config":{"target-group":"` + qosRwFg + `"}}
	}]}`
	t.Run("Update term", processSetRequest(qosRwClassifierUrl(qosClsDscp)+"/terms",
		url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map = map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosClsDscp: map[string]interface{}{"32": qosRwFgTc, "46": qosRwFg2Tc}}}
	t.Run("Verify dropped value unmapped",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, expected_map, false))
	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{
		qosClsDscp + "|af4": map[string]interface{}{"dscp_set@": "32"}}}
	t.Run("Verify narrowed set recorded",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|af4", expected_map, false))

	t.Log("\n\n+++++++++++++ Delete one term ++++++++++++")
	t.Run("Delete term", processDeleteRequest(qosClsTermUrl("af4"), false))
	time.Sleep(1 * time.Second)

	expected_map = map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosClsDscp: map[string]interface{}{"46": qosRwFg2Tc}}}
	t.Run("Verify term values unmapped",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, expected_map, false))
	t.Run("Verify term record removed",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|af4", map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ Delete the classifier ++++++++++++")
	t.Run("Delete classifier", processDeleteRequest(qosRwClassifierUrl(qosClsDscp), false))
	time.Sleep(1 * time.Second)

	t.Run("Verify map removed",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, map[string]interface{}{}, false))
	t.Run("Verify type record removed",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER|"+qosClsDscp, map[string]interface{}{}, false))
	t.Run("Verify term records removed",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|ef", map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_classifier_replace_drops_terms covers a replace that no longer
// carries a term: its record and the values only it mapped go.
func Test_qos_classifier_replace_drops_terms(t *testing.T) {
	cleanuptbl := qosClsCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	pre_req_map := qosRwPreloadFwdGroup()
	pre_req_map["DSCP_TO_TC_MAP"] = map[string]interface{}{
		qosClsDscp: map[string]interface{}{"32": qosRwFgTc, "34": qosRwFgTc, "46": qosRwFg2Tc}}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosClsDscp: map[string]interface{}{"type": "ipv4"}}
	pre_req_map["QOS_UMF_CLASSIFIER_TERM"] = map[string]interface{}{
		qosClsDscp + "|af4": map[string]interface{}{"dscp_set@": "32,34"},
		qosClsDscp + "|ef":  map[string]interface{}{"dscp": "46"},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosClsDscp + `",
		"config":{"name":"` + qosClsDscp + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"ef",
			"config":{"id":"ef"},
			"conditions":{"ipv4":{"config":{"dscp-set":[46,48]}}},
			"actions":{"config":{"target-group":"` + qosRwFg2 + `"}}
		}]}
	}]}`
	t.Run("Replace classifier", processSetRequest(qosRwClassifierUrl(qosClsDscp),
		url_body_json, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosClsDscp: map[string]interface{}{"46": qosRwFg2Tc, "48": qosRwFg2Tc}}}
	t.Run("Verify map holds the new term only",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, expected_map, false))
	t.Run("Verify dropped term record removed",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|af4", map[string]interface{}{}, false))
	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{
		qosClsDscp + "|ef": map[string]interface{}{"dscp_set@": "46,48"}}}
	t.Run("Verify kept term rewritten",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|ef", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_classifier_replace_one_term covers a replace aimed at a single term:
// the map row is rewritten whole, yet the other terms and their values stay.
func Test_qos_classifier_replace_one_term(t *testing.T) {
	cleanuptbl := qosClsCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	pre_req_map := qosRwPreloadFwdGroup()
	pre_req_map["DSCP_TO_TC_MAP"] = map[string]interface{}{
		qosClsDscp: map[string]interface{}{"32": qosRwFgTc, "34": qosRwFgTc, "46": qosRwFg2Tc}}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosClsDscp: map[string]interface{}{"type": "ipv4"}}
	pre_req_map["QOS_UMF_CLASSIFIER_TERM"] = map[string]interface{}{
		qosClsDscp + "|af4": map[string]interface{}{"dscp_set@": "32,34"},
		qosClsDscp + "|ef":  map[string]interface{}{"dscp": "46"},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	url_body_json := `{"openconfig-qos:term":[{
		"id":"af4",
		"config":{"id":"af4"},
		"conditions":{"ipv4":{"config":{"dscp-set":[36,38]}}},
		"actions":{"config":{"target-group":"` + qosRwFg + `"}}
	}]}`
	t.Run("Replace term", processSetRequest(qosClsTermUrl("af4"), url_body_json, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosClsDscp: map[string]interface{}{"36": qosRwFgTc, "38": qosRwFgTc, "46": qosRwFg2Tc}}}
	t.Run("Verify other term value kept",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, expected_map, false))
	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{
		qosClsDscp + "|ef": map[string]interface{}{"dscp": "46"}}}
	t.Run("Verify other term record kept",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|ef", expected_map, false))
	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{
		qosClsDscp + "|af4": map[string]interface{}{"dscp_set@": "36,38"}}}
	t.Run("Verify replaced term recorded",
		verifyDbResult(rclient, "QOS_UMF_CLASSIFIER_TERM|"+qosClsDscp+"|af4", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_classifier_legacy_terms covers a map written before terms were
// recorded, where each value reads back as a term whose id is the value.
func Test_qos_classifier_legacy_terms(t *testing.T) {
	cleanuptbl := qosClsCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	pre_req_map := qosRwPreloadFwdGroup()
	pre_req_map["DSCP_TO_TC_MAP"] = map[string]interface{}{
		qosClsDscp: map[string]interface{}{"10": qosRwFgTc, "46": qosRwFg2Tc}}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosClsDscp: map[string]interface{}{"type": "ipv4"}}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	expected_get_json := `{"openconfig-qos:config":{"dscp":10}}`
	t.Run("Get legacy term", processGetRequest(qosClsTermUrl("10")+"/conditions/ipv4/config",
		nil, expected_get_json, false))

	t.Run("Delete legacy term", processDeleteRequest(qosClsTermUrl("10"), false))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosClsDscp: map[string]interface{}{"46": qosRwFg2Tc}}}
	t.Run("Verify legacy value unmapped",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_classifier_dscp_conflicts covers the term shapes a DSCP classifier
// cannot hold.
func Test_qos_classifier_dscp_conflicts(t *testing.T) {
	cleanuptbl := qosClsCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosRwPreloadFwdGroup())
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ dscp and dscp-set together are rejected ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosClsDscp + `",
		"config":{"name":"` + qosClsDscp + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"dscp":10,"dscp-set":[12,14]}}},
			"actions":{"config":{"target-group":"` + qosRwFg + `"}}
		}]}
	}]}`
	var expected_err error = tlerr.InvalidArgsError{Format: "Classifier term 1 sets both dscp and dscp-set"}
	t.Run("Set both", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)
	t.Run("Verify nothing mapped",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ A term with no target-group is rejected ++++++++++++")
	url_body_json = `{"openconfig-qos:classifier":[{
		"name":"` + qosClsDscp + `",
		"config":{"name":"` + qosClsDscp + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"dscp-set":[12,14]}}}
		}]}
	}]}`
	expected_err = tlerr.InvalidArgsError{Format: "Classifier term 1 has no target-group"}
	t.Run("Set without action", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)
	t.Run("Verify nothing mapped either",
		verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosClsDscp, map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ dscp-set on an ACL-backed classifier is rejected ++++++++++++")
	url_body_json = `{"openconfig-qos:classifier":[{
		"name":"` + qosClsDscp + `",
		"config":{"name":"` + qosClsDscp + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"dscp-set":[12,14],"source-address":"10.1.1.0/24"}}},
			"actions":{"config":{"target-group":"` + qosRwFg + `"}}
		}]}
	}]}`
	expected_err = tlerr.NotSupportedError{
		Format: "Classifier term 1 dscp-set is only supported in a classifier that matches on DSCP alone"}
	t.Run("Set ACL dscp-set", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)
	t.Run("Verify no ACL rule",
		verifyDbResult(rclient, "ACL_RULE|"+qosClsDscp+"|TERM_1", map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	unloadDB(db.ConfigDB, map[string]interface{}{
		"ACL_TABLE": map[string]interface{}{qosClsDscp: ""},
	})
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}
