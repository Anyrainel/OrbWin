//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

type snapshotLease struct {
	held   []object
	path   string
	rootID string
}

var retainedSnapshots struct {
	sync.Mutex
	entries map[string]*snapshotLease
}

func snapshotDir(parent windows.Handle, name string, disposition uint32, sd *windows.SECURITY_DESCRIPTOR, exclusiveDelete bool) (windows.Handle, error) {
	if name != "snapshots" && !snapshotName(name) {
		return 0, errClientConfig
	}
	text, e := windows.NewNTUnicodeString(name)
	if e != nil {
		return 0, e
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE, SecurityDescriptor: sd}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE | windows.FILE_LIST_DIRECTORY)
	share := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE)
	if exclusiveDelete {
		access |= windows.DELETE
		share = 0
	}
	e = windows.NtCreateFile(&h, access, &oa, &status, nil, 0, share, disposition, windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	return h, e
}
func snapshotName(name string) bool {
	return len(name) == 68 && strings.HasPrefix(name, "cfg-") && canonicalPin(name[4:])
}
func cloneSnapshotObject(o object, sid string) (object, error) {
	var h windows.Handle
	if windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(o.file.Fd()), windows.CurrentProcess(), &h, 0, false, windows.DUPLICATE_SAME_ACCESS) != nil {
		return object{}, errClientConfig
	}
	return capture(h, sid, o.directory, o.private)
}
func snapshotNumbers(h windows.Handle, prefix string, out map[string]any) error {
	var id windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &id) != nil {
		return errClientConfig
	}
	out[prefix+"Volume"] = json.Number(strconv.FormatUint(uint64(id.VolumeSerialNumber), 10))
	out[prefix+"High"] = json.Number(strconv.FormatUint(uint64(id.FileIndexHigh), 10))
	out[prefix+"Low"] = json.Number(strconv.FormatUint(uint64(id.FileIndexLow), 10))
	return nil
}
func snapshotManifest(root, dir, file windows.Handle, sha, sid string) (map[string]any, error) {
	value := map[string]any{"schema": json.Number("1"), "product": "orbwin", "ownerSid": sid, "configSha256": sha}
	for _, v := range []struct {
		h      windows.Handle
		prefix string
	}{{root, "root"}, {dir, "generation"}, {file, "config"}} {
		if snapshotNumbers(v.h, v.prefix, value) != nil {
			return nil, errClientConfig
		}
	}
	return value, nil
}
func sameSnapshotManifest(data []byte, expected map[string]any) bool {
	actual, e := checkedClientJSON(data)
	if e != nil {
		return false
	}
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(expected)
	return string(a) == string(b)
}
func snapshotNames(ctx context.Context, h windows.Handle, contents bool) ([]string, error) {
	words := new([8192]uint64)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), 65536)
	var names []string
	class := uint32(windows.FileIdBothDirectoryRestartInfo)
	for batch := 0; batch < 4; batch++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		e := windows.GetFileInformationByHandleEx(h, class, &buf[0], uint32(len(buf)))
		if errors.Is(e, windows.ERROR_NO_MORE_FILES) {
			sort.Strings(names)
			return names, nil
		}
		if e != nil {
			return nil, errClientConfigRecovery
		}
		class = windows.FileIdBothDirectoryInfo
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
			if name != "." && name != ".." {
				if contents {
					if name != "config.json" && name != "generation.init" || len(names) >= 2 {
						return nil, errClientConfigRecovery
					}
				} else if !snapshotName(name) || len(names) >= 32 {
					return nil, errClientConfigRecovery
				}
				names = append(names, name)
			}
			if next == 0 {
				break
			}
			if next < 104+length || next%8 != 0 || at+next >= len(buf) {
				return nil, errClientConfigRecovery
			}
			at += next
		}
		clear(buf)
	}
	return nil, errClientConfigRecovery
}

type nativePrivateRecord struct {
	n                  *nativeClientConfig
	parent             windows.Handle
	name               string
	writable, held     *configData
	beforeID, beforeSD string
}

func (r *nativePrivateRecord) createNew() error {
	descriptor, e := windows.SecurityDescriptorFromString("O:" + r.n.sid + "D:P(A;;FA;;;" + r.n.sid + ")(A;;FA;;;SY)(A;;FA;;;BA)")
	if e != nil {
		return e
	}
	h, e := configOpen(r.parent, r.name, windows.FILE_CREATE, true, r.name == "config.stage", false, descriptor)
	if e != nil {
		return e
	}
	r.writable, e = r.n.hold(h, false)
	return e
}
func (r *nativePrivateRecord) writeAll(data []byte) error {
	if e := recoveryQualificationPrefix(r, data); e != nil {
		return e
	}
	count, e := r.writable.file.Write(data)
	if e != nil || count != len(data) {
		return errClientConfigRecovery
	}
	return nil
}
func (r *nativePrivateRecord) flush() error { return r.writable.file.Sync() }
func (r *nativePrivateRecord) readWritten() ([]byte, error) {
	var e error
	r.beforeID, e = snapshot(windows.Handle(r.writable.file.Fd()), false)
	if e != nil {
		return nil, e
	}
	r.beforeSD, e = dataPolicy(windows.Handle(r.writable.file.Fd()), r.n.sid, false)
	if e != nil {
		return nil, e
	}
	return io.ReadAll(io.NewSectionReader(r.writable.file, 0, clientConfigBudget+1))
}
func (r *nativePrivateRecord) closeWriter() error {
	if e := r.writable.file.Close(); e != nil {
		return e
	}
	r.writable.file = nil
	return nil
}
func (r *nativePrivateRecord) openReadOnly() error {
	h, e := configOpen(r.parent, r.name, windows.FILE_OPEN, false, r.name == "config.stage", false, nil)
	if e != nil {
		return e
	}
	r.held, e = r.n.hold(h, false)
	if e != nil || !sameConfigObject(r.beforeID, r.held.id) || r.beforeSD != r.held.sd {
		return errClientConfigRecovery
	}
	return nil
}
func (r *nativePrivateRecord) readHeld() ([]byte, error) { return r.held.read(clientConfigBudget) }
func (n *nativeClientConfig) createSnapshotRecord(parent windows.Handle, name string, data []byte) (*configData, error) {
	record := &nativePrivateRecord{n: n, parent: parent, name: name}
	if e := durablePrivateRecord(data, record); e != nil {
		return nil, errClientConfigRecovery
	}
	return record.held, nil
}
func (n *nativeClientConfig) startSnapshotIntent(body []byte) error {
	n.poison = true
	record, e := n.createSnapshotRecord(n.parent(), "snapshot.intent", body)
	if e != nil {
		return errClientConfigRecovery
	}
	n.intent = record
	n.intentName = "snapshot.intent"
	return nil
}
func exclusiveSnapshotFile(parent windows.Handle, name string) (windows.Handle, error) {
	return configOpen(parent, name, windows.FILE_OPEN, false, true, true, nil)
}
func disposeSnapshotFile(record *configData) error {
	var flag byte = 1
	var status windows.IO_STATUS_BLOCK
	if windows.NtSetInformationFile(windows.Handle(record.file.Fd()), &status, &flag, 1, 13) != nil {
		return errClientConfigRecovery
	}
	if record.file.Close() != nil {
		return errClientConfigRecovery
	}
	record.file = nil
	return nil
}
func snapshotAbsent(parent windows.Handle, name string, dir bool) error {
	var h windows.Handle
	var e error
	if dir {
		h, e = snapshotDir(parent, name, windows.FILE_OPEN, nil, false)
	} else {
		h, e = configOpen(parent, name, windows.FILE_OPEN, false, name == "config.stage", false, nil)
	}
	if errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
		return nil
	}
	if e == nil {
		_ = windows.CloseHandle(h)
	}
	return errClientConfigRecovery
}
func (n *nativeClientConfig) evictSnapshot(ctx context.Context, parent windows.Handle, names []string, current string) error {
	for _, name := range names {
		if name == current {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		h, e := snapshotDir(parent, name, windows.FILE_OPEN, nil, true)
		if errors.Is(e, windows.NTStatus(0xc0000043)) {
			continue
		}
		if e != nil {
			return errClientConfigRecovery
		}
		capturedDirectory, e := capture(h, n.sid, true, true)
		directory := n.retainAux(capturedDirectory)
		if e != nil {
			return errClientConfigRecovery
		}
		// Keep exclusive DELETE directory handle through final absence. Future
		// readers deny DELETE sharing and therefore cannot enter after this proof.
		entries, e := snapshotNames(ctx, h, true)
		if e != nil || len(entries) != 2 || entries[0] != "config.json" || entries[1] != "generation.init" {
			return errClientConfigRecovery
		}
		file, e := exclusiveSnapshotFile(h, "config.json")
		if errors.Is(e, windows.NTStatus(0xc0000043)) {
			if n.closeAux(directory) != nil {
				return errClientConfigRecovery
			}
			continue
		}
		if e != nil {
			return errClientConfigRecovery
		}
		config, e := n.hold(file, false)
		if e != nil {
			return errClientConfigRecovery
		}
		meta, e := exclusiveSnapshotFile(h, "generation.init")
		if errors.Is(e, windows.NTStatus(0xc0000043)) {
			if config.file.Close() != nil {
				n.poison = true
				return errClientConfigRecovery
			}
			config.file = nil
			if n.closeAux(directory) != nil {
				return errClientConfigRecovery
			}
			continue
		}
		if e != nil {
			return errClientConfigRecovery
		}
		init, e := n.hold(meta, false)
		if e != nil {
			return errClientConfigRecovery
		}
		bytes, e := config.read(clientConfigBudget)
		if e != nil {
			return e
		}
		sha := sha256.Sum256(bytes)
		digest := hex.EncodeToString(sha[:])
		expected, e := snapshotManifest(n.parent(), h, file, digest, n.sid)
		if e != nil {
			return e
		}
		record, e := init.read(8192)
		if e != nil || name != "cfg-"+digest || !sameSnapshotManifest(record, expected) {
			return errClientConfigRecovery
		}
		intent, _ := json.Marshal(map[string]any{"schema": 1, "kind": "evict-derived-snapshot", "generation": name, "binding": expected})
		if n.startSnapshotIntent(intent) != nil {
			return errClientConfigRecovery
		}
		if disposeSnapshotFile(config) != nil || snapshotAbsent(h, "config.json", false) != nil || disposeSnapshotFile(init) != nil || snapshotAbsent(h, "generation.init", false) != nil {
			return errClientConfigRecovery
		}
		var flag byte = 1
		var status windows.IO_STATUS_BLOCK
		if windows.NtSetInformationFile(h, &status, &flag, 1, 13) != nil {
			return errClientConfigRecovery
		}
		if directory.file.Close() != nil {
			return errClientConfigRecovery
		}
		directory.file = nil
		if snapshotAbsent(parent, name, true) != nil {
			return errClientConfigRecovery
		}
		if n.finishIntent() != nil {
			return errClientConfigRecovery
		}
		return nil
	}
	return errors.New("all private client config generations are active; close an old reader and retry")
}
func publishClientSnapshot(ctx context.Context, guard *clientConfigGuard) (string, error) {
	n, ok := guard.native.(*nativeClientConfig)
	if !ok || n.check() != nil || n.config == nil || n.config.check() != nil {
		return "", errClientConfig
	}
	name := configGeneration(guard.body)
	key := n.sid + ":" + n.ancestors[len(n.ancestors)-2].id + ":" + name
	retainedSnapshots.Lock()
	defer retainedSnapshots.Unlock()
	if retainedSnapshots.entries == nil {
		retainedSnapshots.entries = map[string]*snapshotLease{}
	}
	if existing := retainedSnapshots.entries[key]; existing != nil {
		for _, h := range existing.held {
			if h.recheck() != nil {
				return "", errClientConfigRecovery
			}
		}
		return existing.path, nil
	}
	if len(retainedSnapshots.entries) >= 32 {
		return "", errors.New("32 active client snapshots in this CLI; close old readers before retry")
	}
	descriptor, e := windows.SecurityDescriptorFromString("O:" + n.sid + "D:P(A;OICI;FA;;;" + n.sid + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if e != nil {
		return "", e
	}
	h, e := snapshotDir(n.parent(), "snapshots", windows.FILE_OPEN, nil, false)
	if e == nil {
		capturedParent, e := capture(h, n.sid, true, true)
		parent := n.retainAux(capturedParent)
		if e != nil {
			return "", e
		}
		probe, probeErr := snapshotDir(h, name, windows.FILE_OPEN, nil, false)
		if probeErr == nil {
			probeObject, e := capture(probe, n.sid, true, true)
			retained := n.retainAux(probeObject)
			if e != nil || n.closeAux(retained) != nil {
				return "", errClientConfigRecovery
			}
		} else if errors.Is(probeErr, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
			names, e := snapshotNames(ctx, h, false)
			if e != nil {
				return "", e
			}
			if len(names) >= 32 {
				if n.evictSnapshot(ctx, h, names, name) != nil {
					return "", errClientConfigRecovery
				}
			}
		} else {
			return "", errClientConfigRecovery
		}
		if n.closeAux(parent) != nil {
			return "", errClientConfigRecovery
		}
	} else if !errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
		return "", errClientConfigRecovery
	}
	intent, _ := json.Marshal(map[string]any{"schema": 1, "kind": "snapshot", "generation": name})
	if n.startSnapshotIntent(intent) != nil {
		return "", errClientConfigRecovery
	}
	lease := &snapshotLease{path: filepath.Join(filepath.Dir(guard.path), "snapshots", name), rootID: n.ancestors[len(n.ancestors)-2].id}
	retainedSnapshots.entries[key] = lease
	for _, ancestor := range n.ancestors {
		copy, e := cloneSnapshotObject(ancestor, n.sid)
		if copy.file != nil {
			lease.held = append(lease.held, copy)
		}
		if e != nil {
			return "", e
		}
	}
	h, e = snapshotDir(n.parent(), "snapshots", windows.FILE_OPEN_IF, descriptor, false)
	if e != nil {
		return "", errClientConfigRecovery
	}
	parent, e := capture(h, n.sid, true, true)
	if parent.file != nil {
		lease.held = append(lease.held, parent)
	}
	if e != nil {
		return "", e
	}
	generation, e := snapshotDir(h, name, windows.FILE_OPEN, nil, false)
	fresh := errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND)
	if fresh {
		generation, e = snapshotDir(h, name, windows.FILE_CREATE, descriptor, false)
	}
	if e != nil {
		return "", errClientConfigRecovery
	}
	dir, e := capture(generation, n.sid, true, true)
	if dir.file != nil {
		lease.held = append(lease.held, dir)
	}
	if e != nil {
		return "", e
	}
	if fresh {
		created, e := n.createSnapshotRecord(generation, "config.json", guard.body)
		if e != nil {
			return "", e
		}
		if created.file.Close() != nil {
			return "", errClientConfigRecovery
		}
		created.file = nil
	}
	configHandle, e := configOpen(generation, "config.json", windows.FILE_OPEN, false, false, false, nil)
	if e != nil {
		return "", e
	}
	config, e := capture(configHandle, n.sid, false, true)
	if config.file != nil {
		lease.held = append(lease.held, config)
	}
	if e != nil {
		return "", e
	}
	actual, e := io.ReadAll(io.NewSectionReader(config.file, 0, clientConfigBudget+1))
	if e != nil || string(actual) != string(guard.body) {
		return "", errClientConfigRecovery
	}
	expected, e := snapshotManifest(n.parent(), generation, configHandle, name[4:], n.sid)
	if e != nil {
		return "", e
	}
	if fresh {
		body, _ := json.Marshal(expected)
		created, e := n.createSnapshotRecord(generation, "generation.init", body)
		if e != nil {
			return "", e
		}
		if created.file.Close() != nil {
			return "", errClientConfigRecovery
		}
		created.file = nil
	}
	initHandle, e := configOpen(generation, "generation.init", windows.FILE_OPEN, false, false, false, nil)
	if e != nil {
		return "", e
	}
	init, e := capture(initHandle, n.sid, false, true)
	if init.file != nil {
		lease.held = append(lease.held, init)
	}
	if e != nil {
		return "", e
	}
	meta, e := io.ReadAll(io.NewSectionReader(init.file, 0, 8193))
	if e != nil || len(meta) > 8192 || !sameSnapshotManifest(meta, expected) {
		return "", errClientConfigRecovery
	}
	if n.finishIntent() != nil {
		return "", errClientConfigRecovery
	}
	return lease.path, nil
}
