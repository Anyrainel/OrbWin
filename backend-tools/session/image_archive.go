package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const imageArchiveLimit int64 = 64 << 30
const imageArchiveEntryLimit = 100000
const imageArchiveManifestLimit int64 = 1 << 20
const imageArchiveConfigLimit int64 = 4 << 20

type imageArchiveEntry struct {
	Offset, Size int64
	SHA256       string
}
type imageArchiveManifest struct {
	Config   string
	RepoTags []string
	Layers   []string
}
type imageArchiveImage struct {
	ID           string   `json:"id"`
	Tags         []string `json:"tags"`
	OS           string   `json:"os"`
	Architecture string   `json:"architecture"`
	Variant      string   `json:"variant,omitempty"`
	LayerCount   int      `json:"layerCount"`
}
type imageArchiveIndex struct {
	SHA256        string                       `json:"sha256"`
	Bytes         int64                        `json:"bytes"`
	Images        []imageArchiveImage          `json:"images"`
	Entries       map[string]imageArchiveEntry `json:"-"`
	Manifest      []imageArchiveManifest       `json:"-"`
	ExpandedBytes int64                        `json:"-"`
}

// The containerd image store can expose an OCI index/manifest ID, while the
// classic store exposes a config ID. Bind every Docker-manifest config to the
// reviewed root through verified, present descriptors. Missing foreign-platform
// descriptors may be normal in a partial local image; they cannot prove reachability.
func verifyImageArchiveTarget(source io.ReaderAt, index imageArchiveIndex, expected string) error {
	if !imageIDValid(expected) || len(index.Images) == 0 {
		return errors.New("image archive has no valid reviewed target")
	}
	if len(index.Images) == 1 && index.Images[0].ID == expected {
		return nil
	}
	type descriptor struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	}
	rootName := "blobs/sha256/" + strings.TrimPrefix(expected, "sha256:")
	root, ok := index.Entries[rootName]
	if !ok {
		return errors.New("reviewed image ID has no verified config or OCI root descriptor in the archive")
	}
	queue := []descriptor{{Digest: expected, Size: root.Size}}
	visited := map[string]bool{}
	reachable := map[string]bool{}
	var metadataBytes int64
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if !imageIDValid(current.Digest) || current.Size < 1 || current.Size > imageArchiveConfigLimit {
			return errors.New("unsupported OCI image descriptor")
		}
		if visited[current.Digest] {
			continue
		}
		if len(visited) >= 4096 {
			return errors.New("OCI image descriptor graph exceeds its node budget")
		}
		visited[current.Digest] = true
		name := "blobs/sha256/" + strings.TrimPrefix(current.Digest, "sha256:")
		entry, present := index.Entries[name]
		if !present {
			continue
		}
		if entry.Size != current.Size {
			return errors.New("OCI image descriptor size differs from its verified blob")
		}
		metadataBytes += entry.Size
		if metadataBytes > 16<<20 {
			return errors.New("OCI image descriptor graph exceeds its metadata budget")
		}
		data, err := imageArchiveMetadata(source, index.Entries, name, imageArchiveConfigLimit)
		if err != nil {
			return err
		}
		var node struct {
			SchemaVersion int          `json:"schemaVersion"`
			MediaType     string       `json:"mediaType"`
			Config        *descriptor  `json:"config"`
			Manifests     []descriptor `json:"manifests"`
		}
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}
		if node.SchemaVersion != 2 {
			return errors.New("unsupported OCI image schema")
		}
		switch node.MediaType {
		case "application/vnd.oci.image.index.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json":
			if node.Config != nil || len(node.Manifests) < 1 || len(node.Manifests) > 4096 || len(queue)+len(node.Manifests) > 8192 {
				return errors.New("invalid or oversized OCI image index")
			}
			queue = append(queue, node.Manifests...)
		case "application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json":
			if node.Config == nil || len(node.Manifests) != 0 || !imageIDValid(node.Config.Digest) {
				return errors.New("invalid OCI image config descriptor")
			}
			configName := "blobs/sha256/" + strings.TrimPrefix(node.Config.Digest, "sha256:")
			if config, present := index.Entries[configName]; present {
				if config.Size != node.Config.Size {
					return errors.New("OCI image config size differs")
				}
				reachable[node.Config.Digest] = true
			}
		default:
			return errors.New("unsupported OCI image descriptor media type")
		}
	}
	for _, image := range index.Images {
		if !reachable[image.ID] {
			return errors.New("exported config is not reachable from the reviewed OCI image ID")
		}
	}
	return nil
}

func imageArchivePath(name string) (string, error) {
	if name == "" || len(name) > 4096 || strings.ContainsAny(name, "\\\x00\r\n:") || strings.HasPrefix(name, "/") {
		return "", errors.New("image archive has an unsupported outer path")
	}
	clean := strings.TrimSuffix(name, "/")
	if clean == "." || clean != path.Clean(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("image archive outer path escapes or is ambiguous")
	}
	return clean, nil
}
func imageIDValid(id string) bool {
	return strings.HasPrefix(id, "sha256:") && digestValid(strings.TrimPrefix(id, "sha256:"))
}

// A ReaderAt index, never filesystem extraction. Every payload is read before
// acceptance. Bounded metadata stays in memory; layers are hashed as streams.
// The caller owns an immutable/locked file for the index's lifetime.
func inspectImageArchive(source io.ReaderAt, size int64) (imageArchiveIndex, error) {
	index := imageArchiveIndex{Bytes: size, Entries: map[string]imageArchiveEntry{}}
	if size <= 0 || size > imageArchiveLimit {
		return index, errors.New("plain image archive exceeds its 64-GiB budget or is empty")
	}
	whole := sha256.New()
	if copied, err := io.Copy(whole, io.NewSectionReader(source, 0, size)); err != nil || copied != size {
		return index, errors.Join(errors.New("image archive changed or is shorter than its declared size"), err)
	}
	index.SHA256 = hex.EncodeToString(whole.Sum(nil))
	reader := io.NewSectionReader(source, 0, size)
	archive := tar.NewReader(reader)
	seen := map[string]bool{}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return index, fmt.Errorf("invalid image tar: %w", err)
		}
		name, err := imageArchivePath(header.Name)
		if err != nil {
			return index, err
		}
		if seen[name] || len(seen) >= imageArchiveEntryLimit || header.Size < 0 || header.Size > imageArchiveLimit || len(header.PAXRecords) != 0 {
			return index, errors.New("duplicate, oversized or unsupported image archive entry")
		}
		seen[name] = true
		if header.Typeflag == tar.TypeDir {
			if header.Size != 0 {
				return index, errors.New("image archive directory has a payload")
			}
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return index, errors.New("image archive links and special/sparse entries are unsupported")
		}
		offset, err := reader.Seek(0, io.SeekCurrent)
		if err != nil || offset > size-header.Size {
			return index, errors.New("truncated image archive payload")
		}
		digest := sha256.New()
		copied, err := io.Copy(digest, archive)
		if err != nil || copied != header.Size {
			return index, errors.Join(errors.New("incomplete image archive payload"), err)
		}
		sha := hex.EncodeToString(digest.Sum(nil))
		if strings.HasPrefix(name, "blobs/") && (name != "blobs/sha256/"+sha) {
			return index, errors.New("image archive content-addressed blob digest differs")
		}
		index.Entries[name] = imageArchiveEntry{Offset: offset, Size: header.Size, SHA256: sha}
	}
	// Reject concatenated archives or hidden alternate instructions after tar EOF.
	trailing, err := reader.Seek(0, io.SeekCurrent)
	if err != nil || size-trailing > 1<<20 {
		return index, errors.New("image archive has an oversized tar suffix")
	}
	buffer := make([]byte, 32768)
	for {
		n, err := reader.Read(buffer)
		if !bytes.Equal(buffer[:n], make([]byte, n)) {
			return index, errors.New("image archive has a nonzero tar suffix")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return index, err
		}
	}
	manifestBytes, err := imageArchiveMetadata(source, index.Entries, "manifest.json", imageArchiveManifestLimit)
	if err != nil {
		return index, err
	}
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index.Manifest); err != nil {
		return index, fmt.Errorf("unsupported Docker image manifest: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF || len(index.Manifest) < 1 || len(index.Manifest) > 128 {
		return index, errors.New("image archive manifest has an invalid image count or suffix")
	}
	var expanded int64
	verifiedLayers := map[string]string{}
	imageIDs := map[string]bool{}
	for _, manifest := range index.Manifest {
		configBytes, err := imageArchiveMetadata(source, index.Entries, manifest.Config, imageArchiveConfigLimit)
		if err != nil {
			return index, err
		}
		configDigest := sha256.Sum256(configBytes)
		id := "sha256:" + hex.EncodeToString(configDigest[:])
		if imageIDs[id] || len(manifest.Layers) > 4096 || len(manifest.RepoTags) > 1024 {
			return index, errors.New("duplicate image or excessive layers/tags in image archive")
		}
		imageIDs[id] = true
		var config struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Variant      string `json:"variant"`
			RootFS       struct {
				Type    string   `json:"type"`
				DiffIDs []string `json:"diff_ids"`
			} `json:"rootfs"`
		}
		if err := json.Unmarshal(configBytes, &config); err != nil {
			return index, err
		}
		if config.OS != "linux" || config.Architecture == "" || len(config.Architecture) > 64 || len(config.Variant) > 64 || config.RootFS.Type != "layers" || len(config.RootFS.DiffIDs) != len(manifest.Layers) {
			return index, errors.New("unsupported image platform/rootfs metadata")
		}
		for layerIndex, layer := range manifest.Layers {
			entry, ok := index.Entries[layer]
			if !ok || !imageIDValid(config.RootFS.DiffIDs[layerIndex]) {
				return index, errors.New("image manifest layer is absent or has an invalid diff ID")
			}
			actual, ok := verifiedLayers[layer]
			if !ok {
				actual, err = imageArchiveLayerDigest(source, entry, &expanded)
				if err != nil {
					return index, err
				}
				verifiedLayers[layer] = actual
			}
			if actual != config.RootFS.DiffIDs[layerIndex] {
				return index, errors.New("image layer content differs from its config rootfs diff ID")
			}
		}
		index.Images = append(index.Images, imageArchiveImage{ID: id, Tags: manifest.RepoTags, OS: config.OS, Architecture: config.Architecture, Variant: config.Variant, LayerCount: len(manifest.Layers)})
	}
	index.ExpandedBytes = expanded
	return index, nil
}
func imageArchiveMetadata(source io.ReaderAt, entries map[string]imageArchiveEntry, name string, limit int64) ([]byte, error) {
	entry, ok := entries[name]
	if !ok || entry.Size < 1 || entry.Size > limit {
		return nil, fmt.Errorf("image archive metadata %q is absent or exceeds its budget", name)
	}
	return io.ReadAll(io.NewSectionReader(source, entry.Offset, entry.Size))
}
func imageArchiveLayerDigest(source io.ReaderAt, entry imageArchiveEntry, expanded *int64) (string, error) {
	section := io.NewSectionReader(source, entry.Offset, entry.Size)
	var first [2]byte
	_, _ = section.ReadAt(first[:], 0)
	var payload io.Reader = section
	var compressed *gzip.Reader
	if first == [2]byte{0x1f, 0x8b} {
		var err error
		compressed, err = gzip.NewReader(section)
		if err != nil {
			return "", err
		}
		defer compressed.Close()
		payload = compressed
	}
	digest := sha256.New()
	n, err := io.Copy(digest, io.LimitReader(payload, imageArchiveLimit-*expanded+1))
	*expanded += n
	if err != nil || *expanded > imageArchiveLimit {
		return "", errors.Join(errors.New("image layer is invalid or exceeds its expanded byte budget"), err)
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}
