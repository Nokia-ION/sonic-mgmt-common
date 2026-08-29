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
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

/* This file includes common utilities for app module test code. */

// CleanupFunc is the callback function used for cleaning config_db
// entries before and after tests. Should be registered via addCleanupFunc.
type CleanupFunc func() error

var cleanupFuncs map[string]CleanupFunc

// addCleanupFunc registers a cleanup function. Should be called from
// init() of individual test files if they need pre/post cleanup.
func addCleanupFunc(name string, f CleanupFunc) {
	if cleanupFuncs == nil {
		cleanupFuncs = map[string]CleanupFunc{}
	}

	cleanupFuncs[name] = f
}

func TestMain(m *testing.M) {
	// cleanup before tests
	for name, f := range cleanupFuncs {
		if err := f(); err != nil {
			glog.Errorf("%s cleanup failed; err=%v", name, err)
			os.Exit(-1)
		} else {
			glog.Infof("Cleanup %s before tests", name)
		}
	}

	ret := m.Run()

	// cleanup after tests
	for name, f := range cleanupFuncs {
		if err := f(); err != nil {
			glog.Warningf("Cleanup %s failed; err=%v", name, err)
		} else {
			glog.Infof("Cleanup %s after tests", name)
		}
	}

	os.Exit(ret)
}

func processGetRequest(url string, expectedRespJson string, errorCase bool) func(*testing.T) {
	return func(t *testing.T) {
		t.Run("ietf_json", func(t *testing.T) {
			verifyGet(t, GetRequest{Path: url}, expectedRespJson, errorCase)
		})
		t.Run("ygot", func(t *testing.T) {
			verifyGet(t, GetRequest{Path: url, FmtType: TRANSLIB_FMT_YGOT}, expectedRespJson, errorCase)
		})
	}
}

func verifyGet(t *testing.T, req GetRequest, expJson string, expError bool) {
	t.Helper()
	response, err := Get(req)
	switch {
	case err != nil && !expError:
		t.Fatalf("Error %v received for Url: %s", err, req.Path)
	case err == nil && expError:
		t.Fatalf("GET %s did not return an error", req.Path)
	case expError:
		return
	}

	var respJson []byte
	if req.FmtType == TRANSLIB_FMT_YGOT && response.ValueTree != nil {
		respJson, err = dumpIetfJson(response.ValueTree)
		if err != nil {
			t.Fatalf("GET %s returned invalid YGOT. error=%v", req.Path, err)
		}
	} else if req.FmtType == TRANSLIB_FMT_IETF_JSON {
		respJson = response.Payload
	}

	var jResponse, jExpected map[string]interface{}
	if err := json.Unmarshal(respJson, &jResponse); err != nil {
		t.Fatalf("invalid response json; err = %v\npayload = %s", err, respJson)
	}
	if err := json.Unmarshal([]byte(expJson), &jExpected); err != nil {
		t.Fatalf("invalid expected json; err = %v", err)
	}
	if !reflect.DeepEqual(jResponse, jExpected) {
		t.Errorf("GET %s returned invalid response", req.Path)
		t.Errorf("Expected: %s", expJson)
		t.Fatalf("Received: %s", respJson)
	}
}

func processSetRequest(url string, jsonPayload string, oper string, errorCase bool) func(*testing.T) {
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
			t.Errorf("Operation not supported")
		}
		if err != nil && !errorCase {
			t.Errorf("Error %v received for Url: %s", err, url)
		}
	}
}

func processDeleteRequest(url string) func(*testing.T) {
	return func(t *testing.T) {
		_, err := Delete(SetRequest{Path: url})
		if err != nil {
			t.Errorf("Error %v received for Url: %s", err, url)
		}
	}
}

func getConfigDb() *db.DB {
	configDb, _ := db.NewDB(db.Options{
		DBNo:                    db.ConfigDB,
		TableNameSeparator:      "|",
		KeySeparator:            "|",
		ForceNewRedisConnection: false,
	})

	return configDb
}

func TestOCContentMismatch(t *testing.T) {
	tests := []struct {
		template string
		content  string
		want     bool
	}{
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}", content: "state", want: false},
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/config", content: "state", want: true},
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/config/description", content: "state", want: true},
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/state", content: "config", want: true},
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/state/description", content: "config", want: true},
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/state", content: "state", want: false},
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/state", content: "operational", want: false},
		// state leaves mirroring a config leaf are not operational data
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/state/description", content: "operational", want: true},
		{template: "/openconfig-acl:acl/acl-sets/acl-set{}{}/acl-entries/acl-entry{}/state/matched-packets",
			content: "operational", want: false},
		{template: "/openconfig-acl:acl/acl-sets", content: "all", want: false},
		{template: "/openconfig-acl:acl/acl-sets", content: "", want: false},
		{template: "/openconfig-acl:acl/unknown-node", content: "state", want: false},
	}
	for _, tt := range tests {
		if got := ocContentMismatch(tt.template, toOCContentType(tt.content)); got != tt.want {
			t.Errorf("ocContentMismatch(%q, %q) = %v, want %v", tt.template, tt.content, got, tt.want)
		}
	}
}

// newAclTestTree builds an ACL having one rule, with both config and state
// filled in, the way AclApp fills it during a GET.
func newAclTestTree(t *testing.T) (*ocbinds.Device, *ocbinds.OpenconfigAcl_Acl_AclSets_AclSet) {
	t.Helper()
	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)
	aclSet, err := device.Acl.AclSets.NewAclSet("MyACL5", ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4)
	if err != nil {
		t.Fatal(err)
	}
	ygot.BuildEmptyTree(aclSet)
	name := "MyACL5"
	descr := "Description for MyACL5"
	aclSet.Config.Name = &name
	aclSet.Config.Description = &descr
	aclSet.State.Name = &name
	aclSet.State.Description = &descr

	rule, err := aclSet.AclEntries.NewAclEntry(8)
	if err != nil {
		t.Fatal(err)
	}
	ygot.BuildEmptyTree(rule)
	seq := uint32(8)
	counter := uint64(0)
	rule.Config.SequenceId = &seq
	rule.State.SequenceId = &seq
	rule.State.MatchedPackets = &counter
	rule.State.MatchedOctets = &counter

	return device, aclSet
}

func TestPruneOCContentState(t *testing.T) {
	device, aclSet := newAclTestTree(t)
	pruneOCContent(device, ocContentNonConfig)

	if aclSet.Config != nil {
		t.Errorf("STATE prune left config: %+v", aclSet.Config)
	}
	if aclSet.State == nil || aclSet.State.Description == nil {
		t.Errorf("STATE prune dropped state: %+v", aclSet.State)
	}
	if aclSet.Name == nil || aclSet.Type == ocbinds.OpenconfigAcl_ACL_TYPE_UNSET {
		t.Errorf("STATE prune dropped list keys: name=%v type=%v", aclSet.Name, aclSet.Type)
	}
	rule := aclSet.AclEntries.AclEntry[8]
	if rule == nil {
		t.Fatalf("STATE prune dropped acl-entry")
	}
	if rule.Config != nil {
		t.Errorf("STATE prune left rule config: %+v", rule.Config)
	}
	if rule.State == nil || rule.State.MatchedPackets == nil || rule.State.SequenceId == nil {
		t.Errorf("STATE prune dropped rule state: %+v", rule.State)
	}
}

func TestPruneOCContentConfig(t *testing.T) {
	device, aclSet := newAclTestTree(t)
	pruneOCContent(device, ocContentConfig)

	if aclSet.State != nil {
		t.Errorf("CONFIG prune left state: %+v", aclSet.State)
	}
	if aclSet.Config == nil || aclSet.Config.Description == nil {
		t.Errorf("CONFIG prune dropped config: %+v", aclSet.Config)
	}
	rule := aclSet.AclEntries.AclEntry[8]
	if rule == nil {
		t.Fatalf("CONFIG prune dropped acl-entry")
	}
	if rule.State != nil {
		t.Errorf("CONFIG prune left rule state: %+v", rule.State)
	}
	if rule.Config == nil || rule.Config.SequenceId == nil {
		t.Errorf("CONFIG prune dropped rule config: %+v", rule.Config)
	}
}

func TestPruneOCContentOperational(t *testing.T) {
	device, aclSet := newAclTestTree(t)
	pruneOCContent(device, ocContentOperational)

	if aclSet.Config != nil {
		t.Errorf("OPERATIONAL prune left config: %+v", aclSet.Config)
	}
	// All acl-set state leaves mirror a config leaf, so the container goes away.
	if aclSet.State != nil {
		t.Errorf("OPERATIONAL prune left non-operational state: %+v", aclSet.State)
	}
	rule := aclSet.AclEntries.AclEntry[8]
	if rule == nil {
		t.Fatalf("OPERATIONAL prune dropped acl-entry")
	}
	if rule.State == nil || rule.State.MatchedPackets == nil || rule.State.MatchedOctets == nil {
		t.Fatalf("OPERATIONAL prune dropped counters: %+v", rule.State)
	}
	if rule.State.SequenceId != nil {
		t.Errorf("OPERATIONAL prune left non-operational state/sequence-id")
	}
}

func TestPruneOCContentEmptySubtree(t *testing.T) {
	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)
	aclSet, err := device.Acl.AclSets.NewAclSet("MyACL5", ocbinds.OpenconfigAcl_ACL_TYPE_ACL_IPV4)
	if err != nil {
		t.Fatal(err)
	}
	ygot.BuildEmptyTree(aclSet)
	name := "MyACL5"
	aclSet.Config.Name = &name

	// Nothing but the list keys is left after the prune, hence the whole
	// acl-sets subtree must be dropped.
	pruneOCContent(device, ocContentNonConfig)
	if device.Acl != nil && device.Acl.AclSets != nil {
		t.Errorf("STATE prune retained empty acl-sets: %+v", device.Acl.AclSets)
	}
}
