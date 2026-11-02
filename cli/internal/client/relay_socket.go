package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/coder/websocket"
)

func (c *Client) RelayTransport(ctx context.Context, id string) (string, error) {
	query := url.Values{"owner_id": {c.ownerID}, "device_id": {c.deviceID}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/p2p/"+url.PathEscape(id)+"/relay-transport?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "webrtc", nil // Older self-hosted servers.
	}
	if response.StatusCode != http.StatusOK {
		return "", responseError(response)
	}
	var result struct {
		Transport string `json:"transport"`
	}
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Transport != "webrtc" && result.Transport != "websocket" {
		return "", fmt.Errorf("unsupported relay transport")
	}
	return result.Transport, nil
}

func (c *Client) DialRelay(ctx context.Context, id, role string) (*websocket.Conn, error) {
	query := url.Values{"owner_id": {c.ownerID}, "device_id": {c.deviceID}, "role": {role}}
	address := c.baseURL + "/v1/p2p/" + url.PathEscape(id) + "/relay?" + query.Encode()
	conn, _, err := websocket.Dial(ctx, address, &websocket.DialOptions{HTTPClient: c.http, CompressionMode: websocket.CompressionDisabled})
	return conn, err
}
