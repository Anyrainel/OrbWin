package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
)

// Config snapshots are derived DATA only, never executable-cache publication.
// Readers use immutable config.json; builder metadata stays in the fixed master.
type clientConfigSnapshot struct{ path, buildx string }

type clientWriterScope string

const (
	writerRegistryAuth  clientWriterScope = "registry-entries-v2"
	writerRootUnchanged clientWriterScope = "root-unchanged-v1"
)

// The pinned CLI's login/logout fileStore.Save may change only auths. Manifest
// storage needs the master directory but has no config-root mutation authority.
// Context management is rejected at dispatch; it must never inherit auth scope.
func clientConfigMutation(args []string, config string) (clientWriterScope, bool) {
	if len(args) < 5 || args[0] != "--config" || args[1] != config || args[2] != "--host" || args[3] != endpoint {
		return "", false
	}
	switch args[4] {
	case "login", "logout":
		return writerRegistryAuth, true
	case "manifest", "trust":
		return writerRootUnchanged, true
	}
	return "", false
}

func configGeneration(data []byte) string {
	sum := sha256.Sum256(data)
	return "cfg-" + hex.EncodeToString(sum[:])
}
func clientConfigWriter(args []string, config string) bool {
	if len(args) < 5 || args[0] != "--config" || args[1] != config || args[2] != "--host" || args[3] != endpoint {
		return false
	}
	switch args[4] {
	case "login", "logout", "context", "manifest", "trust":
		return true
	}
	return false
}
func withClientConfig(args []string, old, next string) ([]string, error) {
	if len(args) < 4 || args[0] != "--config" || args[1] != old || args[2] != "--host" || args[3] != endpoint {
		return nil, errClientConfig
	}
	out := append([]string(nil), args...)
	out[1] = next
	return out, nil
}
func acquirePrivateClientSnapshot(ctx context.Context) (snapshot *clientConfigSnapshot, resultErr error) {
	guard, e := acquirePrivateClientConfig(ctx)
	if e != nil {
		return nil, e
	}
	defer func() { resultErr = errors.Join(resultErr, guard.close()) }()
	path, e := publishClientSnapshot(ctx, guard)
	if e != nil {
		return nil, e
	}
	return &clientConfigSnapshot{path: path, buildx: filepath.Join(guard.path, "buildx")}, nil
}

func clientBuildxConfig(t target) string {
	if t.buildxConfig != "" {
		return t.buildxConfig
	}
	return filepath.Join(t.config, "buildx")
}
