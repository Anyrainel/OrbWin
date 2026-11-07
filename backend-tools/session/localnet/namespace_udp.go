package localnet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"orbwin.local/session/internal/landatagram"
)

// DatagramConnection exposes packets, never a UDP-as-byte-stream abstraction.
type DatagramConnection = landatagram.Connection

const maxDatagramHelpers = 4096

// reserveDatagram shares the existing manager lifetime/mutex. An uncertain
// cleanup retains its slot; no global map or second namespace owner exists.
func (m *NamespaceManager) reserveDatagram(h NamespaceHandle) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.active[namespaceKey(h.Intent)] != h || !fullID(h.ID) {
		return nil, errors.New("UDP helper is not actively owned")
	}
	if err := m.streamFailure(h.ID); err != nil {
		return nil, err
	}
	if m.datagramSlots == nil {
		m.datagramSlots = make(map[string]int)
	}
	if _, tracked := m.datagramSlots[h.ID]; !tracked && len(m.datagramSlots) >= maxDatagramHelpers {
		return nil, errors.New("retained UDP helper history exhausted; settle exact absent helpers")
	}
	if m.datagramSlots[h.ID] >= landatagram.MaxPeers {
		return nil, errors.New("namespace UDP association capacity exhausted")
	}
	m.datagramSlots[h.ID]++
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if count, exists := m.datagramSlots[h.ID]; exists && count > 0 {
				if count == 1 {
					delete(m.datagramSlots, h.ID)
				} else {
					m.datagramSlots[h.ID] = count - 1
				}
			}
		})
	}, nil
}

func (m *NamespaceManager) finishDatagram(h NamespaceHandle, release func(), err error) error {
	if err == nil {
		release()
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.datagramSlots[h.ID] == 0 {
		// Exact physical-absence settlement already fenced this immutable ID.
		// A late callback must not recreate capacity or poison for a retired helper.
		return nil
	}
	m.poisonMu.Lock()
	defer m.poisonMu.Unlock()
	if m.poisoned == nil {
		m.poisoned = make(map[string]error)
	}
	m.poisoned[h.ID] = fmt.Errorf("UDP revocation unconfirmed; namespace recovery required: %w", err)
	return err
}

// SettleDatagramAbsence only releases this manager's retained UDP capacity after
// read-only confirmation of an exact immutable helper ID's physical absence.
// The namespace owner still owns its ledger/reference settlement; no Delete,
// name search, poison clearing or workload lifecycle action occurs here.
func (m *NamespaceManager) SettleDatagramAbsence(ctx context.Context, helperID string) error {
	if !fullID(helperID) {
		return errors.New("exact UDP helper ID required")
	}
	m.mu.Lock()
	tracked := m.datagramSlots[helperID] > 0
	m.mu.Unlock()
	if !tracked {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := m.verifyDaemon(ctx); err != nil {
		return err
	}
	_, err := m.cli.ContainerInspect(ctx, helperID, client.ContainerInspectOptions{})
	if !cerrdefs.IsNotFound(err) {
		if err != nil {
			return err
		}
		return errors.New("UDP helper still exists; retained capacity cannot settle")
	}
	if err = m.verifyDaemon(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settleDatagramAbsenceLocked(helperID)
}

// Caller MUST hold m.mu and have just confirmed same-daemon exact immutable ID
// absence. The owning namespace ledger must be settled first when used from its
// physical settlement path. This only drops UDP capacity, never ownership truth.
func (m *NamespaceManager) settleDatagramAbsenceLocked(helperID string) error {
	if !fullID(helperID) {
		return errors.New("exact UDP helper ID required")
	}
	delete(m.datagramSlots, helperID)
	return nil
}

// DialDatagram admits a single peer association through the SAME manager as
// TCP. The worker still owns per-packet CIDR checks and its per-lease peer cap.
// ctx controls setup only; the returned association owns Close/revocation.
func (m *NamespaceManager) DialDatagram(ctx context.Context, r ServiceRoute) (DatagramConnection, error) {
	m.mu.Lock()
	h, exists := m.active[namespaceRouteKey(r)]
	if m.closed || !exists || h.ID != r.HelperID || r.Port == 0 {
		m.mu.Unlock()
		return nil, errors.New("UDP route has no active owned namespace")
	}
	m.renewing.Add(1)
	m.mu.Unlock()
	defer m.renewing.Done()
	if err := m.streamFailure(h.ID); err != nil {
		return nil, err
	}
	release, err := m.reserveDatagram(h)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	if err = m.verifyDaemon(ctx); err == nil {
		err = VerifyServiceTarget(ctx, m.cli, h.Intent)
	}
	if err == nil {
		err = VerifyNamespace(ctx, m.cli, h.Intent, h.ID)
	}
	if err != nil {
		release()
		return nil, err
	}
	cleanup := func(cleanCtx context.Context, token, execID string) error {
		err := m.verifyDaemon(cleanCtx)
		if err == nil {
			err = VerifyNamespaceOwnership(cleanCtx, m.cli, h.Intent, h.ID)
			if cerrdefs.IsNotFound(err) {
				if err = m.verifyDaemon(cleanCtx); err != nil {
					return m.finishDatagram(h, release, err)
				}
				release()
				return nil
			}
		}
		if err == nil {
			err = revokeNamespaceStream(cleanCtx, m.cli, h.ID, token)
		}
		if err == nil {
			err = waitDatagramExit(cleanCtx, m.cli, h.ID, execID)
		}
		if err == nil {
			err = m.verifyDaemon(cleanCtx)
		}
		return m.finishDatagram(h, release, err)
	}
	c, err := dialNamespaceDatagram(ctx, m.cli, h.ID, r.Port, cleanup, release)
	if err != nil {
		return nil, err
	}
	if err = m.verifyDaemon(ctx); err == nil {
		err = VerifyServiceTarget(ctx, m.cli, h.Intent)
	}
	if err == nil {
		err = VerifyNamespace(ctx, m.cli, h.Intent, h.ID)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = m.ctx.Err()
	}
	if err == nil {
		err = m.streamFailure(h.ID)
	}
	if err != nil {
		return nil, errors.Join(err, c.Close())
	}
	return c, nil
}

func datagramCommand(helper string, port uint16, token string) ([]string, error) {
	raw, err := hex.DecodeString(token)
	if !fullID(helper) || port == 0 || err != nil || len(raw) != 16 || hex.EncodeToString(raw) != token {
		return nil, errors.New("exact UDP helper, port and fresh lowercase token required")
	}
	return []string{"/orbwin-services", "dial-udp", strconv.Itoa(int(port)), token}, nil
}
func dialNamespaceDatagram(ctx context.Context, cli *client.Client, helper string, port uint16, cleanup func(context.Context, string, string) error, release func()) (DatagramConnection, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		release()
		return nil, err
	}
	token := hex.EncodeToString(nonce[:])
	command, err := datagramCommand(helper, port, token)
	if err != nil {
		release()
		return nil, err
	}
	execID := ""
	revoke := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return cleanup(ctx, token, execID)
	}
	// Once Create is dispatched, even an error can mean an accepted late exec.
	// Fence the token AND observe the exact stopped exec before freeing capacity.
	// Unknown exec identity retains capacity even when the token was fenced.
	created, err := cli.ExecCreate(ctx, helper, client.ExecCreateOptions{Cmd: command, AttachStdin: true, AttachStdout: true, AttachStderr: true})
	if err != nil {
		return nil, errors.Join(err, revoke())
	}
	if !fullID(created.ID) {
		return nil, errors.Join(errors.New("exact UDP exec identity unavailable"), revoke())
	}
	execID = created.ID
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return nil, errors.Join(err, revoke())
	}
	stream := &execConnection{Conn: attached.Conn, reader: attached.Reader, closeWrite: attached.CloseWrite, revoke: revoke}
	return landatagram.New(stream), nil
}

// A control acknowledgement is not process exit. Share the caller's one cleanup
// deadline; require the exact exec identity and helper before freeing its slot.
func waitDatagramExit(ctx context.Context, cli *client.Client, helper, execID string) error {
	if !fullID(helper) || !fullID(execID) {
		return errors.New("UDP exec outcome unknown; retain namespace capacity")
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("bounded UDP cleanup deadline required")
	}
	for {
		state, err := cli.ExecInspect(ctx, execID, client.ExecInspectOptions{})
		if err != nil {
			return err
		}
		if state.ID != execID || state.ContainerID != helper {
			return errors.New("UDP exec identity changed; retain namespace capacity")
		}
		if !state.Running {
			return nil
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
