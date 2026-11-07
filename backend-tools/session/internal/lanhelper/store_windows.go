//go:build windows

package lanhelper

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NativeStore owns one non-shareable installation writer and held ancestor
// handles. Existing foreign/writable/reparse roots are preserved and refused.
// It is opened ONLY inside the authenticated, elevated one-shot helper.
type NativeStore struct {
	mu                 sync.Mutex
	manager            *NativeManager
	pipe               windows.Handle
	installation, path string
	parents            []*os.File
	lock               *os.File
	descriptor         *windows.SECURITY_DESCRIPTOR
	journalDescriptor  *windows.SECURITY_DESCRIPTOR
	journals           map[string]*ProtectedJournal
	closed             bool
	approvedReviewSHA  string
	approvedPurpose    string
	authority          *AuthorizedInstallation
}
type ProtectedJournal struct {
	store  *NativeStore
	file   *os.File
	review OwnerReview
	protectedJournalState
}

func protectedSDDL(sid string, directory bool) string {
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	return "O:BAD:P(A;" + inherit + ";FA;;;SY)(A;" + inherit + ";FA;;;BA)(A;" + inherit + ";FRFX;;;" + sid + ")"
}
func exactDescriptor(file *os.File, want *windows.SECURITY_DESCRIPTOR) error {
	got, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || got.String() != want.String() {
		return errors.New("protected helper descriptor differs")
	}
	return nil
}
func (store *NativeStore) directory(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: store.descriptor}
	if err = windows.CreateDirectory(p, &sa); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	file, err := heldFile(path, true, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return err
	}
	if err = exactDescriptor(file, store.descriptor); err != nil {
		file.Close()
		return err
	}
	store.parents = append(store.parents, file)
	return nil
}
func OpenNativeStore(authority *AuthorizedInstallation, consent *scopeConsent) (store *NativeStore, err error) {
	if authority == nil || authority.worker == nil {
		return nil, errors.New("independently authorized protected installation required")
	}
	manager, installation := authority.manager, authority.installation
	if manager == nil {
		return nil, errors.New("native manager required")
	}
	approvedSHA, err := consent.consume(authority)
	if err != nil {
		return nil, err
	}
	pipe := manager.pipe
	if !recordID(installation) || manager == nil {
		return nil, errors.New("native manager and fixed installation required")
	}
	if err = manager.checkPipe(pipe); err != nil {
		return nil, err
	}
	if _, session, e := nativeToken(windows.CurrentProcess(), true); e != nil || session != manager.session {
		return nil, errors.New("elevated same-session helper required")
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil || len(base) < 3 || base[1:3] != `:\` || strings.ContainsAny(base, `/%*?[]`+"`\x00") {
		return nil, errors.New("fixed local ProgramData root required")
	}
	store = &NativeStore{manager: manager, pipe: pipe, installation: installation, journals: map[string]*ProtectedJournal{}, approvedReviewSHA: approvedSHA, approvedPurpose: consent.purpose, authority: authority}
	owned := store
	defer func() {
		if err != nil {
			owned.Close()
			store = nil
		}
	}()
	// Hold every existing ancestor without delete sharing before creating any
	// child. We never repair a pre-existing security descriptor or follow links.
	for current := base; ; current = filepath.Dir(current) {
		file, e := heldFile(current, true, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
		if e != nil {
			return nil, e
		}
		store.parents = append(store.parents, file)
		if filepath.Dir(current) == current {
			break
		}
	}
	store.descriptor, err = windows.SecurityDescriptorFromString(protectedSDDL(manager.identity.UserSID, true))
	if err != nil {
		return nil, err
	}
	store.journalDescriptor, err = windows.SecurityDescriptorFromString(protectedSDDL(manager.identity.UserSID, false))
	if err != nil {
		return nil, err
	}
	for _, path := range []string{filepath.Join(base, "OrbWin"), filepath.Join(base, "OrbWin", "LANRecords"), filepath.Join(base, "OrbWin", "LANRecords", installation)} {
		if err = store.directory(path); err != nil {
			return nil, err
		}
	}
	store.path = filepath.Join(base, "OrbWin", "LANRecords", installation)
	lockPath := filepath.Join(store.path, "owner.lock")
	p, _ := windows.UTF16PtrFromString(lockPath)
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: store.journalDescriptor}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL, 0, &sa, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_WRITE_THROUGH, 0)
	if err != nil {
		return nil, errors.New("another protected helper writer is active or lock is unavailable")
	}
	store.lock = os.NewFile(uintptr(h), lockPath)
	if store.lock == nil {
		windows.CloseHandle(h)
		return nil, errors.New("protected writer lock unavailable")
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return nil, errors.New("protected writer lock kind differs")
	}
	if err = exactDescriptor(store.lock, store.journalDescriptor); err != nil {
		return nil, err
	}
	return store, nil
}
func (store *NativeStore) PersistReview(record OwnerReview, now time.Time) (journal *ProtectedJournal, err error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || store.manager.checkPipe(store.pipe) != nil {
		return nil, errors.New("protected helper owner is closed")
	}
	if record.Stage.Base.Installation != store.installation {
		return nil, errors.New("review installation differs from native protected writer")
	}
	if record.ReviewSHA256 != store.approvedReviewSHA || record.Purpose != store.approvedPurpose {
		return nil, errors.New("protected writer belongs to a different native scope consent")
	}
	if err = record.Validate(store.manager.identity, now); err != nil {
		return nil, err
	}
	// Count retained denial records without following caller-selected entries.
	directory, err := os.Open(store.path)
	if err != nil {
		return nil, err
	}
	entries, readErr := directory.ReadDir(maxProtectedReviews + 2)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) > maxProtectedReviews {
		return nil, errors.New("protected LAN denial history exhausted or unreadable")
	}
	for _, entry := range entries {
		if entry.Name() == "owner.lock" {
			continue
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") || !recordID(strings.TrimSuffix(entry.Name(), ".jsonl")) {
			return nil, errors.New("foreign protected history entry preserved")
		}
	}
	path := filepath.Join(store.path, record.ReviewID+".jsonl")
	p, _ := windows.UTF16PtrFromString(path)
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: store.journalDescriptor}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.READ_CONTROL, windows.FILE_SHARE_READ, &sa, windows.CREATE_NEW, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_WRITE_THROUGH, 0)
	if err != nil {
		return nil, errors.New("fresh protected review record refused; never replay or overwrite")
	}
	file := os.NewFile(uintptr(h), path)
	if file == nil {
		windows.CloseHandle(h)
		return nil, errors.New("protected review file unavailable")
	}
	journal = &ProtectedJournal{store: store, file: file, review: record}
	store.journals[record.ReviewID] = journal
	if err = exactDescriptor(file, store.journalDescriptor); err != nil {
		return nil, err
	}
	if err = journal.appendLocked(protectedEntry{Kind: "owner-review", Review: &record}); err != nil {
		return nil, err
	}
	return journal, nil
}
func (journal *ProtectedJournal) appendLocked(entry protectedEntry) error {
	return journal.protectedJournalState.append(entry, journal.file.Write, journal.file.Sync)
}
func (store *NativeStore) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil
	}
	store.closed = true
	var err error
	for _, journal := range store.journals {
		err = errors.Join(err, journal.file.Close())
	}
	if store.lock != nil {
		err = errors.Join(err, store.lock.Close())
		store.lock = nil
	}
	for index := len(store.parents) - 1; index >= 0; index-- {
		err = errors.Join(err, store.parents[index].Close())
	}
	store.parents = nil
	return err
}
