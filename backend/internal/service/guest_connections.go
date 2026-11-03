package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RemoveGuestConnection revokes only the caller's pair. Local history and
// completed deliveries survive. A live transfer must be ended explicitly.
func (s *Service) RemoveGuestConnection(ctx context.Context, pair string) (map[string]bool, error) {
	i, err := s.guestCaller(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = uuid.Parse(pair); err != nil {
		return nil, ErrInvalid
	}
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var revoked bool
	err = tx.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM guest_pair_grants WHERE id=$1 AND ((owner_a=$2 AND device_a=$3) OR (owner_b=$2 AND device_b=$3)) FOR UPDATE`, pair, i.OwnerID, i.DeviceID).Scan(&revoked)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !revoked {
		// Creation holds SHARE on this same pair until its transfer commits.
		// Query after acquiring UPDATE so a waiting remover sees that commit.
		var active bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transfer_requests WHERE guest_pair_id=$1 AND state IN ('PENDING','ACCEPTED') AND expires_at>clock_timestamp())`, pair).Scan(&active); err != nil {
			return nil, err
		}
		if active {
			return nil, ErrConflict
		}
		if _, err = tx.Exec(ctx, `UPDATE guest_pair_grants SET revoked_at=clock_timestamp() WHERE id=$1`, pair); err != nil {
			return nil, err
		}
	}
	return map[string]bool{"removed": true}, tx.Commit(ctx)
}
