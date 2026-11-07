//go:build windows && orbwin_recovery_qualification

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

var recoveryQualificationArmed atomic.Uint32

func observeQualificationDeletePending(d *configData) {
	var info struct {
		AllocationSize, EndOfFile int64
		Links                     uint32
		DeletePending, Directory  byte
	}
	if windows.GetFileInformationByHandleEx(windows.Handle(d.file.Fd()), windows.FileStandardInfo, (*byte)(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))) == nil && info.DeletePending != 0 {
		fmt.Println("ACTUAL_STAGE_DELETE_PENDING")
	}
}

// No path/PID/hash/role setter, environment input or IPC. Only a closed one-shot
// event may be armed by a source-compiled fixture in this package.
func armRecoveryQualification(f recoveryQualificationFault) error {
	if f < recoveryHistoricalJournal || f > recoveryLostSnapshotConfigClose || !recoveryQualificationArmed.CompareAndSwap(0, uint32(f)) {
		return errClientConfigRecovery
	}
	return nil
}
func recoveryQualificationLost(n *nativeClientConfig, event recoveryQualificationFault) bool {
	if recoveryQualificationArmed.Load() != uint32(event) {
		return false
	}
	recoveryQualificationParent(n, n.parent())
	if !recoveryQualificationArmed.CompareAndSwap(uint32(event), 255) {
		return false
	}
	fmt.Printf("INJECTED_LOST_RETURN %d\n", event)
	return true
}
func recoveryQualificationParent(n *nativeClientConfig, parent windows.Handle) {
	expected := ".orbwin-private"
	nonce := strings.TrimPrefix(expected, ".orbwin-successor-proof-")
	good := nonce != expected && len(nonce) == 32
	for _, c := range nonce {
		good = good && ((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))
	}
	if _, e := snapshot(parent, true); e != nil {
		good = false
	}
	if _, e := objectPolicy(parent, n.sid, true, true); e != nil {
		good = false
	}
	var buf [1024]uint16
	count, e := windows.GetFinalPathNameByHandle(parent, &buf[0], uint32(len(buf)), 2)
	if e != nil || count == 0 || count >= uint32(len(buf)) {
		good = false
	}
	if good {
		parts := strings.Split(windows.UTF16ToString(buf[:count]), `\`)
		hits := 0
		for i, part := range parts {
			if part == expected {
				hits++
				tail := strings.Join(parts[i+1:], `\`)
				good = good && (tail == "client" || tail == `client\docker`)
			}
		}
		good = good && hits == 1
	}
	if !good {
		fmt.Fprintln(os.Stderr, "SUCCESSOR_CHECKPOINT_NATIVE_NAMESPACE_REFUSED")
		os.Exit(125)
	}
}
func recoveryQualificationExit(f recoveryQualificationFault) {
	if !recoveryQualificationArmed.CompareAndSwap(uint32(f), 255) {
		os.Exit(125)
	}
	if _, e := fmt.Printf("SUCCESSOR_FIXTURE_CUT %d\n", f); e != nil {
		os.Exit(125)
	}
	// Actual process loss, deliberately skipping all defers.
	os.Exit(117)
}
func recoveryQualificationCheckpoint(n *nativeClientConfig, event recoveryQualificationFault) {
	if recoveryQualificationArmed.Load() == uint32(event) {
		recoveryQualificationParent(n, n.parent())
		recoveryQualificationExit(event)
	}
}
func recoveryQualificationBytes(n *nativeClientConfig, old writerRecoveryRecord, normal []byte) ([]byte, error) {
	f := recoveryQualificationFault(recoveryQualificationArmed.Load())
	if f != recoveryHistoricalJournal && f != recoveryHistoricalStagePrepared {
		return normal, nil
	}
	recoveryQualificationParent(n, n.parent())
	// Historical format at the actual record creator; not an observed old
	// binary, external record rewrite or native authority construction.
	return json.Marshal(old)
}
func recoveryQualificationPrefix(r *nativePrivateRecord, body []byte) error {
	event := recoveryQualificationFault(recoveryQualificationArmed.Load())
	count, enabled := recoveryQualificationPrefixCount(event, r.name, body)
	if !enabled {
		return nil
	}
	recoveryQualificationParent(r.n, r.parent)
	if count < 0 || count >= len(body) || r.writable == nil || r.writable.check() != nil {
		return errClientConfigRecovery
	}
	wrote, e := r.writable.file.Write(body[:count])
	if e != nil || wrote != count || r.writable.file.Sync() != nil {
		return errClientConfigRecovery
	}
	actual, e := r.readWritten()
	if e != nil || !bytes.Equal(actual, body[:count]) {
		return errClientConfigRecovery
	}
	recoveryQualificationExit(event)
	return errClientConfigRecovery
}
