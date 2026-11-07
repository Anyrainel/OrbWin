package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/distribution/reference"
	"github.com/moby/moby/client"
)

func imageExportReference(id, selected string) error {
	if !imageIDValid(id) {
		return errors.New("image export requires the full reviewed SHA-256 image ID")
	}
	if selected == id {
		return nil
	}
	if selected == "" || len(selected) > 512 || strings.ContainsAny(selected, "\x00\r\n") {
		return errors.New("select a bounded image reference or its immutable ID")
	}
	parsed, err := reference.ParseNormalizedNamed(selected)
	if err != nil {
		return err
	}
	if _, ok := parsed.(reference.NamedTagged); !ok {
		return errors.New("select an explicit image tag or the full image ID")
	}
	return nil
}

func exportImage(output io.Writer, daemon, id, selected, destination string) (resultErr error) {
	if err := validateImageExport(id, selected, destination); err != nil {
		return err
	}
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	defer cancel()
	defer func() { resultErr = errors.Join(resultErr, cli.Close()) }()
	return exportClientImage(ctx, cli, output, id, selected, destination, nil)
}

// The caller owns the client/context. A source adapter supplies an additional
// identity guard; archive validation/publication are identical for owned and
// selected-source exports. This core cannot load, tag or remove an image.
func validateImageExport(id, selected, destination string) error {
	if err := imageExportReference(id, selected); err != nil {
		return err
	}
	if !filepath.IsAbs(destination) || len(destination) < 3 || destination[1] != ':' || strings.ContainsAny(destination[2:], "\x00\r\n:") || !strings.EqualFold(filepath.Ext(destination), ".tar") {
		return errors.New("select a new .tar file on a local Windows drive")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err == nil {
			return errors.New("image export never replaces an existing file")
		}
		return err
	}
	return nil
}

func exportClientImage(ctx context.Context, cli *client.Client, output io.Writer, id, selected, destination string, guard func() error) (resultErr error) {
	if err := validateImageExport(id, selected, destination); err != nil {
		return err
	}
	verify := func() error {
		if guard != nil {
			if err := guard(); err != nil {
				return err
			}
		}
		inspected, err := cli.ImageInspect(ctx, selected)
		if err != nil {
			return err
		}
		if inspected.ID != id || inspected.Os != "linux" {
			return errors.New("selected image reference changed or is not a Linux image; export refused")
		}
		return nil
	}
	if err := verify(); err != nil {
		return err
	}
	stream, err := cli.ImageSave(ctx, []string{selected})
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, stream.Close()) }()
	index, err := publishImageExport(stream, destination, id, verify)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{
		"complete": true, "path": destination, "image": id, "reference": selected,
		"sha256": index.SHA256, "bytes": index.Bytes, "images": index.Images,
	})
}

// Publication never replaces the destination. Interrupted processes can leave
// only a random named partial, never an apparently completed destination. The
// caller retains source/reader ownership; this function owns its staging handle.
func publishImageExport(source io.Reader, destination, expected string, verify func() error) (index imageArchiveIndex, resultErr error) {
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err == nil {
			return index, errors.New("image export destination already exists")
		}
		return index, err
	}
	token, err := transferToken()
	if err != nil {
		return index, err
	}
	partial := destination + ".orbwin-partial-" + token
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return index, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
		if err := os.Remove(partial); err != nil && !os.IsNotExist(err) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	fmt.Fprintln(os.Stderr, "Saving image through exclusive partial file:", partial)
	n, err := io.Copy(&freeSpaceWriter{file: file, directory: filepath.Dir(destination)}, io.LimitReader(source, imageArchiveLimit+1))
	if err != nil {
		return index, err
	}
	if n > imageArchiveLimit {
		return index, errors.New("image archive exceeds its 64-GiB budget")
	}
	if err := file.Sync(); err != nil {
		return index, err
	}
	before, err := file.Stat()
	if err != nil {
		return index, err
	}
	index, err = inspectImageArchive(file, n)
	if err != nil {
		return index, err
	}
	if err := verifyImageArchiveTarget(file, index, expected); err != nil {
		return index, fmt.Errorf("exported archive differs from reviewed image identity; source retained: %w", err)
	}
	if verify != nil {
		if err := verify(); err != nil {
			return index, err
		}
	}
	after, err := os.Lstat(partial)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return index, errors.Join(errors.New("image export partial changed before publication"), err)
	}
	if err := os.Link(partial, destination); err != nil {
		return index, fmt.Errorf("publish image archive without replacement: %w", err)
	}
	return index, nil
}
