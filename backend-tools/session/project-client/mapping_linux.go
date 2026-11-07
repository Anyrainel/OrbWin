//go:build linux

package main

// Own only a new root-relative metadata file. An open os.Root pins the reviewed
// directory across rename/replacement. Never overwrite, follow a final symlink,
// remove partial files, or claim cancellation rolls back a committed write.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"
)

func mappingFilename(name string) bool {
	return name != "" && len(name) <= 240 && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\") && strings.HasSuffix(strings.ToLower(name), ".json") && !strings.ContainsFunc(name, unicode.IsControl)
}
func rootIdentity(root *os.Root) (string, error) {
	info, err := root.Stat(".")
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("project inode metadata unavailable")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
func createMapping(ctx context.Context, root *os.Root, signature, name, contents string, check func() error) (resultErr error) {
	if !mappingFilename(name) {
		return errors.New("choose a new JSON filename in the project root, without directories")
	}
	if _, err := checkedMapping(contents); err != nil {
		return err
	}
	current, err := rootIdentity(root)
	if err != nil || current != signature {
		return errors.New("opened project directory no longer matches its reviewed identity")
	}
	if err = check(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return fmt.Errorf("new mapping file was not created (existing files are retained): %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			file.Close()
		}
	}()
	defer func() {
		if resultErr != nil {
			resultErr = fmt.Errorf("mapping file may remain at %s; inspect it before retrying: %w", name, resultErr)
		}
	}()
	if _, err = io.WriteString(file, contents); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	named, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !named.Mode().IsRegular() || !os.SameFile(opened, named) || named.Size() != int64(len(contents)) {
		return errors.New("mapping file was replaced while writing")
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	err = file.Close()
	closed = true
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return check()
}

type mappingVolume struct {
	Name, Driver, Scope, CreatedAt, Mountpoint string
	Options                                    map[string]any
	Labels                                     map[string]string
}

func (request workflowRequest) mappingVolumes(ctx context.Context, document mappingDocument) (map[string]mappingVolume, error) {
	names := make([]string, 0, len(document.Volumes))
	for _, volume := range document.Volumes {
		names = append(names, volume.Name)
	}
	sort.Strings(names)
	args := append([]string{"volume", "inspect"}, names...)
	command := request.command(ctx, args...)
	out, diagnostic := &limitBuffer{left: 1 << 20}, &limitBuffer{left: 8192}
	command.Stdout = out
	command.Stderr = diagnostic
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("inspect mapping destinations: %w: %s", err, diagnostic.String())
	}
	var volumes []mappingVolume
	if err := json.Unmarshal(out.Bytes(), &volumes); err != nil {
		return nil, err
	}
	if len(volumes) != len(names) {
		return nil, errors.New("mapping destinations changed")
	}
	result := map[string]mappingVolume{}
	for _, volume := range volumes {
		if result[volume.Name].Name != "" || volume.Driver != "local" || volume.Scope != "local" || len(volume.Options) != 0 || volume.CreatedAt == "" {
			return nil, errors.New("choose existing ordinary local mapping destinations")
		}
		result[volume.Name] = volume
	}
	for _, name := range names {
		if result[name].Name == "" {
			return nil, errors.New("mapping destination is missing")
		}
	}
	return result, nil
}
func saveProjectMapping(ctx context.Context, request workflowRequest, output io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	document, err := checkedMapping(request.Mapping)
	if err != nil {
		return err
	}
	before, err := request.mappingVolumes(ctx, document)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(request.Source)
	if err != nil {
		return err
	}
	defer root.Close()
	check := func() error { return request.verifyProject() }
	if err = createMapping(ctx, root, request.Signature, request.File, request.Mapping, check); err != nil {
		return err
	}
	after, err := request.mappingVolumes(ctx, document)
	if err == nil && !reflect.DeepEqual(before, after) {
		err = errors.New("destination volume metadata changed while saving")
	}
	if err == nil {
		err = request.verifyDaemon(ctx)
	}
	if err == nil {
		err = request.verifyProject()
	}
	if err != nil {
		return fmt.Errorf("mapping file was created but verification failed; inspect %s before retrying: %w", request.File, err)
	}
	digest := sha256.Sum256([]byte(request.Mapping))
	return json.NewEncoder(output).Encode(map[string]any{"version": 1, "path": request.Source + "/" + request.File, "signature": request.Signature, "sha256": hex.EncodeToString(digest[:])})
}
