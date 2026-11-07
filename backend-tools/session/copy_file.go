package main

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/moby/moby/client"
)

const fileCopyBudget int64 = 4 << 30

type fileCopyResult struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func copyFileArchive(archive io.Reader, destination string) (result fileCopyResult, resultErr error) {
	if !regexp.MustCompile(`^[a-zA-Z]:[\\/]`).MatchString(destination) || !filepath.IsAbs(destination) {
		return result, errors.New("select an absolute local Windows drive destination")
	}
	if _, err := os.Lstat(destination); err == nil {
		return result, errors.New("copy never overwrites an existing destination")
	} else if !os.IsNotExist(err) {
		return result, err
	}
	reader := tar.NewReader(archive)
	header, err := reader.Next()
	if err != nil {
		return result, err
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
		return result, errors.New("copy requires a regular file; directories, links and special objects are not extracted")
	}
	if header.Size < 0 || header.Size > fileCopyBudget {
		return result, errors.New("file copy is limited to 4 GiB")
	}
	available, err := hostFreeSpace(filepath.Dir(destination))
	if err != nil {
		return result, err
	}
	if available < uint64(header.Size)+65536 {
		return result, errors.New("not enough available space for the selected file")
	}
	token := make([]byte, 16)
	if _, err = rand.Read(token); err != nil {
		return result, err
	}
	partial := destination + ".orbwin-partial-" + hex.EncodeToString(token)
	file, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return result, err
	}
	defer func() {
		file.Close()
		if err := os.Remove(partial); err != nil && !os.IsNotExist(err) {
			resultErr = errors.Join(resultErr, fmt.Errorf("partial copy retained at %s: %w", partial, err))
		}
	}()
	fmt.Fprintln(os.Stderr, "Copying through exclusive partial file:", partial)
	hash := sha256.New()
	count, err := io.CopyN(io.MultiWriter(file, hash), reader, header.Size)
	if err != nil {
		return result, err
	}
	if count != header.Size {
		return result, io.ErrUnexpectedEOF
	}
	if _, err = reader.Next(); err != io.EOF {
		if err == nil {
			err = errors.New("file archive unexpectedly contains multiple entries")
		}
		return result, err
	}
	if err = file.Sync(); err != nil {
		return result, err
	}
	if err = file.Close(); err != nil {
		return result, err
	}
	// NTFS hard-link publication is exclusive even if another process creates
	// the destination after preflight. No extraction and no replacement rename.
	if err = os.Link(partial, destination); err != nil {
		return result, fmt.Errorf("publish copy without replacement: %w", err)
	}
	return fileCopyResult{Path: destination, Bytes: count, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}
func copyContainerFile(output io.Writer, daemon, target, source, destination string) error {
	if err := fileArguments(daemon, target, source, "read"); err != nil {
		return err
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != daemon || info.Info.OSType != "linux" {
		return errors.New("copy endpoint does not match the recorded owned Linux daemon")
	}
	archive, err := cli.CopyFromContainer(ctx, target, client.CopyFromContainerOptions{SourcePath: source})
	if err != nil {
		return err
	}
	defer archive.Content.Close()
	result, err := copyFileArchive(archive.Content, destination)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}
