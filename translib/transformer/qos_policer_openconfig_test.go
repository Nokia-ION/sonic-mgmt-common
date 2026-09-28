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

// Tests for QoS traffic classification with policing driven through the
// OpenConfig QoS paths:
//
//	/qos/classifiers/classifier[name]/terms/term[id]/conditions/...
//	/qos/scheduler-policies/scheduler-policy[name]/schedulers/scheduler[seq]/...
//	/qos/interfaces/interface[interface-id]/input/scheduler-policy/config/name
//
// SONiC has no native classifier or ingress policer binding, so a policed
// classifier is synthesized as ACL_TABLE_TYPE, ACL_TABLE and ACL_RULE, the
// policing scheduler becomes a POLICER, and the interface binding is the
// interface appearing in the ACL table's port list. See xfmr_qos_policer.go.

import (
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

const (
	qosPolClsV4  = "CLS_UT_BGP"
	qosPolClsV6  = "CLS_UT_BGP6"
	qosPolClsMix = "CLS_UT_MIX"
	qosPolClsMap = "CLS_UT_DSCP"
	qosPolSp     = "POL_UT"
	qosPolName   = "POL_UT_0"
	qosPolIntf   = "Ethernet0"
	qosPolIntf2  = "Ethernet8"

	qosPolClassifiersUrl = "/openconfig-qos:qos/classifiers"
	qosPolSchedPolicyUrl = "/openconfig-qos:qos/scheduler-policies"

	// The ACTIONS every classifier table type declares, whatever the classifier
	// turns out to use. A table keeps the type it was created with, so an
	// action added to the type later would never reach the tables already
	// standing. Mirrors qosAclTableActions in xfmr_qos_policer.go.
	qosAclActions = "PACKET_ACTION,POLICER_ACTION,SET_TC,COUNTER"
)

// qosPolBindUrl is the ingress scheduler-policy binding of one interface.
func qosPolBindUrl(ifName string) string {
	return "/openconfig-qos:qos/interfaces/interface[interface-id=" + ifName +
		"]/input/scheduler-policy"
}

// qosPolClassifierUrl addresses one classifier list instance.
func qosPolClassifierUrl(name string) string {
	return qosPolClassifiersUrl + "/classifier[name=" + name + "]"
}

// qosPolCleanupTbl lists every table a policed classifier and its policer can
// touch. ACL_TABLE_TYPE is shared by all classifiers of a family so the
// classifier delete deliberately leaves it behind; the tests remove it here.
func qosPolCleanupTbl(cls string) map[string]interface{} {
	return map[string]interface{}{
		"ACL_RULE":           map[string]interface{}{cls + "|TERM_1": "", cls + "|TERM_2": ""},
		"ACL_TABLE":          map[string]interface{}{cls: ""},
		"ACL_TABLE_TYPE":     map[string]interface{}{"QOS_POLICER_V4": "", "QOS_POLICER_V6": ""},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{cls: ""},
		"POLICER":            map[string]interface{}{qosPolName: ""},
		"SCHEDULER":          map[string]interface{}{qosPolSp + "@0": ""},
	}
}

////////////////////////////////////////////////////////////////////////////////
// Classifier to ACL synthesis
////////////////////////////////////////////////////////////////////////////////

func Test_qos_policer_classifier_ipv4(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ Classifier with L3/L4 conditions becomes an ACL ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsV4 + `",
		"config":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{
				"ipv4":{"config":{"protocol":"IP_TCP"}},
				"transport":{"config":{"destination-port":179,"tcp-flags":["TCP_SYN"]}}
			}
		}]}
	}]}`
	t.Run("Set policed classifier", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// The table type has to declare every match the rules use, and must list
	// COUNTER even though nothing asked for it, else aclorch strands the rule.
	expected_map := map[string]interface{}{"ACL_TABLE_TYPE": map[string]interface{}{
		"QOS_POLICER_V4": map[string]interface{}{
			"MATCHES@":     "IP_TYPE,IP_PROTOCOL,L4_DST_PORT,TCP_FLAGS",
			"ACTIONS@":     qosAclActions,
			"BIND_POINTS@": "PORT",
		}}}
	t.Run("Verify ACL_TABLE_TYPE", verifyDbResult(rclient, "ACL_TABLE_TYPE|QOS_POLICER_V4", expected_map, false))

	expected_map = map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosPolClsV4: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosPolClsV4,
		}}}
	t.Run("Verify ACL_TABLE", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, expected_map, false))

	// PRIORITY counts down from 60000 so that a lower term id is matched first.
	// PACKET_ACTION is mandatory and forwards; the policer does the dropping.
	expected_map = map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV4 + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"IP_PROTOCOL":   "6",
			"L4_DST_PORT":   "179",
			"TCP_FLAGS":     "0x02/0x02",
		}}}
	t.Run("Verify ACL_RULE", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", expected_map, false))

	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER": map[string]interface{}{
		qosPolClsV4: map[string]interface{}{"type": "ipv4"}}}
	t.Run("Verify classifier bookkeeping", verifyDbResult(rclient, "QOS_UMF_CLASSIFIER|"+qosPolClsV4, expected_map, false))

	t.Log("\n\n+++++++++++++ Delete removes the synthesized ACL objects ++++++++++++")
	t.Run("Delete policed classifier", processDeleteRequest(qosPolClassifierUrl(qosPolClsV4), false))
	time.Sleep(1 * time.Second)

	empty := map[string]interface{}{}
	t.Run("Verify ACL_RULE gone", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", empty, false))
	t.Run("Verify ACL_TABLE gone", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, empty, false))
	t.Run("Verify bookkeeping gone", verifyDbResult(rclient, "QOS_UMF_CLASSIFIER|"+qosPolClsV4, empty, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

func Test_qos_policer_classifier_ipv6(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV6)
	unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ IPv6 classifier uses the v6 table type ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsV6 + `",
		"config":{"name":"` + qosPolClsV6 + `","type":"IPV6"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv6":{"config":{
				"source-address":"2001:db8:1::1/128",
				"destination-address":"2001:db8:2::2/128",
				"protocol":"IP_UDP"
			}}}
		}]}
	}]}`
	t.Run("Set IPv6 policed classifier", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_TABLE_TYPE": map[string]interface{}{
		"QOS_POLICER_V6": map[string]interface{}{
			"MATCHES@":     "IP_TYPE,SRC_IPV6,DST_IPV6,IP_PROTOCOL",
			"ACTIONS@":     qosAclActions,
			"BIND_POINTS@": "PORT",
		}}}
	t.Run("Verify v6 ACL_TABLE_TYPE", verifyDbResult(rclient, "ACL_TABLE_TYPE|QOS_POLICER_V6", expected_map, false))

	expected_map = map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV6 + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV6ANY",
			"SRC_IPV6":      "2001:db8:1::1/128",
			"DST_IPV6":      "2001:db8:2::2/128",
			"IP_PROTOCOL":   "17",
		}}}
	t.Run("Verify v6 ACL_RULE", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV6+"|TERM_1", expected_map, false))

	expected_map = map[string]interface{}{"QOS_UMF_CLASSIFIER": map[string]interface{}{
		qosPolClsV6: map[string]interface{}{"type": "ipv6"}}}
	t.Run("Verify v6 bookkeeping", verifyDbResult(rclient, "QOS_UMF_CLASSIFIER|"+qosPolClsV6, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

func Test_qos_policer_classifier_port_range_and_dscp(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsMix)
	unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ Port range maps to the _RANGE field ++++++++++++")
	// A port range arrives as the string arm of the port union and maps to the
	// range field; a DSCP value rides along as an ordinary match.
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsMix + `",
		"config":{"name":"` + qosPolClsMix + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"5",
			"config":{"id":"5"},
			"conditions":{
				"ipv4":{"config":{"dscp":46,"source-address":"10.1.1.0/24"}},
				"transport":{"config":{"source-port":5000,"destination-port":"1000..2000"}}
			}
		}]}
	}]}`
	t.Run("Set classifier with range", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsMix + "|TERM_5": map[string]interface{}{
			"PRIORITY":          "59995",
			"PACKET_ACTION":     "FORWARD",
			"IP_TYPE":           "IPV4ANY",
			"SRC_IP":            "10.1.1.0/24",
			"DSCP":              "46",
			"L4_SRC_PORT":       "5000",
			"L4_DST_PORT_RANGE": "1000-2000",
		}}}
	t.Run("Verify range ACL_RULE", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsMix+"|TERM_5", expected_map, false))

	expected_map = map[string]interface{}{"ACL_TABLE_TYPE": map[string]interface{}{
		"QOS_POLICER_V4": map[string]interface{}{
			"MATCHES@":     "IP_TYPE,SRC_IP,DSCP,L4_SRC_PORT,L4_DST_PORT_RANGE",
			"ACTIONS@":     qosAclActions,
			"BIND_POINTS@": "PORT",
		}}}
	t.Run("Verify range ACL_TABLE_TYPE", verifyDbResult(rclient, "ACL_TABLE_TYPE|QOS_POLICER_V4", expected_map, false))

	unloadDB(db.ConfigDB, map[string]interface{}{
		"ACL_RULE":           map[string]interface{}{qosPolClsMix + "|TERM_5": ""},
		"ACL_TABLE":          map[string]interface{}{qosPolClsMix: ""},
		"ACL_TABLE_TYPE":     map[string]interface{}{"QOS_POLICER_V4": ""},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{qosPolClsMix: ""},
	})
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_dscp_classifier_untouched guards the pre-existing behaviour:
// a classifier whose terms carry nothing but a DSCP value is a DSCP-to-traffic
// class map and must not be diverted into the ACL path.
func Test_qos_policer_dscp_classifier_untouched(t *testing.T) {
	pre_req_map := map[string]interface{}{
		"QOS_FWD_GROUP": map[string]interface{}{"AF1": map[string]interface{}{"priority": "1"}},
	}
	cleanuptbl := map[string]interface{}{
		"QOS_FWD_GROUP":           map[string]interface{}{"AF1": ""},
		"DSCP_TO_TC_MAP":          map[string]interface{}{qosPolClsMap: ""},
		"QOS_UMF_CLASSIFIER":      map[string]interface{}{qosPolClsMap: ""},
		"ACL_TABLE":               map[string]interface{}{qosPolClsMap: ""},
		"QOS_UMF_CLASSIFIER_TERM": map[string]interface{}{qosPolClsMap + "|1": ""},
	}
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ DSCP-only classifier stays a DSCP_TO_TC_MAP ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsMap + `",
		"config":{"name":"` + qosPolClsMap + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"dscp":10}}},
			"actions":{"config":{"target-group":"AF1"}}
		}]}
	}]}`
	t.Run("Set DSCP classifier", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"DSCP_TO_TC_MAP": map[string]interface{}{
		qosPolClsMap: map[string]interface{}{"10": "1"}}}
	t.Run("Verify DSCP_TO_TC_MAP written", verifyDbResult(rclient, "DSCP_TO_TC_MAP|"+qosPolClsMap, expected_map, false))

	t.Run("Verify no ACL table synthesized", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsMap,
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Scheduler policy to POLICER
////////////////////////////////////////////////////////////////////////////////

// qosPolPreloadClassifier puts a policed classifier straight into CONFIG_DB so
// that a scheduler-policy or binding test does not depend on another test
// having run first. A scheduler is only recognised as an ingress policer when
// the classifier its input names already has a backing ACL table.
func qosPolPreloadClassifier(cls string) map[string]interface{} {
	return map[string]interface{}{
		"ACL_TABLE_TYPE": map[string]interface{}{"QOS_POLICER_V4": map[string]interface{}{
			"MATCHES@":     "IP_TYPE,IP_PROTOCOL,L4_DST_PORT",
			"ACTIONS@":     qosAclActions,
			"BIND_POINTS@": "PORT",
		}},
		"ACL_TABLE": map[string]interface{}{cls: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + cls,
		}},
		"ACL_RULE": map[string]interface{}{cls + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"IP_PROTOCOL":   "6",
			"L4_DST_PORT":   "179",
		}},
		"QOS_UMF_CLASSIFIER": map[string]interface{}{cls: map[string]interface{}{"type": "ipv4"}},
	}
}

func Test_qos_policer_one_rate_two_color(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadClassifier(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ one-rate-two-color becomes a single rate POLICER ++++++++++++")
	url_body_json := `{"openconfig-qos:scheduler-policy":[{
		"name":"` + qosPolSp + `",
		"config":{"name":"` + qosPolSp + `"},
		"schedulers":{"scheduler":[{
			"sequence":0,
			"config":{"sequence":0,"type":"ONE_RATE_TWO_COLOR"},
			"inputs":{"input":[{"id":"` + qosPolClsV4 + `",
				"config":{"id":"` + qosPolClsV4 + `","input-type":"IN_PROFILE"}}]},
			"one-rate-two-color":{
				"config":{"cir":"512000","bc":8192},
				"exceed-action":{"config":{"drop":true}}
			}
		}]}
	}]}`
	t.Run("Set policing scheduler-policy", processSetRequest(qosPolSchedPolicyUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// OpenConfig states cir in bits per second while a byte mode SONiC policer
	// counts bytes per second, so 512000 bps becomes 64000 Bps.
	expected_map := map[string]interface{}{"POLICER": map[string]interface{}{
		qosPolName: map[string]interface{}{
			"meter_type":          "bytes",
			"color":               "blind",
			"mode":                "sr_tcm",
			"cir":                 "64000",
			"cbs":                 "8192",
			"green_packet_action": "forward",
			"red_packet_action":   "drop",
		}}}
	t.Run("Verify POLICER", verifyDbResult(rclient, "POLICER|"+qosPolName, expected_map, false))

	// The rule pointing at the policer is the only record of which classifier
	// the policy polices, which the binding and the read path both rely on.
	expected_map = map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV4 + "|TERM_1": map[string]interface{}{
			"PRIORITY":       "59999",
			"PACKET_ACTION":  "FORWARD",
			"IP_TYPE":        "IPV4ANY",
			"IP_PROTOCOL":    "6",
			"L4_DST_PORT":    "179",
			"POLICER_ACTION": qosPolName,
		}}}
	t.Run("Verify rule stitched to policer", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", expected_map, false))

	// An ingress policer must not leak into the egress SCHEDULER table.
	t.Run("Verify not in SCHEDULER", verifyDbResult(rclient, "SCHEDULER|"+qosPolSp+"@0",
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

func Test_qos_policer_two_rate_three_color(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadClassifier(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ two-rate-three-color becomes a two rate POLICER ++++++++++++")
	url_body_json := `{"openconfig-qos:scheduler-policy":[{
		"name":"` + qosPolSp + `",
		"config":{"name":"` + qosPolSp + `"},
		"schedulers":{"scheduler":[{
			"sequence":0,
			"config":{"sequence":0,"type":"TWO_RATE_THREE_COLOR"},
			"inputs":{"input":[{"id":"` + qosPolClsV4 + `",
				"config":{"id":"` + qosPolClsV4 + `","input-type":"IN_PROFILE"}}]},
			"two-rate-three-color":{
				"config":{"cir":"512000","pir":"1024000","bc":8192,"be":16384},
				"exceed-action":{"config":{"drop":true}}
			}
		}]}
	}]}`
	t.Run("Set two rate scheduler-policy", processSetRequest(qosPolSchedPolicyUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// Exceeding the committed rate is yellow, exceeding the peak rate is red.
	expected_map := map[string]interface{}{"POLICER": map[string]interface{}{
		qosPolName: map[string]interface{}{
			"meter_type":           "bytes",
			"color":                "blind",
			"mode":                 "tr_tcm",
			"cir":                  "64000",
			"pir":                  "128000",
			"cbs":                  "8192",
			"pbs":                  "16384",
			"green_packet_action":  "forward",
			"yellow_packet_action": "drop",
			"red_packet_action":    "drop",
		}}}
	t.Run("Verify two rate POLICER", verifyDbResult(rclient, "POLICER|"+qosPolName, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Interface binding
////////////////////////////////////////////////////////////////////////////////

// qosPolPreloadPolicer adds the POLICER and the rule to policer stitching on top
// of a preloaded classifier, which is the state an interface binding needs.
func qosPolPreloadPolicer(cls string) map[string]interface{} {
	m := qosPolPreloadClassifier(cls)
	m["ACL_RULE"].(map[string]interface{})[cls+"|TERM_1"].(map[string]interface{})["POLICER_ACTION"] = qosPolName
	m["POLICER"] = map[string]interface{}{qosPolName: map[string]interface{}{
		"meter_type":          "bytes",
		"color":               "blind",
		"mode":                "sr_tcm",
		"cir":                 "64000",
		"cbs":                 "8192",
		"green_packet_action": "forward",
		"red_packet_action":   "drop",
	}}
	return m
}

func Test_qos_policer_interface_binding(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadPolicer(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Binding adds the interface to the ACL table ports ++++++++++++")
	url_body_json := `{"config":{"name":"` + qosPolSp + `"}}`
	t.Run("Bind policy on ingress", processSetRequest(qosPolBindUrl(qosPolIntf), url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosPolClsV4: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosPolClsV4,
			"ports@":      qosPolIntf,
		}}}
	t.Run("Verify port bound", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, expected_map, false))

	t.Log("\n\n+++++++++++++ Read the binding back ++++++++++++")
	expected_get_json := `{"openconfig-qos:scheduler-policy":{"config":{"name":"` + qosPolSp +
		`"},"state":{"name":"` + qosPolSp + `"}}}`
	t.Run("Get binding", processGetRequest(qosPolBindUrl(qosPolIntf), nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Unbinding drops the interface again ++++++++++++")
	t.Run("Unbind policy", processDeleteRequest(qosPolBindUrl(qosPolIntf), false))
	time.Sleep(1 * time.Second)

	// Whether the port list ends up empty or absent, the binding must no longer
	// resolve, so assert through the read path rather than on the raw field.
	t.Run("Verify binding gone", processGetRequest(qosPolBindUrl(qosPolIntf), nil, emptyJson, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Read path
////////////////////////////////////////////////////////////////////////////////

// Test_qos_policer_get_classifier covers rebuilding the OpenConfig view of a
// policed classifier from the ACL rules backing it. A policed classifier has no
// DSCP_TO_TC_MAP entry, so without this the read path reports it missing, which
// also makes translib refuse to delete it.
func Test_qos_policer_get_classifier(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosPolPreloadClassifier(qosPolClsV4)
	// Give the rule the full condition set so the reverse mapping is exercised.
	pre_req_map["ACL_RULE"].(map[string]interface{})[qosPolClsV4+"|TERM_1"].(map[string]interface{})["TCP_FLAGS"] = "0x02/0x02"
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Get a policed classifier by name ++++++++++++")
	expected_get_json := `{"openconfig-qos:classifier":[{
		"config":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"name":"` + qosPolClsV4 + `",
		"state":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"terms":{"term":[{
			"conditions":{
				"ipv4":{"config":{"protocol":6}},
				"transport":{"config":{"destination-port":179,
					"tcp-flags":["openconfig-packet-match-types:TCP_SYN"]}}
			},
			"config":{"id":"1"},
			"id":"1",
			"state":{"id":"1"}
		}]}
	}]}`
	t.Run("Get classifier", processGetRequest(qosPolClassifierUrl(qosPolClsV4), nil, expected_get_json, false))

	// A request below the classifier hands the transformer a partially built
	// tree, so these guard against dereferencing a container that is not there.
	t.Log("\n\n+++++++++++++ Get nodes below the classifier ++++++++++++")
	expected_get_json = `{"openconfig-qos:conditions":{
		"ipv4":{"config":{"protocol":6}},
		"transport":{"config":{"destination-port":179,
			"tcp-flags":["openconfig-packet-match-types:TCP_SYN"]}}}}`
	t.Run("Get term conditions", processGetRequest(
		qosPolClassifierUrl(qosPolClsV4)+"/terms/term[id=1]/conditions", nil, expected_get_json, false))

	expected_get_json = `{"openconfig-qos:config":{"destination-port":179,
		"tcp-flags":["openconfig-packet-match-types:TCP_SYN"]}}`
	t.Run("Get transport config", processGetRequest(
		qosPolClassifierUrl(qosPolClsV4)+"/terms/term[id=1]/conditions/transport/config",
		nil, expected_get_json, false))

	expected_get_json = `{"openconfig-qos:protocol":6}`
	t.Run("Get protocol leaf", processGetRequest(
		qosPolClassifierUrl(qosPolClsV4)+"/terms/term[id=1]/conditions/ipv4/config/protocol",
		nil, expected_get_json, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_get_scheduler_policy covers rebuilding the scheduler policy
// view of a policing policy from the POLICER entry realizing it. A policing
// policy has no SCHEDULER entry, so without this the read path reports it
// missing, which also makes translib refuse to delete it.
func Test_qos_policer_get_scheduler_policy(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadPolicer(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Get a policing scheduler-policy by name ++++++++++++")
	// The policer counts bytes per second, so 64000 Bps reads back as the
	// 512000 bits per second OpenConfig states. The input names the classifier
	// the policer meters, recovered from the rule pointing at it.
	expected_get_json := `{"openconfig-qos:scheduler-policy":[{
		"config":{"name":"` + qosPolSp + `"},
		"name":"` + qosPolSp + `",
		"state":{"name":"` + qosPolSp + `"},
		"schedulers":{"scheduler":[{
			"sequence":0,
			"config":{"sequence":0,"type":"openconfig-qos-types:ONE_RATE_TWO_COLOR"},
			"state":{"sequence":0,"type":"openconfig-qos-types:ONE_RATE_TWO_COLOR"},
			"inputs":{"input":[{
				"id":"` + qosPolClsV4 + `",
				"config":{"id":"` + qosPolClsV4 + `","input-type":"IN_PROFILE"},
				"state":{"id":"` + qosPolClsV4 + `","input-type":"IN_PROFILE"}
			}]},
			"one-rate-two-color":{
				"config":{"bc":8192,"cir":"512000"},
				"state":{"bc":8192,"cir":"512000"},
				"exceed-action":{"config":{"drop":true}}
			}
		}]}
	}]}`
	t.Run("Get scheduler policy", processGetRequest(
		qosPolSchedPolicyUrl+"/scheduler-policy[name="+qosPolSp+"]", nil, expected_get_json, false))

	t.Log("\n\n+++++++++++++ Get nodes below the policy ++++++++++++")
	expected_get_json = `{"openconfig-qos:config":{"bc":8192,"cir":"512000"}}`
	t.Run("Get one-rate config", processGetRequest(
		qosPolSchedPolicyUrl+"/scheduler-policy["+"name="+qosPolSp+"]/schedulers/scheduler[sequence=0]"+
			"/one-rate-two-color/config", nil, expected_get_json, false))

	expected_get_json = `{"openconfig-qos:cir":"512000"}`
	t.Run("Get cir leaf", processGetRequest(
		qosPolSchedPolicyUrl+"/scheduler-policy["+"name="+qosPolSp+"]/schedulers/scheduler[sequence=0]"+
			"/one-rate-two-color/config/cir", nil, expected_get_json, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_queue_scheduler_untouched guards the pre-existing queue
// scheduler path: a policy held in SCHEDULER must still read back, since the
// policing support shares the subscribe, read and delete entry points with it.
func Test_qos_policer_queue_scheduler_untouched(t *testing.T) {
	cleanuptbl := map[string]interface{}{
		"SCHEDULER": map[string]interface{}{"POL_DWRR_UT@0": ""},
	}
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, map[string]interface{}{
		"SCHEDULER": map[string]interface{}{"POL_DWRR_UT@0": map[string]interface{}{
			"type":   "DWRR",
			"weight": "10",
		}},
	})
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ A queue scheduler policy still reads back ++++++++++++")
	expected_get_json := `{"openconfig-qos:scheduler-policy":[{
		"config":{"name":"POL_DWRR_UT"},
		"name":"POL_DWRR_UT",
		"state":{"name":"POL_DWRR_UT"},
		"schedulers":{"scheduler":[{
			"sequence":0,
			"config":{"sequence":0},
			"state":{"sequence":0}
		}]}
	}]}`
	t.Run("Get queue scheduler policy", processGetRequest(
		qosPolSchedPolicyUrl+"/scheduler-policy[name=POL_DWRR_UT]", nil, expected_get_json, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_ignores_foreign_policer guards the listing path against the
// POLICER table being shared: control-plane policing puts entries there that no
// classifier points at, and one named "<word>_<digits>" would otherwise be
// mistaken for a scheduler policy.
func Test_qos_policer_ignores_foreign_policer(t *testing.T) {
	cleanuptbl := map[string]interface{}{
		"POLICER": map[string]interface{}{"copp_1": ""},
	}
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, map[string]interface{}{
		"POLICER": map[string]interface{}{"copp_1": map[string]interface{}{
			"meter_type":        "packets",
			"mode":              "sr_tcm",
			"cir":               "600",
			"cbs":               "600",
			"red_packet_action": "drop",
		}},
	})
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ A policer no classifier points at is not a policy ++++++++++++")
	t.Run("List scheduler policies", processGetRequest(qosPolSchedPolicyUrl, nil, emptyJson, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_delete_scheduler_policy checks that deleting a policing
// policy removes the policer and detaches it from the classifier's rules,
// leaving the classifier itself intact.
func Test_qos_policer_delete_scheduler_policy(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadPolicer(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Delete a policing scheduler-policy ++++++++++++")
	t.Run("Delete scheduler policy", processDeleteRequest(
		qosPolSchedPolicyUrl+"/scheduler-policy[name="+qosPolSp+"]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify POLICER gone", verifyDbResult(rclient, "POLICER|"+qosPolName,
		map[string]interface{}{}, false))

	// The classifier outlives the policy, minus the policer binding.
	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV4 + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"IP_PROTOCOL":   "6",
			"L4_DST_PORT":   "179",
		}}}
	t.Run("Verify rule kept without policer", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", expected_map, false))

	expected_map = map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosPolClsV4: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosPolClsV4,
		}}}
	t.Run("Verify classifier kept", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_two_rate_read covers the three-colour read path, where the
// committed rate maps to yellow and the peak rate to red.
func Test_qos_policer_two_rate_read(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosPolPreloadPolicer(qosPolClsV4)
	pre_req_map["POLICER"] = map[string]interface{}{qosPolName: map[string]interface{}{
		"meter_type":           "bytes",
		"color":                "blind",
		"mode":                 "tr_tcm",
		"cir":                  "64000",
		"pir":                  "128000",
		"cbs":                  "8192",
		"pbs":                  "16384",
		"green_packet_action":  "forward",
		"yellow_packet_action": "drop",
		"red_packet_action":    "drop",
	}}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Get a two rate policing policy ++++++++++++")
	expected_get_json := `{"openconfig-qos:two-rate-three-color":{
		"config":{"bc":8192,"be":16384,"cir":"512000","pir":"1024000"},
		"state":{"bc":8192,"be":16384,"cir":"512000","pir":"1024000"},
		"exceed-action":{"config":{"drop":true}}}}`
	t.Run("Get two-rate-three-color", processGetRequest(
		qosPolSchedPolicyUrl+"/scheduler-policy["+"name="+qosPolSp+"]/schedulers/scheduler[sequence=0]"+
			"/two-rate-three-color", nil, expected_get_json, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_delete_with_policer checks that deleting the classifier also
// takes the policer with it, so a delete does not strand a POLICER entry.
func Test_qos_policer_delete_with_policer(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadPolicer(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Deleting the classifier removes its policer ++++++++++++")
	t.Run("Delete classifier", processDeleteRequest(qosPolClassifierUrl(qosPolClsV4), false))
	time.Sleep(1 * time.Second)

	empty := map[string]interface{}{}
	t.Run("Verify ACL_RULE gone", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", empty, false))
	t.Run("Verify ACL_TABLE gone", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, empty, false))
	t.Run("Verify POLICER gone", verifyDbResult(rclient, "POLICER|"+qosPolName, empty, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Rejected configuration
////////////////////////////////////////////////////////////////////////////////

func Test_qos_policer_unsupported_remarking(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadClassifier(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Remarking on exceed is rejected ++++++++++++")
	// The hardware policer can drop what exceeds the rate but cannot remark it.
	url_body_json := `{"openconfig-qos:scheduler-policy":[{
		"name":"` + qosPolSp + `",
		"config":{"name":"` + qosPolSp + `"},
		"schedulers":{"scheduler":[{
			"sequence":0,
			"config":{"sequence":0,"type":"ONE_RATE_TWO_COLOR"},
			"inputs":{"input":[{"id":"` + qosPolClsV4 + `",
				"config":{"id":"` + qosPolClsV4 + `","input-type":"IN_PROFILE"}}]},
			"one-rate-two-color":{
				"config":{"cir":"512000","bc":8192},
				"exceed-action":{"config":{"set-dscp":8}}
			}
		}]}
	}]}`
	expected_err := tlerr.NotSupportedError{
		Format: "Policer exceed-action remarking is not supported; only drop is"}
	t.Run("Set remarking policer", processSetRequest(qosPolSchedPolicyUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify no POLICER written", verifyDbResult(rclient, "POLICER|"+qosPolName,
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

func Test_qos_policer_missing_cir(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadClassifier(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ A policer without a rate is rejected ++++++++++++")
	url_body_json := `{"openconfig-qos:scheduler-policy":[{
		"name":"` + qosPolSp + `",
		"config":{"name":"` + qosPolSp + `"},
		"schedulers":{"scheduler":[{
			"sequence":0,
			"config":{"sequence":0,"type":"ONE_RATE_TWO_COLOR"},
			"inputs":{"input":[{"id":"` + qosPolClsV4 + `",
				"config":{"id":"` + qosPolClsV4 + `","input-type":"IN_PROFILE"}}]},
			"one-rate-two-color":{"config":{"bc":8192}}
		}]}
	}]}`
	expected_err := tlerr.InvalidArgsError{
		Format: "Policing scheduler " + qosPolSp + " needs a cir"}
	t.Run("Set policer without cir", processSetRequest(qosPolSchedPolicyUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify no POLICER written", verifyDbResult(rclient, "POLICER|"+qosPolName,
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

func Test_qos_policer_bind_unknown_policy(t *testing.T) {
	t.Log("\n\n+++++++++++++ Binding a policy that polices nothing is rejected ++++++++++++")
	// Without a policed classifier behind it there is no ACL table to put the
	// interface into, so the binding has nowhere to land.
	url_body_json := `{"config":{"name":"POL_UT_NOSUCH"}}`
	expected_err := tlerr.NotFoundError{
		Format: "Scheduler policy POL_UT_NOSUCH does not police any classifier"}
	t.Run("Bind unknown policy", processSetRequest(qosPolBindUrl(qosPolIntf), url_body_json, "PATCH", true, expected_err))
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

func Test_qos_policer_bad_term_id(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ A term id must be numeric to become a priority ++++++++++++")
	// The term id is turned into an ACL rule priority, so it has to be a number
	// below the priority base.
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsV4 + `",
		"config":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"bgp",
			"config":{"id":"bgp"},
			"conditions":{"transport":{"config":{"destination-port":179}}}
		}]}
	}]}`
	expected_err := tlerr.InvalidArgsError{
		Format: "Classifier term id must be numeric for a policed classifier, got bgp"}
	t.Run("Set non numeric term id", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify no ACL table written", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4,
		map[string]interface{}{}, false))

	t.Log("\n\n+++++++++++++ A term id at or above the priority base is rejected ++++++++++++")
	url_body_json = `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsV4 + `",
		"config":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"60000",
			"config":{"id":"60000"},
			"conditions":{"transport":{"config":{"destination-port":179}}}
		}]}
	}]}`
	expected_err = tlerr.InvalidArgsError{
		Format: "Classifier term id must be below 60000, got 60000"}
	t.Run("Set out of range term id", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	t.Run("Verify still no ACL table", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4,
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_hop_limit_rejected covers a condition that makes a term
// policing but has no ACL_RULE equivalent. Ignoring it would synthesize a rule
// matching every packet on the port, so the whole link would be policed.
func Test_qos_policer_hop_limit_rejected(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ A hop-limit condition is refused ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsV4 + `",
		"config":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"hop-limit":5}}}
		}]}
	}]}`
	expected_err := tlerr.NotSupportedError{
		Format: "Classifier term 1 hop-limit condition is not supported"}
	t.Run("Set hop-limit condition", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", true, expected_err))
	time.Sleep(1 * time.Second)

	// Nothing may be programmed, least of all a rule that matches everything.
	t.Run("Verify no ACL table", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4,
		map[string]interface{}{}, false))
	t.Run("Verify no ACL rule", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1",
		map[string]interface{}{}, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_ethertype covers the L2 ethertype condition in both
// directions: a named type on the way in, and a raw value on the way back out.
func Test_qos_policer_ethertype(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ A named ethertype becomes an ACL match ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsV4 + `",
		"config":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"l2":{"config":{"ethertype":"ETHERTYPE_ARP"}}}
		}]}
	}]}`
	t.Run("Set ethertype condition", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// sonic-acl.yang accepts only an uppercase hex ethertype, so the named type
	// has to be resolved to its value rather than written as a name.
	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV4 + "|TERM_1": map[string]interface{}{
			"PRIORITY":      "59999",
			"PACKET_ACTION": "FORWARD",
			"IP_TYPE":       "IPV4ANY",
			"ETHER_TYPE":    "0x0806",
		}}}
	t.Run("Verify ETHER_TYPE", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)

	t.Log("\n\n+++++++++++++ A raw ethertype reads back ++++++++++++")
	// aclorch also accepts a hex ethertype, which comes back as the numeric arm
	// of the OpenConfig union rather than a named type.
	pre_req_map := qosPolPreloadClassifier(qosPolClsV4)
	pre_req_map["ACL_RULE"] = map[string]interface{}{qosPolClsV4 + "|TERM_1": map[string]interface{}{
		"PRIORITY":      "59999",
		"PACKET_ACTION": "FORWARD",
		"IP_TYPE":       "IPV4ANY",
		"ETHER_TYPE":    "0x0806",
	}}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	expected_get_json := `{"openconfig-qos:config":{"ethertype":2054}}`
	t.Run("Get raw ethertype", processGetRequest(qosPolClassifierUrl(qosPolClsV4)+
		"/terms/term[id=1]/conditions/l2/config", nil, expected_get_json, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Delete scope
////////////////////////////////////////////////////////////////////////////////

// Test_qos_policer_term_delete_keeps_classifier checks that removing one term
// removes only that term's rule. One term is one rule, so a term delete must
// not take the ACL table, the other rules or the policer with it.
func Test_qos_policer_term_delete_keeps_classifier(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosPolPreloadPolicer(qosPolClsV4)
	pre_req_map["ACL_RULE"].(map[string]interface{})[qosPolClsV4+"|TERM_2"] = map[string]interface{}{
		"PRIORITY":       "59998",
		"PACKET_ACTION":  "FORWARD",
		"IP_TYPE":        "IPV4ANY",
		"L4_DST_PORT":    "22",
		"POLICER_ACTION": qosPolName,
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Delete one term of a policed classifier ++++++++++++")
	t.Run("Delete term 2", processDeleteRequest(qosPolClassifierUrl(qosPolClsV4)+"/terms/term[id=2]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify term 2 rule gone", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_2",
		map[string]interface{}{}, false))

	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV4 + "|TERM_1": map[string]interface{}{
			"PRIORITY":       "59999",
			"PACKET_ACTION":  "FORWARD",
			"IP_TYPE":        "IPV4ANY",
			"IP_PROTOCOL":    "6",
			"L4_DST_PORT":    "179",
			"POLICER_ACTION": qosPolName,
		}}}
	t.Run("Verify term 1 rule kept", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", expected_map, false))

	expected_map = map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosPolClsV4: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosPolClsV4,
		}}}
	t.Run("Verify classifier kept", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, expected_map, false))

	expected_map = map[string]interface{}{"POLICER": map[string]interface{}{
		qosPolName: map[string]interface{}{
			"meter_type":          "bytes",
			"color":               "blind",
			"mode":                "sr_tcm",
			"cir":                 "64000",
			"cbs":                 "8192",
			"green_packet_action": "forward",
			"red_packet_action":   "drop",
		}}}
	t.Run("Verify policer kept", verifyDbResult(rclient, "POLICER|"+qosPolName, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_delete_below_term_rejected checks that a delete of a single
// condition is refused rather than silently widening into the whole rule.
func Test_qos_policer_delete_below_term_rejected(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)
	loadDB(db.ConfigDB, qosPolPreloadPolicer(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Delete of a condition below a term is refused ++++++++++++")
	expected_err := tlerr.NotSupportedError{
		Format: "Delete below a term of policed classifier " + qosPolClsV4 +
			" is not supported; delete the term instead"}
	t.Run("Delete conditions", processDeleteRequest(qosPolClassifierUrl(qosPolClsV4)+
		"/terms/term[id=1]/conditions", true, expected_err))
	time.Sleep(1 * time.Second)

	// The rule has to be exactly as it was.
	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV4 + "|TERM_1": map[string]interface{}{
			"PRIORITY":       "59999",
			"PACKET_ACTION":  "FORWARD",
			"IP_TYPE":        "IPV4ANY",
			"IP_PROTOCOL":    "6",
			"L4_DST_PORT":    "179",
			"POLICER_ACTION": qosPolName,
		}}}
	t.Run("Verify rule untouched", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Objects belonging to other features
////////////////////////////////////////////////////////////////////////////////

// Test_qos_policer_foreign_acl_untouched checks that an ACL the operator
// configured through /acl is not mistaken for a synthesized classifier just
// because it shares the name. Claiming it would let a classifier delete tear
// down their table and rules.
func Test_qos_policer_foreign_acl_untouched(t *testing.T) {
	const foreign = "CLS_UT_BGP"
	cleanuptbl := map[string]interface{}{
		"ACL_TABLE": map[string]interface{}{foreign: ""},
		"ACL_RULE":  map[string]interface{}{foreign + "|RULE_1": ""},
	}
	unloadDB(db.ConfigDB, cleanuptbl)

	// A user ACL: an L3 table type, not one of the QoS policer types.
	loadDB(db.ConfigDB, map[string]interface{}{
		"ACL_TABLE": map[string]interface{}{foreign: map[string]interface{}{
			"type":        "L3",
			"stage":       "INGRESS",
			"policy_desc": "operator ACL",
		}},
		"ACL_RULE": map[string]interface{}{foreign + "|RULE_1": map[string]interface{}{
			"PRIORITY":      "999",
			"PACKET_ACTION": "DROP",
			"SRC_IP":        "10.1.1.1/32",
		}},
	})
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Deleting a classifier of the same name spares the ACL ++++++++++++")
	// The classifier does not exist, so the request itself may well fail. What
	// matters is that it does not reach into the operator's ACL.
	t.Run("Delete same named classifier", processDeleteRequest(qosPolClassifierUrl(foreign), true))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		foreign: map[string]interface{}{
			"type":        "L3",
			"stage":       "INGRESS",
			"policy_desc": "operator ACL",
		}}}
	t.Run("Verify ACL table kept", verifyDbResult(rclient, "ACL_TABLE|"+foreign, expected_map, false))

	expected_map = map[string]interface{}{"ACL_RULE": map[string]interface{}{
		foreign + "|RULE_1": map[string]interface{}{
			"PRIORITY":      "999",
			"PACKET_ACTION": "DROP",
			"SRC_IP":        "10.1.1.1/32",
		}}}
	t.Run("Verify ACL rule kept", verifyDbResult(rclient, "ACL_RULE|"+foreign+"|RULE_1", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_policy_delete_keeps_foreign_policer checks that deleting a
// policing policy only removes the policers a classifier points at. The POLICER
// table is shared with control-plane policing, so matching the
// "<policy>_<sequence>" name is not enough to claim one.
func Test_qos_policer_policy_delete_keeps_foreign_policer(t *testing.T) {
	const foreignPolicer = qosPolSp + "_9"
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	cleanuptbl["POLICER"].(map[string]interface{})[foreignPolicer] = ""
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosPolPreloadPolicer(qosPolClsV4)
	// Same name shape, but no classifier rule references it.
	pre_req_map["POLICER"].(map[string]interface{})[foreignPolicer] = map[string]interface{}{
		"meter_type":        "packets",
		"mode":              "sr_tcm",
		"cir":               "600",
		"cbs":               "600",
		"red_packet_action": "drop",
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Policy delete spares a policer that is not ours ++++++++++++")
	t.Run("Delete scheduler policy", processDeleteRequest(
		qosPolSchedPolicyUrl+"/scheduler-policy[name="+qosPolSp+"]", false))
	time.Sleep(1 * time.Second)

	t.Run("Verify our policer gone", verifyDbResult(rclient, "POLICER|"+qosPolName,
		map[string]interface{}{}, false))

	expected_map := map[string]interface{}{"POLICER": map[string]interface{}{
		foreignPolicer: map[string]interface{}{
			"meter_type":        "packets",
			"mode":              "sr_tcm",
			"cir":               "600",
			"cbs":               "600",
			"red_packet_action": "drop",
		}}}
	t.Run("Verify foreign policer kept", verifyDbResult(rclient, "POLICER|"+foreignPolicer, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_table_type_matches_merged checks that a second classifier of
// the same family adds to the shared table type rather than replacing it.
// aclorch builds its SAI match field list from this leaf-list, so dropping a
// match another classifier's rule uses would break that rule.
func Test_qos_policer_table_type_matches_merged(t *testing.T) {
	const second = "CLS_UT_SECOND"
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	cleanuptbl["ACL_TABLE"].(map[string]interface{})[second] = ""
	cleanuptbl["ACL_RULE"].(map[string]interface{})[second+"|TERM_1"] = ""
	cleanuptbl["QOS_UMF_CLASSIFIER"].(map[string]interface{})[second] = ""
	unloadDB(db.ConfigDB, cleanuptbl)

	// The first classifier already declares IP_PROTOCOL and L4_DST_PORT.
	loadDB(db.ConfigDB, qosPolPreloadClassifier(qosPolClsV4))
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ A second classifier folds into the shared type ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + second + `",
		"config":{"name":"` + second + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"source-address":"10.2.2.0/24"}}}
		}]}
	}]}`
	t.Run("Set second classifier", processSetRequest(qosPolClassifiersUrl, url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	// Translib merges a leaf-list on update, so the matches already declared
	// keep their place and the new one lands at the end.
	expected_map := map[string]interface{}{"ACL_TABLE_TYPE": map[string]interface{}{
		"QOS_POLICER_V4": map[string]interface{}{
			"MATCHES@":     "IP_TYPE,IP_PROTOCOL,L4_DST_PORT,SRC_IP",
			"ACTIONS@":     qosAclActions,
			"BIND_POINTS@": "PORT",
		}}}
	t.Run("Verify MATCHES merged", verifyDbResult(rclient, "ACL_TABLE_TYPE|QOS_POLICER_V4", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

////////////////////////////////////////////////////////////////////////////////
// Rebinding and replace
////////////////////////////////////////////////////////////////////////////////

// Test_qos_policer_rebind_moves_interface checks that binding a second policy
// on an interface unbinds the first. The leaf holds one policy, so leaving the
// interface in both port lists would meter its traffic twice.
func Test_qos_policer_rebind_moves_interface(t *testing.T) {
	const (
		clsB  = "CLS_UT_SECOND"
		spB   = "POL_UT2"
		polB  = "POL_UT2_0"
		term1 = "|TERM_1"
	)
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	cleanuptbl["ACL_TABLE"].(map[string]interface{})[clsB] = ""
	cleanuptbl["ACL_RULE"].(map[string]interface{})[clsB+term1] = ""
	cleanuptbl["QOS_UMF_CLASSIFIER"].(map[string]interface{})[clsB] = ""
	cleanuptbl["POLICER"].(map[string]interface{})[polB] = ""
	unloadDB(db.ConfigDB, cleanuptbl)

	// Two classifiers, each policed by its own policy, and the interface bound
	// to the first.
	pre_req_map := qosPolPreloadPolicer(qosPolClsV4)
	pre_req_map["ACL_TABLE"].(map[string]interface{})[qosPolClsV4].(map[string]interface{})["ports@"] = qosPolIntf
	pre_req_map["ACL_TABLE"].(map[string]interface{})[clsB] = map[string]interface{}{
		"type":        "QOS_POLICER_V4",
		"stage":       "INGRESS",
		"policy_desc": "OpenConfig QoS classifier " + clsB,
	}
	pre_req_map["ACL_RULE"].(map[string]interface{})[clsB+term1] = map[string]interface{}{
		"PRIORITY":       "59999",
		"PACKET_ACTION":  "FORWARD",
		"IP_TYPE":        "IPV4ANY",
		"L4_DST_PORT":    "22",
		"POLICER_ACTION": polB,
	}
	pre_req_map["QOS_UMF_CLASSIFIER"].(map[string]interface{})[clsB] = map[string]interface{}{"type": "ipv4"}
	pre_req_map["POLICER"].(map[string]interface{})[polB] = map[string]interface{}{
		"meter_type":          "bytes",
		"color":               "blind",
		"mode":                "sr_tcm",
		"cir":                 "32000",
		"cbs":                 "4096",
		"green_packet_action": "forward",
		"red_packet_action":   "drop",
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Rebinding moves the interface between policies ++++++++++++")
	url_body_json := `{"config":{"name":"` + spB + `"}}`
	t.Run("Bind second policy", processSetRequest(qosPolBindUrl(qosPolIntf), url_body_json, "PATCH", false, nil))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		clsB: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + clsB,
			"ports@":      qosPolIntf,
		}}}
	t.Run("Verify new policy bound", verifyDbResult(rclient, "ACL_TABLE|"+clsB, expected_map, false))

	// The old policy's table must no longer list the interface at all.
	expected_map = map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosPolClsV4: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosPolClsV4,
		}}}
	t.Run("Verify old policy unbound", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_unbind_keeps_other_interface checks that unbinding one
// interface leaves the others bound. The port list is a leaf-list, so the
// request has to name the interface being removed rather than the ones staying.
func Test_qos_policer_unbind_keeps_other_interface(t *testing.T) {
	// The port list is a leafref to PORT, so both interfaces have to be real
	// ones or CVL rejects the write.
	const otherIntf = qosPolIntf2
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosPolPreloadPolicer(qosPolClsV4)
	pre_req_map["ACL_TABLE"].(map[string]interface{})[qosPolClsV4].(map[string]interface{})["ports@"] =
		qosPolIntf + "," + otherIntf
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Unbinding one interface spares the other ++++++++++++")
	t.Run("Unbind first interface", processDeleteRequest(qosPolBindUrl(qosPolIntf), false))
	time.Sleep(1 * time.Second)

	expected_map := map[string]interface{}{"ACL_TABLE": map[string]interface{}{
		qosPolClsV4: map[string]interface{}{
			"type":        "QOS_POLICER_V4",
			"stage":       "INGRESS",
			"policy_desc": "OpenConfig QoS classifier " + qosPolClsV4,
			"ports@":      otherIntf,
		}}}
	t.Run("Verify other interface still bound", verifyDbResult(rclient, "ACL_TABLE|"+qosPolClsV4, expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}

// Test_qos_policer_replace_prunes_terms checks that replacing a classifier drops
// the rules of terms the new content no longer carries. An update has to leave
// them alone, but a replace is the whole classifier.
func Test_qos_policer_replace_prunes_terms(t *testing.T) {
	cleanuptbl := qosPolCleanupTbl(qosPolClsV4)
	unloadDB(db.ConfigDB, cleanuptbl)

	pre_req_map := qosPolPreloadPolicer(qosPolClsV4)
	pre_req_map["ACL_RULE"].(map[string]interface{})[qosPolClsV4+"|TERM_2"] = map[string]interface{}{
		"PRIORITY":       "59998",
		"PACKET_ACTION":  "FORWARD",
		"IP_TYPE":        "IPV4ANY",
		"L4_DST_PORT":    "22",
		"POLICER_ACTION": qosPolName,
	}
	loadDB(db.ConfigDB, pre_req_map)
	time.Sleep(1 * time.Second)

	t.Log("\n\n+++++++++++++ Replacing a classifier prunes the terms it dropped ++++++++++++")
	url_body_json := `{"openconfig-qos:classifier":[{
		"name":"` + qosPolClsV4 + `",
		"config":{"name":"` + qosPolClsV4 + `","type":"IPV4"},
		"terms":{"term":[{
			"id":"1",
			"config":{"id":"1"},
			"conditions":{"ipv4":{"config":{"protocol":"IP_TCP"}},
				"transport":{"config":{"destination-port":179}}}
		}]}
	}]}`
	t.Run("Replace classifier", processSetRequest(qosPolClassifiersUrl, url_body_json, "PUT", false, nil))
	time.Sleep(1 * time.Second)

	t.Run("Verify dropped term gone", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_2",
		map[string]interface{}{}, false))

	expected_map := map[string]interface{}{"ACL_RULE": map[string]interface{}{
		qosPolClsV4 + "|TERM_1": map[string]interface{}{
			"PRIORITY":       "59999",
			"PACKET_ACTION":  "FORWARD",
			"IP_TYPE":        "IPV4ANY",
			"IP_PROTOCOL":    "6",
			"L4_DST_PORT":    "179",
			"POLICER_ACTION": qosPolName,
		}}}
	t.Run("Verify kept term intact", verifyDbResult(rclient, "ACL_RULE|"+qosPolClsV4+"|TERM_1", expected_map, false))

	unloadDB(db.ConfigDB, cleanuptbl)
	t.Log("\n\n+++++++++++++ Done ++++++++++++")
}
