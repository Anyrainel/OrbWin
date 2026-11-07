package sdkstate

import (
	"context"
	"errors"
	"orbwin.local/session/internal/sdkself"
	"sync"
	"testing"
	"time"
)

// Coordinator-only native adapter fakes. No lease validation, process creation,
// SDKstate registration, cohort authority or native retirement grant is tested.
type coldLeafCohortFake struct {
	lease   *sdkself.Lease
	retain  func(context.Context, *LANObservationLeaf) error
	stopped func(context.Context, *LANObservationLeaf) error
	closed  func(context.Context, *LANObservationLeaf) error
}

func (c *coldLeafCohortFake) SDKBacking() *sdkself.Lease                               { return c.lease }
func (*coldLeafCohortFake) CheckNativeSupport(context.Context) error                   { return nil }
func (*coldLeafCohortFake) checkLANLeafAdoption(context.Context, *sdkself.Lease) error { return nil }
func (c *coldLeafCohortFake) RetainBeforeResume(ctx context.Context, l *LANObservationLeaf) error {
	if c.retain != nil {
		return c.retain(ctx, l)
	}
	return nil
}
func (c *coldLeafCohortFake) RecordStopped(ctx context.Context, l *LANObservationLeaf) error {
	if c.stopped != nil {
		return c.stopped(ctx, l)
	}
	return nil
}
func (c *coldLeafCohortFake) RecordClosed(ctx context.Context, l *LANObservationLeaf) error {
	if c.closed != nil {
		return c.closed(ctx, l)
	}
	return nil
}

type coldLeafNativeFake struct {
	mu                        sync.Mutex
	resumes, collects, closes int
	collectErr, closeErr      error
	owner                     *LANObservationLeaf
	resumeHook                func(context.Context) error
	attempted                 bool
}

func (n *coldLeafNativeFake) resume(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.resumeHook != nil {
		if err := n.resumeHook(ctx); err != nil {
			return err
		}
	}
	n.attempted = true
	n.resumes++
	return nil
}
func (n *coldLeafNativeFake) neverResumed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return !n.attempted
}
func (n *coldLeafNativeFake) collect(ctx context.Context) ([]byte, error, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.collects++
	return []byte("data"), ctx.Err(), n.collectErr
}
func (n *coldLeafNativeFake) closeKnown(context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.closes++
	return n.closeErr
}
func newColdLeafCoordinatorFake() (*LANObservationFactory, *coldLeafCohortFake, *coldLeafNativeFake) {
	lease := &sdkself.Lease{}
	co := &coldLeafCohortFake{lease: lease}
	native := &coldLeafNativeFake{}
	f := &LANObservationFactory{lease: lease, cohort: co, active: map[*LANObservationLeaf]struct{}{}, changed: make(chan struct{})}
	f.identity = f
	f.start = func(_ context.Context, leaf *LANObservationLeaf) (lanLeafNative, error) {
		native.owner = leaf
		return native, nil
	}
	return f, co, native
}

func TestColdLeafCancellationBetweenCustodyAndResumeRetiresActualClaim(t *testing.T) {
	f, _, native := newColdLeafCoordinatorFake()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	native.resumeHook = func(context.Context) error {
		cancel() // after Observe's ctx check, before the native Resume attempt
		return &lanLeafNoResume{native.owner, context.Canceled}
	}
	if _, err := f.Observe(ctx); !errors.Is(err, context.Canceled) || f.unknown || len(f.active) != 0 {
		t.Fatal("known never-resumed child was stranded", err)
	}
	if native.attempted || native.resumes != 0 || native.collects != 1 || native.closes != 1 {
		t.Fatal("cancelled child did not follow actual collect/close without Resume")
	}
}

func TestColdLeafNoResumeCannotClearAnAttemptedResume(t *testing.T) {
	f, _, native := newColdLeafCoordinatorFake()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	native.resumeHook = func(context.Context) error {
		native.attempted = true // syscall ambiguity cannot use the no-attempt branch
		return &lanLeafNoResume{native.owner, context.Canceled}
	}
	if _, err := f.Observe(ctx); err == nil || !f.unknown || len(f.active) != 1 || native.collects != 0 || native.closes != 0 {
		t.Fatal("attempted Resume was misclassified as a known cancellation")
	}
}

func TestColdLeafUnstartedReceiptRequiresSealedActualProducerCompletion(t *testing.T) {
	for _, mode := range []string{"empty-shape", "active", "birth", "unknown", "known-no-birth"} {
		t.Run(mode, func(t *testing.T) {
			f, co, _ := newColdLeafCoordinatorFake()
			f.root = &Root{}
			f.transition = &ColdTransition{root: f.root}
			f.adoption = co
			switch mode {
			case "empty-shape":
				f.root = nil
			case "active":
				f.active[&LANObservationLeaf{factory: f}] = struct{}{}
			case "birth":
				f.nativeAttempts = 1
			case "unknown":
				f.unknown = true
			case "known-no-birth":
				f.nativeAttempts = 1
				f.nativeNoBirth = 1
			}
			receipt, err := f.SealUnstartedNative()
			if mode != "known-no-birth" {
				if err == nil || receipt != nil {
					t.Fatal("incomplete native producer issued no-birth receipt")
				}
				return
			}
			if err != nil || receipt.Backing(f) != nil {
				t.Fatal("actual producer receipt unavailable", err)
			}
			copy := *receipt
			if copy.Backing(f) == nil || receipt.Backing(&LANObservationFactory{}) == nil {
				t.Fatal("copied/foreign receipt accepted")
			}
			// This models actual-used coordinator state only. The inert Root has
			// no native backend/control/master; no native release/grant is invoked.
		})
	}
}

func TestColdLeafCopiedHolderCannotReuseActualLeaseAndBridge(t *testing.T) {
	f, _, _ := newColdLeafCoordinatorFake()
	called := false
	clone := &LANObservationFactory{identity: f, root: f.root, transition: f.transition, lease: f.lease, adoption: f.adoption, cohort: f.cohort, active: f.active, start: func(context.Context, *LANObservationLeaf) (lanLeafNative, error) {
		called = true
		return nil, ErrColdLeaf
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := clone.Observe(ctx); err == nil || called || f.unknown || len(f.active) != 0 {
		t.Fatal("copied holder or substituted starter reused actual bridge")
	}
	if clone.SDKBacking() != nil || clone.RootBacking() != nil || clone.TransitionBacking() != nil || clone.SealAdmissions() == nil || clone.CheckNativeAdoption(ctx) == nil {
		t.Fatal("copied holder exposed an actual source capability")
	}
}

func TestColdLeafCancelledAfterClaimBeforeCreateDoesNotPoisonFactory(t *testing.T) {
	f, _, _ := newColdLeafCoordinatorFake()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := 0
	f.start = func(_ context.Context, l *LANObservationLeaf) (lanLeafNative, error) {
		started++
		cancel()
		return nil, knownLANLeafNoBirth(l, context.Canceled)
	}
	if _, err := f.Observe(ctx); !errors.Is(err, context.Canceled) || f.unknown || len(f.active) != 0 || started != 1 {
		t.Fatal("known no-birth cancellation retained a false unknown", err)
	}
	owner, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	if f.SealAdmissions() != nil {
		t.Fatal("known no-birth prevented source seal")
	}
	seal, err := f.Join(owner)
	if err != nil || seal.Backing(f) != nil {
		t.Fatal("zero native births did not join actual coordinator claim")
	}
}

func TestColdLeafNoBirthCannotClearAnActualPartialOwnerSlot(t *testing.T) {
	f, _, native := newColdLeafCoordinatorFake()
	f.start = func(_ context.Context, l *LANObservationLeaf) (lanLeafNative, error) {
		l.setNative(native)
		return nil, knownLANLeafNoBirth(l, context.Canceled)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := f.Observe(ctx); err == nil || !f.unknown || len(f.active) != 1 {
		t.Fatal("matching no-birth error discarded actual partial custody")
	}
	for leaf := range f.active {
		if leaf.nativeOwner() != native {
			t.Fatal("partial native backing lost")
		}
	}
}
func TestColdLeafSealKeepsOfferPumpProgressAndRejectsNewClaims(t *testing.T) {
	f, co, native := newColdLeafCoordinatorFake()
	entered, accept := make(chan struct{}), make(chan struct{})
	co.retain = func(context.Context, *LANObservationLeaf) error { close(entered); <-accept; return nil }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := f.Observe(ctx); result <- err }()
	<-entered
	if f.SealAdmissions() != nil {
		t.Fatal("admission seal waited for custody offer")
	}
	if _, err := f.Observe(ctx); err == nil {
		t.Fatal("sealed source created another claim")
	}
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	if _, err := f.Join(short); err == nil {
		t.Fatal("pending offer counted as native closed")
	}
	stop()
	close(accept) // SAME owner/pump may still accept the actual pending offer.
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if _, err := f.Join(ctx); err != nil {
		t.Fatal(err)
	}
	if native.resumes != 1 || native.collects != 1 || native.closes != 1 {
		t.Fatal("owned leaf did not follow retain/resume/join/close once")
	}
}
func TestColdLeafPartialOfferAckLossDeathAndNativeUncertaintyFenceWithoutReplay(t *testing.T) {
	for _, mode := range []string{"partial-create", "ack-lost", "SDK-peer-death", "native-join", "native-close"} {
		t.Run(mode, func(t *testing.T) {
			f, co, native := newColdLeafCoordinatorFake()
			failure := errors.New(mode)
			switch mode {
			case "partial-create":
				f.start = func(context.Context, *LANObservationLeaf) (lanLeafNative, error) { return native, failure }
			case "ack-lost", "SDK-peer-death":
				co.retain = func(context.Context, *LANObservationLeaf) error { return failure }
			case "native-join":
				native.collectErr = failure
			case "native-close":
				native.closeErr = failure
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := f.Observe(ctx); err == nil || !f.unknown || len(f.active) != 1 {
				t.Fatal("unknown actual claim/custody was dropped")
			}
			before := native.resumes
			if _, err := f.Observe(ctx); err == nil || native.resumes != before {
				t.Fatal("unknown source replayed")
			}
			_ = f.SealAdmissions()
			if _, err := f.Join(ctx); err == nil {
				t.Fatal("uncertain native lifetime issued source closure")
			}
		})
	}
}
