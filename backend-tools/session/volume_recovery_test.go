package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreIntentRetainsCompleteManifestProvenanceAndLegacyRecovery(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	manifest, _ := portableFixture(t)
	receipt, err := beginRestore("restored_data", "owned-engine", manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer receipt.lease.Close()
	defer receipt.file.Close()
	record, err := readTransferRecord(receipt.path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := volumeManifestIdentity(manifest)
	if record.ManifestSHA256 != want || record.SourceEngine != manifest.Engine || record.SourceVolume != manifest.Volume || record.SourceSHA256 != manifest.SHA256 || record.Phase != "intent" {
		t.Fatalf("provenance lost: %+v", record)
	}
	data, err := os.ReadFile(receipt.path)
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]any
	if err := json.Unmarshal(data, &header); err != nil {
		t.Fatal(err)
	}
	// Older receipts retain recovery authority without claiming stronger provenance.
	for _, key := range []string{"manifestSha256", "sourceEngine", "sourceVolume"} {
		delete(header, key)
	}
	legacy, _ := json.Marshal(header)
	if err := os.WriteFile(receipt.path, append(legacy, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if legacyRecord, err := readTransferRecord(receipt.path); err != nil || legacyRecord.ManifestSHA256 != "" {
		t.Fatalf("legacy refused or upgraded implicitly: %+v %v", legacyRecord, err)
	}
	for _, partial := range []map[string]any{
		{"manifestSha256": want}, {"sourceEngine": manifest.Engine}, {"sourceVolume": manifest.Volume},
		{"manifestSha256": "bad", "sourceEngine": manifest.Engine, "sourceVolume": manifest.Volume},
		{"manifestSha256": want, "sourceEngine": "bad\nengine", "sourceVolume": manifest.Volume},
	} {
		copy := map[string]any{}
		for key, value := range header {
			copy[key] = value
		}
		for key, value := range partial {
			copy[key] = value
		}
		bytes, _ := json.Marshal(copy)
		if err := os.WriteFile(receipt.path, append(bytes, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readTransferRecord(receipt.path); err == nil {
			t.Fatalf("partial provenance accepted: %v", partial)
		}
	}
}

func TestVolumeReceiptRetainsInterruptedPhaseAndRefusesConflictingAuthority(t *testing.T) {
	token := strings.Repeat("a", 32)
	header, _ := json.Marshal(map[string]any{"version": 1, "phase": "intent", "transaction": token, "volume": "pending_data", "daemon": "owned-engine", "sourceSha256": strings.Repeat("b", 64)})
	filename := filepath.Join(t.TempDir(), token+".jsonl")
	for _, item := range []struct {
		tail      string
		phase     string
		truncated bool
		refuse    bool
	}{
		{"", "intent", false, false},
		{"{\"phase\":\"created\"}\n{\"phase\":\"importing\"}\n", "importing", false, false},
		{"{\"phase\":\"created\"}\n{\"phase\":", "created", true, false},
		{"{\"phase\":\"complete\"}\n", "", false, true},
		{"{\"phase\":\"created\",\"volume\":\"foreign\"}\n", "", false, true},
		{"{\"phase\":\"created\",\"destinationFingerprint\":\"bad\"}\n", "", false, true},
		{"{\"phase\":\"created\"}\n{\"phase\":\"importing\",\"destinationFingerprint\":\"" + strings.Repeat("f", 64) + "\"}\n", "", false, true},
		{"{\"phase\":\"created\"}\n{\"phase\":\"importing\"}\n{\"phase\":\"verifying\"}\n{\"phase\":\"complete\"}\n{\"phase\":\"discarded\"}\n", "", false, true},
	} {
		data := append(append(append([]byte{}, header...), '\n'), []byte(item.tail)...)
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := readTransferRecord(filename)
		if (err != nil) != item.refuse {
			t.Fatalf("wrong receipt acceptance for %s: %+v %v", item.tail, got, err)
		}
		if err == nil && (got.Phase != item.phase || got.Truncated != item.truncated) {
			t.Fatalf("lost interruption state: %+v", got)
		}
	}
	wrong := filepath.Join(filepath.Dir(filename), strings.Repeat("c", 32)+".jsonl")
	_ = os.WriteFile(wrong, append(header, '\n'), 0600)
	if _, err := readTransferRecord(wrong); err == nil {
		t.Fatal("receipt token did not match owned filename")
	}
}
