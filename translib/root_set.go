// Copyright 2026 Nokia.

package translib

import (
	"bytes"
	"encoding/json"
	"strings"

	pathutil "github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/goyang/pkg/yang"
	"github.com/openconfig/ygot/ygot"
)

// derivedSetOp is one SET target derived from an origin-only payload key.
type derivedSetOp struct {
	Path    string
	Payload []byte
}

type jsonObjectKey struct {
	Key   string
	Value json.RawMessage
}

func isRootSetPath(path string) bool {
	return len(strings.Trim(path, "/")) == 0
}

// expandRootSetRequest rewrites an origin-only / root SET (URI "/") using the
// IETF JSON payload. Each top-level key that maps to a valid YANG path and app
// module becomes one derived SET. Non-root paths are returned unchanged.
// Invalid JSON, an empty object, or any key that cannot select an app returns
// an error and the caller must not crash.
func expandRootSetRequest(path string, payload []byte) ([]derivedSetOp, error) {
	if !isRootSetPath(path) {
		return []derivedSetOp{{Path: path, Payload: payload}}, nil
	}
	ops, err := deriveSetOpsFromPayload(payload)
	if err != nil {
		log.Warningf("root-path SET rejected: %v", err)
		return nil, err
	}
	for _, op := range ops {
		log.Infof("root-path SET derived path %s from payload", op.Path)
	}
	return ops, nil
}

// resolveRootSetRequest rewrites a root SET that maps to exactly one derived
// path. Multi-key payloads must use expandRootSetRequest and Bulk.
func resolveRootSetRequest(path string, payload []byte) (string, []byte, error) {
	ops, err := expandRootSetRequest(path, payload)
	if err != nil {
		return path, payload, err
	}
	if len(ops) != 1 {
		err := tlerr.InvalidArgs("cannot derive a unique app from payload: multiple top-level keys")
		log.Warningf("root-path SET rejected: %v", err)
		return path, payload, err
	}
	return ops[0].Path, ops[0].Payload, nil
}

func deriveSetPathFromPayload(payload []byte) (string, []byte, error) {
	ops, err := deriveSetOpsFromPayload(payload)
	if err != nil {
		return "", nil, err
	}
	if len(ops) != 1 {
		return "", nil, tlerr.InvalidArgs("cannot derive a unique app from payload: multiple top-level keys")
	}
	return ops[0].Path, ops[0].Payload, nil
}

func deriveSetOpsFromPayload(payload []byte) ([]derivedSetOp, error) {
	if len(payload) == 0 {
		return nil, tlerr.InvalidArgs("root-path SET requires a JSON payload")
	}

	keys, err := parseTopLevelJSONObject(payload)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, tlerr.InvalidArgs("root-path SET payload is empty")
	}

	ops := make([]derivedSetOp, 0, len(keys))
	for _, k := range keys {
		op, err := deriveSetOpFromKey(k.Key, k.Value)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func parseTopLevelJSONObject(payload []byte) ([]jsonObjectKey, error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	tok, err := dec.Token()
	if err != nil {
		return nil, tlerr.InvalidArgs("root-path SET payload must be a JSON object")
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, tlerr.InvalidArgs("root-path SET payload must be a JSON object")
	}

	var keys []jsonObjectKey
	seen := make(map[string]bool)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, tlerr.InvalidArgs("root-path SET payload must be a JSON object")
		}
		key, ok := tok.(string)
		if !ok {
			return nil, tlerr.InvalidArgs("root-path SET payload must be a JSON object")
		}
		if seen[key] {
			return nil, tlerr.InvalidArgs("duplicate top-level key %q in root-path SET payload", key)
		}
		seen[key] = true
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, tlerr.InvalidArgs("root-path SET payload must be a JSON object")
		}
		keys = append(keys, jsonObjectKey{Key: key, Value: raw})
	}
	if _, err := dec.Token(); err != nil {
		return nil, tlerr.InvalidArgs("root-path SET payload must be a JSON object")
	}
	return keys, nil
}

func deriveSetOpFromKey(key string, inner json.RawMessage) (derivedSetOp, error) {
	var empty derivedSetOp
	if len(strings.TrimSpace(key)) == 0 {
		return empty, tlerr.InvalidArgs("cannot derive app from empty payload key")
	}

	uri := key
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}

	gpath, err := ygot.StringToPath(uri, ygot.StructuredPath)
	if err != nil {
		return empty, tlerr.InvalidArgs("invalid path derived from payload key %q: %v", key, err)
	}
	if gpath == nil || len(gpath.GetElem()) == 0 {
		return empty, tlerr.InvalidArgs("cannot derive app from payload key %q", key)
	}

	pv := pathutil.NewPathValidator(&pathutil.AppendModulePrefix{})
	if err := pv.Validate(gpath); err != nil {
		return empty, tlerr.InvalidArgs("cannot derive app from payload key %q: %v", key, err)
	}

	derived, err := ygot.PathToString(gpath)
	if err != nil {
		return empty, tlerr.InvalidArgs("cannot convert derived path for payload key %q: %v", key, err)
	}
	if !strings.HasPrefix(derived, "/") {
		derived = "/" + derived
	}

	app, err := getAppModuleInfo(derived)
	if err != nil {
		return empty, err
	}
	if app == nil {
		return empty, tlerr.InvalidArgs("no app module for path %s derived from payload", derived)
	}

	return derivedSetOp{Path: derived, Payload: []byte(inner)}, nil
}

// rewriteOrBulkRootSet expands a SET path from the payload. Origin-only
// payloads are split per top-level module. UPDATE/REPLACE then expand YANG
// lists in the payload into keyed instance paths, but only when the SET URI
// has no list keys yet. More than one derived op, or any list expansion, is
// applied as a Bulk transaction (done is true) so a missing instance can
// fall back to REPLACE per entry. Otherwise req is rewritten in place and
// done is false.
func rewriteOrBulkRootSet(req SetRequest, operation int) (reqOut SetRequest, done bool, resp SetResponse, err error) {
	ops, needBulk, err := expandSetOps(req.Path, req.Payload, operation)
	if err != nil {
		return req, true, SetResponse{ErrSrc: AppErr}, err
	}
	if len(ops) == 0 {
		return req, true, SetResponse{ErrSrc: AppErr}, tlerr.InvalidArgs("root-path SET payload is empty")
	}
	if needBulk {
		resp, err = applyRootSetAsBulk(req, ops, operation)
		return req, true, resp, err
	}
	req.Path = ops[0].Path
	req.Payload = ops[0].Payload
	return req, false, SetResponse{}, nil
}

// applyRootSetAsBulk runs each derived root-path SET in one Bulk transaction.
// Called from Create/Update/Replace before writeMutex is taken, so Bulk can
// acquire the lock itself.
func applyRootSetAsBulk(req SetRequest, ops []derivedSetOp, operation int) (SetResponse, error) {
	br := BulkRequest{
		User:          req.User,
		AuthEnabled:   req.AuthEnabled,
		ClientVersion: req.ClientVersion,
	}
	for _, op := range ops {
		entry := req
		entry.Path = op.Path
		entry.Payload = op.Payload
		br.Request = append(br.Request, BulkRequestEntry{
			Entry:     entry,
			Operation: operation,
		})
	}

	bresp, err := Bulk(br)
	if err == nil {
		return SetResponse{}, nil
	}
	resp := SetResponse{Err: err, ErrSrc: AppErr}
	if n := len(bresp.Response); n > 0 {
		last := bresp.Response[n-1].Entry
		resp.ErrSrc = last.ErrSrc
		if last.Err != nil {
			resp.Err = last.Err
		}
	}
	return resp, err
}

// expandBulkRootSet expands origin-only non-DELETE entries into one Bulk
// entry per payload key (and, for UPDATE/REPLACE, per YANG list instance
// when the URI has no list keys), preserving request order.
func expandBulkRootSet(req BulkRequest) ([]BulkRequestEntry, error) {
	var out []BulkRequestEntry
	for i := range req.Request {
		entry := req.Request[i]
		if entry.Operation == DELETE {
			out = append(out, entry)
			continue
		}
		ops, _, err := expandSetOps(entry.Entry.Path, entry.Entry.Payload, entry.Operation)
		if err != nil {
			return nil, err
		}
		for _, op := range ops {
			e := entry
			e.Entry.Path = op.Path
			e.Entry.Payload = op.Payload
			out = append(out, e)
		}
	}
	return out, nil
}

// expandSetOps derives SET targets from an origin payload and, for
// UPDATE/REPLACE, from YANG lists in that payload when the URI has no list
// keys. needBulk is true when more than one op is produced or when a list
// was expanded to keyed instances.
func expandSetOps(path string, payload []byte, operation int) ([]derivedSetOp, bool, error) {
	ops, err := expandRootSetRequest(path, payload)
	if err != nil {
		return nil, false, err
	}
	needBulk := len(ops) > 1
	if operation != UPDATE && operation != REPLACE {
		return ops, needBulk, nil
	}

	var out []derivedSetOp
	for _, op := range ops {
		expanded, changed, err := expandListInstances(op.Path, op.Payload)
		if err != nil {
			return nil, false, err
		}
		if changed {
			needBulk = true
			for _, e := range expanded {
				log.Infof("list-instance SET derived path %s from payload under %s", e.Path, op.Path)
			}
		}
		out = append(out, expanded...)
	}
	if len(out) == 0 {
		return nil, false, tlerr.InvalidArgs("SET payload is empty")
	}
	return out, needBulk, nil
}

func expandListInstances(path string, payload []byte) ([]derivedSetOp, bool, error) {
	passthrough := []derivedSetOp{{Path: path, Payload: payload}}
	if len(bytes.TrimSpace(payload)) == 0 {
		return passthrough, false, nil
	}
	// Once any parent list instance is named in the URI, nested lists stay
	// on that path as merge UPDATE/REPLACE. Expanding them to keyed Bulk
	// ops can turn UPDATE into REPLACE (for example a new IP on Ethernet).
	if uriHasListKeys(path) {
		return passthrough, false, nil
	}

	entry, err := yangEntryForURI(path)
	if err != nil || entry == nil {
		return passthrough, false, nil
	}

	if entry.IsList() {
		return expandWholeListPayload(path, entry, payload)
	}
	if entry.IsContainer() {
		return expandContainerListPayload(path, entry, payload)
	}
	return passthrough, false, nil
}

func expandWholeListPayload(listPath string, listEntry *yang.Entry, payload []byte) ([]derivedSetOp, bool, error) {
	insts, err := parseListInstances(payload)
	if err != nil {
		return nil, false, err
	}
	if len(insts) == 0 {
		return []derivedSetOp{{Path: listPath, Payload: payload}}, false, nil
	}
	ops, err := keyedOpsForListInstances(listPath, listEntry, insts)
	if err != nil {
		return nil, false, err
	}
	return ops, true, nil
}

func expandContainerListPayload(path string, entry *yang.Entry, payload []byte) ([]derivedSetOp, bool, error) {
	keys, err := parseTopLevelJSONObject(payload)
	if err != nil {
		return []derivedSetOp{{Path: path, Payload: payload}}, false, nil
	}

	var leftover []jsonObjectKey
	var ops []derivedSetOp
	changed := false
	for _, k := range keys {
		child := yangChildByJSONName(entry, k.Key)
		if child != nil && child.IsList() {
			insts, err := parseListInstances(k.Value)
			if err != nil {
				return nil, false, err
			}
			if len(insts) == 0 {
				leftover = append(leftover, k)
				continue
			}
			listPath, err := joinYangURI(path, k.Key)
			if err != nil {
				return nil, false, err
			}
			instOps, err := keyedOpsForListInstances(listPath, child, insts)
			if err != nil {
				return nil, false, err
			}
			ops = append(ops, instOps...)
			changed = true
			continue
		}
		if child != nil && child.IsContainer() {
			childPath, err := joinYangURI(path, k.Key)
			if err != nil {
				return nil, false, err
			}
			subOps, subChanged, err := expandListInstances(childPath, k.Value)
			if err != nil {
				return nil, false, err
			}
			if subChanged {
				ops = append(ops, subOps...)
				changed = true
				continue
			}
		}
		leftover = append(leftover, k)
	}

	if !changed {
		return []derivedSetOp{{Path: path, Payload: payload}}, false, nil
	}
	if len(leftover) > 0 {
		left, err := marshalJSONObjectKeys(leftover)
		if err != nil {
			return nil, false, err
		}
		ops = append([]derivedSetOp{{Path: path, Payload: left}}, ops...)
	}
	return ops, true, nil
}

func keyedOpsForListInstances(listPath string, listEntry *yang.Entry, insts []json.RawMessage) ([]derivedSetOp, error) {
	ops := make([]derivedSetOp, 0, len(insts))
	for _, inst := range insts {
		keyed, err := keyedListInstanceURI(listPath, listEntry, inst)
		if err != nil {
			return nil, err
		}
		ops = append(ops, derivedSetOp{Path: keyed, Payload: []byte(inst)})
	}
	return ops, nil
}

func keyedListInstanceURI(listPath string, listEntry *yang.Entry, instance json.RawMessage) (string, error) {
	keyVals, err := listInstanceKeyValues(listEntry, instance)
	if err != nil {
		return "", err
	}
	gpath, err := ygot.StringToPath(listPath, ygot.StructuredPath)
	if err != nil || gpath == nil || len(gpath.GetElem()) == 0 {
		return "", tlerr.InvalidArgs("invalid list path %s: %v", listPath, err)
	}
	last := gpath.Elem[len(gpath.Elem)-1]
	last.Key = keyVals

	pv := pathutil.NewPathValidator(&pathutil.AppendModulePrefix{})
	if err := pv.Validate(gpath); err != nil {
		return "", tlerr.InvalidArgs("cannot form list instance path under %s: %v", listPath, err)
	}
	derived, err := ygot.PathToString(gpath)
	if err != nil {
		return "", tlerr.InvalidArgs("cannot convert list instance path under %s: %v", listPath, err)
	}
	if !strings.HasPrefix(derived, "/") {
		derived = "/" + derived
	}
	return derived, nil
}

func listInstanceKeyValues(listEntry *yang.Entry, instance json.RawMessage) (map[string]string, error) {
	if listEntry == nil || strings.TrimSpace(listEntry.Key) == "" {
		return nil, tlerr.InvalidArgs("YANG list %s has no keys", yangNodeName(listEntry))
	}
	fields, err := parseTopLevelJSONObject(instance)
	if err != nil {
		return nil, tlerr.InvalidArgs("YANG list %s instance must be a JSON object", yangNodeName(listEntry))
	}
	byName := make(map[string]json.RawMessage, len(fields))
	for _, f := range fields {
		byName[localJSONName(f.Key)] = f.Value
	}
	vals := make(map[string]string)
	for _, keyName := range strings.Fields(listEntry.Key) {
		raw, ok := byName[keyName]
		if !ok {
			return nil, tlerr.InvalidArgs("list %s instance missing key leaf %q", yangNodeName(listEntry), keyName)
		}
		s, err := jsonScalarString(raw)
		if err != nil {
			return nil, tlerr.InvalidArgs("list %s key leaf %q is not a scalar: %v", yangNodeName(listEntry), keyName, err)
		}
		vals[keyName] = s
	}
	return vals, nil
}

func parseListInstances(payload []byte) ([]json.RawMessage, error) {
	trim := bytes.TrimSpace(payload)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil, nil
	}
	if trim[0] == '{' {
		return []json.RawMessage{json.RawMessage(append([]byte(nil), trim...))}, nil
	}
	if trim[0] != '[' {
		return nil, tlerr.InvalidArgs("YANG list payload must be a JSON array or object")
	}

	dec := json.NewDecoder(bytes.NewReader(trim))
	tok, err := dec.Token()
	if err != nil {
		return nil, tlerr.InvalidArgs("YANG list payload must be a JSON array")
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return nil, tlerr.InvalidArgs("YANG list payload must be a JSON array")
	}
	var insts []json.RawMessage
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, tlerr.InvalidArgs("YANG list payload must be a JSON array")
		}
		insts = append(insts, raw)
	}
	if _, err := dec.Token(); err != nil {
		return nil, tlerr.InvalidArgs("YANG list payload must be a JSON array")
	}
	return insts, nil
}

func yangEntryForURI(uri string) (*yang.Entry, error) {
	if ygSchema == nil || ygSchema.RootSchema() == nil {
		return nil, tlerr.InternalError{Format: "YANG schema not initialized"}
	}
	gpath, err := ygot.StringToPath(uri, ygot.StructuredPath)
	if err != nil {
		return nil, err
	}
	pv := pathutil.NewPathValidator(&pathutil.AppendModulePrefix{})
	if err := pv.Validate(gpath); err != nil {
		return nil, err
	}
	entry := ygSchema.RootSchema()
	for _, elem := range gpath.GetElem() {
		child := yangChildByJSONName(entry, elem.GetName())
		if child == nil {
			return nil, tlerr.InvalidArgs("no YANG node %q under %s", elem.GetName(), uri)
		}
		entry = child
	}
	return entry, nil
}

func yangChildByJSONName(entry *yang.Entry, jsonName string) *yang.Entry {
	if entry == nil || entry.Dir == nil {
		return nil
	}
	name := localJSONName(jsonName)
	if ch := entry.Dir[name]; ch != nil {
		return ch
	}
	for _, ch := range entry.Dir {
		if ch != nil && ch.Name == name {
			return ch
		}
	}
	return nil
}

func joinYangURI(parent, childJSONName string) (string, error) {
	parent = strings.TrimRight(parent, "/")
	child := strings.Trim(childJSONName, "/")
	uri := parent + "/" + child
	gpath, err := ygot.StringToPath(uri, ygot.StructuredPath)
	if err != nil {
		return "", tlerr.InvalidArgs("invalid path %s: %v", uri, err)
	}
	pv := pathutil.NewPathValidator(&pathutil.AppendModulePrefix{})
	if err := pv.Validate(gpath); err != nil {
		return "", tlerr.InvalidArgs("invalid path %s: %v", uri, err)
	}
	derived, err := ygot.PathToString(gpath)
	if err != nil {
		return "", tlerr.InvalidArgs("cannot convert path %s: %v", uri, err)
	}
	if !strings.HasPrefix(derived, "/") {
		derived = "/" + derived
	}
	return derived, nil
}

func marshalJSONObjectKeys(keys []jsonObjectKey) ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		keyJSON, err := json.Marshal(k.Key)
		if err != nil {
			return nil, err
		}
		b.Write(keyJSON)
		b.WriteByte(':')
		b.Write(k.Value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func jsonScalarString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return n.String(), nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		if b {
			return "true", nil
		}
		return "false", nil
	}
	return "", tlerr.InvalidArgs("JSON value is not a scalar")
}

func localJSONName(name string) string {
	if i := strings.LastIndex(name, ":"); i >= 0 {
		return name[i+1:]
	}
	return name
}

func uriHasListKeys(path string) bool {
	return strings.Contains(path, "[")
}

func yangNodeName(entry *yang.Entry) string {
	if entry == nil {
		return ""
	}
	return entry.Name
}
