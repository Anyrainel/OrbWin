package main

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// Initialization is a separate protocol, not a partially constructed writer
// guard. These recognizers accept only producer defaults or a provable compact
// producer prefix. The native provider must separately hold the exact marker,
// owner, root identities and exclusive controller before offering recovery.
const freshClientMarker = `{"schema":1,"kind":"fresh-client"}`
const freshClientRecordBudget = 8192
const freshPrivateDirectory = ".orbwin-private"

func freshMarkerPrefix(body []byte) bool {
	return len(body) <= len(freshClientMarker) && bytes.HasPrefix([]byte(freshClientMarker), body)
}

// serde_json leaves HTML and U+2028/U+2029 literal. Go's ordinary encoder is
// another explicitly admitted producer encoding; arbitrary escape spellings
// are accepted only for a complete, closed default object, never an EOF cut.
func freshQuote(text string, rust bool) string {
	if !rust {
		encoded, _ := json.Marshal(text)
		return string(encoded)
	}
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range text {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				const digits = "0123456789abcdef"
				out.WriteString(`\u00`)
				out.WriteByte(digits[byte(r)>>4])
				out.WriteByte(digits[byte(r)&15])
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

type freshFieldPattern struct {
	value            string
	hexStart, hexEnd int // -1 for a literal; otherwise exactly 64 lower hex bytes
}

func freshLiteral(value string) freshFieldPattern {
	return freshFieldPattern{value: value, hexStart: -1, hexEnd: -1}
}

func (p freshFieldPattern) match(body []byte) (int, bool) {
	n := min(len(body), len(p.value))
	nonzero := false
	for i := 0; i < n; i++ {
		if i >= p.hexStart && i < p.hexEnd {
			b := body[i]
			if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
				return 0, false
			}
			nonzero = nonzero || b != '0'
		} else if body[i] != p.value[i] {
			return 0, false
		}
	}
	// A complete all-zero generation cannot be extended into an admitted pin.
	if p.hexEnd >= 0 && n >= p.hexEnd && !nonzero {
		return 0, false
	}
	return n, true
}

func freshCompactPrefix(body []byte, fields []freshFieldPattern) bool {
	if len(fields) == 0 || len(fields) > 7 || len(body) > freshClientRecordBudget {
		return false
	}
	if len(body) == 0 {
		return true
	}
	if body[0] != '{' {
		return false
	}
	var visit func([]byte, uint8) bool
	visit = func(left []byte, used uint8) bool {
		if len(left) == 0 {
			return true
		}
		if int(used) == 1<<len(fields)-1 {
			return bytes.Equal(left, []byte{'}'})
		}
		for i, field := range fields {
			bit := uint8(1 << i)
			if used&bit != 0 {
				continue
			}
			n, ok := field.match(left)
			if !ok {
				continue
			}
			if n == len(left) {
				return true
			}
			rest := left[n:]
			if int(used|bit) == 1<<len(fields)-1 {
				if bytes.Equal(rest, []byte{'}'}) {
					return true
				}
			} else if rest[0] == ',' && visit(rest[1:], used|bit) {
				return true
			}
		}
		return false
	}
	return visit(body[1:], 0)
}

func freshInitPrefix(body []byte, expected map[string]any) bool {
	if len(expected) != 7 || len(body) > freshClientRecordBudget {
		return false
	}
	if actual, err := checkedClientJSON(body); err == nil {
		a, _ := json.Marshal(actual)
		b, _ := json.Marshal(expected)
		return bytes.Equal(a, b)
	}
	keys := make([]string, 0, 7)
	for key := range expected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]freshFieldPattern, 0, 7)
	for _, key := range keys {
		value, _ := json.Marshal(expected[key])
		fields = append(fields, freshLiteral(freshQuote(key, false)+":"+string(value)))
	}
	return freshCompactPrefix(body, fields)
}

func freshSelector(path, profile string) bool {
	prefix := profile + `\` + freshPrivateDirectory + `\runtime\docker-`
	suffix := `\docker-cli-plugins`
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return false
	}
	pin := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return lowerDigest(pin) && pin != zeroWriterDigest
}

func freshDefaultsPrefix(body []byte, profile string) bool {
	if len(body) > freshClientRecordBudget || len(profile) > 4096 || strings.ContainsAny(profile, "\x00?*") {
		return false
	}
	if actual, err := checkedClientJSON(body); err == nil {
		object, ok := actual.(map[string]any)
		if !ok || len(object) != 3 || object["credsStore"] != "orbwin" {
			return false
		}
		auths, ok := object["auths"].(map[string]any)
		if !ok || len(auths) != 0 {
			return false
		}
		plugins, ok := object["cliPluginsExtraDirs"].([]any)
		if !ok || len(plugins) != 1 {
			return false
		}
		path, ok := plugins[0].(string)
		return ok && freshSelector(path, profile)
	}
	for _, rust := range []bool{true, false} {
		prefix := `"cliPluginsExtraDirs":[` + strings.TrimSuffix(freshQuote(profile+`\`+freshPrivateDirectory+`\runtime\docker-`, rust), `"`)
		suffix := strings.TrimPrefix(freshQuote(`\docker-cli-plugins`, rust), `"`) + `]`
		plugin := freshFieldPattern{value: prefix + strings.Repeat("?", 64) + suffix, hexStart: len(prefix), hexEnd: len(prefix) + 64}
		if freshCompactPrefix(body, []freshFieldPattern{freshLiteral(`"auths":{}`), freshLiteral(`"credsStore":"orbwin"`), plugin}) {
			return true
		}
	}
	return false
}
