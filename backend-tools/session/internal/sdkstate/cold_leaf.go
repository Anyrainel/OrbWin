package sdkstate

import (
	"context"
	"errors"
	"orbwin.local/session/internal/sdkself"
	"sync"
	"time"
)

var ErrColdLeaf = sdkself.ErrColdLeaf

// Only the actual starter may produce this result, positively before creation
// was attempted and after any new resources were checked closed. It clears
// only this exact claimed operation; partial native returns never use it.
type lanLeafNoBirth struct {
	leaf  *LANObservationLeaf
	cause error
}

func (e *lanLeafNoBirth) Error() string { return e.cause.Error() }
func (e *lanLeafNoBirth) Unwrap() error { return e.cause }
func knownLANLeafNoBirth(leaf *LANObservationLeaf, cause error) error {
	if cause == nil {
		cause = ErrColdLeaf
	}
	return &lanLeafNoBirth{leaf, cause}
}

// The native owner returns this only before attempting ResumeThread. It still
// owns a created suspended child, so this permits collection, never discarding
// a claim or treating the operation as no-birth.
type lanLeafNoResume struct {
	leaf  *LANObservationLeaf
	cause error
}

func (e *lanLeafNoResume) Error() string { return e.cause.Error() }
func (e *lanLeafNoResume) Unwrap() error { return e.cause }

// Trusted native registration only. The actual SDKstate/Root implementation
// must bind its real cohort/lease and retain real leaf handles before ACK.
// No renderer/JSON adapter or metadata-root constructor is provided.
type lanLeafCohort interface {
	SDKBacking() *sdkself.Lease
	CheckNativeSupport(context.Context) error
	RetainBeforeResume(context.Context, *LANObservationLeaf) error
	RecordStopped(context.Context, *LANObservationLeaf) error
	RecordClosed(context.Context, *LANObservationLeaf) error
}

type lanLeafAdoptionOwner interface {
	checkLANLeafAdoption(context.Context, *sdkself.Lease) error
}
type lanLeafNative interface {
	resume(context.Context) error
	neverResumed() bool
	collect(context.Context) ([]byte, error, error) // query failure versus native uncertainty
	closeKnown(context.Context) error
}
type LANObservationLeaf struct {
	mu      sync.Mutex
	factory *LANObservationFactory
	id      uint64 // actual source claim order, never native generation authority
	ticket  *sdkself.NativeLANClaim
	native  lanLeafNative
	stage   string // producer diagnostic only, never wire authority
}

func (l *LANObservationLeaf) setStage(stage string) { l.mu.Lock(); l.stage = stage; l.mu.Unlock() }
func (l *LANObservationLeaf) setNative(native lanLeafNative) {
	l.mu.Lock()
	l.native = native
	l.mu.Unlock()
}
func (l *LANObservationLeaf) nativeOwner() lanLeafNative {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.native
}

type LANObservationFactory struct {
	mu                            sync.Mutex
	identity                      *LANObservationFactory // exact instance, not copied mutex/maps/capability fields
	root                          *Root                  // immutable actual authority backing, never a descriptor
	transition                    *ColdTransition
	lease                         *sdkself.Lease
	adoption                      lanLeafAdoptionOwner // actual private native Root/master backing
	cohort                        lanLeafCohort
	start                         func(context.Context, *LANObservationLeaf) (lanLeafNative, error)
	active                        map[*LANObservationLeaf]struct{}
	sealed, unknown               bool
	nativeAttempts, nativeNoBirth uint64            // actual physical starter and opaque no-birth receipts only
	unstarted                     *LANLeafUnstarted // assigned once after sealed actual producer completion
	changed                       chan struct{}
	leafSequence                  uint64
}

// Read-only no-birth receipt after prompt admission seal, not a retirement
// grant. The actual native caller still has to prove Root/control/master scope.
type LANLeafUnstarted struct{ factory *LANObservationFactory }

func (f *LANObservationFactory) SealUnstartedNative() (*LANLeafUnstarted, error) {
	if f == nil || f.identity != f {
		return nil, ErrColdLeaf
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.root == nil || f.transition == nil || f.lease == nil || f.adoption == nil || f.cohort == nil || f.start == nil {
		return nil, ErrColdLeaf
	}
	f.sealed = true
	f.signalLocked()
	if f.unknown || len(f.active) != 0 || f.nativeAttempts != f.nativeNoBirth {
		return nil, ErrColdLeaf
	}
	if f.unstarted == nil {
		f.unstarted = &LANLeafUnstarted{f}
	}
	return f.unstarted, nil
}

// This receipt/factory identity is immutable after sealed zero-active issuer
// completion. No Root call, waiting, or lower-lock acquisition is needed.
func (s *LANLeafUnstarted) Backing(f *LANObservationFactory) error {
	if s == nil || s.factory == nil || s.factory != f || f.identity != f || f.unstarted != s {
		return ErrColdLeaf
	}
	return nil
}

type LANLeafSeal struct{ factory *LANObservationFactory }

var coldLeafCustody = struct {
	sync.Mutex
	owners map[*LANObservationFactory]struct{}
}{owners: map[*LANObservationFactory]struct{}{}}

func (f *LANObservationFactory) signalLocked() {
	if f.changed != nil {
		close(f.changed)
	}
	f.changed = make(chan struct{})
}
func (f *LANObservationFactory) Observe(ctx context.Context) (payload []byte, result error) {
	if f == nil || f.identity != f || ctx == nil || ctx.Err() != nil {
		return nil, ErrColdLeaf
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return nil, ErrColdLeaf
	}
	f.mu.Lock()
	if f.sealed || f.unknown || f.lease == nil || f.cohort == nil || f.start == nil || len(f.active) != 0 {
		f.mu.Unlock()
		return nil, ErrColdLeaf
	}
	if f.leafSequence == ^uint64(0) {
		f.unknown = true
		f.mu.Unlock()
		return nil, ErrColdLeaf
	}
	f.leafSequence++
	leaf := &LANObservationLeaf{factory: f, id: f.leafSequence, ticket: sdkself.NewNativeLANClaim(), stage: "claimed"}
	if f.active == nil {
		f.active = map[*LANObservationLeaf]struct{}{}
	}
	f.active[leaf] = struct{}{}
	f.signalLocked()
	f.mu.Unlock()
	completed := false
	defer func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !completed {
			f.unknown = true
			leaf.setStage("unknown")
		} else {
			delete(f.active, leaf)
		}
		f.signalLocked()
	}()
	// A real factory is process-pinned by construction before the first native
	// effect; start stores partial custody in leaf before returning an error.
	native, err := f.start(ctx, leaf)
	if native != nil {
		leaf.setNative(native)
	}
	if err != nil {
		var noBirth *lanLeafNoBirth
		if native == nil && leaf.nativeOwner() == nil && errors.As(err, &noBirth) && noBirth.leaf == leaf {
			completed = true
			leaf.setStage("known-no-birth")
			return nil, noBirth.cause
		}
		return nil, ErrColdLeaf
	}
	if native == nil {
		return nil, ErrColdLeaf
	}
	leaf.setStage("created-suspended")
	// Birth already exists. Query cancellation cannot discard the transfer or
	// let a never-resumed process disappear before its master captures it. Use a
	// separate bounded owner interval, then use query ctx to choose Resume/stop.
	custody, cancelCustody := context.WithTimeout(context.Background(), 10*time.Second)
	err = f.cohort.RetainBeforeResume(custody, leaf)
	cancelCustody()
	if err != nil {
		return nil, ErrColdLeaf
	}
	leaf.setStage("native-custody-retained")
	if ctx.Err() == nil {
		if err := native.resume(ctx); err != nil {
			var noResume *lanLeafNoResume
			if !errors.As(err, &noResume) || noResume.leaf != leaf || leaf.nativeOwner() == nil || !native.neverResumed() {
				return nil, ErrColdLeaf
			}
			leaf.setStage("cancelled-before-resume")
		} else {
			leaf.setStage("resumed")
		}
	}
	payload, queryErr, nativeErr := native.collect(ctx)
	if nativeErr != nil {
		return nil, ErrColdLeaf
	}
	leaf.setStage("process-and-io-joined")
	cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if f.cohort.RecordStopped(cleanup, leaf) != nil {
		return nil, ErrColdLeaf
	}
	if native.closeKnown(cleanup) != nil {
		return nil, ErrColdLeaf
	}
	if f.cohort.RecordClosed(cleanup, leaf) != nil {
		return nil, ErrColdLeaf
	}
	leaf.setStage("closed")
	completed = true
	return payload, queryErr
}

// Prompt source admission seal. It never waits or consumes a protocol frame;
// the SAME authenticated control pump can continue draining actual offers.
func (f *LANObservationFactory) SealAdmissions() error {
	if f == nil || f.identity != f {
		return ErrColdLeaf
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lease == nil || f.cohort == nil || f.start == nil {
		return ErrColdLeaf
	}
	f.sealed = true
	f.signalLocked()
	if f.unknown {
		return ErrColdLeaf
	}
	return nil
}
func (f *LANObservationFactory) Join(ctx context.Context) (*LANLeafSeal, error) {
	if f == nil || f.identity != f || ctx == nil || ctx.Err() != nil {
		return nil, ErrColdLeaf
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return nil, ErrColdLeaf
	}
	for {
		f.mu.Lock()
		if !f.sealed || f.unknown || f.lease == nil || f.cohort == nil || f.start == nil {
			f.mu.Unlock()
			return nil, ErrColdLeaf
		}
		if len(f.active) == 0 {
			f.mu.Unlock()
			return &LANLeafSeal{f}, nil
		}
		changed := f.changed
		f.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}
func (s *LANLeafSeal) Backing(f *LANObservationFactory) error {
	if s == nil || s.factory == nil || s.factory != f || f.identity != f {
		return ErrColdLeaf
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.sealed || f.unknown || len(f.active) != 0 {
		return ErrColdLeaf
	}
	return nil // source-leaf issuer only, NOT complete job/guest retirement
}
func (f *LANObservationFactory) SDKBacking() *sdkself.Lease {
	if f == nil || f.identity != f {
		return nil
	}
	return f.lease
}

func (f *LANObservationFactory) RootBacking() *Root {
	if f == nil || f.identity != f {
		return nil
	}
	return f.root
}
func (f *LANObservationFactory) TransitionBacking() *ColdTransition {
	if f == nil || f.identity != f {
		return nil
	}
	return f.transition
}

// Admission recheck for the actual cold runtime constructor. This does not
// infer native support from callbacks: the opaque adoption is checked first.
func (f *LANObservationFactory) CheckNativeAdoption(ctx context.Context) error {
	if f == nil || f.identity != f || ctx == nil || ctx.Err() != nil {
		return ErrColdLeaf
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sealed || f.unknown || f.lease == nil || f.adoption == nil || f.adoption.checkLANLeafAdoption(ctx, f.lease) != nil || f.cohort == nil || f.start == nil {
		return ErrColdLeaf
	}
	if f.cohort.SDKBacking() != f.lease {
		return ErrColdLeaf
	}
	return nil
}
