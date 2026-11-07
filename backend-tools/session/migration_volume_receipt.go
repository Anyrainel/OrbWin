package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type migrationVolumeIntent struct {
	Version     int                     `json:"version"`
	Token       string                  `json:"token"`
	CreatedAt   string                  `json:"createdAt"`
	Source      migrationSourceIdentity `json:"source"`
	Target      string                  `json:"target"`
	Volume      string                  `json:"volume"`
	Fingerprint string                  `json:"fingerprint"`
	DataRoot    string                  `json:"dataRoot"`
	Mountpoint  string                  `json:"mountpoint"`
	AssetSHA    string                  `json:"assetSha"`
	Destination string                  `json:"destination"`
}

func (intent migrationVolumeIntent) helperName() string {
	return "orbwin-migration-volume-" + intent.Token
}
func (intent migrationVolumeIntent) validate() error {
	_, timeErr := time.Parse(time.RFC3339Nano, intent.CreatedAt)
	if intent.Version != 1 || !validTransferToken(intent.Token) || timeErr != nil || !migrationVolumeSourceProfileValid(intent.Source) || !migrationText(intent.Source.Daemon, 256) || intent.Source.Daemon == "" || !migrationText(intent.Target, 256) || intent.Target == "" || intent.Source.Daemon == intent.Target || !migrationText(intent.Source.Product, 512) || !migrationText(intent.Source.APIVersion, 32) || resourceFileArguments("volume", intent.Volume) != nil || !digestValid(intent.Fingerprint) || !digestValid(intent.AssetSHA) || !migrationText(intent.DataRoot, 4096) || !path.IsAbs(intent.DataRoot) || path.Clean(intent.DataRoot) != intent.DataRoot || intent.DataRoot == "/" || intent.Mountpoint != path.Join(intent.DataRoot, "volumes", intent.Volume, "_data") || !migrationText(intent.Destination, 4096) || intent.Destination == "" {
		return errors.New("invalid source volume maintenance ownership intent")
	}
	return nil
}

type migrationVolumeEvent struct {
	Phase  string          `json:"phase"`
	ID     string          `json:"id,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}
type migrationVolumeRecord struct {
	migrationVolumeIntent
	Phase       string          `json:"phase"`
	ImageID     string          `json:"imageId"`
	ContainerID string          `json:"containerId"`
	Export      json.RawMessage `json:"export,omitempty"`
}

func (record migrationVolumeRecord) advance(event migrationVolumeEvent) (migrationVolumeRecord, error) {
	next := record
	valid := false
	switch event.Phase {
	case "importing":
		valid = record.Phase == "intent"
	case "image":
		valid = record.Phase == "importing" && imageIDValid(event.ID)
		next.ImageID = event.ID
	case "creating":
		valid = record.Phase == "image"
	case "container":
		valid = record.Phase == "creating" && fullContainerID(event.ID)
		next.ContainerID = event.ID
	case "exported":
		valid = record.Phase == "container" && len(event.Result) > 0 && len(event.Result) <= 16384 && json.Valid(event.Result)
		next.Export = append(json.RawMessage(nil), event.Result...)
	case "helper-removed":
		valid = (record.Phase == "container" || record.Phase == "exported") && event.ID == record.ContainerID && record.ContainerID != ""
	case "image-removed":
		valid = (record.Phase == "image" || record.Phase == "helper-removed") && event.ID == record.ImageID && record.ImageID != ""
	case "cleaned":
		valid = record.Phase == "image-removed" || record.Phase == "intent"
	}
	if !valid || (event.Phase != "exported" && len(event.Result) != 0) || ((event.Phase == "importing" || event.Phase == "creating" || event.Phase == "exported" || event.Phase == "cleaned") && event.ID != "") {
		return record, fmt.Errorf("unsupported source volume maintenance transition %s -> %s", record.Phase, event.Phase)
	}
	next.Phase = event.Phase
	return next, nil
}

type migrationVolumeJob struct {
	file, lease *os.File
	record      migrationVolumeRecord
	failed      bool
}

func migrationVolumeDirectory() (string, error) {
	directory, err := transientDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(directory), "migration-volume-jobs"), nil
}
func beginMigrationVolume(intent migrationVolumeIntent) (*migrationVolumeJob, error) {
	if err := intent.validate(); err != nil {
		return nil, err
	}
	dir, err := migrationVolumeDirectory()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lease, err := transientLease(filepath.Join(dir, intent.Token+".lock"), true)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, intent.Token+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		lease.Close()
		return nil, err
	}
	job := &migrationVolumeJob{file: file, lease: lease, record: migrationVolumeRecord{migrationVolumeIntent: intent, Phase: "intent"}}
	if err = json.NewEncoder(file).Encode(intent); err == nil {
		err = file.Sync()
	}
	if err != nil {
		job.close()
		return nil, err
	}
	return job, nil
}
func (job *migrationVolumeJob) close() error { return errors.Join(job.file.Close(), job.lease.Close()) }
func (job *migrationVolumeJob) append(event migrationVolumeEvent) error {
	if job.failed {
		return errors.New("source maintenance receipt write previously failed; retain tools and receipt for review")
	}
	next, err := job.record.advance(event)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(job.file).Encode(event); err == nil {
		err = job.file.Sync()
	}
	if err != nil {
		job.failed = true
		return err
	}
	job.record = next
	return nil
}
func readMigrationVolumeRecord(file *os.File) (migrationVolumeRecord, error) {
	var record migrationVolumeRecord
	info, err := file.Stat()
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return record, errors.New("source maintenance receipt is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return record, err
	}
	if len(data) == 0 || len(data) > 64<<10 || data[len(data)-1] != '\n' {
		return record, errors.New("partial source maintenance receipt; retain it for review")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) > 16 {
		return record, errors.New("source maintenance receipt exceeds phase budget")
	}
	decode := func(data []byte, value any) error {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(value); err != nil {
			return err
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return errors.New("trailing receipt content")
		}
		return nil
	}
	if err := decode(lines[0], &record.migrationVolumeIntent); err != nil {
		return record, err
	}
	if err := record.migrationVolumeIntent.validate(); err != nil {
		return record, err
	}
	if strings.TrimSuffix(filepath.Base(file.Name()), ".jsonl") != record.Token {
		return record, errors.New("source maintenance receipt filename differs")
	}
	record.Phase = "intent"
	for _, line := range lines[1:] {
		var event migrationVolumeEvent
		if err := decode(line, &event); err != nil {
			return record, err
		}
		record, err = record.advance(event)
		if err != nil {
			return record, err
		}
	}
	return record, nil
}
