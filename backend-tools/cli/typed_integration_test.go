package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestColdStartRequiresFreshRunningIdentityBeforeSettingsOrSet(t *testing.T) {
	for _, state := range []string{"stopped", "unknown", "foreign", "malformed", "legacy", "running"} {
		t.Run(state, func(t *testing.T) {
			settings, management, probes := 0, 0, 0
			observe := func(ctx context.Context) error {
				probes++
				raw := observationBytes(func(v map[string]any) {
					v["state"] = "running"
					v["reason"] = "identityVerified"
					if state == "stopped" {
						v["state"] = "stopped"
						v["reason"] = "processExited"
					}
					if state == "unknown" {
						v["state"] = "unknown"
						v["reason"] = "ownershipMarkerUnavailable"
					}
					if state == "foreign" {
						v["directory"] = "foreign"
					}
				})
				if state == "malformed" {
					raw = []byte("{")
				}
				if state == "legacy" {
					return errors.New("unsupported output")
				}
				return managementRunningGate(ctx, gateDirectory, gateExecutable, func(context.Context) ([]byte, error) { return raw, nil })
			}
			err := desiredState(context.Background(), "running=true", func(context.Context) (map[string]any, error) {
				settings++
				if settings == 1 {
					return nil, errors.New("offline")
				}
				return manualSpec(), nil
			}, func(context.Context, []string) error { management++; return nil }, func(context.Context) error { return nil }, confirmedStopped, observe)
			if state == "running" {
				if err != nil || settings != 2 || management != 3 || probes != 1 {
					t.Fatal(err, settings, management, probes)
				}
			} else if err == nil || settings != 1 || management != 1 || probes != 1 {
				t.Fatal(err, settings, management, probes)
			}
		})
	}
}
func TestCancelInRunningProbePreventsAllFurtherReadsAndSet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads, calls := 0, 0
	err := desiredState(ctx, "running=true", func(context.Context) (map[string]any, error) { reads++; return nil, errors.New("offline") }, func(context.Context, []string) error { calls++; return nil }, func(context.Context) error { t.Fatal("readiness"); return nil }, confirmedStopped, func(context.Context) error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) || reads != 1 || calls != 1 {
		t.Fatal(err, reads, calls)
	}
}
func TestReadStatusIsUsefulAndSanitizedWithoutInferringEngineStopped(t *testing.T) {
	fault := errors.New("endpoint unavailable with private diagnostic")
	for _, state := range []string{"running", "stopped", "unknown", "invalid"} {
		report, err := managementStatus(context.Background(), func(context.Context) (map[string]any, error) { return nil, fault }, func(context.Context) (managementObservation, error) {
			if state == "invalid" {
				return managementObservation{}, errColdStartUnqualified
			}
			return managementObservation{State: state, Reason: "fixtureReason", Directory: "private-home", Executable: "private-image", PID: 123}, nil
		})
		if report["engine"] != "unavailable" {
			t.Fatal(report)
		}
		if len(report) != 3 || strings.Contains(strings.Join([]string{report["management"].(string), report["reason"].(string)}, " "), "private") {
			t.Fatal(report)
		}
		if (state == "unknown" || state == "invalid") != (err != nil) {
			t.Fatal(state, err)
		}
	}
	_, err := managementStatus(context.Background(), func(context.Context) (map[string]any, error) { return map[string]any{"ID": "owned"}, nil }, func(context.Context) (managementObservation, error) {
		t.Fatal("warm legacy engine requires new schema")
		return managementObservation{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestConfigReadsClassifyOnlyAuthoritativeStoppedAndPreserveApiFailures(t *testing.T) {
	fault := errors.New("auth or malformed JSON")
	for _, state := range []string{"stopped", "running", "unknown", "invalid"} {
		_, err := classifiedSettings(context.Background(), func(context.Context) (map[string]any, error) { return nil, fault }, func(context.Context) (managementObservation, error) {
			if state == "invalid" {
				return managementObservation{}, errColdStartUnqualified
			}
			return managementObservation{State: state}, nil
		})
		if !errors.Is(err, fault) || (state == "stopped") != errors.Is(err, errManagementUnavailable) {
			t.Fatal(state, err)
		}
	}
	_, err := classifiedSettings(context.Background(), func(context.Context) (map[string]any, error) { return manualSpec(), nil }, func(context.Context) (managementObservation, error) {
		t.Fatal("successful warm legacy read probed status")
		return managementObservation{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
