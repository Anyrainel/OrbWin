package sdkstate

import (
	"bytes"
	"encoding/json"
)

type transitionFinalTransfer struct {
	File, Event uint64
	Object      string
	Nonce       [32]byte
}

// Wire v2 matches managementconfig.upgradeSDKFinalRecord field order. Serving
// identifies the candidate selected by the same committed native journal; it
// does NOT acknowledge a job transfer or permit the launch owner to disappear.
type transitionFinalRecord struct {
	Schema                             int
	Transaction                        string
	Nonce, SDKDigest, ManagementRecord [32]byte
	Sequence                           uint32
	ParentPID                          uint32
	ParentCreation                     uint64
	ParentSession                      uint32
	ParentImage                        [32]byte
	ServingPID                         uint32
	ServingCreation                    uint64
	ServingSession                     uint32
	ServingImage                       [32]byte
}
type transitionCommitMarker struct {
	Schema  int
	Receipt TransitionReceipt
	Final   transitionFinalRecord
}

func parseTransitionFinal(body []byte, scope TransitionScope, digest, nonce [32]byte, p transitionProcess) (transitionFinalRecord, error) {
	var r transitionFinalRecord
	if len(body) == 0 || len(body) > 4096 || json.Unmarshal(body, &r) != nil {
		return r, ErrRecoveryRequired
	}
	canonical, _ := json.Marshal(r)
	if !bytes.Equal(body, canonical) || r.Schema != 2 || r.Transaction != scope.Transaction || r.Nonce != nonce || nonce == [32]byte{} || r.SDKDigest != digest || digest == [32]byte{} || r.ManagementRecord == [32]byte{} || r.Sequence < 1 || r.Sequence > 64 || r.ParentPID != p.PID || r.ParentCreation != p.Creation || r.ParentSession != p.Session || r.ParentImage != p.Image || p.PID == 0 || p.Creation == 0 || p.Session == 0 || p.Image == [32]byte{} || r.ServingPID == 0 || r.ServingPID == p.PID || r.ServingCreation <= p.Creation || r.ServingSession != p.Session || r.ServingImage != p.Image {
		return transitionFinalRecord{}, ErrRecoveryRequired
	}
	return r, nil
}
