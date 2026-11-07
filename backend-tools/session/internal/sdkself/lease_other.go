//go:build !windows

package sdkself

import (
	"context"
	"errors"
	"net"
)

type Lease struct{}

func Hold(context.Context) (*Lease, error) {
	return nil, errors.New("native retained SDK requires Windows")
}
func HoldPeer(context.Context, net.Conn) (*Lease, error) {
	return nil, errors.New("native retained SDK peer requires Windows")
}
func (*Lease) Identity() (Identity, error) {
	return Identity{}, errors.New("native SDK lease unavailable")
}
func (*Lease) Close() error { return nil }
