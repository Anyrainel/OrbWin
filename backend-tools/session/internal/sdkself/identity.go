// Package sdkself retains the actual native SDK image/bundle through detached
// owner lifetime. Copied identity JSON is correlation metadata, never authority.
package sdkself

import (
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

func settleFailedLease(resource *Lease, failure error, close func() error) (*Lease, error) {
	if failure == nil {
		return resource, nil
	}
	if close() != nil {
		return resource, errors.New("native SDK lease cleanup unknown")
	}
	return nil, failure
}

type Identity struct {
	Schema       int    `json:"schema"`
	PID          uint32 `json:"pid"`
	CreationTime string `json:"creationTime"`
	Session      uint32 `json:"session"`
	ImageSHA256  string `json:"imageSha256"`
	BundleSHA256 string `json:"bundleSha256"`
	LeaseHeld    bool   `json:"leaseHeld"`
}

func exactSHA(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s && s != strings.Repeat("0", 64)
}
func (i Identity) Valid() bool {
	n, e := strconv.ParseUint(i.CreationTime, 10, 64)
	return i.Schema == 1 && i.PID > 0 && e == nil && n > 0 && strconv.FormatUint(n, 10) == i.CreationTime && i.Session > 0 && exactSHA(i.ImageSHA256) && exactSHA(i.BundleSHA256) && i.LeaseHeld
}
func SameIdentity(a, b Identity) bool { return a.Valid() && b.Valid() && a == b }
