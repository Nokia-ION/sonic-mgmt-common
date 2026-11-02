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
	"reflect"
	"testing"

	db "github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
)

func init() {
	addCleanupFunc("PORT_SET", clearPortSetDataFromDb)
}

func clearPortSetDataFromDb() error {
	d := getConfigDb()
	if d == nil {
		return errors.New("Failed to connect to config Db")
	}
	defer d.DeleteDB()

	if err := d.DeleteTable(&db.TableSpec{Name: RULE_PORT_SET_TABLE}); err != nil {
		return errors.New("Failed to delete " + RULE_PORT_SET_TABLE)
	}
	if err := d.DeleteTable(&db.TableSpec{Name: PORT_SET_TABLE}); err != nil {
		return errors.New("Failed to delete " + PORT_SET_TABLE)
	}
	return nil
}

func Test_PortNumRangeToDb(t *testing.T) {
	tests := []struct {
		port    string
		want    string
		wantErr bool
	}{
		{port: "80", want: "80"},
		{port: "0", want: "0"},
		{port: "65535", want: "65535"},
		{port: "734..1733", want: "734-1733"},
		{port: "1024..1024", want: "1024-1024"},
		// A port set narrows the match, so a wildcard has no meaning in one.
		{port: "ANY", wantErr: true},
		{port: "65536", wantErr: true},
		{port: "1733..734", wantErr: true},
		{port: "734..", wantErr: true},
		{port: "http", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.port, func(t *testing.T) {
			got, err := portNumRangeToDb(tc.port)
			if (err != nil) != tc.wantErr {
				t.Fatalf("portNumRangeToDb(%q) error = %v, wantErr %v", tc.port, err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("portNumRangeToDb(%q) = %q, want %q", tc.port, got, tc.want)
			}
		})
	}
}

func Test_PortNumRangeToOc(t *testing.T) {
	if got := portNumRangeToOc("734-1733"); got != "734..1733" {
		t.Errorf("range = %v (%T), want \"734..1733\"", got, got)
	}
	// A single port is a number in the openconfig union, not a string.
	if got := portNumRangeToOc("80"); got != uint16(80) {
		t.Errorf("port = %v (%T), want uint16(80)", got, got)
	}
}

func Test_SetRulePortMatch(t *testing.T) {
	tests := []struct {
		name    string
		isSrc   bool
		portVal string
		// fields already on the rule, to check the other half of the
		// L4_x_PORT / L4_x_PORT_RANGE choice is cleared
		existing map[string]string
		want     map[string]string
	}{{
		name:    "destination port",
		portVal: "80",
		want:    map[string]string{L4_DST_PORT: "80"},
	}, {
		name:    "destination range",
		portVal: "734-1733",
		want:    map[string]string{L4_DST_PORT_RANGE: "734-1733"},
	}, {
		name:    "source port",
		isSrc:   true,
		portVal: "22",
		want:    map[string]string{L4_SRC_PORT: "22"},
	}, {
		name:     "range replaces a single port",
		portVal:  "734-1733",
		existing: map[string]string{L4_DST_PORT: "80"},
		want:     map[string]string{L4_DST_PORT_RANGE: "734-1733"},
	}, {
		name:     "single port replaces a range",
		portVal:  "80",
		existing: map[string]string{L4_DST_PORT_RANGE: "734-1733"},
		want:     map[string]string{L4_DST_PORT: "80"},
	}, {
		name:     "source match leaves the destination match alone",
		isSrc:    true,
		portVal:  "22",
		existing: map[string]string{L4_DST_PORT: "80"},
		want:     map[string]string{L4_SRC_PORT: "22", L4_DST_PORT: "80"},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ruleData := db.Value{Field: map[string]string{}}
			for k, v := range tc.existing {
				ruleData.Field[k] = v
			}

			setRulePortMatch(ruleData, tc.isSrc, tc.portVal)

			if len(ruleData.Field) != len(tc.want) {
				t.Fatalf("fields = %v, want %v", ruleData.Field, tc.want)
			}
			for k, v := range tc.want {
				if ruleData.Field[k] != v {
					t.Errorf("field %s = %q, want %q", k, ruleData.Field[k], v)
				}
			}
		})
	}
}

// processSetRequestErr is processSetRequest for the cases that have to be
// rejected; processSetRequest only tolerates an error, it does not require one.
func processSetRequestErr(url string, jsonPayload string, oper string) func(*testing.T) {
	return processSetRequestErrOfType(url, jsonPayload, oper, nil)
}

// processSetRequestErrOfType is processSetRequestErr that also requires the
// error to be of the same type as want, which decides the gRPC code the client
// sees. A nil want accepts any error.
func processSetRequestErrOfType(url string, jsonPayload string, oper string, want error) func(*testing.T) {
	return func(t *testing.T) {
		var err error
		switch oper {
		case "POST":
			_, err = Create(SetRequest{Path: url, Payload: []byte(jsonPayload)})
		case "PATCH":
			_, err = Update(SetRequest{Path: url, Payload: []byte(jsonPayload)})
		case "PUT":
			_, err = Replace(SetRequest{Path: url, Payload: []byte(jsonPayload)})
		case "DELETE":
			_, err = Delete(SetRequest{Path: url})
		default:
			t.Fatalf("Operation %s not supported", oper)
		}
		if err == nil {
			t.Errorf("%s %s did not return an error", oper, url)
			return
		}
		t.Logf("%s %s rejected with: %v", oper, url, err)
		if want != nil && reflect.TypeOf(err) != reflect.TypeOf(want) {
			t.Errorf("%s %s returned %T, want %T", oper, url, err, want)
		}
	}
}

// verifyRuleFields checks the ACL_RULE row of a rule. An empty expected value
// means the field must not be present.
func verifyRuleFields(aclKey, ruleKey string, want map[string]string) func(*testing.T) {
	return func(t *testing.T) {
		d := getConfigDb()
		if d == nil {
			t.Fatal("Failed to connect to config Db")
		}
		defer d.DeleteDB()

		entry, err := d.GetEntry(&db.TableSpec{Name: RULE_TABLE}, db.Key{Comp: []string{aclKey, ruleKey}})
		if err != nil {
			t.Fatalf("Failed to read %s|%s|%s; err=%v", RULE_TABLE, aclKey, ruleKey, err)
		}
		for field, value := range want {
			if got := (&entry).Get(field); got != value {
				t.Errorf("%s|%s|%s field %s = %q, want %q", RULE_TABLE, aclKey, ruleKey, field, got, value)
			}
		}
	}
}

func verifyAclTablePorts(aclKey, port string, wantBound bool) func(*testing.T) {
	return func(t *testing.T) {
		d := getConfigDb()
		if d == nil {
			t.Fatal("Failed to connect to config Db")
		}
		defer d.DeleteDB()

		entry, err := d.GetEntry(&db.TableSpec{Name: ACL_TABLE}, db.Key{Comp: []string{aclKey}})
		if err != nil {
			t.Fatalf("Failed to read %s|%s; err=%v", ACL_TABLE, aclKey, err)
		}
		ports := entry.GetList("ports")
		found := contains(ports, port)
		if found != wantBound {
			t.Errorf("%s ports %v, port %q bound=%v want %v", aclKey, ports, port, found, wantBound)
		}
	}
}

func Test_DefinedSetsApp_PortSetOperations(t *testing.T) {
	url := portSetsUrl + "[name=DST-PORTS-TEST]"

	t.Run("Create_Port_Set_With_Range", processSetRequest(url, portSetRangeCreateJson, "POST", false))
	t.Run("Verify_Port_Set_With_Range", processGetRequest(url, portSetRangeGetJson, false))
	t.Run("Get_All_Port_Sets", processGetRequest(definedSetsUrl, definedSetsGetJson, false))

	t.Run("Create_Existing_Port_Set", processSetRequestErr(url, portSetRangeCreateJson, "POST"))

	descrUrl := url + "/config/description"
	t.Run("Update_Description", processSetRequest(url+"/config", portSetDescrUpdateJson, "PATCH", false))
	t.Run("Verify_Description", processGetRequest(descrUrl, portSetDescrUpdateJson, false))
	t.Run("Delete_Description", processDeleteRequest(descrUrl))
	t.Run("Verify_Description_Deletion", processGetRequest(descrUrl, emptyPortSetDescriptionJson, false))

	t.Run("Replace_With_Single_Port", processSetRequest(url, portSetSinglePortReplaceJson, "PUT", false))
	t.Run("Verify_Single_Port", processGetRequest(url, portSetSinglePortGetJson, false))

	// A replace without ports empties the set, which is allowed while no rule
	// matches on it.
	t.Run("Replace_Without_Ports", processSetRequest(url, portSetNoPortsReplaceJson, "PUT", false))
	t.Run("Verify_Ports_Cleared", processGetRequest(url, portSetNoPortsGetJson, false))

	t.Run("Delete_Port_Set", processDeleteRequest(url))
	t.Run("Verify_Port_Set_Deletion", processGetRequest(url, "", true))
}

func Test_DefinedSetsApp_NegativeTests(t *testing.T) {
	t.Run("Get_Unknown_Port_Set", processGetRequest(portSetsUrl+"[name=NO-SUCH-SET]", "", true))

	// A delete of something that is not there succeeds and does nothing.
	t.Run("Delete_Unknown_Port_Set", processDeleteRequest(portSetsUrl+"[name=NO-SUCH-SET]"))
	t.Run("Delete_Description_Of_Unknown_Port_Set",
		processDeleteRequest(portSetsUrl+"[name=NO-SUCH-SET]/config/description"))
	t.Run("Delete_Ports_Of_Unknown_Port_Set",
		processDeleteRequest(portSetsUrl+"[name=NO-SUCH-SET]/config/port"))

	// Everything a single ACL_RULE port match cannot express.
	t.Run("Create_Port_Set_With_Two_Ports",
		processSetRequestErrOfType(portSetsUrl+"[name=TWO-PORTS]", portSetTwoPortsCreateJson, "POST",
			tlerr.InvalidArgsError{}))
	t.Run("Create_Port_Set_With_Exclude_Port",
		processSetRequestErr(portSetsUrl+"[name=EXCLUDE-PORT]", portSetExcludePortCreateJson, "POST"))
	t.Run("Create_Port_Set_Matching_Any_Port",
		processSetRequestErr(portSetsUrl+"[name=ANY-PORT]", portSetAnyPortCreateJson, "POST"))
	t.Run("Create_Port_Set_With_Inverted_Range",
		processSetRequestErr(portSetsUrl+"[name=BAD-RANGE]", portSetInvertedRangeCreateJson, "POST"))
	t.Run("Create_Port_Set_With_Mismatching_Name",
		processSetRequestErr(portSetsUrl+"[name=NAME-MISMATCH]", portSetRangeCreateJson, "POST"))

	// The prefix sets of the model have no backend here, and their list key is
	// called "name" too, so a request for one must not be taken for a request
	// for the port set of the same name.
	t.Run("Get_Prefix_Set", processGetRequest(prefixSetsUrl+"[name=DST-PORTS-TEST]", "", true))
	t.Run("Create_Prefix_Set",
		processSetRequestErr(prefixSetsUrl+"[name=DST-PORTS-TEST]", prefixSetCreateJson, "POST"))
	t.Run("Delete_Prefix_Sets", processSetRequestErr(definedSetsUrl+"/ipv4-prefix-sets", "", "DELETE"))

	t.Run("Verify_Nothing_Was_Created", processGetRequest(definedSetsUrl, emptyJson, false))
}

// Test_DefinedSetsApp_PortSetRules covers an ACL rule matching on a port set:
// the rule is programmed with the port the set resolves to, and the set it came
// from is reported back on a GET.
func Test_DefinedSetsApp_PortSetRules(t *testing.T) {
	setUrl := portSetsUrl + "[name=DST-PORTS-TEST]"
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL7][type=ACL_IPV4]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=9]"

	t.Run("Create_Port_Set", processSetRequest(setUrl, portSetRangeCreateJson, "POST", false))
	t.Run("Create_Acl", processSetRequest(aclUrl, aclForPortSetCreateJson, "POST", false))

	t.Run("Create_Rule_Matching_Unknown_Port_Set",
		processSetRequestErrOfType(ruleUrl, ruleUnknownPortSetPostJson, "POST", tlerr.NotFoundError{}))
	t.Run("Create_Rule_With_Port_And_Port_Set",
		processSetRequestErr(ruleUrl, rulePortAndPortSetPostJson, "POST"))

	t.Run("Create_Rule_Matching_Port_Set", processSetRequest(ruleUrl, rulePortSetPostJson, "POST", false))
	// The set resolves to a range, so the rule gets the range field. The set
	// name stays out of ACL_RULE, orchagent would drop a rule carrying it.
	t.Run("Verify_Rule_Port_Match", verifyRuleFields("MyACL7_ACL_IPV4", "RULE_9",
		map[string]string{L4_DST_PORT_RANGE: "734-1733", L4_DST_PORT: "", RULE_DST_PORT_SET: ""}))
	t.Run("Verify_Rule_Read_Back", processGetRequest(ruleUrl, rulePortSetGetJson, false))

	t.Run("Delete_Port_Set_In_Use",
		processSetRequestErrOfType(setUrl, "", "DELETE", tlerr.FailedPreconditionError{}))
	t.Run("Delete_Ports_Of_Port_Set_In_Use",
		processSetRequestErrOfType(setUrl+"/config/port", "", "DELETE", tlerr.FailedPreconditionError{}))
	t.Run("Empty_Port_Set_In_Use",
		processSetRequestErrOfType(setUrl, portSetNoPortsReplaceJson, "PUT", tlerr.FailedPreconditionError{}))

	// The rules matching on a set hold the resolved port, so they have to
	// follow a change of its contents.
	t.Run("Replace_Port_Set_With_Single_Port", processSetRequest(setUrl, portSetSinglePortReplaceJson, "PUT", false))
	t.Run("Verify_Rule_Port_Match_Follows_Port_Set", verifyRuleFields("MyACL7_ACL_IPV4", "RULE_9",
		map[string]string{L4_DST_PORT: "1234", L4_DST_PORT_RANGE: ""}))

	// The port match came from the set, so it goes away with the reference.
	t.Run("Stop_Matching_On_Port_Set", processDeleteRequest(ruleUrl+"/transport/config/destination-port-set"))
	t.Run("Verify_Rule_Port_Match_Is_Gone", verifyRuleFields("MyACL7_ACL_IPV4", "RULE_9",
		map[string]string{L4_DST_PORT: "", L4_DST_PORT_RANGE: ""}))
	t.Run("Verify_Rule_Read_Back_Without_Port_Set", processGetRequest(ruleUrl, ruleWithoutPortSetGetJson, false))
	t.Run("Delete_Unused_Port_Set", processDeleteRequest(setUrl))

	t.Run("Delete_Acl", processDeleteRequest(aclUrl))
}

// Test_DefinedSetsApp_SwitchPortSets moves a rule from a port set holding a
// range to one holding a single port, which changes which half of the rule's L4
// port choice is used.
func Test_DefinedSetsApp_SwitchPortSets(t *testing.T) {
	rangeSetUrl := portSetsUrl + "[name=RANGE-SET]"
	portSetUrl := portSetsUrl + "[name=PORT-SET]"
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL7][type=ACL_IPV4]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=9]"

	t.Run("Create_Range_Set", processSetRequest(rangeSetUrl, rangePortSetCreateJson, "POST", false))
	t.Run("Create_Port_Set", processSetRequest(portSetUrl, singlePortSetCreateJson, "POST", false))
	t.Run("Create_Acl", processSetRequest(aclUrl, aclForPortSetCreateJson, "POST", false))

	t.Run("Create_Rule_Matching_Range_Set", processSetRequest(ruleUrl, ruleRangeSetPostJson, "POST", false))
	t.Run("Verify_Range_Match", verifyRuleFields("MyACL7_ACL_IPV4", "RULE_9",
		map[string]string{L4_DST_PORT_RANGE: "901-921", L4_DST_PORT: ""}))

	t.Run("Match_On_Single_Port_Set_Instead", processSetRequest(ruleUrl+"/transport/config",
		rulePortSetUpdateJson, "PATCH", false))
	t.Run("Verify_Port_Match", verifyRuleFields("MyACL7_ACL_IPV4", "RULE_9",
		map[string]string{L4_DST_PORT: "80", L4_DST_PORT_RANGE: ""}))
	t.Run("Verify_New_Reference", processGetRequest(ruleUrl+"/transport/config/destination-port-set",
		rulePortSetRefGetJson, false))
	// The rule no longer matches on the range set, so that one is free to go.
	t.Run("Delete_Range_Set", processDeleteRequest(rangeSetUrl))
	t.Run("Delete_Port_Set_Still_In_Use", processSetRequestErr(portSetUrl, "", "DELETE"))

	t.Run("Delete_Acl", processDeleteRequest(aclUrl))
	t.Run("Delete_Port_Set", processDeleteRequest(portSetUrl))
}

// Test_DefinedSetsApp_PortSetRefCleanup checks that deleting a rule or its ACL
// takes the port set references with it, leaving the set free to be deleted.
func Test_DefinedSetsApp_PortSetRefCleanup(t *testing.T) {
	setUrl := portSetsUrl + "[name=DST-PORTS-TEST]"
	aclUrl := "/openconfig-acl:acl/acl-sets/acl-set[name=MyACL7][type=ACL_IPV4]"
	ruleUrl := aclUrl + "/acl-entries/acl-entry[sequence-id=9]"

	t.Run("Create_Port_Set", processSetRequest(setUrl, portSetRangeCreateJson, "POST", false))
	t.Run("Create_Acl", processSetRequest(aclUrl, aclForPortSetCreateJson, "POST", false))
	t.Run("Create_Rule_Matching_Port_Set", processSetRequest(ruleUrl, rulePortSetPostJson, "POST", false))

	t.Run("Delete_Rule", processDeleteRequest(ruleUrl))
	t.Run("Delete_Port_Set_After_Rule_Delete", processDeleteRequest(setUrl))

	t.Run("Recreate_Port_Set", processSetRequest(setUrl, portSetRangeCreateJson, "POST", false))
	t.Run("Recreate_Rule_Matching_Port_Set", processSetRequest(ruleUrl, rulePortSetPostJson, "POST", false))

	t.Run("Delete_Acl", processDeleteRequest(aclUrl))
	t.Run("Delete_Port_Set_After_Acl_Delete", processDeleteRequest(setUrl))
}

const (
	definedSetsUrl = "/openconfig-defined-sets:defined-sets"
	portSetsUrl    = definedSetsUrl + "/port-sets/port-set"
	prefixSetsUrl  = definedSetsUrl + "/ipv4-prefix-sets/ipv4-prefix-set"
)

var prefixSetCreateJson string = "{\"config\":{\"name\":\"DST-PORTS-TEST\",\"prefix\":[\"10.0.0.0/8\"]}}"

var portSetRangeCreateJson string = "{\"config\":{\"name\":\"DST-PORTS-TEST\",\"port\":[\"734..1733\"],\"description\":\"Ports under test\"}}"

var portSetRangeGetJson string = "{\"openconfig-defined-sets:port-set\":[{\"config\":{\"description\":\"Ports under test\",\"name\":\"DST-PORTS-TEST\",\"port\":[\"734..1733\"]},\"name\":\"DST-PORTS-TEST\",\"state\":{\"description\":\"Ports under test\",\"name\":\"DST-PORTS-TEST\",\"port\":[\"734..1733\"]}}]}"

var definedSetsGetJson string = "{\"openconfig-defined-sets:defined-sets\":{\"port-sets\":{\"port-set\":[{\"config\":{\"description\":\"Ports under test\",\"name\":\"DST-PORTS-TEST\",\"port\":[\"734..1733\"]},\"name\":\"DST-PORTS-TEST\",\"state\":{\"description\":\"Ports under test\",\"name\":\"DST-PORTS-TEST\",\"port\":[\"734..1733\"]}}]}}}"

// A single port comes back as a number, a range as a string; both are members
// of the openconfig port-num-range union.
var portSetSinglePortGetJson string = "{\"openconfig-defined-sets:port-set\":[{\"config\":{\"name\":\"DST-PORTS-TEST\",\"port\":[1234]},\"name\":\"DST-PORTS-TEST\",\"state\":{\"name\":\"DST-PORTS-TEST\",\"port\":[1234]}}]}"

// The rule reports both the port its set resolved to and the set it came from.
var rulePortSetGetJson string = "{\"openconfig-acl:acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":9},\"ipv4\":{\"config\":{\"protocol\":\"openconfig-packet-match-types:IP_TCP\"},\"state\":{\"protocol\":\"openconfig-packet-match-types:IP_TCP\"}},\"sequence-id\":9,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":9},\"transport\":{\"config\":{\"destination-port-set\":\"DST-PORTS-TEST\"},\"state\":{\"destination-port-set\":\"DST-PORTS-TEST\"}}}]}"

var ruleWithoutPortSetGetJson string = "{\"openconfig-acl:acl-entry\":[{\"actions\":{\"config\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"},\"state\":{\"forwarding-action\":\"openconfig-acl:ACCEPT\"}},\"config\":{\"sequence-id\":9},\"ipv4\":{\"config\":{\"protocol\":\"openconfig-packet-match-types:IP_TCP\"},\"state\":{\"protocol\":\"openconfig-packet-match-types:IP_TCP\"}},\"sequence-id\":9,\"state\":{\"matched-octets\":\"0\",\"matched-packets\":\"0\",\"sequence-id\":9}}]}"

var portSetDescrUpdateJson string = "{\"openconfig-defined-sets:description\":\"Verifying port set description update\"}"

var emptyPortSetDescriptionJson string = "{\"openconfig-defined-sets:description\":\"\"}"

var portSetSinglePortReplaceJson string = "{\"config\":{\"name\":\"DST-PORTS-TEST\",\"port\":[1234]}}"

var portSetNoPortsReplaceJson string = "{\"config\":{\"name\":\"DST-PORTS-TEST\",\"description\":\"No ports\"}}"

var portSetNoPortsGetJson string = "{\"openconfig-defined-sets:port-set\":[{\"config\":{\"description\":\"No ports\",\"name\":\"DST-PORTS-TEST\"},\"name\":\"DST-PORTS-TEST\",\"state\":{\"description\":\"No ports\",\"name\":\"DST-PORTS-TEST\"}}]}"

var portSetTwoPortsCreateJson string = "{\"config\":{\"name\":\"TWO-PORTS\",\"port\":[80,443]}}"

var portSetExcludePortCreateJson string = "{\"config\":{\"name\":\"EXCLUDE-PORT\",\"port\":[\"1..1024\"],\"exclude-port\":[22]}}"

var portSetAnyPortCreateJson string = "{\"config\":{\"name\":\"ANY-PORT\",\"port\":[\"ANY\"]}}"

var portSetInvertedRangeCreateJson string = "{\"config\":{\"name\":\"BAD-RANGE\",\"port\":[\"1733..734\"]}}"

var aclForPortSetCreateJson string = "{\"config\":{\"name\":\"MyACL7\",\"type\":\"ACL_IPV4\",\"description\":\"Acl matching on a port set\"}}"

var rulePortSetPostJson string = "{\"sequence-id\":9,\"config\":{\"sequence-id\":9},\"ipv4\":{\"config\":{\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"destination-port-set\":\"DST-PORTS-TEST\"}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}}"

var rangePortSetCreateJson string = "{\"config\":{\"name\":\"RANGE-SET\",\"port\":[\"901..921\"]}}"

var singlePortSetCreateJson string = "{\"config\":{\"name\":\"PORT-SET\",\"port\":[80]}}"

var ruleRangeSetPostJson string = "{\"sequence-id\":9,\"config\":{\"sequence-id\":9},\"ipv4\":{\"config\":{\"protocol\":\"IP_TCP\"}},\"transport\":{\"config\":{\"destination-port-set\":\"RANGE-SET\"}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}}"

var rulePortSetUpdateJson string = "{\"destination-port-set\":\"PORT-SET\"}"

var rulePortSetRefGetJson string = "{\"openconfig-acl:destination-port-set\":\"PORT-SET\"}"

var ruleUnknownPortSetPostJson string = "{\"sequence-id\":9,\"config\":{\"sequence-id\":9},\"transport\":{\"config\":{\"destination-port-set\":\"NO-SUCH-SET\"}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}}"

var rulePortAndPortSetPostJson string = "{\"sequence-id\":9,\"config\":{\"sequence-id\":9},\"transport\":{\"config\":{\"destination-port\":80,\"destination-port-set\":\"DST-PORTS-TEST\"}},\"actions\":{\"config\":{\"forwarding-action\":\"ACCEPT\"}}}"
