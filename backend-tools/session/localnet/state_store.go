package localnet

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Only OpenManagedNamespaceLedger installs the production adapter, from an
// opaque sdkstate root. The plain-filesystem path is a controlled test seam.
// This interface is private: no IPC or renderer can register storage authority.
type serviceStateStore interface {
	Begin(context.Context) error
	Finish() error
	Check() error
	Read(string, int64) ([]byte, error)
	Create(string, []byte) error
	Append(string, []byte, []byte) error
	Replace(string, []byte, []byte) error
	Delete(string, []byte) error
	Names() ([]string, error)
	Close() error
}

func (ledger *NamespaceLedger) lockState(ctx context.Context) error {
	ledger.mu.Lock()
	if ledger.stateStore == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bounded, cancel := context.WithTimeout(ctx, 120*time.Second)
	ledger.stateCancel = cancel
	if err := ledger.stateStore.Begin(bounded); err != nil {
		cancel()
		ledger.stateCancel = nil
		ledger.mu.Unlock()
		return err
	}
	return nil
}
func (ledger *NamespaceLedger) finishState(resultErr *error) {
	if ledger.stateStore != nil {
		*resultErr = errors.Join(*resultErr, ledger.stateStore.Finish())
	}
	if ledger.stateCancel != nil {
		ledger.stateCancel()
		ledger.stateCancel = nil
	}
	ledger.mu.Unlock()
}

func (ledger *NamespaceLedger) stateName(path string) (string, error) {
	if filepath.Dir(path) != ledger.directory || filepath.Base(path) == "." || filepath.Base(path) == ".." {
		return "", errors.New("closed local-services record name required")
	}
	return filepath.Base(path), nil
}
func (ledger *NamespaceLedger) readState(path string, budget int64) ([]byte, error) {
	if ledger.stateStore == nil {
		return readRotationBytes(path, budget)
	}
	name, err := ledger.stateName(path)
	if err != nil {
		return nil, err
	}
	return ledger.stateStore.Read(name, budget)
}
func (ledger *NamespaceLedger) createState(path string, data []byte) error {
	if ledger.stateStore != nil {
		name, err := ledger.stateName(path)
		if err != nil {
			return err
		}
		if _, err = ledger.stateStore.Read(name, int64(len(data))); err == nil {
			return os.ErrExist
		} else if !os.IsNotExist(err) {
			return err
		}
		return ledger.stateStore.Create(name, data)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, err := file.Write(data)
	if err == nil && n != len(data) {
		err = errors.New("state write incomplete")
	}
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}
func (ledger *NamespaceLedger) appendState(path string, before, extra []byte) error {
	if ledger.stateStore != nil {
		name, err := ledger.stateName(path)
		if err != nil {
			return err
		}
		return ledger.stateStore.Append(name, before, extra)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	n, err := file.Write(extra)
	if err == nil && n != len(extra) {
		err = errors.New("state append incomplete")
	}
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}
func (ledger *NamespaceLedger) deleteState(path string, before []byte) error {
	if ledger.stateStore == nil {
		return os.Remove(path)
	}
	name, err := ledger.stateName(path)
	if err != nil {
		return err
	}
	return ledger.stateStore.Delete(name, before)
}
func (ledger *NamespaceLedger) replaceState(path string, data []byte, budget int64) error {
	before, err := ledger.readState(path, budget)
	if os.IsNotExist(err) {
		return ledger.createState(path, data)
	}
	if err != nil {
		return err
	}
	if ledger.stateStore != nil {
		name, err := ledger.stateName(path)
		if err != nil {
			return err
		}
		return ledger.stateStore.Replace(name, before, data)
	}
	return errors.New("managed replacement is unavailable to plain fixture path")
}
func (ledger *NamespaceLedger) writeRotationState(path string, data []byte, allowIdentical bool) error {
	if ledger.stateStore == nil {
		return writeRotationExclusive(path, data, allowIdentical)
	}
	before, err := ledger.readState(path, int64(len(data)))
	if err == nil {
		if allowIdentical && bytes.Equal(before, data) {
			return nil
		}
		return errors.New("managed rotation archive exists; preserve it for review")
	}
	if !os.IsNotExist(err) {
		return err
	}
	return ledger.createState(path, data)
}
func (ledger *NamespaceLedger) checkState() error {
	if ledger.stateStore == nil {
		return nil
	}
	return ledger.stateStore.Check()
}

func (ledger *NamespaceLedger) removeStateCurrent(path string, budget int64) error {
	before, err := ledger.readState(path, budget)
	if err != nil {
		return err
	}
	return ledger.deleteState(path, before)
}
func (ledger *NamespaceLedger) removeRotationState(path, digest string) error {
	if ledger.stateStore == nil {
		return removeRotationExact(path, digest)
	}
	data, err := ledger.readState(path, authorityStateBudget)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if rotationDigest(data) != digest {
		return errors.New("managed rotation artifact differs; preserve it")
	}
	return ledger.deleteState(path, data)
}
func (ledger *NamespaceLedger) checkRotationState(path, digest string, missingOK bool) error {
	if ledger.stateStore == nil {
		return checkRotationExact(path, digest, missingOK)
	}
	data, err := ledger.readState(path, authorityStateBudget)
	if os.IsNotExist(err) && missingOK {
		return nil
	}
	if err != nil {
		return err
	}
	if rotationDigest(data) != digest {
		return errors.New("managed rotation artifact differs; preserve it")
	}
	return nil
}

type stateEntry string

func (e stateEntry) Name() string    { return string(e) }
func (stateEntry) IsDir() bool       { return false }
func (stateEntry) Type() fs.FileMode { return 0 }
func (stateEntry) Info() (fs.FileInfo, error) {
	return nil, errors.New("managed record metadata is available only through its native capability")
}
func (ledger *NamespaceLedger) stateEntries() ([]os.DirEntry, error) {
	if ledger.stateStore == nil {
		return os.ReadDir(ledger.directory)
	}
	names, err := ledger.stateStore.Names()
	if err != nil {
		return nil, err
	}
	entries := make([]os.DirEntry, len(names))
	for i, name := range names {
		entries[i] = stateEntry(name)
	}
	return entries, nil
}
