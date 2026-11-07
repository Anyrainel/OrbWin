//go:build windows

package lanhelper

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanlease"
)

func (store *NativeStore) historyLocked(id string) (ParsedHistory, error) {
	var file *os.File
	if journal := store.journals[id]; journal != nil {
		if journal.failed {
			return ParsedHistory{}, errors.New("current protected journal has an unknown write")
		}
		file = journal.file
	} else {
		var err error
		file, err = heldFile(filepath.Join(store.path, id+".jsonl"), false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
		if err != nil {
			return ParsedHistory{}, err
		}
		defer file.Close()
		if err = exactDescriptor(file, store.journalDescriptor); err != nil {
			return ParsedHistory{}, err
		}
	}
	stat, err := file.Stat()
	if err != nil || stat.Size() <= 0 || stat.Size() > protectedJournalBudget {
		return ParsedHistory{}, errors.New("bounded protected history unavailable")
	}
	data, err := io.ReadAll(io.NewSectionReader(file, 0, stat.Size()))
	if err != nil {
		return ParsedHistory{}, err
	}
	history, err := ParseProtectedHistory(data)
	if err != nil || history.Review.Stage.Base.Installation != store.installation || history.Review.Stage.Base.UserSID != store.manager.identity.UserSID {
		return ParsedHistory{}, errors.New("protected history belongs to a different owner or is incomplete")
	}
	return history, nil
}
func (store *NativeStore) tombstonesLocked() ([]lanlease.Tombstone, error) {
	directory, err := os.Open(store.path)
	if err != nil {
		return nil, err
	}
	entries, readErr := directory.ReadDir(maxProtectedReviews + 2)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) > maxProtectedReviews+1 {
		return nil, errors.New("protected history read/budget unknown")
	}
	var result []lanlease.Tombstone
	owner := lanlease.NewOwner(store.installation, store.manager.identity)
	for _, entry := range entries {
		if entry.Name() == "owner.lock" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		if entry.IsDir() || entry.Name() != id+".jsonl" || !recordID(id) {
			return nil, errors.New("foreign protected history entry preserved")
		}
		history, err := store.historyLocked(id)
		if err != nil {
			return nil, err
		}
		if history.StageIntent != nil {
			tombstone := history.StageIntent.Tombstone
			if err = owner.RestoreTombstone(tombstone); err != nil {
				return nil, err
			}
			result = append(result, tombstone)
		}
	}
	return result, nil
}
func (store *NativeStore) Tombstones() ([]lanlease.Tombstone, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || store.manager.checkPipe(store.pipe) != nil {
		return nil, errors.New("protected history owner is closed")
	}
	return store.tombstonesLocked()
}
