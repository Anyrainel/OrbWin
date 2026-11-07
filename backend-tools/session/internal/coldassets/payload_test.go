package coldassets

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func TestPayloadBoundsOriginAndIndependentDecodedBytes(t *testing.T) {
	elf := make([]byte, 64)
	copy(elf, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	elf[18] = 62
	sum := sha256.Sum256(elf)
	digest, encoded := hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(elf)
	p, err := decodePayloads(encoded, digest, encoded, digest)
	if err != nil {
		t.Fatal(err)
	}
	p.Tools[0] = 0
	if p.Trust[0] != 0x7f {
		t.Fatal("fixed payload roles alias mutable backing bytes")
	}
	for _, bad := range []struct{ data, digest string }{{encoded, strings.Repeat("0", 64)}, {encoded, strings.ToUpper(digest)}, {encoded[:len(encoded)-1], digest}, {strings.Repeat("A", base64.StdEncoding.EncodedLen(maxPayload)+4), digest}} {
		if _, err := decodePayload(bad.data, bad.digest); err == nil {
			t.Fatal("uncompiled/partial/oversized payload accepted")
		}
	}
	elf[18] = 3
	sum = sha256.Sum256(elf)
	if _, err := decodePayload(base64.StdEncoding.EncodeToString(elf), hex.EncodeToString(sum[:])); err == nil {
		t.Fatal("non-amd64 payload accepted")
	}
}
