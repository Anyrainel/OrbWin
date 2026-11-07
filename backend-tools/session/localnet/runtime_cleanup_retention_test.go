package localnet

import (
	"context"
	"errors"
	"testing"
	"time"
)

type terminalGenerationOwner struct {
	calls   int
	failure error
}

func (o *terminalGenerationOwner) Close(context.Context) error { o.calls++; return o.failure }
func (o *terminalGenerationOwner) Wait(context.Context) error  { return nil }

func TestRuntimeRetainsTerminalGenerationAfterActualShutdownWorkerReturnedError(t *testing.T) {
	owner := &terminalGenerationOwner{failure: errors.New("owned helper removal unconfirmed")}
	g := &serviceGeneration{shutdown: newGenerationShutdown(owner, nil, nil)}
	r := &ServiceRuntime{generation: g}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if r.stopGeneration(ctx) == nil || r.generation != g {
		t.Fatal("terminal worker error discarded actual generation")
	}
	select {
	case <-g.shutdown.done:
	default:
		t.Fatal("fixture did not reach actual terminal worker result")
	}
	if r.stopGeneration(ctx) == nil || r.generation != g || owner.calls != 1 {
		t.Fatal("repeat erased failure/reference or redispatched Close")
	}
}
