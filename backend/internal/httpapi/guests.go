package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/simiooo/pinelink/backend/internal/accounts"
	"github.com/simiooo/pinelink/backend/internal/auth"
	"github.com/simiooo/pinelink/backend/internal/service"
)

func guestCookieName(r *http.Request) string {
	if r.Header.Get("X-Pinelink-Surface") == "app" {
		return "__Host-pinelink_flutter_guest"
	}
	return "__Host-pinelink_guest"
}
func guestToken(r *http.Request) string {
	if v := r.Header.Get("X-Pinelink-Guest-Session"); v != "" {
		return v
	}
	return cookieValue(r, guestCookieName(r))
}
func (a *API) guestCSRF(r *http.Request, token string) bool {
	if cookieValue(r, guestCookieName(r)) == "" && r.Header.Get("X-Pinelink-Guest-Session") != "" {
		return r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") == ""
	}
	return r.Header.Get("Origin") == a.service.Accounts().Origin() && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Pinelink-CSRF")), []byte(a.service.Accounts().CSRF(token))) == 1
}
func readGuestBody(r *http.Request, v any) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r.Body, 16385))
	if e != nil || len(b) > 16384 || !utf8.Valid(b) || json.Unmarshal(b, v) != nil {
		return nil, &accounts.Error{Status: 400, Code: "GUEST_INVALID_REQUEST"}
	}
	return b, nil
}
func (a *API) registerGuests(mux *http.ServeMux) {
	s := a.service.Accounts()
	if s == nil || !s.GuestsEnabled() {
		return
	}
	mux.HandleFunc("POST /v1/guest/challenge", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if !authOrigin(r, s) {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_ORIGIN"})
			return
		}
		if e := a.authRate(r, "guest-start", 120, time.Hour); e != nil {
			authFailure(w, e)
			return
		}
		var in accounts.GuestChallengeInput
		if _, e := readGuestBody(r, &in); e != nil {
			authFailure(w, e)
			return
		}
		if in.Kind == "native" && (r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "") {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_CLIENT_KIND"})
			return
		}
		if (in.Surface == "app") != (r.Header.Get("X-Pinelink-Surface") == "app") {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_SURFACE"})
			return
		}
		result, e := s.GuestChallenge(r.Context(), in)
		if e != nil {
			authFailure(w, e)
			return
		}
		writeJSON(w, 201, result)
	})
	mux.HandleFunc("POST /v1/guest/enroll", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if !authOrigin(r, s) {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_ORIGIN"})
			return
		}
		if e := a.authRate(r, "guest-enroll", 120, time.Hour); e != nil {
			authFailure(w, e)
			return
		}
		var in accounts.GuestEnrollInput
		b, e := readGuestBody(r, &in)
		if e != nil {
			authFailure(w, e)
			return
		}
		surface := "web"
		if r.Header.Get("X-Pinelink-Surface") == "app" {
			surface = "app"
		}
		p, e := s.EnrollGuest(r.Context(), in, b, r.Header.Get("X-Pinelink-Signature"), accounts.GuestClientContext{Surface: surface, NativeAllowed: r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") == ""})
		if e != nil {
			authFailure(w, e)
			return
		}
		if p.Kind == "native" && (r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "") {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_CLIENT_KIND"})
			return
		}
		if (p.Surface == "app") != (r.Header.Get("X-Pinelink-Surface") == "app") {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_SURFACE"})
			return
		}
		response := map[string]any{"guest": p, "csrf": s.CSRF(p.Token)}
		if p.Kind == "web" {
			http.SetCookie(w, &http.Cookie{Name: guestCookieName(r), Value: p.Token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
		} else {
			response["session_token"] = p.Token
		}
		writeJSON(w, 200, response)
	})
	mux.HandleFunc("POST /v1/guest/invites", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if e := a.authRate(r, "guest-invite", 120, time.Hour); e != nil {
			authFailure(w, e)
			return
		}
		var in struct {
			ID string `json:"request_id"`
		}
		if !readRequest(w, r, &in) {
			return
		}
		v, e := a.service.CreateGuestInvite(r.Context(), in.ID)
		respond(w, v, e)
	})
	mux.HandleFunc("POST /v1/guest/join", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if e := a.authRate(r, "guest-join", 30, 10*time.Minute); e != nil {
			authFailure(w, e)
			return
		}
		var in struct {
			Token string `json:"token"`
			Code  string `json:"code"`
		}
		if !readRequest(w, r, &in) {
			return
		}
		v, e := a.service.JoinGuestInvite(r.Context(), in.Token, in.Code)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /v1/guest/connections", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		v, e := a.service.GuestConnections(r.Context())
		respond(w, v, e)
	})
	mux.HandleFunc("DELETE /v1/guest/connections/{pair}", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		v, e := a.service.RemoveGuestConnection(r.Context(), r.PathValue("pair"))
		respond(w, v, e)
	})
	mux.HandleFunc("POST /v1/guest/candidates/{candidate}/approve", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		v, e := a.service.RespondGuestCandidate(r.Context(), r.PathValue("candidate"), true)
		respond(w, v, e)
	})
	mux.HandleFunc("POST /v1/guest/candidates/{candidate}/decline", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		v, e := a.service.RespondGuestCandidate(r.Context(), r.PathValue("candidate"), false)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /v1/guest/session", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		p, e := s.GuestSession(r.Context(), guestToken(r))
		if e != nil {
			authFailure(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"guest": p, "csrf": s.CSRF(guestToken(r))})
	})
}

func guestRouteAllowed(r *http.Request) bool {
	p := r.URL.Path
	switch p {
	case "/v1/guest/challenge", "/v1/guest/enroll", "/v1/guest/invites", "/v1/guest/join", "/v1/p2p":
		return r.Method == http.MethodPost
	case "/v1/guest/session", "/v1/guest/connections", "/v1/transfers", "/v1/tasks":
		return r.Method == http.MethodGet
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 3 || parts[0] != "v1" {
		return false
	}
	if len(parts) == 5 && parts[1] == "guest" && parts[2] == "candidates" {
		return r.Method == http.MethodPost && (parts[4] == "approve" || parts[4] == "decline")
	}
	if len(parts) == 4 && parts[1] == "guest" && parts[2] == "connections" {
		return r.Method == http.MethodDelete
	}
	if len(parts) == 3 && (parts[1] == "transfers" || parts[1] == "tasks") {
		return r.Method == http.MethodGet
	}
	if len(parts) != 4 {
		return false
	}
	switch parts[1] {
	case "p2p":
		switch parts[3] {
		case "progress":
			return r.Method == http.MethodGet || r.Method == http.MethodPost
		case "relay", "relay-transport":
			return r.Method == http.MethodGet
		case "relay-ticket", "complete":
			return r.Method == http.MethodPost
		}
	case "transfers":
		return r.Method == http.MethodPost && (parts[3] == "accept" || parts[3] == "reject" || parts[3] == "cancel")
	case "tasks":
		return r.Method == http.MethodPost && (parts[3] == "pause" || parts[3] == "resume" || parts[3] == "cancel")
	}
	// Signalling and future endpoint families are not part of the bounded guest
	// WebSocket transport; never expose them through a broad prefix permission.
	return false
}
func (a *API) mintRelayTicket(w http.ResponseWriter, r *http.Request) {
	authHeaders(w)
	var in struct {
		Role string `json:"role"`
		From *int64 `json:"resume_from,omitempty"`
	}
	if !readRequest(w, r, &in) {
		return
	}
	var from []int64
	if in.From != nil {
		from = append(from, *in.From)
	}
	v, e := a.service.MintRelayTicket(r.Context(), r.PathValue("id"), in.Role, from...)
	respond(w, v, e)
}
func (a *API) browserRelayIdentity(r *http.Request) (auth.Identity, bool, error) {
	var ticket string
	for _, p := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "pinelink-ticket.") {
			if ticket != "" {
				return auth.Identity{}, true, service.ErrUnauthorized
			}
			ticket = strings.TrimPrefix(p, "pinelink-ticket.")
		}
	}
	if ticket == "" {
		return auth.Identity{}, false, nil
	}
	if a.service.Accounts() == nil || r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/v1/p2p/") || !strings.HasSuffix(r.URL.Path, "/relay") || r.Header.Get("Origin") != a.service.Accounts().Origin() {
		return auth.Identity{}, true, service.ErrUnauthorized
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 {
		return auth.Identity{}, true, service.ErrUnauthorized
	}
	i, e := a.service.ConsumeRelayTicket(r.Context(), ticket, parts[2], r.URL.Query().Get("role"))
	if e == nil {
		matched := false
		if i.GuestSessionID != "" {
			for _, name := range []string{"__Host-pinelink_guest", "__Host-pinelink_flutter_guest"} {
				p, err := a.service.Accounts().GuestSession(r.Context(), cookieValue(r, name))
				if err == nil && p.Kind == "web" && p.SessionID == i.GuestSessionID {
					matched = true
					break
				}
			}
		} else {
			for _, name := range []string{sessionCookie, flutterSessionCookie} {
				p, err := a.service.Accounts().Session(r.Context(), cookieValue(r, name))
				if err == nil && p.SessionID == i.SessionID {
					matched = true
					break
				}
			}
		}
		if !matched {
			return auth.Identity{}, true, service.ErrUnauthorized
		}
	}
	return i, true, e
}
