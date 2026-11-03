package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/simiooo/pinelink/backend/internal/auth"
	"github.com/simiooo/pinelink/backend/internal/service"
)

func (a *API) withDeviceAuth(next http.Handler) http.Handler {
	if !a.deviceAuthRequired && a.service.Accounts() == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.deviceAuthRequired && !a.service.Accounts().Required() && !strings.HasPrefix(r.Header.Get("X-Pinelink-Owner"), "acct-") {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/v1/auth/") || (r.Method == http.MethodGet && r.URL.Path == "/v1/replication/info") {
			next.ServeHTTP(w, r)
			return
		}
		ownerID := r.Header.Get("X-Pinelink-Owner")
		deviceID := r.Header.Get("X-Pinelink-Device")
		sessionID := ""
		if accountService := a.service.Accounts(); accountService != nil && (accountService.Required() || strings.HasPrefix(ownerID, "acct-")) {
			token := accountToken(r)
			principal, err := accountService.Session(r.Context(), token)
			if err != nil || principal.AccountID != ownerID || principal.DeviceID != deviceID {
				writeJSON(w, 401, map[string]string{"code": "ACCOUNT_SIGN_IN_REQUIRED"})
				return
			}
			if r.Method != "GET" && r.Method != "HEAD" && !a.accountCSRF(r, token) {
				writeJSON(w, 403, map[string]string{"code": "AUTH_CSRF"})
				return
			}
			sessionID = principal.SessionID
		}
		registration := r.Method == http.MethodPost && r.URL.Path == "/v1/devices/register"
		var body []byte
		if r.Body != nil {
			var err error
			body, err = io.ReadAll(io.LimitReader(r.Body, 16<<20+1))
			if err != nil {
				writeError(w, err)
				return
			}
			_ = r.Body.Close()
		}
		if err := a.service.VerifyDevice(r.Context(), ownerID, deviceID, r.Header.Get("X-Pinelink-Timestamp"), r.Method, r.URL.RequestURI(), body, r.Header.Get("X-Pinelink-Signature"), registration); err != nil {
			writeError(w, err)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/devices/revoke" && !revocationTargetsSelf(body, deviceID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"code": "DEVICE_REVOCATION_FORBIDDEN", "message": "device may revoke its own registration"})
			return
		}
		if len(body) > 0 && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var err error
			body, err = bindAuthenticatedJSON(body, ownerID, deviceID)
			if err != nil {
				writeError(w, service.ErrInvalid)
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		query := r.URL.Query()
		// The authenticated principal is authoritative even when query identity is omitted.
		query.Set("owner_id", ownerID)
		query.Set("device_id", deviceID)
		if sessionID != "" {
			query.Set("scope_id", ownerID)
		}
		r.URL.RawQuery = query.Encode()
		r = r.WithContext(auth.WithIdentity(r.Context(), auth.Identity{OwnerID: ownerID, DeviceID: deviceID, SessionID: sessionID}))
		if sessionID != "" {
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			done := make(chan struct{})
			defer close(done)
			go func() {
				tick := time.NewTicker(time.Second)
				defer tick.Stop()
				for {
					select {
					case <-done:
						return
					case <-ctx.Done():
						return
					case <-tick.C:
						if a.service.Accounts().SessionAlive(ctx, sessionID) != nil {
							cancel()
							return
						}
					}
				}
			}()
			r = r.WithContext(ctx)
		}
		if err := a.authorizeResource(r); err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func revocationTargetsSelf(body []byte, principal string) bool {
	var request struct {
		DeviceID string `json:"device_id"`
	}
	return json.Unmarshal(body, &request) == nil && request.DeviceID == principal && principal != ""
}

func bindAuthenticatedJSON(body []byte, owner, device string) ([]byte, error) {
	// Raw values preserve 64-bit cursors and signed nested envelopes. Decoding
	// through interface{} would round integers above 2^53 before verification.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	for key, value := range map[string]string{"owner_id": owner, "device_id": device, "sender_owner_id": owner, "sender_device_id": device} {
		if _, present := fields[key]; present {
			fields[key], _ = json.Marshal(value)
		}
	}
	return json.Marshal(fields)
}

func (a *API) authorizeResource(r *http.Request) error {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "v1" && parts[1] == "uploads" && parts[2] != "prepare" && parts[2] != "reuse" {
		return a.authorizeUpload(r, parts[2])
	}
	if len(parts) >= 3 && parts[0] == "v1" && parts[1] == "pulls" {
		return a.authorizePull(r, parts[2])
	}
	return nil
}

func (a *API) authorizeUpload(r *http.Request, uploadID string) error {
	identity, ok := auth.FromContext(r.Context())
	if !ok {
		return nil
	}
	return a.service.AuthorizeUpload(r.Context(), uploadID, identity.OwnerID, identity.DeviceID)
}

func (a *API) authorizePull(r *http.Request, pullID string) error {
	identity, ok := auth.FromContext(r.Context())
	if !ok {
		return nil
	}
	return a.service.AuthorizePull(r.Context(), pullID, identity.OwnerID, identity.DeviceID)
}
