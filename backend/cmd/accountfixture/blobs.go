package main

import (
	"bytes"
	"context"
	"github.com/simiooo/pinelink/backend/internal/service"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Only the loopback test fixture uses memory storage. The deployed server uses R2.
type fixtureBlobs struct {
	mu                   sync.RWMutex
	data                 map[string][]byte
	delay                atomic.Int64
	puts, putBytes, gets atomic.Int64
}

func (b *fixtureBlobs) wait(ctx context.Context) error {
	timer := time.NewTimer(time.Duration(b.delay.Load()) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (b *fixtureBlobs) PutChunk(ctx context.Context, key string, data []byte) error {
	if e := b.wait(ctx); e != nil {
		return e
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[key] = bytes.Clone(data)
	b.puts.Add(1)
	b.putBytes.Add(int64(len(data)))
	return nil
}
func (b *fixtureBlobs) GetChunk(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	if e := b.wait(ctx); e != nil {
		return nil, e
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.data[key]
	if !ok {
		return nil, service.ErrNotFound
	}
	if start < 0 || end < start || end >= int64(len(v)) {
		return nil, service.ErrInvalid
	}
	b.gets.Add(1)
	return io.NopCloser(bytes.NewReader(bytes.Clone(v[start : end+1]))), nil
}
func (b *fixtureBlobs) DeleteChunk(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.data, key)
	return nil
}
