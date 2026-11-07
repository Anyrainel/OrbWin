package wsltrust

import (
	"errors"
	"sync"
)

var errGuestIssuer = errors.New("guest issuer sealed or issued guest work requires native retirement")

// Producer-owned admission evidence, never a caller or persisted flag. Every
// possible Host native dispatch enters this mutex BEFORE calling its launcher.
type guestAdmission struct {
	mu             sync.Mutex
	sealed         bool
	issued, active uint64
}
type SealedGuestIssuer struct{ producer *guestAdmission }

func (a *guestAdmission) begin() (func(), error) {
	if a == nil {
		return nil, errGuestIssuer
	}
	a.mu.Lock()
	if a.sealed || a.issued == ^uint64(0) {
		a.mu.Unlock()
		return nil, errGuestIssuer
	}
	a.issued++
	a.active++
	a.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { a.mu.Lock(); a.active--; a.mu.Unlock() }) }, nil
}
func (a *guestAdmission) seal() (*SealedGuestIssuer, error) {
	if a == nil {
		return nil, errGuestIssuer
	}
	a.mu.Lock()
	a.sealed = true
	a.mu.Unlock()
	return &SealedGuestIssuer{a}, nil
}

// NoGuestIssued is valid only for this actual sealed producer after its real
// callers joined. Any issued status/help/bootstrap is conservatively nonzero;
// returned Windows proxies do not prove detached guest actions retired.
func (s *SealedGuestIssuer) NoGuestIssued() error {
	if s == nil || s.producer == nil {
		return errGuestIssuer
	}
	a := s.producer
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.sealed || a.issued != 0 || a.active != 0 {
		return errGuestIssuer
	}
	return nil
}
