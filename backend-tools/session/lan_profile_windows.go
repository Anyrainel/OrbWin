//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

// Observation only: no caller-provided adapter identity and no activation.
// These synchronous native calls require the caller's process wall-clock cap.
func lanProfiles(ctx context.Context, output io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	interfaces, err := lanobserve.Observe(ctx)
	if err != nil {
		return err
	}
	guids := make([]string, 0, len(interfaces.Adapters))
	for _, adapter := range interfaces.Adapters {
		guids = append(guids, adapter.GUID)
	}
	profiles, err := lanprofile.Observe(ctx, guids)
	if err != nil {
		return err
	} // No partial category result on failed enumeration.
	return json.NewEncoder(output).Encode(profiles)
}
