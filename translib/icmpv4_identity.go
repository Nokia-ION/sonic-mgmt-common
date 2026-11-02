////////////////////////////////////////////////////////////////////////////////
//                                                                            //
//  Copyright 2026 Nokia.                                                    //
//                                                                            //
//  Licensed under the Apache License, Version 2.0 (the "License");           //
//  you may not use this file except in compliance with the License.          //
//                                                                            //
////////////////////////////////////////////////////////////////////////////////

package translib

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

//go:embed testdata/icmpv4-identities-ref.yang
var icmpv4IdentitiesRefYang string

var (
	icmpv4TypeIdentityToNum map[string]uint8
	icmpv4CodeIdentityToNum map[string]uint8
)

func init() {
	icmpv4TypeIdentityToNum, icmpv4CodeIdentityToNum = parseIcmpv4IdentityMaps(icmpv4IdentitiesRefYang)
}

func parseIcmpv4IdentityMaps(yang string) (types, codes map[string]uint8) {
	types = make(map[string]uint8)
	codes = make(map[string]uint8)
	typeRe := regexp.MustCompile(`(?s)identity (\w+) \{.*?\}`)
	typeValRe := regexp.MustCompile(`value (\d+)`)
	codeValRe := regexp.MustCompile(`code (\d+)`)
	for _, block := range typeRe.FindAllString(yang, -1) {
		name := typeRe.FindStringSubmatch(block)[1]
		if m := typeValRe.FindStringSubmatch(block); m != nil {
			n, _ := strconv.ParseUint(m[1], 10, 8)
			types[name] = uint8(n)
		}
		if m := codeValRe.FindStringSubmatch(block); m != nil {
			n, _ := strconv.ParseUint(m[1], 10, 8)
			codes[name] = uint8(n)
		}
	}
	return types, codes
}

// OpenConfig icmpv4-types identity names (from icmpv4-identities-ref.yang, not
// installed in the model bundle) mapped to numeric ICMP_TYPE/ICMP_CODE for
// openconfig-acl-ext icmpv4 leaves, which remain uint8 in yang. gNMI JSON-IETF
// clients may send qualified identity strings; normalize to uint8 before ygot.

func icmpv4IdentityLeafName(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.LastIndex(raw, ":"); idx >= 0 {
		raw = raw[idx+1:]
	}
	return raw
}

func icmpv4IdentityToNumber(raw string, codeLeaf bool) (uint8, bool) {
	name := icmpv4IdentityLeafName(raw)
	if num, err := strconv.ParseUint(name, 10, 8); err == nil {
		return uint8(num), true
	}
	if codeLeaf {
		if n, ok := icmpv4CodeIdentityToNum[name]; ok {
			return n, true
		}
		return 0, false
	}
	if n, ok := icmpv4TypeIdentityToNum[name]; ok {
		return n, true
	}
	return 0, false
}

func icmpv4BareKey(key string) string {
	if idx := strings.LastIndex(key, ":"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}

func normalizeIcmpv4IdentityNode(v interface{}, inIcmpv4 bool) bool {
	changed := false
	switch node := v.(type) {
	case map[string]interface{}:
		nextIn := inIcmpv4
		for k, val := range node {
			if icmpv4BareKey(k) == "icmpv4" {
				if normalizeIcmpv4IdentityNode(val, true) {
					changed = true
				}
				continue
			}
			if nextIn && (k == "type" || k == "code" || icmpv4BareKey(k) == "type" || icmpv4BareKey(k) == "code") {
				bare := icmpv4BareKey(k)
				if s, ok := val.(string); ok {
					if num, ok := icmpv4IdentityToNumber(s, bare == "code"); ok {
						node[k] = num
						changed = true
					}
				}
				continue
			}
			if normalizeIcmpv4IdentityNode(val, nextIn) {
				changed = true
			}
		}
	case []interface{}:
		for _, item := range node {
			if normalizeIcmpv4IdentityNode(item, inIcmpv4) {
				changed = true
			}
		}
	}
	return changed
}

func normalizeIcmpv4IdentityPayload(uri string, payload *[]byte) error {
	if payload == nil || len(*payload) == 0 {
		return nil
	}
	payloadStr := string(*payload)
	if !strings.Contains(uri, "icmpv4") && !strings.Contains(payloadStr, "icmpv4") {
		return nil
	}

	var v interface{}
	if err := json.Unmarshal(*payload, &v); err != nil {
		return err
	}

	changed := false
	if s, ok := v.(string); ok {
		codeLeaf := strings.HasSuffix(uri, "/code")
		if num, ok := icmpv4IdentityToNumber(s, codeLeaf); ok {
			out, err := json.Marshal(num)
			if err != nil {
				return err
			}
			*payload = out
			return nil
		}
		return nil
	}

	if normalizeIcmpv4IdentityNode(v, strings.Contains(uri, "icmpv4")) {
		changed = true
	}
	if !changed {
		return nil
	}
	out, err := json.Marshal(v)
	if err != nil {
		return err
	}
	*payload = out
	return nil
}
