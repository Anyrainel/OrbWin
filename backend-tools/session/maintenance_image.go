package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

const maintenanceImageID = "sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30"
const maintenanceConfigID = "sha256:f439434f9e6739ae2f80313f260d2084bb89feaf7220a06a5398e6b90797e2e3"
const maintenanceDiffID = "sha256:41058dd4c122275500b8ee145e3ff1692ea26c7c568f55c00caf2878e7c61db0"
const maintenanceLayerBlob = "759efa4719c9a44e7f2561a03bff2434dd66e99c916d2e4d50b055bedf554144"
const maintenanceArchiveLimit = 8 << 20

// The image is immutable untagged cache, never an ephemeral job resource.
// A load may complete after owner cancellation; no image rollback/removal is safe
// or necessary. All callers verify the owned daemon before reaching this helper.
type maintenanceClient interface {
	ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error)
	ImageLoad(context.Context, io.Reader, ...client.ImageLoadOption) (client.ImageLoadResult, error)
}

func checkMaintenanceArchiveFile(file *os.File) (imageArchiveIndex, error) {
	var empty imageArchiveIndex
	info, err := file.Stat()
	if err != nil {
		return empty, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maintenanceArchiveLimit {
		return empty, errors.New("maintenance image must be a bounded regular archive")
	}
	return checkMaintenanceArchive(file, info.Size())
}
func checkMaintenanceArchive(source io.ReaderAt, size int64) (imageArchiveIndex, error) {
	var empty imageArchiveIndex
	if size <= 0 || size > maintenanceArchiveLimit {
		return empty, errors.New("maintenance archive exceeds its byte budget")
	}
	// Pin the compressed input before the general archive reader inflates any
	// layer. A small, untrusted build candidate must not expand to its general
	// 64-GiB user-archive budget before we discover it is not this tool image.
	reader := tar.NewReader(io.NewSectionReader(source, 0, size))
	layerFound, manifestFound := false, false
	for count := 0; ; count++ {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return empty, err
		}
		if count >= 256 || header.Size < 0 || header.Size > maintenanceArchiveLimit {
			return empty, errors.New("maintenance tar exceeds its entry budget")
		}
		if header.Name == "manifest.json" {
			if manifestFound || header.Size > 4096 {
				return empty, errors.New("maintenance manifest is duplicated or oversized")
			}
			var manifest []imageArchiveManifest
			data, err := io.ReadAll(io.LimitReader(reader, 4097))
			if err != nil {
				return empty, err
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return empty, err
			}
			if len(manifest) != 1 || manifest[0].Config != "blobs/sha256/"+maintenanceConfigID[len("sha256:"):] || len(manifest[0].RepoTags) != 0 || !reflect.DeepEqual(manifest[0].Layers, []string{"blobs/sha256/" + maintenanceLayerBlob}) {
				return empty, errors.New("maintenance payload manifest differs from its pins")
			}
			manifestFound = true
		}
		if header.Name == "blobs/sha256/"+maintenanceLayerBlob {
			if layerFound {
				return empty, errors.New("duplicate maintenance layer")
			}
			hash := sha256.New()
			if _, err := io.Copy(hash, reader); err != nil {
				return empty, err
			}
			if hex.EncodeToString(hash.Sum(nil)) != maintenanceLayerBlob {
				return empty, errors.New("maintenance compressed layer differs")
			}
			layerFound = true
		}
	}
	if !manifestFound || !layerFound {
		return empty, errors.New("maintenance pinned payloads are absent")
	}
	index, err := inspectImageArchive(source, size)
	if err != nil {
		return empty, err
	}
	if len(index.Images) != 1 || index.Images[0].ID != maintenanceConfigID || index.Images[0].OS != "linux" || index.Images[0].Architecture != "amd64" || len(index.Images[0].Tags) != 0 || len(index.Manifest) != 1 || len(index.Manifest[0].RepoTags) != 0 {
		return empty, errors.New("maintenance archive image identity/platform/tags differ")
	}
	if err := verifyImageArchiveTarget(source, index, maintenanceImageID); err != nil {
		return empty, err
	}
	data, err := imageArchiveMetadata(source, index.Entries, "index.json", 1<<20)
	if err != nil {
		return empty, err
	}
	var top struct {
		SchemaVersion int `json:"schemaVersion"`
		Manifests     []struct {
			Digest      string            `json:"digest"`
			Annotations map[string]string `json:"annotations"`
		} `json:"manifests"`
		Annotations map[string]string `json:"annotations"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return empty, err
	}
	if top.SchemaVersion != 2 || len(top.Manifests) != 1 || top.Manifests[0].Digest != maintenanceImageID || len(top.Annotations) != 0 || len(top.Manifests[0].Annotations) != 0 {
		return empty, errors.New("maintenance OCI root has unexpected names or identities")
	}
	return index, nil
}

func inspectMaintenanceArchive(output io.Writer, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	index, err := checkMaintenanceArchiveFile(file)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"schema": 1, "sha256": index.SHA256, "bytes": index.Bytes, "root": maintenanceImageID, "config": maintenanceConfigID, "diffIds": []string{maintenanceDiffID}, "os": "linux", "architecture": "amd64", "tagFree": true})
}

func verifyMaintenanceImage(ctx context.Context, cli maintenanceClient) error {
	result, err := cli.ImageInspect(ctx, maintenanceImageID)
	if err != nil {
		return err
	}
	image := result.InspectResponse
	if image.ID != maintenanceImageID || image.Os != "linux" || image.Architecture != "amd64" || !reflect.DeepEqual(image.RootFS.Layers, []string{maintenanceDiffID}) {
		return errors.New("maintenance image identity/platform/rootfs differ")
	}
	return nil
}

func ensureMaintenanceImage(ctx context.Context, cli maintenanceClient) error {
	err := verifyMaintenanceImage(ctx, cli)
	if err == nil {
		return nil
	}
	if !cerrdefs.IsNotFound(err) {
		return err
	}
	file, err := os.Open(os.Getenv("ORBWIN_MAINTENANCE_IMAGE"))
	if err != nil {
		return errors.New("bundled offline maintenance image is unavailable; repair this installation")
	}
	defer file.Close()
	expected := os.Getenv("ORBWIN_MAINTENANCE_IMAGE_SHA256")
	if !digestValid(expected) {
		return errors.New("verified maintenance archive checksum is unavailable")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maintenanceArchiveLimit {
		return errors.New("bundled maintenance image is not a bounded regular file")
	}
	// Freeze a bounded private copy: validation and Load must consume the same
	// bytes even if the application bundle is replaced concurrently.
	payload, err := io.ReadAll(io.LimitReader(file, maintenanceArchiveLimit+1))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	if len(payload) > maintenanceArchiveLimit || hex.EncodeToString(digest[:]) != expected {
		return errors.New("bundled maintenance image checksum differs; no load sent")
	}
	_, err = checkMaintenanceArchive(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	loaded, err := cli.ImageLoad(ctx, bytes.NewReader(payload), client.ImageLoadWithQuiet(true))
	if err != nil {
		return err
	}
	limited := &io.LimitedReader{R: loaded, N: (64 << 10) + 1}
	decoder := json.NewDecoder(limited)
	for {
		var event struct {
			Error       string `json:"error"`
			ErrorDetail *struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		readErr := decoder.Decode(&event)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			err = readErr
			break
		}
		if event.Error != "" || event.ErrorDetail != nil && event.ErrorDetail.Message != "" {
			err = errors.New("maintenance image load was rejected; inspect current state before retrying")
			break
		}
	}
	if limited.N <= 0 {
		err = errors.Join(err, errors.New("maintenance load response exceeds its budget"))
	}
	err = errors.Join(err, loaded.Close())
	if err != nil {
		return err
	}
	return verifyMaintenanceImage(ctx, cli)
}
