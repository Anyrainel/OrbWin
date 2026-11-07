package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Re-emit a fixed-path Docker manifest and exactly its validated payloads. Never
// forward unreviewed original tags or alternate OCI/repositories instructions.
func writeImageImportArchive(ctx context.Context, output io.Writer, source io.ReaderAt, index imageArchiveIndex, staging []string) error {
	if len(staging) != len(index.Images) || len(index.Manifest) != len(index.Images) {
		return errors.New("image import staging plan differs")
	}
	manifest := make([]imageArchiveManifest, len(index.Manifest))
	type payload struct {
		name  string
		entry imageArchiveEntry
	}
	payloads := []payload{}
	seen := map[string]bool{}
	for i, item := range index.Manifest {
		config, ok := index.Entries[item.Config]
		if !ok || "sha256:"+config.SHA256 != index.Images[i].ID {
			return errors.New("image import config entry differs")
		}
		name := config.SHA256 + ".json"
		manifest[i] = imageArchiveManifest{Config: name, RepoTags: []string{staging[i]}, Layers: []string{}}
		if !seen[name] {
			payloads = append(payloads, payload{name, config})
			seen[name] = true
		}
		for _, layer := range item.Layers {
			entry, ok := index.Entries[layer]
			if !ok {
				return errors.New("image import layer entry is absent")
			}
			name := "layers/" + entry.SHA256 + ".tar"
			manifest[i].Layers = append(manifest[i].Layers, name)
			if !seen[name] {
				payloads = append(payloads, payload{name, entry})
				seen[name] = true
			}
		}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	writer := tar.NewWriter(output)
	if err := writer.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	for _, payload := range payloads {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writer.WriteHeader(&tar.Header{Name: payload.name, Mode: 0600, Size: payload.entry.Size, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
			return err
		}
		digest := sha256.New()
		n, err := io.Copy(writer, io.TeeReader(io.NewSectionReader(source, payload.entry.Offset, payload.entry.Size), digest))
		if err != nil {
			return err
		}
		if n != payload.entry.Size || hex.EncodeToString(digest.Sum(nil)) != payload.entry.SHA256 {
			return errors.New("image archive payload changed during import; staged image outcome is unconfirmed")
		}
	}
	return writer.Close()
}

// Verify a staged image by reading Docker's own Save stream back. Hash every
// payload without extracting or spooling it. Match the exact source config and
// its ordered rootfs layer digests; ImageInspect IDs can be OCI index digests.
func verifyImageImportStream(source io.Reader, expected string, diffIDs []string) error {
	limited := &io.LimitedReader{R: source, N: imageArchiveLimit + 1}
	archive := tar.NewReader(limited)
	seen := map[string]bool{}
	digests := map[string]string{}
	diffs := map[string]string{}
	var manifest []imageArchiveManifest
	var expanded int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, err := imageArchivePath(header.Name)
		if err != nil {
			return err
		}
		if seen[name] || len(seen) >= imageArchiveEntryLimit || len(header.PAXRecords) != 0 {
			return errors.New("staged image Save has duplicate or unsupported entries")
		}
		seen[name] = true
		if header.Typeflag == tar.TypeDir && header.Size == 0 {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || header.Size < 0 || header.Size > imageArchiveLimit {
			return errors.New("staged image Save has unsupported payload types/sizes")
		}
		if name == "manifest.json" {
			if header.Size > imageArchiveManifestLimit {
				return errors.New("staged image manifest exceeds its budget")
			}
			data, err := io.ReadAll(archive)
			if err != nil {
				return err
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&manifest); err != nil {
				return err
			}
			if decoder.Decode(&struct{}{}) != io.EOF {
				return errors.New("staged image manifest has a suffix")
			}
			continue
		}
		digest := sha256.New()
		buffer := bufio.NewReader(io.TeeReader(archive, digest))
		prefix, _ := buffer.Peek(2)
		var payload io.Reader = buffer
		var compressed *gzip.Reader
		if bytes.Equal(prefix, []byte{0x1f, 0x8b}) {
			compressed, err = gzip.NewReader(buffer)
			if err != nil {
				return err
			}
			payload = compressed
		}
		diff := sha256.New()
		n, copyErr := io.Copy(diff, io.LimitReader(payload, imageArchiveLimit+(16<<20)-expanded+1))
		expanded += n
		if compressed != nil {
			copyErr = errors.Join(copyErr, compressed.Close())
		}
		if copyErr != nil || expanded > imageArchiveLimit+(16<<20) {
			return errors.Join(errors.New("staged image content exceeds its verification budget or is corrupt"), copyErr)
		}
		if _, err := io.Copy(io.Discard, buffer); err != nil {
			return err
		}
		sha := hex.EncodeToString(digest.Sum(nil))
		if strings.HasPrefix(name, "blobs/") && name != "blobs/sha256/"+sha {
			return errors.New("staged image blob checksum differs")
		}
		digests[name] = "sha256:" + sha
		diffs[name] = "sha256:" + hex.EncodeToString(diff.Sum(nil))
	}
	// Read/check the bounded suffix too; a successful prefix is not a full save.
	buffer := make([]byte, 32768)
	var suffix int64
	for {
		n, err := limited.Read(buffer)
		suffix += int64(n)
		if suffix > 1<<20 || !bytes.Equal(buffer[:n], make([]byte, n)) {
			return errors.New("staged image Save has an unsupported suffix")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if limited.N <= 0 {
		return errors.New("staged image Save exceeds its archive budget")
	}
	if len(manifest) != 1 || digests[manifest[0].Config] != expected || len(manifest[0].Layers) != len(diffIDs) {
		return errors.New("staged image config does not match the reviewed archive")
	}
	for i, layer := range manifest[0].Layers {
		if diffs[layer] != diffIDs[i] {
			return fmt.Errorf("staged image layer %d differs from the reviewed rootfs", i)
		}
	}
	return nil
}
