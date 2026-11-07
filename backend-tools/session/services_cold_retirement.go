package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errColdServiceDrain = errors.New("cold SDK retirement drain unavailable")

// A stage of the EXISTING control owner, not another server or runtime actor.
// seal is the actual factory admission seal; join is the actual entry/holder
// join. Poll only reports that operation's result. It issues no native grant.
type coldServiceDrain struct {
	mu                sync.Mutex
	started, finished bool
	result            error
	panicValue        any
}

func (d *coldServiceDrain) begin(ctx context.Context, seal func() error, join func(context.Context) error) error {
	if d == nil || ctx == nil || ctx.Err() != nil || seal == nil || join == nil {
		return errColdServiceDrain
	}
	if _, bounded := ctx.Deadline(); !bounded {
		return errColdServiceDrain
	}
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return errColdServiceDrain
	}
	d.started, d.result = true, errColdServiceDrain
	d.mu.Unlock()
	// Prompt source seal before returning to the SAME control pump. An error
	// or panic is terminal for this exact operation, never cached nil success.
	sealed := false
	func() {
		defer func() {
			if value := recover(); value != nil {
				d.mu.Lock()
				d.panicValue = value
				d.mu.Unlock()
			}
		}()
		sealed = seal() == nil
	}()
	if !sealed {
		d.finish(errColdServiceDrain)
		return errColdServiceDrain
	}
	// A frame cancellation must not drop or cancel the admitted native join.
	// This bounded owner remains held by the entry and the goroutine; its pump
	// continues servicing custody offers while join waits for real leaf owners.
	go func() {
		result := errColdServiceDrain
		defer func() {
			if value := recover(); value != nil {
				d.mu.Lock()
				d.panicValue = value
				d.mu.Unlock()
				result = errColdServiceDrain
			}
			d.finish(result)
		}()
		owner, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result = join(owner)
	}()
	return nil
}

func (d *coldServiceDrain) finish(result error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.result, d.finished = result, true
}

func (d *coldServiceDrain) poll() (joined bool, err error) {
	if d == nil {
		return false, errColdServiceDrain
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started {
		return false, errColdServiceDrain
	}
	if !d.finished {
		return false, nil
	}
	if d.result != nil {
		return false, errColdServiceDrain
	}
	return true, nil
}

func (d *coldServiceDrain) requireJoined() error {
	joined, err := d.poll()
	if err != nil || !joined {
		return errColdServiceDrain
	}
	return nil
}
