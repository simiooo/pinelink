package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStopCancelsAndJoinsWorker(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	w := NewWorker(func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		<-release
		return nil
	})
	result := make(chan error, 1)
	go func() { result <- w.Start(context.Background()) }()
	<-entered
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := w.Stop(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop must join the loop: %v", err)
	}
	close(release)
	if err := w.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestStopRacingWithStart(t *testing.T) {
	for range 100 {
		var calls atomic.Int32
		w := NewWorker(func(ctx context.Context) error {
			calls.Add(1)
			<-ctx.Done()
			return nil
		})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); _ = w.Start(ctx) }()
		for range 2 {
			go func() {
				defer wg.Done()
				if err := w.Stop(ctx); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		cancel()
		if err := w.Start(context.Background()); err != nil || calls.Load() > 1 {
			t.Fatalf("stopped worker restarted: calls=%d, err=%v", calls.Load(), err)
		}
	}
}

func TestWorkerFailureCanBeStopped(t *testing.T) {
	failure := errors.New("startup failed")
	w := NewWorker(func(context.Context) error { return failure })
	if err := w.Start(context.Background()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := w.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}
