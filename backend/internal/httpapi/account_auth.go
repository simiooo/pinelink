package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/simiooo/pinelink/backend/internal/accounts"
)

const sessionCookie = "__Host-pinelink_session"
const flutterSessionCookie = "__Host-pinelink_flutter_session"

func accountCookieName(r *http.Request) string {
	if r.Header.Get("X-Pinelink-Surface") == "app" {
		return flutterSessionCookie
	}
	return sessionCookie
}
func claimCookieName(request accounts.Request) string {
	if request.Surface == "app" {
		return flutterSessionCookie
	}
	return sessionCookie
}

const browserCookie = "__Host-pinelink_auth"

func authFailure(w http.ResponseWriter, err error) {
	var fault *accounts.Error
	if errors.As(err, &fault) {
		writeJSON(w, fault.Status, map[string]string{"code": fault.Code})
		return
	}
	writeJSON(w, 503, map[string]string{"code": "AUTH_UNAVAILABLE"})
}
func authHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store, no-transform")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
func accountToken(r *http.Request) string {
	if v := r.Header.Get("X-Pinelink-Session"); v != "" {
		return v
	}
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return cookieValue(r, accountCookieName(r))
}
func browserValue(w http.ResponseWriter, r *http.Request) string {
	value := cookieValue(r, browserCookie)
	if len(value) != 43 {
		b := make([]byte, 32)
		rand.Read(b)
		value = base64.RawURLEncoding.EncodeToString(b)
		http.SetCookie(w, &http.Cookie{Name: browserCookie, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 3600})
	}
	return value
}
func authOrigin(r *http.Request, s *accounts.Service) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") == ""
	}
	return origin == s.Origin()
}
func (a *API) registerAccounts(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/auth/capabilities", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if a.service.Accounts() == nil {
			writeJSON(w, 200, map[string]any{"enabled": false, "required": false})
			return
		}
		writeJSON(w, 200, a.service.Accounts().Capabilities())
	})
	if a.service.Accounts() == nil {
		return
	}
	s := a.service.Accounts()
	mux.HandleFunc("POST /v1/auth/requests", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if !authOrigin(r, s) {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_ORIGIN"})
			return
		}
		if err := a.authRate(r, "start", 60, time.Hour); err != nil {
			authFailure(w, err)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8193))
		var in accounts.CreateRequest
		if err != nil || len(body) > 8192 || !utf8.Valid(body) || json.Unmarshal(body, &in) != nil {
			authFailure(w, &accounts.Error{Status: 400, Code: "AUTH_INVALID_REQUEST"})
			return
		}
		if in.Kind == "native" && (r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "") {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_CLIENT_KIND"})
			return
		}
		browser := ""
		if in.Kind == "web" {
			browser = browserValue(w, r)
		}
		result, err := s.Create(r.Context(), in, browser)
		if err != nil {
			authFailure(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"request": result})
	})
	mux.HandleFunc("GET /v1/auth/session", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		token := accountToken(r)
		p, err := s.Session(r.Context(), token)
		if err != nil {
			authFailure(w, err)
			return
		}
		account, err := s.Account(r.Context(), p.AccountID)
		if err != nil {
			authFailure(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"account": account, "device_id": p.DeviceID, "expires_at": p.Expires, "csrf": s.CSRF(token)})
	})
	mux.HandleFunc("POST /v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		token := accountToken(r)
		p, err := s.Session(r.Context(), token)
		if err != nil {
			if cookieValue(r, accountCookieName(r)) != "" && !authOrigin(r, s) {
				authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_ORIGIN"})
				return
			}
			http.SetCookie(w, &http.Cookie{Name: accountCookieName(r), Path: "/", Secure: true, HttpOnly: true, MaxAge: -1})
			writeJSON(w, 200, map[string]bool{"signed_out": true})
			return
		}
		if !a.accountCSRF(r, token) {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_CSRF"})
			return
		}
		if err = s.Logout(r.Context(), p); err != nil {
			authFailure(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: accountCookieName(r), Path: "/", Secure: true, HttpOnly: true, MaxAge: -1})
		writeJSON(w, 200, map[string]bool{"signed_out": true})
	})
	mux.HandleFunc("GET /v1/auth/google/open", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		location, err := s.GoogleOpen(r.Context(), r.URL.Query().Get("state"), browserValue(w, r))
		if err != nil {
			http.Redirect(w, r, s.Origin()+"/login?error=GOOGLE_CANCELLED", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, location, http.StatusSeeOther)
	})
	mux.HandleFunc("GET /v1/auth/google/callback", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		result, err := s.GoogleCallback(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"), r.URL.Query().Get("error"), cookieValue(r, browserCookie))
		if err != nil {
			code := "GOOGLE_UNAVAILABLE"
			if r.URL.Query().Get("error") == "access_denied" {
				code = "GOOGLE_CANCELLED"
			}
			path := "/login"
			if result.Kind == "native" || result.Surface == "app" {
				path = "/auth/complete"
			}
			http.Redirect(w, r, s.Origin()+path+"?error="+code, http.StatusSeeOther)
			return
		}
		path := "/auth/return"
		if result.Surface == "app" {
			path = "/auth/complete"
		}
		if result.Kind == "native" {
			path = "/auth/device"
		}
		http.Redirect(w, r, s.Origin()+path+"?request_id="+url.QueryEscape(result.ID), http.StatusSeeOther)
	})
	mux.HandleFunc("GET /v1/auth/native-review", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		result, err := s.NativeReview(r.Context(), r.URL.Query().Get("request_id"), cookieValue(r, browserCookie))
		if err != nil {
			authFailure(w, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /v1/auth/native-confirm", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if !authOrigin(r, s) || r.Header.Get("Origin") == "" {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_ORIGIN"})
			return
		}
		var in struct {
			ID     string `json:"request_id"`
			Accept bool   `json:"accept"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&in) != nil {
			authFailure(w, &accounts.Error{Status: 400, Code: "AUTH_INVALID_REQUEST"})
			return
		}
		err := s.NativeConfirm(r.Context(), in.ID, cookieValue(r, browserCookie), r.Header.Get("X-Pinelink-CSRF"), in.Accept)
		if err != nil {
			authFailure(w, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"confirmed": true})
	})
	mux.HandleFunc("POST /v1/auth/link/inspect", func(w http.ResponseWriter, r *http.Request) {
		authHeaders(w)
		if !authOrigin(r, s) {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_ORIGIN"})
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&in) != nil {
			authFailure(w, &accounts.Error{Status: 400, Code: "AUTH_INVALID_REQUEST"})
			return
		}
		if err := a.authRate(r, "inspect", 40, time.Minute); err != nil {
			authFailure(w, err)
			return
		}
		result, err := s.InspectLink(r.Context(), in.Token)
		if err != nil {
			authFailure(w, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /v1/auth/requests/{id}/{action}", a.accountAction)
}

type accountInput struct {
	Verifier string `json:"verifier"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Grant    string `json:"reset_grant"`
	accounts.VerifyInput
	accounts.ClaimInput
}

func (a *API) authRate(r *http.Request, kind string, limit int, window time.Duration) error {
	return a.service.Accounts().Rate(r.Context(), "ip/"+kind+"/"+a.trustedClientIP(r), limit, window)
}
func (a *API) trustedClientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if peer, e := netip.ParseAddr(ip); e == nil && a.service.Accounts() != nil {
		ip = a.service.Accounts().ClientIP(peer, r.Header.Get("X-Pinelink-Client-IP"))
	}
	return ip
}
func (a *API) accountCSRF(r *http.Request, token string) bool {
	s := a.service.Accounts()
	if cookieValue(r, accountCookieName(r)) == "" || accountToken(r) != cookieValue(r, accountCookieName(r)) {
		return r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") == ""
	}
	if !authOrigin(r, s) || r.Header.Get("Origin") == "" {
		return false
	}
	want := s.CSRF(token)
	return subtle.ConstantTimeCompare([]byte(want), []byte(r.Header.Get("X-Pinelink-CSRF"))) == 1
}
func (a *API) accountAction(w http.ResponseWriter, r *http.Request) {
	authHeaders(w)
	s := a.service.Accounts()
	if !authOrigin(r, s) {
		authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_ORIGIN"})
		return
	}
	if err := a.authRate(r, "action", 240, time.Minute); err != nil {
		authFailure(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	var in accountInput
	if err != nil || len(body) > 16384 || !utf8.Valid(body) || json.Unmarshal(body, &in) != nil {
		authFailure(w, &accounts.Error{Status: 400, Code: "AUTH_INVALID_REQUEST"})
		return
	}
	request, err := s.Proof(r.Context(), r.PathValue("id"), in.Verifier, r.Header.Get("X-Pinelink-Auth-Signature"), r.Method, r.URL.Path, body, cookieValue(r, browserCookie))
	if err != nil {
		authFailure(w, err)
		return
	}
	var result any = map[string]bool{"ok": true}
	switch r.PathValue("action") {
	case "state":
		result, err = s.RequestState(r.Context(), request)
	case "cancel":
		err = s.Cancel(r.Context(), request)
	case "password-login":
		err = s.PasswordLogin(r.Context(), request, in.Email, in.Password)
		if err == nil {
			request, err = s.Request(r.Context(), request.ID)
			if err == nil {
				result, err = s.RequestState(r.Context(), request)
			}
		}
	case "password-signup":
		result, err = s.StartEmail(r.Context(), request, "signup", in.Email, in.Password)
	case "reset-start":
		result, err = s.StartEmail(r.Context(), request, "reset", in.Email, "")
	case "email-status":
		result, err = s.EmailStatus(r.Context(), request, in.ChallengeID)
	case "email-resend":
		result, err = s.Resend(r.Context(), request, in.ChallengeID, in.Generation)
	case "email-verify":
		result, err = s.VerifyEmail(r.Context(), request, in.VerifyInput)
	case "reset-complete":
		err = s.ResetPassword(r.Context(), request, in.Grant, in.Password)
	case "google-start":
		var location string
		location, err = s.GoogleStart(r.Context(), request)
		result = map[string]string{"url": location}
	case "claim":
		if request.Kind == "web" && ((request.Surface == "app") != (r.Header.Get("X-Pinelink-Surface") == "app")) {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_CLIENT_SURFACE"})
			return
		}
		if request.Kind == "native" && (r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "") {
			authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_CLIENT_KIND"})
			return
		}
		var claim accounts.ClaimResult
		claim, err = s.Claim(r.Context(), request, in.ClaimInput)
		if err == nil {
			if request.Kind == "native" {
				if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
					authFailure(w, &accounts.Error{Status: 403, Code: "AUTH_CLIENT_KIND"})
					return
				}
				result = map[string]any{"account": claim.Account, "device_id": claim.DeviceID, "expires_at": claim.Expires, "continuation": claim.Continue, "session_token": claim.Token}
			} else {
				http.SetCookie(w, &http.Cookie{Name: claimCookieName(request), Value: claim.Token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: claim.Expires, MaxAge: 14 * 24 * 3600})
				result = map[string]any{"account": claim.Account, "device_id": claim.DeviceID, "expires_at": claim.Expires, "continuation": claim.Continue, "csrf": s.CSRF(claim.Token)}
			}
		}
	default:
		err = &accounts.Error{Status: 404, Code: "AUTH_ROUTE_NOT_FOUND"}
	}
	if err != nil {
		authFailure(w, err)
		return
	}
	writeJSON(w, 200, result)
}
