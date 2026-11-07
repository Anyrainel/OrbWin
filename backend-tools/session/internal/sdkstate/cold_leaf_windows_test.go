//go:build windows

package sdkstate

import (
	"context"
	"orbwin.local/session/internal/sdkself"
	"testing"
	"time"
)

func TestColdLeafFactoryCannotBeMintedFromSelfLeaseOrBorrowedControl(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// The inert lease fabricates no native resources. A real lease follows the
	// same actual-native-transition/control/master refusal BEFORE any lease use.
	self := &sdkself.Lease{}
	for _, transition := range []*ColdTransition{nil, {}, {native: &nativeColdBackend{}}} {
		if factory, err := transition.NewColdLANObservationFactory(ctx, &ColdControl{}, self); factory != nil || err == nil {
			t.Fatal("lease/control shape minted actual Root/master authority")
		}
	}
}
