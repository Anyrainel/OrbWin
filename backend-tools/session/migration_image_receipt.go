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
	"time"
)

type migrationImageIntent struct {
	Version     int                     `json:"version"`
	Transaction string                  `json:"transaction"`
	CreatedAt   string                  `json:"createdAt"`
	Source      migrationSourceIdentity `json:"source"`
	Target      string                  `json:"target"`
	Image       string                  `json:"image"`
	Reference   string                  `json:"reference"`
	Destination string                  `json:"destination"`
}
type migrationImageAcknowledgement struct {
	Phase     string   `json:"phase"`
	SHA256    string   `json:"sha256"`
	Bytes     int64    `json:"bytes"`
	ConfigIDs []string `json:"configIds"`
}
type migrationImageRecord struct {
	migrationImageIntent
	Acknowledgement *migrationImageAcknowledgement `json:"acknowledgement,omitempty"`
}

func (intent migrationImageIntent) validate() error {
	_, err := time.Parse(time.RFC3339Nano, intent.CreatedAt)
	path := intent.Destination
	drivePath := len(path) >= 3 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' && strings.ContainsAny(path[2:3], "/\\")
	if intent.Version != 1 || !validTransferToken(intent.Transaction) || err != nil || !migrationVolumeSourceProfileValid(intent.Source) || intent.Source.Daemon == "" || !migrationText(intent.Source.Daemon, 256) || !migrationText(intent.Source.Product, 512) || !migrationText(intent.Source.APIVersion, 32) || intent.Target == "" || !migrationText(intent.Target, 256) || intent.Target == intent.Source.Daemon || imageExportReference(intent.Image, intent.Reference) != nil || !migrationText(path, 4096) || !drivePath || !strings.EqualFold(filepath.Ext(path), ".tar") || strings.Contains(path[2:], ":") {
		return errors.New("invalid source image export intent")
	}
	return nil
}
func (ack migrationImageAcknowledgement) validate() error {
	if ack.Phase != "exported" || !digestValid(ack.SHA256) || ack.Bytes <= 0 || ack.Bytes > imageArchiveLimit || len(ack.ConfigIDs) < 1 || len(ack.ConfigIDs) > 128 {
		return errors.New("invalid source image export acknowledgement")
	}
	seen := map[string]bool{}
	for _, id := range ack.ConfigIDs {
		if !imageIDValid(id) || seen[id] {
			return errors.New("invalid or repeated exported config identity")
		}
		seen[id] = true
	}
	return nil
}
func migrationImageDirectory() (string, error) {
	dir, err := transientDirectory()
	return filepath.Join(filepath.Dir(dir), "migration-image-exports"), err
}

type migrationImageJob struct {
	file, lease *os.File
	record      migrationImageRecord
	failed      bool
}

func beginMigrationImage(intent migrationImageIntent) (*migrationImageJob, error) {
	if err := intent.validate(); err != nil {
		return nil, err
	}
	dir, err := migrationImageDirectory()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	// Two files per retained job. Refuse new work when the bounded directory
	// reaches retention; never erase records to make room for another export.
	folder, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	entries, readErr := folder.ReadDir(8193)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	err = errors.Join(readErr, folder.Close())
	if err != nil {
		return nil, err
	}
	if len(entries) >= 8192 {
		return nil, errors.New("image export receipt retention budget reached; preserve and review records")
	}
	lease, err := transientLease(filepath.Join(dir, intent.Transaction+".lock"), true)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, intent.Transaction+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		lease.Close()
		return nil, err
	}
	job := &migrationImageJob{file: file, lease: lease, record: migrationImageRecord{migrationImageIntent: intent}}
	if err = json.NewEncoder(file).Encode(intent); err == nil {
		err = file.Sync()
	}
	if err != nil {
		return nil, errors.Join(fmt.Errorf("image export intent %s was not acknowledged: %w", file.Name(), err), job.close())
	}
	return job, nil
}
func (job *migrationImageJob) close() error { return errors.Join(job.file.Close(), job.lease.Close()) }
func (job *migrationImageJob) acknowledge(ack migrationImageAcknowledgement) error {
	if job.failed || job.record.Acknowledgement != nil {
		return errors.New("image export acknowledgement cannot be repeated after success or a failed write")
	}
	if err := ack.validate(); err != nil {
		return err
	}
	err := json.NewEncoder(job.file).Encode(ack)
	if err == nil {
		err = job.file.Sync()
	}
	if err != nil {
		job.failed = true
		return fmt.Errorf("archive may remain but export acknowledgement failed at %s: %w", job.file.Name(), err)
	}
	copy := ack
	copy.ConfigIDs = append([]string(nil), ack.ConfigIDs...)
	job.record.Acknowledgement = &copy
	return nil
}
func observeMigrationImageReceipt(filename, target string) (migrationImageRecord, error) {
	var record migrationImageRecord
	before, err := os.Lstat(filename)
	if err != nil {
		return record, err
	}
	if !before.Mode().IsRegular() || before.Size() > 64<<10 {
		return record, errors.New("source image receipt is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return record, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, opened) {
		file.Close()
		return record, errors.New("source image receipt changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	err = errors.Join(err, file.Close())
	if err != nil {
		return record, err
	}
	after, err := os.Lstat(filename)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || len(data) > 64<<10 {
		return record, errors.New("source image receipt changed or exceeded its budget")
	}
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) < 2 || len(lines) > 3 || len(lines[len(lines)-1]) != 0 {
		return record, errors.New("source image receipt is incomplete or has unexpected events")
	}
	if err = strictCloneJSON(lines[0], &record.migrationImageIntent); err != nil {
		return record, err
	}
	if err = record.migrationImageIntent.validate(); err != nil {
		return record, err
	}
	if filepath.Base(filename) != record.Transaction+".jsonl" || record.Target != target {
		return record, errors.New("source image receipt identity/target differs")
	}
	if len(lines) == 3 {
		var ack migrationImageAcknowledgement
		if err = strictCloneJSON(lines[1], &ack); err != nil {
			return record, err
		}
		if err = ack.validate(); err != nil {
			return record, err
		}
		record.Acknowledgement = &ack
	}
	return record, nil
}
