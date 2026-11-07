//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMappingCreationRetainsExistingFilesAndRejectsLinks(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	signature, _ := rootIdentity(root)
	check := func() error { return nil }
	name := "mapping 例.json"
	if err = createMapping(context.Background(), root, signature, name, mappingFixture, check); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(directory, name))
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if err = createMapping(context.Background(), root, signature, name, mappingFixture, check); err == nil {
		t.Fatal("existing file accepted")
	}
	got, _ := os.ReadFile(filepath.Join(directory, name))
	if string(got) != mappingFixture {
		t.Fatal("existing content changed")
	}
	outside := filepath.Join(t.TempDir(), "missing.json")
	if err = os.Symlink(outside, filepath.Join(directory, "linked.json")); err != nil {
		t.Fatal(err)
	}
	if err = createMapping(context.Background(), root, signature, "linked.json", mappingFixture, check); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err = os.Stat(outside); !os.IsNotExist(err) {
		t.Fatal("outside file created")
	}
	for _, name := range []string{"../escape.json", "sub/file.json", "wrong.yml"} {
		if err = createMapping(context.Background(), root, signature, name, mappingFixture, check); err == nil {
			t.Fatal("unsafe filename accepted")
		}
	}
}
func TestMappingCancellationReplacementAndPostWriteFailure(t *testing.T) {
	directory := t.TempDir()
	root, _ := os.OpenRoot(directory)
	defer root.Close()
	signature, _ := rootIdentity(root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := createMapping(ctx, root, signature, "cancel.json", mappingFixture, func() error { return nil }); err == nil {
		t.Fatal("cancel accepted")
	}
	if _, err := os.Stat(filepath.Join(directory, "cancel.json")); !os.IsNotExist(err) {
		t.Fatal("cancel wrote file")
	}
	if err := createMapping(context.Background(), root, "wrong", "identity.json", mappingFixture, func() error { return nil }); err == nil {
		t.Fatal("changed root accepted")
	}
	calls := 0
	err := createMapping(context.Background(), root, signature, "post.json", mappingFixture, func() error {
		calls++
		if calls > 1 {
			return errors.New("disconnected")
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "may remain") {
		t.Fatal("post-write uncertainty missing")
	}
	if got, _ := os.ReadFile(filepath.Join(directory, "post.json")); string(got) != mappingFixture {
		t.Fatal("uncertain new file not retained")
	}
	moved := directory + "-moved"
	if err = os.Rename(directory, moved); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(moved)
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	err = createMapping(context.Background(), root, signature, "replacement.json", mappingFixture, func() error {
		current, _ := identity(directory)
		if current != signature {
			return errors.New("replaced")
		}
		return nil
	})
	if err == nil {
		t.Fatal("replaced source path accepted")
	}
	if _, err = os.Stat(filepath.Join(moved, "replacement.json")); !os.IsNotExist(err) {
		t.Fatal("old root written despite failed check")
	}
}
func TestMappingConcurrentCreationAndReadonlyRoot(t *testing.T) {
	directory := t.TempDir()
	root, _ := os.OpenRoot(directory)
	defer root.Close()
	signature, _ := rootIdentity(root)
	var group sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			results <- createMapping(context.Background(), root, signature, "one.json", mappingFixture, func() error { return nil })
		}()
	}
	group.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successes %d", success)
	}
	if os.Geteuid() != 0 {
		os.Chmod(directory, 0500)
		defer os.Chmod(directory, 0700)
		if err := createMapping(context.Background(), root, signature, "readonly.json", mappingFixture, func() error { return nil }); err == nil {
			t.Fatal("read-only root accepted")
		}
	}
}
