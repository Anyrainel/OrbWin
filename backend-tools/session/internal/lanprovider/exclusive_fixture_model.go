package lanprovider

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// Fixture-only policy, verified against the installed Microsoft CDXML enums.
// No allow/enable/update operation is represented. These are not MI RTTI flags.
const fixtureDisabled uint16 = 2
const fixtureBlock uint16 = 4
const fixtureInbound uint16 = 1
const fixturePrivate uint16 = 2
const fixtureEdgeBlock uint16 = 0
const fixtureAlreadyExists uint32 = 11
const fixtureNotSupported uint32 = 7

var fixtureNamePattern = regexp.MustCompile(`^OrbWin-LAN-proof-[0-9a-f]{32}-[iru]$`)

type fixtureIntent struct {
	name, actor string
	incumbent   bool
}
type fixtureCompletion struct {
	dispatched, terminal, closed, returnedFullKeys, recorded bool
	status                                                   uint32
	keys                                                     observedRuleKeys
}
type fixtureOwnership struct {
	intent fixtureIntent
	keys   observedRuleKeys
}
type fixtureDefinition struct {
	keys    observedRuleKeys
	rule    typedInstance
	filters [7]typedInstance
}

func fixturePrivilegePrecondition(elevated bool) error {
	if !elevated {
		return errors.New("pre-elevated administrator token required; automatic elevation is unavailable")
	}
	return nil
}
func fixtureGrantOwnership(intent fixtureIntent, result fixtureCompletion) (fixtureOwnership, error) {
	if !result.recorded || !fixturePositiveCompletion(intent, result) {
		return fixtureOwnership{}, errors.New("fixture create did not establish exclusive acknowledged ownership")
	}
	return fixtureOwnership{intent: intent, keys: result.keys}, nil
}

// A candidate acknowledged by the provider is not yet a durable live capability.
// Journaling may persist these own candidate keys without minting authority.
func fixturePositiveCompletion(intent fixtureIntent, result fixtureCompletion) bool {
	return fixtureNamePattern.MatchString(intent.name) && intent.actor != "" && !intent.incumbent && result.dispatched && result.terminal && result.closed && result.status == 0 && result.returnedFullKeys && fixtureCompleteKeys(result.keys) && result.keys.instanceID == intent.name
}
func fixtureCompleteKeys(keys observedRuleKeys) bool {
	for _, s := range []string{keys.instanceID, keys.creationClassName, keys.policyRuleName, keys.systemCreationClassName, keys.systemName} {
		if s == "" || len(s) > 2048 || strings.ContainsRune(s, 0) {
			return false
		}
	}
	return true
}

// A journal implementation must append and flush before returning success.
// A failed acknowledgement flush deliberately withholds the live capability.
type fixtureJournal interface {
	record(string, fixtureIntent, fixtureCompletion) error
}

func fixtureCreateTransaction(ctx context.Context, j fixtureJournal, intent fixtureIntent, dispatch func() fixtureCompletion) (fixtureCompletion, fixtureOwnership, error) {
	if ctx.Err() != nil {
		return fixtureCompletion{}, fixtureOwnership{}, ctx.Err()
	}
	if e := j.record("create-intent", intent, fixtureCompletion{}); e != nil {
		return fixtureCompletion{}, fixtureOwnership{}, e
	}
	if ctx.Err() != nil {
		return fixtureCompletion{}, fixtureOwnership{}, ctx.Err()
	}
	result := dispatch()
	if e := j.record("create-result", intent, result); e != nil {
		return result, fixtureOwnership{}, e
	}
	result.recorded = true
	if ctx.Err() != nil {
		return result, fixtureOwnership{}, ctx.Err()
	}
	owner, e := fixtureGrantOwnership(intent, result)
	return result, owner, e
}
func fixtureDeleteTransaction(ctx context.Context, j fixtureJournal, owner fixtureOwnership, retained, current fixtureDefinition, dispatch func() error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	// Validate the non-durable portions before writing a retirement intent.
	if e := fixtureRetireGuard(owner, retained, current, true, true); e != nil {
		return e
	}
	if e := j.record("remove-intent", owner.intent, fixtureCompletion{keys: owner.keys, returnedFullKeys: true}); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := dispatch(); e != nil {
		return e
	}
	return j.record("remove-result", owner.intent, fixtureCompletion{terminal: true, closed: true, keys: owner.keys, returnedFullKeys: true})
}
func fixtureKnownCollision(result fixtureCompletion) bool {
	return result.dispatched && result.terminal && result.closed && result.status == fixtureAlreadyExists && !result.returnedFullKeys
}
func fixtureRaceOwner(a, b fixtureIntent, ar, br fixtureCompletion) (fixtureOwnership, error) {
	if a.name != b.name || a.actor == b.actor || a.incumbent || b.incumbent || !ar.recorded || !br.recorded {
		return fixtureOwnership{}, errors.New("fixture race identities or durable acknowledgements differ")
	}
	if fixtureKnownCollision(br) {
		return fixtureGrantOwnership(a, ar)
	}
	if fixtureKnownCollision(ar) {
		return fixtureGrantOwnership(b, br)
	}
	return fixtureOwnership{}, errors.New("fixture race has no single acknowledged winner and definite collision")
}
func sameFixtureDefinition(a, b fixtureDefinition) bool {
	if a.keys != b.keys || !sameTypedInstance(a.rule, b.rule) {
		return false
	}
	for i := 0; i < 7; i++ {
		if !sameTypedInstance(a.filters[i], b.filters[i]) {
			return false
		}
	}
	return true
}

// This is an exclusive live fixture-owner check plus exact observations, NOT a
// provider CAS. Never expose a serialized receipt as a constructor for this cap.
func fixtureRetireGuard(owner fixtureOwnership, retained, current fixtureDefinition, liveOwner, journalSynced bool) error {
	if !liveOwner || !journalSynced || !fixtureNamePattern.MatchString(owner.intent.name) || owner.intent.incumbent || owner.keys.instanceID != owner.intent.name || retained.keys != owner.keys || !sameFixtureDefinition(retained, current) {
		return errors.New("fixture exact live ownership/state/retirement intent not proven; no delete fallback")
	}
	return nil
}
