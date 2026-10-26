package transformer

import (
	"regexp"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/openconfig/ygot/ygot"
)

// network-instance/tables lists the tables the instance's protocols install
// routes into. It holds nothing of its own: a table exists for each address
// family a configured protocol instance carries, so it is derived on GET and
// refused on SET.
const niContainerTargetTables = "network-instance-tables"

// The two keys of a table arrive in either order: translib sorts the keys of
// a gNMI path by name.
var niTablesReqUriRe = regexp.MustCompile(niContainerUriPfx +
	`/network-instance\[name=([^\]]+)\]/tables(?:/table((?:\[[^\]]+\])*)(?:/.*)?)?$`)
var niTableKeyRe = regexp.MustCompile(`\[(protocol|address-family)=([^\]]+)\]`)

type niTableKey struct {
	protocol ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE
	af       ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY
}

var niTableProtocols = map[string]ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE{
	"BGP":             ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
	"STATIC":          ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
	"LOCAL_AGGREGATE": ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE,
}

var niTableAfs = map[string]ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY{
	"IPV4": ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
	"IPV6": ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6,
}

func niTableIdentity(v string) string {
	if i := strings.LastIndex(v, ":"); i >= 0 {
		v = v[i+1:]
	}
	return v
}

func niTablePrefixAf(prefix string) ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY {
	if strings.Contains(prefix, ":") {
		return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6
	}
	return ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4
}

// niTablesFromDb lists the tables of niName: both families for a BGP instance,
// which runs a RIB for each, and the families of the prefixes the static
// routes and the local aggregates hold.
func niTablesFromDb(cfgDb *db.DB, niName string) []niTableKey {
	var tables []niTableKey
	if _, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpGlobalsTbl}, db.Key{Comp: []string{niName}}); err == nil {
		for _, af := range []ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY{ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4, ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6} {
			tables = append(tables, niTableKey{ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP, af})
		}
	}
	afsOf := func(protocol ocbinds.E_OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE, prefixes []string) {
		seen := map[ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY]bool{}
		for _, p := range prefixes {
			seen[niTablePrefixAf(p)] = true
		}
		for _, af := range []ocbinds.E_OpenconfigTypes_ADDRESS_FAMILY{ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV4, ocbinds.OpenconfigTypes_ADDRESS_FAMILY_IPV6} {
			if seen[af] {
				tables = append(tables, niTableKey{protocol, af})
			}
		}
	}
	afsOf(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC, niStaticRoutePrefixesFromDb(cfgDb, niName))
	afsOf(ocbinds.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_LOCAL_AGGREGATE, niLocalAggrSortedPrefixes(niLocalAggrRows(cfgDb, niName, "")))
	return tables
}

// niTablesToYang fills the tables of niName that uri names: all of them, or
// the one its keys name.
func niTablesToYang(ygRoot *ygot.GoStruct, cfgDb *db.DB, niName, uri string) error {
	m := niTablesReqUriRe.FindStringSubmatch(uri)
	var want *niTableKey
	if len(m) >= 3 && m[2] != "" {
		keys := map[string]string{}
		for _, k := range niTableKeyRe.FindAllStringSubmatch(m[2], -1) {
			keys[k[1]] = niTableIdentity(k[2])
		}
		protocol, okP := niTableProtocols[keys["protocol"]]
		af, okA := niTableAfs[keys["address-family"]]
		if !okP || !okA {
			return nil
		}
		want = &niTableKey{protocol, af}
	}
	niObj, err := niContainerGetOrCreateNi(ygRoot, niName)
	if err != nil || niObj == nil {
		return err
	}
	for _, t := range niTablesFromDb(cfgDb, niName) {
		if want != nil && t != *want {
			continue
		}
		if niObj.Tables == nil {
			niObj.Tables = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Tables{}
		}
		tObj, ok := niObj.Tables.Table[ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Tables_Table_Key{Protocol: t.protocol, AddressFamily: t.af}]
		if !ok || tObj == nil {
			if tObj, err = niObj.Tables.NewTable(t.protocol, t.af); err != nil {
				return err
			}
		}
		ygot.BuildEmptyTree(tObj)
		tObj.Config.Protocol = t.protocol
		tObj.Config.AddressFamily = t.af
		ocMirrorConfigToState(tObj)
	}
	return nil
}
