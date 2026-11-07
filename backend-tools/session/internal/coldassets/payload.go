// Package coldassets exposes only fixed ELF bytes compiled into the admitted
// SDK manager. It has no path, environment, hash or mutable-byte input API.
package coldassets

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

var ErrAssets = errors.New("compiled cold SDK execution payloads unavailable")

const maxPayload = 16 << 20

type Payloads struct {
	Tools, Trust             []byte
	ToolsSHA256, TrustSHA256 string
}

// Each call returns independent byte slices. They are provenance from the
// actual manager image lease, not a general permission to execute Linux code.
func Load() (Payloads, error) { return compiledPayloads() }

func decodePayloads(tools, toolsHash, trust, trustHash string) (Payloads, error) {
	a, err := decodePayload(tools, toolsHash)
	if err != nil {
		return Payloads{}, ErrAssets
	}
	b, err := decodePayload(trust, trustHash)
	if err != nil {
		clear(a)
		return Payloads{}, ErrAssets
	}
	return Payloads{a, b, toolsHash, trustHash}, nil
}

func decodePayload(encoded, digest string) ([]byte, error) {
	if len(encoded) < 88 || len(encoded) > base64.StdEncoding.EncodedLen(maxPayload) || len(digest) != 64 {
		return nil, ErrAssets
	}
	want, err := hex.DecodeString(digest)
	if err != nil || hex.EncodeToString(want) != digest || len(want) != 32 {
		return nil, ErrAssets
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) < 64 || len(data) > maxPayload {
		return nil, ErrAssets
	}
	sum := sha256.Sum256(data)
	if data[0] != 0x7f || string(data[1:4]) != "ELF" || data[4] != 2 || data[5] != 1 || data[6] != 1 || binary.LittleEndian.Uint16(data[18:20]) != 62 || hex.EncodeToString(sum[:]) != digest {
		clear(data)
		return nil, ErrAssets
	}
	return data, nil
}
