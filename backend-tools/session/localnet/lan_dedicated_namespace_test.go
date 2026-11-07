package localnet

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestLANDedicatedHelperRemovalPreservesHTTPAndOtherLAN(t *testing.T) {
	base := &namespaceReferenceFake{dialFactory: func() (net.Conn, error) { return &referenceMemoryConn{}, nil }}
	shared := newSharedNamespaces(base, nil, nil)
	id, stamp := strings.Repeat("a", 64), "started"
	http, err := shared.Acquire(context.Background(), id, stamp)
	if err != nil {
		t.Fatal(err)
	}
	firstOwner, secondOwner := strings.Repeat("c", 64), strings.Repeat("d", 64)
	first, err := shared.borrowLAN(context.Background(), firstOwner, id, stamp)
	if err != nil {
		t.Fatal(err)
	}
	second, err := shared.borrowLAN(context.Background(), secondOwner, id, stamp)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == http.ID || first.ID == second.ID || second.ID == http.ID || len(shared.refs) != 3 || base.acquires.Load() != 3 {
		t.Fatal("native LAN leases shared lifecycle resources")
	}
	if err = shared.release(context.Background(), firstOwner, first); err != nil {
		t.Fatal(err)
	}
	if base.releases.Load() != 1 || len(shared.refs) != 2 {
		t.Fatal("LAN closure held or removed unrelated namespace")
	}
	for _, entry := range []struct {
		owner  string
		handle NamespaceHandle
	}{{httpNamespaceOwner, http}, {secondOwner, second}} {
		route := ServiceRoute{ContainerID: id, StartedAt: stamp, HelperID: entry.handle.ID, NamespaceLeaseSHA: entry.handle.Intent.LANLeaseSHA, Port: 80}
		conn, e := shared.dialOwner(context.Background(), entry.owner, route)
		if e != nil {
			t.Fatal("unrelated route lost after LAN close", e)
		}
		if e = conn.Close(); e != nil {
			t.Fatal(e)
		}
	}
	if err = shared.release(context.Background(), secondOwner, second); err != nil {
		t.Fatal(err)
	}
	if err = shared.Release(context.Background(), http); err != nil {
		t.Fatal(err)
	}
}
func TestLANNamespacePurposeBindsLedgerKeyAndNativeLabelsPreservingHTTPEncoding(t *testing.T) {
	http := namespaceFixture()
	before, _ := json.Marshal(http)
	if strings.Contains(string(before), "LANLeaseSHA") {
		t.Fatal("ordinary HTTP ledger encoding changed")
	}
	first, second := http, http
	first.LANLeaseSHA = strings.Repeat("e", 64)
	second.LANLeaseSHA = strings.Repeat("f", 64)
	if namespaceKey(first) == namespaceKey(http) || namespaceKey(first) == namespaceKey(second) {
		t.Fatal("LAN intent ownership collided")
	}
	config, err := NamespaceConfiguration(first)
	if err != nil || config.Config.Labels["dev.orbwin.lan-lease"] != first.LANLeaseSHA || string(config.HostConfig.NetworkMode) != "container:"+http.ContainerID {
		t.Fatal("dedicated helper changed workload namespace or lost immutable ownership", err)
	}
	first.LANLeaseSHA = "name"
	if _, err = NamespaceConfiguration(first); err == nil {
		t.Fatal("noncanonical native LAN purpose accepted")
	}
}
