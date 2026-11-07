//go:build windows

package lanhelper

import (
	"errors"
	"sync"
	"time"
)

type scopeConsent struct {
	mu                 sync.Mutex
	authority          *AuthorizedInstallation
	reviewSHA, purpose string
	expires            time.Time
	used               bool
}

func (consent *scopeConsent) consume(authority *AuthorizedInstallation) (string, error) {
	if consent == nil {
		return "", errors.New("native exact-scope confirmation required")
	}
	consent.mu.Lock()
	defer consent.mu.Unlock()
	if consent.used || consent.authority != authority || !time.Now().Before(consent.expires) {
		return "", errors.New("native scope confirmation expired or already consumed")
	}
	consent.used = true
	return consent.reviewSHA, nil
}
