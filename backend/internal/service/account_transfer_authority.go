package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

// Account transfers may target an active device on the same account, or a
// specific device covered by an unexpired, mutually confirmed connection.
// The device and grant locks serialize creation with revocation.
func (s *Service) authorizeAccountTransfer(ctx context.Context, tx pgx.Tx, req P2PCreateRequest) error {
	i, ok := auth.FromContext(ctx)
	if !ok || i.SessionID == "" {
		// Retain the unsigned local-development protocol; managed identities
		// must never use this path without an account session.
		if s.accounts.Required() || strings.HasPrefix(req.SenderOwnerID, "acct-") || strings.HasPrefix(req.RecipientOwnerID, "acct-") || (ok && i.HasSession()) {
			return ErrUnauthorized
		}
		return nil
	}
	if i.OwnerID != req.SenderOwnerID || i.DeviceID != req.SenderDeviceID {
		return ErrUnauthorized
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM devices WHERE owner_id=$1 AND device_id=$2 FOR SHARE`, req.RecipientOwnerID, req.RecipientDeviceID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !active {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if req.SenderOwnerID == req.RecipientOwnerID && req.GuestPairID == "" {
		return nil
	}
	if _, err = uuid.Parse(req.GuestPairID); err != nil {
		return ErrUnauthorized
	}
	var pair string
	err = tx.QueryRow(ctx, `SELECT id::text FROM guest_pair_grants WHERE id=$1 AND revoked_at IS NULL AND expires_at>clock_timestamp() AND (((owner_a,device_a,owner_b,device_b)=($2,$3,$4,$5)) OR ((owner_b,device_b,owner_a,device_a)=($2,$3,$4,$5))) FOR SHARE`, req.GuestPairID, req.SenderOwnerID, req.SenderDeviceID, req.RecipientOwnerID, req.RecipientDeviceID).Scan(&pair)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthorized
	}
	return err
}
