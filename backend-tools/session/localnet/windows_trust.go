package localnet

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RootStore is always a physical CurrentUser Root store in production. Tests
// supply a private memory store; neither a store name nor user SID comes from IPC.
type RootStore interface {
	UserID() string
	Contains(der []byte) (bool, error)
	Add(der []byte) error
	Remove(der []byte) error
	Close() error
}
type WindowsTrustStatus struct {
	RootSHA256 string `json:"rootSha256"`
	Present    bool   `json:"present"`
	Observed   bool   `json:"observed"`
	Managed    bool   `json:"managed"`
	State      string `json:"state"`
	Sequence   uint64 `json:"sequence"`
}
type windowsTrustRecord struct {
	Version         int    `json:"version"`
	Daemon          string `json:"daemon"`
	Installation    string `json:"installation"`
	UserID          string `json:"userId"`
	Scope           string `json:"scope"`
	RootSHA256      string `json:"rootSha256"`
	RootCertificate []byte `json:"rootCertificate"`
	State           string `json:"state"`
	Sequence        uint64 `json:"sequence"`
}

const windowsTrustScope = "physical-current-user-root"
const windowsTrustBudget = 32768

func (ledger *NamespaceLedger) trustPath(root string) string {
	return filepath.Join(ledger.directory, "windows-trust-"+root+".json")
}
func (record windowsTrustRecord) validate(daemon, installation, userID, root string) error {
	if record.Version != 1 || record.Daemon != daemon || record.Installation != installation || record.UserID != userID || userID == "" || len(userID) > 256 || strings.ContainsAny(userID, "\x00\r\n") || record.Scope != windowsTrustScope || record.RootSHA256 != root || record.Sequence == 0 {
		return errors.New("Windows trust receipt owner, scope or version differs")
	}
	switch record.State {
	case "install-intent", "installed", "remove-intent", "removed":
	default:
		return errors.New("Windows trust receipt state is unsupported")
	}
	_, err := validateAuthorityCertificate(record.RootCertificate, installation, root)
	return err
}
func (ledger *NamespaceLedger) readWindowsTrust(installation, userID, root string) (*windowsTrustRecord, error) {
	if !fullID(root) {
		return nil, errors.New("exact reviewed root fingerprint required")
	}
	path := ledger.trustPath(root)
	data, err := ledger.readState(path, windowsTrustBudget)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record windowsTrustRecord
	if err := strictJSON(data, &record); err != nil {
		return nil, err
	}
	if err := record.validate(ledger.daemon, installation, userID, root); err != nil {
		return nil, err
	}
	return &record, nil
}
func (ledger *NamespaceLedger) writeWindowsTrust(record windowsTrustRecord) (resultErr error) {
	if record.Sequence == math.MaxUint64 {
		return errors.New("Windows trust receipt sequence exhausted")
	}
	record.Sequence++
	if err := record.validate(ledger.daemon, record.Installation, record.UserID, record.RootSHA256); err != nil {
		return err
	}
	data, err := marshalTrust(record)
	if err != nil {
		return err
	}
	if ledger.stateStore != nil {
		return ledger.replaceState(ledger.trustPath(record.RootSHA256), data, windowsTrustBudget)
	}
	file, err := os.CreateTemp(ledger.directory, ".windows-trust-*.partial")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(file.Name()); err != nil && !os.IsNotExist(err) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	replace := ledger.trustReplace
	if replace == nil {
		replace = replaceSettings
	}
	return replace(file.Name(), ledger.trustPath(record.RootSHA256))
}
func marshalTrust(record windowsTrustRecord) ([]byte, error) {
	// Reuse strict bounded JSON encoding conventions; no signing key is recorded.
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	if len(data) > windowsTrustBudget {
		return nil, errors.New("Windows trust receipt exceeds its budget")
	}
	return data, nil
}

func trustStatus(root string, present bool, record *windowsTrustRecord) WindowsTrustStatus {
	status := WindowsTrustStatus{RootSHA256: root, Present: present, Observed: true, State: "absent"}
	if record != nil {
		status.Sequence = record.Sequence
		status.State = record.State
		status.Managed = record.State != "removed"
	}
	if present && !status.Managed {
		status.State = "present-unmanaged"
	}
	return status
}

// Status is observational: a pending receipt is never completed by a read.
func (ledger *NamespaceLedger) WindowsTrustStatus(store RootStore, installation, root string) (out WindowsTrustStatus, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return WindowsTrustStatus{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() || store == nil {
		return WindowsTrustStatus{}, errors.New("exclusive owner and current-user store required")
	}
	record, err := ledger.readWindowsTrust(installation, store.UserID(), root)
	if err != nil {
		return WindowsTrustStatus{}, err
	}
	var der []byte
	if record != nil {
		der = record.RootCertificate
	} else {
		state, _, err := ledger.readAuthority(installation)
		if err != nil {
			return WindowsTrustStatus{}, err
		}
		if state.RootSHA256 != root {
			return WindowsTrustStatus{}, errors.New("public authority differs from reviewed root")
		}
		der = state.RootCertificate
	}
	present, err := store.Contains(der)
	status := trustStatus(root, present, record)
	status.Observed = err == nil
	return status, err
}

// A failed flush/replace or native call may already have committed. Report a
// fresh receipt/store observation without interpreting an error as absence.
func (ledger *NamespaceLedger) trustErrorStatus(store RootStore, installation, root string, der []byte, cause error) (WindowsTrustStatus, error) {
	record, readErr := ledger.readWindowsTrust(installation, store.UserID(), root)
	present, inspectErr := store.Contains(der)
	status := trustStatus(root, present, record)
	status.Observed = inspectErr == nil
	return status, errors.Join(cause, readErr, inspectErr)
}

func (ledger *NamespaceLedger) InstallWindowsTrust(store RootStore, installation, root string, now time.Time) (out WindowsTrustStatus, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return WindowsTrustStatus{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() || store == nil {
		return WindowsTrustStatus{}, errors.New("exclusive owner and current-user store required")
	}
	// Retain one native record guard through signing verification, intent,
	// exact CurrentUserROOT mutation and the resulting receipt flush.
	authority, err := ledger.loadAuthorityLocked(installation, root, now)
	if err != nil {
		return WindowsTrustStatus{}, err
	}
	record, err := ledger.readWindowsTrust(installation, store.UserID(), root)
	if err != nil {
		return WindowsTrustStatus{}, err
	}
	der := authority.RootCertificate()
	present, err := store.Contains(der)
	if err != nil {
		return WindowsTrustStatus{}, err
	}
	if present && (record == nil || record.State == "removed") {
		return trustStatus(root, true, record), nil
	}
	if record != nil && record.State == "remove-intent" {
		return trustStatus(root, present, record), errors.New("finish the pending root removal before installing again")
	}
	if present && record.State == "installed" {
		return trustStatus(root, true, record), nil
	}
	if record == nil {
		entries, err := ledger.stateEntries()
		if err != nil {
			return WindowsTrustStatus{}, err
		}
		count := 0
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "windows-trust-") {
				count++
			}
		}
		if count >= 32 {
			return WindowsTrustStatus{}, errors.New("Windows trust receipt budget reached; retain records for review")
		}
		record = &windowsTrustRecord{Version: 1, Daemon: ledger.daemon, Installation: installation, UserID: store.UserID(), Scope: windowsTrustScope, RootSHA256: root, RootCertificate: der}
	}
	if !present {
		record.State = "install-intent"
		if err := ledger.writeWindowsTrust(*record); err != nil {
			return ledger.trustErrorStatus(store, installation, root, der, err)
		}
		record.Sequence++
		if err := ledger.checkState(); err != nil {
			return WindowsTrustStatus{}, err
		}
		if err := store.Add(der); err != nil {
			return ledger.trustErrorStatus(store, installation, root, der, fmt.Errorf("Windows root installation was not confirmed; reread before retrying: %w", err))
		}
		if err := ledger.checkState(); err != nil {
			return WindowsTrustStatus{}, err
		}
		present, err = store.Contains(der)
		if err != nil || !present {
			status := trustStatus(root, present, record)
			status.Observed = err == nil
			return status, errors.Join(errors.New("Windows root presence was not confirmed; installation intent retained"), err)
		}
	}
	record.State = "installed"
	if err := ledger.writeWindowsTrust(*record); err != nil {
		return ledger.trustErrorStatus(store, installation, root, der, err)
	}
	record.Sequence++
	return trustStatus(root, true, record), nil
}

func (ledger *NamespaceLedger) RemoveWindowsTrust(store RootStore, installation, root string) (out WindowsTrustStatus, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return WindowsTrustStatus{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() || store == nil {
		return WindowsTrustStatus{}, errors.New("exclusive owner and current-user store required")
	}
	record, err := ledger.readWindowsTrust(installation, store.UserID(), root)
	if err != nil {
		return WindowsTrustStatus{}, err
	}
	if record == nil {
		return WindowsTrustStatus{}, errors.New("no owned Windows trust receipt; preexisting roots must be reviewed in Windows")
	}
	present, err := store.Contains(record.RootCertificate)
	if err != nil {
		return WindowsTrustStatus{}, err
	}
	if record.State == "removed" {
		if present {
			return trustStatus(root, true, record), errors.New("this root was added after completed removal and is unmanaged")
		}
		return trustStatus(root, false, record), nil
	}
	record.State = "remove-intent"
	if err := ledger.writeWindowsTrust(*record); err != nil {
		return ledger.trustErrorStatus(store, installation, root, record.RootCertificate, err)
	}
	record.Sequence++
	var removeErr error
	if present {
		if err := ledger.checkState(); err != nil {
			return WindowsTrustStatus{}, err
		}
		removeErr = store.Remove(record.RootCertificate)
		if err := ledger.checkState(); err != nil {
			return WindowsTrustStatus{}, errors.Join(removeErr, err)
		}
	}
	present, err = store.Contains(record.RootCertificate)
	if err != nil || present {
		status := trustStatus(root, present, record)
		status.Observed = err == nil
		return status, errors.Join(errors.New("Windows root removal was not confirmed; removal intent retained"), removeErr, err)
	}
	record.State = "removed"
	if err := ledger.writeWindowsTrust(*record); err != nil {
		return ledger.trustErrorStatus(store, installation, root, record.RootCertificate, errors.Join(removeErr, err))
	}
	record.Sequence++
	return trustStatus(root, false, record), removeErr
}
