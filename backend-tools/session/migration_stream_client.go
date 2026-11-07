package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/moby/moby/client"
)

// Keep a concrete SDK transport: Moby v0.6.2 obtains its hijack dialer from
// *http.Transport, separately from ordinary RoundTrip. RegisterProtocol applies
// the bounded metadata/scope wrapper to ordinary HTTP while the request hook
// protects BOTH paths. Neither path may fall back to a TCP/default engine dial.
// The caller owns the connection session; close releases both HTTP idle pools.
// This is for fixed-output maintenance exec, not interactive stdin/half-close.
func newMigrationStreamClient(dial func(context.Context, string, string) (net.Conn, error), wrap func(http.RoundTripper) http.RoundTripper, hook client.RequestHook) (*client.Client, func(), error) {
	inner := &http.Transport{DialContext: dial, MaxIdleConns: 2, MaxConnsPerHost: 2, IdleConnTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	outer := &http.Transport{DialContext: dial}
	outer.RegisterProtocol("http", wrap(inner))
	httpClient := &http.Client{Transport: outer, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("source redirects are refused") }}
	cli, err := client.New(client.WithHost("tcp://127.0.0.1:0"), client.WithHTTPClient(httpClient), client.WithAPIVersionNegotiation(), client.WithHTTPRequestHook(hook))
	closePools := func() { outer.CloseIdleConnections(); inner.CloseIdleConnections() }
	if err != nil {
		closePools()
		return nil, nil, err
	}
	return cli, func() { cli.Close(); closePools() }, nil
}
