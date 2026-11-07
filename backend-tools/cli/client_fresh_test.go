package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFreshMarkerOnlyExactProducerPrefixes(t *testing.T) {
	for end := 0; end <= len(freshClientMarker); end++ {
		if !freshMarkerPrefix([]byte(freshClientMarker[:end])) {
			t.Fatalf("producer marker cut %d refused", end)
		}
	}
	for _, bad := range []string{`null`, `[]`, ` {`, `{"kind":"fresh-client"}`, freshClientMarker + "\n", `{"schema":2`, `{"schema":1,"kind":"legacy`} {
		if freshMarkerPrefix([]byte(bad)) {
			t.Fatalf("nonproducer marker admitted: %q", bad)
		}
	}
}

func TestFreshInitializationCutsBoundToActualRoot(t *testing.T) {
	expected := map[string]any{"schema": json.Number("1"), "product": "orbwin", "ownerSid": "S-1-5-21-123", "rootVolume": json.Number("123"), "rootHigh": json.Number("0"), "rootLow": json.Number("456"), "admission": "fresh"}
	for _, body := range []string{
		`{"admission":"fresh","ownerSid":"S-1-5-21-123","product":"orbwin","rootHigh":0,"rootLow":456,"rootVolume":123,"schema":1}`,
		`{"schema":1,"product":"orbwin","ownerSid":"S-1-5-21-123","rootVolume":123,"rootHigh":0,"rootLow":456,"admission":"fresh"}`,
	} {
		for end := 0; end <= len(body); end++ {
			if !freshInitPrefix([]byte(body[:end]), expected) {
				t.Fatalf("actual root metadata cut %d refused: %s", end, body[:end])
			}
		}
		for _, bad := range []string{strings.Replace(body, "456", "457", 1), strings.Replace(body, `"schema":1`, `"schema":null`, 1), strings.Replace(body, `"admission":"fresh"`, `"admission":"import"`, 1), body[:len(body)-1] + `,"schema":1}`, body + "garbage"} {
			if freshInitPrefix([]byte(bad), expected) {
				t.Fatalf("foreign/duplicate/malformed metadata admitted: %s", bad)
			}
		}
	}
	if freshInitPrefix([]byte(`{"Schema":1`), expected) || freshInitPrefix([]byte(`{"schema":1 `), expected) {
		t.Fatal("nonproducer partial encoding admitted")
	}
}

func TestFreshDefaultsEveryCutAndFieldOrder(t *testing.T) {
	profile := "C:\\Users\\A<&>é\u2028\u2029\\u003c"
	path := profile + `\.orbwin-private\runtime\docker-` + strings.Repeat("a", 64) + `\docker-cli-plugins`
	for _, rust := range []bool{true, false} {
		fields := []string{`"auths":{}`, `"credsStore":"orbwin"`, `"cliPluginsExtraDirs":[` + freshQuote(path, rust) + `]`}
		for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
			body := "{" + fields[order[0]] + "," + fields[order[1]] + "," + fields[order[2]] + "}"
			for end := 0; end <= len(body); end++ {
				if !freshDefaultsPrefix([]byte(body[:end]), profile) {
					t.Fatalf("default cut %d/%d encodingRust=%v order=%v refused: %s", end, len(body), rust, order, body[:end])
				}
			}
		}
	}
}

func TestFreshDefaultsRefuseUserDataAndUnownedSelectors(t *testing.T) {
	profile := `C:\Users\fixture`
	path := profile + `\.orbwin-private\runtime\docker-` + strings.Repeat("a", 64) + `\docker-cli-plugins`
	valid := `{"auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":[` + freshQuote(path, true) + `]}`
	for _, bad := range []string{
		strings.Replace(valid, `"auths":{}`, `"auths":{"registry":{"auth":"opaque"}}`, 1),
		strings.Replace(valid, `"auths":{}`, `"auths":null`, 1),
		strings.Replace(valid, `"auths":{}`, `"Auths":{}`, 1),
		strings.Replace(valid, `"orbwin"`, `"desktop"`, 1),
		strings.Replace(valid, strings.Repeat("a", 64), strings.Repeat("0", 64), 1),
		strings.Replace(valid, strings.Repeat("a", 64), strings.Repeat("A", 64), 1),
		strings.Replace(valid, `fixture`, `other`, 1),
		strings.Replace(valid, `docker-cli-plugins`, `docker-cli-plugins\\..\\foreign`, 1),
		strings.Replace(valid, `"auths":{}`, `"auths":{},"auths":{}`, 1),
		valid[:len(valid)-1] + `,"custom":true}`,
		valid + `[]`, `null`, `[]`, `{"credsStore":"orbwin","auths":{"x":`,
	} {
		if freshDefaultsPrefix([]byte(bad), profile) {
			t.Fatalf("unowned initialization content admitted: %s", bad)
		}
	}
	// Even an EOF cut after a complete zero hash cannot acquire a future valid pin.
	zero := strings.Replace(valid, strings.Repeat("a", 64), strings.Repeat("0", 64), 1)
	end := strings.Index(zero, strings.Repeat("0", 64)) + 64
	if freshDefaultsPrefix([]byte(zero[:end]), profile) {
		t.Fatal("complete zero selector hash admitted as extendable prefix")
	}
}

func TestFreshRustQuotePreservesLiteralEscapeText(t *testing.T) {
	text := "\\u003c<&>\u2028\u2029\x00\b\f\n\r\t\"\\"
	encoded := freshQuote(text, true)
	var roundtrip string
	if json.Unmarshal([]byte(encoded), &roundtrip) != nil || roundtrip != text || !strings.Contains(encoded, `\\u003c<&>`) {
		t.Fatalf("literal escape text corrupted: %s", encoded)
	}
}

func TestFreshRecoveryDoesNotClaimOriginalRestoration(t *testing.T) {
	review := &clientWriterRecovery{freshInitialization: true}
	if review.acceptsRestoreOriginal() || !strings.Contains(review.summary(), "empty credential settings") || strings.Contains(review.summary(), "Restore the exact") {
		t.Fatal("initialization recovery misrepresented as original restoration")
	}
	if (&clientWriterRecovery{localReplace: true}).acceptsRestoreOriginal() || !(&clientWriterRecovery{}).acceptsRestoreOriginal() {
		t.Fatal("restore-original compatibility changed")
	}
}
