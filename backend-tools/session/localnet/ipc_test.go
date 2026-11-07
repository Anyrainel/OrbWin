package localnet

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestServiceFrameRejectsOversizeBeforeAllocatingAndDetectsTruncation(t *testing.T) {
	for _, size := range []uint32{0, ipcRequestBudget + 1, ^uint32(0)} {
		var header [4]byte
		binary.LittleEndian.PutUint32(header[:], size)
		if _, err := readServiceFrame(bytes.NewReader(header[:]), ipcRequestBudget); err == nil {
			t.Fatal("accepted unbounded frame", size)
		}
	}
	var encoded bytes.Buffer
	if err := writeServiceFrame(&encoded, []byte("typed request"), ipcRequestBudget); err != nil {
		t.Fatal(err)
	}
	frame := encoded.Bytes()
	if _, err := readServiceFrame(bytes.NewReader(frame[:len(frame)-1]), ipcRequestBudget); err == nil {
		t.Fatal("truncated frame accepted")
	}
	decoded, err := readServiceFrame(bytes.NewReader(frame), ipcRequestBudget)
	if err != nil || string(decoded) != "typed request" {
		t.Fatal(string(decoded), err)
	}
}
