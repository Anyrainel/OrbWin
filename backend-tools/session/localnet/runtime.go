package localnet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"orbwin.local/session/internal/sdkstate"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanprovider"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/wsltrust"
)

type RuntimeOptions struct {
	Restriction          *RestrictiveStartup // Comparison-only startup profile; nil preserves production.
	SelfLease            *sdkself.Lease      // Borrowed actual startup lease, held through runtime/IPC settlement.
	Client               *client.Client
	Ledger               *NamespaceLedger
	Installation         string
	ToolsAsset           []byte
	ToolsSHA256          string
	BinarySHA256         string
	SelfIdentity         *sdkself.Identity
	RootStoreFactory     func(bool) (RootStore, error)
	WSLTrustAsset        []byte
	WSLTrustSHA256       string
	WSLTrustFactory      func() (WSLTrustClient, error)
	WSLRegistrations     func() ([]wsltrust.Registration, error)
	WSLRegistryFactory   func() (WSLTrustRegistry, error)
	LAN                  *LANAdapters                                                                                      // Internal native registration; nil keeps activation unavailable.
	NativeLAN            bool                                                                                              // Fixed ordinary/cold SDK entrypoints only; never an IPC/renderer authority field.
	portProbe            func(ServicePorts) error                                                                          // Controlled contract fixtures only.
	lanOptionsReader     func(context.Context) (LANOptions, error)                                                         // Internal read-only contract fixtures.
	coldLANLeaf          *sdkstate.LANObservationFactory                                                                   // Actual cold-only native holder; no ordinary factory substitution.
	lanGenerationBuilder func(context.Context, RuntimeOptions, string, chan lanOperationEvent) (*serviceGeneration, error) // Controlled late-resource fixtures.
}
type RuntimeSnapshot struct {
	Instance       string            `json:"instance"`
	Sequence       uint64            `json:"sequence"`
	DaemonID       string            `json:"daemonId"`
	Installation   string            `json:"installation"`
	BinarySHA256   string            `json:"binarySha256,omitempty"`
	SelfIdentity   *sdkself.Identity `json:"selfIdentity,omitempty"`
	ToolsSHA256    string            `json:"toolsSha256"`
	WSLTrustSHA256 string            `json:"wslTrustSha256,omitempty"`
	SettingsSnapshot
	Phase                 string                  `json:"phase"`
	Error                 string                  `json:"error,omitempty"`
	Candidates            []ServiceCandidate      `json:"candidates,omitempty"`
	Services              []ServiceStatus         `json:"services,omitempty"`
	Authority             *AuthorityInfo          `json:"authority,omitempty"`
	WindowsTrust          *WindowsTrustStatus     `json:"windowsTrust,omitempty"`
	WSLTrust              *WSLTrustStatus         `json:"wslTrust,omitempty"`
	WSLDistributions      []wsltrust.Registration `json:"wslDistributions,omitempty"`
	WSLDistributionsRead  bool                    `json:"wslDistributionsRead"`
	WSLTrustReceipts      []WSLTrustReceiptInfo   `json:"wslTrustReceipts,omitempty"`
	WSLTrustReceiptsRead  bool                    `json:"wslTrustReceiptsRead"`
	WSLTrustReceiptsError string                  `json:"wslTrustReceiptsError,omitempty"`
	Rotation              *AuthorityRotationInfo  `json:"rotation,omitempty"`
	LAN                   []LANStatus             `json:"lan,omitempty"`
	InventoryReadAt       *time.Time              `json:"inventoryReadAt,omitempty"`
	UpdatedAt             time.Time               `json:"updatedAt"`
}
type runtimeCommand struct {
	expectedServer *ExpectedServer
	kind           string
	revision       uint64
	settings       ServiceSettings
	rootSHA256     string
	newRootSHA256  string
	ports          ServicePorts
	wslReview      WSLTrustReview
	lanStage       *lanhelper.StageRequest
	lanLeaseSHA    string
	lanReviewID    string
	lanReviewSHA   string
	lanReviewInput *lanReviewRecord
	lanOperationID string
	lanAdmitted    *lanReviewRecord
	ctx            context.Context
	reply          chan runtimeReply
}
type runtimeReply struct {
	restrictedSHA     string
	deferred          bool
	settings          SettingsSnapshot
	authority         AuthorityInfo
	lan               LANStatus
	lanReconciliation *LANReconciliation
	lanReview         *LANReview
	err               error
}
type serviceGeneration struct {
	lanOnly  bool
	host     *ServiceHost
	shared   *sharedNamespaces
	lan      *lanController
	shutdown *generationShutdown
	relay    *RelayManager
	networks []RelayNetwork
	relayID  string
	started  time.Time
}

// ServiceRuntime consumes its ledger only on successful construction. One actor
// serializes durable settings and generation transitions; Snapshot remains a
// separate short read. Caller loss cannot cancel committed enabled intent.
// It owns helpers/listeners only, never engine or workload lifecycle.
type ServiceRuntime struct {
	opts              RuntimeOptions
	ctx               context.Context
	cancel            context.CancelFunc
	done              chan struct{}
	cleanupResult     chan struct{} // Cleanup returned; done separately requires positive resource release.
	commands          chan runtimeCommand
	mu                sync.RWMutex
	state             RuntimeSnapshot
	result            error
	settings          SettingsSnapshot // Actor-owned fields below.
	generation        *serviceGeneration
	attempts          int
	retryAt           time.Time
	residualChecked   bool
	settingsUncertain bool
	rotationPending   bool
	lanQueries        sync.WaitGroup
	lanQueryMu        sync.Mutex
	lanQueryActive    bool
	lanQueriesClosing bool
	lanReviews        *lanReviewOwner
	lanOperations     map[string]*lanOperation
	lanEvents         chan lanOperationEvent
	lanWork           sync.WaitGroup
	lanInstallation   string
	lanRecoveryActive bool
	lanJournal        lanReviewJournal // Explicit inert test seam; production derives from opaque ledger.
	lanJournalActive  bool
}

func NewServiceRuntime(options RuntimeOptions) (*ServiceRuntime, error) {
	if options.Client == nil || options.Ledger == nil || options.Installation == "" {
		return nil, errors.New("owned client, exclusive owner and installation required")
	}
	if options.NativeLAN {
		if err := validateNativeLANRegistration(options); err != nil {
			return nil, err
		}
	}
	if options.Restriction != nil {
		copied := *options.Restriction
		if !copied.valid() || options.SelfLease == nil {
			return nil, errRestricted
		}
		options.Restriction = &copied
	}
	if options.SelfIdentity != nil {
		copy := *options.SelfIdentity
		if !copy.Valid() || copy.ImageSHA256 != options.BinarySHA256 {
			return nil, errors.New("retained SDK identity differs from runtime image")
		}
		options.SelfIdentity = &copy
	}
	if err := validateToolsAsset(options.ToolsAsset, options.ToolsSHA256, strings.Repeat("0", 32)); err != nil {
		return nil, err
	}
	settings, err := options.Ledger.LoadSettings()
	if err != nil {
		return nil, err
	}
	if options.Restriction != nil && settings.Settings.Enabled {
		return nil, errRestricted
	}
	options.ToolsAsset = append([]byte(nil), options.ToolsAsset...)
	if options.LAN != nil {
		copied := *options.LAN
		if copied.ValidatePublication == nil {
			copied.ValidatePublication = func(ctx context.Context, stage lanhelper.StageRequest) (time.Time, error) {
				return validateLANPublication(ctx, options.Client, options.Ledger.daemon, stage.Publication)
			}
		}
		options.LAN = &copied
	}
	if len(options.WSLTrustAsset) != 0 || options.WSLTrustSHA256 != "" {
		if err := wsltrust.ValidateHelperAsset(options.WSLTrustAsset, options.WSLTrustSHA256); err != nil {
			return nil, err
		}
		options.WSLTrustAsset = append([]byte(nil), options.WSLTrustAsset...)
	}
	if options.WSLTrustFactory == nil {
		options.WSLTrustFactory = func() (WSLTrustClient, error) {
			return newWSLTrustClient(options.WSLTrustAsset, options.WSLTrustSHA256)
		}
	}
	if options.WSLRegistrations == nil {
		options.WSLRegistrations = registeredWSLTrustClients
	}
	if options.WSLRegistryFactory == nil {
		options.WSLRegistryFactory = newWSLTrustRegistry
	}
	options.Ledger.mu.Lock()
	options.Ledger.wslRegistryFactory = options.WSLRegistryFactory
	options.Ledger.mu.Unlock()
	if options.RootStoreFactory == nil {
		options.RootStoreFactory = OpenCurrentUserRootStore
	}
	var instance [16]byte
	if _, err := rand.Read(instance[:]); err != nil {
		return nil, err
	}
	runtime := &ServiceRuntime{opts: options, done: make(chan struct{}), cleanupResult: make(chan struct{}), commands: make(chan runtimeCommand, 8), settings: settings, lanReviews: newLANReviewOwner(), lanOperations: map[string]*lanOperation{}, lanEvents: make(chan lanOperationEvent, 16)}
	runtime.ctx, runtime.cancel = context.WithCancel(context.Background())
	runtime.state = RuntimeSnapshot{Instance: hex.EncodeToString(instance[:]), Sequence: 1, SettingsSnapshot: settings, Phase: "starting", UpdatedAt: time.Now().UTC(), DaemonID: options.Ledger.daemon, Installation: options.Installation, BinarySHA256: options.BinarySHA256, ToolsSHA256: options.ToolsSHA256}
	runtime.state.WSLTrustSHA256 = options.WSLTrustSHA256
	runtime.state.SelfIdentity = options.SelfIdentity
	if options.NativeLAN {
		adapters, registrationErr := newNativeLANRegistration(options)
		if registrationErr != nil {
			if adapters == nil {
				runtime.cancel()
				return nil, registrationErr
			}
			return retainFailedLANRegistration(runtime, adapters, registrationErr)
		}
		if adapters == nil {
			runtime.cancel()
			return nil, ErrLANAdapterUnavailable
		}
		// Same fixed source validation as explicitly supplied internal adapters.
		adapters.ValidatePublication = func(ctx context.Context, stage lanhelper.StageRequest) (time.Time, error) {
			return validateLANPublication(ctx, options.Client, options.Ledger.daemon, stage.Publication)
		}
		runtime.opts.LAN = adapters
	}
	if info, infoErr := options.Ledger.AuthorityInfo(options.Installation); infoErr == nil {
		runtime.state.Authority = &info
	} else if !os.IsNotExist(infoErr) {
		runtime.state.Error = infoErr.Error()
	}
	_ = runtime.refreshRotation()
	go runtime.loop()
	return runtime, nil
}

// A nonnil failed constructor result retains the exact partial native owner.
// No actor, authority refresh or second cleanup is started; the existing cold
// actual-callee witness therefore cannot classify this result as unstarted.
func retainFailedLANRegistration(runtime *ServiceRuntime, adapters *LANAdapters, failure error) (*ServiceRuntime, error) {
	runtime.opts.LAN = adapters
	runtime.result = failure
	runtime.state.Phase, runtime.state.Error = "cleanup-pending", failure.Error()
	runtime.cancel()
	close(runtime.cleanupResult)
	return runtime, failure // done remains open: Close reports error; Wait retains owners.
}
func (runtime *ServiceRuntime) Snapshot() (RuntimeSnapshot, error) {
	runtime.mu.RLock()
	encoded, err := json.Marshal(runtime.state)
	runtime.mu.RUnlock()
	if err != nil {
		return RuntimeSnapshot{}, err
	}
	var snapshot RuntimeSnapshot
	err = json.Unmarshal(encoded, &snapshot)
	return snapshot, err
}
func (runtime *ServiceRuntime) publish(phase string, cause error, candidates []ServiceCandidate, statuses []ServiceStatus) {
	lanStatuses := runtime.lanOperationStatuses()
	message := ""
	if cause != nil {
		message = cause.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
	}
	runtime.mu.Lock()
	runtime.state.SettingsSnapshot = runtime.settings
	runtime.state.Phase = phase
	runtime.state.Error = message
	if candidates != nil {
		runtime.state.Candidates = candidates
	}
	runtime.state.Services = statuses
	runtime.state.LAN = lanStatuses
	runtime.touchStateLocked()
	runtime.mu.Unlock()
}

// Sequence is the ordering authority; wall time is only display metadata. Fast
// Windows updates can share a timestamp, and wall time may move backwards.
func (runtime *ServiceRuntime) touchStateLocked() {
	runtime.state.Sequence++
	runtime.state.UpdatedAt = time.Now().UTC()
}

// Preserve the last successful inventory across disabled ticks and failed reads.
// A successful empty read is distinct from never having read the inventory.
func (runtime *ServiceRuntime) recordInventory(candidates []ServiceCandidate) {
	now := time.Now().UTC()
	runtime.mu.Lock()
	runtime.state.Candidates = candidates
	runtime.state.InventoryReadAt = &now
	runtime.touchStateLocked()
	runtime.mu.Unlock()
}
func (runtime *ServiceRuntime) request(ctx context.Context, command runtimeCommand) (runtimeReply, error) {
	if runtime.ctx.Err() != nil {
		return runtimeReply{}, errors.New("local-services process is stopping")
	}
	command.ctx = ctx
	command.reply = make(chan runtimeReply, 1)
	select {
	case <-runtime.ctx.Done():
		return runtimeReply{}, errors.New("local-services process is stopping")
	case <-runtime.done:
		return runtimeReply{}, errors.New("local-services process stopped")
	case <-ctx.Done():
		return runtimeReply{}, ctx.Err()
	case runtime.commands <- command:
	}
	select {
	case <-runtime.ctx.Done():
		return runtimeReply{}, errors.New("local-services process is stopping; reread any unconfirmed mutation")
	case reply := <-command.reply:
		return reply, reply.err
	case <-runtime.done:
		return runtimeReply{}, errors.New("local-services process stopped")
	case <-ctx.Done():
		return runtimeReply{}, ctx.Err()
	}
}
func (runtime *ServiceRuntime) Configure(ctx context.Context, revision uint64, settings ServiceSettings) (SettingsSnapshot, error) {
	// Copy before enqueue so the actor never observes subsequent caller edits.
	encoded, err := json.Marshal(settings)
	if err != nil {
		return SettingsSnapshot{}, err
	}
	var copied ServiceSettings
	if err = json.Unmarshal(encoded, &copied); err != nil {
		return SettingsSnapshot{}, err
	}
	reply, requestErr := runtime.request(ctx, runtimeCommand{kind: "configure", revision: revision, settings: copied})
	encoded, err = json.Marshal(reply.settings)
	if err != nil {
		return SettingsSnapshot{}, errors.Join(requestErr, err)
	}
	var result SettingsSnapshot
	err = json.Unmarshal(encoded, &result)
	return result, errors.Join(requestErr, err)
}
func (runtime *ServiceRuntime) CreateAuthority(ctx context.Context, revision uint64) (AuthorityInfo, error) {
	reply, err := runtime.request(ctx, runtimeCommand{kind: "authority", revision: revision})
	reply.authority.PublicCertificatePEM = append([]byte(nil), reply.authority.PublicCertificatePEM...)
	return reply.authority, err
}

// AdoptAuthority resumes interrupted authority publication only after the caller
// reviews the exact public fingerprint. It never generates or replaces a key.
func (runtime *ServiceRuntime) AdoptAuthority(ctx context.Context, revision uint64, rootSHA256 string) (AuthorityInfo, error) {
	reply, err := runtime.request(ctx, runtimeCommand{kind: "adopt-authority", revision: revision, rootSHA256: rootSHA256})
	reply.authority.PublicCertificatePEM = append([]byte(nil), reply.authority.PublicCertificatePEM...)
	return reply.authority, err
}

func (runtime *ServiceRuntime) WindowsTrust(ctx context.Context, operation string, revision uint64, root string) error {
	_, err := runtime.request(ctx, runtimeCommand{kind: operation, revision: revision, rootSHA256: root})
	return err
}

func (runtime *ServiceRuntime) SelectedWSLTrust(ctx context.Context, operation string, revision uint64, root string, review WSLTrustReview) error {
	_, err := runtime.request(ctx, runtimeCommand{kind: operation, revision: revision, rootSHA256: root, wslReview: review})
	return err
}

func (runtime *ServiceRuntime) RotateAuthority(ctx context.Context, operation string, revision uint64, oldRoot, newRoot string, ports ServicePorts) error {
	_, err := runtime.request(ctx, runtimeCommand{kind: operation, revision: revision, rootSHA256: oldRoot, newRootSHA256: newRoot, ports: ports})
	return err
}

func (runtime *ServiceRuntime) refreshRotation() error {
	info, err := runtime.opts.Ledger.AuthorityRotationInfo(runtime.opts.Installation)
	runtime.rotationPending = info != nil || err != nil
	runtime.mu.Lock()
	runtime.state.Rotation = info
	runtime.touchStateLocked()
	runtime.mu.Unlock()
	return err
}
func (runtime *ServiceRuntime) Retry(ctx context.Context) error {
	_, err := runtime.request(ctx, runtimeCommand{kind: "retry"})
	return err
}

func (runtime *ServiceRuntime) refreshWSLTrustReceipts() error {
	infos, err := runtime.opts.Ledger.ListWSLTrustReceipts(runtime.opts.Installation)
	message := ""
	if err != nil {
		message = err.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
	}
	runtime.mu.Lock()
	runtime.state.WSLTrustReceipts = infos
	runtime.state.WSLTrustReceiptsRead = true
	runtime.state.WSLTrustReceiptsError = message
	runtime.touchStateLocked()
	runtime.mu.Unlock()
	return err
}
func (runtime *ServiceRuntime) Preview(ctx context.Context) error {
	_, err := runtime.request(ctx, runtimeCommand{kind: "preview"})
	return err
}
func (runtime *ServiceRuntime) Close(ctx context.Context) error {
	runtime.cancel()
	select {
	case <-runtime.done:
		return runtime.result
	case <-runtime.cleanupResult:
		return runtime.result
	default:
	}
	select {
	case <-runtime.done:
		return runtime.result
	case <-runtime.cleanupResult:
		return runtime.result
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (runtime *ServiceRuntime) loop() {
	defer runtime.finishShutdown()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	runtime.tick()
	for {
		select {
		case <-runtime.ctx.Done():
			return
		case <-ticker.C:
			runtime.tick()
		case event := <-runtime.lanEvents:
			if event.resourceObservation {
				runtime.publishLANOperations()
			} else {
				runtime.applyLANEvent(event)
			}
		case command := <-runtime.commands:
			if runtime.ctx.Err() != nil {
				command.reply <- runtimeReply{err: errors.New("local-services process is stopping")}
				continue
			}
			if err := command.ctx.Err(); err != nil {
				command.reply <- runtimeReply{err: err}
				continue
			}
			reply := runtime.apply(command)
			if !reply.deferred {
				command.reply <- reply
			}
			if reply.err == nil && command.kind != "preview" && command.kind != "restricted-fingerprint" {
				runtime.tick()
			}
		}
	}
}

// Cleanup return and positive resource release are separate events. Close may
// report an administrative/native failure immediately, but ordinary Wait must
// retain the same SDK state/image owners until every resource is known closed.
// The helper belongs to the runtime, not a replaceable service generation.
func (runtime *ServiceRuntime) finishShutdown() {
	released := false
	defer func() {
		if released {
			close(runtime.done)
		} else {
			if runtime.result == nil {
				runtime.result = errors.New("local-services cleanup remains unconfirmed")
			}
			runtime.publish("cleanup-pending", runtime.result, nil, nil)
		}
		close(runtime.cleanupResult)
		if !released {
			// Keep this original actor and its complete object graph strongly
			// retained. No retry, replacement owner or implicit cleanup follows.
			_ = runtime.Wait(context.Background())
		}
	}()
	runtime.closeLANQueries()
	runtime.lanWork.Wait()
	cleanup, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	runtime.result = errors.Join(runtime.result, runtime.stopGeneration(cleanup))
	cancel()
	if runtime.generation != nil {
		runtime.publish("cleanup-pending", runtime.result, nil, nil)
		joinedErr := runtime.generation.shutdown.Wait(context.Background())
		runtime.result = errors.Join(runtime.result, joinedErr)
		if joinedErr != nil {
			return
		}
		runtime.generation = nil
	}
	if runtime.opts.LAN != nil && runtime.opts.LAN.Helper != nil {
		runtime.publish("cleanup-pending", runtime.result, nil, nil)
		if err := runtime.opts.LAN.Helper.Close(context.Background()); err != nil {
			runtime.result = errors.Join(runtime.result, err)
			return
		}
	}
	if err := runtime.opts.Ledger.Close(); err != nil {
		runtime.result = errors.Join(runtime.result, err)
		return
	}
	runtime.publish("stopped", runtime.result, nil, nil)
	released = true
}
func (runtime *ServiceRuntime) apply(command runtimeCommand) runtimeReply {
	if command.kind == "restricted-fingerprint" {
		sha, err := runtime.restrictedFingerprint()
		return runtimeReply{restrictedSHA: sha, err: err}
	}
	if runtime.opts.Restriction != nil {
		if command.kind != "configure" || command.settings.Enabled || runtime.checkExpectedServer(command.expectedServer) != nil {
			return runtimeReply{err: errRestricted}
		}
	} else if command.expectedServer != nil && runtime.checkExpectedServer(command.expectedServer) != nil {
		return runtimeReply{err: errRestricted}
	}
	switch command.kind {
	case "lan-cleanup-review":
		return runtime.reviewPriorCleanup(command)
	case "lan-cleanup":
		return runtime.consumePriorCleanup(command)
	case "lan-owner-review":
		if command.lanReviewInput == nil || runtime.settingsUncertain || runtime.rotationPending || command.revision != runtime.settings.Revision {
			return runtimeReply{err: errors.New("reload current owner settings before LAN review")}
		}
		input := command.lanReviewInput
		if err := runtime.bindLANInstallation(input.stage.Base.Installation); err != nil {
			return runtimeReply{err: err}
		}
		review, err := runtime.lanReviews.issue(input.review.Selection, command.revision, input.stage, "activate", "", time.Now())
		if err != nil {
			return runtimeReply{err: err}
		}
		return runtime.persistLANIssued(command, review)
	case "lan-activate", "lan-remove", "lan-settle-remove":
		return runtime.activateLAN(command)
	case "lan-remove-review", "lan-settle-remove-review":
		for _, operation := range runtime.lanOperations {
			if operation.status.LeaseSHA != command.lanLeaseSHA {
				continue
			}
			if operation.controller == nil {
				continue
			}
			for _, status := range operation.controller.statuses() {
				purpose, eligible := "remove", status.CanReviewRemoval
				if command.kind == "lan-settle-remove-review" {
					purpose, eligible = "settle-remove", status.CanSettleRemoval
				}
				if status.LeaseSHA == command.lanLeaseSHA && eligible {
					review, err := runtime.lanReviews.issue(operation.record.review.Selection, runtime.settings.Revision, operation.record.stage, purpose, command.lanLeaseSHA, time.Now())
					if err != nil {
						return runtimeReply{err: err}
					}
					return runtime.persistLANIssued(command, review)
				}
			}
		}
		return runtimeReply{err: errors.New("exact positively owned LAN rule is unavailable for removal review")}
	case "lan-inspect", "lan-settle-helper":
		var operation *lanOperation
		if command.kind == "lan-settle-helper" {
			operation = runtime.lanOperations[command.lanOperationID]
		} else {
			for _, candidate := range runtime.lanOperations {
				if candidate.status.LeaseSHA == command.lanLeaseSHA {
					operation = candidate
					break
				}
			}
		}
		if operation == nil || operation.status.LeaseSHA == "" {
			return runtimeReply{err: errors.New("exact retained LAN recovery operation unavailable")}
		}
		if operation.controller == nil && operation.recovered == nil && operation.record.cleanup != nil && operation.settled && command.kind == "lan-inspect" {
			return runtime.recoverLANReview(command, LANReconciliation{ReviewID: operation.record.review.ReviewID, ReviewSHA256: operation.record.review.ReviewSHA256, Outcome: "operation", OperationID: operation.record.operationID})
		}
		if operation.controller == nil && command.kind != "lan-inspect" {
			return runtimeReply{err: errors.New("protected cleanup still requires independent exact closure observation")}
		}
		if operation.reading {
			return runtimeReply{err: errors.New("one LAN recovery observation is already active")}
		}
		operation.reading = true
		leaseSHA := operation.status.LeaseSHA
		retainedStatus := operation.status
		var originalCleanup *lanhelper.ProtectedReviewObservation
		if operation.recovered != nil {
			copy := *operation.recovered
			originalCleanup = &copy
		}
		runtime.lanWork.Add(1)
		go func() {
			defer runtime.lanWork.Done()
			ctx, cancel := context.WithTimeout(runtime.ctx, 5*time.Second)
			defer cancel()
			var status LANStatus
			var err error
			var cleanupProof *lanprovider.CleanupObservation
			if operation.controller == nil {
				status = retainedStatus
				var reader lanPriorCleanup
				if runtime.opts.LAN != nil {
					reader, _ = runtime.opts.LAN.Helper.(lanPriorCleanup)
				}
				if originalCleanup == nil || reader == nil {
					err = errors.New("protected LAN cleanup inspection unavailable")
				} else {
					proof, e := reader.ReadCleanup(ctx, leaseSHA)
					err = e
					if err == nil && matchesProtectedCleanup(*originalCleanup, proof) {
						status = priorCleanupStatus(status, *originalCleanup, proof)
						cleanupProof = &proof
					} else if err == nil {
						err = errors.New("protected native completion remains uncertain")
					}
				}
			} else if command.kind == "lan-inspect" {
				status, err = operation.controller.inspect(ctx, leaseSHA)
			} else {
				status, err = operation.controller.settleHelper(ctx, leaseSHA)
			}
			event := lanOperationEvent{operationID: operation.record.operationID, status: status, err: err, complete: true, observation: true, cleanupProof: cleanupProof}
			select {
			case runtime.lanEvents <- event:
			case <-runtime.ctx.Done():
			}
		}()
		return runtimeReply{}
	case "lan-cancel":
		operation := runtime.lanOperations[command.lanOperationID]
		if operation == nil {
			return runtimeReply{err: errors.New("exact retained LAN operation unavailable")}
		}
		if !operation.status.CanCancel {
			return runtimeReply{}
		}
		operation.cancel()
		operation.cancelRequested = true
		if operation.status.LeaseSHA != "" {
			controller, sha := operation.controller, operation.status.LeaseSHA
			if controller != nil {
				go controller.stop(runtime.ctx, sha)
			}
		}
		operation.status.CanCancel = false
		runtime.publishLANOperations()
		return runtimeReply{}
	}
	if command.kind == "lan-reconcile-review" {
		if !validLANID(command.lanReviewID) || !fullID(command.lanReviewSHA) {
			return runtimeReply{err: errors.New("exact LAN review identity required")}
		}
		reconciliation := runtime.lanReviews.reconcile(command.lanReviewID, command.lanReviewSHA)
		if reconciliation.Outcome == "unknown" {
			return runtime.recoverLANReview(command, reconciliation)
		}
		if operation := runtime.lanOperations[reconciliation.OperationID]; operation != nil && operation.settled && (operation.recovered != nil || operation.record.cleanup != nil) {
			return runtime.recoverLANReview(command, reconciliation)
		}
		return runtimeReply{lanReconciliation: &reconciliation}
	}
	if runtime.lanJournalActive {
		return runtimeReply{err: errors.New("private LAN review admission is in progress")}
	}
	rotationErr := runtime.refreshRotation()
	if runtime.rotationPending && (command.kind == "configure" || command.kind == "authority" || command.kind == "adopt-authority" || command.kind == "trust-install" || command.kind == "wsl-trust-install") {
		return runtimeReply{err: errors.Join(errors.New("resolve the retained authority rotation before changing settings or installing trust"), rotationErr)}
	}
	switch command.kind {
	case "lan-start":
		if runtime.settingsUncertain || runtime.rotationPending || command.revision != runtime.settings.Revision || !runtime.settings.Settings.Enabled || runtime.generation == nil || command.lanStage == nil {
			return runtimeReply{err: errors.New("review current enabled service generation before LAN activation")}
		}
		status, err := runtime.generation.lan.start(command.ctx, *command.lanStage)
		return runtimeReply{lan: status, err: err}
	case "lan-stop":
		if runtime.generation == nil {
			return runtimeReply{err: errors.New("LAN generation is not active")}
		}
		return runtimeReply{err: runtime.generation.lan.stop(command.ctx, command.lanLeaseSHA)}
	case "wsl-trust-receipts":
		return runtimeReply{err: runtime.refreshWSLTrustReceipts()}
	case "retire-wsl-trust-receipt":
		if runtime.settingsUncertain || command.revision != runtime.settings.Revision {
			return runtimeReply{err: errors.New("reload current settings before retiring selected WSL trust intent")}
		}
		_, err := runtime.opts.Ledger.RetireWSLTrustReceipt(command.ctx, runtime.opts.Installation, command.rootSHA256, command.wslReview, time.Now())
		err = errors.Join(err, runtime.refreshWSLTrustReceipts())
		runtime.mu.Lock()
		if previous := runtime.state.WSLTrust; previous != nil && previous.DistributionID == command.wslReview.DistributionID && previous.RootSHA256 == command.rootSHA256 {
			runtime.state.WSLTrust = nil
		}
		runtime.touchStateLocked()
		runtime.mu.Unlock()
		return runtimeReply{err: err}
	case "wsl-trust-distributions":
		registrations, err := runtime.opts.WSLRegistrations()
		if err != nil {
			return runtimeReply{err: err}
		}
		if len(registrations) > 128 {
			return runtimeReply{err: errors.New("selected WSL registration list exceeds its budget")}
		}
		for _, registration := range registrations {
			if !wsltrust.ValidRegistrationID(registration.ID) || registration.Name == "" || len(registration.Name) > 128 {
				return runtimeReply{err: errors.New("selected WSL registration metadata differs")}
			}
		}
		runtime.mu.Lock()
		runtime.state.WSLDistributions = append([]wsltrust.Registration(nil), registrations...)
		runtime.state.WSLDistributionsRead = true
		runtime.touchStateLocked()
		runtime.mu.Unlock()
		return runtimeReply{}
	case "wsl-trust-status", "wsl-trust-install", "wsl-trust-remove":
		write := command.kind != "wsl-trust-status"
		if write && (runtime.settingsUncertain || command.revision != runtime.settings.Revision) {
			return runtimeReply{err: errors.New("reload current settings before changing selected WSL trust")}
		}
		if command.kind == "wsl-trust-install" && command.rootSHA256 != runtime.settings.Settings.RootSHA256 {
			return runtimeReply{err: errors.New("review selected authority before installing WSL trust")}
		}
		client, err := runtime.opts.WSLTrustFactory()
		status := WSLTrustStatus{DistributionID: command.wslReview.DistributionID, RootSHA256: command.rootSHA256, OwnerState: "unavailable"}
		if err == nil && client == nil {
			err = errors.New("selected WSL trust transport is unavailable")
		}
		if err == nil {
			status, err = runtime.opts.Ledger.WSLTrust(command.ctx, client, runtime.opts.Installation, command.rootSHA256, strings.TrimPrefix(command.kind, "wsl-trust-"), command.wslReview, time.Now())
		}
		runtime.mu.Lock()
		runtime.state.WSLTrust = &status
		runtime.touchStateLocked()
		runtime.mu.Unlock()
		return runtimeReply{err: err}
	case "rotate-authority", "resume-authority-rotation", "cancel-authority-rotation":
		if runtime.settingsUncertain || command.revision != runtime.settings.Revision || runtime.settings.Settings.Enabled {
			return runtimeReply{err: errors.New("disable local access and review the current settings revision before rotating")}
		}
		if command.kind == "rotate-authority" && command.rootSHA256 != runtime.settings.Settings.RootSHA256 {
			return runtimeReply{err: errors.New("review the selected old authority fingerprint before rotation")}
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		err := runtime.stopGeneration(cleanup)
		cancel()
		if err != nil {
			return runtimeReply{err: err}
		}
		previousRoot := runtime.settings.Settings.RootSHA256
		runtime.publish("rotating-authority", nil, nil, nil)
		if command.kind == "cancel-authority-rotation" {
			err = runtime.opts.Ledger.CancelAuthorityRotation(runtime.opts.Installation, command.rootSHA256, command.newRootSHA256)
		} else {
			var store RootStore
			store, err = runtime.opts.RootStoreFactory(false)
			if err == nil && store == nil {
				err = errors.New("current-user root observation is unavailable")
			}
			if err == nil {
				newRoot := command.newRootSHA256
				if command.kind == "rotate-authority" {
					var info *AuthorityRotationInfo
					info, err = runtime.opts.Ledger.BeginAuthorityRotation(store, runtime.opts.Installation, command.rootSHA256, command.revision, command.ports, time.Now())
					if err == nil {
						newRoot = info.NewRootSHA256
					}
				}
				if err == nil {
					_, err = runtime.opts.Ledger.ResumeAuthorityRotation(store, runtime.opts.Installation, command.rootSHA256, newRoot, time.Now())
				}
				err = errors.Join(err, store.Close())
			}
		}
		actual, readErr := runtime.opts.Ledger.LoadSettings()
		if readErr == nil && actual.Revision >= runtime.settings.Revision {
			runtime.settings = actual
		} else {
			runtime.settingsUncertain = true
			readErr = errors.Join(readErr, errors.New("rotation settings could not be recovered without revision regression"))
		}
		err = errors.Join(err, readErr, runtime.refreshAuthorityReview(), runtime.refreshRotation())
		if previousRoot != runtime.settings.Settings.RootSHA256 {
			runtime.mu.Lock()
			runtime.state.WindowsTrust = nil
			runtime.state.WSLTrust = nil
			runtime.mu.Unlock()
		}
		phase := "disabled"
		if runtime.rotationPending {
			phase = "rotation-pending"
		}
		runtime.publish(phase, err, nil, nil)
		return runtimeReply{settings: runtime.settings, err: err}
	case "trust-status", "trust-install", "trust-remove":
		if !fullID(command.rootSHA256) {
			return runtimeReply{err: errors.New("exact reviewed root fingerprint required")}
		}
		write := command.kind != "trust-status"
		if write && (runtime.settingsUncertain || command.revision != runtime.settings.Revision) {
			return runtimeReply{err: errors.New("reload current settings before changing Windows trust")}
		}
		if command.kind == "trust-install" {
			if command.rootSHA256 != runtime.settings.Settings.RootSHA256 {
				return runtimeReply{err: errors.New("review the selected authority before installing Windows trust")}
			}
			if _, err := runtime.opts.Ledger.LoadAuthority(runtime.opts.Installation, command.rootSHA256, time.Now()); err != nil {
				return runtimeReply{err: err}
			}
		}
		store, err := runtime.opts.RootStoreFactory(write)
		if err == nil && store == nil {
			err = errors.New("Windows root store is unavailable")
		}
		status := WindowsTrustStatus{RootSHA256: command.rootSHA256, State: "unavailable"}
		if err == nil {
			switch command.kind {
			case "trust-status":
				status, err = runtime.opts.Ledger.WindowsTrustStatus(store, runtime.opts.Installation, command.rootSHA256)
			case "trust-install":
				status, err = runtime.opts.Ledger.InstallWindowsTrust(store, runtime.opts.Installation, command.rootSHA256, time.Now())
			case "trust-remove":
				status, err = runtime.opts.Ledger.RemoveWindowsTrust(store, runtime.opts.Installation, command.rootSHA256)
			}
			err = errors.Join(err, store.Close())
		}
		if status.RootSHA256 == "" {
			status.RootSHA256 = command.rootSHA256
			status.State = "unavailable"
		}
		runtime.mu.Lock()
		runtime.state.WindowsTrust = &status
		runtime.touchStateLocked()
		runtime.mu.Unlock()
		return runtimeReply{err: err}
	case "configure":
		previous := runtime.settings
		// Refuse an incompatible reviewed port/root before persisting enabled
		// intent or allocating any helpers. Disabling must remain available when
		// signing state is expired or unavailable.
		if command.settings.Enabled {
			authority, err := runtime.opts.Ledger.LoadAuthority(runtime.opts.Installation, command.settings.RootSHA256, time.Now())
			if err != nil {
				return runtimeReply{err: err}
			}
			if authority.RevocationHTTPPort() != command.settings.Ports.HTTP {
				return runtimeReply{err: errors.New("integration HTTP port requires a reviewed authority rotation")}
			}
		}
		stored, err := runtime.commitSettings(command.revision, command.settings)
		if err != nil && stored.Revision <= previous.Revision {
			return runtimeReply{err: err}
		}
		runtime.settings = stored
		runtime.publish("applying", nil, nil, nil)
		runtime.attempts = 0
		runtime.retryAt = time.Time{}
		if previous.Settings.Ports != stored.Settings.Ports || previous.Settings.RootSHA256 != stored.Settings.RootSHA256 {
			cleanup, cancel := context.WithTimeout(runtime.ctx, 35*time.Second)
			err = errors.Join(err, runtime.demoteHTTP(cleanup))
			cancel()
		}
		runtime.publish("applying", err, nil, nil)
		return runtimeReply{settings: stored, err: err}
	case "authority", "adopt-authority":
		if runtime.settingsUncertain {
			return runtimeReply{err: errors.New("saved settings are unreadable; restore them and retry before changing authority selection")}
		}
		if command.revision != runtime.settings.Revision || runtime.settings.Settings.Enabled {
			return runtimeReply{err: errors.New("disable integration and review the current settings revision before creating an authority")}
		}
		if runtime.settings.Settings.RootSHA256 != "" {
			return runtimeReply{err: errors.New("an authority is already selected; rotation requires a separate reviewed transaction")}
		}
		var authority *Authority
		var err error
		if command.kind == "adopt-authority" {
			authority, err = runtime.opts.Ledger.LoadAuthority(runtime.opts.Installation, command.rootSHA256, time.Now())
			if err == nil && authority.RevocationHTTPPort() != runtime.settings.Settings.Ports.HTTP {
				err = fmt.Errorf("saved authority uses HTTP port %d; save that port before resuming setup", authority.RevocationHTTPPort())
			}
		} else {
			authority, err = runtime.opts.Ledger.CreateAuthorityWithCRLPort(runtime.opts.Installation, time.Now(), runtime.settings.Settings.Ports.HTTP)
		}
		if err != nil {
			return runtimeReply{err: errors.Join(err, runtime.refreshAuthorityReview())}
		}
		settings := runtime.settings.Settings
		settings.RootSHA256 = authority.Fingerprint()
		stored, storeErr := runtime.commitSettings(command.revision, settings)
		if storeErr != nil && stored.Revision <= runtime.settings.Revision {
			return runtimeReply{err: errors.Join(storeErr, runtime.refreshAuthorityReview())}
		}
		runtime.settings = stored
		info, err := runtime.opts.Ledger.AuthorityInfo(runtime.opts.Installation)
		if err == nil {
			runtime.mu.Lock()
			runtime.state.Authority = &info
			runtime.touchStateLocked()
			runtime.mu.Unlock()
		}
		err = errors.Join(storeErr, err)
		runtime.publish("disabled", err, nil, nil)
		return runtimeReply{authority: info, err: err}
	case "retry":
		cleanup, cancel := context.WithTimeout(runtime.ctx, 35*time.Second)
		err := runtime.stopGeneration(cleanup)
		cancel()
		runtime.attempts = 0
		runtime.retryAt = time.Time{}
		runtime.residualChecked = false
		return runtimeReply{err: err}
	case "preview":
		ctx, cancel := context.WithTimeout(runtime.ctx, 5*time.Second)
		defer cancel()
		candidates, err := DiscoverServices(ctx, runtime.opts.Client, runtime.opts.Ledger.daemon, runtime.settings.Settings.Preferences)
		if err == nil {
			runtime.recordInventory(candidates)
		}
		return runtimeReply{err: err}
	}
	return runtimeReply{err: errors.New("unknown local-services operation")}
}

// Refresh public review metadata after an incomplete setup attempt, so the UI
// can offer explicit adoption without requiring a process restart.
func (runtime *ServiceRuntime) refreshAuthorityReview() error {
	info, err := runtime.opts.Ledger.AuthorityInfo(runtime.opts.Installation)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	runtime.mu.Lock()
	runtime.state.Authority = &info
	runtime.touchStateLocked()
	runtime.mu.Unlock()
	return nil
}

// A rename/write-through or draft-cleanup error may follow a committed replace.
// Re-read before deciding that desired intent stayed unchanged; report the error
// even when its actual committed revision can be recovered. Callers must not
// blindly replay a mutation whose acknowledgement was uncertain.
func (runtime *ServiceRuntime) commitSettings(revision uint64, settings ServiceSettings) (SettingsSnapshot, error) {
	stored, err := runtime.opts.Ledger.StoreSettings(revision, settings)
	if err == nil {
		return stored, nil
	}
	actual, readErr := runtime.opts.Ledger.LoadSettings()
	if readErr == nil && actual.Revision < runtime.settings.Revision {
		readErr = errors.New("saved settings revision regressed; retain the original record for recovery")
	}
	if readErr != nil {
		// Desired intent may have changed, but cannot be determined. Retire only
		// local ingress rather than continue serving an unverified enabled intent.
		runtime.settingsUncertain = true
		cleanup, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		cause := errors.Join(err, readErr, runtime.stopGeneration(cleanup))
		cancel()
		runtime.residualChecked = false
		runtime.publish("unavailable", cause, nil, nil)
		return SettingsSnapshot{}, cause
	}
	return actual, errors.Join(err, readErr)
}
func (runtime *ServiceRuntime) stopGeneration(ctx context.Context) error {
	if runtime.generation == nil {
		return nil
	}
	generation := runtime.generation
	if err := generation.shutdown.Close(ctx); err != nil {
		// The exact generation remains an owner even when its worker has returned
		// a terminal error. A later nil-generation cleanup must never erase it.
		return err
	}
	runtime.generation = nil
	return nil
}
func (runtime *ServiceRuntime) prepareImage(ctx context.Context) (ToolsImage, error) {
	return runtime.opts.Ledger.PrepareToolsImage(ctx, runtime.opts.Client, runtime.opts.ToolsAsset, runtime.opts.ToolsSHA256)
}
func (runtime *ServiceRuntime) recoverResidual(ctx context.Context) error {
	namespaces, err := runtime.opts.Ledger.records()
	if err != nil {
		return err
	}
	relays, err := runtime.opts.Ledger.relayRecords()
	if err != nil {
		return err
	}
	if len(namespaces)+len(relays) == 0 {
		return nil
	}
	image, err := runtime.prepareImage(ctx)
	if err != nil {
		return err
	}
	relay, err := OpenRelayManager(ctx, runtime.opts.Client, runtime.opts.Ledger, image)
	if err != nil {
		return err
	}
	manager, err := OpenNamespaceManagerBorrowingLedger(ctx, runtime.opts.Client, runtime.opts.Ledger, image)
	if err != nil {
		return errors.Join(err, relay.Close(ctx))
	}
	return errors.Join(relay.Close(ctx), manager.Close(ctx))
}
func (runtime *ServiceRuntime) startGeneration(ctx context.Context) error {
	authority, err := runtime.opts.Ledger.LoadAuthority(runtime.opts.Installation, runtime.settings.Settings.RootSHA256, time.Now())
	if err != nil {
		return err
	}
	image, err := runtime.prepareImage(ctx)
	if err != nil {
		return err
	}
	relay, err := OpenRelayManager(ctx, runtime.opts.Client, runtime.opts.Ledger, image)
	if err != nil {
		return err
	}
	if runtime.opts.portProbe != nil {
		relay.portProbe = runtime.opts.portProbe
	}
	namespaces, err := OpenNamespaceManagerBorrowingLedger(ctx, runtime.opts.Client, runtime.opts.Ledger, image)
	if err != nil {
		return errors.Join(err, relay.Close(ctx))
	}
	backend, err := OwnServiceBackend(namespaces, relay)
	if err != nil {
		return errors.Join(err, relay.Close(ctx), namespaces.Close(ctx))
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		return errors.Join(err, backend.Close(ctx))
	}
	shared := shareNamespaces(backend)
	view, err := newHTTPNamespaceView(shared)
	if err != nil {
		return errors.Join(err, shared.Close(ctx))
	}
	host, err := StartServiceHostWithPorts(view, authority, hex.EncodeToString(token[:]), runtime.settings.Settings.Ports)
	if err != nil {
		return errors.Join(err, shared.Close(ctx))
	}
	lan := runtime.newRuntimeLANController(shared)
	runtime.generation = &serviceGeneration{host: host, shared: shared, lan: lan, shutdown: newGenerationShutdown(host, lan, shared), relay: relay, started: time.Now()}
	return nil
}
func (runtime *ServiceRuntime) promoteHTTP(ctx context.Context) error {
	generation := runtime.generation
	if generation == nil || !generation.lanOnly {
		return errors.New("exact LAN-only generation required")
	}
	authority, err := runtime.opts.Ledger.LoadAuthority(runtime.opts.Installation, runtime.settings.Settings.RootSHA256, time.Now())
	if err != nil {
		return err
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		return err
	}
	view, err := newHTTPNamespaceView(generation.shared)
	if err != nil {
		return err
	}
	host, err := StartServiceHostWithPorts(view, authority, hex.EncodeToString(token[:]), runtime.settings.Settings.Ports)
	if err != nil {
		return err
	}
	generation.host = host
	generation.lanOnly = false
	generation.shutdown = newGenerationShutdown(host, generation.lan, generation.shared)
	return nil
}
func (runtime *ServiceRuntime) demoteHTTP(ctx context.Context) error {
	generation := runtime.generation
	if generation == nil || generation.lanOnly {
		return nil
	}
	if err := generation.host.Close(ctx); err != nil {
		return err
	}
	if err := generation.relay.Stop(ctx); err != nil {
		return err
	}
	generation.host = nil
	generation.lanOnly = true
	generation.networks = nil
	generation.relayID = ""
	generation.shutdown = newGenerationShutdown(generation.shared, generation.lan, generation.shared)
	return nil
}

// Plain LAN ports borrow the same exclusive ledger/backend without requiring
// an HTTP authority, local names, host listener or guest relay activation.
func (runtime *ServiceRuntime) startLANOnlyGeneration(ctx context.Context) error {
	g, err := buildLANOnlyGeneration(ctx, runtime.opts, runtime.lanInstallation, runtime.lanEvents)
	if err == nil {
		runtime.generation = g
	}
	return err
}

func buildLANOnlyGeneration(ctx context.Context, opts RuntimeOptions, installation string, events chan lanOperationEvent) (*serviceGeneration, error) {
	image, err := opts.Ledger.PrepareToolsImage(ctx, opts.Client, opts.ToolsAsset, opts.ToolsSHA256)
	if err != nil {
		return nil, err
	}
	relay, err := OpenRelayManager(ctx, opts.Client, opts.Ledger, image)
	if err != nil {
		return nil, err
	}
	namespaces, err := OpenNamespaceManagerBorrowingLedger(ctx, opts.Client, opts.Ledger, image)
	if err != nil {
		return nil, errors.Join(err, relay.Close(ctx))
	}
	backend, err := OwnServiceBackend(namespaces, relay)
	if err != nil {
		return nil, errors.Join(err, relay.Close(ctx), namespaces.Close(ctx))
	}
	shared := shareNamespaces(backend)
	if installation == "" && opts.LAN != nil {
		installation = opts.LAN.Installation
	}
	lan := newLANController(installation, shared, opts.LAN)
	if events != nil {
		lan.onChange = func() {
			select {
			case events <- lanOperationEvent{resourceObservation: true}:
			default:
			}
		}
	}
	return &serviceGeneration{lanOnly: true, shared: shared, lan: lan, shutdown: newGenerationShutdown(shared, lan, shared), relay: relay, started: time.Now()}, nil
}
func runtimeNetworks(ids []string, statuses []ServiceStatus) ([]RelayNetwork, error) {
	names := map[string]bool{}
	for _, status := range statuses {
		if status.State == "active" {
			for _, name := range status.Names {
				if strings.HasSuffix(name, ".orbwin.test") {
					names[name] = true
				}
			}
		}
	}
	if len(names) == 0 || len(ids) == 0 {
		return nil, nil
	}
	if len(names)*len(ids) > 256 {
		return nil, errors.New("client-network aliases exceed the supported total of 256")
	}
	aliases := make([]string, 0, len(names))
	for name := range names {
		aliases = append(aliases, name)
	}
	sort.Strings(aliases)
	networks := make([]RelayNetwork, 0, len(ids))
	for _, id := range ids {
		networks = append(networks, RelayNetwork{ID: id, Aliases: append([]string(nil), aliases...)})
	}
	return networks, nil
}
func (runtime *ServiceRuntime) tick() {
	if runtime.ctx.Err() != nil {
		return
	}
	if runtime.lanJournalActive {
		if runtime.generation != nil {
			runtime.generation.lan.kickRefresh()
		}
		runtime.publishLANOperations()
		return
	}
	rotationErr := runtime.refreshRotation()
	if runtime.rotationPending {
		cleanup, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		err := runtime.stopGeneration(cleanup)
		cancel()
		runtime.publish("rotation-pending", errors.Join(errors.New("an authority rotation requires explicit resume or cancellation"), rotationErr, err), nil, nil)
		return
	}
	ctx, cancel := context.WithTimeout(runtime.ctx, 20*time.Second)
	defer cancel()
	if runtime.generation != nil {
		runtime.generation.lan.kickRefresh()
	}
	if runtime.attempts >= 3 || time.Now().Before(runtime.retryAt) {
		runtime.publishLANOperations()
		return
	}
	var err error
	if runtime.settingsUncertain {
		var actual SettingsSnapshot
		actual, err = runtime.opts.Ledger.LoadSettings()
		if err == nil && actual.Revision < runtime.settings.Revision {
			err = errors.New("saved settings revision regressed; restore the original record before retrying")
		}
		if err == nil {
			runtime.settings = actual
			runtime.settingsUncertain = false
		}
	}
	if err == nil && !runtime.residualChecked && runtime.generation == nil {
		err = runtime.recoverResidual(ctx)
		runtime.residualChecked = err == nil
	}
	if err == nil && !runtime.settings.Settings.Enabled {
		err = runtime.demoteHTTP(ctx)
		if err == nil {
			runtime.publish("disabled", nil, nil, nil)
			return
		}
	}
	if err == nil && runtime.settings.Settings.Enabled && runtime.generation != nil && runtime.generation.lanOnly {
		if err = runtime.promoteHTTP(ctx); err != nil {
			runtime.attempts++
			runtime.retryAt = time.Now().Add(time.Duration(runtime.attempts) * time.Second)
			runtime.publish("unavailable", err, nil, nil)
			return
		}
	}
	if err == nil && runtime.generation == nil {
		runtime.publish("starting", nil, nil, nil)
		err = runtime.startGeneration(ctx)
	}
	var candidates []ServiceCandidate
	var statuses []ServiceStatus
	if err == nil {
		candidates, err = DiscoverServices(ctx, runtime.opts.Client, runtime.opts.Ledger.daemon, runtime.settings.Settings.Preferences)
		if err == nil {
			runtime.recordInventory(candidates)
		}
	}
	if err == nil {
		statuses, err = runtime.generation.host.Reconcile(ctx, candidates, false)
	}
	if err == nil {
		var networks []RelayNetwork
		networks, err = runtimeNetworks(runtime.settings.Settings.Networks, statuses)
		if err == nil && runtime.generation.relayID != "" && !reflect.DeepEqual(networks, runtime.generation.networks) {
			err = runtime.generation.relay.Stop(ctx)
			if err == nil {
				runtime.generation.relayID = ""
			}
		}
		if err == nil && runtime.generation.relayID == "" {
			runtime.generation.relayID, err = runtime.generation.relay.StartWithPorts(ctx, runtime.generation.host.Gateway(), networks, runtime.settings.Settings.Ports)
			if err == nil {
				runtime.generation.networks = networks
			}
		}
		if err == nil {
			err = runtime.generation.relay.Renew(ctx)
		}
	}
	if err != nil {
		cleanup, done := context.WithTimeout(context.Background(), 35*time.Second)
		err = errors.Join(err, runtime.demoteHTTP(cleanup))
		done()
		runtime.attempts++
		runtime.retryAt = time.Now().Add(time.Duration(runtime.attempts) * time.Second)
		runtime.publish("unavailable", err, candidates, nil)
		return
	}
	if time.Since(runtime.generation.started) > 30*time.Second {
		runtime.attempts = 0
	}
	runtime.publish("running", nil, candidates, statuses)
}
