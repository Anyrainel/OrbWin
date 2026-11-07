package localnet

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"orbwin.local/session/internal/sdkstate"
	"sync"
	"time"
)

// ColdSigningValidator is the activation validator for a cold transition. It
// opens only the held snapshot's legacy authority with the existing fixed
// installation/fingerprint DPAPI domain. It never creates a replacement CA or
// consumes historical trust/rotation receipts as mutation authority.
type ColdSigningValidator struct {
	mu            sync.Mutex
	used          bool
	scope         sdkstate.TransitionScope
	authorityHash [32]byte
	authority     *Authority
	review        ColdStateReview
}

func NewColdSigningValidator() *ColdSigningValidator { return &ColdSigningValidator{} }

func (v *ColdSigningValidator) ValidateColdSDK(snapshot *sdkstate.ColdSnapshot) error {
	if v == nil || snapshot == nil {
		return sdkstate.ErrState
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.used {
		return sdkstate.ErrRecoveryRequired
	}
	v.used = true // A failed semantic/DPAPI attempt cannot be retried by absence.
	preservation := NewColdStateValidator()
	if err := preservation.ValidateColdSDK(snapshot); err != nil {
		return err
	}
	review, err := preservation.Review()
	if err != nil {
		return err
	}
	data, err := snapshot.Read(authorityStateName)
	if err != nil {
		return sdkstate.ErrRecoveryRequired
	}
	defer clear(data)
	authority, err := admitColdSigner(snapshot.Scope(), review, data, time.Now(), openAuthorityState)
	if err != nil {
		return err
	}
	v.scope, v.authorityHash, v.authority = snapshot.Scope(), sha256.Sum256(data), authority
	review.Activation = "candidate-signing-validated"
	v.review = review
	return nil
}

// CandidateAuthority keeps semantic admission tied to the actual retained
// transition and its imported bytes. It is neither ordinary-start admission nor
// proof of historical trust/rule ownership; final management commit is separate.
func (v *ColdSigningValidator) CandidateAuthority(ctx context.Context, transition *sdkstate.ColdTransition) (*Authority, ColdStateReview, error) {
	if v == nil || transition == nil {
		return nil, ColdStateReview{}, sdkstate.ErrState
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.authority == nil {
		return nil, ColdStateReview{}, sdkstate.ErrRecoveryRequired
	}
	root, receipt, err := transition.CandidateRoot(ctx)
	if err != nil || root == nil || receipt.Scope != v.scope {
		return nil, ColdStateReview{}, sdkstate.ErrRecoveryRequired
	}
	matched := false
	for _, file := range receipt.Files {
		if file.Name == authorityStateName {
			matched = file.Disposition == "active-data" && file.SHA256 == v.authorityHash
		}
	}
	if !matched || v.authority.validate(time.Now()) != nil {
		return nil, ColdStateReview{}, sdkstate.ErrRecoveryRequired
	}
	return v.authority, v.review, nil
}

func admitColdSigner(scope sdkstate.TransitionScope, review ColdStateReview, data []byte, now time.Time, open func(authorityState, string, string, time.Time) (*Authority, error)) (*Authority, error) {
	if review.Authority != "opaque-key" || review.Validity != "current" || review.Rotation != "none" || review.Activation != "native-signing-admission-required" || open == nil {
		return nil, errors.New("cold SDK signing state requires explicit recovery; checkpoint retained")
	}
	var state authorityState
	if len(data) == 0 || len(data) > authorityStateBudget || coldJSON(data, &state) != nil || state.RootSHA256 != review.RootSHA256 {
		return nil, sdkstate.ErrRecoveryRequired
	}
	if _, err := validateAuthorityPublic(state, scope.Installation); err != nil {
		return nil, err
	}
	authority, err := open(state, scope.Installation, review.RootSHA256, now)
	if err != nil || authority == nil || authority.validate(now) != nil || !bytes.Equal(authority.RootCertificate(), state.RootCertificate) {
		return nil, errors.New("cold SDK protected signer and preserved public authority could not be verified")
	}
	// Exercise the actual key without publishing a leaf, listener, trust entry or
	// revocation mutation. A matching public certificate alone is insufficient.
	var challenge [32]byte
	if _, err := rand.Read(challenge[:]); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(challenge[:])
	signature, err := ecdsa.SignASN1(rand.Reader, authority.signer, digest[:])
	if err != nil || !ecdsa.VerifyASN1(&authority.signer.PublicKey, digest[:], signature) {
		return nil, sdkstate.ErrRecoveryRequired
	}
	return authority, nil
}

var _ sdkstate.ColdValidator = (*ColdSigningValidator)(nil)
