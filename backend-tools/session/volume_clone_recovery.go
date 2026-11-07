package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func cloneKnownStagingName(name string) bool {
	const snapshot = "snapshot.orbwin-volume"
	if name == snapshot {
		return true
	}
	if !strings.HasPrefix(name, snapshot+".orbwin-partial-") {
		return false
	}
	token := strings.TrimPrefix(name, snapshot+".orbwin-partial-")
	token = strings.TrimSuffix(token, ".payload")
	return validTransferToken(token)
}

type cloneStagingFile struct {
	path string
	info os.FileInfo
}

// Enumerate and validate before any deletion. Only this recorded job's fixed,
// canonical directory and known regular files are eligible; no recursive remove.
func cloneStagingFiles(directory string, record cloneRecord) ([]cloneStagingFile, error) {
	stage := filepath.Join(directory, record.Transaction)
	info, err := os.Lstat(stage)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(stage)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Clean(stage), canonical) {
		return nil, errors.Join(errors.New("clone staging directory has an alias or unsupported type; retained"), err)
	}
	folder, err := os.Open(stage)
	if err != nil {
		return nil, err
	}
	opened, err := folder.Stat()
	if err != nil || !os.SameFile(info, opened) {
		folder.Close()
		return nil, errors.New("clone staging directory changed while opening")
	}
	entries, err := folder.ReadDir(17)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	err = errors.Join(err, folder.Close())
	if err != nil {
		return nil, err
	}
	if len(entries) > 16 {
		return nil, errors.New("clone staging directory exceeds its entry budget; retained")
	}
	var files []cloneStagingFile
	for _, entry := range entries {
		path := filepath.Join(stage, entry.Name())
		current, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if record.StagingDiscarded || !cloneKnownStagingName(entry.Name()) || !current.Mode().IsRegular() || current.Size() > volumePayloadLimit+volumeManifestLimit+65536 {
			return nil, errors.New("unknown or changed clone staging entry; no deletion attempted")
		}
		files = append(files, cloneStagingFile{path: path, info: current})
	}
	return files, nil
}

func volumeClones(output io.Writer, daemon string) error {
	directory, err := cloneDirectory()
	if err != nil {
		return err
	}
	folder, err := os.Open(directory)
	if os.IsNotExist(err) {
		return json.NewEncoder(output).Encode(map[string]any{"records": []cloneRecord{}})
	}
	if err != nil {
		return err
	}
	entries, err := folder.ReadDir(16385)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	err = errors.Join(err, folder.Close())
	if err != nil {
		return err
	}
	if len(entries) > 16384 {
		return errors.New("clone journal directory exceeds its inspection budget")
	}
	records := []cloneRecord{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		record, err := readCloneRecord(path)
		if err != nil {
			return fmt.Errorf("clone receipt %s could not be inspected; retained: %w", entry.Name(), err)
		}
		if record.Daemon != daemon {
			return errors.New("clone receipt belongs to another engine; retained for review")
		}
		lease, leaseErr := transientLease(filepath.Join(directory, record.Transaction+".lock"), false)
		if leaseErr != nil {
			record.Active = true
			if transientBusy(leaseErr) {
				record.Message = "Clone client still holds its job lease."
			} else {
				record.Message = "Clone inactivity could not be confirmed; staging cannot be discarded."
			}
		} else {
			if err := lease.Close(); err != nil {
				return err
			}
		}
		files, stageErr := cloneStagingFiles(directory, record)
		record.CleanupAvailable = !record.Active && !record.Truncated && stageErr == nil
		if stageErr != nil {
			record.Message = stageErr.Error()
		}
		for _, file := range files {
			if filepath.Base(file.path) == "snapshot.orbwin-volume" {
				record.SnapshotPresent = true
				record.SnapshotPath = file.path
			}
		}
		if record.StagingDiscarded && stageErr == nil {
			continue
		}
		if record.Message == "" {
			switch {
			case record.Truncated:
				record.Message = "Clone receipt was interrupted; preserve staging and inspect before cleanup."
			case record.Phase == "restoring":
				record.Message = "Destination completion is unconfirmed. Review restore receipts and the target before restoring the saved snapshot into a fresh name."
			case record.Phase == "complete":
				record.Message = "Clone was verified. Only leftover local staging needs review."
			case record.SnapshotPresent:
				record.Message = "Saved snapshot is present but not revalidated by this read. Restore validates all bytes before creating a fresh destination."
			default:
				record.Message = "Snapshot capture was not confirmed; source volume was not changed by cloning."
			}
		}
		records = append(records, record)
		if len(records) > 512 {
			return errors.New("retained clone recovery rows exceed their response budget; preserve journals for review")
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Transaction < records[j].Transaction })
	return json.NewEncoder(output).Encode(map[string]any{"records": records})
}

func discardCloneStaging(output io.Writer, daemon, token string) error {
	if !validTransferToken(token) {
		return errors.New("select a recorded clone transaction")
	}
	directory, err := cloneDirectory()
	if err != nil {
		return err
	}
	lease, err := transientLease(filepath.Join(directory, token+".lock"), false)
	if err != nil {
		return fmt.Errorf("clone is active or its inactivity is unknown: %w", err)
	}
	defer lease.Close()
	path := filepath.Join(directory, token+".jsonl")
	record, err := readCloneRecord(path)
	if err != nil {
		return err
	}
	if record.Daemon != daemon || record.Truncated {
		return errors.New("clone receipt owner or complete journal could not be confirmed; staging retained")
	}
	files, err := cloneStagingFiles(directory, record)
	if err != nil {
		return err
	}
	for _, file := range files {
		current, err := os.Lstat(file.path)
		if err != nil || !os.SameFile(current, file.info) || current.Size() != file.info.Size() || !current.ModTime().Equal(file.info.ModTime()) {
			return errors.New("clone staging changed before deletion; remaining files retained")
		}
		if err := os.Remove(file.path); err != nil {
			return err
		}
	}
	stage := filepath.Join(directory, token)
	if err := os.Remove(stage); err != nil && !os.IsNotExist(err) {
		return err
	}
	if !record.StagingDiscarded {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		job := &cloneJob{file: file, record: record}
		err = errors.Join(job.append(cloneTransition{Phase: "staging-discarded"}), file.Close())
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(output).Encode(map[string]any{"transaction": token, "stagingDiscarded": true, "message": "Only the recorded local snapshot/partial files were removed. Source and destination volumes were not changed; public job and restore receipts remain."})
}
