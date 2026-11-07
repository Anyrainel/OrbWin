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

type migrationImageHistoryEntry struct {
	Transaction        string                            `json:"transaction"`
	CreatedAt          string                            `json:"createdAt"`
	SourceKind         string                            `json:"sourceKind"`
	SourceDaemon       string                            `json:"sourceDaemon"`
	Image              string                            `json:"image"`
	Reference          string                            `json:"reference"`
	Destination        string                            `json:"destination"`
	Uncertain          bool                              `json:"uncertain"`
	ExportAcknowledged bool                              `json:"exportAcknowledged"`
	SHA256             string                            `json:"sha256,omitempty"`
	ConfigIDs          []string                          `json:"configIds,omitempty"`
	Imports            []migrationImageImportObservation `json:"imports,omitempty"`
}
type migrationImageImportObservation struct {
	Transaction     string `json:"transaction"`
	Phase           string `json:"phase"`
	VerifiedConfigs int    `json:"verifiedConfigs"`
}
type migrationImportCandidate struct {
	Target, SHA256 string
	ConfigIDs      []string
	migrationImageImportObservation
}

func migrationHistoryReceiptNames(directory string) ([]string, error) {
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
		return nil, errors.New("migration history directory exceeds its observation budget")
	}
	var names []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") && validTransferToken(strings.TrimSuffix(entry.Name(), ".jsonl")) {
			names = append(names, entry.Name())
		}
	}
	if len(names) > 256 {
		return nil, errors.New("migration history exceeds 256 receipts; inspect an exact receipt")
	}
	sort.Strings(names)
	return names, nil
}
func observeMigrationImageImports() ([]migrationImportCandidate, int, error) {
	dir, err := imageImportDirectory()
	if err != nil {
		return nil, 0, err
	}
	names, err := migrationHistoryReceiptNames(dir)
	if err != nil {
		return nil, 0, err
	}
	var candidates []migrationImportCandidate
	unavailable := 0
	budget := int64(32 << 20)
	for _, name := range names {
		record, err := readImageImportRecordWithBudget(filepath.Join(dir, name), &budget)
		if errors.Is(err, errImageImportReadBudget) {
			return nil, 0, err
		}
		if err != nil || record.Truncated {
			unavailable++
			continue
		}
		candidate := migrationImportCandidate{Target: record.Daemon, SHA256: record.SHA256, migrationImageImportObservation: migrationImageImportObservation{Transaction: record.Transaction, Phase: record.Phase, VerifiedConfigs: len(record.Verified)}}
		for _, image := range record.Images {
			candidate.ConfigIDs = append(candidate.ConfigIDs, image.ConfigID)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, unavailable, nil
}
func linkMigrationImageImports(source migrationImageRecord, imports []migrationImportCandidate) []migrationImageImportObservation {
	if source.Acknowledgement == nil {
		return nil
	}
	configs := map[string]bool{}
	for _, id := range source.Acknowledgement.ConfigIDs {
		configs[id] = true
	}
	var linked []migrationImageImportObservation
	for _, candidate := range imports {
		if candidate.Target != source.Target || candidate.SHA256 != source.Acknowledgement.SHA256 || len(candidate.ConfigIDs) != len(configs) {
			continue
		}
		seen := map[string]bool{}
		matches := true
		for _, id := range candidate.ConfigIDs {
			if !configs[id] || seen[id] {
				matches = false
				break
			}
			seen[id] = true
		}
		if matches {
			linked = append(linked, candidate.migrationImageImportObservation)
		}
	}
	return linked
}
func migrationImageHistory(output io.Writer, target string) error {
	if target == "" || !migrationText(target, 256) {
		return errors.New("registered target required for image export history")
	}
	dir, err := migrationImageDirectory()
	if err != nil {
		return err
	}
	names, err := migrationHistoryReceiptNames(dir)
	if err != nil {
		return err
	}
	imports, unavailable, err := observeMigrationImageImports()
	if err != nil {
		return err
	}
	records := []migrationImageHistoryEntry{}
	for _, name := range names {
		item := migrationImageHistoryEntry{Transaction: strings.TrimSuffix(name, ".jsonl"), Uncertain: true}
		record, err := observeMigrationImageReceipt(filepath.Join(dir, name), target)
		if err == nil {
			item = migrationImageHistoryEntry{Transaction: record.Transaction, CreatedAt: record.CreatedAt, SourceKind: record.Source.Kind, SourceDaemon: record.Source.Daemon, Image: record.Image, Reference: record.Reference, Destination: record.Destination, ExportAcknowledged: record.Acknowledgement != nil}
			if record.Acknowledgement != nil {
				item.SHA256 = record.Acknowledgement.SHA256
				item.ConfigIDs = record.Acknowledgement.ConfigIDs
				item.Imports = linkMigrationImageImports(record, imports)
			}
		}
		records = append(records, item)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt == records[j].CreatedAt {
			return records[i].Transaction < records[j].Transaction
		}
		return records[i].CreatedAt > records[j].CreatedAt
	})
	data, err := json.Marshal(map[string]any{"target": target, "records": records, "unavailableImportReceipts": unavailable})
	if err != nil {
		return err
	}
	if len(data) > 2<<20 {
		return errors.New("image export history exceeds its response budget")
	}
	_, err = output.Write(append(data, '\n'))
	return err
}
