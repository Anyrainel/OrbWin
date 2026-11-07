//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"orbwin.local/session/internal/lanobserve"
)

// lanInterfaces is the read-only command hook for main.go. It accepts no address,
// interface, engine or trust argument and never launches a privileged process.
func lanInterfaces(ctx context.Context, output io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	snapshot, err := lanobserve.Observe(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(snapshot)
}
