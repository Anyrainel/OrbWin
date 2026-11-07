package localnet

import (
	"context"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
)

// Only the actual cold holder installs this reader before actor construction.
// Native unknown stays in the factory even when observation is unavailable;
// there is no fallback to generic os/exec after a cold native refusal.
func readColdNativeLANChild(ctx context.Context, factory *sdkstate.LANObservationFactory) (NativeLANObservation, error) {
	var result NativeLANObservation
	if factory == nil {
		return result, sdkself.ErrColdLeaf
	}
	body, err := factory.Observe(ctx)
	if err != nil {
		return result, err
	}
	if err := strictJSON(body, &result); err != nil {
		return NativeLANObservation{}, err
	}
	return result, nil
}
