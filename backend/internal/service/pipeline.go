package service

import (
	"context"
	"sync"
)

const relayPipelineWindow = 8

// orderedPipeline bounds both active I/O and completed, unconsumed buffers.
// Consumers see the original order, even when storage completes out of order.
// Every started fetch is cancelled and joined before the caller returns.
func orderedPipeline[T any](ctx context.Context, start, end, window int, fetch func(context.Context, int) (T, error), consume func(int, T) error) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	if start >= end {
		return nil
	}
	window = max(1, min(window, end-start))
	type result struct {
		value T
		err   error
	}
	pending := make([]chan result, window)
	launch := func(index int) {
		channel := make(chan result, 1)
		pending[(index-start)%window] = channel
		workers.Add(1)
		go func() {
			defer workers.Done()
			value, err := fetch(ctx, index)
			channel <- result{value, err}
		}()
	}
	for index := start; index < start+window; index++ {
		launch(index)
	}
	for index := start; index < end; index++ {
		var next result
		select {
		case <-ctx.Done():
			return ctx.Err()
		case next = <-pending[(index-start)%window]:
		}
		if next.err != nil {
			return next.err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := consume(index, next.value); err != nil {
			return err
		}
		if index+window < end {
			launch(index + window)
		}
	}
	return nil
}
