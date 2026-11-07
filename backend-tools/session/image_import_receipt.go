package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type imageImportStage struct {
	ConfigID  string `json:"configId"`
	Reference string `json:"reference"`
}
type imageImportIntent struct {
	Version            int                `json:"version"`
	Transaction        string             `json:"transaction"`
	Daemon             string             `json:"daemon"`
	CreatedAt          string             `json:"createdAt"`
	Images             []imageImportStage `json:"images"`
	PreviousReferences map[string]string  `json:"previousReferences"`
	imageImportRequest
}
type imageImportEvent struct {
	Phase     string `json:"phase"`
	ConfigID  string `json:"configId,omitempty"`
	ImageID   string `json:"imageId,omitempty"`
	Reference string `json:"reference,omitempty"`
}
type imageImportRecord struct {
	imageImportIntent
	Phase            string            `json:"phase"`
	Verified         map[string]string `json:"verified"`
	Tagged           map[string]string `json:"tagged"`
	Previous         map[string]string `json:"previous"`
	PendingPrevious  string            `json:"pendingPrevious,omitempty"`
	PendingReference string            `json:"pendingReference,omitempty"`
	Truncated        bool              `json:"truncated"`
	Active           bool              `json:"active"`
}
type imageImportJob struct {
	file, lease *os.File
	record      imageImportRecord
}

func imageImportDirectory() (string, error) {
	dir, err := transientDirectory()
	return filepath.Join(filepath.Dir(dir), "image-imports"), err
}
func imageImportStageName(token string, index int) string {
	return fmt.Sprintf("docker.io/orbwin-imports/%s/image-%d:loaded", token, index)
}
func imageImportPreviousName(token string, index int) string {
	return fmt.Sprintf("docker.io/orbwin-imports/%s/previous-%d:retained", token, index)
}
func validateImageImportIntent(intent imageImportIntent) error {
	if intent.Version != 1 || !validTransferToken(intent.Transaction) || intent.Daemon == "" || len(intent.Daemon) > 256 || strings.ContainsAny(intent.Daemon, "\x00\r\n") || !digestValid(intent.SHA256) || len(intent.Source) > 4096 || strings.ContainsAny(intent.Source, "\x00\r\n") || len(intent.Images) < 1 || len(intent.Images) > 128 || len(intent.Tags) > 512 {
		return errors.New("invalid image import receipt header")
	}
	if _, err := time.Parse(time.RFC3339Nano, intent.CreatedAt); err != nil {
		return err
	}
	configs := map[string]bool{}
	for i, image := range intent.Images {
		if !imageIDValid(image.ConfigID) || configs[image.ConfigID] || image.Reference != imageImportStageName(intent.Transaction, i) {
			return errors.New("image import receipt stage identity differs")
		}
		configs[image.ConfigID] = true
	}
	refs := map[string]bool{}
	previousCount := 0
	for i, tag := range intent.Tags {
		ref, err := imageImportReference(tag.Reference)
		if err != nil || ref != tag.Reference || refs[ref] || tag.ExpectedID != "" && !imageIDValid(tag.ExpectedID) || tag.ConfigID != "" && !configs[tag.ConfigID] || tag.Apply && (tag.ConfigID == "" || tag.ExpectedID != "" && !tag.Replace) || !tag.Apply && tag.Replace {
			return errors.New("image import receipt tag identity/consent differs")
		}
		refs[ref] = true
		if tag.Apply && tag.ExpectedID != "" {
			previousCount++
			if intent.PreviousReferences[ref] != imageImportPreviousName(intent.Transaction, i) {
				return errors.New("image import previous-content reference differs")
			}
		}
	}
	if len(intent.PreviousReferences) != previousCount {
		return errors.New("image import previous-content reference set differs")
	}
	return nil
}
func applyImageImportEvent(record *imageImportRecord, event imageImportEvent) error {
	if record.Phase == "complete" || record.Truncated {
		return errors.New("completed or interrupted image import receipt cannot transition")
	}
	plain := event.ConfigID == "" && event.ImageID == "" && event.Reference == ""
	switch event.Phase {
	case "loading":
		if record.Phase != "intent" || !plain {
			return errors.New("invalid image import loading transition")
		}
	case "verified":
		if record.Phase != "loading" && record.Phase != "verified" || !imageIDValid(event.ImageID) || record.Verified[event.ConfigID] != "" || event.Reference != "" {
			return errors.New("invalid staged image verification transition")
		}
		found := false
		for _, image := range record.Images {
			found = found || image.ConfigID == event.ConfigID
		}
		if !found {
			return errors.New("verified config was not in image import intent")
		}
		record.Verified[event.ConfigID] = event.ImageID
	case "tagging":
		if record.Phase != "verified" && record.Phase != "previous-retained" || len(record.Verified) != len(record.Images) || !plain || record.PendingPrevious != "" {
			return errors.New("image reference writes require every staged config verified")
		}
		for _, tag := range record.Tags {
			if tag.Apply && tag.ExpectedID != "" && record.Previous[tag.Reference] != tag.ExpectedID {
				return errors.New("image replacement requires retained previous content")
			}
		}
	case "previous-intent":
		if record.Phase != "verified" && record.Phase != "previous-retained" || len(record.Verified) != len(record.Images) || event.ConfigID != "" || record.PendingPrevious != "" {
			return errors.New("invalid previous-image retention intent")
		}
		found := false
		for _, tag := range record.Tags {
			if tag.Apply && tag.ExpectedID != "" && record.Previous[tag.Reference] == "" {
				found = tag.Reference == event.Reference && tag.ExpectedID == event.ImageID
				break
			}
		}
		if !found {
			return errors.New("previous-image retention is not the next reviewed replacement")
		}
		record.PendingPrevious = event.Reference
	case "previous-retained":
		if record.Phase != "previous-intent" || record.PendingPrevious != event.Reference || event.ConfigID != "" {
			return errors.New("unconfirmed previous image cannot be acknowledged")
		}
		found := false
		for _, tag := range record.Tags {
			found = found || tag.Reference == event.Reference && tag.ExpectedID == event.ImageID && tag.Apply
		}
		if !found {
			return errors.New("previous image acknowledgement identity differs")
		}
		record.Previous[event.Reference] = event.ImageID
		record.PendingPrevious = ""
	case "tag-intent":
		if record.Phase != "tagging" && record.Phase != "tagged" || record.PendingReference != "" || record.Verified[event.ConfigID] != event.ImageID || event.ImageID == "" {
			return errors.New("invalid image import tag intent")
		}
		found := false
		for _, tag := range record.Tags {
			if tag.Apply && record.Tagged[tag.Reference] == "" {
				found = tag.Reference == event.Reference && tag.ConfigID == event.ConfigID
				break
			}
		}
		if !found {
			return errors.New("image import tag intent is not the next reviewed reference")
		}
		record.PendingReference = event.Reference
	case "tagged":
		if record.Phase != "tag-intent" || record.PendingReference != event.Reference || record.Verified[event.ConfigID] != event.ImageID || event.ImageID == "" {
			return errors.New("unconfirmed image reference cannot be acknowledged")
		}
		record.Tagged[event.Reference] = event.ImageID
		record.PendingReference = ""
	case "complete":
		if record.Phase != "tagging" && record.Phase != "tagged" || !plain || record.PendingReference != "" {
			return errors.New("invalid image import completion")
		}
		for _, tag := range record.Tags {
			if tag.Apply && record.Tagged[tag.Reference] == "" {
				return errors.New("image import has unconfirmed references")
			}
		}
	default:
		return errors.New("unsupported image import receipt transition")
	}
	record.Phase = event.Phase
	return nil
}
func (job *imageImportJob) append(event imageImportEvent) error {
	// Maps are mutable: validate a detached copy before a durable append.
	copy := job.record
	copy.Verified = map[string]string{}
	copy.Tagged = map[string]string{}
	copy.Previous = map[string]string{}
	for key, value := range job.record.Verified {
		copy.Verified[key] = value
	}
	for key, value := range job.record.Tagged {
		copy.Tagged[key] = value
	}
	for key, value := range job.record.Previous {
		copy.Previous[key] = value
	}
	if err := applyImageImportEvent(&copy, event); err != nil {
		return err
	}
	if err := json.NewEncoder(job.file).Encode(event); err != nil {
		return err
	}
	if err := job.file.Sync(); err != nil {
		return err
	}
	job.record = copy
	return nil
}
func (job *imageImportJob) close() error { return errors.Join(job.file.Close(), job.lease.Close()) }
func beginImageImport(daemon string, request imageImportRequest, index imageArchiveIndex) (*imageImportJob, error) {
	dir, err := imageImportDirectory()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	folder, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	entries, err := folder.ReadDir(8193)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	err = errors.Join(err, folder.Close())
	if err != nil {
		return nil, err
	}
	if len(entries) > 8192 {
		return nil, errors.New("image import receipt directory exceeds its retention budget")
	}
	count := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") {
			count++
		}
	}
	if count >= 4096 {
		return nil, errors.New("image import receipt retention budget reached; preserve and review prior records")
	}
	token, err := transferToken()
	if err != nil {
		return nil, err
	}
	intent := imageImportIntent{Version: 1, Transaction: token, Daemon: daemon, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), imageImportRequest: request}
	intent.PreviousReferences = map[string]string{}
	for i, tag := range request.Tags {
		if tag.Apply && tag.ExpectedID != "" {
			intent.PreviousReferences[tag.Reference] = imageImportPreviousName(token, i)
		}
	}
	for i, image := range index.Images {
		intent.Images = append(intent.Images, imageImportStage{ConfigID: image.ID, Reference: imageImportStageName(token, i)})
	}
	if err := validateImageImportIntent(intent); err != nil {
		return nil, err
	}
	lease, err := transientLease(filepath.Join(dir, token+".lock"), true)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, token+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		lease.Close()
		return nil, err
	}
	job := &imageImportJob{file: file, lease: lease, record: imageImportRecord{imageImportIntent: intent, Phase: "intent", Verified: map[string]string{}, Tagged: map[string]string{}, Previous: map[string]string{}}}
	err = json.NewEncoder(file).Encode(intent)
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		return nil, errors.Join(err, job.close())
	}
	return job, nil
}
func readImageImportRecord(path string) (imageImportRecord, error) {
	return readImageImportRecordWithBudget(path, nil)
}

var errImageImportReadBudget = errors.New("image import linkage exceeds its shared read budget")

// A history observer supplies a shared byte budget. Charge bytes actually read,
// including malformed/changing files, before parsing. Existing exact-receipt
// callers retain the same per-file limit and validation.
func readImageImportRecordWithBudget(path string, budget *int64) (imageImportRecord, error) {
	record := imageImportRecord{Verified: map[string]string{}, Tagged: map[string]string{}, Previous: map[string]string{}, Phase: "intent"}
	before, err := os.Lstat(path)
	if err != nil {
		return record, err
	}
	if !before.Mode().IsRegular() || before.Size() > 4<<20 {
		return record, errors.New("image import receipt must be a bounded regular file")
	}
	limit := int64(4 << 20)
	if budget != nil && *budget < limit {
		limit = *budget
	}
	if limit <= 0 || before.Size() > limit {
		return record, errImageImportReadBudget
	}
	file, err := os.Open(path)
	if err != nil {
		return record, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		file.Close()
		return record, errors.New("image import receipt changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	sharedExceeded := budget != nil && int64(len(data)) > *budget
	if budget != nil {
		*budget -= int64(len(data))
		if *budget < 0 {
			*budget = 0
		}
	}
	err = errors.Join(err, file.Close())
	if sharedExceeded || int64(len(data)) > limit && limit < 4<<20 {
		return record, errors.Join(err, errImageImportReadBudget)
	}
	if err != nil {
		return record, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || len(data) > 4<<20 {
		return record, errors.New("image import receipt changed or exceeds its budget")
	}
	lines := bytes.Split(data, []byte{'\n'})
	for i, line := range lines {
		if i == len(lines)-1 {
			record.Truncated = len(line) != 0
			break
		}
		if i == 0 {
			if err := strictCloneJSON(line, &record.imageImportIntent); err != nil {
				return record, err
			}
			if err := validateImageImportIntent(record.imageImportIntent); err != nil {
				return record, err
			}
			if filepath.Base(path) != record.Transaction+".jsonl" {
				return record, errors.New("image import receipt filename differs")
			}
		} else {
			var event imageImportEvent
			if err := strictCloneJSON(line, &event); err != nil {
				return record, err
			}
			if err := applyImageImportEvent(&record, event); err != nil {
				return record, err
			}
		}
	}
	if record.Version != 1 {
		return record, errors.New("image import receipt header is incomplete")
	}
	return record, nil
}
func imageImportRecordStatus(daemon, token string) (imageImportRecord, error) {
	var record imageImportRecord
	if !validTransferToken(token) {
		return record, errors.New("select a recorded image import transaction")
	}
	dir, err := imageImportDirectory()
	if err != nil {
		return record, err
	}
	record, err = readImageImportRecord(filepath.Join(dir, token+".jsonl"))
	if err != nil {
		return record, err
	}
	if record.Daemon != daemon {
		return record, errors.New("image import receipt belongs to another engine")
	}
	lease, err := transientLease(filepath.Join(dir, token+".lock"), false)
	if err != nil {
		record.Active = true
	} else if err := lease.Close(); err != nil {
		return record, err
	}
	return record, nil
}
func imageImportRecordOutput(output io.Writer, daemon, token string) error {
	record, err := imageImportRecordStatus(daemon, token)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(record)
}
func imageImportHistory(output io.Writer, daemon string) error {
	dir, err := imageImportDirectory()
	if err != nil {
		return err
	}
	folder, err := os.Open(dir)
	if os.IsNotExist(err) {
		return json.NewEncoder(output).Encode(map[string]any{"records": []any{}})
	}
	if err != nil {
		return err
	}
	entries, err := folder.ReadDir(8193)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	err = errors.Join(err, folder.Close())
	if err != nil {
		return err
	}
	if len(entries) > 8192 {
		return errors.New("image import history exceeds its directory budget")
	}
	var readBytes int64
	records := []map[string]any{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		readBytes += info.Size()
		if readBytes > 64<<20 {
			return errors.New("image import history exceeds its read budget; use the exact recorded transaction to inspect a receipt")
		}
		record, err := imageImportRecordStatus(daemon, strings.TrimSuffix(entry.Name(), ".jsonl"))
		if err != nil {
			return fmt.Errorf("image import receipt %s was retained: %w", entry.Name(), err)
		}
		applied, skipped := 0, 0
		for _, tag := range record.Tags {
			if tag.Apply {
				applied++
			} else {
				skipped++
			}
		}
		records = append(records, map[string]any{"transaction": record.Transaction, "createdAt": record.CreatedAt, "phase": record.Phase, "active": record.Active, "truncated": record.Truncated, "sha256": record.SHA256, "imageCount": len(record.Images), "appliedTags": len(record.Tagged), "requestedTags": applied, "skippedTags": skipped, "pendingReference": record.PendingReference})
		if len(records) > 512 {
			return errors.New("image import history exceeds its response budget; inspect the exact recorded transaction")
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i]["createdAt"].(string) > records[j]["createdAt"].(string) })
	return json.NewEncoder(output).Encode(map[string]any{"records": records})
}
