package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simiooo/pinelink/backend/internal/accounts"
)

type BlobStore interface {
	PutChunk(ctx context.Context, key string, data []byte) error
	GetChunk(ctx context.Context, key string, start, end int64) (io.ReadCloser, error)
	DeleteChunk(ctx context.Context, key string) error
}

type Service struct {
	db             *pgxpool.Pool
	blobs          BlobStore
	now            func() time.Time
	replicationKey ed25519.PrivateKey
	relayTransport string
	accounts       *accounts.Service
}

func New(db *pgxpool.Pool, blobs BlobStore) *Service {
	return &Service{db: db, blobs: blobs, now: time.Now, relayTransport: "webrtc"}
}

// ConfigureRelayTransport is startup-only configuration, before handlers run.
func (s *Service) ConfigureRelayTransport(value string) error {
	if value != "webrtc" && value != "websocket" {
		return fmt.Errorf("unsupported relay transport")
	}
	s.relayTransport = value
	return nil
}

func (s *Service) RelayTransport() string { return s.relayTransport }

func (s *Service) Ready(ctx context.Context) error {
	return s.db.Ping(ctx)
}

func (s *Service) GetBlob(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	return s.blobs.GetChunk(ctx, key, start, end)
}

func validIdentity(value string) bool {
	return len(value) > 0 && len(value) <= 128 && strings.TrimSpace(value) == value
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func chunkCount(size int64, chunkSize int) int {
	if size == 0 {
		return 0
	}
	return int((size + int64(chunkSize) - 1) / int64(chunkSize))
}

func expectedChunkSize(total int64, chunkSize, index, count int) int64 {
	if index == count-1 {
		remaining := total - int64(index)*int64(chunkSize)
		return remaining
	}
	return int64(chunkSize)
}

func addEvent(ctx context.Context, tx pgx.Tx, eventType, aggregateID string, version int64, payload any) error {
	if tr, ok := payload.(Transfer); ok {
		payload = transferEvent(tr)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, event_type, aggregate_id, aggregate_version, payload)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.NewString(), eventType, aggregateID, version, encoded)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func retentionEvent(retention string) string {
	if retention == "PERSISTENT" {
		return "PERSISTENCE_CHANGED"
	}
	return "DELIVERY_AVAILABLE"
}

func expiryFor(now time.Time, retention string) *time.Time {
	if retention != "TEMP_24H" {
		return nil
	}
	expires := now.Add(24 * time.Hour)
	return &expires
}
