//go:build windows && amd64

package lanhelper

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Deliberate copies of exclusively inert objects. Reflection avoids hiding the
// scenario behind a copylocks suppression or ever copying real native handles.
func reflectedOwnerCopy[T any](owner *T) *T {
	copy := new(T)
	reflect.ValueOf(copy).Elem().Set(reflect.ValueOf(owner).Elem())
	return copy
}

func expectOwnerClosePanic(t *testing.T, close func() error) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("fake close did not panic")
		}
	}()
	_ = close()
}

func TestLANRegistrationNativeClosePanicKeepsUnknownOnEveryReplay(t *testing.T) {
	manager := newNativeManager(0, 0)
	managerCalls := 0
	expectOwnerClosePanic(t, func() error {
		return manager.closeUsing(func() error { managerCalls++; panic("fake manager close panic") })
	})
	for range 2 {
		if !errors.Is(manager.Close(), errNativeOwnerCloseUnknown) || managerCalls != 1 {
			t.Fatal("manager panic became nil/retried")
		}
	}
	authority := newAuthorizedInstallation(nil, "")
	authorityCalls := 0
	expectOwnerClosePanic(t, func() error {
		return authority.closeUsing(func() error { authorityCalls++; panic("fake authority close panic") })
	})
	for range 2 {
		if !errors.Is(authority.Close(), errNativeOwnerCloseUnknown) || authorityCalls != 1 {
			t.Fatal("installation panic became nil/retried")
		}
	}
}

func TestLANRegistrationActualSettlersReturnExactPartialAfterClosePanic(t *testing.T) {
	creatorFailure := errors.New("fake constructor read failed")
	panicValue := errors.New("fake native cleanup panic")
	manager := newNativeManager(0, 0)
	managerCalls := 0
	gotManager, managerErr := settleOrdinarySelfFailure(manager, creatorFailure, func() error {
		return manager.closeUsing(func() error { managerCalls++; panic(panicValue) })
	})
	if gotManager != manager || !errors.Is(managerErr, creatorFailure) || !errors.Is(managerErr, errNativeOwnerCloseUnknown) || manager.cleanupPanic != panicValue || !errors.Is(manager.Close(), errNativeOwnerCloseUnknown) || managerCalls != 1 {
		t.Fatal("actual manager settlement lost panic/partial/unknown cache", managerErr)
	}
	authority := newAuthorizedInstallation(manager, "")
	authorityCalls := 0
	gotAuthority, authorityErr := settleInstallationFailure(authority, creatorFailure, func() error {
		return authority.closeUsing(func() error { authorityCalls++; panic(panicValue) })
	})
	if gotAuthority != authority || !errors.Is(authorityErr, creatorFailure) || !errors.Is(authorityErr, errNativeOwnerCloseUnknown) || authority.cleanupPanic != panicValue || !errors.Is(authority.Close(), errNativeOwnerCloseUnknown) || authorityCalls != 1 {
		t.Fatal("actual installation settlement lost panic/partial/unknown cache", authorityErr)
	}
	var trace []string
	outerManager := &fixedRoleOwnerFake{name: "outer manager", trace: &trace}
	if !errors.Is(settleFixedRoleOwners(gotAuthority, outerManager), errNativeOwnerCloseUnknown) || len(trace) != 0 || authorityCalls != 1 {
		t.Fatal("partial constructor panic released manager or retried authority")
	}
}

func TestLANRegistrationAdminSettlerPanicIsTerminalWithoutClosingManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	manager := newNativeManager(0, 0)
	authority := newAuthorizedInstallation(manager, "")
	owner := &AdministrativeClient{ctx: ctx, cancel: cancel, done: make(chan struct{}), manager: manager, authority: authority}
	owner.nativeOrigin = owner
	if !claimAdministrativeClient(owner) {
		t.Fatal("fake slot occupied")
	}
	t.Cleanup(func() {
		if manager.process != 0 || manager.image != nil || len(authority.held) != 0 {
			t.Fatal("native resources in fake cleanup")
		}
		releaseAdministrativeClient(owner)
	})
	panicValue := errors.New("fake administrative authority cleanup panic")
	creatorFailure := errors.New("fake administrative ctor failed")
	authorityCalls, managerCalls := 0, 0
	got, err := settleAdministrativeConstruction(owner, creatorFailure, func() error {
		if closeErr := authority.closeUsing(func() error { authorityCalls++; panic(panicValue) }); closeErr != nil {
			return closeErr
		}
		return manager.closeUsing(func() error { managerCalls++; return nil })
	})
	if got != owner || !errors.Is(err, creatorFailure) || !errors.Is(err, errNativeOwnerCloseUnknown) || owner.cleanupPanic != panicValue || !errors.Is(authority.Close(), errNativeOwnerCloseUnknown) || authorityCalls != 1 || managerCalls != 0 {
		t.Fatal("actual admin settlement released parent or hid panic", err)
	}
	select {
	case <-owner.done:
	default:
		t.Fatal("terminal partial client result not published")
	}
	for range 2 {
		if !errors.Is(owner.Close(context.Background()), errNativeOwnerCloseUnknown) || authorityCalls != 1 || managerCalls != 0 {
			t.Fatal("partial client replay lost unknown or reclosed")
		}
	}
	if !owner.closed || ctx.Err() == nil {
		t.Fatal("partial client remained usable")
	}
	// Prove the manager's actual once boundary was not consumed by either the
	// failed constructor cleanup or Client.Close replay. This owner is inert.
	if manager.closeUsing(func() error { managerCalls++; return nil }) != nil || managerCalls != 1 {
		t.Fatal("manager Close was consumed after authority cleanup panic")
	}
}

func TestLANRegistrationNativeCopiesCannotCloseOrMintInstallation(t *testing.T) {
	manager := newNativeManager(0, 0)
	copyBefore := reflectedOwnerCopy(manager)
	calls := 0
	if !errors.Is(copyBefore.closeUsing(func() error { calls++; return nil }), errNativeOwnerCopy) || calls != 0 {
		t.Fatal("pre-close manager copy dispatched close")
	}
	if _, err := copyBefore.AuthorizeInstallation(strings.Repeat("a", 32)); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copied manager reached native installation ctor", err)
	}
	if _, err := holdSignedInstallation(copyBefore, strings.Repeat("a", 32), false, false); err == nil {
		t.Fatal("internal copied manager reached native construction")
	}
	if manager.closeUsing(func() error { calls++; return nil }) != nil || calls != 1 {
		t.Fatal("original incomplete manager lost legal cleanup")
	}
	if !errors.Is(reflectedOwnerCopy(manager).Close(), errNativeOwnerCopy) || manager.Close() != nil {
		t.Fatal("post-close manager copy/replay erased ownership")
	}
	authority := newAuthorizedInstallation(manager, "")
	authorityCopy := reflectedOwnerCopy(authority)
	authorityCalls := 0
	if !errors.Is(authorityCopy.closeUsing(func() error { authorityCalls++; return nil }), errNativeOwnerCopy) || authorityCalls != 0 {
		t.Fatal("installation copy dispatched cleanup")
	}
	if authority.closeUsing(func() error { authorityCalls++; return nil }) != nil || authorityCalls != 1 {
		t.Fatal("actual incomplete installation cleanup refused")
	}
	if !errors.Is(reflectedOwnerCopy(authority).Close(), errNativeOwnerCopy) || authority.Close() != nil {
		t.Fatal("post-close installation copy/replay erased ownership")
	}
	if (&NativeManager{}).Close() == nil || (&AuthorizedInstallation{}).Close() == nil {
		t.Fatal("zero owner lazily adopted itself")
	}
}

func TestLANRegistrationAdministrativeCopyCannotForwardOriginalNativeOwners(t *testing.T) {
	owner, s := fakeAdminOwner(t)
	copy := reflectedOwnerCopy(owner)
	if !errors.Is(copy.Close(context.Background()), errNativeOwnerCopy) {
		t.Fatal("copy forwarded Close to original lower owners")
	}
	if _, err := copy.currentSession(context.Background()); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copy admitted a native session", err)
	}
	if !errors.Is(copy.PersistReview(context.Background(), OwnerReview{}), errNativeOwnerCopy) {
		t.Fatal("copy reached native review")
	}
	if _, err := copy.readObservation(context.Background(), nil); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copy reached native observation", err)
	}
	if _, err := copy.ReadRule(context.Background(), Prepared{}); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copy reached native rule read", err)
	}
	if _, err := copy.RecoverRemoval(context.Background(), Prepared{}); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copy reached native removal read", err)
	}
	if _, err := copy.ReadCleanup(context.Background(), ""); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copy reached native cleanup read", err)
	}
	if _, err := copy.RecoverReview(context.Background(), "", ""); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copy reached native review read", err)
	}
	if _, err := settleAdministrativeConstruction(copy, errors.New("fake copy ctor"), func() error { panic("copied settlement dispatched cleanup") }); !errors.Is(err, errNativeOwnerCopy) {
		t.Fatal("copy settled original constructor owners", err)
	}
	if owner.closed || owner.ctx.Err() != nil {
		t.Fatal("copy changed original owner")
	}
	close(s.opened)
	if s.finish() != nil || owner.Close(context.Background()) != nil {
		t.Fatal("original owner lost legitimate cleanup")
	}
	if !errors.Is(reflectedOwnerCopy(owner).Close(context.Background()), errNativeOwnerCopy) || owner.Close(context.Background()) != nil {
		t.Fatal("slot release broke original replay/copy refusal")
	}
}

type fixedRoleOwnerFake struct {
	name  string
	trace *[]string
	err   error
}

func (f *fixedRoleOwnerFake) Close() error { *f.trace = append(*f.trace, f.name); return f.err }

func TestLANRegistrationFixedRoleGateDoesNotReleaseManagerAfterUnknownAuthority(t *testing.T) {
	var trace []string
	failure := errors.New("fake fixed role partial authority")
	authority := &fixedRoleOwnerFake{name: "authority", trace: &trace, err: failure}
	manager := &fixedRoleOwnerFake{name: "manager", trace: &trace}
	if !errors.Is(settleFixedRoleOwners(authority, manager), failure) || !reflect.DeepEqual(trace, []string{"authority"}) {
		t.Fatal("fixed role released manager before unknown authority")
	}
	trace = nil
	authority.err = nil
	if settleFixedRoleOwners(authority, manager) != nil || !reflect.DeepEqual(trace, []string{"authority", "manager"}) {
		t.Fatal("fixed close order changed")
	}
	trace = nil
	manager.err = failure
	if !errors.Is(settleFixedRoleOwners(authority, manager), failure) || !reflect.DeepEqual(trace, []string{"authority", "manager"}) {
		t.Fatal("manager cleanup failure became success")
	}
}

func TestLANRegistrationNativeCloseFailureIsStoredAndPartialCreatorsNeverRetry(t *testing.T) {
	failure := errors.New("fake native close unknown")
	original := errors.New("fake creator failed")
	manager := newNativeManager(0, 0)
	calls := 0
	manager.closeUsing(func() error { calls++; return failure })
	for range 2 {
		got, err := settleOrdinarySelfFailure(manager, original, manager.Close)
		if got != manager || !errors.Is(err, failure) || !errors.Is(err, original) || calls != 1 {
			t.Fatal("native manager partial lost or retried", err)
		}
	}
	authority := newAuthorizedInstallation(nil, "")
	authorityCalls := 0
	authority.closeUsing(func() error { authorityCalls++; return failure })
	for range 2 {
		got, err := settleInstallationFailure(authority, original, authority.Close)
		if got != authority || !errors.Is(err, failure) || !errors.Is(err, original) || authorityCalls != 1 {
			t.Fatal("installation partial lost or retried", err)
		}
	}
}

func TestLANRegistrationAdministrativeConstructorExposesTerminalPartialAndKeepsSlot(t *testing.T) {
	failure := errors.New("fake authority close unknown")
	authority := newAuthorizedInstallation(nil, "")
	calls := 0
	authority.closeUsing(func() error { calls++; return failure })
	manager := newNativeManager(0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	owner := &AdministrativeClient{ctx: ctx, cancel: cancel, done: make(chan struct{}), manager: manager, authority: authority}
	owner.nativeOrigin = owner
	if !claimAdministrativeClient(owner) {
		t.Fatal("fake slot occupied")
	}
	t.Cleanup(func() {
		if manager.process != 0 || manager.image != nil || len(authority.held) != 0 {
			t.Fatal("fixture cannot release native custody")
		}
		releaseAdministrativeClient(owner)
	})
	got, err := settleAdministrativeConstruction(owner, errors.New("fake constructor error"), func() error { return settleFixedRoleOwners(owner.authority, owner.manager) })
	if got != owner || !errors.Is(err, failure) || ctx.Err() == nil || !owner.closed {
		t.Fatal("partial constructor result not terminal", err)
	}
	for range 2 {
		if !errors.Is(owner.Close(context.Background()), failure) || calls != 1 {
			t.Fatal("partial client close lost stored failure or retried")
		}
	}
	if claimAdministrativeClient(&AdministrativeClient{}) {
		t.Fatal("unknown owner admitted replacement")
	}
	if owner.manager != manager || owner.authority != authority {
		t.Fatal("partial native graph lost")
	}
}

func TestLANRegistrationKnownConstructorFailureReleasesOnlySettledFakeOwner(t *testing.T) {
	manager := newNativeManager(0, 0)
	calls := 0
	manager.closeUsing(func() error { calls++; return nil })
	ctx, cancel := context.WithCancel(context.Background())
	owner := &AdministrativeClient{ctx: ctx, cancel: cancel, done: make(chan struct{}), manager: manager}
	owner.nativeOrigin = owner
	if !claimAdministrativeClient(owner) {
		t.Fatal("fake slot occupied")
	}
	t.Cleanup(func() { releaseAdministrativeClient(owner) })
	failure := errors.New("fake validation failure")
	got, err := settleAdministrativeConstruction(owner, failure, func() error { return settleFixedRoleOwners(owner.authority, owner.manager) })
	if got != nil || !errors.Is(err, failure) || calls != 1 || ctx.Err() == nil {
		t.Fatal("known settled failure retained or retried owner", err)
	}
	administrativeClientOwner.Lock()
	released := administrativeClientOwner.client == nil
	administrativeClientOwner.Unlock()
	if !released {
		t.Fatal("settled constructor did not release slot")
	}
}
