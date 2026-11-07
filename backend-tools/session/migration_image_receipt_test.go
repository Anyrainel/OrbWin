package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func imageReceiptIntent() migrationImageIntent {
	return migrationImageIntent{Version: 1, Transaction: strings.Repeat("a", 32), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: migrationVolumeTestIntent().Source, Target: "target", Image: "sha256:" + strings.Repeat("b", 64), Reference: "app:reviewed", Destination: `C:\new-image.tar`}
}
func imageReceiptAck() migrationImageAcknowledgement {
	return migrationImageAcknowledgement{Phase: "exported", SHA256: strings.Repeat("c", 64), Bytes: 4096, ConfigIDs: []string{"sha256:" + strings.Repeat("d", 64)}}
}

func TestMigrationImageReceiptValidation(t *testing.T) {
	for _, change := range []func(*migrationImageIntent){
		func(i *migrationImageIntent) { i.Target = i.Source.Daemon },
		func(i *migrationImageIntent) { i.Source.EngineVersion = "unknown" },
		func(i *migrationImageIntent) { i.Destination = "1:/image.tar" },
		func(i *migrationImageIntent) { i.Destination = "C:/image.tar:stream" },
		func(i *migrationImageIntent) { i.Reference = "untagged" },
		func(i *migrationImageIntent) { i.Transaction = "../foreign" },
	} {
		intent := imageReceiptIntent()
		change(&intent)
		if intent.validate() == nil {
			t.Fatalf("invalid intent accepted: %+v", intent)
		}
	}
	for _, change := range []func(*migrationImageAcknowledgement){
		func(a *migrationImageAcknowledgement) { a.ConfigIDs = append(a.ConfigIDs, a.ConfigIDs[0]) },
		func(a *migrationImageAcknowledgement) { a.ConfigIDs = nil },
		func(a *migrationImageAcknowledgement) { a.SHA256 = "bad" },
		func(a *migrationImageAcknowledgement) { a.Bytes = imageArchiveLimit + 1 },
		func(a *migrationImageAcknowledgement) { a.Phase = "imported" },
	} {
		ack := imageReceiptAck()
		change(&ack)
		if ack.validate() == nil {
			t.Fatalf("invalid acknowledgement accepted: %+v", ack)
		}
	}
}
func TestMigrationImageReceiptOrderingAndLostResponse(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("export ownership leases require the Windows SDK client")
	}
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	intent := imageReceiptIntent()
	var receipt string
	export := func(w io.Writer) error {
		dir, _ := migrationImageDirectory()
		files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if len(files) != 1 {
			t.Fatalf("export called without unique intent: %v", files)
		}
		receipt = files[0]
		record, err := observeMigrationImageReceipt(receipt, intent.Target)
		if err != nil || record.Acknowledgement != nil || record.Image != intent.Image {
			t.Fatalf("intent before save: %+v %v", record, err)
		}
		ack := imageReceiptAck()
		return json.NewEncoder(w).Encode(map[string]any{"complete": true, "path": intent.Destination, "image": intent.Image, "reference": intent.Reference, "sha256": ack.SHA256, "bytes": ack.Bytes, "images": []imageArchiveImage{{ID: ack.ConfigIDs[0]}}})
	}
	err := journalMigrationImage(context.Background(), imageReceiptBrokenWriter{}, intent.Source, intent.Target, intent.Image, intent.Reference, intent.Destination, export)
	if err == nil {
		t.Fatal("lost response reported success")
	}
	before, _ := os.ReadFile(receipt)
	record, err := observeMigrationImageReceipt(receipt, intent.Target)
	if err != nil || record.Acknowledgement == nil || record.Acknowledgement.ConfigIDs[0] == intent.Image {
		t.Fatalf("durable config identity missing: %+v %v", record, err)
	}
	after, _ := os.ReadFile(receipt)
	if !bytes.Equal(before, after) {
		t.Fatal("observation changed receipt")
	}
	// The writer also refuses a second successful acknowledgement; readers
	// refuse appended events independently of the in-process state.
	intent.Transaction = strings.Repeat("e", 32)
	job, err := beginMigrationImage(intent)
	if err != nil {
		t.Fatal(err)
	}
	defer job.close()
	if err := job.acknowledge(imageReceiptAck()); err != nil {
		t.Fatal(err)
	}
	if err := job.acknowledge(imageReceiptAck()); err == nil {
		t.Fatal("successful acknowledgement repeated")
	}
}

type imageReceiptBrokenWriter struct{}

func (imageReceiptBrokenWriter) Write([]byte) (int, error) {
	return 0, errors.New("response disconnected")
}

func TestMigrationImageReceiptRefusesTornForeignAndRepeatedEvents(t *testing.T) {
	intent := imageReceiptIntent()
	ack := imageReceiptAck()
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(intent); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(&encoded).Encode(ack); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	name := filepath.Join(t.TempDir(), intent.Transaction+".jsonl")
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	if record, err := observeMigrationImageReceipt(name, intent.Target); err != nil || record.Acknowledgement == nil {
		t.Fatalf("valid receipt refused: %+v %v", record, err)
	}
	if _, err := observeMigrationImageReceipt(name, "foreign"); err == nil {
		t.Fatal("foreign target accepted")
	}
	for _, invalid := range [][]byte{data[:len(data)-1], append(append([]byte{}, data...), []byte("{}\n")...), bytes.Replace(data, []byte(`"phase":"exported"`), []byte(`"phase":"complete"`), 1), bytes.Replace(data, []byte(`"version":1`), []byte(`"version":2`), 1)} {
		if err := os.WriteFile(name, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := observeMigrationImageReceipt(name, intent.Target); err == nil {
			t.Fatalf("invalid receipt accepted: %s", invalid)
		}
	}
	wrong := filepath.Join(filepath.Dir(name), strings.Repeat("f", 32)+".jsonl")
	if err := os.WriteFile(wrong, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := observeMigrationImageReceipt(wrong, intent.Target); err == nil {
		t.Fatal("foreign filename accepted")
	}
	if _, err := observeMigrationImageReceipt(filepath.Dir(name), intent.Target); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestMigrationImageReceiptFailureAndCancellationRemainUnacknowledged(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("export ownership leases require the Windows SDK client")
	}
	for _, mode := range []string{"export", "cancel", "mismatch", "invalid-digest"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
			intent := imageReceiptIntent()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err := journalMigrationImage(ctx, io.Discard, intent.Source, intent.Target, intent.Image, intent.Reference, intent.Destination, func(w io.Writer) error {
				if mode == "export" {
					return errors.New("stream interrupted after publication")
				}
				if mode == "cancel" {
					cancel()
					return nil
				}
				ack := imageReceiptAck()
				path := intent.Destination
				if mode == "mismatch" {
					path = "foreign.tar"
				}
				if mode == "invalid-digest" {
					ack.SHA256 = "bad"
				}
				return json.NewEncoder(w).Encode(map[string]any{"complete": true, "path": path, "image": intent.Image, "reference": intent.Reference, "sha256": ack.SHA256, "bytes": ack.Bytes, "images": []imageArchiveImage{{ID: ack.ConfigIDs[0]}}})
			})
			if err == nil || !strings.Contains(err.Error(), "not acknowledged") {
				t.Fatalf("missing uncertainty: %v", err)
			}
			dir, _ := migrationImageDirectory()
			files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
			if len(files) != 1 {
				t.Fatal(files)
			}
			record, err := observeMigrationImageReceipt(files[0], intent.Target)
			if err != nil || record.Acknowledgement != nil {
				t.Fatalf("false acknowledgement: %+v %v", record, err)
			}
		})
	}
	t.Run("intent-write", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(root, "transient"))
		dir, _ := migrationImageDirectory()
		if err := os.WriteFile(dir, []byte("block"), 0600); err != nil {
			t.Fatal(err)
		}
		intent := imageReceiptIntent()
		called := false
		err := journalMigrationImage(context.Background(), io.Discard, intent.Source, intent.Target, intent.Image, intent.Reference, intent.Destination, func(io.Writer) error { called = true; return nil })
		if err == nil || called {
			t.Fatalf("export ran without durable intent: %v %v", called, err)
		}
	})
	t.Run("ack-write", func(t *testing.T) {
		t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
		job, err := beginMigrationImage(imageReceiptIntent())
		if err != nil {
			t.Fatal(err)
		}
		defer job.lease.Close()
		name := job.file.Name()
		if _, err := beginMigrationImage(imageReceiptIntent()); err == nil {
			t.Fatal("duplicate job accepted")
		}
		job.file.Close()
		if err := job.acknowledge(imageReceiptAck()); err == nil || !job.failed {
			t.Fatal("failed acknowledgement not latched")
		}
		if err := job.acknowledge(imageReceiptAck()); err == nil {
			t.Fatal("failed write retried")
		}
		record, err := observeMigrationImageReceipt(name, "target")
		if err != nil || record.Acknowledgement != nil {
			t.Fatalf("write failure acknowledged: %v", err)
		}
	})
}
