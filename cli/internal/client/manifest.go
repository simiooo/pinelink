package client

import (
	"context"
	"net/http"
	"net/url"
)

type ManifestEnvelope struct {
	OwnerID           string `json:"owner_id"`
	DeviceID          string `json:"device_id"`
	EncryptionSuite   string `json:"encryption_suite"`
	EncryptedManifest []byte `json:"encrypted_manifest"`
}

func (c *Client) GetManifest(ctx context.Context, entryID, ownerID, deviceID, transferID string) (ManifestEnvelope, error) {
	var envelope ManifestEnvelope
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}, "transfer_id": {transferID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/files/"+url.PathEscape(entryID)+"/manifest?"+query.Encode(), nil, &envelope)
	return envelope, err
}

func (c *Client) PutManifestEnvelope(ctx context.Context, entryID, ownerID, deviceID, targetOwnerID, targetDeviceID string, ciphertext []byte) error {
	request := map[string]any{
		"owner_id": ownerID, "device_id": deviceID, "target_owner_id": targetOwnerID,
		"target_device_id": targetDeviceID, "encrypted_manifest": ciphertext,
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/files/"+url.PathEscape(entryID)+"/envelopes", request, nil)
}
