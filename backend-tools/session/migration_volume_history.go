package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// A receipt observation is not a lease, current source inspection or proof that
// its destination exists. Recovery separately acquires the owning process lease.
type migrationVolumeHistoryEntry struct {
	Transaction        string                        `json:"transaction"`
	CreatedAt          string                        `json:"createdAt"`
	SourceKind         string                        `json:"sourceKind"`
	SourceDaemon       string                        `json:"sourceDaemon"`
	Volume             string                        `json:"volume"`
	Destination        string                        `json:"destination"`
	Phase              string                        `json:"phase"`
	ExportAcknowledged bool                          `json:"exportAcknowledged"`
	Uncertain          bool                          `json:"uncertain"`
	ManifestSHA256     string                        `json:"manifestSha256,omitempty"`
	PayloadSHA256      string                        `json:"payloadSha256,omitempty"`
	Restores           []migrationRestoreObservation `json:"restores,omitempty"`
}

type migrationRestoreObservation struct {
	Transaction string `json:"transaction"`
	Volume      string `json:"volume"`
	Phase       string `json:"phase"`
}

// Read the owning receipts, never engines or a second execution ledger. Complete
// observations are historical; current volume ownership is a separate read.
func observeMigrationRestores() ([]transferRecord, error) {
	directory, err := volumeTransferDirectory()
	if err != nil {
		return nil, err
	}
	folder, err := os.Open(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
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
		return nil, errors.New("restore history directory exceeds its observation budget")
	}
	var records []transferRecord
	count := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".jsonl") || !validTransferToken(strings.TrimSuffix(entry.Name(), ".jsonl")) {
			continue
		}
		count++
		if count > 256 {
			return nil, errors.New("restore linkage exceeds 256 receipts; use an exact receipt")
		}
		record, readErr := observeRestoreReceipt(filepath.Join(directory, entry.Name()))
		if readErr == nil && !record.Truncated && record.ManifestSHA256 != "" {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Transaction < records[j].Transaction })
	return records, nil
}

func observeRestoreReceipt(filename string) (transferRecord, error) {
	before, err := os.Lstat(filename)
	if err != nil {
		return transferRecord{}, err
	}
	if !before.Mode().IsRegular() || before.Size() > 64<<10 {
		return transferRecord{}, errors.New("restore receipt is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return transferRecord{}, err
	}
	after, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, after) {
		file.Close()
		return transferRecord{}, errors.New("restore receipt changed while opening")
	}
	record, readErr := parseTransferRecord(file, filename)
	return record, errors.Join(readErr, file.Close())
}

func migrationManifestProvenance(record migrationVolumeRecord) (manifest, payload string) {
	var result struct {
		Version                                            int `json:"version"`
		Path, Volume, SourceEngine, ManifestSHA256, SHA256 string
	}
	if json.Unmarshal(record.Export, &result) != nil || result.Version != 1 || result.Path != record.Destination || result.Volume != record.Volume || result.SourceEngine != record.Source.Daemon || !digestValid(result.ManifestSHA256) || !digestValid(result.SHA256) {
		return "", ""
	}
	return result.ManifestSHA256, result.SHA256
}

func linkMigrationRestores(item *migrationVolumeHistoryEntry, source migrationVolumeRecord, restores []transferRecord) {
	item.ManifestSHA256, item.PayloadSHA256 = migrationManifestProvenance(source)
	if item.ManifestSHA256 == "" {
		return
	}
	for _, record := range restores {
		if !record.Truncated && record.Daemon == source.Target && record.SourceEngine == source.Source.Daemon && record.SourceVolume == source.Volume && record.ManifestSHA256 == item.ManifestSHA256 && record.SourceSHA256 == item.PayloadSHA256 {
			item.Restores = append(item.Restores, migrationRestoreObservation{Transaction: record.Transaction, Volume: record.Volume, Phase: record.Phase})
		}
	}
}

func observeMigrationVolumeReceipt(filename, target string) (migrationVolumeRecord, error) {
	var record migrationVolumeRecord
	before, err := os.Lstat(filename)
	if err != nil {
		return record, err
	}
	if !before.Mode().IsRegular() || before.Size() > 64<<10 {
		return record, errors.New("receipt is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return record, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil {
		return record, err
	}
	if !os.SameFile(before, after) {
		return record, errors.New("receipt changed while opening")
	}
	record, err = readMigrationVolumeRecord(file)
	if err != nil {
		return record, err
	}
	if record.Target != target {
		return record, errors.New("receipt belongs to another target")
	}
	return record, nil
}

func migrationVolumeHistory(output io.Writer, target string) error {
	if target == "" || !migrationText(target, 256) {
		return errors.New("registered target required for source history")
	}
	directory, err := migrationVolumeDirectory()
	if err != nil {
		return err
	}
	folder, err := os.Open(directory)
	if os.IsNotExist(err) {
		return json.NewEncoder(output).Encode(map[string]any{"target": target, "records": []migrationVolumeHistoryEntry{}})
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
		return errors.New("source history directory exceeds its budget; use an exact receipt")
	}
	records := []migrationVolumeHistoryEntry{}
	restores, err := observeMigrationRestores()
	if err != nil {
		return err
	}
	var bytes int64
	for _, entry := range entries {
		token := strings.TrimSuffix(entry.Name(), ".jsonl")
		if !strings.HasSuffix(entry.Name(), ".jsonl") || !validTransferToken(token) {
			continue
		}
		if len(records) >= 256 {
			return errors.New("source history exceeds 256 receipts; use an exact receipt")
		}
		// Charge the complete parser budget, including uncertain/changed entries.
		bytes += 64 << 10
		if bytes > 16<<20 {
			return errors.New("source history exceeds its read budget")
		}
		item := migrationVolumeHistoryEntry{Transaction: token, Uncertain: true}
		record, err := observeMigrationVolumeReceipt(filepath.Join(directory, entry.Name()), target)
		if err == nil {
			item = migrationVolumeHistoryEntry{Transaction: token, CreatedAt: record.CreatedAt, SourceKind: record.Source.Kind, SourceDaemon: record.Source.Daemon, Volume: record.Volume, Destination: record.Destination, Phase: record.Phase, ExportAcknowledged: len(record.Export) > 0}
			linkMigrationRestores(&item, record, restores)
		}
		records = append(records, item)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt == records[j].CreatedAt {
			return records[i].Transaction < records[j].Transaction
		}
		return records[i].CreatedAt > records[j].CreatedAt
	})
	data, err := json.Marshal(map[string]any{"target": target, "records": records})
	if err != nil {
		return err
	}
	if len(data) > 2<<20 {
		return errors.New("source history exceeds its response budget")
	}
	_, err = output.Write(append(data, '\n'))
	return err
}
