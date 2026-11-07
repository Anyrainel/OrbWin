//go:build windows

package main

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

var retainedJobs clientOwnership

func runClientWriter(ctx context.Context, cmd *exec.Cmd, admission clientWriterAdmission) (*retiredClientTree, error) {
	var binding clientWriterBinding
	if admission != nil {
		value, ok := admission.claim()
		if !ok {
			return nil, errors.New("client writer admission already claimed; no dispatch")
		}
		binding = clientCapturedBinding(value)
	}
	if ctx.Err() != nil {
		return nil, clientNeverStartedError(ctx.Err(), binding, !retainedJobs.isUnknown())
	}
	if admission != nil {
		if err := admission.validateInvocation(cmd); err != nil {
			return nil, clientNeverStartedError(err, binding, !retainedJobs.isUnknown())
		}
	}
	if !retainedJobs.begin() {
		return nil, errors.New("prior client retirement unknown or resource budget exhausted; no dispatch")
	}
	defer retainedJobs.end()
	var beforeResume func(uintptr) error
	if admission != nil {
		producer, ok := admission.(interface{ recordBirth(uintptr) error })
		if !ok {
			return nil, clientNeverStartedError(errClientConfigRecovery, binding, true)
		}
		beforeResume = producer.recordBirth
	}
	tree, err := createNativeClientAdmitted(ctx, cmd, &retainedJobs, beforeResume)
	if tree == nil {
		// The native producer returned no successful CreateProcess and joined
		// checked precreation cleanup before this synchronous call can return.
		return nil, clientNeverStartedError(err, binding, !retainedJobs.isUnknown())
	}
	var quiesced func(*executionQuiescedClient)
	if receiver, ok := admission.(interface {
		recordQuiesced(*executionQuiescedClient)
	}); ok {
		quiesced = receiver.recordQuiesced
	}
	retireCtx := ctx
	if err != nil {
		var cancel context.CancelFunc
		retireCtx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
	}
	token, retirementErr := retireClientTreeWithQuiescence(retireCtx, tree, binding, retainedJobs.markUnknown, quiesced)
	if err != nil {
		// Creation may have returned an owned suspended/partially started tree
		// and requested its termination. Cleanup is required, but its later
		// signaled root is not natural writer completion.
		token = nil
	}
	if retainedJobs.isUnknown() {
		retainedJobs.retainOwner(tree)
	}
	return token, errors.Join(err, retirementErr)
}
func runClientCommand(ctx context.Context, cmd *exec.Cmd, detached bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Exact service Start has an independent owner; transient containment never
	// changes this narrowly selected detached lifetime.
	if detached {
		return cmd.Run()
	}
	_, err := runClientWriter(ctx, cmd, nil)
	return err
}
