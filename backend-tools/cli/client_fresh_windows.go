//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A fresh capability is deliberately separate from nativeClientConfig: until
// marker-last completion there is no initialized namespace or writer guard.
// Every actual handle, including failed captures, remains owned until checked
// release. Unknown closure retains the exclusive controller and fences reentry.
type nativeFreshInitialization struct {
	sid, profile, local, docker, credentials, plugins string
	ancestors                                         []*object // volume, profile ancestry, private directory, client root
	directories                                       []*object
	data                                              []*configData
	owner, marker, init, config                       *configData
	tree                                              *object
	markerBody, initBody, configBody                  []byte
	rootNames, dockerNames                            []string
	ownerPreflightID, ownerPreflightSD                string
	failed, transferred                               bool
}

var retainedFreshInitializations struct {
	sync.Mutex
	roots []*nativeFreshInitialization
}

func (f *nativeFreshInitialization) fence() error {
	f.failed = true
	retainedConfigGuards.Lock()
	retainedConfigGuards.failed = true
	retainedConfigGuards.Unlock()
	return errClientConfigRecovery
}
func (f *nativeFreshInitialization) closeData(d *configData) error {
	if d != nil && d.file != nil {
		if d.file.Close() != nil {
			return f.fence()
		}
		d.file = nil
	}
	return nil
}
func (f *nativeFreshInitialization) closeDirectory(o *object) error {
	if o != nil && o.file != nil {
		if o.file.Close() != nil {
			return f.fence()
		}
		o.file = nil
	}
	return nil
}
func (f *nativeFreshInitialization) release() error {
	if f.transferred {
		return nil
	}
	if f.failed {
		return errClientConfigRecovery
	}
	for i := len(f.data) - 1; i >= 0; i-- {
		if f.data[i] != f.owner && f.closeData(f.data[i]) != nil {
			return errClientConfigRecovery
		}
	}
	for i := len(f.directories) - 1; i >= 0; i-- {
		if f.closeDirectory(f.directories[i]) != nil {
			return errClientConfigRecovery
		}
	}
	for i := len(f.ancestors) - 1; i >= 0; i-- {
		if f.closeDirectory(f.ancestors[i]) != nil {
			return errClientConfigRecovery
		}
	}
	if f.closeData(f.owner) != nil {
		return errClientConfigRecovery
	}
	retainedFreshInitializations.Lock()
	defer retainedFreshInitializations.Unlock()
	for i, v := range retainedFreshInitializations.roots {
		if v == f {
			retainedFreshInitializations.roots = append(retainedFreshInitializations.roots[:i], retainedFreshInitializations.roots[i+1:]...)
			break
		}
	}
	return nil
}
func freshMissing(e error) bool { return errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) }

// Only fixed protocol names may be created/deleted; profile ancestry uses the
// existing metadata-only relative opener. LIST authority is never inherited.
func freshOpen(parent windows.Handle, name string, dir bool, disposition uint32, write, remove, list, broad, exclusive bool, sd *windows.SECURITY_DESCRIPTOR) (windows.Handle, error) {
	if name != "client" && name != "docker" && name != "client.create-intent" && name != "controller.lock" && name != "client.init" && name != "config.json" {
		return 0, errClientConfigRecovery
	}
	text, e := windows.NewNTUnicodeString(name)
	if e != nil {
		return 0, e
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE, SecurityDescriptor: sd}
	oa.Length = uint32(unsafe.Sizeof(oa))
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE)
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	share := uint32(windows.FILE_SHARE_READ)
	if dir {
		access = directoryShareAccess(access)
		options |= windows.FILE_DIRECTORY_FILE
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
	if list {
		access |= windows.FILE_LIST_DIRECTORY
	}
	if broad {
		share |= windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	}
	if exclusive {
		share = 0
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	e = windows.NtCreateFile(&h, access, &oa, &status, nil, 0, share, disposition, options, 0, 0)
	runtime.KeepAlive(sd)
	return h, e
}
func (f *nativeFreshInitialization) holdData(h windows.Handle) (*configData, error) {
	d, e := captureConfig(h, f.sid, false)
	if d != nil {
		f.data = append(f.data, d)
	}
	return d, e
}
func (f *nativeFreshInitialization) holdDir(h windows.Handle, private, ancestor bool) (*object, error) {
	o, e := capture(h, f.sid, true, private)
	p := &o
	if o.file != nil {
		if ancestor {
			f.ancestors = append(f.ancestors, p)
		} else {
			f.directories = append(f.directories, p)
		}
	}
	return p, e
}
func freshHandle(o *object) windows.Handle            { return windows.Handle(o.file.Fd()) }
func (f *nativeFreshInitialization) root() *object    { return f.ancestors[len(f.ancestors)-1] }
func (f *nativeFreshInitialization) private() *object { return f.ancestors[len(f.ancestors)-2] }
func (f *nativeFreshInitialization) descriptor(dir bool) (*windows.SECURITY_DESCRIPTOR, error) {
	flags := ""
	if dir {
		flags = "OICI"
	}
	return windows.SecurityDescriptorFromString("O:" + f.sid + "D:P(A;" + flags + ";FA;;;" + f.sid + ")(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)")
}
func (f *nativeFreshInitialization) expectedInit() (map[string]any, error) {
	var root windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(freshHandle(f.root()), &root) != nil {
		return nil, errClientConfigRecovery
	}
	number := func(v uint32) json.Number { return json.Number(strconv.FormatUint(uint64(v), 10)) }
	return map[string]any{"schema": json.Number("1"), "product": "orbwin", "ownerSid": f.sid, "rootVolume": number(root.VolumeSerialNumber), "rootHigh": number(root.FileIndexHigh), "rootLow": number(root.FileIndexLow), "admission": "fresh"}, nil
}
func freshAllowedName(name string, docker bool) bool {
	if docker {
		return name == "config.json"
	}
	return name == "controller.lock" || name == "client.init" || name == "docker"
}

// Parse the native bounded directory response without accepting alternate case,
// duplicate names, malformed offsets, or names outside this closed protocol.
func freshDirectoryBatch(buf []byte, docker bool, seen map[string]bool) ([]string, error) {
	var names []string
	for at := 0; ; {
		if at+104 > len(buf) {
			return nil, errClientConfigRecovery
		}
		next := int(binary.LittleEndian.Uint32(buf[at:]))
		length := int(binary.LittleEndian.Uint32(buf[at+60:]))
		if length < 2 || length%2 != 0 || length > 510 || at+104+length > len(buf) {
			return nil, errClientConfigRecovery
		}
		units := make([]uint16, length/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(buf[at+104+i*2:])
		}
		name := windows.UTF16ToString(units)
		canonical, encodeErr := windows.UTF16FromString(name)
		if encodeErr != nil || len(canonical) != len(units)+1 {
			return nil, errClientConfigRecovery
		}
		for i := range units {
			if canonical[i] != units[i] {
				return nil, errClientConfigRecovery
			}
		}
		if name != "." && name != ".." {
			limit := 3
			if docker {
				limit = 1
			}
			if !freshAllowedName(name, docker) || seen[name] || len(seen) >= limit {
				return nil, errClientConfigRecovery
			}
			seen[name] = true
			names = append(names, name)
		}
		if next == 0 {
			return names, nil
		}
		if next < 104+length || next%8 != 0 || at+next >= len(buf) {
			return nil, errClientConfigRecovery
		}
		at += next
	}
}
func (f *nativeFreshInitialization) inventory(ctx context.Context, parent windows.Handle, name string, held *object, docker bool) ([]string, error) {
	h, e := freshOpen(parent, name, true, windows.FILE_OPEN, false, false, true, true, false, nil)
	if e != nil {
		return nil, errClientConfigRecovery
	}
	o, e := f.holdDir(h, true, false)
	if e != nil {
		return nil, e
	}
	if o.id != held.id || o.sdSHA != held.sdSHA {
		return nil, errClientConfigRecovery
	}
	words := new([8192]uint64)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), 65536)
	class := uint32(windows.FileIdBothDirectoryRestartInfo)
	seen := map[string]bool{}
	var names []string
	for batch := 0; batch < 4; batch++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		e = windows.GetFileInformationByHandleEx(h, class, &buf[0], uint32(len(buf)))
		if errors.Is(e, windows.ERROR_NO_MORE_FILES) {
			if o.recheck() != nil || held.recheck() != nil {
				return nil, errClientConfigRecovery
			}
			if f.closeDirectory(o) != nil {
				return nil, errClientConfigRecovery
			}
			sort.Strings(names)
			return names, nil
		}
		if e != nil {
			return nil, errClientConfigRecovery
		}
		class = windows.FileIdBothDirectoryInfo
		batchNames, x := freshDirectoryBatch(buf, docker, seen)
		if x != nil {
			return nil, x
		}
		names = append(names, batchNames...)
		clear(buf)
	}
	return nil, errClientConfigRecovery
}
func freshNamesEqual(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }
func (f *nativeFreshInitialization) optionalData(parent windows.Handle, name string, broad bool) (*configData, []byte, error) {
	h, e := freshOpen(parent, name, false, windows.FILE_OPEN, false, false, false, broad, false, nil)
	if freshMissing(e) {
		return nil, nil, nil
	}
	if e != nil {
		return nil, nil, errClientConfigRecovery
	}
	d, e := f.holdData(h)
	if e != nil {
		return d, nil, e
	}
	b, e := d.read(freshClientRecordBudget)
	return d, b, e
}
func (f *nativeFreshInitialization) immutable(parent windows.Handle, name string, old *configData, body []byte) (*configData, error) {
	if old == nil {
		return nil, nil
	}
	h, e := freshOpen(parent, name, false, windows.FILE_OPEN, false, false, false, false, false, nil)
	if e != nil {
		return nil, errClientConfigRecovery
	}
	d, e := f.holdData(h)
	if e != nil || d.id != old.id || d.sd != old.sd {
		return nil, errClientConfigRecovery
	}
	actual, e := d.read(freshClientRecordBudget)
	if e != nil || !bytes.Equal(actual, body) || f.closeData(old) != nil {
		return nil, errClientConfigRecovery
	}
	return d, nil
}
func (f *nativeFreshInitialization) origin() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if f.failed || f.transferred || ordinary() != nil {
		return errClientConfigRecovery
	}
	sid, _, _, e := processFacts(windows.CurrentProcess())
	if e != nil || sid != f.sid {
		return errClientConfigRecovery
	}
	profile, local, e := nativeLocations()
	if e != nil || profile != f.profile || local != f.local {
		return errClientConfigRecovery
	}
	docker, e := existingCache("docker")
	if e != nil || docker != f.docker {
		return errClientConfigRecovery
	}
	credentials, e := existingCache("credentials")
	if e != nil || credentials != f.credentials {
		return errClientConfigRecovery
	}
	for _, o := range f.ancestors {
		if o.recheck() != nil {
			return errClientConfigRecovery
		}
	}
	if f.tree != nil && f.tree.recheck() != nil {
		return errClientConfigRecovery
	}
	if f.owner == nil || f.owner.check() != nil {
		return errClientConfigRecovery
	}
	b, e := f.owner.read(0)
	if e != nil || len(b) != 0 {
		return errClientConfigRecovery
	}
	return nil
}
func (f *nativeFreshInitialization) recheck(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || f.origin() != nil {
		return errClientConfigRecovery
	}
	b, e := f.marker.read(freshClientRecordBudget)
	if e != nil || !bytes.Equal(b, f.markerBody) || !freshMarkerPrefix(b) {
		return errClientConfigRecovery
	}
	expected, e := f.expectedInit()
	if e != nil {
		return e
	}
	for _, item := range []struct {
		d     *configData
		body  []byte
		valid func([]byte) bool
	}{{f.init, f.initBody, func(b []byte) bool { return freshInitPrefix(b, expected) }}, {f.config, f.configBody, func(b []byte) bool { return freshDefaultsPrefix(b, f.profile) }}} {
		if item.d == nil {
			continue
		}
		b, e := item.d.read(freshClientRecordBudget)
		if e != nil || !bytes.Equal(b, item.body) || !item.valid(b) {
			return errClientConfigRecovery
		}
	}
	names, e := f.inventory(ctx, freshHandle(f.private()), "client", f.root(), false)
	if e != nil || !freshNamesEqual(names, f.rootNames) {
		return errClientConfigRecovery
	}
	if f.tree != nil {
		names, e = f.inventory(ctx, freshHandle(f.root()), "docker", f.tree, true)
		if e != nil || !freshNamesEqual(names, f.dockerNames) {
			return errClientConfigRecovery
		}
	}
	return f.legacyAbsent()
}

// Denial only, no legacy import or policy adoption. All components derive from
// the actual token KnownFolder, and only a native NAME_NOT_FOUND means absent.
func (f *nativeFreshInitialization) legacyAbsent() error {
	path := filepath.Join(f.local, "OrbWinData", "home", ".rdorbwin", "docker")
	if len(path) < 4 || len(path) > 8192 || len(strings.Split(path[3:], `\`)) > 32 || path[1:3] != `:\` {
		return errClientConfigRecovery
	}
	text, _ := windows.UTF16PtrFromString(path[:3])
	h, e := windows.CreateFile(text, directoryShareAccess(windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES), windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return errClientConfigRecovery
	}
	// These denial-only handles have no accepted private DACL, but remain tracked
	// for checked closure just like privileged fresh directory capabilities.
	hold := func(h windows.Handle) *object {
		o := &object{file: os.NewFile(uintptr(h), "fixed legacy denial")}
		f.directories = append(f.directories, o)
		return o
	}
	held := []*object{hold(h)}
	if _, e = snapshot(h, true); e != nil {
		return errClientConfigRecovery
	}
	absent := false
	for _, part := range strings.Split(path[3:], `\`) {
		if !safePart(part) {
			return errClientConfigRecovery
		}
		text, e := windows.NewNTUnicodeString(part)
		if e != nil {
			return errClientConfigRecovery
		}
		oa := windows.OBJECT_ATTRIBUTES{RootDirectory: h, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
		oa.Length = uint32(unsafe.Sizeof(oa))
		var next windows.Handle
		var status windows.IO_STATUS_BLOCK
		e = windows.NtCreateFile(&next, directoryShareAccess(windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE), &oa, &status, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
		if freshMissing(e) {
			absent = true
			break
		}
		if e != nil {
			return errClientConfigRecovery
		}
		held = append(held, hold(next))
		h = next
		if _, e = snapshot(h, true); e != nil {
			return errClientConfigRecovery
		}
	}
	for i := len(held) - 1; i >= 0; i-- {
		if f.closeDirectory(held[i]) != nil {
			return errClientConfigRecovery
		}
	}
	if !absent {
		return errClientConfigRecovery
	}
	return nil
}
func preparePrivateFreshRecovery(ctx context.Context) (review *clientWriterRecovery, handled bool, resultErr error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, true, errClientConfigRecovery
	}
	profile, local, e := nativeLocations()
	if e != nil {
		return nil, true, e
	}
	sid, _, _, e := processFacts(windows.CurrentProcess())
	if e != nil {
		return nil, true, e
	}
	if len(profile) < 4 || profile[1:3] != `:\` || len(strings.Split(profile[3:], `\`)) > 16 {
		return nil, true, errClientConfigRecovery
	}
	retainedConfigGuards.Lock()
	blocked := retainedConfigGuards.failed || len(retainedConfigGuards.roots) >= 16
	retainedConfigGuards.Unlock()
	if blocked {
		return nil, true, errClientConfigRecovery
	}
	f := &nativeFreshInitialization{sid: sid, profile: profile, local: local}
	retainedFreshInitializations.Lock()
	if len(retainedFreshInitializations.roots) >= 16 {
		retainedFreshInitializations.Unlock()
		return nil, true, errClientConfigRecovery
	}
	retainedFreshInitializations.roots = append(retainedFreshInitializations.roots, f)
	retainedFreshInitializations.Unlock()
	success := false
	defer func() {
		if !success {
			if f.release() != nil {
				resultErr = errClientConfigRecovery
				handled = true
			}
		}
	}()
	text, _ := windows.UTF16PtrFromString(profile[:3])
	h, e := windows.CreateFile(text, directoryShareAccess(windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES), windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, true, errClientConfigRecovery
	}
	if _, e = f.holdDir(h, false, true); e != nil {
		return nil, true, e
	}
	parts := strings.Split(profile[3:], `\`)
	for i, part := range append(append([]string{}, parts...), ".orbwin-private") {
		h, e = relative(freshHandle(f.ancestors[len(f.ancestors)-1]), part, true)
		if e != nil {
			return nil, true, errClientConfigRecovery
		}
		if _, e = f.holdDir(h, i >= len(parts), true); e != nil {
			return nil, true, e
		}
	}
	private := f.ancestors[len(f.ancestors)-1]
	marker, body, e := f.optionalData(freshHandle(private), "client.create-intent", true)
	if e != nil {
		return nil, true, e
	}
	if marker == nil {
		return nil, false, nil
	}
	if clientBootstrapClass(body) == bootstrapChoiceRequired || clientBootstrapClass(body) == bootstrapImport {
		return nil, true, errClientBootstrapDesktop
	}
	if !freshMarkerPrefix(body) {
		return nil, true, errClientConfigRecovery
	}
	f.marker, f.markerBody = marker, body
	f.docker, e = existingCache("docker")
	if e != nil {
		return nil, true, e
	}
	f.credentials, e = existingCache("credentials")
	if e != nil {
		return nil, true, e
	}
	f.plugins = filepath.Join(filepath.Dir(f.docker), "docker-cli-plugins")
	h, e = freshOpen(freshHandle(private), "client", true, windows.FILE_OPEN, false, false, false, false, false, nil)
	rootExists := e == nil
	if e != nil && !freshMissing(e) {
		return nil, true, errClientConfigRecovery
	}
	if rootExists {
		if _, e = f.holdDir(h, true, true); e != nil {
			return nil, true, e
		}
		f.rootNames, e = f.inventory(ctx, freshHandle(private), "client", f.root(), false)
		if e != nil {
			return nil, true, e
		}
		// Classify before creating any control. Retain permissive data preflight
		// handles only until same-object immutable admission after controller lock.
		oldOwner, ownerBody, x := f.optionalData(freshHandle(f.root()), "controller.lock", true)
		if x != nil || len(ownerBody) != 0 {
			return nil, true, errClientConfigRecovery
		}
		if oldOwner != nil {
			f.ownerPreflightID, f.ownerPreflightSD = oldOwner.id, oldOwner.sd
		}
		if oldOwner != nil && f.closeData(oldOwner) != nil {
			return nil, true, errClientConfigRecovery
		}
		f.init, f.initBody, e = f.optionalData(freshHandle(f.root()), "client.init", true)
		if e != nil {
			return nil, true, e
		}
		expected, x := f.expectedInit()
		if x != nil || f.init != nil && !freshInitPrefix(f.initBody, expected) {
			return nil, true, errClientConfigRecovery
		}
		h, e = freshOpen(freshHandle(f.root()), "docker", true, windows.FILE_OPEN, false, false, false, false, false, nil)
		if e == nil {
			f.tree, e = f.holdDir(h, true, false)
			if e != nil {
				return nil, true, e
			}
			f.dockerNames, e = f.inventory(ctx, freshHandle(f.root()), "docker", f.tree, true)
			if e != nil {
				return nil, true, e
			}
			f.config, f.configBody, e = f.optionalData(freshHandle(f.tree), "config.json", true)
			if e != nil || f.config != nil && !freshDefaultsPrefix(f.configBody, f.profile) {
				return nil, true, errClientConfigRecovery
			}
		} else if !freshMissing(e) {
			return nil, true, errClientConfigRecovery
		}
		// Every inventory entry must have an independently captured actual cap.
		// A delete/recreate race may not turn an unclassified entry into an
		// apparently absent record just because its name stayed unchanged.
		classified := []string{}
		if oldOwner != nil {
			classified = append(classified, "controller.lock")
		}
		if f.init != nil {
			classified = append(classified, "client.init")
		}
		if f.tree != nil {
			classified = append(classified, "docker")
		}
		sort.Strings(classified)
		if !freshNamesEqual(f.rootNames, classified) {
			return nil, true, errClientConfigRecovery
		}
		if f.tree != nil {
			classified = nil
			if f.config != nil {
				classified = []string{"config.json"}
			}
			if !freshNamesEqual(f.dockerNames, classified) {
				return nil, true, errClientConfigRecovery
			}
		}
	}
	if f.legacyAbsent() != nil {
		return nil, true, errClientConfigRecovery
	}
	sd, e := f.descriptor(true)
	if e != nil {
		return nil, true, e
	}
	if !rootExists {
		h, e = freshOpen(freshHandle(private), "client", true, windows.FILE_OPEN_IF, false, false, false, false, false, sd)
		if e != nil {
			return nil, true, e
		}
		if _, e = f.holdDir(h, true, true); e != nil {
			return nil, true, e
		}
	}
	sd, e = f.descriptor(false)
	if e != nil {
		return nil, true, e
	}
	h, e = freshOpen(freshHandle(f.root()), "controller.lock", false, windows.FILE_OPEN_IF, true, false, false, false, true, sd)
	if e != nil {
		return nil, true, errClientConfigRecovery
	}
	f.owner, e = f.holdData(h)
	if e != nil {
		return nil, true, e
	}
	if f.ownerPreflightID != "" && (f.owner.id != f.ownerPreflightID || f.owner.sd != f.ownerPreflightSD) {
		return nil, true, errClientConfigRecovery
	}
	ownerBody, e := f.owner.read(0)
	if e != nil || len(ownerBody) != 0 {
		return nil, true, errClientConfigRecovery
	}
	f.marker, e = f.immutable(freshHandle(private), "client.create-intent", f.marker, f.markerBody)
	if e != nil {
		return nil, true, e
	}
	f.init, e = f.immutable(freshHandle(f.root()), "client.init", f.init, f.initBody)
	if e != nil {
		return nil, true, e
	}
	if f.tree != nil {
		f.config, e = f.immutable(freshHandle(f.tree), "config.json", f.config, f.configBody)
		if e != nil {
			return nil, true, e
		}
	}
	// Controls are the only permitted additions since preflight.
	names, e := f.inventory(ctx, freshHandle(private), "client", f.root(), false)
	if e != nil {
		return nil, true, e
	}
	want := append([]string{}, f.rootNames...)
	if !rootExists || !containsFreshName(want, "controller.lock") {
		want = append(want, "controller.lock")
		sort.Strings(want)
	}
	if !freshNamesEqual(names, want) {
		return nil, true, errClientConfigRecovery
	}
	f.rootNames = names
	if f.recheck(ctx) != nil {
		return nil, true, errClientConfigRecovery
	}
	success = true
	return &clientWriterRecovery{freshInitialization: true, deadline: time.Now().Add(5 * time.Minute), closeNative: f.release, restoreNative: func() (*clientConfigGuard, error) { return f.apply(ctx) }}, true, nil
}
func containsFreshName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
func (f *nativeFreshInitialization) dispose(parent windows.Handle, name string, d *configData, body []byte) error {
	if d == nil || d.check() != nil {
		return errClientConfigRecovery
	}
	id, sd := d.id, d.sd
	if f.closeData(d) != nil {
		return errClientConfigRecovery
	}
	h, e := freshOpen(parent, name, false, windows.FILE_OPEN, false, true, false, false, true, nil)
	if e != nil {
		return errClientConfigRecovery
	}
	exact, e := f.holdData(h)
	if e != nil || exact.id != id || exact.sd != sd {
		return errClientConfigRecovery
	}
	actual, e := exact.read(freshClientRecordBudget)
	if e != nil || !bytes.Equal(actual, body) {
		return errClientConfigRecovery
	}
	var flag byte = 1
	var status windows.IO_STATUS_BLOCK
	if windows.NtSetInformationFile(h, &status, &flag, 1, 13) != nil {
		return errClientConfigRecovery
	}
	if f.closeData(exact) != nil {
		return errClientConfigRecovery
	}
	h, e = freshOpen(parent, name, false, windows.FILE_OPEN, false, false, false, true, false, nil)
	if freshMissing(e) {
		return nil
	}
	if e == nil {
		_, _ = f.holdData(h)
	}
	return errClientConfigRecovery
}
func (f *nativeFreshInitialization) createRecord(parent windows.Handle, name string, body []byte) (*configData, error) {
	if len(body) > freshClientRecordBudget {
		return nil, errClientConfigRecovery
	}
	sd, e := f.descriptor(false)
	if e != nil {
		return nil, e
	}
	h, e := freshOpen(parent, name, false, windows.FILE_CREATE, true, false, false, false, false, sd)
	if e != nil {
		return nil, errClientConfigRecovery
	}
	d, e := f.holdData(h)
	if e != nil {
		return nil, e
	}
	initialID, initialSD := d.id, d.sd
	count, e := d.file.Write(body)
	if e != nil || count != len(body) || d.file.Sync() != nil {
		return nil, errClientConfigRecovery
	}
	id, e := snapshot(h, false)
	if e != nil || !sameConfigObject(initialID, id) {
		return nil, errClientConfigRecovery
	}
	actualSD, e := dataPolicy(h, f.sid, false)
	if e != nil || actualSD != initialSD {
		return nil, errClientConfigRecovery
	}
	actual, e := io.ReadAll(io.NewSectionReader(d.file, 0, freshClientRecordBudget+1))
	if e != nil || !bytes.Equal(actual, body) {
		return nil, errClientConfigRecovery
	}
	if f.closeData(d) != nil {
		return nil, errClientConfigRecovery
	}
	readonly, actual, e := f.optionalData(parent, name, false)
	if e != nil || readonly == nil || !sameConfigObject(id, readonly.id) || readonly.sd != initialSD || !bytes.Equal(actual, body) {
		return nil, errClientConfigRecovery
	}
	return readonly, nil
}
func (f *nativeFreshInitialization) apply(ctx context.Context) (*clientConfigGuard, error) {
	if f.recheck(ctx) != nil {
		return nil, errClientConfigRecovery
	}
	if f.tree == nil {
		sd, e := f.descriptor(true)
		if e != nil {
			return nil, e
		}
		h, e := freshOpen(freshHandle(f.root()), "docker", true, windows.FILE_CREATE, false, false, false, false, false, sd)
		if e != nil {
			return nil, errClientConfigRecovery
		}
		f.tree, e = f.holdDir(h, true, false)
		if e != nil {
			return nil, e
		}
		f.rootNames = append(f.rootNames, "docker")
		sort.Strings(f.rootNames)
	}
	expected, e := f.expectedInit()
	if e != nil {
		return nil, e
	}
	initBody, _ := json.Marshal(expected)
	defaults, _ := json.Marshal(map[string]any{"auths": map[string]any{}, "credsStore": "orbwin", "cliPluginsExtraDirs": []string{f.plugins}})
	if !freshDefaultsPrefix(defaults, f.profile) {
		return nil, errClientConfigRecovery
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.init == nil || !sameSnapshotManifest(f.initBody, expected) {
		if f.init != nil && f.dispose(freshHandle(f.root()), "client.init", f.init, f.initBody) != nil {
			return nil, errClientConfigRecovery
		}
		f.init, e = f.createRecord(freshHandle(f.root()), "client.init", initBody)
		if e != nil {
			return nil, e
		}
		f.initBody = initBody
		if !containsFreshName(f.rootNames, "client.init") {
			f.rootNames = append(f.rootNames, "client.init")
			sort.Strings(f.rootNames)
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.config == nil || !bytes.Equal(f.configBody, defaults) {
		if f.config != nil && f.dispose(freshHandle(f.tree), "config.json", f.config, f.configBody) != nil {
			return nil, errClientConfigRecovery
		}
		f.config, e = f.createRecord(freshHandle(f.tree), "config.json", defaults)
		if e != nil {
			return nil, e
		}
		f.configBody = defaults
		f.dockerNames = []string{"config.json"}
	}
	if f.recheck(ctx) != nil || !sameSnapshotManifest(f.initBody, expected) || !bytes.Equal(f.configBody, defaults) || privateClientRouting(f.configBody, f.plugins) != nil || f.legacyAbsent() != nil {
		return nil, errClientConfigRecovery
	}
	// No writable record remains. The exclusive empty controller is the only
	// writable cap transferred into the completed ordinary writer ownership.
	for _, d := range f.data {
		if d != f.owner && d != f.marker && d != f.init && d != f.config && d.file != nil {
			return nil, errClientConfigRecovery
		}
	}
	if f.dispose(freshHandle(f.private()), "client.create-intent", f.marker, f.markerBody) != nil {
		return nil, errClientConfigRecovery
	}
	ancestors := make([]object, 0, len(f.ancestors)+1)
	for _, o := range f.ancestors {
		ancestors = append(ancestors, *o)
	}
	ancestors = append(ancestors, *f.tree)
	n := &nativeClientConfig{sid: f.sid, ancestors: ancestors, owner: f.owner, init: f.init, config: f.config, data: []*configData{f.owner, f.init, f.config}, path: filepath.Join(f.profile, ".orbwin-private", "client", "docker")}
	// This is the first point at which a complete nativeClientConfig exists.
	if n.check() != nil {
		return nil, errClientConfigRecovery
	}
	retainedConfigGuards.Lock()
	if retainedConfigGuards.failed || len(retainedConfigGuards.roots) >= 16 {
		retainedConfigGuards.Unlock()
		return nil, errClientConfigRecovery
	}
	retainedConfigGuards.roots = append(retainedConfigGuards.roots, n)
	retainedConfigGuards.Unlock()
	f.transferred = true
	retainedFreshInitializations.Lock()
	for i, v := range retainedFreshInitializations.roots {
		if v == f {
			retainedFreshInitializations.roots = append(retainedFreshInitializations.roots[:i], retainedFreshInitializations.roots[i+1:]...)
			break
		}
	}
	retainedFreshInitializations.Unlock()
	return &clientConfigGuard{native: n, path: n.path, plugins: f.plugins, body: defaults}, nil
}
