package localnet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

type relayRecord struct {
	Version int         `json:"version"`
	Daemon  string      `json:"daemon"`
	Intent  RelayIntent `json:"intent"`
	ID      string      `json:"-"`
}

func (ledger *NamespaceLedger) relayPath(session string) string {
	return filepath.Join(ledger.directory, "relay-"+session+".receipt")
}
func (ledger *NamespaceLedger) BeginRelay(intent RelayIntent) (resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return errors.New("local-services owner is closed")
	}
	if err := intent.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(relayRecord{Version: 1, Daemon: ledger.daemon, Intent: intent})
	if err != nil {
		return err
	}
	return ledger.createState(ledger.relayPath(intent.Session), append(data, '\n'))
}
func (ledger *NamespaceLedger) CreatedRelay(intent RelayIntent, id string) (resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return errors.New("local-services owner is closed")
	}
	if err := intent.validate(); err != nil {
		return err
	}
	if !fullID(id) {
		return errors.New("accepted full relay ID required")
	}
	record, err := ledger.readRelay(ledger.relayPath(intent.Session))
	if err != nil {
		return err
	}
	if record.ID != "" || !reflect.DeepEqual(record.Intent, intent) {
		return errors.New("relay create differs from immutable pending intent")
	}
	before, err := ledger.readState(ledger.relayPath(intent.Session), 128<<10)
	if err != nil {
		return err
	}
	data, err := json.Marshal(acceptedNamespace{ID: id})
	if err != nil {
		return err
	}
	return ledger.appendState(ledger.relayPath(intent.Session), before, append(data, '\n'))
}
func (ledger *NamespaceLedger) readRelay(path string) (relayRecord, error) {
	var record relayRecord
	data, err := ledger.readState(path, 128<<10)
	if err != nil {
		return record, err
	}
	if len(data) > 128<<10 || len(data) == 0 || data[len(data)-1] != '\n' {
		return record, errors.New("partial or changed relay record; retain for review")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) < 1 || len(lines) > 2 {
		return record, errors.New("unexpected relay record length")
	}
	if err = strictJSON(lines[0], &record); err != nil {
		return record, err
	}
	if record.Version != 1 || record.Daemon != ledger.daemon || record.Intent.validate() != nil || filepath.Base(path) != "relay-"+record.Intent.Session+".receipt" {
		return record, errors.New("relay record filename, daemon or intent differs")
	}
	if len(lines) == 2 {
		var accepted acceptedNamespace
		if err = strictJSON(lines[1], &accepted); err != nil {
			return record, err
		}
		if !fullID(accepted.ID) {
			return record, errors.New("invalid accepted relay ID")
		}
		record.ID = accepted.ID
	}
	return record, nil
}
func (ledger *NamespaceLedger) relayRecords() (out []relayRecord, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return nil, err
	}
	defer ledger.finishState(&resultErr)
	return ledger.readRelayRecords()
}
func (ledger *NamespaceLedger) readRelayRecords() ([]relayRecord, error) {
	if !ledger.owned() {
		return nil, errors.New("local-services owner is closed")
	}
	entries, err := ledger.stateEntries()
	if err != nil {
		return nil, err
	}
	var records []relayRecord
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "relay-") || !strings.HasSuffix(entry.Name(), ".receipt") {
			continue
		}
		if len(records) >= 32 || entry.IsDir() {
			return nil, errors.New("relay receipt budget exceeded or invalid directory entry")
		}
		record, err := ledger.readRelay(filepath.Join(ledger.directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("review %s: %w", entry.Name(), err)
		}
		records = append(records, record)
	}
	return records, nil
}

// Only exact confirmed removal/known-ID absence authorizes settlement. Unknown
// create acceptance cannot be forgotten merely because its name is absent now.
func (ledger *NamespaceLedger) settleRelay(record relayRecord) (resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return errors.New("local-services owner is closed")
	}
	if !fullID(record.ID) {
		return errors.New("unknown relay acceptance must remain recorded")
	}
	current, err := ledger.readRelay(ledger.relayPath(record.Intent.Session))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, record) {
		return errors.New("relay receipt changed before settlement")
	}
	return ledger.removeStateCurrent(ledger.relayPath(record.Intent.Session), 128<<10)
}
