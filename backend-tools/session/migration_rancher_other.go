//go:build !windows

package main

import (
	"context"
	"errors"
	"io"

	"github.com/moby/moby/client"
)

func openRancherVolumeMigrationSource(context.Context, *migrationVolumeScope, migrationSourceIdentity) (*client.Client, migrationSourceIdentity, func(), error) {
	return nil, migrationSourceIdentity{}, nil, errors.New("WSL source volume transfer requires the Windows controller")
}

func migrationWSLSources(io.Writer) error {
	return errors.New("WSL source selection requires the Windows controller")
}
func reviewRancherMigration(io.Writer, string, string, string) error {
	return errors.New("WSL source inspection requires the Windows controller")
}
func exportRancherMigrationImage(io.Writer, string, string, string, string, string, string, string) error {
	return errors.New("WSL source image export requires the Windows controller")
}
