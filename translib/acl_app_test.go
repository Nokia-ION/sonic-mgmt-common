////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2019 Broadcom. The term Broadcom refers to Broadcom Inc. and/or //
//  its subsidiaries.                                                         //
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

package translib

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	db "github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

func init() {
	initPortTableForACLTests()
	addCleanupFunc("ACL", clearAclDataFromDb)
}

// This will test GET on /openconfig-acl:acl
func Test_AclApp_TopLevelPath(t *testing.T) {
	url := "/openconfig-acl:acl"

	t.Run("Empty_Response_Top_Level", processGetRequest(url, emptyJson, false))

	t.Run("Bulk_Create_Top_Level", processSetRequest(url, bulkAclCreateJsonRequest, "POST", false))

	t.Run("Get_Full_Acl_Tree_Top_Level", processGetRequest(url, bulkAclCreateJsonResponse, false))

	// Delete all bindings before deleting at top level
	t.Run("Delete_All_Bindings_Top_Level", processDeleteRequest("/openconfig-acl:acl/interfaces"))
	t.Run("Delete_Full_ACl_Tree_Top_Level", processDeleteRequest(url))

	t.Run("Verify_Top_Level_Delete", processGetRequest(url, emptyJson, false))
}

func Test_AclApp_SingleAclOperations(t *testing.T) {
	url := "/openconfig-acl:acl/acl-sets/acl-set"

	t.Run("Create_One_Acl_With_Multiple_Rules(PATCH)", processSetRequest(url, oneAclCreateWithRulesJsonRequest, "PATCH", false))

	url += "[name=MyACL3][type=ACL_IPV4]"

	t.Run("Verify_Create_One_Acl_With_Multiple_Rules", processGetRequest(url, oneAclCreateWithRulesJsonResponse, false))

	aclDescrUrl := url + "/config/description"
	t.Run("Delete Acl_Description", processDeleteRequest(aclDescrUrl))
	t.Run("Verify_Acl_Description_Deletion", processGetRequest(aclDescrUrl, emptyAclDescriptionJson, false))

	createAclDescrUrl := url + "/config"
	t.Run("Create_new_Acl_Description", processSetRequest(createAclDescrUrl, aclDescrUpdateJson, "POST", false))
	t.Run("Verify_Description_of_Acl", processGetRequest(aclDescrUrl, aclDescrUpdateJson, false))

	t.Run("Delete_One_Acl_With_All_Its_Rules", processDeleteRequest(url))

	t.Run("Verify_One_Acl_Delete", processGetRequest(url, "", true))
}

// processGetContentRequest issues a GET with the content query parameter, which
// is how a gNMI Get of type CONFIG/STATE/OPERATIONAL reaches an app.
func processGetContentRequest(url, content, expectedRespJson string, errorCase bool) func(*testing.T) {
	return func(t *testing.T) {
		req := GetRequest{Path: url, QueryParams: QueryParameters{Content: content}}
		verifyGet(t, req, expectedRespJson, errorCase)
	}
}

func Test_AclApp_GetContentFilter(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=8]"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))
	t.Run("Get_All_Unchanged", processGetRequest(aclUrl, oneAclCreateJsonResponse, false))
	t.Run("Get_State", processGetContentRequest(aclUrl,
		QueryParametersContentState, oneAclCreateJsonStateResponse, false))
	t.Run("Get_Config", processGetContentRequest(aclUrl,
		QueryParametersContentConfig, oneAclCreateJsonConfigResponse, false))
	t.Run("Get_State_On_Config_Path", processGetContentRequest(aclUrl+"/config",
		QueryParametersContentState, "", true))
	t.Run("Get_Config_On_State_Path", processGetContentRequest(aclUrl+"/state",
		QueryParametersContentConfig, "", true))

	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))
	// Rule counters are the only operational data of an ACL; every other state
	// leaf mirrors a config leaf.
	t.Run("Get_Operational", processGetContentRequest(aclUrl,
		QueryParametersContentOperational, oneAclRuleJsonOperationalResponse, false))
	t.Run("Get_Operational_Counter", processGetContentRequest(ruleUrl+"/state/matched-packets",
		QueryParametersContentOperational, ruleMatchedPacketsJsonResponse, false))
	t.Run("Get_Operational_On_State_Leaf", processGetContentRequest(aclUrl+"/state/description",
		QueryParametersContentOperational, "", true))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

func getCountersDb() *db.DB {
	countersDb, _ := db.NewDB(getDBOptions(db.CountersDB))
	return countersDb
}

// seedAclRuleCounters publishes counters for a rule the way the flex counter
// service does: an ACL_COUNTER_RULE_MAP entry pointing at a COUNTERS row.
func seedAclRuleCounters(t *testing.T, aclKey, ruleKey, oid, pkts, octets string) {
	t.Helper()

	d := getCountersDb()
	if d == nil {
		t.Fatal("Failed to connect to counters db")
	}

	mapTs := &db.TableSpec{Name: ACL_COUNTER_RULE_MAP}
	cntTs := &db.TableSpec{Name: COUNTERS_TABLE}
	t.Cleanup(func() {
		d.DeleteMapAll(mapTs)
		d.DeleteEntry(cntTs, asKey(oid))
		d.DeleteDB()
	})

	if err := d.SetMap(mapTs, aclKey+":"+ruleKey, oid); err != nil {
		t.Fatalf("Failed to write %s; err=%v", ACL_COUNTER_RULE_MAP, err)
	}
	counters := db.Value{Field: map[string]string{
		ACL_COUNTER_ATTR_PACKETS: pkts,
		ACL_COUNTER_ATTR_BYTES:   octets,
	}}
	if err := d.SetEntry(cntTs, asKey(oid), counters); err != nil {
		t.Fatalf("Failed to write counters of %s; err=%v", oid, err)
	}
}

func Test_AclApp_RuleCounters(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=8]"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))
	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))

	// Counters read as zero while ACL flex counter polling is disabled.
	t.Run("Get_Counter_Without_Polling", processGetRequest(ruleUrl+"/state/matched-packets",
		ruleMatchedPacketsJsonResponse, false))

	seedAclRuleCounters(t, "MyACL5_ACL_IPV4", "RULE_8", "oid:0x9000000000aca", "42", "4200")

	t.Run("Get_Matched_Packets", processGetRequest(ruleUrl+"/state/matched-packets",
		`{"openconfig-acl:matched-packets":"42"}`, false))
	t.Run("Get_Matched_Octets", processGetRequest(ruleUrl+"/state/matched-octets",
		`{"openconfig-acl:matched-octets":"4200"}`, false))
	t.Run("Get_Operational_Counters", processGetContentRequest(ruleUrl+"/state",
		QueryParametersContentOperational,
		`{"openconfig-acl:state":{"matched-octets":"4200","matched-packets":"42"}}`, false))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
}

func TestGetAclKeyStr(t *testing.T) {
	tests := []struct {
		aclName string
		aclType string
		want    string
	}{
		{aclName: "MyACL1", aclType: "ACL_IPV4", want: "MyACL1_ACL_IPV4"},
		{aclName: "ACL-12-IPV4", aclType: "ACL_IPV4", want: "ACL_12_IPV4_ACL_IPV4"},
		{aclName: "My ACL", aclType: "ACL_IPV6", want: "My_ACL_ACL_IPV6"},
		{aclName: "*", aclType: "*", want: "*_*"},
	}

	for _, tt := range tests {
		t.Run(tt.aclName, func(t *testing.T) {
			if got := getAclKeyStr(tt.aclName, tt.aclType); got != tt.want {
				t.Errorf("getAclKeyStr(%q, %q) = %q; want %q", tt.aclName, tt.aclType, got, tt.want)
			}
		})
	}
}

func Test_AclApp_SingleRuleOperations(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=8]"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))
	t.Run("Get_One_Acl_Without_Rule", processGetRequest(aclUrl, oneAclCreateJsonResponse, false))

	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))
	t.Run("Get_One_Rule", processGetRequest(ruleUrl, responseOneRuleJson, false))

	// Change Source/Desination address and protocol
	t.Run("Update_Existing_Rule", processSetRequest(ruleUrl, requestOneRulePatchJson, "PATCH", false))
	t.Run("Verify_One_Rule_Updation", processGetRequest(ruleUrl, responseOneRulePatchJson, false))

	tcpFlagsUrl := ruleUrl + "/transport/config/tcp-flags"
	t.Run("Delete_Tcp_Flags_Field", processDeleteRequest(tcpFlagsUrl))
	t.Run("Verify_Tcp_Flags_Deletion", processGetRequest(tcpFlagsUrl, emptyJson, false))

	dscpUrl := ruleUrl + "/ipv4/config/dscp"
	t.Run("Delete_IPv4_Dscp_Field", processDeleteRequest(dscpUrl))
	t.Run("Verify_IPv4_Dscp_Deletion", processGetRequest(dscpUrl, emptyRuleDscpJson, false))

	protocolUrl := ruleUrl + "/ipv4/config/protocol"
	t.Run("Delete_IPv4_Protocol_Field", processSetRequest(protocolUrl, "", "DELETE", true))
	// t.Run("Verify_IPv4_Protocol_Deletion", processGetRequest(protocolUrl, emptyJson, false))

	transportConfigUrl := ruleUrl + "/transport"
	t.Run("Delete_Transport_Container", processDeleteRequest(transportConfigUrl))
	t.Run("Verify_Transport_Container_Deletion", processGetRequest(transportConfigUrl, emptyJson, false))

	ipv4ConfigUrl := ruleUrl + "/ipv4/config"
	t.Run("Delete_IPv4_Config_Container", processSetRequest(ipv4ConfigUrl, "", "DELETE", true))
	// t.Run("Verify_IPv4_Config_Container_Deletion", processGetRequest(ipv4ConfigUrl, emptyJson, false))

	t.Run("Delete_One_Rule", processDeleteRequest(ruleUrl))
	t.Run("Verify_One_Rule_Delete", processGetRequest(ruleUrl, "", true))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

// This will test PUT (Replace) operation by  Replacing multiple Rules with one Rule in an Acl
func Test_AclApp_ReplaceMultipleRulesWithOneRule(t *testing.T) {
	url := "/openconfig-acl:acl/acl-sets/acl-set"

	t.Run("Create_One_Acl_With_Multiple_Rules(PATCH)", processSetRequest(url, oneAclCreateWithRulesJsonRequest, "PATCH", false))

	url += "[name=MyACL3][type=ACL_IPV4]"

	t.Run("Verify_Create_One_Acl_With_Multiple_Rules", processGetRequest(url, oneAclCreateWithRulesJsonResponse, false))

	t.Run("Replace_All_Rules_With_One_Rule", processSetRequest(url, replaceMultiRulesWithOneRuleJsonRequest, "PUT", false))
	t.Run("Verify_Acl_With_Replaced_Rules", processGetRequest(url, replaceMultiRulesWithOneRuleJsonResponse, false))

	t.Run("Delete_One_Acl_With_All_Its_Rules", processDeleteRequest(url))
	t.Run("Verify_One_Acl_Delete", processGetRequest(url, "", true))
}

// This will test PATCH operation by  modifying Description of an Acl
func Test_AclApp_AclDescriptionUpdation(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	descrUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]/config/description"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))

	t.Run("Update_Description_of_Acl", processSetRequest(descrUrl, aclDescrUpdateJson, "PATCH", false))
	t.Run("Verify_Description_of_Acl", processGetRequest(descrUrl, aclDescrUpdateJson, false))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

func Test_AclApp_AclIngressBindingOperations(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=8]"
	bindingUrl := "/openconfig-acl:acl/interfaces/interface[id=Ethernet4]/ingress-acl-sets/ingress-acl-set[set-name=MyACL5][type=ACL_IPV4]"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))

	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))

	t.Run("Create_Ingress_Acl_set", processSetRequest(bindingUrl, ingressAclSetCreateJsonRequest, "POST", false))
	t.Run("Verify_Ingress_Aclset_Creation", processGetRequest(bindingUrl, ingressAclSetCreateJsonResponse, false))
	t.Run("Get_Port_Binding_From_Ingress_AclEntry_Level", processGetRequest(bindingUrl+"/acl-entries/acl-entry[sequence-id=8]", getBindingAclEntryResponse, false))

	t.Run("Delete_Binding_From_Ingress_Aclset", processDeleteRequest(bindingUrl))
	t.Run("Verify_Binding_From_Ingress_Aclset_Deletion", processGetRequest(bindingUrl, "", true))
	t.Run("Delete_One_Rule", processDeleteRequest(ruleUrl))
	t.Run("Verify_One_Rule_Delete", processGetRequest(ruleUrl, "", true))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

func Test_AclApp_AclEgressBindingOperations(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=8]"
	bindingUrl := "/openconfig-acl:acl/interfaces/interface[id=Ethernet4]/egress-acl-sets/egress-acl-set[set-name=MyACL5][type=ACL_IPV4]"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))

	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))

	t.Run("Create_Egress_Acl_set", processSetRequest(bindingUrl, ingressAclSetCreateJsonRequest, "POST", false))
	t.Run("Verify_Egress_Aclset_Creation", processGetRequest(bindingUrl, egressAclSetCreateJsonResponse, false))
	t.Run("Get_Port_Binding_From_Egress_AclEntry_Level", processGetRequest(bindingUrl+"/acl-entries/acl-entry[sequence-id=8]", getBindingAclEntryResponse, false))

	t.Run("Delete_Binding_From_Egress_Aclset", processDeleteRequest(bindingUrl))
	t.Run("Verify_Binding_From_Egress_Aclset_Deletion", processGetRequest(bindingUrl, "", true))
	t.Run("Delete_One_Rule", processDeleteRequest(ruleUrl))
	t.Run("Verify_One_Rule_Delete", processGetRequest(ruleUrl, "", true))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

// Test_AclApp_SwitchPortMatch moves a rule between a single L4 port and a port
// range, in both directions. The two are a yang choice, so the one being left
// behind has to be cleared as the other one is set.
func Test_AclApp_SwitchPortMatch(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=8]"
	transportUrl := ruleUrl + "/transport/config"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))
	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))
	t.Run("Verify_Rule_Ports", verifyRuleFields("MyACL5_ACL_IPV4", "RULE_8", map[string]string{
		"L4_SRC_PORT": "101", "L4_DST_PORT": "100",
		"L4_SRC_PORT_RANGE": "", "L4_DST_PORT_RANGE": ""}))

	t.Run("Update_Ports_To_Ranges", processSetRequest(transportUrl, rulePortRangesUpdateJson, "PATCH", false))
	t.Run("Verify_Ranges", verifyRuleFields("MyACL5_ACL_IPV4", "RULE_8", map[string]string{
		"L4_SRC_PORT_RANGE": "801-811", "L4_DST_PORT_RANGE": "901-921",
		"L4_SRC_PORT": "", "L4_DST_PORT": ""}))
	t.Run("Verify_Ranges_Read_Back", processGetRequest(transportUrl, rulePortRangesGetJson, false))

	t.Run("Update_Ranges_Back_To_Ports", processSetRequest(transportUrl, rulePortsUpdateJson, "PATCH", false))
	t.Run("Verify_Ports", verifyRuleFields("MyACL5_ACL_IPV4", "RULE_8", map[string]string{
		"L4_SRC_PORT": "22", "L4_DST_PORT": "80",
		"L4_SRC_PORT_RANGE": "", "L4_DST_PORT_RANGE": ""}))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

func Test_AclApp_GetOperationsFromMultipleTreeLevels(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=8]"
	bindingUrl := "/openconfig-acl:acl/interfaces/interface[id=Ethernet4]/egress-acl-sets/egress-acl-set[set-name=MyACL5][type=ACL_IPV4]"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))
	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))
	t.Run("Create_Egress_Acl_set_Port_Binding", processSetRequest(bindingUrl, ingressAclSetCreateJsonRequest, "POST", false))

	t.Run("Get_Acl_Tree_From_AclSets_level", processGetRequest("/openconfig-acl:acl/acl-sets", getFromAclSetsTreeLevelResponse, false))

	t.Run("Get_All_Ports_Bindings_From_Interfaces_Tree_Level", processGetRequest("/openconfig-acl:acl/interfaces", getAllPortsFromInterfacesTreeLevelResponse, false))

	t.Run("Get_One_Port_Binding_From_Interface_Tree_Level", processGetRequest("/openconfig-acl:acl/interfaces/interface[id=Ethernet4]", getPortBindingFromInterfaceTreeLevelResponse, false))

	t.Run("Delete_Binding_From_Egress_Aclset", processDeleteRequest(bindingUrl))
	t.Run("Verify_Binding_From_Egress_Aclset_Deletion", processGetRequest(bindingUrl, "", true))

	t.Run("Delete_One_Rule", processDeleteRequest(ruleUrl))
	t.Run("Verify_One_Rule_Delete", processGetRequest(ruleUrl, "", true))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

func Test_AclApp_AddNewPortBindingToAlreadyBindedAcl(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]"
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=8]"
	bindingUrl := "/openconfig-acl:acl/interfaces/interface[id=Ethernet4]/egress-acl-sets/egress-acl-set[set-name=MyACL5][type=ACL_IPV4]"

	t.Run("Create_One_Acl_Without_Rule", processSetRequest(aclUrl, oneAclCreateJsonRequest, "POST", false))
	t.Run("Create_One_Rule", processSetRequest(ruleUrl, requestOneRulePostJson, "POST", false))
	t.Run("Create_Egress_Acl_set_Port_Binding", processSetRequest(bindingUrl, ingressAclSetCreateJsonRequest, "POST", false))

	newBindingUrl := "/openconfig-acl:acl/interfaces/interface[id=Ethernet0]/egress-acl-sets/egress-acl-set[set-name=MyACL5][type=ACL_IPV4]"
	t.Run("Create_New_Egress_Acl_set_Port_Binding", processSetRequest(newBindingUrl, ingressAclSetCreateJsonRequest, "POST", false))

	t.Run("Get_All_Ports_Bindings_From_Interfaces_Tree_Level", processGetRequest("/openconfig-acl:acl/interfaces", getMultiportBindingOnSingleAclResponse, false))

	t.Run("Delete_All_Bindings_Top_Level", processDeleteRequest("/openconfig-acl:acl/interfaces"))
	t.Run("Delete_All_Rules_Not_Acl", processDeleteRequest("/openconfig-acl:acl/acl-sets/acl-set[name=MyACL5][type=ACL_IPV4]/acl-entries"))

	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))

	t.Run("Verify_Top_Level_Delete", processGetRequest("/openconfig-acl:acl", emptyJson, false))
}

func Test_AclApp_IPv6AclAndRule(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL6][type=ACL_IPV6]"
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL6][type=ACL_IPV6]/acl-entries/acl-entry[sequence-id=6]"
	bindingUrl := "/openconfig-acl:acl/interfaces/interface[id=Ethernet4]/ingress-acl-sets/ingress-acl-set[set-name=MyACL6][type=ACL_IPV6]"

	t.Run("Create_One_IPv6_Acl_Without_Rule", processSetRequest(aclUrl, oneIPv6AclCreateJsonRequest, "POST", false))
	t.Run("Verify_One_IPv6_Acl_Without_Rule_Creation", processGetRequest(aclUrl, oneIPv6AclCreateJsonResponse, false))

	t.Run("Create_One_IPv6_Rule", processSetRequest(ruleUrl, oneIPv6RuleCreateJsonRequest, "POST", false))
	t.Run("Verify_One_IPv6_Rule_Creation", processGetRequest(ruleUrl, oneIPv6RuleCreateJsonResponse, false))

	t.Run("Create_Ingress_Acl_set", processSetRequest(bindingUrl, ingressIPv6AclSetCreateJsonRequest, "POST", false))
	t.Run("Verify_Ingress_Aclset_Creation", processGetRequest(bindingUrl, ingressIPv6AclSetCreateJsonResponse, false))

	t.Run("Get_Acl_Tree_From_AclSet_level", processGetRequest("/openconfig-acl:acl/acl-sets/acl-set", getIPv6AclsFromAclSetListLevelResponse, false))
	t.Run("Get_All_Ports_Bindings_From_Interfaces_Tree_Level", processGetRequest("/openconfig-acl:acl/interfaces", getIPv6AllPortsBindingsResponse, false))

	t.Run("Delete_Binding_From_Ingress_Aclset", processDeleteRequest(bindingUrl))
	t.Run("Verify_Binding_From_Ingress_Aclset_Deletion", processGetRequest(bindingUrl, "", true))

	pktActionUrl := ruleUrl + "/actions/config/forwarding-action"
	t.Run("Delete_Packet_Action_Field", processSetRequest(pktActionUrl, "", "DELETE", true))
	// t.Run("Verify_Packet_Action_Field_Deletion", processGetRequest(pktActionUrl, emptyJson, false))

	ipv6ConfigUrl := ruleUrl + "/ipv6/config"
	t.Run("Delete_IPv6_Config", processSetRequest(ipv6ConfigUrl, "", "DELETE", true))
	// t.Run("Verify_IPv6_Config_Deletion", processGetRequest(ipv6ConfigUrl, emptyJson, false))

	t.Run("Delete_One_Rule", processDeleteRequest(ruleUrl))
	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

// Test_AclApp_IcmpMatch covers the ICMPv4 and ICMPv6 type/code match fields.
// openconfig-acl-ext augments them into an acl-entry's ipv4 and ipv6 containers
// instead of openconfig-packet-match, so a client may name the container either
// plainly or qualified with the extension module; both must reach the same
// ACL_RULE fields.
func Test_AclApp_IcmpMatch(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL8][type=ACL_IPV4]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=8]"
	icmpUrl := ruleUrl + "/ipv4/icmpv4"

	t.Run("Create_Acl", processSetRequest(aclUrl, icmpv4AclCreateJson, "POST", false))
	t.Run("Create_Rule_With_Icmpv4_Match", processSetRequest(ruleUrl, icmpv4RuleCreateJson, "POST", false))
	t.Run("Verify_Icmpv4_Match", verifyRuleFields("MyACL8_ACL_IPV4", "RULE_8", map[string]string{
		"IP_PROTOCOL": "1", "ICMP_TYPE": "5", "ICMP_CODE": "1"}))
	t.Run("Read_Back_Icmpv4_Match", processGetRequest(icmpUrl+"/config", icmpv4ConfigGetJson, false))

	qualifiedUrl := ruleUrl + "/ipv4/openconfig-acl-ext:icmpv4/config"
	t.Run("Update_Icmpv4_Type_Module_Qualified", processSetRequest(qualifiedUrl, icmpv4TypeUpdateJson, "PATCH", false))
	t.Run("Verify_Icmpv4_Type_Update", verifyRuleFields("MyACL8_ACL_IPV4", "RULE_8", map[string]string{
		"ICMP_TYPE": "8", "ICMP_CODE": "1"}))

	t.Run("Delete_Icmpv4_Match", processDeleteRequest(icmpUrl))
	t.Run("Verify_Icmpv4_Match_Is_Gone", verifyRuleFields("MyACL8_ACL_IPV4", "RULE_8", map[string]string{
		"ICMP_TYPE": "", "ICMP_CODE": ""}))

	t.Run("Delete_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_Acl_Delete", processGetRequest(aclUrl, "", true))

	aclv6Url := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL9][type=ACL_IPV6]"
	rulev6Url := aclv6Url + "/acl-entries/acl-entry[sequence-id=6]"

	t.Run("Create_IPv6_Acl", processSetRequest(aclv6Url, icmpv6AclCreateJson, "POST", false))
	t.Run("Create_Rule_With_Icmpv6_Match", processSetRequest(rulev6Url, icmpv6RuleCreateJson, "POST", false))
	// The icmpv6 type is an openconfig-icmpv6-types identity, mapped to the
	// numeric ICMPv6 message type: ECHO_REPLY is 129.
	t.Run("Verify_Icmpv6_Match", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"IP_PROTOCOL": "58", "ICMPV6_TYPE": "129"}))

	// An ICMPv6 code is an identity of its type, so the pair travels together:
	// under DESTINATION_UNREACHABLE (type 1), port unreachable is code 4.
	icmpv6ConfigUrl := rulev6Url + "/ipv6/icmpv6/config"
	t.Run("Set_Icmpv6_Type_And_Code", processSetRequest(icmpv6ConfigUrl, icmpv6TypeCodeJson, "PATCH", false))
	t.Run("Verify_Icmpv6_Type_And_Code", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"ICMPV6_TYPE": "1", "ICMPV6_CODE": "4"}))
	t.Run("Read_Back_Icmpv6_Type_And_Code", processGetRequest(icmpv6ConfigUrl, icmpv6TypeCodeGetJson, false))

	// A code alone is enough once the rule already carries the type it belongs
	// to: an update merges into the ACL_RULE row rather than replacing it.
	t.Run("Update_Icmpv6_Code_Alone", processSetRequest(icmpv6ConfigUrl, icmpv6CodeOnlyJson, "PATCH", false))
	t.Run("Verify_Icmpv6_Code_Alone", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"ICMPV6_TYPE": "1", "ICMPV6_CODE": "1"}))

	// A code of a different type than the configured one would install a rule
	// matching a combination no packet carries. TIME_EXCEEDED_HOP_LIMIT is
	// code 0 of type 3, and 0 is also a valid code of type 1, so this is
	// rejected on the identity rather than on the number.
	t.Run("Reject_Icmpv6_Code_Of_Other_Type", processSetRequestErr(icmpv6ConfigUrl, icmpv6MismatchedCodeJson, "PATCH"))
	// A per-type group identity is held to its type like a code is.
	t.Run("Reject_Icmpv6_Code_Group_Of_Other_Type", processSetRequestErr(icmpv6ConfigUrl, icmpv6OtherGroupCodeJson, "PATCH"))
	// Moving the type out from under a stored code would leave the two
	// mismatched: ECHO_REPLY has only code 0, and the rule carries code 1.
	t.Run("Reject_Icmpv6_Type_Leaving_Stale_Code", processSetRequestErr(icmpv6ConfigUrl, icmpv6TypeOnlyJson, "PATCH"))
	// A rejected request must not have disturbed the rule.
	t.Run("Verify_Icmpv6_Match_Unchanged", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"ICMPV6_TYPE": "1", "ICMPV6_CODE": "1"}))

	// The group identity of the configured type stands for every code of it,
	// so the rule matches on the type alone and the stored code 1 goes.
	t.Run("Set_Icmpv6_Code_Group_Identity", processSetRequest(icmpv6ConfigUrl, icmpv6GroupCodeJson, "PATCH", false))
	t.Run("Verify_Icmpv6_Code_Group_Matches_Type_Only", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"ICMPV6_TYPE": "1", "ICMPV6_CODE": ""}))
	t.Run("Restore_Icmpv6_Code", processSetRequest(icmpv6ConfigUrl, icmpv6CodeOnlyJson, "PATCH", false))
	t.Run("Verify_Icmpv6_Code_Restored", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"ICMPV6_TYPE": "1", "ICMPV6_CODE": "1"}))

	// A code on a rule with no type at all has nothing to be interpreted
	// against, so it is refused rather than matching that number under every
	// type.
	rulev6NoTypeUrl := aclv6Url + "/acl-entries/acl-entry[sequence-id=7]"
	t.Run("Create_Rule_Without_Icmpv6_Type", processSetRequest(rulev6NoTypeUrl, icmpv6RuleNoTypeJson, "POST", false))
	t.Run("Reject_Icmpv6_Code_Without_Type", processSetRequestErr(rulev6NoTypeUrl+"/ipv6/icmpv6/config", icmpv6CodeOnlyJson, "PATCH"))
	t.Run("Reject_Icmpv6_Code_Group_Without_Type", processSetRequestErr(rulev6NoTypeUrl+"/ipv6/icmpv6/config", icmpv6OtherGroupCodeJson, "PATCH"))
	t.Run("Verify_No_Icmpv6_Code_Written", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_7", map[string]string{
		"ICMPV6_TYPE": "", "ICMPV6_CODE": ""}))

	t.Run("Delete_Icmpv6_Code_Only", processDeleteRequest(rulev6Url+"/ipv6/icmpv6/config/code"))
	t.Run("Verify_Icmpv6_Code_Is_Gone", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"ICMPV6_TYPE": "1", "ICMPV6_CODE": ""}))

	t.Run("Delete_Icmpv6_Match", processDeleteRequest(rulev6Url+"/ipv6/icmpv6"))
	t.Run("Verify_Icmpv6_Match_Is_Gone", verifyRuleFields("MyACL9_ACL_IPV6", "RULE_6", map[string]string{
		"ICMPV6_TYPE": "", "ICMPV6_CODE": ""}))

	t.Run("Delete_IPv6_Acl", processDeleteRequest(aclv6Url))
	t.Run("Verify_IPv6_Acl_Delete", processGetRequest(aclv6Url, "", true))
}

// Test_AclApp_LogAction checks that LOG_SYSLOG, which an ACL_RULE cannot carry,
// is refused instead of being dropped, while LOG_NONE is accepted.
func Test_AclApp_LogAction(t *testing.T) {
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL10][type=ACL_IPV6]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=10]"

	t.Run("Create_Acl", processSetRequest(aclUrl, logActionAclCreateJson, "POST", false))

	t.Run("Create_Rule_With_Log_Syslog",
		processSetRequestErrOfType(ruleUrl, logSyslogRuleCreateJson, "POST", tlerr.NotSupportedError{}))
	t.Run("Verify_Rule_Not_Created", processGetRequest(ruleUrl, "", true))

	t.Run("Create_Rule_With_Log_None", processSetRequest(ruleUrl, logNoneRuleCreateJson, "POST", false))
	t.Run("Verify_Rule_Created", verifyRuleFields("MyACL10_ACL_IPV6", "RULE_10", map[string]string{
		"PACKET_ACTION": "FORWARD", "ICMPV6_TYPE": "128"}))

	t.Run("Update_Rule_To_Log_Syslog",
		processSetRequestErrOfType(ruleUrl+"/actions/config", logSyslogActionUpdateJson, "PATCH", tlerr.NotSupportedError{}))
	t.Run("Replace_Acl_With_Log_Syslog",
		processSetRequestErrOfType(aclUrl, logSyslogAclReplaceJson, "PUT", tlerr.NotSupportedError{}))
	t.Run("Verify_Rule_Unchanged", verifyRuleFields("MyACL10_ACL_IPV6", "RULE_10", map[string]string{
		"PACKET_ACTION": "FORWARD", "ICMPV6_TYPE": "128"}))

	t.Run("Delete_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_Acl_Delete", processGetRequest(aclUrl, "", true))
}

func Test_AclApp_L2AclAndRule(t *testing.T) {
	t.Skip("AclApp does not support L2 ACLs")

	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL2][type=ACL_L2]"
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL2][type=ACL_L2]/acl-entries/acl-entry[sequence-id=2]"
	bindingUrl := "/openconfig-acl:acl/interfaces/interface[id=Ethernet0]/ingress-acl-sets/ingress-acl-set[set-name=MyACL2][type=ACL_L2]"

	t.Run("Create_One_L2_Acl_Without_Rule", processSetRequest(aclUrl, oneL2AclCreateJsonRequest, "POST", false))
	t.Run("Verify_One_L2_Acl_Without_Rule_Creation", processGetRequest(aclUrl, oneL2AclCreateJsonResponse, false))

	t.Run("Create_One_L2_Rule", processSetRequest(ruleUrl, oneL2RuleCreateJsonRequest, "POST", false))
	t.Run("Verify_One_L2_Rule_Creation", processGetRequest(ruleUrl, oneL2RuleCreateJsonResponse, false))

	t.Run("Create_Ingress_L2_Acl_set", processSetRequest(bindingUrl, ingressL2AclSetCreateJsonRequest, "POST", false))
	t.Run("Verify_Ingress_L2_Aclset_Creation", processGetRequest(bindingUrl, ingressL2AclSetCreateJsonResponse, false))

	t.Run("Get_Acl_Tree_From_AclSet_level", processGetRequest("/openconfig-acl:acl/acl-sets/acl-set", getL2AclsFromAclSetListLevelResponse, false))
	t.Run("Get_All_Ports_Bindings_From_Interfaces_Tree_Level", processGetRequest("/openconfig-acl:acl/interfaces", getL2AllPortsBindingsResponse, false))

	t.Run("Delete_Binding_From_Ingress_Aclset", processDeleteRequest(bindingUrl))
	t.Run("Verify_Binding_From_Ingress_Aclset_Deletion", processGetRequest(bindingUrl, "", true))

	etherTypeUrl := ruleUrl + "/l2/config/ethertype"
	t.Run("Delete_Ethertype_Field", processDeleteRequest(etherTypeUrl))
	t.Run("Verify_L2_Ethertype_Field_Deletion", processGetRequest(ruleUrl+"/l2/config", emptyJson, false))

	t.Run("Delete_Transport_Src_Port_Field", processDeleteRequest(ruleUrl+"/transport/config/source-port"))
	t.Run("Delete_Transport_Dst_Port_Field", processDeleteRequest(ruleUrl+"/transport/config/destination-port"))
	t.Run("Delete_Transport_Tcp_Flags_Field", processDeleteRequest(ruleUrl+"/transport/config/tcp-flags"))
	t.Run("Verify_Transport_Src_Dst_Fields_Deletion", processGetRequest(ruleUrl+"/transport/config", emptyJson, false))

	t.Run("Delete_One_Rule", processDeleteRequest(ruleUrl))
	t.Run("Delete_One_Acl", processDeleteRequest(aclUrl))
	t.Run("Verify_One_Acl_Delete", processGetRequest(aclUrl, "", true))
}

func Test_AclApp_NegativeTests(t *testing.T) {
	// Verify GET returns errors for non-existing ACL
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL2][type=ACL_L2]"
	t.Run("Verify_Non_Existing_Acl_GET_Error", processGetRequest(aclUrl, "", true))

	// Verify GET returns errors for non-existing Rule
	ruleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL2][type=ACL_L2]/acl-entries/acl-entry[sequence-id=2]"
	t.Run("Verify_Non_Existing_Rule_GET_Error", processGetRequest(ruleUrl, "", true))

	// Verify Error on giving Invalid Interface in payload during binding creation
	url := "/openconfig-acl:acl"
	t.Run("Create_Acl_With_Invalid_Interface_Binding", processSetRequest(url, aclCreateWithInvalidInterfaceBinding, "POST", true))

	// Verify error if duplicate Acl is created using POST
	aclUrl = "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL6][type=ACL_IPV6]"
	t.Run("Create_One_IPV6_Acl_Without_Rule", processSetRequest(aclUrl, oneIPv6AclCreateJsonRequest, "POST", false))
	t.Run("Verify_One_IPV6_Acl_Without_Rule_Creation", processGetRequest(aclUrl, oneIPv6AclCreateJsonResponse, false))
	t.Run("Verify_Error_On_Create_Duplicate_IPV6_Acl", processSetRequest(aclUrl, oneIPv6AclCreateJsonRequest, "POST", true))

	// Verify error if duplicate Rule is created using POST
	multiRuleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL3][type=ACL_IPV4]"
	t.Run("Create_One_Acl_With_Multiple_Rules(PATCH)", processSetRequest(multiRuleUrl, oneAclCreateWithRulesJsonRequest, "PATCH", false))
	t.Run("Verify_Create_One_Acl_With_Multiple_Rules", processGetRequest(multiRuleUrl, oneAclCreateWithRulesJsonResponse, false))

	duplicateRuleUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL3][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=1]"
	t.Run("Create_One_Duplicate_Rule", processSetRequest(duplicateRuleUrl, requestOneDuplicateRulePostJson, "POST", true))

	topLevelUrl := "/openconfig-acl:acl"
	t.Run("Delete_Full_ACl_Tree_Top_Level", processDeleteRequest(topLevelUrl))
	t.Run("Verify_Top_Level_Delete", processGetRequest(topLevelUrl, emptyJson, false))
}

// THis will delete ACL table and Rules Table from DB
func clearAclDataFromDb() error {
	var err error
	ruleTable := db.TableSpec{Name: "ACL_RULE"}
	aclTable := db.TableSpec{Name: "ACL_TABLE"}

	d := getConfigDb()
	if d == nil {
		err = errors.New("Failed to connect to config Db")
		return err
	}
	defer d.DeleteDB()
	if err = d.DeleteTable(&ruleTable); err != nil {
		err = errors.New("Failed to delete Rules Table")
		return err
	}
	if err = d.DeleteTable(&aclTable); err != nil {
		err = errors.New("Failed to delete Acl Table")
		return err
	}
	return err
}

func initPortTableForACLTests() {
	d := getConfigDb()
	defer d.DeleteDB()
	portTS := &db.TableSpec{Name: "PORT"}
	for i, ifName := range []string{"Ethernet0", "Ethernet4"} {
		v, err := d.GetEntry(portTS, asKey(ifName))
		if isNotFoundError(err) {
			v.Field = map[string]string{"admin_status": "up", "mtu": "9100", "speed": "4000", "alias": fmt.Sprintf("Eth1/%d", i)}
			err = d.CreateEntry(portTS, asKey(ifName), v)
		}
		if err != nil {
			panic(err.Error())
		}
	}
}

func Test_AclApp_Subscribe(t *testing.T) {
	aclListPath := "/openconfig-acl:acl/acl-sets/acl-set[name=*][type=*]"
	ruleListPath := aclListPath + "/acl-entries/acl-entry[sequence-id=*]"

	t.Run("top", func(t *testing.T) {
		tv := testTranslateSubscribe(t, "/openconfig-acl:acl")
		tv.VerifyCount(2, 1)
		tv.VerifyTarget(aclListPath, aclTableNInfo("*", "*", ""))
		tv.VerifyChild(ruleListPath, ruleTableNInfo("*", "*", "*", ""))
		tv.VerifyTarget("/openconfig-acl:acl/interfaces", bindingTableNInfo())
	})

	t.Run("aclsets", func(t *testing.T) {
		tv := testTranslateSubscribe(t, "/openconfig-acl:acl/acl-sets/acl-set")
		tv.VerifyCount(1, 1)
		tv.VerifyTarget(aclListPath, aclTableNInfo("*", "*", ""))
		tv.VerifyChild(ruleListPath, ruleTableNInfo("*", "*", "*", ""))
	})

	// A dash cannot appear in a SONiC table name, so a subscription has to watch
	// the same substituted key that a Get or a Set of this acl-set writes to,
	// while the notification keeps reporting the requested name.
	t.Run("acl_name_with_dash", func(t *testing.T) {
		aPath := "/openconfig-acl:acl/acl-sets/acl-set[name=ACL-12-IPV4][type=ACL_IPV4]"
		tv := testTranslateSubscribe(t, aPath)
		tv.VerifyCount(1, 1)
		tv.VerifyTarget(aPath, aclTableNInfo("ACL_12_IPV4", "ACL_IPV4", ""))
		tv.VerifyChild(aPath+"/acl-entries/acl-entry[sequence-id=*]",
			ruleTableNInfo("ACL_12_IPV4", "ACL_IPV4", "*", ""))
	})

	for _, k := range []string{"*,*", "TEST1,*", "*,ACL_IPV4", "TEST2,ACL_IPV6"} {
		parts := strings.Split(k, ",")
		aName, aType := parts[0], parts[1]
		aPath := fmt.Sprintf("/openconfig-acl:acl/acl-sets/acl-set[name=%s][type=%s]", aName, aType)

		aclListPath := fmt.Sprintf("/openconfig-acl:acl/acl-sets/acl-set[name=%s][type=%s]", aName, aType)
		ruleListPath := aclListPath + "/acl-entries/acl-entry[sequence-id=*]"

		t.Run("acl="+k, func(t *testing.T) {
			tv := testTranslateSubscribe(t, aPath)
			tv.VerifyCount(1, 1)
			tv.VerifyTarget(aclListPath, aclTableNInfo(aName, aType, ""))
			tv.VerifyChild(ruleListPath, ruleTableNInfo(aName, aType, "*", ""))
		})

		t.Run("acl="+k+"/descr", func(t *testing.T) {
			tv := testTranslateSubscribe(t, aPath+"/config/description")
			tv.VerifyCount(1, 0)
			tv.VerifyTarget(aclListPath+"/config/description",
				aclTableNInfo(aName, aType, `{"":{"policy_desc": ""}}`))
		})

		t.Run("acl="+k+"/rules", func(t *testing.T) {
			tv := testTranslateSubscribe(t, aPath+"/acl-entries")
			tv.VerifyCount(1, 0)
			tv.VerifyTarget(ruleListPath, ruleTableNInfo(aName, aType, "*", ""))
		})

		ruleTests := []struct{ name, subPath, ruleId, fields string }{
			{
				name:   "rule=*",
				ruleId: "*",
			}, {
				name:   "rule=55",
				ruleId: "55",
			}, {
				name:    "rule_actions",
				subPath: "/actions",
				ruleId:  "*",
				fields:  `{"config": {"PACKET_ACTION": "forwarding-action"}, "state": {"PACKET_ACTION": "forwarding-action"}}`,
			}, {
				name:    "rule_action_config",
				subPath: "/actions/config",
				ruleId:  "*",
				fields:  `{"": {"PACKET_ACTION": "forwarding-action"}}`,
			}, {
				name:    "rule_forwarding",
				subPath: "/actions/state/forwarding-action",
				ruleId:  "66",
				fields:  `{"": {"PACKET_ACTION": ""}}`,
			}}

		for _, r := range ruleTests {
			t.Run("acl="+k+"/"+r.name, func(t *testing.T) {
				rPath := aclListPath + fmt.Sprintf("/acl-entries/acl-entry[sequence-id=%s]%s", r.ruleId, r.subPath)
				tv := testTranslateSubscribe(t, rPath)
				tv.VerifyCount(1, 0)
				tv.VerifyTarget(rPath, ruleTableNInfo(aName, aType, r.ruleId, r.fields))
			})
		}
	}
}

// A subscription that leaves the rule key as a wildcard makes translib resolve
// the notification path through processSubscribe, and it then checks that the
// result still matches what the client asked for. The acl-set name cannot be
// recovered from the CONFIG_DB key when it contains a dash, so the requested
// name has to be carried through untouched or every notification is dropped.
func Test_AclApp_ProcessSubscribe(t *testing.T) {
	const dbKey = "ACL_12_IPV4_ACL_IPV4"

	d := getConfigDb()
	if d == nil {
		t.Fatal("cannot connect to CONFIG_DB")
	}
	defer d.DeleteDB()

	aclTs := &db.TableSpec{Name: ACL_TABLE}
	if err := d.SetEntry(aclTs, asKey(dbKey),
		db.Value{Field: map[string]string{ACL_TYPE: SONIC_ACL_TYPE_IPV4}}); err != nil {
		t.Fatalf("cannot create %s: %v", dbKey, err)
	}
	t.Cleanup(func() { d.DeleteEntry(aclTs, asKey(dbKey)) })

	app := &AclApp{}
	tests := []struct {
		name      string
		template  string
		wantName  string
		wantSeqId string
	}{{
		name:      "requested_name_with_dash_is_preserved",
		template:  "/openconfig-acl:acl/acl-sets/acl-set[name=ACL-12-IPV4][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=*]",
		wantName:  "ACL-12-IPV4",
		wantSeqId: "10",
	}, {
		name:      "wildcard_name_is_resolved_from_db",
		template:  "/openconfig-acl:acl/acl-sets/acl-set[name=*][type=*]/acl-entries/acl-entry[sequence-id=*]",
		wantName:  "ACL_12_IPV4",
		wantSeqId: "10",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqPath, err := ygot.StringToPath(tc.template, ygot.StructuredPath, ygot.StringSlicePath)
			if err != nil {
				t.Fatalf("bad template %s: %v", tc.template, err)
			}

			resp, err := app.processSubscribe(processSubRequest{
				path:  path.Clone(reqPath),
				dbno:  db.ConfigDB,
				table: &db.TableSpec{Name: RULE_TABLE},
				key:   db.NewKey(dbKey, "RULE_10"),
				dbs:   getReadOnlyDB(),
			})
			if err != nil {
				t.Fatalf("processSubscribe: %v", err)
			}

			if got := resp.path.Elem[2].Key["name"]; got != tc.wantName {
				t.Errorf("name = %q, want %q", got, tc.wantName)
			}
			if got := resp.path.Elem[4].Key["sequence-id"]; got != tc.wantSeqId {
				t.Errorf("sequence-id = %q, want %q", got, tc.wantSeqId)
			}
			if path.HasWildcardKey(resp.path) {
				t.Errorf("unresolved wildcard in %s", path.String(resp.path))
			}
			// This is the check translib applies before sending a notification.
			if !path.Matches(resp.path, reqPath) {
				t.Errorf("resolved path %s does not match requested template %s",
					path.String(resp.path), tc.template)
			}
		})
	}

	t.Run("default_rule_is_not_an_acl_entry", func(t *testing.T) {
		reqPath, err := ygot.StringToPath(tests[0].template, ygot.StructuredPath, ygot.StringSlicePath)
		if err != nil {
			t.Fatalf("bad template: %v", err)
		}
		_, err = app.processSubscribe(processSubRequest{
			path:  path.Clone(reqPath),
			dbno:  db.ConfigDB,
			table: &db.TableSpec{Name: RULE_TABLE},
			key:   db.NewKey(dbKey, "DEFAULT_RULE"),
			dbs:   getReadOnlyDB(),
		})
		if err == nil {
			t.Error("DEFAULT_RULE accepted as an acl-entry")
		}
	})
}

func Test_StripWildcardListKeys(t *testing.T) {
	tests := []struct {
		uri         string
		want        string
		wantChanged bool
	}{{
		uri:         "/openconfig-acl:acl/acl-sets/acl-set[name=ACL1][type=ACL_IPV4]/acl-entries/acl-entry[sequence-id=*]",
		want:        "/openconfig-acl:acl/acl-sets/acl-set[name=ACL1][type=ACL_IPV4]/acl-entries/acl-entry",
		wantChanged: true,
	}, {
		uri:         "/openconfig-acl:acl/acl-sets/acl-set[name=*][type=*]/acl-entries",
		want:        "/openconfig-acl:acl/acl-sets/acl-set/acl-entries",
		wantChanged: true,
	}, {
		// Half a key set cannot be served either way, so it is left as is.
		uri:         "/openconfig-acl:acl/acl-sets/acl-set[name=ACL1][type=*]",
		want:        "/openconfig-acl:acl/acl-sets/acl-set[name=ACL1][type=*]",
		wantChanged: false,
	}, {
		uri:         "/openconfig-acl:acl/acl-sets/acl-set[name=ACL1][type=ACL_IPV4]",
		want:        "/openconfig-acl:acl/acl-sets/acl-set[name=ACL1][type=ACL_IPV4]",
		wantChanged: false,
	}, {
		uri:         "/openconfig-acl:acl/acl-sets",
		want:        "/openconfig-acl:acl/acl-sets",
		wantChanged: false,
	}}

	for _, tc := range tests {
		t.Run(tc.uri, func(t *testing.T) {
			got, changed := stripWildcardListKeys(tc.uri)
			if changed != tc.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tc.wantChanged)
			}
			if got != tc.want {
				t.Errorf("uri = %q, want %q", got, tc.want)
			}
		})
	}
}

func aclTableNInfo(namePattern, typePattern, fieldsJson string) *notificationAppInfo {
	return &notificationAppInfo{
		dbno:                db.ConfigDB,
		table:               &db.TableSpec{Name: "ACL_TABLE"},
		key:                 db.NewKey(namePattern + "_" + typePattern),
		dbFldYgPathInfoList: parseFieldsJSON(fieldsJson),
		isOnChangeSupported: true,
		pType:               OnChange,
	}
}

func ruleTableNInfo(namePattern, typePattern, seqPattern, fieldsJson string) *notificationAppInfo {
	return &notificationAppInfo{
		dbno:                db.ConfigDB,
		table:               &db.TableSpec{Name: "ACL_RULE"},
		key:                 db.NewKey(namePattern+"_"+typePattern, "RULE_"+seqPattern),
		dbFldYgPathInfoList: parseFieldsJSON(fieldsJson),
		isOnChangeSupported: true,
		pType:               OnChange,
	}
}

func bindingTableNInfo() *notificationAppInfo {
	return &notificationAppInfo{
		dbno:                db.MaxDB, // app is not mapping bindings path to any DB
		isOnChangeSupported: false,
		pType:               Sample,
	}
}

/***************************************************************************/
///////////                  JSON Data for Tests              ///////////////
/***************************************************************************/
var emptyJson string = "{}"

var bulkAclCreateJsonRequest string = "{\"acl-sets\":{\"acl-set\":[{\"name\":\"MyACL1\",\"type\":\"ACL_IPV4\",\"config\":{\"name\":\"MyACL1\",\"type\":\"ACL_IPV4\",\"description\":\"Description for MyACL1\"},\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":1,\"config\":{\"sequence-id\":1,\"description\":\"Description for MyACL1 Rule Seq 1\"},\"ipv4\":{\"config\":{\"source-address\":\"11.1.1.1/32\",\"destination-address\":\"21.1.1.1/32\",\"dscp\":1,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":101,\"destination-port\":201}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}},{\"sequence-id\":2,\"config\":{\"sequence-id\":2,\"description\":\"Description for MyACL1 Rule Seq 2\"},\"ipv4\":{\"config\":{\"source-address\":\"11.1.1.2/32\",\"destination-address\":\"21.1.1.2/32\",\"dscp\":2,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":102,\"destination-port\":202}},\"actions\":{\"config\":{\"forwarding-action\":\"DROP\"}}},{\"sequence-id\":3,\"config\":{\"sequence-id\":3,\"description\":\"Description for MyACL1 Rule Seq 3\"},\"ipv4\":{\"config\":{\"source-address\":\"11.1.1.3/32\",\"destination-address\":\"21.1.1.3/32\",\"dscp\":3,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":103,\"destination-port\":203}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}},{\"sequence-id\":4,\"config\":{\"sequence-id\":4,\"description\":\"Description for MyACL1 Rule Seq 4\"},\"ipv4\":{\"config\":{\"source-address\":\"11.1.1.4/32\",\"destination-address\":\"21.1.1.4/32\",\"dscp\":4,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":104,\"destination-port\":204}},\"actions\":{\"config\":{\"forwarding-action\":\"DROP\"}}},{\"sequence-id\":5,\"config\":{\"sequence-id\":5,\"description\":\"Description for MyACL1 Rule Seq 5\"},\"ipv4\":{\"config\":{\"source-address\":\"11.1.1.5/32\",\"destination-address\":\"21.1.1.5/32\",\"dscp\":5,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":105,\"destination-port\":205}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}}]}},{\"name\":\"MyACL2\",\"type\":\"ACL_IPV4\",\"config\":{\"name\":\"MyACL2\",\"type\":\"ACL_IPV4\",\"description\":\"Description for MyACL2\"},\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":1,\"config\":{\"sequence-id\":1,\"description\":\"Description for Rule Seq 1\"},\"ipv4\":{\"config\":{\"source-address\":\"12.1.1.1/32\",\"destination-address\":\"22.1.1.1/32\",\"dscp\":1,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":101,\"destination-port\":201}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}},{\"sequence-id\":2,\"config\":{\"sequence-id\":2,\"description\":\"Description for Rule Seq 2\"},\"ipv4\":{\"config\":{\"source-address\":\"12.1.1.2/32\",\"destination-address\":\"22.1.1.2/32\",\"dscp\":2,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":102,\"destination-port\":202}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}},{\"sequence-id\":3,\"config\":{\"sequence-id\":3,\"description\":\"Description for Rule Seq 3\"},\"ipv4\":{\"config\":{\"source-address\":\"12.1.1.3/32\",\"destination-address\":\"22.1.1.3/32\",\"dscp\":3,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":103,\"destination-port\":203}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}},{\"sequence-id\":4,\"config\":{\"sequence-id\":4,\"description\":\"Description for Rule Seq 4\"},\"ipv4\":{\"config\":{\"source-address\":\"12.1.1.4/32\",\"destination-address\":\"22.1.1.4/32\",\"dscp\":4,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":104,\"destination-port\":204}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}},{\"sequence-id\":5,\"config\":{\"sequence-id\":5,\"description\":\"Description for Rule Seq 5\"},\"ipv4\":{\"config\":{\"source-address\":\"12.1.1.5/32\",\"destination-address\":\"22.1.1.5/32\",\"dscp\":5,\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"source-port\":105,\"destination-port\":205}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}}]}}]},\"interfaces\":{\"interface\":[{\"id\":\"Ethernet0\",\"config\":{\"id\":\"Ethernet0\"},\"interface-ref\":{\"config\":{\"interface\":\"Ethernet0\"}},\"ingress-acl-sets\":{\"ingress-acl-set\":[{\"set-name\":\"MyACL1\",\"type\":\"ACL_IPV4\",\"config\":{\"set-name\":\"MyACL1\",\"type\":\"ACL_IPV4\"}}]}},{\"id\":\"Ethernet4\",\"config\":{\"id\":\"Ethernet4\"},\"interface-ref\":{\"config\":{\"interface\":\"Ethernet4\"}},\"ingress-acl-sets\":{\"ingress-acl-set\":[{\"set-name\":\"MyACL2\",\"type\":\"ACL_IPV4\",\"config\":{\"set-name\":\"MyACL2\",\"type\":\"ACL_IPV4\"}}]}}]}}"

var bulkAclCreateJsonResponse string = "{\"openconfig-acl:acl\":{\"acl-sets\":{\"acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":1},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.1/32\",\"dscp\":1,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.1/32\"},\"state\":{\"destination-address\":\"21.1.1.1/32\",\"dscp\":1,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.1/32\"}},\"sequence-id\":1,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":1},\"transport\":{\"config\":{\"destination-port\":201,\"source-port\":101},\"state\":{\"destination-port\":201,\"source-port\":101}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:DROP\"},\"state\":{\"forwarding-action\":\"openconfig-acl:DROP\"}},\"config\":{\"sequence-id\":2},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.2/32\",\"dscp\":2,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.2/32\"},\"state\":{\"destination-address\":\"21.1.1.2/32\",\"dscp\":2,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.2/32\"}},\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2},\"transport\":{\"config\":{\"destination-port\":202,\"source-port\":102},\"state\":{\"destination-port\":202,\"source-port\":102}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":3},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.3/32\",\"dscp\":3,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.3/32\"},\"state\":{\"destination-address\":\"21.1.1.3/32\",\"dscp\":3,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.3/32\"}},\"sequence-id\":3,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":3},\"transport\":{\"config\":{\"destination-port\":203,\"source-port\":103},\"state\":{\"destination-port\":203,\"source-port\":103}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:DROP\"},\"state\":{\"forwarding-action\":\"openconfig-acl:DROP\"}},\"config\":{\"sequence-id\":4},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.4/32\",\"dscp\":4,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.4/32\"},\"state\":{\"destination-address\":\"21.1.1.4/32\",\"dscp\":4,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.4/32\"}},\"sequence-id\":4,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":4},\"transport\":{\"config\":{\"destination-port\":204,\"source-port\":104},\"state\":{\"destination-port\":204,\"source-port\":104}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":5},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.5/32\",\"dscp\":5,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.5/32\"},\"state\":{\"destination-address\":\"21.1.1.5/32\",\"dscp\":5,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.5/32\"}},\"sequence-id\":5,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":5},\"transport\":{\"config\":{\"destination-port\":205,\"source-port\":105},\"state\":{\"destination-port\":205,\"source-port\":105}}}]},\"config\":{\"description\":\"Description for MyACL1\",\"name\":\"MyACL1\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"name\":\"MyACL1\",\"state\":{\"description\":\"Description for MyACL1\",\"name\":\"MyACL1\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"},{\"acl-entries\":{\"acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":1},\"ipv4\":{\"config\":{\"destination-address\":\"22.1.1.1/32\",\"dscp\":1,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.1/32\"},\"state\":{\"destination-address\":\"22.1.1.1/32\",\"dscp\":1,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.1/32\"}},\"sequence-id\":1,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":1},\"transport\":{\"config\":{\"destination-port\":201,\"source-port\":101},\"state\":{\"destination-port\":201,\"source-port\":101}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":2},\"ipv4\":{\"config\":{\"destination-address\":\"22.1.1.2/32\",\"dscp\":2,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.2/32\"},\"state\":{\"destination-address\":\"22.1.1.2/32\",\"dscp\":2,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.2/32\"}},\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2},\"transport\":{\"config\":{\"destination-port\":202,\"source-port\":102},\"state\":{\"destination-port\":202,\"source-port\":102}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":3},\"ipv4\":{\"config\":{\"destination-address\":\"22.1.1.3/32\",\"dscp\":3,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.3/32\"},\"state\":{\"destination-address\":\"22.1.1.3/32\",\"dscp\":3,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.3/32\"}},\"sequence-id\":3,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":3},\"transport\":{\"config\":{\"destination-port\":203,\"source-port\":103},\"state\":{\"destination-port\":203,\"source-port\":103}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":4},\"ipv4\":{\"config\":{\"destination-address\":\"22.1.1.4/32\",\"dscp\":4,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.4/32\"},\"state\":{\"destination-address\":\"22.1.1.4/32\",\"dscp\":4,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.4/32\"}},\"sequence-id\":4,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":4},\"transport\":{\"config\":{\"destination-port\":204,\"source-port\":104},\"state\":{\"destination-port\":204,\"source-port\":104}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":5},\"ipv4\":{\"config\":{\"destination-address\":\"22.1.1.5/32\",\"dscp\":5,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.5/32\"},\"state\":{\"destination-address\":\"22.1.1.5/32\",\"dscp\":5,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"12.1.1.5/32\"}},\"sequence-id\":5,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":5},\"transport\":{\"config\":{\"destination-port\":205,\"source-port\":105},\"state\":{\"destination-port\":205,\"source-port\":105}}}]},\"config\":{\"description\":\"Description for MyACL2\",\"name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"name\":\"MyACL2\",\"state\":{\"description\":\"Description for MyACL2\",\"name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]},\"interfaces\":{\"interface\":[{\"config\":{\"id\":\"Ethernet0\"},\"id\":\"Ethernet0\",\"ingress-acl-sets\":{\"ingress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":1,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":1}},{\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2}},{\"sequence-id\":3,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":3}},{\"sequence-id\":4,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":4}},{\"sequence-id\":5,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":5}}]},\"config\":{\"set-name\":\"MyACL1\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL1\",\"state\":{\"set-name\":\"MyACL1\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]},\"state\":{\"id\":\"Ethernet0\"}},{\"config\":{\"id\":\"Ethernet4\"},\"id\":\"Ethernet4\",\"ingress-acl-sets\":{\"ingress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":1,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":1}},{\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2}},{\"sequence-id\":3,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":3}},{\"sequence-id\":4,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":4}},{\"sequence-id\":5,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":5}}]},\"config\":{\"set-name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL2\",\"state\":{\"set-name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]},\"state\":{\"id\":\"Ethernet4\"}}]}}}"

var oneAclCreateWithRulesJsonRequest string = "{\"acl-set\":[{ \"name\": \"MyACL3\", \"type\": \"ACL_IPV4\", \"config\": { \"name\": \"MyACL3\", \"type\": \"ACL_IPV4\", \"description\": \"Description for MyACL3\" }, \"acl-entries\": { \"acl-entry\": [ { \"sequence-id\": 1, \"config\": { \"sequence-id\": 1, \"description\": \"Description for MyACL3 Rule Seq 1\" }, \"ipv4\": { \"config\": { \"source-address\": \"11.1.1.1/32\", \"destination-address\": \"21.1.1.1/32\", \"dscp\": 1, \"protocol\": \"IP_TCP\" } }, \"transport\": { \"config\": { \"source-port\": 101, \"destination-port\": 201 } }, \"actions\": { \"config\": { \"forwarding-action\": \"ACCEPT\" } } }, { \"sequence-id\": 2, \"config\": { \"sequence-id\": 2, \"description\": \"Description for MyACL3 Rule Seq 2\" }, \"ipv4\": { \"config\": { \"source-address\": \"11.1.1.2/32\", \"destination-address\": \"21.1.1.2/32\", \"dscp\": 2, \"protocol\": \"IP_UDP\" } }, \"transport\": { \"config\": { \"source-port\": 102, \"destination-port\": 202 } }, \"actions\": { \"config\": { \"forwarding-action\": \"DROP\" } } }, { \"sequence-id\": 3, \"config\": { \"sequence-id\": 3, \"description\": \"Description for MyACL3 Rule Seq 3\" }, \"ipv4\": { \"config\": { \"source-address\": \"11.1.1.3/32\", \"destination-address\": \"21.1.1.3/32\", \"dscp\": 3, \"protocol\": \"IP_TCP\" } }, \"transport\": { \"config\": { \"source-port\": 103, \"destination-port\": 203 } }, \"actions\": { \"config\": { \"forwarding-action\": \"ACCEPT\" } } }, { \"sequence-id\": 4, \"config\": { \"sequence-id\": 4, \"description\": \"Description for MyACL3 Rule Seq 4\" }, \"ipv4\": { \"config\": { \"source-address\": \"11.1.1.4/32\", \"destination-address\": \"21.1.1.4/32\", \"dscp\": 4, \"protocol\": \"IP_TCP\" } }, \"transport\": { \"config\": { \"source-port\": 104, \"destination-port\": 204 } }, \"actions\": { \"config\": { \"forwarding-action\": \"DROP\" } } }, { \"sequence-id\": 5, \"config\": { \"sequence-id\": 5, \"description\": \"Description for MyACL3 Rule Seq 5\" }, \"ipv4\": { \"config\": { \"source-address\": \"11.1.1.5/32\", \"destination-address\": \"21.1.1.5/32\", \"dscp\": 5, \"protocol\": \"IP_TCP\" } }, \"transport\": { \"config\": { \"source-port\": 105, \"destination-port\": 205 } }, \"actions\": { \"config\": { \"forwarding-action\": \"ACCEPT\" } } } ] }}]}"

var oneAclCreateWithRulesJsonResponse string = "{\"openconfig-acl:acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":1},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.1/32\",\"dscp\":1,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.1/32\"},\"state\":{\"destination-address\":\"21.1.1.1/32\",\"dscp\":1,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.1/32\"}},\"sequence-id\":1,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":1},\"transport\":{\"config\":{\"destination-port\":201,\"source-port\":101},\"state\":{\"destination-port\":201,\"source-port\":101}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:DROP\"},\"state\":{\"forwarding-action\":\"openconfig-acl:DROP\"}},\"config\":{\"sequence-id\":2},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.2/32\",\"dscp\":2,\"protocol\":\"openconfig-packet-match-types:IP_UDP\",\"source-address\":\"11.1.1.2/32\"},\"state\":{\"destination-address\":\"21.1.1.2/32\",\"dscp\":2,\"protocol\":\"openconfig-packet-match-types:IP_UDP\",\"source-address\":\"11.1.1.2/32\"}},\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2},\"transport\":{\"config\":{\"destination-port\":202,\"source-port\":102},\"state\":{\"destination-port\":202,\"source-port\":102}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":3},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.3/32\",\"dscp\":3,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.3/32\"},\"state\":{\"destination-address\":\"21.1.1.3/32\",\"dscp\":3,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.3/32\"}},\"sequence-id\":3,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":3},\"transport\":{\"config\":{\"destination-port\":203,\"source-port\":103},\"state\":{\"destination-port\":203,\"source-port\":103}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:DROP\"},\"state\":{\"forwarding-action\":\"openconfig-acl:DROP\"}},\"config\":{\"sequence-id\":4},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.4/32\",\"dscp\":4,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.4/32\"},\"state\":{\"destination-address\":\"21.1.1.4/32\",\"dscp\":4,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.4/32\"}},\"sequence-id\":4,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":4},\"transport\":{\"config\":{\"destination-port\":204,\"source-port\":104},\"state\":{\"destination-port\":204,\"source-port\":104}}},{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":5},\"ipv4\":{\"config\":{\"destination-address\":\"21.1.1.5/32\",\"dscp\":5,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.5/32\"},\"state\":{\"destination-address\":\"21.1.1.5/32\",\"dscp\":5,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11.1.1.5/32\"}},\"sequence-id\":5,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":5},\"transport\":{\"config\":{\"destination-port\":205,\"source-port\":105},\"state\":{\"destination-port\":205,\"source-port\":105}}}]},\"config\":{\"description\":\"Description for MyACL3\",\"name\":\"MyACL3\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"name\":\"MyACL3\",\"state\":{\"description\":\"Description for MyACL3\",\"name\":\"MyACL3\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]}"

var oneAclCreateJsonRequest string = "{\"config\": {\"name\": \"MyACL5\",\"type\": \"ACL_IPV4\",\"description\": \"Description for MyACL5\"}}"
var oneAclCreateJsonResponse string = "{\"openconfig-acl:acl-set\":[{\"config\":{\"description\":\"Description for MyACL5\",\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"name\":\"MyACL5\",\"state\":{\"description\":\"Description for MyACL5\",\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]}"
var oneAclCreateJsonStateResponse string = "{\"openconfig-acl:acl-set\":[{\"name\":\"MyACL5\",\"state\":{\"description\":\"Description for MyACL5\",\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]}"
var oneAclCreateJsonConfigResponse string = "{\"openconfig-acl:acl-set\":[{\"config\":{\"description\":\"Description for MyACL5\",\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"}]}"
var oneAclRuleJsonOperationalResponse string = "{\"openconfig-acl:acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\"}}]},\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"}]}"
var ruleMatchedPacketsJsonResponse string = "{\"openconfig-acl:matched-packets\":\"0\"}"

var aclDescrUpdateJson string = "{\"openconfig-acl:description\":\"Verifying ACL Description Update\"}"

var requestOneRulePostJson string = "{\"sequence-id\": 8,\"config\": {\"sequence-id\": 8,\"description\": \"Description for MyACL5 Rule Seq 8\"},\"ipv4\": {\"config\": {\"source-address\": \"4.4.4.4/24\",\"destination-address\": \"5.5.5.5/24\",\"protocol\": \"IP_TCP\"}},\"transport\": {\"config\": {\"source-port\": 101,\"destination-port\": 100,\"tcp-flags\": [\"TCP_FIN\",\"TCP_ACK\"]}},\"actions\": {\"config\": {\"forwarding-action\": \"ACCEPT\"}}}"

var requestOneRulePatchJson string = "{\"acl-entry\": [{\"sequence-id\": 8,\"config\": {\"sequence-id\": 8,\"description\": \"Description for MyACL5 Rule Seq 8\"},\"ipv4\": {\"config\": {\"source-address\": \"4.8.4.8/24\",\"destination-address\": \"15.5.15.5/24\",\"protocol\": \"IP_L2TP\"}},\"transport\": {\"config\": {\"source-port\": 101,\"destination-port\": 100,\"tcp-flags\": [\"TCP_FIN\",\"TCP_ACK\",\"TCP_RST\",\"TCP_ECE\"]}},\"actions\": {\"config\": {\"forwarding-action\": \"ACCEPT\"}}}]}"

var responseOneRuleJson string = "{\"openconfig-acl:acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":8},\"ipv4\":{\"config\":{\"destination-address\":\"5.5.5.5/24\",\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"4.4.4.4/24\"},\"state\":{\"destination-address\":\"5.5.5.5/24\",\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"4.4.4.4/24\"}},\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8},\"transport\":{\"config\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]},\"state\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]}}}]}"

var responseOneRulePatchJson string = "{\"openconfig-acl:acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":8},\"ipv4\":{\"config\":{\"destination-address\":\"15.5.15.5/24\",\"protocol\":\"openconfig-packet-match-types:IP_L2TP\",\"source-address\":\"4.8.4.8/24\"},\"state\":{\"destination-address\":\"15.5.15.5/24\",\"protocol\":\"openconfig-packet-match-types:IP_L2TP\",\"source-address\":\"4.8.4.8/24\"}},\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8},\"transport\":{\"config\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_RST\",\"openconfig-packet-match-types:TCP_ACK\",\"openconfig-packet-match-types:TCP_ECE\"]},\"state\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_RST\",\"openconfig-packet-match-types:TCP_ACK\",\"openconfig-packet-match-types:TCP_ECE\"]}}}]}"

var rulePortRangesUpdateJson string = "{\"source-port\":\"801..811\",\"destination-port\":\"901..921\"}"

var rulePortsUpdateJson string = "{\"source-port\":22,\"destination-port\":80}"

var rulePortRangesGetJson string = "{\"openconfig-acl:config\":{\"destination-port\":\"901..921\",\"source-port\":\"801..811\",\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]}}"

// ICMP match test data. The icmpv4/icmpv6 containers come from
// openconfig-acl-ext, so they read back qualified with that module.
var icmpv4AclCreateJson string = "{\"config\":{\"name\":\"MyACL8\",\"type\":\"ACL_IPV4\",\"description\":\"ICMPv4 match\"}}"
var icmpv4RuleCreateJson string = "{\"sequence-id\":8,\"config\":{\"sequence-id\":8},\"ipv4\":{\"config\":{\"protocol\":\"IP_ICMP\"},\"icmpv4\":{\"config\":{\"type\":5,\"code\":1}}},\"actions\":{\"config\":{\"forwarding-action\":\"DROP\"}}}"
var icmpv4ConfigGetJson string = "{\"openconfig-acl-ext:config\":{\"code\":1,\"type\":5}}"
var icmpv4TypeUpdateJson string = "{\"type\":8}"

var icmpv6AclCreateJson string = "{\"config\":{\"name\":\"MyACL9\",\"type\":\"ACL_IPV6\",\"description\":\"ICMPv6 match\"}}"
var icmpv6RuleCreateJson string = "{\"sequence-id\":6,\"config\":{\"sequence-id\":6},\"ipv6\":{\"config\":{\"protocol\":58},\"icmpv6\":{\"config\":{\"type\":\"openconfig-icmpv6-types:ECHO_REPLY\"}}},\"actions\":{\"config\":{\"forwarding-action\":\"DROP\"}}}"

// DESTINATION_UNREACHABLE is type 1 and DST_UNREACHABLE_PORT its code 4.
var icmpv6TypeCodeJson string = "{\"type\":\"openconfig-icmpv6-types:DESTINATION_UNREACHABLE\",\"code\":\"openconfig-icmpv6-types:DST_UNREACHABLE_PORT\"}"
var icmpv6TypeCodeGetJson string = "{\"openconfig-acl-ext:config\":{\"code\":\"openconfig-icmpv6-types:DST_UNREACHABLE_PORT\",\"type\":\"openconfig-icmpv6-types:DESTINATION_UNREACHABLE\"}}"

// TIME_EXCEEDED_HOP_LIMIT is a code of type 3, not of DESTINATION_UNREACHABLE.
var icmpv6MismatchedCodeJson string = "{\"type\":\"openconfig-icmpv6-types:DESTINATION_UNREACHABLE\",\"code\":\"openconfig-icmpv6-types:TIME_EXCEEDED_HOP_LIMIT\"}"

// DST_UNREACHABLE_DST_ADMIN_PROHIBITED is code 1 of DESTINATION_UNREACHABLE.
var icmpv6CodeOnlyJson string = "{\"code\":\"openconfig-icmpv6-types:DST_UNREACHABLE_DST_ADMIN_PROHIBITED\"}"
var icmpv6TypeOnlyJson string = "{\"type\":\"openconfig-icmpv6-types:ECHO_REPLY\"}"
var icmpv6RuleNoTypeJson string = "{\"sequence-id\":7,\"config\":{\"sequence-id\":7},\"ipv6\":{\"config\":{\"protocol\":58}},\"actions\":{\"config\":{\"forwarding-action\":\"DROP\"}}}"

// DST_UNREACHABLE_CODE is the identity the codes of DESTINATION_UNREACHABLE
// are grouped under; ECHO_REQUEST_CODE is the one of ECHO_REQUEST.
var icmpv6GroupCodeJson string = "{\"type\":\"openconfig-icmpv6-types:DESTINATION_UNREACHABLE\",\"code\":\"openconfig-icmpv6-types:DST_UNREACHABLE_CODE\"}"
var icmpv6OtherGroupCodeJson string = "{\"code\":\"openconfig-icmpv6-types:ECHO_REQUEST_CODE\"}"

var logActionAclCreateJson string = "{\"config\":{\"name\":\"MyACL10\",\"type\":\"ACL_IPV6\",\"description\":\"log-action\"}}"
var logSyslogRuleCreateJson string = "{\"sequence-id\":10,\"config\":{\"sequence-id\":10},\"ipv6\":{\"config\":{\"protocol\":58},\"icmpv6\":{\"config\":{\"type\":\"openconfig-icmpv6-types:ECHO_REQUEST\",\"code\":\"openconfig-icmpv6-types:ECHO_REQUEST_NO_CODE\"}}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\",\"log-action\":\"LOG_SYSLOG\"}}}"
var logNoneRuleCreateJson string = "{\"sequence-id\":10,\"config\":{\"sequence-id\":10},\"ipv6\":{\"config\":{\"protocol\":58},\"icmpv6\":{\"config\":{\"type\":\"openconfig-icmpv6-types:ECHO_REQUEST\",\"code\":\"openconfig-icmpv6-types:ECHO_REQUEST_NO_CODE\"}}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\",\"log-action\":\"LOG_NONE\"}}}"
var logSyslogActionUpdateJson string = "{\"forwarding-action\":\"ACCEPT\",\"log-action\":\"LOG_SYSLOG\"}"
var logSyslogAclReplaceJson string = "{\"acl-set\":[{\"name\":\"MyACL10\",\"type\":\"ACL_IPV6\",\"config\":{\"name\":\"MyACL10\",\"type\":\"ACL_IPV6\"},\"acl-entries\":{\"acl-entry\":[" + logSyslogRuleCreateJson + "]}}]}"

var emptyAclDescriptionJson string = "{\"openconfig-acl:description\":\"\"}"
var emptyRuleDscpJson string = "{\"openconfig-acl:dscp\":0}"

var ingressAclSetCreateJsonRequest string = "{ \"openconfig-acl:config\": { \"set-name\": \"MyACL5\", \"type\": \"ACL_IPV4\" }}"
var ingressAclSetCreateJsonResponse string = "{\"openconfig-acl:ingress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8}}]},\"config\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL5\",\"state\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]}"

var egressAclSetCreateJsonResponse string = "{\"openconfig-acl:egress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8}}]},\"config\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL5\",\"state\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]}"

var replaceMultiRulesWithOneRuleJsonRequest string = "{\"acl-set\": [{\"name\": \"MyACL3\",\"type\": \"ACL_IPV4\",\"config\": {\"name\": \"MyACL3\",\"type\": \"ACL_IPV4\",\"description\": \"Description for MyACL3\"},\"acl-entries\": {\"acl-entry\": [{\"sequence-id\": 8,\"config\": {\"sequence-id\": 8,\"description\": \"Description for MyACL3 Rule Seq 8\"},\"ipv4\": {\"config\": {\"source-address\": \"81.1.1.1/32\",\"destination-address\": \"91.1.1.1/32\",\"protocol\": \"IP_TCP\"}},\"transport\": {\"config\": {\"source-port\": \"801..811\",\"destination-port\": \"901..921\"}},\"actions\": {\"config\": {\"forwarding-action\": \"REJECT\"}}}]}}]}"

var replaceMultiRulesWithOneRuleJsonResponse string = "{\"openconfig-acl:acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:DROP\"},\"state\":{\"forwarding-action\":\"openconfig-acl:DROP\"}},\"config\":{\"sequence-id\":8},\"ipv4\":{\"config\":{\"destination-address\":\"91.1.1.1/32\",\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"81.1.1.1/32\"},\"state\":{\"destination-address\":\"91.1.1.1/32\",\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"81.1.1.1/32\"}},\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8},\"transport\":{\"config\":{\"destination-port\":\"901..921\",\"source-port\":\"801..811\"},\"state\":{\"destination-port\":\"901..921\",\"source-port\":\"801..811\"}}}]},\"config\":{\"description\":\"Description for MyACL3\",\"name\":\"MyACL3\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"name\":\"MyACL3\",\"state\":{\"description\":\"Description for MyACL3\",\"name\":\"MyACL3\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]}"

var getFromAclSetsTreeLevelResponse string = "{\"openconfig-acl:acl-sets\":{\"acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":8},\"ipv4\":{\"config\":{\"destination-address\":\"5.5.5.5/24\",\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"4.4.4.4/24\"},\"state\":{\"destination-address\":\"5.5.5.5/24\",\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"4.4.4.4/24\"}},\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8},\"transport\":{\"config\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]},\"state\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]}}}]},\"config\":{\"description\":\"Description for MyACL5\",\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"name\":\"MyACL5\",\"state\":{\"description\":\"Description for MyACL5\",\"name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]}}"

var getAllPortsFromInterfacesTreeLevelResponse string = "{\"openconfig-acl:interfaces\":{\"interface\":[{\"config\":{\"id\":\"Ethernet4\"},\"egress-acl-sets\":{\"egress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8}}]},\"config\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL5\",\"state\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]},\"id\":\"Ethernet4\",\"state\":{\"id\":\"Ethernet4\"}}]}}"

var getPortBindingFromInterfaceTreeLevelResponse string = "{\"openconfig-acl:interface\":[{\"config\":{\"id\":\"Ethernet4\"},\"egress-acl-sets\":{\"egress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8}}]},\"config\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL5\",\"state\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]},\"id\":\"Ethernet4\",\"state\":{\"id\":\"Ethernet4\"}}]}"

var getBindingAclEntryResponse string = "{\"openconfig-acl:acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8}}]}"

var getMultiportBindingOnSingleAclResponse string = "{\"openconfig-acl:interfaces\":{\"interface\":[{\"config\":{\"id\":\"Ethernet0\"},\"egress-acl-sets\":{\"egress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8}}]},\"config\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL5\",\"state\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]},\"id\":\"Ethernet0\",\"state\":{\"id\":\"Ethernet0\"}},{\"config\":{\"id\":\"Ethernet4\"},\"egress-acl-sets\":{\"egress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":8,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":8}}]},\"config\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"set-name\":\"MyACL5\",\"state\":{\"set-name\":\"MyACL5\",\"type\":\"openconfig-acl:ACL_IPV4\"},\"type\":\"openconfig-acl:ACL_IPV4\"}]},\"id\":\"Ethernet4\",\"state\":{\"id\":\"Ethernet4\"}}]}}"

var oneIPv6AclCreateJsonRequest string = "{\"config\": {\"name\": \"MyACL6\",\"type\": \"ACL_IPV6\",\"description\": \"Description for IPv6 ACL MyACL6\"}}"
var oneIPv6AclCreateJsonResponse string = "{\"openconfig-acl:acl-set\":[{\"config\":{\"description\":\"Description for IPv6 ACL MyACL6\",\"name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"name\":\"MyACL6\",\"state\":{\"description\":\"Description for IPv6 ACL MyACL6\",\"name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"type\":\"openconfig-acl:ACL_IPV6\"}]}"

var oneIPv6RuleCreateJsonRequest string = "{\"sequence-id\": 6,\"config\": {\"sequence-id\": 6,\"description\": \"Description for MyACL6 Rule Seq 6\"},\"ipv6\": {\"config\": {\"source-address\": \"11::67/64\",\"destination-address\": \"22::87/64\",\"protocol\": \"IP_TCP\",\"dscp\": 11}},\"transport\": {\"config\": {\"source-port\": 101,\"destination-port\": 100,\"tcp-flags\": [\"TCP_FIN\",\"TCP_ACK\"]}},\"actions\": {\"config\": {\"forwarding-action\": \"ACCEPT\"}}}"
var oneIPv6RuleCreateJsonResponse string = "{\"openconfig-acl:acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":6},\"ipv6\":{\"config\":{\"destination-address\":\"22::87/64\",\"dscp\":11,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11::67/64\"},\"state\":{\"destination-address\":\"22::87/64\",\"dscp\":11,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11::67/64\"}},\"sequence-id\":6,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":6},\"transport\":{\"config\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]},\"state\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]}}}]}"

var ingressIPv6AclSetCreateJsonRequest string = "{ \"openconfig-acl:config\": { \"set-name\": \"MyACL6\", \"type\": \"ACL_IPV6\" }}"
var ingressIPv6AclSetCreateJsonResponse string = "{\"openconfig-acl:ingress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":6,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":6}}]},\"config\":{\"set-name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"set-name\":\"MyACL6\",\"state\":{\"set-name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"type\":\"openconfig-acl:ACL_IPV6\"}]}"

var getIPv6AclsFromAclSetListLevelResponse string = "{\"openconfig-acl:acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":6},\"ipv6\":{\"config\":{\"destination-address\":\"22::87/64\",\"dscp\":11,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11::67/64\"},\"state\":{\"destination-address\":\"22::87/64\",\"dscp\":11,\"protocol\":\"openconfig-packet-match-types:IP_TCP\",\"source-address\":\"11::67/64\"}},\"sequence-id\":6,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":6},\"transport\":{\"config\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]},\"state\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]}}}]},\"config\":{\"description\":\"Description for IPv6 ACL MyACL6\",\"name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"name\":\"MyACL6\",\"state\":{\"description\":\"Description for IPv6 ACL MyACL6\",\"name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"type\":\"openconfig-acl:ACL_IPV6\"}]}"

var getIPv6AllPortsBindingsResponse string = "{\"openconfig-acl:interfaces\":{\"interface\":[{\"config\":{\"id\":\"Ethernet4\"},\"id\":\"Ethernet4\",\"ingress-acl-sets\":{\"ingress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":6,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":6}}]},\"config\":{\"set-name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"set-name\":\"MyACL6\",\"state\":{\"set-name\":\"MyACL6\",\"type\":\"openconfig-acl:ACL_IPV6\"},\"type\":\"openconfig-acl:ACL_IPV6\"}]},\"state\":{\"id\":\"Ethernet4\"}}]}}"

var oneL2AclCreateJsonRequest string = "{\"config\": {\"name\": \"MyACL2\",\"type\": \"ACL_L2\",\"description\": \"Description for L2 ACL MyACL2\"}}"
var oneL2AclCreateJsonResponse string = "{\"openconfig-acl:acl-set\":[{\"config\":{\"description\":\"Description for L2 ACL MyACL2\",\"name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"name\":\"MyACL2\",\"state\":{\"description\":\"Description for L2 ACL MyACL2\",\"name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"type\":\"openconfig-acl:ACL_L2\"}]}"

var oneL2RuleCreateJsonRequest string = "{\"sequence-id\": 2,\"config\": {\"sequence-id\": 2,\"description\": \"Description for MyACL2 Rule Seq 2\"},\"l2\": {\"config\": {\"ethertype\": \"ETHERTYPE_VLAN\"}},\"transport\": {\"config\": {\"source-port\": 101,\"destination-port\": 100,\"tcp-flags\": [\"TCP_FIN\",\"TCP_ACK\"]}},\"actions\": {\"config\": {\"forwarding-action\": \"ACCEPT\"}}}"

var oneL2RuleCreateJsonResponse string = "{\"openconfig-acl:acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":2},\"l2\":{\"config\":{\"ethertype\":\"openconfig-packet-match-types:ETHERTYPE_VLAN\"},\"state\":{\"ethertype\":\"openconfig-packet-match-types:ETHERTYPE_VLAN\"}},\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2},\"transport\":{\"config\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]},\"state\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]}}}]}"

var ingressL2AclSetCreateJsonRequest string = "{ \"openconfig-acl:config\": { \"set-name\": \"MyACL2\", \"type\": \"ACL_L2\" }}"
var ingressL2AclSetCreateJsonResponse string = "{\"openconfig-acl:ingress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2}}]},\"config\":{\"set-name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"set-name\":\"MyACL2\",\"state\":{\"set-name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"type\":\"openconfig-acl:ACL_L2\"}]}"

var getL2AclsFromAclSetListLevelResponse string = "{\"openconfig-acl:acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":2},\"l2\":{\"config\":{\"ethertype\":\"openconfig-packet-match-types:ETHERTYPE_VLAN\"},\"state\":{\"ethertype\":\"openconfig-packet-match-types:ETHERTYPE_VLAN\"}},\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2},\"transport\":{\"config\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]},\"state\":{\"destination-port\":100,\"source-port\":101,\"tcp-flags\":[\"openconfig-packet-match-types:TCP_FIN\",\"openconfig-packet-match-types:TCP_ACK\"]}}}]},\"config\":{\"description\":\"Description for L2 ACL MyACL2\",\"name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"name\":\"MyACL2\",\"state\":{\"description\":\"Description for L2 ACL MyACL2\",\"name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"type\":\"openconfig-acl:ACL_L2\"}]}"

var getL2AllPortsBindingsResponse string = "{\"openconfig-acl:interfaces\":{\"interface\":[{\"config\":{\"id\":\"Ethernet0\"},\"id\":\"Ethernet0\",\"ingress-acl-sets\":{\"ingress-acl-set\":[{\"acl-entries\":{\"acl-entry\":[{\"sequence-id\":2,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":2}}]},\"config\":{\"set-name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"set-name\":\"MyACL2\",\"state\":{\"set-name\":\"MyACL2\",\"type\":\"openconfig-acl:ACL_L2\"},\"type\":\"openconfig-acl:ACL_L2\"}]},\"state\":{\"id\":\"Ethernet0\"}}]}}"

var aclCreateWithInvalidInterfaceBinding string = "{ \"acl-sets\": { \"acl-set\": [ { \"name\": \"MyACL1\", \"type\": \"ACL_IPV4\", \"config\": { \"name\": \"MyACL1\", \"type\": \"ACL_IPV4\", \"description\": \"Description for MyACL1\" }, \"acl-entries\": { \"acl-entry\": [ { \"sequence-id\": 1, \"config\": { \"sequence-id\": 1, \"description\": \"Description for MyACL1 Rule Seq 1\" }, \"ipv4\": { \"config\": { \"source-address\": \"11.1.1.1/32\", \"destination-address\": \"21.1.1.1/32\", \"dscp\": 1, \"protocol\": \"IP_TCP\" } }, \"transport\": { \"config\": { \"source-port\": 101, \"destination-port\": 201 } }, \"actions\": { \"config\": { \"forwarding-action\": \"ACCEPT\" } } } ] } } ] }, \"interfaces\": { \"interface\": [ { \"id\": \"Ethernet2112\", \"config\": { \"id\": \"Ethernet2112\" }, \"interface-ref\": { \"config\": { \"interface\": \"Ethernet2112\" } }, \"ingress-acl-sets\": { \"ingress-acl-set\": [ { \"set-name\": \"MyACL1\", \"type\": \"ACL_IPV4\", \"config\": { \"set-name\": \"MyACL1\", \"type\": \"ACL_IPV4\" } } ] } } ] }}"

var requestOneDuplicateRulePostJson string = "{\"sequence-id\": 1,\"config\": {\"sequence-id\": 1,\"description\": \"Description for MyACL3 Rule Seq 1\"},\"ipv4\": {\"config\": {\"source-address\": \"4.4.4.4/24\",\"destination-address\": \"5.5.5.5/24\",\"protocol\": \"IP_TCP\"}},\"transport\": {\"config\": {\"source-port\": 101,\"destination-port\": 100,\"tcp-flags\": [\"TCP_FIN\",\"TCP_ACK\"]}},\"actions\": {\"config\": {\"forwarding-action\": \"ACCEPT\"}}}"
