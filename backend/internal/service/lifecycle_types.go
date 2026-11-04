package service

import (
	"encoding/json"
	"time"
)

type StateConflictError struct {
	Resource string
	Current  any
}

func (e *StateConflictError) Error() string { return "state version conflict" }

type Transfer struct {
	ID                      string     `json:"id"`
	EntryID                 *string    `json:"entry_id,omitempty"`
	DeliveryMode            string     `json:"delivery_mode"`
	SenderOwnerID           string     `json:"sender_owner_id"`
	SenderDeviceID          string     `json:"sender_device_id"`
	RecipientOwnerID        string     `json:"recipient_owner_id"`
	RecipientDeviceID       string     `json:"recipient_device_id"`
	State                   string     `json:"state"`
	Version                 int64      `json:"version"`
	FailureReason           *string    `json:"failure_reason,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
	RespondedAt             *time.Time `json:"responded_at,omitempty"`
	ExpiresAt               time.Time  `json:"expires_at"`
	FileName                *string    `json:"file_name,omitempty"`
	SizeBytes               *int64     `json:"size_bytes,omitempty"`
	ContentRoot             *string    `json:"content_root,omitempty"`
	EncryptionSuite         *string    `json:"encryption_suite,omitempty"`
	ContentKind             *string    `json:"content_kind,omitempty"`
	EncryptedManifest       []byte     `json:"encrypted_manifest,omitempty"`
	SenderEncryptedManifest []byte     `json:"sender_encrypted_manifest,omitempty"`
	ReceivedBytes           int64      `json:"received_bytes"`
	ProgressRevision        int64      `json:"progress_revision"`
	ProgressSampledAt       *time.Time `json:"progress_sampled_at"`
	ActiveRoute             string     `json:"active_route"`
	RouteEpoch              int64      `json:"route_epoch"`
	RevocationEpoch         int64      `json:"revocation_epoch"`
}

type TransferCreateRequest struct {
	EntryID           string `json:"entry_id"`
	DeliveryMode      string `json:"delivery_mode"`
	SenderOwnerID     string `json:"sender_owner_id"`
	SenderDeviceID    string `json:"sender_device_id"`
	RecipientOwnerID  string `json:"recipient_owner_id"`
	RecipientDeviceID string `json:"recipient_device_id"`
}

type TransferActionRequest struct {
	OwnerID           string `json:"owner_id"`
	DeviceID          string `json:"device_id"`
	ExpectedVersion   int64  `json:"expected_version"`
	Reason            string `json:"reason,omitempty"`
	AcceptContentKind string `json:"accept_content_kind,omitempty"`
}

type TransferList struct {
	Transfers  []Transfer `json:"transfers"`
	NextBefore *time.Time `json:"next_before,omitempty"`
}

type Task struct {
	ID                string     `json:"id"`
	Kind              string     `json:"kind"`
	ReferenceID       string     `json:"reference_id"`
	OwnerID           string     `json:"owner_id"`
	DeviceID          string     `json:"device_id"`
	State             string     `json:"state"`
	ProgressBytes     int64      `json:"progress_bytes"`
	TotalBytes        int64      `json:"total_bytes"`
	Version           int64      `json:"version"`
	ErrorCode         *string    `json:"error_code,omitempty"`
	ErrorMessage      *string    `json:"error_message,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	ProgressRevision  int64      `json:"progress_revision"`
	ProgressSampledAt *time.Time `json:"progress_sampled_at"`
}

type TaskActionRequest struct {
	OwnerID         string `json:"owner_id"`
	DeviceID        string `json:"device_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason,omitempty"`
}

type TaskProgressRequest struct {
	OwnerID       string `json:"owner_id"`
	DeviceID      string `json:"device_id"`
	ProgressBytes int64  `json:"progress_bytes"`
}

type TaskList struct {
	Tasks      []Task     `json:"tasks"`
	NextBefore *time.Time `json:"next_before,omitempty"`
}

type SyncEvent struct {
	Sequence         int64           `json:"sequence"`
	EventID          string          `json:"event_id"`
	EventType        string          `json:"event_type"`
	AggregateID      string          `json:"aggregate_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	Payload          json.RawMessage `json:"payload"`
	OccurredAt       time.Time       `json:"occurred_at"`
}

type SyncResponse struct {
	Events          []SyncEvent `json:"events"`
	CurrentSequence int64       `json:"current_sequence"`
}
