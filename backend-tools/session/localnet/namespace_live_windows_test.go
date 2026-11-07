//go:build windows

package localnet

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// This opt-in proof targets only the registered OrbWin daemon and creates unique
// disposable fixtures. It never discovers or mutates another engine's resources.
func TestOwnedNamespaceStreamRevocation(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" {
		t.Skip("explicit owned daemon qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cli, err := client.New(client.WithHost("npipe:////./pipe/docker-rancher-desktop-orbwin"), client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	const daemon = "d9c00194-ea9d-439a-8561-6a0ec580fbd5"
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != daemon || info.Info.OSType != "linux" {
		t.Fatal("owned daemon mismatch", err)
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "artifacts/qualification/network-probe"))
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	if err = w.WriteHeader(&tar.Header{Name: "probe", Mode: 0755, Size: int64(len(fixture))}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(fixture); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	imported, err := cli.ImageImport(ctx, client.ImageImportSource{SourceName: "-", Source: &archive}, "", client.ImageImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	decoder := json.NewDecoder(io.LimitReader(imported, 65536))
	fixtureID := ""
	for decoder.Decode(&result) == nil {
		if result.Error != "" {
			t.Fatal(result.Error)
		}
		if strings.HasPrefix(result.Status, "sha256:") {
			fixtureID = result.Status
		}
	}
	imported.Close()
	if !fullID(strings.TrimPrefix(fixtureID, "sha256:")) {
		t.Fatal("fixture import missing full image ID")
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, e := cli.ImageRemove(clean, fixtureID, client.ImageRemoveOptions{}); e != nil {
			t.Error(e)
		}
	}()
	name := fmt.Sprintf("orbwin-p2-stream-%d", time.Now().UnixNano())
	created, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{Name: name, Config: &container.Config{Image: fixtureID, Cmd: []string{"/probe"}, Labels: map[string]string{"dev.orbwin.qualification": name}}, HostConfig: &container.HostConfig{ReadonlyRootfs: true, CapDrop: []string{"ALL"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, e := cli.ContainerRemove(clean, created.ID, client.ContainerRemoveOptions{Force: true}); e != nil {
			t.Error(e)
		}
	}()
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	target, err := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := os.ReadFile(filepath.Join(root, "artifacts/backend/orbwin-services"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(asset)
	directory := filepath.Join(root, "artifacts/qualification", name)
	ledger, err := OpenNamespaceLedger(directory, daemon)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	image, err := ledger.PrepareToolsImage(ctx, cli, asset, hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := OpenNamespaceManagerWithLedger(ctx, cli, ledger, image)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if e := manager.Close(clean); e != nil {
			t.Error(e)
		}
	}()
	handle, err := manager.Acquire(ctx, created.ID, target.Container.State.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	route := ServiceRoute{ContainerID: created.ID, StartedAt: handle.Intent.StartedAt, HelperID: handle.ID, Port: 39081}
	for round := 0; round < 32; round++ {
		connection, e := manager.Dial(ctx, route)
		if e != nil {
			t.Fatal(round, e)
		}
		_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
		if _, e = io.WriteString(connection, "GET /idle HTTP/1.1\r\nHost: fixture\r\n\r\n"); e != nil {
			connection.Close()
			t.Fatal(e)
		}
		reader := bufio.NewReader(connection)
		line, e := reader.ReadString('\n')
		if e != nil || !strings.Contains(line, "200") {
			connection.Close()
			t.Fatal(line, e)
		}
		if e = connection.Close(); e != nil {
			t.Fatal(round, e)
		}
		if e = manager.Renew(ctx, handle); e != nil {
			t.Fatal(round, e)
		}
		state, e := cli.ContainerInspect(ctx, handle.ID, client.ContainerInspectOptions{})
		if e != nil || !state.Container.State.Running || state.Container.State.OOMKilled || len(state.Container.ExecIDs) != 0 {
			t.Fatalf("round %d: child remained or helper failed: %+v, %v", round, state.Container, e)
		}
	}
	for batch := 0; batch < 8; batch++ {
		var ready, complete sync.WaitGroup
		ready.Add(4)
		complete.Add(4)
		release := make(chan struct{})
		failures := make(chan error, 4)
		for range 4 {
			go func() {
				defer complete.Done()
				connection, e := manager.Dial(ctx, route)
				if e != nil {
					ready.Done()
					failures <- e
					return
				}
				_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
				_, e = io.WriteString(connection, "GET /idle HTTP/1.1\r\nHost: fixture\r\n\r\n")
				if e == nil {
					line, readErr := bufio.NewReader(connection).ReadString('\n')
					if readErr != nil || !strings.Contains(line, "200") {
						e = fmt.Errorf("idle response %q: %v", line, readErr)
					}
				}
				ready.Done()
				<-release
				closed := connection.Close()
				if e != nil {
					failures <- e
				} else if closed != nil {
					failures <- closed
				}
			}()
		}
		ready.Wait()
		close(release)
		complete.Wait()
		close(failures)
		for e := range failures {
			t.Fatal(batch, e)
		}
		if err = manager.Renew(ctx, handle); err != nil {
			t.Fatal(err)
		}
		state, e := cli.ContainerInspect(ctx, handle.ID, client.ContainerInspectOptions{})
		if e != nil || !state.Container.State.Running || state.Container.State.OOMKilled || len(state.Container.ExecIDs) != 0 {
			t.Fatalf("batch %d retained exec or helper failed: %+v %v", batch, state.Container, e)
		}
	}
	if err = manager.Release(ctx, handle); err != nil {
		t.Fatal(err)
	}
	qualifyOwnedRelay(t, ctx, cli, manager, image, created.ID, directory)
	after, err := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil || !after.Container.State.Running || after.Container.State.StartedAt != target.Container.State.StartedAt {
		t.Fatal("target incarnation changed", err)
	}
	report := map[string]any{"daemon": daemon, "target": created.ID, "targetStartedAt": handle.Intent.StartedAt, "helper": handle.ID, "tools": image, "sequentialRevocations": 32, "concurrentBatches": 8, "streamsPerBatch": 4, "remainingExecsAfterEach": 0, "helperOOMKilled": false, "targetIncarnationPreserved": true, "completedAt": time.Now().UTC()}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "stream-proof.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("64 idle stream revocations acknowledged (32 sequential, 8 batches of 4); zero remaining execs; helper healthy; target unchanged; evidence %s", directory)
}
