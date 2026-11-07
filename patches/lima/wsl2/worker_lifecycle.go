// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package wsl2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

type driverCommand struct {
	cmd        *exec.Cmd
	output     bytes.Buffer
	done       chan struct{}
	result     error
	beforeStop bool
	scriptPath string
	diagnostic string
}
type driverWorkers struct {
	mu       sync.Mutex
	commands []*driverCommand
	stopping bool
	unknown  bool
}

// Publish the actual command before its one Run/Wait and before go. Results are
// owned even when no receiver remains on the optional notification channel.
func (w *driverWorkers) launch(cmd *exec.Cmd, keepalive bool, reports chan<- error, scriptPath string) error {
	w.mu.Lock()
	if w.stopping || w.unknown || cmd == nil || cmd.Process != nil || cmd.Cancel != nil || cmd.WaitDelay != 0 {
		w.mu.Unlock()
		return errors.New("WSL command lifecycle refused")
	}
	r := &driverCommand{cmd: cmd, done: make(chan struct{}), scriptPath: scriptPath}
	cmd.Stdout = &r.output
	cmd.Stderr = &r.output
	w.commands = append(w.commands, r)
	w.mu.Unlock()
	go func() {
		locked := false
		defer func() {
			if recover() != nil {
				if locked {
					w.unknown = true
					w.mu.Unlock()
				} else {
					w.mu.Lock()
					w.unknown = true
					w.mu.Unlock()
				}
				reportDriverError(reports, errors.New("WSL worker panic; original command retained"))
				for {
					time.Sleep(time.Minute)
					runtime.KeepAlive(w)
					runtime.KeepAlive(r)
				}
			}
		}()
		// Seal and ORIGINAL Start are serialized. Queued goroutines cannot activate
		// the guest after Stop's no-new-birth boundary.
		w.mu.Lock()
		locked = true
		if w.stopping || w.unknown {
			unknown := w.unknown
			locked = false
			w.mu.Unlock()
			if unknown {
				w.retainWorker(r, errors.New("WSL command birth domain unavailable"), reports)
			}
			// Seal won before Start; no command was entered.
			if err := r.removeScript(); err != nil {
				w.retainWorker(r, err, reports)
			}
			w.mu.Lock()
			close(r.done)
			w.mu.Unlock()
			return
		}
		// This exact prearmed route is before every native/stdio Start effect.
		preBirthErr := cmd.Err
		startErr := cmd.Start()
		if startErr != nil && preBirthErr != nil && cmd.Process == nil {
			locked = false
			w.mu.Unlock()
			if err := r.removeScript(); err != nil {
				w.retainWorker(r, err, reports)
			}
			w.mu.Lock()
			r.beforeStop = !w.stopping
			r.result = startErr
			close(r.done)
			w.mu.Unlock()
			reportDriverError(reports, startErr)
			return
		}
		if startErr != nil {
			w.unknown = true
			locked = false
			w.mu.Unlock()
			reportDriverError(reports, startErr)
			for {
				time.Sleep(time.Minute)
				runtime.KeepAlive(w)
				runtime.KeepAlive(r)
			}
		}
		locked = false
		w.mu.Unlock()
		err := cmd.Wait() // sole original waiter, after actual Start normal return
		if cmd.ProcessState == nil {
			w.retainWorker(r, errors.New("WSL command wait unavailable"), reports)
		}
		r.diagnostic = boundedDriverOutput(r.output.Bytes())
		r.output = bytes.Buffer{}
		cmd.Stdout, cmd.Stderr = nil, nil
		if cleanupErr := r.removeScript(); cleanupErr != nil {
			w.retainWorker(r, cleanupErr, reports)
		}
		w.mu.Lock()
		r.beforeStop = !w.stopping
		r.result = err
		if keepalive && r.beforeStop && err == nil {
			r.result = errors.New("WSL keepalive exited before stop")
		}
		result := r.result
		close(r.done)
		w.mu.Unlock()
		if result != nil && r.beforeStop {
			reportDriverError(reports, fmt.Errorf("WSL worker ended: %w", result))
		}
	}()

	return nil
}
func reportDriverError(reports chan<- error, err error) {
	if err != nil {
		select {
		case reports <- err:
		default:
		}
	}
}
func (w *driverWorkers) seal() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopping = true
	return !w.unknown
}

func (w *driverWorkers) join(ctx context.Context) error {
	w.mu.Lock()
	commands := append([]*driverCommand(nil), w.commands...)
	w.mu.Unlock()
	var result error
	for _, r := range commands {
		select {
		case <-r.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		w.mu.Lock()
		if r.beforeStop {
			result = errors.Join(result, r.result)
		}
		w.mu.Unlock()
	}
	return result
}

// Only the exact generated provision role is removed after original settlement.
func (r *driverCommand) removeScript() error {
	if r.scriptPath == "" {
		return nil
	}
	if err := os.Remove(r.scriptPath); err != nil {
		return fmt.Errorf("remove settled WSL provision script: %w", err)
	}
	r.scriptPath = ""
	return nil
}
func boundedDriverOutput(b []byte) string {
	if len(b) > 4096 {
		b = b[len(b)-4096:]
	}
	return string(b)
}
func (w *driverWorkers) retainWorker(r *driverCommand, err error, reports chan<- error) {
	w.mu.Lock()
	w.unknown = true
	w.mu.Unlock()
	reportDriverError(reports, err)
	for {
		time.Sleep(time.Minute)
		runtime.KeepAlive(w)
		runtime.KeepAlive(r)
	}
}

// No new permission: settle only the exact closed script role after conversion
// failed. An os.PathError retains its original path on failure; the caller's
// existing partial-Start keeper retains the joined error and original driver.
func cleanupDriverScriptFailure(path string, conversionErr error) error {
	r := driverCommand{scriptPath: path}
	return errors.Join(conversionErr, r.removeScript())
}
