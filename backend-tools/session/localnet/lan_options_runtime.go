package localnet

import (
	"context"
	"errors"
	"time"
)

// ReadLANOptions runs off the settings actor. Exactly one bounded read is owned
// until its diagnostic child has actually exited, not merely until RPC timeout.
func (runtime *ServiceRuntime) ReadLANOptions(ctx context.Context) (LANOptions, error) {
	var result LANOptions
	err := runtime.readLANQuery(ctx, func(ctx context.Context) error {
		var err error
		if runtime.opts.lanOptionsReader != nil {
			result, err = runtime.opts.lanOptionsReader(ctx)
		} else {
			result, err = collectLANOptions(ctx, runtime.lanOptionsSource(), time.Now)
		}
		return err
	})
	return result, err
}
func (runtime *ServiceRuntime) readLANQuery(ctx context.Context, read func(context.Context) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	runtime.lanQueryMu.Lock()
	if runtime.lanQueriesClosing || runtime.lanQueryActive || runtime.ctx.Err() != nil {
		runtime.lanQueryMu.Unlock()
		return errors.New("LAN options observation is already active or stopping")
	}
	runtime.lanQueryActive = true
	runtime.lanQueries.Add(1)
	runtime.lanQueryMu.Unlock()
	defer func() {
		runtime.lanQueryMu.Lock()
		runtime.lanQueryActive = false
		runtime.lanQueryMu.Unlock()
		runtime.lanQueries.Done()
	}()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(runtime.ctx, cancel)
	defer stop()
	return read(ctx)
}

func (runtime *ServiceRuntime) closeLANQueries() {
	runtime.lanQueryMu.Lock()
	runtime.lanQueriesClosing = true
	runtime.lanQueryMu.Unlock()
	runtime.lanQueries.Wait()
}
