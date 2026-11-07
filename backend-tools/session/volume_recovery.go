package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type transferRecord struct {
	Version                int    `json:"version"`
	Phase                  string `json:"phase"`
	Transaction            string `json:"transaction"`
	Volume                 string `json:"volume"`
	Daemon                 string `json:"daemon"`
	SourceSHA256           string `json:"sourceSha256"`
	ManifestSHA256         string `json:"manifestSha256,omitempty"`
	SourceEngine           string `json:"sourceEngine,omitempty"`
	SourceVolume           string `json:"sourceVolume,omitempty"`
	DestinationFingerprint string `json:"destinationFingerprint,omitempty"`
	Truncated              bool   `json:"truncated,omitempty"`
	Active                 bool   `json:"active"`
	Exists                 bool   `json:"exists"`
	Owned                  bool   `json:"owned"`
	Message                string `json:"message,omitempty"`
}

func volumeTransferDirectory() (string, error) {
	directory, err := transientDirectory()
	return filepath.Join(filepath.Dir(directory), "volume-transfers"), err
}
func validTransferToken(token string) bool {
	raw, err := hex.DecodeString(token)
	return err == nil && len(raw) == 16 && token == strings.ToLower(token)
}
func readTransferRecord(filename string) (transferRecord, error) {
	file, err := os.Open(filename)
	if err != nil {
		return transferRecord{}, err
	}
	defer file.Close()
	return parseTransferRecord(file, filename)
}
func parseTransferRecord(input io.Reader, filename string) (transferRecord, error) {
	var record transferRecord
	data, err := io.ReadAll(io.LimitReader(input, 65537))
	if err != nil {
		return record, err
	}
	if len(data) > 65536 {
		return record, errors.New("volume receipt exceeds 64 KiB")
	}
	lines := bytes.Split(data, []byte{'\n'})
	for index, line := range lines {
		if index == len(lines)-1 {
			if len(line) > 0 {
				record.Truncated = true
			}
			break
		}
		if len(line) == 0 {
			return record, errors.New("empty volume receipt record")
		}
		if index == 0 {
			if err = json.Unmarshal(line, &record); err != nil {
				return record, err
			}
			if record.Version != 1 || record.Phase != "intent" || !validTransferToken(record.Transaction) || filepath.Base(filename) != record.Transaction+".jsonl" || resourceFileArguments("volume", record.Volume) != nil || record.Daemon == "" || !digestValid(record.SourceSHA256) || record.DestinationFingerprint != "" {
				return record, errors.New("invalid volume ownership receipt")
			}
			if record.ManifestSHA256 != "" || record.SourceEngine != "" || record.SourceVolume != "" {
				if !digestValid(record.ManifestSHA256) || record.SourceEngine == "" || !migrationText(record.SourceEngine, 256) || resourceFileArguments("volume", record.SourceVolume) != nil {
					return record, errors.New("invalid volume restore provenance")
				}
			}
			continue
		}
		var next struct {
			Phase                  string `json:"phase"`
			Volume                 string `json:"volume,omitempty"`
			DestinationFingerprint string `json:"destinationFingerprint,omitempty"`
		}
		if err = json.Unmarshal(line, &next); err != nil {
			return record, err
		}
		if next.Volume != "" && next.Volume != record.Volume {
			return record, errors.New("conflicting destination receipt")
		}
		if next.DestinationFingerprint != "" {
			if next.Phase != "created" || !digestValid(next.DestinationFingerprint) {
				return record, errors.New("invalid destination metadata acknowledgement")
			}
			record.DestinationFingerprint = next.DestinationFingerprint
		}
		valid := next.Phase == "discarded" && record.Phase != "complete" && record.Phase != "discarded"
		valid = valid || (record.Phase == "intent" && next.Phase == "created") || (record.Phase == "created" && next.Phase == "importing") || (record.Phase == "importing" && next.Phase == "verifying") || (record.Phase == "verifying" && next.Phase == "complete")
		if !valid {
			return record, errors.New("invalid volume receipt phase transition")
		}
		record.Phase = next.Phase
	}
	if record.Version != 1 {
		return record, errors.New("receipt header was not completed")
	}
	return record, nil
}
func inspectTransferRecord(ctx context.Context, cli *client.Client, record *transferRecord) error {
	inspected, err := cli.VolumeInspect(ctx, record.Volume, client.VolumeInspectOptions{})
	if cerrdefs.IsNotFound(err) {
		record.Message = "Destination is absent. An interrupted create may still be uncertain; the ownership record is retained."
		return nil
	}
	if err != nil {
		return err
	}
	record.Exists = true
	record.Owned = inspected.Volume.Labels["dev.orbwin.transfer"] == record.Transaction && inspected.Volume.Labels["dev.orbwin.source-sha256"] == record.SourceSHA256
	if !record.Owned {
		record.Message = "Name belongs to another or replaced volume; recovery will not remove it."
	}
	return nil
}
func volumeTransfers(output io.Writer, daemon string) error {
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	defer cli.Close()
	defer cancel()
	directory, err := volumeTransferDirectory()
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.jsonl"))
	if err != nil {
		return err
	}
	if len(files) > 1024 {
		return errors.New("more than 1,024 volume receipts require explicit archival before recovery")
	}
	records := []transferRecord{}
	for _, filename := range files {
		record, err := readTransferRecord(filename)
		if err != nil {
			return fmt.Errorf("read volume receipt %s: %w", filepath.Base(filename), err)
		}
		if record.Daemon != daemon {
			continue
		}
		lease, err := transientLease(filepath.Join(directory, record.Transaction+".lock"), false)
		if transientBusy(err) {
			record.Active = true
		} else if err != nil {
			return err
		} else {
			defer lease.Close()
		}
		if err = inspectTransferRecord(ctx, cli, &record); err != nil {
			return err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Volume < records[j].Volume })
	return json.NewEncoder(output).Encode(records)
}
func discardVolumeTransfer(output io.Writer, daemon, token string) error {
	if !validTransferToken(token) {
		return errors.New("select a recorded pending transfer")
	}
	directory, err := volumeTransferDirectory()
	if err != nil {
		return err
	}
	lease, err := transientLease(filepath.Join(directory, token+".lock"), false)
	if err != nil {
		return fmt.Errorf("pending transfer is active or unavailable: %w", err)
	}
	defer lease.Close()
	filename := filepath.Join(directory, token+".jsonl")
	record, err := readTransferRecord(filename)
	if err != nil {
		return err
	}
	if record.Daemon != daemon || record.Phase == "complete" || record.Phase == "discarded" || record.Truncated {
		return errors.New("only an inactive, unambiguous incomplete transfer can be discarded")
	}
	cli, ctx, cancel, err := transferClient(daemon)
	if err != nil {
		return err
	}
	defer cli.Close()
	defer cancel()
	// Reconcile only exact inactive recorded tools before checking attachments.
	toolsDirectory, err := transientDirectory()
	if err != nil {
		return err
	}
	if _, err = recoverTransients(cli, toolsDirectory); err != nil {
		return err
	}
	if err = inspectTransferRecord(ctx, cli, &record); err != nil {
		return err
	}
	if !record.Exists || !record.Owned {
		return errors.New("pending volume is absent or its ownership differs; no deletion attempted")
	}
	before, err := transferVolume(ctx, cli, record.Volume)
	if err != nil {
		return err
	}
	if err = isolatedDestination(ctx, cli, before, ""); err != nil {
		return err
	}
	if before.Labels["dev.orbwin.transfer"] != token || before.Labels["dev.orbwin.source-sha256"] != record.SourceSHA256 {
		return errors.New("pending volume ownership changed; no deletion attempted")
	}
	if _, err = cli.VolumeRemove(ctx, record.Volume, client.VolumeRemoveOptions{Force: false}); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = json.NewEncoder(file).Encode(map[string]string{"phase": "discarded"}); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]string{"discarded": record.Volume, "transaction": token, "message": "Only the recorded inactive unreferenced pending destination was removed. Source package retained."})
}
