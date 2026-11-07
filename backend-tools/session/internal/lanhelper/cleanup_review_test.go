package lanhelper

import (
	"context"
	"errors"
	"strings"
	"testing"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

func TestCleanupReviewBindsProtectedOriginalScopeBeforeConsent(t *testing.T) {
	original, prepared := protectedRecordFixture(t)
	removal := original
	removal.Purpose, removal.LeaseSHA256 = "remove", prepared.LeaseSHA
	// Correlation/lifetime belong to this new attended review, not activation.
	removal.OperationID = strings.Repeat("4", 32)
	removal.Stage.Base.Transaction = strings.Repeat("5", 32)
	removal.Stage.Base.IssuedAt = "2026-10-26T00:00:00Z"
	if err := validateCleanupReview(removal, prepared.Spec); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*OwnerReview, *lanlease.Spec){
		func(r *OwnerReview, _ *lanlease.Spec) { r.Purpose = "activate" },
		func(r *OwnerReview, _ *lanlease.Spec) { r.LeaseSHA256 = "other" },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Base.Installation = strings.Repeat("6", 32) },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Base.UserSID = "S-1-5-18" },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Base.Scope.LocalPort++ },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Base.Scope.ClientNetworks = []string{"192.168.5.0/24"} },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Base.Scope.WorkerSHA256 = strings.Repeat("7", 64) },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Publication.ContainerID = strings.Repeat("8", 64) },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Publication.Daemon = "other-daemon" },
		func(r *OwnerReview, _ *lanlease.Spec) { r.Stage.Publication.Generation++ },
	} {
		record, spec := removal, prepared.Spec
		mutate(&record, &spec)
		if validateCleanupReview(record, spec) == nil {
			t.Fatal("changed protected consent scope accepted")
		}
	}
}

func TestCleanupConsentRetainsOriginalScopeThroughNICLossAndProfileDrift(t *testing.T) {
	record, prepared := protectedRecordFixture(t)
	for _, failure := range []string{"NIC unplugged", "profile changed"} {
		calls := 0
		observe := func(context.Context, lanfirewall.Scope) (lanfirewall.InterfaceBinding, error) {
			calls++
			return lanfirewall.InterfaceBinding{}, errors.New(failure)
		}
		if _, err := consentInterface(context.Background(), record, nil, observe); err == nil || calls != 1 {
			t.Fatal("activation skipped current interface eligibility")
		}
		record.Purpose, record.LeaseSHA256 = "remove", prepared.LeaseSHA
		for i := 0; i < 2; i++ { // Both before and after attended consent.
			binding, err := consentInterface(context.Background(), record, &prepared.Spec, observe)
			if err != nil || calls != 1 || binding.Profile != "private" || binding.Alias != prepared.Spec.Worker.InterfaceAlias {
				t.Fatal("cleanup rebound or required a current interface", err)
			}
		}
		record.Purpose = "activate"
		record.LeaseSHA256 = ""
		if _, err := consentInterface(context.Background(), record, nil, observe); err == nil || calls != 2 {
			t.Fatal("activation skipped post-consent freshness")
		}
	}
}
