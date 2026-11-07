//go:build linux

// OrbWin's small guest file transaction. It can reach only the selected mount;
// no daemon socket, host path, target PID namespace or network is exposed.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const editLimit = 256 << 10

type editRequest struct {
	Path     string `json:"path"`
	Expected string `json:"expected"`
	Text     string `json:"text"`
}
type editResult struct {
	Committed bool   `json:"committed"`
	Conflict  bool   `json:"conflict"`
	SHA256    string `json:"sha256,omitempty"`
	Backup    string `json:"backup,omitempty"`
	Receipt   string `json:"receipt,omitempty"`
}

func hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func regularText(file *os.File) ([]byte, unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return nil, stat, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Size < 0 || stat.Size > editLimit {
		return nil, stat, errors.New("edit requires a regular text file up to 256 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, editLimit+1))
	if err != nil {
		return nil, stat, err
	}
	if len(data) > editLimit || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return nil, stat, errors.New("edit requires complete UTF-8 text without NUL bytes")
	}
	return data, stat, nil
}
func sameVersion(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Size == b.Size && a.Mtim == b.Mtim && a.Nlink == b.Nlink
}
func edit(rootPath string, request editRequest, phaseHook func(string)) (result editResult, resultErr error) {
	expected, err := hex.DecodeString(request.Expected)
	if err != nil || len(expected) != 32 || !strings.HasPrefix(request.Path, "/") || len(request.Path) > 2048 || strings.ContainsRune(request.Path, 0) || !utf8.ValidString(request.Text) || strings.ContainsRune(request.Text, 0) || len(request.Text) > editLimit {
		return result, errors.New("invalid bounded file edit request")
	}
	clean := path.Clean(request.Path)
	relative := strings.TrimPrefix(clean, "/")
	base := path.Base(relative)
	if relative == "" || relative == "." || base == ".." {
		return result, errors.New("select an existing regular file")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return result, err
	}
	defer root.Close()
	directory, err := root.Open(path.Dir(relative))
	if err != nil {
		return result, err
	}
	defer directory.Close()
	dirfd := int(directory.Fd())
	fd, err := unix.Openat(dirfd, base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return result, err
	}
	original := os.NewFile(uintptr(fd), base)
	defer original.Close()
	data, originalStat, err := regularText(original)
	if err != nil {
		return result, err
	}
	if hash(data) != strings.ToLower(request.Expected) {
		return result, errors.New("file changed since preview; reload before saving")
	}
	attrs, err := unix.Flistxattr(fd, nil)
	if err != nil {
		return result, err
	}
	if attrs != 0 || originalStat.Nlink != 1 {
		return result, errors.New("files with extended attributes or multiple hard links require an explicit external editing workflow")
	}
	tokenBytes := make([]byte, 16)
	if _, err = rand.Read(tokenBytes); err != nil {
		return result, err
	}
	token := hex.EncodeToString(tokenBytes)
	draftName := ".orbwin-draft-" + token
	backupName := ".orbwin-backup-" + token
	receiptName := ".orbwin-edit-" + token + ".jsonl"
	staged, err := unix.Openat(dirfd, draftName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return result, err
	}
	draft := os.NewFile(uintptr(staged), draftName)
	retainStaged, receiptCreated := false, false
	defer func() {
		draft.Close()
		if !retainStaged {
			unix.Unlinkat(dirfd, draftName, 0)
			if receiptCreated {
				unix.Unlinkat(dirfd, receiptName, 0)
			}
		}
	}()
	if err = unix.Fchown(staged, int(originalStat.Uid), int(originalStat.Gid)); err != nil {
		return result, err
	}
	if err = unix.Fchmod(staged, originalStat.Mode&07777); err != nil {
		return result, err
	}
	if _, err = draft.Write([]byte(request.Text)); err != nil {
		return result, err
	}
	if err = draft.Sync(); err != nil {
		return result, err
	}
	receiptfd, err := unix.Openat(dirfd, receiptName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_APPEND|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return result, err
	}
	receiptCreated = true
	receipt := os.NewFile(uintptr(receiptfd), receiptName)
	defer receipt.Close()
	newHash := hash([]byte(request.Text))
	encoder := json.NewEncoder(receipt)
	if err = encoder.Encode(map[string]any{"schema": 1, "phase": "prepared", "path": clean, "expected": request.Expected, "newSHA256": newHash, "draft": draftName, "backup": backupName}); err != nil {
		return result, err
	}
	if err = receipt.Sync(); err != nil {
		return result, err
	}
	if err = directory.Sync(); err != nil {
		return result, err
	}
	if phaseHook != nil {
		phaseHook("prepared")
	}
	var current unix.Stat_t
	if err = unix.Fstatat(dirfd, base, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return result, err
	}
	if !sameVersion(originalStat, current) {
		return result, errors.New("file metadata changed during editing; source retained")
	}
	if _, err = original.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	data, _, err = regularText(original)
	if err != nil {
		return result, err
	}
	if hash(data) != strings.ToLower(request.Expected) {
		return result, errors.New("file contents changed during editing; source retained")
	}
	// EXCHANGE always retains the displaced inode, even if an external writer
	// races the final check. The original is never unlinked by this transaction.
	retainStaged = true
	result.Backup = path.Join("/", path.Dir(relative), draftName)
	result.Receipt = path.Join("/", path.Dir(relative), receiptName)
	if err = unix.Renameat2(dirfd, base, dirfd, draftName, unix.RENAME_EXCHANGE); err != nil {
		return result, fmt.Errorf("atomic exchange did not report success; staged draft/receipt retained for inspection: %w", err)
	}
	if phaseHook != nil {
		phaseHook("exchanged")
	}
	result.Committed = true
	result.SHA256 = newHash
	result.Backup = path.Join("/", path.Dir(relative), draftName)
	result.Receipt = path.Join("/", path.Dir(relative), receiptName)
	if err = directory.Sync(); err != nil {
		return result, err
	}
	if err = unix.Renameat2(dirfd, draftName, dirfd, backupName, unix.RENAME_NOREPLACE); err != nil {
		return result, fmt.Errorf("new content committed; displaced version retained at %s: %w", result.Backup, err)
	}
	result.Backup = path.Join("/", path.Dir(relative), backupName)
	backupfd, err := unix.Openat(dirfd, backupName, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return result, err
	}
	backup := os.NewFile(uintptr(backupfd), backupName)
	defer backup.Close()
	old, oldStat, err := regularText(backup)
	oldAttrs, attrsErr := unix.Flistxattr(backupfd, nil)
	if err != nil || attrsErr != nil || oldAttrs != 0 || !sameVersion(originalStat, oldStat) || hash(old) != strings.ToLower(request.Expected) {
		result.Conflict = true
		resultErr = errors.New("a concurrent change was captured at commit; both the saved file and displaced version were retained")
	}
	if err = encoder.Encode(map[string]any{"phase": "committed", "result": result}); err != nil {
		return result, errors.Join(resultErr, err)
	}
	if err = receipt.Sync(); err != nil {
		return result, errors.Join(resultErr, err)
	}
	return result, errors.Join(resultErr, directory.Sync())
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "hold" {
		// A killed host client cannot leave this maintenance process indefinitely.
		// Container removal remains the normal earlier cancellation/cleanup path.
		time.Sleep(1830 * time.Second)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "check-volume" {
		count, err := checkVolume("/source")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(125)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"entries": count, "attributesChecked": true})
		return
	}
	scanner := bufio.NewScanner(io.LimitReader(os.Stdin, (2<<20)+1))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	if !scanner.Scan() {
		fmt.Fprintln(os.Stderr, "missing bounded edit frame")
		os.Exit(125)
	}
	decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
	decoder.DisallowUnknownFields()
	var request editRequest
	if err := decoder.Decode(&request); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(125)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		fmt.Fprintln(os.Stderr, "unexpected edit input")
		os.Exit(125)
	}
	result, err := edit("/source", request, nil)
	json.NewEncoder(os.Stdout).Encode(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(125)
	}
}
