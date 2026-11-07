//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"net"
	"orbwin.local/session/internal/sdkself"
	"strings"
	"testing"
	"time"
)

func TestCurrentUserServicePipeCarriesTypedRequestsAndKeepsRuntimeIndependent(t *testing.T) {
	// Real current-user pipe framing only, with explicitly injected inert peer
	// authentication. This does not qualify native SDK identity or cache origin.
	identity := sdkself.Identity{Schema: 1, PID: 123, CreationTime: "123456789", Session: 1, ImageSHA256: strings.Repeat("a", 64), BundleSHA256: strings.Repeat("b", 64), LeaseHeld: true}
	runtime, _ := runtimeFixtureWithOptions(t, nil, func(options *RuntimeOptions) {
		options.SelfIdentity = &identity
		options.BinarySHA256 = identity.ImageSHA256
	})
	callWireFixture := func(ctx context.Context, home, installation string, request ServiceRequest) (ServiceResponse, error) {
		return dialAuthenticatedServiceIPC(ctx, home, installation, request, func(context.Context, net.Conn) (servicePeer, error) {
			return &sdkPeerFixture{identity: identity}, nil
		})
	}
	home := runtime.opts.Ledger.directory
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- ServeServiceIPC(ctx, runtime, home, runtime.opts.Installation) }()
	var response ServiceResponse
	var err error
	deadline := time.Now().Add(time.Second)
	for {
		callCtx, done := context.WithTimeout(context.Background(), 100*time.Millisecond)
		response, err = callWireFixture(callCtx, home, runtime.opts.Installation, ServiceRequest{Version: 1, Operation: "snapshot"})
		done()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if response.Error != "" || response.State == nil {
		t.Fatal(response)
	}
	callCtx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	bad, err := callWireFixture(callCtx, home, runtime.opts.Installation, ServiceRequest{Version: 1, Operation: "shell"})
	if err != nil || !strings.Contains(bad.Error, "unsupported") {
		t.Fatal(bad, err)
	}
	if response, err = callWireFixture(callCtx, home, runtime.opts.Installation, ServiceRequest{Version: 1, Operation: "snapshot"}); err != nil || response.State == nil {
		t.Fatal("client disconnect stopped runtime", err)
	}
	if _, err = callWireFixture(callCtx, home, runtime.opts.Installation, ServiceRequest{Version: 1, Operation: "stop-process"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pipe retained after actual runtime shutdown")
	}
}
func TestServiceRequestCannotSupplyUntypedAdministrativeFields(t *testing.T) {
	runtime, _ := runtimeFixture(t)
	for _, raw := range []string{`{"version":1,"operation":"snapshot","command":"delete"}`, `{"version":2,"operation":"snapshot"}`, `{"version":1,"operation":"snapshot","settings":{"enabled":true}}`, `{"version":1,"operation":"snapshot","rootSha256":"unreviewed"}`, `{"version":1,"operation":"adopt-authority","rootSha256":"unreviewed"}`, `{"version":1,"operation":"snapshot","ports":{"http":80,"https":443}}`, `{"version":1,"operation":"snapshot","newRootSha256":"unreviewed"}`, `{"version":1,"operation":"rotate-authority"}`} {
		response := dispatchServiceRequest(context.Background(), runtime, []byte(raw))
		if response.Error == "" {
			encoded, _ := json.Marshal(response)
			t.Fatal("untyped request accepted", string(encoded))
		}
	}
}
