package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

func TestContainerEditMappingRespectsNestedMountsAndLinks(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, test := range []struct {
		name, source, linkAt, wantVolume, wantPath string
		mounts                                     []container.MountPoint
	}{
		{"volume", "/data/project/file", "", "data", "/project/file", []container.MountPoint{{Type: mount.TypeVolume, Name: "data", Destination: "/data", RW: true}}},
		{"nested read-only", "/data/private/file", "", "", "", []container.MountPoint{{Type: mount.TypeVolume, Name: "data", Destination: "/data", RW: true}, {Type: mount.TypeVolume, Name: "private", Destination: "/data/private", RW: false}}},
		{"nested bind", "/data/project/file", "", "", "", []container.MountPoint{{Type: mount.TypeBind, Destination: "/data/project", RW: true}, {Type: mount.TypeVolume, Name: "data", Destination: "/data", RW: true}}},
		{"prefix boundary", "/database/file", "", "", "", []container.MountPoint{{Type: mount.TypeVolume, Name: "data", Destination: "/data", RW: true}}},
		{"link ancestry", "/data/link/file", "/data/link", "", "", []container.MountPoint{{Type: mount.TypeVolume, Name: "data", Destination: "/data", RW: true}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/json") {
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Mounts": test.mounts})
					return
				}
				if r.Method != http.MethodHead {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
					return
				}
				mode := os.FileMode(0644)
				if r.URL.Query().Get("path") == test.linkAt {
					mode |= os.ModeSymlink
				}
				data, _ := json.Marshal(map[string]any{"name": "file", "mode": mode})
				w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(data))
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.56"))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			got := containerEditSource(context.Background(), cli, id, test.source)
			if got.Volume != test.wantVolume || got.Path != test.wantPath || (got.Volume == "" && got.Reason == "") {
				t.Fatalf("unsafe/incorrect edit mapping: %+v", got)
			}
		})
	}
}

func TestStandaloneSourcesCannotSelectHostPathsOrImageTags(t *testing.T) {
	for _, item := range []struct{ kind, target string }{{"volume", "/var/lib/docker"}, {"volume", "--help"}, {"image", "alpine:latest"}, {"image", strings.Repeat("z", 64)}, {"host", "C:/data"}} {
		if resourceFileArguments(item.kind, item.target) == nil {
			t.Fatalf("accepted source %v", item)
		}
	}
	if err := resourceFileArguments("image", "sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := resourceFileArguments("volume", "project_data"); err != nil {
		t.Fatal(err)
	}
}
func TestVolumeReplacementMetadataIsAConflict(t *testing.T) {
	original := volume.Volume{Name: "data", Driver: "local", CreatedAt: "before", Mountpoint: "/engine/path"}
	changed := original
	changed.CreatedAt = "after"
	if volumeSame(original, changed) {
		t.Fatal("replacement volume accepted")
	}
	changed = original
	changed.Labels = map[string]string{"owner": "other"}
	if volumeSame(original, changed) {
		t.Fatal("changed ownership accepted")
	}
	if !volumeSame(original, original) {
		t.Fatal("unchanged descriptor rejected")
	}
}
