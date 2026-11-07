package sdkstate

import (
	"encoding/json"
	"strings"
	"testing"
)

func sdkColdGrantFixture() transitionGrant {
	return transitionGrant{
		Schema:     2,
		Scope:      TransitionScope{Transaction: strings.Repeat("a", 32), Installation: strings.Repeat("b", 32), Daemon: "daemon", SID: "S-1-5-21-1", ManagementRetirement: [32]byte{1}, SDKImage: [32]byte{2}},
		Parent:     transitionProcess{PID: 1, Creation: 2, Session: 3, Image: [32]byte{4}},
		LegacyRoot: 12, Controller: 13, Denial: 14,
		Final:     transitionFinalTransfer{File: 15, Event: 16, Object: "01234567:0123456789abcdef", Nonce: [32]byte{1}},
		Processes: []transitionInheritedProcess{{Identity: transitionProcess{PID: 2, Creation: 3, Session: 3, Image: [32]byte{5}}, Process: 17, Token: 18, Image: 19}},
		Control:   30,
	}
}

func TestSDKColdGrantRejectsAliasedOwnership(t *testing.T) {
	// Every role participates in one ownership domain, including the already
	// adopted grant. Check all pairs rather than only adjacent role aliases.
	for first := 0; first < 10; first++ {
		for second := first + 1; second < 10; second++ {
			g := sdkColdGrantFixture()
			raw := uint64(11)
			roles := []*uint64{&raw, &g.LegacyRoot, &g.Controller, &g.Denial, &g.Final.File, &g.Final.Event, &g.Processes[0].Process, &g.Processes[0].Token, &g.Processes[0].Image, &g.Control}
			*roles[second] = *roles[first]
			body, _ := json.Marshal(g)
			if _, err := parseTransitionGrant(body, raw, g.Scope.SID, 3); err == nil {
				t.Fatalf("accepted aliased close owners %d/%d", first, second)
			}
		}
	}
}

func TestSDKColdGrantStructuralAdmission(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*transitionGrant)
	}{
		{"zero handle", func(g *transitionGrant) { g.Denial = 0 }},
		{"invalid handle", func(g *transitionGrant) { g.Controller = uint64(^uintptr(0)) }},
		{"duplicate process", func(g *transitionGrant) {
			p := g.Processes[0]
			p.Process, p.Token, p.Image = 20, 21, 22
			g.Processes = append(g.Processes, p)
		}},
		{"parent as retired", func(g *transitionGrant) { g.Processes[0].Identity = g.Parent }},
		{"foreign session", func(g *transitionGrant) { g.Processes[0].Identity.Session++ }},
		{"missing generation", func(g *transitionGrant) { g.Processes[0].Identity.Creation = 0 }},
		{"missing image", func(g *transitionGrant) { g.Processes[0].Identity.Image = [32]byte{} }},
		{"missing nonce", func(g *transitionGrant) { g.Final.Nonce = [32]byte{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := sdkColdGrantFixture()
			tc.edit(&g)
			body, _ := json.Marshal(g)
			if _, err := parseTransitionGrant(body, 11, g.Scope.SID, 3); err == nil {
				t.Fatal("accepted malformed launch grant")
			}
		})
	}
	g := sdkColdGrantFixture()
	body, _ := json.Marshal(g)
	if _, err := parseTransitionGrant(body, 11, g.Scope.SID, 3); err != nil {
		t.Fatal(err)
	}
	for _, malformed := range [][]byte{append(append([]byte(nil), body...), '\n'), []byte(strings.Replace(string(body), `"Schema":2`, `"Schema":2,"Schema":2`, 1)), []byte(strings.Replace(string(body), `"Schema":2`, `"Unknown":1,"Schema":2`, 1))} {
		if _, err := parseTransitionGrant(malformed, 11, g.Scope.SID, 3); err == nil {
			t.Fatal("accepted noncanonical grant")
		}
	}
}
