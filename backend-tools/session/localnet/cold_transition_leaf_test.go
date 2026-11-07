package localnet

import (
	"context"
	"errors"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
	"testing"
	"time"
)

func TestColdLANSourceNeverFallsBackAfterNativeHolderRefusal(t *testing.T) {
	// A zero holder is intentionally NOT native authority. Selecting the actual
	// cold source must return that refusal rather than spawning an ordinary
	// diagnostic executable as a convenience fallback.
	r := &ServiceRuntime{opts: RuntimeOptions{Ledger: &NamespaceLedger{}, coldLANLeaf: &sdkstate.LANObservationFactory{}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := r.lanOptionsSource().native(ctx); !errors.Is(err, sdkself.ErrColdLeaf) {
		t.Fatal("cold native refusal lost or generic fallback selected", err)
	}
	if _, err := readColdNativeLANChild(ctx, nil); !errors.Is(err, sdkself.ErrColdLeaf) {
		t.Fatal("missing native holder became a fallback route", err)
	}
}
