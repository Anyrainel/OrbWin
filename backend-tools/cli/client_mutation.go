package main

// The native owner is serialized by its guard/review. This state is not a
// persisted record, resource capability or permission to replay an operation.
// An armed attempt survives errors/panics until its owner is quarantined.
type privateMutationState struct{ active bool }

func (s *privateMutationState) admissible() bool { return s != nil && !s.active }

// Admission is pre-effect. Only mutate may issue the native rename/disposition.
// Once armed, verify owns the complete checked close/reopen/identity/body/absence
// suffix. No error path runs another suffix operation or retries a native call.
// A positive completion bit, not a nil named error, distinguishes panic/drop.
func checkedPrivateMutation(state *privateMutationState, admit, mutate, verify, fence func() error) (resultErr error) {
	if state == nil || admit == nil || mutate == nil || verify == nil || fence == nil {
		return errClientConfigRecovery
	}
	terminal := func() error {
		if e := fence(); e != nil {
			return e
		}
		return errClientConfigRecovery
	}
	if !state.admissible() {
		return terminal()
	}
	if e := admit(); e != nil {
		return e
	}
	state.active = true
	completed := false
	defer func() {
		if !completed {
			resultErr = terminal()
		}
	}()
	if e := mutate(); e != nil {
		return e
	}
	if e := verify(); e != nil {
		return e
	}
	state.active = false
	completed = true
	return nil
}
