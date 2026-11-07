package main

import (
	"context"
	"errors"
	"strings"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/wsltrust"
)

// Source tools share one receipt protocol. A Rancher receipt additionally binds
// the exact ordinary WSL registration, name and machine. Recovery opens that
// connection anew and revalidates it; a missing/replaced registration is never
// substituted. The target's host identity remains sufficient for tool recovery.
func migrationVolumeSourceProfileValid(source migrationSourceIdentity) bool {
	switch source.Kind {
	case "docker-desktop":
		return source.EngineVersion == "29.8.0" && source.WSL == nil
	case "rancher-desktop":
		return source.EngineVersion == "29.5.3" && migrationVolumeWSLValid(source.WSL) && source.WSL.Name != ""
	default:
		return false
	}
}
func migrationVolumeWSLValid(wsl *migrationWSLIdentity) bool {
	if wsl == nil || !wsltrust.ValidRegistrationID(wsl.Registration) || len(wsl.Machine) != 32 || strings.Trim(wsl.Machine, "0123456789abcdef") != "" || !migrationText(wsl.Name, 256) {
		return false
	}
	name := strings.ToLower(wsl.Name)
	return !strings.HasPrefix(name, "docker-desktop") && !strings.HasPrefix(name, "rancher-desktop") && !strings.HasPrefix(name, "lima-")
}
func openMigrationVolumeSource(ctx context.Context, scope *migrationVolumeScope, selected migrationSourceIdentity) (*client.Client, migrationSourceIdentity, func(), error) {
	switch selected.Kind {
	case "docker-desktop":
		if selected.WSL != nil {
			break
		}
		cli, closeSource, err := newDesktopVolumeMigrationClient(scope)
		return cli, selected, closeSource, err
	case "rancher-desktop":
		if !migrationVolumeWSLValid(selected.WSL) {
			break
		}
		return openRancherVolumeMigrationSource(ctx, scope, selected)
	}
	return nil, selected, nil, errors.New("unsupported source volume connection; review the exact source first")
}
func verifyVolumeMigrationSource(ctx context.Context, cli *client.Client, target string, selected migrationSourceIdentity) (migrationSourceIdentity, error) {
	if selected.Kind == "docker-desktop" && selected.WSL == nil {
		return verifyDesktopMigrationSource(ctx, cli, target, selected.Daemon)
	}
	if selected.Kind == "rancher-desktop" && migrationVolumeWSLValid(selected.WSL) && selected.WSL.Name != "" {
		identity, err := verifyRancherMigrationSource(ctx, cli, target, selected.Daemon)
		copy := *selected.WSL
		identity.WSL = &copy
		return identity, err
	}
	return migrationSourceIdentity{}, errors.New("source volume identity is incomplete")
}
