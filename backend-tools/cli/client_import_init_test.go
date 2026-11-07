package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestImportCompletedCodecGoldenAndNativeFacts(t *testing.T) {
	b, e := os.ReadFile("../../fixtures/completed-import.json")
	if e != nil {
		t.Fatal(e)
	}
	sid := "S-1-5-21-test"
	root := clientImportID{1, 2, 3}
	profile := `C:\Users\test`
	v, e := parseClientImportInit(b, sid, root, profile)
	if e != nil {
		t.Fatal(e)
	}
	encoded, e := canonicalImportJSON(v)
	if e != nil || !bytes.Equal(b, encoded) {
		t.Fatal("golden producer mismatch")
	}
	for name, body := range map[string][]byte{
		"unknown":        bytes.Replace(b, []byte(`"schema":2`), []byte(`"schema":2,"unknown":0`), 1),
		"duplicate":      bytes.Replace(b, []byte(`"schema":2`), []byte(`"schema":2,"schema":2`), 1),
		"nested-unknown": bytes.Replace(b, []byte(`"volume":4`), []byte(`"volume":4,"other":0`), 1),
		"wrong-type":     bytes.Replace(b, []byte(`"rootVolume":1`), []byte(`"rootVolume":"1"`), 1),
		"null-id":        bytes.Replace(b, []byte(`"volume":4`), []byte(`"volume":null`), 1),
		"zero-binding":   bytes.Replace(b, []byte(strings.Repeat("1", 64)), []byte(strings.Repeat("0", 64)), 1),
		"over-budget":    append(bytes.Clone(b), bytes.Repeat([]byte(" "), 16*1024)...),
	} {
		if _, e := parseClientImportInit(body, sid, root, profile); e == nil {
			t.Fatal(name, "admitted")
		}
	}
	for n := 0; n < len(b); n++ {
		if _, e := parseClientImportInit(b[:n], sid, root, profile); e == nil {
			t.Fatal("partial init admitted", n)
		}
	}
	for _, facts := range []struct {
		sid     string
		id      clientImportID
		profile string
	}{{"other", root, profile}, {sid, clientImportID{1, 2, 4}, profile}, {sid, root, `D:\Users\test`}} {
		if _, e := parseClientImportInit(b, facts.sid, facts.id, facts.profile); e == nil {
			t.Fatal("wrong actual native facts admitted")
		}
	}
	var data any
	if json.Unmarshal(b, &data) != nil {
		t.Fatal("golden JSON")
	}
}
func TestImportBootstrapSharedRequiresDesktopChoice(t *testing.T) {
	for _, literal := range []string{freshClientMarker, clientImportMarker} {
		for n := 0; n <= len(literal); n++ {
			body := []byte(literal[:n])
			both := bytes.HasPrefix([]byte(freshClientMarker), body) && bytes.HasPrefix([]byte(clientImportMarker), body)
			kind := clientBootstrapClass(body)
			if both && kind != bootstrapChoiceRequired {
				t.Fatal(n, "lost ambiguity")
			}
			if !both && kind != map[bool]clientBootstrapKind{true: bootstrapFresh, false: bootstrapImport}[literal == freshClientMarker] {
				t.Fatal(n, "wrong disjoint kind")
			}
		}
	}
	for _, b := range []string{" ", "{}", `{"kind":"fresh-client","schema":1}`, `{"schema":1,"kind":"other"}`} {
		if clientBootstrapClass([]byte(b)) != bootstrapUnsupported {
			t.Fatal("unknown marker admitted", b)
		}
	}
}
