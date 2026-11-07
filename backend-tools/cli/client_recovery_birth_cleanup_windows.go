//go:build windows

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// Birth-last completion is distinct from restoring a lost original. Once its
// exact old records are finalized, a new ordinary managed-only transaction owns
// any selector update. No old restore hash, original bytes, or no-birth grant is
// invented, and credential operations are never started.
func prepareBirthCleanupRefresh(ctx context.Context, g *clientConfigGuard, n *nativeClientConfig, birthFile *configData, birthBytes []byte, birth writerBirthRecord, prepareFile *configData, prepareBytes []byte) (*clientWriterRecovery, error) {
	profile, _, e := nativeLocations()
	if e != nil {
		return nil, e
	}
	current := n.config
	body := append([]byte(nil), g.body...)
	selector, e := ownedManagedSelector(body, profile)
	if e != nil || objectDataStrict(current) != nil {
		return nil, errClientConfigRecovery
	}
	target, e := recoveryTarget(body, g.plugins, profile)
	if e != nil {
		return nil, e
	}
	check := func() error {
		if ctx.Err() != nil || n.checkOrigin() != nil || objectDataStrict(current) != nil {
			return errClientConfigRecovery
		}
		actual, e := current.read(clientConfigBudget)
		if e != nil || !bytes.Equal(actual, body) {
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
		if _, e = existingCache("credentials"); e != nil {
			return errClientConfigRecovery
		}
		if owned, err := ownedManagedSelector(actual, profile); err != nil || owned != selector {
			return errClientConfigRecovery
		}
		actual, e = birthFile.read(8192)
		if e != nil || !bytes.Equal(actual, birthBytes) || n.stoppedWriterRoot(birth) != nil {
			return errClientConfigRecovery
		}
		if prepareFile != nil {
			actual, e = prepareFile.read(4096)
			if e != nil || !bytes.Equal(actual, prepareBytes) {
				return errClientConfigRecovery
			}
			p, e := parseWriterPrepare(actual)
			if e != nil {
				return errClientConfigRecovery
			}
			i, e := parseWriterIntent([]byte(birth.IntentBody))
			if e != nil || !p.matches(i) {
				return errClientConfigRecovery
			}
		}
		for _, name := range []string{"writer.intent", "writer.original", "recovery.intent", "recovery.next", "snapshot.intent"} {
			if n.birthCleanupAbsent(n.parent(), name) != nil {
				return errClientConfigRecovery
			}
		}
		if n.birthCleanupAbsent(n.directory(), "config.stage") != nil {
			return errClientConfigRecovery
		}
		return nil
	}
	if check() != nil {
		return nil, errClientConfigRecovery
	}
	n.birth, n.prepare = birthFile, prepareFile
	review := &clientWriterRecovery{deadline: time.Now().Add(5 * time.Minute), finishOnly: true, closeNative: n.releaseRecoveryOwner}
	review.restoreNative = func() (*clientConfigGuard, error) {
		e := completedWriterRefresh(check, func() error {
			if prepareFile != nil && n.disposeExactRecord("writer.prepare", prepareFile) != nil {
				return n.successorFence()
			}
			n.prepare = nil
			if n.disposeExactRecord("writer.birth", birthFile) != nil {
				return n.successorFence()
			}
			n.birth = nil
			n.poison = false
			return nil
		}, func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !bytes.Equal(target, body) {
				if n.replaceManagedConfig(target) != nil {
					return errClientConfigRecovery
				}
			}
			actual, e := n.config.read(clientConfigBudget)
			if e != nil || !bytes.Equal(actual, target) || privateClientRouting(actual, g.plugins) != nil {
				return errClientConfigRecovery
			}
			g.body = actual
			return nil
		})
		if e != nil {
			return nil, e
		}
		return g, nil
	}
	return review, nil
}
func (n *nativeClientConfig) birthCleanupAbsent(parent windows.Handle, name string) error {
	h, e := configOpen(parent, name, windows.FILE_OPEN, false, false, false, nil)
	if freshMissing(e) {
		return nil
	}
	if e == nil {
		d, x := n.hold(h, false)
		if x != nil {
			return errClientConfigRecovery
		}
		if d.file.Close() != nil {
			return n.successorFence()
		}
		d.file = nil
	}
	return errClientConfigRecovery
}
