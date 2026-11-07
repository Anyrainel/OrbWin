//go:build windows

package localnet

import (
	"context"
	"testing"
	"time"
)

func TestColdPreparationRefusesMissingNativeEntryBeforeAnyEffect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if p, err := ConsumeColdService(ctx, 0); err == nil || p != nil {
		t.Fatal("missing inherited native grant admitted cold entry")
	}
	canceled, done := context.WithCancel(ctx)
	done()
	if p, err := ConsumeColdService(canceled, 27); err == nil || p != nil {
		t.Fatal("canceled pre-consumption call adopted a numeric handle")
	}
	p := &ColdServicePreparation{signing: NewColdSigningValidator()}
	if _, err := p.Inventory(ctx); err == nil {
		t.Fatal("semantic validator without native transition supplied inventory")
	}
	if _, err := p.StartRuntime(ctx, RuntimeOptions{}, nil); err == nil {
		t.Fatal("empty prepared holder started an actor")
	}
	if p.Commit(ctx) == nil {
		t.Fatal("empty prepared holder finalized a transition")
	}
	if _, err := p.Inventory(context.Background()); err == nil {
		t.Fatal("unbounded native admission accepted")
	}
}
