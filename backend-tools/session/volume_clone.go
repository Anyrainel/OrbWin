package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type cloneIntent struct {
	Version      int    `json:"version"`
	Phase        string `json:"phase"`
	Transaction  string `json:"transaction"`
	Daemon       string `json:"daemon"`
	SourceVolume string `json:"sourceVolume"`
	Volume       string `json:"volume"`
}
type cloneRecord struct {
	cloneIntent
	SourceSHA256       string `json:"sourceSha256,omitempty"`
	Bytes              int64  `json:"bytes"`
	Entries            int    `json:"entries"`
	RestoreTransaction string `json:"restoreTransaction,omitempty"`
	StagingDiscarded   bool   `json:"stagingDiscarded"`
	Truncated          bool   `json:"truncated"`
	Active             bool   `json:"active"`
	SnapshotPresent    bool   `json:"snapshotPresent"`
	CleanupAvailable   bool   `json:"cleanupAvailable"`
	SnapshotPath       string `json:"snapshotPath,omitempty"`
	Message            string `json:"message,omitempty"`
}
type cloneTransition struct {
	Phase              string `json:"phase"`
	SHA256             string `json:"sha256,omitempty"`
	Bytes              int64  `json:"bytes,omitempty"`
	Entries            int    `json:"entries,omitempty"`
	RestoreTransaction string `json:"restoreTransaction,omitempty"`
}
type cloneJob struct {
	file, lease *os.File
	directory   string
	record      cloneRecord
}

// These acknowledgements include the portable transfer's full dataset identity.
// Payload bytes alone do not bind the source or file metadata to this clone.
type cloneSnapshotAcknowledgement struct {
	Path           string `json:"path"`
	Volume         string `json:"volume"`
	Bytes          int64  `json:"bytes"`
	SHA256         string `json:"sha256"`
	ManifestSHA256 string `json:"manifestSha256"`
	SourceEngine   string `json:"sourceEngine"`
	Entries        int    `json:"entries"`
	Version        int    `json:"version"`
}

type cloneRestoreAcknowledgement struct {
	Complete       bool   `json:"complete"`
	Volume         string `json:"volume"`
	Transaction    string `json:"transaction"`
	Receipt        string `json:"receipt"`
	SourceSHA256   string `json:"sourceSha256"`
	ManifestSHA256 string `json:"manifestSha256"`
	SourceEngine   string `json:"sourceEngine"`
	SourceVolume   string `json:"sourceVolume"`
	Entries        int    `json:"entries"`
	Bytes          int64  `json:"bytes"`
}

func readCloneSnapshotAcknowledgement(data []byte, daemon, source, path string) (cloneSnapshotAcknowledgement, error) {
	var captured cloneSnapshotAcknowledgement
	if len(data) > 16384 {
		return captured, errors.New("clone snapshot acknowledgement exceeds its budget")
	}
	if err := strictCloneJSON(data, &captured); err != nil {
		return captured, err
	}
	if captured.Path != path || captured.Volume != source || captured.Version != 1 || captured.SourceEngine != daemon || !digestValid(captured.SHA256) || !digestValid(captured.ManifestSHA256) || captured.Bytes < 0 || captured.Bytes > volumePayloadLimit || captured.Entries < 1 || captured.Entries > volumeEntryLimit {
		return captured, errors.New("clone snapshot acknowledgement identity differs")
	}
	return captured, nil
}

func readCloneRestoreAcknowledgement(data []byte, captured cloneSnapshotAcknowledgement, target string) (cloneRestoreAcknowledgement, error) {
	var accepted cloneRestoreAcknowledgement
	if len(data) > 16384 {
		return accepted, errors.New("clone restore acknowledgement exceeds its budget")
	}
	if err := strictCloneJSON(data, &accepted); err != nil {
		return accepted, err
	}
	if !accepted.Complete || accepted.Volume != target || !validTransferToken(accepted.Transaction) || accepted.SourceSHA256 != captured.SHA256 || accepted.ManifestSHA256 != captured.ManifestSHA256 || accepted.SourceEngine != captured.SourceEngine || accepted.SourceVolume != captured.Volume || accepted.Entries != captured.Entries || accepted.Bytes != captured.Bytes {
		return accepted, errors.New("clone restore verification acknowledgement differs; review the retained transfer")
	}
	return accepted, nil
}

func cloneDirectory() (string, error) {
	transient, err := transientDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(transient), "volume-clones"), nil
}
func cloneSnapshotPath(directory, token string) string {
	return filepath.Join(directory, token, "snapshot.orbwin-volume")
}
func strictCloneJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("unexpected clone record suffix")
	}
	return nil
}
func applyCloneTransition(record *cloneRecord, next cloneTransition) error {
	if record.StagingDiscarded {
		return errors.New("discarded clone staging cannot transition")
	}
	switch next.Phase {
	case "snapshot":
		if record.Phase != "intent" || !digestValid(next.SHA256) || next.Bytes < 0 || next.Bytes > volumePayloadLimit || next.Entries < 1 || next.Entries > volumeEntryLimit || next.RestoreTransaction != "" {
			return errors.New("invalid clone snapshot transition")
		}
		record.SourceSHA256, record.Bytes, record.Entries = next.SHA256, next.Bytes, next.Entries
	case "restoring":
		if record.Phase != "snapshot" || next.SHA256 != "" || next.Bytes != 0 || next.Entries != 0 || next.RestoreTransaction != "" {
			return errors.New("invalid clone restore transition")
		}
	case "complete":
		if record.Phase != "restoring" || !validTransferToken(next.RestoreTransaction) || next.SHA256 != "" || next.Bytes != 0 || next.Entries != 0 {
			return errors.New("unverified clone completion refused")
		}
		record.RestoreTransaction = next.RestoreTransaction
	case "staging-discarded":
		if next.SHA256 != "" || next.Bytes != 0 || next.Entries != 0 || next.RestoreTransaction != "" {
			return errors.New("invalid clone staging disposition")
		}
		record.StagingDiscarded = true
		return nil
	default:
		return errors.New("unsupported clone transition")
	}
	record.Phase = next.Phase
	return nil
}
func readCloneRecord(path string) (cloneRecord, error) {
	var record cloneRecord
	before, err := os.Lstat(path)
	if err != nil {
		return record, err
	}
	if !before.Mode().IsRegular() || before.Size() > 65536 {
		return record, errors.New("clone receipt must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return record, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		file.Close()
		return record, errors.New("clone receipt changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	err = errors.Join(err, file.Close())
	if err != nil {
		return record, err
	}
	if len(data) > 65536 {
		return record, errors.New("clone receipt exceeds its budget")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return record, errors.New("clone receipt changed while reading")
	}
	lines := bytes.Split(data, []byte{'\n'})
	for index, line := range lines {
		if index == len(lines)-1 {
			record.Truncated = len(line) != 0
			break
		}
		if len(line) == 0 {
			return record, errors.New("empty clone receipt line")
		}
		if index == 0 {
			if err := strictCloneJSON(line, &record.cloneIntent); err != nil {
				return record, err
			}
			if record.Version != 1 || record.Phase != "intent" || !validTransferToken(record.Transaction) || filepath.Base(path) != record.Transaction+".jsonl" || record.Daemon == "" || len(record.Daemon) > 256 || strings.ContainsAny(record.Daemon, "\x00\r\n") || resourceFileArguments("volume", record.SourceVolume) != nil || resourceFileArguments("volume", record.Volume) != nil || record.SourceVolume == record.Volume {
				return record, errors.New("clone receipt identity differs")
			}
		} else {
			var next cloneTransition
			if err := strictCloneJSON(line, &next); err != nil {
				return record, err
			}
			if err := applyCloneTransition(&record, next); err != nil {
				return record, err
			}
		}
	}
	if record.Version != 1 {
		return record, errors.New("clone intent header is incomplete")
	}
	return record, nil
}
func (job *cloneJob) append(next cloneTransition) error {
	copy := job.record
	if err := applyCloneTransition(&copy, next); err != nil {
		return err
	}
	if err := json.NewEncoder(job.file).Encode(next); err != nil {
		return err
	}
	if err := job.file.Sync(); err != nil {
		return err
	}
	job.record = copy
	return nil
}
func (job *cloneJob) close() error { return errors.Join(job.file.Close(), job.lease.Close()) }
func beginClone(daemon, source, target string) (*cloneJob, error) {
	directory, err := cloneDirectory()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	folder, err := os.Open(directory)
	if err != nil {
		return nil, err
	}
	entries, err := folder.ReadDir(16385)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	err = errors.Join(err, folder.Close())
	if err != nil {
		return nil, err
	}
	count := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			count++
		}
	}
	if len(entries) > 16384 || count >= 4096 {
		return nil, errors.New("clone journal retention budget reached; preserve and review existing records before another clone")
	}
	token, err := transferToken()
	if err != nil {
		return nil, err
	}
	lease, err := transientLease(filepath.Join(directory, token+".lock"), true)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, token+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		lease.Close()
		return nil, err
	}
	job := &cloneJob{file: file, lease: lease, directory: directory, record: cloneRecord{cloneIntent: cloneIntent{Version: 1, Phase: "intent", Transaction: token, Daemon: daemon, SourceVolume: source, Volume: target}}}
	if err := json.NewEncoder(file).Encode(job.record.cloneIntent); err != nil {
		job.close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		job.close()
		return nil, err
	}
	if err := os.Mkdir(filepath.Join(directory, token), 0700); err != nil {
		job.close()
		return nil, fmt.Errorf("clone intent retained at %s: %w", file.Name(), err)
	}
	return job, nil
}

// Compose the verified quiesced export and isolated restore, rather than invent
// another data-copy engine. The job never deletes a source or destination volume.
// On process loss, independent restore receipts own pending-target recovery;
// saved packages remain available for explicit restore into a fresh name.
func cloneVolume(output io.Writer, daemon, source, target string) (resultErr error) {
	if resourceFileArguments("volume", source) != nil || resourceFileArguments("volume", target) != nil || source == target {
		return errors.New("select a source volume and a distinct new destination name")
	}
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	_, err = transferVolume(ctx, cli, source)
	if err == nil {
		err = volumeIdle(ctx, cli, source, "")
	}
	if err == nil {
		_, inspectErr := cli.VolumeInspect(ctx, target, client.VolumeInspectOptions{})
		if !cerrdefs.IsNotFound(inspectErr) {
			if inspectErr == nil {
				err = errors.New("clone requires a new volume name; existing data is never overwritten")
			} else {
				err = inspectErr
			}
		}
	}
	err = errors.Join(err, cli.Close())
	cancel()
	if err != nil {
		return err
	}
	job, err := beginClone(daemon, source, target)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, job.close()) }()
	packagePath := cloneSnapshotPath(job.directory, job.record.Transaction)
	fmt.Fprintln(os.Stderr, "Clone captures a full quiesced snapshot; temporary host staging needs roughly twice the payload size. Recovery receipt:", job.file.Name())
	var exported bytes.Buffer
	if err := exportVolume(&exported, daemon, source, packagePath); err != nil {
		return fmt.Errorf("clone snapshot incomplete; source unchanged, staging retained for review: %w", err)
	}
	captured, err := readCloneSnapshotAcknowledgement(exported.Bytes(), daemon, source, packagePath)
	if err != nil {
		return err
	}
	if err := job.append(cloneTransition{Phase: "snapshot", SHA256: captured.SHA256, Bytes: captured.Bytes, Entries: captured.Entries}); err != nil {
		return err
	}
	identity, err := os.Lstat(packagePath)
	if err != nil || !identity.Mode().IsRegular() {
		return errors.Join(errors.New("clone snapshot file identity unavailable"), err)
	}
	if err := job.append(cloneTransition{Phase: "restoring"}); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Snapshot captured. Creating a full, isolated guest copy with content/metadata verification:", target)
	var restored bytes.Buffer
	if err := restoreVolume(&restored, daemon, packagePath, target); err != nil {
		return fmt.Errorf("clone destination not confirmed; saved snapshot %s and independent restore receipt retained: %w", packagePath, err)
	}
	accepted, err := readCloneRestoreAcknowledgement(restored.Bytes(), captured, target)
	if err != nil {
		return err
	}
	if err := job.append(cloneTransition{Phase: "complete", RestoreTransaction: accepted.Transaction}); err != nil {
		return err
	}
	retained := true
	warning := ""
	current, err := os.Lstat(packagePath)
	if err == nil && current.Mode().IsRegular() && os.SameFile(identity, current) && identity.Size() == current.Size() && identity.ModTime().Equal(current.ModTime()) {
		err = os.Remove(packagePath)
		if err == nil {
			retained = false
			err = os.Remove(filepath.Dir(packagePath))
			if err == nil {
				err = job.append(cloneTransition{Phase: "staging-discarded"})
			}
		}
	} else if os.IsNotExist(err) {
		retained = false
	} else if err == nil {
		err = errors.New("snapshot identity changed; retained for review")
	}
	if err != nil {
		warning = "Clone verified, but staging cleanup was not confirmed. Review clone snapshots: " + err.Error()
	}
	return json.NewEncoder(output).Encode(map[string]any{"complete": true, "sourceVolume": source, "volume": target, "transaction": job.record.Transaction, "restoreTransaction": accepted.Transaction, "sourceSha256": captured.SHA256, "bytes": captured.Bytes, "entries": captured.Entries, "snapshotRetained": retained, "warning": warning, "copyOnWrite": false})
}
