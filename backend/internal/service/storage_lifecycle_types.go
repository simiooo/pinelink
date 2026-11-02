package service

import "time"

type Device struct {
	OwnerID    string     `json:"owner_id"`
	DeviceID   string     `json:"device_id"`
	PublicKey  *string    `json:"public_key,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

type DeviceRequest struct {
	OwnerID          string `json:"owner_id"`
	DeviceID         string `json:"device_id"`
	PublicKey        string `json:"public_key,omitempty"`
	SigningPublicKey string `json:"signing_public_key,omitempty"`
}

type DeviceList struct {
	Devices []Device `json:"devices"`
}

type BroadcastRequest struct {
	EntryID          string `json:"entry_id"`
	SenderOwnerID    string `json:"sender_owner_id"`
	SenderDeviceID   string `json:"sender_device_id"`
	RecipientOwnerID string `json:"recipient_owner_id"`
}

type FileActionRequest struct {
	OwnerID         string `json:"owner_id"`
	DeviceID        string `json:"device_id"`
	TransferID      string `json:"transfer_id,omitempty"`
	ExpectedVersion int64  `json:"expected_version"`
}

type Quota struct {
	ScopeID    string    `json:"scope_id"`
	QuotaBytes int64     `json:"quota_bytes"`
	UsedBytes  int64     `json:"used_bytes"`
	Version    int64     `json:"version"`
	UpdatedAt  time.Time `json:"updated_at"`
}
