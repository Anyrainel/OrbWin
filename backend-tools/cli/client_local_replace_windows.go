//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

// Caller retains the actual exclusive controller and native init/root chain.
// This provider accepts only the full local-replace producer record, never an
// EOF fragment or any writer/initialization/import journal. Cancel/close retain
// all bytes; an apply error preserves the exact remaining records. No child,
// writer settlement or execution-quiescence certificate is created here.
func prepareLocalReplaceReview(ctx context.Context, g *clientConfigGuard, intent *configData, encoded []byte, record localReplaceIntent, nextSHA string) (*clientWriterRecovery, error) {
	n := g.native.(*nativeClientConfig)
	if ctx == nil || ctx.Err() != nil || n.checkOrigin() != nil || objectDataStrict(n.config) != nil || intent == nil || intent.check() != nil {
		return nil, errClientConfigRecovery
	}
	for _, name := range []string{"writer.prepare", "writer.original", "writer.birth", "recovery.intent", "recovery.next", "snapshot.intent"} {
		if snapshotAbsent(n.parent(), name, false) != nil {
			return nil, errClientConfigRecovery
		}
	}
	current := n.config
	currentBytes := append([]byte(nil), g.body...)
	if !localReplaceCurrentMatches(record, nextSHA, digestBytes(currentBytes)) {
		return nil, errClientConfigRecovery
	}
	profile, _, e := nativeLocations()
	if e != nil {
		return nil, e
	}
	next, changed, e := refreshPrivateManagedField(currentBytes, g.plugins, profile)
	if e != nil {
		return nil, e
	}
	validated := currentBytes
	if changed {
		validated = next
	}
	if privateClientRouting(validated, g.plugins) != nil {
		return nil, errClientConfigRecovery
	}
	var stage *configData
	var stageBytes []byte
	h, e := configOpen(n.directory(), "config.stage", windows.FILE_OPEN, false, true, false, nil)
	if e == nil {
		stage, e = n.hold(h, false)
		if e != nil {
			return nil, e
		}
		stageBytes, e = stage.read(clientConfigBudget)
		if e != nil {
			return nil, e
		}
	} else if !errors.Is(e, windows.STATUS_OBJECT_NAME_NOT_FOUND) {
		return nil, errClientConfigRecovery
	}
	checkCurrent := func() error {
		if ctx.Err() != nil || n.checkOrigin() != nil || objectDataStrict(current) != nil {
			return errClientConfigRecovery
		}
		actual, e := current.read(clientConfigBudget)
		if e != nil || !bytes.Equal(actual, currentBytes) {
			return errClientConfigRecovery
		}
		actual, e = intent.read(4096)
		if e != nil || !bytes.Equal(actual, encoded) {
			return errClientConfigRecovery
		}
		for _, name := range []string{"writer.prepare", "writer.original", "writer.birth", "recovery.intent", "recovery.next", "snapshot.intent"} {
			if snapshotAbsent(n.parent(), name, false) != nil {
				return errClientConfigRecovery
			}
		}
		return nil
	}
	if checkCurrent() != nil {
		return nil, errClientConfigRecovery
	}
	n.intent, n.intentName, n.poison = intent, "writer.intent", true
	review := &clientWriterRecovery{deadline: time.Now().Add(5 * time.Minute), localReplace: true, closeNative: n.releaseRecoveryOwner}
	review.restoreNative = func() (*clientConfigGuard, error) {
		if checkCurrent() != nil {
			return nil, errClientConfigRecovery
		}
		if stage != nil {
			actual, e := stage.read(clientConfigBudget)
			if e != nil || !bytes.Equal(actual, stageBytes) {
				return nil, errClientConfigRecovery
			}
			if e := n.disposeOwnedFile(n.directory(), "config.stage", stage); e != nil {
				return nil, e
			}
		}
		// The old local transaction ends only after the retained current file is
		// rechecked and its exact unpublished stage is absent. Intent is last.
		if checkCurrent() != nil || snapshotAbsent(n.directory(), "config.stage", false) != nil || n.disposeExactRecord("writer.intent", intent) != nil {
			return nil, errClientConfigRecovery
		}
		n.intent, n.poison = nil, false
		// Refresh is a separate durable local transaction under this SAME
		// controller. It may fail with its own retained intent, never replay auth.
		if changed && n.replaceManagedConfig(next) != nil {
			return nil, errClientConfigRecovery
		}
		if n.check() != nil || objectDataStrict(n.config) != nil {
			return nil, errClientConfigRecovery
		}
		body, e := n.config.read(clientConfigBudget)
		if e != nil || !bytes.Equal(body, validated) || privateClientRouting(body, g.plugins) != nil {
			return nil, errClientConfigRecovery
		}
		g.body = append([]byte(nil), body...)
		return g, nil
	}
	return review, nil
}
