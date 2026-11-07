package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func migrationVolumeTestIntent() migrationVolumeIntent {
	return migrationVolumeIntent{Version: 1, Token: strings.Repeat("a", 32), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: migrationSourceIdentity{Kind: "docker-desktop", Daemon: "source", EngineVersion: "29.8.0", APIVersion: "1.56", Product: "Docker Desktop"}, Target: "target", Volume: "selected", Fingerprint: strings.Repeat("b", 64), DataRoot: "/var/lib/docker", Mountpoint: "/var/lib/docker/volumes/selected/_data", AssetSHA: strings.Repeat("c", 64), Destination: "C:\\new-package.orbvolume"}
}

func TestMigrationVolumeReceiptBindsSourceProfileAndWSL(t *testing.T) {
	valid := migrationVolumeTestIntent()
	valid.Source = migrationSourceIdentity{Kind: "rancher-desktop", Daemon: "source", EngineVersion: "29.5.3", APIVersion: "1.54", WSL: &migrationWSLIdentity{Registration: "231810b0-fa21-48a8-b0a7-86f9a1360029", Name: "Ubuntu", Machine: strings.Repeat("1", 32)}}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*migrationVolumeIntent){
		func(v *migrationVolumeIntent) { v.Source.Kind = "other" },
		func(v *migrationVolumeIntent) { v.Source.EngineVersion = "29.8.0" },
		func(v *migrationVolumeIntent) { v.Source.WSL = nil },
		func(v *migrationVolumeIntent) { v.Source.WSL.Machine = "" },
		func(v *migrationVolumeIntent) { v.Source.WSL.Registration = "Ubuntu" },
		func(v *migrationVolumeIntent) { v.Source.WSL.Name = "" },
		func(v *migrationVolumeIntent) { v.Source.WSL.Name = "rancher-desktop" },
		func(v *migrationVolumeIntent) { v.Source.WSL.Name = "lima-orbwin" },
		func(v *migrationVolumeIntent) { v.Source.Kind = "docker-desktop"; v.Source.EngineVersion = "29.8.0" },
	} {
		copy := valid
		wsl := *valid.Source.WSL
		copy.Source.WSL = &wsl
		change(&copy)
		if copy.validate() == nil {
			t.Fatalf("foreign/incomplete source receipt accepted: %+v", copy.Source)
		}
	}
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	job, err := beginMigrationVolume(valid)
	if err != nil {
		t.Fatal(err)
	}
	job.close()
	reopened, err := openMigrationVolumeJob(valid.Target, valid.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.close()
	if reopened.record.Source.WSL == nil || *reopened.record.Source.WSL != *valid.Source.WSL {
		t.Fatal("source connection tuple did not survive receipt")
	}
}
func TestMigrationVolumeReceiptOrderingLeaseAndTruncation(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	intent := migrationVolumeTestIntent()
	job, err := beginMigrationVolume(intent)
	if err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("d", 64)
	container := strings.Repeat("e", 64)
	if _, err := openMigrationVolumeJob("target", intent.Token); err == nil {
		t.Fatal("active lease was adopted")
	}
	if err := job.append(migrationVolumeEvent{Phase: "container", ID: container}); err == nil {
		t.Fatal("container acknowledged before image intent")
	}
	for _, event := range []migrationVolumeEvent{{Phase: "importing"}, {Phase: "image", ID: image}, {Phase: "creating"}, {Phase: "container", ID: container}, {Phase: "exported", Result: json.RawMessage(`{"path":"C:\\new-package.orbvolume"}`)}, {Phase: "helper-removed", ID: container}, {Phase: "image-removed", ID: image}, {Phase: "cleaned"}} {
		if err := job.append(event); err != nil {
			t.Fatal(err)
		}
	}
	filename := job.file.Name()
	if err := job.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := openMigrationVolumeJob("foreign", intent.Token); err == nil {
		t.Fatal("foreign target adopted")
	}
	reopened, err := openMigrationVolumeJob("target", intent.Token)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.record.Phase != "cleaned" || reopened.record.ImageID != image || reopened.record.ContainerID != container || len(reopened.record.Export) == 0 {
		t.Fatal("settled receipt differs")
	}
	reopened.close()
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString(`{"phase":`)
	file.Close()
	if _, err := openMigrationVolumeJob("target", intent.Token); err == nil {
		t.Fatal("partial receipt was replayed")
	}
}
func TestMigrationVolumeScopeDeniesSourceWorkloadWrites(t *testing.T) {
	intent := migrationVolumeTestIntent()
	helper := strings.Repeat("d", 64)
	exec := strings.Repeat("e", 64)
	image := "sha256:" + strings.Repeat("f", 64)
	scope := &migrationVolumeScope{token: intent.Token, asset: intent.AssetSHA, helper: intent.helperName(), image: image}
	scope.acceptContainer(helper)
	scope.acceptExec(exec)
	for _, test := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/v1.56/volumes/selected", true}, {"GET", "/v1.56/containers/" + helper + "/archive", true}, {"POST", "/v1.56/containers/" + helper + "/start", true}, {"POST", "/v1.56/exec/" + exec + "/start", true}, {"DELETE", "/v1.56/containers/" + helper + "?force=1", true}, {"DELETE", "/v1.56/images/" + image + "?noprune=1", true},
		{"DELETE", "/v1.56/volumes/selected", false}, {"POST", "/v1.56/images/load", false}, {"POST", "/v1.56/images/" + image + "/tag", false}, {"POST", "/v1.56/containers/" + strings.Repeat("b", 64) + "/start", false}, {"DELETE", "/v1.56/containers/" + helper + "?force=1&v=1", false}, {"DELETE", "/v1.56/images/" + image + "?force=1&noprune=1", false}, {"POST", "/v1.56/containers/create?name=foreign", false}, {"POST", "/v1.56/exec/" + strings.Repeat("b", 64) + "/start", false},
	} {
		request, _ := http.NewRequest(test.method, "http://source"+test.path, nil)
		if (scope.allow(request) == nil) != test.allowed {
			t.Errorf("scope mismatch: %s %s", test.method, test.path)
		}
	}
	configuration, err := migrationVolumeConfiguration(intent, image)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.HostConfig.Mounts[0].Type != "bind" || !configuration.HostConfig.Mounts[0].ReadOnly || configuration.HostConfig.Mounts[0].BindOptions.CreateMountpoint || !configuration.HostConfig.ReadonlyRootfs || configuration.HostConfig.NetworkMode != "none" || !configuration.HostConfig.AutoRemove || configuration.Config.Cmd[0] != "/orbwin-fs" {
		t.Fatal("source helper could mutate/recreate a volume or execute workload code")
	}
}

func TestMigrationVolumeReceiptFailureDoesNotAdvanceOrMutateCleanup(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
	job, err := beginMigrationVolume(migrationVolumeTestIntent())
	if err != nil {
		t.Fatal(err)
	}
	defer job.lease.Close()
	if err := job.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := job.append(migrationVolumeEvent{Phase: "importing"}); err == nil || !job.failed || job.record.Phase != "intent" {
		t.Fatal("receipt write failure advanced durable state")
	}
	if err := cleanupMigrationVolume(context.Background(), nil, nil, job); err == nil {
		t.Fatal("uncertain persistence allowed cleanup")
	}
}
func TestMigrationVolumeCleanupRefusesForeignOrAdoptedResources(t *testing.T) {
	for _, scenario := range []string{"", "wrong-daemon", "foreign-helper", "writable-mount", "tagged-image", "unknown-image", "unknown-helper"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transient"))
			intent := migrationVolumeTestIntent()
			job, err := beginMigrationVolume(intent)
			if err != nil {
				t.Fatal(err)
			}
			defer job.close()
			image := "sha256:" + strings.Repeat("d", 64)
			helper := strings.Repeat("e", 64)
			job.append(migrationVolumeEvent{Phase: "importing"})
			if scenario != "unknown-image" {
				job.append(migrationVolumeEvent{Phase: "image", ID: image})
				job.append(migrationVolumeEvent{Phase: "creating"})
			}
			if scenario != "unknown-image" && scenario != "unknown-helper" {
				job.append(migrationVolumeEvent{Phase: "container", ID: helper})
			}
			configuration, _ := migrationVolumeConfiguration(intent, image)
			deletes := []string{}
			scope := &migrationVolumeScope{token: intent.Token, asset: intent.AssetSHA, helper: intent.helperName()}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "DELETE" {
					deletes = append(deletes, r.URL.Path)
					w.Write([]byte(`[]`))
					return
				}
				switch r.URL.Path {
				case "/v1.56/info":
					id := "source"
					if scenario == "wrong-daemon" {
						id = "foreign"
					}
					json.NewEncoder(w).Encode(map[string]string{"ID": id, "OSType": "linux", "OperatingSystem": "Docker Desktop"})
				case "/v1.56/version":
					w.Write([]byte(`{"Version":"29.8.0","Os":"linux","Arch":"amd64","ApiVersion":"1.56"}`))
				case "/v1.56/images/json":
					w.Write([]byte(`[]`))
				case "/v1.56/containers/" + helper + "/json":
					if scenario == "foreign-helper" {
						configuration.Config.Labels["dev.orbwin.import-token"] = "foreign"
					}
					if scenario == "writable-mount" {
						configuration.HostConfig.Mounts[0].ReadOnly = false
					}
					json.NewEncoder(w).Encode(map[string]any{"Id": helper, "Name": "/" + intent.helperName(), "Image": image, "Config": configuration.Config, "HostConfig": configuration.HostConfig})
				case "/v1.56/containers/" + intent.helperName() + "/json":
					http.Error(w, `{"message":"missing"}`, 404)
				case "/v1.56/images/" + image + "/json":
					tags := []string{}
					if scenario == "tagged-image" {
						tags = []string{"user/adopted:kept"}
					}
					json.NewEncoder(w).Encode(map[string]any{"Id": image, "Os": "linux", "Architecture": "amd64", "RepoTags": tags, "Config": map[string]any{"Labels": map[string]string{"dev.orbwin.component": "file-maintenance-tools", "dev.orbwin.import-token": intent.Token, "dev.orbwin.source-sha256": intent.AssetSHA}}})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithHTTPClient(&http.Client{Transport: migrationVolumeTransport{server.Client().Transport, scope}}), client.WithAPIVersion("1.56"), client.WithHTTPRequestHook(scope.allow))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			err = cleanupMigrationVolume(context.Background(), cli, scope, job)
			if scenario == "" {
				if err != nil || job.record.Phase != "cleaned" || len(deletes) != 2 {
					t.Fatalf("cleanup did not settle exact tools: %v %v", err, deletes)
				}
			} else {
				if err == nil {
					t.Fatal("uncertain/foreign source cleanup succeeded")
				}
				if scenario == "tagged-image" {
					if len(deletes) != 1 || !strings.Contains(deletes[0], "/containers/") {
						t.Fatal("adopted image removed")
					}
				} else if len(deletes) != 0 {
					t.Fatalf("foreign/unknown resource removed: %v", deletes)
				}
			}
			if bytes.Contains(job.record.Export, []byte("complete")) {
				t.Fatal("cleanup manufactured an export")
			}
		})
	}
}
