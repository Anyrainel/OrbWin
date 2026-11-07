package main

import "bytes"

// Pure selection only. This does not admit an owner or interpret EOF as authority.
func recoveryQualificationPrefixCount(f recoveryQualificationFault, name string, body []byte) (int, bool) {
	if name == "recovery.next" && f == recoveryCutNextPrefix {
		return len(body) / 2, len(body) > 1
	}
	if name != "writer.intent" {
		return 0, false
	}
	if f == recoveryWriterIntentPrefix {
		_, e := parseWriterIntent(body)
		return len(body) / 2, e == nil
	}
	if f < recoveryLocalEmpty || f > recoveryLocalLate {
		return 0, false
	}
	if _, _, e := parseLocalReplaceIntent(body); e != nil {
		return 0, false
	}
	needle := []byte(`"baseSha256":"`)
	at := bytes.Index(body, needle)
	if at < 0 {
		return 0, false
	}
	start := at + len(needle)
	switch f {
	case recoveryLocalEmpty:
		return 0, true
	case recoveryLocalEarly:
		return start + 8, true
	case recoveryLocalBase:
		return start + 65, true
	case recoveryLocalLate:
		return len(body) - 1, true
	}
	return 0, false
}
