package main

import (
	"encoding/json"
	"fmt"
	"orbwin.local/session/internal/sdkstate"
	"testing"
)

func TestColdControlAdmissionBusyReturnsToPumpWithoutConvertingUnknownErrors(t *testing.T) {
	for _, operation := range []string{"commit", "offer-controller"} {
		stage, err := coldServiceAdmissionStage(operation, "original", sdkstate.ErrBusy)
		if err != nil || stage != operation+"-pending" {
			t.Fatal("known no-effect busy blocked sole pump", stage, err)
		}
		for _, refused := range []error{sdkstate.ErrRecoveryRequired, fmt.Errorf("ambiguous: %w", sdkstate.ErrBusy)} {
			stage, err = coldServiceAdmissionStage(operation, "original", refused)
			if stage != "original" || err != refused {
				t.Fatal("unknown/admitted error gained a retry stage", stage, err)
			}
		}
	}
	if _, err := coldServiceAdmissionStage("retain-controller", "original", sdkstate.ErrBusy); err == nil {
		t.Fatal("post-offer operation gained pre-admission retry")
	}
}

func TestColdControlLeafFramesHaveOneClosedRoleAndExactCanonicalShape(t *testing.T) {
	for _, request := range []coldServiceRequest{
		{Schema: 1, Sequence: 3, Operation: "adopt-leaf-master", Master: &sdkstate.ColdLeafMasterRecord{Process: 711}},
		{Schema: 1, Sequence: 3, Operation: "ack-leaf", Leaf: &sdkstate.ColdLeafAckRecord{Offer: 1, Leaf: 2, Kind: "birth", Process: 711}},
		{Schema: 1, Sequence: 3, Operation: "ack-leaf", Leaf: &sdkstate.ColdLeafAckRecord{Offer: 2, Leaf: 2, Kind: "stopped", Process: 711}},
		{Schema: 1, Sequence: 3, Operation: "ack-leaf", Leaf: &sdkstate.ColdLeafAckRecord{Offer: 3, Leaf: 2, Kind: "closed"}},
	} {
		body, _ := json.Marshal(request)
		if _, err := decodeColdServiceRequest(body, 3); err != nil {
			t.Fatal("canonical closed variant refused", string(body), err)
		}
		request.Supervisor = &sdkstate.ColdSupervisorRecord{Process: 811, PID: 12, Creation: 24, Session: 1, Image: [32]byte{1}}
		body, _ = json.Marshal(request)
		if _, err := decodeColdServiceRequest(body, 3); err == nil {
			t.Fatal("mixed native role descriptions admitted")
		}
	}
	for _, leaf := range []sdkstate.ColdLeafAckRecord{
		{Offer: 1, Leaf: 2, Kind: "closed", Process: 711},
		{Offer: 1, Leaf: 2, Kind: "birth"},
		{Offer: 1, Leaf: 0, Kind: "stopped", Process: 711},
		{Offer: 0, Leaf: 2, Kind: "birth", Process: 711},
		{Offer: 1, Leaf: 2, Kind: "unknown", Process: 711},
	} {
		body, _ := json.Marshal(coldServiceRequest{Schema: 1, Sequence: 3, Operation: "ack-leaf", Leaf: &leaf})
		if _, err := decodeColdServiceRequest(body, 3); err == nil {
			t.Fatal("invalid closed leaf description admitted", leaf)
		}
	}
}

func TestColdControlClosedFramesNeverAcceptFlagsOrReplay(t *testing.T) {
	good, _ := json.Marshal(coldServiceRequest{Schema: 1, Sequence: 1, Operation: "inventory"})
	if _, err := decodeColdServiceRequest(good, 1); err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{good, []byte(`{"Schema":1,"Sequence":2,"Operation":"inventory","OwnerAccepted":true}`), []byte(`{"Schema":1,"Schema":1,"Sequence":2,"Operation":"inventory"}`), []byte(`{"Schema":1,"Sequence":2,"Operation":"kill"}`), []byte(`{"Schema":1,"Sequence":2,"Operation":"inventory"} `), []byte(`{"Schema":1,"Sequence":2`)} {
		if _, err := decodeColdServiceRequest(body, 2); err == nil {
			t.Fatal("replayed/partial/foreign control operation admitted")
		}
	}
	for _, operation := range []string{"inventory", "start-runtime", "commit", "offer-controller", "retain-controller", "keepalive", "seal-runtime", "drain-runtime", "retire"} {
		body, _ := json.Marshal(coldServiceRequest{Schema: 1, Sequence: 2, Operation: operation})
		if _, err := decodeColdServiceRequest(body, 2); err != nil {
			t.Fatal(err)
		}
	}
}

func TestColdControlProvisionalTargetOnlyOccupiesItsReservedFrame(t *testing.T) {
	target := &sdkstate.ColdSupervisorRecord{Process: 711, PID: 12, Creation: 24, Session: 1, Image: [32]byte{1}}
	request := coldServiceRequest{Schema: 1, Sequence: 2, Operation: "provision-supervisor", Supervisor: target}
	body, _ := json.Marshal(request)
	if _, err := decodeColdServiceRequest(body, 2); err != nil {
		t.Fatal(err)
	}
	request.Operation = "start-runtime"
	body, _ = json.Marshal(request)
	if _, err := decodeColdServiceRequest(body, 2); err == nil {
		t.Fatal("native target supplied at wrong operation")
	}
	request.Operation = "provision-supervisor"
	request.Supervisor = nil
	body, _ = json.Marshal(request)
	if _, err := decodeColdServiceRequest(body, 2); err == nil {
		t.Fatal("absent native target description accepted")
	}
}
