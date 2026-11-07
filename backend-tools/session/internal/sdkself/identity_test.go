package sdkself

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSDKSelfIdentityExactCorrelationAndNoJSONLeaseConstructor(t *testing.T) {
	identity := Identity{1, 123, "123456789", 1, strings.Repeat("a", 64), strings.Repeat("b", 64), true}
	if !identity.Valid() || !SameIdentity(identity, identity) {
		t.Fatal("positive copied native correlation rejected")
	}
	for _, mutate := range []func(*Identity){func(i *Identity) { i.Schema = 2 }, func(i *Identity) { i.PID = 0 }, func(i *Identity) { i.CreationTime = "0123" }, func(i *Identity) { i.CreationTime = "0" }, func(i *Identity) { i.Session = 0 }, func(i *Identity) { i.ImageSHA256 = strings.Repeat("0", 64) }, func(i *Identity) { i.BundleSHA256 = strings.Repeat("B", 64) }, func(i *Identity) { i.LeaseHeld = false }, func(i *Identity) { i.PID++ }} {
		other := identity
		mutate(&other)
		if SameIdentity(identity, other) {
			t.Fatal("unknown or changed peer identity accepted")
		}
	}
	var lease Lease
	data, _ := json.Marshal(identity)
	if json.Unmarshal(data, &lease) != nil {
		t.Fatal("fixture decode failed")
	}
	if _, err := lease.Identity(); err == nil {
		t.Fatal("JSON constructed native lease")
	}
	if strings.Contains(fmt.Sprintf("%#v", &lease), "123456789") {
		t.Fatal("lease formatter leaked copied identity")
	}
	if _, err := Hold(nil); err == nil {
		t.Fatal("nil context admitted native lease")
	}
	if _, err := HoldPeer(context.Background(), nil); err == nil {
		t.Fatal("nil pipe peer admitted")
	}
}
func TestSDKSelfFailedCleanupPreservesPartialResource(t *testing.T) {
	resource := &Lease{}
	failure := errors.New("qualification failure")
	for _, unknown := range []bool{false, true} {
		calls := 0
		partial, err := settleFailedLease(resource, failure, func() error {
			calls++
			if unknown {
				return errors.New("native close detail")
			}
			return nil
		})
		if calls != 1 || err == nil || unknown && partial != resource || !unknown && partial != nil || strings.Contains(err.Error(), "native close detail") {
			t.Fatal("failed cleanup lost resource or exposed native detail")
		}
	}
}
