//go:build windows

package sdkstate

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

type nativeRoot struct {
	sid, home, installation, daemon string
	session                         uint32
	token                           windows.Token
	chain                           []object
	parent, root                    windows.Handle
	owner                           *nativeRecord
	init                            *nativeRecord
	partials                        []*nativeRecord
	admitted, closed, initialized   bool
	closeErr                        error
}
type nativeRecord struct {
	mu                            sync.Mutex
	o                             object
	body                          []byte
	closed                        bool
	closeErr                      error
	disposing, dispositionUnknown bool
	retainOnOriginUnknown         bool
}

func (r *nativeRecord) data() []byte { return r.body }
func (r *nativeRecord) check() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.o.recheck() != nil {
		return ErrState
	}
	return nil
}
func (r *nativeRecord) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed {
		r.closed = true
		origin := func() error {
			if r.disposing {
				if r.dispositionUnknown {
					return ErrRecoveryRequired
				}
				return nil
			}
			return r.o.recheck()
		}
		r.closeErr = closeCheckedRecord(r.retainOnOriginUnknown, origin, r.o.file.Close)
		if r.closeErr == nil {
			clear(r.body)
			r.body = nil
		}
	}
	if r.closeErr != nil {
		return ErrRecoveryRequired
	}
	return nil
}
func privateSD(sid string, dir bool) (*windows.SECURITY_DESCRIPTOR, error) {
	flags := ""
	if dir {
		flags = "OICI"
	}
	return windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;" + flags + ";FA;;;" + sid + ")(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)")
}
func current() (sid string, session uint32, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var thread windows.Token
	if e := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &thread); e == nil {
		_ = thread.Close()
		return "", 0, ErrState
	} else if e != windows.ERROR_NO_TOKEN {
		return "", 0, ErrState
	}
	t := windows.GetCurrentProcessToken()
	var elevation, ac, size uint32
	for _, v := range []struct {
		class uint32
		p     *uint32
	}{{windows.TokenElevation, &elevation}, {29, &ac}, {windows.TokenSessionId, &session}} {
		if windows.GetTokenInformation(t, v.class, (*byte)(unsafe.Pointer(v.p)), 4, &size) != nil || size != 4 {
			return "", 0, ErrState
		}
	}
	restricted, e := t.IsRestricted()
	if e != nil || restricted || elevation != 0 || ac != 0 || session == 0 {
		return "", 0, ErrState
	}
	u, e := t.GetTokenUser()
	if e != nil || u == nil || u.User.Sid == nil {
		return "", 0, ErrState
	}
	sid = u.User.Sid.String()
	if !strings.HasPrefix(sid, "S-1-5-21-") && !strings.HasPrefix(sid, "S-1-12-1-") {
		return "", 0, ErrState
	}
	return sid, session, nil
}
func ntOpen(parent windows.Handle, name string, dir bool, disposition uint32, sd *windows.SECURITY_DESCRIPTOR, write, remove, exclusive bool) (windows.Handle, error) {
	if !safePart(name) {
		return 0, ErrState
	}
	text, e := windows.NewNTUnicodeString(name)
	if e != nil {
		return 0, ErrState
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE, SecurityDescriptor: sd}
	oa.Length = uint32(unsafe.Sizeof(oa))
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE)
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	share := uint32(windows.FILE_SHARE_READ)
	if dir {
		options |= windows.FILE_DIRECTORY_FILE
		access |= windows.FILE_LIST_DIRECTORY
		share |= windows.FILE_SHARE_WRITE
	} else {
		options |= windows.FILE_NON_DIRECTORY_FILE
		access |= windows.GENERIC_READ
	}
	if write {
		access |= windows.GENERIC_WRITE
	}
	if remove {
		access |= windows.DELETE
	}
	if exclusive {
		share = 0
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	e = windows.NtCreateFile(&h, access, &oa, &status, nil, 0, share, disposition, options, 0, 0)
	runtime.KeepAlive(sd)
	if e != nil {
		return 0, e
	}
	return h, nil
}
func (r *nativeRoot) hold(h windows.Handle, dir, private bool) error {
	o, e := capture(h, r.sid, dir, private)
	if o.file != nil {
		r.chain = append(r.chain, o)
	}
	return e
}
func (r *nativeRoot) record(h windows.Handle) (*nativeRecord, error) {
	o, e := capture(h, r.sid, false, true)
	n := &nativeRecord{o: o}
	if o.file != nil && e != nil {
		r.partials = append(r.partials, n)
	}
	return n, e
}
func localPath(s string) bool {
	return len(s) >= 3 && s[1:3] == `:\` && filepath.Clean(s) == s && !strings.ContainsAny(s[2:], ":/\x00")
}
func (r *nativeRoot) walkProfile(ctx context.Context, profile string) error {
	if !localPath(profile) || len(strings.Split(profile[3:], `\`)) > 16 {
		return ErrState
	}
	p, e := windows.UTF16PtrFromString(profile[:3])
	if e != nil {
		return ErrState
	}
	h, e := windows.CreateFile(p, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_LIST_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return ErrState
	}
	if r.hold(h, true, false) != nil {
		return ErrState
	}
	for _, part := range strings.Split(profile[3:], `\`) {
		if ctx.Err() != nil {
			return ErrState
		}
		h, e = ntOpen(windows.Handle(r.chain[len(r.chain)-1].file.Fd()), part, true, windows.FILE_OPEN, nil, false, false, false)
		if e != nil || r.hold(h, true, false) != nil {
			return ErrState
		}
	}
	return nil
}

// The legacy home is fixed by native LocalAppData. This walks metadata only,
// never source bytes, and can only DENY fresh state. No caller path grants rights.
func legacyAbsent(ctx context.Context, home string) (err error) {
	if !localPath(home) || len(strings.Split(home[3:], `\`)) > 32 {
		return ErrState
	}
	var files []*os.File
	defer func() {
		for i := len(files) - 1; i >= 0; i-- {
			if files[i].Close() != nil {
				err = ErrRecoveryRequired
			}
		}
	}()
	p, e := windows.UTF16PtrFromString(home[:3])
	if e != nil {
		return ErrState
	}
	h, e := windows.CreateFile(p, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return ErrState
	}
	files = append(files, os.NewFile(uintptr(h), "SDK legacy denial metadata"))
	parts := append(strings.Split(home[3:], `\`), ".rdorbwin", "local-services")
	for _, part := range parts {
		if ctx.Err() != nil {
			return ErrState
		}
		h, e = ntOpen(h, part, true, windows.FILE_OPEN, nil, false, false, false)
		if e == windows.STATUS_OBJECT_NAME_NOT_FOUND {
			return nil
		}
		if e != nil {
			return ErrState
		}
		f := os.NewFile(uintptr(h), "SDK legacy denial metadata")
		if f == nil {
			return ErrState
		}
		files = append(files, f)
		if _, e = snapshot(h, true); e != nil {
			return ErrState
		}
	}
	return ErrLegacyState
}

type stateBinding struct {
	Version                                                     int
	Product, Installation, Daemon, SID, Home, RootID, Admission string
}

func (r *nativeRoot) binding() stateBinding {
	return stateBinding{1, "orbwin", r.installation, r.daemon, r.sid, r.home, r.chain[len(r.chain)-1].id, "fresh"}
}
func openNative(ctx context.Context, installation, daemon, selectedHome string, mode openRestriction) (out backend, err error) {
	phase := "native identity"
	// Bounded fixed diagnostics retain typed refusal without exposing paths,
	// state bytes or identity values. These are observations, never recovery input.
	defer func() {
		if err != nil {
			err = fmt.Errorf("SDK private state %s: %w", phase, err)
		}
	}()
	r := &nativeRoot{installation: installation, daemon: daemon}
	defer func() {
		defer func() {
			if recover() != nil {
				r.admitted = true
				out, err = r, ErrRecoveryRequired
			}
		}()
		if recover() != nil {
			r.admitted = true // original partial owner stays live; no cleanup suffix after panic
			out, err = r, ErrRecoveryRequired
			return
		}
		if err != nil {
			if r.admitted {
				out = r
				return
			}
			if r.close() != nil {
				out = r
				err = ErrRecoveryRequired
			} else {
				out = nil
			}
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r.sid, r.session, err = current()
	if err != nil {
		return nil, err
	}
	// Explicit-user KnownFolder lookup requires QUERY|IMPERSONATE access.
	// This retains the original token slot; no impersonation or elevation occurs.
	err = windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, &r.token)
	if err != nil {
		return nil, ErrState
	}
	profile, e := r.token.KnownFolderPath(windows.FOLDERID_Profile, 0)
	if e != nil {
		return nil, ErrState
	}
	app, e := r.token.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if e != nil || !localPath(app) {
		return nil, ErrState
	}
	r.home = filepath.Join(app, "OrbWinData", "home")
	if !strings.EqualFold(strings.TrimPrefix(selectedHome, `\\?\`), r.home) {
		return nil, ErrState
	}
	phase = "legacy presence check"
	if err = legacyAbsent(ctx, r.home); err != nil {
		return nil, err
	}
	phase = "profile origin"
	if err = r.walkProfile(ctx, profile); err != nil {
		return nil, err
	}
	sd, e := privateSD(r.sid, true)
	if e != nil {
		return nil, ErrState
	}
	parent := windows.Handle(r.chain[len(r.chain)-1].file.Fd())
	phase = "private parent"
	h, e := ntOpen(parent, ".orbwin-private", true, windows.FILE_OPEN_IF, sd, false, false, false)
	if e != nil || r.hold(h, true, true) != nil {
		return nil, ErrState
	}
	r.parent = h
	phase = "migration fence inspection"
	for _, fence := range []string{"services.migration", "services.create-intent"} {
		fh, e := ntOpen(r.parent, fence, false, windows.FILE_OPEN, nil, false, false, false)
		if e == nil {
			n, _ := r.record(fh)
			r.partials = append(r.partials, n)
			return nil, ErrRecoveryRequired
		}
		if e != windows.STATUS_OBJECT_NAME_NOT_FOUND {
			return nil, ErrRecoveryRequired
		}
	}
	phase = "root admission"
	h, e = ntOpen(r.parent, "services", true, windows.FILE_OPEN, nil, false, false, false)
	fresh := e == windows.STATUS_OBJECT_NAME_NOT_FOUND
	if mode == newServiceOnly && !fresh {
		if e != nil {
			r.admitted = true // native reply was not confirmed absence: preserve this original prefix
			if h != 0 && h != windows.InvalidHandle {
				_ = r.hold(h, true, true)
			}
			return r, ErrRecoveryRequired
		}
		if r.hold(h, true, true) != nil {
			r.admitted = true
			return r, ErrRecoveryRequired
		}
		return nil, restrictNativeAdmission(mode, false)
	}
	var intent *nativeRecord
	if fresh {
		r.admitted = true
		data, _ := json.Marshal(struct {
			Version                                  int
			Product, Installation, Daemon, SID, Home string
		}{1, "orbwin", installation, daemon, r.sid, r.home})
		intent, err = r.createFile(ctx, r.parent, "services.create-intent", data)
		if err != nil {
			return r, ErrRecoveryRequired
		}
		h, e = ntOpen(r.parent, "services", true, windows.FILE_CREATE, sd, false, false, false)
	}
	if e != nil || r.hold(h, true, true) != nil {
		return nil, ErrRecoveryRequired
	}
	r.root = h
	disposition := uint32(windows.FILE_OPEN)
	if fresh {
		disposition = windows.FILE_CREATE
	}
	fileSD, e := privateSD(r.sid, false)
	if e != nil {
		return nil, ErrState
	}
	phase = "exclusive owner"
	lock, e := ntOpen(r.root, "controller.lock", false, disposition, fileSD, true, false, true)
	if e != nil {
		if e == windows.STATUS_OBJECT_NAME_NOT_FOUND {
			return nil, ErrRecoveryRequired
		}
		return nil, ErrBusy
	}
	r.owner, e = r.record(lock)
	if r.owner != nil {
		r.owner.retainOnOriginUnknown = true
	}
	if e != nil {
		return nil, ErrState
	}
	if e = r.readInto(ctx, r.owner, 1); e != nil || len(r.owner.body) != 0 {
		return nil, ErrRecoveryRequired
	}
	if fresh {
		phase = "initial binding publication"
		data, _ := json.Marshal(r.binding())
		init, e := r.createFile(ctx, r.root, "state.init", data)
		if e != nil {
			return r, ErrRecoveryRequired
		}
		if init.close() != nil {
			return r, ErrRecoveryRequired
		}
	}
	phase = "initialized binding verification"
	init, e := ntOpen(r.root, "state.init", false, windows.FILE_OPEN, nil, false, false, false)
	if e != nil {
		return nil, ErrRecoveryRequired
	}
	r.init, e = r.record(init)
	if e != nil || r.readInto(ctx, r.init, 4096) != nil {
		return nil, ErrRecoveryRequired
	}
	want, _ := json.Marshal(r.binding())
	if !bytes.Equal(want, r.init.body) {
		return nil, ErrRecoveryRequired
	}
	if fresh {
		phase = "fresh admission completion"
		if r.deleteFile(ctx, r.parent, "services.create-intent", intent) != nil {
			return r, ErrRecoveryRequired
		}
	}
	phase = "record enumeration"
	if _, e = r.list(ctx); e != nil {
		return nil, fmt.Errorf("%w: %w", ErrRecoveryRequired, e)
	}
	r.initialized = true
	r.admitted = false
	r.pruneClosed()
	phase = "final origin verification"
	if e = r.check(); e != nil {
		return nil, fmt.Errorf("%w: %w", ErrRecoveryRequired, e)
	}
	return r, nil
}
func (r *nativeRoot) check() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	sid, session, e := current()
	if e != nil || sid != r.sid || session != r.session || r.closed || !r.initialized {
		return ErrState
	}
	for i, o := range r.chain {
		if o.recheck() != nil {
			return fmt.Errorf("%w: held ancestor %d", ErrState, i)
		}
	}
	if r.owner == nil || r.owner.check() != nil {
		return fmt.Errorf("%w: held owner", ErrState)
	}
	if r.init == nil || r.init.check() != nil {
		return fmt.Errorf("%w: held initialization", ErrState)
	}
	return nil
}
func (r *nativeRoot) readInto(ctx context.Context, n *nativeRecord, budget int64) error {
	stat, e := n.o.file.Stat()
	if e != nil || stat.Size() < 0 || stat.Size() > budget {
		return ErrState
	}
	n.body = make([]byte, stat.Size())
	if _, e = n.o.file.Seek(0, io.SeekStart); e != nil {
		return ErrState
	}
	for at := 0; at < len(n.body); {
		if ctx.Err() != nil {
			return ErrState
		}
		count, e := n.o.file.Read(n.body[at:])
		if e != nil || count == 0 {
			return ErrState
		}
		at += count
	}
	var tail [1]byte
	count, e := n.o.file.Read(tail[:])
	if count != 0 || e != io.EOF || n.o.recheck() != nil {
		return ErrState
	}
	return nil
}
func (r *nativeRoot) read(ctx context.Context, name string, budget int64) (recordLease, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r.check() != nil {
		return nil, ErrState
	}
	h, e := ntOpen(r.root, name, false, windows.FILE_OPEN, nil, false, true, false)
	if e == windows.STATUS_OBJECT_NAME_NOT_FOUND {
		return nil, ErrNotExist
	}
	if e != nil {
		return nil, ErrState
	}
	n, e := r.record(h)
	if e != nil {
		return n, e
	}
	if e = r.readInto(ctx, n, budget); e != nil {
		return n, e
	}
	return n, nil
}
func (r *nativeRoot) createFile(ctx context.Context, parent windows.Handle, name string, data []byte) (n *nativeRecord, err error) {
	sd, e := privateSD(r.sid, false)
	if e != nil {
		return nil, ErrState
	}
	h, e := ntOpen(parent, name, false, windows.FILE_CREATE, sd, true, true, true)
	if e != nil {
		return nil, ErrState
	}
	n, e = r.record(h)
	r.partials = append(r.partials, n)
	if e != nil {
		return n, ErrState
	}
	for left := data; len(left) > 0; {
		if ctx.Err() != nil {
			return n, ErrState
		}
		count, e := n.o.file.Write(left)
		if e != nil || count == 0 {
			return n, ErrState
		}
		left = left[count:]
	}
	if n.o.file.Sync() != nil {
		return n, ErrState
	}
	// Update metadata only after this owner's exact flushed CREATE write, then
	// independently read all bounded bytes back from the same native FILE_OBJECT.
	id, e := snapshot(h, false)
	if e != nil {
		return n, ErrState
	}
	n.o.id = id
	if r.readInto(ctx, n, int64(len(data))) != nil || !bytes.Equal(n.body, data) {
		return n, ErrState
	}
	return n, nil
}
func deleteNative(n *nativeRecord) error {
	if n.check() != nil {
		return ErrState
	}
	// The same held object was authenticated before disposition admission. A
	// successful delete-pending transition may make name queries unavailable;
	// close that known transition, not a fresh namespace/path lookup. Unknown
	// disposition is latched and can never turn into positive cleanup on Close.
	n.mu.Lock()
	n.disposing = true
	n.dispositionUnknown = true
	n.mu.Unlock()
	flag := byte(1)
	var status windows.IO_STATUS_BLOCK
	if windows.NtSetInformationFile(windows.Handle(n.o.file.Fd()), &status, &flag, 1, windows.FileDispositionInformation) != nil {
		return ErrState
	}
	n.mu.Lock()
	n.dispositionUnknown = false
	n.mu.Unlock()
	return n.close()
}

func (r *nativeRoot) deleteFile(ctx context.Context, parent windows.Handle, name string, n *nativeRecord) error {
	if ctx.Err() != nil || deleteNative(n) != nil {
		return ErrState
	}
	h, e := ntOpen(parent, name, false, windows.FILE_OPEN, nil, false, false, false)
	if e == nil {
		v, _ := r.record(h)
		r.partials = append(r.partials, v)
		return ErrState
	}
	if e != windows.STATUS_OBJECT_NAME_NOT_FOUND {
		return ErrState
	}
	return nil
}
func renameNative(parent windows.Handle, source *nativeRecord, name string, replace bool) error {
	type info struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	text, e := windows.UTF16FromString(name)
	if e != nil {
		return ErrState
	}
	var shape info
	buf := make([]byte, int(unsafe.Offsetof(shape.Name))+(len(text)-1)*2)
	v := (*info)(unsafe.Pointer(&buf[0]))
	if replace {
		v.Replace = 1
	}
	v.Root = parent
	v.Length = uint32((len(text) - 1) * 2)
	copy(unsafe.Slice(&v.Name[0], len(text)-1), text[:len(text)-1])
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(windows.Handle(source.o.file.Fd()), &status, &buf[0], uint32(len(buf)), windows.FileRenameInformation)
}
func (r *nativeRoot) operation(ctx context.Context, name, kind string, old recordLease, data []byte) (*nativeRecord, error) {
	if r.check() != nil {
		return nil, ErrState
	}
	r.admitted = true
	previous := ""
	if old != nil {
		n, ok := old.(*nativeRecord)
		if !ok || n.check() != nil {
			return nil, ErrState
		}
		previous = n.o.id
	}
	digest := sha256.Sum256(data)
	encoded, _ := json.Marshal(struct {
		Version                       int
		Name, Kind, Previous, NextSHA string
	}{1, name, kind, previous, hex.EncodeToString(digest[:])})
	return r.createFile(ctx, r.root, "state.op", encoded)
}
func (r *nativeRoot) publish(ctx context.Context, name string, old recordLease, data []byte) (recordLease, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	intent, e := r.operation(ctx, name, "publish", old, data)
	if e != nil {
		return nil, ErrRecoveryRequired
	}
	nonce := make([]byte, 16)
	if _, e = rand.Read(nonce); e != nil {
		return nil, ErrRecoveryRequired
	}
	stageName := ".sdkstate-stage-" + hex.EncodeToString(nonce)
	stage, e := r.createFile(ctx, r.root, stageName, data)
	if e != nil {
		return stage, ErrRecoveryRequired
	}
	id := stage.o.id
	// Lease exchange: expected target is checked while immutable; only this
	// guard releases its reader. Private parent +sole SDK data lock stay held.
	if old != nil {
		if old.check() != nil || old.close() != nil {
			return stage, ErrRecoveryRequired
		}
	}
	if ctx.Err() != nil || renameNative(r.root, stage, name, old != nil) != nil || stage.o.file.Sync() != nil || stage.close() != nil {
		return stage, ErrRecoveryRequired
	}
	h, e := ntOpen(r.root, name, false, windows.FILE_OPEN, nil, false, true, false)
	if e != nil {
		return nil, ErrRecoveryRequired
	}
	fresh, e := r.record(h)
	if e != nil {
		return fresh, ErrRecoveryRequired
	}
	if !sameObject(fresh.o.id, id) || r.readInto(ctx, fresh, int64(len(data))) != nil || !bytes.Equal(fresh.body, data) {
		return fresh, ErrRecoveryRequired
	}
	if r.deleteFile(ctx, r.root, "state.op", intent) != nil {
		return fresh, ErrRecoveryRequired
	}
	r.admitted = false
	r.pruneClosed()
	return fresh, nil
}
func (r *nativeRoot) remove(ctx context.Context, name string, old recordLease) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	intent, e := r.operation(ctx, name, "delete", old, nil)
	if e != nil {
		return ErrRecoveryRequired
	}
	n, ok := old.(*nativeRecord)
	if !ok || r.deleteFile(ctx, r.root, name, n) != nil {
		return ErrRecoveryRequired
	}
	if r.deleteFile(ctx, r.root, "state.op", intent) != nil {
		return ErrRecoveryRequired
	}
	r.admitted = false
	r.pruneClosed()
	return nil
}
func (r *nativeRoot) list(ctx context.Context) ([]string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	names, e := directoryNames(ctx, r.root)
	if e != nil {
		return nil, e
	}
	var out []string
	for _, name := range names {
		if name == "controller.lock" || name == "state.init" {
			continue
		}
		if recordBudget(name) == 0 {
			return nil, ErrRecoveryRequired
		}
		out = append(out, name)
	}
	return out, nil
}
func (r *nativeRoot) close() error {
	if r == nil {
		return nil
	}
	if r.closed {
		return r.closeErr
	}
	if r.admitted {
		return ErrRecoveryRequired
	}
	r.closed = true
	var closers []func() error
	for _, p := range r.partials {
		if p != nil {
			n := p
			closers = append(closers, n.close)
		}
	}
	if r.init != nil {
		closers = append(closers, r.init.close)
	}
	for i := len(r.chain) - 1; i >= 0; i-- {
		file := r.chain[i].file
		closers = append(closers, file.Close)
	}
	if r.token != 0 {
		token := r.token
		closers = append(closers, token.Close)
	}
	owner := func() error {
		if r.owner != nil {
			return r.owner.close()
		}
		return nil
	}
	r.closeErr = closeOwnerLast(closers, owner)
	return r.closeErr

}
func directoryBuffer() []byte {
	// FILE_ID_BOTH_DIR_INFO requires DWORDLONG alignment. A byte array may be
	// stack allocated with only byte alignment; use an explicitly aligned backing
	// object even when the compiler can keep this bounded allocation on the stack.
	words := new([65536 / 8]uint64)
	return unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), 65536)
}
func directoryNames(ctx context.Context, h windows.Handle) ([]string, error) {
	var names []string
	buf := directoryBuffer()
	if uintptr(unsafe.Pointer(&buf[0]))%8 != 0 {
		return nil, fmt.Errorf("%w: directory buffer alignment", ErrState)
	}
	class := uint32(windows.FileIdBothDirectoryRestartInfo)
	for batch := 0; batch < 64; batch++ {
		if ctx.Err() != nil {
			return nil, ErrState
		}
		e := windows.GetFileInformationByHandleEx(h, class, &buf[0], uint32(len(buf)))
		if e == windows.ERROR_NO_MORE_FILES {
			return names, nil
		}
		if e != nil {
			return nil, fmt.Errorf("%w: directory enumeration: %w", ErrState, e)
		}
		class = windows.FileIdBothDirectoryInfo
		for at := 0; ; {
			if at < 0 || at+104 > len(buf) {
				return nil, ErrState
			}
			next := int(binary.LittleEndian.Uint32(buf[at:]))
			length := int(binary.LittleEndian.Uint32(buf[at+60:]))
			if length < 2 || length%2 != 0 || length > 510 || at+104+length > len(buf) {
				return nil, ErrState
			}
			units := make([]uint16, length/2)
			for i := range units {
				units[i] = binary.LittleEndian.Uint16(buf[at+104+i*2:])
			}
			name := windows.UTF16ToString(units)
			if name != "." && name != ".." {
				if !safePart(name) || len(names) >= maxEntries+2 {
					return nil, ErrState
				}
				names = append(names, name)
			}
			if next == 0 {
				break
			}
			if next < 104+length || next%8 != 0 || at+next >= len(buf) {
				return nil, ErrState
			}
			at += next
		}
		clear(buf)
	}
	return nil, ErrState
}

// Last-write metadata can settle only when the write FILE_OBJECT closes. The
// post-close immutable reader must match native object ID/attributes/link count
// and all bytes; it then captures its own stable metadata for future rechecks.
func sameObject(a, b string) bool {
	x, y := strings.Split(a, ":"), strings.Split(b, ":")
	return len(x) >= 4 && len(y) >= 4 && strings.Join(x[:4], ":") == strings.Join(y[:4], ":")
}
func (r *nativeRoot) pruneClosed() {
	keep := r.partials[:0]
	for _, n := range r.partials {
		n.mu.Lock()
		closed := n.closed && n.closeErr == nil
		n.mu.Unlock()
		if !closed {
			keep = append(keep, n)
		}
	}
	r.partials = keep
}
