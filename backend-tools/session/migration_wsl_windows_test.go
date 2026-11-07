//go:build windows

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"orbwin.local/session/internal/wsltrust"
)

func TestMigrationWSLRejectsUnreviewedIdentityBeforeExecution(t *testing.T) {
	for _, item := range []struct{ registration, machine string }{{"Ubuntu", ""}, {"--help", ""}, {"../foreign", ""}, {"00000000-0000-4000-8000-000000000001", "bad-machine"}, {"00000000-0000-4000-8000-000000000001", strings.Repeat("A", 32)}} {
		if _, err := newRancherMigrationSession(context.Background(), item.registration, item.machine); err == nil {
			t.Fatal("unreviewed WSL identity accepted")
		}
	}
}
func TestMigrationWSLRejectsChangedBundleBeforeStartingDistro(t *testing.T) {
	registrations, err := wsltrust.Registrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(registrations) == 0 {
		t.Skip("no ordinary WSL registration to exercise pre-execution asset validation")
	}
	asset := filepath.Join(t.TempDir(), "docker")
	if err := os.WriteFile(asset, make([]byte, 128), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORBWIN_MIGRATION_DOCKER", asset)
	t.Setenv("ORBWIN_MIGRATION_DOCKER_SHA256", strings.Repeat("0", 64))
	if _, err := newRancherMigrationSession(context.Background(), registrations[0].ID, ""); err == nil {
		t.Fatal("changed/unverified bundle accepted")
	}
}
