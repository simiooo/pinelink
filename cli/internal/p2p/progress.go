package p2p

import (
	"context"
	"sync"
	"time"
)

type flushRequest struct {
	ctx    context.Context
	result chan error
}

// One bounded worker reports the latest checkpoint. The data callback never waits for per-frame HTTP.
type progressReporter struct {
	mu       sync.Mutex
	latest   int64
	observed bool
	dirty    bool
	flush    chan flushRequest
	cancel   context.CancelFunc
	done     chan struct{}
}

func newProgressReporter(ctx context.Context, interval time.Duration, checkpoint func() error, send func(context.Context, int64) error) *progressReporter {
	ctx, cancel := context.WithCancel(ctx)
	r := &progressReporter{flush: make(chan flushRequest), cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		report := func(parent context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := parent.Err(); err != nil {
				return err
			}
			r.mu.Lock()
			n, dirty := r.latest, r.dirty
			r.mu.Unlock()
			if !dirty {
				return nil
			}
			if err := checkpoint(); err != nil {
				return err
			}
			// Both a caller's Flush deadline and worker Close cancel the I/O.
			call, cancel := context.WithTimeout(ctx, 2*time.Second)
			stop := context.AfterFunc(parent, cancel)
			defer stop()
			defer cancel()
			if err := call.Err(); err != nil {
				return err
			}
			if err := send(call, n); err != nil {
				return err
			}
			r.mu.Lock()
			if r.latest == n {
				r.dirty = false
			}
			r.mu.Unlock()
			return nil
		}
		for {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = report(ctx)
			case req := <-r.flush:
				req.result <- report(req.ctx)
			}
		}
	}()
	return r
}

func (r *progressReporter) Observe(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.observed || n > r.latest {
		r.latest = n
		r.observed = true
		r.dirty = true
	}
}

func (r *progressReporter) Flush(ctx context.Context) error {
	req := flushRequest{ctx: ctx, result: make(chan error, 1)}
	select {
	case r.flush <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		return context.Canceled
	}
	select {
	case err := <-req.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		return context.Canceled
	}
}

func (r *progressReporter) Close() { r.cancel(); <-r.done }
