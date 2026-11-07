//go:build windows

package localnet

import (
	"context"
	"orbwin.local/session/internal/sdkstate"
	"orbwin.local/session/internal/wsltrust"
	"sync"
	"time"
)

// Runtime-owned factory admission, installed before its actor starts. Every
// actual Host is retained and sealed; unknown/custom issuers cannot claim zero
// guest work. Counters are producer evidence, never options/disk/caller flags.
type coldGuestIssuer struct {
	mu               sync.Mutex
	base             func() (WSLTrustClient, error)
	custom           bool
	retained         []WSLTrustClient
	sealed, unknown  bool
	active, produced uint64
	hosts            map[*wsltrust.Host]*wsltrust.SealedGuestIssuer
}

func newColdGuestIssuer(options RuntimeOptions) *coldGuestIssuer {
	base := options.WSLTrustFactory
	if base == nil {
		asset := append([]byte(nil), options.WSLTrustAsset...)
		digest := options.WSLTrustSHA256
		base = func() (WSLTrustClient, error) { return newWSLTrustClient(asset, digest) }
	}
	return &coldGuestIssuer{base: base, custom: options.WSLTrustFactory != nil, hosts: map[*wsltrust.Host]*wsltrust.SealedGuestIssuer{}}
}
func (g *coldGuestIssuer) factory() (WSLTrustClient, error) {
	g.mu.Lock()
	if g.sealed || g.unknown || g.produced == ^uint64(0) {
		g.mu.Unlock()
		return nil, sdkstate.ErrRecoveryRequired
	}
	g.produced++
	g.active++
	if g.custom {
		g.unknown = true
	} // Callback may issue work outside its returned Host.
	g.mu.Unlock()
	value, err := g.base()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active--
	if value != nil {
		g.retained = append(g.retained, value)
	}
	host, ok := value.(*wsltrust.Host)
	if ok && host != nil {
		if _, seen := g.hosts[host]; !seen {
			g.hosts[host] = nil
		}
	}
	if err != nil || !ok || host == nil {
		g.unknown = true
		if err == nil {
			return nil, sdkstate.ErrRecoveryRequired
		}
		return value, err
	}
	if g.sealed {
		seal, sealErr := host.SealGuestIssuer()
		g.hosts[host] = seal
		if sealErr != nil {
			g.unknown = true
		}
		return nil, sdkstate.ErrRecoveryRequired
	}
	if g.custom {
		return nil, sdkstate.ErrRecoveryRequired
	}
	return value, nil
}

// Seal BEFORE actor Join. A factory already admitted is joined; a Host it
// produces during sealing cannot escape unsealed to a late caller.
func (g *coldGuestIssuer) seal(ctx context.Context) error {
	if g == nil || !coldPreparationContext(ctx) {
		return sdkstate.ErrRecoveryRequired
	}
	g.mu.Lock()
	g.sealed = true
	for host, existing := range g.hosts {
		if existing == nil {
			seal, err := host.SealGuestIssuer()
			if err != nil {
				g.unknown = true
			}
			g.hosts[host] = seal
		}
	}
	g.mu.Unlock()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		g.mu.Lock()
		joined := g.active == 0
		g.mu.Unlock()
		if joined {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Read-only final check after the SAME actor joined. Zero factory calls proves
// no Host existed in this actual runtime. Otherwise every retained native Host
// must provide its own sealed zero-dispatch capability. Issued work still needs
// the distinct actual Linux guest retirement provider, which is not present.
func (g *coldGuestIssuer) verifyNoGuestIssued() error {
	if g == nil {
		return sdkstate.ErrRecoveryRequired
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.sealed || g.unknown || g.active != 0 {
		return sdkstate.ErrRecoveryRequired
	}
	if g.produced == 0 {
		return nil
	}
	if len(g.hosts) == 0 {
		return sdkstate.ErrRecoveryRequired
	}
	for _, seal := range g.hosts {
		if seal == nil || seal.NoGuestIssued() != nil {
			return sdkstate.ErrRecoveryRequired
		}
	}
	return nil
}
