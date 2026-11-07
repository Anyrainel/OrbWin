package localnet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLANUUID36GlobalIdentityUsesSeparatePersistedNativeNamespace32(t *testing.T) {
	global := "d9c00194-1234-4234-8234-123456789abc"
	sid := "S-1-5-21-1-2-3-1001"
	directory := t.TempDir()
	ledger, err := OpenNamespaceLedger(directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	id, err := ledger.LANInstallationID(global, sid)
	if err != nil || !validLANID(id) || id == global {
		t.Fatal("real UUID36 failed independent LAN derivation", id, err)
	}
	if repeated, err := ledger.LANInstallationID(global, sid); err != nil || repeated != id {
		t.Fatal("LAN namespace was not stable", repeated, err)
	}
	runtime := &ServiceRuntime{opts: RuntimeOptions{Installation: global, LAN: &LANAdapters{Installation: id}}, state: RuntimeSnapshot{Installation: global}}
	controller := runtime.newRuntimeLANController(newSharedNamespaces(&namespaceReferenceFake{}, nil, nil))
	if controller.installation != id || runtime.state.Installation != global || runtime.opts.Installation != global {
		t.Fatal("LAN changed global CA/WSL installation truth")
	}
	runtime.generation = &serviceGeneration{lan: controller}
	if err = runtime.bindLANInstallation(id); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "lan-identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ledger.LANInstallationID(global, "S-1-5-21-1-2-3-1002"); err == nil {
		t.Fatal("changed native SID rebound prior LAN identity")
	}
	after, _ := os.ReadFile(filepath.Join(directory, "lan-identity.json"))
	if string(after) != string(data) {
		t.Fatal("mismatched ownership overwrote denial pin")
	}
	if runtime.state.Installation != global {
		t.Fatal("LAN binding rewrote public global installation")
	}
}
func TestLANNamespaceIncludesOwnedHomeDaemonGlobalAndNativeSID(t *testing.T) {
	directory := t.TempDir()
	global := "d9c00194-1234-4234-8234-123456789abc"
	sid := "S-1-5-21-1-2-3-1001"
	base, err := deriveLANBinding(directory, "owned", global, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, parts := range [][4]string{{filepath.Join(directory, "other"), "owned", global, sid}, {directory, "other", global, sid}, {directory, "owned", "d9c00194-1234-4234-8234-123456789abd", sid}, {directory, "owned", global, "S-1-5-21-1-2-3-1002"}} {
		binding, err := deriveLANBinding(parts[0], parts[1], parts[2], parts[3])
		if err != nil || binding.Namespace == base.Namespace {
			t.Fatal("native owner scope collapsed", err)
		}
	}
}
