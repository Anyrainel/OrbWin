package main

import (
	"strings"
	"testing"
)

func TestClosedServiceRestrictionSuffix(t *testing.T) {
	base := []string{"local-services-serve", "home", "installation", "daemon", "tools", "hash"}
	a, mode, _, err := parseServiceRestriction(append(append([]string{}, base...), "--require-new-sdk-state"))
	if err != nil || mode != 1 || len(a) != 6 {
		t.Fatal("initial suffix")
	}
	a, mode, sha, err := parseServiceRestriction(append(append([]string{}, base...), "--require-disabled-sdk-state", strings.Repeat("a", 64)))
	if err != nil || mode != 2 || len(a) != 6 || len(sha) != 64 {
		t.Fatal("restart suffix")
	}
	if _, _, _, err = parseServiceRestriction(append(append([]string{}, base...), "--require-disabled-sdk-state", strings.Repeat("A", 64))); err == nil {
		t.Fatal("noncanonical digest")
	}
	if _, _, _, err = parseServiceRestriction(append(append([]string{}, base...), "--require-disabled-sdk-state", strings.Repeat("a", 64), "--require-new-sdk-state")); err == nil {
		t.Fatal("contradictory suffixes")
	}
	a, mode, _, err = parseServiceRestriction(base)
	if err != nil || mode != 0 || len(a) != 6 {
		t.Fatal("ordinary arguments changed")
	}
}
