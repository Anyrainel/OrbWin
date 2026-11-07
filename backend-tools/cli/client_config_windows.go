//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// Consumer of the desktop-created namespace. Writer settlement may replace the
// root only with a scope-merged owner-authored stage under the retained intent.
// It never initializes/imports private roots or touches WinCredMan.
type configData struct {
	file        *os.File
	id, sd, sid string
	writer      bool
}

func dataPolicy(h windows.Handle, sid string, writer bool) (string, error) {
	if !writer {
		return objectPolicy(h, sid, false, true)
	}
	sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil || sd == nil {
		return "", errClientConfig
	}
	owner, _, e := sd.Owner()
	if e != nil || owner == nil || owner.String() != sid {
		return "", errClientConfig
	}
	acl, _, e := sd.DACL()
	if e != nil || acl == nil || acl.AceCount != 3 {
		return "", errClientConfig
	}
	control, _, e := sd.Control()
	if e != nil {
		return "", errClientConfig
	}
	seen := map[string]bool{}
	var uniform byte
	for i := uint32(0); i < 3; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil || ace.Header.AceSize < 16 || ace.Header.AceType != 0 || uint32(ace.Mask) != 0x1f01ff {
			return "", errClientConfig
		}
		principal := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !principal.IsValid() || principal.Len() > int(ace.Header.AceSize)-8 {
			return "", errClientConfig
		}
		name := principal.String()
		if seen[name] || (name != sid && name != "S-1-5-18" && name != "S-1-5-32-544") {
			return "", errClientConfig
		}
		seen[name] = true
		if i == 0 {
			uniform = ace.Header.AceFlags
		}
		if ace.Header.AceFlags != uniform {
			return "", errClientConfig
		}
	}
	if (uniform == 0x10 && control&windows.SE_DACL_PROTECTED != 0) || (uniform != 0x10 && (uniform != 0 || control&windows.SE_DACL_PROTECTED == 0)) {
		return "", errClientConfig
	}
	sum := sha256.Sum256([]byte(sd.String()))
	return hex.EncodeToString(sum[:]), nil
}
func captureConfig(h windows.Handle, sid string, writer bool) (*configData, error) {
	file := os.NewFile(uintptr(h), "private client config object")
	if file == nil {
		return nil, errClientConfig
	}
	data := &configData{file: file, sid: sid, writer: writer}
	id, e := snapshot(h, false)
	if e != nil {
		return data, e
	}
	sd, e := dataPolicy(h, sid, writer)
	if e != nil {
		return data, e
	}
	data.id = id
	data.sd = sd
	return data, nil
}
func (d *configData) check() error {
	if d == nil || d.file == nil {
		return errClientConfig
	}
	h := windows.Handle(d.file.Fd())
	id, e := snapshot(h, false)
	if e != nil || id != d.id {
		return errClientConfig
	}
	sd, e := dataPolicy(h, d.sid, d.writer)
	if e != nil || sd != d.sd {
		return errClientConfig
	}
	return nil
}
func (d *configData) read(budget int64) ([]byte, error) {
	if d.check() != nil {
		return nil, errClientConfig
	}
	info, e := d.file.Stat()
	if e != nil || info.Size() > budget {
		return nil, errClientConfig
	}
	out, e := io.ReadAll(io.NewSectionReader(d.file, 0, budget+1))
	if e != nil || int64(len(out)) > budget || d.check() != nil {
		return nil, errClientConfig
	}
	return out, nil
}
func configOpen(parent windows.Handle, name string, disposition uint32, write, remove, exclusive bool, sd *windows.SECURITY_DESCRIPTOR) (windows.Handle, error) {
	if name != "client.create-intent" && name != "client.import-intent" && name != "controller.lock" && name != "writer.prepare" && name != "writer.intent" && name != "writer.original" && name != "writer.birth" && name != "recovery.intent" && name != "recovery.next" && name != "client.init" && name != "config.json" && name != "snapshot.intent" && name != "generation.init" && name != "config.stage" {
		return 0, errClientConfig
	}
	text, e := windows.NewNTUnicodeString(name)
	if e != nil {
		return 0, errClientConfig
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE, SecurityDescriptor: sd}
	oa.Length = uint32(unsafe.Sizeof(oa))
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE | windows.GENERIC_READ)
	if write {
		access |= windows.GENERIC_WRITE
	}
	if remove || name == "recovery.next" {
		access |= windows.DELETE
	}
	share := uint32(windows.FILE_SHARE_READ)
	if exclusive {
		share = 0
	}
	var h windows.Handle
	var status windows.IO_STATUS_BLOCK
	e = windows.NtCreateFile(&h, access, &oa, &status, nil, 0, share, disposition, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	runtime.KeepAlive(sd)
	return h, e
}

type nativeClientConfig struct {
	sid                         string
	ancestors                   []object
	owner, init, config, intent *configData
	original                    *configData
	birth                       *configData
	prepare                     *configData
	unknownQuery                windows.Handle
	data                        []*configData
	poison                      bool
	successorUnknown            bool
	mutation                    privateMutationState
	path                        string
	intentName                  string
	originalID, originalSD      string
	originalSHA                 [32]byte
	auxiliary                   []*object
}

type nativeConfigRegistry struct {
	sync.Mutex
	roots  []*nativeClientConfig
	failed bool
}

var retainedConfigGuards nativeConfigRegistry

func (r *nativeConfigRegistry) fence() {
	r.Lock()
	r.failed = true
	r.Unlock()
}

// The registry owns strong retention and the process-wide admission fence.
// It is not the native controller. Never hold its mutex during native work:
// a failed rename/close must be able to reenter fence() from that work.
func (r *nativeConfigRegistry) acquire(n *nativeClientConfig, work func() (*clientConfigGuard, error)) (*clientConfigGuard, error) {
	if r == nil || n == nil || work == nil {
		return nil, errClientConfigRecovery
	}
	r.Lock()
	if r.failed || len(r.roots) >= 16 {
		r.Unlock()
		return nil, errClientConfigRecovery
	}
	r.roots = append(r.roots, n) // retain BEFORE any native effect
	r.Unlock()
	completed := false
	defer func() {
		if !completed {
			n.poison = true
			r.fence() // also runs during panic; no cleanup or retention release
		}
	}()
	guard, err := work()
	if err != nil {
		return nil, err
	}
	if guard == nil || guard.native != n {
		return nil, errClientConfigRecovery
	}
	r.Lock()
	completed = !r.failed
	r.Unlock()
	if !completed {
		return nil, errClientConfigRecovery
	}
	return guard, nil
}

func (n *nativeClientConfig) parent() windows.Handle {
	return windows.Handle(n.ancestors[len(n.ancestors)-2].file.Fd())
}
func (n *nativeClientConfig) directory() windows.Handle {
	return windows.Handle(n.ancestors[len(n.ancestors)-1].file.Fd())
}
func (n *nativeClientConfig) hold(h windows.Handle, writer bool) (*configData, error) {
	d, e := captureConfig(h, n.sid, writer)
	if d != nil {
		n.data = append(n.data, d)
	}
	return d, e
}
func (n *nativeClientConfig) check() error {
	if n == nil || n.poison {
		return errClientConfigRecovery
	}
	return n.checkOrigin()
}
func (n *nativeClientConfig) checkOrigin() error {
	if n == nil || n.successorUnknown || !n.mutation.admissible() {
		return errClientConfigRecovery
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ordinary() != nil {
		return errClientConfig
	}
	sid, _, _, e := processFacts(windows.CurrentProcess())
	if e != nil || sid != n.sid {
		return errClientConfig
	}
	for _, o := range n.ancestors {
		if o.recheck() != nil {
			return errClientConfig
		}
	}
	for _, d := range []*configData{n.owner, n.init} {
		if d == nil || d.check() != nil {
			return errClientConfig
		}
	}
	return nil
}

// Metadata is generated from actual held handles. No caller-provided native ID
// or descriptor hash is accepted. The exact original never enters JSON/errors.
func configDescriptorSHA(h windows.Handle) (string, error) {
	sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil || sd == nil || !sd.IsValid() {
		return "", errClientConfigRecovery
	}
	control, _, e := sd.Control()
	length := sd.Length()
	if e != nil || control&windows.SE_SELF_RELATIVE == 0 || length < 20 || length > 65536 {
		return "", errClientConfigRecovery
	}
	digest := sha256.Sum256(unsafe.Slice((*byte)(unsafe.Pointer(sd)), int(length)))
	runtime.KeepAlive(sd)
	return hex.EncodeToString(digest[:]), nil
}
func (n *nativeClientConfig) beginWriter(binding [32]byte, scope clientWriterScope, keys []string, base []byte) error {
	if n.check() != nil || n.config == nil || n.config.check() != nil || binding == [32]byte{} {
		return errClientConfig
	}
	actual, e := n.config.read(clientConfigBudget)
	if e != nil || string(actual) != string(base) {
		return errClientConfig
	}
	n.originalID, n.originalSD = n.config.id, n.config.sd
	n.originalSHA = sha256.Sum256(actual)
	n.poison = true
	// Fixed CREATE_NEW + protected three-principal FA descriptor; writer close
	// and independently verified read-only admission precede intent and loan.
	baseSD, e := configDescriptorSHA(windows.Handle(n.config.file.Fd()))
	if e != nil {
		return errClientConfigRecovery
	}
	keyBytes, e := writerKeysJSON(keys)
	if e != nil {
		return errClientConfigRecovery
	}
	keySHA := sha256.Sum256(keyBytes)
	value := map[string]any{
		"schema": 3, "kind": "docker-writer", "binding": hex.EncodeToString(binding[:]),
		"mutation": string(scope), "keysSha256": hex.EncodeToString(keySHA[:]),
		"baseSha256": hex.EncodeToString(n.originalSHA[:]), "baseSdSha256": baseSD,
	}
	if snapshotNumbers(windows.Handle(n.config.file.Fd()), "base", value) != nil {
		return errClientConfigRecovery
	}
	value["kind"] = "docker-writer-prepare"
	prepare, e := json.Marshal(value)
	if e != nil || len(prepare) > 4096 {
		return errClientConfigRecovery
	}
	n.prepare, e = n.createMutationRecord(n.parent(), "writer.prepare", prepare)
	if e != nil {
		return errClientConfigRecovery
	}
	n.original, e = n.createMutationRecord(n.parent(), "writer.original", actual)
	if e != nil {
		return errClientConfigRecovery
	}
	originalSD, e := configDescriptorSHA(windows.Handle(n.original.file.Fd()))
	if e != nil {
		return errClientConfigRecovery
	}
	value["kind"] = "docker-writer"
	value["originalSha256"], value["originalSdSha256"] = hex.EncodeToString(n.originalSHA[:]), originalSD
	for _, entry := range []struct {
		data   *configData
		prefix string
	}{{n.config, "base"}, {n.original, "original"}} {
		if snapshotNumbers(windows.Handle(entry.data.file.Fd()), entry.prefix, value) != nil {
			return errClientConfigRecovery
		}
	}
	intent, e := json.Marshal(value)
	if e != nil || len(intent) > 4096 {
		return errClientConfigRecovery
	}
	n.intent, e = n.createMutationRecord(n.parent(), "writer.intent", intent)
	if e != nil {
		return errClientConfigRecovery
	}
	n.intentName = "writer.intent"
	if _, e = n.readOriginal(); e != nil {
		return errClientConfigRecovery
	}
	if n.config.check() != nil {
		return errClientConfigRecovery
	}
	if e = n.config.file.Close(); e != nil {
		return n.successorFence()
	}
	n.config.file, n.config = nil, nil
	n.poison = false
	return nil
}
func (n *nativeClientConfig) readOriginal() ([]byte, error) {
	if n.original == nil || n.originalID == "" || n.checkOrigin() != nil {
		return nil, errClientConfigRecovery
	}
	actual, e := n.original.read(clientConfigBudget)
	if e != nil || sha256.Sum256(actual) != n.originalSHA {
		return nil, errClientConfigRecovery
	}
	return actual, nil
}
func sameConfigObject(a, b string) bool {
	aa := strings.Split(a, ":")
	bb := strings.Split(b, ":")
	return len(aa) >= 4 && len(bb) >= 4 && strings.Join(aa[:4], ":") == strings.Join(bb[:4], ":")
}
func (n *nativeClientConfig) readAfterWriter(plugins string) ([]byte, error) {
	if n.check() != nil || n.intent == nil || n.intent.check() != nil {
		return nil, errClientConfigRecovery
	}
	n.poison = true
	var writable *configData
	var originalID, originalSD string
	var bytes []byte
	open := func(write bool) (*configData, error) {
		deadline := time.Now().Add(250 * time.Millisecond)
		for {
			// Separate remove=false: flush admission never requests DELETE.
			h, e := configOpen(n.directory(), "config.json", windows.FILE_OPEN, write, false, false, nil)
			if e == nil {
				return n.hold(h, true)
			}
			if !errors.Is(e, windows.NTStatus(0xc0000043)) || time.Now().After(deadline) {
				return nil, errClientConfigRecovery
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	e := writerDurabilitySequence(func(stage writerCommitStep) error {
		switch stage {
		case writerAdmitWritable:
			var e error
			writable, e = open(true)
			if e != nil {
				return e
			}
			bytes, e = writable.read(clientConfigBudget)
			if e != nil || privateClientRouting(bytes, plugins) != nil {
				return errClientConfigRecovery
			}
			originalID, originalSD = writable.id, writable.sd
		case writerFlush:
			if writable.file.Sync() != nil {
				return errClientConfigRecovery
			}
		case writerRecheckFlushed:
			actual, e := writable.read(clientConfigBudget)
			if e != nil || string(actual) != string(bytes) {
				return errClientConfigRecovery
			}
		case writerCloseWritable:
			if writable.file.Close() != nil {
				return n.successorFence()
			}
			writable.file = nil
		case writerAdmitReadOnly:
			data, e := open(false)
			if e != nil {
				return e
			}
			actual, e := data.read(clientConfigBudget)
			if e != nil || data.id != originalID || data.sd != originalSD || string(actual) != string(bytes) {
				return errClientConfigRecovery
			}
			n.config = data
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return bytes, nil
}
func (n *nativeClientConfig) finishIntent() error {
	if !n.poison {
		return errClientConfigRecovery
	}
	e := finishPrivateMutation(func() error {
		if n.intent == nil || n.checkOrigin() != nil || n.intent.check() != nil || n.config == nil || n.config.check() != nil {
			return errClientConfigRecovery
		}
		if n.original != nil {
			if _, e := n.readOriginal(); e != nil {
				return errClientConfigRecovery
			}
		}
		return nil
	}, func() error {
		if e := n.disposeExactRecord(n.intentName, n.intent); e != nil {
			return e
		}
		n.intent = nil
		return nil
	}, func() error {
		// Original is last. A crash after intent removal leaves an orphan;
		// startup refuses it. No disk marker supplies execution quiescence.
		if n.original != nil {
			if _, e := n.readOriginal(); e != nil {
				return errClientConfigRecovery
			}
			if e := n.disposeExactRecord("writer.original", n.original); e != nil {
				return e
			}
			n.original = nil
		}
		return nil
	})
	if e != nil {
		return errClientConfigRecovery
	}
	if n.prepare != nil {
		if e := n.disposeExactRecord("writer.prepare", n.prepare); e != nil {
			return errClientConfigRecovery
		}
		n.prepare = nil
	}
	if n.birth != nil {
		if e := n.disposeExactRecord("writer.birth", n.birth); e != nil {
			return errClientConfigRecovery
		}
		n.birth = nil
	}
	n.poison = false
	return n.check()
}
func (n *nativeClientConfig) disposeExactRecord(name string, record *configData) error {
	if n.checkOrigin() != nil || record == nil || record.check() != nil {
		return errClientConfigRecovery
	}
	expectedID, expectedSD := record.id, record.sd
	expectedBody, e := record.read(clientConfigBudget)
	if e != nil {
		return errClientConfigRecovery
	}
	if record.file.Close() != nil {
		return n.successorFence()
	}
	record.file = nil
	h, e := configOpen(n.parent(), name, windows.FILE_OPEN, false, true, false, nil)
	if e != nil {
		return errClientConfigRecovery
	}
	fresh, e := n.hold(h, false)
	if e != nil || fresh.id != expectedID || fresh.sd != expectedSD {
		return errClientConfigRecovery
	}
	actual, e := fresh.read(clientConfigBudget)
	if e != nil || !bytes.Equal(actual, expectedBody) {
		return errClientConfigRecovery
	}
	return n.disposeOwnedFile(n.parent(), name, fresh)
}

// The delete-capable object is already captured in n.data. Admission refusals
// precede disposition; after disposition starts every suffix error retains it.
func (n *nativeClientConfig) disposeOwnedFile(parent windows.Handle, name string, record *configData) error {
	return checkedPrivateMutation(&n.mutation, func() error {
		if n.checkOrigin() != nil || !n.mutationDataHeld(record) || record.check() != nil {
			return errClientConfigRecovery
		}
		return nil
	}, func() error {
		var disposition byte = 1
		var status windows.IO_STATUS_BLOCK
		e := windows.NtSetInformationFile(windows.Handle(record.file.Fd()), &status, &disposition, 1, 13)
		if e == nil && name == "config.stage" && recoveryQualificationLost(n, recoveryLostStageDisposition) {
			observeQualificationDeletePending(record)
			return errClientConfigRecovery
		}
		return e
	}, func() error {
		if record.file.Close() != nil {
			return errClientConfigRecovery
		}
		record.file = nil
		return n.verifyMutationAbsence(parent, name)
	}, n.successorFence)
}

func (n *nativeClientConfig) mutationDataHeld(record *configData) bool {
	if record != nil {
		for _, held := range n.data {
			if held == record {
				return true
			}
		}
	}
	return false
}

// Called only within the closed post-mutation verifier. Do not call public
// checkOrigin here: it deliberately refuses a still-live native attempt.
func (n *nativeClientConfig) verifyMutationAbsence(parent windows.Handle, name string) error {
	h, e := configOpen(parent, name, windows.FILE_OPEN, false, false, false, nil)
	if errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
		return nil
	}
	if e == nil {
		_, _ = n.hold(h, false) // retain this unexpected actual object; no suffix close
	}
	return errClientConfigRecovery
}

// These existing writer/local publication records need the same checked writer
// close boundary as NEXT. A known pre-publication write/admission error can keep
// its journal for review; an actual Close error immediately quarantines owner.
func (n *nativeClientConfig) createMutationRecord(parent windows.Handle, name string, body []byte) (*configData, error) {
	record := &successorRecordIO{&nativePrivateRecord{n: n, parent: parent, name: name}}
	if e := durablePrivateRecord(body, record); e != nil {
		return nil, e
	}
	return record.held, nil
}
func (n *nativeClientConfig) replaceManagedConfig(next []byte) error {
	if n.check() != nil || n.config == nil || n.config.check() != nil {
		return errClientConfig
	}
	old, e := n.config.read(clientConfigBudget)
	if e != nil {
		return e
	}
	base := sha256.Sum256(old)
	intent, e := localReplaceProducerBytes(hex.EncodeToString(base[:]), "")
	if e != nil {
		return e
	}
	n.poison = true
	n.intent, e = n.createMutationRecord(n.parent(), "writer.intent", intent)
	if e != nil {
		return errClientConfigRecovery
	}
	n.intentName = "writer.intent"
	expected := n.config.id
	if n.config.file.Close() != nil {
		return n.successorFence()
	}
	n.config.file = nil
	handle, e := configOpen(n.directory(), "config.json", windows.FILE_OPEN, false, false, false, nil)
	if e != nil {
		return errClientConfigRecovery
	}
	target, e := n.hold(handle, true)
	if e != nil || target.id != expected {
		return errClientConfigRecovery
	}
	body, e := target.read(clientConfigBudget)
	if e != nil || string(body) != string(old) {
		return errClientConfigRecovery
	}
	if target.file.Close() != nil {
		return n.successorFence()
	}
	target.file = nil
	if e = n.publishMergedConfig(next); e != nil {
		return e
	}
	return n.finishIntent()
}

func (n *nativeClientConfig) commitMergedConfig(next []byte) error {
	if !n.poison || n.intent == nil || n.intent.check() != nil || n.checkOrigin() != nil || n.config == nil || n.config.check() != nil {
		return errClientConfigRecovery
	}
	if n.config.file.Close() != nil {
		return n.successorFence()
	}
	n.config.file = nil
	return n.publishMergedConfig(next)
}

// Caller owns an existing durable intent. Never clear it here: any failed write,
// flush, close, atomic publication or independent reopen requires recovery.
func (n *nativeClientConfig) publishMergedConfig(next []byte) error {
	if n.checkOrigin() != nil {
		return errClientConfigRecovery
	}
	stage, e := n.createMutationRecord(n.directory(), "config.stage", next)
	if e != nil {
		return errClientConfigRecovery
	}
	recoveryQualificationCheckpoint(n, recoveryHistoricalStagePrepared)
	id, sd := stage.id, stage.sd
	name, _ := windows.UTF16FromString("config.json")
	name = name[:len(name)-1]
	size := 20 + len(name)*2
	words := make([]uint64, (size+7)/8)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
	buf[0] = 1
	*(*windows.Handle)(unsafe.Pointer(&buf[8])) = n.directory()
	*(*uint32)(unsafe.Pointer(&buf[16])) = uint32(len(name) * 2)
	for i, u := range name {
		*(*uint16)(unsafe.Pointer(&buf[20+i*2])) = u
	}
	return checkedPrivateMutation(&n.mutation, func() error {
		if n.checkOrigin() != nil || !n.mutationDataHeld(stage) || stage.check() != nil {
			return errClientConfigRecovery
		}
		actual, e := stage.read(clientConfigBudget)
		if e != nil || !bytes.Equal(actual, next) {
			return errClientConfigRecovery
		}
		return nil
	}, func() error {
		var status windows.IO_STATUS_BLOCK
		e := windows.NtSetInformationFile(windows.Handle(stage.file.Fd()), &status, &buf[0], uint32(size), 10)
		if e == nil && recoveryQualificationLost(n, recoveryLostTargetRename) {
			return errClientConfigRecovery
		}
		return e
	}, func() error {
		if stage.file.Close() != nil {
			return errClientConfigRecovery
		}
		stage.file = nil
		handle, e := configOpen(n.directory(), "config.json", windows.FILE_OPEN, false, false, false, nil)
		if e != nil {
			return errClientConfigRecovery
		}
		current, e := n.hold(handle, true)
		if e != nil || !sameConfigObject(id, current.id) || current.sd != sd {
			return errClientConfigRecovery
		}
		actual, e := current.read(clientConfigBudget)
		if e != nil || !bytes.Equal(actual, next) {
			return errClientConfigRecovery
		}
		n.config = current
		return n.verifyMutationAbsence(n.directory(), "config.stage")
	}, n.successorFence)
}
func (n *nativeClientConfig) abortUnstarted() ([]byte, error) {
	if n.check() != nil || n.intent == nil || n.originalID == "" {
		return nil, errClientConfigRecovery
	}
	n.poison = true
	h, e := configOpen(n.directory(), "config.json", windows.FILE_OPEN, false, false, false, nil)
	if e != nil {
		return nil, errClientConfigRecovery
	}
	data, e := n.hold(h, true)
	if e != nil {
		return nil, e
	}
	original, e := n.readOriginal()
	if e != nil {
		return nil, errClientConfigRecovery
	}
	body, e := data.read(clientConfigBudget)
	if e != nil || string(body) != string(original) || data.id != n.originalID || data.sd != n.originalSD || sha256.Sum256(body) != n.originalSHA {
		return nil, errClientConfigRecovery
	}
	n.config = data
	if n.finishIntent() != nil {
		return nil, errClientConfigRecovery
	}
	n.originalID = ""
	n.originalSD = ""
	n.originalSHA = [32]byte{}
	return body, nil
}
func (n *nativeClientConfig) retainAux(value object) *object {
	p := &value
	n.auxiliary = append(n.auxiliary, p)
	return p
}
func (n *nativeClientConfig) closeAux(value *object) error {
	if value == nil || value.file == nil {
		return nil
	}
	if value.recheck() != nil {
		n.poison = true
		return errClientConfigRecovery
	}
	if value.file.Close() != nil {
		return n.successorFence()
	}
	value.file = nil
	return nil
}
func (n *nativeClientConfig) close() error {
	if n.check() != nil || n.intent != nil || n.original != nil || n.prepare != nil || n.birth != nil {
		return errClientConfigRecovery
	}
	for _, d := range n.data {
		if d == n.owner || d.file == nil {
			continue
		}
		if d.check() != nil {
			return errClientConfigRecovery
		}
		if d.file.Close() != nil {
			return n.successorFence()
		}
		d.file = nil
	}
	for _, value := range n.auxiliary {
		if n.closeAux(value) != nil {
			return errClientConfigRecovery
		}
	}
	for i := len(n.ancestors) - 1; i >= 0; i-- {
		o := &n.ancestors[i]
		if o.recheck() != nil {
			return errClientConfigRecovery
		}
		if o.file.Close() != nil {
			return n.successorFence()
		}
	}
	if n.owner.check() != nil {
		return errClientConfigRecovery
	}
	if n.owner.file.Close() != nil {
		return n.successorFence()
	}
	n.owner.file = nil
	retainedConfigGuards.Lock()
	for i, held := range retainedConfigGuards.roots {
		if held == n {
			retainedConfigGuards.roots = append(retainedConfigGuards.roots[:i], retainedConfigGuards.roots[i+1:]...)
			break
		}
	}
	retainedConfigGuards.Unlock()
	return nil
}
func acquirePrivateClientConfig(ctx context.Context) (guard *clientConfigGuard, err error) {
	return acquirePrivateClientConfigMode(ctx, false)
}
func acquirePrivateClientConfigMode(ctx context.Context, recovery bool) (guard *clientConfigGuard, err error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errClientConfig
	}
	docker, e := existingCache("docker")
	if e != nil {
		return nil, e
	}
	if _, e = existingCache("credentials"); e != nil {
		return nil, e
	}
	profile, _, e := nativeLocations()
	if e != nil {
		return nil, e
	}
	sid, _, _, e := processFacts(windows.CurrentProcess())
	if e != nil {
		return nil, e
	}
	if len(profile) < 4 || profile[1:3] != `:\` || len(strings.Split(profile[3:], `\`)) > 16 {
		return nil, errClientConfig
	}
	n := &nativeClientConfig{sid: sid, path: filepath.Join(profile, ".orbwin-private", "client", "docker")}
	return retainedConfigGuards.acquire(n, func() (*clientConfigGuard, error) {
		volume, _ := windows.UTF16PtrFromString(profile[:3])
		h, e := windows.CreateFile(volume, directoryShareAccess(windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES), windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if e != nil {
			return nil, errClientConfig
		}
		o, e := capture(h, sid, true, false)
		if o.file != nil {
			n.ancestors = append(n.ancestors, o)
		}
		if e != nil {
			return nil, e
		}
		parts := strings.Split(profile[3:], `\`)
		for i, part := range append(append([]string{}, parts...), ".orbwin-private", "client", "docker") {
			h, e = relative(windows.Handle(n.ancestors[len(n.ancestors)-1].file.Fd()), part, true)
			if e != nil {
				return nil, errClientConfig
			}
			o, e = capture(h, sid, true, i >= len(parts))
			if o.file != nil {
				n.ancestors = append(n.ancestors, o)
			}
			if e != nil {
				return nil, e
			}
		}
		// Persistent fresh admission fence lives next to client, not inside it.
		privateParent := windows.Handle(n.ancestors[len(n.ancestors)-3].file.Fd())
		probe := func(parent windows.Handle, name string) (bool, error) {
			text, _ := windows.NewNTUnicodeString(name)
			oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: text, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
			oa.Length = uint32(unsafe.Sizeof(oa))
			var status windows.IO_STATUS_BLOCK
			var handle windows.Handle
			e := windows.NtCreateFile(&handle, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, &oa, &status, nil, 0, windows.FILE_SHARE_READ, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
			if e == nil {
				_, _ = n.hold(handle, false)
				return true, nil
			}
			if !errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
				return false, errClientConfigRecovery
			}
			return false, nil
		}
		present, e := probe(privateParent, "client.create-intent")
		if e != nil || present {
			return nil, errClientConfigRecovery
		}
		if !recovery && refusePendingPrivateRecords(func(name string) (bool, error) { return probe(n.parent(), name) }) != nil {
			return nil, errClientConfigRecovery
		}
		h, e = configOpen(n.parent(), "controller.lock", windows.FILE_OPEN, true, false, true, nil)
		if e != nil {
			return nil, errClientConfig
		}
		n.owner, e = n.hold(h, false)
		if e != nil {
			return nil, e
		}
		for _, name := range []string{"client.import-intent", "client.import-original", "client.import-source", "client.import-target"} {
			present, e := probe(n.parent(), name)
			if e != nil || present {
				return nil, errClientBootstrapDesktop
			}
		}
		h, e = configOpen(n.parent(), "client.init", windows.FILE_OPEN, false, false, false, nil)
		if e != nil {
			return nil, errClientConfigRecovery
		}
		n.init, e = n.hold(h, false)
		if e != nil {
			return nil, e
		}
		init, e := n.init.read(16 * 1024)
		if e != nil {
			return nil, e
		}
		var root windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(n.parent(), &root) != nil {
			return nil, errClientConfig
		}
		expected := map[string]any{"schema": json.Number("1"), "product": "orbwin", "ownerSid": sid, "rootVolume": json.Number(strconv.FormatUint(uint64(root.VolumeSerialNumber), 10)), "rootHigh": json.Number(strconv.FormatUint(uint64(root.FileIndexHigh), 10)), "rootLow": json.Number(strconv.FormatUint(uint64(root.FileIndexLow), 10)), "admission": "fresh"}
		actual, e := checkedClientJSON(init)
		if e != nil {
			return nil, e
		}
		actualBytes, _ := json.Marshal(actual)
		expectedBytes, _ := json.Marshal(expected)
		if string(actualBytes) != string(expectedBytes) {
			if _, e := parseClientImportInit(init, sid, clientImportID{root.VolumeSerialNumber, root.FileIndexHigh, root.FileIndexLow}, profile); e != nil {
				return nil, errClientConfigRecovery
			}
		}
		h, e = configOpen(n.directory(), "config.json", windows.FILE_OPEN, false, false, false, nil)
		if e != nil {
			return nil, errClientConfig
		}
		n.config, e = n.hold(h, true)
		if e != nil {
			return nil, e
		}
		body, e := n.config.read(clientConfigBudget)
		if e != nil {
			return nil, e
		}
		plugins := filepath.Join(filepath.Dir(docker), "docker-cli-plugins")
		if n.check() != nil {
			return nil, errClientConfig
		}
		if recovery {
			return &clientConfigGuard{native: n, path: n.path, plugins: plugins, body: body}, nil
		}
		if next, changed, e := refreshPrivateManagedField(body, plugins, profile); e != nil {
			return nil, e
		} else if changed {
			if n.replaceManagedConfig(next) != nil {
				return nil, errClientConfigRecovery
			}
			body = next
		}
		return &clientConfigGuard{native: n, path: n.path, plugins: plugins, body: body}, nil
	})
}
