package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const gateDirectory = `C:\owned\home\AppData\Local\rancher-desktop-orbwin`
const gateExecutable = `C:\OrbWin\runtime\backend\rdd.exe`

func observationBytes(change func(map[string]any)) []byte {
	value := map[string]any{"schema": 1, "generation": "rdd-service-identity-v1", "instance": "rancher-desktop-orbwin", "directory": gateDirectory, "executable": gateExecutable, "pid": 123, "state": "stopped", "reason": "processExited"}
	if change != nil {
		change(value)
	}
	raw, _ := json.Marshal(value)
	return raw
}

func gateFromBytes(ctx context.Context, raw []byte) error {
	return managementStartGate(ctx, gateDirectory, gateExecutable, func(context.Context) ([]byte, error) { return raw, nil })
}

func TestStrictServiceObservationAdmitsOnlyQualifiedStopped(t *testing.T) {
	if err := gateFromBytes(context.Background(), observationBytes(nil)); !errors.Is(err, errManagementUnavailable) {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"false", func(v map[string]any) { v["state"] = false }},
		{"running", func(v map[string]any) { v["state"] = "running"; v["reason"] = "identityVerified" }},
		{"unknown", func(v map[string]any) { v["state"] = "unknown"; v["reason"] = "processInspectionFailed" }},
		{"unsupported schema", func(v map[string]any) { v["schema"] = 2 }},
		{"unsupported generation", func(v map[string]any) { v["generation"] = "legacy" }},
		{"foreign instance", func(v map[string]any) { v["instance"] = "rancher-desktop-2" }},
		{"foreign home", func(v map[string]any) { v["directory"] = `C:\foreign\home\AppData\Local\rancher-desktop-orbwin` }},
		{"foreign executable", func(v map[string]any) { v["executable"] = `C:\foreign\rdd.exe` }},
		{"missing PID", func(v map[string]any) { delete(v, "pid") }},
		{"negative PID", func(v map[string]any) { v["pid"] = -1 }},
		{"large PID", func(v map[string]any) { v["pid"] = uint64(1) << 32 }},
		{"fractional PID", func(v map[string]any) { v["pid"] = 1.5 }},
		{"string PID", func(v map[string]any) { v["pid"] = "123" }},
		{"wrong stopped reason", func(v map[string]any) { v["reason"] = "ownershipMarkerUnavailable" }},
		{"unknown field", func(v map[string]any) { v["extra"] = "value" }},
		{"case variant field", func(v map[string]any) { v["State"] = "running" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := gateFromBytes(context.Background(), observationBytes(test.change))
			if err == nil || errors.Is(err, errManagementUnavailable) || !errors.Is(err, errColdStartUnqualified) {
				t.Fatal(err)
			}
		})
	}
	raw := string(observationBytes(nil))
	for _, invalid := range []string{"", "null", "false", "{", "[]", raw + raw, raw[:len(raw)-1] + `,"state":"running"}`, strings.Repeat(" ", 16<<10) + raw} {
		if err := gateFromBytes(context.Background(), []byte(invalid)); err == nil || errors.Is(err, errManagementUnavailable) {
			t.Fatal(invalid, err)
		}
	}
}

func TestLegacyPermissionAndCanceledProbeNeverAdmitStartup(t *testing.T) {
	for _, fault := range []error{errors.New("legacy unknown --output flag"), errors.New("permission denied"), errors.New("process inspection failed")} {
		err := managementStartGate(context.Background(), gateDirectory, gateExecutable, func(context.Context) ([]byte, error) { return nil, fault })
		if !errors.Is(err, fault) || !errors.Is(err, errColdStartUnqualified) || errors.Is(err, errManagementUnavailable) || !strings.Contains(err.Error(), "desktop") {
			t.Fatal(err)
		}
	}
	for _, before := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if before {
			cancel()
		}
		calls := 0
		err := managementStartGate(ctx, gateDirectory, gateExecutable, func(context.Context) ([]byte, error) { calls++; cancel(); return observationBytes(nil), nil })
		cancel()
		if !errors.Is(err, context.Canceled) || errors.Is(err, errManagementUnavailable) || (before && calls != 0) {
			t.Fatal(err, calls)
		}
	}
}

func TestFailedSettingsNeverStartWithoutTypedStoppedObservation(t *testing.T) {
	for _, fault := range []error{errors.New("authentication failure"), errors.New("malformed settings JSON"), errors.New("API identity failure")} {
		for _, state := range []string{"running", "unknown"} {
			calls := 0
			err := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) { return nil, fault }, func(context.Context, []string) error { calls++; return nil }, func(context.Context) error { t.Fatal("readiness probe after refused fallback"); return nil }, func(ctx context.Context) error {
				return gateFromBytes(ctx, observationBytes(func(v map[string]any) { v["state"] = state }))
			})
			if !errors.Is(err, fault) || calls != 0 {
				t.Fatal(state, err, calls)
			}
		}
	}
	for _, gate := range []func(context.Context) error{nil, func(context.Context) error { return nil }, func(context.Context) error { return errColdStartUnqualified }} {
		calls := 0
		err := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) { return nil, errors.New("offline") }, func(context.Context, []string) error { calls++; return nil }, func(context.Context) error { return nil }, gate)
		if err == nil || calls != 0 {
			t.Fatal(err, calls)
		}
	}
}

func TestWarmLegacySettingsNeverRequireMachineStatus(t *testing.T) {
	calls := 0
	err := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) { return manualSpec(), nil }, func(context.Context, []string) error { calls++; return nil }, func(context.Context) error { return nil }, func(context.Context) error {
		t.Fatal("machine status required on warm legacy path")
		return errColdStartUnqualified
	})
	if err != nil || calls != 2 {
		t.Fatal(err, calls)
	}
}

func TestActualTypedStoppedGateStartsOnceAndRechecksPolicy(t *testing.T) {
	reads, starts, sets, waits := 0, 0, 0, 0
	err := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) {
		reads++
		if reads == 1 {
			return nil, errors.New("offline")
		}
		return manualSpec(), nil
	}, func(_ context.Context, args []string) error {
		switch args[0] {
		case "service":
			starts++
		case "set":
			sets++
		case "ctl":
			waits++
		}
		return nil
	}, func(context.Context) error { return nil }, func(ctx context.Context) error { return gateFromBytes(ctx, observationBytes(nil)) }, confirmedRunning)
	if err != nil || reads != 2 || starts != 1 || sets != 1 || waits != 1 {
		t.Fatal(err, reads, starts, sets, waits)
	}
}
