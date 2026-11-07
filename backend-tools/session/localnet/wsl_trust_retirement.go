package localnet

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"orbwin.local/session/internal/wsltrust"
)

// WSLTrustRegistry reads raw current-user registration presence. It is separate
// from the filtered ordinary-distro list and from Linux helper dispatch.
type WSLTrustRegistry interface {
	UserID() string
	Present(string) (bool, error)
}
type WSLTrustReceiptInfo struct {
	DistributionID    string     `json:"distributionId"`
	MachineID         string     `json:"machineId"`
	RootSHA256        string     `json:"rootSha256"`
	State             string     `json:"state"`
	OwnerSequence     uint64     `json:"ownerSequence"`
	GuestSequence     uint64     `json:"guestSequence"`
	RegistrationState string     `json:"registrationState"`
	RegistrationError string     `json:"registrationError,omitempty"`
	RetiredAt         *time.Time `json:"retiredAt,omitempty"`
}

func (record *wslTrustRecord) receiptInfo(registrationState string) WSLTrustReceiptInfo {
	return WSLTrustReceiptInfo{DistributionID: record.DistributionID, MachineID: record.MachineID, RootSHA256: record.RootSHA256, State: record.State, OwnerSequence: record.Sequence, GuestSequence: record.GuestSequence, RegistrationState: registrationState, RetiredAt: record.RetiredAt}
}
func (ledger *NamespaceLedger) wslRegistry(user string) (WSLTrustRegistry, error) {
	factory := ledger.wslRegistryFactory
	if factory == nil {
		factory = newWSLTrustRegistry
	}
	registry, err := factory()
	if err != nil {
		return nil, err
	}
	if registry == nil || registry.UserID() == "" || (user != "" && registry.UserID() != user) {
		return nil, errors.New("current Windows user differs from selected WSL receipt registry")
	}
	return registry, nil
}
func (ledger *NamespaceLedger) readAllWSLTrust(installation, user string) ([]*wslTrustRecord, error) {
	entries, err := ledger.wslTrustEntries()
	if err != nil {
		return nil, err
	}
	var records []*wslTrustRecord
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "wsl-trust-") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(name, "wsl-trust-"), ".json")
		if !strings.HasSuffix(name, ".json") || len(body) != 36+1+64 || body[36] != '-' {
			return nil, errors.New("retained WSL trust receipt name differs; preserve it for review")
		}
		record, err := ledger.readWSLTrust(installation, user, body[:36], body[37:])
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, errors.New("selected WSL trust receipt disappeared during inspection")
		}
		records = append(records, record)
		if len(records) > 128 {
			return nil, errors.New("retained WSL trust receipt count exceeds its budget")
		}
	}
	return records, nil
}
func registrationObservation(registry WSLTrustRegistry, id string) (string, error) {
	present, err := registry.Present(id)
	if err != nil {
		return "unknown", err
	}
	if present {
		return "present", nil
	}
	return "absent", nil
}

// ListWSLTrustReceipts never starts a distro or changes intent. An unreadable
// registry produces an explicitly unknown row, never a retirement opportunity.
func (ledger *NamespaceLedger) ListWSLTrustReceipts(installation string) (out []WSLTrustReceiptInfo, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return nil, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return nil, errors.New("exclusive selected WSL trust owner required")
	}
	registry, err := ledger.wslRegistry("")
	if err != nil {
		return nil, err
	}
	records, err := ledger.readAllWSLTrust(installation, registry.UserID())
	if err != nil {
		return nil, err
	}
	infos := make([]WSLTrustReceiptInfo, 0, len(records))
	for _, record := range records {
		presence, err := registrationObservation(registry, record.DistributionID)
		info := record.receiptInfo(presence)
		if err != nil {
			info.RegistrationError = err.Error()
			if len(info.RegistrationError) > 512 {
				info.RegistrationError = info.RegistrationError[:512]
			}
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].DistributionID == infos[j].DistributionID {
			return infos[i].RootSHA256 < infos[j].RootSHA256
		}
		return infos[i].DistributionID < infos[j].DistributionID
	})
	return infos, nil
}
func (ledger *NamespaceLedger) retirementWriteError(installation, user, distribution, root string, cause error) (WSLTrustReceiptInfo, error) {
	record, err := ledger.readWSLTrust(installation, user, distribution, root)
	if err != nil || record == nil {
		return WSLTrustReceiptInfo{DistributionID: distribution, RootSHA256: root, State: "unavailable", RegistrationState: "unknown"}, errors.Join(cause, err)
	}
	return record.receiptInfo("unknown"), cause
}

// RetireWSLTrustReceipt acknowledges a missing registration, not trust removal.
// It retains public ownership history and never calls WSL, a root store, or a
// registration deletion API. Intent/final states use the same flushed CAS owner
// journal as trust changes. Raw presence is checked before intent, before final
// commit and after it; every future rotation rechecks it independently too.
func (ledger *NamespaceLedger) RetireWSLTrustReceipt(ctx context.Context, installation, root string, review WSLTrustReview, now time.Time) (out WSLTrustReceiptInfo, resultErr error) {
	empty := WSLTrustReceiptInfo{DistributionID: review.DistributionID, RootSHA256: root, State: "unavailable", RegistrationState: "unknown"}
	if !fullID(root) || !wsltrust.ValidRegistrationID(review.DistributionID) || now.IsZero() {
		return empty, errors.New("reviewed receipt identities and retirement time required")
	}
	if err := ledger.lockState(ctx); err != nil {
		return WSLTrustReceiptInfo{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return empty, errors.New("exclusive selected WSL trust owner required")
	}
	registry, err := ledger.wslRegistry("")
	if err != nil {
		return empty, err
	}
	record, err := ledger.readWSLTrust(installation, registry.UserID(), review.DistributionID, root)
	if err != nil {
		return empty, err
	}
	if record == nil {
		return empty, errors.New("no owned selected WSL trust receipt to retire")
	}
	info := record.receiptInfo("unknown")
	if record.Sequence != review.OwnerSequence || record.GuestSequence != review.GuestSequence || record.MachineID != review.MachineID {
		return info, errors.New("retained WSL receipt changed; reread before retirement")
	}
	if record.State == "removed" {
		return info, errors.New("certificate removal is already recorded; retirement is unnecessary")
	}
	if err := ctx.Err(); err != nil {
		return info, err
	}
	presence, err := registrationObservation(registry, record.DistributionID)
	info.RegistrationState = presence
	if err != nil || presence != "absent" {
		return info, errors.Join(errors.New("exact WSL registration is present or unreadable; retirement refused"), err)
	}
	if record.State == "registration-retired" {
		return info, nil
	}
	if err := ctx.Err(); err != nil {
		return info, err
	}
	if record.State != "registration-retiring" {
		record.State = "registration-retiring"
		record.RetiredAt = nil
		if err := ledger.writeWSLTrust(record); err != nil {
			return ledger.retirementWriteError(installation, registry.UserID(), review.DistributionID, root, err)
		}
	}
	presence, err = registrationObservation(registry, record.DistributionID)
	info = record.receiptInfo(presence)
	if err != nil || presence != "absent" {
		return info, errors.Join(errors.New("WSL registration changed during retirement; intent retained"), err)
	}
	if err := ctx.Err(); err != nil {
		return info, err
	}
	retiredAt := now.UTC()
	record.State, record.RetiredAt = "registration-retired", &retiredAt
	if err := ledger.writeWSLTrust(record); err != nil {
		return ledger.retirementWriteError(installation, registry.UserID(), review.DistributionID, root, err)
	}
	presence, err = registrationObservation(registry, record.DistributionID)
	if err != nil || presence != "absent" {
		// Even if this compensating journal write fails, rotation's independent
		// raw registry check still refuses a restored or unreadable registration.
		record.State, record.RetiredAt = "registration-retiring", nil
		writeErr := ledger.writeWSLTrust(record)
		info, readErr := ledger.retirementWriteError(installation, registry.UserID(), review.DistributionID, root, errors.Join(errors.New("retirement could not confirm registration absence; review retained intent"), err, writeErr))
		info.RegistrationState = presence
		return info, readErr
	}
	return record.receiptInfo(presence), nil
}
