//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// Fixed-name absence only. Existing objects are retained and checked-closed;
// failure never becomes absence, nor grants permission to remove that object.
func (n *nativeClientConfig) localEOFAbsent(parent windows.Handle, name string) error {
	h, e := configOpen(parent, name, windows.FILE_OPEN, false, false, false, nil)
	if errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
		return nil
	}
	if e == nil {
		d, captureErr := n.hold(h, false)
		if captureErr != nil {
			return errClientConfigRecovery
		}
		if d.file.Close() != nil {
			return n.successorFence()
		}
		d.file = nil
	}
	return errClientConfigRecovery
}

func prepareLocalEOFReview(ctx context.Context, g *clientConfigGuard, intent *configData, encoded []byte) (*clientWriterRecovery, error) {
	n := g.native.(*nativeClientConfig)
	if ctx == nil || ctx.Err() != nil || intent == nil || n.config == nil || len(n.ancestors) < 3 {
		return nil, errClientConfigRecovery
	}
	profile, _, e := nativeLocations()
	if e != nil {
		return nil, e
	}
	current := n.config
	currentBytes := append([]byte(nil), g.body...)
	privateParent := windows.Handle(n.ancestors[len(n.ancestors)-3].file.Fd())
	check := func() error {
		if ctx.Err() != nil || n.checkOrigin() != nil || objectDataStrict(current) != nil || intent.check() != nil {
			return errClientConfigRecovery
		}
		actual, e := current.read(clientConfigBudget)
		if e != nil || !bytes.Equal(actual, currentBytes) {
			return errClientConfigRecovery
		}
		actual, e = intent.read(4096)
		if e != nil || !bytes.Equal(actual, encoded) || !localReplaceEOFPrefix(actual, digestBytes(currentBytes)) {
			return errClientConfigRecovery
		}
		p, _, e := nativeLocations()
		if e != nil || p != profile {
			return errClientConfigRecovery
		}
		docker, e := existingCache("docker")
		if e != nil || filepath.Join(filepath.Dir(docker), "docker-cli-plugins") != g.plugins {
			return errClientConfigRecovery
		}
		if _, e := existingCache("credentials"); e != nil {
			return errClientConfigRecovery
		}
		for _, name := range []string{"writer.prepare", "writer.original", "writer.birth", "recovery.intent", "recovery.next", "snapshot.intent"} {
			if n.localEOFAbsent(n.parent(), name) != nil {
				return errClientConfigRecovery
			}
		}
		// These are siblings of client, NOT children of the client directory.
		// No import producer exists yet: this second fixed fence is conservative
		// future compatibility and must be reconciled with its adopted protocol.
		for _, name := range []string{"client.create-intent", "client.import-intent"} {
			if n.localEOFAbsent(privateParent, name) != nil {
				return errClientConfigRecovery
			}
		}
		return n.localEOFAbsent(n.directory(), "config.stage")
	}
	plan, e := prepareLocalEOFPlan(encoded, currentBytes, profile, g.plugins, objectDataStrict(current) == nil, check)
	if e != nil {
		return nil, e
	}
	n.intent, n.intentName, n.poison = intent, "writer.intent", true
	review := &clientWriterRecovery{deadline: time.Now().Add(5 * time.Minute), localReplace: true, closeNative: n.releaseRecoveryOwner}
	review.restoreNative = func() (*clientConfigGuard, error) {
		e := applyLocalEOF(check, func() error {
			if n.disposeExactRecord("writer.intent", intent) != nil {
				return n.successorFence()
			}
			n.intent, n.poison = nil, false
			return nil
		}, func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if plan.changed && n.replaceManagedConfig(plan.target) != nil {
				return n.successorFence()
			}
			if n.check() != nil || objectDataStrict(n.config) != nil {
				return errClientConfigRecovery
			}
			actual, e := n.config.read(clientConfigBudget)
			if e != nil || !bytes.Equal(actual, plan.target) || privateClientRouting(actual, g.plugins) != nil {
				return errClientConfigRecovery
			}
			g.body = append([]byte(nil), actual...)
			return nil
		})
		if e != nil {
			return nil, e
		}
		return g, nil
	}
	return review, nil
}
