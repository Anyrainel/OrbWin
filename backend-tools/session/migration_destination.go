package main

// Read owning receipts and selected metadata only. No content scan, helper,
// reservation, source-engine connection or write is authorized by this check.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"time"

	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

type migrationDestinationResult struct {
	Version            int      `json:"version"`
	Target             string   `json:"target"`
	Source             string   `json:"source"`
	SourceVolume       string   `json:"sourceVolume"`
	Destination        string   `json:"destination"`
	RestoreTransaction string   `json:"restoreTransaction"`
	ExportTransactions []string `json:"exportTransactions"`
	ManifestSHA256     string   `json:"manifestSha256"`
	CreatedAt          string   `json:"createdAt"`
	ObservedAt         string   `json:"observedAt"`
}

func destinationReceiptMatches(record transferRecord, selected volume.Volume, target, source, sourceVolume string) error {
	fingerprint, err := migrationVolumeFingerprint(selected)
	if err != nil {
		return err
	}
	if record.Truncated || record.Phase != "complete" || record.Daemon != target || record.SourceEngine != source || record.SourceVolume != sourceVolume || record.Volume != selected.Name || !digestValid(record.ManifestSHA256) || !digestValid(record.DestinationFingerprint) || record.DestinationFingerprint != fingerprint || selected.CreatedAt == "" || selected.Driver != "local" || selected.Scope != "local" || len(selected.Options) != 0 || selected.ClusterVolume != nil || selected.Labels["dev.orbwin.transfer"] != record.Transaction || selected.Labels["dev.orbwin.source-sha256"] != record.SourceSHA256 {
		return errors.New("destination does not match a completed restore with recorded source and metadata identity; inspect its receipt and current volume before reuse")
	}
	return nil
}

func readMigrationDestination(ctx context.Context, cli *client.Client, target, source, sourceVolume, destination string) (migrationDestinationResult, error) {
	var result migrationDestinationResult
	if target == "" || source == "" || source == target || !migrationText(source, 256) || resourceFileArguments("volume", sourceVolume) != nil || resourceFileArguments("volume", destination) != nil {
		return result, errors.New("select the reviewed source and an existing owned destination")
	}
	verify := func() error {
		info, err := cli.Info(ctx, client.InfoOptions{})
		if err != nil {
			return err
		}
		if info.Info.ID != target || info.Info.OSType != "linux" {
			return errors.New("destination engine differs from the registered owned Linux daemon")
		}
		return nil
	}
	if err := verify(); err != nil {
		return result, err
	}
	inspected, err := cli.VolumeInspect(ctx, destination, client.VolumeInspectOptions{})
	if err != nil {
		return result, err
	}
	before := inspected.Volume
	if before.Name != destination {
		return result, errors.New("engine returned another destination volume")
	}
	token := before.Labels["dev.orbwin.transfer"]
	if !validTransferToken(token) {
		return result, errors.New("selected volume has no recorded OrbWin restore ownership; review manually prepared data separately")
	}
	directory, err := volumeTransferDirectory()
	if err != nil {
		return result, err
	}
	filename := filepath.Join(directory, token+".jsonl")
	record, err := observeRestoreReceipt(filename)
	if err != nil {
		return result, err
	}
	if err = destinationReceiptMatches(record, before, target, source, sourceVolume); err != nil {
		return result, err
	}
	var output bytes.Buffer
	if err = migrationVolumeHistory(&output, target); err != nil {
		return result, err
	}
	var history struct {
		Records []migrationVolumeHistoryEntry `json:"records"`
	}
	if err = json.Unmarshal(output.Bytes(), &history); err != nil {
		return result, err
	}
	exports := []string{}
	for _, item := range history.Records {
		if item.Uncertain || !item.ExportAcknowledged || item.SourceDaemon != source || item.Volume != sourceVolume || item.ManifestSHA256 != record.ManifestSHA256 || item.PayloadSHA256 != record.SourceSHA256 {
			continue
		}
		for _, restored := range item.Restores {
			if restored.Transaction == record.Transaction && restored.Volume == destination && restored.Phase == "complete" {
				exports = append(exports, item.Transaction)
				break
			}
		}
	}
	if len(exports) == 0 {
		return result, errors.New("no source export acknowledgement matches this restore's full manifest; no origin linkage was accepted")
	}
	afterReceipt, err := observeRestoreReceipt(filename)
	if err != nil {
		return result, err
	}
	if !reflect.DeepEqual(record, afterReceipt) {
		return result, errors.New("restore receipt changed during destination review")
	}
	after, err := cli.VolumeInspect(ctx, destination, client.VolumeInspectOptions{})
	if err != nil {
		return result, err
	}
	if err = destinationReceiptMatches(record, after.Volume, target, source, sourceVolume); err != nil {
		return result, err
	}
	if err = verify(); err != nil {
		return result, err
	}
	return migrationDestinationResult{Version: 1, Target: target, Source: source, SourceVolume: sourceVolume, Destination: destination, RestoreTransaction: record.Transaction, ExportTransactions: exports, ManifestSHA256: record.ManifestSHA256, CreatedAt: before.CreatedAt, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func reviewMigrationDestination(output io.Writer, target, source, sourceVolume, destination string) error {
	cli, closeClient, err := volumeSizeClient(destination)
	if err != nil {
		return err
	}
	defer closeClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := readMigrationDestination(ctx, cli, target, source, sourceVolume, destination)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(result)
}
