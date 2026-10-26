package transformer

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// bgp/global/dynamic-neighbor-prefixes maps onto
// BGP_GLOBALS_LISTEN_PREFIX|<vrf>|<prefix>, which frrcfgd renders as
// "bgp listen range <prefix> peer-group <peer_group>". FRR listens on a range
// only for a peer-group, so a prefix is refused without one. The table holds
// what FRR is given as is, so state reports CONFIG_DB.
const (
	niBgpListenPfxTbl    = "BGP_GLOBALS_LISTEN_PREFIX"
	niBgpListenPfxPgFld  = "peer_group"
	niBgpTargetListenPfx = "global-listen-prefix"
)

type niBgpListenPfxObj = ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_DynamicNeighborPrefixes_DynamicNeighborPrefix

var niBgpListenPfxReqUriRe = regexp.MustCompile(niBgpProtoUriPfx +
	`/bgp/global/dynamic-neighbor-prefixes(?:/dynamic-neighbor-prefix(?:\[prefix=([^\]]+)\](?:/(prefix|config|state)(?:/(prefix|peer-group))?)?)?)?$`)

func matchNiBgpListenPfxUri(uri string) (*niBgpReqInfo, bool) {
	m := niBgpListenPfxReqUriRe.FindStringSubmatch(uri)
	if len(m) < 6 {
		return nil, false
	}
	return &niBgpReqInfo{
		niName:         m[1],
		protoName:      m[2],
		listenPrefix:   m[3],
		container:      m[4],
		targetLeaf:     m[5],
		targetPathKind: niBgpTargetListenPfx,
	}, true
}

func validateNiBgpListenPrefix(prefix string) error {
	_, ipNet, err := net.ParseCIDR(prefix)
	if err != nil || ipNet.String() != prefix {
		return tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP dynamic-neighbor-prefix %q is not a normalized IPv4 or IPv6 prefix", prefix)}
	}
	return nil
}

func niBgpListenPfxObjs(s *ygot.GoStruct, niName, protoName string) map[string]*niBgpListenPfxObj {
	bgpObj := getNiBgpProtoObj(s, niName, protoName, false)
	if bgpObj == nil || bgpObj.Global == nil || bgpObj.Global.DynamicNeighborPrefixes == nil {
		return nil
	}
	return bgpObj.Global.DynamicNeighborPrefixes.DynamicNeighborPrefix
}

// niBgpListenPfxRows maps the dynamic-neighbor-prefix entries a payload carries,
// only prefix when one is named. existing reports the peer-group a row already
// holds, so that a write naming none keeps it.
func niBgpListenPfxRows(objs map[string]*niBgpListenPfxObj, niName, prefix string, existing func(string) string) (map[string]db.Value, error) {
	rows := map[string]db.Value{}
	names := make([]string, 0, len(objs))
	for p := range objs {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		obj := objs[p]
		if obj == nil || (prefix != "" && p != prefix) {
			continue
		}
		if err := validateNiBgpListenPrefix(p); err != nil {
			return nil, err
		}
		pg := ""
		if obj.Config != nil {
			if obj.Config.Prefix != nil && *obj.Config.Prefix != p {
				return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP dynamic-neighbor-prefix config prefix (%s) does not match key (%s)", *obj.Config.Prefix, p)}
			}
			if obj.Config.PeerGroup != nil {
				pg = strings.TrimSpace(*obj.Config.PeerGroup)
			}
		}
		if pg == "" && existing != nil {
			pg = existing(p)
		}
		if pg == "" {
			return nil, tlerr.InvalidArgsError{Format: fmt.Sprintf("BGP dynamic-neighbor-prefix %s needs a peer-group: FRR listens on a range only for a peer-group", p)}
		}
		rows[niName+"|"+p] = db.Value{Field: map[string]string{niBgpListenPfxPgFld: pg}}
	}
	return rows, nil
}

func niBgpListenPfxKeys(cfgDb *db.DB, niName string) []string {
	var prefixes []string
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: niBgpListenPfxTbl})
	if err != nil {
		return nil
	}
	for _, k := range keys {
		if len(k.Comp) == 2 && k.Comp[0] == niName {
			prefixes = append(prefixes, k.Comp[1])
		}
	}
	sort.Strings(prefixes)
	return prefixes
}

// niBgpListenPfxYangToDb maps a SET or a DELETE at or below
// dynamic-neighbor-prefixes.
func niBgpListenPfxYangToDb(inParams XfmrParams, reqInfo *niBgpReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	cfgDb := inParams.dbs[db.ConfigDB]
	if cfgDb == nil {
		return res, tlerr.InvalidArgsError{Format: "ConfigDB nil for network-instance BGP dynamic-neighbor-prefixes"}
	}
	if reqInfo.listenPrefix != "" {
		if err := validateNiBgpListenPrefix(reqInfo.listenPrefix); err != nil {
			return res, err
		}
	}
	if inParams.oper == DELETE {
		if reqInfo.container != "" {
			return res, tlerr.NotSupportedError{
				Format: "DELETE is supported only for a dynamic-neighbor-prefix instance or the list; a prefix needs its peer-group",
				Path:   inParams.requestUri,
			}
		}
		rows := map[string]db.Value{}
		if reqInfo.listenPrefix != "" {
			rows[reqInfo.niName+"|"+reqInfo.listenPrefix] = db.Value{Field: map[string]string{}}
		} else {
			for _, p := range niBgpListenPfxKeys(cfgDb, reqInfo.niName) {
				rows[reqInfo.niName+"|"+p] = db.Value{Field: map[string]string{}}
			}
		}
		if len(rows) > 0 {
			res[niBgpListenPfxTbl] = rows
		}
		return res, nil
	}
	existing := func(p string) string {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpListenPfxTbl}, db.Key{Comp: []string{reqInfo.niName, p}})
		if err != nil {
			return ""
		}
		return entry.Field[niBgpListenPfxPgFld]
	}
	rows, err := niBgpListenPfxRows(niBgpListenPfxObjs(inParams.ygRoot, reqInfo.niName, reqInfo.protoName), reqInfo.niName, reqInfo.listenPrefix, existing)
	if err != nil {
		return res, err
	}
	if len(rows) == 0 {
		return res, tlerr.InvalidArgsError{Format: "BGP dynamic-neighbor-prefixes payload names no prefix"}
	}
	if inParams.invokeCRUSubtreeOnce != nil {
		*inParams.invokeCRUSubtreeOnce = true
	}
	res[niBgpListenPfxTbl] = rows
	return res, nil
}

// niBgpListenPfxToYang fills the dynamic-neighbor-prefix entries of the
// instance from CONFIG_DB, only prefix when one is named.
func niBgpListenPfxToYang(cfgDb *db.DB, ygRoot *ygot.GoStruct, niName, protoName, prefix string) error {
	prefixes := []string{prefix}
	if prefix == "" {
		prefixes = niBgpListenPfxKeys(cfgDb, niName)
	}
	for _, p := range prefixes {
		entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpListenPfxTbl}, db.Key{Comp: []string{niName, p}})
		if err != nil {
			if prefix != "" {
				return err
			}
			continue
		}
		bgpObj := getNiBgpProtoObj(ygRoot, niName, protoName, true)
		if bgpObj == nil {
			return nil
		}
		if bgpObj.Global == nil {
			bgpObj.Global = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{}
		}
		if bgpObj.Global.DynamicNeighborPrefixes == nil {
			bgpObj.Global.DynamicNeighborPrefixes = &ocbinds.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_DynamicNeighborPrefixes{}
		}
		obj, ok := bgpObj.Global.DynamicNeighborPrefixes.DynamicNeighborPrefix[p]
		if !ok || obj == nil {
			if obj, err = bgpObj.Global.DynamicNeighborPrefixes.NewDynamicNeighborPrefix(p); err != nil {
				return err
			}
		}
		ygot.BuildEmptyTree(obj)
		key := p
		obj.Prefix = &key
		obj.Config.Prefix = &key
		if pg := strings.TrimSpace(entry.Field[niBgpListenPfxPgFld]); pg != "" {
			obj.Config.PeerGroup = &pg
		}
		ocMirrorConfigToState(obj)
	}
	return nil
}

func init() {
	niBgpPathKeyTables[niBgpListenPfxTbl] = []string{"network-instance", "dynamic-neighbor-prefix"}
	niBgpPathListKeyLeaf["dynamic-neighbor-prefix"] = "prefix"
	niBgpGlobalSubscVirtual["/bgp/global/dynamic-neighbor-prefixes"] = true
	niBgpGlobalSubscVirtual["/bgp/global/dynamic-neighbor-prefixes/dynamic-neighbor-prefix"] = true
	for _, twin := range []string{"config", "state"} {
		niBgpGlobalSubscMaps["/bgp/global/dynamic-neighbor-prefixes/dynamic-neighbor-prefix/"+twin] = niBgpSubscMap{
			dbNum: db.ConfigDB, table: niBgpListenPfxTbl, key: niBgpSubscKeyNiListenPfx,
			fields: map[string][]string{"peer-group": {niBgpListenPfxPgFld}},
		}
	}
	for _, leaf := range []string{"prefix", "config/prefix", "config/peer-group"} {
		niBgpMappedPayloadLeaves["global/dynamic-neighbor-prefixes/dynamic-neighbor-prefix/"+leaf] = true
	}
}
