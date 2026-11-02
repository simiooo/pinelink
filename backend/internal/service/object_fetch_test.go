package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
)

type faultingReads struct {
	BlobStore
	failures      int32
	failure       error
	bodyFailure   bool
	beforeFailure func()
	reads, closed atomic.Int32
}

type failedReadBody struct {
	io.Reader
	failure error
	closed  *atomic.Int32
}

func (b *failedReadBody) Read(data []byte) (int, error) {
	n, err := b.Reader.Read(data)
	if errors.Is(err, io.EOF) {
		err = b.failure
	}
	return n, err
}
func (b *failedReadBody) Close() error { b.closed.Add(1); return nil }

func (b *faultingReads) GetChunk(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	if b.reads.Add(1) <= b.failures {
		if b.beforeFailure != nil {
			b.beforeFailure()
		}
		if b.bodyFailure {
			return &failedReadBody{bytes.NewReader([]byte("uncommitted prefix")), b.failure, &b.closed}, nil
		}
		return nil, b.failure
	}
	return b.BlobStore.GetChunk(ctx, key, start, end)
}

func TestSmallChunkDiscardsPartialReadsAndRetries(t *testing.T) {
	for _, bodyFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "partial-body"}[bodyFailure], func(t *testing.T) {
			data := bytes.Repeat([]byte("ciphertext"), 20)
			backing := &testBlobs{objects: map[string][]byte{"chunk": data}}
			failure := error(context.DeadlineExceeded)
			if bodyFailure {
				failure = io.ErrUnexpectedEOF
			}
			store := &faultingReads{BlobStore: backing, failures: 2, failure: failure, bodyFailure: bodyFailure}
			s := New(nil, store)
			checks := 0
			got, err := s.readSmallChunk(t.Context(), Chunk{Key: "chunk", Size: int64(len(data)), Hash: digest(data)}, func(context.Context) error { checks++; return nil })
			if err != nil || !bytes.Equal(got, data) || store.reads.Load() != 3 || checks != 3 {
				t.Fatal("read did not recover a verified whole chunk", err, store.reads.Load(), checks)
			}
			if bodyFailure && store.closed.Load() != 2 {
				t.Fatal("failed response bodies not closed", store.closed.Load())
			}
		})
	}
}

func TestSmallChunkRetriesAreBoundedAndReauthorized(t *testing.T) {
	for _, cause := range []error{ErrUnauthorized, ErrNotFound, ErrChecksum, context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			store := &faultingReads{failures: 100, failure: cause}
			s := New(nil, store)
			_, err := s.readSmallChunk(t.Context(), Chunk{Size: 1}, func(context.Context) error { return nil })
			want := int32(1)
			if errors.Is(cause, context.DeadlineExceeded) {
				want = smallObjectReadAttempts
			}
			if !errors.Is(err, cause) || store.reads.Load() != want {
				t.Fatal("permanent error retried or retry budget exceeded", err, store.reads.Load())
			}
		})
	}
	store := &faultingReads{failures: 100, failure: context.DeadlineExceeded}
	s := New(nil, store)
	checks := 0
	_, err := s.readSmallChunk(t.Context(), Chunk{Size: 1}, func(context.Context) error {
		checks++
		if checks > 1 {
			return ErrUnauthorized
		}
		return nil
	})
	if !errors.Is(err, ErrUnauthorized) || store.reads.Load() != 1 {
		t.Fatal("retry bypassed current authority", err, store.reads.Load())
	}
}

func TestRelayReadRetryObservesDeviceRevocation(t *testing.T) {
	s, _, backing := testStorage(t)
	data := []byte("ciphertext")
	tr := newTestTransfer(t, s, data, false)
	spool := spoolData(t, s, tr, data)
	tr = transferAction(t, s, tr, "accept")
	store := &faultingReads{BlobStore: backing, failures: 1, failure: context.DeadlineExceeded}
	store.beforeFailure = func() {
		if _, err := s.RevokeDevice(t.Context(), DeviceRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID}); err != nil {
			t.Fatal(err)
		}
	}
	s.blobs = store
	if data, err := s.readRelayChunk(t.Context(), spool.ID, 0); err == nil || len(data) != 0 || store.reads.Load() != 1 {
		t.Fatal("revoked receiver retried object I/O", err, len(data), store.reads.Load())
	}
}
