package lanhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanprovider"
)

func priorCleanupRecordFixture(t *testing.T) (OwnerReview, lanprovider.CleanupObservation) {
	t.Helper()
	original, prepared := protectedRecordFixture(t)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	record := original
	record.RuntimeInstance = strings.Repeat("4", 32)
	record.ReviewID = strings.Repeat("5", 32)
	record.ReviewSHA256 = strings.Repeat("6", 64)
	record.OperationID = strings.Repeat("7", 32)
	record.Transaction = strings.Repeat("8", 32)
	record.LeaseSHA256 = prepared.LeaseSHA
	record.Purpose = "cleanup-remove"
	record.Cleanup = &CleanupReview{RuntimeInstance: original.RuntimeInstance, ReviewID: original.ReviewID, ReviewSHA256: original.ReviewSHA256, OperationID: original.OperationID, IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(3 * time.Minute).Format(time.RFC3339Nano)}
	proof := lanprovider.CleanupObservation{RuntimeInstance: original.RuntimeInstance, ReviewID: original.ReviewID, ReviewSHA256: original.ReviewSHA256, OperationID: original.OperationID, LeaseSHA: prepared.LeaseSHA, Spec: prepared.Spec, Phase: "created", ChildExited: true, NamespaceAbsent: true, NativeSequence: 3, ReadAt: now}
	return record, proof
}

func TestLANPriorCleanupRecordRetainsOldCallerAndBindsCurrentNativeSID(t *testing.T) {
	record, proof := priorCleanupRecordFixture(t)
	now, _ := time.Parse(time.RFC3339Nano, record.Cleanup.IssuedAt)
	identity := proof.Spec.Request
	current := lanfirewall.CallerIdentity{PID: identity.CallerPID, CreationTime: identity.CallerCreationTime, UserSID: identity.UserSID, ImageSHA256: identity.CallerImageSHA256}
	current.PID += 100
	current.CreationTime = "999"
	current.ImageSHA256 = strings.Repeat("9", 64)
	before, _ := json.Marshal(record.Stage)
	if err := record.Validate(current, now); err != nil {
		t.Fatal("fresh native peer could not review immutable prior ownership", err)
	}
	if err := validatePriorCleanupMapping(record, proof); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(record.Stage)
	if !bytes.Equal(before, after) || record.Stage.Base.CallerPID == current.PID {
		t.Fatal("original stage caller rewritten")
	}
	for _, mutate := range []func(*OwnerReview){func(r *OwnerReview) { r.Cleanup.RuntimeInstance = "" }, func(r *OwnerReview) { r.Cleanup.ReviewID = r.ReviewID }, func(r *OwnerReview) { r.Cleanup.OperationID = r.OperationID }, func(r *OwnerReview) { r.Cleanup.ReviewSHA256 = strings.Repeat("a", 64) }, func(r *OwnerReview) { r.Transaction = r.Stage.Base.Transaction }, func(r *OwnerReview) { r.Purpose = "activate" }, func(r *OwnerReview) { r.Cleanup.ExpiresAt = now.Add(-time.Second).Format(time.RFC3339Nano) }} {
		changed := record
		cleanup := *record.Cleanup
		changed.Cleanup = &cleanup
		mutate(&changed)
		if changed.Validate(current, now) == nil {
			t.Fatal("malformed or nonfresh cleanup mapping accepted")
		}
	}
	current.UserSID = "S-1-5-21-9-8-7-1001"
	if record.Validate(current, now) == nil {
		t.Fatal("different current native account adopted original ownership")
	}
	var journal bytes.Buffer
	var state protectedJournalState
	if err := state.append(protectedEntry{Kind: "owner-review", Review: &record}, journal.Write, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProtectedHistory(journal.Bytes())
	if err != nil || parsed.Review.Cleanup == nil || parsed.Review.RuntimeInstance != record.RuntimeInstance || parsed.Review.Cleanup.RuntimeInstance != proof.RuntimeInstance {
		t.Fatal("durable fresh/original owner mapping lost", err)
	}
}

func TestLANPriorCleanupConsentRequiresMatchingNativeProtectedMapping(t *testing.T) {
	record, proof := priorCleanupRecordFixture(t)
	for _, mutate := range []func(*lanprovider.CleanupObservation){func(p *lanprovider.CleanupObservation) { p.RuntimeInstance = strings.Repeat("a", 32) }, func(p *lanprovider.CleanupObservation) { p.ReviewID = strings.Repeat("b", 32) }, func(p *lanprovider.CleanupObservation) { p.OperationID = strings.Repeat("c", 32) }, func(p *lanprovider.CleanupObservation) { p.LeaseSHA = strings.Repeat("d", 64) }, func(p *lanprovider.CleanupObservation) { p.ChildExited = false }, func(p *lanprovider.CleanupObservation) { p.NamespaceAbsent = false }, func(p *lanprovider.CleanupObservation) { p.Spec.Request.CallerPID++ }, func(p *lanprovider.CleanupObservation) { p.Phase = "remove-intent"; p.RuleAbsent = true }} {
		changed := proof
		mutate(&changed)
		if validatePriorCleanupMapping(record, changed) == nil {
			t.Fatal("cleanup consent used a different protected original mapping")
		}
	}
	record.Purpose = "cleanup-settle"
	proof.Phase, proof.RuleAbsent, proof.NativeSequence = "remove-intent", true, 4
	if err := validatePriorCleanupMapping(record, proof); err != nil {
		t.Fatal(err)
	}
	binding, err := consentInterface(context.Background(), record, &proof.Spec, nil)
	if err != nil || binding.Alias != proof.Spec.Worker.InterfaceAlias {
		t.Fatal("cleanup consent required present original NIC", err)
	}
	proof.RuleAbsent = false
	if validatePriorCleanupMapping(record, proof) == nil {
		t.Fatal("settlement consent allowed present rule")
	}
}

func TestLANPriorCleanupAdminConversationCannotStageOrUseOtherCleanupPurpose(t *testing.T) {
	for _, purpose := range []string{"cleanup-remove", "cleanup-settle"} {
		record, _ := priorCleanupRecordFixture(t)
		record.Purpose = purpose
		var session adminConversation
		if session.accept(AdminFrame{Version: 2, Kind: "review", Sequence: 1, Review: &record}) != nil || session.reviewed() != nil {
			t.Fatal("prior cleanup review rejected")
		}
		sequence := uint64(2)
		for _, wrong := range []string{"stage", "remove", "settle-remove", "cleanup-remove", "cleanup-settle"} {
			if wrong == purpose {
				continue
			}
			if session.accept(AdminFrame{Version: 2, Kind: wrong, Sequence: sequence}) == nil {
				t.Fatal("prior cleanup review authorized another capability", wrong)
			}
			sequence++
		}
		frame := AdminFrame{Version: 2, Kind: purpose, Sequence: sequence}
		var data bytes.Buffer
		if WriteAdmin(&data, frame) != nil {
			t.Fatal("closed prior cleanup command rejected")
		}
		decoded, err := ReadAdmin(&data)
		if err != nil || session.accept(decoded) != nil {
			t.Fatal("purpose-matched prior cleanup rejected", err)
		}
		frame.Sequence++
		if session.accept(frame) == nil {
			t.Fatal("prior cleanup command replayed")
		}
	}
}
