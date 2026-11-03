package service

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/simiooo/pinelink/backend/internal/accounts"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

func (s *Service) ConfigureAccounts(cfg accounts.Config) error {
	var err error
	s.accounts, err = accounts.New(s.db, cfg)
	return err
}
func (s *Service) Accounts() *accounts.Service {
	if s == nil {
		return nil
	}
	return s.accounts
}
func (s *Service) RunAccountMail(ctx context.Context) error {
	if s.accounts == nil {
		<-ctx.Done()
		return nil
	}
	s.accounts.RunMail(ctx)
	return nil
}

// Account then session locks use the same order as reset/logout. Data I/O
// remains outside database transactions; its later durable commit is fenced.
func (s *Service) beginAccountTx(ctx context.Context, deviceLock ...string) (pgx.Tx, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	identity, ok := auth.FromContext(ctx)
	if !ok || !identity.HasSession() || s.accounts == nil {
		return tx, nil
	}
	if identity.GuestSessionID != "" {
		if !s.accounts.GuestsEnabled() {
			tx.Rollback(ctx)
			return nil, ErrUnauthorized
		}
		var current *string
		err = tx.QueryRow(ctx, `SELECT current_session_id::text FROM guest_identities WHERE owner_id=$1 AND device_id=$2 AND revoked_at IS NULL FOR SHARE`, identity.OwnerID, identity.DeviceID).Scan(&current)
		var valid bool
		if err == nil && current != nil && *current == identity.GuestSessionID {
			err = tx.QueryRow(ctx, `SELECT revoked_at IS NULL AND expires_at>clock_timestamp() FROM guest_sessions WHERE id=$1 AND owner_id=$2 FOR SHARE`, identity.GuestSessionID, identity.OwnerID).Scan(&valid)
		}
		if err != nil || !valid {
			tx.Rollback(ctx)
			return nil, ErrUnauthorized
		}
		if err = s.fenceDevice(ctx, tx, identity, deviceLock); err != nil {
			tx.Rollback(ctx)
			return nil, err
		}
		return tx, nil
	}
	var owner string
	var epoch int64
	err = tx.QueryRow(ctx, `SELECT account_id,credential_epoch FROM account_sessions WHERE id=$1`, identity.SessionID).Scan(&owner, &epoch)
	var current int64
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT credential_epoch FROM accounts WHERE id=$1 FOR SHARE`, owner).Scan(&current)
	}
	var valid bool
	if err == nil && epoch == current {
		err = tx.QueryRow(ctx, `SELECT revoked_at IS NULL AND expires_at>clock_timestamp() FROM account_sessions WHERE id=$1 FOR SHARE`, identity.SessionID).Scan(&valid)
	}
	if err != nil || epoch != current || !valid || (identity.OwnerID != "" && identity.OwnerID != owner) {
		tx.Rollback(ctx)
		return nil, ErrUnauthorized
	}
	if err = s.fenceDevice(ctx, tx, identity, deviceLock); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func (s *Service) fenceDevice(ctx context.Context, tx pgx.Tx, i auth.Identity, lock []string) error {
	if i.OwnerID == "" || i.DeviceID == "" {
		return nil
	}
	mode := "SHARE"
	if len(lock) > 0 && lock[0] == "UPDATE" {
		mode = "UPDATE"
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM devices WHERE owner_id=$1 AND device_id=$2 FOR `+mode, i.OwnerID, i.DeviceID).Scan(&active)
	if err != nil || !active {
		return ErrUnauthorized
	}
	return nil
}
