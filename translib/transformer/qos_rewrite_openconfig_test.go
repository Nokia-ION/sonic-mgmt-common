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

// Tests for the actions a QoS classifier term applies to the traffic it
// selects, driven through the OpenConfig QoS paths:
//
//	/qos/classifiers/classifier[name]/terms/term[id]/actions/config/target-group
//	/qos/interfaces/interface[interface-id]/input/classifiers/classifier[type]
//
// This is the model's equivalent of a policy-map class that assigns a traffic
// class to matched traffic:
//
//	policy-map type quality-of-service REWRITE
//	   class QOS-TC-SSH
//	      set traffic-class 4
//
// target-group names a forwarding group, whose priority is the traffic class.
// A term that carries such an action and matches on more than a DSCP value
// cannot be a DSCP_TO_TC_MAP entry, so the classifier is realized as an ingress
// ACL, the same machinery a policed classifier uses, with the traffic class on
// the rule as the SET_TC action (SAI_ACL_ENTRY_ATTR_ACTION_SET_TC). See
// xfmr_qos_policer.go.
//
// remark/set-dscp is the SET_DSCP action (SAI_ACL_ENTRY_ATTR_ACTION_SET_DSCP).
// set-dot1p and set-mpls-tc have no ingress ACL backend action, so they are
// rejected rather than silently dropped.

import (
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const (
	qosRwCls     = "CLS_UT_RW"
	qosRwClsV6   = "CLS_UT_RW6"
	qosRwPolCls  = "CLS_UT_RW_POL"
	qosRwDscpCls = "CLS_UT_RW_DSCP"
	qosRwFg      = "FG_UT_RW_AF4"
	qosRwFgTc    = "4"
	qosRwFg2     = "FG_UT_RW_EF"
	qosRwFg2Tc   = "6"
	qosRwIntf    = "Ethernet0"

	qosRwClassifiersUrl = "/openconfig-qos:qos/classifiers"
)

// qosRwClassifierUrl addresses one classifier list instance.
func qosRwClassifierUrl(name string) string {
	return qosRwClassifiersUrl + "/classifier[name=" + name + "]"
}

// qosRwIntfClassifiersUrl is the classifier binding of one interface.
func qosRwIntfClassifiersUrl(ifName string) string {
	return "/openconfig-qos:qos/interfaces/interface[interface-id=" + ifName +
		"]/input/classifiers"
}

func qosRwCleanupTbl() map[string]interface{} {
	return map[string]interface{}{
		"ACL_RULE": map[string]interface{}{
			qosRwCls + "|TERM_1":   "",
			qosRwCls + "|TERM_2":   "",
			qosRwClsV6 + "|TERM_1": "",
		},
		"ACL_TABLE": map[string]interface{}{qosRwCls: "", qosRwClsV6: ""},
		"ACL_TABLE_TYPE": map[string]interface{}{
			"QOS_POLICER_V4": "", "QOS_POLICER_V6": "",
		},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{qosRwCls: "", qosRwClsV6: ""},
		"QOS_FWD_GROUP":      map[string]interface{}{qosRwFg: "", qosRwFg2: ""},
	}
}

// qosRwPreloadFwdGroup puts the forwarding groups a term's target-group names
// into CONFIG_DB. target-group carries a name, SET_TC carries a number, so the
// group is what translates between the two in either direction.
func qosRwPreloadFwdGroup() map[string]interface{} {
	return map[string]interface{}{
		"QOS_FWD_GROUP": map[string]interface{}{
			qosRwFg:  map[string]interface{}{"priority": qosRwFgTc},
			qosRwFg2: map[string]interface{}{"priority": qosRwFg2Tc},
		},
	}
}

////////////////////////////////////////////////////////////////////////////////
// Write path
////////////////////////////////////////////////////////////////////////////////

func Test_qos_rewrite_set_traffic_class(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosRwPreloadFwdGroup())
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ A class that sets traffic class ++++++++++++")
	// Two classes of one policy map, each selecting a service by port and
	// moving what it selects to a forwarding class.
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{
				"ipv4":{"config":{"protocol":"IP_TCP"}},
				"transport":{"config":{"destination-port":22}}
			},
			"actions":{"config":{"target-group":"` + qosRwFg + `"}}
		},{
			"id":"2",
			"config":{"id":"2"},
			"conditions":{
				"ipv4":{"config":{"protocol":"IP_UDP"}},
				"transport":{"config":{"destination-port":69}}
			},
			"actions":{"config":{"target-group":"` + qosRwFg2 + `"}}
		}]}
	}]}`
	t.Run("Set rewriting classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// The table type declares every action a classifier can need whether this
	// one uses it or not, because a table keeps the type it was created with.
	expected_map := map[string]interface{}{"ACL_TABLE_TYPE": map[string]interface{}{
		"QOS_POLICER_V4": map[string]interface{}{
			"MATCHES@":     "IP_TYPE,IP_PROTOCOL,L4_DST_PORT",
			"ACTIONS@":     qosAclActions,
			"BIND_POINTS@": "PORT",
		}}}
	t.Run("Verify ACL_TABLE_TYPE", verifyDbResult(rclient, "ACL_TABLE_TYPE|QOS_POLICER_V4", expected_map, false))

	// target-group resolves through the forwarding group's priority, which is
	// the traffic class SAI programs as the SET_TC action.
	expected_map = map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosRwCls + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"IP_PROTOCOL":   "6",
			"L4_DST_PORT":   "22",
			"SET_TC":        qosRwFgTc,
		}}}
	t.Run("Verify rule one", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_1", expected_map, false))

	expected_map = map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosRwCls + "|TERM_2": map[string]interface{}{
			"PRIORITY":      "59998",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"IP_PROTOCOL":   "17",
			"L4_DST_PORT":   "69",
			"SET_TC":        qosRwFg2Tc,
		}}}
	t.Run("Verify rule two", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_2", expected_map, false))

	t.Log("\n\n+++++++++++++ Delete removes the synthesized ACL objects ++++++++++++")
	t.Run("Delete classifier", processDeleteRequest(qosRwClassifierUrl(qosRwCls), false))
	time.Sleep(1 * time.Second)

	empty := map[string]interface{}{}
	t.Run("Verify rule gone", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_1", empty, false))
	t.Run("Verify table gone", verifyDbResult(rclient, "ACL_TABLE|"+qosRwCls, empty, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_traffic_class_only covers a class that moves traffic to
// another forwarding class. The match is richer than a DSCP value, so it cannot
// be the DSCP_TO_TC_MAP entry the same action would have produced on a
// DSCP-only term.
func Test_qos_rewrite_traffic_class_only(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosRwPreloadFwdGroup())
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ set traffic-class on a v6 classifier ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosRwClsV6 + `",
		"config":{"name":"` + qosRwClsV6 + `","type":"IPV6"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv6":{"config":{"protocol":"IP_TCP","source-address":"2001:db8::1/128"}}},
			"actions":{"config":{"target-group":"` + qosRwFg + `"}}
		}]}
	}]}`
	t.Run("Set classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosRwClsV6 + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV6ANY",
			"IP_PROTOCOL":   "6",
			"SRC_IPV6":      "2001:db8::1/128",
			"SET_TC":        qosRwFgTc,
		}}}
	t.Run("Verify rule", verifyDbResult(rclient, "ACL_RULE|"+qosRwClsV6+"|TERM_1", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_dscp_match_term_becomes_a_rule covers a classifier that has
// both a term needing an ACL and a term that on its own would have been a
// DSCP_TO_TC_MAP entry. A classifier is one thing or the other, so the second
// term has to become a rule too rather than being dropped without a word.
func Test_qos_rewrite_dscp_match_term_becomes_a_rule(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosRwPreloadFwdGroup())
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ A DSCP-only term of an ACL classifier ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"transport":{"config":{"destination-port":22}}},
			"actions":{"config":{"target-group":"` + qosRwFg2 + `"}}
		},{
			"id":"2",
			"config":{"id":"2"},
			"conditions":{"ipv4":{"config":{"dscp":24}}},
			"actions":{"config":{"target-group":"` + qosRwFg + `"}}
		}]}
	}]}`
	t.Run("Set classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosRwCls + "|TERM_2": map[string]interface{}{
			"PRIORITY":      "59998",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"DSCP":          "24",
			"SET_TC":        qosRwFgTc,
		}}}
	t.Run("Verify DSCP term is a rule", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_2", expected_map, false))

	// Nothing may land in the map table, else the same term is classified twice.
	t.Run("Verify no DSCP_TO_TC_MAP", verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosRwCls,
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_change_target_group_only covers moving a term that is already
// configured to another forwarding class, by writing the leaf and nothing else.
// Such a request carries no conditions, and the term goes on matching what it
// did.
func Test_qos_rewrite_change_target_group_only(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosRwPreloadFwdGroup()
	pre_req_map["ACL_TABLE"] = map[string]interface{}{qosRwCls: map[string]interface{}{
		"type":        "QOS_POLICER_V4",
		"stage":       "INGRESS",
		"policy_desc": "OpenConfig QoS classifier " + qosRwCls,
	}}
	pre_req_map["ACL_RULE"] = map[string]interface{}{qosRwCls + "|TERM_1": map[string]interface{}{
		"PRIORITY":      "59999",
		"PACKET_ACTION": "FORWARD",
		"IP_TYPE":       "IPV4ANY",
		"IP_PROTOCOL":   "6",
		"L4_DST_PORT":   "22",
		"SET_TC":        qosRwFgTc,
	}}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosRwCls: map[string]interface{}{"type": "ipv4"},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Write target-group on its own ++++++++++++")
	url := qosRwClassifierUrl(qosRwCls) +
		"/terms/term[id=1]/actions/config/target-group"
	t.Run("Change target-group", processSetRequest(url,
		`{"openconfig-qos:target-group":"`+qosRwFg2+`"}`, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// The new traffic class is in place and the term still selects what it did.
	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosRwCls + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"IP_PROTOCOL":   "6",
			"L4_DST_PORT":   "22",
			"SET_TC":        qosRwFg2Tc,
		}}}
	t.Run("Verify ACL_RULE", verifyDbResult(rclient,
		"ACL_RULE|"+qosRwCls+"|TERM_1", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Read path
////////////////////////////////////////////////////////////////////////////////

// Test_qos_rewrite_get_classifier covers rebuilding the OpenConfig view of the
// actions from the rule fields holding them. SET_TC is a number and
// target-group is a leafref to a forwarding group, so the group is what makes
// the traffic class reportable.
func Test_qos_rewrite_get_classifier(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosRwPreloadFwdGroup()
	pre_req_map["ACL_TABLE_TYPE"] = map[string]interface{}{"QOS_POLICER_V4": map[string]interface{}{
		"MATCHES@":     "IP_TYPE,L4_DST_PORT",
		"ACTIONS@":     qosAclActions,
		"BIND_POINTS@": "PORT",
	}}
	pre_req_map["ACL_TABLE"] = map[string]interface{}{qosRwCls: map[string]interface{}{
		"type":        "QOS_POLICER_V4",
		"stage":       "INGRESS",
		"policy_desc": "OpenConfig QoS classifier " + qosRwCls,
	}}
	pre_req_map["ACL_RULE"] = map[string]interface{}{qosRwCls + "|TERM_1": map[string]interface{}{
		"PRIORITY":      "59999",
		"PACKET_ACTION": "FORWARD",
		"IP_TYPE":       "IPV4ANY",
		"L4_DST_PORT":   "22",
		"SET_TC":        qosRwFgTc,
	}}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosRwCls: map[string]interface{}{"type": "ipv4"},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Get the actions of a rewriting classifier ++++++++++++")
	expected_get_json := `{"openconfig-qos:actions":{
		"config":{"target-group":"` + qosRwFg + `"},
		"state":{"target-group":"` + qosRwFg + `"}}}`
	t.Run("Get term actions", processGetRequest(
		qosRwClassifierUrl(qosRwCls)+"/terms/term[id=1]/actions", nil, expected_get_json, false))

	expected_get_json = `{"openconfig-qos:target-group":"` + qosRwFg + `"}`
	t.Run("Get target-group leaf", processGetRequest(
		qosRwClassifierUrl(qosRwCls)+"/terms/term[id=1]/actions/config/target-group",
		nil, expected_get_json, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_get_no_forwarding_group covers a traffic class no forwarding
// group carries. target-group is a leafref, so naming a group that does not
// exist is worse than leaving the action out until one does.
func Test_qos_rewrite_get_no_forwarding_group(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := map[string]interface{}{
		"ACL_TABLE": map[string]interface{}{qosRwCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwCls,
		}},
		"ACL_RULE": map[string]interface{}{qosRwCls + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"L4_DST_PORT":   "22",
			"SET_TC":        "6",
		}},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{
			qosRwCls: map[string]interface{}{"type": "ipv4"},
		},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ An unresolvable traffic class is not reported ++++++++++++")
	t.Run("Get term actions", processGetRequest(
		qosRwClassifierUrl(qosRwCls)+"/terms/term[id=1]/actions", nil, emptyJson, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Interface binding
////////////////////////////////////////////////////////////////////////////////

// Test_qos_rewrite_interface_binding covers applying a rewriting classifier to
// an interface. A rewriting classifier carries no policer, so it has no
// scheduler-policy to be bound through and is named directly instead.
func Test_qos_rewrite_interface_binding(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosRwPreloadFwdGroup()
	pre_req_map["ACL_TABLE"] = map[string]interface{}{qosRwCls: map[string]interface{}{
		"type":        "QOS_POLICER_V4",
		"stage":       "INGRESS",
		"policy_desc": "OpenConfig QoS classifier " + qosRwCls,
	}}
	pre_req_map["ACL_RULE"] = map[string]interface{}{qosRwCls + "|TERM_1": map[string]interface{}{
		"PRIORITY":      "59999",
		"PACKET_ACTION": "FORWARD",
		"IP_TYPE":       "IPV4ANY",
		"L4_DST_PORT":   "22",
		"SET_TC":        qosRwFgTc,
	}}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosRwCls: map[string]interface{}{"type": "ipv4"},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Binding adds the interface to the table ports ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"type":"IPV4",
		"config":{"type":"IPV4","name":"` + qosRwCls + `"}}]}`
	t.Run("Bind classifier", processSetRequest(qosRwIntfClassifiersUrl(qosRwIntf),
		url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosRwCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwCls,
			"ports@":      qosRwIntf,
		}}}
	t.Run("Verify port bound", verifyDbResult(rclient, "ACL_TABLE|"+qosRwCls, expected_map, false))

	t.Log("\n\n+++++++++++++ Read the binding back ++++++++++++")
	expected_get_json := `{"openconfig-qos:classifiers":{"classifier":[{
		"type":"IPV4",
		"config":{"type":"IPV4","name":"` + qosRwCls + `"},
		"state":{"type":"IPV4","name":"` + qosRwCls + `"}}]}}`
	t.Run("Get binding", processGetRequest(qosRwIntfClassifiersUrl(qosRwIntf),
		nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Read one list instance back ++++++++++++")
	intfClsV4Url := qosRwIntfClassifiersUrl(qosRwIntf) + "/classifier[type=IPV4]"
	expected_get_json = `{"openconfig-qos:classifier":[{
		"type":"IPV4",
		"config":{"type":"IPV4","name":"` + qosRwCls + `"},
		"state":{"type":"IPV4","name":"` + qosRwCls + `"}}]}`
	t.Run("Get IPV4 instance", processGetRequest(intfClsV4Url, nil, expected_get_json, false))
	t.Run("Get config name", processGetRequest(intfClsV4Url+"/config/name", nil,
		`{"openconfig-qos:name":"`+qosRwCls+`"}`, false))
	t.Run("Get state name", processGetRequest(intfClsV4Url+"/state/name", nil,
		`{"openconfig-qos:name":"`+qosRwCls+`"}`, false))
	t.Run("Get unbound IPV6 instance", processGetRequest(
		qosRwIntfClassifiersUrl(qosRwIntf)+"/classifier[type=IPV6]", nil, "", true))

	t.Log("\n\n+++++++++++++ Unbinding drops the interface again ++++++++++++")
	t.Run("Unbind classifier", processDeleteRequest(qosRwIntfClassifiersUrl(qosRwIntf), false))
	time.Sleep(1 * time.Second)

	t.Run("Verify binding gone", processGetRequest(qosRwIntfClassifiersUrl(qosRwIntf),
		nil, emptyJson, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_interface_dscp_binding_get covers reading back a DSCP map
// bound to an interface, both as the list and as one list instance. SONiC
// applies the map to both families, so it is reported under both keys.
func Test_qos_rewrite_interface_dscp_binding_get(t *testing.T) {
	cleanuptbl := map[string]interface{}{
		"PORT_QOS_MAP":       map[string]interface{}{qosRwIntf: ""},
		"DSCP_TO_TC_MAP":     map[string]interface{}{qosRwDscpCls: ""},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{qosRwDscpCls: ""},
	}
	unloadDB(db.ConfigDB, cleanuptbl)

	loadDB(db.ConfigDB, map[string]interface{}{
		"DSCP_TO_TC_MAP":     map[string]interface{}{qosRwDscpCls: map[string]interface{}{"34": "4"}},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{qosRwDscpCls: map[string]interface{}{"type": "ipv4"}},
		"PORT_QOS_MAP": map[string]interface{}{
			qosRwIntf: map[string]interface{}{"dscp_to_tc_map": qosRwDscpCls},
		},
	})
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Read the binding back ++++++++++++")
	expected_get_json := `{"openconfig-qos:classifiers":{"classifier":[{
		"type":"IPV4",
		"config":{"type":"IPV4","name":"` + qosRwDscpCls + `"},
		"state":{"type":"IPV4","name":"` + qosRwDscpCls + `"}},{
		"type":"IPV6",
		"config":{"type":"IPV6","name":"` + qosRwDscpCls + `_ipv6"},
		"state":{"type":"IPV6","name":"` + qosRwDscpCls + `_ipv6"}}]}}`
	t.Run("Get binding", processGetRequest(qosRwIntfClassifiersUrl(qosRwIntf),
		nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Read one list instance back ++++++++++++")
	intfClsV4Url := qosRwIntfClassifiersUrl(qosRwIntf) + "/classifier[type=IPV4]"
	expected_get_json = `{"openconfig-qos:classifier":[{
		"type":"IPV4",
		"config":{"type":"IPV4","name":"` + qosRwDscpCls + `"},
		"state":{"type":"IPV4","name":"` + qosRwDscpCls + `"}}]}`
	t.Run("Get IPV4 instance", processGetRequest(intfClsV4Url, nil, expected_get_json, false))
	t.Run("Get config name", processGetRequest(intfClsV4Url+"/config/name", nil,
		`{"openconfig-qos:name":"`+qosRwDscpCls+`"}`, false))
	t.Run("Get state name", processGetRequest(intfClsV4Url+"/state/name", nil,
		`{"openconfig-qos:name":"`+qosRwDscpCls+`"}`, false))
	t.Run("Get unbound MPLS instance", processGetRequest(
		qosRwIntfClassifiersUrl(qosRwIntf)+"/classifier[type=MPLS]", nil, "", true))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_interface_unbind_one_classifier covers unbinding by naming
// the list instance rather than the whole container. The binding lives in the
// backing table's ports, so the subscribe path has to report that table for the
// parent check to find the instance, and the classifier a scheduler-policy put
// on the same interface has to survive.
func Test_qos_rewrite_interface_unbind_one_classifier(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	cleanuptbl["ACL_TABLE"].(map[string]interface{})[qosRwPolCls] = ""
	cleanuptbl["ACL_RULE"].(map[string]interface{})[qosRwPolCls+"|TERM_1"] = ""
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosRwPreloadFwdGroup()
	// The rewriting classifier is bound here, under /input/classifiers.
	pre_req_map["ACL_TABLE"] = map[string]interface{}{
		qosRwCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwCls,
			"ports@":      qosRwIntf,
		},
		// The policed one was bound by its scheduler-policy instead.
		qosRwPolCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwPolCls,
			"ports@":      qosRwIntf,
		},
	}
	pre_req_map["ACL_RULE"] = map[string]interface{}{
		qosRwCls + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"L4_DST_PORT":   "22",
			"SET_TC":        qosRwFgTc,
		},
		qosRwPolCls + "|TERM_1": map[string]interface{}{
			"PRIORITY":       "59999",
			"PACKET_ACTION":  "FORWARD",
			"IP_TYPE":        "IPV4ANY",
			"L4_DST_PORT":    "179",
			"POLICER_ACTION": "SP_UT_RW_0",
		},
	}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosRwCls:    map[string]interface{}{"type": "ipv4"},
		qosRwPolCls: map[string]interface{}{"type": "ipv4"},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Unbind the named classifier instance ++++++++++++")
	t.Run("Unbind one classifier", processDeleteRequest(
		qosRwIntfClassifiersUrl(qosRwIntf)+"/classifier[type=IPV4]", false))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosRwCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwCls,
		}}}
	t.Run("Verify port unbound", verifyDbResult(rclient, "ACL_TABLE|"+qosRwCls, expected_map, false))

	t.Log("\n\n+++++++++++++ The policed classifier stays bound ++++++++++++")
	expected_map = map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosRwPolCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwPolCls,
			"ports@":      qosRwIntf,
		}}}
	t.Run("Verify policer still bound", verifyDbResult(rclient, "ACL_TABLE|"+qosRwPolCls, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_interface_unbind_leaves_policer covers unbinding on an
// interface whose only ACL-backed classifier is a policed one. That classifier
// belongs to /input/scheduler-policy, so nothing here is its to unbind, and the
// DSCP map that is reported under this list key is what the delete clears.
func Test_qos_rewrite_interface_unbind_leaves_policer(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	cleanuptbl["ACL_TABLE"].(map[string]interface{})[qosRwPolCls] = ""
	cleanuptbl["ACL_RULE"].(map[string]interface{})[qosRwPolCls+"|TERM_1"] = ""
	cleanuptbl["PORT_QOS_MAP"] = map[string]interface{}{qosRwIntf: ""}
	cleanuptbl["DSCP_TO_TC_MAP"] = map[string]interface{}{qosRwDscpCls: ""}
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosRwPreloadFwdGroup()
	pre_req_map["ACL_TABLE"] = map[string]interface{}{qosRwPolCls: map[string]interface{}{
		"type":        "QOS_POLICER_V4",
		"stage":       "INGRESS",
		"policy_desc": "OpenConfig QoS classifier " + qosRwPolCls,
		"ports@":      qosRwIntf,
	}}
	pre_req_map["ACL_RULE"] = map[string]interface{}{qosRwPolCls + "|TERM_1": map[string]interface{}{
		"PRIORITY":       "59999",
		"PACKET_ACTION":  "FORWARD",
		"IP_TYPE":        "IPV4ANY",
		"L4_DST_PORT":    "179",
		"POLICER_ACTION": "SP_UT_RW_0",
	}}
	pre_req_map["QOS_UMF_CLASSIFIER"] = map[string]interface{}{
		qosRwPolCls: map[string]interface{}{"type": "ipv4"},
	}
	pre_req_map["DSCP_TO_TC_MAP"] = map[string]interface{}{
		qosRwDscpCls: map[string]interface{}{"0": "0"},
	}
	pre_req_map["PORT_QOS_MAP"] = map[string]interface{}{
		qosRwIntf: map[string]interface{}{"dscp_to_tc_map": qosRwDscpCls},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Unbind the classifier of that type ++++++++++++")
	t.Run("Unbind classifier", processDeleteRequest(
		qosRwIntfClassifiersUrl(qosRwIntf)+"/classifier[type=IPV4]", false))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ The policed classifier stays bound ++++++++++++")
	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosRwPolCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwPolCls,
			"ports@":      qosRwIntf,
		}}}
	t.Run("Verify policer still bound", verifyDbResult(rclient,
		"ACL_TABLE|"+qosRwPolCls, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Rejected configuration
////////////////////////////////////////////////////////////////////////////////

// Test_qos_rewrite_bind_policed_classifier covers naming a policed classifier
// as an interface classifier. Its port list belongs to the scheduler-policy
// that polices it, so this is not the path that binds it.
func Test_qos_rewrite_bind_policed_classifier(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	cleanuptbl["ACL_TABLE"].(map[string]interface{})[qosRwPolCls] = ""
	cleanuptbl["ACL_RULE"].(map[string]interface{})[qosRwPolCls+"|TERM_1"] = ""
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := map[string]interface{}{
		"ACL_TABLE": map[string]interface{}{qosRwPolCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwPolCls,
		}},
		"ACL_RULE": map[string]interface{}{qosRwPolCls + "|TERM_1": map[string]interface{}{
			"PRIORITY":       "59999",
			"PACKET_ACTION":  "FORWARD",
			"IP_TYPE":        "IPV4ANY",
			"L4_DST_PORT":    "179",
			"POLICER_ACTION": "SP_UT_RW_0",
		}},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{
			qosRwPolCls: map[string]interface{}{"type": "ipv4"},
		},
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ A policed classifier is not bound here ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"type":"IPV4",
		"config":{"type":"IPV4","name":"` + qosRwPolCls + `"}}]}`
	expected_err := tlerr.NotSupportedError{
		Format: "Classifier " + qosRwPolCls + " is policed and is bound " +
			"through its scheduler-policy, not as an interface classifier",
	}
	t.Run("Bind policed classifier", processSetRequest(qosRwIntfClassifiersUrl(qosRwIntf),
		url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	// The interface was never added to the table it names.
	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosRwPolCls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosRwPolCls,
		}}}
	t.Run("Verify not bound", verifyDbResult(rclient,
		"ACL_TABLE|"+qosRwPolCls, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_unknown_forwarding_group covers a target-group naming a
// forwarding group that is not configured. The traffic class is only known
// through the group, so there is nothing to write.
func Test_qos_rewrite_unknown_forwarding_group(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ An unknown target-group is rejected ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"transport":{"config":{"destination-port":22}}},
			"actions":{"config":{"target-group":"FG_UT_NO_SUCH"}}
		}]}
	}]}`
	expected_err := tlerr.NotFoundError{Format: "Forwarding group FG_UT_NO_SUCH not found"}
	t.Run("Set classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify no rule written", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_1",
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_set_dscp covers a DSCP classifier whose terms also rewrite
// the DSCP, the policy-map class that matches DSCP 4 and 5 and sets DSCP 4. The
// remark makes the classifier an ACL; a dscp-set becomes one masked DSCP match
// and set-dscp the SET_DSCP action.
func Test_qos_rewrite_set_dscp(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosRwPreloadFwdGroup())
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ DSCP match and set-dscp in one term ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"dscp-set":[4,5]}}},
			"actions":{"config":{"target-group":"` + qosRwFg + `"},
				"remark":{"config":{"set-dscp":4}}}
		},{
			"id":"2",
			"config":{"id":"2"},
			"conditions":{"ipv4":{"config":{"dscp":7}}},
			"actions":{"config":{"target-group":"` + qosRwFg2 + `"},
				"remark":{"config":{"set-dscp":6}}}
		}]}
	}]}`
	t.Run("Set classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosRwCls + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"DSCP":          "4/62",
			"SET_TC":        qosRwFgTc,
			"SET_DSCP":      "4",
		}}}
	t.Run("Verify dscp-set rule", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_1", expected_map, false))

	expected_map = map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosRwCls + "|TERM_2": map[string]interface{}{
			"PRIORITY":      "59998",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"DSCP":          "7",
			"SET_TC":        qosRwFg2Tc,
			"SET_DSCP":      "6",
		}}}
	t.Run("Verify dscp rule", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_2", expected_map, false))

	t.Run("Verify nothing mapped", verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosRwCls,
		map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ Read the term back ++++++++++++")
	termUrl := qosRwClassifierUrl(qosRwCls) + "/terms/term[id=1]"
	t.Run("Get dscp-set", processGetRequest(termUrl+"/conditions/ipv4/config/dscp-set", nil,
		`{"openconfig-qos:dscp-set":[4,5]}`, false))
	t.Run("Get remark", processGetRequest(termUrl+"/actions/remark", nil,
		`{"openconfig-qos:remark":{"config":{"set-dscp":4},"state":{"set-dscp":4}}}`, false))

	t.Log("\n\n+++++++++++++ A dscp-set no value/mask covers is rejected ++++++++++++")
	url_body_json = `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"3",
			"config":{"id":"3"},
			"conditions":{"ipv4":{"config":{"dscp-set":[1,2,3]}}},
			"actions":{"remark":{"config":{"set-dscp":0}}}
		}]}
	}]}`
	expected_err := tlerr.NotSupportedError{
		Format: "Classifier term 3 dscp-set [1 2 3] is not a single DSCP value/mask; split it across terms"}
	t.Run("Set uncoverable set", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)
	t.Run("Verify no rule written", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_3",
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_unsupported_remark covers the remark fields aclorch has no
// ingress action for. Accepting them silently would leave the operator
// believing the packet is rewritten when nothing touches it.
func Test_qos_rewrite_unsupported_remark(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	time.Sleep(1 * time.Second)

	expected_err := tlerr.NotSupportedError{
		Format: "Classifier remark actions set-dot1p and set-mpls-tc are not supported"}

	t.Log("\n\n+++++++++++++ set-mpls-tc is rejected ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"transport":{"config":{"destination-port":22}}},
			"actions":{"remark":{"config":{"set-dscp":32,"set-mpls-tc":4}}}
		}]}
	}]}`
	t.Run("Set set-mpls-tc classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify no set-mpls-tc rule", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_1",
		map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ set-dot1p is rejected ++++++++++++")
	url_body_json = `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"transport":{"config":{"destination-port":22}}},
			"actions":{"remark":{"config":{"set-dot1p":3}}}
		}]}
	}]}`
	t.Run("Set set-dot1p classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify no set-dot1p rule", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_1",
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_rewrite_without_conditions covers a class that acts but selects
// nothing. The rule would match every packet on the port, so the request fails
// rather than programming that. A remark is used to route the term to the ACL
// path; the missing match is caught before the remark is.
func Test_qos_rewrite_without_conditions(t *testing.T) {
	cleanuptbl := qosRwCleanupTbl()
	unloadDB(db.ConfigDB, cleanuptbl)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ An action with no match is rejected ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosRwCls + `",
		"config":{"name":"` + qosRwCls + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"actions":{"remark":{"config":{"set-dscp":32}}}
		}]}
	}]}`
	expected_err := tlerr.NotSupportedError{
		Format: "Classifier term 1 has no condition that maps to an ACL match"}
	t.Run("Set classifier", processSetRequest(qosRwClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify no rule written", verifyDbResult(rclient, "ACL_RULE|"+qosRwCls+"|TERM_1",
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}
