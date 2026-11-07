//go:build windows

package sdkstate

import (
	"context"
	"orbwin.local/session/internal/sdkself"
	"testing"
	"time"
)

func TestSDKColdProvisionalTargetCannotBeCapturedFromRecordOrFinalPID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	record := ColdSupervisorRecord{Process: 711, PID: 12, Creation: 24, Session: 1, Image: [32]byte{1}}
	transition := &ColdTransition{}
	if transition.ProvisionNativeSupervisor(ctx, &ColdControl{}, record) == nil {
		t.Fatal("record/control shape supplied actual native Root and target")
	}
	if transition.AdoptNativeLANMaster(ctx, &ColdControl{}, &sdkself.Lease{}, ColdLeafMasterRecord{Process: 721}) == nil {
		t.Fatal("record/self/control shape supplied actual Root/master roundtrip")
	}
	a := &nativeColdAuthority{}
	if a.holdPendingSupervisor(ctx, transitionFinalRecord{ServingPID: record.PID, ServingCreation: record.Creation, ServingSession: record.Session, ServingImage: record.Image}) == nil || a.pendingSupervisor != nil {
		t.Fatal("final metadata created an uncaptured process/PID target")
	}
}
