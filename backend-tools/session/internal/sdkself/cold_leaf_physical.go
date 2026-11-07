package sdkself

import "errors"

var ErrColdLeaf = errors.New("cold SDK native leaf ownership unavailable")

// A local operation identity only. It grants no Root/cohort/Resume admission.
// Actual no-birth/no-Resume classifications are minted solely by the physical
// owner after inspecting its own syscall-attempt and checked-cleanup state.
type NativeLANClaim struct{ _ byte }     // nonzero size: live claim pointers stay distinct
func NewNativeLANClaim() *NativeLANClaim { return &NativeLANClaim{} }

type nativeLANNoBirth struct {
	claim *NativeLANClaim
	cause error
}

func (e *nativeLANNoBirth) Error() string { return e.cause.Error() }
func (e *nativeLANNoBirth) Unwrap() error { return e.cause }
func nativeLANNoBirthFor(claim *NativeLANClaim, cause error) error {
	if cause == nil {
		cause = ErrColdLeaf
	}
	return &nativeLANNoBirth{claim, cause}
}
func IsNativeLANNoBirth(err error, claim *NativeLANClaim) bool {
	var noBirth *nativeLANNoBirth
	return claim != nil && errors.As(err, &noBirth) && noBirth.claim == claim
}

type nativeLANNoResume struct {
	claim *NativeLANClaim
	cause error
}

func (e *nativeLANNoResume) Error() string { return e.cause.Error() }
func (e *nativeLANNoResume) Unwrap() error { return e.cause }
func IsNativeLANNoResume(err error, claim *NativeLANClaim) bool {
	var noResume *nativeLANNoResume
	return claim != nil && errors.As(err, &noResume) && noResume.claim == claim
}
