package service

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrUnauthorized = errors.New("device authentication failed")

func (s *Service) VerifyDevice(ctx context.Context, ownerID, deviceID, timestamp, method, uri string, body []byte, signature string, registration bool) error {
	var status string
	var publicKey *string
	err := s.db.QueryRow(ctx, `SELECT status, signing_public_key FROM devices WHERE owner_id=$1 AND device_id=$2`, ownerID, deviceID).Scan(&status, &publicKey)
	if errors.Is(err, pgx.ErrNoRows) {
		if registration {
			var request DeviceRequest
			if json.Unmarshal(body, &request) != nil || request.OwnerID != ownerID || request.DeviceID != deviceID {
				return ErrUnauthorized
			}
			publicKey = &request.SigningPublicKey
			status = "ACTIVE"
			err = nil
		} else {
			return ErrUnauthorized
		}
	}
	if err != nil {
		return err
	}
	if publicKey == nil || *publicKey == "" || (status != "ACTIVE" && !registration) {
		return ErrUnauthorized
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrUnauthorized
	}
	if delta := time.Since(time.Unix(seconds, 0)); delta < -5*time.Minute || delta > 5*time.Minute {
		return ErrUnauthorized
	}
	key, err := base64.StdEncoding.DecodeString(*publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return ErrUnauthorized
	}
	signatureBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return ErrUnauthorized
	}
	digest := sha256.Sum256(body)
	payload := method + "\n" + uri + "\n" + timestamp + "\n" + hex.EncodeToString(digest[:])
	if !ed25519.Verify(ed25519.PublicKey(key), []byte(payload), signatureBytes) {
		return ErrUnauthorized
	}
	return nil
}

func (s *Service) AuthorizeUpload(ctx context.Context, uploadID, ownerID, deviceID string) error {
	result, err := s.db.Exec(ctx, `SELECT 1 FROM upload_sessions WHERE id=$1 AND owner_id=$2 AND device_id=$3`, uploadID, ownerID, deviceID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) AuthorizePull(ctx context.Context, pullID, ownerID, deviceID string) error {
	var entry string
	var tr *string
	err := s.db.QueryRow(ctx, `SELECT p.entry_id,p.transfer_id FROM pull_sessions p JOIN tasks t ON t.reference_id=p.id AND t.kind='DOWNLOAD'
        WHERE p.id=$1 AND t.owner_id=$2 AND t.device_id=$3`, pullID, ownerID, deviceID).Scan(&entry, &tr)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return fileReadAccess(ctx, s.db, entry, ownerID, deviceID, stringValue(tr))
}

func (s *Service) VerifyIdentityFields(ownerID, deviceID string) error {
	if !validIdentity(ownerID) || !validIdentity(deviceID) {
		return fmt.Errorf("%w: missing verified identity", ErrUnauthorized)
	}
	return nil
}
