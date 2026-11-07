package localnet

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func testToolsAsset() ([]byte, string) {
	asset := make([]byte, 128)
	copy(asset, "\x7fELF")
	asset[4], asset[5], asset[18] = 2, 1, 62
	digest := sha256.Sum256(asset)
	return asset, hex.EncodeToString(digest[:])
}

func TestToolsImportVerifiesArchiveAndReturnedOwnership(t *testing.T) {
	testStaticToolsImport(t, ImportToolsImage, "orbwin-services", "local-services-tools")
}
func TestFileToolsImportHasSeparateExecutableAndOwnership(t *testing.T) {
	testStaticToolsImport(t, ImportFileToolsImage, "orbwin-fs", "file-maintenance-tools")
}
func testStaticToolsImport(t *testing.T, importImage func(context.Context, *client.Client, []byte, string, string) (string, error), executable, component string) {
	asset, digest := testToolsAsset()
	token := strings.Repeat("a", 32)
	imageID := "sha256:" + strings.Repeat("b", 64)
	for _, failure := range []string{"", "foreign-label", "wrong-image", "import-error", "oversized", "no-id", "conflicting-id"} {
		t.Run(failure, func(t *testing.T) {
			imports, inspections := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "POST /v1.56/images/create":
					imports++
					if r.URL.Query().Get("fromSrc") != "-" || r.URL.Query().Get("repo") != "" {
						t.Error("import selected a remote source or mutable tag")
					}
					changes := strings.Join(r.URL.Query()["changes"], "\n")
					if !strings.Contains(changes, "dev.orbwin.source-sha256="+digest) || !strings.Contains(changes, "dev.orbwin.import-token="+token) || !strings.Contains(changes, "dev.orbwin.component="+component) {
						t.Error("import omitted asset ownership")
					}
					reader := tar.NewReader(r.Body)
					entries := 0
					for {
						header, err := reader.Next()
						if err == io.EOF {
							break
						}
						if err != nil {
							t.Error(err)
							return
						}
						entries++
						if header.Typeflag == tar.TypeReg {
							body, err := io.ReadAll(reader)
							if err != nil || header.Name != executable || header.Mode != 0755 || !bytes.Equal(body, asset) {
								t.Error("unexpected executable archive entry")
							}
						} else if header.Typeflag != tar.TypeDir || (header.Name != "etc/" && header.Name != "run/" && header.Name != "run/orbwin/") {
							t.Error("unexpected scratch image content")
						}
					}
					if entries != 4 {
						t.Errorf("unexpected image entry count: %d", entries)
					}
					message := map[string]string{"status": imageID}
					switch failure {
					case "import-error":
						message = map[string]string{"error": "fixture refused"}
					case "oversized":
						message["status"] = strings.Repeat("x", 65537)
					case "no-id":
						message["status"] = "done"
					}
					_ = json.NewEncoder(w).Encode(message)
					if failure == "conflicting-id" {
						_ = json.NewEncoder(w).Encode(map[string]string{"status": "sha256:" + strings.Repeat("e", 64)})
					}
				case "GET /v1.56/images/" + imageID + "/json":
					inspections++
					labels := map[string]string{"dev.orbwin.component": component, "dev.orbwin.source-sha256": digest, "dev.orbwin.import-token": token}
					id := imageID
					if failure == "foreign-label" {
						labels["dev.orbwin.import-token"] = strings.Repeat("c", 32)
					}
					if failure == "wrong-image" {
						id = "sha256:" + strings.Repeat("d", 64)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{"Labels": labels}})
				default:
					t.Errorf("unexpected mutation/API call: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected API call", 500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.56"))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			got, err := importImage(context.Background(), cli, asset, digest, token)
			if failure == "" && (err != nil || got != imageID) {
				t.Fatalf("valid local image import failed: %q %v", got, err)
			}
			if failure != "" && (err == nil || got != "") {
				t.Fatalf("uncertain/foreign image was accepted: %q %v", got, err)
			}
			if imports != 1 {
				t.Fatal("import retried an uncertain mutation")
			}
			if (failure == "import-error" || failure == "oversized" || failure == "no-id" || failure == "conflicting-id") && inspections != 0 {
				t.Fatal("invalid import result was inspected/adopted")
			}
		})
	}
}

func TestToolsImportRejectsUnverifiedAssetsBeforeAPI(t *testing.T) {
	asset, digest := testToolsAsset()
	for _, fixture := range []struct {
		asset         []byte
		digest, token string
	}{
		{asset, strings.Repeat("0", 64), strings.Repeat("a", 32)},
		{[]byte("not ELF"), digest, strings.Repeat("a", 32)},
		{asset, digest, strings.Repeat("A", 32)},
		{asset, digest, "short"},
	} {
		// A nil client deliberately proves rejection occurs before any API call.
		if _, err := ImportToolsImage(context.Background(), nil, fixture.asset, fixture.digest, fixture.token); err == nil {
			t.Fatal("unverified asset reached import")
		}
	}
}
