package lanprovider

import (
	"io"
	"net/http"
	"sync/atomic"
)

const cleanupDaemonResponseBudget = 4 << 20

// A daemon-ID check is logical evidence, not native peer authentication. Keep
// even a hostile fixed-pipe response bounded, including bytes after JSON EOF.
type cleanupBudgetBody struct {
	body   io.ReadCloser
	left   int64
	failed *atomic.Bool
	closed bool
}

func (b *cleanupBudgetBody) Read(p []byte) (int, error) {
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	if b.left < 0 {
		return 0, ErrCleanupPending
	}
	if int64(len(p)) > b.left+1 {
		p = p[:b.left+1]
	}
	n, e := b.body.Read(p)
	b.left -= int64(n)
	if b.left < 0 {
		b.failed.Store(true)
		return n, ErrCleanupPending
	}
	if e != nil && e != io.EOF {
		b.failed.Store(true)
	}
	return n, e
}
func (b *cleanupBudgetBody) Close() error {
	if b.closed {
		return ErrCleanupPending
	}
	_, e := io.Copy(io.Discard, b)
	b.closed = true
	closeErr := b.body.Close()
	if e != nil || closeErr != nil || b.failed.Load() {
		b.failed.Store(true)
		return ErrCleanupPending
	}
	return nil
}
func cleanupResponseBound(failed *atomic.Bool) func(*http.Response) {
	return func(r *http.Response) {
		if r == nil || r.Body == nil {
			failed.Store(true)
			return
		}
		if r.ContentLength > cleanupDaemonResponseBudget {
			failed.Store(true)
		}
		r.Body = &cleanupBudgetBody{body: r.Body, left: cleanupDaemonResponseBudget, failed: failed}
	}
}
