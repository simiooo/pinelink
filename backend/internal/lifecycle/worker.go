// Package lifecycle adapts blocking background loops to the Kratos server lifecycle.
package lifecycle

import (
	"context"
	"errors"
	"sync"
)

// Worker owns one run of a background loop. Stop cancels and joins it, including
// when Stop races with Start. A stopped worker cannot be restarted.
type Worker struct {
	run     func(context.Context) error
	mu      sync.Mutex
	started bool
	stopped bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewWorker(run func(context.Context) error) *Worker {
	return &Worker{run: run}
}

func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return nil
	}
	if w.started {
		w.mu.Unlock()
		return errors.New("worker already started")
	}
	w.started = true
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	done := w.done
	w.mu.Unlock()
	defer close(done)
	defer cancel()
	return w.run(ctx)
}

func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	w.stopped = true
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
