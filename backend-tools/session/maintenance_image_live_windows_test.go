//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

// Inject only the initial cache miss. The actual archive Load and subsequent
// inspection use the fixed owned pipe. No baseline image is deleted for a test.
type missingMaintenanceOnce struct {
	*client.Client
	missing bool
}

func (cli *missingMaintenanceOnce) ImageInspect(ctx context.Context, id string, options ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	if !cli.missing {
		cli.missing = true
		return client.ImageInspectResult{}, cerrdefs.ErrNotFound
	}
	return cli.Client.ImageInspect(ctx, id, options...)
}
func TestOwnedOfflineMaintenanceLoadPreservesTags(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" || os.Getenv("ORBWIN_TEST_MAINTENANCE_ARCHIVE") == "" {
		t.Skip("explicit owned endpoint and staged offline asset")
	}
	filename := os.Getenv("ORBWIN_TEST_MAINTENANCE_ARCHIVE")
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	index, err := checkMaintenanceArchiveFile(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORBWIN_MAINTENANCE_IMAGE", filename)
	t.Setenv("ORBWIN_MAINTENANCE_IMAGE_SHA256", index.SHA256)
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != "d9c00194-ea9d-439a-8561-6a0ec580fbd5" || info.Info.OSType != "linux" {
		t.Fatalf("owned identity differs: %v", err)
	}
	tags := func() map[string][]string {
		result, err := cli.ImageList(ctx, client.ImageListOptions{All: true})
		if err != nil {
			t.Fatal(err)
		}
		values := map[string][]string{}
		for _, image := range result.Items {
			if len(image.RepoTags) > 0 {
				names := append([]string(nil), image.RepoTags...)
				sort.Strings(names)
				values[image.ID] = names
			}
		}
		return values
	}
	before := tags()
	wrapped := &missingMaintenanceOnce{Client: cli}
	if err := ensureMaintenanceImage(ctx, wrapped); err != nil {
		t.Fatal(err)
	}
	after := tags()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("untagged maintenance load changed image tags")
	}
	output := os.Getenv("ORBWIN_QUALIFY_OUTPUT")
	if output != "" {
		data, _ := json.MarshalIndent(map[string]any{"success": true, "daemon": info.Info.ID, "archiveSha256": index.SHA256, "initialMissInjected": true, "actualLoadAndReadback": true, "tagsPreserved": true, "actualEmptyEngine": false}, "", "  ")
		if err := os.WriteFile(filepath.Join(output, "p2-maintenance-live-20261025.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
