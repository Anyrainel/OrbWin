//go:build windows && amd64

package lanprovider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanobserve"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"
)

const fixtureOptIn = "disabled-block-create-duplicate-race-v1"

type fixtureRunSummary struct {
	Version                                                                                   int `json:"version"`
	Complete, DisabledBlockOnly, IncumbentPreserved, SingleRaceWinner, RequiredUnknownRefused bool
	CreatesDispatched, DeletesAcknowledged, PendingIntents                                    int
	ReceiptDirectory, ExecutableSHA256                                                        string
	ActivationAvailable, ProviderCAS, ProductIntegration                                      bool
	Events                                                                                    []fixturePublicEvent
}
type fixturePublicEvent struct {
	Actor                                                    string
	Dispatched, Terminal, Closed, ReturnedFullKeys, Recorded bool
	Status                                                   uint32
}

func fixtureSanitizeCompletion(actor string, r fixtureCompletion) fixturePublicEvent {
	return fixturePublicEvent{actor, r.dispatched, r.terminal, r.closed, r.returnedFullKeys, r.recorded, r.status}
}

type fixtureRetirement struct {
	owner       fixtureOwnership
	journal     *fixtureDiskJournal
	policy      fixtureNativePolicy
	baseline    fixtureDefinition
	hasBaseline bool
}

func TestMIFixtureMutationLayoutMatchesCompiledHeader(t *testing.T) {
	if unsafe.Offsetof(fixtureMutationSessionTable{}.createInstance) != 32 || unsafe.Offsetof(fixtureMutationSessionTable{}.deleteInstance) != 40 {
		t.Fatal("fixture mutation ABI differs")
	}
	if fixtureDisabled != 2 || fixtureBlock != 4 || fixtureInbound != 1 || fixturePrivate != 2 || fixtureEdgeBlock != 0 || fixtureAlreadyExists != 11 {
		t.Fatal("fixture primary enum contract changed")
	}
}

// This single opt-in is a proposed administrative qualification, NEVER a normal
// test or product entrypoint. Root reviews hash/bounds before any explicit run.
func TestMICIMExclusiveCreateDisabledBlockNative(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_LAN_CIM_EXCLUSIVE") != fixtureOptIn {
		t.Skip("source/compile only until root review; pre-elevated fixed disabled BLOCK fixture requires external deadline")
	}
	admin, e := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if e != nil {
		t.Fatal("administrator token precondition unavailable")
	}
	member, e := windows.Token(0).IsMember(admin)
	if e != nil || !member || fixturePrivilegePrecondition(windows.GetCurrentProcessToken().IsElevated()) != nil {
		t.Fatal("pre-elevated administrator token required before dispatch; no automatic elevation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, e := runCIMExclusiveFixture(ctx)
	data, _ := json.Marshal(out)
	t.Log(string(data))
	if e != nil {
		t.Fatal(e)
	}
	if !out.Complete || !out.IncumbentPreserved || !out.SingleRaceWinner || !out.RequiredUnknownRefused || out.PendingIntents != 0 || !out.DisabledBlockOnly || out.ActivationAvailable || out.ProviderCAS || out.ProductIntegration {
		t.Fatal("inert fixture incomplete or authority overstated")
	}
}
func runCIMExclusiveFixture(ctx context.Context) (out fixtureRunSummary, err error) {
	out = fixtureRunSummary{Version: 1, DisabledBlockOnly: true}
	defer func() {
		out.Complete = err == nil && out.PendingIntents == 0 && out.IncumbentPreserved && out.SingleRaceWinner && out.RequiredUnknownRefused
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Before even schema queries, establish own program and an independent fresh
	// protected receipt root. No private configuration or foreign rule data read.
	programFile, program, hash, e := fixtureHoldOwnProgram()
	if e != nil {
		return out, e
	}
	defer func() {
		if programFile.Close() != nil {
			err = errors.Join(err, errors.New("fixture program handle close failed"))
		}
	}()
	out.ExecutableSHA256 = hash
	var nonceBytes [16]byte
	if _, e = rand.Read(nonceBytes[:]); e != nil {
		return out, errors.New("fixture nonce unavailable")
	}
	nonce := hex.EncodeToString(nonceBytes[:])
	root, e := fixtureNewProtectedRoot(nonce)
	if e != nil {
		return out, e
	}
	defer func() {
		if root.close() != nil {
			err = errors.Join(err, errors.New("fixture receipt directory handle close failed"))
		}
	}()
	out.ReceiptDirectory = root.path
	journals := map[string]*fixtureDiskJournal{}
	for _, actor := range []string{"A", "B", "C", "D", "U"} {
		j, e := root.newJournal(actor)
		if e != nil {
			return out, e
		}
		journals[actor] = j
		defer func() {
			if j.file.Close() != nil {
				err = errors.Join(err, errors.New("fixture journal handle close failed"))
			}
		}()
	}
	declarations, e := readTraversalDeclarations(ctx)
	if e != nil {
		return out, errors.New("fixture fresh rule/filter schema unavailable")
	}
	observed, e := lanobserve.Observe(ctx)
	if e != nil {
		return out, errors.New("fixture loopback identity observation unavailable")
	}
	alias := ""
	for _, adapter := range observed.Adapters {
		if adapter.Type != 24 || adapter.OperationalStatus != 1 {
			continue
		}
		for _, address := range adapter.Addresses {
			if address.Address == "127.0.0.1" {
				if alias != "" {
					return out, errors.New("fixture loopback identity ambiguous")
				}
				alias = adapter.Alias
			}
		}
	}
	if e = fixtureValidateLoopbackAlias(alias); e != nil {
		return out, e
	}
	makePolicy := func(actor, suffix, port string, incumbent bool) fixtureNativePolicy {
		return fixtureNativePolicy{intent: fixtureIntent{name: "OrbWin-LAN-proof-" + nonce + "-" + suffix, actor: actor, incumbent: incumbent}, program: program, alias: alias, port: port}
	}
	a, b := makePolicy("A", "i", "65432", false), makePolicy("B", "i", "65433", true)
	c, d, u := makePolicy("C", "r", "65432", false), makePolicy("D", "r", "65433", false), makePolicy("U", "u", "65432", false)
	for _, p := range []fixtureNativePolicy{a, b, c, d, u} {
		journals[p.intent.actor].policy = p
		journals[p.intent.actor].programSHA = hash
	}
	// Preserve recovery time inside the original cumulative deadline; neither this
	// reserve nor the external hard kill guarantees native drain/late-commit safety.
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) < 30*time.Second {
		return out, errors.New("fixture requires cumulative cleanup reserve")
	}
	work, cancelWork := context.WithDeadline(ctx, deadline.Add(-20*time.Second))
	defer cancelWork()
	var retire []*fixtureRetirement
	defer func() {
		for i := len(retire) - 1; i >= 0; i-- {
			r := retire[i]
			if !r.hasBaseline {
				out.PendingIntents++
				continue
			}
			current, e := fixtureReadDefinition(ctx, r.owner.keys, declarations)
			if e != nil || fixtureValidateDefinition(current, r.policy) != nil {
				out.PendingIntents++
				err = errors.Join(err, errors.New("fixture retirement observation unavailable/drift; exact receipt retained"))
				continue
			}
			e = fixtureDeleteTransaction(ctx, r.journal, r.owner, r.baseline, current, func() error { return fixtureDispatchDelete(ctx, r.owner, declarations.rule) })
			if e != nil {
				out.PendingIntents++
				err = errors.Join(err, errors.New("fixture removal pending; no fallback"))
				continue
			}
			out.DeletesAcknowledged++
			absent, e := fixtureNameAbsent(ctx, r.owner.intent.name, declarations.rule)
			if e != nil || !absent {
				out.PendingIntents++
				err = errors.Join(err, errors.New("fixture removal acknowledged but absence not established"))
			}
		}
	}()
	dispatch := func(p fixtureNativePolicy, unsupported bool, ready func() error) (fixtureCompletion, fixtureOwnership, error) {
		result, owner, e := fixtureCreateTransaction(work, journals[p.intent.actor], p.intent, func() fixtureCompletion { return fixtureDispatchCreate(work, p, declarations.rule, unsupported, ready) })
		return result, owner, e
	}
	for _, p := range []fixtureNativePolicy{a, c, u} {
		absent, e := fixtureNameAbsent(work, p.intent.name, declarations.rule)
		if e != nil || !absent {
			return out, errors.New("fixture fresh nonce absence not established; no adoption")
		}
	}
	ar, ao, e := dispatch(a, false, nil)
	out.Events = append(out.Events, fixtureSanitizeCompletion("A", ar))
	if ar.dispatched {
		out.CreatesDispatched++
	}
	if e != nil {
		if ar.dispatched && !fixtureKnownCollision(ar) {
			out.PendingIntents++
		}
		return out, errors.New("fixture incumbent create unacknowledged; no adoption")
	}
	incumbent := &fixtureRetirement{owner: ao, journal: journals["A"], policy: a}
	retire = append(retire, incumbent)
	incumbent.baseline, e = fixtureReadDefinition(work, ao.keys, declarations)
	if e != nil || fixtureValidateDefinition(incumbent.baseline, a) != nil {
		return out, errors.New("fixture incumbent scope readback unavailable; no cleanup fallback")
	}
	if journals["A"].recordDefinition(ao, incumbent.baseline) != nil {
		return out, errors.New("fixture incumbent snapshot flush failed; cleanup withheld")
	}
	incumbent.hasBaseline = true
	br, _, _ := dispatch(b, false, nil)
	out.Events = append(out.Events, fixtureSanitizeCompletion("B", br))
	if br.dispatched {
		out.CreatesDispatched++
	}
	if !fixtureKnownCollision(br) || !br.recorded {
		incumbent.hasBaseline = false
		out.PendingIntents++
		return out, errors.New("fixture duplicate not a durably recorded definite collision; incumbent preservation unknown")
	}
	after, e := fixtureReadDefinition(work, ao.keys, declarations)
	if e != nil || !sameFixtureDefinition(incumbent.baseline, after) {
		return out, errors.New("fixture duplicate changed incumbent; no adoption/delete fallback")
	}
	out.IncumbentPreserved = true
	// Required unknown option must be refused by the provider after dispatch.
	// A local setter rejection is recorded but is not provider qualification.
	ur, uo, _ := dispatch(u, true, nil)
	out.Events = append(out.Events, fixtureSanitizeCompletion("U", ur))
	if ur.dispatched {
		out.CreatesDispatched++
	}
	if uo.intent.name != "" {
		r := &fixtureRetirement{owner: uo, journal: journals["U"], policy: u}
		retire = append(retire, r)
		r.baseline, e = fixtureReadDefinition(work, uo.keys, declarations)
		r.hasBaseline = e == nil && fixtureValidateDefinition(r.baseline, u) == nil && r.journal.recordDefinition(uo, r.baseline) == nil
		return out, errors.New("fixture provider accepted unsupported mandatory option")
	}
	if !ur.recorded || !ur.dispatched || !ur.terminal || !ur.closed || ur.status != fixtureNotSupported || ur.returnedFullKeys {
		if ur.dispatched {
			out.PendingIntents++
		}
		return out, errors.New("fixture unsupported required option lacks definite native refusal")
	}
	absent, e := fixtureNameAbsent(work, u.intent.name, declarations.rule)
	if e != nil || !absent {
		out.PendingIntents++
		return out, errors.New("fixture required-option refusal did not establish absence")
	}
	out.RequiredUnknownRefused = true
	// Both independent sessions prepare before either dispatches. Each actor's
	// durable intent precedes this barrier. No worker goroutine is abandoned.
	var wg sync.WaitGroup
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	var cr, dr fixtureCompletion
	var co, do fixtureOwnership
	wait := func() error {
		ready <- struct{}{}
		select {
		case <-release:
			return work.Err()
		case <-work.Done():
			return work.Err()
		}
	}
	wg.Add(2)
	go func() { defer wg.Done(); cr, co, _ = dispatch(c, false, wait) }()
	go func() { defer wg.Done(); dr, do, _ = dispatch(d, false, wait) }()
	prepared := 0
	for prepared < 2 {
		select {
		case <-ready:
			prepared++
		case <-work.Done():
			close(release)
			wg.Wait()
			if cr.dispatched {
				out.PendingIntents++
			}
			if dr.dispatched {
				out.PendingIntents++
			}
			return out, errors.New("fixture concurrent preparation not settled before deadline")
		}
	}
	close(release)
	wg.Wait()
	out.Events = append(out.Events, fixtureSanitizeCompletion("C", cr), fixtureSanitizeCompletion("D", dr))
	if cr.dispatched {
		out.CreatesDispatched++
	}
	if dr.dispatched {
		out.CreatesDispatched++
	}
	winner, e := fixtureRaceOwner(c.intent, d.intent, cr, dr)
	if e != nil {
		out.PendingIntents++
		return out, errors.New("fixture concurrent create not exactly one acknowledged winner and collision")
	}
	if winner.intent.actor == "C" && co.intent.name == "" || winner.intent.actor == "D" && do.intent.name == "" {
		out.PendingIntents++
		return out, errors.New("fixture winner did not retain durably acknowledged live authority")
	}
	policy := c
	if winner.intent.actor == "D" {
		policy = d
	}
	r := &fixtureRetirement{owner: winner, journal: journals[winner.intent.actor], policy: policy}
	retire = append(retire, r)
	r.baseline, e = fixtureReadDefinition(work, winner.keys, declarations)
	if e != nil || fixtureValidateDefinition(r.baseline, policy) != nil {
		return out, errors.New("fixture race winner typed scope unavailable")
	}
	if r.journal.recordDefinition(winner, r.baseline) != nil {
		return out, errors.New("fixture race snapshot flush failed; cleanup withheld")
	}
	r.hasBaseline = true
	out.SingleRaceWinner = true
	return out, nil
}
