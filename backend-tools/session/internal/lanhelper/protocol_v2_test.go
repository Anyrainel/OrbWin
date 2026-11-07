package lanhelper

import (
	"bytes"
	"encoding/binary"
	"io"
	"orbwin.local/session/internal/lanfirewall"
	"strings"
	"testing"
)

func protocolFrame(kind string, seq uint64) Frame {
	sha := strings.Repeat("a", 64)
	f := Frame{Version: 2, Kind: kind, Sequence: seq, LeaseSHA: sha}
	switch kind {
	case "stage-request":
		f.LeaseSHA = ""
		f.Stage = &StageRequest{Base: lanfirewall.Request{Operation: "create"}}
	case "prepared":
		f.Prepared = &Prepared{LeaseSHA: sha, Sequence: seq}
	case "child-ack", "worker-hello":
		f.Child = &Child{Identity: lanfirewall.CallerIdentity{PID: 123}}
	case "commit", "worker-grant":
		f.Grant = &Grant{}
		f.Grant.Rule.LeaseSHA = sha
	}
	return f
}
func TestV2FramingRejectsDuplicateUnknownTrailingOversizeAndBodyConfusion(t *testing.T) {
	for _, data := range []string{`{"version":2,"kind":"stop","sequence":1,"sequence":2,"leaseSha256":"` + strings.Repeat("a", 64) + `"}`, `{"version":2,"kind":"worker-hello","sequence":1,"leaseSha256":"` + strings.Repeat("a", 64) + `","child":{"identity":{"PID":1,"PID":2}}}`, `{"version":2,"kind":"stop","sequence":1,"leaseSha256":"` + strings.Repeat("a", 64) + `","command":"do not execute"}`, `{"version":2,"kind":"stop","sequence":1,"leaseSha256":"` + strings.Repeat("a", 64) + `"} {}`, strings.Repeat(" ", FrameBudget+1)} {
		if _, e := Decode([]byte(data)); e == nil {
			t.Fatal("unsafe/ambiguous wire accepted")
		}
	}
	f := protocolFrame("stop", 1)
	f.Stream = &Stream{ID: 1}
	if f.Validate() == nil {
		t.Fatal("body accepted on closed control tag")
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], FrameBudget+1)
	if _, e := Read(bytes.NewReader(header[:])); e == nil {
		t.Fatal("oversized allocation accepted")
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 2 {
		p = p[:2]
	}
	return w.Buffer.Write(p)
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
func TestV2FramesHandleFragmentationAndShortWritesWithoutRawStreamOverread(t *testing.T) {
	f := protocolFrame("stop", 1)
	w := &shortWriter{}
	if e := Write(w, f); e != nil {
		t.Fatal(e)
	}
	w.Buffer.WriteString("raw-stream")
	got, e := Read(&w.Buffer)
	if e != nil || got.Kind != "stop" || w.Buffer.String() != "raw-stream" {
		t.Fatal("frame consumed or corrupted following raw bytes")
	}
	if Write(zeroWriter{}, f) != io.ErrShortWrite {
		t.Fatal("zero write did not fail")
	}
}
func TestAdministrativeSessionRejectsReplayReorderAndUnknownOutcomeRetry(t *testing.T) {
	s := &Session{}
	for n, kind := range []string{"stage-request", "prepared", "child-ack", "commit"} {
		if e := s.Accept(protocolFrame(kind, uint64(n+1))); e != nil {
			t.Fatal(e)
		}
	}
	if s.Accept(protocolFrame("stage-request", 1)) == nil {
		t.Fatal("terminal create conversation replayed")
	}
	s = &Session{}
	if s.Accept(protocolFrame("prepared", 2)) == nil {
		t.Fatal("out of order native reply accepted")
	}
	if e := s.Accept(protocolFrame("stage-request", 1)); e != nil {
		t.Fatal(e)
	}
	s.Unknown()
	if s.Accept(protocolFrame("stage-request", 1)) == nil {
		t.Fatal("unknown create reset session")
	}
}
func TestV2RejectsLoneWrongCaseAndCaseAliasKeysAtEveryTypedLevel(t *testing.T) {
	sha := strings.Repeat("a", 64)
	for _, data := range []string{
		`{"Version":2,"kind":"stop","sequence":1,"leaseSha256":"` + sha + `"}`,
		`{"version":2,"Version":2,"kind":"stop","sequence":1,"leaseSha256":"` + sha + `"}`,
		`{"version":2,"kind":"worker-hello","sequence":1,"leaseSha256":"` + sha + `","child":{"Identity":{"PID":123}}}`,
		`{"version":2,"kind":"worker-hello","sequence":1,"leaseSha256":"` + sha + `","child":{"identity":{"PID":123,"pid":124}}}`,
		`{"version":2,"kind":"stage-request","sequence":1,"stage":{"base":{"operation":"create","Scope":{}}}}`,
		`{"version":2,"kind":"stop","sequence":1,"leaseSha256":"` + sha + `","child":null}`,
	} {
		if _, e := Decode([]byte(data)); e == nil {
			t.Fatal("wrong-case/alias/null field accepted")
		}
	}
}

func TestV2DatagramFramesRetainPacketTagAndRejectBodyConfusion(t *testing.T) {
	for _, kind := range []string{"datagram-open", "datagram-ready"} {
		f := Frame{Version: 2, Kind: kind, Sequence: 7, LeaseSHA: strings.Repeat("a", 64), Stream: &Stream{ID: 7}}
		if kind == "datagram-open" {
			f.Stream.Peer = "192.168.4.20:60000"
		}
		var buffer bytes.Buffer
		if Write(&buffer, f) != nil {
			t.Fatal("typed datagram frame refused")
		}
		decoded, e := Read(&buffer)
		if e != nil || decoded.Kind != kind || *decoded.Stream != *f.Stream {
			t.Fatal("datagram tag/identity changed")
		}
		f.Child = &Child{Identity: lanfirewall.CallerIdentity{PID: 1}}
		if f.Validate() == nil {
			t.Fatal("datagram body confusion accepted")
		}
	}
}
