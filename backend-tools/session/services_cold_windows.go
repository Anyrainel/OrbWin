//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"orbwin.local/session/internal/coldassets"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
	"orbwin.local/session/localnet"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/moby/moby/client"
)

// Native cold entry ownership, not an additional runtime actor or dispatcher.
// The sole future inherited-handle CLI launcher must retain this exact object
// from inventory through serving-owner handoff and positively joined shutdown.
// It is deliberately not wired into ordinary runServiceArguments/main yet.
type coldServiceEntry struct {
	mu                                  sync.Mutex
	prepared                            *localnet.ColdServicePreparation
	self                                *sdkself.Lease
	leaves                              *sdkstate.LANObservationFactory
	client                              *client.Client
	payloads                            coldassets.Payloads
	runtime                             *localnet.ColdServiceRuntime
	unknown                             bool
	control                             *sdkstate.ColdControl
	serveDone                           chan struct{}
	serveErr                            error
	closing, retirementPrepared, closed bool
	drain                               coldServiceDrain
	closedSelf                          *sdkself.JoinedLeaseClose
	joinedSource                        *sdkstate.ColdJoinedSource
}

// Keep the whole actual entry/transition/runtime/leaf graph strongly reachable
// before an admitted drain. Unknown control/join/close never removes it.
var coldServiceDrainCustody = struct {
	sync.Mutex
	owners map[*coldServiceEntry]struct{}
}{owners: map[*coldServiceEntry]struct{}{}}

func prepareColdServiceEntry(ctx context.Context, raw uintptr) (*coldServiceEntry, sdkstate.TransitionReceipt, error) {
	var empty sdkstate.TransitionReceipt
	if ctx == nil || ctx.Err() != nil || raw == 0 || raw == ^uintptr(0) {
		return nil, empty, sdkstate.ErrState
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return nil, empty, sdkstate.ErrState
	}
	// Empty developer payloads refuse before any native/cold namespace effect.
	payloads, err := coldassets.Load()
	if err != nil {
		return nil, empty, err
	}
	e := &coldServiceEntry{payloads: payloads, unknown: true}
	e.self, err = sdkself.Hold(ctx)
	if err != nil || e.self == nil {
		return e, empty, sdkstate.ErrRecoveryRequired
	}
	e.prepared, err = localnet.ConsumeColdService(ctx, raw)
	if err != nil || e.prepared == nil {
		return e, empty, sdkstate.ErrRecoveryRequired
	}
	receipt, err := e.prepared.Inventory(ctx)
	identity, identityErr := e.self.Identity()
	if err != nil || identityErr != nil || identity.ImageSHA256 == "" {
		return e, empty, sdkstate.ErrRecoveryRequired
	}
	e.control, err = e.prepared.NativeControl(ctx)
	if err != nil || e.control == nil {
		return e, empty, sdkstate.ErrRecoveryRequired
	}
	// The endpoint is the same fixed OrbWin engine endpoint as ordinary services.
	// No environment Docker context, caller home/path or hash selects any owner.
	e.client, err = client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return e, empty, sdkstate.ErrRecoveryRequired
	}
	e.unknown = false
	return e, receipt, nil
}

func runColdServiceArguments(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "local-services-cold" {
		return false, nil
	}
	if len(args) != 2 {
		return true, sdkstate.ErrState
	}
	raw, err := strconv.ParseUint(args[1], 10, strconv.IntSize)
	if err != nil || raw == 0 || uintptr(raw) == ^uintptr(0) || strconv.FormatUint(raw, 10) != args[1] {
		return true, sdkstate.ErrState
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	e, _, err := prepareColdServiceEntry(ctx, uintptr(raw))
	cancel()
	if err == nil {
		err = e.serveControl()
	}
	if e != nil && !e.closed {
		// Unknown native preparation/control/cleanup must not drop the actual
		// owners or let os.File/Pinner finalizers undo exclusion. The native
		// launcher retains this exact child/job and reports inspection-required.
		// No replay, automatic rollback or product/guest stop occurs in this hold.
		fmt.Fprintln(os.Stderr, "OrbWin cold SDK: retained owner requires inspection")
		for {
			time.Sleep(time.Minute)
			runtime.KeepAlive(e)
		}
	}
	return true, err
}

func (e *coldServiceEntry) serveControl() error {
	if e == nil || e.control == nil || e.unknown {
		return sdkstate.ErrRecoveryRequired
	}
	for sequence := uint32(1); sequence != 0; sequence++ {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		var head [4]byte
		err := e.readControl(ctx, head[:])
		size := binary.LittleEndian.Uint32(head[:])
		if err != nil || size == 0 || size > 4096 {
			cancel()
			return sdkstate.ErrRecoveryRequired
		}
		body := make([]byte, size)
		if e.readControl(ctx, body) != nil {
			cancel()
			return sdkstate.ErrRecoveryRequired
		}
		request, err := decodeColdServiceRequest(body, sequence)
		if err != nil {
			cancel()
			return err
		}
		var receipt *sdkstate.TransitionReceipt
		var controller *[4]uint64
		var leaf *sdkstate.ColdLeafOfferRecord
		stage := request.Operation
		switch request.Operation {
		case "provision-supervisor":
			err = e.prepared.ProvisionSupervisor(ctx, e.control, *request.Supervisor)
			stage = "supervisor-provisioned"
		case "adopt-leaf-master":
			e.mu.Lock()
			unavailable := e.unknown || e.runtime != nil || e.leaves != nil
			e.mu.Unlock()
			if unavailable {
				err = sdkstate.ErrRecoveryRequired
			} else {
				var factory *sdkstate.LANObservationFactory
				factory, err = e.prepared.AdoptLeafMaster(ctx, e.control, e.self, *request.Master)
				if err == nil {
					e.mu.Lock()
					e.leaves = factory
					e.mu.Unlock()
				}
			}
			stage = "leaf-master-adopted"
		case "ack-leaf":
			err = e.prepared.AcknowledgeLeaf(ctx, e.control, *request.Leaf)
			stage = "leaf-acknowledged"
		case "inventory":
			value, result := e.prepared.Inventory(ctx)
			err = result
			if result == nil {
				receipt = &value
			}
		case "start-runtime":
			var holder *localnet.ColdServiceRuntime
			holder, err = e.startRuntime(ctx, nil)
			if err == nil {
				// Same existing runtime/server loop and ORIGINAL reserved listener.
				// This ACK is start admission, never runtime-ready publication.
				e.mu.Lock()
				e.serveDone = make(chan struct{})
				e.mu.Unlock()
				go func() {
					result := holder.ServeIPC(context.Background())
					e.mu.Lock()
					e.serveErr = result
					if !e.closing {
						e.unknown = true
					}
					close(e.serveDone)
					e.mu.Unlock()
				}()
				stage = "runtime-start-admitted"
			}
		case "commit":
			err = e.prepared.Commit(ctx)
			stage = "committed-parent-retained"
			stage, err = coldServiceAdmissionStage(request.Operation, stage, err)
		case "offer-controller":
			value, result := e.prepared.OfferSupervisor(ctx)
			err = result
			if err == nil {
				controller = &value
			}
			stage, err = coldServiceAdmissionStage(request.Operation, stage, err)
		case "retain-controller":
			err = e.prepared.AcceptSupervisor(ctx)
			stage = "supervisor-accepted"
		case "seal-runtime":
			err = e.beginRetire(ctx)
			stage = "runtime-draining"
		case "drain-runtime":
			var joined bool
			joined, err = e.drain.poll()
			stage = "runtime-draining"
			if joined {
				err = e.publishJoinedSource(ctx)
				if err == nil {
					stage = "runtime-joined"
				}
			}
		case "retire":
			// Every original control Read/Write checks the actual native owner.
			err = e.drain.requireJoined()
			if err == nil {
				err = e.checkJoinedSource(ctx)
			}
			stage = "retirement-admitted"
		case "keepalive": // no state, guest or engine operation
		}
		if err == nil && (request.Operation == "keepalive" || request.Operation == "drain-runtime") {
			leaf, err = e.prepared.NextLeafOffer(ctx, e.control)
		}
		reply, marshalErr := json.Marshal(struct {
			Schema, Sequence uint32
			Stage            string
			Receipt          *sdkstate.TransitionReceipt
			Controller       *[4]uint64                    `json:",omitempty"`
			Leaf             *sdkstate.ColdLeafOfferRecord `json:",omitempty"`
			Error            string
		}{1, sequence, stage, receipt, controller, leaf, coldControlError(err)})
		if marshalErr != nil || len(reply) > 2<<20 {
			cancel()
			return sdkstate.ErrRecoveryRequired
		}
		binary.LittleEndian.PutUint32(head[:], uint32(len(reply)))
		if e.writeControl(ctx, head[:]) != nil || e.writeControl(ctx, reply) != nil {
			cancel()
			return sdkstate.ErrRecoveryRequired
		}
		if err == nil && request.Operation == "retire" {
			err = e.finishRetire(ctx)
			cancel()
			return err
		}
		cancel()
		if err != nil {
			return err
		}
	}
	return sdkstate.ErrRecoveryRequired
}

func coldControlError(err error) string {
	if err != nil {
		return "cold SDK transition refused; original owner retained"
	}
	return ""
}
func (e *coldServiceEntry) readControl(ctx context.Context, body []byte) error {
	for len(body) > 0 {
		n, err := e.control.Read(ctx, body)
		if err != nil || n <= 0 || n > len(body) {
			return sdkstate.ErrRecoveryRequired
		}
		body = body[n:]
	}
	return nil
}
func (e *coldServiceEntry) writeControl(ctx context.Context, body []byte) error {
	for len(body) > 0 {
		chunk := body
		if len(chunk) > 64<<10 {
			chunk = chunk[:64<<10]
		}
		n, err := e.control.Write(ctx, chunk)
		if err != nil || n <= 0 || n > len(body) {
			return sdkstate.ErrRecoveryRequired
		}
		body = body[n:]
	}
	return nil
}

func (e *coldServiceEntry) startRuntime(ctx context.Context, adapters *localnet.LANAdapters) (*localnet.ColdServiceRuntime, error) {
	if e == nil || ctx == nil || ctx.Err() != nil {
		return nil, sdkstate.ErrState
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.unknown || e.prepared == nil || e.self == nil || e.client == nil || e.runtime != nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	if e.leaves == nil || e.leaves.SDKBacking() != e.self || e.leaves.CheckNativeAdoption(ctx) != nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	receipt, err := e.prepared.Inventory(ctx)
	identity, identityErr := e.self.Identity()
	if err != nil || identityErr != nil {
		return nil, sdkstate.ErrRecoveryRequired
	}
	e.unknown = true
	e.runtime, err = e.prepared.StartRuntime(ctx, localnet.RuntimeOptions{Client: e.client, Installation: receipt.Scope.Installation, ToolsAsset: e.payloads.Tools, ToolsSHA256: e.payloads.ToolsSHA256, BinarySHA256: identity.ImageSHA256, SelfIdentity: &identity, WSLTrustAsset: e.payloads.Trust, WSLTrustSHA256: e.payloads.TrustSHA256, LAN: adapters, NativeLAN: true}, e.leaves)
	if err != nil || e.runtime == nil {
		return e.runtime, sdkstate.ErrRecoveryRequired
	}
	e.unknown = false
	return e.runtime, nil
}

// Final closure reuses the exact existing actor/IPC join. The entry's client
// and extra self-image lease close BEFORE the transition releases predecessor
// exclusion. Unknown sub-close is terminal and retains the original graph.
func (e *coldServiceEntry) beginRetire(ctx context.Context) error {
	if e == nil || ctx == nil || ctx.Err() != nil {
		return sdkstate.ErrState
	}
	e.mu.Lock()
	if e.unknown || e.closing || e.runtime == nil || e.leaves == nil || e.self == nil || e.leaves.SDKBacking() != e.self {
		e.mu.Unlock()
		return sdkstate.ErrRecoveryRequired
	}
	leaves := e.leaves
	e.mu.Unlock()
	coldServiceDrainCustody.Lock()
	coldServiceDrainCustody.owners[e] = struct{}{}
	coldServiceDrainCustody.Unlock()
	return e.drain.begin(ctx, leaves.SealAdmissions, e.prepareRetire)
}

// Join before the admission reply, without holding e.mu across the actual IPC
// server's return (that server publishes its joined result under this mutex).
func (e *coldServiceEntry) prepareRetire(ctx context.Context) error {
	if e == nil || ctx == nil || ctx.Err() != nil {
		return sdkstate.ErrState
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return sdkstate.ErrState
	}
	e.mu.Lock()
	if e.unknown || e.closing || e.runtime == nil || e.client == nil || e.self == nil || e.serveDone == nil {
		e.mu.Unlock()
		return sdkstate.ErrRecoveryRequired
	}
	e.closing = true
	holder, done := e.runtime, e.serveDone
	e.mu.Unlock()
	if holder.Join(ctx) != nil {
		return sdkstate.ErrRecoveryRequired
	}
	select {
	case <-done:
	case <-ctx.Done():
		return sdkstate.ErrRecoveryRequired
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.serveErr != nil || e.client.Close() != nil {
		e.unknown = true
		return sdkstate.ErrRecoveryRequired
	}
	var err error
	e.closedSelf, err = e.self.CloseForJoinedSource()
	if err != nil || e.closedSelf == nil || ctx.Err() != nil {
		e.unknown = true
		return sdkstate.ErrRecoveryRequired
	}
	e.retirementPrepared = true
	return nil
}

// The admission reply precedes closing its actual control FILE_OBJECT. Exit
// zero, observed through the retained process, is the later positive result.
func (e *coldServiceEntry) finishRetire(ctx context.Context) error {
	if e.checkJoinedSource(ctx) != nil {
		return sdkstate.ErrRecoveryRequired
	}
	e.mu.Lock()
	if !e.retirementPrepared || !e.closing || e.unknown || e.closed {
		e.mu.Unlock()
		return sdkstate.ErrRecoveryRequired
	}
	e.unknown = true
	holder := e.runtime
	e.mu.Unlock()
	if holder.Close(ctx) != nil {
		return sdkstate.ErrRecoveryRequired
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	clear(e.payloads.Tools)
	clear(e.payloads.Trust)
	e.closed, e.unknown = true, false
	coldServiceDrainCustody.Lock()
	delete(coldServiceDrainCustody.owners, e)
	coldServiceDrainCustody.Unlock()
	return nil
}

// Called only by the ORIGINAL control dispatcher after its actual drain join.
// It publishes a real lower participant after checked extra-self closure, not
// the stage/count callback. The ordinary control frame order remains unchanged.
func (e *coldServiceEntry) publishJoinedSource(ctx context.Context) error {
	if e == nil {
		return sdkstate.ErrRecoveryRequired
	}
	e.mu.Lock()
	if e.unknown || !e.retirementPrepared || !e.closing || e.closedSelf == nil || e.prepared == nil {
		e.mu.Unlock()
		return sdkstate.ErrRecoveryRequired
	}
	p, control, closed := e.prepared, e.control, e.closedSelf
	e.mu.Unlock()
	source, err := p.JoinedRuntimeSource(ctx, control, closed)
	if err != nil || source == nil {
		return sdkstate.ErrRecoveryRequired
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.unknown || e.prepared != p || e.control != control || e.closedSelf != closed || e.joinedSource != nil && e.joinedSource != source {
		return sdkstate.ErrRecoveryRequired
	}
	e.joinedSource = source
	return nil
}

func (e *coldServiceEntry) checkJoinedSource(ctx context.Context) error {
	if e == nil {
		return sdkstate.ErrRecoveryRequired
	}
	e.mu.Lock()
	p, control, source := e.prepared, e.control, e.joinedSource
	known := !e.unknown && e.retirementPrepared && e.closing
	e.mu.Unlock()
	if !known || p == nil || source == nil {
		return sdkstate.ErrRecoveryRequired
	}
	return p.CheckJoinedRuntimeSource(ctx, control, source)
}
