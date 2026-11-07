package sdkstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"slices"
	"sort"
	"sync"
)

// The retirement authority is an actual inherited native owner capability,
// never a receipt/JSON/boolean constructor. Its backend also retains the exact
// compatible old controller.lock and publication exclusion through the copy.
type ColdRetirement struct{ s *coldRetirementState }
type coldRetirementState struct {
	mu     sync.Mutex
	scope  TransitionScope
	native coldAuthority
	used   bool
}
type coldAuthority interface {
	check(context.Context) error
	begin(context.Context) (coldBackend, error)
	close() error
}
type TransitionScope struct {
	Transaction, Installation, Daemon, SID string
	ManagementRetirement, SDKImage         [32]byte
}
type transitionProcess struct {
	PID      uint32
	Creation uint64
	Session  uint32
	Image    [32]byte
}
type TransitionFile struct {
	Name, Source, Target, Disposition string
	Size                              uint64
	SHA256                            [32]byte
}
type TransitionReceipt struct {
	Schema                                                      int
	Scope                                                       TransitionScope
	LegacyRoot, LegacyController, Root, Init, Fence, Quarantine string
	Files                                                       []TransitionFile
	Inventory, Active, Historical                               [32]byte
}

func (r TransitionReceipt) valid() bool {
	if r.Schema != 1 || !transitionID(r.Scope.Transaction) || !installationID(r.Scope.Installation) || !validDaemon(r.Scope.Daemon) || r.Scope.SID == "" || r.Scope.ManagementRetirement == [32]byte{} || r.Scope.SDKImage == [32]byte{} || r.LegacyRoot == "" || r.LegacyController == "" || r.Root == "" || r.Init == "" || r.Fence == "" || r.Quarantine == "" || r.Inventory == [32]byte{} || r.Active == [32]byte{} || r.Historical == [32]byte{} || len(r.Files) > maxEntries {
		return false
	}
	seen := map[string]bool{}
	identities := map[string]bool{}
	var total uint64
	for i, f := range r.Files {
		if recordBudget(f.Name) == 0 || seen[f.Name] || i > 0 && r.Files[i-1].Name >= f.Name || f.Source == "" || f.Target == "" || f.Source == f.Target || identities[f.Source] || identities[f.Target] || f.SHA256 == [32]byte{} || f.Size > uint64(recordBudget(f.Name)) || f.Disposition != transitionDisposition(f.Name) {
			return false
		}
		seen[f.Name] = true
		identities[f.Source], identities[f.Target] = true, true
		total += f.Size
		if total > maxReadBytes {
			return false
		}
	}
	all, active, historical := transitionHashes(r.Files)
	return r.Inventory == all && r.Active == active && r.Historical == historical
}

// Digest identifies this complete immutable source/target inventory. It is
// correlation for the management journal, never a replacement for a live owner.
func (r TransitionReceipt) Digest() ([32]byte, error) {
	if !r.valid() {
		return [32]byte{}, ErrRecoveryRequired
	}
	data, err := json.Marshal(r)
	if err != nil {
		return [32]byte{}, ErrRecoveryRequired
	}
	return sha256.Sum256(data), nil
}
func transitionID(v string) bool {
	if len(v) != 32 {
		return false
	}
	nonzero := false
	for _, c := range v {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
		nonzero = nonzero || c != '0'
	}
	return nonzero
}
func transitionDisposition(name string) string {
	if name == "authority-v1.json" || name == "settings-v1.json" {
		return "active-data"
	}
	return "historical-only"
}

// Snapshot exposes copies of actual cold source data only to the SDK's semantic
// validator. It has no caller-bytes constructor and cannot authorize a mutation.
type ColdSnapshot struct {
	scope   TransitionScope
	records map[string][]byte
}

func (s *ColdSnapshot) Scope() TransitionScope { return s.scope }
func (s *ColdSnapshot) Names() []string {
	names := make([]string, 0, len(s.records))
	for n := range s.records {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func (s *ColdSnapshot) Read(name string) ([]byte, error) {
	if s == nil || recordBudget(name) == 0 {
		return nil, ErrState
	}
	data, ok := s.records[name]
	if !ok {
		return nil, ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

type ColdValidator interface{ ValidateColdSDK(*ColdSnapshot) error }

// Trusted in-process participant registered once before its actor starts.
// VerifyJoined is read-only: it must not call Root methods or wait for work
// requiring the root transaction mutex. RootBacking is the exact capability.
type ColdRuntimeParticipant interface {
	RootBacking() *Root
	LeafFactoryBacking() *LANObservationFactory
	VerifyJoined(context.Context) error
}

// A distinct actual callee-completion proof. This is not VerifyJoined or SDK
// retirement; it only permits settling known master-query resources after a
// constructor failed before starting the actor and all its partial owners closed.
type ColdRuntimeUnstarted interface {
	ColdRuntimeParticipant
	PrepareUnstarted(context.Context) error
	VerifyUnstarted(context.Context) error
}
type coldBackend interface {
	check(context.Context) error
	capture(context.Context) (*ColdSnapshot, error)
	admit(context.Context) error
	seal(context.Context) error
	create(context.Context) error
	copy(context.Context, string, []byte, string) (TransitionFile, error)
	finish(context.Context, []TransitionFile) (TransitionReceipt, *Root, error)
	// Positive native final-owner transfer must precede closing the candidate
	// root. A committed marker alone is insufficient to release publication and
	// predecessor exclusion. Unknown transfer retains every owner without retry.
	// Called under the transition and root guard mutexes; implementations consume
	// an already acknowledged transfer and must not call Root methods or wait
	// for a peer to perform work that needs those mutexes.
	prepareCommittedClose(TransitionReceipt) error
	bindRuntime(*Root, ColdRuntimeParticipant) error
	close() error
}
type ColdTransition struct {
	mu                         sync.Mutex
	retirement                 *ColdRetirement
	native                     coldBackend
	root                       *Root
	receipt                    TransitionReceipt
	attempted, unknown         bool
	closed                     bool
	commitAttempted, committed bool
	runtime                    ColdRuntimeParticipant
	leafFactory                *LANObservationFactory
}

func (t *ColdTransition) RegisterRuntime(ctx context.Context, participant ColdRuntimeParticipant) error {
	if t == nil || !bounded(ctx) || participant == nil {
		return ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.unknown || t.commitAttempted || t.runtime != nil || t.root == nil || t.root.Check() != nil || t.native == nil || participant.RootBacking() != t.root {
		return ErrRecoveryRequired
	}
	factory := participant.LeafFactoryBacking()
	if factory == nil || factory.RootBacking() != t.root || factory.TransitionBacking() != t {
		return ErrRecoveryRequired
	}
	if t.native.bindRuntime(t.root, participant) != nil {
		return ErrRecoveryRequired
	}
	t.runtime = participant
	t.leafFactory = factory
	return nil
}

// Commit consumes the original native final slot once. Durable management
// commit must already have released that slot; a receipt or caller boolean
// cannot mint ordinary-start admission. Unknown replies retain all owners.
func (t *ColdTransition) Commit(ctx context.Context) error {
	if t == nil || !bounded(ctx) {
		return ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.unknown || t.commitAttempted || t.native == nil || t.root == nil || t.root.Check() != nil || !t.receipt.valid() {
		return ErrRecoveryRequired
	}
	// The commit marker cannot race an admitted record mutation or reader. This
	// is the same native root owner used by every candidate SDK transaction.
	t.root.s.mu.Lock()
	defer t.root.s.mu.Unlock()
	if t.root.s.active != nil {
		return ErrBusy
	}
	if t.root.s.closed || t.root.s.poison || t.root.s.b.check() != nil {
		return ErrRecoveryRequired
	}
	t.commitAttempted, t.unknown = true, true
	native, ok := t.native.(interface {
		commit(context.Context, TransitionReceipt) error
	})
	if !ok || native.commit(ctx, t.receipt) != nil || t.native.check(ctx) != nil {
		t.root.s.poison = true
		return ErrRecoveryRequired
	}
	t.committed, t.unknown = true, false
	return nil
}

func BeginColdTransition(ctx context.Context, retirement *ColdRetirement, validator ColdValidator) (*ColdTransition, error) {
	if !bounded(ctx) || retirement == nil || retirement.s == nil || validator == nil {
		return nil, ErrState
	}
	r := retirement.s
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.used || r.native == nil || r.native.check(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	r.used = true
	b, err := r.native.begin(ctx)
	t := &ColdTransition{retirement: retirement, native: b, attempted: true, unknown: true}
	if err != nil || b == nil {
		return t, ErrRecoveryRequired
	}
	snapshot, err := b.capture(ctx)
	if err != nil || snapshot == nil || snapshot.scope != r.scope {
		return t, ErrRecoveryRequired
	}
	defer func() {
		for _, body := range snapshot.records {
			clear(body)
		}
	}()
	if validator.ValidateColdSDK(snapshot) != nil || b.check(ctx) != nil {
		return t, ErrRecoveryRequired
	}
	// Outside-root fence is durable BEFORE seal/create/copy. Admitted errors
	// retain the old lock/tree and partial new objects, with no reset or replay.
	if b.admit(ctx) != nil || b.seal(ctx) != nil || b.create(ctx) != nil {
		return t, ErrRecoveryRequired
	}
	var files []TransitionFile
	for _, name := range snapshot.Names() {
		data := snapshot.records[name]
		role := transitionDisposition(name)
		file, err := b.copy(ctx, name, data, role)
		if err != nil || file.Name != name || file.Disposition != role || file.Size != uint64(len(data)) || file.SHA256 != sha256.Sum256(data) {
			return t, ErrRecoveryRequired
		}
		files = append(files, file)
	}
	receipt, root, err := b.finish(ctx, files)
	t.root = root
	if err != nil || root == nil || !receipt.valid() || receipt.Scope != r.scope || !slices.Equal(receipt.Files, files) || b.check(ctx) != nil {
		return t, ErrRecoveryRequired
	}
	t.receipt = receipt
	t.unknown = false
	return t, nil
}
func (t *ColdTransition) CandidateRoot(ctx context.Context) (*Root, TransitionReceipt, error) {
	if t == nil || !bounded(ctx) {
		return nil, TransitionReceipt{}, ErrState
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.unknown || t.native == nil || t.native.check(ctx) != nil || t.root == nil || t.root.Check() != nil || !t.receipt.valid() {
		return nil, TransitionReceipt{}, ErrRecoveryRequired
	}
	receipt := t.receipt
	receipt.Files = append([]TransitionFile(nil), receipt.Files...)
	return t.root, receipt, nil
}
func (t *ColdTransition) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	if t.unknown {
		return ErrRecoveryRequired
	}
	if !t.committed || t.native == nil || t.root == nil || t.root.s == nil || !t.receipt.valid() {
		return ErrRecoveryRequired
	}
	// Keep record admission excluded across native final-owner transfer and
	// positive root close. A guard already in flight is a retryable busy result
	// before transfer; no external Root.Begin can enter between these steps.
	s := t.root.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		return ErrBusy
	}
	if s.closed || s.poison || s.b.check() != nil {
		return ErrRecoveryRequired
	}
	if t.runtime == nil || t.runtime.RootBacking() != t.root || t.leafFactory == nil || t.runtime.LeafFactoryBacking() != t.leafFactory || t.leafFactory.RootBacking() != t.root || t.leafFactory.TransitionBacking() != t || t.runtime.VerifyJoined(context.Background()) != nil {
		return ErrRecoveryRequired
	}
	t.unknown = true
	if t.native.prepareCommittedClose(t.receipt) != nil {
		return ErrRecoveryRequired
	}
	// Equivalent to Root.Close while retaining its guard mutex across transfer.
	s.closed = true
	s.closeErr = s.b.close()
	if s.closeErr != nil {
		s.poison = true
		return ErrRecoveryRequired
	}
	if t.native != nil && t.native.close() != nil {
		t.unknown = true
		return ErrRecoveryRequired
	}
	t.closed = true
	t.unknown = false
	return nil
}

func transitionHashes(files []TransitionFile) (all, active, historical [32]byte) {
	a, b, c := sha256.New(), sha256.New(), sha256.New()
	for _, file := range files {
		data, _ := json.Marshal(file)
		_ = json.NewEncoder(a).Encode(file)
		if file.Disposition == "active-data" {
			_, _ = b.Write(data)
		} else {
			_, _ = c.Write(data)
		}
	}
	copy(all[:], a.Sum(nil))
	copy(active[:], b.Sum(nil))
	copy(historical[:], c.Sum(nil))
	return
}
