package localnet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The ledger's exclusive process handle is released by Windows after a crash.
// An immutable flushed intent precedes Create; a flushed accepted full ID
// precedes Start. Unknown accepted creates and damaged records are retained.
// Only known-ID absence or exact owned removal permits record deletion.
// The caller supplies a validated persistent-home subdirectory, not a UI path.
type NamespaceLedger struct {
	mu                 sync.Mutex
	directory, daemon  string
	lease              *os.File
	closed             bool
	closeErr           error
	stateStore         serviceStateStore
	stateCancel        context.CancelFunc
	trustReplace       func(string, string) error // Controlled filesystem-acknowledgement fixtures only.
	rotationReplace    func(string, string) error // Controlled rotation interruption fixtures only.
	wslRegistryFactory func() (WSLTrustRegistry, error)
	pendingImport      *toolsImageReceipt // actual producer's possibly live untagged engine import
}
type namespaceRecord struct {
	Version int             `json:"version"`
	Daemon  string          `json:"daemon"`
	Intent  NamespaceIntent `json:"intent"`
	ID      string          `json:"-"`
}
type acceptedNamespace struct {
	ID string `json:"id"`
}

// OpenNamespaceLedger is the explicit plain-filesystem fixture seam. Windows
// mode bits here do not establish native storage authority. The SDK production
// path must use OpenManagedNamespaceLedger with its retained sdkstate.Root.
func OpenNamespaceLedger(directory, daemon string) (*NamespaceLedger, error) {
	if !filepath.IsAbs(directory) || daemon == "" || len(daemon) > 256 || strings.ContainsAny(daemon, "\x00\r\n") {
		return nil, errors.New("persistent local-services directory and recorded daemon required")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	lease, err := serviceLease(filepath.Join(directory, "controller.lock"))
	if err != nil {
		return nil, fmt.Errorf("local-services controller ownership unavailable: %w", err)
	}
	return &NamespaceLedger{directory: directory, daemon: daemon, lease: lease}, nil
}
func (ledger *NamespaceLedger) Close() error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.closed {
		return ledger.closeErr
	}
	if ledger.stateStore != nil && ledger.pendingImport != nil {
		return errors.New("native SDK image import remains unconfirmed; actual ledger owner retained")
	}
	ledger.closed = true
	if ledger.stateStore != nil {
		ledger.closeErr = ledger.stateStore.Close()
		return ledger.closeErr
	}
	if ledger.lease == nil {
		return nil
	}
	ledger.closeErr = ledger.lease.Close()
	return ledger.closeErr
}
func (ledger *NamespaceLedger) owned() bool {
	if ledger.stateStore != nil {
		return !ledger.closed && ledger.daemon != "" && ledger.stateStore.Check() == nil
	}
	return !ledger.closed && ledger.lease != nil && filepath.IsAbs(ledger.directory) && ledger.daemon != ""
}
func (ledger *NamespaceLedger) path(session string) string {
	return filepath.Join(ledger.directory, "namespace-"+session+".jsonl")
}
func (ledger *NamespaceLedger) Begin(intent NamespaceIntent) (resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return errors.New("local-services ledger is closed")
	}
	if err := intent.validate(); err != nil {
		return err
	}
	existing, err := ledger.readRecords()
	if err != nil {
		return err
	}
	if len(existing) >= 512 {
		return errors.New("local-services ownership directory is full; review retained records")
	}
	data, err := json.Marshal(namespaceRecord{Version: 1, Daemon: ledger.daemon, Intent: intent})
	if err != nil {
		return err
	}
	return ledger.createState(ledger.path(intent.Session), append(data, '\n'))
}
func (ledger *NamespaceLedger) Created(intent NamespaceIntent, id string) (resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return errors.New("local-services ledger is closed")
	}
	if err := intent.validate(); err != nil {
		return err
	}
	if !fullID(id) || id == intent.ContainerID {
		return errors.New("accepted full namespace helper ID required")
	}
	record, err := ledger.read(ledger.path(intent.Session))
	if err != nil {
		return err
	}
	if record.Intent != intent || record.ID != "" {
		return errors.New("namespace create differs from its immutable pending intent")
	}
	before, err := ledger.readState(ledger.path(intent.Session), 8192)
	if err != nil {
		return err
	}
	data, err := json.Marshal(acceptedNamespace{ID: id})
	if err != nil {
		return err
	}
	return ledger.appendState(ledger.path(intent.Session), before, append(data, '\n'))
}
func strictJSON(data []byte, result any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("unexpected local-services record suffix")
	}
	return nil
}
func (ledger *NamespaceLedger) read(path string) (namespaceRecord, error) {
	var record namespaceRecord
	data, err := ledger.readState(path, 8192)
	if err != nil {
		return record, err
	}
	if len(data) > 8192 || len(data) == 0 || data[len(data)-1] != '\n' {
		return record, errors.New("damaged/partial local-services ownership record; retain for review")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) < 1 || len(lines) > 2 {
		return record, errors.New("unexpected local-services ownership record length")
	}
	if err := strictJSON(lines[0], &record); err != nil {
		return record, err
	}
	if record.Version != 1 || record.Daemon != ledger.daemon || record.Intent.validate() != nil || filepath.Base(path) != "namespace-"+record.Intent.Session+".jsonl" {
		return record, errors.New("local-services record does not match its filename/daemon/intent")
	}
	if len(lines) == 2 {
		var accepted acceptedNamespace
		if err := strictJSON(lines[1], &accepted); err != nil {
			return record, err
		}
		if !fullID(accepted.ID) || accepted.ID == record.Intent.ContainerID {
			return record, errors.New("invalid accepted namespace helper identity")
		}
		record.ID = accepted.ID
	}
	return record, nil
}
func (ledger *NamespaceLedger) records() (out []namespaceRecord, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return nil, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return nil, errors.New("local-services ledger is closed")
	}
	return ledger.readRecords()
}
func (ledger *NamespaceLedger) readRecords() ([]namespaceRecord, error) {
	entries, err := ledger.stateEntries()
	if err != nil {
		return nil, err
	}
	var records []namespaceRecord
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		if len(records) >= 512 || entry.IsDir() {
			return nil, errors.New("local-services ownership directory exceeds supported records or contains an invalid entry")
		}
		record, err := ledger.read(filepath.Join(ledger.directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("review %s: %w", entry.Name(), err)
		}
		records = append(records, record)
	}
	return records, nil
}

// Called only after an exact recorded ID is removed/not found. Pending intents
// without an accepted ID are never forgotten merely because their name is absent.
func (ledger *NamespaceLedger) settled(record namespaceRecord) (resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return errors.New("local-services ledger is closed")
	}
	current, err := ledger.read(ledger.path(record.Intent.Session))
	if err != nil {
		return err
	}
	if record.ID == "" || current != record {
		return errors.New("cannot settle an unknown or changed namespace create")
	}
	return ledger.removeStateCurrent(ledger.path(record.Intent.Session), 8192)
}
