package main

// The portable format never extracts onto Windows. It holds a bounded manifest
// followed by concatenated regular-file bytes; Linux path/metadata interpretation
// happens only in a fresh isolated engine volume. Validate the entire package
// before any destination write. File offsets and hashes bind every payload byte.
import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const volumeMagic = "ORBWIN-VOLUME-1\n"
const volumePayloadLimit int64 = 64 << 30
const volumeManifestLimit int64 = 32 << 20
const volumeEntryLimit = 100000

type volumeEntry struct {
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`
	Link     string    `json:"link,omitempty"`
	Mode     int64     `json:"mode"`
	UID      int       `json:"uid"`
	GID      int       `json:"gid"`
	Modified time.Time `json:"modified"`
	Offset   int64     `json:"offset"`
	Size     int64     `json:"size"`
	SHA256   string    `json:"sha256,omitempty"`
}
type volumeManifest struct {
	Version int           `json:"version"`
	Engine  string        `json:"engine"`
	Volume  string        `json:"volume"`
	Created time.Time     `json:"created"`
	Bytes   int64         `json:"bytes"`
	SHA256  string        `json:"sha256"`
	Entries []volumeEntry `json:"entries"`
}

// Identify the complete validated dataset, including metadata and source, rather
// than only concatenated file bytes. This does not authenticate a package author.
func volumeManifestIdentity(manifest volumeManifest) (string, error) {
	if err := manifest.validate(); err != nil {
		return "", err
	}
	if !migrationText(manifest.Engine, 256) {
		return "", errors.New("invalid source engine in manifest provenance")
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = io.WriteString(hash, "orbwin-volume-manifest-v1\n")
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func portablePath(value string, root bool) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\\") && !strings.HasPrefix(value, "/") && path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../") && (root || value != ".")
}
func digestValid(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && value == strings.ToLower(value)
}

func (manifest volumeManifest) validate() error {
	if manifest.Version != 1 || manifest.Engine == "" || len(manifest.Engine) > 256 || resourceFileArguments("volume", manifest.Volume) != nil || manifest.Bytes < 0 || manifest.Bytes > volumePayloadLimit || !digestValid(manifest.SHA256) || len(manifest.Entries) == 0 || len(manifest.Entries) > volumeEntryLimit {
		return errors.New("invalid or unsupported portable-volume manifest")
	}
	entries := map[string]volumeEntry{}
	var offset int64
	for _, entry := range manifest.Entries {
		if !portablePath(entry.Path, true) || entry.Mode < 0 || entry.Mode > 07777 || entry.UID < 0 || entry.GID < 0 || entry.Offset != offset || entry.Size < 0 || entry.Size > manifest.Bytes-offset {
			return fmt.Errorf("invalid entry metadata: %q", entry.Path)
		}
		if _, exists := entries[entry.Path]; exists {
			return fmt.Errorf("duplicate entry: %q", entry.Path)
		}
		switch entry.Kind {
		case "file":
			if entry.Link != "" || !digestValid(entry.SHA256) {
				return errors.New("invalid regular-file version")
			}
			offset += entry.Size
		case "directory":
			if entry.Size != 0 || entry.Link != "" || entry.SHA256 != "" {
				return errors.New("invalid directory entry")
			}
		case "symlink":
			if entry.Mode != 0777 || entry.Size != 0 || entry.SHA256 != "" || entry.Link == "" || len(entry.Link) > 4096 || !utf8.ValidString(entry.Link) || strings.ContainsAny(entry.Link, "\x00\\") || strings.HasPrefix(entry.Link, "/") || !portablePath(path.Clean(path.Join(path.Dir(entry.Path), entry.Link)), true) {
				return fmt.Errorf("escaping or unsupported symbolic link: %q", entry.Path)
			}
		case "hardlink":
			if entry.Size != 0 || entry.SHA256 != "" || !portablePath(entry.Link, false) {
				return fmt.Errorf("invalid hard link: %q", entry.Path)
			}
		default:
			return fmt.Errorf("unsupported entry kind: %q", entry.Kind)
		}
		entries[entry.Path] = entry
	}
	if offset != manifest.Bytes {
		return errors.New("manifest does not cover the full payload")
	}
	if root, exists := entries["."]; !exists || root.Kind != "directory" {
		return errors.New("volume root metadata is missing")
	}
	for _, entry := range manifest.Entries {
		for ancestor := path.Dir(entry.Path); ancestor != "."; ancestor = path.Dir(ancestor) {
			if parent, exists := entries[ancestor]; !exists || parent.Kind != "directory" {
				return fmt.Errorf("missing or non-directory ancestor of %q", entry.Path)
			}
		}
		if entry.Kind == "hardlink" {
			target, exists := entries[entry.Link]
			if !exists || target.Kind != "file" {
				return errors.New("hard links must name a regular file in this volume")
			}
			if entry.Mode != target.Mode || entry.UID != target.UID || entry.GID != target.GID || !entry.Modified.Equal(target.Modified) {
				return errors.New("hard-link inode metadata must match its regular-file representative")
			}
		}
	}
	return nil
}

// payload is an exclusive host staging file, not a path derived from the archive.
// All content, including sparse logical expansion, is bounded by payloadLimit.
func collectVolumeArchive(archive io.Reader, payload io.Writer, engine, name string) (volumeManifest, error) {
	manifest := volumeManifest{Version: 1, Engine: engine, Volume: name, Created: time.Now().UTC()}
	limited := &io.LimitedReader{R: archive, N: volumePayloadLimit + volumeManifestLimit + 1}
	reader := tar.NewReader(limited)
	payloadHash := sha256.New()
	root := ""
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, err
		}
		if len(manifest.Entries) >= volumeEntryLimit {
			return manifest, errors.New("volume exceeds 100,000-entry package limit")
		}
		raw := strings.TrimSuffix(header.Name, "/")
		if root == "" {
			if !portablePath(raw, false) || strings.Contains(raw, "/") || header.Typeflag != tar.TypeDir {
				return manifest, errors.New("archive must begin with its volume root directory")
			}
			root = raw
		}
		relative := "."
		if raw != root {
			if !strings.HasPrefix(raw, root+"/") {
				return manifest, errors.New("archive entry escaped the selected volume")
			}
			relative = strings.TrimPrefix(raw, root+"/")
		}
		if !portablePath(relative, true) || len(header.Xattrs) != 0 {
			return manifest, fmt.Errorf("unsupported path or extended attributes: %q", header.Name)
		}
		for key := range header.PAXRecords {
			if key != "path" && key != "linkpath" && key != "mtime" && key != "atime" && key != "ctime" {
				return manifest, fmt.Errorf("unsupported archive metadata %q on %q", key, header.Name)
			}
		}
		entry := volumeEntry{Path: relative, Mode: header.Mode, UID: header.Uid, GID: header.Gid, Modified: header.ModTime.UTC(), Offset: manifest.Bytes}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			entry.Kind = "file"
			entry.Size = header.Size
			if entry.Size < 0 || entry.Size > volumePayloadLimit-manifest.Bytes {
				return manifest, errors.New("volume payload exceeds 64 GiB")
			}
			hash := sha256.New()
			if _, err = io.CopyN(io.MultiWriter(payload, payloadHash, hash), reader, entry.Size); err != nil {
				return manifest, err
			}
			entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
			manifest.Bytes += entry.Size
		case tar.TypeDir:
			entry.Kind = "directory"
		case tar.TypeSymlink:
			entry.Kind = "symlink"
			entry.Link = header.Linkname
		case tar.TypeLink:
			entry.Kind = "hardlink"
			if !strings.HasPrefix(header.Linkname, root+"/") {
				return manifest, errors.New("hard link escaped volume root")
			}
			entry.Link = strings.TrimPrefix(header.Linkname, root+"/")
		default:
			return manifest, fmt.Errorf("unsupported special/sparse object at %q", header.Name)
		}
		manifest.Entries = append(manifest.Entries, entry)
	}
	if limited.N <= 0 {
		return manifest, errors.New("volume archive budget exceeded")
	}
	manifest.SHA256 = hex.EncodeToString(payloadHash.Sum(nil))
	// Tar hard-link headers may omit inode fields. The data-bearing regular
	// header describes the same inode; normalize its metadata for all members.
	files := map[string]volumeEntry{}
	for _, entry := range manifest.Entries {
		if entry.Kind == "file" {
			files[entry.Path] = entry
		}
	}
	for index, entry := range manifest.Entries {
		if entry.Kind == "hardlink" {
			if target, exists := files[entry.Link]; exists {
				manifest.Entries[index].Mode = target.Mode
				manifest.Entries[index].UID = target.UID
				manifest.Entries[index].GID = target.GID
				manifest.Entries[index].Modified = target.Modified
			}
		}
	}
	return manifest, manifest.validate()
}

func writeVolumePackage(writer io.Writer, manifest volumeManifest, payload io.Reader) error {
	if err := manifest.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if int64(len(data)) > volumeManifestLimit {
		return errors.New("volume manifest exceeds 32 MiB")
	}
	if err = writeFull(writer, []byte(volumeMagic)); err != nil {
		return err
	}
	if err = binary.Write(writer, binary.LittleEndian, uint64(len(data))); err != nil {
		return err
	}
	if err = writeFull(writer, data); err != nil {
		return err
	}
	count, err := io.CopyN(writer, payload, manifest.Bytes)
	if err != nil {
		return err
	}
	if count != manifest.Bytes {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// Keep the file handle open through import; validate and later read the same
// handle. The import rehashes its stream again, detecting concurrent host writes.
func readVolumePackage(reader io.ReaderAt, size int64) (volumeManifest, int64, error) {
	var manifest volumeManifest
	prefix := make([]byte, len(volumeMagic)+8)
	if _, err := reader.ReadAt(prefix, 0); err != nil {
		return manifest, 0, err
	}
	if string(prefix[:len(volumeMagic)]) != volumeMagic {
		return manifest, 0, errors.New("not an OrbWin portable-volume package")
	}
	length := binary.LittleEndian.Uint64(prefix[len(volumeMagic):])
	if length == 0 || length > uint64(volumeManifestLimit) {
		return manifest, 0, errors.New("invalid volume manifest length")
	}
	data := make([]byte, int(length))
	if _, err := reader.ReadAt(data, int64(len(prefix))); err != nil {
		return manifest, 0, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, 0, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return manifest, 0, errors.New("unexpected manifest suffix")
	}
	if err := manifest.validate(); err != nil {
		return manifest, 0, err
	}
	start := int64(len(prefix)) + int64(length)
	if size != start+manifest.Bytes {
		return manifest, 0, errors.New("truncated package or unexpected trailing data")
	}
	if err := verifyVolumePayload(reader, start, manifest); err != nil {
		return manifest, 0, err
	}
	return manifest, start, nil
}
func verifyVolumePayload(reader io.ReaderAt, start int64, manifest volumeManifest) error {
	whole := sha256.New()
	for _, entry := range manifest.Entries {
		if entry.Kind != "file" {
			continue
		}
		hash := sha256.New()
		if _, err := io.Copy(io.MultiWriter(hash, whole), io.NewSectionReader(reader, start+entry.Offset, entry.Size)); err != nil {
			return err
		}
		if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return fmt.Errorf("file checksum mismatch: %q", entry.Path)
		}
	}
	if hex.EncodeToString(whole.Sum(nil)) != manifest.SHA256 {
		return errors.New("volume payload checksum mismatch")
	}
	return nil
}

// Directories/regular files precede links, and no entry can have link ancestry.
// A validated package therefore cannot redirect an extraction outside /source.
func volumeRestoreArchive(writer io.Writer, reader io.ReaderAt, start int64, manifest volumeManifest) error {
	if err := manifest.validate(); err != nil {
		return err
	}
	entries := append([]volumeEntry(nil), manifest.Entries...)
	rank := func(entry volumeEntry) int {
		switch entry.Kind {
		case "directory":
			return 0
		case "file":
			return 1
		case "hardlink":
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := rank(entries[i]), rank(entries[j])
		if a != b {
			return a < b
		}
		return entries[i].Path < entries[j].Path
	})
	archive := tar.NewWriter(writer)
	for _, entry := range entries {
		// Copy to the tools root with a fixed source/ prefix. Copying '.' into
		// /source makes Moby treat root metadata as destination metadata and skip
		// restoring its mtime. No archive-controlled path can leave this mount.
		name := "source"
		if entry.Path != "." {
			name += "/" + entry.Path
		}
		link := entry.Link
		if entry.Kind == "hardlink" {
			link = "source/" + link
		}
		header := &tar.Header{Name: name, Mode: entry.Mode, Uid: entry.UID, Gid: entry.GID, ModTime: entry.Modified, Size: entry.Size, Linkname: link, Format: tar.FormatPAX}
		switch entry.Kind {
		case "directory":
			header.Typeflag = tar.TypeDir
		case "file":
			header.Typeflag = tar.TypeReg
		case "hardlink":
			header.Typeflag = tar.TypeLink
		case "symlink":
			header.Typeflag = tar.TypeSymlink
		}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if entry.Kind == "file" {
			hash := sha256.New()
			count, err := io.Copy(io.MultiWriter(archive, hash), io.NewSectionReader(reader, start+entry.Offset, entry.Size))
			if err != nil {
				return err
			}
			if count != entry.Size || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
				return errors.New("package changed while restoring; pending target must remain isolated")
			}
		}
	}
	return archive.Close()
}
