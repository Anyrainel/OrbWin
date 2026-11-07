//go:build windows && amd64

package lanhelper

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

type stagedFileID struct {
	Volume uint64
	ID     [16]byte
}

func (store *NativeStore) newProtectedFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: store.journalDescriptor}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL, windows.FILE_SHARE_READ, &sa, windows.CREATE_NEW, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_WRITE_THROUGH, 0)
	if err != nil {
		return nil, errors.New("fresh protected stage file refused; no overwrite/replay")
	}
	file := os.NewFile(uintptr(h), path)
	if file == nil {
		windows.CloseHandle(h)
		return nil, errors.New("protected stage handle unavailable")
	}
	if err = exactDescriptor(file, store.journalDescriptor); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// Stage derives every path/file identity from held native assets. The protected
// denial reservation flushes BEFORE exclusive directory/file creation. Failure
// never unreserves a lease/path/transaction/nonce or copies the SDK as a worker.
func (journal *ProtectedJournal) Stage(ctx context.Context) (Prepared, error) {
	store := journal.store
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || journal.failed || journal.sequence != 1 || journal.review.Purpose != "activate" || store.journals[journal.review.ReviewID] != journal || store.manager.checkPipe(store.pipe) != nil {
		return Prepared{}, errors.New("fresh approved protected stage required")
	}
	if err := journal.review.Validate(store.manager.identity, time.Now()); err != nil {
		return Prepared{}, err
	}
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	previous, err := store.tombstonesLocked()
	if err != nil {
		return Prepared{}, err
	}
	base := filepath.Dir(filepath.Dir(filepath.Dir(store.path)))
	if err = ensureNativeStageSpace(base, store.authority.assets.Assets[2].Size+nativeRuleRecordBudget+FrameBudget); err != nil {
		return Prepared{}, err
	}
	owner := lanlease.NewOwner(store.installation, store.manager.identity)
	for _, tombstone := range previous {
		if err = owner.RestoreTombstone(tombstone); err != nil {
			return Prepared{}, err
		}
	}
	binding, err := observeConsentInterface(ctx, journal.review.Stage.Base.Scope)
	if err != nil {
		return Prepared{}, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return Prepared{}, err
	}
	leaseID := hex.EncodeToString(nonce[:])
	root := filepath.Join(base, "OrbWin", "LAN")
	leaseDir := filepath.Join(root, store.installation, leaseID)
	path := filepath.Join(leaseDir, "orbwin-session.exe")
	intent := stagingIntent{Tombstone: lanlease.Tombstone{LeaseID: leaseID, WorkerPath: path, Transaction: journal.review.Transaction, Nonce: journal.review.Stage.Base.Nonce}}
	// Conflicting retained nonce/transaction refuses before any stage intent or
	// file write. A separate owner validates the new reservation shape only.
	for _, old := range previous {
		if old.Transaction == intent.Tombstone.Transaction || old.Nonce == intent.Tombstone.Nonce || old.LeaseID == leaseID || old.WorkerPath == path {
			return Prepared{}, errors.New("retained LAN stage identity cannot be reused")
		}
	}
	if err = journal.appendLocked(protectedEntry{Kind: "stage-intent", StageIntent: &intent}); err != nil {
		return Prepared{}, err
	}
	for _, directory := range []string{root, filepath.Join(root, store.installation)} {
		if err = store.directory(directory); err != nil {
			return Prepared{}, err
		}
	}
	p, _ := windows.UTF16PtrFromString(leaseDir)
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: store.descriptor}
	if err = windows.CreateDirectory(p, &sa); err != nil {
		return Prepared{}, errors.New("fresh private worker directory refused; reservation retained")
	}
	if err = store.directory(leaseDir); err != nil {
		return Prepared{}, err
	}
	worker, err := store.newProtectedFile(path)
	if err != nil {
		return Prepared{}, err
	}
	signed := store.authority.assets.Assets[2]
	n, copyErr := io.Copy(worker, io.NewSectionReader(store.authority.worker, 0, int64(signed.Size)))
	flushErr := worker.Sync()
	closeErr := worker.Close()
	if copyErr != nil || flushErr != nil || closeErr != nil || uint64(n) != signed.Size {
		return Prepared{}, errors.New("protected worker copy/flush unknown; reservation retained")
	}
	image, err := heldFile(path, false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return Prepared{}, err
	}
	defer image.Close()
	if err = exactDescriptor(image, store.journalDescriptor); err != nil {
		return Prepared{}, err
	}
	sha, err := imageDigest(image)
	if err != nil || sha != signed.SHA256 || sha != journal.review.Stage.Base.Scope.WorkerSHA256 {
		return Prepared{}, errors.New("staged dedicated worker digest differs")
	}
	var id stagedFileID
	if err = windows.GetFileInformationByHandleEx(windows.Handle(image.Fd()), windows.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id))); err != nil {
		return Prepared{}, err
	}
	spec := lanlease.Spec{Request: journal.review.Stage.Base, Publication: journal.review.Stage.Publication, LeaseID: leaseID, WorkerRoot: root, Worker: lanfirewall.WorkerBinding{Path: path, FileID: hex.EncodeToString(id.ID[:]), VolumeSerial: strconv.FormatUint(id.Volume, 10), SHA256: sha, InterfaceAlias: binding.Alias}}
	lease, err := owner.Prepare(spec, time.Now())
	if err != nil {
		return Prepared{}, err
	}
	prepared := Prepared{Spec: spec, LeaseSHA: lease.Snapshot(time.Now()).LeaseSHA, Sequence: 2}
	boot, err := store.newProtectedFile(filepath.Join(leaseDir, "lease-v2.json"))
	if err != nil {
		return Prepared{}, err
	}
	writeErr := Write(boot, Frame{Version: 2, Kind: "prepared", Sequence: 2, LeaseSHA: prepared.LeaseSHA, Prepared: &prepared})
	flushErr = boot.Sync()
	closeErr = boot.Close()
	if writeErr != nil || flushErr != nil || closeErr != nil {
		return Prepared{}, errors.New("protected worker boot flush unknown; reservation retained")
	}
	if err = ctx.Err(); err != nil {
		return Prepared{}, err
	}
	if err = journal.appendLocked(protectedEntry{Kind: "prepared", Prepared: &prepared}); err != nil {
		return Prepared{}, err
	}
	return prepared, nil
}
