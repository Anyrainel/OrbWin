//go:build windows && amd64

package lanhelper

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanprovider"
)

// This native source metadata is private protected state, never an IPC input.
// Provider's read-only recovery bridge mirrors its exact fixed schema.
type nativeRuleMetadata struct {
	Version         int                        `json:"version"`
	ReviewID        string                     `json:"reviewId"`
	ReviewSHA256    string                     `json:"reviewSha256"`
	OperationID     string                     `json:"operationId"`
	RuntimeInstance string                     `json:"runtimeInstance"`
	Prepared        Prepared                   `json:"prepared"`
	Child           lanfirewall.CallerIdentity `json:"child"`
	Namespace       nativeRuleNamespace        `json:"namespace"`
}
type nativeRuleNamespace struct {
	ContainerID string `json:"containerId"`
	DaemonID    string `json:"daemonId"`
}
type nativeRuleJournal struct {
	mu       sync.Mutex
	file     *os.File
	leaseSHA string
	state    nativeRuleWriteState
	closed   bool
}

func (journal *nativeRuleJournal) FlushOwnedRuleEvent(event lanprovider.OwnedRuleEvent) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed || event.LeaseSHA() != journal.leaseSHA {
		return errors.New("native rule event belongs to a different protected owner")
	}
	data, err := event.ProtectedBytes()
	if err != nil {
		return err
	}
	return journal.state.append(event.Kind(), data, journal.file.Write, journal.file.Sync)
}
func (journal *nativeRuleJournal) Close() error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if journal.closed {
		return nil
	}
	journal.closed = true
	return journal.file.Close()
}
func ensureNativeStageSpace(path string, required uint64) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var available, total, free uint64
	if err = windows.GetDiskFreeSpaceEx(p, &available, &total, &free); err != nil || available < required {
		return errors.New("insufficient confirmed space for bounded protected LAN records/assets")
	}
	return nil
}

// Metadata and worst bounded event capacity are checked BEFORE provider entry.
// This is a free-space observation, not a filesystem reservation: later disk
// failure still poisons the writer and leaves protected intent unknown.
func (store *NativeStore) newRuleJournal(record OwnerReview, prepared Prepared, child *NativeManager, namespaceID string) (*nativeRuleJournal, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || child == nil || !ValidSHA(namespaceID) || namespaceID == prepared.Spec.Publication.ContainerID || record.ReviewSHA256 != store.approvedReviewSHA || record.Purpose != "activate" || record.Stage.Base.Digest() != prepared.Spec.Request.Digest() || child.identity.ImageSHA256 != prepared.Spec.Worker.SHA256 || store.manager.checkPipe(store.pipe) != nil {
		return nil, errors.New("exact consented native rule owner required")
	}
	base := filepath.Dir(filepath.Dir(filepath.Dir(store.path)))
	if err := ensureNativeStageSpace(base, nativeRuleRecordBudget+FrameBudget); err != nil {
		return nil, err
	}
	root := filepath.Join(base, "OrbWin", "LANRules")
	directory := filepath.Join(root, store.installation)
	for _, path := range []string{root, directory} {
		if err := store.directory(path); err != nil {
			return nil, err
		}
	}
	meta := nativeRuleMetadata{Version: 2, ReviewID: record.ReviewID, ReviewSHA256: record.ReviewSHA256, OperationID: record.OperationID, RuntimeInstance: record.RuntimeInstance, Prepared: prepared, Child: child.identity, Namespace: nativeRuleNamespace{ContainerID: namespaceID, DaemonID: prepared.Spec.Publication.Daemon}}
	data, err := json.Marshal(meta)
	if err != nil || len(data) > FrameBudget {
		return nil, errors.New("native protected owner metadata budget exceeded")
	}
	metadata, err := store.newProtectedFile(filepath.Join(directory, prepared.LeaseSHA+".owner.json"))
	if err != nil {
		return nil, err
	}
	n, writeErr := metadata.Write(data)
	flushErr := metadata.Sync()
	closeErr := metadata.Close()
	if writeErr != nil || n != len(data) || flushErr != nil || closeErr != nil {
		return nil, errors.New("native owner metadata flush unknown; no provider dispatch")
	}
	file, err := store.newProtectedFile(filepath.Join(directory, prepared.LeaseSHA+".jsonl"))
	if err != nil {
		return nil, err
	}
	return &nativeRuleJournal{file: file, leaseSHA: prepared.LeaseSHA}, nil
}
