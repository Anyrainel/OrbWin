//go:build windows

package localnet

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNativeRootAdapterUsesExactDERAndContextOwnershipInMemory(t *testing.T) {
	// Deliberately bypass the system-store factory. This test never opens ROOT.
	handle, err := windows.CertOpenStore(windows.CERT_STORE_PROV_MEMORY, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	store := &windowsRootStore{handle: handle, userID: "memory-store-user", write: true}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	first, err := NewAuthority("00000000-0000-4000-8000-000000000001", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewAuthority("00000000-0000-4000-8000-000000000001", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a, b := first.RootCertificate(), second.RootCertificate()
	if present, err := store.Contains(a); err != nil || present {
		t.Fatal(present, err)
	}
	if err := store.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(a); err == nil {
		t.Fatal("adapter replaced/duplicated an existing root")
	}
	if err := store.Add(b); err != nil {
		t.Fatal(err)
	}
	for range 32 {
		if err := store.Remove(a); err != nil {
			t.Fatal(err)
		}
		if present, err := store.Contains(a); err != nil || present {
			t.Fatal("removed certificate retained", present, err)
		}
		if present, err := store.Contains(b); err != nil || !present {
			t.Fatal("same-subject unrelated certificate removed", present, err)
		}
		if err := store.Add(a); err != nil {
			t.Fatal(err)
		}
	}
	store.write = false
	if err := store.Add(a); err == nil {
		t.Fatal("read-only adapter accepted add")
	}
	if err := store.Remove(a); err == nil {
		t.Fatal("read-only adapter accepted removal")
	}
	// The diagnostic flag reports outstanding native contexts while still closing
	// the store. This checks enumeration/add/delete ownership without touching ROOT.
	closeErr := windows.CertCloseStore(store.handle, windows.CERT_CLOSE_STORE_CHECK_FLAG)
	store.handle = 0
	if closeErr != nil {
		t.Fatal("native certificate context leaked", closeErr)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Contains(a); err == nil {
		t.Fatal("closed adapter accepted a read")
	}
}

func TestMissingReadOnlyRootStoreIsAbsentUntilClosed(t *testing.T) {
	store := &windowsRootStore{userID: "memory-store-user", missing: true}
	if present, err := store.Contains([]byte{1}); err != nil || present {
		t.Fatal(present, err)
	}
	if err := store.Add([]byte{1}); err == nil {
		t.Fatal("missing read-only store accepted a write")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Contains([]byte{1}); err == nil {
		t.Fatal("closed missing-store adapter accepted a read")
	}
}
