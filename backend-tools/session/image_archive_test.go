package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type imageTarTestEntry struct {
	name string
	data []byte
	kind byte
}

func imageTarTestBytes(t *testing.T, entries []imageTarTestEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		if err := writer.WriteHeader(&tar.Header{Name: entry.name, Size: int64(len(entry.data)), Mode: 0600, Typeflag: entry.kind}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func imageTarTestFixture(t *testing.T, compressed bool) ([]imageTarTestEntry, string) {
	t.Helper()
	layer := imageTarTestBytes(t, []imageTarTestEntry{{"probe", []byte("fixture payload"), tar.TypeReg}})
	diff := sha256.Sum256(layer)
	config, err := json.Marshal(map[string]any{"os": "linux", "architecture": "amd64", "rootfs": map[string]any{"type": "layers", "diff_ids": []string{"sha256:" + hex.EncodeToString(diff[:])}}})
	if err != nil {
		t.Fatal(err)
	}
	id := sha256.Sum256(config)
	if compressed {
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		if _, err := writer.Write(layer); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		layer = buffer.Bytes()
	}
	manifest, err := json.Marshal([]imageArchiveManifest{{Config: "config.json", RepoTags: []string{"example/probe:fixture"}, Layers: []string{"layer.tar"}}})
	if err != nil {
		t.Fatal(err)
	}
	return []imageTarTestEntry{{"manifest.json", manifest, tar.TypeReg}, {"config.json", config, tar.TypeReg}, {"layer.tar", layer, tar.TypeReg}}, "sha256:" + hex.EncodeToString(id[:])
}

func TestImageArchiveVerifiesConfigAndExpandedLayerIdentity(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		entries, id := imageTarTestFixture(t, compressed)
		data := imageTarTestBytes(t, entries)
		index, err := inspectImageArchive(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if len(index.Images) != 1 || index.Images[0].ID != id || index.Images[0].Architecture != "amd64" || index.Images[0].LayerCount != 1 || index.Images[0].Tags[0] != "example/probe:fixture" {
			t.Fatalf("unexpected index: %+v", index)
		}
		entries[2].data = []byte("altered layer bytes")
		changed := imageTarTestBytes(t, entries)
		if _, err := inspectImageArchive(bytes.NewReader(changed), int64(len(changed))); err == nil {
			t.Fatal("changed rootfs was accepted")
		}
	}
}

func TestImageArchiveBindsContainerdIndexAndManifestIDs(t *testing.T) {
	for _, scenario := range []string{"index", "manifest", "missing config", "wrong size", "unrelated root"} {
		t.Run(scenario, func(t *testing.T) {
			entries, configID := imageTarTestFixture(t, false)
			config := entries[1].data
			entries = append(entries, imageTarTestEntry{"blobs/sha256/" + configID[7:], config, tar.TypeReg})
			descriptor := map[string]any{"digest": configID, "size": len(config)}
			if scenario == "missing config" {
				descriptor["digest"] = "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
			}
			if scenario == "wrong size" {
				descriptor["size"] = len(config) + 1
			}
			manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.manifest.v1+json", "config": descriptor, "layers": []any{}})
			manifestHash := sha256.Sum256(manifest)
			manifestID := "sha256:" + hex.EncodeToString(manifestHash[:])
			entries = append(entries, imageTarTestEntry{"blobs/sha256/" + manifestID[7:], manifest, tar.TypeReg})
			root, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json", "manifests": []any{
				map[string]any{"digest": manifestID, "size": len(manifest)},
				map[string]any{"digest": "sha256:" + string(bytes.Repeat([]byte{'1'}, 64)), "size": 123}, // not locally available platform
			}})
			rootHash := sha256.Sum256(root)
			expected := "sha256:" + hex.EncodeToString(rootHash[:])
			entries = append(entries, imageTarTestEntry{"blobs/sha256/" + expected[7:], root, tar.TypeReg})
			if scenario == "manifest" {
				expected = manifestID
			}
			if scenario == "unrelated root" {
				expected = "sha256:" + string(bytes.Repeat([]byte{'2'}, 64))
			}
			data := imageTarTestBytes(t, entries)
			index, err := inspectImageArchive(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			err = verifyImageArchiveTarget(bytes.NewReader(data), index, expected)
			if (err == nil) != (scenario == "index" || scenario == "manifest") {
				t.Fatalf("unexpected target verification: %v", err)
			}
		})
	}
}
func TestImageArchiveRejectsAmbiguousOuterInputs(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "a/../manifest.json", "a\\manifest.json", "C:/file", "./manifest.json", "."} {
		if _, err := imageArchivePath(name); err == nil {
			t.Fatalf("accepted outer path %q", name)
		}
	}
	entries, _ := imageTarTestFixture(t, false)
	for _, extra := range []imageTarTestEntry{
		{"config.json", []byte("duplicate"), tar.TypeReg},
		{"link", nil, tar.TypeSymlink},
		{"blobs/sha256/0000000000000000000000000000000000000000000000000000000000000000", []byte("wrong digest"), tar.TypeReg},
	} {
		data := imageTarTestBytes(t, append(append([]imageTarTestEntry{}, entries...), extra))
		if _, err := inspectImageArchive(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatalf("accepted %+v", extra)
		}
	}
	data := imageTarTestBytes(t, entries)
	for _, invalid := range [][]byte{data[:len(data)/2], append(append([]byte{}, data...), 1), []byte("not a tar")} {
		if _, err := inspectImageArchive(bytes.NewReader(invalid), int64(len(invalid))); err == nil {
			t.Fatal("invalid/truncated/suffixed archive accepted")
		}
	}
	if _, err := inspectImageArchive(bytes.NewReader(data), imageArchiveLimit+1); err == nil {
		t.Fatal("oversized archive accepted")
	}
	if _, err := inspectImageArchive(bytes.NewReader(data), int64(len(data)+512)); err == nil {
		t.Fatal("short underlying archive accepted with a larger declared size")
	}
}

type imageExportBrokenReader struct{ source io.Reader }

func (r imageExportBrokenReader) Read(data []byte) (int, error) {
	n, _ := r.source.Read(data)
	return n, errors.New("injected export disconnect")
}
func TestImageExportPublishesOnlyVerifiedNewDestination(t *testing.T) {
	entries, id := imageTarTestFixture(t, false)
	data := imageTarTestBytes(t, entries)
	for _, scenario := range []string{"success", "disconnect", "wrong image", "reference changed", "late collision"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			destination := filepath.Join(dir, "Unicode 例 image.tar")
			var reader io.Reader = bytes.NewReader(data)
			expected := id
			verify := func() error { return nil }
			switch scenario {
			case "disconnect":
				reader = imageExportBrokenReader{source: reader}
			case "wrong image":
				expected = "sha256:" + string(bytes.Repeat([]byte{'0'}, 64))
			case "reference changed":
				verify = func() error { return errors.New("source reference changed") }
			case "late collision":
				verify = func() error { return os.WriteFile(destination, []byte("retained original"), 0600) }
			}
			_, err := publishImageExport(reader, destination, expected, verify)
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected result: %v", err)
			}
			listed, listErr := os.ReadDir(dir)
			if listErr != nil {
				t.Fatal(listErr)
			}
			for _, entry := range listed {
				if entry.Name() != filepath.Base(destination) {
					t.Fatalf("ordinary failure left staging %s", entry.Name())
				}
			}
			actual, readErr := os.ReadFile(destination)
			switch scenario {
			case "success":
				if readErr != nil || !bytes.Equal(actual, data) {
					t.Fatalf("published archive differs: %v", readErr)
				}
			case "late collision":
				if readErr != nil || string(actual) != "retained original" {
					t.Fatal("late existing file was replaced")
				}
			default:
				if !os.IsNotExist(readErr) {
					t.Fatal("failed export published a destination")
				}
			}
		})
	}
}
