package accounts

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

const googleIssuer = "https://accounts.google.com"

type GoogleClaims struct{ Subject, Email, Name string }
type GoogleProvider interface {
	URL(string, string, string) string
	Exchange(context.Context, string, string, string) (GoogleClaims, error)
}
type oidcGoogle struct {
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func newGoogleProvider(cfg Config) (GoogleProvider, error) {
	u, err := url.Parse(cfg.GoogleRedirectURI)
	if err != nil || u.Scheme+"://"+u.Host != cfg.Origin || u.User != nil || u.RawQuery != "" || u.Fragment != "" || cfg.GoogleClientID == "" || cfg.GoogleClientSecret == "" {
		return nil, fmt.Errorf("invalid Google configuration")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	ctx := oidc.ClientContext(context.Background(), client)
	keys := oidc.NewRemoteKeySet(ctx, "https://www.googleapis.com/oauth2/v3/certs")
	return &oidcGoogle{config: oauth2.Config{ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret, RedirectURL: cfg.GoogleRedirectURI, Scopes: []string{"openid", "email", "profile"}, Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token", AuthStyle: oauth2.AuthStyleInParams}}, verifier: oidc.NewVerifier(googleIssuer, keys, &oidc.Config{ClientID: cfg.GoogleClientID, SupportedSigningAlgs: []string{"RS256"}})}, nil
}
func (p *oidcGoogle) URL(state, nonce, verifier string) string {
	return p.config.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", "select_account"))
}
func (p *oidcGoogle) Exchange(ctx context.Context, code, verifier, nonce string) (GoogleClaims, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	ctx = oidc.ClientContext(ctx, client)
	token, err := p.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return GoogleClaims{}, denied()
	}
	raw, _ := token.Extra("id_token").(string)
	id, err := p.verifier.Verify(ctx, raw)
	if err != nil || id.Subject == "" || len(id.Subject) > 255 || nonce == "" || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(nonce)) != 1 {
		return GoogleClaims{}, denied()
	}
	var claims struct {
		Email      string `json:"email"`
		Verified   bool   `json:"email_verified"`
		Name       string `json:"name"`
		Authorized string `json:"azp"`
	}
	if id.Claims(&claims) != nil || !claims.Verified || len(claims.Email) > 320 || len(claims.Name) > 512 || (len(id.Audience) > 1 && claims.Authorized != p.config.ClientID) || (claims.Authorized != "" && claims.Authorized != p.config.ClientID) {
		return GoogleClaims{}, denied()
	}
	return GoogleClaims{Subject: id.Subject, Email: claims.Email, Name: claims.Name}, nil
}

type googleFlow struct {
	State, Verifier string
	Created         time.Time
}

func (s *Service) GoogleStart(ctx context.Context, r Request) (string, error) {
	if s.google == nil {
		return "", fault(503, "GOOGLE_UNAVAILABLE")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	current, err := s.lockRequest(ctx, tx, r)
	if err != nil {
		return "", err
	}
	if current.Status != "PENDING" {
		return "", fault(409, "AUTH_STATE_CHANGED")
	}
	var encrypted []byte
	var nonce *string
	if err = tx.QueryRow(ctx, `SELECT provider_verifier,provider_nonce FROM account_auth_requests WHERE id=$1`, r.ID).Scan(&encrypted, &nonce); err != nil {
		return "", err
	}
	var flow googleFlow
	if len(encrypted) > 0 {
		plain, err := s.open(encrypted)
		if err != nil || json.Unmarshal(plain, &flow) != nil {
			return "", denied()
		}
	} else {
		flow = googleFlow{State: randomToken(32), Verifier: randomToken(32), Created: s.now()}
		n := randomToken(32)
		nonce = &n
		data, _ := json.Marshal(flow)
		if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET provider_state_hash=$2,provider_nonce=$3,provider_verifier=$4 WHERE id=$1`, r.ID, s.mac("google-state", flow.State), n, s.seal(data)); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	// Native clients first bind the external browser; web clients retain their initiating cookie.
	return s.config.Origin + "/api/v1/auth/google/open?state=" + url.QueryEscape(flow.State), nil
}
func (s *Service) GoogleOpen(ctx context.Context, state, browser string) (string, error) {
	if len(state) != 43 || s.google == nil {
		return "", denied()
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	r, err := requestRow(tx.QueryRow(ctx, `SELECT `+requestFields+` FROM account_auth_requests WHERE provider_state_hash=$1 FOR UPDATE`, s.mac("google-state", state)))
	if err != nil || r.Status != "PENDING" || !s.now().Before(r.Expires) {
		return "", denied()
	}
	if len(r.BrowserHash) == 0 {
		if r.Kind != "native" {
			return "", denied()
		}
		if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET browser_hash=$2 WHERE id=$1`, r.ID, s.mac("browser", browser)); err != nil {
			return "", err
		}
	} else if subtle.ConstantTimeCompare(r.BrowserHash, s.mac("browser", browser)) != 1 {
		return "", denied()
	}
	var encrypted []byte
	var nonce string
	if err = tx.QueryRow(ctx, `SELECT provider_verifier,provider_nonce FROM account_auth_requests WHERE id=$1`, r.ID).Scan(&encrypted, &nonce); err != nil {
		return "", err
	}
	plain, err := s.open(encrypted)
	var flow googleFlow
	if err != nil || json.Unmarshal(plain, &flow) != nil {
		return "", denied()
	}
	if s.now().Sub(flow.Created) > 10*time.Minute {
		return "", fault(410, "AUTH_EXPIRED")
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return s.google.URL(state, nonce, flow.Verifier), nil
}
func (s *Service) GoogleCallback(ctx context.Context, state, code, providerError, browser string) (Request, error) {
	var empty Request
	if s.google == nil || len(state) != 43 {
		return empty, denied()
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	r, err := requestRow(tx.QueryRow(ctx, `SELECT `+requestFields+` FROM account_auth_requests WHERE provider_state_hash=$1 FOR UPDATE`, s.mac("google-state", state)))
	if err != nil {
		return empty, err
	}
	if r.Status != "PENDING" || !s.now().Before(r.Expires) || subtle.ConstantTimeCompare(r.BrowserHash, s.mac("browser", browser)) != 1 {
		return empty, denied()
	}
	var encrypted []byte
	var nonce string
	if err = tx.QueryRow(ctx, `SELECT provider_verifier,provider_nonce FROM account_auth_requests WHERE id=$1`, r.ID).Scan(&encrypted, &nonce); err != nil {
		return empty, err
	}
	plain, err := s.open(encrypted)
	var flow googleFlow
	if err != nil || json.Unmarshal(plain, &flow) != nil {
		return empty, denied()
	}
	if s.now().Sub(flow.Created) > 10*time.Minute {
		return r, fault(410, "AUTH_EXPIRED")
	}
	status := "GOOGLE_EXCHANGING"
	if providerError != "" || code == "" {
		status = "CANCELLED"
	}
	if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET status=$2,provider_state_hash=NULL,provider_verifier=NULL WHERE id=$1`, r.ID, status); err != nil {
		return empty, err
	}
	if err = tx.Commit(ctx); err != nil {
		return empty, err
	}
	if status == "CANCELLED" {
		return r, fault(400, "GOOGLE_CANCELLED")
	}
	claims, err := s.google.Exchange(ctx, code, flow.Verifier, nonce)
	if err != nil {
		_, _ = s.db.Exec(ctx, `UPDATE account_auth_requests SET status='FAILED' WHERE id=$1 AND status='GOOGLE_EXCHANGING'`, r.ID)
		return r, denied()
	}
	tx, err = s.db.Begin(ctx)
	if err != nil {
		return r, err
	}
	defer tx.Rollback(ctx)
	r, err = s.lockRequest(ctx, tx, r)
	if err != nil {
		return r, err
	}
	if r.Status != "GOOGLE_EXCHANGING" {
		return r, denied()
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,1))`, googleIssuer+"/"+claims.Subject); err != nil {
		return r, err
	}
	var account string
	err = tx.QueryRow(ctx, `SELECT account_id FROM account_identities WHERE issuer=$1 AND subject=$2`, googleIssuer, claims.Subject).Scan(&account)
	if errors.Is(err, pgx.ErrNoRows) {
		account, err = s.enrollAccount(ctx, tx, claims.Email, claims.Name)
		if err != nil {
			return r, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO account_identities(issuer,subject,account_id) VALUES($1,$2,$3)`, googleIssuer, claims.Subject, account); err != nil {
			return r, err
		}
	} else if err != nil {
		return r, err
	}
	var epoch int64
	if err = tx.QueryRow(ctx, `SELECT credential_epoch FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&epoch); err != nil {
		return r, err
	}
	status = "READY"
	if r.Kind == "native" {
		status = "CONSENT"
	}
	if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET status=$2,account_id=$3,credential_epoch=$4,ready_until=now()+interval '5 minutes' WHERE id=$1`, r.ID, status, account, epoch); err != nil {
		return r, err
	}
	if err = tx.Commit(ctx); err != nil {
		return r, err
	}
	r.Status = status
	r.AccountID = &account
	r.Epoch = &epoch
	return r, nil
}
func (s *Service) NativeReview(ctx context.Context, id, browser string) (map[string]any, error) {
	r, err := s.Request(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.Kind != "native" || r.Status != "CONSENT" || !s.now().Before(r.Expires) || subtle.ConstantTimeCompare(r.BrowserHash, s.mac("browser", browser)) != 1 {
		return nil, denied()
	}
	var fresh bool
	if err = s.db.QueryRow(ctx, `SELECT ready_until>clock_timestamp() FROM account_auth_requests WHERE id=$1`, id).Scan(&fresh); err != nil {
		return nil, err
	}
	if !fresh {
		return nil, fault(410, "AUTH_EXPIRED")
	}
	bytes := s.mac("device-code", r.ID)
	code := fmt.Sprintf("%08d", binary.BigEndian.Uint32(bytes[:4])%100000000)
	return map[string]any{"id": r.ID, "device_name": r.DeviceName, "code": code, "csrf": fmt.Sprintf("%x", s.mac("native-confirm", browser+"/"+r.ID))}, nil
}
func (s *Service) NativeConfirm(ctx context.Context, id, browser, csrf string, accept bool) error {
	if _, err := s.NativeReview(ctx, id, browser); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(csrf), []byte(fmt.Sprintf("%x", s.mac("native-confirm", browser+"/"+id)))) != 1 {
		return denied()
	}
	status := "CANCELLED"
	if accept {
		status = "READY"
	}
	result, err := s.db.Exec(ctx, `UPDATE account_auth_requests SET status=$2,ready_until=now()+interval '5 minutes' WHERE id=$1 AND status='CONSENT' AND expires_at>now() AND ready_until>clock_timestamp()`, id, status)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fault(409, "AUTH_STATE_CHANGED")
	}
	return nil
}
func (s *Service) RequestState(ctx context.Context, r Request) (map[string]any, error) {
	result := map[string]any{"request": r}
	if r.AccountID != nil && (r.Status == "READY" || r.Status == "CLAIMED") {
		a, err := s.Account(ctx, *r.AccountID)
		if err != nil {
			return nil, err
		}
		result["account"] = a
	}
	if r.Kind == "native" {
		bytes := s.mac("device-code", r.ID)
		result["device_code"] = fmt.Sprintf("%08d", binary.BigEndian.Uint32(bytes[:4])%100000000)
	}
	return result, nil
}
