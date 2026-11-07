package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func TestCloneSnapshotAcknowledgementAcceptsPortableExport(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("portable export's host free-space check requires Windows")
	}
	manifest, payload := portableFixture(t)
	var archive bytes.Buffer
	if err := volumeRestoreArchive(&archive, bytes.NewReader(payload), 0, manifest); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/containers/clone-helper/archive") || r.URL.Query().Get("path") != "/source" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString([]byte(`{"name":"source"}`)))
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	destination := filepath.Join(t.TempDir(), "snapshot.orbwin-volume")
	var output bytes.Buffer
	check := func() error { return nil }
	// Exercise the actual producer, including its full manifest/source fields;
	// only the Docker archive endpoint is replaced with a local fixture.
	if err := exportVolumeContents(context.Background(), cli, &output, manifest.Engine, manifest.Volume, destination, "clone-helper", check, check); err != nil {
		t.Fatal(err)
	}
	captured, err := readCloneSnapshotAcknowledgement(output.Bytes(), manifest.Engine, manifest.Volume, destination)
	if err != nil {
		t.Fatalf("portable export result rejected: %s: %v", output.Bytes(), err)
	}
	file, err := os.Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	saved, _, err := readVolumePackage(file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	wantManifest, err := volumeManifestIdentity(saved)
	if err != nil || captured.ManifestSHA256 != wantManifest || captured.SHA256 != saved.SHA256 {
		t.Fatalf("acknowledgement differs from saved dataset: %+v %s %v", captured, wantManifest, err)
	}
}

func TestCloneAcknowledgementsRequireSamePortableDataset(t *testing.T) {
	captured := cloneSnapshotAcknowledgement{Path: "snapshot.orbwin-volume", Volume: "source_data", Bytes: 17, SHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64), SourceEngine: "owned-engine", Entries: 2, Version: 1}
	encode := func(value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if _, err := readCloneSnapshotAcknowledgement(encode(captured), "owned-engine", "source_data", captured.Path); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*cloneSnapshotAcknowledgement){
		func(ack *cloneSnapshotAcknowledgement) { ack.SourceEngine = "other-engine" },
		func(ack *cloneSnapshotAcknowledgement) { ack.Volume = "other_data" },
		func(ack *cloneSnapshotAcknowledgement) { ack.ManifestSHA256 = "" },
		func(ack *cloneSnapshotAcknowledgement) { ack.SHA256 = "" },
	} {
		wrong := captured
		change(&wrong)
		if _, err := readCloneSnapshotAcknowledgement(encode(wrong), "owned-engine", "source_data", captured.Path); err == nil {
			t.Fatalf("different or incomplete snapshot accepted: %+v", wrong)
		}
	}
	// This is the portable restore response, including all provenance fields
	// that were previously rejected after a successful destination write.
	response := map[string]any{"complete": true, "volume": "cloned_data", "transaction": strings.Repeat("c", 32), "receipt": "restore.jsonl", "sourceSha256": captured.SHA256, "manifestSha256": captured.ManifestSHA256, "sourceEngine": captured.SourceEngine, "sourceVolume": captured.Volume, "entries": captured.Entries, "bytes": captured.Bytes}
	if _, err := readCloneRestoreAcknowledgement(encode(response), captured, "cloned_data"); err != nil {
		t.Fatalf("portable restore fields rejected: %v", err)
	}
	for key, wrong := range map[string]any{"complete": false, "volume": "other_data", "transaction": "", "sourceSha256": strings.Repeat("d", 64), "manifestSha256": strings.Repeat("d", 64), "sourceEngine": "other-engine", "sourceVolume": "other_source", "entries": 3, "bytes": int64(18)} {
		t.Run(key, func(t *testing.T) {
			changed := make(map[string]any, len(response))
			for field, value := range response {
				changed[field] = value
			}
			changed[key] = wrong
			if _, err := readCloneRestoreAcknowledgement(encode(changed), captured, "cloned_data"); err == nil {
				t.Fatalf("different restored dataset accepted: %s", key)
			}
		})
	}
	response["unexpected"] = true
	if _, err := readCloneRestoreAcknowledgement(encode(response), captured, "cloned_data"); err == nil {
		t.Fatal("unknown acknowledgement field accepted")
	}
}

func fixtureCloneJob(t *testing.T) *cloneJob {
	t.Helper()
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transients"))
	job, err := beginClone("owned-engine", "source_data", "cloned_data")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if job.file != nil {
			job.close()
		}
	})
	return job
}
func TestCloneReceiptRequiresVerifiedOrderedSnapshotAndRestoreAcknowledgement(t *testing.T) {
	job := fixtureCloneJob(t)
	if err := job.append(cloneTransition{Phase: "complete", RestoreTransaction: strings.Repeat("c", 32)}); err == nil {
		t.Fatal("unverified clone completed")
	}
	if err := job.append(cloneTransition{Phase: "snapshot", SHA256: strings.Repeat("b", 64), Bytes: 17, Entries: 2}); err != nil {
		t.Fatal(err)
	}
	if err := job.append(cloneTransition{Phase: "restoring"}); err != nil {
		t.Fatal(err)
	}
	if err := job.append(cloneTransition{Phase: "complete", RestoreTransaction: strings.Repeat("c", 32)}); err != nil {
		t.Fatal(err)
	}
	record, err := readCloneRecord(job.file.Name())
	if err != nil || record.Phase != "complete" || record.SourceVolume != "source_data" || record.Volume != "cloned_data" || record.SourceSHA256 != strings.Repeat("b", 64) || record.RestoreTransaction != strings.Repeat("c", 32) {
		t.Fatal(record, err)
	}
	if err := job.append(cloneTransition{Phase: "staging-discarded"}); err != nil {
		t.Fatal(err)
	}
	if err := job.append(cloneTransition{Phase: "restoring"}); err == nil {
		t.Fatal("discarded snapshot resumed")
	}
}
func TestCloneRecoveryPreservesInterruptedAndConflictingRecords(t *testing.T) {
	job := fixtureCloneJob(t)
	path := job.file.Name()
	if err := job.close(); err != nil {
		t.Fatal(err)
	}
	job.file = nil
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(before, []byte(`{"phase":"snapshot","sha256":`)...), 0600); err != nil {
		t.Fatal(err)
	}
	record, err := readCloneRecord(path)
	if err != nil || !record.Truncated || record.Phase != "intent" {
		t.Fatal(record, err)
	}
	if err := discardCloneStaging(&bytes.Buffer{}, "owned-engine", job.record.Transaction); err == nil {
		t.Fatal("interrupted receipt staging deleted")
	}
	if err := os.WriteFile(path, append(before, []byte("{\"phase\":\"complete\",\"restoreTransaction\":\""+strings.Repeat("c", 32)+"\"}\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCloneRecord(path); err == nil {
		t.Fatal("completion without snapshot/restore accepted")
	}
}
func TestCloneStagingDisposalRequiresInactiveLeaseAndKnownFiles(t *testing.T) {
	job := fixtureCloneJob(t)
	path := cloneSnapshotPath(job.directory, job.record.Transaction)
	if err := os.WriteFile(path, []byte("owned temporary snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := discardCloneStaging(&bytes.Buffer{}, "owned-engine", job.record.Transaction); err == nil {
		t.Fatal("active clone staging deleted")
	}
	if err := job.close(); err != nil {
		t.Fatal(err)
	}
	job.file = nil
	foreign := filepath.Join(filepath.Dir(path), "foreign.txt")
	if err := os.WriteFile(foreign, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := discardCloneStaging(&bytes.Buffer{}, "owned-engine", job.record.Transaction); err == nil {
		t.Fatal("unknown staging data deleted")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "owned temporary snapshot" {
		t.Fatal("preflight partially removed data", err)
	}
	if err := os.Remove(foreign); err != nil {
		t.Fatal(err)
	}
	partial := path + ".orbwin-partial-" + strings.Repeat("d", 32) + ".payload"
	if err := os.WriteFile(partial, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := discardCloneStaging(&bytes.Buffer{}, "foreign-engine", job.record.Transaction); err == nil {
		t.Fatal("foreign daemon discarded staging")
	}
	if err := discardCloneStaging(&bytes.Buffer{}, "owned-engine", job.record.Transaction); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("known owned staging remained", err)
	}
	record, err := readCloneRecord(filepath.Join(job.directory, job.record.Transaction+".jsonl"))
	if err != nil || !record.StagingDiscarded || record.Phase != "intent" {
		t.Fatal("public job receipt lost or completion fabricated", record, err)
	}
	var output bytes.Buffer
	if err := volumeClones(&output, "owned-engine"); err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Records []cloneRecord `json:"records"`
	}
	if err := json.Unmarshal(output.Bytes(), &reply); err != nil || len(reply.Records) != 0 {
		t.Fatal("disposed staging retained as an active recovery row", reply, err)
	}
}
