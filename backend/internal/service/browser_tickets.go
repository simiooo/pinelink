package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"github.com/simiooo/pinelink/backend/internal/auth"
	"time"
)

func (s *Service) MintRelayTicket(ctx context.Context, id, role string, from ...int64) (map[string]any, error) {
	i, ok := auth.FromContext(ctx)
	if !ok || !i.HasSession() || s.accounts == nil {
		return nil, ErrUnauthorized
	}
	if role != "sender" && role != "receiver" {
		return nil, ErrInvalid
	}
	tx, e := s.beginAccountTx(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	tr, e := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfer_requests WHERE id=$1 FOR SHARE`, id))
	if e != nil {
		return nil, ErrNotFound
	}
	if !s.now().Before(tr.ExpiresAt) || (tr.State != "PENDING" && tr.State != "ACCEPTED") {
		return nil, ErrExpired
	}
	if (role == "sender" && (tr.SenderOwnerID != i.OwnerID || tr.SenderDeviceID != i.DeviceID)) || (role == "receiver" && (tr.RecipientOwnerID != i.OwnerID || tr.RecipientDeviceID != i.DeviceID || tr.State != "ACCEPTED")) {
		return nil, ErrNotFound
	}
	var resume *int64
	if len(from) > 0 {
		if len(from) != 1 || role != "receiver" || from[0] < 0 || from[0] > tr.ReceivedBytes || (from[0]%(64<<10) != 0 && (tr.SizeBytes == nil || from[0] != *tr.SizeBytes)) {
			return nil, ErrInvalid
		}
		resume = &from[0]
	}
	var count int
	// Serialize the capacity reservation; simultaneous tabs must not all pass
	// a stale count and create more live tickets than the per-device limit.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,918))`, i.OwnerID+"/"+i.DeviceID); e != nil {
		return nil, e
	}
	e = tx.QueryRow(ctx, `SELECT count(*) FROM relay_browser_tickets WHERE owner_id=$1 AND device_id=$2 AND expires_at>clock_timestamp() AND consumed_at IS NULL`, i.OwnerID, i.DeviceID).Scan(&count)
	if e != nil {
		return nil, e
	}
	if count >= 8 {
		return nil, ErrQuotaExceeded
	}
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return nil, e
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	expires := s.now().Add(30 * time.Second)
	_, e = tx.Exec(ctx, `INSERT INTO relay_browser_tickets(token_hash,transfer_id,owner_id,device_id,role,account_session_id,guest_session_id,expires_at,resume_from) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,$8,$9)`, s.accounts.OpaqueHash("relay-ticket", token), id, i.OwnerID, i.DeviceID, role, i.SessionID, i.GuestSessionID, expires, resume)
	if e != nil {
		return nil, e
	}
	return map[string]any{"ticket": token, "expires_at": expires}, tx.Commit(ctx)
}
func (s *Service) ConsumeRelayTicket(ctx context.Context, token, id, role string) (auth.Identity, error) {
	var i auth.Identity
	if s.accounts == nil || len(token) != 43 {
		return i, ErrUnauthorized
	}
	hash := s.accounts.OpaqueHash("relay-ticket", token)
	e := s.db.QueryRow(ctx, `SELECT owner_id,device_id,COALESCE(account_session_id::text,''),COALESCE(guest_session_id::text,''),resume_from FROM relay_browser_tickets WHERE token_hash=$1 AND transfer_id=$2 AND role=$3 AND consumed_at IS NULL AND expires_at>clock_timestamp()`, hash, id, role).Scan(&i.OwnerID, &i.DeviceID, &i.SessionID, &i.GuestSessionID, &i.RelayFromBytes)
	if e != nil {
		return i, ErrUnauthorized
	}
	tx, e := s.beginAccountTx(auth.WithIdentity(ctx, i))
	if e != nil {
		return i, e
	}
	defer tx.Rollback(ctx)
	result, e := tx.Exec(ctx, `UPDATE relay_browser_tickets SET consumed_at=clock_timestamp() WHERE token_hash=$1 AND transfer_id=$2 AND role=$3 AND consumed_at IS NULL AND expires_at>clock_timestamp() AND EXISTS(SELECT 1 FROM devices WHERE owner_id=$4 AND device_id=$5 AND status='ACTIVE')`, hash, id, role, i.OwnerID, i.DeviceID)
	if e != nil {
		return i, e
	}
	if result.RowsAffected() != 1 {
		return i, ErrUnauthorized
	}
	return i, tx.Commit(ctx)
}
