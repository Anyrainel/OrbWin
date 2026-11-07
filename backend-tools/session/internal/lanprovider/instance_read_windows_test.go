//go:build windows && amd64

package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
	"unsafe"
)

func TestMIInstanceReadOffsetsMatchCompiledHeader(t *testing.T) {
	checks := []struct{ got, want uintptr }{
		{unsafe.Offsetof(instanceReadSessionTable{}.getInstance), 16}, {unsafe.Offsetof(instanceReadSessionTable{}.queryInstances), 64},
		{unsafe.Offsetof(instanceReadOperationTable{}.getInstance), 24},
		{unsafe.Offsetof(instanceReadTable{}.getClassName), 32}, {unsafe.Offsetof(instanceReadTable{}.getNamespace), 48},
		{unsafe.Offsetof(instanceReadTable{}.getElementCount), 56}, {unsafe.Offsetof(instanceReadTable{}.addElement), 64},
		{unsafe.Offsetof(instanceReadTable{}.getElement), 88}, {unsafe.Offsetof(instanceReadTable{}.getElementAt), 96},
		{unsafe.Offsetof(localInstanceApplicationTable{}.newInstance), 24}, {unsafe.Offsetof(persistentStoreOptionsTable{}.setCustomOption), 24},
	}
	for i, c := range checks {
		if c.got != c.want {
			t.Fatalf("compiled-header ABI mismatch at contract %d", i)
		}
	}
}
func TestMINewReadsPreCancelledDoNotOpenProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	schemas, e := ObserveFilterSchemas(ctx)
	if !errors.Is(e, context.Canceled) || schemas.Complete || len(schemas.Classes) != 0 {
		t.Fatal("cancelled schema read acquired state")
	}
	keys, e := ObserveRuleKeyRoundTrip(ctx)
	if !errors.Is(e, context.Canceled) || keys.Complete || keys.Found {
		t.Fatal("cancelled key read acquired state")
	}
}

func TestMINewReadsRequireCumulativeDeadlineBeforeOpeningProvider(t *testing.T) {
	schemas, e := ObserveFilterSchemas(context.Background())
	if e == nil || schemas.Complete || len(schemas.Classes) != 0 {
		t.Fatal("unbounded metadata read accepted")
	}
	keys, e := ObserveRuleKeyRoundTrip(context.Background())
	if e == nil || keys.Complete || keys.Found {
		t.Fatal("unbounded instance read accepted")
	}
}

// These tests are compiled but NEVER execute without root's explicit opt-in and
// a separately enforced external 15s child deadline. No provider mutation occurs.
func TestMIFilterSchemasReadOnlyNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_LAN_MI_FILTER_SCHEMA") != "1" {
		t.Skip("root review and external deadline required; fixed metadata read only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := ObserveFilterSchemas(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !out.Complete || len(out.Classes) != 14 || out.SchemaReviewed || out.ActivationAvailable || out.AtomicCreateQualified {
		t.Fatal("fixed schemas incomplete or activation claimed")
	}
	data, e := json.Marshal(out)
	if e != nil || len(data) > MaxOutput {
		t.Fatal("sanitized schema output bound")
	}
	t.Log(string(data))
}
func TestMIRuleKeysReadOnlyNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_LAN_MI_RULE_KEYS") != "1" {
		t.Skip("root review and external deadline required; fixed instance read only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, e := ObserveRuleKeyRoundTrip(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !out.Complete || !out.RuleSchemaRechecked || out.Owned || out.ActivationAvailable || out.AtomicSnapshot || out.MutationAuthorized {
		t.Fatal("fixed key observation incomplete or authority claimed")
	}
	// not-found is an honest observation but is NOT a positive round-trip gate.
	if out.Status != "not-found" && (!out.Found || out.RoundTrips != 2 || out.KeyCount != 4 || !out.ExactIDAndKeysMatched) {
		t.Fatal("positive identity round-trip incomplete")
	}
	data, e := json.Marshal(out)
	if e != nil || len(data) > MaxOutput {
		t.Fatal("sanitized key output bound")
	}
	t.Log(string(data))
}
