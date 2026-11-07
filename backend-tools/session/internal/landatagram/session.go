package landatagram

import (
	"context"
	"errors"
	"io"
	"net/netip"
	"sync"
	"time"
)

const MaxPeers = 16

// PeerSlots is ephemeral capacity only. Each reservation belongs to ONE peer;
// authorization and association reuse remain with the lease's Windows worker.
// Failed/unknown cleanup must retain its reservation until exact settlement.
type PeerSlots struct {
	mu    sync.Mutex
	peers map[netip.AddrPort]bool
}

// ProcessWatchdog is ONLY for a dedicated association process. Deadline expiry
// invokes terminate rather than pretending a nonpollable syscall returned a
// timeout. The owner must make terminate end that whole process.
type ProcessWatchdog struct {
	mu                        sync.Mutex
	now                       func() time.Time
	after                     func(time.Duration, func()) watchdogTimer
	terminate                 func()
	read, write               watchdogArm
	closing, joined, terminal bool
	closeTimer                watchdogTimer
}
type watchdogTimer interface{ Stop() bool }
type watchdogArm struct {
	deadline              time.Time
	active                bool
	operation, generation uint64
	timer                 watchdogTimer
}

func NewProcessWatchdog(terminate func()) *ProcessWatchdog {
	return newProcessWatchdog(time.Now, func(d time.Duration, f func()) watchdogTimer { return time.AfterFunc(d, f) }, terminate)
}
func newProcessWatchdog(now func() time.Time, after func(time.Duration, func()) watchdogTimer, terminate func()) *ProcessWatchdog {
	if terminate == nil {
		panic("dedicated process termination required")
	}
	return &ProcessWatchdog{now: now, after: after, terminate: terminate}
}
func (w *ProcessWatchdog) arm(a *watchdogArm) {
	if a.timer != nil {
		a.timer.Stop()
	}
	a.generation++
	generation := a.generation
	d := a.deadline.Sub(w.now())
	if d < 0 {
		d = 0
	}
	a.timer = w.after(d, func() {
		w.mu.Lock()
		fire := !w.joined && !w.terminal && a.active && a.generation == generation
		if fire {
			w.terminal = true
		}
		w.mu.Unlock()
		if fire {
			w.terminate()
		}
	})
}
func (w *ProcessWatchdog) set(a *watchdogArm, t time.Time, budget time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.terminal || w.joined || w.closing {
		return io.ErrClosedPipe
	}
	a.deadline = bounded(w.now(), t, budget)
	if a.active {
		w.arm(a)
	}
	return nil
}
func (w *ProcessWatchdog) SetReadDeadline(t time.Time) error  { return w.set(&w.read, t, ReadBudget) }
func (w *ProcessWatchdog) SetWriteDeadline(t time.Time) error { return w.set(&w.write, t, WriteBudget) }
func (w *ProcessWatchdog) begin(a *watchdogArm, budget time.Duration) (func(), error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.terminal || w.joined || w.closing || a.active {
		return nil, io.ErrClosedPipe
	}
	a.deadline = bounded(w.now(), a.deadline, budget)
	a.active = true
	a.operation++
	operation := a.operation
	w.arm(a)
	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if a.active && a.operation == operation {
			a.active = false
			a.generation++
			if a.timer != nil {
				a.timer.Stop()
			}
		}
	}, nil
}
func (w *ProcessWatchdog) BeginRead() (func(), error)  { return w.begin(&w.read, ReadBudget) }
func (w *ProcessWatchdog) BeginWrite() (func(), error) { return w.begin(&w.write, WriteBudget) }

// StartClose must precede possibly blocking descriptor Close. Neither Close's
// return nor operation completion cancels this final whole-process deadline.
func (w *ProcessWatchdog) StartClose() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing || w.joined || w.terminal {
		return
	}
	w.closing = true
	w.closeTimer = w.after(2*time.Second, func() {
		w.mu.Lock()
		fire := !w.joined && !w.terminal
		if fire {
			w.terminal = true
		}
		w.mu.Unlock()
		if fire {
			w.terminate()
		}
	})
}

// Joined may be called ONLY after both owned packet pumps actually returned.
func (w *ProcessWatchdog) Joined() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.joined = true
	for _, a := range []*watchdogArm{&w.read, &w.write} {
		a.active = false
		a.generation++
		if a.timer != nil {
			a.timer.Stop()
		}
	}
	if w.closeTimer != nil {
		w.closeTimer.Stop()
	}
}

func (s *PeerSlots) Reserve(peer netip.AddrPort) (func(), error) {
	if !peer.IsValid() || !peer.Addr().Is4() || peer.Addr().IsUnspecified() || peer.Port() == 0 {
		return nil, errors.New("exact IPv4 UDP peer required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peers == nil {
		s.peers = make(map[netip.AddrPort]bool)
	}
	if s.peers[peer] || len(s.peers) >= MaxPeers {
		return nil, errors.New("UDP peer already reserved or lease capacity exhausted")
	}
	s.peers[peer] = true
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); delete(s.peers, peer); s.mu.Unlock() }) }, nil
}

// Bridge uses exactly two pumps, no packet queues. Both endpoints MUST unblock
// pending ReadPacket/WritePacket on Close; the caller supplies owned resources.
// Returning joins both pumps. No packet is retried after an uncertain write.
func Bridge(ctx context.Context, left, right Connection) error {
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), left.Close(), right.Close())
	}
	var once sync.Once
	closeBoth := func() { once.Do(func() { _ = left.Close(); _ = right.Close() }) }
	stop := context.AfterFunc(ctx, closeBoth)
	defer stop()
	defer closeBoth()
	results := make(chan error, 2)
	pump := func(dst, src Connection) {
		var buffer [MaxPayload + 1]byte
		for {
			n, err := src.ReadPacket(buffer[:])
			if err != nil {
				results <- err
				return
			}
			if n < 0 || n > MaxPayload {
				results <- ErrOversize
				return
			}
			written, err := dst.WritePacket(buffer[:n])
			if err != nil {
				results <- err
				return
			}
			if written != n {
				results <- io.ErrShortWrite
				return
			}
		}
	}
	go pump(left, right)
	go pump(right, left)
	first := <-results
	closeBoth()
	second := <-results
	return errors.Join(first, second, ctx.Err())
}
