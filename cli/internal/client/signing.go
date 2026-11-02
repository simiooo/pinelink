package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

type signingTransport struct {
	base     http.RoundTripper
	identity *secure.Identity
	ownerID  string
	deviceID string
	offset   time.Duration
}

func (t *signingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.identity == nil {
		return t.base.RoundTrip(request)
	}
	var body []byte
	var err error
	if request.Body != nil {
		body, err = io.ReadAll(io.LimitReader(request.Body, 16<<20+1))
		if err != nil {
			return nil, err
		}
		_ = request.Body.Close()
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
	}
	timestamp := strconv.FormatInt(time.Now().Add(t.offset).Unix(), 10)
	digest := sha256.Sum256(body)
	payload := request.Method + "\n" + request.URL.RequestURI() + "\n" + timestamp + "\n" + hex.EncodeToString(digest[:])
	request.Header.Set("X-Pinelink-Owner", t.ownerID)
	request.Header.Set("X-Pinelink-Device", t.deviceID)
	request.Header.Set("X-Pinelink-Timestamp", timestamp)
	request.Header.Set("X-Pinelink-Signature", t.identity.Sign([]byte(payload)))
	return t.base.RoundTrip(request)
}
