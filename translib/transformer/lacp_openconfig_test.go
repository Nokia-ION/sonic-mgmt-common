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

//go:build testapp
// +build testapp

package transformer_test

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/redis/go-redis/v9"
)

const lacpTestMember = "Ethernet0"

// actor state bits: 0=activity, 1=timeout, 2=aggregatable, 3=sync, 4=collecting, 5=distributing
const lacpActorStateAllUp = "61" // bits 0,2,3,4,5

func lacpPortChannelPrereq(pcName string) map[string]interface{} {
	return map[string]interface{}{
		"PORTCHANNEL": map[string]interface{}{
			pcName: map[string]interface{}{
				"admin_status": "up",
			},
		},
	}
}

func loadLacpPortChannel(pcName string) {
	loadDB(db.ConfigDB, lacpPortChannelPrereq(pcName))
	time.Sleep(1 * time.Second)
}

func unloadLacpPortChannel(pcName string) {
	unloadDB(db.ConfigDB, lacpPortChannelPrereq(pcName))
	time.Sleep(1 * time.Second)
}

func seedPortChannelField(pcName, field, value string) {
	key := "PORTCHANNEL|" + pcName
	if _, err := rclient.HSet(context.Background(), key, field, value).Result(); err != nil {
		panic(err)
	}
	time.Sleep(1 * time.Second)
}

func seedLacpMemberState(t *testing.T, pcName string) *redis.Client {
	t.Helper()

	stateClient := getDbClient(int(db.StateDB))
	if stateClient == nil {
		t.Fatalf("Failed to get StateDB redis client")
	}

	lagTableKey := "LAG_TABLE|" + pcName
	if _, err := stateClient.HSet(context.Background(), lagTableKey, "setup.runner.name", "runner").Result(); err != nil {
		t.Fatalf("Failed to seed %s: %v", lagTableKey, err)
	}

	lagMemberKey := "LAG_MEMBER_TABLE|" + pcName + "|" + lacpTestMember
	fields := map[string]interface{}{
		"runner.actor_lacpdu_info.state":    lacpActorStateAllUp,
		"runner.actor_lacpdu_info.system":   "00:11:22:33:44:55",
		"runner.actor_lacpdu_info.key":      "123",
		"runner.partner_lacpdu_info.system": "00:aa:bb:cc:dd:ee",
		"runner.partner_lacpdu_info.key":    "456",
	}
	if _, err := stateClient.HMSet(context.Background(), lagMemberKey, fields).Result(); err != nil {
		t.Fatalf("Failed to seed %s: %v", lagMemberKey, err)
	}
	time.Sleep(1 * time.Second)
	return stateClient
}

func cleanupLacpMemberState(stateClient *redis.Client, pcName string) {
	if stateClient == nil {
		return
	}
	stateClient.Del(context.Background(), "LAG_TABLE|"+pcName)
	stateClient.Del(context.Background(), "LAG_MEMBER_TABLE|"+pcName+"|"+lacpTestMember)
	db.CloseRedisClient(stateClient)
}

// Test_openconfig_lacp_config_interval covers GET of config/interval from PORTCHANNEL
// fast_rate in CONFIG_DB (written by YangToDb_lacp_intfs_xfmr on SET).
func Test_openconfig_lacp_config_interval(t *testing.T) {
	const pcName = "PortChannel113"
	intervalURL := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName + "]/config/interval"

	t.Log("\n\n+++++++++++++ LACP CONFIG INTERVAL ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	t.Log("\n\n--- GET interval SLOW (fast_rate=false) ---")
	seedPortChannelField(pcName, "fast_rate", "false")
	t.Run("GET interval SLOW", processGetRequest(intervalURL, nil,
		"{\"openconfig-lacp:interval\": \"SLOW\"}", false))

	t.Log("\n\n--- GET interval FAST (fast_rate=true) ---")
	seedPortChannelField(pcName, "fast_rate", "true")
	t.Run("GET interval FAST", processGetRequest(intervalURL, nil,
		"{\"openconfig-lacp:interval\": \"FAST\"}", false))
}

// Test_openconfig_lacp_config_fallback covers GET of config/fallback from PORTCHANNEL
// fallback in CONFIG_DB (written by YangToDb_lacp_intfs_xfmr on SET).
func Test_openconfig_lacp_config_fallback(t *testing.T) {
	const pcName = "PortChannel114"
	fallbackURL := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName + "]/config/fallback"

	t.Log("\n\n+++++++++++++ LACP CONFIG FALLBACK ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	t.Log("\n\n--- GET fallback true ---")
	seedPortChannelField(pcName, "fallback", "true")
	t.Run("GET fallback true", processGetRequest(fallbackURL, nil,
		"{\"openconfig-lacp:fallback\": true}", false))

	t.Log("\n\n--- GET fallback false ---")
	seedPortChannelField(pcName, "fallback", "false")
	t.Run("GET fallback false", processGetRequest(fallbackURL, nil,
		"{\"openconfig-lacp:fallback\": false}", false))
}

// Test_openconfig_lacp_config_lacp_mode covers SET/GET of config/lacp-mode mapped to
// PORTCHANNEL.active in CONFIG_DB (written by YangToDb_lacp_intfs_xfmr on SET).
func Test_openconfig_lacp_config_lacp_mode(t *testing.T) {
	const pcName = "PortChannel116"
	lacpModeURL := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName + "]/config/lacp-mode"

	t.Log("\n\n+++++++++++++ LACP CONFIG LACP-MODE ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	t.Log("\n\n--- SET lacp-mode PASSIVE ---")
	t.Run("SET lacp-mode PASSIVE", processSetRequest(lacpModeURL,
		`{"openconfig-lacp:lacp-mode": "PASSIVE"}`, "PATCH", false, nil))

	key := "PORTCHANNEL|" + pcName
	active, err := rclient.HGet(context.Background(), key, "active").Result()
	if err != nil {
		t.Fatalf("Failed to read PORTCHANNEL.active for %s: %v", pcName, err)
	}
	if active != "false" {
		t.Fatalf("Expected PORTCHANNEL.active=false after PASSIVE SET, got %q", active)
	}

	t.Run("GET lacp-mode PASSIVE", processGetRequest(lacpModeURL, nil,
		`{"openconfig-lacp:lacp-mode": "PASSIVE"}`, false))

	t.Log("\n\n--- SET lacp-mode ACTIVE ---")
	t.Run("SET lacp-mode ACTIVE", processSetRequest(lacpModeURL,
		`{"openconfig-lacp:lacp-mode": "ACTIVE"}`, "PATCH", false, nil))

	active, err = rclient.HGet(context.Background(), key, "active").Result()
	if err != nil {
		t.Fatalf("Failed to read PORTCHANNEL.active for %s: %v", pcName, err)
	}
	if active != "true" {
		t.Fatalf("Expected PORTCHANNEL.active=true after ACTIVE SET, got %q", active)
	}

	t.Run("GET lacp-mode ACTIVE", processGetRequest(lacpModeURL, nil,
		`{"openconfig-lacp:lacp-mode": "ACTIVE"}`, false))
}

// Test_openconfig_lacp_member_state covers GET of member state leaves derived from
// teamd-populated STATE_DB LAG_MEMBER_TABLE entries.
func Test_openconfig_lacp_member_state(t *testing.T) {
	const pcName = "PortChannel115"

	t.Log("\n\n+++++++++++++ LACP MEMBER STATE ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	stateClient := seedLacpMemberState(t, pcName)
	defer cleanupLacpMemberState(stateClient, pcName)

	memberBase := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName +
		"]/members/member[interface=" + lacpTestMember + "]/state"

	t.Run("GET aggregatable", processGetRequest(memberBase+"/aggregatable", nil,
		"{\"openconfig-lacp:aggregatable\": true}", false))
	t.Run("GET collecting", processGetRequest(memberBase+"/collecting", nil,
		"{\"openconfig-lacp:collecting\": true}", false))
	t.Run("GET distributing", processGetRequest(memberBase+"/distributing", nil,
		"{\"openconfig-lacp:distributing\": true}", false))
	t.Run("GET oper-key", processGetRequest(memberBase+"/oper-key", nil,
		"{\"openconfig-lacp:oper-key\": 123}", false))
	t.Run("GET partner-key", processGetRequest(memberBase+"/partner-key", nil,
		"{\"openconfig-lacp:partner-key\": 456}", false))
	t.Run("GET system-id", processGetRequest(memberBase+"/system-id", nil,
		"{\"openconfig-lacp:system-id\": \"00:11:22:33:44:55\"}", false))
	t.Run("GET partner-id", processGetRequest(memberBase+"/partner-id", nil,
		"{\"openconfig-lacp:partner-id\": \"00:aa:bb:cc:dd:ee\"}", false))
}
