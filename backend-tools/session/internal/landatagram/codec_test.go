package landatagram

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"
)

type memoryStream struct {
	in              *bytes.Reader
	out             bytes.Buffer
	closed          bool
	readAt, writeAt time.Time
	chunk           int
}

func (m *memoryStream) Read(b []byte) (int, error) {
	if m.chunk > 0 && len(b) > m.chunk {
		b = b[:m.chunk]
	}
	return m.in.Read(b)
}
func (m *memoryStream) Write(b []byte) (int, error) {
	if m.chunk > 0 && len(b) > m.chunk {
		b = b[:m.chunk]
	}
	return m.out.Write(b)
}
func (m *memoryStream) Close() error                       { m.closed = true; return nil }
func (m *memoryStream) SetReadDeadline(t time.Time) error  { m.readAt = t; return nil }
func (m *memoryStream) SetWriteDeadline(t time.Time) error { m.writeAt = t; return nil }
func TestPacketsFragmentationEmptyAndShortDestination(t *testing.T) {
	var wire bytes.Buffer
	for _, p := range [][]byte{nil, []byte("drop-whole"), []byte("next"), bytes.Repeat([]byte{255}, MaxPayload)} {
		if err := WriteFrame(&wire, p); err != nil {
			t.Fatal(err)
		}
	}
	m := &memoryStream{in: bytes.NewReader(wire.Bytes()), chunk: 3}
	c := New(m)
	if n, e := c.ReadPacket(nil); n != 0 || e != nil {
		t.Fatal("empty lost", n, e)
	}
	dst := bytes.Repeat([]byte{9}, 2)
	if n, e := c.ReadPacket(dst); n != 0 || !errors.Is(e, ErrShortBuffer) || !bytes.Equal(dst, []byte{9, 9}) {
		t.Fatal("prefix exposed", n, e, dst)
	}
	buffer := make([]byte, MaxPayload)
	if n, e := c.ReadPacket(buffer); n != 4 || e != nil || string(buffer[:n]) != "next" {
		t.Fatal("lost framing", n, e)
	}
	if n, e := c.ReadPacket(buffer); n != MaxPayload || e != nil || buffer[n-1] != 255 {
		t.Fatal(n, e)
	}
	if _, e := c.ReadPacket(buffer); e != io.EOF || !m.closed {
		t.Fatal("EOF not terminal", e)
	}
}
func TestMalformedTruncatedAndOversizedTerminal(t *testing.T) {
	var over [4]byte
	binary.BigEndian.PutUint32(over[:], MaxPayload+1)
	for _, p := range [][]byte{{0}, {0, 0, 0, 2, 1}, over[:]} {
		m := &memoryStream{in: bytes.NewReader(p)}
		c := New(m)
		if _, e := c.ReadPacket(make([]byte, MaxPayload)); e == nil || !m.closed {
			t.Fatal("malformed accepted", p, e)
		}
	}
}
func TestWritesBoundedAndDeadlineClamped(t *testing.T) {
	now := time.Unix(100, 0)
	m := &memoryStream{in: bytes.NewReader(nil), chunk: 1}
	c := newCodec(m, func() time.Time { return now })
	if _, e := c.WritePacket(make([]byte, MaxPayload+1)); !errors.Is(e, ErrOversize) || m.out.Len() != 0 || m.closed {
		t.Fatal("oversize emitted")
	}
	if n, e := c.WritePacket(nil); n != 0 || e != nil || !bytes.Equal(m.out.Bytes(), []byte{0, 0, 0, 0}) {
		t.Fatal(n, e)
	}
	if !m.writeAt.Equal(now.Add(WriteBudget)) {
		t.Fatal("unbounded write")
	}
	early := now.Add(time.Second)
	_ = c.SetDeadline(early)
	if !m.readAt.Equal(early) || !m.writeAt.Equal(early) {
		t.Fatal("caller deadline ignored")
	}
	_ = c.SetDeadline(time.Time{})
	if !m.readAt.Equal(now.Add(ReadBudget)) {
		t.Fatal("idle bound removed")
	}
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
func TestNonprogressWriterFails(t *testing.T) {
	if !errors.Is(WriteFrame(zeroWriter{}, []byte("x")), io.ErrShortWrite) {
		t.Fatal("nonprogress accepted")
	}
}
