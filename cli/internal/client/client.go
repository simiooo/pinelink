package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

type Client struct {
	baseURL  string
	http     *http.Client
	ownerID  string
	deviceID string
}

func New(baseURL string, identity *secure.Identity, ownerID, deviceID string) (*Client, error) {
	return NewWithClock(baseURL, identity, ownerID, deviceID, 0)
}
func NewWithClock(baseURL string, identity *secure.Identity, ownerID, deviceID string, offset time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}
	transport := &signingTransport{base: http.DefaultTransport, identity: identity, ownerID: ownerID, deviceID: deviceID, offset: offset}
	return &Client{baseURL: baseURL, ownerID: ownerID, deviceID: deviceID, http: &http.Client{Timeout: 5 * time.Minute, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Prepare(ctx context.Context, request PrepareRequest) (PrepareResponse, error) {
	var response PrepareResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/uploads/prepare", request, &response)
	return response, err
}

func (c *Client) UploadChunk(ctx context.Context, uploadID string, index int, digest string, data []byte) error {
	path := fmt.Sprintf("/v1/uploads/%s/chunks/%d", url.PathEscape(uploadID), index)
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-Chunk-SHA256", digest)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError(response)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	return nil
}

func (c *Client) CompleteUpload(ctx context.Context, uploadID string) (CompleteResponse, error) {
	var response CompleteResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/uploads/"+url.PathEscape(uploadID)+"/complete", nil, &response)
	return response, err
}

func (c *Client) CommitReuse(ctx context.Context, token string) (CompleteResponse, error) {
	var response CompleteResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/uploads/reuse/commit", map[string]string{"reuse_token": token}, &response)
	return response, err
}

func (c *Client) CreatePull(ctx context.Context, entryID, ownerID, deviceID, transferID string) (Pull, error) {
	var response Pull
	request := map[string]string{"owner_id": ownerID, "device_id": deviceID, "transfer_id": transferID}
	err := c.doJSON(ctx, http.MethodPost, "/v1/files/"+url.PathEscape(entryID)+"/pulls", request, &response)
	return response, err
}

func (c *Client) Content(ctx context.Context, pullID string, offset int64) (*http.Response, error) {
	return c.ContentRange(ctx, pullID, offset, -1)
}

func (c *Client) ContentRange(ctx context.Context, pullID string, start, end int64) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/pulls/"+url.PathEscape(pullID)+"/content", nil)
	if err != nil {
		return nil, err
	}
	if end >= 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	} else if start > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		return nil, responseError(response)
	}
	return response, nil
}

func (c *Client) CompletePull(ctx context.Context, pullID, deviceID, contentRoot string) (PullCompleteResponse, error) {
	var response PullCompleteResponse
	request := map[string]string{"device_id": deviceID, "content_root": contentRoot}
	err := c.doJSON(ctx, http.MethodPost, "/v1/pulls/"+url.PathEscape(pullID)+"/complete", request, &response)
	return response, err
}

func (c *Client) doJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return responseError(response)
	}
	if output == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func responseError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var apiError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Current struct {
			State string `json:"state"`
		} `json:"current"`
	}
	if json.Unmarshal(body, &apiError) == nil && apiError.Message != "" {
		if apiError.Code == "STATE_CONFLICT" && apiError.Current.State != "" {
			return fmt.Errorf("server returned %s (%s): %s (state=%s)", response.Status, apiError.Code, apiError.Message, apiError.Current.State)
		}
		return fmt.Errorf("server returned %s (%s): %s", response.Status, apiError.Code, apiError.Message)
	}
	return fmt.Errorf("server returned %s", response.Status)
}
