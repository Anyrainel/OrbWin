package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanprovider"
	"time"
)

func bootstrapHex(value string, bytes int) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == bytes && hex.EncodeToString(raw) == value
}
func runLANBootstrap(args []string, out io.Writer) (bool, error) {
	if (len(args) == 3 && (args[0] == "lan-rule-observation" || args[0] == "lan-removal-observation" || args[0] == "lan-cleanup-observation") && bootstrapHex(args[1], 16) && bootstrapHex(args[2], 32)) || (len(args) == 4 && args[0] == "lan-review-observation" && bootstrapHex(args[1], 16) && bootstrapHex(args[2], 16) && bootstrapHex(args[3], 32)) {
		stop := lanhelper.StartReadOnlyObservationCap()
		defer stop()
	}
	if handled, err := runLANCleanupBootstrapWith(args, out, lanprovider.ReadProtectedCleanup); handled {
		return handled, err
	}
	return runLANBootstrapWith(args, out, lanhelper.RunElevationLauncher, lanprovider.ReadProtectedRule, lanhelper.ReadProtectedReviewObservation, lanprovider.ReadProtectedRemoval)
}

func runLANCleanupBootstrapWith(args []string, out io.Writer, read func(context.Context, string, string) (lanprovider.CleanupObservation, error)) (bool, error) {
	if len(args) == 0 || args[0] != "lan-cleanup-observation" {
		return false, nil
	}
	bad := errors.New("closed protected cleanup observation refused")
	if len(args) != 3 || !bootstrapHex(args[1], 16) || !bootstrapHex(args[2], 32) || read == nil {
		return true, bad
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	observation, err := read(ctx, args[1], args[2])
	if err != nil || ctx.Err() != nil {
		return true, bad
	}
	data, err := json.Marshal(observation)
	if err != nil || len(data)+1 > 16<<10 || writeFull(out, append(data, '\n')) != nil {
		return true, bad
	}
	return true, nil
}
func runLANBootstrapWith(args []string, out io.Writer, launch func(context.Context, []string) error, observe func(context.Context, string, string) (lanlease.RuleAcknowledgement, error), review func(context.Context, string, string, string) (lanhelper.ProtectedReviewObservation, error), removal func(context.Context, string, string) (lanlease.RemovalAcknowledgement, error)) (bool, error) {
	if len(args) == 0 || (args[0] != "lan-elevation-launcher" && args[0] != "lan-rule-observation" && args[0] != "lan-review-observation" && args[0] != "lan-removal-observation") {
		return false, nil
	}
	bad := errors.New("closed LAN bootstrap refused")
	if args[0] == "lan-review-observation" {
		if len(args) != 4 || !bootstrapHex(args[1], 16) || !bootstrapHex(args[2], 16) || !bootstrapHex(args[3], 32) {
			return true, bad
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		observation, err := review(ctx, args[1], args[2], args[3])
		if err != nil || ctx.Err() != nil {
			return true, bad
		}
		data, err := json.Marshal(observation)
		if err != nil || len(data)+1 > 16<<10 {
			return true, bad
		}
		if writeFull(out, append(data, '\n')) != nil {
			return true, bad
		}
		return true, nil
	}
	if len(args) != 3 || !bootstrapHex(args[1], 16) {
		return true, bad
	}
	if args[0] == "lan-elevation-launcher" {
		if !bootstrapHex(args[2], 16) {
			return true, bad
		}
		if launch(context.Background(), args[1:]) != nil {
			return true, bad
		}
		return true, nil
	}
	if !bootstrapHex(args[2], 32) {
		return true, bad
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if args[0] == "lan-removal-observation" {
		if removal == nil {
			return true, bad
		}
		ack, err := removal(ctx, args[1], args[2])
		if err != nil || ctx.Err() != nil {
			return true, bad
		}
		data, err := json.Marshal(ack)
		if err != nil || len(data)+1 > 16<<10 || writeFull(out, append(data, '\n')) != nil {
			return true, bad
		}
		return true, nil
	}
	ack, err := observe(ctx, args[1], args[2])
	if err != nil || ctx.Err() != nil {
		return true, bad
	}
	data, err := json.Marshal(ack)
	if err != nil || len(data)+1 > 16<<10 {
		return true, bad
	}
	if writeFull(out, append(data, '\n')) != nil {
		return true, bad
	}
	return true, nil
}
