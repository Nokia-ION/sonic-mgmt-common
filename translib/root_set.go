// Copyright 2026 Nokia.

package translib

import (
	"encoding/json"
	"strings"

	pathutil "github.com/Azure/sonic-mgmt-common/translib/path"
	"github.com/Azure/sonic-mgmt-common/translib/tlerr"
	log "github.com/golang/glog"
	"github.com/openconfig/ygot/ygot"
)

func isRootSetPath(path string) bool {
	return len(strings.Trim(path, "/")) == 0
}

// resolveRootSetRequest rewrites an origin-only / root SET (URI "/") using the
// IETF JSON payload. Processing continues only when exactly one top-level key
// maps to a valid YANG path and an app module. Otherwise an error is returned
// and the caller must not crash.
func resolveRootSetRequest(path string, payload []byte) (string, []byte, error) {
	if !isRootSetPath(path) {
		return path, payload, nil
	}
	derived, unwrapped, err := deriveSetPathFromPayload(payload)
	if err != nil {
		log.Warningf("root-path SET rejected: %v", err)
		return path, payload, err
	}
	log.Infof("root-path SET derived path %s from payload", derived)
	return derived, unwrapped, nil
}

func deriveSetPathFromPayload(payload []byte) (string, []byte, error) {
	if len(payload) == 0 {
		return "", nil, tlerr.InvalidArgs("root-path SET requires a JSON payload")
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(payload, &obj); err != nil {
		return "", nil, tlerr.InvalidArgs("root-path SET payload must be a JSON object")
	}
	if len(obj) == 0 {
		return "", nil, tlerr.InvalidArgs("root-path SET payload is empty")
	}
	if len(obj) != 1 {
		return "", nil, tlerr.InvalidArgs("cannot derive a unique app from payload: multiple top-level keys")
	}

	var key string
	var inner json.RawMessage
	for k, v := range obj {
		key = k
		inner = v
	}
	if len(strings.TrimSpace(key)) == 0 {
		return "", nil, tlerr.InvalidArgs("cannot derive app from empty payload key")
	}

	uri := key
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}

	gpath, err := ygot.StringToPath(uri, ygot.StructuredPath)
	if err != nil {
		return "", nil, tlerr.InvalidArgs("invalid path derived from payload key %q: %v", key, err)
	}
	if gpath == nil || len(gpath.GetElem()) == 0 {
		return "", nil, tlerr.InvalidArgs("cannot derive app from payload key %q", key)
	}

	pv := pathutil.NewPathValidator(&pathutil.AppendModulePrefix{})
	if err := pv.Validate(gpath); err != nil {
		return "", nil, tlerr.InvalidArgs("cannot derive app from payload key %q: %v", key, err)
	}

	derived, err := ygot.PathToString(gpath)
	if err != nil {
		return "", nil, tlerr.InvalidArgs("cannot convert derived path for payload key %q: %v", key, err)
	}
	if !strings.HasPrefix(derived, "/") {
		derived = "/" + derived
	}

	app, err := getAppModuleInfo(derived)
	if err != nil {
		return "", nil, err
	}
	if app == nil {
		return "", nil, tlerr.InvalidArgs("no app module for path %s derived from payload", derived)
	}

	return derived, []byte(inner), nil
}
