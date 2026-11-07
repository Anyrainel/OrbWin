//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImageImportSourceHandleDeniesConcurrentWriteAndDelete(t *testing.T) {
	entries, _ := imageTarTestFixture(t, false)
	data := imageTarTestBytes(t, entries)
	path := filepath.Join(t.TempDir(), "locked 例.tar")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	file, _, err := imageImportOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	if writer, err := os.OpenFile(path, os.O_WRONLY, 0600); err == nil {
		writer.Close()
		file.Close()
		t.Fatal("source accepted a concurrent writable handle")
	}
	if err := os.Rename(path, path+".moved"); err == nil {
		file.Close()
		t.Fatal("source accepted a concurrent rename")
	}
	if err := os.Remove(path); err == nil {
		file.Close()
		t.Fatal("source accepted a concurrent deletion")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("source lock leaked:", err)
	}
	writer.Close()
}
