package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/ygot/ygot"
)

func TestIpv6RaCfgTableName(t *testing.T) {
	tests := []struct {
		name    string
		ifName  string
		index   string
		want    string
		wantErr bool
	}{
		{name: "ethernet index 0", ifName: "Ethernet1", index: "0", want: "INTERFACE"},
		{name: "ethernet empty index", ifName: "Ethernet1", index: "", want: "INTERFACE"},
		{name: "vlan", ifName: "Vlan100", index: "", want: "VLAN_INTERFACE"},
		{name: "portchannel", ifName: "PortChannel1", index: "0", want: "PORTCHANNEL_INTERFACE"},
		{name: "vlan subinterface", ifName: "Ethernet0", index: "10", want: "VLAN_SUB_INTERFACE"},
		{name: "loopback rejected", ifName: "Loopback0", index: "0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ipv6RaCfgTableName(tt.ifName, tt.index)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ipv6RaCfgTableName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ipv6RaCfgTableName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIpv6RaDbKey(t *testing.T) {
	if got := ipv6RaDbKey("Ethernet1", "0"); got != "Ethernet1" {
		t.Fatalf("index 0 key = %q", got)
	}
	if got := ipv6RaDbKey("Ethernet0", "10"); got != "Ethernet0.10" {
		t.Fatalf("subif key = %q", got)
	}
}

func TestYangToDbIpv6RaInterval(t *testing.T) {
	interval := uint32(60)
	got, err := YangToDb_ipv6_ra_interval_xfmr(XfmrParams{
		uri:   "/openconfig-interfaces:interfaces/interface[name=Ethernet1]/subinterfaces/subinterface[index=0]/ipv6/router-advertisement/config/interval",
		param: &interval,
		oper:  UPDATE,
	})
	if err != nil {
		t.Fatalf("YangToDb_ipv6_ra_interval_xfmr() error = %v", err)
	}
	want := map[string]string{ipv6RaFieldInterval: "60"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestYangToDbIpv6RaVlanAndSubif(t *testing.T) {
	lifetime := uint32(1800)
	got, err := YangToDb_ipv6_ra_lifetime_xfmr(XfmrParams{
		uri:   "/openconfig-interfaces:interfaces/interface[name=Vlan100]/openconfig-vlan:routed-vlan/ipv6/router-advertisement/config/lifetime",
		param: &lifetime,
		oper:  UPDATE,
	})
	if err != nil {
		t.Fatalf("vlan lifetime xfmr error = %v", err)
	}
	if got[ipv6RaFieldLifetime] != "1800" {
		t.Fatalf("vlan lifetime = %v", got)
	}

	suppress := true
	got, err = YangToDb_ipv6_ra_suppress_xfmr(XfmrParams{
		uri:   "/openconfig-interfaces:interfaces/interface[name=Ethernet0]/subinterfaces/subinterface[index=10]/ipv6/router-advertisement/config/suppress",
		param: &suppress,
		oper:  UPDATE,
	})
	if err != nil {
		t.Fatalf("subif suppress xfmr error = %v", err)
	}
	if got[ipv6RaFieldSuppress] != "true" {
		t.Fatalf("subif suppress = %v", got)
	}
}

func TestYangToDbIpv6RaPrefixValidLifetime(t *testing.T) {
	lifetime := uint32(3600)
	got, err := YangToDb_ipv6_ra_prefix_valid_lifetime_xfmr(XfmrParams{
		uri:   "/openconfig-interfaces:interfaces/interface[name=Vlan100]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8:100::/64]/config/valid-lifetime",
		param: &lifetime,
		oper:  UPDATE,
	})
	if err != nil {
		t.Fatalf("YangToDb_ipv6_ra_prefix_valid_lifetime_xfmr() error = %v", err)
	}
	want := map[string]string{ipv6RaPrefixFieldValidLifetime: "3600"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestIpv6RaPrefixReqPath_prefersFullUri(t *testing.T) {
	full := "/openconfig-interfaces:interfaces/interface[name=Vlan100]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8:100::/64]/config/valid-lifetime"
	short := "/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8:100::/64]/config/valid-lifetime"
	got := ipv6RaPrefixReqPath(full, short)
	if got != full {
		t.Fatalf("ipv6RaPrefixReqPath() = %q want %q", got, full)
	}
	pi := ipv6RaPrefixPathInfo(full, short)
	if pi.Var("name") != "Vlan100" || pi.Var("prefix") != "2001:db8:100::/64" {
		t.Fatalf("path vars name=%q prefix=%q", pi.Var("name"), pi.Var("prefix"))
	}
}

func TestYangToDb_intf_tbl_key_xfmr_ra_prefix_leaf_uri(t *testing.T) {
	leafUri := "/openconfig-interfaces:interfaces/interface[name=Vlan100]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8:100::/64]/config/valid-lifetime"
	dbMap := RedisDbMap{
		db.ConfigDB: {
			"VLAN_INTERFACE": {
				"Vlan100|2001:db8:100::1/64": {Field: map[string]string{}},
			},
		},
	}
	got, err := YangToDb_intf_tbl_key_xfmr(XfmrParams{
		uri:        leafUri,
		requestUri: leafUri,
		dbDataMap:  &dbMap,
		curDb:      db.ConfigDB,
	})
	if err != nil {
		t.Fatalf("YangToDb_intf_tbl_key_xfmr() error = %v", err)
	}
	if got != "Vlan100|2001:db8:100::1/64" {
		t.Fatalf("YangToDb_intf_tbl_key_xfmr() = %q", got)
	}
}

func TestIpv6RaPrefixDbKeyFromMap(t *testing.T) {
	dbMap := RedisDbMap{
		db.ConfigDB: {
			"VLAN_INTERFACE": {
				"Vlan100|2001:db8:100::1/64": {Field: map[string]string{}},
			},
		},
	}
	key, err := ipv6RaPrefixDbKey("Vlan100", "", "2001:db8:100::/64", &dbMap, db.ConfigDB, nil)
	if err != nil {
		t.Fatalf("ipv6RaPrefixDbKey() error = %v", err)
	}
	if key != "Vlan100|2001:db8:100::1/64" {
		t.Fatalf("ipv6RaPrefixDbKey() = %q", key)
	}
}

func TestDbToYangIpv6RaFields(t *testing.T) {
	dbMap := RedisDbMap{
		db.ConfigDB: {
			"INTERFACE": {
				"Ethernet1": {Field: map[string]string{
					ipv6RaFieldInterval:      "60",
					ipv6RaFieldLifetime:      "180",
					ipv6RaFieldReachableTime: "30000",
					ipv6RaFieldSuppress:      "true",
					ipv6RaFieldEnable:        "true",
					ipv6RaFieldManaged:       "false",
					ipv6RaFieldOtherConfig:   "true",
				}},
			},
		},
	}
	uri := "/openconfig-interfaces:interfaces/interface[name=Ethernet1]/subinterfaces/subinterface[index=0]/ipv6/router-advertisement/config"
	in := XfmrParams{uri: uri, key: "Ethernet1", curDb: db.ConfigDB, dbDataMap: &dbMap}

	got, err := DbToYang_ipv6_ra_interval_xfmr(in)
	if err != nil {
		t.Fatalf("interval GET error = %v", err)
	}
	if got["interval"] != uint32(60) {
		t.Fatalf("interval GET = %v", got)
	}
	got, err = DbToYang_ipv6_ra_suppress_xfmr(in)
	if err != nil {
		t.Fatalf("suppress GET error = %v", err)
	}
	if got["suppress"] != true {
		t.Fatalf("suppress GET = %v", got)
	}
	got, err = DbToYang_ipv6_ra_enable_xfmr(in)
	if err != nil || got["enable"] != true {
		t.Fatalf("enable GET = %v err=%v", got, err)
	}
	got, err = DbToYang_ipv6_ra_managed_xfmr(in)
	if err != nil || got["managed"] != false {
		t.Fatalf("managed GET = %v err=%v", got, err)
	}
	got, err = DbToYang_ipv6_ra_other_config_xfmr(in)
	if err != nil || got["other-config"] != true {
		t.Fatalf("other-config GET = %v err=%v", got, err)
	}
	got, err = DbToYang_ipv6_ra_reachable_time_xfmr(in)
	if err != nil || got["reachable-time"] != uint32(30000) {
		t.Fatalf("reachable-time GET = %v err=%v", got, err)
	}
}

func TestYangToDbIpv6RaExtendedFields(t *testing.T) {
	enable := true
	got, err := YangToDb_ipv6_ra_enable_xfmr(XfmrParams{
		uri:   "/openconfig-interfaces:interfaces/interface[name=Ethernet1]/subinterfaces/subinterface[index=0]/ipv6/router-advertisement/config/enable",
		param: &enable,
		oper:  UPDATE,
	})
	if err != nil || got[ipv6RaFieldEnable] != "true" {
		t.Fatalf("enable xfmr = %v err=%v", got, err)
	}
}

func TestIsIpv6RaPrefixOnlyEntry(t *testing.T) {
	ra := db.Value{Field: map[string]string{ipv6RaPrefixFieldDisableAutoconfig: "true"}}
	addr := db.Value{Field: map[string]string{"family": "IPv6"}}
	both := db.Value{Field: map[string]string{
		"family":                       "IPv6",
		ipv6RaPrefixFieldValidLifetime: "1800",
	}}
	if !isIpv6RaPrefixOnlyEntry(ra) {
		t.Fatal("expected RA-only row")
	}
	if isIpv6RaPrefixOnlyEntry(addr) {
		t.Fatal("assigned address must not be RA-only")
	}
	if isIpv6RaPrefixOnlyEntry(both) {
		t.Fatal("row with family must not be RA-only")
	}
	vlanAddr := db.Value{Field: map[string]string{
		"NULL":                             "NULL",
		ipv6RaPrefixFieldPreferredLifetime: "900",
	}}
	if isIpv6RaPrefixOnlyEntry(vlanAddr) {
		t.Fatal("assigned VLAN IPv6 row with NULL must not be RA-only")
	}
}

func TestSubscribe_ipv6_ra_prefixes_xfmr_parentCheck(t *testing.T) {
	prefixesUri := "/openconfig-interfaces:interfaces/interface[name=Vlan100]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes"
	leafUri := prefixesUri + "/prefix[prefix=2001:db8:100::/64]/config/disable-advertisement"

	out, err := Subscribe_ipv6_ra_prefixes_xfmr(XfmrSubscInParams{
		uri:        prefixesUri,
		requestURI: "",
		subscProc:  TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ipv6_ra_prefixes_xfmr: %v", err)
	}
	if !out.isVirtualTbl {
		t.Fatalf("expected virtual table for parent check, got dbDataMap=%v", out.dbDataMap)
	}

	out, err = Subscribe_ipv6_ra_prefixes_xfmr(XfmrSubscInParams{
		uri:        leafUri,
		requestURI: leafUri,
		subscProc:  TRANSLATE_SUBSCRIBE,
	})
	if err != nil {
		t.Fatalf("Subscribe_ipv6_ra_prefixes_xfmr keyed: %v", err)
	}
	if !out.isVirtualTbl {
		t.Fatalf("expected virtual table for keyed prefix path")
	}
}

func TestYangToDb_ipv6_ra_prefixes_xfmr_delete(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Ethernet0"
	ts := &db.TableSpec{Name: "INTERFACE"}
	_ = cfgDb.SetEntry(ts, db.Key{Comp: []string{ifName, "2001:db8:1::/64"}}, db.Value{Field: map[string]string{
		ipv6RaPrefixFieldDisableAutoconfig: "true",
	}})
	_ = cfgDb.SetEntry(ts, db.Key{Comp: []string{ifName, "2001:db8:2::/64"}}, db.Value{Field: map[string]string{
		"family": "IPv6",
	}})

	uri := "/openconfig-interfaces:interfaces/interface[name=Ethernet0]/subinterfaces/subinterface[index=0]/openconfig-if-ip:ipv6/router-advertisement/prefixes"
	got, err := YangToDb_ipv6_ra_prefixes_xfmr(XfmrParams{
		d:          cfgDb,
		uri:        uri,
		requestUri: uri,
		oper:       DELETE,
	})
	if err != nil {
		t.Fatalf("YangToDb_ipv6_ra_prefixes_xfmr: %v", err)
	}
	delMap := got["INTERFACE"]
	if len(delMap) != 1 || delMap[ifName+"|2001:db8:1::/64"].Field != nil {
		t.Fatalf("delete map = %v", delMap)
	}
}

func TestYangToDb_ipv6_ra_prefixes_xfmr_update_preferred_lifetime(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Vlan100"
	const prefix = "2002:db8:100::/64"
	ts := &db.TableSpec{Name: "VLAN_INTERFACE"}
	_ = cfgDb.SetEntry(ts, db.Key{Comp: []string{ifName, prefix}}, db.Value{Field: map[string]string{"NULL": "NULL"}})

	prefixesUri := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes"
	reqUri := prefixesUri + "/prefix[prefix=" + prefix + "]/config/preferred-lifetime"
	val := uint32(900)

	got, err := YangToDb_ipv6_ra_prefixes_xfmr(XfmrParams{
		d:          cfgDb,
		curDb:      db.ConfigDB,
		uri:        prefixesUri,
		requestUri: reqUri,
		oper:       UPDATE,
		param:      &val,
	})
	if err != nil {
		t.Fatalf("YangToDb_ipv6_ra_prefixes_xfmr: %v", err)
	}
	m := got["VLAN_INTERFACE"]
	if m == nil {
		t.Fatalf("expected VLAN_INTERFACE mutations, got %v", got)
	}
	entry := m[ifName+"|"+prefix]
	if entry.Field == nil || entry.Field[ipv6RaPrefixFieldPreferredLifetime] != "900" {
		t.Fatalf("mutation = %v", entry.Field)
	}
}

func TestYangToDb_ipv6_ra_prefixes_xfmr_delete_prefix_keeps_address(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Vlan100"
	const prefix = "2002:db8:100::/64"
	ts := &db.TableSpec{Name: "VLAN_INTERFACE"}
	_ = cfgDb.SetEntry(ts, db.Key{Comp: []string{ifName, prefix}}, db.Value{Field: map[string]string{
		"NULL":                             "NULL",
		ipv6RaPrefixFieldPreferredLifetime: "900",
	}})

	prefixesUri := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes"
	reqUri := prefixesUri + "/prefix[prefix=" + prefix + "]"

	got, err := YangToDb_ipv6_ra_prefixes_xfmr(XfmrParams{
		d:          cfgDb,
		curDb:      db.ConfigDB,
		uri:        prefixesUri,
		requestUri: reqUri,
		oper:       DELETE,
	})
	if err != nil {
		t.Fatalf("YangToDb_ipv6_ra_prefixes_xfmr: %v", err)
	}
	m := got["VLAN_INTERFACE"]
	if m == nil {
		t.Fatalf("expected mutations, got %v", got)
	}
	entry := m[ifName+"|"+prefix]
	if entry.Field == nil {
		t.Fatalf("expected field clears, got full row delete %v", entry)
	}
	if entry.Field[ipv6RaPrefixFieldPreferredLifetime] != "" {
		t.Fatalf("expected preferred lifetime cleared, got %v", entry.Field)
	}
	if _, ok := entry.Field["NULL"]; ok {
		t.Fatalf("must not delete NULL placeholder: %v", entry.Field)
	}
}

func TestYangToDb_ipv6_ra_prefix_disable_advertisement_vlan_only(t *testing.T) {
	subifUri := "/openconfig-interfaces:interfaces/interface[name=Ethernet1]/subinterfaces/subinterface[index=0]/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8::/64]/config/disable-advertisement"
	_, err := YangToDb_ipv6_ra_prefix_disable_advertisement_xfmr(XfmrParams{
		uri:        subifUri,
		requestUri: subifUri,
		param:      true,
		oper:       UPDATE,
	})
	if err == nil {
		t.Fatal("expected NotSupported on subinterface disable-advertisement")
	}

	vlanUri := "/openconfig-interfaces:interfaces/interface[name=Vlan100]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8::/64]/config/disable-advertisement"
	got, err := YangToDb_ipv6_ra_prefix_disable_advertisement_xfmr(XfmrParams{
		uri:        vlanUri,
		requestUri: vlanUri,
		param:      true,
		oper:       UPDATE,
	})
	if err != nil || got[ipv6RaPrefixFieldDisableAdv] != "true" {
		t.Fatalf("vlan disable-advertisement = %v err=%v", got, err)
	}
}

func TestDbToYang_ipv6_ra_prefix_valid_lifetime_liveDb(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Vlan100"
	const ocPrefix = "2001:db8:100::/64"
	_ = cfgDb.SetEntry(&db.TableSpec{Name: "VLAN_INTERFACE"}, db.Key{Comp: []string{ifName, "2001:db8:100::1/64"}}, db.Value{
		Field: map[string]string{
			"NULL":                         "NULL",
			ipv6RaPrefixFieldValidLifetime: "3600",
		},
	})
	uri := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=" + ocPrefix + "]/config/valid-lifetime"
	in := XfmrParams{
		d:          cfgDb,
		curDb:      db.ConfigDB,
		uri:        uri,
		requestUri: uri,
		oper:       GET,
	}
	got, err := DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr(in)
	if err != nil {
		t.Fatalf("DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr: %v", err)
	}
	v, ok := got["valid-lifetime"].(uint32)
	if !ok || v != 3600 {
		t.Fatalf("got %v want valid-lifetime 3600", got)
	}
}

func TestDbToYang_ipv6_ra_prefixes_xfmr_routedVlan(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Vlan100"
	_ = cfgDb.SetEntry(&db.TableSpec{Name: "VLAN_INTERFACE"}, db.Key{Comp: []string{ifName, "2001:db8:100::1/64"}}, db.Value{
		Field: map[string]string{"NULL": "NULL"},
	})
	_ = cfgDb.SetEntry(&db.TableSpec{Name: "VLAN_INTERFACE"}, db.Key{Comp: []string{ifName, "2001:db8:104::1/64"}}, db.Value{
		Field: map[string]string{
			"NULL":                             "NULL",
			ipv6RaPrefixFieldPreferredLifetime: "800",
		},
	})

	prefixesUri := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes"
	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)
	var ygRoot ygot.GoStruct = device
	dbs := [db.MaxDB]*db.DB{}
	dbs[db.ConfigDB] = cfgDb
	err = DbToYang_ipv6_ra_prefixes_xfmr(XfmrParams{
		d:          cfgDb,
		dbs:        dbs,
		curDb:      db.ConfigDB,
		uri:        prefixesUri,
		requestUri: prefixesUri,
		oper:       GET,
		ygRoot:     &ygRoot,
	})
	if err != nil {
		t.Fatalf("DbToYang_ipv6_ra_prefixes_xfmr: %v", err)
	}
	if device.Interfaces == nil {
		t.Fatalf("missing interfaces in ygot")
	}
	intfObj := device.Interfaces.Interface[ifName]
	if intfObj == nil || intfObj.RoutedVlan == nil || intfObj.RoutedVlan.Ipv6 == nil ||
		intfObj.RoutedVlan.Ipv6.RouterAdvertisement == nil ||
		intfObj.RoutedVlan.Ipv6.RouterAdvertisement.Prefixes == nil {
		t.Fatalf("missing prefixes container in ygot")
	}
	if len(intfObj.RoutedVlan.Ipv6.RouterAdvertisement.Prefixes.Prefix) < 2 {
		t.Fatalf("expected at least 2 prefixes, got %d", len(intfObj.RoutedVlan.Ipv6.RouterAdvertisement.Prefixes.Prefix))
	}
}

func TestDbToYang_ipv6_ra_prefixes_xfmr_routedVlan_configOnly(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Vlan100"
	const ocPrefix = "2001:db8:100::/64"
	_ = cfgDb.SetEntry(&db.TableSpec{Name: "VLAN_INTERFACE"}, db.Key{Comp: []string{ifName, "2001:db8:100::1/64"}}, db.Value{
		Field: map[string]string{
			"NULL":                             "NULL",
			ipv6RaPrefixFieldPreferredLifetime: "600",
		},
	})

	configUri := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=" + ocPrefix + "]/config"
	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)
	var ygRoot ygot.GoStruct = device
	dbs := [db.MaxDB]*db.DB{}
	dbs[db.ConfigDB] = cfgDb
	err = DbToYang_ipv6_ra_prefixes_xfmr(XfmrParams{
		d:          cfgDb,
		dbs:        dbs,
		curDb:      db.ConfigDB,
		uri:        configUri,
		requestUri: configUri,
		oper:       GET,
		ygRoot:     &ygRoot,
	})
	if err != nil {
		t.Fatalf("DbToYang_ipv6_ra_prefixes_xfmr: %v", err)
	}
	pref := device.Interfaces.Interface[ifName].RoutedVlan.Ipv6.RouterAdvertisement.Prefixes.Prefix[ocPrefix]
	if pref == nil || pref.Config == nil {
		t.Fatalf("missing prefix config in ygot")
	}
	if pref.Config.PreferredLifetime == nil || *pref.Config.PreferredLifetime != 600 {
		t.Fatalf("preferred-lifetime = %v want 600", pref.Config.PreferredLifetime)
	}
	if pref.State != nil && pref.State.PreferredLifetime != nil {
		t.Fatalf("state should not be filled for config-only GET")
	}
}

func TestDbToYang_ipv6_ra_prefixes_xfmr_routedVlan_configOnly_hostListKey(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Vlan100"
	const hostPrefix = "2001:db8:104::1/64"
	_ = cfgDb.SetEntry(&db.TableSpec{Name: "VLAN_INTERFACE"}, db.Key{Comp: []string{ifName, "2001:db8:104::/64"}}, db.Value{
		Field: map[string]string{
			"NULL":                             "NULL",
			ipv6RaPrefixFieldPreferredLifetime: "600",
		},
	})

	configUri := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=" + hostPrefix + "]/config"
	device := &ocbinds.Device{}
	ygot.BuildEmptyTree(device)
	var ygRoot ygot.GoStruct = device
	dbs := [db.MaxDB]*db.DB{}
	dbs[db.ConfigDB] = cfgDb
	err = DbToYang_ipv6_ra_prefixes_xfmr(XfmrParams{
		d:          cfgDb,
		dbs:        dbs,
		curDb:      db.ConfigDB,
		uri:        configUri,
		requestUri: configUri,
		oper:       GET,
		ygRoot:     &ygRoot,
	})
	if err != nil {
		t.Fatalf("DbToYang_ipv6_ra_prefixes_xfmr: %v", err)
	}
	pref := device.Interfaces.Interface[ifName].RoutedVlan.Ipv6.RouterAdvertisement.Prefixes.Prefix[hostPrefix]
	if pref == nil || pref.Config == nil {
		t.Fatalf("missing prefix config in ygot at list key %q", hostPrefix)
	}
	if pref.Config.Prefix == nil || *pref.Config.Prefix != "2001:db8:104::/64" {
		t.Fatalf("config prefix = %v want network form 2001:db8:104::/64", pref.Config.Prefix)
	}
	if pref.Config.PreferredLifetime == nil || *pref.Config.PreferredLifetime != 600 {
		t.Fatalf("preferred-lifetime = %v want 600", pref.Config.PreferredLifetime)
	}
	if _, dup := device.Interfaces.Interface[ifName].RoutedVlan.Ipv6.RouterAdvertisement.Prefixes.Prefix["2001:db8:104::/64"]; dup {
		t.Fatalf("duplicate prefix list entry at network key should be pruned")
	}
}

func TestDbToYang_ipv6_ra_prefix_valid_lifetime_shortUri(t *testing.T) {
	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB: %v", err)
	}
	defer cfgDb.DeleteDB()

	const ifName = "Vlan100"
	_ = cfgDb.SetEntry(&db.TableSpec{Name: "VLAN_INTERFACE"}, db.Key{Comp: []string{ifName, "2001:db8:100::1/64"}}, db.Value{
		Field: map[string]string{
			"NULL":                         "NULL",
			ipv6RaPrefixFieldValidLifetime: "3600",
		},
	})
	reqUri := "/openconfig-interfaces:interfaces/interface[name=" + ifName + "]/openconfig-vlan:routed-vlan/openconfig-if-ip:ipv6/router-advertisement/prefixes/prefix[prefix=2001:db8:100::/64]/config/valid-lifetime"
	in := XfmrParams{
		d:          cfgDb,
		curDb:      db.ConfigDB,
		uri:        "valid-lifetime",
		requestUri: reqUri,
		oper:       GET,
	}
	got, err := DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr(in)
	if err != nil {
		t.Fatalf("DbToYang_ipv6_ra_prefix_valid_lifetime_xfmr: %v", err)
	}
	v, ok := got["valid-lifetime"].(uint32)
	if !ok || v != 3600 {
		t.Fatalf("got %v want valid-lifetime 3600", got)
	}
}
