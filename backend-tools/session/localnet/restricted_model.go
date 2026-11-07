package localnet

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
)

var errRestricted = errors.New("local-services restrictive condition differs; request not applied")

// ExpectedServer is comparison DATA, never a native lease or permission.
type ExpectedServer struct {
	PID          uint32 `json:"pid"`
	CreationTime string `json:"creationTime"`
}

func (e ExpectedServer) valid() bool {
	n, err := strconv.ParseUint(e.CreationTime, 10, 64)
	return e.PID != 0 && err == nil && n != 0 && strconv.FormatUint(n, 10) == e.CreationTime
}
func compareExpectedServer(expected *ExpectedServer, pid uint32, creation string) error {
	if expected == nil || !expected.valid() || expected.PID != pid || expected.CreationTime != creation {
		return errRestricted
	}
	return nil
}

// RestrictiveStartup only removes operations; it cannot supply a native owner.
type RestrictiveStartup struct {
	RequireNew          bool
	DisabledStateSHA256 string
}

func validRestrictedSHA(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func (r RestrictiveStartup) valid() bool {
	return r.RequireNew && r.DisabledStateSHA256 == "" || !r.RequireNew && validRestrictedSHA(r.DisabledStateSHA256)
}
func restrictedOperation(operation string, enabled bool) bool {
	return operation == "snapshot" || operation == "stop-process" || operation == "configure" && !enabled
}

// Exact bounded raw record inventory. Disabled/schema/daemon validation belongs
// to the existing actual settings reader under the SAME native state Guard.
func restrictedInventory(records map[string][]byte) (string, error) {
	if len(records) == 0 || len(records) > 2 {
		return "", errRestricted
	}
	if _, ok := records["settings-v1.json"]; !ok {
		return "", errRestricted
	}
	names := make([]string, 0, len(records))
	for name, body := range records {
		if name != "settings-v1.json" && name != "lan-identity.json" || len(body) == 0 || len(body) > 128<<10 || name == "lan-identity.json" && len(body) > 2048 {
			return "", errRestricted
		}
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	h.Write([]byte("orbwin-sdk-disabled-state-v1\x00"))
	var size [8]byte
	for _, name := range names {
		binary.LittleEndian.PutUint64(size[:], uint64(len(name)))
		h.Write(size[:])
		h.Write([]byte(name))
		binary.LittleEndian.PutUint64(size[:], uint64(len(records[name])))
		h.Write(size[:])
		h.Write(records[name])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
