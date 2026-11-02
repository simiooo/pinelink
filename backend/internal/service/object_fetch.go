package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"time"
)

const smallObjectReadTimeout = 5 * time.Second
const smallObjectReadAttempts = 3

// Each attempt reads at most one relay-sized immutable chunk. A failed read
// publishes no bytes, and authority is checked again before every retry.
func (s *Service) readSmallChunk(ctx context.Context, chunk Chunk, authorize func(context.Context) error) ([]byte, error) {
	if chunk.Size < 0 || chunk.Size > 64<<10 {
		return nil, ErrInvalid
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := authorize(ctx); err != nil {
			return nil, err
		}
		data, err := s.fetchSmallChunk(ctx, chunk)
		if err == nil {
			return data, nil
		}
		if ctx.Err() != nil || attempt+1 == smallObjectReadAttempts || !retryableObjectRead(err) {
			return nil, err
		}
		if err := relayWait(ctx, time.Duration(1<<attempt)*100*time.Millisecond); err != nil {
			return nil, err
		}
	}
}

func (s *Service) fetchSmallChunk(ctx context.Context, chunk Chunk) ([]byte, error) {
	read, cancel := context.WithTimeout(ctx, smallObjectReadTimeout)
	defer cancel()
	object, err := s.blobs.GetChunk(read, chunk.Key, 0, chunk.Size-1)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(object, chunk.Size+1))
	closeErr := object.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	digest := sha256.Sum256(data)
	if int64(len(data)) != chunk.Size || hex.EncodeToString(digest[:]) != chunk.Hash {
		return nil, ErrChecksum
	}
	return data, nil
}

func retryableObjectRead(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var network net.Error
	return errors.As(err, &network)
}
