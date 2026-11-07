package main

import (
	"errors"
	"fmt"
	"testing"

	"orbwin.local/session/internal/sdkstate"
	"orbwin.local/session/localnet"
)

func TestServiceStateExitCodesPreserveTypedColdTransitionAndOriginFailures(t *testing.T) {
	for _, test := range []struct {
		err  error
		code int
	}{{localnet.ErrServiceNotRunning, 126}, {sdkstate.ErrLegacyState, 124}, {sdkstate.ErrRecoveryRequired, 123}, {sdkstate.ErrState, 122}, {errors.New("other failure"), 125}} {
		if code := serviceExitCode(fmt.Errorf("private cause: %w", test.err)); code != test.code {
			t.Fatal("typed startup state lost across child exit", code, test.code)
		}
	}
	if serviceExitCode(errors.Join(sdkstate.ErrLegacyState, sdkstate.ErrRecoveryRequired)) != 124 {
		t.Fatal("legacy transition hidden by follow-up cleanup uncertainty")
	}
}
