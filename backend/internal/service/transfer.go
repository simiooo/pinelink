package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const transferColumns = `id, entry_id, delivery_mode, sender_owner_id, sender_device_id,
	recipient_owner_id, recipient_device_id, state, version, failure_reason,
	created_at, updated_at, responded_at, expires_at,
	COALESCE(transfer_requests.file_name, (SELECT file_name FROM file_entries WHERE id=transfer_requests.entry_id)),
	COALESCE(transfer_requests.size_bytes, (SELECT size_bytes FROM file_versions WHERE id=(SELECT version_id FROM file_entries WHERE id=transfer_requests.entry_id))), content_root,
	encryption_suite, encrypted_manifest, sender_encrypted_manifest, received_bytes,
	progress_revision, progress_sampled_at, active_route, route_epoch, revocation_epoch, content_kind`

func (s *Service) CreateTransfer(ctx context.Context, req TransferCreateRequest) (Transfer, error) {
	if strings.HasPrefix(req.SenderOwnerID, "guest-") || strings.HasPrefix(req.RecipientOwnerID, "guest-") {
		return Transfer{}, ErrUnauthorized
	}
	if req.DeliveryMode == "" {
		req.DeliveryMode = "ASYNC"
	}
	if req.DeliveryMode != "ONLINE" && req.DeliveryMode != "ASYNC" && req.DeliveryMode != "BROADCAST" {
		return Transfer{}, fmt.Errorf("%w: invalid delivery mode", ErrInvalid)
	}
	if !validIdentity(req.EntryID) || !validIdentity(req.SenderOwnerID) || !validIdentity(req.SenderDeviceID) ||
		!validIdentity(req.RecipientOwnerID) || !validIdentity(req.RecipientDeviceID) {
		return Transfer{}, fmt.Errorf("%w: transfer identities are required", ErrInvalid)
	}
	tx, err := s.beginFileObject(ctx, req.EntryID)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	var expires *time.Time
	err = tx.QueryRow(ctx, `SELECT state, expires_at FROM file_entries WHERE id=$1 AND owner_id=$2 AND reference_kind<>'DELIVERY'`, req.EntryID, req.SenderOwnerID).Scan(&state, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrNotFound
	}
	if err != nil {
		return Transfer{}, err
	}
	if state != "AVAILABLE" || (expires != nil && !s.now().Before(*expires)) {
		return Transfer{}, ErrExpired
	}
	transfer := Transfer{ID: uuid.NewString(), EntryID: &req.EntryID, DeliveryMode: req.DeliveryMode,
		SenderOwnerID: req.SenderOwnerID, SenderDeviceID: req.SenderDeviceID,
		RecipientOwnerID: req.RecipientOwnerID, RecipientDeviceID: req.RecipientDeviceID,
		State: "PENDING", Version: 1, ExpiresAt: s.now().Add(24 * time.Hour)}
	if req.DeliveryMode == "ONLINE" {
		transfer.ExpiresAt = s.now().Add(10 * time.Minute)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO transfer_requests (id, entry_id, delivery_mode, sender_owner_id, sender_device_id,
			recipient_owner_id, recipient_device_id, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, transfer.ID, req.EntryID, req.DeliveryMode, req.SenderOwnerID, req.SenderDeviceID,
		req.RecipientOwnerID, req.RecipientDeviceID, transfer.ExpiresAt)
	if err != nil {
		return Transfer{}, err
	}
	if err := addEvent(ctx, tx, "TRANSFER_REQUESTED", transfer.ID, 1, transfer); err != nil {
		return Transfer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return s.GetTransfer(ctx, transfer.ID, req.SenderOwnerID, req.SenderDeviceID)
}

func (s *Service) GetTransfer(ctx context.Context, id, ownerID, deviceID string) (Transfer, error) {
	transfer, err := scanTransfer(s.db.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfer_requests WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrNotFound
	}
	if err != nil {
		return Transfer{}, err
	}
	if !participant(transfer, ownerID, deviceID) {
		return Transfer{}, ErrNotFound
	}
	return presentTransfer(transfer, ownerID, deviceID), nil
}

func (s *Service) ListTransfers(ctx context.Context, ownerID, deviceID, role, state string, before *time.Time, limit int) (TransferList, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT ` + transferColumns + ` FROM transfer_requests WHERE ((sender_owner_id=$1 AND sender_device_id=$2) OR (recipient_owner_id=$1 AND recipient_device_id=$2))`
	args := []any{ownerID, deviceID}
	if role == "sender" {
		query = `SELECT ` + transferColumns + ` FROM transfer_requests WHERE sender_owner_id=$1 AND sender_device_id=$2`
	} else if role == "recipient" {
		query = `SELECT ` + transferColumns + ` FROM transfer_requests WHERE recipient_owner_id=$1 AND recipient_device_id=$2`
	} else {
		role = ""
	}
	if state != "" {
		args = append(args, state)
		query += fmt.Sprintf(" AND state=$%d", len(args))
	}
	if role == "" {
		query += " AND (sender_device_id=$2 OR recipient_device_id=$2)"
	}
	if before != nil {
		args = append(args, *before)
		query += fmt.Sprintf(" AND created_at<$%d", len(args))
	}
	args = append(args, limit+1)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return TransferList{}, err
	}
	defer rows.Close()
	var result TransferList
	for rows.Next() {
		transfer, err := scanTransfer(rows)
		if err != nil {
			return TransferList{}, err
		}
		if len(result.Transfers) == limit {
			result.NextBefore = &transfer.CreatedAt
			break
		}
		result.Transfers = append(result.Transfers, presentTransfer(transfer, ownerID, deviceID))
	}
	return result, rows.Err()
}

func (s *Service) RespondTransfer(ctx context.Context, id, action string, req TransferActionRequest) (Transfer, error) {
	tx, transfer, err := s.beginTransferObjects(ctx, id)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	desired, eventType := "", ""
	switch action {
	case "accept", "reject":
		if transfer.RecipientOwnerID != req.OwnerID || transfer.RecipientDeviceID != req.DeviceID {
			return Transfer{}, ErrNotFound
		}
		if action == "accept" {
			desired, eventType = "ACCEPTED", "TRANSFER_ACCEPTED"
		} else {
			desired, eventType = "REJECTED", "TRANSFER_REJECTED"
		}
	case "cancel":
		if transfer.SenderOwnerID != req.OwnerID || transfer.SenderDeviceID != req.DeviceID {
			return Transfer{}, ErrNotFound
		}
		desired, eventType = "CANCELLED", "TRANSFER_CANCELLED"
	default:
		return Transfer{}, fmt.Errorf("%w: unknown transfer action", ErrInvalid)
	}
	if transfer.State == desired {
		if action == "accept" && transfer.ContentKind != nil && req.AcceptContentKind != *transfer.ContentKind {
			return Transfer{}, fmt.Errorf("%w: text consent required", ErrInvalid)
		}
		return presentTransfer(transfer, req.OwnerID, req.DeviceID), tx.Commit(ctx)
	}
	if action == "accept" && transfer.ContentKind != nil && req.AcceptContentKind != *transfer.ContentKind {
		return Transfer{}, fmt.Errorf("%w: text consent required", ErrInvalid)
	}
	if transfer.Version != req.ExpectedVersion {
		return Transfer{}, &StateConflictError{Resource: "transfer", Current: presentTransfer(transfer, req.OwnerID, req.DeviceID)}
	}
	if transfer.State != "PENDING" && !(action == "cancel" && transfer.State == "ACCEPTED") {
		return Transfer{}, &StateConflictError{Resource: "transfer", Current: presentTransfer(transfer, req.OwnerID, req.DeviceID)}
	}
	var withinDeadline bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz>clock_timestamp()`, transfer.ExpiresAt).Scan(&withinDeadline); err != nil {
		return Transfer{}, err
	}
	if !withinDeadline {
		transfer.State = "EXPIRED"
		transfer.Version++
		transfer.RevocationEpoch++
		transfer.UpdatedAt = s.now()
		if _, err := tx.Exec(ctx, `UPDATE transfer_requests SET state='EXPIRED', version=$2, revocation_epoch=revocation_epoch+1, updated_at=now() WHERE id=$1`, id, transfer.Version); err != nil {
			return Transfer{}, err
		}
		if err := releaseTransferResources(ctx, tx, transfer); err != nil {
			return Transfer{}, err
		}
		if err := addEvent(ctx, tx, "TRANSFER_EXPIRED", id, transfer.Version, transfer); err != nil {
			return Transfer{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Transfer{}, err
		}
		return Transfer{}, ErrExpired
	}
	if action == "accept" && transfer.EntryID != nil {
		var state string
		var expires *time.Time
		if err := tx.QueryRow(ctx, `SELECT state, expires_at FROM file_entries WHERE id=$1`, *transfer.EntryID).Scan(&state, &expires); err != nil {
			return Transfer{}, err
		}
		if state != "AVAILABLE" || (expires != nil && !s.now().Before(*expires)) {
			return Transfer{}, &StateConflictError{Resource: "file_entry", Current: map[string]any{"id": *transfer.EntryID, "state": state, "expires_at": expires}}
		}
	}
	transfer.State = desired
	transfer.Version++
	transfer.UpdatedAt = s.now()
	transfer.RespondedAt = &transfer.UpdatedAt
	transfer.FailureReason = nil
	if req.Reason != "" {
		transfer.FailureReason = &req.Reason
	}
	if _, err := tx.Exec(ctx, `
		UPDATE transfer_requests SET state=$2, version=$3, responded_at=now(), updated_at=now(), failure_reason=$4 WHERE id=$1
	`, id, desired, transfer.Version, transfer.FailureReason); err != nil {
		return Transfer{}, err
	}
	if desired == "CANCELLED" || desired == "REJECTED" {
		transfer.RevocationEpoch++
		if _, err := tx.Exec(ctx, `UPDATE transfer_requests SET revocation_epoch=revocation_epoch+1 WHERE id=$1`, id); err != nil {
			return Transfer{}, err
		}
		if err := releaseTransferResources(ctx, tx, transfer); err != nil {
			return Transfer{}, err
		}
		errorCode := "TRANSFER_" + desired
		if _, err := tx.Exec(ctx, `
			UPDATE tasks SET state='CANCELLED', version=version+1, error_code=$2,
				error_message=$3, updated_at=now(), completed_at=now()
			WHERE kind='DOWNLOAD' AND state IN ('RUNNING','PAUSED','QUEUED') AND reference_id IN (
				SELECT id FROM pull_sessions WHERE transfer_id=$1
			)
		`, id, errorCode, req.Reason); err != nil {
			return Transfer{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE pull_sessions SET state='FAILED', version=version+1, error_code=$2, updated_at=now()
			WHERE transfer_id=$1 AND state='PULLING'
		`, id, errorCode); err != nil {
			return Transfer{}, err
		}
	}
	if desired == "ACCEPTED" && transfer.DeliveryMode == "ONLINE" {
		var total int64
		if transfer.SizeBytes != nil {
			total = *transfer.SizeBytes
		}
		if _, err := createTask(ctx, tx, "DOWNLOAD", transfer.ID, transfer.RecipientOwnerID, transfer.RecipientDeviceID, total); err != nil {
			return Transfer{}, err
		}
	}
	if desired == "ACCEPTED" {
		if err := grantDelivery(ctx, tx, transfer); err != nil {
			return Transfer{}, err
		}
	}
	if desired == "CANCELLED" && transfer.DeliveryMode == "ONLINE" {
		if _, err := tx.Exec(ctx, `
			UPDATE tasks SET state='CANCELLED', version=version+1, error_code='TRANSFER_CANCELLED', updated_at=now(), completed_at=now()
			WHERE kind='DOWNLOAD' AND reference_id=$1 AND state IN ('RUNNING','PAUSED','QUEUED')
		`, id); err != nil {
			return Transfer{}, err
		}
	}
	if err := addEvent(ctx, tx, eventType, id, transfer.Version, transfer); err != nil {
		return Transfer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return presentTransfer(transfer, req.OwnerID, req.DeviceID), nil
}

func participant(transfer Transfer, ownerID, deviceID string) bool {
	return (transfer.SenderOwnerID == ownerID && transfer.SenderDeviceID == deviceID) ||
		(transfer.RecipientOwnerID == ownerID && transfer.RecipientDeviceID == deviceID)
}

func scanTransfer(row rowScanner) (Transfer, error) {
	var transfer Transfer
	err := row.Scan(&transfer.ID, &transfer.EntryID, &transfer.DeliveryMode,
		&transfer.SenderOwnerID, &transfer.SenderDeviceID, &transfer.RecipientOwnerID,
		&transfer.RecipientDeviceID, &transfer.State, &transfer.Version, &transfer.FailureReason,
		&transfer.CreatedAt, &transfer.UpdatedAt, &transfer.RespondedAt, &transfer.ExpiresAt,
		&transfer.FileName, &transfer.SizeBytes, &transfer.ContentRoot, &transfer.EncryptionSuite,
		&transfer.EncryptedManifest, &transfer.SenderEncryptedManifest, &transfer.ReceivedBytes,
		&transfer.ProgressRevision, &transfer.ProgressSampledAt, &transfer.ActiveRoute, &transfer.RouteEpoch, &transfer.RevocationEpoch, &transfer.ContentKind)
	return transfer, err
}
