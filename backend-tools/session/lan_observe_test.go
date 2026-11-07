package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"orbwin.local/session/internal/lanobserve"
)

func TestLANInterfacesCancelledReadHasNoOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if err := lanInterfaces(ctx, &output); err == nil || output.Len() != 0 {
		t.Fatalf("cancelled observation emitted output: %v %d", err, output.Len())
	}
}

func TestLANInterfacesReadOnlyNative(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("ORBWIN_QUALIFY_LAN_OBSERVE") != "1" {
		t.Skip("opt-in read-only Windows adapter observation")
	}
	var output bytes.Buffer
	if err := lanInterfaces(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() > lanobserve.MaxOutput+1 {
		t.Fatal("native result exceeds output budget")
	}
	var snapshot lanobserve.Snapshot
	if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ActivationAvailable || snapshot.ProfileSource != "unavailable" {
		t.Fatal("native read inferred authorization/profile")
	}
	for _, adapter := range snapshot.Adapters {
		if adapter.ActivationAvailable || adapter.Profile != "unknown" {
			t.Fatal("adapter falsely qualified for activation")
		}
	}
	t.Log(string(output.Bytes()))
}
