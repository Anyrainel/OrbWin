//go:build windows

package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type destinationRace struct {
	reader      io.Reader
	destination string
	once        bool
}

func (r *destinationRace) Read(data []byte) (int, error) {
	if !r.once {
		r.once = true
		if err := os.WriteFile(r.destination, []byte("concurrent-owner"), 0600); err != nil {
			return 0, err
		}
	}
	return r.reader.Read(data)
}
func TestCopyBinaryFilePublishesOnlyCompleteExclusiveBytes(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "copy space ü.bin")
	content := strings.Repeat("\x00binary\xff", 40000)
	archive := fixtureArchive(t, archiveItem{name: "source", kind: tar.TypeReg, data: content})
	result, err := copyFileArchive(bytes.NewReader(archive), destination)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != content {
		t.Fatal("copied bytes changed", err)
	}
	hash := sha256.Sum256([]byte(content))
	if result.Bytes != int64(len(content)) || result.SHA256 != hex.EncodeToString(hash[:]) {
		t.Fatal("copy metadata differs")
	}
	if _, err = copyFileArchive(bytes.NewReader(archive), destination); err == nil {
		t.Fatal("existing destination overwritten")
	}
	contents, _ := os.ReadFile(destination)
	if string(contents) != content {
		t.Fatal("existing bytes changed")
	}
	for name, input := range map[string]io.Reader{
		"short": bytes.NewReader(archive[:len(archive)/2]),
		"link":  bytes.NewReader(fixtureArchive(t, archiveItem{name: "link", kind: tar.TypeSymlink})),
		"race":  &destinationRace{reader: bytes.NewReader(archive), destination: filepath.Join(directory, "race.bin")},
	} {
		destination := filepath.Join(directory, name+".bin")
		if _, err := copyFileArchive(input, destination); err == nil {
			t.Fatalf("unsafe copy %s succeeded", name)
		}
		if name == "race" {
			data, _ := os.ReadFile(destination)
			if string(data) != "concurrent-owner" {
				t.Fatal("racing destination overwritten")
			}
		} else if _, err := os.Stat(destination); !os.IsNotExist(err) {
			t.Fatalf("failed %s copy published final output", name)
		}
	}
	partials, _ := filepath.Glob(filepath.Join(directory, "*.orbwin-partial-*"))
	if len(partials) != 0 {
		t.Fatal("normal error retained partials")
	}
}
