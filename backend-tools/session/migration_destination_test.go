package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

func TestMigrationDestinationReadOnlyOriginAndReplacementChecks(t *testing.T) {
	for _, mode := range []string{"valid", "wrong-daemon", "changed-daemon", "wrong-source", "wrong-source-volume", "wrong-label", "wrong-name", "legacy", "incomplete", "truncated", "replaced", "missing-export", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
			manifest, _ := portableFixture(t)
			intent := migrationVolumeTestIntent()
			manifest.Engine = intent.Source.Daemon
			manifest.Volume = intent.Volume
			manifestHash, _ := volumeManifestIdentity(manifest)
			restore, err := beginRestore("restored", intent.Target, manifest)
			if err != nil {
				t.Fatal(err)
			}
			selected := volume.Volume{Name: "restored", Driver: "local", Scope: "local", CreatedAt: "2026-10-25T00:00:00Z", Mountpoint: "/volumes/restored", Labels: map[string]string{"dev.orbwin.transfer": restore.token, "dev.orbwin.source-sha256": manifest.SHA256}}
			fingerprint, _ := migrationVolumeFingerprint(selected)
			created := map[string]string{"phase": "created", "volume": "restored", "destinationFingerprint": fingerprint}
			if mode == "legacy" {
				delete(created, "destinationFingerprint")
			}
			if err := restore.append(created); err != nil {
				t.Fatal(err)
			}
			if mode != "incomplete" {
				for _, phase := range []string{"importing", "verifying", "complete"} {
					if err := restore.append(map[string]string{"phase": phase}); err != nil {
						t.Fatal(err)
					}
				}
			}
			restore.file.Close()
			restore.lease.Close()
			if mode == "truncated" {
				f, err := os.OpenFile(restore.path, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				f.WriteString(`{"phase":`)
				f.Close()
			}
			if mode != "missing-export" {
				source, err := beginMigrationVolume(intent)
				if err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(map[string]any{"version": 1, "path": intent.Destination, "volume": intent.Volume, "sourceEngine": intent.Source.Daemon, "manifestSha256": manifestHash, "sha256": manifest.SHA256})
				for _, event := range []migrationVolumeEvent{{Phase: "importing"}, {Phase: "image", ID: "sha256:" + strings.Repeat("d", 64)}, {Phase: "creating"}, {Phase: "container", ID: strings.Repeat("e", 64)}, {Phase: "exported", Result: data}} {
					if err := source.append(event); err != nil {
						t.Fatal(err)
					}
				}
				source.close()
			}
			infos, inspections := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("mutation attempted: %s", r.Method)
					http.Error(w, "mutation", 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					infos++
					daemon := intent.Target
					if mode == "wrong-daemon" || mode == "changed-daemon" && infos > 1 {
						daemon = "foreign"
					}
					json.NewEncoder(w).Encode(map[string]any{"ID": daemon, "OSType": "linux"})
				case strings.HasSuffix(r.URL.Path, "/volumes/restored"):
					inspections++
					value := selected
					if mode == "wrong-name" {
						value.Name = "foreign"
					}
					if mode == "replaced" && inspections > 1 {
						value.CreatedAt = "2026-10-25T00:00:01Z"
					}
					if mode == "wrong-label" {
						value.Labels = map[string]string{"dev.orbwin.transfer": restore.token, "dev.orbwin.source-sha256": strings.Repeat("f", 64)}
					}
					json.NewEncoder(w).Encode(value)
				default:
					t.Errorf("unexpected request (no data scan allowed): %s", r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithHTTPClient(&http.Client{Transport: volumeSizeTransport{http.DefaultTransport, "restored"}}), client.WithAPIVersion("1.56"))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			source, sourceVolume := manifest.Engine, manifest.Volume
			if mode == "wrong-source" {
				source = "other"
			}
			if mode == "wrong-source-volume" {
				sourceVolume = "other"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			result, err := readMigrationDestination(ctx, cli, intent.Target, source, sourceVolume, "restored")
			if mode == "valid" {
				if err != nil || result.RestoreTransaction != restore.token || result.ManifestSHA256 != manifestHash || len(result.ExportTransactions) != 1 || result.ExportTransactions[0] != intent.Token || result.ObservedAt == "" {
					t.Fatalf("valid destination refused: %+v %v", result, err)
				}
			} else if err == nil {
				t.Fatalf("unsafe destination accepted: %s %+v", mode, result)
			}
		})
	}
}
