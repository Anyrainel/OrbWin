//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/client"
)

func TestRancherMigrationVolumeNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" || os.Getenv("ORBWIN_QUALIFY_MIGRATION_SOURCE") != "rancher-desktop" {
		t.Skip("explicit Rancher and owned-engine disposable fixture opt-ins required")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "artifacts/qualification/p2-rancher-source-inventory-20261025.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Source migrationSourceIdentity `json:"source"`
	}
	if err := json.Unmarshal(data, &inventory); err != nil {
		t.Fatal(err)
	}
	if !migrationVolumeSourceProfileValid(inventory.Source) {
		t.Fatal("verified Rancher WSL source required")
	}
	t.Setenv("ORBWIN_MIGRATION_DOCKER", filepath.Join(root, "artifacts/runtime-tools/linux/docker"))
	t.Setenv("ORBWIN_MIGRATION_DOCKER_SHA256", "2b3470ed501ae182223ca6d7e43fd92260eeec5cbfeb502418dc5153d25531f9")
	qualifyMigrationVolumeNative(t, migrationVolumeNativeOptions{kind: "rancher-desktop", sourceName: "rancher", wsl: inventory.Source.WSL, open: func(ctx context.Context) (*client.Client, func(), error) {
		// Test-only unrestricted fixture client. The production export/recovery
		// commands use the exact job scope; this seeds/removes only nonce-owned
		// fixtures with the common test's identity/label guards.
		cli, _, closeClient, err := rancherMigrationClient(ctx, inventory.Source.WSL.Registration, inventory.Source.WSL.Machine, func(base http.RoundTripper) http.RoundTripper { return base }, func(*http.Request) error { return nil })
		return cli, closeClient, err
	}})
}
