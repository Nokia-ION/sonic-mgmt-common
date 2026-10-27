package transformer

import (
	"reflect"
	"regexp"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/openconfig/ygot/ygot"
)

// niBgpPgLeafFlds maps the peer-group leaves that are plain BGP_PEER_GROUP
// fields, by the config container holding them below the peer-group. The
// config and timers containers also hold leaves the peer-group config
// transformer maps; the others are served here alone.
var niBgpPgLeafFlds = map[string]map[string]string{
	"config": {
		"local-as":      "local_asn",
		"peer-type":     niBgpNbrPeerTypeFld,
		"auth-password": niBgpNbrAuthPwdFld,
		"description":   "name",
	},
	"timers/config": {
		"connect-retry":                  niBgpNbrConnRetryFld,
		"minimum-advertisement-interval": "min_adv_interval",
	},
	"transport/config": {
		"tcp-mss":       "tcp_mss",
		"passive-mode":  niBgpNbrPassiveModeFld,
		"local-address": niBgpNbrLocalAddrFld,
	},
	"ebgp-multihop/config": {
		"enabled":      "ebgp_multihop",
		"multihop-ttl": "ebgp_multihop_ttl",
	},
	"enable-bfd/config": {
		"enabled": "bfd",
	},
	"graceful-restart/config": {
		"enabled": "graceful_restart_enable",
	},
}

// niBgpPgAfLeafFlds maps the peer-group afi-safi leaves that are plain
// BGP_PEER_GROUP_AF fields. A peer-group has no prefix-limit-received, so the
// prefix-limit leaves map onto the maximum-prefix fields directly.
var niBgpPgAfLeafFlds = map[string]map[string]string{
	"ipv4-unicast/config": {"send-default-route": "send_default_route"},
	"ipv6-unicast/config": {"send-default-route": "send_default_route"},
	"ipv4-unicast/prefix-limit/config": {
		"max-prefixes":          niBgpPgMaxPfxLimitFld,
		"prevent-teardown":      niBgpNbrMaxPfxWarnOnlyFld,
		"warning-threshold-pct": niBgpNbrMaxPfxThreshFld,
	},
	"ipv6-unicast/prefix-limit/config": {
		"max-prefixes":          niBgpPgMaxPfxLimitFld,
		"prevent-teardown":      niBgpNbrMaxPfxWarnOnlyFld,
		"warning-threshold-pct": niBgpNbrMaxPfxThreshFld,
	},
}

var niBgpPgKind = &niBgpPeerKind{
	scope:         "peer-group",
	payloadPfx:    "peer-groups/peer-group",
	tbl:           niBgpPeerGroupTbl,
	afTbl:         niBgpPeerGroupAfTbl,
	leafFlds:      niBgpPgLeafFlds,
	ownContainers: map[string]bool{"transport/config": true, "ebgp-multihop/config": true, "enable-bfd/config": true, "graceful-restart/config": true},
	afLeafFlds:    niBgpPgAfLeafFlds,
	leafReqRe: regexp.MustCompile(niBgpPgUriPfx +
		`/(` + niBgpPeerLeafContainers(niBgpPgLeafFlds) + `)(?:/([a-z-]+))?$`),
	afLeafReqRe: regexp.MustCompile(niBgpPgUriPfx +
		`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(` + niBgpPeerLeafContainers(niBgpPgAfLeafFlds) + `|add-paths/config)(?:/([a-z-]+))?$`),
	afFamilyReqRe: regexp.MustCompile(niBgpPgUriPfx +
		`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(ipv4-unicast|ipv6-unicast|ipv4-unicast/prefix-limit|ipv6-unicast/prefix-limit)$`),
	normalizeAf: normalizeNiBgpPgAfiSafiName,
	obj: func(s *ygot.GoStruct, niName, protoName, peer string, create bool) reflect.Value {
		return reflect.ValueOf(getNiBgpPeerGroupObj(s, niName, protoName, peer, create))
	},
	afObj: func(s *ygot.GoStruct, niName, protoName, peer, afiSafi string, create bool) reflect.Value {
		return reflect.ValueOf(getNiBgpPeerGroupAfObjByName(s, niName, protoName, peer, afiSafi, create))
	},
	resMap:    makeNiBgpPgResMap,
	afResMap:  makeNiBgpPgAfResMap,
	parentRow: niBgpPgParentRowIfMissing,
}

var YangToDb_ni_bgp_pg_cfg_xfmr SubTreeXfmrYangToDb = func(inParams XfmrParams) (map[string]map[string]db.Value, error) {
	return niBgpPeerCfgYangToDb(niBgpPgKind, inParams, yangToDbNiBgpPgCfgBase)
}

var DbToYang_ni_bgp_pg_cfg_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	return niBgpPeerCfgDbToYang(niBgpPgKind, inParams, dbToYangNiBgpPgCfgBase)
}

func init() {
	niBgpPgKind.registerSubsc(niBgpPgSubscMaps, niBgpPgSubscVirtual, niBgpSubscKeyNiPg, niBgpSubscKeyNiPgAf)
	for _, f := range niBgpPgKind.afInstOwnedFlds() {
		niBgpPgAfInstOwnedFlds[f] = true
	}
}
