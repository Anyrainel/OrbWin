package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type maintenanceFake struct {
	inspections, loads int
	inspect            func(int) (client.ImageInspectResult, error)
	load               func(context.Context, io.Reader) (client.ImageLoadResult, error)
}

func (fake *maintenanceFake) ImageInspect(ctx context.Context, id string, options ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	fake.inspections++
	if id != maintenanceImageID {
		return client.ImageInspectResult{}, errors.New("unexpected image selector")
	}
	return fake.inspect(fake.inspections)
}
func (fake *maintenanceFake) ImageLoad(ctx context.Context, input io.Reader, options ...client.ImageLoadOption) (client.ImageLoadResult, error) {
	fake.loads++
	if fake.load != nil {
		return fake.load(ctx, input)
	}
	return nil, errors.New("unexpected load")
}
func maintenanceInspection(t *testing.T, arch string) client.ImageInspectResult {
	t.Helper()
	var result client.ImageInspectResult
	if err := json.Unmarshal([]byte(`{"Id":"`+maintenanceImageID+`","Os":"linux","Architecture":"`+arch+`","RootFS":{"Type":"layers","Layers":["`+maintenanceDiffID+`"]}}`), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMaintenanceCacheRequiresExactIdentityAndOnlyNotFoundCanLoad(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		arch    string
		success bool
	}{{"cached", nil, "amd64", true}, {"wrong-platform", nil, "arm64", false}, {"permission", cerrdefs.ErrPermissionDenied, "", false}, {"cancelled", context.Canceled, "", false}} {
		t.Run(test.name, func(t *testing.T) {
			fake := &maintenanceFake{inspect: func(int) (client.ImageInspectResult, error) { return maintenanceInspection(t, test.arch), test.err }}
			err := ensureMaintenanceImage(context.Background(), fake)
			if (err == nil) != test.success || fake.loads != 0 {
				t.Fatalf("cache fallback differs: %v, loads %d", err, fake.loads)
			}
		})
	}
}
func TestMaintenanceTamperedAndForeignArchivesNeverReachLoad(t *testing.T) {
	file := filepath.Join(t.TempDir(), "archive.tar")
	data := []byte("not a trusted archive")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORBWIN_MAINTENANCE_IMAGE", file)
	for _, digest := range []string{"", strings.Repeat("0", 64), func() string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }()} {
		t.Setenv("ORBWIN_MAINTENANCE_IMAGE_SHA256", digest)
		fake := &maintenanceFake{inspect: func(int) (client.ImageInspectResult, error) { return client.ImageInspectResult{}, cerrdefs.ErrNotFound }}
		if err := ensureMaintenanceImage(context.Background(), fake); err == nil || fake.loads != 0 {
			t.Fatal("unverified archive reached image Load")
		}
	}
}
func TestMaintenanceBundledAssetLoadFailuresAndPostInspection(t *testing.T) {
	filename := os.Getenv("ORBWIN_TEST_MAINTENANCE_ARCHIVE")
	if filename == "" {
		t.Skip("explicit staged offline asset qualification")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	t.Setenv("ORBWIN_MAINTENANCE_IMAGE", filename)
	t.Setenv("ORBWIN_MAINTENANCE_IMAGE_SHA256", hex.EncodeToString(sum[:]))
	for _, test := range []struct {
		name, body           string
		success, postMissing bool
	}{{"loaded", "{\"stream\":\"Loaded image\"}\n", true, false}, {"error", "{\"error\":\"refused\"}\n", false, false}, {"malformed", "broken", false, false}, {"too-large", strings.Repeat("{}\n", 30000), false, false}, {"unconfirmed", "{}\n", false, true}} {
		t.Run(test.name, func(t *testing.T) {
			fake := &maintenanceFake{inspect: func(call int) (client.ImageInspectResult, error) {
				if call == 1 || test.postMissing {
					return client.ImageInspectResult{}, cerrdefs.ErrNotFound
				}
				return maintenanceInspection(t, "amd64"), nil
			}, load: func(ctx context.Context, input io.Reader) (client.ImageLoadResult, error) {
				actual, err := io.ReadAll(input)
				if err != nil || sha256.Sum256(actual) != sum {
					t.Fatal("asset stream differs")
				}
				return io.NopCloser(strings.NewReader(test.body)), nil
			}}
			err := ensureMaintenanceImage(context.Background(), fake)
			if (err == nil) != test.success || fake.loads != 1 {
				t.Fatalf("load outcome differs: %v, loads %d", err, fake.loads)
			}
		})
	}
}

func TestMaintenanceLoadUsesFrozenBytesAfterBundleReplacement(t *testing.T) {
	filename := os.Getenv("ORBWIN_TEST_MAINTENANCE_ARCHIVE")
	if filename == "" {
		t.Skip("explicit staged offline asset qualification")
	}
	payload, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "maintenance.tar")
	if err := os.WriteFile(copyPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	t.Setenv("ORBWIN_MAINTENANCE_IMAGE", copyPath)
	t.Setenv("ORBWIN_MAINTENANCE_IMAGE_SHA256", hex.EncodeToString(sum[:]))
	fake := &maintenanceFake{inspect: func(call int) (client.ImageInspectResult, error) {
		if call == 1 {
			return client.ImageInspectResult{}, cerrdefs.ErrNotFound
		}
		return maintenanceInspection(t, "amd64"), nil
	}, load: func(ctx context.Context, input io.Reader) (client.ImageLoadResult, error) {
		if err := os.WriteFile(copyPath, []byte("replaced bundle"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(input)
		if err != nil || sha256.Sum256(data) != sum {
			t.Fatal("Load consumed changed bundle bytes")
		}
		return io.NopCloser(strings.NewReader("{}\n")), nil
	}}
	if err := ensureMaintenanceImage(context.Background(), fake); err != nil {
		t.Fatal(err)
	}
}
