//go:build testapp
// +build testapp

package transformer_test

import (
	"testing"
	"time"

	"github.com/Azure/sonic-mgmt-common/translib/db"
)

func Test_openconfig_routed_vlan_ipv6_ra_prefix_get(t *testing.T) {
	const ifName = "Vlan100"
	const ocPrefix = "2001:db8:104::/64"
	preReqMap := map[string]interface{}{
		"VLAN_INTERFACE": map[string]interface{}{
			ifName + "|" + ocPrefix: map[string]interface{}{
				"NULL":                              "NULL",
				"ipv6_ra_prefix_preferred_lifetime": "600",
			},
		},
	}
	loadDB(db.ConfigDB, preReqMap)
	defer unloadDB(db.ConfigDB, map[string]interface{}{
		"VLAN_INTERFACE": map[string]interface{}{ifName + "|" + ocPrefix: ""},
	})

	base := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=" + ocPrefix + "]"

	t.Run("GET prefix config container", processGetRequest(base+"/config", nil,
		`{"openconfig-if-ip:config":{"prefix":"2001:db8:104::/64","preferred-lifetime":600}}`, false))
	time.Sleep(100 * time.Millisecond)

	t.Run("GET preferred-lifetime leaf", processGetRequest(base+"/config/preferred-lifetime", nil,
		`{"openconfig-if-ip:preferred-lifetime":600}`, false))
	time.Sleep(100 * time.Millisecond)

	hostBase := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8:104::1/64]"
	t.Run("GET host-style key config container", processGetRequest(hostBase+"/config", nil,
		`{"openconfig-if-ip:config":{"prefix":"2001:db8:104::/64","preferred-lifetime":600}}`, false))
	time.Sleep(100 * time.Millisecond)

	t.Run("GET host-style key state container", processGetRequest(hostBase+"/state", nil,
		`{"openconfig-if-ip:state":{"prefix":"2001:db8:104::/64","preferred-lifetime":600}}`, false))
	time.Sleep(100 * time.Millisecond)

	t.Run("GET host-style key instance", processGetRequest(hostBase, nil,
		`{"openconfig-if-ip:prefix":[{"prefix":"2001:db8:104::1/64","config":{"prefix":"2001:db8:104::/64","preferred-lifetime":600},"state":{"prefix":"2001:db8:104::/64","preferred-lifetime":600}}]}`, false))
	time.Sleep(100 * time.Millisecond)

	t.Run("GET with host-style list key", processGetRequest(
		"/openconfig-interfaces:interfaces/interface[name="+ifName+"]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8:104::1/64]/config/preferred-lifetime",
		nil, `{"openconfig-if-ip:preferred-lifetime":600}`, false))
}
