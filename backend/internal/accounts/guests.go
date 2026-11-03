package accounts

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
	"unicode"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type GuestChallengeInput struct {
	SigningKey string `json:"signing_key"`
	Kind       string `json:"client_kind"`
	Surface    string `json:"surface"`
}
type GuestEnrollInput struct {
	Challenge     string `json:"challenge"`
	DeviceID      string `json:"device_id"`
	EncryptionKey string `json:"encryption_key"`
	Name          string `json:"device_name"`
}
type GuestClientContext struct {
	Surface       string
	NativeAllowed bool
}
type GuestPrincipal struct {
	OwnerID   string    `json:"owner_id"`
	DeviceID  string    `json:"device_id"`
	SessionID string    `json:"-"`
	Name      string    `json:"device_name"`
	Kind      string    `json:"client_kind"`
	Surface   string    `json:"surface"`
	Expires   time.Time `json:"expires_at"`
	Token     string    `json:"-"`
}

var guestDeviceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{3,159}$`)

func (s *Service) OpaqueHash(domain, value string) []byte { return s.mac("guest/"+domain, value) }

func (s *Service) GuestsEnabled() bool { return s != nil && s.config.GuestsEnabled }
func (s *Service) GuestMaxFileBytes() int64 {
	if s.config.GuestMaxFileBytes > 0 {
		return s.config.GuestMaxFileBytes
	}
	return 64 << 20
}
func (s *Service) GuestResourceLimits() (int64, int64, int64) {
	daily, ip, global := s.config.GuestDailyBytes, s.config.GuestIPDailyBytes, s.config.GuestGlobalDailyBytes
	if daily == 0 {
		daily = 256 << 20
	}
	if ip == 0 {
		ip = 512 << 20
	}
	if global == 0 {
		global = 8 << 30
	}
	return daily, ip, global
}
func (s *Service) GuestChallenge(ctx context.Context, in GuestChallengeInput) (map[string]any, error) {
	key, err := base64.StdEncoding.DecodeString(in.SigningKey)
	if !s.GuestsEnabled() || err != nil || len(key) != ed25519.PublicKeySize || base64.StdEncoding.EncodeToString(key) != in.SigningKey || (in.Kind != "web" && in.Kind != "native") || (in.Surface != "web" && in.Surface != "app") {
		return nil, fault(400, "GUEST_INVALID_REQUEST")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// A global budget also bounds unauthenticated challenge storage.
	if err = s.rate(ctx, tx, "guest-challenges", 20000, time.Hour); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	expires := s.now().Add(time.Minute)
	_, err = tx.Exec(ctx, `INSERT INTO guest_challenges(id,signing_key,client_kind,surface,expires_at) VALUES($1,$2,$3,$4,$5)`, id, in.SigningKey, in.Kind, in.Surface, expires)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"challenge": id, "expires_at": expires, "unix_time": s.now().Unix()}, nil
}
func (s *Service) guestToken(id string) string {
	return base64.RawURLEncoding.EncodeToString(s.mac("guest-token", id))
}
func (s *Service) guestHash(token string) []byte { return s.mac("guest-session", token) }
func (s *Service) EnrollGuest(ctx context.Context, in GuestEnrollInput, body []byte, signature string, client GuestClientContext) (GuestPrincipal, error) {
	var result GuestPrincipal
	if !s.GuestsEnabled() || !guestDeviceID.MatchString(in.DeviceID) || len(in.Name) > 100 || strings.TrimSpace(in.Name) == "" || strings.IndexFunc(in.Name, unicode.IsControl) >= 0 {
		return result, fault(400, "GUEST_INVALID_REQUEST")
	}
	if key, err := age.ParseX25519Recipient(in.EncryptionKey); err != nil || key.String() != in.EncryptionKey {
		return result, fault(400, "GUEST_INVALID_KEY")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var key string
	var expires time.Time
	var consumed *time.Time
	var priorHash []byte
	var priorSession *string
	err = tx.QueryRow(ctx, `SELECT signing_key,client_kind,surface,expires_at,consumed_at,body_hash,session_id::text FROM guest_challenges WHERE id=$1 FOR UPDATE`, in.Challenge).Scan(&key, &result.Kind, &result.Surface, &expires, &consumed, &priorHash, &priorSession)
	if err != nil {
		return result, fault(401, "GUEST_PROOF_INVALID")
	}
	// Reject a mismatched browser/native surface before consuming a proof,
	// renewing a lease or creating the identity.
	if result.Surface != client.Surface {
		return result, fault(403, "AUTH_SURFACE")
	}
	if result.Kind == "native" && !client.NativeAllowed {
		return result, fault(403, "AUTH_CLIENT_KIND")
	}
	public, e := base64.StdEncoding.DecodeString(key)
	sig, e2 := base64.StdEncoding.DecodeString(signature)
	digest := sha256.Sum256(body)
	wire := []byte("pinelink-guest-enroll-v1\nPOST\n/v1/guest/enroll\n" + hex.EncodeToString(digest[:]))
	if e != nil || e2 != nil || len(public) != 32 || !ed25519.Verify(public, wire, sig) {
		return result, fault(401, "GUEST_PROOF_INVALID")
	}
	if consumed != nil {
		if priorSession == nil || subtle.ConstantTimeCompare(priorHash, digest[:]) != 1 || s.now().Sub(*consumed) > time.Minute {
			return result, fault(409, "GUEST_PROOF_USED")
		}
		err = tx.QueryRow(ctx, `SELECT g.owner_id,g.device_id,g.display_name,s.id::text,s.expires_at FROM guest_identities g JOIN guest_sessions s ON s.id=g.current_session_id JOIN devices d ON d.owner_id=g.owner_id AND d.device_id=g.device_id WHERE s.id=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND g.revoked_at IS NULL AND d.status='ACTIVE'`, *priorSession).Scan(&result.OwnerID, &result.DeviceID, &result.Name, &result.SessionID, &result.Expires)
		if err != nil {
			return result, fault(401, "GUEST_EXPIRED")
		}
		result.Token = s.guestToken(result.SessionID)
		return result, tx.Commit(ctx)
	}
	if !s.now().Before(expires) {
		return result, fault(410, "GUEST_PROOF_EXPIRED")
	}
	// The signing-key lock serializes first use from tabs without trusting a client owner ID.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,915))`, key); err != nil {
		return result, err
	}
	var storedKey string
	var current *string
	var revoked *time.Time
	err = tx.QueryRow(ctx, `SELECT owner_id,device_id,encryption_key,display_name,client_kind,surface,current_session_id::text,revoked_at FROM guest_identities WHERE signing_key=$1 FOR UPDATE`, key).Scan(&result.OwnerID, &result.DeviceID, &storedKey, &result.Name, &result.Kind, &result.Surface, &current, &revoked)
	if err == pgx.ErrNoRows {
		if err = s.rate(ctx, tx, "guest-identities", 2000, 24*time.Hour); err != nil {
			return result, err
		}
		result.OwnerID = "guest-" + uuid.NewString()
		result.DeviceID = in.DeviceID
		result.Name = in.Name
		// Recover the challenge's trusted surface after scanning a missing identity.
		err = tx.QueryRow(ctx, `SELECT client_kind,surface FROM guest_challenges WHERE id=$1`, in.Challenge).Scan(&result.Kind, &result.Surface)
		if err != nil {
			return result, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO devices(owner_id,device_id,public_key,signing_public_key,last_seen_at) VALUES($1,$2,$3,$4,now())`, result.OwnerID, result.DeviceID, in.EncryptionKey, key); err != nil {
			return result, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO guest_identities(owner_id,device_id,signing_key,encryption_key,display_name,client_kind,surface) VALUES($1,$2,$3,$4,$5,$6,$7)`, result.OwnerID, result.DeviceID, key, in.EncryptionKey, result.Name, result.Kind, result.Surface); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	} else {
		var kind, surface string
		if err = tx.QueryRow(ctx, `SELECT client_kind,surface FROM guest_challenges WHERE id=$1`, in.Challenge).Scan(&kind, &surface); err != nil {
			return result, err
		}
		if revoked != nil || storedKey != in.EncryptionKey || result.DeviceID != in.DeviceID || result.Kind != kind || result.Surface != surface {
			return result, fault(403, "GUEST_IDENTITY_CHANGED")
		}
		var active bool
		if err = tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM devices WHERE owner_id=$1 AND device_id=$2 FOR SHARE`, result.OwnerID, result.DeviceID).Scan(&active); err != nil || !active {
			return result, fault(403, "DEVICE_REVOKED")
		}
	}
	result.Expires = s.now().Add(24 * time.Hour)
	if current != nil {
		var valid bool
		err = tx.QueryRow(ctx, `SELECT revoked_at IS NULL AND expires_at>clock_timestamp() FROM guest_sessions WHERE id=$1 FOR UPDATE`, *current).Scan(&valid)
		if err != nil {
			return result, err
		}
		if valid {
			result.SessionID = *current
		}
	}
	if result.SessionID == "" {
		result.SessionID = uuid.NewString()
		result.Token = s.guestToken(result.SessionID)
		if _, err = tx.Exec(ctx, `INSERT INTO guest_sessions(id,owner_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`, result.SessionID, result.OwnerID, s.guestHash(result.Token), result.Expires); err != nil {
			return result, err
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE guest_sessions SET expires_at=$2 WHERE id=$1`, result.SessionID, result.Expires); err != nil {
			return result, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE guest_identities SET current_session_id=$2 WHERE owner_id=$1`, result.OwnerID, result.SessionID); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE guest_challenges SET consumed_at=now(),body_hash=$2,session_id=$3 WHERE id=$1`, in.Challenge, digest[:], result.SessionID); err != nil {
		return result, err
	}
	result.Token = s.guestToken(result.SessionID)
	return result, tx.Commit(ctx)
}
func (s *Service) GuestSession(ctx context.Context, token string) (GuestPrincipal, error) {
	var p GuestPrincipal
	if !s.GuestsEnabled() || len(token) != 43 {
		return p, fault(401, "GUEST_EXPIRED")
	}
	err := s.db.QueryRow(ctx, `SELECT g.owner_id,g.device_id,g.display_name,g.client_kind,g.surface,s.id::text,s.expires_at FROM guest_sessions s JOIN guest_identities g ON g.owner_id=s.owner_id AND g.current_session_id=s.id JOIN devices d ON d.owner_id=g.owner_id AND d.device_id=g.device_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND g.revoked_at IS NULL AND d.status='ACTIVE'`, s.guestHash(token)).Scan(&p.OwnerID, &p.DeviceID, &p.Name, &p.Kind, &p.Surface, &p.SessionID, &p.Expires)
	if err != nil {
		return p, fault(401, "GUEST_EXPIRED")
	}
	return p, nil
}
func (s *Service) GuestSessionAlive(ctx context.Context, id string) error {
	if !s.GuestsEnabled() {
		return fault(401, "GUEST_EXPIRED")
	}
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM guest_sessions s JOIN guest_identities g ON g.current_session_id=s.id JOIN devices d ON d.owner_id=g.owner_id AND d.device_id=g.device_id WHERE s.id=$1 AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND g.revoked_at IS NULL AND d.status='ACTIVE')`, id).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return fault(401, "GUEST_EXPIRED")
	}
	return nil
}
