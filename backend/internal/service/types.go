package service

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrExpired       = errors.New("expired")
	ErrInvalid       = errors.New("invalid request")
	ErrIncomplete    = errors.New("upload incomplete")
	ErrChecksum      = errors.New("checksum mismatch")
	ErrReuseVerified = errors.New("reuse already committed")
)

type PrepareRequest struct {
	ScopeID            string `json:"scope_id"`
	EncryptionDomainID string `json:"encryption_domain_id"`
	OwnerID            string `json:"owner_id"`
	DeviceID           string `json:"device_id"`
	FileName           string `json:"file_name"`
	RetentionClass     string `json:"retention_class"`
	HashVersion        string `json:"hash_version"`
	DedupKey           string `json:"dedup_key"`
	ContentRoot        string `json:"content_root"`
	SizeBytes          int64  `json:"size_bytes"`
	ChunkSize          int    `json:"chunk_size"`
	EncryptionSuite    string `json:"encryption_suite,omitempty"`
	EncryptedManifest  []byte `json:"encrypted_manifest,omitempty"`
}

type Manifest struct {
	VersionID         string `json:"version_id"`
	ContentRoot       string `json:"content_root"`
	SizeBytes         int64  `json:"size_bytes"`
	ChunkSize         int    `json:"chunk_size"`
	ChunkCount        int    `json:"chunk_count"`
	EncryptionSuite   string `json:"encryption_suite,omitempty"`
	EncryptedManifest []byte `json:"encrypted_manifest,omitempty"`
}

type PrepareResponse struct {
	Status         string   `json:"status"`
	UploadID       string   `json:"upload_id,omitempty"`
	TaskID         string   `json:"task_id,omitempty"`
	ReuseToken     string   `json:"reuse_token,omitempty"`
	ReceivedChunks []int    `json:"received_chunks,omitempty"`
	Manifest       Manifest `json:"manifest"`
}

type ReuseCommitRequest struct {
	ReuseToken string `json:"reuse_token"`
}

type FileEntry struct {
	ID             string     `json:"id"`
	VersionID      string     `json:"version_id"`
	OwnerID        string     `json:"owner_id"`
	DeviceID       string     `json:"device_id"`
	FileName       string     `json:"file_name"`
	RetentionClass string     `json:"retention_class"`
	State          string     `json:"state"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type CompleteResponse struct {
	File   FileEntry `json:"file"`
	Reused bool      `json:"reused"`
}

type PullCreateRequest struct {
	OwnerID    string `json:"owner_id"`
	DeviceID   string `json:"device_id"`
	TransferID string `json:"transfer_id,omitempty"`
}

type Pull struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	TransferID  string `json:"transfer_id,omitempty"`
	EntryID     string `json:"entry_id"`
	DeviceID    string `json:"device_id"`
	FileName    string `json:"file_name"`
	ContentRoot string `json:"content_root"`
	SizeBytes   int64  `json:"size_bytes"`
}

type PullCompleteRequest struct {
	DeviceID    string `json:"device_id"`
	ContentRoot string `json:"content_root"`
}

type PullCompleteResponse struct {
	FirstDelivery bool `json:"first_delivery"`
}

type Content struct {
	VersionID  string
	Generation string
	Pull       Pull
	ChunkSize  int
	TotalSize  int64
}

type Chunk struct {
	Index int
	Hash  string
	Key   string
	Size  int64
}

type StoredEvent struct {
	EventID          string
	EventType        string
	AggregateID      string
	AggregateVersion int64
	Payload          json.RawMessage
	OccurredAt       time.Time
}
