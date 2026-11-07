package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeImageHistoryFixture(t *testing.T, dir string, intent migrationImageIntent, ack *migrationImageAcknowledgement) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	json.NewEncoder(&data).Encode(intent)
	if ack != nil {
		json.NewEncoder(&data).Encode(ack)
	}
	name := filepath.Join(dir, intent.Transaction+".jsonl")
	if err := os.WriteFile(name, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}
func writeImageHistoryImport(t *testing.T, dir, token, sha string, configs []string, complete bool) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	intent := imageImportIntent{Version: 1, Transaction: token, Daemon: "target", CreatedAt: imageReceiptIntent().CreatedAt, imageImportRequest: imageImportRequest{Source: `C:\archive.tar`, SHA256: sha}}
	for i, id := range configs {
		intent.Images = append(intent.Images, imageImportStage{ConfigID: id, Reference: imageImportStageName(token, i)})
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.Encode(intent)
	if complete {
		encoder.Encode(imageImportEvent{Phase: "loading"})
		for _, id := range configs {
			encoder.Encode(imageImportEvent{Phase: "verified", ConfigID: id, ImageID: "sha256:" + strings.Repeat("e", 64)})
		}
		encoder.Encode(imageImportEvent{Phase: "tagging"})
		encoder.Encode(imageImportEvent{Phase: "complete"})
	}
	name := filepath.Join(dir, token+".jsonl")
	if err := os.WriteFile(name, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}
func TestMigrationImageHistoryLinksExactArchiveConfigSet(t *testing.T) {
	source := migrationImageRecord{migrationImageIntent: imageReceiptIntent()}
	ack := imageReceiptAck()
	ack.ConfigIDs = append(ack.ConfigIDs, "sha256:"+strings.Repeat("f", 64))
	source.Acknowledgement = &ack
	match := migrationImportCandidate{Target: "target", SHA256: ack.SHA256, ConfigIDs: []string{ack.ConfigIDs[1], ack.ConfigIDs[0]}, migrationImageImportObservation: migrationImageImportObservation{Transaction: strings.Repeat("b", 32), Phase: "intent"}}
	if got := linkMigrationImageImports(source, []migrationImportCandidate{match}); len(got) != 1 || got[0].Phase != "intent" {
		t.Fatalf("matching unfinished intent not distinguished: %+v", got)
	}
	for _, change := range []func(*migrationImportCandidate){func(c *migrationImportCandidate) { c.Target = "foreign" }, func(c *migrationImportCandidate) { c.SHA256 = strings.Repeat("0", 64) }, func(c *migrationImportCandidate) { c.ConfigIDs = c.ConfigIDs[:1] }, func(c *migrationImportCandidate) { c.ConfigIDs = []string{ack.ConfigIDs[0], ack.ConfigIDs[0]} }, func(c *migrationImportCandidate) { c.ConfigIDs = []string{source.Image, ack.ConfigIDs[0]} }} {
		candidate := match
		change(&candidate)
		if len(linkMigrationImageImports(source, []migrationImportCandidate{candidate})) != 0 {
			t.Fatal("mismatched archive/target/config set linked")
		}
	}
	source.Acknowledgement = nil
	if len(linkMigrationImageImports(source, []migrationImportCandidate{match})) != 0 {
		t.Fatal("intent without export acknowledgement linked")
	}
}
func TestMigrationImageHistoryObservesOwningReceiptsWithoutMutation(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	dir, _ := migrationImageDirectory()
	imports, _ := imageImportDirectory()
	intent := imageReceiptIntent()
	ack := imageReceiptAck()
	name := writeImageHistoryFixture(t, dir, intent, &ack)
	importName := writeImageHistoryImport(t, imports, strings.Repeat("b", 32), ack.SHA256, ack.ConfigIDs, true)
	intent.Transaction = strings.Repeat("c", 32)
	writeImageHistoryFixture(t, dir, intent, nil)
	intent.Transaction = strings.Repeat("d", 32)
	bad := writeImageHistoryFixture(t, dir, intent, &ack)
	if err := os.WriteFile(bad, []byte("torn"), 0600); err != nil {
		t.Fatal(err)
	}
	brokenImport := writeImageHistoryImport(t, imports, strings.Repeat("f", 32), ack.SHA256, ack.ConfigIDs, true)
	file, err := os.OpenFile(brokenImport, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("torn")
	file.Close()
	before, _ := os.ReadFile(name)
	importBefore, _ := os.ReadFile(importName)
	var output bytes.Buffer
	if err := migrationImageHistory(&output, "target"); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Target      string                       `json:"target"`
		Records     []migrationImageHistoryEntry `json:"records"`
		Unavailable int                          `json:"unavailableImportReceipts"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Target != "target" || len(result.Records) != 3 || result.Unavailable != 1 {
		t.Fatalf("missing history/uncertainty: %s", output.Bytes())
	}
	for _, row := range result.Records {
		switch row.Transaction {
		case strings.Repeat("a", 32):
			if !row.ExportAcknowledged || len(row.Imports) != 1 || row.Imports[0].Phase != "complete" || row.Imports[0].VerifiedConfigs != 1 {
				t.Fatalf("valid import not linked: %+v", row)
			}
		case strings.Repeat("c", 32):
			if row.ExportAcknowledged || len(row.Imports) != 0 || row.Uncertain {
				t.Fatalf("intent publication inferred: %+v", row)
			}
		case strings.Repeat("d", 32):
			if !reflect.DeepEqual(row, migrationImageHistoryEntry{Transaction: row.Transaction, Uncertain: true}) {
				t.Fatalf("invalid metadata leaked: %+v", row)
			}
		}
	}
	after, _ := os.ReadFile(name)
	importAfter, _ := os.ReadFile(importName)
	if !bytes.Equal(before, after) || !bytes.Equal(importBefore, importAfter) {
		t.Fatal("history mutated receipts")
	}
}
func TestMigrationImageHistoryBudgets(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	dir, _ := imageImportDirectory()
	ack := imageReceiptAck()
	name := writeImageHistoryImport(t, dir, strings.Repeat("b", 32), ack.SHA256, ack.ConfigIDs, false)
	data, _ := os.ReadFile(name)
	budget := int64(len(data))
	record, err := readImageImportRecordWithBudget(name, &budget)
	if err != nil || budget != 0 || record.Phase != "intent" {
		t.Fatalf("exact budget failed: %d %v", budget, err)
	}
	budget = int64(len(data) - 1)
	if _, err := readImageImportRecordWithBudget(name, &budget); !errors.Is(err, errImageImportReadBudget) {
		t.Fatal("shared budget not enforced")
	}
	if err := os.WriteFile(name, []byte("bad\n"), 0600); err != nil {
		t.Fatal(err)
	}
	budget = 10
	if _, err := readImageImportRecordWithBudget(name, &budget); err == nil || budget != 6 {
		t.Fatalf("malformed read was not charged: %d %v", budget, err)
	}
	for i := 0; i < 257; i++ {
		name := filepath.Join(dir, fmt.Sprintf("%032x.jsonl", i))
		if err := os.WriteFile(name, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	if err := migrationImageHistory(&output, "target"); err == nil || output.Len() != 0 {
		t.Fatalf("receipt count returned partial history: %v", err)
	}
}

func TestEmptyMigrationImageHistoryBindsRequestedTarget(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "not-created", "transient"))
	var output bytes.Buffer
	if err := migrationImageHistory(&output, "reviewed-target"); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Target  string                       `json:"target"`
		Records []migrationImageHistoryEntry `json:"records"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil || response.Target != "reviewed-target" || len(response.Records) != 0 {
		t.Fatal(output.String(), err)
	}
}

func TestMigrationImageHistoryResponseBudgetHasNoPartialOutput(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	dir, _ := migrationImageDirectory()
	imports, _ := imageImportDirectory()
	ack := imageReceiptAck()
	for i := 0; i < 256; i++ {
		intent := imageReceiptIntent()
		intent.Transaction = fmt.Sprintf("%032x", i)
		writeImageHistoryFixture(t, dir, intent, &ack)
		writeImageHistoryImport(t, imports, fmt.Sprintf("%032x", i), ack.SHA256, ack.ConfigIDs, false)
	}
	var output bytes.Buffer
	if err := migrationImageHistory(&output, "target"); err == nil || !strings.Contains(err.Error(), "response budget") || output.Len() != 0 {
		t.Fatalf("oversized link matrix published partial history: %v, %d bytes", err, output.Len())
	}
}
