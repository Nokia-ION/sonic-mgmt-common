////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia, Inc.                                                //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");          //
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

package transformer

import (
	"fmt"
	"net"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/Azure/sonic-mgmt-common/translib/db"
	"github.com/Azure/sonic-mgmt-common/translib/ocbinds"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	"github.com/openconfig/ygot/ygot"
)

const (
	rpolPrefixTbl            = "PREFIX"
	rpolPrefixActionFld      = "action"
	rpolPrefixActionPermit   = "permit"
	rpolPrefixSetModeIpv4    = "IPv4"
	rpolPrefixSetModeIpv6    = "IPv6"
	rpolMasklengthRangeExact = "exact"

	// rpolPrefixSeqStep is the spacing used when allocating the sequence number
	// that PREFIX carries in its key. OpenConfig keys a prefix by ip-prefix and
	// masklength-range only and has no sequence-number leaf, so the sequence is
	// chosen here rather than requested. FRR numbers prefix-list entries it is
	// given without one the same way, from 5 upwards in steps of 5, and since
	// every entry written through OpenConfig is a permit the order it produces
	// does not change which prefixes the list matches.
	rpolPrefixSeqStep = 5
)

// rpolPrefixRow is one PREFIX row, holding both the sequence number that only
// CONFIG_DB knows about and the two key components OpenConfig addresses it by.
type rpolPrefixRow struct {
	seq             uint32
	ipPrefix        string
	masklengthRange string
	action          string
}

// rpolPrefixEntry is a prefix a request asks to be written, before a sequence
// number has been allocated for it.
type rpolPrefixEntry struct {
	ipPrefix        string
	masklengthRange string
}

// rpolPrefixSetModeToDb narrows the OpenConfig mode enum onto the two spellings
// PREFIX_SET stores. FRR keeps its prefix-lists per address family and the
// CONFIG_DB enum is IPv4 or IPv6, so MIXED is refused rather than resolved to
// one of them.
func rpolPrefixSetModeToDb(v ocbinds.E_OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode) (string, error) {
	switch v {
	case ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV4:
		return rpolPrefixSetModeIpv4, nil
	case ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV6:
		return rpolPrefixSetModeIpv6, nil
	case ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_MIXED:
		return "", tlerr.NotSupportedError{Format: "routing-policy prefix-set mode MIXED has no CONFIG_DB mapping; configure one prefix-set per address family"}
	default:
		return "", tlerr.InvalidArgsError{Format: "unsupported routing-policy prefix-set mode value in request payload"}
	}
}

func rpolDbToPrefixSetMode(v string) (ocbinds.E_OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ipv4":
		return ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV4, true
	case "ipv6":
		return ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV6, true
	}
	return ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_UNSET, false
}

func parseRpolPrefixSetModeFromParam(param interface{}) (ocbinds.E_OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode, bool) {
	switch v := param.(type) {
	case ocbinds.E_OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode:
		return v, v != ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_UNSET
	case *ocbinds.E_OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode:
		if v != nil {
			return *v, *v != ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_UNSET
		}
	case string:
		switch strings.ToUpper(strings.TrimSpace(v)) {
		case "IPV4":
			return ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV4, true
		case "IPV6":
			return ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_IPV6, true
		case "MIXED":
			return ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_MIXED, true
		}
	}
	return ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_UNSET, false
}

// parseRpolPrefixSetNameFromParam reads a prefix-set name out of a leaf targeted
// request payload, which can arrive as the bare leaf value or as the config
// object that carries it.
func parseRpolPrefixSetNameFromParam(param interface{}) (string, bool) {
	if param == nil {
		return "", false
	}
	if name, ok := parseRpolStringValue(param); ok {
		return name, true
	}

	rv := reflect.ValueOf(param)
	for rv.IsValid() && (rv.Kind() == reflect.Interface || rv.Kind() == reflect.Ptr) {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return "", false
	}

	if rv.Kind() == reflect.Struct {
		fv := rv.FieldByName("Name")
		if fv.IsValid() && fv.CanInterface() {
			if name, ok := parseRpolStringValue(fv.Interface()); ok {
				return name, true
			}
		}
	}

	if rv.Kind() == reflect.Map {
		for _, mk := range rv.MapKeys() {
			k := fmt.Sprint(mk.Interface())
			if k == "name" || strings.HasSuffix(k, ":name") {
				mv := rv.MapIndex(mk)
				if mv.IsValid() && mv.CanInterface() {
					if name, ok := parseRpolStringValue(mv.Interface()); ok {
						return name, true
					}
				}
			}
		}
	}
	return "", false
}

func getRpolPrefixSetObj(s *ygot.GoStruct, prefixSetName string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet {
	if s == nil || *s == nil {
		return nil
	}

	deviceObj, ok := (*s).(*ocbinds.Device)
	if !ok || deviceObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(deviceObj)

	if deviceObj.RoutingPolicy == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy{}
	}
	if deviceObj.RoutingPolicy.DefinedSets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets{}
	}
	if deviceObj.RoutingPolicy.DefinedSets.PrefixSets == nil {
		if !createIfMissing {
			return nil
		}
		deviceObj.RoutingPolicy.DefinedSets.PrefixSets = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets{}
	}

	if psObj, ok := deviceObj.RoutingPolicy.DefinedSets.PrefixSets.PrefixSet[prefixSetName]; ok && psObj != nil {
		ygot.BuildEmptyTree(psObj)
		return psObj
	}
	if !createIfMissing {
		return nil
	}
	psObj, err := deviceObj.RoutingPolicy.DefinedSets.PrefixSets.NewPrefixSet(prefixSetName)
	if err != nil || psObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(psObj)
	return psObj
}

func getRpolPrefixObj(s *ygot.GoStruct, prefixSetName, ipPrefix, masklengthRange string, createIfMissing bool) *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Prefixes_Prefix {
	psObj := getRpolPrefixSetObj(s, prefixSetName, createIfMissing)
	if psObj == nil {
		return nil
	}
	if psObj.Prefixes == nil {
		if !createIfMissing {
			return nil
		}
		psObj.Prefixes = &ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Prefixes{}
	}

	key := ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Prefixes_Prefix_Key{
		IpPrefix:        ipPrefix,
		MasklengthRange: masklengthRange,
	}
	if pfxObj, ok := psObj.Prefixes.Prefix[key]; ok && pfxObj != nil {
		ygot.BuildEmptyTree(pfxObj)
		return pfxObj
	}
	if !createIfMissing {
		return nil
	}
	pfxObj, err := psObj.Prefixes.NewPrefix(ipPrefix, masklengthRange)
	if err != nil || pfxObj == nil {
		return nil
	}
	ygot.BuildEmptyTree(pfxObj)
	return pfxObj
}

// rpolIpPrefixAf returns the PREFIX_SET mode spelling and the prefix length of a
// CIDR prefix. A prefix carrying host bits is refused rather than masked, since
// OpenConfig addresses a prefix entry by the very string that was written and
// silently canonicalising it would make a GET report a different list key than
// the SET used.
func rpolIpPrefixAf(ipPrefix string) (string, int, error) {
	s := strings.TrimSpace(ipPrefix)
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return "", 0, tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy prefix ip-prefix %s is not a valid CIDR prefix", ipPrefix)}
	}
	if !ipNet.IP.Equal(ip) {
		return "", 0, tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy prefix ip-prefix %s carries host bits; use its canonical form %s", ipPrefix, ipNet.String())}
	}
	prefixLen, _ := ipNet.Mask.Size()
	if !strings.Contains(s, ":") && ip.To4() != nil {
		return rpolPrefixSetModeIpv4, prefixLen, nil
	}
	return rpolPrefixSetModeIpv6, prefixLen, nil
}

// validateRpolPrefixEntry checks a prefix entry key against the address family
// rules FRR applies. "exact" renders as a bare prefix and N..M renders as
// "ge N le M", which FRR only accepts when the range widens the prefix it
// qualifies without running past the address family's maximum length.
func validateRpolPrefixEntry(ipPrefix, masklengthRange string) (string, error) {
	mode, prefixLen, err := rpolIpPrefixAf(ipPrefix)
	if err != nil {
		return "", err
	}

	r := strings.TrimSpace(masklengthRange)
	if r == "" {
		return "", tlerr.InvalidArgsError{Format: "routing-policy prefix masklength-range is missing"}
	}
	if r == rpolMasklengthRangeExact {
		return mode, nil
	}

	maxLen := 32
	if mode == rpolPrefixSetModeIpv6 {
		maxLen = 128
	}
	parts := strings.SplitN(r, "..", 2)
	if len(parts) != 2 {
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy prefix masklength-range %s must be \"exact\" or \"N..M\"", masklengthRange)}
	}
	low, lowErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	high, highErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if lowErr != nil || highErr != nil {
		return "", tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy prefix masklength-range %s must be \"exact\" or \"N..M\"", masklengthRange)}
	}
	if low > high || low < prefixLen || high > maxLen {
		return "", tlerr.InvalidArgsError{
			Format: fmt.Sprintf("routing-policy prefix masklength-range %s must satisfy %d <= N <= M <= %d for prefix %s",
				masklengthRange, prefixLen, maxLen, ipPrefix),
		}
	}
	return mode, nil
}

func makeRpolPrefixResMap(prefixSetName string, seq uint32, ipPrefix, masklengthRange string, fields map[string]string) map[string]map[string]db.Value {
	key := fmt.Sprintf("%s|%d|%s|%s", prefixSetName, seq, ipPrefix, masklengthRange)
	return map[string]map[string]db.Value{
		rpolPrefixTbl: {
			key: db.Value{Field: fields},
		},
	}
}

// rpolPrefixRowsFromDb returns the PREFIX rows of one set. OpenConfig has no
// sequence-number leaf, so a set holding the same prefix and range at two
// sequence numbers can only be reported once; the lowest sequence wins, which is
// the copy FRR reaches first.
func rpolPrefixRowsFromDb(cfgDb *db.DB, prefixSetName string) []rpolPrefixRow {
	if cfgDb == nil {
		return nil
	}
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolPrefixTbl})
	if err != nil {
		return nil
	}

	byOcKey := make(map[string]rpolPrefixRow)
	for _, k := range keys {
		row, ok := rpolPrefixRowFromKey(k, prefixSetName)
		if !ok {
			continue
		}
		if entry, entryErr := cfgDb.GetEntry(&db.TableSpec{Name: rpolPrefixTbl}, k); entryErr == nil {
			row.action = strings.TrimSpace(entry.Field[rpolPrefixActionFld])
		}
		ocKey := row.ipPrefix + "|" + row.masklengthRange
		if cur, dup := byOcKey[ocKey]; dup {
			if cur.seq <= row.seq {
				continue
			}
			xfmrLogInfo("routing-policy prefix-set %v holds %v at sequence %v and %v; reporting the lower one",
				prefixSetName, ocKey, cur.seq, row.seq)
		}
		byOcKey[ocKey] = row
	}

	rows := make([]rpolPrefixRow, 0, len(byOcKey))
	for _, row := range byOcKey {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].seq < rows[j].seq })
	return rows
}

// rpolPrefixRowFromKey reads a PREFIX key of the set given. PREFIX_NOSEQ_LIST
// shares the table with a three component key and is out of scope, so only the
// four component form is recognised.
func rpolPrefixRowFromKey(k db.Key, prefixSetName string) (rpolPrefixRow, bool) {
	if len(k.Comp) != 4 || k.Comp[0] != prefixSetName {
		return rpolPrefixRow{}, false
	}
	seq, err := strconv.ParseUint(strings.TrimSpace(k.Comp[1]), 10, 32)
	if err != nil {
		return rpolPrefixRow{}, false
	}
	return rpolPrefixRow{seq: uint32(seq), ipPrefix: k.Comp[2], masklengthRange: k.Comp[3]}, true
}

// rpolPrefixSetNamesFromDb returns every prefix-set name CONFIG_DB holds data
// for, taking PREFIX into account as well so that a set whose PREFIX_SET row is
// missing is still reported rather than hidden.
func rpolPrefixSetNamesFromDb(cfgDb *db.DB) []string {
	if cfgDb == nil {
		return nil
	}
	seen := make(map[string]bool)
	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolPrefixSetTbl}); err == nil {
		for _, k := range keys {
			if len(k.Comp) == 1 && strings.TrimSpace(k.Comp[0]) != "" {
				seen[k.Comp[0]] = true
			}
		}
	}
	if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolPrefixTbl}); err == nil {
		for _, k := range keys {
			if len(k.Comp) >= 1 && strings.TrimSpace(k.Comp[0]) != "" {
				seen[k.Comp[0]] = true
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// rpolAllocPrefixSeqs allocates the sequence number each entry is stored under.
// An entry that is already configured keeps its sequence, so that rewriting it
// updates the row instead of adding a second copy of the same prefix, and a new
// entry takes the next free step. Every entry of one request is allocated in one
// pass because a container level write can add several prefixes to a set that
// CONFIG_DB does not hold any of yet.
func rpolAllocPrefixSeqs(cfgDb *db.DB, prefixSetName string, entries []rpolPrefixEntry) map[rpolPrefixEntry]uint32 {
	existing := make(map[rpolPrefixEntry]uint32)
	var maxSeq uint32
	if cfgDb != nil {
		if keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolPrefixTbl}); err == nil {
			for _, k := range keys {
				row, ok := rpolPrefixRowFromKey(k, prefixSetName)
				if !ok {
					continue
				}
				entry := rpolPrefixEntry{ipPrefix: row.ipPrefix, masklengthRange: row.masklengthRange}
				if cur, dup := existing[entry]; !dup || row.seq < cur {
					existing[entry] = row.seq
				}
				if row.seq > maxSeq {
					maxSeq = row.seq
				}
			}
		}
	}

	allocated := make(map[rpolPrefixEntry]uint32, len(entries))
	for _, entry := range entries {
		if seq, ok := existing[entry]; ok {
			allocated[entry] = seq
			continue
		}
		if _, ok := allocated[entry]; ok {
			// The same entry twice in one request needs one row, not two.
			continue
		}
		maxSeq = (maxSeq/rpolPrefixSeqStep + 1) * rpolPrefixSeqStep
		allocated[entry] = maxSeq
	}
	return allocated
}

// rpolPrefixSetModeFields returns the PREFIX_SET fields to write alongside
// whatever a request carries. frrcfgd drops a prefix-set that reaches it without
// a mode, and every prefix in it with the set, so a request that only names a
// set still seeds the mode CONFIG_DB defaults to. An already stored mode is left
// alone so that naming a set does not turn an IPv6 one into IPv4.
func rpolPrefixSetModeFields(cfgDb *db.DB, prefixSetName, mode string) map[string]string {
	if mode != "" {
		return map[string]string{rpolPrefixSetModeFld: mode}
	}
	if cfgDb != nil {
		if entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolPrefixSetTbl}, db.Key{Comp: []string{prefixSetName}}); err == nil {
			if strings.TrimSpace(entry.Field[rpolPrefixSetModeFld]) != "" {
				return map[string]string{}
			}
		}
	}
	return map[string]string{rpolPrefixSetModeFld: rpolPrefixSetModeIpv4}
}

// rpolPayloadPrefixEntries returns the prefix entries a payload carries beneath
// one prefix-set, validating each key as it goes. A container level request is
// mapped by replaying the prefix-set node, so this is where the prefixes of such
// a request are picked up.
func rpolPayloadPrefixEntries(psObj *ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet) ([]rpolPrefixEntry, error) {
	if psObj == nil || psObj.Prefixes == nil || len(psObj.Prefixes.Prefix) == 0 {
		return nil, nil
	}

	entries := make([]rpolPrefixEntry, 0, len(psObj.Prefixes.Prefix))
	for key, pfxObj := range psObj.Prefixes.Prefix {
		ipPrefix := strings.TrimSpace(key.IpPrefix)
		masklengthRange := strings.TrimSpace(key.MasklengthRange)
		if pfxObj != nil && pfxObj.Config != nil {
			if err := rpolCheckPrefixKeyLeaf(pfxObj.Config.IpPrefix, ipPrefix, "ip-prefix"); err != nil {
				return nil, err
			}
			if err := rpolCheckPrefixKeyLeaf(pfxObj.Config.MasklengthRange, masklengthRange, "masklength-range"); err != nil {
				return nil, err
			}
		}
		entries = append(entries, rpolPrefixEntry{ipPrefix: ipPrefix, masklengthRange: masklengthRange})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ipPrefix != entries[j].ipPrefix {
			return entries[i].ipPrefix < entries[j].ipPrefix
		}
		return entries[i].masklengthRange < entries[j].masklengthRange
	})
	return entries, nil
}

func rpolCheckPrefixKeyLeaf(payload *string, key, leafName string) error {
	if payload == nil {
		return nil
	}
	if v := strings.TrimSpace(*payload); v != "" && v != key {
		return tlerr.InvalidArgsError{
			Format: fmt.Sprintf("routing-policy prefix %s payload (%s) does not match key (%s)", leafName, v, key),
		}
	}
	return nil
}

// rpolMapPrefixEntries maps prefix entries onto PREFIX rows. Every row written
// through OpenConfig is a permit: a prefix-set is purely a match set in
// OpenConfig, with accept and reject carried by the policy statement that
// references it, so permit is the only action the model can express. frrcfgd
// also ignores a PREFIX row that reaches it without an action, so the field is
// written rather than left to a default. An action already stored is left alone,
// which keeps a deny entry configured by other means from being rewritten.
func rpolMapPrefixEntries(cfgDb *db.DB, prefixSetName string, entries []rpolPrefixEntry, setMode string) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	if len(entries) == 0 {
		return res, nil
	}

	for _, entry := range entries {
		mode, err := validateRpolPrefixEntry(entry.ipPrefix, entry.masklengthRange)
		if err != nil {
			return res, err
		}
		if setMode != "" && mode != setMode {
			return res, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("routing-policy prefix %s is %s but prefix-set %s has mode %s",
					entry.ipPrefix, mode, prefixSetName, setMode),
			}
		}
	}

	seqs := rpolAllocPrefixSeqs(cfgDb, prefixSetName, entries)
	for _, entry := range entries {
		seq, ok := seqs[entry]
		if !ok {
			return res, tlerr.InvalidArgsError{
				Format: fmt.Sprintf("routing-policy could not allocate a sequence number for prefix %s in set %s", entry.ipPrefix, prefixSetName),
			}
		}
		fields := map[string]string{}
		if rpolStoredPrefixAction(cfgDb, prefixSetName, seq, entry) == "" {
			fields[rpolPrefixActionFld] = rpolPrefixActionPermit
		}
		res = mergeDbResMapFields(res, makeRpolPrefixResMap(prefixSetName, seq, entry.ipPrefix, entry.masklengthRange, fields))
	}
	return res, nil
}

func rpolStoredPrefixAction(cfgDb *db.DB, prefixSetName string, seq uint32, entry rpolPrefixEntry) string {
	if cfgDb == nil {
		return ""
	}
	key := db.Key{Comp: []string{prefixSetName, strconv.FormatUint(uint64(seq), 10), entry.ipPrefix, entry.masklengthRange}}
	row, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolPrefixTbl}, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(row.Field[rpolPrefixActionFld])
}

// rpolStoredPrefixSetMode returns the mode CONFIG_DB holds for a set, or an
// empty string when the set is unknown or carries no mode.
func rpolStoredPrefixSetMode(cfgDb *db.DB, prefixSetName string) string {
	if cfgDb == nil {
		return ""
	}
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolPrefixSetTbl}, db.Key{Comp: []string{prefixSetName}})
	if err != nil {
		return ""
	}
	mode := strings.TrimSpace(entry.Field[rpolPrefixSetModeFld])
	switch strings.ToLower(mode) {
	case "ipv4":
		return rpolPrefixSetModeIpv4
	case "ipv6":
		return rpolPrefixSetModeIpv6
	}
	return ""
}

// rpolDeletePrefixSetResMap removes a prefix-set together with the prefixes it
// holds. The PREFIX rows are keyed by the set name and would otherwise be left
// behind referring to a set that no longer exists.
func rpolDeletePrefixSetResMap(cfgDb *db.DB, prefixSetName string) map[string]map[string]db.Value {
	res := makeRpolPrefixSetResMap(prefixSetName, nil)
	if cfgDb == nil {
		return res
	}
	keys, err := cfgDb.GetKeys(&db.TableSpec{Name: rpolPrefixTbl})
	if err != nil {
		return res
	}
	for _, k := range keys {
		if len(k.Comp) < 1 || k.Comp[0] != prefixSetName {
			continue
		}
		res = mergeDbResMap(res, map[string]map[string]db.Value{
			rpolPrefixTbl: {strings.Join(k.Comp, "|"): db.Value{Field: nil}},
		})
	}
	return res
}

// rpolResolvePrefixSetMode works out the address family a match-prefix-set
// condition applies to. The condition only names a set, so the family comes from
// the set itself: the stored mode first, then the mode the same request is
// writing for that set, and last the family of the prefixes the set already
// holds. Nothing is guessed when none of those answer, because picking one would
// silently drive the wrong ROUTE_MAP field.
func rpolResolvePrefixSetMode(inParams XfmrParams, prefixSetName string) (string, error) {
	cfgDb := inParams.dbs[db.ConfigDB]
	if mode := rpolStoredPrefixSetMode(cfgDb, prefixSetName); mode != "" {
		return mode, nil
	}

	if psObj := getRpolPrefixSetObj(inParams.ygRoot, prefixSetName, false); psObj != nil {
		if psObj.Config != nil {
			if mode, err := rpolPrefixSetModeToDb(psObj.Config.Mode); err == nil {
				return mode, nil
			}
		}
		if psObj.Prefixes != nil {
			for key := range psObj.Prefixes.Prefix {
				if mode, _, err := rpolIpPrefixAf(key.IpPrefix); err == nil {
					return mode, nil
				}
			}
		}
	}

	for _, row := range rpolPrefixRowsFromDb(cfgDb, prefixSetName) {
		if mode, _, err := rpolIpPrefixAf(row.ipPrefix); err == nil {
			return mode, nil
		}
	}

	return "", tlerr.InvalidArgsError{
		Format: fmt.Sprintf("routing-policy prefix-set %s is not configured; create it under defined-sets/prefix-sets with a mode before matching on it", prefixSetName),
	}
}

// rpolMapPrefixSetRequest maps a request targeted at a prefix-set node. The
// prefixes the payload carries beneath the set are mapped in the same call so
// that one replay covers the whole set, which is how a container level request
// reaches them, and so that the sequence numbers of a set being created are
// allocated together.
func rpolMapPrefixSetRequest(inParams XfmrParams, reqInfo *rpolReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	cfgDb := inParams.dbs[db.ConfigDB]
	psObj := getRpolPrefixSetObj(inParams.ygRoot, reqInfo.prefixSetName, false)

	if name, ok := parseRpolPrefixSetNameFromParam(inParams.param); ok && name != reqInfo.prefixSetName {
		return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("prefix-set name payload (%s) does not match key (%s)", name, reqInfo.prefixSetName)}
	}
	if psObj != nil && psObj.Config != nil && psObj.Config.Name != nil {
		if name := strings.TrimSpace(*psObj.Config.Name); name != "" && name != reqInfo.prefixSetName {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("prefix-set config name payload (%s) does not match key (%s)", name, reqInfo.prefixSetName)}
		}
	}

	mode := ""
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetModeLeaf {
		ocMode := ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_UNSET
		if v, ok := parseRpolPrefixSetModeFromParam(inParams.param); ok {
			ocMode = v
		} else if psObj != nil && psObj.Config != nil {
			ocMode = psObj.Config.Mode
		}
		if ocMode == ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_UNSET &&
			reqInfo.targetLeaf == rpolPrefixSetModeLeaf {
			return res, tlerr.InvalidArgsError{Format: "routing-policy prefix-set config mode is missing in request payload"}
		}
		if ocMode != ocbinds.OpenconfigRoutingPolicy_RoutingPolicy_DefinedSets_PrefixSets_PrefixSet_Config_Mode_UNSET {
			dbMode, err := rpolPrefixSetModeToDb(ocMode)
			if err != nil {
				return res, err
			}
			mode = dbMode
		}
	}

	if reqInfo.targetLeaf == rpolPrefixSetModeLeaf {
		return makeRpolPrefixSetResMap(reqInfo.prefixSetName, map[string]string{rpolPrefixSetModeFld: mode}), nil
	}

	res = makeRpolPrefixSetResMap(reqInfo.prefixSetName, rpolPrefixSetModeFields(cfgDb, reqInfo.prefixSetName, mode))

	entries, err := rpolPayloadPrefixEntries(psObj)
	if err != nil {
		return res, err
	}
	if len(entries) == 0 {
		return res, nil
	}

	effectiveMode := mode
	if effectiveMode == "" {
		effectiveMode = rpolStoredPrefixSetMode(cfgDb, reqInfo.prefixSetName)
	}
	prefixRes, err := rpolMapPrefixEntries(cfgDb, reqInfo.prefixSetName, entries, effectiveMode)
	if err != nil {
		return res, err
	}
	return mergeDbResMapFields(res, prefixRes), nil
}

// rpolMapPrefixEntryRequest maps a request targeted at one prefix entry. The
// prefix-set row is seeded when it is missing, with the mode taken from the
// address family of the prefix itself, which the entry states unambiguously.
func rpolMapPrefixEntryRequest(inParams XfmrParams, reqInfo *rpolReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	cfgDb := inParams.dbs[db.ConfigDB]

	if reqInfo.targetLeaf == rpolIpPrefixLeaf {
		if v, ok := parseRpolStringValue(inParams.param); ok && v != reqInfo.ipPrefix {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy prefix ip-prefix payload (%s) does not match key (%s)", v, reqInfo.ipPrefix)}
		}
	}
	if reqInfo.targetLeaf == rpolMasklengthRangeLeaf {
		if v, ok := parseRpolStringValue(inParams.param); ok && v != reqInfo.masklengthRange {
			return res, tlerr.InvalidArgsError{Format: fmt.Sprintf("routing-policy prefix masklength-range payload (%s) does not match key (%s)", v, reqInfo.masklengthRange)}
		}
	}
	if pfxObj := getRpolPrefixObj(inParams.ygRoot, reqInfo.prefixSetName, reqInfo.ipPrefix, reqInfo.masklengthRange, false); pfxObj != nil && pfxObj.Config != nil {
		if err := rpolCheckPrefixKeyLeaf(pfxObj.Config.IpPrefix, reqInfo.ipPrefix, "ip-prefix"); err != nil {
			return res, err
		}
		if err := rpolCheckPrefixKeyLeaf(pfxObj.Config.MasklengthRange, reqInfo.masklengthRange, "masklength-range"); err != nil {
			return res, err
		}
	}

	mode, err := validateRpolPrefixEntry(reqInfo.ipPrefix, reqInfo.masklengthRange)
	if err != nil {
		return res, err
	}
	storedMode := rpolStoredPrefixSetMode(cfgDb, reqInfo.prefixSetName)
	if storedMode != "" && storedMode != mode {
		return res, tlerr.InvalidArgsError{
			Format: fmt.Sprintf("routing-policy prefix %s is %s but prefix-set %s has mode %s",
				reqInfo.ipPrefix, mode, reqInfo.prefixSetName, storedMode),
		}
	}

	entries := []rpolPrefixEntry{{ipPrefix: reqInfo.ipPrefix, masklengthRange: reqInfo.masklengthRange}}
	res, err = rpolMapPrefixEntries(cfgDb, reqInfo.prefixSetName, entries, mode)
	if err != nil {
		return res, err
	}
	if storedMode == "" {
		res = mergeDbResMapFields(res, makeRpolPrefixSetResMap(reqInfo.prefixSetName, map[string]string{rpolPrefixSetModeFld: mode}))
	}
	return res, nil
}

// rpolDeletePrefixEntryResMap removes one prefix entry. ip-prefix and
// masklength-range are both list keys, so removing either of them removes the
// entry they identify rather than clearing a field of it.
func rpolDeletePrefixEntryResMap(cfgDb *db.DB, reqInfo *rpolReqInfo) (map[string]map[string]db.Value, error) {
	res := make(map[string]map[string]db.Value)
	entry := rpolPrefixEntry{ipPrefix: reqInfo.ipPrefix, masklengthRange: reqInfo.masklengthRange}
	seqs := rpolAllocPrefixSeqs(cfgDb, reqInfo.prefixSetName, []rpolPrefixEntry{entry})
	seq, ok := seqs[entry]
	if !ok {
		return res, tlerr.NotFoundError{Format: fmt.Sprintf("routing-policy prefix %s %s is not configured in prefix-set %s",
			reqInfo.ipPrefix, reqInfo.masklengthRange, reqInfo.prefixSetName)}
	}
	return makeRpolPrefixResMap(reqInfo.prefixSetName, seq, reqInfo.ipPrefix, reqInfo.masklengthRange, nil), nil
}

// rpolReadPrefixSet populates a prefix-set from CONFIG_DB. The prefixes are
// populated alongside the set config so that one replay answers a request
// anchored anywhere in the set, with translib pruning the result to the node
// that was asked for.
func rpolReadPrefixSet(inParams XfmrParams, reqInfo *rpolReqInfo) error {
	cfgDb := inParams.dbs[db.ConfigDB]
	entry, err := cfgDb.GetEntry(&db.TableSpec{Name: rpolPrefixSetTbl}, db.Key{Comp: []string{reqInfo.prefixSetName}})
	rows := rpolPrefixRowsFromDb(cfgDb, reqInfo.prefixSetName)
	if err != nil && len(rows) == 0 {
		return err
	}

	psObj := getRpolPrefixSetObj(inParams.ygRoot, reqInfo.prefixSetName, true)
	if psObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy prefix-set object in YANG tree"}
	}
	name := reqInfo.prefixSetName
	psObj.Name = &name
	if psObj.Config == nil {
		return nil
	}
	// Keep the key leaf and the config leaf in step on GET so that both /name
	// and /config/name answer with the same value.
	psObj.Config.Name = &name
	if reqInfo.targetLeaf == "" || reqInfo.targetLeaf == rpolPrefixSetModeLeaf {
		if mode, ok := rpolDbToPrefixSetMode(entry.Field[rpolPrefixSetModeFld]); ok {
			psObj.Config.Mode = mode
		}
	}
	if reqInfo.targetLeaf == rpolPrefixSetModeLeaf {
		return nil
	}

	for _, row := range rows {
		if err := rpolPopulatePrefixEntry(inParams.ygRoot, reqInfo.prefixSetName, row.ipPrefix, row.masklengthRange, ""); err != nil {
			return err
		}
	}
	return nil
}

// rpolReadPrefixEntry populates one prefix entry from CONFIG_DB.
func rpolReadPrefixEntry(inParams XfmrParams, reqInfo *rpolReqInfo) error {
	cfgDb := inParams.dbs[db.ConfigDB]
	found := false
	for _, row := range rpolPrefixRowsFromDb(cfgDb, reqInfo.prefixSetName) {
		if row.ipPrefix == reqInfo.ipPrefix && row.masklengthRange == reqInfo.masklengthRange {
			found = true
			break
		}
	}
	if !found {
		return tlerr.NotFoundError{Format: fmt.Sprintf("routing-policy prefix %s %s is not configured in prefix-set %s",
			reqInfo.ipPrefix, reqInfo.masklengthRange, reqInfo.prefixSetName)}
	}
	return rpolPopulatePrefixEntry(inParams.ygRoot, reqInfo.prefixSetName, reqInfo.ipPrefix, reqInfo.masklengthRange, reqInfo.targetLeaf)
}

// rpolPopulatePrefixEntry fills one prefix entry, keeping the key leaves and the
// config leaves in step so that a request for either answers with the same
// value.
func rpolPopulatePrefixEntry(ygRoot *ygot.GoStruct, prefixSetName, ipPrefix, masklengthRange, targetLeaf string) error {
	pfxObj := getRpolPrefixObj(ygRoot, prefixSetName, ipPrefix, masklengthRange, true)
	if pfxObj == nil {
		return tlerr.InvalidArgsError{Format: "Unable to locate routing-policy prefix object in YANG tree"}
	}

	pfx := ipPrefix
	rng := masklengthRange
	if targetLeaf == "" || targetLeaf == rpolIpPrefixLeaf {
		pfxObj.IpPrefix = &pfx
	}
	if targetLeaf == "" || targetLeaf == rpolMasklengthRangeLeaf {
		pfxObj.MasklengthRange = &rng
	}
	if pfxObj.Config == nil {
		return nil
	}
	if targetLeaf == "" || targetLeaf == rpolIpPrefixLeaf {
		pfxObj.Config.IpPrefix = &pfx
	}
	if targetLeaf == "" || targetLeaf == rpolMasklengthRangeLeaf {
		pfxObj.Config.MasklengthRange = &rng
	}
	return nil
}
