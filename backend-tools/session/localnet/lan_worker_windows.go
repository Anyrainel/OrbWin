//go:build windows

package localnet

import (
	"context"
	"net"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanworker"
)

func nativeLANManager(ctx context.Context) (lanfirewall.CallerIdentity, error) {
	if e := ctx.Err(); e != nil {
		return lanfirewall.CallerIdentity{}, e
	}
	return lanworker.ManagerIdentity()
}
func nativeLANLaunch(ctx context.Context, prepared lanhelper.Prepared, dial func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
	child, err := lanworker.LaunchNative(ctx, prepared, dial)
	if child == nil {
		return nil, err
	}
	return child, err
}
func nativeLANLaunchDatagram(ctx context.Context, prepared lanhelper.Prepared, dial func(context.Context, lanhelper.Stream) (landatagram.Connection, error)) (LANWorker, error) {
	child, err := lanworker.LaunchDatagramNative(ctx, prepared, dial)
	if child == nil {
		return nil, err
	}
	return child, err
}
