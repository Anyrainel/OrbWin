package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestOwnedEngineRejectsWrongIdentity(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" {
		t.Skip("requires the recorded owned running engine")
	}
	var output bytes.Buffer
	code, err := run(strings.NewReader(`{"kind":"open","container":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","daemonId":"intentionally-wrong-identity","shell":"sh","cols":80,"rows":24}`), &output)
	if code != 125 || err == nil || !strings.Contains(err.Error(), "endpoint does not match") {
		t.Fatalf("did not reject owned endpoint identity before exec: %d %v", code, err)
	}
}

type partialWriter struct{ bytes.Buffer }

func (w *partialWriter) Write(data []byte) (int, error) {
	if len(data) > 2 {
		data = data[:2]
	}
	return w.Buffer.Write(data)
}
func TestInputPreservesShortWrites(t *testing.T) {
	var writer partialWriter
	input := []byte("héllo\x00\x1b[A")
	if err := writeFull(&writer, input); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(writer.Bytes(), input) {
		t.Fatal("input truncated or transformed")
	}
}
func TestArrayEncodedInput(t *testing.T) {
	msg, err := decode([]byte(`{"kind":"input","data":[97,0,98]}`))
	if err != nil || !bytes.Equal(msg.Data, []byte{'a', 0, 'b'}) {
		t.Fatalf("input bytes changed: %v %v", msg.Data, err)
	}
}

func TestRejectForeignOrMalformedOpen(t *testing.T) {
	valid := message{Kind: "open", Container: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DaemonID: "owned", Shell: "sh", Cols: 80, Rows: 24}
	if err := validateOpen(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*message){func(m *message) { m.Container = "--help" }, func(m *message) { m.DaemonID = "" }, func(m *message) { m.Shell = "cmd.exe" }, func(m *message) { m.Cols = 0 }, func(m *message) { m.Rows = 1000 }} {
		candidate := valid
		mutate(&candidate)
		if validateOpen(candidate) == nil {
			t.Fatal("accepted invalid configuration")
		}
	}
}
func TestProtocolRejectsUnknownAndTrailingInput(t *testing.T) {
	for _, line := range []string{`{"kind":"input","command":"host shell"}`, `{"kind":"close"} {"kind":"open"}`, `[]`} {
		if _, err := decode([]byte(line)); err == nil {
			t.Fatalf("accepted %s", line)
		}
	}
	if _, err := decode([]byte(`{"kind":"input","data":"aGVsbG8="}`)); err != nil {
		t.Fatal(err)
	}
}
