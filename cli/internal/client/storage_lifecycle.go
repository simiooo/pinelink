package client

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

type Device struct {
	OwnerID   string  `json:"owner_id"`
	DeviceID  string  `json:"device_id"`
	PublicKey *string `json:"public_key,omitempty"`
	Status    string  `json:"status"`
}

type DeviceList struct {
	Devices []Device `json:"devices"`
}

type FileSnapshot struct {
	ReferenceKind  string     `json:"reference_kind"`
	ID             string     `json:"id"`
	VersionID      string     `json:"version_id"`
	OwnerID        string     `json:"owner_id"`
	DeviceID       string     `json:"device_id"`
	FileName       string     `json:"file_name"`
	RetentionClass string     `json:"retention_class"`
	State          string     `json:"state"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Version        int64      `json:"version"`
}

type Quota struct {
	ScopeID    string `json:"scope_id"`
	QuotaBytes int64  `json:"quota_bytes"`
	UsedBytes  int64  `json:"used_bytes"`
	Version    int64  `json:"version"`
}

func (c *Client) DeviceAction(ctx context.Context, action, ownerID, deviceID, publicKey, signingPublicKey string) (Device, error) {
	var device Device
	request := map[string]string{"owner_id": ownerID, "device_id": deviceID, "public_key": publicKey, "signing_public_key": signingPublicKey}
	err := c.doJSON(ctx, http.MethodPost, "/v1/devices/"+action, request, &device)
	return device, err
}

func (c *Client) ListDevices(ctx context.Context, ownerID string) (DeviceList, error) {
	var devices DeviceList
	err := c.doJSON(ctx, http.MethodGet, "/v1/devices?owner_id="+url.QueryEscape(ownerID), nil, &devices)
	return devices, err
}

func (c *Client) RecipientPublicKey(ctx context.Context, ownerID, deviceID string) (string, error) {
	var result struct {
		PublicKey string `json:"public_key"`
	}
	err := c.doJSON(ctx, http.MethodGet, "/v1/devices/"+url.PathEscape(ownerID)+"/"+url.PathEscape(deviceID)+"/public-key", nil, &result)
	return result.PublicKey, err
}

func (c *Client) Broadcast(ctx context.Context, entryID, senderOwner, senderDevice, recipientOwner string) (TransferList, error) {
	var transfers TransferList
	request := map[string]string{"entry_id": entryID, "sender_owner_id": senderOwner, "sender_device_id": senderDevice, "recipient_owner_id": recipientOwner}
	err := c.doJSON(ctx, http.MethodPost, "/v1/broadcasts", request, &transfers)
	return transfers, err
}

func (c *Client) GetFile(ctx context.Context, entryID, ownerID string, access ...string) (FileSnapshot, error) {
	var file FileSnapshot
	query := url.Values{"owner_id": {ownerID}}
	if len(access) >= 2 {
		query.Set("device_id", access[0])
		query.Set("transfer_id", access[1])
	}
	err := c.doJSON(ctx, http.MethodGet, "/v1/files/"+url.PathEscape(entryID)+"?"+query.Encode(), nil, &file)
	return file, err
}

func (c *Client) FileAction(ctx context.Context, entryID, action, ownerID, deviceID string, version int64, transfer ...string) (FileSnapshot, error) {
	var file FileSnapshot
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "expected_version": version}
	if len(transfer) > 0 && transfer[0] != "" {
		request["transfer_id"] = transfer[0]
	}
	err := c.doJSON(ctx, http.MethodPost, "/v1/files/"+url.PathEscape(entryID)+"/"+action, request, &file)
	return file, err
}

func (c *Client) GetQuota(ctx context.Context, scopeID string) (Quota, error) {
	var quota Quota
	err := c.doJSON(ctx, http.MethodGet, "/v1/quota?scope_id="+url.QueryEscape(scopeID), nil, &quota)
	return quota, err
}
