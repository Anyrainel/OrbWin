//go:build windows

package lanworker

import (
	"testing"
	"time"
)

// In-memory typed packet fake only; no native pipe/socket/process APIs.
func TestNativeDatagramBridgeKeepsEmptyAndBinaryPacketsThenJoins(t *testing.T) {
	a, b := newMemoryDatagram(), newMemoryDatagram()
	done := make(chan struct{})
	go func() { copyNativeDatagrams(a, b); close(done) }()
	a.replies <- []byte{}
	a.replies <- []byte{0, 1, 0, 2}
	if len(getPacket(t, b.requests)) != 0 || string(getPacket(t, b.requests)) != string([]byte{0, 1, 0, 2}) {
		t.Fatal("native typed bridge changed packet framing")
	}
	b.replies <- []byte("reply")
	if string(getPacket(t, a.requests)) != "reply" {
		t.Fatal("reverse packet missing")
	}
	a.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("datagram closure did not join both directions")
	}
	select {
	case <-b.closed:
	default:
		t.Fatal("remote typed association was not closed")
	}
}
