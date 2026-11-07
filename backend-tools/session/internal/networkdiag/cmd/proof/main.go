// A dedicated read-only qualification entry point, not the shared OrbWin SDK.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"orbwin.local/session/internal/networkdiag"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "network diagnostic proof accepts no arguments")
		os.Exit(125)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	loopback, loopErr := networkdiag.ObserveLoopback(ctx)
	proxy, proxyErr := networkdiag.ObserveProxy(ctx)
	// Errors are represented by bounded enums, never raw syscall/DNS error text.
	data, err := json.Marshal(struct {
		Loopback networkdiag.LoopbackSnapshot `json:"loopback"`
		Proxy    networkdiag.ProxySnapshot    `json:"proxy"`
		Complete bool                         `json:"complete"`
	}{loopback, proxy, loopErr == nil && proxyErr == nil && proxy.CurrentUser.State == "observed" && proxy.WinHTTPDefault.State == "observed"})
	if err != nil || len(data) > networkdiag.MaxOutput {
		fmt.Fprintln(os.Stderr, "network diagnostic output exceeds budget")
		os.Exit(125)
	}
	os.Stdout.Write(append(data, '\n'))
}
