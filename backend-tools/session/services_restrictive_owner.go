package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
	"orbwin.local/session/localnet"
)

// The actual producer stack keeps this original graph alive on any uncertain
// settlement/panic; there is no asynchronous retry or suffix cleanup.
type serviceStartupOwners struct {
	self    *sdkself.Lease
	root    *sdkstate.Root
	client  *client.Client
	ledger  *localnet.NamespaceLedger
	runtime *localnet.ServiceRuntime
}

func retainServiceStartup(o *serviceStartupOwners) {
	fmt.Fprintln(os.Stderr, "Local-services cleanup could not be confirmed; further setup is blocked.")
	for {
		time.Sleep(time.Minute)
		runtime.KeepAlive(o)
	}
}
