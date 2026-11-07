//go:build windows

package localnet

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSettingsCASPersistsReviewedNonsecretStateAndPreventsLostUpdates(t *testing.T) {
	ledger, err := OpenNamespaceLedger(t.TempDir(), "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	initial, err := ledger.LoadSettings()
	if err != nil || initial.Revision != 0 || initial.Settings.Enabled {
		t.Fatal(initial, err)
	}
	settings := DefaultSettings()
	settings.Ports = ServicePorts{HTTP: 18080, HTTPS: 18443}
	settings.RootSHA256 = strings.Repeat("a", 64)
	settings.Enabled = true
	settings.Preferences = []ServicePreference{{Selector: ServiceSelector{ContainerID: strings.Repeat("b", 64)}, Port: 8080}}
	stored, err := ledger.StoreSettings(0, settings)
	if err != nil || stored.Revision != 1 {
		t.Fatal(stored, err)
	}
	settings.Preferences[0].Port = 9000
	if stored.Settings.Preferences[0].Port != 8080 {
		t.Fatal("stored snapshot aliases caller preferences")
	}
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		workers.Add(1)
		go func() { defer workers.Done(); _, e := ledger.StoreSettings(1, stored.Settings); results <- e }()
	}
	workers.Wait()
	close(results)
	success, refused := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			refused++
		}
	}
	if success != 1 || refused != 1 {
		t.Fatal("concurrent callers overwrote the same revision", success, refused)
	}
	loaded, err := ledger.LoadSettings()
	if err != nil || loaded.Revision != 2 || loaded.Settings.Preferences[0].Port != 8080 {
		t.Fatal(loaded, err)
	}
	// A crash-left draft never replaces the flushed committed revision.
	if err = os.WriteFile(filepath.Join(ledger.directory, ".settings-abandoned.partial"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	ledger.Close()
	reopened, err := OpenNamespaceLedger(ledger.directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if loaded, err = reopened.LoadSettings(); err != nil || loaded.Revision != 2 {
		t.Fatal(loaded, err)
	}
}
func TestSettingsDamagedForeignAndUnreviewedStatesRemainUntouched(t *testing.T) {
	ledger, err := OpenNamespaceLedger(t.TempDir(), "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	settings := DefaultSettings()
	settings.Enabled = true
	if _, err = ledger.StoreSettings(0, settings); err == nil {
		t.Fatal("enabled without reviewed root")
	}
	settings.Enabled = false
	settings.Networks = []string{strings.Repeat("a", 64), strings.Repeat("a", 64)}
	if _, err = ledger.StoreSettings(0, settings); err == nil {
		t.Fatal("duplicate selected networks accepted")
	}
	settings = DefaultSettings()
	if _, err = ledger.StoreSettings(0, settings); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ledger.directory, settingsName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ledger.Close()
	foreign, err := OpenNamespaceLedger(ledger.directory, "foreign-daemon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = foreign.LoadSettings(); err == nil {
		t.Fatal("foreign daemon adopted settings")
	}
	foreign.Close()
	ledger, err = OpenNamespaceLedger(ledger.directory, "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if err = os.WriteFile(path, append(before, []byte("garbage")...), 0600); err != nil {
		t.Fatal(err)
	}
	corrupt, _ := os.ReadFile(path)
	if _, err = ledger.StoreSettings(1, settings); err == nil {
		t.Fatal("corrupt state silently replaced")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(corrupt) {
		t.Fatal("corrupt evidence modified")
	}
}
