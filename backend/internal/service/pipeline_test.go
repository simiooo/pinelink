package service

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestOrderedPipelineBoundsReorderingAndReadAhead(t *testing.T) {
	const count, window = 20, 4
	entered := make(chan int, count)
	consumed := make(chan int, count)
	release := make([]chan struct{}, count)
	for i := range release {
		release[i] = make(chan struct{})
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	var outstanding, peak atomic.Int32
	go func() {
		done <- orderedPipeline(ctx, 0, count, window, func(ctx context.Context, index int) (int, error) {
			n := outstanding.Add(1)
			for {
				old := peak.Load()
				if old >= n || peak.CompareAndSwap(old, n) {
					break
				}
			}
			entered <- index
			select {
			case <-release[index]:
				return index, nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}, func(index, value int) error {
			if index != value {
				return errors.New("reordered result")
			}
			outstanding.Add(-1)
			consumed <- index
			return nil
		})
	}()
	initial := make([]int, window)
	for i := range initial {
		select {
		case initial[i] = <-entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	slices.Sort(initial)
	if !slices.Equal(initial, []int{0, 1, 2, 3}) {
		t.Fatal(initial)
	}
	for _, i := range []int{3, 2, 1} {
		close(release[i])
	}
	select {
	case <-consumed:
		t.Fatal("consumed past a blocked prefix")
	default:
	}
	close(release[0])
	for i := window; i < count; i++ {
		close(release[i])
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		if got := <-consumed; got != i {
			t.Fatalf("consumed %d, want %d", got, i)
		}
	}
	if peak.Load() != window || outstanding.Load() != 0 {
		t.Fatal("unbounded pending buffers", peak.Load(), outstanding.Load())
	}
}

func TestOrderedPipelineCancelsAndJoinsOnConsumerFailure(t *testing.T) {
	var active, started atomic.Int32
	all := make(chan struct{})
	want := errors.New("consumer stopped")
	err := orderedPipeline(t.Context(), 7, 100, 4, func(ctx context.Context, index int) (int, error) {
		active.Add(1)
		defer active.Add(-1)
		if started.Add(1) == 4 {
			close(all)
		}
		if index == 7 {
			<-all
			return index, nil
		}
		<-ctx.Done()
		return 0, ctx.Err()
	}, func(index, value int) error { return want })
	if !errors.Is(err, want) || active.Load() != 0 || started.Load() != 4 {
		t.Fatal(err, active.Load(), started.Load())
	}
}
