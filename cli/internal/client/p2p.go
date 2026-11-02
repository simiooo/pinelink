package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type P2PCreateRequest struct {
	SenderOwnerID           string `json:"sender_owner_id"`
	SenderDeviceID          string `json:"sender_device_id"`
	RecipientOwnerID        string `json:"recipient_owner_id"`
	RecipientDeviceID       string `json:"recipient_device_id"`
	FileName                string `json:"file_name"`
	SizeBytes               int64  `json:"size_bytes"`
	ContentRoot             string `json:"content_root"`
	EncryptionSuite         string `json:"encryption_suite"`
	EncryptedManifest       []byte `json:"encrypted_manifest"`
	SenderEncryptedManifest []byte `json:"sender_encrypted_manifest"`
}

type P2PSignal struct {
	ReplyTo    int64     `json:"reply_to,omitempty"`
	Sequence   int64     `json:"sequence"`
	Channel    string    `json:"channel"`
	Payload    string    `json:"payload"`
	SenderRole string    `json:"sender_role"`
	CreatedAt  time.Time `json:"created_at"`
}

type P2PSignalList struct {
	Signals []P2PSignal `json:"signals"`
}

func (c *Client) CreateP2P(ctx context.Context, request P2PCreateRequest) (Transfer, error) {
	var transfer Transfer
	err := c.doJSON(ctx, http.MethodPost, "/v1/p2p", request, &transfer)
	return transfer, err
}

func (c *Client) SubmitP2PSignal(ctx context.Context, transferID, ownerID, deviceID, role, channel, payload string, replyTo int64) (P2PSignal, error) {
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "role": role, "channel": channel, "payload": payload, "reply_to": replyTo}
	var signal P2PSignal
	err := c.doJSON(ctx, http.MethodPost, "/v1/p2p/"+url.PathEscape(transferID)+"/signals", request, &signal)
	return signal, err
}
func (c *Client) AddP2PSignal(ctx context.Context, transferID, ownerID, deviceID, role, channel, payload string) error {
	_, err := c.SubmitP2PSignal(ctx, transferID, ownerID, deviceID, role, channel, payload, 0)
	return err
}

func (c *Client) P2PSignals(ctx context.Context, transferID, ownerID, deviceID string, after int64) (P2PSignalList, error) {
	var signals P2PSignalList
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}, "after": {strconv.FormatInt(after, 10)}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/p2p/"+url.PathEscape(transferID)+"/signals?"+query.Encode(), nil, &signals)
	return signals, err
}

func (c *Client) P2PProgress(ctx context.Context, transferID, ownerID, deviceID string, received int64) error {
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "received_bytes": received}
	return c.doJSON(ctx, http.MethodPost, "/v1/p2p/"+url.PathEscape(transferID)+"/progress", request, nil)
}

type P2PProgressSnapshot struct {
	TransferID       string    `json:"transfer_id"`
	AttemptID        string    `json:"attempt_id"`
	State            string    `json:"state"`
	StateVersion     int64     `json:"state_version"`
	ProgressRevision int64     `json:"progress_revision"`
	Direction        string    `json:"direction"`
	DisplayMetricID  string    `json:"display_metric_id"`
	ExpiresAt        time.Time `json:"expires_at"`
	Route            struct {
		Kind  string `json:"kind"`
		Epoch int64  `json:"epoch"`
	} `json:"route"`
	Delivery ByteObservation `json:"delivery"`
	Spool    struct {
		ByteObservation
		StateVersion int64      `json:"state_version"`
		SealedAt     *time.Time `json:"sealed_at"`
	} `json:"spool"`
	Completion struct {
		Delivered bool `json:"delivered"`
		Stored    bool `json:"stored"`
	} `json:"completion"`
	Capabilities map[string]bool `json:"capabilities"`
}

type ByteObservation struct {
	State             string     `json:"state"`
	Basis             string     `json:"basis"`
	AcknowledgedBytes int64      `json:"acknowledged_bytes"`
	TotalBytes        *int64     `json:"total_bytes"`
	Revision          int64      `json:"revision"`
	SampledAt         *time.Time `json:"sampled_at"`
}

func (c *Client) GetP2PProgress(ctx context.Context, id, ownerID, deviceID string) (P2PProgressSnapshot, error) {
	var response P2PProgressSnapshot
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/p2p/"+url.PathEscape(id)+"/progress?"+query.Encode(), nil, &response)
	return response, err
}

func (c *Client) ObserveP2PProgress(ctx context.Context, transferID, ownerID, deviceID string, received int64, route string, epoch int64) error {
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "received_bytes": received, "route": route, "route_epoch": epoch}
	return c.doJSON(ctx, http.MethodPost, "/v1/p2p/"+url.PathEscape(transferID)+"/progress", request, nil)
}

func (c *Client) CompleteP2P(ctx context.Context, transferID, ownerID, deviceID string, version int64) (Transfer, error) {
	var transfer Transfer
	request := map[string]any{"owner_id": ownerID, "device_id": deviceID, "expected_version": version}
	err := c.doJSON(ctx, http.MethodPost, "/v1/p2p/"+url.PathEscape(transferID)+"/complete", request, &transfer)
	return transfer, err
}
