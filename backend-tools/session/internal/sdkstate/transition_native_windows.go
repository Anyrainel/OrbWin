//go:build windows

package sdkstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"runtime"
	"sort"
	"time"
	"unsafe"
)

type coldSourceFile struct {
	name, id string
	file     *os.File
	data     []byte
	hash     [32]byte
}
type nativeColdBackend struct {
	authority                            *nativeColdAuthority
	r                                    *nativeRoot
	source                               []coldSourceFile
	snapshot                             *ColdSnapshot
	fence                                *nativeRecord
	quarantine                           object
	receipt                              TransitionReceipt
	ipc                                  *coldIPCState
	runtime                              ColdRuntimeParticipant
	rootCapability                       *Root
	commitRecord                         *nativeRecord
	supervisorOffer                      *ColdSupervisorOffer
	leafMaster                           *nativeColdLANMaster
	joinedSource                         *ColdJoinedSource
	closePrepared, rootClosed, closed    bool
	controllerFile                       *os.File
	admitted, created, finished, unknown bool
}

func (b *nativeColdBackend) bindRuntime(root *Root, participant ColdRuntimeParticipant) error {
	if b == nil || !b.finished || b.unknown || b.runtime != nil || b.ipc != nil || root == nil || root.s == nil || !nativeColdRuntimeOrigin(participant) || participant.RootBacking() != root {
		return ErrRecoveryRequired
	}
	backend, ok := root.s.b.(*transitionBackend)
	if !ok || backend.cold != b || b.leafMaster == nil || b.leafMaster.unknown || b.leafMaster.closed || b.leafMaster.closeAttempted || b.leafMaster.factory == nil || participant.LeafFactoryBacking() != b.leafMaster.factory || b.leafMaster.factory.RootBacking() != root {
		return ErrRecoveryRequired
	}
	b.runtime, b.rootCapability = participant, root
	return nil
}

func (b *nativeColdBackend) reserveIPC(ctx context.Context, receipt TransitionReceipt) (*coldIPCState, error) {
	if b.check(ctx) != nil || !b.finished || b.ipc != nil || b.authority.pipe == nil || b.runtime == nil || b.rootCapability == nil || b.runtime.RootBacking() != b.rootCapability {
		return nil, ErrRecoveryRequired
	}
	want, e := b.receipt.Digest()
	got, f := receipt.Digest()
	if e != nil || f != nil || want != got {
		return nil, ErrRecoveryRequired
	}
	b.ipc = &coldIPCState{listener: &coldIPCListener{Listener: b.authority.pipe}}
	return b.ipc, nil
}

func (a *nativeColdAuthority) begin(ctx context.Context) (coldBackend, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.used || a.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	a.used = true
	return &nativeColdBackend{authority: a, r: &nativeRoot{sid: a.sid, home: a.home, installation: a.grant.Scope.Installation, daemon: a.grant.Scope.Daemon, session: a.session}}, nil
}
func (b *nativeColdBackend) check(ctx context.Context) error {
	if b == nil || b.unknown || b.authority.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	if b.fence != nil && b.fence.check() != nil {
		return ErrRecoveryRequired
	}
	if b.created && (b.r.owner == nil || b.r.owner.check() != nil) {
		return ErrRecoveryRequired
	}
	if b.finished && b.r.check() != nil {
		return ErrRecoveryRequired
	}
	return nil
}
func (b *nativeColdBackend) capture(ctx context.Context) (*ColdSnapshot, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if b.check(ctx) != nil || b.snapshot != nil {
		return nil, ErrRecoveryRequired
	}
	names, err := directoryNames(ctx, b.authority.legacy)
	if err != nil || len(names) > maxEntries+1 {
		return nil, ErrState
	}
	sort.Strings(names)
	s := &ColdSnapshot{scope: b.authority.grant.Scope, records: map[string][]byte{}}
	var total int64
	for _, name := range names {
		if name == "controller.lock" {
			continue
		}
		budget := recordBudget(name)
		if budget == 0 || !bounded(ctx) {
			return nil, ErrState
		}
		h, err := openColdSource(b.authority.legacy, name, true)
		if err != nil {
			return nil, ErrState
		}
		f := os.NewFile(uintptr(h), "held opaque legacy SDK record")
		b.source = append(b.source, coldSourceFile{name: name, file: f})
		if legacyTransitionObject(h, b.authority.sid, false) != nil {
			return nil, ErrState
		}
		info, err := f.Stat()
		if err != nil || info.Size() < 0 || info.Size() > budget || total+info.Size() > maxReadBytes {
			return nil, ErrState
		}
		data, err := io.ReadAll(io.LimitReader(f, budget+1))
		if err != nil || int64(len(data)) != info.Size() {
			clear(data)
			return nil, ErrState
		}
		id, err := snapshot(h, false)
		if err != nil {
			clear(data)
			return nil, ErrState
		}
		file := &b.source[len(b.source)-1]
		file.id = id
		file.data = data
		file.hash = sha256.Sum256(data)
		s.records[name] = data
		total += int64(len(data))
	}
	if b.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	b.snapshot = s
	return s, nil
}
func (b *nativeColdBackend) admit(ctx context.Context) error {
	if b.check(ctx) != nil || b.snapshot == nil || b.admitted {
		return ErrRecoveryRequired
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r := b.r
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return ErrState
	}
	r.token = token
	if r.walkProfile(ctx, b.authority.profile) != nil {
		return ErrState
	}
	sd, err := privateSD(r.sid, true)
	if err != nil {
		return ErrState
	}
	h, err := ntOpen(windows.Handle(r.chain[len(r.chain)-1].file.Fd()), ".orbwin-private", true, windows.FILE_OPEN_IF, sd, false, false, false)
	if err != nil || r.hold(h, true, true) != nil {
		return ErrState
	}
	r.parent = h
	// Never let an interrupted fresh Create or prior import become this import.
	for _, name := range []string{"services", "services.create-intent", "services.migration"} {
		probe, e := ntOpen(r.parent, name, name == "services", windows.FILE_OPEN, nil, false, false, false)
		if e == nil {
			n, _ := r.record(probe)
			r.partials = append(r.partials, n)
			return ErrRecoveryRequired
		}
		if e != windows.STATUS_OBJECT_NAME_NOT_FOUND {
			return ErrRecoveryRequired
		}
	}
	legacyID, err := snapshot(b.authority.legacy, true)
	if err != nil {
		return ErrState
	}
	controllerID, err := snapshot(b.authority.controller, false)
	if err != nil {
		return ErrState
	}
	b.receipt = TransitionReceipt{Schema: 1, Scope: b.authority.grant.Scope, LegacyRoot: legacyID, LegacyController: controllerID}
	var inventory []TransitionFile
	for _, f := range b.source {
		inventory = append(inventory, TransitionFile{Name: f.name, Source: f.id, Size: uint64(len(f.data)), SHA256: f.hash, Disposition: transitionDisposition(f.name)})
	}
	data, err := json.Marshal(struct {
		Schema                 int
		Scope                  TransitionScope
		LegacyRoot, Controller string
		Source                 []TransitionFile
	}{1, b.receipt.Scope, legacyID, controllerID, inventory})
	if err != nil || len(data) > 2<<20 {
		return ErrState
	}
	r.admitted = true
	b.admitted = true
	b.unknown = true
	b.fence, err = r.createFile(ctx, r.parent, "services.migration", data)
	if err != nil || b.fence == nil || b.fence.check() != nil {
		return ErrRecoveryRequired
	}
	b.receipt.Fence = b.fence.o.id
	b.unknown = false
	return nil
}
func (b *nativeColdBackend) seal(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if b.check(ctx) != nil || !b.admitted || b.created {
		return ErrRecoveryRequired
	}
	b.unknown = true
	for i, f := range b.source {
		if !bounded(ctx) {
			return ErrRecoveryRequired
		}
		h := windows.Handle(f.file.Fd())
		if sealColdObject(h, b.authority.sid, false) != nil {
			return ErrRecoveryRequired
		}
		after, err := snapshot(h, false)
		if err != nil || after != f.id {
			return ErrRecoveryRequired
		}
		data, _ := json.Marshal(struct{ Name, ID string }{f.name, after})
		record, err := b.r.createFile(ctx, b.r.parent, "services.seal-"+b.receipt.Scope.Transaction+"-"+coldNumber(i+1), data)
		if err != nil || record == nil {
			return ErrRecoveryRequired
		}
	}
	// The compatible writer lock has a metadata-only WRITE_DAC observation, so
	// its original data-sharing reservation is never closed/exchanged for sealing.
	controller, err := openColdSource(b.authority.legacy, "controller.lock", false)
	if err != nil {
		return ErrRecoveryRequired
	}
	controllerFile := os.NewFile(uintptr(controller), "legacy SDK lock ACL observation")
	b.authority.files = append(b.authority.files, controllerFile)
	if sealColdObject(controller, b.authority.sid, false) != nil || sealColdObject(b.authority.legacy, b.authority.sid, true) != nil {
		return ErrRecoveryRequired
	}
	names, err := directoryNames(ctx, b.authority.legacy)
	if err != nil || len(names) != len(b.source)+1 {
		return ErrRecoveryRequired
	}
	known := map[string]bool{"controller.lock": true}
	for _, f := range b.source {
		known[f.name] = true
		if objectPrivate(windows.Handle(f.file.Fd()), b.authority.sid, false) != nil {
			return ErrRecoveryRequired
		}
	}
	for _, name := range names {
		if !known[name] {
			return ErrRecoveryRequired
		}
		delete(known, name)
	}
	if len(known) != 0 || objectPrivate(b.authority.legacy, b.authority.sid, true) != nil {
		return ErrRecoveryRequired
	}
	b.unknown = false
	return nil
}
func (b *nativeColdBackend) create(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if b.check(ctx) != nil || !b.admitted || b.created {
		return ErrRecoveryRequired
	}
	b.unknown = true
	r := b.r
	sd, err := privateSD(r.sid, true)
	if err != nil {
		return ErrState
	}
	h, err := ntOpen(r.parent, "services.quarantine-"+b.receipt.Scope.Transaction, true, windows.FILE_CREATE, sd, false, false, false)
	if err != nil {
		return ErrRecoveryRequired
	}
	b.quarantine, err = capture(h, r.sid, true, true)
	if err != nil {
		return ErrRecoveryRequired
	}
	b.receipt.Quarantine = b.quarantine.id
	h, err = ntOpen(r.parent, "services", true, windows.FILE_CREATE, sd, false, false, false)
	if err != nil || r.hold(h, true, true) != nil {
		return ErrRecoveryRequired
	}
	r.root = h
	b.receipt.Root = r.chain[len(r.chain)-1].id
	fileSD, err := privateSD(r.sid, false)
	if err != nil {
		return ErrState
	}
	lock, err := ntOpen(r.root, "controller.lock", false, windows.FILE_CREATE, fileSD, true, false, true)
	if err != nil {
		return ErrRecoveryRequired
	}
	r.owner, err = r.record(lock)
	if err != nil || r.readInto(ctx, r.owner, 1) != nil || len(r.owner.body) != 0 {
		return ErrRecoveryRequired
	}
	b.created = true
	b.unknown = false
	return nil
}
func (b *nativeColdBackend) copy(ctx context.Context, name string, data []byte, disposition string) (TransitionFile, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if b.check(ctx) != nil || !b.created || recordBudget(name) == 0 || int64(len(data)) > recordBudget(name) || disposition != transitionDisposition(name) {
		return TransitionFile{}, ErrState
	}
	var source *coldSourceFile
	for i := range b.source {
		if b.source[i].name == name {
			source = &b.source[i]
			break
		}
	}
	if source == nil || source.hash != sha256.Sum256(data) {
		return TransitionFile{}, ErrState
	}
	b.unknown = true
	parent := b.r.root
	if disposition == "historical-only" {
		parent = windows.Handle(b.quarantine.file.Fd())
	}
	n, err := b.r.createFile(ctx, parent, name, data)
	if err != nil || n == nil || n.check() != nil {
		return TransitionFile{}, ErrRecoveryRequired
	}
	file := TransitionFile{Name: name, Source: source.id, Target: n.o.id, Disposition: disposition, Size: uint64(len(data)), SHA256: source.hash}
	body, _ := json.Marshal(file)
	journal, err := b.r.createFile(ctx, b.r.parent, "services.copy-"+b.receipt.Scope.Transaction+"-"+coldNumber(len(b.receipt.Files)+1), body)
	if err != nil || journal == nil {
		return TransitionFile{}, ErrRecoveryRequired
	}
	b.receipt.Files = append(b.receipt.Files, file)
	b.unknown = false
	return file, nil
}
func (b *nativeColdBackend) finish(ctx context.Context, files []TransitionFile) (TransitionReceipt, *Root, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if b.check(ctx) != nil || !b.created || b.finished || len(files) != len(b.source) || len(files) != len(b.receipt.Files) {
		return TransitionReceipt{}, nil, ErrRecoveryRequired
	}
	b.unknown = true
	binding := b.r.binding()
	binding.Admission = "cold-v1"
	data, _ := json.Marshal(binding)
	init, err := b.r.createFile(ctx, b.r.root, "state.init", data)
	if err != nil || init == nil {
		return TransitionReceipt{}, nil, ErrRecoveryRequired
	}
	b.r.init = init
	b.receipt.Init = init.o.id
	b.receipt.Inventory, b.receipt.Active, b.receipt.Historical = transitionHashes(files)
	if !b.receipt.valid() {
		return TransitionReceipt{}, nil, ErrRecoveryRequired
	}
	manifest, _ := json.Marshal(b.receipt)
	record, err := b.r.createFile(ctx, b.r.parent, "services.manifest-"+b.receipt.Scope.Transaction, manifest)
	if err != nil || record == nil {
		return TransitionReceipt{}, nil, ErrRecoveryRequired
	}
	b.r.initialized = true
	b.r.admitted = false
	b.finished = true
	b.unknown = false
	root := &Root{&rootState{b: &transitionBackend{cold: b}}}
	if root.Check() != nil {
		return TransitionReceipt{}, root, ErrRecoveryRequired
	}
	return b.receipt, root, nil
}
func (b *nativeColdBackend) prepareCommittedClose(receipt TransitionReceipt) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Offered/returned copies need the complete logical-supervisor/positive job
	// retirement chain before any local authority can be released. That chain is
	// not implemented yet; neither object comparison nor a protocol reply alone
	// may promote these held copies into a final close grant.
	if b == nil || b.supervisorOffer == nil || b.supervisorOffer.prepareOwnerRelease() != nil {
		return ErrRecoveryRequired
	}
	if b == nil || b.closePrepared || b.closed || b.check(ctx) != nil || b.commitRecord == nil || b.commitRecord.check() != nil || !b.authority.finalConsumed || b.runtime == nil || b.rootCapability == nil || b.runtime.RootBacking() != b.rootCapability || b.runtime.VerifyJoined(ctx) != nil || b.ipc == nil {
		return ErrRecoveryRequired
	}
	want, e := b.receipt.Digest()
	got, f := receipt.Digest()
	if e != nil || f != nil || want != got {
		return ErrRecoveryRequired
	}
	b.ipc.mu.Lock()
	joined := b.ipc.joined && !b.ipc.serving && !b.ipc.unknown
	b.ipc.mu.Unlock()
	if !joined {
		return ErrRecoveryRequired
	}
	// This exact original read-only controller is the final outer close owner.
	// Never close a duplicate numeric handle or derive ownership from its path.
	for _, file := range b.authority.files {
		if file != nil && file.Fd() == uintptr(b.authority.controller) {
			if b.controllerFile != nil {
				return ErrRecoveryRequired
			}
			b.controllerFile = file
		}
	}
	if b.controllerFile == nil {
		return ErrRecoveryRequired
	}
	if b.authority.controlIO == nil || b.authority.controlIO.prepareClose() != nil {
		return ErrRecoveryRequired
	}
	if b.leafMaster == nil || b.leafMaster.factory != b.runtime.LeafFactoryBacking() || b.leafMaster.prepareOwnerRelease() != nil {
		return ErrRecoveryRequired
	}
	b.closePrepared = true
	return nil
}
func (b *nativeColdBackend) close() error {
	if b == nil || !b.closePrepared || !b.rootClosed || b.closed || b.unknown || b.controllerFile == nil {
		return ErrRecoveryRequired
	}
	b.unknown = true
	a := b.authority
	var resources []func() error
	for _, source := range b.source {
		resources = append(resources, source.file.Close)
	}
	if b.quarantine.file != nil {
		resources = append(resources, b.quarantine.file.Close)
	}
	resources = append(resources, b.ipc.listener.Close) // cached positive original close
	resources = append(resources, b.leafMaster.closeKnown)
	resources = append(resources, a.controlIO.closeFinal)
	for _, returned := range b.supervisorOffer.returned {
		if returned != 0 {
			h := returned
			resources = append(resources, func() error { return windows.CloseHandle(h) })
		}
	}
	for _, file := range a.files {
		if file != b.controllerFile {
			resources = append(resources, file.Close)
		}
	}
	if a.finalEvent != 0 {
		event := a.finalEvent
		resources = append(resources, func() error { return windows.CloseHandle(event) })
	}
	for _, process := range a.processes {
		resources = append(resources, func() error { return windows.CloseHandle(process) })
	}
	for _, token := range a.tokens {
		resources = append(resources, token.Close)
	}
	a.supervisorMu.RLock()
	if a.pendingSupervisor != nil && a.pendingSupervisor.process != 0 {
		serving := a.pendingSupervisor.process
		resources = append(resources, func() error { return windows.CloseHandle(serving) })
	}
	a.supervisorMu.RUnlock()
	if a.parent != 0 {
		parent := a.parent
		resources = append(resources, func() error { return windows.CloseHandle(parent) })
	}
	if a.self != nil {
		resources = append(resources, a.self.Close)
	}
	if closeOwnerLast(resources, b.controllerFile.Close) != nil {
		return ErrRecoveryRequired
	}
	a.closed, b.closed, b.unknown = true, true, false
	return nil
}

type transitionBackend struct{ cold *nativeColdBackend }

func (b *transitionBackend) check() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return b.cold.check(ctx)
}
func (b *transitionBackend) read(ctx context.Context, n string, budget int64) (recordLease, error) {
	if b.cold.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	return b.cold.r.read(ctx, n, budget)
}
func (b *transitionBackend) publish(ctx context.Context, n string, old recordLease, data []byte) (recordLease, error) {
	if b.cold.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	return b.cold.r.publish(ctx, n, old, data)
}
func (b *transitionBackend) remove(ctx context.Context, n string, old recordLease) error {
	if b.cold.check(ctx) != nil {
		return ErrRecoveryRequired
	}
	return b.cold.r.remove(ctx, n, old)
}
func (b *transitionBackend) list(ctx context.Context) ([]string, error) {
	if b.cold.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	return b.cold.r.list(ctx)
}
func (b *transitionBackend) close() error {
	if b == nil || b.cold == nil || !b.cold.closePrepared || b.cold.unknown || b.cold.rootClosed {
		return ErrRecoveryRequired
	}
	if b.cold.r.close() != nil {
		b.cold.unknown = true
		return ErrRecoveryRequired
	}
	b.cold.rootClosed = true
	return nil
}

func coldNumber(value int) string {
	const digits = "0123456789abcdef"
	var text [8]byte
	for i := 7; i >= 0; i-- {
		text[i] = digits[value&15]
		value >>= 4
	}
	return string(text[:])
}
func openColdSource(parent windows.Handle, name string, data bool) (windows.Handle, error) {
	text, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, ErrState
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	access := uint32(windows.READ_CONTROL | windows.WRITE_DAC | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	if data {
		access |= windows.FILE_READ_DATA
		share = 0
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, access, &oa, &status, nil, 0, share, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	return h, err
}
func sealColdObject(h windows.Handle, sid string, dir bool) error {
	if legacyTransitionObject(h, sid, dir) != nil {
		return ErrState
	}
	sd, err := privateSD(sid, dir)
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return ErrState
	}
	if windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil) != nil || objectPrivate(h, sid, dir) != nil {
		return ErrRecoveryRequired
	}
	return nil
}
