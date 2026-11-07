package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func manualSpec() map[string]any {
	return map[string]any{"application": map[string]any{"addPath": "manual"}}
}

func TestCanceledColdServiceStartDoesNotReadOrSetAfterward(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads, calls := 0, 0
	err := desiredState(ctx, "running=true", func(context.Context) (map[string]any, error) {
		reads++
		return nil, errors.New("offline")
	}, func(context.Context, []string) error {
		calls++
		cancel()
		return nil
	}, func(context.Context) error { t.Fatal("Docker read after cancellation"); return nil }, confirmedStopped, confirmedRunning)
	if !errors.Is(err, context.Canceled) || reads != 1 || calls != 1 || !strings.Contains(err.Error(), "desired state was not requested") {
		t.Fatal(err, reads, calls)
	}
}

func TestCanceledReadinessWaitDoesNotProbeOrRollback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := desiredState(ctx, "running=true", func(context.Context) (map[string]any, error) { return manualSpec(), nil }, func(context.Context, []string) error {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil
	}, func(context.Context) error { t.Fatal("Docker read after canceled wait"); return nil }, confirmedStopped, confirmedRunning)
	if !errors.Is(err, context.Canceled) || calls != 2 || !strings.Contains(err.Error(), "readiness remains unconfirmed") {
		t.Fatal(err, calls)
	}
}

func TestFailedSetRetainsAcceptanceUncertaintyForEverySetting(t *testing.T) {
	for _, value := range []string{"running=true", "running=false", "kubernetes.enabled=true", "kubernetes.enabled=false"} {
		calls := 0
		fault := errors.New("client exited after dispatch")
		err := desiredState(context.Background(), value, func(context.Context) (map[string]any, error) { return manualSpec(), nil }, func(context.Context, []string) error { calls++; return fault }, func(context.Context) error { t.Fatal("readiness after failed set"); return nil }, confirmedStopped, confirmedRunning)
		if !errors.Is(err, fault) || calls != 1 || !strings.Contains(err.Error(), "acceptance is unconfirmed") {
			t.Fatal(value, err, calls)
		}
	}
}
func TestServiceProfile(t *testing.T) {
	for _, v := range [][]string{{"--secure-port", "6600", "--controllers", "*,-path-management", "-v", "2"}, {"--secure-port=6600", "--controllers=*,-path-management"}} {
		if e := validateServiceArgs(v); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range [][]string{nil, {"--secure-port", "6600"}, {"--secure-port", "6600", "--controllers", "*"}, {"--secure-port", "6600", "--controllers", "*,-path-management", "--controllers", "*"}, {"--secure-port", "80", "--controllers", "*,-path-management"}, {"--secure-port", "6600", "--controllers", "*,-path-management", "--unknown", "secret"}, {"--secure-port", "6600", "--controllers"}} {
		if validateServiceArgs(v) == nil {
			t.Fatal(v)
		}
	}
}
func TestColdStartDelegatesSameServiceThenGuardsDesiredState(t *testing.T) {
	reads := 0
	calls := [][]string{}
	checks := 0
	e := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) {
		reads++
		if reads == 1 {
			return nil, errors.New("offline")
		}
		return manualSpec(), nil
	}, func(_ context.Context, args []string) error { calls = append(calls, args); return nil }, func(context.Context) error { checks++; return nil }, confirmedStopped, confirmedRunning)
	if e != nil || reads != 2 || checks != 1 {
		t.Fatal(e, reads, checks)
	}
	if len(calls) != 3 || !reflect.DeepEqual(calls[0], []string{"service", "start", "--controllers=*,-path-management", "--timeout=90s"}) || calls[1][0] != "set" || calls[2][0] != "ctl" {
		t.Fatal(calls)
	}
}
func TestWarmStartDoesNotRestartService(t *testing.T) {
	calls := [][]string{}
	e := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) { return manualSpec(), nil }, func(_ context.Context, args []string) error { calls = append(calls, args); return nil }, func(context.Context) error { return nil }, confirmedStopped, confirmedRunning)
	if e != nil || len(calls) != 2 || calls[0][0] != "set" {
		t.Fatal(e, calls)
	}
}
func TestCanceledRequestDoesNotDispatchMutation(t *testing.T) {
	for _, offline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		e := desiredState(ctx, "running=true", func(context.Context) (map[string]any, error) {
			if offline {
				return nil, errors.New("offline")
			}
			return manualSpec(), nil
		}, func(context.Context, []string) error { calls++; return nil }, func(context.Context) error { return nil }, confirmedStopped, confirmedRunning)
		if !errors.Is(e, context.Canceled) || calls != 0 {
			t.Fatal(offline, e, calls)
		}
	}
}
func TestOfflineStopAndConfigDoNotStartService(t *testing.T) {
	for _, value := range []string{"running=false", "kubernetes.enabled=true", "kubernetes.enabled=false"} {
		calls := 0
		e := desiredState(context.Background(), value, func(context.Context) (map[string]any, error) { return nil, errors.New("offline") }, func(context.Context, []string) error { calls++; return nil }, func(context.Context) error { return nil }, confirmedStopped, confirmedRunning)
		if e == nil || calls != 0 {
			t.Fatal(value, e, calls)
		}
	}
}
func TestColdStartFailureAndUnsafePolicyDoNotSetDesiredState(t *testing.T) {
	for _, fault := range []string{"service", "policy", "reread"} {
		reads := 0
		calls := 0
		e := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) {
			reads++
			if reads == 1 || fault == "reread" {
				return nil, errors.New("offline")
			}
			return map[string]any{"application": map[string]any{"addPath": "front"}}, nil
		}, func(context.Context, []string) error {
			calls++
			if fault == "service" {
				return errors.New("service failed")
			}
			return nil
		}, func(context.Context) error { return nil }, confirmedStopped, confirmedRunning)
		if e == nil || calls != 1 {
			t.Fatal(fault, e, calls)
		}
	}
}
func TestCancellationAfterAcceptedSetNeverRestartsOrRollsBack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	e := desiredState(ctx, "running=true", func(context.Context) (map[string]any, error) { return manualSpec(), nil }, func(context.Context, []string) error { calls++; cancel(); return nil }, func(context.Context) error { t.Fatal("identity read after cancellation"); return nil }, confirmedStopped, confirmedRunning)
	if !errors.Is(e, context.Canceled) || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestFailedDesiredStateOrReadinessDoesNotReportSuccess(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		calls := 0
		want := errors.New("fault")
		e := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) { return manualSpec(), nil }, func(context.Context, []string) error {
			calls++
			if calls == failAt {
				return want
			}
			return nil
		}, func(context.Context) error { calls++; return want }, confirmedStopped, confirmedRunning)
		if !errors.Is(e, want) || calls != failAt {
			t.Fatal(failAt, e, calls)
		}
	}
}

// Transaction tests above inject an independently confirmed stopped fixture.
func confirmedStopped(context.Context) error { return errManagementUnavailable }

func confirmedRunning(context.Context) error { return nil }
