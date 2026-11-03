package accounts

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"filippo.io/age"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Error struct {
	Status int
	Code   string
}

func (e *Error) Error() string            { return e.Code }
func fault(status int, code string) error { return &Error{status, code} }
func denied() error                       { return fault(401, "AUTH_INVALID") }

type Config struct {
	Required                                                                     bool
	Origin, EncryptionKey, GoogleClientID, GoogleClientSecret, GoogleRedirectURI string
	ResendKey, MailFrom, ReplyTo                                                 string
	QuotaBytes                                                                   int64
	BreachCheck                                                                  bool
	TrustedProxies                                                               []netip.Prefix
	MailClient                                                                   *http.Client
}
type Service struct {
	db       *pgxpool.Pool
	config   Config
	key      []byte
	kdfSlots chan struct{}
	dummy    string
	google   GoogleProvider
	sender   MailSender
	now      func() time.Time
}

func New(db *pgxpool.Pool, cfg Config) (*Service, error) {
	if !cfg.Required && cfg.Origin == "" {
		return nil, nil
	}
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
		return nil, fmt.Errorf("invalid account origin")
	}
	cfg.Origin = strings.TrimSuffix(cfg.Origin, "/")
	key, err := base64.StdEncoding.DecodeString(cfg.EncryptionKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("account encryption key must be 32 bytes")
	}
	if cfg.QuotaBytes <= 0 {
		cfg.QuotaBytes = 1 << 30
	}
	s := &Service{db: db, config: cfg, key: key, kdfSlots: make(chan struct{}, 2), now: time.Now}
	s.config.BreachCheck = false
	s.dummy, err = s.passwordHash(context.Background(), "a non-user authentication dummy password 2026!")
	if err != nil {
		return nil, err
	}
	s.config.BreachCheck = cfg.BreachCheck
	if cfg.GoogleClientID != "" || cfg.GoogleClientSecret != "" {
		s.google, err = newGoogleProvider(cfg)
		if err != nil {
			return nil, err
		}
	}
	if cfg.ResendKey != "" {
		if cfg.MailFrom == "" {
			return nil, fmt.Errorf("mail sender required")
		}
		client := cfg.MailClient
		if client == nil {
			client = &http.Client{Timeout: 12 * time.Second}
		}
		s.sender = &ResendSender{Key: cfg.ResendKey, From: cfg.MailFrom, ReplyTo: cfg.ReplyTo, Client: client}
	}
	return s, nil
}
func (s *Service) Required() bool { return s != nil && s.config.Required }
func (s *Service) Origin() string { return s.config.Origin }
func (s *Service) Capabilities() map[string]any {
	return map[string]any{"enabled": true, "required": s.config.Required, "google": s.google != nil, "password": true, "email": s.sender != nil, "password_min": 15, "password_max": 128}
}
func (s *Service) Rate(ctx context.Context, bucket string, limit int, window time.Duration) error {
	return s.rate(ctx, s.db, bucket, limit, window)
}

type rateDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) rate(ctx context.Context, db rateDB, bucket string, limit int, window time.Duration) error {
	stamp := s.now().Unix() / int64(window.Seconds())
	var count int
	err := db.QueryRow(ctx, `INSERT INTO account_rate_limits(key_hash,window_start,hits,expires_at) VALUES($1,$2,1,$3) ON CONFLICT(key_hash,window_start) DO UPDATE SET hits=account_rate_limits.hits+1 RETURNING hits`, s.mac("rate", bucket), stamp, s.now().Add(window+time.Hour)).Scan(&count)
	if err != nil {
		return err
	}
	if count > limit {
		return fault(429, "AUTH_RATE_LIMITED")
	}
	return nil
}

type Request struct {
	Surface     string    `json:"surface"`
	ID          string    `json:"id"`
	ProofKey    string    `json:"-"`
	Challenge   string    `json:"-"`
	Kind        string    `json:"client_kind"`
	BrowserHash []byte    `json:"-"`
	Continue    string    `json:"continuation"`
	DeviceName  string    `json:"device_name"`
	Status      string    `json:"status"`
	AccountID   *string   `json:"-"`
	Epoch       *int64    `json:"-"`
	Expires     time.Time `json:"expires_at"`
}
type CreateRequest struct {
	Surface    string `json:"surface"`
	ProofKey   string `json:"proof_key"`
	Challenge  string `json:"challenge"`
	Kind       string `json:"client_kind"`
	Continue   string `json:"continuation"`
	DeviceName string `json:"device_name"`
}

func (s *Service) Create(ctx context.Context, in CreateRequest, browser string) (Request, error) {
	key, err := base64.StdEncoding.DecodeString(in.ProofKey)
	c, err2 := base64.RawURLEncoding.DecodeString(in.Challenge)
	if err != nil || len(key) != 32 || err2 != nil || len(c) != 32 || (in.Kind != "web" && in.Kind != "native") || len(in.DeviceName) > 80 {
		return Request{}, fault(400, "AUTH_INVALID_REQUEST")
	}
	if in.Surface == "" {
		in.Surface = "web"
	}
	if in.Surface != "web" && in.Surface != "app" {
		return Request{}, fault(400, "AUTH_INVALID_REQUEST")
	}
	switch in.Continue {
	case "", "home":
		in.Continue = "home"
	case "tasks", "history", "settings":
	default:
		return Request{}, fault(400, "AUTH_INVALID_CONTINUATION")
	}
	r := Request{Surface: in.Surface, ID: uuid.NewString(), ProofKey: in.ProofKey, Challenge: in.Challenge, Kind: in.Kind, Continue: in.Continue, DeviceName: in.DeviceName, Status: "PENDING", Expires: s.now().Add(time.Hour)}
	if r.Kind == "web" {
		if browser == "" {
			return Request{}, denied()
		}
		r.BrowserHash = s.mac("browser", browser)
	}
	_, err = s.db.Exec(ctx, `INSERT INTO account_auth_requests(id,proof_key,pkce_challenge,client_kind,browser_hash,continuation,device_name,expires_at,surface) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.ID, r.ProofKey, r.Challenge, r.Kind, r.BrowserHash, r.Continue, r.DeviceName, r.Expires, r.Surface)
	return r, err
}
func requestRow(row pgx.Row) (Request, error) {
	var r Request
	err := row.Scan(&r.ID, &r.ProofKey, &r.Challenge, &r.Kind, &r.BrowserHash, &r.Continue, &r.DeviceName, &r.Status, &r.AccountID, &r.Epoch, &r.Expires, &r.Surface)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, denied()
	}
	return r, err
}

const requestFields = `id::text,proof_key,pkce_challenge,client_kind,browser_hash,continuation,device_name,status,account_id,credential_epoch,expires_at,surface`

func (s *Service) Request(ctx context.Context, id string) (Request, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Request{}, denied()
	}
	return requestRow(s.db.QueryRow(ctx, `SELECT `+requestFields+` FROM account_auth_requests WHERE id=$1`, id))
}
func (s *Service) Proof(ctx context.Context, id, verifier, signature, method, path string, body []byte, browser string) (Request, error) {
	r, err := s.Request(ctx, id)
	if err != nil {
		return r, err
	}
	if !s.now().Before(r.Expires) {
		return r, fault(410, "AUTH_EXPIRED")
	}
	challenge := base64.RawURLEncoding.EncodeToString(hash(verifier))
	if len(verifier) < 43 || len(verifier) > 128 || subtle.ConstantTimeCompare([]byte(challenge), []byte(r.Challenge)) != 1 {
		return r, denied()
	}
	if r.Kind == "web" && subtle.ConstantTimeCompare(r.BrowserHash, s.mac("browser", browser)) != 1 {
		return r, denied()
	}
	key, _ := base64.StdEncoding.DecodeString(r.ProofKey)
	sig, err := base64.StdEncoding.DecodeString(signature)
	sum := sha256.Sum256(body)
	payload := "pinelink-auth-v1\n" + method + "\n" + path + "\n" + hex.EncodeToString(sum[:])
	if err != nil || !ed25519.Verify(ed25519.PublicKey(key), []byte(payload), sig) {
		return r, denied()
	}
	return r, nil
}
func (s *Service) lockRequest(ctx context.Context, tx pgx.Tx, r Request) (Request, error) {
	current, err := requestRow(tx.QueryRow(ctx, `SELECT `+requestFields+` FROM account_auth_requests WHERE id=$1 FOR UPDATE`, r.ID))
	if err != nil {
		return current, err
	}
	if !s.now().Before(current.Expires) || current.Status == "CANCELLED" {
		return current, fault(410, "AUTH_EXPIRED")
	}
	return current, nil
}
func (s *Service) PasswordLogin(ctx context.Context, r Request, email, password string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return denied()
	}
	if err = s.Rate(ctx, "login/"+email, 10, 15*time.Minute); err != nil {
		return err
	}
	var account, phc string
	var epoch int64
	err = s.db.QueryRow(ctx, `SELECT p.account_id,p.password_phc,a.credential_epoch FROM account_passwords p JOIN accounts a ON a.id=p.account_id WHERE p.email=$1`, email).Scan(&account, &phc, &epoch)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	found := err == nil
	if !found {
		phc = s.dummy
	}
	valid, err := s.passwordVerify(ctx, password, phc)
	if err != nil {
		return err
	}
	if !valid || !found {
		return fault(401, "INVALID_CREDENTIALS")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	current, err := s.lockRequest(ctx, tx, r)
	if err != nil {
		return err
	}
	if current.Status != "PENDING" {
		if current.Status == "READY" && current.AccountID != nil && *current.AccountID == account {
			return nil
		}
		return fault(409, "AUTH_STATE_CHANGED")
	}
	// Recheck the credential while holding the account lock: reset fences a late KDF.
	var livePHC string
	var liveEpoch int64
	if err = tx.QueryRow(ctx, `SELECT p.password_phc,a.credential_epoch FROM accounts a JOIN account_passwords p ON p.account_id=a.id WHERE a.id=$1 FOR UPDATE OF a`, account).Scan(&livePHC, &liveEpoch); err != nil {
		return err
	}
	if livePHC != phc || liveEpoch != epoch {
		return denied()
	}
	_, err = tx.Exec(ctx, `UPDATE account_auth_requests SET account_id=$2,credential_epoch=$3,status='READY',ready_until=now()+interval '5 minutes' WHERE id=$1`, r.ID, account, epoch)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Account struct {
	ID      string   `json:"id"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Methods []string `json:"methods"`
}

func (s *Service) Account(ctx context.Context, id string) (Account, error) {
	a := Account{ID: id, Methods: []string{}}
	err := s.db.QueryRow(ctx, `SELECT contact_email,display_name FROM accounts WHERE id=$1`, id).Scan(&a.Email, &a.Name)
	if err != nil {
		return a, err
	}
	rows, err := s.db.Query(ctx, `SELECT 'password' FROM account_passwords WHERE account_id=$1 UNION SELECT 'google' FROM account_identities WHERE account_id=$1`, id)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var m string
		if err = rows.Scan(&m); err != nil {
			return a, err
		}
		a.Methods = append(a.Methods, m)
	}
	return a, rows.Err()
}

type Principal struct {
	SessionID, AccountID, DeviceID string
	Expires                        time.Time
}

func (s *Service) Session(ctx context.Context, token string) (Principal, error) {
	var p Principal
	if len(token) != 43 {
		return p, denied()
	}
	err := s.db.QueryRow(ctx, `SELECT s.id::text,s.account_id,s.device_id,s.expires_at FROM account_sessions s JOIN accounts a ON a.id=s.account_id JOIN devices d ON d.owner_id=s.account_id AND d.device_id=s.device_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND a.credential_epoch=s.credential_epoch AND d.status='ACTIVE'`, s.mac("session", token)).Scan(&p.SessionID, &p.AccountID, &p.DeviceID, &p.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, denied()
	}
	return p, err
}
func (s *Service) SessionAlive(ctx context.Context, id string) error {
	var valid bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_sessions s JOIN accounts a ON a.id=s.account_id JOIN devices d ON d.owner_id=s.account_id AND d.device_id=s.device_id WHERE s.id=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND a.credential_epoch=s.credential_epoch AND d.status='ACTIVE')`, id).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return denied()
	}
	return nil
}
func (s *Service) CSRF(token string) string {
	return base64.RawURLEncoding.EncodeToString(s.mac("csrf", token))
}

type ClaimInput struct {
	DeviceID        string `json:"device_id"`
	SigningKey      string `json:"signing_key"`
	EncryptionKey   string `json:"encryption_key"`
	DeviceSignature string `json:"device_signature"`
}
type ClaimResult struct {
	Account  Account   `json:"account"`
	DeviceID string    `json:"device_id"`
	Expires  time.Time `json:"expires_at"`
	Continue string    `json:"continuation"`
	Token    string    `json:"-"`
}

func (s *Service) Claim(ctx context.Context, r Request, in ClaimInput) (ClaimResult, error) {
	var result ClaimResult
	key, err := base64.StdEncoding.DecodeString(in.SigningKey)
	sig, e := base64.StdEncoding.DecodeString(in.DeviceSignature)
	if _, ageErr := age.ParseX25519Recipient(in.EncryptionKey); ageErr != nil {
		return result, denied()
	}
	if r.AccountID == nil || r.Epoch == nil || len(in.DeviceID) == 0 || len(in.DeviceID) > 128 || len(in.EncryptionKey) < 20 || len(in.EncryptionKey) > 200 || err != nil || len(key) != 32 || e != nil {
		return result, denied()
	}
	payload := "pinelink-enrollment-v1\n" + r.ID + "\n" + *r.AccountID + "\n" + in.DeviceID + "\n" + in.SigningKey + "\n" + in.EncryptionKey
	if !ed25519.Verify(ed25519.PublicKey(key), []byte(payload), sig) {
		return result, denied()
	}
	fingerprint := hash(payload)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	current, err := s.lockRequest(ctx, tx, r)
	if err != nil {
		return result, err
	}
	if current.AccountID == nil || *current.AccountID != *r.AccountID {
		return result, denied()
	}
	var epoch int64
	if err = tx.QueryRow(ctx, `SELECT credential_epoch FROM accounts WHERE id=$1 FOR UPDATE`, *r.AccountID).Scan(&epoch); err != nil {
		return result, err
	}
	if current.Epoch == nil || epoch != *current.Epoch {
		return result, denied()
	}
	if current.Status == "CLAIMED" {
		var cipher, fp []byte
		var until time.Time
		if err = tx.QueryRow(ctx, `SELECT result_cipher,claim_fingerprint,claimed_until FROM account_auth_requests WHERE id=$1`, r.ID).Scan(&cipher, &fp, &until); err != nil {
			return result, err
		}
		if !s.now().Before(until) || subtle.ConstantTimeCompare(fp, fingerprint) != 1 {
			return result, denied()
		}
		plain, err := s.open(cipher)
		if err != nil {
			return result, denied()
		}
		var replay struct {
			Token   string
			Device  string
			Expires time.Time
		}
		if json.Unmarshal(plain, &replay) != nil {
			return result, denied()
		}
		var valid bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_sessions s JOIN devices d ON d.owner_id=s.account_id AND d.device_id=s.device_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND s.credential_epoch=$2 AND d.status='ACTIVE')`, s.mac("session", replay.Token), epoch).Scan(&valid); err != nil {
			return result, err
		}
		if !valid {
			return result, denied()
		}
		result.Token = replay.Token
		result.Expires = replay.Expires
	} else {
		if current.Status != "READY" {
			return result, fault(409, "AUTH_NOT_READY")
		}
		var fresh bool
		if err = tx.QueryRow(ctx, `SELECT ready_until>now() FROM account_auth_requests WHERE id=$1`, r.ID).Scan(&fresh); err != nil || !fresh {
			return result, fault(410, "AUTH_EXPIRED")
		}
		var oldKey, oldEnc *string
		var status string
		err = tx.QueryRow(ctx, `SELECT signing_public_key,public_key,status FROM devices WHERE owner_id=$1 AND device_id=$2 FOR UPDATE`, *r.AccountID, in.DeviceID).Scan(&oldKey, &oldEnc, &status)
		if err == nil {
			if status != "ACTIVE" {
				return result, fault(409, "DEVICE_REVOKED")
			}
			if oldKey == nil || *oldKey != in.SigningKey || oldEnc == nil || *oldEnc != in.EncryptionKey {
				return result, denied()
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			if _, err = tx.Exec(ctx, `INSERT INTO devices(owner_id,device_id,public_key,signing_public_key,last_seen_at) VALUES($1,$2,$3,$4,now())`, *r.AccountID, in.DeviceID, in.EncryptionKey, in.SigningKey); err != nil {
				return result, err
			}
		} else {
			return result, err
		}
		result.Token = randomToken(32)
		result.Expires = s.now().Add(14 * 24 * time.Hour)
		if _, err = tx.Exec(ctx, `INSERT INTO account_sessions(id,token_hash,account_id,device_id,credential_epoch,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, uuid.NewString(), s.mac("session", result.Token), *r.AccountID, in.DeviceID, epoch, result.Expires); err != nil {
			return result, err
		}
		encoded, _ := json.Marshal(struct {
			Token, Device string
			Expires       time.Time
		}{result.Token, in.DeviceID, result.Expires})
		if _, err = tx.Exec(ctx, `UPDATE account_auth_requests SET status='CLAIMED',result_cipher=$2,claim_fingerprint=$3,claimed_until=$4 WHERE id=$1`, r.ID, s.seal(encoded), fingerprint, s.now().Add(time.Minute)); err != nil {
			return result, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	result.Account, err = s.Account(ctx, *r.AccountID)
	result.DeviceID = in.DeviceID
	result.Continue = r.Continue
	return result, err
}
func (s *Service) Cancel(ctx context.Context, r Request) error {
	_, err := s.db.Exec(ctx, `UPDATE account_auth_requests SET status='CANCELLED',provider_verifier=NULL,provider_state_hash=NULL WHERE id=$1 AND status NOT IN ('CLAIMED','CANCELLED')`, r.ID)
	return err
}
func (s *Service) Logout(ctx context.Context, p Principal) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, p.AccountID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE account_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1 AND account_id=$2`, p.SessionID, p.AccountID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) enrollAccount(ctx context.Context, tx pgx.Tx, email, name string) (string, error) {
	id := "acct-" + uuid.NewString()
	_, err := tx.Exec(ctx, `INSERT INTO accounts(id,contact_email,display_name) VALUES($1,$2,$3)`, id, email, name)
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO scope_quotas(scope_id,quota_bytes) VALUES($1,$2)`, id, s.config.QuotaBytes)
	return id, err
}
