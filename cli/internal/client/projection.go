package client

import (
	"context"
	"net/http"
	"net/url"
)

type TaskProjection struct {
	State         string  `json:"state"`
	ProgressBytes int64   `json:"progress_bytes"`
	TotalBytes    int64   `json:"total_bytes"`
	Version       int64   `json:"version"`
	ErrorCode     *string `json:"error_code,omitempty"`
}

func (c *Client) TaskProjection(ctx context.Context, id, ownerID, deviceID string) (TaskProjection, error) {
	var projection TaskProjection
	query := url.Values{"owner_id": {ownerID}, "device_id": {deviceID}}
	err := c.doJSON(ctx, http.MethodGet, "/v1/tasks/"+url.PathEscape(id)+"/projection?"+query.Encode(), nil, &projection)
	return projection, err
}
