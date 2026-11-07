package main

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Both current producers serialize their maps in sorted key order. Rust's
// pinned serde_json lacks preserve_order; its only extra field is nextSha256.
// This produces metadata bytes, never native admission or stopped-writer proof.
func localReplaceProducerBytes(baseSHA, nextSHA string) ([]byte, error) {
	if !lowerDigest(baseSHA) || nextSHA != "" && !lowerDigest(nextSHA) {
		return nil, errClientConfigRecovery
	}
	value := map[string]any{"schema": 1, "kind": "config-replace", "baseSha256": baseSHA}
	if nextSHA != "" {
		value["nextSha256"] = nextSHA
	}
	return json.Marshal(value)
}

// Exact compact current-producer prefixes only. An EOF is not authority: the
// native caller separately requires strict current, its held controller and all
// independent writer/stage/initialization/import conflicts absent. In particular
// a schema3 writer cannot reach intent production without prepare+original.
func localReplaceEOFPrefix(body []byte, currentSHA string) bool {
	if len(body) > 4096 || !lowerDigest(currentSHA) || !incompleteWriterBirth(body) {
		return false
	}
	for _, next := range []string{"", strings.Repeat("0", 64)} {
		template, e := localReplaceProducerBytes(currentSHA, next)
		if e != nil || len(body) >= len(template) {
			continue
		}
		start := -1
		if next != "" {
			needle := []byte(`"nextSha256":"`)
			start = bytes.Index(template, needle) + len(needle)
		}
		match := true
		for i, b := range body {
			if start >= 0 && i >= start && i < start+64 {
				if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
					match = false
					break
				}
			} else if b != template[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

type localEOFPlan struct {
	target  []byte
	changed bool
}

// Actual native observation is mandatory and remains owned by the caller.
// Returning this data plan cannot fabricate an executable/config capability.
func prepareLocalEOFPlan(intent, current []byte, profile, plugins string, strict bool, nativeCheck func() error) (localEOFPlan, error) {
	var plan localEOFPlan
	if nativeCheck == nil || !strict {
		return plan, errClientConfigRecovery
	}
	if e := nativeCheck(); e != nil {
		return plan, e
	}
	if _, e := ownedManagedSelector(current, profile); e != nil || !localReplaceEOFPrefix(intent, digestBytes(current)) {
		return plan, errClientConfigRecovery
	}
	target, e := recoveryTarget(current, plugins, profile)
	if e != nil {
		return plan, e
	}
	return localEOFPlan{target: target, changed: !bytes.Equal(target, current)}, nil
}

// The old local intent is the final old effect. Refresh is an independently
// journaled transaction under the same native controller, never replay/auth.
func applyLocalEOF(check, dispose, refresh func() error) error {
	if check == nil || dispose == nil || refresh == nil {
		return errClientConfigRecovery
	}
	if e := check(); e != nil {
		return e
	}
	if e := dispose(); e != nil {
		return e
	}
	return refresh()
}
