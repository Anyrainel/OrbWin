package lanhelper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLANAssetBundleNodeInterop(t *testing.T) {
	data, err := os.ReadFile("testdata/node-bundle/assets.json")
	if err != nil {
		t.Fatal(err)
	}
	signature, err := os.ReadFile("testdata/node-bundle/assets.sig")
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile("testdata/node-bundle/public-key.hex")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyAssetBundle(data, signature, string(key)); err != nil {
		t.Fatal("Node-signed public fixture did not verify in Go", err)
	}
}

func TestLANAssetBundleRequiresIndependentSignatureAndExactCanonicalClosedRoles(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	bundle := AssetBundle{Version: 1, Assets: []BundleAsset{{"manager", "orbwin-session.exe", strings.Repeat("a", 64), 55 << 20}, {"helper", "lan/orbwin-lan-helper.exe", strings.Repeat("b", 64), 12 << 20}, {"worker", "lan/orbwin-lan-worker.exe", strings.Repeat("c", 64), 12 << 20}}}
	good, _ := json.Marshal(bundle)
	sign := func(data []byte) []byte {
		return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, append([]byte(bundleSigningDomain), data...))))
	}
	key := hex.EncodeToString(public)
	if _, err = VerifyAssetBundle(good, sign(good), key); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(string) string
	}{
		{"whitespace", func(raw string) string { return raw + "\n" }},
		{"wrong-order", func(raw string) string { return strings.Replace(raw, `"version":1,"assets":`, `"assets":`, 1) }},
		{"duplicate", func(raw string) string { return strings.Replace(raw, `"version":1`, `"version":1,"version":1`, 1) }},
		{"unknown", func(raw string) string { return strings.Replace(raw, `"version":1`, `"version":1,"command":"run"`, 1) }},
		{"case", func(raw string) string { return strings.Replace(raw, `"role"`, `"Role"`, 1) }},
		{"unsafe-path", func(raw string) string { return strings.Replace(raw, "lan/orbwin-lan-helper.exe", "../other.exe", 1) }},
		{"wrong-role", func(raw string) string { return strings.Replace(raw, `"role":"helper"`, `"role":"manager"`, 1) }},
		{"over-budget", func(raw string) string { return strings.Replace(raw, `"size":57671680`, `"size":268435457`, 1) }},
		{"fraction", func(raw string) string { return strings.Replace(raw, `"size":57671680`, `"size":1.0`, 1) }},
		{"zero-size", func(raw string) string { return strings.Replace(raw, `"size":57671680`, `"size":0`, 1) }},
		{"null", func(raw string) string { return strings.Replace(raw, `"size":57671680`, `"size":null`, 1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bad := []byte(test.mutate(string(good)))
			if _, err := VerifyAssetBundle(bad, sign(bad), key); err == nil {
				t.Fatal("signed malformed bundle accepted")
			}
		})
	}
	for _, untrusted := range []string{"", strings.Repeat("0", 64), strings.ToUpper(key), strings.Repeat("1", 64)} {
		if _, err = VerifyAssetBundle(good, sign(good), untrusted); err == nil {
			t.Fatal("unconfigured/foreign key accepted")
		}
	}
	bad := append([]byte(nil), good...)
	bad[len(bad)-3] ^= 1
	if _, err = VerifyAssetBundle(bad, sign(good), key); err == nil {
		t.Fatal("tampered bytes accepted")
	}
	if _, err = VerifyAssetBundle(good, append(sign(good), '\n'), key); err == nil {
		t.Fatal("noncanonical signature accepted")
	}
	if _, err = VerifyAssetBundle(good, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, good))), key); err == nil {
		t.Fatal("wrong-domain signature accepted")
	}
}
