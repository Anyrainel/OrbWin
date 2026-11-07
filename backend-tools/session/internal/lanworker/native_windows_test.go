//go:build windows

package lanworker

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
)

// Layout/constants only: no process/token/file/socket/pipe/native API calls.
func TestNativeWorkerABIOnly(t *testing.T) {
	if unsafe.Sizeof(nativeFileID{}) != 24 || unsafe.Offsetof(nativeFileID{}.Volume) != 0 || unsafe.Offsetof(nativeFileID{}.ID) != 8 {
		t.Fatal("FILE_ID_INFO differs from reviewed Windows SDK layout")
	}
	if ^windows.SO_REUSEADDR != -5 {
		t.Fatal("SO_EXCLUSIVEADDRUSE SDK expression differs")
	}
	if unsafe.Sizeof(windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}) != 144 || unsafe.Sizeof(windows.ProcessInformation{}) != 24 || unsafe.Sizeof(windows.StartupInfo{}) != 104 {
		t.Fatal("reviewed amd64 process/job ABI differs")
	}
}

func TestNativeWorkerSessionBindingRejectsSameSIDOtherSession(t *testing.T) {
	manager := &heldNativeProcess{session: 1}
	child := &heldNativeProcess{session: 2}
	child.identity = manager.identity
	if sameNativeSession(manager, child) || sameNativeSession(manager, nil) {
		t.Fatal("same SID/request identity bypassed native token session binding")
	}
	child.session = 1
	if !sameNativeSession(manager, child) {
		t.Fatal("same observed session refused")
	}
}

func TestNativeWorkerOpenedChildMustMatchOriginalCreationHandle(t *testing.T) {
	child := &heldNativeProcess{}
	child.identity.CreationTime = "100"
	if !createdHandleMatches(child, windows.Filetime{LowDateTime: 100}) || createdHandleMatches(child, windows.Filetime{LowDateTime: 101}) || createdHandleMatches(nil, windows.Filetime{LowDateTime: 100}) {
		t.Fatal("reused PID/opened handle bypassed original child creation binding")
	}
}

func TestNativeWorkerUDPChannelBindsExactPeerAndProtocol(t *testing.T) {
	scope := lanfirewall.Scope{Protocol: "udp", ClientNetworks: []string{"192.168.4.0/24"}}
	for _, peer := range []string{"192.168.4.20", "192.168.4.20:0", "10.0.0.20:1234", "[::ffff:192.168.4.20]:1234"} {
		if nativeReviewedPeer(scope, peer) {
			t.Fatalf("unreviewed UDP peer %q accepted", peer)
		}
	}
	if !nativeReviewedPeer(scope, "192.168.4.20:1234") {
		t.Fatal("reviewed exact UDP peer refused")
	}
	scope.Protocol = "tcp"
	if nativeReviewedPeer(scope, "192.168.4.20:1234") {
		t.Fatal("UDP channel became TCP byte-stream fallback")
	}
}
