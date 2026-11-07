package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var errManagementUnavailable = errors.New("confirmed owned management service is stopped")
var errColdStartUnqualified = errors.New("cold startup is unsupported or could not be verified; start OrbWin from the desktop application")

type managementObservation struct {
	Schema     int    `json:"schema"`
	Generation string `json:"generation"`
	Instance   string `json:"instance"`
	Directory  string `json:"directory"`
	Executable string `json:"executable"`
	PID        int64  `json:"pid"`
	State      string `json:"state"`
	Reason     string `json:"reason"`
}

// This is a transient upstream observation gate, not lifecycle state. Only the
// exact supported-generation stopped result admits one existing rdd start.
// Legacy flags/logs, API failure text and ambiguous process evidence never do.
func decodeManagementObservation(ctx context.Context, directory, executable string, probe func(context.Context) ([]byte, error)) (managementObservation, error) {
	var status managementObservation
	if ctx.Err() != nil {
		return status, ctx.Err()
	}
	raw, err := probe(ctx)
	if ctx.Err() != nil {
		return status, ctx.Err()
	}
	if err != nil {
		return status, errors.Join(errColdStartUnqualified, err)
	}
	if len(raw) > 16<<10 {
		return status, errors.Join(errColdStartUnqualified, errors.New("management observation exceeds limit"))
	}
	// Reject duplicate fields as well as unknown fields. In particular a second
	// state/identity field cannot override an earlier contradictory observation.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return status, errors.Join(errColdStartUnqualified, errors.New("invalid management observation"))
	}
	seen := map[string]bool{}
	allowed := map[string]bool{"schema": true, "generation": true, "instance": true, "directory": true, "executable": true, "pid": true, "state": true, "reason": true}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return status, errors.Join(errColdStartUnqualified, errors.New("invalid management observation"))
		}
		name, ok := key.(string)
		if !ok || !allowed[name] || seen[name] {
			return status, errors.Join(errColdStartUnqualified, errors.New("duplicate management observation field"))
		}
		seen[name] = true
		var value json.RawMessage
		if err = decoder.Decode(&value); err != nil {
			return status, errors.Join(errColdStartUnqualified, errors.New("invalid management observation"))
		}
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&status); err != nil {
		return status, errors.Join(errColdStartUnqualified, errors.New("invalid management observation schema"))
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return status, errors.Join(errColdStartUnqualified, errors.New("multiple management observations"))
	}
	if status.Schema != 1 || status.Generation != "rdd-service-identity-v1" || status.Instance != "rancher-desktop-orbwin" || directory == "" || executable == "" || !strings.EqualFold(status.Directory, directory) || !strings.EqualFold(status.Executable, executable) || status.PID < 0 || status.PID > 1<<32-1 {
		return status, errors.Join(errColdStartUnqualified, errors.New("management observation identity mismatch"))
	}
	switch status.State {
	case "running":
		if status.PID == 0 || status.Reason != "identityVerified" {
			return status, errColdStartUnqualified
		}
	case "stopped":
		if status.PID == 0 || status.Reason != "processExited" {
			return status, errColdStartUnqualified
		}
	case "unknown":
		allowedReasons := map[string]bool{"generationUnavailable": true, "generationMalformed": true, "generationIdentityMismatch": true, "pidUnavailable": true, "pidMismatch": true, "diagnosticChangedDuringProbe": true, "invalidProcessObservation": true, "processInspectionFailed": true, "processIdentityMismatch": true, "ownershipMarkerUnavailable": true, "processChangedDuringProbe": true, "homeInspectionFailed": true, "identityNameFailed": true, "unsupportedPlatform": true}
		if !allowedReasons[status.Reason] {
			return status, errColdStartUnqualified
		}
	default:
		return status, errColdStartUnqualified
	}
	if ctx.Err() != nil {
		return status, ctx.Err()
	}
	return status, nil
}

func managementStateGate(ctx context.Context, directory, executable, want string, probe func(context.Context) ([]byte, error)) error {
	status, err := decodeManagementObservation(ctx, directory, executable, probe)
	if err != nil {
		return err
	}
	if status.State != want {
		return errors.Join(errColdStartUnqualified, errors.New("management service is not confirmed "+want))
	}
	if want == "stopped" {
		return errManagementUnavailable
	}
	return nil
}
func managementStartGate(ctx context.Context, directory, executable string, probe func(context.Context) ([]byte, error)) error {
	return managementStateGate(ctx, directory, executable, "stopped", probe)
}
func managementRunningGate(ctx context.Context, directory, executable string, probe func(context.Context) ([]byte, error)) error {
	return managementStateGate(ctx, directory, executable, "running", probe)
}

// Reads never start anything. Preserve the owning API error even when the
// independent observation can explain that management is confirmed stopped.
func classifiedSettings(ctx context.Context, read func(context.Context) (map[string]any, error), observe func(context.Context) (managementObservation, error)) (map[string]any, error) {
	spec, err := read(ctx)
	if err == nil {
		return spec, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	status, probeErr := observe(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if probeErr != nil {
		return nil, errors.Join(err, probeErr)
	}
	if status.State == "stopped" {
		return nil, errors.Join(err, errManagementUnavailable)
	}
	return nil, errors.Join(err, errors.New("management state: "+status.State+"; original settings failure remains unresolved"))
}
func managementStatus(ctx context.Context, check func(context.Context) (map[string]any, error), observe func(context.Context) (managementObservation, error)) (map[string]any, error) {
	info, engineErr := check(ctx)
	if engineErr == nil {
		return safeInfo(info), nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	status, err := observe(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return map[string]any{"engine": "unavailable", "management": "unknown", "reason": "unverified"}, err
	}
	out := map[string]any{"engine": "unavailable", "management": status.State, "reason": status.Reason}
	if status.State == "unknown" {
		return out, errColdStartUnqualified
	}
	// A stopped management process does not prove engine/workloads stopped.
	return out, nil
}

// validateServiceArgs accepts the pinned installation's persisted service shape.
// No service is implicitly created or reconfigured. The transient start override
// repeats the disabled path controller as defense in depth.
func validateServiceArgs(args []string) error {
	if len(args) > 32 {
		return errors.New("owned service arguments exceed the supported profile")
	}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		key, value, equals := strings.Cut(args[i], "=")
		if !equals {
			if i+1 >= len(args) {
				return errors.New("incomplete owned service arguments")
			}
			i++
			value = args[i]
		}
		if seen[key] {
			return errors.New("duplicate owned service argument")
		}
		seen[key] = true
		switch key {
		case "--controllers":
			if value != "*,-path-management" {
				return errors.New("owned service must disable path management")
			}
		case "--secure-port":
			port, e := strconv.Atoi(value)
			if e != nil || port < 1024 || port > 65535 {
				return errors.New("unsupported management API port")
			}
		case "-v", "--v":
			level, e := strconv.Atoi(value)
			if e != nil || level < 0 || level > 10 {
				return errors.New("unsupported management verbosity")
			}
		default:
			return errors.New("owned service arguments differ from the supported installation profile")
		}
	}
	if !seen["--controllers"] || !seen["--secure-port"] {
		return errors.New("owned service configuration is incomplete; prepare it in OrbWin")
	}
	return nil
}

// These callbacks are transient request I/O, not persisted lifecycle state. The
// only transitions are upstream service start and rdd set. No rollback/restart
// is invented if a request fails or cancellation follows acceptance.
func desiredState(ctx context.Context, value string, settings func(context.Context) (map[string]any, error), management func(context.Context, []string) error, check func(context.Context) error, stopped func(context.Context) error, running ...func(context.Context) error) error {
	spec, e := settings(ctx)
	if e != nil && value == "running=true" {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if stopped == nil {
			return errors.Join(e, errColdStartUnqualified)
		}
		gateErr := stopped(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !errors.Is(gateErr, errManagementUnavailable) {
			return errors.Join(e, errColdStartUnqualified, gateErr)
		}
		if e = management(ctx, []string{"service", "start", "--controllers=*,-path-management", "--timeout=90s"}); e != nil {
			return fmt.Errorf("owned management service could not start; existing data retained: %w", e)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("management service start dispatched; engine desired state was not requested: %w", ctx.Err())
		}
		if len(running) != 1 || running[0] == nil {
			return errors.New("management service start dispatched; supported running observation unavailable; engine desired state was not requested")
		}
		if e = running[0](ctx); e != nil {
			return fmt.Errorf("management service start dispatched; running identity remains unconfirmed; engine desired state was not requested: %w", e)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("management service start dispatched; engine desired state was not requested: %w", ctx.Err())
		}
		spec, e = settings(ctx)
	}
	if e != nil {
		return e
	}
	application, ok := spec["application"].(map[string]any)
	if !ok || application["addPath"] != "manual" {
		return errors.New("owned PATH policy is not manual; desired-state change refused")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e = management(ctx, []string{"set", value, "--timeout=210s"}); e != nil {
		return fmt.Errorf("desired-state request failed; acceptance is unconfirmed; inspect status before retrying: %w", e)
	}
	if value == "running=true" {
		if ctx.Err() != nil {
			return fmt.Errorf("desired state accepted; startup remains unconfirmed and rdd may still converge: %w", ctx.Err())
		}
		if e = management(ctx, []string{"ctl", "wait-condition", "limavms/orbwin", "Running=True", "--reason=Started", "--since=startup", "--namespace=rancher-desktop", "--timeout=180s"}); e != nil {
			return fmt.Errorf("desired state accepted; startup remains unconfirmed: %w", e)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("desired state accepted; owned Docker readiness remains unconfirmed: %w", ctx.Err())
		}
		if e = check(ctx); e != nil {
			return fmt.Errorf("desired state accepted; owned Docker readiness remains unconfirmed: %w", e)
		}
	}
	return nil
}
