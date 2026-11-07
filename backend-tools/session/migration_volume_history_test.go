package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationHistoryLinksOnlyFullSourceManifestAndRestoreReceipts(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	manifest, _ := portableFixture(t)
	intent := migrationVolumeTestIntent()
	manifest.Engine, manifest.Volume = intent.Source.Daemon, intent.Volume
	digest, err := volumeManifestIdentity(manifest)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := json.Marshal(map[string]any{"version": 1, "path": intent.Destination, "volume": intent.Volume, "sourceEngine": intent.Source.Daemon, "manifestSha256": digest, "sha256": manifest.SHA256})
	source := migrationVolumeRecord{migrationVolumeIntent: intent, Export: result}
	restore := transferRecord{Transaction: strings.Repeat("d", 32), Daemon: intent.Target, Volume: "restored", Phase: "complete", SourceEngine: manifest.Engine, SourceVolume: manifest.Volume, ManifestSHA256: digest, SourceSHA256: manifest.SHA256}
	var linked migrationVolumeHistoryEntry
	linkMigrationRestores(&linked, source, []transferRecord{restore})
	if linked.ManifestSHA256 != digest || len(linked.Restores) != 1 {
		t.Fatalf("matching receipt not linked: %+v", linked)
	}
	for _, change := range []func(*transferRecord){
		func(r *transferRecord) { r.Daemon = "foreign" }, func(r *transferRecord) { r.SourceEngine = "foreign" },
		func(r *transferRecord) { r.SourceVolume = "foreign" }, func(r *transferRecord) { r.ManifestSHA256 = strings.Repeat("e", 64) },
		func(r *transferRecord) { r.ManifestSHA256 = "" }, func(r *transferRecord) { r.SourceSHA256 = strings.Repeat("e", 64) },
		func(r *transferRecord) { r.Truncated = true },
	} {
		changed := restore
		change(&changed)
		item := migrationVolumeHistoryEntry{}
		linkMigrationRestores(&item, source, []transferRecord{changed})
		if len(item.Restores) != 0 {
			t.Fatalf("mismatched restore linked: %+v", changed)
		}
	}
	legacy := source
	legacy.Export = []byte(`{"version":1,"sha256":"` + manifest.SHA256 + `"}`)
	linked = migrationVolumeHistoryEntry{}
	linkMigrationRestores(&linked, legacy, []transferRecord{restore})
	if linked.ManifestSHA256 != "" || len(linked.Restores) != 0 {
		t.Fatal("legacy payload upgraded to provenance")
	}
	for key, replacement := range map[string]any{"version": 2, "path": "other-package", "volume": "other-volume", "sourceEngine": "other-engine", "manifestSha256": "bad", "sha256": "bad"} {
		var changed map[string]any
		if err := json.Unmarshal(source.Export, &changed); err != nil {
			t.Fatal(err)
		}
		changed[key] = replacement
		copy := source
		copy.Export, _ = json.Marshal(changed)
		item := migrationVolumeHistoryEntry{}
		linkMigrationRestores(&item, copy, []transferRecord{restore})
		if item.ManifestSHA256 != "" || len(item.Restores) != 0 {
			t.Fatalf("foreign source export accepted: %s", key)
		}
	}
	// Exercise the descriptor-based receipt scanner with a real fsynced intent.
	job, err := beginRestore("restored", intent.Target, manifest)
	if err != nil {
		t.Fatal(err)
	}
	job.file.Close()
	job.lease.Close()
	before, _ := os.ReadFile(job.path)
	observations, err := observeMigrationRestores()
	if err != nil || len(observations) != 1 || observations[0].Phase != "intent" {
		t.Fatalf("intent observation: %+v %v", observations, err)
	}
	after, _ := os.ReadFile(job.path)
	if !bytes.Equal(before, after) {
		t.Fatal("observation modified restore receipt")
	}
	if err := os.WriteFile(job.path, append(before, []byte(`{"phase":`)...), 0600); err != nil {
		t.Fatal(err)
	}
	observations, err = observeMigrationRestores()
	if err != nil || len(observations) != 0 {
		t.Fatalf("torn receipt linked: %+v %v", observations, err)
	}
}

func TestRestoreLinkageBudgetRefusesWithoutPartialHistory(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	sourceDir, _ := migrationVolumeDirectory()
	restoreDir, _ := volumeTransferDirectory()
	for _, dir := range []string{sourceDir, restoreDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 257; i++ {
		if err := os.WriteFile(filepath.Join(restoreDir, fmt.Sprintf("%032x.jsonl", i)), []byte("broken"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := migrationVolumeHistory(&output, "target"); err == nil || output.Len() != 0 {
		t.Fatal("unbounded or partial linkage response")
	}
}

func TestSourceVolumeHistoryPreservesReceiptsAndSeparatesCleanupFromExport(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	directory, _ := migrationVolumeDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	intent := migrationVolumeTestIntent()
	header, _ := json.Marshal(intent)
	filename := filepath.Join(directory, intent.Token+".jsonl")
	// Never-dispatched cleanup is not an acknowledged export.
	data := append(append(header, '\n'), []byte("{\"phase\":\"cleaned\"}\n")...)
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := migrationVolumeHistory(&output, "target"); err != nil {
		t.Fatal(err)
	}
	var history struct {
		Target  string                        `json:"target"`
		Records []migrationVolumeHistoryEntry `json:"records"`
	}
	if err := json.Unmarshal(output.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Target != "target" || len(history.Records) != 1 || history.Records[0].Uncertain || history.Records[0].Phase != "cleaned" || history.Records[0].ExportAcknowledged {
		t.Fatalf("wrong cleanup observation: %s", output.String())
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(data, after) {
		t.Fatal("history modified the receipt")
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 1 {
		t.Fatal("history created host artifacts")
	}
	output.Reset()
	if err := migrationVolumeHistory(&output, "other-target"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(output.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Target != "other-target" || !history.Records[0].Uncertain || history.Records[0].Destination != "" || history.Records[0].SourceDaemon != "" {
		t.Fatalf("foreign metadata exposed: %s", output.String())
	}
}

func TestEmptySourceVolumeHistoryBindsRequestedTargetWithoutCreatingDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	t.Setenv("ORBWIN_TRANSIENT_DIR", root)
	var output bytes.Buffer
	if err := migrationVolumeHistory(&output, "reviewed-target"); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Target  string                        `json:"target"`
		Records []migrationVolumeHistoryEntry `json:"records"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil || response.Target != "reviewed-target" || len(response.Records) != 0 {
		t.Fatal(output.String(), err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("empty history changed receipt storage", err)
	}
}

func TestSourceVolumeHistoryShowsTornAndNonregularReceiptsAsUncertain(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	directory, _ := migrationVolumeDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, strings.Repeat("a", 32)+".jsonl"), []byte("{partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, strings.Repeat("b", 32)+".jsonl"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "untrusted.jsonl"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := migrationVolumeHistory(&output, "target"); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Records []migrationVolumeHistoryEntry `json:"records"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 || !result.Records[0].Uncertain || !result.Records[1].Uncertain {
		t.Fatalf("uncertain receipts lost: %s", output.String())
	}
}

func TestSourceVolumeHistoryIsBoundedWithoutPartialResponse(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	directory, _ := migrationVolumeDirectory()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 257; i++ {
		token := fmt.Sprintf("%032x", i)
		if err := os.WriteFile(filepath.Join(directory, token+".jsonl"), []byte("broken"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := migrationVolumeHistory(&output, "target"); err == nil || output.Len() != 0 {
		t.Fatal("unbounded or partial history accepted")
	}
}
