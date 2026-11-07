//go:build windows

package localnet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"orbwin.local/session/internal/gateway"
)

func TestOwnedRelayRecoveryBeforeStart(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" {
		t.Skip("explicit owned engine qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cli, err := client.New(client.WithHost("npipe:////./pipe/docker-rancher-desktop-orbwin"), client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	const daemon = "d9c00194-ea9d-439a-8561-6a0ec580fbd5"
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != daemon {
		t.Fatal("owned daemon mismatch", err)
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "artifacts/qualification", fmt.Sprintf("orbwin-p2-relay-recovery-%d", time.Now().UnixNano()))
	ledger, err := OpenNamespaceLedger(directory, daemon)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { ledger.Close() }()
	asset, err := os.ReadFile(filepath.Join(root, "artifacts/backend/orbwin-services"))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(asset)
	image, err := ledger.PrepareToolsImage(ctx, cli, asset, hex.EncodeToString(hash[:]))
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := cli.NetworkInspect(ctx, "bridge", client.NetworkInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var cases []map[string]any
	for index, accepted := range []bool{true, false} {
		session := fmt.Sprintf("%032x", time.Now().UnixNano())
		identity, e := gateway.NewIdentity(session, time.Now())
		if e != nil {
			t.Fatal(e)
		}
		intent := RelayIntent{Session: session, ImageID: image.ID, AssetSHA: image.AssetSHA, BridgeID: bridge.Network.ID, Gateway: GatewayDescriptor{Port: 9, Session: session, PublicRoot: identity.PublicRoot(), ExpiresAt: identity.ExpiresAt()}, Ports: ServicePorts{HTTP: uint16(28080 + index), HTTPS: uint16(28443 + index)}}
		options, e := RelayConfiguration(intent)
		if e != nil {
			t.Fatal(e)
		}
		if e = ledger.BeginRelay(intent); e != nil {
			t.Fatal(e)
		}
		created, e := cli.ContainerCreate(ctx, options)
		if e != nil {
			t.Fatal(e)
		}
		defer func() {
			cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			if _, removeErr := cli.ContainerRemove(cleanup, created.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil && !cerrdefs.IsNotFound(removeErr) {
				t.Error(removeErr)
			}
		}()
		if accepted {
			if e = ledger.CreatedRelay(intent, created.ID); e != nil {
				t.Fatal(e)
			}
		}
		before, e := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
		if e != nil {
			t.Fatal(e)
		}
		// Replace the exclusive owner without ever starting the helper. The
		// unknown case deliberately omits accepting its returned ID in the receipt.
		ledger.Close()
		ledger, e = OpenNamespaceLedger(directory, daemon)
		if e != nil {
			t.Fatal(e)
		}
		manager, e := OpenRelayManager(ctx, cli, ledger, image)
		if e != nil {
			encoded, _ := json.MarshalIndent(map[string]any{"accepted": accepted, "helper": created.ID, "beforeNetworkSettings": before.Container.NetworkSettings, "error": e.Error()}, "", "  ")
			_ = os.WriteFile(filepath.Join(directory, fmt.Sprintf("failed-%d.json", index)), encoded, 0600)
			t.Fatal("real pre-start recovery", e)
		}
		if records, readErr := ledger.relayRecords(); readErr != nil || len(records) != 0 {
			t.Fatal(records, readErr)
		}
		if _, inspectErr := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{}); !cerrdefs.IsNotFound(inspectErr) {
			t.Fatal("stopped helper was not recovered", inspectErr)
		}
		if e = manager.Close(ctx); e != nil {
			t.Fatal(e)
		}
		cases = append(cases, map[string]any{"acceptedIDRecorded": accepted, "helper": created.ID, "beforeNetworkSettings": before.Container.NetworkSettings, "recoveredWithoutStart": true})
	}
	encoded, err := json.MarshalIndent(map[string]any{"daemon": daemon, "ownerReplacement": "explicit lease close/reopen, not process-kill proof", "cases": cases}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "proof.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("real accepted-ID and unknown-acceptance stopped helpers recovered without starting them:", directory)
}
