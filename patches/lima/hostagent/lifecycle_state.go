// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package hostagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

type routineGroup struct {
	mu     sync.Mutex
	sealed bool
	active int
	idle   chan struct{}
}

func (g *routineGroup) start(fn func()) bool {
	g.mu.Lock()
	if g.sealed {
		g.mu.Unlock()
		return false
	}
	if g.idle == nil {
		g.idle = make(chan struct{})
	}
	g.active++
	g.mu.Unlock()
	go func() {
		defer func() {
			if recover() != nil {
				fmt.Fprintln(os.Stderr, "Hostagent worker panic; original routine retained")
				for {
					time.Sleep(time.Minute)
					runtime.KeepAlive(g)
					runtime.KeepAlive(fn)
				}
			}
			g.mu.Lock()
			g.active--
			if g.sealed && g.active == 0 {
				close(g.idle)
			}
			g.mu.Unlock()
		}()
		fn()
	}()
	return true
}
func (g *routineGroup) seal() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.sealed {
		return
	}
	g.sealed = true
	if g.idle == nil {
		g.idle = make(chan struct{})
	}
	if g.active == 0 {
		close(g.idle)
	}
}
func (g *routineGroup) join(ctx context.Context) error {
	g.mu.Lock()
	idle := g.idle
	sealed := g.sealed
	g.mu.Unlock()
	if !sealed {
		return errors.New("hostagent routines not sealed")
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func waitRequirementRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Snapshot under lock; run callbacks outside it. A failed/panicked prefix is
// cached and never retried, and the dependent suffix remains retained.
type cleanupStack struct {
	mu        sync.Mutex
	entries   []func() error
	attempted bool
	late      bool
	result    error
}

func (s *cleanupStack) add(fn func() error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, fn)
	if s.attempted {
		s.late = true
		s.result = errors.New("late hostagent cleanup registration")
	}
	return !s.attempted
}
func (s *cleanupStack) close() (result error) {
	s.mu.Lock()
	if s.attempted {
		result = s.result
		s.mu.Unlock()
		return
	}
	s.attempted = true
	s.result = errors.New("hostagent cleanup incomplete")
	entries := append([]func() error(nil), s.entries...)
	s.mu.Unlock()
	defer func() {
		if recover() != nil {
			result = errors.New("hostagent cleanup panic")
		}
		s.mu.Lock()
		if s.late {
			result = errors.Join(result, errors.New("late hostagent cleanup registration"))
		}
		s.result = result
		s.mu.Unlock()
	}()
	for i := len(entries) - 1; i >= 0; i-- {
		s.mu.Lock()
		late := s.late
		s.mu.Unlock()
		if late {
			return errors.New("late hostagent cleanup registration")
		}
		if result = entries[i](); result != nil {
			return
		}
	}
	return nil
}
