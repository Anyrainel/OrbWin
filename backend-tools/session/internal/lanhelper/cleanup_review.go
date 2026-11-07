package lanhelper

import (
	"context"
	"errors"
	"reflect"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanprovider"
)

// A new removal review has a fresh operation identity/lifetime, but cannot
// relabel a protected rule's immutable network or workload scope. This check
// grants no authority: only the native cleanup capability supplies the spec.
func validateCleanupReview(record OwnerReview, protected lanlease.Spec) error {
	if (record.Purpose != "remove" && record.Purpose != "settle-remove" && record.Purpose != "cleanup-remove" && record.Purpose != "cleanup-settle") || !ValidSHA(record.LeaseSHA256) ||
		record.Stage.Base.Installation != protected.Request.Installation ||
		record.Stage.Base.UserSID != protected.Request.UserSID ||
		!reflect.DeepEqual(record.Stage.Base.Scope, protected.Request.Scope) ||
		record.Stage.Publication != protected.Publication {
		return errors.New("removal consent differs from protected original scope")
	}
	return nil
}

func validatePriorCleanupMapping(record OwnerReview, observed lanprovider.CleanupObservation) error {
	c := record.Cleanup
	if c == nil || c.RuntimeInstance != observed.RuntimeInstance || c.ReviewID != observed.ReviewID || c.ReviewSHA256 != observed.ReviewSHA256 || c.OperationID != observed.OperationID || record.LeaseSHA256 != observed.LeaseSHA || !observed.ChildExited || !observed.NamespaceAbsent || !reflect.DeepEqual(record.Stage.Base, observed.Spec.Request) || record.Stage.Publication != observed.Spec.Publication || validateCleanupReview(record, observed.Spec) != nil {
		return errors.New("fresh cleanup review differs from protected original ownership")
	}
	if record.Purpose == "cleanup-remove" && observed.Phase == "created" && observed.NativeSequence == 3 || record.Purpose == "cleanup-settle" && (observed.Phase == "remove-intent" && observed.NativeSequence == 4 || observed.Phase == "removed" && observed.NativeSequence == 5) && observed.RuleAbsent {
		return nil
	}
	return errors.New("cleanup capability differs from protected native phase")
}

// Removal displays the authenticated original filter scope even after its NIC
// disappears or changes profile. It never rebinds that scope to today's NIC.
// Activation still obtains fresh native eligibility before and after consent.
func consentInterface(ctx context.Context, record OwnerReview, cleanup *lanlease.Spec, observe func(context.Context, lanfirewall.Scope) (lanfirewall.InterfaceBinding, error)) (lanfirewall.InterfaceBinding, error) {
	if record.Purpose == "activate" && cleanup == nil && observe != nil {
		return observe(ctx, record.Stage.Base.Scope)
	}
	if cleanup == nil || validateCleanupReview(record, *cleanup) != nil || cleanup.Worker.InterfaceAlias == "" {
		return lanfirewall.InterfaceBinding{}, errors.New("protected cleanup review scope required")
	}
	scope := cleanup.Request.Scope
	return lanfirewall.InterfaceBinding{GUID: scope.InterfaceGUID, LUID: scope.InterfaceLUID, Alias: cleanup.Worker.InterfaceAlias, Address: scope.LocalAddress, Profile: scope.Profile}, nil
}
