package client

import "time"

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
	UploadID       string   `json:"upload_id"`
	TaskID         string   `json:"task_id"`
	ReuseToken     string   `json:"reuse_token"`
	ReceivedChunks []int    `json:"received_chunks"`
	Manifest       Manifest `json:"manifest"`
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

type PullCompleteResponse struct {
	FirstDelivery bool `json:"first_delivery"`
}
