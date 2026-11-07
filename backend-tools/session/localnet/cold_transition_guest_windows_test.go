//go:build windows

package localnet

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestColdGuestIssuerSealsActualFactoryAndJoinsLateAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0
	g := newColdGuestIssuer(RuntimeOptions{WSLTrustFactory: func() (WSLTrustClient, error) { calls++; return nil, errors.New("native adapter") }})
	if g.seal(ctx) != nil || g.verifyNoGuestIssued() != nil {
		t.Fatal("actual zero-call producer failed to seal")
	}
	if _, err := g.factory(); err == nil || calls != 0 {
		t.Fatal("late factory dispatch crossed sealed admission")
	}
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	g = newColdGuestIssuer(RuntimeOptions{WSLTrustFactory: func() (WSLTrustClient, error) {
		close(entered)
		<-release
		return nil, errors.New("unknown native issuer")
	}})
	go func() { defer close(returned); _, _ = g.factory() }()
	<-entered
	short, done := context.WithTimeout(ctx, 15*time.Millisecond)
	if g.seal(short) == nil {
		t.Fatal("sealing ignored actual in-flight factory")
	}
	done()
	close(release)
	<-returned
	if g.seal(ctx) != nil || g.verifyNoGuestIssued() == nil {
		t.Fatal("unknown factory result minted no-guest proof")
	}
}
