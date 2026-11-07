//go:build windows

package sdkstate

import (
	"context"
	"golang.org/x/sys/windows"
	"testing"
	"time"
)

func TestSDKColdControllerOfferCannotBeBuiltFromHandlesOrReceipt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Correlation numbers alone cannot expose a remote controller. The actual
	// native committed transition and retained candidate are always mandatory.
	o := &ColdSupervisorOffer{remote: [4]windows.Handle{101, 102, 103, 104}}
	if _, err := o.NativeHandles(); err == nil || o.VerifyRetained(ctx) == nil {
		t.Fatal("numeric handles manufactured a native controller offer")
	}
	for _, transition := range []*ColdTransition{nil, {}, {committed: true}} {
		if offer, err := transition.OfferNativeSupervisor(ctx); err == nil || offer != nil {
			t.Fatal("missing actual native transition admitted controller transfer")
		}
	}
	b := &nativeColdBackend{supervisorOffer: o}
	if b.prepareCommittedClose(TransitionReceipt{}) == nil {
		t.Fatal("offered native copies released without full retirement")
	}
	if (&nativeColdBackend{}).prepareCommittedClose(TransitionReceipt{}) == nil {
		t.Fatal("TEMP-only path released exclusions without an external holder")
	}
}
