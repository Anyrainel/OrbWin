//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestVolumeAuditDetectsAttributesOmittedByArchiveAPIs(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "data")
	if err := os.WriteFile(file, []byte("retained"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("data", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if count, err := checkVolume(root); err != nil || count != 3 {
		t.Fatalf("ordinary tree refused: %d %v", count, err)
	}
	if err := unix.Setxattr(file, "user.orbwin-proof", []byte("must-not-drop"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := checkVolume(root); err == nil {
		t.Fatal("attribute-bearing tree was accepted")
	}
	if err := unix.Removexattr(file, "user.orbwin-proof"); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := checkVolume(root); err == nil {
		t.Fatal("special object was accepted")
	}
}
