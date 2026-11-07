//go:build windows

package sdkstate

import (
	"context"
	"orbwin.local/session/internal/sdkself"
	"testing"
	"time"
)

type joinedSourceForwardAdapter struct {
	root    *Root
	factory *LANObservationFactory
	calls   int
}

func (a *joinedSourceForwardAdapter) RootBacking() *Root { a.calls++; return a.root }
func (a *joinedSourceForwardAdapter) LeafFactoryBacking() *LANObservationFactory {
	a.calls++
	return a.factory
}
func (a *joinedSourceForwardAdapter) VerifyJoined(context.Context) error { a.calls++; return nil }

func TestColdJoinedSourceAdaptersCannotMintFromForwardedScope(t *testing.T) {
	// Type rejection is independent of the forwarded pointer values; no actual
	// positive native Root is fabricated. It occurs before adapter callbacks.
	a := &joinedSourceForwardAdapter{root: &Root{}, factory: &LANObservationFactory{}}
	if nativeColdRuntimeOrigin(a) || a.calls != 0 {
		t.Fatal("forwarded scope minted concrete runtime origin")
	}
	b := &nativeColdBackend{finished: true}
	if b.bindRuntime(a.root, a) == nil || a.calls != 0 || b.runtime != nil {
		t.Fatal("callback adapter reached native registration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	target := &ColdTransition{}
	if r, err := target.JoinedSource(ctx, &ColdControl{}, &sdkself.JoinedLeaseClose{}); r != nil || err == nil {
		t.Fatal("empty scope minted joined source")
	}
	r := &ColdJoinedSource{transition: target, control: &ColdControl{}}
	if r.Check(ctx, &ColdTransition{}, r.control) == nil || r.Check(ctx, target, &ColdControl{}) == nil {
		t.Fatal("foreign transition/control borrowed source participant")
	}
}

func TestColdJoinedSourceActualMailboxAndSealBarriers(t *testing.T) {
	for _, mode := range []string{"pending-ack", "retained-query", "active-claim", "unsealed", "unknown", "foreign-factory"} {
		t.Run(mode, func(t *testing.T) {
			m := &nativeColdLANMaster{}
			f := &LANObservationFactory{sealed: true, adoption: m, cohort: m, active: map[*LANObservationLeaf]struct{}{}}
			f.identity = f
			m.factory = f
			m.pump = &coldLANLeafPump{native: m, factory: f}
			switch mode {
			case "pending-ack":
				m.pump.pending = &coldLANLeafOperation{}
			case "retained-query":
				m.retained = &nativeColdLeafRetention{}
			case "active-claim":
				f.active[&LANObservationLeaf{factory: f}] = struct{}{}
			case "unsealed":
				f.sealed = false
			case "unknown":
				m.pump.unknown = true
			case "foreign-factory":
				f = &LANObservationFactory{}
			}
			if m.joinedSourceBacking(f) == nil {
				t.Fatal("pending native source scope became joined", mode)
			}
		})
	}
}

func TestColdJoinedSourceCopiedParticipantAndCancellationCannotReplaceRegisteredOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	target := &ColdTransition{root: &Root{s: &rootState{}}}
	control := &ColdControl{}
	b := &nativeColdBackend{}
	r := &ColdJoinedSource{transition: target, root: target.root, control: control, backend: b}
	b.joinedSource = r
	copy := *r
	if copy.Check(ctx, target, control) == nil {
		t.Fatal("copied participant reused original registration")
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if r.Check(canceled, target, control) == nil || b.joinedSource != r {
		t.Fatal("cancellation consumed or replaced actual participant")
	}
}
