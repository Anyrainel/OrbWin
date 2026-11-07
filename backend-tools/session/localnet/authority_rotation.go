package localnet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

const rotationStateName = "authority-rotation-v1.json"
const rotationBudget = 3 * settingsBudget

type AuthorityRotationInfo struct {
	OldRootSHA256     string       `json:"oldRootSha256"`
	NewRootSHA256     string       `json:"newRootSha256"`
	Ports             ServicePorts `json:"ports"`
	OldRevision       uint64       `json:"oldRevision"`
	TargetRevision    uint64       `json:"targetRevision"`
	OldCertificatePEM []byte       `json:"oldCertificatePem"`
}
type authorityRotation struct {
	Version        int              `json:"version"`
	Daemon         string           `json:"daemon"`
	Installation   string           `json:"installation"`
	Old            SettingsSnapshot `json:"old"`
	Target         SettingsSnapshot `json:"target"`
	OldCertificate []byte           `json:"oldCertificate"`
	NewCertificate []byte           `json:"newCertificate"`
	OldStateSHA256 string           `json:"oldStateSha256"`
	NewStateSHA256 string           `json:"newStateSha256"`
}

func rotationDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (record authorityRotation) info() AuthorityRotationInfo {
	return AuthorityRotationInfo{OldRootSHA256: record.Old.Settings.RootSHA256, NewRootSHA256: record.Target.Settings.RootSHA256, Ports: record.Target.Settings.Ports, OldRevision: record.Old.Revision, TargetRevision: record.Target.Revision, OldCertificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: record.OldCertificate})}
}
func (record authorityRotation) validate(daemon, installation string) error {
	if record.Version != 1 || record.Daemon != daemon || record.Installation != installation || record.Old.Revision == 0 || record.Old.Revision == math.MaxUint64 || record.Target.Revision != record.Old.Revision+1 || record.Old.Settings.Enabled || record.Target.Settings.Enabled || !fullID(record.OldStateSHA256) || !fullID(record.NewStateSHA256) || record.Old.Settings.RootSHA256 == record.Target.Settings.RootSHA256 {
		return errors.New("rotation version, owner or settings transition differs")
	}
	if err := record.Old.Settings.validate(); err != nil {
		return err
	}
	if err := record.Target.Settings.validate(); err != nil {
		return err
	}
	wanted := record.Old.Settings
	wanted.RootSHA256 = record.Target.Settings.RootSHA256
	wanted.Ports = record.Target.Settings.Ports
	if !reflect.DeepEqual(wanted, record.Target.Settings) {
		return errors.New("rotation cannot change workload preferences or network selections")
	}
	if _, err := validateAuthorityCertificate(record.OldCertificate, installation, record.Old.Settings.RootSHA256); err != nil {
		return err
	}
	_, err := validateAuthorityCertificate(record.NewCertificate, installation, record.Target.Settings.RootSHA256)
	return err
}
func (ledger *NamespaceLedger) rotationPath() string {
	return filepath.Join(ledger.directory, rotationStateName)
}
func (ledger *NamespaceLedger) rotationArtifact(root, suffix string) string {
	return filepath.Join(ledger.directory, ".rotation-"+root+"."+suffix)
}

// All paths are fixed or derived from a validated full digest. Reads reject
// aliases/changed files; writes never replace unknown preparations or archives.
func readRotationBytes(path string, budget int64) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > budget {
		return nil, errors.New("rotation artifact must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		file.Close()
		return nil, errors.New("rotation artifact changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, budget+1))
	err = errors.Join(err, file.Close())
	if err != nil {
		return nil, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || int64(len(data)) > budget {
		return nil, errors.New("rotation artifact changed while reading")
	}
	return data, nil
}
func writeRotationExclusive(path string, data []byte, allowIdentical bool) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) && allowIdentical {
		existing, readErr := readRotationBytes(path, authorityStateBudget)
		if readErr == nil && bytes.Equal(existing, data) {
			return nil
		}
		return errors.Join(errors.New("rotation archive conflicts; existing bytes retained"), readErr)
	}
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}
func (ledger *NamespaceLedger) readRotation(installation string) (*authorityRotation, error) {
	data, err := ledger.readState(ledger.rotationPath(), rotationBudget)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record authorityRotation
	if err := strictJSON(data, &record); err != nil {
		return nil, err
	}
	if err := record.validate(ledger.daemon, installation); err != nil {
		return nil, err
	}
	return &record, nil
}
func (ledger *NamespaceLedger) AuthorityRotationInfo(installation string) (out *AuthorityRotationInfo, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return nil, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return nil, errors.New("exclusive rotation owner required")
	}
	record, err := ledger.readRotation(installation)
	if err != nil || record == nil {
		return nil, err
	}
	info := record.info()
	return &info, nil
}
func (ledger *NamespaceLedger) rotationTrustAbsent(store RootStore, installation string, record *authorityRotation) error {
	if store == nil {
		return errors.New("current-user root observation required before rotation")
	}
	if err := ledger.rotationWSLTrustSettled(installation, store.UserID()); err != nil {
		return err
	}
	trust, err := ledger.readWindowsTrust(installation, store.UserID(), record.Old.Settings.RootSHA256)
	if err != nil {
		return err
	}
	if trust != nil && trust.State != "removed" {
		return errors.New("settle removal of the old Windows trust receipt before rotating")
	}
	present, err := store.Contains(record.OldCertificate)
	if err != nil {
		return err
	}
	if present {
		return errors.New("remove the old Windows root before rotating; preexisting roots require Windows certificate review")
	}
	return nil
}
func (ledger *NamespaceLedger) rotationIngressSettled() error {
	namespaces, err := ledger.readRecords()
	if err != nil {
		return err
	}
	relays, err := ledger.readRelayRecords()
	if err != nil {
		return err
	}
	if len(namespaces)+len(relays) != 0 {
		return errors.New("settle local ingress cleanup before rotating authority")
	}
	return nil
}

func (ledger *NamespaceLedger) BeginAuthorityRotation(store RootStore, installation, oldRoot string, revision uint64, ports ServicePorts, now time.Time) (out *AuthorityRotationInfo, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return nil, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() || !fullID(oldRoot) {
		return nil, errors.New("exclusive owner and reviewed old authority required")
	}
	if err := ports.validate(); err != nil {
		return nil, err
	}
	if pending, err := ledger.readRotation(installation); err != nil || pending != nil {
		return nil, errors.Join(errors.New("a retained rotation must be explicitly resumed or canceled"), err)
	}
	old, err := ledger.readSettings()
	if err != nil {
		return nil, err
	}
	if old.Revision != revision || revision == 0 || revision == math.MaxUint64 || old.Settings.Enabled || old.Settings.RootSHA256 != oldRoot {
		return nil, errors.New("disable access and review the current authority/settings revision before rotation")
	}
	if err := ledger.rotationIngressSettled(); err != nil {
		return nil, err
	}
	oldData, err := ledger.readState(filepath.Join(ledger.directory, authorityStateName), authorityStateBudget)
	if err != nil {
		return nil, err
	}
	var oldState authorityState
	if err := strictJSON(oldData, &oldState); err != nil {
		return nil, err
	}
	if _, err := validateAuthorityPublic(oldState, installation); err != nil {
		return nil, err
	}
	if oldState.RootSHA256 != oldRoot {
		return nil, errors.New("reviewed old authority differs from the stored public root")
	}
	record := authorityRotation{Version: 1, Daemon: ledger.daemon, Installation: installation, Old: old, OldCertificate: oldState.RootCertificate, OldStateSHA256: rotationDigest(oldData)}
	if err := ledger.rotationTrustAbsent(store, installation, &record); err != nil {
		return nil, err
	}
	entries, err := ledger.stateEntries()
	if err != nil {
		return nil, err
	}
	preparations := 0
	for _, entry := range entries {
		if len(entry.Name()) >= 10 && entry.Name()[:10] == ".rotation-" {
			preparations++
		}
	}
	if preparations >= 64 {
		return nil, errors.New("retained rotation preparations reached their budget; review them before creating another root")
	}
	created, err := NewAuthorityWithCRLPort(installation, now, ports.HTTP)
	if err != nil {
		return nil, err
	}
	newData, err := encodeProtectedAuthority(created, installation)
	if err != nil || len(newData) > authorityStateBudget {
		return nil, errors.Join(errors.New("new protected authority preparation failed"), err)
	}
	record.Target = SettingsSnapshot{Revision: revision + 1, Settings: old.Settings}
	record.Target.Settings.RootSHA256 = created.Fingerprint()
	record.Target.Settings.Ports = ports
	record.NewCertificate = created.RootCertificate()
	record.NewStateSHA256 = rotationDigest(newData)
	if err := record.validate(ledger.daemon, installation); err != nil {
		return nil, err
	}
	publicArchive := filepath.Join(ledger.directory, "authority-retired-"+oldRoot+".pem")
	if err := ledger.writeRotationState(publicArchive, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: oldState.RootCertificate}), true); err != nil {
		return nil, err
	}
	if err := ledger.writeRotationState(ledger.rotationArtifact(created.Fingerprint(), "old"), oldData, false); err != nil {
		return nil, err
	}
	if err := ledger.writeRotationState(ledger.rotationArtifact(created.Fingerprint(), "new"), newData, false); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(record)
	if err != nil || len(encoded) > rotationBudget {
		return nil, errors.Join(errors.New("rotation journal exceeds its budget"), err)
	}
	if err := ledger.writeRotationState(ledger.rotationPath(), encoded, false); err != nil {
		return nil, err
	}
	info := record.info()
	return &info, nil
}

func (ledger *NamespaceLedger) checkedRotation(installation, oldRoot, newRoot string) (*authorityRotation, error) {
	record, err := ledger.readRotation(installation)
	if err != nil {
		return nil, err
	}
	if record == nil || record.Old.Settings.RootSHA256 != oldRoot || record.Target.Settings.RootSHA256 != newRoot {
		return nil, errors.New("review the exact pending old and new fingerprints before resolving rotation")
	}
	return record, nil
}
func removeRotationExact(path, digest string) error {
	data, err := readRotationBytes(path, authorityStateBudget)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if rotationDigest(data) != digest {
		return errors.New("rotation artifact differs; preserve it for review")
	}
	return os.Remove(path)
}

func checkRotationExact(path, digest string, missingOK bool) error {
	data, err := readRotationBytes(path, authorityStateBudget)
	if os.IsNotExist(err) && missingOK {
		return nil
	}
	if err != nil {
		return err
	}
	if rotationDigest(data) != digest {
		return errors.New("rotation artifact differs; retain it for review")
	}
	return nil
}

func (ledger *NamespaceLedger) ResumeAuthorityRotation(store RootStore, installation, oldRoot, newRoot string, now time.Time) (out SettingsSnapshot, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return SettingsSnapshot{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return SettingsSnapshot{}, errors.New("exclusive rotation owner required")
	}
	record, err := ledger.checkedRotation(installation, oldRoot, newRoot)
	if err != nil {
		return SettingsSnapshot{}, err
	}
	if err := ledger.rotationIngressSettled(); err != nil {
		return SettingsSnapshot{}, err
	}
	if err := ledger.rotationTrustAbsent(store, installation, record); err != nil {
		return SettingsSnapshot{}, err
	}
	settings, err := ledger.readSettings()
	if err != nil {
		return SettingsSnapshot{}, err
	}
	if !reflect.DeepEqual(settings, record.Old) && !reflect.DeepEqual(settings, record.Target) {
		return SettingsSnapshot{}, errors.New("settings changed outside the pending rotation; retain all recovery artifacts")
	}
	activePath := filepath.Join(ledger.directory, authorityStateName)
	active, err := ledger.readState(activePath, authorityStateBudget)
	if err != nil {
		return SettingsSnapshot{}, err
	}
	digest := rotationDigest(active)
	if digest != record.OldStateSHA256 && digest != record.NewStateSHA256 {
		return SettingsSnapshot{}, errors.New("active authority differs from both recorded rotation states")
	}
	if digest == record.OldStateSHA256 && !reflect.DeepEqual(settings, record.Old) {
		return SettingsSnapshot{}, errors.New("target settings with old authority is not a valid rotation boundary")
	}
	newData := active
	if digest == record.OldStateSHA256 {
		backup, err := ledger.readState(ledger.rotationArtifact(newRoot, "old"), authorityStateBudget)
		if err != nil || rotationDigest(backup) != record.OldStateSHA256 {
			return SettingsSnapshot{}, errors.Join(errors.New("old authority recovery copy is missing or changed"), err)
		}
		newData, err = ledger.readState(ledger.rotationArtifact(newRoot, "new"), authorityStateBudget)
		if err != nil {
			return SettingsSnapshot{}, err
		}
	}
	if rotationDigest(newData) != record.NewStateSHA256 {
		return SettingsSnapshot{}, errors.New("prepared new authority differs from its recorded digest")
	}
	var newState authorityState
	if err := strictJSON(newData, &newState); err != nil {
		return SettingsSnapshot{}, err
	}
	if _, err := validateAuthorityPublic(newState, installation); err != nil {
		return SettingsSnapshot{}, err
	}
	if newState.RootSHA256 != newRoot {
		return SettingsSnapshot{}, errors.New("prepared authority fingerprint differs")
	}
	if err := ledger.checkState(); err != nil {
		return SettingsSnapshot{}, err
	}
	authority, err := openAuthorityState(newState, installation, newRoot, now)
	if err != nil {
		return SettingsSnapshot{}, err
	}
	if authority.RevocationHTTPPort() != record.Target.Settings.Ports.HTTP {
		return SettingsSnapshot{}, errors.New("prepared authority revocation port differs from reviewed target")
	}
	if err := ledger.checkRotationState(ledger.rotationArtifact(newRoot, "old"), record.OldStateSHA256, reflect.DeepEqual(settings, record.Target)); err != nil {
		return SettingsSnapshot{}, err
	}
	if err := ledger.checkRotationState(ledger.rotationArtifact(newRoot, "new"), record.NewStateSHA256, digest == record.NewStateSHA256); err != nil {
		return SettingsSnapshot{}, err
	}
	if digest == record.OldStateSHA256 {
		if ledger.stateStore != nil {
			if err := ledger.replaceState(activePath, newData, authorityStateBudget); err != nil {
				return SettingsSnapshot{}, err
			}
			if err := ledger.removeRotationState(ledger.rotationArtifact(newRoot, "new"), record.NewStateSHA256); err != nil {
				return SettingsSnapshot{}, err
			}
		} else {
			replace := ledger.rotationReplace
			if replace == nil {
				replace = replaceSettings
			}
			if err := replace(ledger.rotationArtifact(newRoot, "new"), activePath); err != nil {
				return SettingsSnapshot{}, err
			}
		}
	}
	if reflect.DeepEqual(settings, record.Old) {
		if _, err := ledger.storeSettingsLocked(record.Old.Revision, record.Target.Settings); err != nil {
			return SettingsSnapshot{}, err
		}
	}
	verified, err := ledger.readSettings()
	if err != nil || !reflect.DeepEqual(verified, record.Target) {
		return SettingsSnapshot{}, errors.Join(errors.New("new authority settings commit was not confirmed"), err)
	}
	if err := ledger.removeRotationState(ledger.rotationArtifact(newRoot, "old"), record.OldStateSHA256); err != nil {
		return verified, err
	}
	if err := ledger.removeRotationState(ledger.rotationArtifact(newRoot, "new"), record.NewStateSHA256); err != nil {
		return verified, err
	}
	// Journal is settled last; a failed delete remains an explicit resumable state.
	return verified, ledger.removeStateCurrent(ledger.rotationPath(), rotationBudget)
}

func (ledger *NamespaceLedger) CancelAuthorityRotation(installation, oldRoot, newRoot string) (resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return errors.New("exclusive rotation owner required")
	}
	record, err := ledger.checkedRotation(installation, oldRoot, newRoot)
	if err != nil {
		return err
	}
	settings, err := ledger.readSettings()
	if err != nil {
		return err
	}
	active, err := ledger.readState(filepath.Join(ledger.directory, authorityStateName), authorityStateBudget)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(settings, record.Old) || rotationDigest(active) != record.OldStateSHA256 {
		return errors.New("rotation already switched state; resume it instead of rolling back")
	}
	if err := ledger.checkRotationState(ledger.rotationArtifact(newRoot, "old"), record.OldStateSHA256, true); err != nil {
		return err
	}
	if err := ledger.checkRotationState(ledger.rotationArtifact(newRoot, "new"), record.NewStateSHA256, true); err != nil {
		return err
	}
	archive := filepath.Join(ledger.directory, "authority-abandoned-"+newRoot+".pem")
	if err := ledger.writeRotationState(archive, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: record.NewCertificate}), true); err != nil {
		return err
	}
	if err := ledger.removeRotationState(ledger.rotationArtifact(newRoot, "old"), record.OldStateSHA256); err != nil {
		return err
	}
	if err := ledger.removeRotationState(ledger.rotationArtifact(newRoot, "new"), record.NewStateSHA256); err != nil {
		return err
	}
	return ledger.removeStateCurrent(ledger.rotationPath(), rotationBudget)
}
