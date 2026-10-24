//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/docker/docker-credential-helpers/credentials"
)

func TestOwnedWindowsVaultRoundTripAndForeignRecordRefusal(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_CREDENTIALS") != "1" {
		t.Skip("Explicit opt-in for uniquely named synthetic Windows vault records")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal("Cannot allocate a fixture ID")
	}
	server := "orbwin-qualification-" + hex.EncodeToString(id[:]) + ".invalid"
	key, _ := keyFor(server)
	v := windowsVault{}
	if _, err := v.get(key); !errors.Is(err, missing) {
		t.Fatal("Fixture target already exists or could not be checked")
	}
	defer func() {
		if err := v.remove(key); err != nil {
			t.Error("Synthetic credential cleanup failed")
		}
	}()
	h := helper{v}
	if err := h.Add(&credentials.Credentials{ServerURL: server, Username: "orbwin-proof", Secret: "disposable-synthetic-secret"}); err != nil {
		var status syscall.Errno
		if errors.As(v.put(entry{key: key, username: "orbwin-proof", secret: "disposable-synthetic-secret", label: owner}), &status) {
			t.Logf("Windows credential status: %d", uint32(status))
		}
		t.Fatal("Windows vault store failed:", err)
	}
	user, secret, err := h.Get(server)
	if err != nil || user != "orbwin-proof" || secret != "disposable-synthetic-secret" {
		t.Fatal("Windows vault round-trip failed")
	}
	if h.Delete(server) != nil || h.Delete(server) != nil {
		t.Fatal("Windows erase was not idempotent")
	}
	foreign := entry{key: key, username: "foreign-proof", secret: "foreign-disposable-synthetic-secret", label: "OrbWin qualification foreign-owner fixture"}
	if v.put(foreign) != nil {
		t.Fatal("Cannot create the synthetic foreign-owner fixture")
	}
	if h.Add(&credentials.Credentials{ServerURL: server, Username: "overwrite", Secret: "synthetic"}) == nil || h.Delete(server) == nil {
		t.Fatal("Foreign-owner fixture was changed")
	}
	if _, _, err := h.Get(server); err == nil {
		t.Fatal("Foreign-owner fixture was disclosed")
	}
	actual, err := v.get(key)
	if err != nil || actual != foreign {
		t.Fatal("Foreign-owner fixture was not preserved")
	}
}
