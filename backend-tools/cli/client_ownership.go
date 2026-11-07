package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

type clientWriterBinding interface{ binding() [32]byte }
type clientWriterAdmission interface {
	clientWriterBinding
	claim() ([32]byte, bool)
	validateInvocation(*exec.Cmd) error
}
type clientCapturedBinding [32]byte

func (b clientCapturedBinding) binding() [32]byte { return [32]byte(b) }

type neverStartedClient struct {
	sync.Mutex
	binding  [32]byte
	consumed bool
}

func (n *neverStartedClient) consume(binding [32]byte) bool {
	if n == nil {
		return false
	}
	n.Lock()
	defer n.Unlock()
	if n.consumed || n.binding != binding || binding == [32]byte{} {
		return false
	}
	n.consumed = true
	return true
}

type writerStartError struct {
	sync.Mutex
	cause     error
	unstarted *neverStartedClient
}

func (e *writerStartError) Error() string { return e.cause.Error() }
func (e *writerStartError) Unwrap() error { return e.cause }
func (e *writerStartError) takeNeverStarted() *neverStartedClient {
	e.Lock()
	defer e.Unlock()
	token := e.unstarted
	e.unstarted = nil
	return token
}
func clientNeverStartedError(err error, admission clientWriterBinding, known bool) error {
	if admission == nil || !known {
		return err
	}
	return &writerStartError{cause: err, unstarted: &neverStartedClient{binding: admission.binding()}}
}

type retiredClientTree struct {
	sync.Mutex
	binding  [32]byte
	consumed bool
}

// Distinct from natural config-writer completion: this certificate only permits
// releasing the live controller with every pending record preserved.
type executionQuiescedClient struct{ retiredClientTree }

func (r *retiredClientTree) consume(binding [32]byte) bool {
	if r == nil {
		return false
	}
	r.Lock()
	defer r.Unlock()
	if r.consumed || r.binding != binding || binding == [32]byte{} {
		return false
	}
	r.consumed = true
	return true
}

type clientTree interface {
	waitRoot(context.Context) (uint32, error)
	terminate() error
	empty() (bool, error)
	finishIO(context.Context) (error, error)
	close() error
}

// One owner waits the actual returned process handle, retires descendants, then
// joins IO and closes resources. A writer token additionally requires natural
// root completion and successful IO. Confirmed termination cannot establish
// that a synchronous config save/rename completed before the writer was killed.
func retireClientTree(ctx context.Context, tree clientTree, admission clientWriterBinding, unknown func()) (*retiredClientTree, error) {
	return retireClientTreeWithQuiescence(ctx, tree, admission, unknown, nil)
}
func retireClientTreeWithQuiescence(ctx context.Context, tree clientTree, admission clientWriterBinding, unknown func(), quiesced func(*executionQuiescedClient)) (*retiredClientTree, error) {
	var binding [32]byte
	if admission != nil {
		binding = admission.binding()
	}
	code, rootErr := tree.waitRoot(ctx)
	terminateErr := tree.terminate()
	retirement, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if rootErr != nil {
		var err error
		code, err = tree.waitRoot(retirement)
		if err != nil {
			unknown()
			return nil, errors.Join(rootErr, err, errors.New("client root retirement unknown"))
		}
	}
	for {
		empty, err := tree.empty()
		if err != nil {
			unknown()
			return nil, errors.Join(err, errors.New("client tree retirement unknown"))
		}
		if empty {
			break
		}
		select {
		case <-retirement.Done():
			unknown()
			return nil, errors.New("client tree retirement unknown")
		case <-time.After(10 * time.Millisecond):
		}
	}
	copyErr, joinErr := tree.finishIO(retirement)
	if joinErr != nil {
		unknown()
		return nil, errors.Join(joinErr, errors.New("client IO retirement unknown"))
	}
	if err := tree.close(); err != nil {
		unknown()
		return nil, errors.Join(err, errors.New("client resource close unknown"))
	}
	if admission != nil && quiesced != nil {
		quiesced(&executionQuiescedClient{retiredClientTree: retiredClientTree{binding: binding}})
	}
	var token *retiredClientTree
	if admission != nil && rootErr == nil && terminateErr == nil && copyErr == nil {
		token = &retiredClientTree{binding: binding}
	}
	var exitErr error
	if code != 0 {
		exitErr = fmt.Errorf("client exited with status %d", code)
	}
	return token, errors.Join(rootErr, terminateErr, exitErr, copyErr)
}

// This table bounds owned native resources and latches every uncertain Close.
// An unknown syscall outcome is retained evidence, not a guarantee the kernel
// handle is still open; it is never retried or treated as positive retirement.
type clientOwnership struct {
	sync.Mutex
	resources map[uintptr]bool
	unknown   bool
	active    bool
	retained  []any // Strong owners prevent finalizers retiring unknown IO handles.
}

func (o *clientOwnership) retainOwner(owner any) {
	o.Lock()
	defer o.Unlock()
	o.retained = append(o.retained, owner)
}
func (o *clientOwnership) isUnknown() bool { o.Lock(); defer o.Unlock(); return o.unknown }

func (o *clientOwnership) begin() bool {
	o.Lock()
	defer o.Unlock()
	if o.unknown || o.active || len(o.resources) != 0 {
		return false
	}
	o.active = true
	return true
}
func (o *clientOwnership) end() {
	o.Lock()
	defer o.Unlock()
	if !o.unknown && len(o.resources) == 0 {
		o.active = false
	}
}

func (o *clientOwnership) admissible() bool {
	o.Lock()
	defer o.Unlock()
	return !o.unknown && len(o.resources) < 16
}
func (o *clientOwnership) retain(id uintptr) {
	o.Lock()
	defer o.Unlock()
	if o.resources == nil {
		o.resources = map[uintptr]bool{}
	}
	o.resources[id] = true
}
func (o *clientOwnership) markUnknown() { o.Lock(); o.unknown = true; o.Unlock() }
func (o *clientOwnership) close(id uintptr, closeNative func() error) error {
	o.Lock()
	defer o.Unlock()
	if !o.resources[id] {
		return errors.New("client resource ownership unavailable")
	}
	if o.unknown {
		return errors.New("client resource retirement previously unknown")
	}
	if e := closeNative(); e != nil {
		o.unknown = true
		return e
	}
	delete(o.resources, id)
	return nil
}
