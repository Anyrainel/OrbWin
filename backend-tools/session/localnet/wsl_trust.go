package localnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orbwin.local/session/internal/wsltrust"
)

// WSLTrustClient supplies the authenticated current Windows SID and fixed,
// separately attested helper transport. Production creates it from bundled bytes,
// never a renderer-provided executable, command, path or Windows user identity.
type WSLTrustClient interface {
	UserID() string
	Call(context.Context, wsltrust.Request) (*wsltrust.Status, error)
}

type WSLTrustReview struct {
	DistributionID string `json:"distributionId"`
	MachineID      string `json:"machineId"`
	GuestSequence  uint64 `json:"guestSequence"`
	OwnerSequence  uint64 `json:"ownerSequence"`
}
type WSLTrustStatus struct {
	DistributionID string           `json:"distributionId"`
	RootSHA256     string           `json:"rootSha256"`
	OwnerState     string           `json:"ownerState"`
	OwnerSequence  uint64           `json:"ownerSequence"`
	Managed        bool             `json:"managed"`
	Observed       bool             `json:"observed"`
	Guest          *wsltrust.Status `json:"guest,omitempty"`
}
type wslTrustRecord struct {
	Version         int        `json:"version"`
	Daemon          string     `json:"daemon"`
	Installation    string     `json:"installation"`
	WindowsUser     string     `json:"windowsUser"`
	DistributionID  string     `json:"distributionId"`
	MachineID       string     `json:"machineId"`
	RootSHA256      string     `json:"rootSha256"`
	RootCertificate []byte     `json:"rootCertificate"`
	State           string     `json:"state"`
	Sequence        uint64     `json:"sequence"`
	GuestSequence   uint64     `json:"guestSequence"`
	RetiredAt       *time.Time `json:"retiredAt,omitempty"`
}

const wslTrustBudget = 32768

func (record wslTrustRecord) request(operation string) wsltrust.Request {
	return wsltrust.Request{Version: 1, Operation: operation, Installation: record.Installation, WindowsUser: record.WindowsUser, DistributionID: record.DistributionID, RootSHA256: record.RootSHA256, RootCertificate: append([]byte(nil), record.RootCertificate...), ExpectedSequence: record.GuestSequence, ExpectedMachineID: record.MachineID}
}
func (record wslTrustRecord) validate(daemon, installation, user, distribution, root string) error {
	if record.Version != 1 || record.Sequence == 0 || record.Daemon != daemon || record.Installation != installation || record.WindowsUser != user || record.DistributionID != distribution || record.RootSHA256 != root {
		return errors.New("selected WSL trust receipt owner or version differs")
	}
	switch record.State {
	case "install-intent", "installed", "remove-intent", "removed", "registration-retiring", "registration-retired":
	default:
		return errors.New("selected WSL trust receipt state differs")
	}
	if (record.State == "registration-retired") != (record.RetiredAt != nil) || (record.RetiredAt != nil && record.RetiredAt.IsZero()) {
		return errors.New("selected WSL retirement timestamp/state differs")
	}
	if (record.State == "installed" || record.State == "removed") && record.GuestSequence == 0 {
		return errors.New("settled WSL trust requires a confirmed guest receipt sequence")
	}
	_, err := record.request("remove").Validate()
	return err
}
func (ledger *NamespaceLedger) wslTrustPath(distribution, root string) string {
	return filepath.Join(ledger.directory, "wsl-trust-"+distribution+"-"+root+".json")
}
func (ledger *NamespaceLedger) readWSLTrust(installation, user, distribution, root string) (*wslTrustRecord, error) {
	if !wsltrust.ValidRegistrationID(distribution) || !fullID(root) {
		return nil, errors.New("exact selected registration and root required")
	}
	data, err := ledger.readState(ledger.wslTrustPath(distribution, root), wslTrustBudget)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read selected WSL trust receipt: %w", err)
	}
	var record wslTrustRecord
	if err := strictJSON(data, &record); err != nil {
		return nil, err
	}
	if err := record.validate(ledger.daemon, installation, user, distribution, root); err != nil {
		return nil, err
	}
	return &record, nil
}
func (ledger *NamespaceLedger) wslTrustEntries() ([]os.DirEntry, error) {
	if ledger.stateStore != nil {
		return ledger.stateEntries()
	}
	directory, err := os.Open(ledger.directory)
	if err != nil {
		return nil, err
	}
	entries, err := directory.ReadDir(4097)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	err = errors.Join(err, directory.Close())
	if err != nil {
		return nil, err
	}
	if len(entries) > 4096 {
		return nil, errors.New("local-services state directory exceeds its inspection budget")
	}
	return entries, nil
}
func (ledger *NamespaceLedger) writeWSLTrust(record *wslTrustRecord) (resultErr error) {
	if record.Sequence == math.MaxUint64 {
		return errors.New("selected WSL trust receipt sequence exhausted")
	}
	copy := *record
	copy.Sequence++
	if err := copy.validate(ledger.daemon, copy.Installation, copy.WindowsUser, copy.DistributionID, copy.RootSHA256); err != nil {
		return err
	}
	entries, err := ledger.wslTrustEntries()
	if err != nil {
		return err
	}
	count, drafts := 0, 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wsl-trust-") {
			count++
		}
		if strings.HasPrefix(entry.Name(), ".wsl-trust-") {
			drafts++
		}
	}
	if (record.Sequence == 0 && count >= 128) || drafts >= 64 {
		return errors.New("selected WSL trust receipt/draft budget reached; retain records for review")
	}
	data, err := json.Marshal(copy)
	if err != nil || len(data) > wslTrustBudget {
		return errors.Join(errors.New("selected WSL trust receipt exceeds its budget"), err)
	}
	if ledger.stateStore != nil {
		if err := ledger.replaceState(ledger.wslTrustPath(copy.DistributionID, copy.RootSHA256), data, wslTrustBudget); err != nil {
			return err
		}
		*record = copy
		return nil
	}
	file, err := os.CreateTemp(ledger.directory, ".wsl-trust-*.partial")
	if err != nil {
		return err
	}
	// Like the Windows Root journal, delete only this invocation's draft. The
	// atomic replacement may have committed even when it returned an error.
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
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	replace := ledger.trustReplace
	if replace == nil {
		replace = replaceSettings
	}
	if err := replace(file.Name(), ledger.wslTrustPath(copy.DistributionID, copy.RootSHA256)); err != nil {
		return err
	}
	*record = copy
	return nil
}
func wslTrustStatus(distribution, root string, record *wslTrustRecord, guest *wsltrust.Status) WSLTrustStatus {
	status := WSLTrustStatus{DistributionID: distribution, RootSHA256: root, OwnerState: "unmanaged", Guest: guest}
	status.Observed = guest != nil && guest.Observed && guest.DistributionID == distribution && guest.RootSHA256 == root && guest.OSID == "ubuntu" && guest.OSVersion == "24.04" && len(guest.MachineID) == 32
	if record != nil {
		status.Observed = status.Observed && guest.MachineID == record.MachineID && guest.Sequence >= record.GuestSequence
		status.OwnerState, status.OwnerSequence = record.State, record.Sequence
		status.Managed = record.State != "removed" && record.State != "registration-retired" && status.Observed && guest.Managed
	}
	return status
}
func (ledger *NamespaceLedger) wslTrustWriteError(installation, user, distribution, root string, guest *wsltrust.Status, cause error) (WSLTrustStatus, error) {
	record, err := ledger.readWSLTrust(installation, user, distribution, root)
	status := wslTrustStatus(distribution, root, record, guest)
	if err != nil {
		status.OwnerState = "unavailable"
		status.Managed = false
		status.Observed = false
	}
	return status, errors.Join(cause, err)
}
func validateWSLObservation(request wsltrust.Request, status *wsltrust.Status) error {
	if status == nil || !status.Observed || status.RootSHA256 != request.RootSHA256 || status.DistributionID != request.DistributionID || status.OSID != "ubuntu" || status.OSVersion != "24.04" || len(status.MachineID) != 32 || (request.ExpectedMachineID != "" && status.MachineID != request.ExpectedMachineID) {
		return errors.New("selected WSL trust observation or identity is unavailable or differs")
	}
	check := request
	check.Operation, check.ExpectedMachineID = "status", status.MachineID
	if _, err := check.Validate(); err != nil {
		return err
	}
	switch status.State {
	case "absent", "present-unmanaged", "install-intent", "installed", "remove-intent", "removed":
	default:
		return errors.New("selected WSL trust observation state differs")
	}
	if (status.Managed || status.State == "removed") && status.Sequence == 0 {
		return errors.New("selected WSL trust receipt sequence is missing")
	}
	return nil
}

// WSLTrust serializes host receipts with other ledger mutations. Guest status is
// always freshly read; reads never settle intents. A flushed host intent precedes
// dispatch. Caller loss/unknown acknowledgement retains it without replay. Both
// reviewed sequences and the machine ID guard stale dialogs and delayed calls.
// Removal needs only owned public bytes, even after signing-state loss/expiry.
func (ledger *NamespaceLedger) WSLTrust(ctx context.Context, client WSLTrustClient, installation, root, operation string, review WSLTrustReview, now time.Time) (out WSLTrustStatus, resultErr error) {
	empty := wslTrustStatus(review.DistributionID, root, nil, nil)
	if client == nil || !fullID(root) || !wsltrust.ValidRegistrationID(review.DistributionID) || (operation != "status" && operation != "install" && operation != "remove") {
		return empty, errors.New("selected WSL trust client, operation and reviewed target required")
	}
	if err := ledger.lockState(ctx); err != nil {
		return WSLTrustStatus{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return empty, errors.New("exclusive selected WSL trust owner required")
	}
	var der []byte
	if operation == "install" {
		authority, err := ledger.loadAuthorityLocked(installation, root, now)
		if err != nil {
			return empty, err
		}
		der = authority.RootCertificate()
	}
	if operation == "install" {
		current, _, err := ledger.readAuthority(installation)
		if err != nil {
			return empty, err
		}
		if current.RootSHA256 != root {
			return empty, errors.New("selected authority changed before WSL trust installation")
		}
		if pending, err := ledger.readRotation(installation); err != nil || pending != nil {
			return empty, errors.Join(errors.New("settle authority rotation before installing WSL trust"), err)
		}
	}
	user := client.UserID()
	record, err := ledger.readWSLTrust(installation, user, review.DistributionID, root)
	if err != nil {
		return empty, err
	}
	if record != nil {
		der = append([]byte(nil), record.RootCertificate...)
	}
	if record != nil && operation != "status" && (record.State == "registration-retired" || (record.State == "registration-retiring" && operation == "install")) {
		return wslTrustStatus(review.DistributionID, root, record, nil), errors.New("selected registration retirement prevents trust installation/adoption; pending retirement may still use explicit trust removal")
	}
	if operation == "remove" && record == nil {
		return empty, errors.New("no Windows-owned selected WSL trust receipt; unowned guest state is preserved")
	}
	if der == nil {
		state, _, err := ledger.readAuthority(installation)
		if err != nil {
			return empty, err
		}
		if state.RootSHA256 != root {
			return empty, errors.New("reviewed public authority differs")
		}
		der = state.RootCertificate
	}
	request := wsltrust.Request{Version: 1, Operation: operation, Installation: installation, DistributionID: review.DistributionID, WindowsUser: user, RootSHA256: root, RootCertificate: der, ExpectedSequence: review.GuestSequence, ExpectedMachineID: review.MachineID}
	if _, err := request.Validate(); err != nil {
		return wslTrustStatus(review.DistributionID, root, record, nil), err
	}
	if record != nil {
		if review.MachineID != "" && review.MachineID != record.MachineID {
			return empty, errors.New("reviewed selected WSL machine differs from its owned receipt")
		}
		request.ExpectedMachineID = record.MachineID
	}
	ownerSequence := uint64(0)
	if record != nil {
		ownerSequence = record.Sequence
	}
	if operation != "status" && review.OwnerSequence != ownerSequence {
		return wslTrustStatus(review.DistributionID, root, record, nil), errors.New("Windows-owned WSL trust intent changed; reread before applying")
	}
	observation := request
	observation.Operation = "status"
	guest, err := client.Call(ctx, observation)
	status := wslTrustStatus(review.DistributionID, root, record, guest)
	if err != nil {
		return status, err
	}
	if err := validateWSLObservation(observation, guest); err != nil {
		status.Managed = false
		status.Observed = false
		return status, err
	}
	if record != nil && guest.Sequence < record.GuestSequence {
		return status, errors.New("selected WSL trust receipt regressed; preserve state for review")
	}
	if operation == "status" {
		return status, nil
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if guest.Sequence != review.GuestSequence || guest.MachineID != review.MachineID {
		return status, errors.New("selected WSL trust changed; reread both receipt sequences before applying")
	}
	if operation == "install" {
		if record == nil || record.State == "removed" {
			if guest.Managed {
				return status, errors.New("guest trust has no matching active Windows ownership intent; preserve it for review")
			}
			if guest.FilePresent || guest.BundlePresent {
				return status, nil
			}
		}
		if record != nil && record.State == "remove-intent" {
			return status, errors.New("finish pending selected WSL trust removal before installing again")
		}
	} else if record.State == "removed" {
		if guest.FilePresent || guest.BundlePresent || guest.Managed {
			return status, errors.New("selected WSL root was externally re-added after removal and is unmanaged")
		}
		return status, nil
	}
	if record == nil {
		record = &wslTrustRecord{Version: 1, Daemon: ledger.daemon, Installation: installation, WindowsUser: user, DistributionID: review.DistributionID, MachineID: guest.MachineID, RootSHA256: root, RootCertificate: append([]byte(nil), der...)}
	}
	// An explicit install can acknowledge an already observed completed guest
	// action after lost transport, without dispatching that action a second time.
	if operation == "install" && guest.State == "installed" && guest.Managed && guest.FilePresent && guest.BundlePresent {
		if record.State == "installed" && record.GuestSequence == guest.Sequence {
			return status, nil
		}
		record.State, record.GuestSequence = "installed", guest.Sequence
		if err := ledger.writeWSLTrust(record); err != nil {
			return ledger.wslTrustWriteError(installation, user, review.DistributionID, root, guest, err)
		}
		return wslTrustStatus(review.DistributionID, root, record, guest), nil
	}
	record.State, record.GuestSequence = operation+"-intent", guest.Sequence
	record.RetiredAt = nil
	if err := ledger.writeWSLTrust(record); err != nil {
		return ledger.wslTrustWriteError(installation, user, review.DistributionID, root, guest, err)
	}
	if err := ledger.checkState(); err != nil {
		return empty, err
	}
	guest, err = client.Call(ctx, request)
	if originErr := ledger.checkState(); originErr != nil {
		return empty, errors.Join(err, originErr)
	}
	status = wslTrustStatus(review.DistributionID, root, record, guest)
	if err != nil {
		return status, err
	}
	if err := validateWSLObservation(request, guest); err != nil {
		status.Managed = false
		status.Observed = false
		return status, err
	}
	if guest.Sequence < record.GuestSequence {
		return status, errors.New("selected WSL mutation returned a regressed receipt sequence")
	}
	if operation == "install" && (guest.State != "installed" || !guest.Managed || !guest.FilePresent || !guest.BundlePresent) {
		return status, errors.New("selected WSL root installation was not confirmed; intent retained")
	}
	if operation == "remove" && (guest.State != "removed" || guest.Managed || guest.FilePresent || guest.BundlePresent) {
		return status, errors.New("selected WSL root removal was not confirmed; intent retained")
	}
	record.State = "installed"
	if operation == "remove" {
		record.State = "removed"
	}
	record.GuestSequence = guest.Sequence
	if err := ledger.writeWSLTrust(record); err != nil {
		return ledger.wslTrustWriteError(installation, user, review.DistributionID, root, guest, err)
	}
	return wslTrustStatus(review.DistributionID, root, record, guest), nil
}

// Rotation never starts distros or interprets an unreachable one as untrusted.
// Every retained receipt is validated; malformed/foreign metadata is preserved
// and prevents silently bypassing a pending removal by renaming its file.
func (ledger *NamespaceLedger) rotationWSLTrustSettled(installation, user string) error {
	records, err := ledger.readAllWSLTrust(installation, user)
	if err != nil {
		return err
	}
	var registry WSLTrustRegistry
	for _, record := range records {
		if record != nil && record.State == "registration-retired" {
			if registry == nil {
				registry, err = ledger.wslRegistry(user)
				if err != nil {
					return err
				}
			}
			present, err := registry.Present(record.DistributionID)
			if err != nil || present {
				return errors.Join(errors.New("retired WSL registration presence is unknown or returned; preserve trust records for review"), err)
			}
			continue
		}
		if record == nil || record.State != "removed" {
			return errors.New("settle removal of owned selected WSL trust before rotating authority")
		}
	}
	return nil
}
