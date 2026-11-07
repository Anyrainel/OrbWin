package main

// Read-only Docker archives: no guest executable, extraction, host path or
// helper container. Close/cancel releases the HTTP stream, including on limits.
// A live directory is not a consistent snapshot. Bound bytes as well as entries
// because reaching the next tar header can otherwise read an enormous file.
import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/moby/moby/client"
)

const archiveBudget = 8 << 20
const previewBudget = 256 << 10

type fileEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Size int64  `json:"size"`
	Mode int64  `json:"mode"`
	Link string `json:"link,omitempty"`
}
type fileResult struct {
	Entries    []fileEntry     `json:"entries"`
	Complete   bool            `json:"complete"`
	Text       *string         `json:"text,omitempty"`
	SHA256     string          `json:"sha256,omitempty"`
	EditSource *fileEditSource `json:"editSource,omitempty"`
}

func fileArguments(daemon, container, source, action string) error {
	if daemon == "" || len(daemon) > 256 || strings.ContainsAny(daemon, "\x00\r\n") || len(container) != 64 {
		return errors.New("recorded daemon and full container ID required")
	}
	if _, err := hex.DecodeString(container); err != nil {
		return errors.New("invalid container ID")
	}
	if len(source) > 2048 || !strings.HasPrefix(source, "/") || strings.ContainsRune(source, 0) || strings.Contains(source, "\\") || !utf8.ValidString(source) {
		return errors.New("use an absolute Linux container path (maximum 2048 bytes)")
	}
	if action != "list" && action != "read" {
		return errors.New("unsupported file action")
	}
	return nil
}
func archiveName(name string) (string, error) {
	clean := path.Clean(name)
	if !utf8.ValidString(name) || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsRune(name, 0) {
		return "", errors.New("archive contains an unsafe entry name")
	}
	return clean, nil
}
func readArchive(input io.Reader, action string) (fileResult, error) {
	result := fileResult{Entries: []fileEntry{}}
	limited := &io.LimitedReader{R: input, N: archiveBudget}
	reader := tar.NewReader(limited)
	root, err := reader.Next()
	if err != nil {
		return result, err
	}
	absoluteRoot := root.Name == "/" && root.Typeflag == tar.TypeDir
	rootName := root.Name
	if absoluteRoot {
		rootName = "."
	}
	prefix, err := archiveName(rootName)
	if err != nil {
		return result, err
	}
	if action == "read" {
		if root.Typeflag != tar.TypeReg && root.Typeflag != tar.TypeRegA {
			return result, errors.New("text preview requires a regular file; symbolic links and devices are not previewed")
		}
		if root.Size < 0 || root.Size > previewBudget {
			return result, errors.New("text preview is limited to 256 KiB; use the explicitly targeted Docker CLI to copy larger files")
		}
		data, err := io.ReadAll(io.LimitReader(reader, previewBudget+1))
		if err != nil {
			return result, err
		}
		if int64(len(data)) != root.Size || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
			return result, errors.New("this file is not a complete UTF-8 text file")
		}
		text := string(data)
		result.Text = &text
		hash := sha256.Sum256(data)
		result.SHA256 = hex.EncodeToString(hash[:])
		result.Complete = true
		return result, nil
	}
	if root.Typeflag != tar.TypeDir {
		return result, errors.New("open a directory to list entries, or choose Preview text for a file")
	}
	seen := map[string]bool{}
	// Sparse tar entries may describe much more logical data than their wire
	// representation. Bound skipped payload too; a context deadline alone does
	// not interrupt CPU-only sparse expansion inside archive/tar.
	payloadBudget := int64(archiveBudget)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			result.Complete = limited.N > 0
			return result, nil
		}
		if err != nil {
			if limited.N == 0 {
				return result, nil
			}
			return result, err
		}
		entryName := header.Name
		if absoluteRoot {
			entryName = strings.TrimPrefix(entryName, "/")
		}
		name, err := archiveName(entryName)
		if err != nil {
			return result, err
		}
		if prefix != "." {
			if !strings.HasPrefix(name, prefix+"/") {
				return result, errors.New("archive entry escapes its requested directory")
			}
			name = strings.TrimPrefix(name, prefix+"/")
		}
		canSkip := header.Size <= payloadBudget
		if canSkip {
			payloadBudget -= header.Size
		}
		if name == "." || strings.Contains(name, "/") {
			if !canSkip {
				return result, nil
			}
			continue
		}
		if seen[name] {
			return result, errors.New("archive contains duplicate directory entries")
		}
		seen[name] = true
		kind := "special"
		switch header.Typeflag {
		case tar.TypeDir:
			kind = "directory"
		case tar.TypeReg, tar.TypeRegA:
			kind = "file"
		case tar.TypeSymlink:
			kind = "symlink"
		case tar.TypeLink:
			kind = "hardlink"
		}
		result.Entries = append(result.Entries, fileEntry{Name: name, Kind: kind, Size: header.Size, Mode: header.Mode, Link: header.Linkname})
		if !canSkip || len(result.Entries) >= 256 {
			return result, nil
		}
	}
}
func files(output io.Writer, daemon, container, source, action string) error {
	if err := fileArguments(daemon, container, source, action); err != nil {
		return err
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != daemon || info.Info.OSType != "linux" {
		return errors.New("file endpoint does not match the recorded owned Linux daemon")
	}
	archive, err := cli.CopyFromContainer(ctx, container, client.CopyFromContainerOptions{SourcePath: source})
	if err != nil {
		return err
	}
	defer archive.Content.Close()
	result, err := readArchive(archive.Content, action)
	if err != nil {
		return err
	}
	if action == "read" {
		result.EditSource = containerEditSource(ctx, cli, container, source)
	}
	return json.NewEncoder(output).Encode(result)
}
