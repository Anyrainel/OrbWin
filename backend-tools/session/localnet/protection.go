package localnet

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
)

const protectedKeyHeader = "ORBWIN-LOCAL-KEY-1\n"

func protectionEntropy(installation, root string) ([]byte, error) {
	raw, err := hex.DecodeString(root)
	if !regexp.MustCompile(`^[a-f0-9-]{32,36}$`).MatchString(installation) || err != nil || len(raw) != 32 || hex.EncodeToString(raw) != root {
		return nil, errors.New("registered installation and exact local root fingerprint are required")
	}
	hash := sha256.Sum256([]byte("OrbWin local signing state v1\x00" + installation + "\x00" + root))
	return hash[:], nil
}
