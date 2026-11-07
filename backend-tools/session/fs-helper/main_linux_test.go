//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEditPreservesBackupAndRejectsStaleOrLinkedFiles(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "config.txt")
	old := "original\n"
	os.WriteFile(file, []byte(old), 0640)
	request := editRequest{Path: "/config.txt", Expected: hash([]byte(old)), Text: "changed\n"}
	result, err := edit(root, request, nil)
	if err != nil || !result.Committed {
		t.Fatal(result, err)
	}
	backup, err := os.ReadFile(filepath.Join(root, result.Backup))
	if err != nil || string(backup) != old {
		t.Fatal("original version not retained", err)
	}
	current, _ := os.ReadFile(file)
	if string(current) != "changed\n" {
		t.Fatal("new bytes not committed")
	}
	if _, err = edit(root, request, nil); err == nil {
		t.Fatal("stale edit accepted")
	}
	request.Expected = hash(current)
	if _, err = edit(root, request, func(phase string) {
		if phase == "prepared" {
			os.WriteFile(file, []byte("external"), 0640)
		}
	}); err == nil {
		t.Fatal("external edit was overwritten")
	}
	current, _ = os.ReadFile(file)
	if string(current) != "external" {
		t.Fatal("external bytes not retained")
	}
	os.Symlink("/etc/passwd", filepath.Join(root, "link"))
	request.Path = "/link"
	if _, err = edit(root, request, nil); err == nil {
		t.Fatal("symbolic link followed")
	}
}
