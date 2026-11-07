//go:build !windows

package localnet

import (
	"context"
	"net"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
)

func nativeLANManager(context.Context) (lanfirewall.CallerIdentity, error) {
	return lanfirewall.CallerIdentity{}, ErrLANAdapterUnavailable
}
func nativeLANLaunch(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
	return nil, ErrLANAdapterUnavailable
}
func nativeLANLaunchDatagram(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (landatagram.Connection, error)) (LANWorker, error) {
	return nil, ErrLANAdapterUnavailable
}
