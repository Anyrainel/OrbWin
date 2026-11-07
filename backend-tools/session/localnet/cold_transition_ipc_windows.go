//go:build windows

package localnet

import (
	"context"
	"net"
	"orbwin.local/session/internal/sdkstate"
)

// ServeColdServiceIPC reuses the exact already reserved native pipe and the
// ordinary SDK dispatcher/worker join. It never selects a path, binds a second
// listener, or treats listener ownership as proof of engine/runtime readiness.
func ServeColdServiceIPC(ctx context.Context, runtime *ServiceRuntime, lease *sdkstate.ColdIPCLease) error {
	if runtime == nil || lease == nil {
		return sdkstate.ErrState
	}
	return lease.Serve(ctx, func(listener net.Listener) error {
		return serveServiceIPCListener(ctx, runtime, listener)
	})
}
