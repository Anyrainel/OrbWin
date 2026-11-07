package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"orbwin.local/session/internal/networkdiag"
)

// A fixed local observation, independent of engine state. The caller must also
// bound this process: synchronous Windows proxy APIs have no cancellation hook.
// No host/port/proxy input is accepted and no raw errors or settings are emitted.
func networkDiagnostics(ctx context.Context, output io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	loopback, loopErr := networkdiag.ObserveLoopback(ctx)
	proxy, proxyErr := networkdiag.ObserveProxy(ctx)
	data, err := json.Marshal(struct {
		Schema   int                          `json:"schema"`
		Loopback networkdiag.LoopbackSnapshot `json:"loopback"`
		Proxy    networkdiag.ProxySnapshot    `json:"proxy"`
		Complete bool                         `json:"complete"`
	}{1, loopback, proxy, loopErr == nil && proxyErr == nil && proxy.CurrentUser.State == "observed" && proxy.WinHTTPDefault.State == "observed"})
	if err != nil || len(data) > networkdiag.MaxOutput {
		return errors.New("network diagnostic output exceeds budget")
	}
	_, err = output.Write(append(data, '\n'))
	return err
}
