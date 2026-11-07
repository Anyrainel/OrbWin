package localnet

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func execFrame(stream byte, data []byte) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
	return append(header, data...)
}
func framedConnection(t *testing.T, data []byte) *execConnection {
	t.Helper()
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	go func() {
		defer right.Close()
		for len(data) > 0 {
			size := 3
			if size > len(data) {
				size = len(data)
			}
			if _, err := right.Write(data[:size]); err != nil {
				return
			}
			data = data[size:]
		}
	}()
	return &execConnection{Conn: left, reader: left, closeWrite: func() error { return nil }}
}
func TestExecConnectionPreservesBinaryAcrossFragmentedFrames(t *testing.T) {
	expected := bytes.Repeat([]byte{0, 255, 13, 10, 1}, 20000)
	data := append(execFrame(1, expected[:3]), execFrame(1, nil)...)
	data = append(data, execFrame(1, expected[3:])...)
	connection := framedConnection(t, data)
	defer connection.Close()
	got, err := io.ReadAll(connection)
	if err != nil || !bytes.Equal(got, expected) {
		t.Fatalf("exec framing changed payload: %d %v", len(got), err)
	}
}
func TestExecConnectionRefusesMalformedFramesAndSurfacesDiagnostics(t *testing.T) {
	excessive := execFrame(1, nil)
	binary.BigEndian.PutUint32(excessive[4:], maxExecFrame+1)
	for _, data := range [][]byte{[]byte{1, 0, 0}, execFrame(0, nil), excessive, execFrame(2, []byte(strings.Repeat("x", 8193))), execFrame(3, []byte("engine refused exec")), execFrame(1, []byte("truncated"))[:12]} {
		connection := framedConnection(t, data)
		_, err := io.ReadAll(connection)
		connection.Close()
		if err == nil {
			t.Fatalf("accepted malformed frame: %v", data[:min(len(data), 12)])
		}
	}
	connection := framedConnection(t, execFrame(2, []byte("dial loopback: connection refused")))
	if _, err := io.ReadAll(connection); err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatal("tunnel failure became successful EOF")
	}
}
func TestExecConnectionReadDeadlineAndHalfCloseRemainUsable(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	halfClosed := false
	connection := &execConnection{Conn: left, reader: left, closeWrite: func() error { halfClosed = true; return nil }}
	if err := connection.CloseWrite(); err != nil || !halfClosed {
		t.Fatal("write half-close was not forwarded")
	}
	if err := connection.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	_, err := connection.Read(make([]byte, 1))
	timeout, ok := err.(net.Error)
	if !ok || !timeout.Timeout() {
		t.Fatalf("blocked Docker exec read ignored deadline: %v", err)
	}
}

func TestExecConnectionResumesAHeaderAfterReadDeadline(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	frame := execFrame(1, []byte("resumed binary\x00response"))
	release := make(chan struct{})
	go func() {
		defer right.Close()
		if _, err := right.Write(frame[:3]); err != nil {
			return
		}
		<-release
		_, _ = right.Write(frame[3:])
	}()
	connection := &execConnection{Conn: left, reader: left, closeWrite: func() error { return nil }}
	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	if _, err := connection.Read(make([]byte, 32)); err == nil {
		t.Fatal("partial header read did not time out")
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	close(release)
	response, err := io.ReadAll(connection)
	if err != nil || string(response) != "resumed binary\x00response" {
		t.Fatalf("resuming after timeout corrupted framing: %q %v", response, err)
	}
}
