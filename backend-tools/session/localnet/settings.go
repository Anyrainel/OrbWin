package localnet

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
)

const settingsName = "settings-v1.json"
const settingsBudget = 128 << 10

type ServiceSettings struct {
	Enabled     bool                `json:"enabled"`
	Ports       ServicePorts        `json:"ports"`
	RootSHA256  string              `json:"rootSha256,omitempty"`
	Preferences []ServicePreference `json:"preferences,omitempty"`
	Networks    []string            `json:"networks,omitempty"`
}
type SettingsSnapshot struct {
	Revision uint64          `json:"revision"`
	Settings ServiceSettings `json:"settings"`
}
type settingsRecord struct {
	Version int    `json:"version"`
	Daemon  string `json:"daemon"`
	SettingsSnapshot
}

func DefaultSettings() ServiceSettings { return ServiceSettings{Ports: DefaultServicePorts()} }
func (settings ServiceSettings) validate() error {
	if err := settings.Ports.validate(); err != nil {
		return err
	}
	if (settings.Enabled && settings.RootSHA256 == "") || (settings.RootSHA256 != "" && !fullID(settings.RootSHA256)) {
		return errors.New("reviewed local authority fingerprint required before enabling")
	}
	if _, err := preferencesBySelector(settings.Preferences); err != nil {
		return err
	}
	if len(settings.Networks) > 8 {
		return errors.New("selected client networks exceed eight")
	}
	previous := ""
	for _, id := range settings.Networks {
		if !fullID(id) || id <= previous {
			return errors.New("unique sorted full client network IDs required")
		}
		previous = id
	}
	return nil
}
func (ledger *NamespaceLedger) LoadSettings() (out SettingsSnapshot, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return SettingsSnapshot{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return SettingsSnapshot{}, errors.New("exclusive local-services owner required")
	}
	return ledger.readSettings()
}
func (ledger *NamespaceLedger) readSettings() (SettingsSnapshot, error) {
	path := filepath.Join(ledger.directory, settingsName)
	data, err := ledger.readState(path, settingsBudget)
	if os.IsNotExist(err) {
		return SettingsSnapshot{Settings: DefaultSettings()}, nil
	}
	if err != nil {
		return SettingsSnapshot{}, err
	}
	var record settingsRecord
	if err = strictJSON(data, &record); err != nil {
		return SettingsSnapshot{}, err
	}
	if record.Version != 1 || record.Daemon != ledger.daemon || record.Revision == 0 {
		return SettingsSnapshot{}, errors.New("settings version, daemon or revision differs")
	}
	if err = record.Settings.validate(); err != nil {
		return SettingsSnapshot{}, err
	}
	return record.SettingsSnapshot, nil
}

// Reload after any write error before retrying: a replacement may have committed
// before durability confirmation or draft cleanup failed. CAS prevents replaying
// a stale revision. No unknown file is deleted or silently reset.
func (ledger *NamespaceLedger) StoreSettings(expected uint64, settings ServiceSettings) (out SettingsSnapshot, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return SettingsSnapshot{}, err
	}
	defer ledger.finishState(&resultErr)
	return ledger.storeSettingsLocked(expected, settings)
}
func (ledger *NamespaceLedger) storeSettingsLocked(expected uint64, settings ServiceSettings) (snapshot SettingsSnapshot, resultErr error) {
	if !ledger.owned() {
		return snapshot, errors.New("exclusive local-services owner required")
	}
	if err := settings.validate(); err != nil {
		return snapshot, err
	}
	current, err := ledger.readSettings()
	if err != nil {
		return snapshot, err
	}
	if current.Revision != expected {
		return snapshot, errors.New("local-services settings changed; reload before applying")
	}
	if expected == math.MaxUint64 {
		return snapshot, errors.New("settings revision exhausted")
	}
	record := settingsRecord{Version: 1, Daemon: ledger.daemon, SettingsSnapshot: SettingsSnapshot{Revision: expected + 1, Settings: settings}}
	data, err := marshalSettings(record)
	if err != nil {
		return snapshot, err
	}
	if ledger.stateStore != nil {
		if err := ledger.replaceState(filepath.Join(ledger.directory, settingsName), data, settingsBudget); err != nil {
			return snapshot, err
		}
		var committed settingsRecord
		if err := strictJSON(data, &committed); err != nil {
			return snapshot, err
		}
		return committed.SettingsSnapshot, nil
	}
	draft, err := os.CreateTemp(ledger.directory, ".settings-*.partial")
	if err != nil {
		return snapshot, err
	}
	defer func() {
		if cleanupErr := os.Remove(draft.Name()); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			resultErr = errors.Join(resultErr, cleanupErr)
		}
	}()
	if err = draft.Chmod(0600); err == nil {
		_, err = draft.Write(data)
	}
	if err == nil {
		err = draft.Sync()
	}
	closed := draft.Close()
	if err != nil || closed != nil {
		return snapshot, errors.Join(err, closed)
	}
	if err = replaceSettings(draft.Name(), filepath.Join(ledger.directory, settingsName)); err != nil {
		return snapshot, err
	}
	// Decode the committed bytes so returned slices never alias caller inputs.
	var committed settingsRecord
	if err = strictJSON(data, &committed); err != nil {
		return snapshot, err
	}
	return committed.SettingsSnapshot, nil
}

func marshalSettings(record settingsRecord) ([]byte, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	if len(data) > settingsBudget {
		return nil, errors.New("settings exceed their serialized budget")
	}
	return data, nil
}
