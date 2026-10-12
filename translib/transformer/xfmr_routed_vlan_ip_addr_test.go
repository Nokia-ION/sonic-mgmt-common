package transformer

import (
	"reflect"
	"testing"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/ygot/ygot"
)

func Test_routedVlanIpAddrApplyReplace_ListPruneIPv4(t *testing.T) {
	const ifName = "Vlan100"
	const tblName = "VLAN_INTERFACE"
	const ipKeep = "10.0.0.1"
	const ipDrop = "10.0.0.2"

	cfgDb, err := db.NewDB(getDBOptions(db.ConfigDB))
	if err != nil {
		t.Fatalf("db.NewDB(CONFIG_DB): %v", err)
	}
	defer cfgDb.DeleteDB()

	ts := &db.TableSpec{Name: tblName}
	nullVal := db.Value{Field: map[string]string{"NULL": "NULL"}}
	for _, suffix := range []string{ipKeep + "/24", ipDrop + "/24"} {
		if err := cfgDb.SetEntry(ts, db.Key{Comp: []string{ifName, suffix}}, nullVal); err != nil {
			t.Fatalf("SetEntry(%s|%s): %v", ifName, suffix, err)
		}
	}

	d := &ocbinds.Device{}
	d.Interfaces = &ocbinds.OpenconfigInterfaces_Interfaces{}
	iface, _ := d.Interfaces.NewInterface(ifName)
	iface.RoutedVlan = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan{}
	iface.RoutedVlan.Ipv4 = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4{}
	iface.RoutedVlan.Ipv4.Addresses = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses{}
	addr, _ := iface.RoutedVlan.Ipv4.Addresses.NewAddress(ipKeep)
	addr.Config = &ocbinds.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses_Address_Config{
		Ip:           ygot.String(ipKeep),
		PrefixLength: ygot.Uint8(24),
	}

	ygRoot := ygot.GoStruct(d)
	subOpDataMap := make(map[Operation]*map[db.DBNum]map[string]map[string]db.Value)
	inParams := XfmrParams{
		d:            cfgDb,
		ygRoot:       &ygRoot,
		oper:         REPLACE,
		uri:          "/openconfig-interfaces:interfaces/interface[name=Vlan100]/routed-vlan/ipv4/addresses",
		requestUri:   "/openconfig-interfaces:interfaces/interface[name=Vlan100]/routed-vlan/ipv4/addresses",
		subOpDataMap: subOpDataMap,
	}

	targetPath := routedVlanAddrEffectivePath(inParams)
	vlanIntfmap := map[string]map[string]db.Value{
		tblName: {
			ifName + "|" + ipKeep + "/24": {Field: map[string]string{"NULL": "NULL"}},
		},
	}
	err = routedVlanIpAddrApplyReplace(inParams, ifName, tblName, tblName, iface.RoutedVlan, vlanIntfmap, targetPath, "")
	if err != nil {
		t.Fatalf("routedVlanIpAddrApplyReplace() error = %v", err)
	}

	wantDel := map[string]map[string]db.Value{
		tblName: {
			ifName + "|" + ipDrop + "/24": {},
		},
	}
	delOps := inParams.subOpDataMap[DELETE]
	if delOps == nil || (*delOps)[db.ConfigDB] == nil {
		t.Fatalf("expected DELETE subOpDataMap, got nil")
	}
	if !reflect.DeepEqual((*delOps)[db.ConfigDB], wantDel) {
		t.Errorf("subOp DELETE got %v, want %v", (*delOps)[db.ConfigDB], wantDel)
	}
}

func Test_routedVlanIpAddrApplyReplace_UnsupportedPath(t *testing.T) {
	inParams := XfmrParams{
		oper:       REPLACE,
		requestUri: "/openconfig-interfaces:interfaces/interface[name=Vlan100]/routed-vlan/ipv4/addresses/address[ip=10.0.0.1]/state",
	}
	targetPath := routedVlanAddrEffectivePath(inParams)
	err := routedVlanIpAddrApplyReplace(inParams, "Vlan100", "VLAN_INTERFACE", "VLAN_INTERFACE", nil, nil, targetPath, "10.0.0.1")
	if err == nil {
		t.Fatal("expected NotSupported for REPLACE on state path")
	}
}
