package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic DATA only: these strings never open files or grant native authority.
func ordinaryRawTestBody(plugins, extra string) []byte {
	dirs, e := json.Marshal([]string{plugins})
	if e != nil {
		panic(e)
	}
	return []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":` + string(dirs) + extra + `}`)
}
func TestOrdinaryRawContractBoundaries(t *testing.T) {
	const plugins = "ordinary-raw-test-plugins"
	body := func(extra string) []byte { return ordinaryRawTestBody(plugins, extra) }
	arrays := func(n int) []byte { return body(`,"opaque":` + strings.Repeat("[", n) + `0` + strings.Repeat("]", n)) }
	nodes := func(n int) []byte { return body(`,"opaque":[` + strings.Repeat("0,", n-1) + `0]`) }
	sized := func(extra int) []byte {
		overhead := len(body(`,"opaque":""`))
		return body(`,"opaque":"` + strings.Repeat("x", clientConfigBudget-overhead+extra) + `"`)
	}
	cases := []struct {
		name   string
		make   func() []byte
		accept bool
	}{
		{"depth64", func() []byte { return arrays(63) }, true}, {"depth65", func() []byte { return arrays(64) }, false},
		// root/store/plugin-array/plugin-string/opaque-array = five counted values.
		{"count65537", func() []byte { return nodes(65532) }, true}, {"count65538", func() []byte { return nodes(65533) }, false},
		{"oneMiB", func() []byte { return sized(0) }, true}, {"overOneMiB", func() []byte { return sized(1) }, false},
		{"validPair", func() []byte { return body(`,"opaque":"\uD800\uDC00"`) }, true},
		{"literalReplacement", func() []byte { return body(",\"opaque\":\"\uFFFD\"") }, true},
		{"escapedLiteral", func() []byte { return body(`,"opaque":"\\uD800"`) }, true},
		{"unpairedValue", func() []byte { return body(`,"opaque":"\uD800"`) }, false},
		{"unpairedKey", func() []byte { return body(`,"\uDC00":0`) }, false},
		{"invalidUTF8", func() []byte {
			return bytes.Replace(body(`,"opaque":"invalid-byte"`), []byte("invalid-byte"), []byte{0xff}, 1)
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := c.make()
			original := bytes.Clone(data)
			if c.name == "oneMiB" && len(data) != clientConfigBudget {
				t.Fatal("fixture byte count")
			}
			_, e := checkedDockerConfigJSON(data)
			if (e == nil) != c.accept || (privateClientRouting(data, plugins) == nil) != c.accept {
				t.Fatal("ordinary raw acceptance differs from supported contract")
			}
			if !bytes.Equal(data, original) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestOrdinaryRawRecoveryPreservation(t *testing.T) {
	profile := "ordinary-raw-test-profile"
	old := filepath.Join(profile, ".orbwin-private", "runtime", "docker-"+strings.Repeat("a", 64), "docker-cli-plugins")
	next := filepath.Join(profile, ".orbwin-private", "runtime", "docker-"+strings.Repeat("b", 64), "docker-cli-plugins")
	extras := `,"auths":{"kept.example":{ "auth":"synthetic-original", "opaque":1e9999 },"target.example":{"auth":"old"}},"opaque":{ "n":1e9999,"n":123456789012345678901234567890 },"opaque":[ 1 , 2 ],"currentContext":"keep-context","plugins":{"x":{"hooks":"keep"}},"HttpHeaders":false`
	base := ordinaryRawTestBody(old, extras)
	original := bytes.Clone(base)
	incoming := ordinaryRawTestBody(old, `,"auths":{"kept.example":{"auth":"untrusted-change"},"target.example":{"auth":"synthetic-new"}},"opaque":false`)
	merged, e := mergePrivateWriter(base, incoming, old, writerRegistryAuth, []string{"target.example"})
	if e != nil {
		t.Fatal(e)
	}
	preserved := []string{`{ "auth":"synthetic-original", "opaque":1e9999 }`, `{ "n":1e9999,"n":123456789012345678901234567890 }`, `"opaque":[ 1 , 2 ]`, `"currentContext":"keep-context"`, `"plugins":{"x":{"hooks":"keep"}}`, `"HttpHeaders":false`}
	for _, raw := range preserved {
		if !bytes.Contains(merged, []byte(raw)) {
			t.Fatal("unrelated ordinary data lost")
		}
	}
	if !bytes.Contains(merged, []byte(`"auth":"synthetic-new"`)) {
		t.Fatal("command-bound auth update missing")
	}
	same, e := mergePrivateWriter(base, base, old, writerRootUnchanged, nil)
	if e != nil || !bytes.Equal(same, base) {
		t.Fatal("exact unchanged baseline lost")
	}
	refreshed, changed, e := refreshPrivateManagedField(base, next, profile)
	if e != nil || !changed || privateClientRouting(refreshed, next) != nil {
		t.Fatal("managed refresh failed")
	}
	for _, raw := range preserved {
		if !bytes.Contains(refreshed, []byte(raw)) {
			t.Fatal("refresh changed unrelated ordinary data")
		}
	}
	if !bytes.Equal(base, original) {
		t.Fatal("source changed")
	}
	bad := []struct {
		name string
		body []byte
	}{
		{"unicode", ordinaryRawTestBody(old, `,"opaque":"\uD800"`)},
		{"depth", ordinaryRawTestBody(old, `,"opaque":`+strings.Repeat("[", 64)+`0`+strings.Repeat("]", 64))},
		{"size", append(bytes.Clone(base), bytes.Repeat([]byte(" "), clientConfigBudget)...)},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			original := bytes.Clone(c.body)
			if out, e := mergePrivateWriter(c.body, c.body, old, writerRegistryAuth, []string{"target.example"}); e == nil || out != nil {
				t.Fatal("unsupported baseline emitted merge output")
			}
			if out, _, e := refreshPrivateManagedField(c.body, next, profile); e == nil || out != nil {
				t.Fatal("unsupported baseline emitted refresh output")
			}
			if !bytes.Equal(c.body, original) {
				t.Fatal("unsupported source changed")
			}
		})
	}
}
