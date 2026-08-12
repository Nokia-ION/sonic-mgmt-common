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
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
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

// portChannelField reads a PORTCHANNEL field from ConfigDB, returning "" when absent.
func portChannelField(t *testing.T, pcName, field string) string {
	t.Helper()
	val, err := rclient.HGet(context.Background(), "PORTCHANNEL|"+pcName, field).Result()
	if err == redis.Nil {
		return ""
	}
	if err != nil {
		t.Fatalf("Failed to read PORTCHANNEL.%s for %s: %v", field, pcName, err)
	}
	return val
}

// Test_openconfig_lacp_config_fallback_timeout covers SET/GET of config/fallback-timeout
// mapped to PORTCHANNEL.fallback_timeout in CONFIG_DB.
func Test_openconfig_lacp_config_fallback_timeout(t *testing.T) {
	const pcName = "PortChannel117"
	base := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName + "]/config"

	t.Log("\n\n+++++++++++++ LACP CONFIG FALLBACK-TIMEOUT ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	t.Log("\n\n--- SET fallback and fallback-timeout together ---")
	t.Run("SET fallback + timeout", processSetRequest(base,
		`{"openconfig-lacp:config": {"fallback": true, "fallback-timeout": 120}}`,
		"PATCH", false, nil))

	if got := portChannelField(t, pcName, "fallback_timeout"); got != "120" {
		t.Fatalf("Expected PORTCHANNEL.fallback_timeout=120, got %q", got)
	}

	t.Run("GET fallback-timeout", processGetRequest(base+"/fallback-timeout", nil,
		`{"openconfig-lacp:fallback-timeout": 120}`, false))
}

// Test_openconfig_lacp_config_fallback_timeout_default verifies that enabling fallback
// without an explicit timeout adopts the 90 second default.
func Test_openconfig_lacp_config_fallback_timeout_default(t *testing.T) {
	const pcName = "PortChannel118"
	base := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName + "]/config"

	t.Log("\n\n+++++++++++++ LACP CONFIG FALLBACK-TIMEOUT DEFAULT ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	t.Run("SET fallback only", processSetRequest(base+"/fallback",
		`{"openconfig-lacp:fallback": true}`, "PATCH", false, nil))

	if got := portChannelField(t, pcName, "fallback_timeout"); got != "90" {
		t.Fatalf("Expected default PORTCHANNEL.fallback_timeout=90, got %q", got)
	}

	t.Run("GET fallback-timeout default", processGetRequest(base+"/fallback-timeout", nil,
		`{"openconfig-lacp:fallback-timeout": 90}`, false))
}

// Test_openconfig_lacp_config_fallback_timeout_requires_fallback verifies that a
// fallback-timeout SET is rejected unless fallback is enabled.
func Test_openconfig_lacp_config_fallback_timeout_requires_fallback(t *testing.T) {
	const pcName = "PortChannel119"
	timeoutURL := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName +
		"]/config/fallback-timeout"

	t.Log("\n\n+++++++++++++ LACP FALLBACK-TIMEOUT WITHOUT FALLBACK ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	expectedErr := tlerr.InvalidArgsError{
		Format: "fallback-timeout requires fallback to be enabled on " + pcName}
	t.Run("SET timeout without fallback", processSetRequest(timeoutURL,
		`{"openconfig-lacp:fallback-timeout": 120}`, "PATCH", true, expectedErr))

	if got := portChannelField(t, pcName, "fallback_timeout"); got != "" {
		t.Fatalf("Expected PORTCHANNEL.fallback_timeout to stay unset, got %q", got)
	}
}

// Test_openconfig_lacp_config_fallback_timeout_runtime verifies that the timeout
// can be changed after the PortChannel already carries one.
func Test_openconfig_lacp_config_fallback_timeout_runtime(t *testing.T) {
	const pcName = "PortChannel120"
	base := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName + "]/config"

	t.Log("\n\n+++++++++++++ LACP FALLBACK-TIMEOUT RUNTIME CHANGE ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	t.Run("SET fallback + timeout", processSetRequest(base,
		`{"openconfig-lacp:config": {"fallback": true, "fallback-timeout": 120}}`,
		"PATCH", false, nil))

	t.Run("SET timeout again", processSetRequest(base+"/fallback-timeout",
		`{"openconfig-lacp:fallback-timeout": 200}`, "PATCH", false, nil))

	if got := portChannelField(t, pcName, "fallback_timeout"); got != "200" {
		t.Fatalf("Expected PORTCHANNEL.fallback_timeout=200, got %q", got)
	}

	t.Run("GET updated fallback-timeout", processGetRequest(base+"/fallback-timeout", nil,
		`{"openconfig-lacp:fallback-timeout": 200}`, false))
}

// Test_openconfig_lacp_config_fallback_timeout_toggle_preserves verifies that
// disabling fallback keeps fallback_timeout in CONFIG_DB and re-enabling without
// an explicit timeout does not reset it to the default.
func Test_openconfig_lacp_config_fallback_timeout_toggle_preserves(t *testing.T) {
	const pcName = "PortChannel122"
	base := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName + "]/config"

	t.Log("\n\n+++++++++++++ LACP FALLBACK-TIMEOUT TOGGLE PRESERVE ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	t.Run("SET fallback + timeout", processSetRequest(base,
		`{"openconfig-lacp:config": {"fallback": true, "fallback-timeout": 30}}`,
		"PATCH", false, nil))

	if got := portChannelField(t, pcName, "fallback_timeout"); got != "30" {
		t.Fatalf("Expected PORTCHANNEL.fallback_timeout=30, got %q", got)
	}

	t.Run("SET fallback false", processSetRequest(base+"/fallback",
		`{"openconfig-lacp:fallback": false}`, "PATCH", false, nil))

	if got := portChannelField(t, pcName, "fallback_timeout"); got != "30" {
		t.Fatalf("Expected PORTCHANNEL.fallback_timeout=30 after disable, got %q", got)
	}

	t.Run("SET fallback true", processSetRequest(base+"/fallback",
		`{"openconfig-lacp:fallback": true}`, "PATCH", false, nil))

	if got := portChannelField(t, pcName, "fallback_timeout"); got != "30" {
		t.Fatalf("Expected PORTCHANNEL.fallback_timeout=30 after re-enable, got %q", got)
	}

	t.Run("GET fallback-timeout", processGetRequest(base+"/fallback-timeout", nil,
		`{"openconfig-lacp:fallback-timeout": 30}`, false))
}

// Test_openconfig_lacp_state_fallback_timeout covers GET of state/fallback-timeout,
// which teamd publishes into STATE_DB LAG_TABLE as runner.fallback_timeout.
func Test_openconfig_lacp_state_fallback_timeout(t *testing.T) {
	const pcName = "PortChannel121"
	stateURL := "/openconfig-lacp:lacp/interfaces/interface[name=" + pcName +
		"]/state/fallback-timeout"

	t.Log("\n\n+++++++++++++ LACP STATE FALLBACK-TIMEOUT ++++++++++++")
	loadLacpPortChannel(pcName)
	defer unloadLacpPortChannel(pcName)

	stateClient := getDbClient(int(db.StateDB))
	if stateClient == nil {
		t.Fatalf("Failed to get StateDB redis client")
	}
	defer func() {
		stateClient.Del(context.Background(), "LAG_TABLE|"+pcName)
		db.CloseRedisClient(stateClient)
	}()

	fields := map[string]interface{}{
		"runner.fallback":         "true",
		"runner.fallback_timeout": "120",
	}
	if _, err := stateClient.HMSet(context.Background(), "LAG_TABLE|"+pcName, fields).Result(); err != nil {
		t.Fatalf("Failed to seed LAG_TABLE|%s: %v", pcName, err)
	}
	time.Sleep(1 * time.Second)

	t.Run("GET state fallback-timeout", processGetRequest(stateURL, nil,
		`{"openconfig-lacp:fallback-timeout": 120}`, false))
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
