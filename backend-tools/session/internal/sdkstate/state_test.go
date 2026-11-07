package sdkstate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type fakeRecord struct {
	body     []byte
	closed   bool
	closeErr error
	closes   int
}

func (f *fakeRecord) data() []byte { return f.body }
func (f *fakeRecord) check() error {
	if f.closed {
		return ErrState
	}
	return nil
}
func (f *fakeRecord) close() error {
	if !f.closed {
		f.closed = true
		f.closes++
	}
	return f.closeErr
}

type fakeBackend struct {
	files             map[string][]byte
	mutations, closes int
	fault, closeFault bool
	checkFault        bool
	records           []*fakeRecord
	corruptFresh      bool
}

func (f *fakeBackend) check() error {
	if f.checkFault {
		return ErrState
	}
	return nil
}
func (f *fakeBackend) read(_ context.Context, n string, _ int64) (recordLease, error) {
	b, ok := f.files[n]
	if !ok {
		return nil, ErrNotExist
	}
	r := &fakeRecord{body: append([]byte(nil), b...)}
	f.records = append(f.records, r)
	return r, nil
}
func (f *fakeBackend) publish(_ context.Context, n string, old recordLease, b []byte) (recordLease, error) {
	f.mutations++
	if old != nil && old.close() != nil {
		return nil, ErrState
	}
	f.files[n] = append([]byte(nil), b...)
	fresh := &fakeRecord{body: append([]byte(nil), b...)}
	if f.corruptFresh {
		fresh.body = append(fresh.body, []byte("bad")...)
	}
	f.records = append(f.records, fresh)
	if f.fault {
		return fresh, ErrState
	}
	return fresh, nil
}
func (f *fakeBackend) remove(_ context.Context, n string, old recordLease) error {
	f.mutations++
	if old.close() != nil {
		return ErrState
	}
	delete(f.files, n)
	if f.fault {
		return ErrState
	}
	return nil
}
func (f *fakeBackend) list(context.Context) ([]string, error) {
	var out []string
	for n := range f.files {
		out = append(out, n)
	}
	return out, nil
}
func (f *fakeBackend) close() error {
	f.closes++
	if f.closeFault {
		return ErrState
	}
	return nil
}
func fixture(t *testing.T) (*Root, *fakeBackend, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	b := &fakeBackend{files: map[string][]byte{}}
	return &Root{&rootState{b: b}}, b, ctx
}
func TestSDKStateGuardRetainsOriginAndRequiresNativePriorRead(t *testing.T) {
	r, b, ctx := fixture(t)
	name := "settings-v1.json"
	b.files[name] = []byte("old")
	g, e := r.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if g.Replace(name, []byte("old"), []byte("new")) == nil || b.mutations != 0 {
		t.Fatal("supplied bytes minted authority")
	}
	data, e := g.Read(name, 128<<10)
	if e != nil {
		t.Fatal(e)
	}
	data[0] = 'x'
	if !bytes.Equal(b.records[0].body, []byte("old")) {
		t.Fatal("caller mutated retained snapshot")
	}
	if _, e = r.Begin(ctx); !errors.Is(e, ErrBusy) {
		t.Fatal("overlapping writer")
	}
	if r.Close() == nil || b.closes != 0 || b.records[0].closed {
		t.Fatal("active origin released")
	}
	if g.Check() != nil {
		t.Fatal("external-action guard lost")
	}
	copy := *g
	if copy.Replace(name, []byte("old"), []byte("new")) != nil {
		t.Fatal("known replace")
	}
	if g.Replace(name, []byte("old"), []byte("again")) == nil || b.mutations != 1 {
		t.Fatal("stale replacement replay")
	}
	if g.Close() != nil || copy.Close() != nil {
		t.Fatal("close")
	}
	if r.Close() != nil || r.Close() != nil || b.closes != 1 {
		t.Fatal("duplicate owner close")
	}
}
func TestSDKStateAdmittedUnknownRetainsOwnerAndBlocksReplay(t *testing.T) {
	for _, kind := range []string{"create", "replace", "append", "delete"} {
		t.Run(kind, func(t *testing.T) {
			r, b, ctx := fixture(t)
			g, _ := r.Begin(ctx)
			n := "settings-v1.json"
			if kind != "create" {
				b.files[n] = []byte("old")
				_, _ = g.Read(n, 128<<10)
			}
			b.fault = true
			var e error
			switch kind {
			case "create":
				e = g.Create(n, []byte("new"))
			case "replace":
				e = g.Replace(n, []byte("old"), []byte("new"))
			case "append":
				e = g.Append(n, []byte("old"), []byte("tail"))
			case "delete":
				e = g.Delete(n, []byte("old"))
			}
			if e == nil || b.mutations != 1 {
				t.Fatal("unknown mutation accepted")
			}
			if g.Create("authority-v1.json", []byte("x")) == nil || b.mutations != 1 {
				t.Fatal("unknown replay")
			}
			_ = g.Close()
			if r.Close() == nil || b.closes != 0 {
				t.Fatal("uncertain owner released")
			}
			if _, e = r.Begin(ctx); e == nil {
				t.Fatal("uncertain owner reused")
			}
		})
	}
}
func TestSDKStateAbsenceCreateAndForeignNameBounds(t *testing.T) {
	r, b, ctx := fixture(t)
	g, _ := r.Begin(ctx)
	if _, e := g.Read("settings-v1.json", 128<<10); !errors.Is(e, ErrNotExist) {
		t.Fatal(e)
	}
	if g.Create("settings-v1.json", []byte("first")) != nil {
		t.Fatal("fresh create")
	}
	if !errors.Is(g.Create("settings-v1.json", []byte("replacement")), ErrExist) || b.mutations != 1 {
		t.Fatal("Create overwrote")
	}
	for _, name := range []string{"../settings-v1.json", `C:\state.json`, "state.init", "controller.lock", "state.op", ".sdkstate-stage-a", "settings-v1.json:stream", "Settings-v1.json"} {
		if g.Create(name, []byte("x")) == nil {
			t.Fatal("foreign role accepted", name)
		}
	}
	b.files["unknown"] = []byte("x")
	if _, e := g.List(); e == nil {
		t.Fatal("unknown record hidden")
	}
	if fmt.Sprintf("%#v", r) != "SDKStateRoot(private native owner)" || fmt.Sprintf("%#v", g) != "SDKStateGuard(retained native records)" {
		t.Fatal("private fields logged")
	}
}
func TestSDKStateCloseUnknownLatchedAcrossCopies(t *testing.T) {
	r, b, ctx := fixture(t)
	b.files["settings-v1.json"] = []byte("x")
	g, _ := r.Begin(ctx)
	_, _ = g.Read("settings-v1.json", 128<<10)
	b.records[0].closeErr = ErrState
	copy := *g
	if g.Close() == nil || copy.Close() == nil || b.records[0].closes != 1 || r.Close() == nil || b.closes != 0 {
		t.Fatal("unknown record close lost")
	}
}
func TestSDKStateActualCloseGraphOwnerLast(t *testing.T) {
	for fail := -1; fail < 4; fail++ {
		var calls []int
		var closers []func() error
		for i := 0; i < 3; i++ {
			id := i
			closers = append(closers, func() error {
				calls = append(calls, id)
				if fail == id {
					return ErrState
				}
				return nil
			})
		}
		owner := func() error {
			calls = append(calls, 3)
			if fail == 3 {
				return ErrState
			}
			return nil
		}
		e := closeOwnerLast(closers, owner)
		want := []int{0, 1, 2, 3}
		if fail >= 0 {
			want = want[:fail+1]
		}
		if !reflect.DeepEqual(calls, want) || (e != nil) != (fail >= 0) {
			t.Fatal(fail, calls, e)
		}
	}
}
func TestSDKStateClosedRecordInventory(t *testing.T) {
	for _, name := range []string{"authority-v1.json", "authority-rotation-v1.json", "lan-identity.json", "namespace-" + string(bytes.Repeat([]byte{'a'}, 32)) + ".jsonl", "relay-" + string(bytes.Repeat([]byte{'b'}, 32)) + ".receipt", "tools-image-" + string(bytes.Repeat([]byte{'c'}, 64)) + ".receipt", "wsl-trust-12345678-1234-1234-1234-123456789012-" + string(bytes.Repeat([]byte{'a'}, 64)) + ".json"} {
		if recordBudget(name) == 0 {
			t.Fatal("required role missing", name)
		}
	}
}

func TestSDKStateGuardFinishRechecksOriginAfterExternalAction(t *testing.T) {
	r, b, ctx := fixture(t)
	b.files["settings-v1.json"] = []byte("x")
	g, _ := r.Begin(ctx)
	if _, e := g.Read("settings-v1.json", 128<<10); e != nil {
		t.Fatal(e)
	}
	b.checkFault = true
	if g.Close() == nil || r.Close() == nil || b.closes != 0 || b.records[0].closes != 1 {
		t.Fatal("late native origin failure became positive completion")
	}
}

func TestSDKStateConcurrentAdmissionAndCancelledRequestNoDispatch(t *testing.T) {
	r, b, ctx := fixture(t)
	g, e := r.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := r.Begin(ctx); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if !errors.Is(e, ErrBusy) {
			t.Fatal("concurrent owner admitted", e)
		}
	}
	_ = g.Close()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = r.Begin(cancelled); e == nil || b.mutations != 0 {
		t.Fatal("cancelled owner dispatched")
	}
}

func TestSDKStateRetainedBudgetProjectsBeforeDispatchAndAccountsExchange(t *testing.T) {
	r, b, ctx := fixture(t)
	g, _ := r.Begin(ctx)
	n := "settings-v1.json"
	if g.Create(n, []byte("old")) != nil || g.s.used != 3 {
		t.Fatal("created snapshot uncounted")
	}
	if g.Replace(n, []byte("old"), []byte("larger")) != nil || g.s.used != 6 {
		t.Fatal("replace growth uncounted")
	}
	if g.Append(n, []byte("larger"), []byte("xx")) != nil || g.s.used != 8 {
		t.Fatal("append growth uncounted")
	}
	if g.Replace(n, []byte("largerxx"), []byte("x")) != nil || g.s.used != 1 {
		t.Fatal("replacement shrink not released")
	}
	// Seed the already-authenticated cumulative footprint to exercise the hard
	// boundary without allocating128MiB in a pure policy test. Admission goes
	// through the production Guard.change path and actual backend call counter.
	g.s.used = maxReadBytes
	calls := b.mutations
	if g.Create("authority-v1.json", []byte("x")) == nil || b.mutations != calls {
		t.Fatal("aggregate Create dispatched past budget")
	}
	if g.Replace(n, []byte("x"), []byte("xx")) == nil || b.mutations != calls {
		t.Fatal("aggregate replacement dispatched past budget")
	}
	if g.Append(n, []byte("x"), []byte("tail")) == nil || b.mutations != calls {
		t.Fatal("aggregate append dispatched past budget")
	}
	g.s.used = 1
	if g.Delete(n, []byte("x")) != nil || g.s.used != 0 {
		t.Fatal("deleted body footprint retained")
	}
	if recordBudget("lan-review-"+string(bytes.Repeat([]byte{'d'}, 32))+".json") != 64<<10 {
		t.Fatal("closed issued review role missing")
	}
}
func TestSDKStateOwnerOriginRefusalNeverClosesActualOwner(t *testing.T) {
	calls := 0
	close := func() error { calls++; return nil }
	if closeCheckedRecord(true, func() error { return ErrState }, close) == nil || calls != 0 {
		t.Fatal("owner released on origin refusal")
	}
	if closeCheckedRecord(false, func() error { return ErrState }, close) == nil || calls != 1 {
		t.Fatal("non-owner record cleanup did not surface unknown")
	}
	if closeCheckedRecord(true, func() error { return nil }, close) != nil || calls != 2 {
		t.Fatal("positive owner retirement failed")
	}
	r, b, _ := fixture(t)
	b.checkFault = true
	copy := *r
	if r.Close() == nil || copy.Close() == nil || b.closes != 0 {
		t.Fatal("root origin refusal released owner")
	}
}

func TestSDKStateUnexpectedFreshFootprintCannotClearAdmission(t *testing.T) {
	r, b, ctx := fixture(t)
	b.files["settings-v1.json"] = []byte("x")
	g, _ := r.Begin(ctx)
	_, _ = g.Read("settings-v1.json", 128<<10)
	g.s.used = maxReadBytes - 1
	b.corruptFresh = true
	if g.Replace("settings-v1.json", []byte("x"), []byte("y")) == nil || b.mutations != 1 {
		t.Fatal("oversized fresh result accepted")
	}
	_ = g.Close()
	if r.Close() == nil || b.closes != 0 {
		t.Fatal("bad result cleared owner exclusion")
	}
}
