// Copyright 2026 Nokia.

package translib

import (
	"bytes"
	"encoding/json"
	"strings"

	pathutil "github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
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

// rewriteOrBulkRootSet expands a SET path from the payload. If more than one
// top-level key is present, the ops are applied as a Bulk transaction and
// done is true. Otherwise req is rewritten in place and done is false.
func rewriteOrBulkRootSet(req SetRequest, operation int) (reqOut SetRequest, done bool, resp SetResponse, err error) {
	ops, err := expandRootSetRequest(req.Path, req.Payload)
	if err != nil {
		return req, true, SetResponse{ErrSrc: AppErr}, err
	}
	if len(ops) == 0 {
		return req, true, SetResponse{ErrSrc: AppErr}, tlerr.InvalidArgs("root-path SET payload is empty")
	}
	if len(ops) > 1 {
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
// entry per payload key, preserving request order.
func expandBulkRootSet(req BulkRequest) ([]BulkRequestEntry, error) {
	var out []BulkRequestEntry
	for i := range req.Request {
		entry := req.Request[i]
		if entry.Operation == DELETE {
			out = append(out, entry)
			continue
		}
		ops, err := expandRootSetRequest(entry.Entry.Path, entry.Entry.Payload)
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
