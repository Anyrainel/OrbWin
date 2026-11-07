//go:build windows

package wsltrust

import (
	"crypto/rand"
	"errors"
	"syscall"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRawRegistrationPresenceUsesPrivateRegistryAndPreservesFilteredOutKeys(t *testing.T) {
	// Only a cryptographically unique, newly created disposable key is written.
	// Never touch HKCU Lxss, certificate stores, registrations or user settings.
	path := `Software\OrbWinWslTrustQualification-` + rand.Text()
	root, existed, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		root.Close()
		t.Fatal("qualification key unexpectedly existed; preserve it")
	}
	id := "00000000-0000-4000-8000-000000000002"
	damaged := "00000000-0000-4000-8000-000000000003"
	t.Cleanup(func() {
		for _, id := range []string{id, damaged} {
			if err := registry.DeleteKey(root, "{"+id+"}"); err != nil && !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
				t.Error(err)
			}
		}
		if err := root.Close(); err != nil {
			t.Error(err)
		}
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error("retain unexpected qualification registry state for review", err)
		}
	})
	if present, err := registrationPresentAt(root, id); err != nil || present {
		t.Fatal("missing private registration misclassified", present, err)
	}
	key, _, err := registry.CreateKey(root, "{"+id+"}", registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := key.SetStringValue("DistributionName", "docker-desktop-data"); err != nil {
		key.Close()
		t.Fatal(err)
	}
	if err := key.SetDWordValue("Version", 1); err != nil {
		key.Close()
		t.Fatal(err)
	}
	if err := key.Close(); err != nil {
		t.Fatal(err)
	}
	if ordinaryRegistration(id, "docker-desktop-data", 1) {
		t.Fatal("protected legacy registration entered ordinary list")
	}
	if present, err := registrationPresentAt(root, id); err != nil || !present {
		t.Fatal("filtered-out registration became retirement permission", present, err)
	}
	key, _, err = registry.CreateKey(root, "{"+damaged+"}", registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if err := key.Close(); err != nil {
		t.Fatal(err)
	}
	if present, err := registrationPresentAt(root, damaged); err != nil || !present {
		t.Fatal("damaged registration metadata became absence", present, err)
	}
	if present, err := registrationPresentAt(registry.Key(0), id); err == nil || present {
		t.Fatal("invalid registry handle became absence", present, err)
	}
}
