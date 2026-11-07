//go:build windows && amd64

package lanhelper

import (
	"encoding/binary"
	"testing"
)

func TestLANConsentTaskDialogLayout(t *testing.T) {
	config, button := packedTaskDialog(nil, nil, nil, nil, 0x1122334455667788)
	if len(config) != 160 || len(button) != 12 || binary.LittleEndian.Uint32(config[0:4]) != 160 || binary.LittleEndian.Uint32(config[20:24]) != 0x808 || binary.LittleEndian.Uint32(config[24:28]) != 8 || binary.LittleEndian.Uint32(config[72:76]) != 2 || binary.LittleEndian.Uint64(config[140:148]) != 0x1122334455667788 || binary.LittleEndian.Uint32(button[0:4]) != 1000 {
		t.Fatal("packed buffer encoding differs from compiled SDK header offsets")
	}
}
