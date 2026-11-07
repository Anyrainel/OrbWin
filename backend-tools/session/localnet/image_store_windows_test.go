//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

type imageStoreDaemon struct {
	mu                      sync.Mutex
	t                       *testing.T
	ledger                  *NamespaceLedger
	asset, token, id, fault string
	imports, lists          int
	exists                  bool
}

func (daemon *imageStoreDaemon) serve(w http.ResponseWriter, r *http.Request) {
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	encode := func(value any) { _ = json.NewEncoder(w).Encode(value) }
	switch r.Method + " " + r.URL.Path {
	case "GET /v1.56/info":
		id := "owned-daemon"
		if daemon.fault == "foreign-daemon" {
			id = "other"
		}
		encode(map[string]string{"ID": id, "OSType": "linux"})
	case "POST /v1.56/images/create":
		daemon.imports++
		receipt, err := daemon.ledger.readImageReceipt(daemon.asset)
		if err != nil || receipt.ID != "" {
			daemon.t.Errorf("image import preceded flushed intent: %+v %v", receipt, err)
			http.Error(w, "missing intent", 500)
			return
		}
		daemon.token = receipt.Token
		daemon.id = "sha256:" + fmt.Sprintf("%064x", daemon.imports)
		daemon.exists = daemon.fault != "uncertain-missing"
		_, _ = io.Copy(io.Discard, r.Body)
		if strings.HasPrefix(daemon.fault, "uncertain") {
			http.Error(w, "import reply lost", 500)
			return
		}
		encode(map[string]string{"status": daemon.id})
	case "GET /v1.56/images/json":
		daemon.lists++
		var filters client.Filters
		if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil {
			daemon.t.Error(err)
		}
		if len(filters["label"]) != 3 {
			daemon.t.Error("pending import lookup was not scoped by all ownership labels")
		}
		for _, expected := range []string{"dev.orbwin.component=local-services-tools", "dev.orbwin.source-sha256=" + daemon.asset, "dev.orbwin.import-token=" + daemon.token} {
			if !filters["label"][expected] {
				daemon.t.Error("pending import lookup omitted exact token/asset/component")
			}
		}
		items := []map[string]string{}
		if daemon.exists {
			items = append(items, map[string]string{"Id": daemon.id})
		}
		if daemon.fault == "ambiguous" {
			items = append(items, map[string]string{"Id": "sha256:" + strings.Repeat("f", 64)})
		}
		encode(items)
	default:
		if r.Method == "GET" && r.URL.Path == "/v1.56/images/"+daemon.id+"/json" {
			if !daemon.exists {
				w.WriteHeader(404)
				encode(map[string]string{"message": "No such image"})
				return
			}
			token := daemon.token
			if daemon.fault == "foreign-label" {
				token = strings.Repeat("0", 32)
			}
			encode(map[string]any{"Id": daemon.id, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{"Labels": map[string]string{"dev.orbwin.component": "local-services-tools", "dev.orbwin.source-sha256": daemon.asset, "dev.orbwin.import-token": token}}})
			return
		}
		daemon.t.Errorf("unexpected image API/mutation: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected API", 500)
	}
}
func imageStoreFixture(t *testing.T, fault string) (*NamespaceLedger, *imageStoreDaemon, *client.Client, []byte, string) {
	t.Helper()
	ledger, err := OpenNamespaceLedger(t.TempDir(), "owned-daemon")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ledger.Close() })
	binary, asset := testToolsAsset()
	daemon := &imageStoreDaemon{t: t, ledger: ledger, asset: asset, fault: fault}
	server := httptest.NewServer(http.HandlerFunc(daemon.serve))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost("tcp://"+server.Listener.Addr().String()), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })
	return ledger, daemon, cli, binary, asset
}
func TestToolsImageStoreImportsOnceAndReusesExactRecordedProvenance(t *testing.T) {
	ledger, daemon, cli, binary, asset := imageStoreFixture(t, "")
	image, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
	if err != nil || again != image {
		t.Fatal("committed image was reimported/replaced")
	}
	if _, err := ledger.ArchiveToolsImageReceipt(context.Background(), cli, asset, image.Token); err == nil {
		t.Fatal("existing committed image receipt was retired")
	}
	daemon.mu.Lock()
	imports := daemon.imports
	daemon.mu.Unlock()
	if imports != 1 {
		t.Fatal("image reuse dispatched another import")
	}
	if records, err := ledger.records(); err != nil || len(records) != 0 {
		t.Fatal("image receipt was mistaken for namespace ownership")
	}
	manager, err := OpenNamespaceManagerWithLedger(context.Background(), cli, ledger, image)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.CreateAuthority(strings.Repeat("c", 32), time.Now()); err != nil {
		t.Fatal("authority setup lost the common exclusive owner")
	}
	if other, err := OpenNamespaceLedger(ledger.directory, "owned-daemon"); err == nil {
		other.Close()
		t.Fatal("image-to-manager handoff lost the process lease")
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.AuthorityInfo(strings.Repeat("c", 32)); err == nil {
		t.Fatal("manager close did not close its transferred owner")
	}
}
func TestToolsImageStoreRecoversUnknownAcceptanceWithoutRepeatingImport(t *testing.T) {
	for _, fault := range []string{"uncertain", "uncertain-missing"} {
		t.Run(fault, func(t *testing.T) {
			ledger, daemon, cli, binary, asset := imageStoreFixture(t, fault)
			if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
				t.Fatal("lost import reply became success")
			}
			if fault == "uncertain-missing" {
				if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
					t.Fatal("missing unknown import was forgotten/replayed")
				}
				daemon.mu.Lock()
				daemon.exists = true
				daemon.mu.Unlock()
			}
			daemon.mu.Lock()
			daemon.fault = "ambiguous"
			daemon.mu.Unlock()
			if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
				t.Fatal("ambiguous ownership lookup selected an image")
			}
			daemon.mu.Lock()
			daemon.fault = "foreign-label"
			daemon.mu.Unlock()
			if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
				t.Fatal("foreign image labels were adopted")
			}
			daemon.mu.Lock()
			daemon.fault = ""
			daemon.mu.Unlock()
			image, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
			if err != nil || image.ID == "" {
				t.Fatalf("late owned image could not be recovered: %+v %v", image, err)
			}
			daemon.mu.Lock()
			imports := daemon.imports
			daemon.mu.Unlock()
			if imports != 1 {
				t.Fatal("uncertain import was retried")
			}
		})
	}
}
func TestToolsImageStoreExplicitRepairPreservesOldReceiptAndNeverDeletesImages(t *testing.T) {
	ledger, daemon, cli, binary, asset := imageStoreFixture(t, "")
	image, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := os.ReadFile(ledger.imagePath(asset))
	daemon.mu.Lock()
	daemon.exists = false
	daemon.mu.Unlock()
	if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
		t.Fatal("missing known image was automatically replaced")
	}
	if _, err := ledger.ArchiveToolsImageReceipt(context.Background(), cli, asset, strings.Repeat("0", 32)); err == nil {
		t.Fatal("stale review retired a different receipt")
	}
	archive, err := ledger.ArchiveToolsImageReceipt(context.Background(), cli, asset, image.Token)
	if err != nil {
		t.Fatal(err)
	}
	archived, _ := os.ReadFile(archive)
	if string(old) != string(archived) {
		t.Fatal("repair discarded or modified provenance")
	}
	if _, err := os.Lstat(ledger.imagePath(asset)); !os.IsNotExist(err) {
		t.Fatal("active missing-image receipt was not retired")
	}
	next, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
	if err != nil || next.Token == image.Token || next.ID == image.ID {
		t.Fatal("explicit repair did not create a fresh separately owned import")
	}
	if filepath.Dir(archive) != ledger.directory {
		t.Fatal("receipt archive escaped persistent service ownership")
	}
}
func TestToolsImageStoreRefusesForeignEndpointAndDamagedReceipt(t *testing.T) {
	ledger, daemon, cli, binary, asset := imageStoreFixture(t, "foreign-daemon")
	if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
		t.Fatal("foreign endpoint was imported into")
	}
	if _, err := os.Lstat(ledger.imagePath(asset)); !os.IsNotExist(err) {
		t.Fatal("foreign endpoint caused ownership record creation")
	}
	daemon.mu.Lock()
	daemon.fault = ""
	daemon.mu.Unlock()
	if err := os.WriteFile(ledger.imagePath(asset), []byte(`{"version":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset); err == nil {
		t.Fatal("damaged receipt was regenerated")
	}
	daemon.mu.Lock()
	imports := daemon.imports
	daemon.mu.Unlock()
	if imports != 0 {
		t.Fatal("damaged/foreign setup dispatched an import")
	}
}
func TestZeroServiceOwnerCannotWriteState(t *testing.T) {
	var ledger NamespaceLedger
	installation := strings.Repeat("c", 32)
	if _, err := ledger.CreateAuthority(installation, time.Now()); err == nil {
		t.Fatal("zero owner wrote authority state")
	}
	if _, err := ledger.AuthorityInfo(installation); err == nil {
		t.Fatal("zero owner read authority state")
	}
	binary, asset := testToolsAsset()
	if _, err := ledger.PrepareToolsImage(context.Background(), nil, binary, asset); err == nil {
		t.Fatal("zero owner dispatched an image import")
	}
	if err := ledger.Begin(namespaceFixture()); err == nil {
		t.Fatal("zero owner wrote namespace intent")
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestToolsImageArchiveConflictRetainsOriginalAndExistingArchive(t *testing.T) {
	ledger, daemon, cli, binary, asset := imageStoreFixture(t, "uncertain-missing")
	_, _ = ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
	receipt, err := ledger.readImageReceipt(asset)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(ledger.imagePath(asset))
	archive := filepath.Join(ledger.directory, "tools-image-"+asset+"-"+receipt.Token+".archived-receipt")
	if err := os.WriteFile(archive, []byte("preserve conflicting archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ArchiveToolsImageReceipt(context.Background(), cli, asset, receipt.Token); err == nil {
		t.Fatal("conflicting archive was overwritten")
	}
	after, _ := os.ReadFile(ledger.imagePath(asset))
	other, _ := os.ReadFile(archive)
	if string(before) != string(after) || string(other) != "preserve conflicting archive" {
		t.Fatal("archive conflict changed existing evidence")
	}
	daemon.mu.Lock()
	imports := daemon.imports
	daemon.mu.Unlock()
	if imports != 1 {
		t.Fatal("archive conflict replayed an import")
	}
}
func TestToolsImageManagerHandoffFailureRetainsCallerOwnership(t *testing.T) {
	ledger, _, cli, binary, asset := imageStoreFixture(t, "")
	image, err := ledger.PrepareToolsImage(context.Background(), cli, binary, asset)
	if err != nil {
		t.Fatal(err)
	}
	bad := image
	bad.Token = strings.Repeat("0", 32)
	if manager, err := OpenNamespaceManagerWithLedger(context.Background(), cli, ledger, bad); err == nil {
		manager.Close(context.Background())
		t.Fatal("uncommitted image reference was accepted")
	}
	if _, err := ledger.CreateAuthority(strings.Repeat("c", 32), time.Now()); err != nil {
		t.Fatal("failed handoff released or invalidated the caller's ownership")
	}
	if other, err := OpenNamespaceLedger(ledger.directory, "owned-daemon"); err == nil {
		other.Close()
		t.Fatal("failed handoff opened an ownership gap")
	}
}
