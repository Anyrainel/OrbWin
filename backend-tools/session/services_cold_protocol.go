package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"orbwin.local/session/internal/sdkstate"
)

// Closed private lifecycle frames on the original grant-owned native pipe;
// never public ServiceRequest dispatch or a source of resource authority.
type coldServiceRequest struct {
	Schema, Sequence uint32
	Operation        string
	Supervisor       *sdkstate.ColdSupervisorRecord `json:",omitempty"`
	Master           *sdkstate.ColdLeafMasterRecord `json:",omitempty"`
	Leaf             *sdkstate.ColdLeafAckRecord    `json:",omitempty"`
}

// Only the exact lower pre-admission sentinel may return to the same pump for
// custody progress. An admitted/ambiguous failure is terminal and never gains
// this retry stage. No source or native ownership is issued by these DATA.
func coldServiceAdmissionStage(operation, stage string, err error) (string, error) {
	if err == sdkstate.ErrBusy {
		switch operation {
		case "commit", "offer-controller":
			return operation + "-pending", nil
		}
	}
	return stage, err
}

func decodeColdServiceRequest(body []byte, next uint32) (coldServiceRequest, error) {
	var r coldServiceRequest
	if len(body) == 0 || len(body) > 4096 || next == 0 || json.Unmarshal(body, &r) != nil {
		return r, errors.New("cold SDK control frame refused")
	}
	canonical, _ := json.Marshal(r)
	if !bytes.Equal(body, canonical) || r.Schema != 1 || r.Sequence != next {
		return r, errors.New("cold SDK control sequence refused")
	}
	switch r.Operation {
	case "provision-supervisor":
		if r.Supervisor == nil || !r.Supervisor.Valid() || r.Master != nil || r.Leaf != nil {
			return r, errors.New("cold SDK provisional native target refused")
		}
		return r, nil
	case "adopt-leaf-master":
		if r.Supervisor != nil || r.Leaf != nil || r.Master == nil || r.Master.Process == 0 || r.Master.Process == uint64(^uintptr(0)) || uint64(uintptr(r.Master.Process)) != r.Master.Process {
			return r, errors.New("cold SDK native master description refused")
		}
		return r, nil
	case "ack-leaf":
		if r.Supervisor != nil || r.Master != nil || r.Leaf == nil || !r.Leaf.Valid() {
			return r, errors.New("cold SDK leaf acknowledgement refused")
		}
		return r, nil
	case "inventory", "start-runtime", "commit", "offer-controller", "retain-controller", "keepalive", "seal-runtime", "drain-runtime", "retire":
		if r.Supervisor != nil || r.Master != nil || r.Leaf != nil {
			return r, errors.New("cold SDK target at wrong operation refused")
		}
		return r, nil
	default:
		return r, errors.New("cold SDK control operation refused")
	}
}
