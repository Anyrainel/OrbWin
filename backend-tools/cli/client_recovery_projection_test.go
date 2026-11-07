//go:build windows

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// This test also runs from an exact-literal namespace-projected copy of the
// complete source. It must exercise the actual recognizers, not a duplicate
// projection model. Neither run invokes native filesystem/process providers.
func TestRecoverySuccessorNamespaceProjection(t *testing.T) {
	profile := "C:\\Users\\<owner>\u2028"
	selector := func(pin string) string {
		return profile + `\` + freshPrivateDirectory + `\runtime\docker-` + strings.Repeat(pin, 64) + `\docker-cli-plugins`
	}
	if !freshSelector(selector("a"), profile) || !validRecoverySelector(selector("c")) {
		t.Fatal("current fixed namespace is not recognized by both selector boundaries")
	}
	wrong := strings.Replace(selector("c"), freshPrivateDirectory, freshPrivateDirectory+"-foreign", 1)
	if freshSelector(wrong, profile) || validRecoverySelector(wrong) {
		t.Fatal("a neighboring namespace was admitted")
	}
	defaults := []byte(`{"auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":[` + freshQuote(selector("a"), false) + `]}`)
	for cut := 0; cut <= len(defaults); cut++ {
		if !freshDefaultsPrefix(defaults[:cut], profile) {
			t.Fatalf("actual fresh default producer prefix %d refused", cut)
		}
	}
	i, ib, _, _ := recoveryFixture(t)
	target, e := recoveryTarget(defaults, selector("c"), profile)
	if e != nil {
		t.Fatal(e)
	}
	v := recoveryV2From(writerRecoveryRecord{Schema: 1, Kind: "restore-original", Binding: i.Binding, CurrentVolume: 1, CurrentHigh: 2, CurrentLow: 3, CurrentSD: i.BaseSD, CurrentSHA: digestBytes(defaults), RestoreSHA: digestBytes(target), IntentBody: string(ib)}, selector("c"))
	encoded, e := recoveryV2Bytes(v)
	if e != nil {
		t.Fatal(e)
	}
	for cut := 0; cut < len(encoded); cut++ {
		partial, e := validateNextBody(encoded[:cut], v, defaults, profile)
		if e != nil || !partial {
			t.Fatalf("actual successor NEXT producer prefix %d refused: %v", cut, e)
		}
	}
	if partial, e := validateNextBody(encoded, v, defaults, profile); e != nil || partial {
		t.Fatal("complete actual successor refused")
	}
	foreign := v
	foreign.TargetSelector = wrong
	bad, _ := json.Marshal(foreign)
	if _, e := validateNextBody(bad, v, defaults, profile); e == nil {
		t.Fatal("foreign selector accepted by complete NEXT")
	}
}
