//go:build windows && amd64

package lanprovider

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
	"unsafe"
)

func TestMITraversalLayoutMatchesCompiledHeader(t *testing.T) {
	checks := []struct{ got, want uintptr }{
		{unsafe.Offsetof(associationReadSessionTable{}.associatorInstances), 72},
		{unsafe.Sizeof(nativeShortArray{}), 16}, {unsafe.Offsetof(nativeShortArray{}.data), 0}, {unsafe.Offsetof(nativeShortArray{}.count), 8},
		{unsafe.Sizeof(nativeStringArray{}), 16}, {unsafe.Offsetof(nativeStringArray{}.data), 0}, {unsafe.Offsetof(nativeStringArray{}.count), 8},
		{unsafe.Sizeof(nativeDatetimeValue{}), 36}, {unsafe.Offsetof(nativeDatetimeValue{}.isTimestamp), 0}, {unsafe.Offsetof(nativeDatetimeValue{}.parts), 4},
	}
	for i, c := range checks {
		if c.got != c.want {
			t.Fatalf("traversal ABI contract %d differs", i)
		}
	}
}
func TestMITraversalWithoutDeadlineCannotOpenProvider(t *testing.T) {
	if out, e := ObserveFilterTraversal(context.Background()); e == nil || out.Complete {
		t.Fatal("unbounded traversal accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, e := ObserveFilterTraversal(ctx); e == nil || out.Complete {
		t.Fatal("cancelled traversal accepted")
	}
}

func TestUnexpectedTraversalCursorIsCancelledAndDrainedWithoutValueCopy(t *testing.T) {
	cursor := &fakeReadCursor{steps: []cursorStep{{hasValue: true}, {settled: true}}}
	if _, e := requireNativeTraversalCursor(context.Background(), cursor); e == nil || cursor.cancels != 1 || cursor.closes != 1 || cursor.copies != 0 || cursor.next != 2 {
		t.Fatal("unexpected acquired cursor abandoned or borrowed value copied")
	}
}

// Two separately reviewed stages, both compiled but opt-in only. Root enforces
// the external 15s worker hard limit. No provider mutation or product integration.
func TestMIFilterTraversalSchemasReadOnlyNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_LAN_MI_TRAVERSAL_SCHEMA") != "1" {
		t.Skip("root review and external deadline required; fixed schema gate only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, e := readTraversalDeclarations(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if len(d.rule.properties) != 39 || len(d.filters) != 7 {
		t.Fatal("fresh typed declarations incomplete")
	}
	for i, filter := range d.filters {
		if filter.class != filterFamilies[i].class || len(filter.properties) < 19 || len(filter.properties) > 24 {
			t.Fatal("fresh fixed typed filter declaration incomplete")
		}
	}
	t.Log(`{"schema":1,"complete":true,"filterClasses":14,"ruleProperties":39,"declarationsMatch":true,"activationAvailable":false,"mutationAuthorized":false}`)
}
func TestMIFilterTraversalReadOnlyNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_LAN_MI_FILTER_TRAVERSAL") != "1" {
		t.Skip("root review, preceding schema gate and external deadline required; fixed traversal only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, e := ObserveFilterTraversal(ctx)
	if e != nil {
		t.Fatal(e)
	}
	data, e := json.Marshal(out)
	if e != nil || len(data) > MaxOutput {
		t.Fatal("sanitized traversal output bound")
	}
	t.Log(string(data))
	if !out.Complete || out.Status != "observed" || !out.SchemaRechecked || out.Families != 7 || out.Passes != 2 || out.ForwardReads != 14 || out.ReverseReads != 14 || out.RuleFullReads != 3 || !out.TypedValuesStable || !out.ReverseIdentityMatched || out.Owned || out.ActivationAvailable || out.AtomicSnapshot || out.MutationAuthorized {
		t.Fatal("fixed typed traversal incomplete or authority claimed")
	}
}
