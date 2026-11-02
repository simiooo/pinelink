package p2p

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestProgressDoesNotBlockFramesAndFlushesLatestCheckpoint(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls, checkpoints atomic.Int64
	var delivered atomic.Int64
	r := newProgressReporter(t.Context(), time.Millisecond, func() error { checkpoints.Add(1); return nil }, func(ctx context.Context, n int64) error {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		delivered.Store(n)
		return nil
	})
	defer r.Close()
	r.Observe(1)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("report did not start")
	}
	done := make(chan struct{})
	go func() {
		for n := int64(2); n <= 10000; n++ {
			r.Observe(n)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("data observations blocked on HTTP")
	}
	close(release)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := r.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if delivered.Load() != 10000 {
		t.Fatalf("final checkpoint=%d", delivered.Load())
	}
	if calls.Load() > 3 || checkpoints.Load() != calls.Load() {
		t.Fatalf("unbounded sampling: calls=%d checkpoints=%d", calls.Load(), checkpoints.Load())
	}
}

func TestProgressRetriesFailedObservationWithoutRegression(t *testing.T) {
	var calls, bytes atomic.Int64
	r := newProgressReporter(t.Context(), time.Hour, func() error { return nil }, func(ctx context.Context, n int64) error {
		if calls.Add(1) == 1 {
			return errors.New("transient control-plane failure")
		}
		bytes.Store(n)
		return nil
	})
	defer r.Close()
	r.Observe(300)
	if err := r.Flush(t.Context()); err == nil {
		t.Fatal("expected transient error")
	}
	r.Observe(100)
	if err := r.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if bytes.Load() != 300 {
		t.Fatalf("checkpoint regressed to %d", bytes.Load())
	}
	if err := r.Flush(t.Context()); err != nil || calls.Load() != 2 {
		t.Fatalf("duplicate checkpoint refreshed: %v / %d", err, calls.Load())
	}
}

func TestProgressCloseCancelsInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	r := newProgressReporter(t.Context(), time.Millisecond, func() error { return nil }, func(ctx context.Context, _ int64) error { close(started); <-ctx.Done(); return ctx.Err() })
	r.Observe(0)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("report did not start")
	}
	closed := make(chan struct{})
	go func() { r.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("worker leaked on close")
	}
}

func TestProgressCloseAlsoCancelsExplicitFlush(t *testing.T) {
	started := make(chan struct{})
	r := newProgressReporter(t.Context(), time.Hour, func() error { return nil }, func(ctx context.Context, _ int64) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	r.Observe(100)
	flushed := make(chan error, 1)
	go func() { flushed <- r.Flush(context.Background()) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("flush did not start")
	}
	closed := make(chan struct{})
	go func() { r.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not cancel flush I/O")
	}
	if err := <-flushed; !errors.Is(err, context.Canceled) {
		t.Fatalf("flush result=%v", err)
	}
}
