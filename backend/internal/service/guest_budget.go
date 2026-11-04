package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/simiooo/pinelink/backend/internal/auth"
	"strings"
)

func (s *Service) authorizeGuestTransfer(ctx context.Context, tx pgx.Tx, req P2PCreateRequest) error {
	if !strings.HasPrefix(req.SenderOwnerID, "guest-") && !strings.HasPrefix(req.RecipientOwnerID, "guest-") {
		return s.authorizeAccountTransfer(ctx, tx, req)
	}
	i, ok := auth.FromContext(ctx)
	if !ok || !i.HasSession() || s.accounts == nil || !s.accounts.GuestsEnabled() || i.OwnerID != req.SenderOwnerID || i.DeviceID != req.SenderDeviceID {
		return ErrUnauthorized
	}
	if req.GuestPairID == "" {
		return ErrUnauthorized
	}
	if _, e := uuid.Parse(req.GuestPairID); e != nil {
		return ErrInvalid
	}
	var pair string
	e := tx.QueryRow(ctx, `SELECT p.id::text FROM guest_pair_grants p JOIN devices a ON a.owner_id=p.owner_a AND a.device_id=p.device_a JOIN devices b ON b.owner_id=p.owner_b AND b.device_id=p.device_b WHERE p.id=$1 AND p.revoked_at IS NULL AND p.expires_at>clock_timestamp() AND a.status='ACTIVE' AND b.status='ACTIVE' AND(((p.owner_a,p.device_a,p.owner_b,p.device_b)=($2,$3,$4,$5)) OR((p.owner_b,p.device_b,p.owner_a,p.device_a)=($2,$3,$4,$5))) FOR SHARE OF p`, req.GuestPairID, req.SenderOwnerID, req.SenderDeviceID, req.RecipientOwnerID, req.RecipientDeviceID).Scan(&pair)
	if e == pgx.ErrNoRows {
		return ErrUnauthorized
	}
	if e != nil {
		return e
	}
	var valid bool
	fileLimit := s.accounts.GuestMaxFileBytes()
	maxCipher := fileLimit + ((fileLimit+65535)/65536)*32
	if req.SizeBytes > maxCipher || req.EncryptionSuite != "DARE20-AGE-X25519-v1" || len(req.EncryptedManifest) > 65536 || len(req.SenderEncryptedManifest) > 65536 {
		return ErrQuotaExceeded
	}
	// All anonymous reservations use one ordering, including both peers and the service budget.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(915,1)`); e != nil {
		return e
	}
	var period int64
	if e = tx.QueryRow(ctx, `SELECT floor(extract(epoch FROM clock_timestamp())/86400)::bigint`).Scan(&period); e != nil {
		return e
	}
	budgets := []struct {
		key   string
		limit int64
	}{}
	daily, ip, global := s.accounts.GuestResourceLimits()
	budgets = append(budgets, struct {
		key   string
		limit int64
	}{"global", global}, struct {
		key   string
		limit int64
	}{"ip/" + i.ResourceIP, ip})
	cost := max(req.SizeBytes+int64(len(req.EncryptedManifest)+len(req.SenderEncryptedManifest))+1024, 64<<10)
	for _, owner := range []string{req.SenderOwnerID, req.RecipientOwnerID} {
		if !strings.HasPrefix(owner, "guest-") {
			continue
		}
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM guest_identities g JOIN guest_sessions s ON s.id=g.current_session_id WHERE g.owner_id=$1 AND g.revoked_at IS NULL AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp())`, owner).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return ErrUnauthorized
		}
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM transfer_requests WHERE (sender_owner_id=$1 OR recipient_owner_id=$1) AND state IN ('PENDING','ACCEPTED') AND expires_at>clock_timestamp()`, owner).Scan(&count); e != nil {
			return e
		}
		if count >= 2 {
			return ErrQuotaExceeded
		}
		budgets = append(budgets, struct {
			key   string
			limit int64
		}{"owner/" + owner, daily})
	}
	for _, b := range budgets {
		if cost > b.limit {
			return ErrQuotaExceeded
		}
		var n int64
		e = tx.QueryRow(ctx, `INSERT INTO guest_resource_budgets(key_hash,period,bytes_reserved,expires_at) VALUES($1,$2::bigint,$3,to_timestamp(($2::bigint+2)*86400)) ON CONFLICT(key_hash,period) DO UPDATE SET bytes_reserved=guest_resource_budgets.bytes_reserved+EXCLUDED.bytes_reserved WHERE guest_resource_budgets.bytes_reserved+EXCLUDED.bytes_reserved<=$4 RETURNING bytes_reserved`, s.accounts.OpaqueHash("budget", b.key), period, cost, b.limit).Scan(&n)
		if e == pgx.ErrNoRows {
			return ErrQuotaExceeded
		}
		if e != nil {
			return e
		}
	}
	return nil
}
