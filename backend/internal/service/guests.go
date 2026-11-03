package service

import (
	"context"
	"encoding/base32"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

type GuestInvite struct {
	ID       string    `json:"id"`
	Token    string    `json:"token,omitempty"`
	Code     string    `json:"code,omitempty"`
	Expires  time.Time `json:"expires_at"`
	Consumed bool      `json:"consumed"`
}

var guestCodeEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

type GuestPeer struct {
	InviteID      string    `json:"invite_id"`
	ID            string    `json:"pair_id"`
	Owner         string    `json:"owner_id"`
	Device        string    `json:"device_id"`
	Name          string    `json:"device_name"`
	EncryptionKey string    `json:"encryption_key"`
	SigningKey    string    `json:"signing_key"`
	Expires       time.Time `json:"expires_at"`
	Online        bool      `json:"online"`
}
type GuestCandidate struct {
	ID       string    `json:"id"`
	InviteID string    `json:"invite_id"`
	Name     string    `json:"device_name"`
	Device   string    `json:"device_id"`
	Status   string    `json:"status"`
	Incoming bool      `json:"incoming"`
	Expires  time.Time `json:"expires_at"`
}
type GuestConnectionList struct {
	Invites    []GuestInvite    `json:"invites"`
	Candidates []GuestCandidate `json:"candidates"`
	Peers      []GuestPeer      `json:"peers"`
}

func (s *Service) guestCaller(ctx context.Context) (auth.Identity, error) {
	i, ok := auth.FromContext(ctx)
	if !ok || !i.HasSession() || s.accounts == nil || !s.accounts.GuestsEnabled() {
		return i, ErrUnauthorized
	}
	return i, nil
}
func (s *Service) inviteSecrets(id string) (string, string) {
	// Deterministic secrets allow an authenticated idempotent retry without storing the plaintext.
	token := base64.RawURLEncoding.EncodeToString(s.accounts.OpaqueHash("invite-token", id)[:16])
	code := guestCodeEncoding.EncodeToString(s.accounts.OpaqueHash("invite-code", id)[:5])
	return token, code
}
func (s *Service) CreateGuestInvite(ctx context.Context, id string) (GuestInvite, error) {
	i, e := s.guestCaller(ctx)
	if e != nil {
		return GuestInvite{}, e
	}
	if _, e = uuid.Parse(id); e != nil {
		return GuestInvite{}, ErrInvalid
	}
	tx, e := s.beginAccountTx(ctx)
	if e != nil {
		return GuestInvite{}, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,916))`, i.OwnerID); e != nil {
		return GuestInvite{}, e
	}
	result := GuestInvite{ID: id}
	result.Token, result.Code = s.inviteSecrets(id)
	var owner, device string
	e = tx.QueryRow(ctx, `SELECT owner_id,device_id,expires_at,consumed_at IS NOT NULL OR revoked_at IS NOT NULL FROM guest_invites WHERE id=$1`, id).Scan(&owner, &device, &result.Expires, &result.Consumed)
	if e == nil {
		if owner != i.OwnerID || device != i.DeviceID {
			return GuestInvite{}, ErrNotFound
		}
		return result, tx.Commit(ctx)
	}
	if e != pgx.ErrNoRows {
		return GuestInvite{}, e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM guest_invites WHERE owner_id=$1 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at>clock_timestamp()`, i.OwnerID).Scan(&count); e != nil {
		return GuestInvite{}, e
	}
	if count >= 3 {
		return GuestInvite{}, ErrQuotaExceeded
	}
	result.Expires = s.now().Add(10 * time.Minute)
	_, e = tx.Exec(ctx, `INSERT INTO guest_invites(id,token_hash,code_hash,owner_id,device_id,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, id, s.accounts.OpaqueHash("invite", result.Token), s.accounts.OpaqueHash("code", result.Code), i.OwnerID, i.DeviceID, result.Expires)
	if e != nil {
		return GuestInvite{}, e
	}
	return result, tx.Commit(ctx)
}
func (s *Service) JoinGuestInvite(ctx context.Context, token, code string) (GuestCandidate, error) {
	i, e := s.guestCaller(ctx)
	if e != nil {
		return GuestCandidate{}, e
	}
	code = strings.NewReplacer(" ", "", "-", "", "O", "0", "I", "1", "L", "1").Replace(strings.ToUpper(strings.TrimSpace(code)))
	var hash []byte
	column := "token_hash"
	if token != "" && code == "" && len(token) == 22 {
		hash = s.accounts.OpaqueHash("invite", token)
	} else if token == "" && len(code) == 8 {
		if _, e = guestCodeEncoding.DecodeString(code); e != nil {
			return GuestCandidate{}, ErrInvalid
		}
		column = "code_hash"
		hash = s.accounts.OpaqueHash("code", code)
	} else {
		return GuestCandidate{}, ErrInvalid
	}
	tx, e := s.beginAccountTx(ctx)
	if e != nil {
		return GuestCandidate{}, e
	}
	defer tx.Rollback(ctx)
	var id, owner, device string
	var expires time.Time
	var consumed, revoked bool
	e = tx.QueryRow(ctx, `SELECT id::text,owner_id,device_id,expires_at,consumed_at IS NOT NULL,revoked_at IS NOT NULL FROM guest_invites WHERE `+column+`=$1 FOR UPDATE`, hash).Scan(&id, &owner, &device, &expires, &consumed, &revoked)
	if e == pgx.ErrNoRows {
		return GuestCandidate{}, ErrNotFound
	}
	if e != nil {
		return GuestCandidate{}, e
	}
	if owner == i.OwnerID && device == i.DeviceID {
		return GuestCandidate{}, ErrInvalid
	}
	var result GuestCandidate
	result.InviteID = id
	result.Expires = expires
	// A retry may read its own accepted request, but cannot join someone else's consumed invitation.
	e = tx.QueryRow(ctx, `SELECT id::text,status FROM guest_pair_candidates WHERE invite_id=$1 AND owner_id=$2 AND device_id=$3`, id, i.OwnerID, i.DeviceID).Scan(&result.ID, &result.Status)
	if e == nil {
		if revoked || !s.now().Before(expires) {
			return GuestCandidate{}, ErrExpired
		}
		return result, tx.Commit(ctx)
	}
	if e != pgx.ErrNoRows {
		return result, e
	}
	if consumed || revoked || !s.now().Before(expires) {
		return result, ErrExpired
	}
	var active bool
	if e = tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM devices WHERE owner_id=$1 AND device_id=$2`, owner, device).Scan(&active); e != nil || !active {
		return result, ErrNotFound
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM guest_pair_candidates WHERE invite_id=$1`, id).Scan(&count); e != nil {
		return result, e
	}
	if count >= 8 {
		return result, ErrQuotaExceeded
	}
	result.ID = uuid.NewString()
	result.Status = "WAITING"
	if _, e = tx.Exec(ctx, `INSERT INTO guest_pair_candidates(id,invite_id,owner_id,device_id) VALUES($1,$2,$3,$4)`, result.ID, id, i.OwnerID, i.DeviceID); e != nil {
		return result, e
	}
	return result, tx.Commit(ctx)
}
func (s *Service) RespondGuestCandidate(ctx context.Context, id string, approve bool) (map[string]any, error) {
	i, e := s.guestCaller(ctx)
	if e != nil {
		return nil, e
	}
	if _, e = uuid.Parse(id); e != nil {
		return nil, ErrInvalid
	}
	tx, e := s.beginAccountTx(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var invite, owner, device string
	var expires time.Time
	var consumed, revoked bool
	var selected *string
	e = tx.QueryRow(ctx, `SELECT v.id::text,v.owner_id,v.device_id,v.expires_at,v.consumed_at IS NOT NULL,v.revoked_at IS NOT NULL,v.selected_candidate::text FROM guest_invites v JOIN guest_pair_candidates c ON c.invite_id=v.id WHERE c.id=$1 FOR UPDATE OF v`, id).Scan(&invite, &owner, &device, &expires, &consumed, &revoked, &selected)
	if e == pgx.ErrNoRows || owner != i.OwnerID || device != i.DeviceID {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	var peerOwner, peerDevice, status string
	e = tx.QueryRow(ctx, `SELECT owner_id,device_id,status FROM guest_pair_candidates WHERE id=$1 FOR UPDATE`, id).Scan(&peerOwner, &peerDevice, &status)
	if e != nil {
		return nil, e
	}
	if consumed && selected != nil && *selected == id && approve {
		return map[string]any{"approved": true}, tx.Commit(ctx)
	}
	if consumed || revoked || !s.now().Before(expires) {
		return nil, ErrExpired
	}
	if !approve {
		if status == "APPROVED" {
			return nil, ErrInvalid
		}
		_, e = tx.Exec(ctx, `UPDATE guest_pair_candidates SET status='DECLINED' WHERE id=$1`, id)
		if e != nil {
			return nil, e
		}
		return map[string]any{"approved": false}, tx.Commit(ctx)
	}
	if status != "WAITING" {
		return nil, ErrInvalid
	}
	// A candidate is bound to registered keys and cannot be accepted after revocation.
	var valid bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices d LEFT JOIN guest_identities g ON g.owner_id=d.owner_id LEFT JOIN guest_sessions s ON s.id=g.current_session_id WHERE d.owner_id=$1 AND d.device_id=$2 AND d.status='ACTIVE' AND(g.owner_id IS NULL OR(g.revoked_at IS NULL AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp())))`, peerOwner, peerDevice).Scan(&valid)
	if e != nil {
		return nil, e
	}
	if !valid {
		return nil, ErrUnauthorized
	}
	pair := uuid.NewString()
	_, e = tx.Exec(ctx, `INSERT INTO guest_pair_grants(id,invite_id,owner_a,device_a,owner_b,device_b,expires_at) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()+interval '24 hours')`, pair, invite, i.OwnerID, i.DeviceID, peerOwner, peerDevice)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE guest_invites SET consumed_at=clock_timestamp(),selected_candidate=$2 WHERE id=$1`, invite, id); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE guest_pair_candidates SET status=CASE WHEN id=$2 THEN 'APPROVED' ELSE 'DECLINED' END WHERE invite_id=$1 AND status='WAITING'`, invite, id); e != nil {
		return nil, e
	}
	return map[string]any{"approved": true, "pair_id": pair}, tx.Commit(ctx)
}
func (s *Service) GuestConnections(ctx context.Context) (GuestConnectionList, error) {
	result := GuestConnectionList{Invites: []GuestInvite{}, Candidates: []GuestCandidate{}, Peers: []GuestPeer{}}
	i, e := s.guestCaller(ctx)
	if e != nil {
		return result, e
	}
	if _, e = s.db.Exec(ctx, `UPDATE devices SET last_seen_at=clock_timestamp() WHERE owner_id=$1 AND device_id=$2 AND status='ACTIVE' AND last_seen_at<clock_timestamp()-interval '5 seconds'`, i.OwnerID, i.DeviceID); e != nil {
		return result, e
	}
	rows, e := s.db.Query(ctx, `SELECT id::text,expires_at,consumed_at IS NOT NULL FROM guest_invites WHERE owner_id=$1 AND device_id=$2 AND expires_at>clock_timestamp() AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 10`, i.OwnerID, i.DeviceID)
	if e != nil {
		return result, e
	}
	for rows.Next() {
		var v GuestInvite
		if e = rows.Scan(&v.ID, &v.Expires, &v.Consumed); e != nil {
			rows.Close()
			return result, e
		}
		result.Invites = append(result.Invites, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, e
	}
	rows, e = s.db.Query(ctx, `SELECT c.id::text,c.invite_id::text,COALESCE(g.display_name,d.device_id),c.device_id,c.status,(v.owner_id=$1 AND v.device_id=$2),v.expires_at FROM guest_pair_candidates c JOIN guest_invites v ON v.id=c.invite_id JOIN devices d ON d.owner_id=c.owner_id AND d.device_id=c.device_id LEFT JOIN guest_identities g ON g.owner_id=c.owner_id WHERE ((v.owner_id=$1 AND v.device_id=$2) OR(c.owner_id=$1 AND c.device_id=$2)) AND v.expires_at>clock_timestamp() AND v.revoked_at IS NULL ORDER BY c.created_at DESC LIMIT 32`, i.OwnerID, i.DeviceID)
	if e != nil {
		return result, e
	}
	for rows.Next() {
		var v GuestCandidate
		if e = rows.Scan(&v.ID, &v.InviteID, &v.Name, &v.Device, &v.Status, &v.Incoming, &v.Expires); e != nil {
			rows.Close()
			return result, e
		}
		result.Candidates = append(result.Candidates, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return result, e
	}
	rows, e = s.db.Query(ctx, `SELECT p.id::text,d.owner_id,d.device_id,COALESCE(g.display_name,d.device_id),d.public_key,d.signing_public_key,p.expires_at,d.last_seen_at>clock_timestamp()-interval '15 seconds',p.invite_id::text FROM guest_pair_grants p JOIN devices d ON (d.owner_id,d.device_id)=CASE WHEN p.owner_a=$1 AND p.device_a=$2 THEN (p.owner_b,p.device_b) ELSE(p.owner_a,p.device_a) END LEFT JOIN guest_identities g ON g.owner_id=d.owner_id WHERE ((p.owner_a=$1 AND p.device_a=$2) OR(p.owner_b=$1 AND p.device_b=$2)) AND p.revoked_at IS NULL AND p.expires_at>clock_timestamp() AND d.status='ACTIVE' ORDER BY p.expires_at DESC LIMIT 32`, i.OwnerID, i.DeviceID)
	if e != nil {
		return result, e
	}
	defer rows.Close()
	for rows.Next() {
		var v GuestPeer
		if e = rows.Scan(&v.ID, &v.Owner, &v.Device, &v.Name, &v.EncryptionKey, &v.SigningKey, &v.Expires, &v.Online, &v.InviteID); e != nil {
			return result, e
		}
		result.Peers = append(result.Peers, v)
	}
	return result, rows.Err()
}
