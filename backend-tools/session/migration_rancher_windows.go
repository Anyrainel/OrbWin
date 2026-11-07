//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/wsltrust"
)

func openRancherVolumeMigrationSource(ctx context.Context, scope *migrationVolumeScope, selected migrationSourceIdentity) (*client.Client, migrationSourceIdentity, func(), error) {
	cli, session, closeSource, err := rancherMigrationClient(ctx, selected.WSL.Registration, selected.WSL.Machine, func(base http.RoundTripper) http.RoundTripper { return migrationVolumeTransport{base, scope} }, scope.allow)
	if err != nil {
		return nil, selected, nil, err
	}
	wsl := session.identity()
	if selected.WSL.Name != "" && selected.WSL.Name != wsl.Name {
		closeSource()
		return nil, selected, nil, errors.New("source WSL registration name changed; receipt retained")
	}
	selected.WSL = &wsl
	return cli, selected, closeSource, nil
}

func migrationWSLSources(output io.Writer) error {
	registrations, err := wsltrust.Registrations()
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"version": 1, "registrations": registrations, "message": "Listing does not start distributions. Explicit source inspection uses Linux root and may start the selected ordinary Ubuntu 24.04 distribution; no container engine is started."})
}
func reviewRancherMigration(output io.Writer, target, registration, machine string) error {
	owned, _, cancel, err := transferClient(target)
	if err != nil {
		return err
	}
	defer cancel()
	defer owned.Close()
	ctx, cancelSource := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelSource()
	cli, session, closeSource, err := newRancherMigrationClient(ctx, registration, machine)
	if err != nil {
		return err
	}
	defer closeSource()
	result, err := inspectMigrationSource(ctx, cli, target, func(expected string) (migrationSourceIdentity, error) {
		return verifyRancherMigrationSource(ctx, cli, target, expected)
	})
	if err != nil {
		return err
	}
	wsl := session.identity()
	result.Source.WSL = &wsl
	return json.NewEncoder(output).Encode(result)
}
func exportRancherMigrationImage(output io.Writer, target, source, registration, machine, image, reference, destination string) error {
	if source == "" || source == target || machine == "" {
		return errors.New("review the distinct Rancher source and selected WSL machine first")
	}
	if err := validateImageExport(image, reference, destination); err != nil {
		return err
	}
	owned, _, cancel, err := transferClient(target)
	if err != nil {
		return err
	}
	defer cancel()
	defer owned.Close()
	ctx, cancelSource := context.WithTimeout(context.Background(), 1800*time.Second)
	defer cancelSource()
	cli, session, closeSource, err := newRancherMigrationClient(ctx, registration, machine)
	if err != nil {
		return err
	}
	defer closeSource()
	var identity migrationSourceIdentity
	guard := func() error {
		var err error
		identity, err = verifyRancherMigrationSource(ctx, cli, target, source)
		wsl := session.identity()
		identity.WSL = &wsl
		return err
	}
	if err := guard(); err != nil {
		return err
	}
	return journalMigrationImage(ctx, output, identity, target, image, reference, destination, func(w io.Writer) error {
		return exportClientImage(ctx, cli, w, image, reference, destination, guard)
	})
}
