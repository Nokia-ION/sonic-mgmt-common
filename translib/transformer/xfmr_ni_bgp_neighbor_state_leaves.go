package transformer

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

// Where a neighbor state leaf is read from. FRR's view of the session, as
// bgpmon publishes it, is preferred; a setting FRR does not report reads the
// configuration.
const (
	niBgpNbrStateSrcApplied = iota
	niBgpNbrStateSrcStats
	niBgpNbrStateSrcConfig
	// A per-family setting FRR applies to the neighbor, read from the first
	// family that carries it, IPv4 unicast before IPv6 unicast.
	niBgpNbrStateSrcAfStats
)

var niBgpNbrStateAfStatsFamilies = []string{niBgpAfiSafiIPv4, niBgpAfiSafiIPv6}

type niBgpNbrStateLeaf struct {
	src int
	fld string
}

// niBgpNbrStateLeafFlds maps the neighbor state leaves served here, by the
// state container holding them below the neighbor. The state and
// transport/state containers also hold leaves the neighbor state transformer
// serves; the others are served here alone.
var niBgpNbrStateLeafFlds = map[string]map[string]niBgpNbrStateLeaf{
	"state": {
		"local-as":      {niBgpNbrStateSrcApplied, "local_as"},
		"peer-type":     {niBgpNbrStateSrcApplied, "peer_type"},
		"description":   {niBgpNbrStateSrcApplied, "description"},
		"neighbor-port": {niBgpNbrStateSrcApplied, niBgpNbrAppliedRmtPortFld},
		// FRR does not report the password, so state reads what is configured.
		"auth-password": {niBgpNbrStateSrcConfig, niBgpNbrAuthPwdFld},
	},
	"timers/state": {
		"connect-retry":                  {niBgpNbrStateSrcApplied, "connect_retry"},
		"hold-time":                      {niBgpNbrStateSrcApplied, "hold_time"},
		"keepalive-interval":             {niBgpNbrStateSrcApplied, "keepalive_interval"},
		"minimum-advertisement-interval": {niBgpNbrStateSrcApplied, "min_adv_interval"},
	},
	"transport/state": {
		"tcp-mss": {niBgpNbrStateSrcApplied, "tcp_mss"},
		// FRR reports passive only while the session waits in Active.
		"passive-mode": {niBgpNbrStateSrcConfig, niBgpNbrPassiveModeFld},
	},
	"ebgp-multihop/state": {
		"enabled":      {niBgpNbrStateSrcApplied, niBgpNbrAppliedMultihopTtlFld},
		"multihop-ttl": {niBgpNbrStateSrcApplied, niBgpNbrAppliedMultihopTtlFld},
	},
	"enable-bfd/state": {
		"enabled": {niBgpNbrStateSrcApplied, "bfd"},
	},
	// FRR reports the graceful restart mode in effect, which the global
	// setting decides unless the neighbor sets its own, so state reads the
	// neighbor's own setting.
	"graceful-restart/state": {
		"enabled": {niBgpNbrStateSrcConfig, "graceful_restart_enable"},
	},
	// FRR applies allowas-in per family; the configuration writes the one
	// count to every family of the neighbor.
	"as-path-options/state": {
		"allow-own-as": {niBgpNbrStateSrcAfStats, niBgpNbrAfStatsAllowOwnAsFld},
	},
}

const niBgpNbrAfStatsAllowOwnAsFld = "allow_own_as"

// The last NOTIFICATION FRR keeps is the one behind the last reset, so the
// leaves of one direction are filled only when it went that way.
var niBgpNbrLastNotifLeaves = []string{"last-notification-time", "last-notification-error-code", "last-notification-error-subcode"}

const (
	niBgpNbrAppliedMultihopTtlFld  = "ebgp_multihop_ttl"
	niBgpNbrStatsLastNotifDirFld   = "last_notification_direction"
	niBgpNbrStatsLastNotifSubFld   = "last_notification_error_subcode"
	niBgpNbrStatsLastNotifTimeFld  = "last_notification_time"
	niBgpNbrStateMessagesContainer = "state/messages"
)

var niBgpNbrStateOwnContainers = map[string]bool{"timers/state": true, "ebgp-multihop/state": true, "enable-bfd/state": true, "graceful-restart/state": true,
	"as-path-options/state": true}

var niBgpNbrStateLeafReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/(state|timers/state|transport/state|ebgp-multihop/state|enable-bfd/state|graceful-restart/state|as-path-options/state)(?:/([a-z-]+))?$`)
var niBgpNbrStateMsgReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/state/messages(?:/(sent|received)(?:/([A-Za-z-]+))?)?$`)

type niBgpNbrStateLeafReq struct {
	niName, protoName, neighborAddr string
	container, leaf                 string
	direction                       string // messages only: "" for both
}

// owned reports a request served here alone: a container of its own, a leaf
// of niBgpNbrStateLeafFlds, or a last NOTIFICATION leaf.
func (r *niBgpNbrStateLeafReq) owned() bool {
	if niBgpNbrStateOwnContainers[r.container] {
		return true
	}
	if r.container == niBgpNbrStateMessagesContainer {
		for _, l := range niBgpNbrLastNotifLeaves {
			if r.leaf == l {
				return true
			}
		}
		return false
	}
	_, ok := niBgpNbrStateLeafFlds[r.container][r.leaf]
	return r.leaf != "" && ok
}

func matchNiBgpNbrStateLeafReq(uri string) (*niBgpNbrStateLeafReq, bool) {
	if m := niBgpNbrStateLeafReqUriRe.FindStringSubmatch(uri); len(m) == 6 {
		return &niBgpNbrStateLeafReq{niName: m[1], protoName: m[2], neighborAddr: m[3], container: m[4], leaf: m[5]}, true
	}
	if m := niBgpNbrStateMsgReqUriRe.FindStringSubmatch(uri); len(m) == 6 {
		return &niBgpNbrStateLeafReq{niName: m[1], protoName: m[2], neighborAddr: m[3], container: niBgpNbrStateMessagesContainer,
			direction: m[4], leaf: m[5]}, true
	}
	return nil, false
}

// niBgpNbrStateLeafSet fills field f, a state leaf, from raw.
func niBgpNbrStateLeafSet(container, leaf string, f reflect.Value, raw string) error {
	if container == "ebgp-multihop/state" && leaf == "enabled" {
		ttl, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 8)
		if err != nil {
			return err
		}
		return niBgpGlobalLeafFromDb(f, strconv.FormatBool(ttl > 1))
	}
	return niBgpNbrLeafFromDb(leaf, f, raw)
}

// niBgpNbrErrorSubcodes maps a NOTIFICATION error code and subcode onto the
// OpenConfig subcode, per RFC 4271 and RFC 4486.
var niBgpNbrErrorSubcodes = map[[2]int]ocbinds.E_OpenconfigBgpTypes_BGP_ERROR_SUBCODE{
	{1, 1}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_CONNECTION_NOT_SYNCHRONIZED,
	{1, 2}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_BAD_MESSAGE_LENGTH,
	{1, 3}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_BAD_MESSAGE_TYPE,
	{2, 1}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_UNSUPPORTED_VERSION_NUMBER,
	{2, 2}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_BAD_PEER_AS,
	{2, 3}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_BAD_BGP_IDENTIFIER,
	{2, 4}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_UNSUPPORTED_OPTIONAL_PARAMETER,
	{2, 6}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_UNACCEPTABLE_HOLD_TIME,
	{2, 7}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_UNSUPPORTED_CAPABILITY,
	{3, 1}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_MALFORMED_ATTRIBUTE_LIST,
	{3, 2}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_UNRECOGNIZED_WELL_KNOWN_ATTRIBUTE,
	{3, 3}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_MISSING_WELL_KNOWN_ATTRIBUTE,
	{3, 4}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_ATTRIBUTE_FLAGS_ERROR,
	{3, 5}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_ATTRIBUTE_LENGTH_ERROR,
	{3, 6}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_INVALID_ORIGIN_ATTRIBUTE,
	{3, 8}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_INVALID_NEXT_HOP_ATTRIBUTE,
	{3, 9}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_OPTIONAL_ATTRIBUTE_ERROR,
	{3, 10}: ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_INVALID_NETWORK_FIELD,
	{3, 11}: ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_MALFORMED_AS_PATH,
	{5, 1}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_RECEIVE_UNEXPECTED_MESSAGE_OPENSENT,
	{5, 2}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_RECEIVE_UNEXPECTED_MESSAGE_OPENCONFIRM,
	{5, 3}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_RECEIVE_UNEXPECTED_MESSAGE_ESTABLISHED,
	{6, 1}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_MAX_NUM_PREFIXES_REACHED,
	{6, 2}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_ADMINISTRATIVE_SHUTDOWN,
	{6, 3}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_PEER_DE_CONFIGURED,
	{6, 4}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_ADMINISTRATIVE_RESET,
	{6, 5}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_CONNECTION_REJECTED,
	{6, 6}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_OTHER_CONFIG_CHANGE,
	{6, 7}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_CONN_COLLISION_RESOLUTION,
	{6, 8}:  ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_OUT_OF_RESOURCES,
}

func niBgpNbrErrorSubcode(code, subcode int) (ocbinds.E_OpenconfigBgpTypes_BGP_ERROR_SUBCODE, bool) {
	if subcode == 0 {
		return ocbinds.OpenconfigBgpTypes_BGP_ERROR_SUBCODE_UNSPECIFIC, true
	}
	v, ok := niBgpNbrErrorSubcodes[[2]int{code, subcode}]
	return v, ok
}

// niBgpNbrStateRows reads the rows the state leaves come from, each only when
// it is held: the applied and counter rows only while bgpmon is current.
func niBgpNbrStateRows(inParams XfmrParams, niName, neighborAddr string) (map[int]db.Value, error) {
	rows := map[int]db.Value{}
	if stateDb := inParams.dbs[db.StateDB]; stateDb != nil {
		if entry, found, err := niBgpNbrAppliedFresh(stateDb, niName, neighborAddr); err != nil {
			return nil, err
		} else if found {
			rows[niBgpNbrStateSrcApplied] = entry
		}
		if entry, found, err := niBgpNbrStatsFresh(stateDb, niName, neighborAddr); err != nil {
			return nil, err
		} else if found {
			rows[niBgpNbrStateSrcStats] = entry
		}
		for _, afiSafi := range niBgpNbrStateAfStatsFamilies {
			entry, found, err := niBgpNbrAfStatsFresh(stateDb, niName, neighborAddr, afiSafi)
			if err != nil {
				return nil, err
			}
			if found && strings.TrimSpace(entry.Field[niBgpNbrAfStatsAllowOwnAsFld]) != "" {
				rows[niBgpNbrStateSrcAfStats] = entry
				break
			}
		}
	}
	if cfgDb := inParams.dbs[db.ConfigDB]; cfgDb != nil {
		if entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrTbl}, db.Key{Comp: []string{niName, neighborAddr}}); err == nil {
			rows[niBgpNbrStateSrcConfig] = entry
		}
	}
	return rows, nil
}

// niBgpNbrStateLeavesToYang fills the state leaves of r from rows: in
// r.container only, and r.leaf only, when they are named.
func niBgpNbrStateLeavesToYang(ygRoot *ygot.GoStruct, r *niBgpNbrStateLeafReq, rows map[int]db.Value) error {
	for c, leaves := range niBgpNbrStateLeafFlds {
		if r.container != "" && r.container != c {
			continue
		}
		var held []string
		for leaf, src := range leaves {
			if (r.leaf == "" || r.leaf == leaf) && strings.TrimSpace(rows[src.src].Field[src.fld]) != "" {
				held = append(held, leaf)
			}
		}
		if len(held) == 0 {
			continue
		}
		node := ocWalkPath(reflect.ValueOf(getNiBgpNbrObj(ygRoot, r.niName, r.protoName, r.neighborAddr, true)), c, true)
		if !node.IsValid() {
			continue
		}
		for _, leaf := range held {
			src := leaves[leaf]
			if err := niBgpNbrStateLeafSet(c, leaf, ocFieldByPath(node, leaf), rows[src.src].Field[src.fld]); err != nil {
				return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid value %q for neighbor %s %s %s: %v",
					rows[src.src].Field[src.fld], r.neighborAddr, c, leaf, err)}
			}
		}
	}
	if (r.container == "state" && r.leaf == "") || r.container == niBgpNbrStateMessagesContainer {
		return niBgpNbrLastNotifToYang(ygRoot, r, rows[niBgpNbrStateSrcStats])
	}
	return nil
}

// niBgpNbrLastNotifToYang fills the last NOTIFICATION leaves of the direction
// it went in.
func niBgpNbrLastNotifToYang(ygRoot *ygot.GoStruct, r *niBgpNbrStateLeafReq, stats db.Value) error {
	dir := strings.TrimSpace(stats.Field[niBgpNbrStatsLastNotifDirFld])
	if dir == "" || (r.direction != "" && r.direction != dir) {
		return nil
	}
	if r.leaf != "" && r.container == niBgpNbrStateMessagesContainer && !r.owned() {
		return nil
	}
	wants := func(leaf string) bool {
		return r.leaf == "" || r.leaf == leaf || r.container != niBgpNbrStateMessagesContainer
	}
	code, codeErr := strconv.Atoi(strings.TrimSpace(stats.Field[niBgpNbrStatsLastNotifErrCodeFld]))
	if codeErr != nil {
		return nil
	}
	stateObj := getNiBgpNbrStateObj(ygRoot, r.niName, r.protoName, r.neighborAddr, true)
	if stateObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(stateObj)
	msgs := stateObj.Messages
	ygot.BuildEmptyTree(msgs)
	var codeFld, subFld, timeFld reflect.Value
	if dir == niBgpNbrSentDir {
		v := reflect.ValueOf(msgs.Sent).Elem()
		codeFld, subFld, timeFld = v.FieldByName("LastNotificationErrorCode"), v.FieldByName("LastNotificationErrorSubcode"), v.FieldByName("LastNotificationTime")
	} else {
		v := reflect.ValueOf(msgs.Received).Elem()
		codeFld, subFld, timeFld = v.FieldByName("LastNotificationErrorCode"), v.FieldByName("LastNotificationErrorSubcode"), v.FieldByName("LastNotificationTime")
	}
	if wants("last-notification-error-code") {
		if ocCode, ok := niBgpNbrLastNotifErrCodeFromFrr(uint8(code)); ok {
			codeFld.Set(reflect.ValueOf(ocCode))
		}
	}
	if wants("last-notification-error-subcode") {
		if sub, err := strconv.Atoi(strings.TrimSpace(stats.Field[niBgpNbrStatsLastNotifSubFld])); err == nil {
			if ocSub, ok := niBgpNbrErrorSubcode(code, sub); ok {
				subFld.Set(reflect.ValueOf(ocSub))
			}
		}
	}
	if wants("last-notification-time") {
		if secs, err := strconv.ParseUint(strings.TrimSpace(stats.Field[niBgpNbrStatsLastNotifTimeFld]), 10, 64); err == nil {
			ns := secs * niBgpNbrLastEstNsPerSec
			timeFld.Set(reflect.ValueOf(&ns))
		}
	}
	return nil
}

var DbToYang_ni_bgp_nbr_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	r, ok := matchNiBgpNbrStateLeafReq(inParams.requestUri)
	if ok {
		if err := validateNiBgpProtoName(r.protoName); err != nil {
			return err
		}
	}
	if !ok || !r.owned() {
		if err := dbToYangNiBgpNbrStateBase(inParams); err != nil {
			return err
		}
		if !ok || r.leaf != "" {
			return nil
		}
	}
	rows, err := niBgpNbrStateRows(inParams, r.niName, r.neighborAddr)
	if err != nil {
		return err
	}
	return niBgpNbrStateLeavesToYang(inParams.ygRoot, r, rows)
}

// niBgpNbrStateLeafSubscribe maps a subscription to a request served here
// alone: CONFIG_DB and BGP_NEIGH_APPLIED change only with the configuration
// or the session and are offered on-change, the counter row is sampled.
func niBgpNbrStateLeafSubscribe(r *niBgpNbrStateLeafReq) XfmrSubscOutParams {
	var result XfmrSubscOutParams
	ni, nbr := r.niName, r.neighborAddr
	if ni == "" {
		ni = "*"
	}
	if nbr == "" {
		nbr = "*"
	}
	key := ni + "|" + nbr
	if r.container == niBgpNbrStateMessagesContainer {
		result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrStatsTbl: {key: {}}}}
		result.onChange = OnchangeDisable
		result.nOpts = &notificationOpts{pType: Sample}
		return result
	}
	applied, config, afStats := map[string]string{}, map[string]string{}, map[string]string{}
	for leaf, src := range niBgpNbrStateLeafFlds[r.container] {
		if r.leaf != "" && r.leaf != leaf {
			continue
		}
		switch src.src {
		case niBgpNbrStateSrcConfig:
			config[src.fld] = leaf
		case niBgpNbrStateSrcAfStats:
			afStats[src.fld] = leaf
		default:
			applied[src.fld] = leaf
		}
	}
	result.dbDataMap = RedisDbSubscribeMap{}
	if len(applied) > 0 || len(afStats) > 0 {
		result.dbDataMap[db.StateDB] = map[string]map[string]map[string]string{}
	}
	if len(applied) > 0 {
		result.dbDataMap[db.StateDB][niBgpNbrAppliedTbl] = map[string]map[string]string{key: applied}
	}
	if len(afStats) > 0 {
		result.dbDataMap[db.StateDB][niBgpNbrAfStatsTbl] = map[string]map[string]string{key + "|*": afStats}
	}
	if len(config) > 0 {
		result.dbDataMap[db.ConfigDB] = map[string]map[string]map[string]string{niBgpNbrTbl: {key: config}}
	}
	result.onChange = OnchangeEnable
	result.nOpts = &notificationOpts{pType: OnChange}
	return result
}

var Subscribe_ni_bgp_nbr_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	if inParams.subscProc == TRANSLATE_SUBSCRIBE {
		uri := inParams.uri
		if reqUri := strings.TrimSuffix(inParams.requestURI, "/"); reqUri != "" && strings.HasPrefix(reqUri, strings.TrimSuffix(uri, "/")) {
			uri = reqUri
		}
		pathInfo := NewPathInfo(uri)
		if r, ok := matchNiBgpNbrStateLeafReq(niBgpNbrSubscKeyedUri(uri, pathInfo)); ok && r.owned() {
			if err := validateNiBgpProtoNameInUri(inParams.uri); err != nil {
				return XfmrSubscOutParams{}, err
			}
			r.niName, r.neighborAddr = strings.TrimSpace(pathInfo.Var("name")), pathInfo.Var("neighbor-address")
			return niBgpNbrStateLeafSubscribe(r), nil
		}
	}
	return subscribeNiBgpNbrStateBase(inParams)
}

// niBgpNbrSubscKeyedUri fills the list keys a subscription leaves open with a
// wildcard, so that the request patterns, which require them, still match.
func niBgpNbrSubscKeyedUri(uri string, pathInfo *PathInfo) string {
	uri = strings.Replace(uri, "/network-instance/", "/network-instance[name=*]/", 1)
	uri = strings.Replace(uri, "/neighbors/neighbor/", "/neighbors/neighbor[neighbor-address=*]/", 1)
	return uri
}

// The neighbor afi-safi state leaves served here: the instance's identity and
// what FRR runs it with, and the prefix limit FRR enforces.
var niBgpNbrAfStateLeafFlds = map[string]string{
	"max-prefixes":          "max_prefixes",
	"prevent-teardown":      "prevent_teardown",
	"warning-threshold-pct": "warning_threshold_pct",
}

var niBgpNbrAfStateLeafReqUriRe = regexp.MustCompile(niBgpNbrStateUriPfx +
	`/afi-safis/afi-safi\[afi-safi-name=([^\]]+)\]/(state|ipv4-unicast/prefix-limit/state|ipv6-unicast/prefix-limit/state|ipv4-unicast/prefix-limit-received/state|ipv6-unicast/prefix-limit-received/state)(?:/([a-z-]+))?$`)

type niBgpNbrAfStateLeafReq struct {
	niName, protoName, neighborAddr, afiSafi string
	container, leaf                          string
}

func (r *niBgpNbrAfStateLeafReq) owned() bool {
	if r.container != "state" {
		return true
	}
	switch r.leaf {
	case "afi-safi-name", "enabled", niBgpNbrSendCommTypeLeaf:
		return true
	}
	return false
}

func matchNiBgpNbrAfStateLeafReq(uri string) (*niBgpNbrAfStateLeafReq, bool) {
	m := niBgpNbrAfStateLeafReqUriRe.FindStringSubmatch(uri)
	if len(m) != 7 {
		return nil, false
	}
	afiSafi, ok := normalizeNiBgpNbrAfiSafiName(m[4])
	if !ok {
		return nil, false
	}
	return &niBgpNbrAfStateLeafReq{niName: m[1], protoName: m[2], neighborAddr: m[3], afiSafi: afiSafi, container: m[5], leaf: m[6]}, true
}

// niBgpNbrAfPfxLimitStateContainer names the prefix-limit state container of
// afiSafi that carries the limit: prefix-limit-received when the configured
// limit counts every prefix received.
func niBgpNbrAfPfxLimitStateContainer(cfgDb *db.DB, niName, neighborAddr, afiSafi string) string {
	family := "ipv4-unicast"
	if afiSafi == niBgpNbrAfiSafiIPv6 {
		family = "ipv6-unicast"
	}
	cont := "prefix-limit"
	if cfgDb != nil {
		if entry, err := cfgDb.GetEntry(&db.TableSpec{Name: niBgpNbrAfTbl}, db.Key{Comp: []string{niName, neighborAddr, afiSafi}}); err == nil &&
			strings.TrimSpace(entry.Field[niBgpNbrMaxPfxForceFld]) == "true" {
			cont = "prefix-limit-received"
		}
	}
	return family + "/" + cont + "/state"
}

// niBgpNbrAfStateLeavesToYang fills the afi-safi state leaves of r from the
// BGP_NEIGH_AF_STATS row, which bgpmon writes for every family FRR runs.
func niBgpNbrAfStateLeavesToYang(inParams XfmrParams, r *niBgpNbrAfStateLeafReq) error {
	stateDb := inParams.dbs[db.StateDB]
	if stateDb == nil {
		return nil
	}
	entry, found, err := niBgpNbrAfStatsFresh(stateDb, r.niName, r.neighborAddr, r.afiSafi)
	if err != nil || !found {
		return err
	}
	afObj := getNiBgpNbrAfObj(inParams.ygRoot, r.niName, r.protoName, r.neighborAddr, r.afiSafi, true)
	if afObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(afObj)
	if r.container == "state" || r.container == "" {
		if r.leaf == "" || r.leaf == "afi-safi-name" {
			if name, ok := niBgpNbrAfiSafiEnumFromName(r.afiSafi); ok {
				afObj.State.AfiSafiName = name
			}
		}
		if r.leaf == "" || r.leaf == "enabled" {
			enabled := true
			afObj.State.Enabled = &enabled
		}
		if r.leaf == "" || r.leaf == niBgpNbrSendCommTypeLeaf {
			var comm []ocbinds.E_OpenconfigBgp_CommunityType
			for _, c := range strings.Split(entry.Field["send_community"], ",") {
				switch strings.TrimSpace(c) {
				case "standard":
					comm = append(comm, ocbinds.OpenconfigBgp_CommunityType_STANDARD)
				case "extended":
					comm = append(comm, ocbinds.OpenconfigBgp_CommunityType_EXTENDED)
				case "large":
					comm = append(comm, ocbinds.OpenconfigBgp_CommunityType_LARGE)
				}
			}
			if len(comm) > 0 {
				afObj.State.SendCommunityType = comm
			}
		}
		if r.container == "state" {
			return nil
		}
	}
	if strings.TrimSpace(entry.Field["max_prefixes"]) == "" {
		return nil
	}
	cont := niBgpNbrAfPfxLimitStateContainer(inParams.dbs[db.ConfigDB], r.niName, r.neighborAddr, r.afiSafi)
	if r.container != "" && r.container != cont {
		return nil
	}
	node := ocWalkPath(reflect.ValueOf(afObj), cont, true)
	for leaf, fld := range niBgpNbrAfStateLeafFlds {
		if (r.leaf != "" && r.leaf != leaf) || strings.TrimSpace(entry.Field[fld]) == "" {
			continue
		}
		if err := niBgpGlobalLeafFromDb(ocFieldByPath(node, leaf), entry.Field[fld]); err != nil {
			return tlerr.InvalidArgsError{Format: fmt.Sprintf("invalid %s.%s value %q for neighbor %s: %v", niBgpNbrAfStatsTbl, fld, entry.Field[fld], r.neighborAddr, err)}
		}
	}
	return nil
}

var DbToYang_ni_bgp_nbr_af_state_xfmr SubTreeXfmrDbToYang = func(inParams XfmrParams) error {
	r, ok := matchNiBgpNbrAfStateLeafReq(inParams.requestUri)
	if !ok {
		// A read above the afi-safi state containers reaches here once per
		// subtree, with uri naming it while requestUri names the anchor.
		r, ok = matchNiBgpNbrAfStateLeafReq(inParams.uri)
		if ok && r.leaf != "" {
			ok = false
		}
	}
	if ok {
		if err := validateNiBgpProtoName(r.protoName); err != nil {
			return err
		}
	}
	if !ok || !r.owned() {
		if err := dbToYangNiBgpNbrAfStateBase(inParams); err != nil {
			return err
		}
		if !ok || r.leaf != "" {
			return nil
		}
	}
	return niBgpNbrAfStateLeavesToYang(inParams, r)
}

var Subscribe_ni_bgp_nbr_af_state_xfmr SubTreeXfmrSubscribe = func(inParams XfmrSubscInParams) (XfmrSubscOutParams, error) {
	if inParams.subscProc == TRANSLATE_SUBSCRIBE {
		uri := inParams.uri
		if reqUri := strings.TrimSuffix(inParams.requestURI, "/"); reqUri != "" && strings.HasPrefix(reqUri, strings.TrimSuffix(uri, "/")) {
			uri = reqUri
		}
		pathInfo := NewPathInfo(uri)
		keyed := strings.Replace(niBgpNbrSubscKeyedUri(uri, pathInfo), "/afi-safis/afi-safi/", "/afi-safis/afi-safi[afi-safi-name=*]/", 1)
		if r, ok := matchNiBgpNbrAfStateLeafReq(keyed); ok && r.container != "state" {
			if err := validateNiBgpProtoNameInUri(inParams.uri); err != nil {
				return XfmrSubscOutParams{}, err
			}
			ni := niBgpSubscVar(pathInfo, "name")
			nbr := niBgpSubscVar(pathInfo, "neighbor-address")
			flds := map[string]string{}
			for leaf, fld := range niBgpNbrAfStateLeafFlds {
				if r.leaf == "" || r.leaf == leaf {
					flds[fld] = leaf
				}
			}
			var result XfmrSubscOutParams
			result.dbDataMap = RedisDbSubscribeMap{db.StateDB: {niBgpNbrAfStatsTbl: {ni + "|" + nbr + "|" + niBgpSubscAf(pathInfo): flds}}}
			result.onChange = OnchangeEnable
			result.nOpts = &notificationOpts{pType: OnChange}
			return result, nil
		}
	}
	return subscribeNiBgpNbrAfStateBase(inParams)
}
