package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Service) RegisterDevice(ctx context.Context, req DeviceRequest) (Device, error) {
	if !validIdentity(req.OwnerID) || !validIdentity(req.DeviceID) {
		return Device{}, fmt.Errorf("%w: owner and device are required", ErrInvalid)
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO devices (owner_id, device_id, public_key, signing_public_key, last_seen_at)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),now())
		ON CONFLICT (owner_id, device_id) DO UPDATE SET
			public_key=COALESCE(NULLIF(EXCLUDED.public_key,''),devices.public_key),
			signing_public_key=COALESCE(NULLIF(EXCLUDED.signing_public_key,''),devices.signing_public_key),
			status='ACTIVE', last_seen_at=now(), revoked_at=NULL
	`, req.OwnerID, req.DeviceID, req.PublicKey, req.SigningPublicKey)
	if err != nil {
		return Device{}, err
	}
	return s.GetDevice(ctx, req.OwnerID, req.DeviceID)
}

func (s *Service) GetDevice(ctx context.Context, ownerID, deviceID string) (Device, error) {
	var device Device
	err := s.db.QueryRow(ctx, `SELECT owner_id, device_id, public_key, status, created_at, last_seen_at, revoked_at FROM devices WHERE owner_id=$1 AND device_id=$2`, ownerID, deviceID).
		Scan(&device.OwnerID, &device.DeviceID, &device.PublicKey, &device.Status, &device.CreatedAt, &device.LastSeenAt, &device.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return device, err
}

func (s *Service) ListDevices(ctx context.Context, ownerID string) (DeviceList, error) {
	rows, err := s.db.Query(ctx, `SELECT owner_id, device_id, public_key, status, created_at, last_seen_at, revoked_at FROM devices WHERE owner_id=$1 ORDER BY created_at`, ownerID)
	if err != nil {
		return DeviceList{}, err
	}
	defer rows.Close()
	var result DeviceList
	for rows.Next() {
		var device Device
		if err := rows.Scan(&device.OwnerID, &device.DeviceID, &device.PublicKey, &device.Status, &device.CreatedAt, &device.LastSeenAt, &device.RevokedAt); err != nil {
			return DeviceList{}, err
		}
		result.Devices = append(result.Devices, device)
	}
	return result, rows.Err()
}

type RecipientPublicKey struct {
	OwnerID   string `json:"owner_id"`
	DeviceID  string `json:"device_id"`
	PublicKey string `json:"public_key"`
}

// Explicit target identity is separate from the authenticated caller. Discovery
// returns only the active device's encryption public key, not its private roster.
func (s *Service) GetRecipientPublicKey(ctx context.Context, ownerID, deviceID string) (RecipientPublicKey, error) {
	if !validIdentity(ownerID) || !validIdentity(deviceID) {
		return RecipientPublicKey{}, ErrInvalid
	}
	result := RecipientPublicKey{OwnerID: ownerID, DeviceID: deviceID}
	err := s.db.QueryRow(ctx, `SELECT public_key FROM devices WHERE owner_id=$1 AND device_id=$2 AND status='ACTIVE' AND public_key IS NOT NULL AND public_key<>''`, ownerID, deviceID).Scan(&result.PublicKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecipientPublicKey{}, ErrNotFound
	}
	return result, err
}

func (s *Service) RevokeDevice(ctx context.Context, req DeviceRequest) (Device, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE devices SET status='REVOKED', revoked_at=now() WHERE owner_id=$1 AND device_id=$2 AND status='ACTIVE'`, req.OwnerID, req.DeviceID)
	if err != nil {
		return Device{}, err
	}
	if result.RowsAffected() == 0 {
		return Device{}, ErrNotFound
	}
	if _, err = tx.Exec(ctx, `UPDATE tasks SET state='CANCELLED',version=version+1,error_code='DEVICE_REVOKED',updated_at=now(),completed_at=now()
		WHERE owner_id=$1 AND device_id=$2 AND state IN ('RUNNING','PAUSED','QUEUED')`, req.OwnerID, req.DeviceID); err != nil {
		return Device{}, err
	}
	if err := addEvent(ctx, tx, "DEVICE_REVOKED", req.OwnerID+"/"+req.DeviceID, time.Now().UnixNano(), req); err != nil {
		return Device{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Device{}, err
	}
	// The device row denies new I/O immediately. Cleanup retries in maintenance
	// if a process exits between device revocation and per-object transactions.
	if err := NewMaintenance(s.db, s.blobs, time.Second, nil).revokeInactive(ctx); err != nil {
		return Device{}, err
	}
	return s.GetDevice(ctx, req.OwnerID, req.DeviceID)
}

func (s *Service) CreateBroadcast(ctx context.Context, req BroadcastRequest) (TransferList, error) {
	if !validIdentity(req.EntryID) || !validIdentity(req.SenderOwnerID) || !validIdentity(req.SenderDeviceID) || !validIdentity(req.RecipientOwnerID) {
		return TransferList{}, fmt.Errorf("%w: broadcast identities are required", ErrInvalid)
	}
	devices, err := s.ListDevices(ctx, req.RecipientOwnerID)
	if err != nil {
		return TransferList{}, err
	}
	tx, err := s.beginFileObject(ctx, req.EntryID)
	if err != nil {
		return TransferList{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM file_entries WHERE id=$1 AND owner_id=$2 AND reference_kind<>'DELIVERY' AND(expires_at IS NULL OR expires_at>clock_timestamp())`, req.EntryID, req.SenderOwnerID).Scan(&state); err != nil {
		return TransferList{}, err
	}
	if state != "AVAILABLE" {
		return TransferList{}, ErrConflict
	}
	var result TransferList
	for _, device := range devices.Devices {
		if device.Status != "ACTIVE" {
			continue
		}
		transfer := Transfer{ID: uuid.NewString(), EntryID: &req.EntryID, DeliveryMode: "BROADCAST",
			SenderOwnerID: req.SenderOwnerID, SenderDeviceID: req.SenderDeviceID,
			RecipientOwnerID: req.RecipientOwnerID, RecipientDeviceID: device.DeviceID,
			State: "PENDING", Version: 1, ExpiresAt: s.now().Add(24 * time.Hour)}
		if _, err := tx.Exec(ctx, `
			INSERT INTO transfer_requests (id, entry_id, delivery_mode, sender_owner_id, sender_device_id,
				recipient_owner_id, recipient_device_id, expires_at)
			VALUES ($1,$2,'BROADCAST',$3,$4,$5,$6,$7)
		`, transfer.ID, req.EntryID, req.SenderOwnerID, req.SenderDeviceID, req.RecipientOwnerID, device.DeviceID, transfer.ExpiresAt); err != nil {
			return TransferList{}, err
		}
		if err := addEvent(ctx, tx, "TRANSFER_REQUESTED", transfer.ID, 1, transfer); err != nil {
			return TransferList{}, err
		}
		result.Transfers = append(result.Transfers, transfer)
	}
	if len(result.Transfers) == 0 {
		return TransferList{}, fmt.Errorf("%w: no active recipient devices", ErrInvalid)
	}
	if err := tx.Commit(ctx); err != nil {
		return TransferList{}, err
	}
	return result, nil
}
