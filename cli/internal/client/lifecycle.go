package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Transfer struct {
	ID                      string    `json:"id"`
	EntryID                 *string   `json:"entry_id,omitempty"`
	DeliveryMode            string    `json:"delivery_mode"`
	SenderOwnerID           string    `json:"sender_owner_id"`
	SenderDeviceID          string    `json:"sender_device_id"`
	RecipientOwnerID        string    `json:"recipient_owner_id"`
	RecipientDeviceID       string    `json:"recipient_device_id"`
	State                   string    `json:"state"`
	Version                 int64     `json:"version"`
	FailureReason           *string   `json:"failure_reason,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	ExpiresAt               time.Time `json:"expires_at"`
	FileName                *string   `json:"file_name,omitempty"`
	SizeBytes               *int64    `json:"size_bytes,omitempty"`
	ContentRoot             *string   `json:"content_root,omitempty"`
	EncryptionSuite         *string   `json:"encryption_suite,omitempty"`
	EncryptedManifest       []byte    `json:"encrypted_manifest,omitempty"`
	SenderEncryptedManifest []byte    `json:"sender_encrypted_manifest,omitempty"`
	ReceivedBytes           int64     `json:"received_bytes"`
}

type TransferCreateRequest struct {
	EntryID           string `json:"entry_id"`
	DeliveryMode      string `json:"delivery_mode"`
	SenderOwnerID     string `json:"sender_owner_id"`
	SenderDeviceID    string `json:"sender_device_id"`
	RecipientOwnerID  string `json:"recipient_owner_id"`
	RecipientDeviceID string `json:"recipient_device_id"`
}

type TransferList struct {
	Transfers  []Transfer `json:"transfers"`
	NextBefore *time.Time `json:"next_before,omitempty"`
}

type Task struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	ReferenceID   string    `json:"reference_id"`
	OwnerID       string    `json:"owner_id"`
	DeviceID      string    `json:"device_id"`
	State         string    `json:"state"`
	ProgressBytes int64     `json:"progress_bytes"`
	TotalBytes    int64     `json:"total_bytes"`
	Version       int64     `json:"version"`
	ErrorCode     *string   `json:"error_code,omitempty"`
	ErrorMessage  *string   `json:"error_message,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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

func (c *Client) CreateTransfer(ctx context.Context, request TransferCreateRequest) (Transfer, error) {
	var transfer Transfer
	err := c.doJSON(ctx, http.MethodPost, "/v1/transfers", request, &transfer)
	return transfer, err
}

func (c *Client) GetTransfer(ctx context.Context, id, ownerID, deviceID string) (Transfer, error) {
	var transfer Transfer
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/transfers/"+url.PathEscape(id)+"?"+query.Encode(), nil, &transfer)
	return transfer, err
}

func (c *Client) ListTransfers(ctx context.Context, path, ownerID, deviceID, role, state string, limit int) (TransferList, error) {
	var transfers TransferList
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}, "state": {state}, "limit": {strconv.Itoa(limit)}}
	if role != "" {
		query.Set("role", role)
	}
	err := c.doJSON(ctx, http.MethodGet, path+"?"+query.Encode(), nil, &transfers)
	return transfers, err
}

func (c *Client) TransferAction(ctx context.Context, id, action, ownerID, deviceID string, version int64, reason string) (Transfer, error) {
	var transfer Transfer
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "expected_version": version, "reason": reason}
	err := c.doJSON(ctx, http.MethodPost, "/v1/transfers/"+url.PathEscape(id)+"/"+action, request, &transfer)
	return transfer, err
}

func (c *Client) GetTask(ctx context.Context, id, ownerID, deviceID string) (Task, error) {
	var task Task
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id)+"?"+query.Encode(), nil, &task)
	return task, err
}

func (c *Client) ListTasks(ctx context.Context, ownerID, deviceID, kind, state string, includeDeleted bool, limit int) (TaskList, error) {
	var tasks TaskList
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}, "kind": {kind}, "state": {state},
		"include_deleted": {strconv.FormatBool(includeDeleted)}, "limit": {strconv.Itoa(limit)}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/tasks?"+query.Encode(), nil, &tasks)
	return tasks, err
}

func (c *Client) TaskAction(ctx context.Context, id, action, ownerID, deviceID string, version int64, reason ...string) (Task, error) {
	var task Task
	message := ""
	if len(reason) > 0 {
		message = reason[0]
	}
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "expected_version": version, "reason": message}
	err := c.doJSON(ctx, http.MethodPost, "/v1/tasks/"+url.PathEscape(id)+"/"+action, request, &task)
	return task, err
}

func (c *Client) TaskProgress(ctx context.Context, id, ownerID, deviceID string, progress int64) (Task, error) {
	var task Task
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "progress_bytes": progress}
	err := c.doJSON(ctx, http.MethodPost, "/v1/tasks/"+url.PathEscape(id)+"/progress", request, &task)
	return task, err
}

func (c *Client) GetPull(ctx context.Context, pullID, ownerID, deviceID string) (Pull, error) {
	var pull Pull
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/pulls/"+url.PathEscape(pullID)+"?"+query.Encode(), nil, &pull)
	return pull, err
}

func (c *Client) Sync(ctx context.Context, after int64, limit int, ownerID, deviceID string) (SyncResponse, error) {
	var response SyncResponse
	query := url.Values{"after": {strconv.FormatInt(after, 10)}, "limit": {strconv.Itoa(limit)}, "owner_id": {ownerID}, "device_id": {deviceID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/sync?"+query.Encode(), nil, &response)
	return response, err
}
