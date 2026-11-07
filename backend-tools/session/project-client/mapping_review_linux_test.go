//go:build linux

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestMappingReviewExactBytesAndSemantics(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	signature, _ := rootIdentity(root)
	bytes := []byte(mappingFixture)
	digest := sha256.Sum256(bytes)
	review := mappingReview{File: "mapping.json", SHA256: hex.EncodeToString(digest[:]), Contents: mappingFixture}
	path := filepath.Join(directory, review.File)
	if err = os.WriteFile(path, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	check := func() error { return nil }
	if err = inspectProjectMapping(context.Background(), root, signature, review, check); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		bytes  []byte
		review mappingReview
	}{
		{"changed bytes", append(append([]byte{}, bytes...), '\n'), review},
		{"wrong hash", bytes, mappingReview{File: review.File, SHA256: strings.Repeat("f", 64), Contents: review.Contents}},
		{"wrong semantic", bytes, mappingReview{File: review.File, SHA256: review.SHA256, Contents: strings.Replace(mappingFixture, "restored", "foreign", -1)}},
		{"oversize", make([]byte, (256<<10)+1), review},
		{"unsupported schema", []byte(`{"services":{"web":{"command":"bad"}},"volumes":{}}`), review},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, tc.bytes, 0600); err != nil {
				t.Fatal(err)
			}
			if err := inspectProjectMapping(context.Background(), root, signature, tc.review, check); err == nil {
				t.Fatal("accepted unsafe mapping")
			}
		})
	}
	for _, file := range []string{"../outside.json", "nested/map.json", "/tmp/map.json"} {
		bad := review
		bad.File = file
		if inspectProjectMapping(context.Background(), root, signature, bad, check) == nil {
			t.Fatal(file)
		}
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if inspectProjectMapping(context.Background(), root, signature, review, check) == nil {
		t.Fatal("missing accepted")
	}
}
func TestMappingReviewRejectsSymlinkFifoAndDirectory(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	signature, _ := rootIdentity(root)
	digest := sha256.Sum256([]byte(mappingFixture))
	review := mappingReview{File: "mapping.json", SHA256: hex.EncodeToString(digest[:]), Contents: mappingFixture}
	path := filepath.Join(directory, review.File)
	outside := filepath.Join(t.TempDir(), "outside.json")
	os.WriteFile(outside, []byte(mappingFixture), 0600)
	if err = os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if inspectProjectMapping(context.Background(), root, signature, review, func() error { return nil }) == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(path)
	if err = syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if inspectProjectMapping(context.Background(), root, signature, review, func() error { return nil }) == nil {
		t.Fatal("fifo accepted")
	}
	os.Remove(path)
	os.Mkdir(path, 0700)
	if inspectProjectMapping(context.Background(), root, signature, review, func() error { return nil }) == nil {
		t.Fatal("directory accepted")
	}
	if got, _ := os.ReadFile(outside); string(got) != mappingFixture {
		t.Fatal("outside bytes changed")
	}
}
func TestMappingReviewReplacementCancellationAndRootBoundary(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	signature, _ := rootIdentity(root)
	digest := sha256.Sum256([]byte(mappingFixture))
	review := mappingReview{File: "mapping.json", SHA256: hex.EncodeToString(digest[:]), Contents: mappingFixture}
	path := filepath.Join(directory, review.File)
	os.WriteFile(path, []byte(mappingFixture), 0600)
	if inspectProjectMapping(context.Background(), root, "wrong", review, func() error { return nil }) == nil {
		t.Fatal("wrong root accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(inspectProjectMapping(ctx, root, signature, review, func() error { return nil }), context.Canceled) {
		t.Fatal("cancel ignored")
	}
	calls := 0
	check := func() error {
		calls++
		if calls == 2 {
			os.Rename(path, path+".old")
			os.WriteFile(path, []byte(mappingFixture), 0600)
		}
		return nil
	}
	if inspectProjectMapping(context.Background(), root, signature, review, check) == nil {
		t.Fatal("identical-byte replacement accepted")
	}
	ctx, cancel = context.WithCancel(context.Background())
	calls = 0
	if !errors.Is(inspectProjectMapping(ctx, root, signature, review, func() error {
		calls++
		if calls == 2 {
			cancel()
		}
		return nil
	}), context.Canceled) {
		t.Fatal("late cancel ignored")
	}
	// Pinning the old root never reads a replacement directory's same-name file.
	renamed := directory + "-renamed"
	if err = os.Rename(directory, renamed); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(renamed)
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("foreign"), 0600)
	if err = inspectProjectMapping(context.Background(), root, signature, review, func() error {
		actual, e := identity(directory)
		if e != nil {
			return e
		}
		if actual != signature {
			return errors.New("root replaced")
		}
		return nil
	}); err == nil {
		t.Fatal("replaced source root accepted")
	}
	if got, _ := os.ReadFile(path); string(got) != "foreign" {
		t.Fatal("replacement modified")
	}
}
