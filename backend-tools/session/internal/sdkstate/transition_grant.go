package sdkstate

import (
	"bytes"
	"encoding/json"
)

type transitionInheritedProcess struct {
	Identity              transitionProcess
	Process, Token, Image uint64
}

type transitionGrant struct {
	Schema                 int
	Scope                  TransitionScope
	Parent                 transitionProcess
	LegacyRoot, Controller uint64
	Processes              []transitionInheritedProcess
	Final                  transitionFinalTransfer
	Denial                 uint64
	Control                uint64
}

// Validate the entire launch description before adopting any described handle.
// Every numeric handle has exactly one close owner. Even an authenticated
// launcher bug must not make two os.File/token owners close the same value.
// This is structural admission only; native inheritance, access, peer, image,
// token and object checks remain mandatory in ConsumeColdRetirement.
func parseTransitionGrant(body []byte, grantHandle uint64, sid string, session uint32) (transitionGrant, error) {
	var g transitionGrant
	if len(body) == 0 || len(body) > 64<<10 || json.Unmarshal(body, &g) != nil {
		return g, ErrState
	}
	canonical, err := json.Marshal(g)
	if err != nil || !bytes.Equal(canonical, body) || g.Schema != 2 || !transitionID(g.Scope.Transaction) || !installationID(g.Scope.Installation) || !validDaemon(g.Scope.Daemon) || sid == "" || g.Scope.SID != sid || g.Scope.ManagementRetirement == [32]byte{} || g.Scope.SDKImage == [32]byte{} || g.Parent.PID == 0 || g.Parent.Creation == 0 || g.Parent.Session != session || g.Parent.Image == [32]byte{} || len(g.Processes) > 32 || g.Final.Nonce == [32]byte{} || g.Final.Object == "" {
		return transitionGrant{}, ErrState
	}
	seen := make(map[uint64]bool, 7+3*len(g.Processes))
	claim := func(h uint64) bool {
		if h == 0 || h == uint64(^uintptr(0)) || uint64(uintptr(h)) != h || seen[h] {
			return false
		}
		seen[h] = true
		return true
	}
	for _, h := range []uint64{grantHandle, g.LegacyRoot, g.Controller, g.Denial, g.Final.File, g.Final.Event, g.Control} {
		if !claim(h) {
			return transitionGrant{}, ErrState
		}
	}
	pids := map[uint32]bool{g.Parent.PID: true}
	for _, old := range g.Processes {
		p := old.Identity
		if p.PID == 0 || p.Creation == 0 || p.Session != session || p.Image == [32]byte{} || pids[p.PID] || !claim(old.Process) || !claim(old.Token) || !claim(old.Image) {
			return transitionGrant{}, ErrState
		}
		pids[p.PID] = true
	}
	return g, nil
}
