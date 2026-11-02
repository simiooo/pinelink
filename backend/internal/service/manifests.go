package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type ManifestEnvelope struct {
	OwnerID           string `json:"owner_id"`
	DeviceID          string `json:"device_id"`
	EncryptionSuite   string `json:"encryption_suite"`
	EncryptedManifest []byte `json:"encrypted_manifest"`
}

type PutEnvelopeRequest struct {
	OwnerID           string `json:"owner_id"`
	DeviceID          string `json:"device_id"`
	TargetOwnerID     string `json:"target_owner_id"`
	TargetDeviceID    string `json:"target_device_id"`
	EncryptedManifest []byte `json:"encrypted_manifest"`
}

func (s *Service) GetManifest(ctx context.Context, entryID, ownerID, deviceID, transferID string) (ManifestEnvelope, error) {
	tx, err := s.beginEntryAccess(ctx, entryID, transferID)
	if err != nil {
		return ManifestEnvelope{}, err
	}
	defer tx.Rollback(ctx)
	if err = fileReadAccess(ctx, tx, entryID, ownerID, deviceID, transferID); err != nil {
		return ManifestEnvelope{}, err
	}
	var versionID, suite string
	if err = tx.QueryRow(ctx, `SELECT e.version_id,v.encryption_suite FROM file_entries e JOIN file_versions v ON v.id=e.version_id WHERE e.id=$1`, entryID).Scan(&versionID, &suite); err != nil {
		return ManifestEnvelope{}, err
	}
	var envelope ManifestEnvelope
	envelope.OwnerID, envelope.DeviceID, envelope.EncryptionSuite = ownerID, deviceID, suite
	err = tx.QueryRow(ctx, `SELECT ciphertext FROM manifest_envelopes WHERE version_id=$1 AND owner_id=$2 AND device_id=$3`, versionID, ownerID, deviceID).Scan(&envelope.EncryptedManifest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManifestEnvelope{}, ErrNotFound
	}
	if err != nil {
		return ManifestEnvelope{}, err
	}
	return envelope, tx.Commit(ctx)
}

func (s *Service) PutManifestEnvelope(ctx context.Context, entryID string, req PutEnvelopeRequest) error {
	if len(req.EncryptedManifest) == 0 || len(req.EncryptedManifest) > 1<<20 {
		return fmt.Errorf("%w: invalid encrypted manifest size", ErrInvalid)
	}
	tx, err := s.beginFileObject(ctx, entryID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = fileReadAccess(ctx, tx, entryID, req.OwnerID, req.DeviceID, ""); err != nil {
		return err
	}
	var versionID string
	var suite string
	err = tx.QueryRow(ctx, `
		SELECT e.version_id, v.encryption_suite FROM file_entries e JOIN file_versions v ON v.id=e.version_id
		WHERE e.id=$1 AND e.owner_id=$2 AND e.state='AVAILABLE'
	`, entryID, req.OwnerID).Scan(&versionID, &suite)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if suite == "" {
		return fmt.Errorf("%w: plaintext entries cannot receive an encrypted manifest", ErrInvalid)
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM devices WHERE owner_id=$1 AND device_id=$2`, req.TargetOwnerID, req.TargetDeviceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "ACTIVE" {
		return fmt.Errorf("%w: target device is not active", ErrConflict)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO manifest_envelopes (version_id, owner_id, device_id, ciphertext, updated_at)
		VALUES ($1,$2,$3,$4,now())
		ON CONFLICT (version_id, owner_id, device_id) DO UPDATE SET ciphertext=EXCLUDED.ciphertext, updated_at=now()
	`, versionID, req.TargetOwnerID, req.TargetDeviceID, req.EncryptedManifest)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
