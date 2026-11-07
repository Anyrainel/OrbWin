//go:build linux

package wsltrust

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func (store *Store) result(request Request, cause error) (Status, error) {
	status, _, err := store.observe(request)
	return status, errors.Join(cause, err)
}
func (store *Store) install(ctx context.Context, request Request, status Status, record *receipt, lock *os.File) (Status, error) {
	if !status.Managed && (status.FilePresent || status.BundlePresent) {
		return status, nil
	}
	if record != nil && record.State == "remove-intent" {
		return status, errors.New("finish pending removal before installing this root again")
	}
	if status.Managed && status.FilePresent && status.BundlePresent {
		if record.State != "installed" {
			record.State = "installed"
			if err := store.save(record); err != nil {
				return store.result(request, err)
			}
		}
		return store.result(request, nil)
	}
	if record == nil {
		record = &receipt{Version: 1, Installation: request.Installation, DistributionID: request.DistributionID, MachineID: status.MachineID, WindowsUser: request.WindowsUser, RootSHA256: request.RootSHA256, Certificate: append([]byte(nil), request.RootCertificate...)}
	}
	record.State = "install-intent"
	if err := store.save(record); err != nil {
		return store.result(request, err)
	}
	if !status.FilePresent {
		file, err := store.root.OpenFile(store.anchor(request), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return store.result(request, err)
		}
		_, err = file.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: request.RootCertificate}))
		if err == nil {
			err = file.Sync()
		}
		err = errors.Join(err, file.Close())
		if err == nil {
			err = store.syncDirectory(store.paths.anchors)
		}
		if err != nil {
			return store.result(request, err)
		}
	}
	warning, updateErr := store.update(ctx, lock)
	record.UpdaterWarning = warning || updateErr != nil
	observed, _, readErr := store.observe(request)
	if readErr != nil || !observed.FilePresent || !observed.BundlePresent {
		writeErr := store.save(record)
		return store.result(request, errors.Join(errors.New("selected distro root installation was not confirmed; intent retained"), updateErr, readErr, writeErr))
	}
	record.State = "installed"
	writeErr := store.save(record)
	return store.result(request, errors.Join(updateErr, writeErr))
}

func (store *Store) ownedLinks(request Request) ([]string, error) {
	directory, err := store.root.Open(store.paths.certs)
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(16385)
	closed := directory.Close()
	if err != nil || closed != nil {
		return nil, errors.Join(err, closed)
	}
	if len(entries) > 16384 {
		return nil, errors.New("certificate directory exceeds its scan budget")
	}
	pemName := store.base(request) + ".pem"
	anchor := filepath.Join(store.root.Name(), store.anchor(request))
	var links []string
	for _, entry := range entries {
		name := filepath.Join(store.paths.certs, entry.Name())
		if entry.Type()&os.ModeSymlink == 0 {
			if entry.Name() == pemName {
				return nil, errors.New("owned certificate link conflicts with a regular file")
			}
			continue
		}
		target, err := store.root.Readlink(name)
		if err != nil {
			return nil, err
		}
		if entry.Name() == pemName {
			resolved := target
			if !filepath.IsAbs(target) {
				resolved = filepath.Join(store.root.Name(), store.paths.certs, target)
			}
			if filepath.Clean(resolved) != anchor {
				return nil, errors.New("owned certificate link target changed; preserved")
			}
			links = append(links, name)
		} else if target == pemName || filepath.Clean(target) == filepath.Join(store.root.Name(), store.paths.certs, pemName) {
			links = append(links, name)
		}
	}
	return links, nil
}
func (store *Store) remove(ctx context.Context, request Request, status Status, record *receipt, lock *os.File) (Status, error) {
	if record == nil {
		if status.FilePresent || status.BundlePresent {
			return status, errors.New("root has no owned receipt; preexisting certificates are preserved")
		}
		// A host may have persisted intent and lost transport before this guest
		// began installing. An exact absent-root removal must leave a durable
		// tombstone: otherwise the delayed install could still accept sequence 0
		// after removal was reported complete. This never removes unowned bytes.
		record = &receipt{Version: 1, Installation: request.Installation, DistributionID: request.DistributionID, MachineID: status.MachineID, WindowsUser: request.WindowsUser, RootSHA256: request.RootSHA256, Certificate: append([]byte(nil), request.RootCertificate...)}
	}
	if record.State == "removed" {
		if status.FilePresent || status.BundlePresent {
			return status, errors.New("root was externally re-added after removal and is unmanaged")
		}
		return status, nil
	}
	links, err := store.ownedLinks(request)
	if err != nil {
		return status, err
	}
	record.State = "remove-intent"
	if err := store.save(record); err != nil {
		return store.result(request, err)
	}
	if status.FilePresent {
		// Re-observe canonical bytes before deleting an owned anchor. Never
		// truncate or rewrite a file that another root administrator changed.
		if _, _, err := store.observe(request); err != nil {
			return status, err
		}
		if err := store.root.Remove(store.anchor(request)); err != nil {
			return store.result(request, err)
		}
		if err := store.syncDirectory(store.paths.anchors); err != nil {
			return store.result(request, err)
		}
	}
	for _, link := range links {
		if err := store.root.Remove(link); err != nil && !os.IsNotExist(err) {
			return store.result(request, err)
		}
	}
	if len(links) > 0 {
		if err := store.syncDirectory(store.paths.certs); err != nil {
			return store.result(request, err)
		}
	}
	var updateErr error
	if status.FilePresent || status.BundlePresent || len(links) > 0 {
		warning, err := store.update(ctx, lock)
		record.UpdaterWarning = warning || err != nil
		updateErr = err
	}
	observed, _, readErr := store.observe(request)
	if readErr != nil || observed.FilePresent || observed.BundlePresent {
		return observed, errors.Join(errors.New("selected distro root removal was not confirmed; other copies may still be trusted"), updateErr, readErr)
	}
	record.State = "removed"
	writeErr := store.save(record)
	return store.result(request, errors.Join(updateErr, writeErr))
}

type updaterOutput struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (output *updaterOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	remaining := 16384 - len(output.data)
	take := len(data)
	if take > remaining {
		take = remaining
		output.truncated = true
	}
	output.data = append(output.data, data[:take]...)
	return len(data), nil
}
func (output *updaterOutput) warning() bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.truncated || bytes.HasPrefix(output.data, []byte("E: ")) || bytes.Contains(output.data, []byte("\nE: "))
}
func runUpdater(program string, args []string, lock *os.File) (bool, error) {
	if lock == nil {
		return false, errors.New("certificate refresh requires an inherited trust-operation lock")
	}
	// Once accepted, a root refresh is not rolled back on renderer/client loss.
	// The inherited lock keeps later helpers out while an accepted updater runs.
	// Killing this helper externally can still leave an unconfirmed transaction.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, program, args...)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	command.ExtraFiles = []*os.File{lock}
	command.WaitDelay = 2 * time.Second
	output := &updaterOutput{}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if err != nil {
		return true, fmt.Errorf("selected distro certificate refresh was not confirmed (%v); inspect distro updater/hooks, not the application's signing keys", err)
	}
	return output.warning(), nil
}
func (store *Store) systemUpdate(_ context.Context, lock *os.File) (bool, error) {
	if strings.TrimSpace(store.root.Name()) != "/" {
		return false, errors.New("system updater refuses a non-system filesystem boundary")
	}
	return runUpdater(filepath.Join("/", store.paths.updater), nil, lock)
}
