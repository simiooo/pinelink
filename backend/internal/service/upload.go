package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

func (s *Service) PrepareUpload(ctx context.Context, req PrepareRequest) (PrepareResponse, error) {
	if identity, ok := auth.FromContext(ctx); ok && identity.SessionID != "" && req.ScopeID != identity.OwnerID {
		return PrepareResponse{}, ErrUnauthorized
	}
	if strings.HasPrefix(req.ScopeID, "p2p-") || strings.HasPrefix(req.ScopeID, "relay-owner:") {
		return PrepareResponse{}, ErrInvalid
	}
	if !validIdentity(req.ScopeID) || !validIdentity(req.EncryptionDomainID) ||
		!validIdentity(req.OwnerID) || !validIdentity(req.DeviceID) || !validIdentity(req.FileName) {
		return PrepareResponse{}, fmt.Errorf("%w: identity and file name are required", ErrInvalid)
	}
	if req.RetentionClass == "" {
		req.RetentionClass = "TEMP_24H"
	}
	if req.RetentionClass != "TEMP_24H" && req.RetentionClass != "PERSISTENT" {
		return PrepareResponse{}, fmt.Errorf("%w: unsupported retention class", ErrInvalid)
	}
	if req.HashVersion != "sha256-v1" || !validSHA256(req.DedupKey) || !validSHA256(req.ContentRoot) {
		return PrepareResponse{}, fmt.Errorf("%w: unsupported or malformed hashes", ErrInvalid)
	}
	if req.SizeBytes < 0 || req.SizeBytes > 1<<40 {
		return PrepareResponse{}, fmt.Errorf("%w: invalid file size", ErrInvalid)
	}
	if req.ChunkSize == 0 {
		req.ChunkSize = 4 << 20
	}
	if req.ChunkSize < 64<<10 || req.ChunkSize > 16<<20 {
		return PrepareResponse{}, fmt.Errorf("%w: chunk size outside 64 KiB..16 MiB", ErrInvalid)
	}
	count := chunkCount(req.SizeBytes, req.ChunkSize)

	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return PrepareResponse{}, err
	}
	defer tx.Rollback(ctx)

	versionID := uuid.NewString()
	var returnedID string
	err = tx.QueryRow(ctx, `
		INSERT INTO file_versions (
			id, scope_id, encryption_domain_id, hash_version, dedup_key, content_root,
			size_bytes, chunk_size, chunk_count, state, encryption_suite
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'UPLOADING',$10)
		ON CONFLICT (scope_id, encryption_domain_id, hash_version, dedup_key) WHERE state<>'COLLECTED' DO NOTHING
		RETURNING id
	`, versionID, req.ScopeID, req.EncryptionDomainID, req.HashVersion, req.DedupKey,
		req.ContentRoot, req.SizeBytes, req.ChunkSize, count, req.EncryptionSuite).Scan(&returnedID)
	if err == nil {
		if err := reserveQuota(ctx, tx, req.ScopeID, req.SizeBytes); err != nil {
			return PrepareResponse{}, err
		}
		uploadID := uuid.NewString()
		if _, err = tx.Exec(ctx, `
			INSERT INTO upload_sessions (id, version_id, owner_id, device_id, file_name, retention_class,storage_generation)
			VALUES ($1,$2,$3,$4,$5,$6,(SELECT storage_generation FROM file_versions WHERE id=$2))
		`, uploadID, returnedID, req.OwnerID, req.DeviceID, req.FileName, req.RetentionClass); err != nil {
			return PrepareResponse{}, fmt.Errorf("create upload session: %w", err)
		}
		if len(req.EncryptedManifest) > 0 {
			if _, err = tx.Exec(ctx, `
				INSERT INTO manifest_envelopes (version_id, owner_id, device_id, ciphertext)
				VALUES ($1,$2,$3,$4)
			`, returnedID, req.OwnerID, req.DeviceID, req.EncryptedManifest); err != nil {
				return PrepareResponse{}, fmt.Errorf("store encrypted manifest: %w", err)
			}
		}
		taskID, err := createTask(ctx, tx, "UPLOAD", uploadID, req.OwnerID, req.DeviceID, req.SizeBytes)
		if err != nil {
			return PrepareResponse{}, fmt.Errorf("create upload task: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return PrepareResponse{}, err
		}
		return PrepareResponse{
			Status:   "UPLOAD_REQUIRED",
			UploadID: uploadID,
			TaskID:   taskID,
			Manifest: Manifest{VersionID: returnedID, ContentRoot: req.ContentRoot, SizeBytes: req.SizeBytes, ChunkSize: req.ChunkSize, ChunkCount: count, EncryptionSuite: req.EncryptionSuite, EncryptedManifest: req.EncryptedManifest},
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return PrepareResponse{}, fmt.Errorf("prepare file version: %w", err)
	}

	requestedSize := req.SizeBytes
	var state, suite string
	var envelope []byte
	err = tx.QueryRow(ctx, `
		SELECT id, content_root, size_bytes, chunk_size, chunk_count, state, encryption_suite
		FROM file_versions
		WHERE scope_id=$1 AND encryption_domain_id=$2 AND hash_version=$3 AND dedup_key=$4 AND state<>'COLLECTED'
		FOR UPDATE
	`, req.ScopeID, req.EncryptionDomainID, req.HashVersion, req.DedupKey).Scan(
		&versionID, &req.ContentRoot, &req.SizeBytes, &req.ChunkSize, &count, &state, &suite,
	)
	if err != nil {
		return PrepareResponse{}, fmt.Errorf("load file version: %w", err)
	}
	if requestedSize != req.SizeBytes {
		return PrepareResponse{}, fmt.Errorf("%w: dedup key maps to different content size", ErrConflict)
	}
	_ = tx.QueryRow(ctx, `SELECT ciphertext FROM manifest_envelopes WHERE version_id=$1 AND owner_id=$2 AND device_id=$3`, versionID, req.OwnerID, req.DeviceID).Scan(&envelope)
	manifest := Manifest{VersionID: versionID, ContentRoot: req.ContentRoot, SizeBytes: req.SizeBytes, ChunkSize: req.ChunkSize, ChunkCount: count, EncryptionSuite: suite, EncryptedManifest: envelope}

	if state == "UPLOADING" {
		var uploadID, taskID string
		if err = tx.QueryRow(ctx, `
			SELECT u.id, t.id FROM upload_sessions u
			JOIN tasks t ON t.kind='UPLOAD' AND t.reference_id=u.id
			WHERE u.version_id=$1 AND u.state='OPEN' AND u.owner_id=$2 AND u.device_id=$3
		`, versionID, req.OwnerID, req.DeviceID).Scan(&uploadID, &taskID); err != nil {
			return PrepareResponse{}, fmt.Errorf("load open upload: %w", err)
		}
		rows, err := tx.Query(ctx, `SELECT chunk_index FROM upload_chunks WHERE upload_id=$1 ORDER BY chunk_index`, uploadID)
		if err != nil {
			return PrepareResponse{}, err
		}
		var received []int
		for rows.Next() {
			var index int
			if err := rows.Scan(&index); err != nil {
				rows.Close()
				return PrepareResponse{}, err
			}
			received = append(received, index)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return PrepareResponse{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return PrepareResponse{}, err
		}
		return PrepareResponse{Status: "UPLOAD_IN_PROGRESS", UploadID: uploadID, TaskID: taskID, ReceivedChunks: received, Manifest: manifest}, nil
	}
	if state != "SEALED" {
		return PrepareResponse{}, fmt.Errorf("%w: file version cannot be reused in state %s", ErrConflict, state)
	}
	if suite != "" && len(envelope) == 0 {
		if err = tx.Commit(ctx); err != nil {
			return PrepareResponse{}, err
		}
		return PrepareResponse{Status: "ENVELOPE_REQUIRED", Manifest: manifest}, nil
	}

	token := uuid.NewString()
	if _, err = tx.Exec(ctx, `
		INSERT INTO reuse_tokens (token, version_id, owner_id, device_id, file_name, retention_class, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, token, versionID, req.OwnerID, req.DeviceID, req.FileName, req.RetentionClass, s.now().Add(10*time.Minute)); err != nil {
		return PrepareResponse{}, fmt.Errorf("create reuse token: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return PrepareResponse{}, err
	}
	return PrepareResponse{Status: "REUSE_AVAILABLE", ReuseToken: token, Manifest: manifest}, nil
}

func (s *Service) UploadChunk(ctx context.Context, uploadID string, index int, suppliedHash string, data []byte) error {
	return s.putUploadChunk(ctx, uploadID, index, suppliedHash, data)
}

func (s *Service) CompleteUpload(ctx context.Context, uploadID string) (CompleteResponse, error) {
	result, err := s.sealUpload(ctx, uploadID)
	if errors.Is(err, errUploadSealed) {
		result, err = s.sealUpload(ctx, uploadID)
	}
	if errors.Is(err, ErrChecksum) {
		s.failCorruptUpload(ctx, uploadID)
	}
	return result, err
}

func (s *Service) CommitReuse(ctx context.Context, token string) (CompleteResponse, error) {
	tx, err := s.beginAccountTx(ctx)
	if err != nil {
		return CompleteResponse{}, err
	}
	defer tx.Rollback(ctx)
	var parent string
	if err := tx.QueryRow(ctx, `SELECT version_id FROM reuse_tokens WHERE token=$1`, token).Scan(&parent); errors.Is(err, pgx.ErrNoRows) {
		return CompleteResponse{}, ErrNotFound
	} else if err != nil {
		return CompleteResponse{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM file_versions WHERE id=$1 FOR UPDATE`, parent); err != nil {
		return CompleteResponse{}, err
	}
	entry, err := entryByReuseToken(ctx, tx, token)
	if err == nil {
		if entry.State != "AVAILABLE" || (entry.ExpiresAt != nil && !s.now().Before(*entry.ExpiresAt)) {
			return CompleteResponse{}, ErrExpired
		}
		if err := tx.Commit(ctx); err != nil {
			return CompleteResponse{}, err
		}
		return CompleteResponse{File: entry, Reused: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CompleteResponse{}, err
	}

	var versionID, ownerID, deviceID, fileName, retention string
	var expires time.Time
	var consumed *time.Time
	var state string
	err = tx.QueryRow(ctx, `
		SELECT r.version_id, r.owner_id, r.device_id, r.file_name, r.retention_class,
		       r.expires_at, r.consumed_at, v.state
		FROM reuse_tokens r JOIN file_versions v ON v.id=r.version_id
		WHERE r.token=$1 FOR UPDATE OF r
	`, token).Scan(&versionID, &ownerID, &deviceID, &fileName, &retention, &expires, &consumed, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return CompleteResponse{}, ErrNotFound
	}
	if err != nil {
		return CompleteResponse{}, err
	}
	if consumed != nil {
		return CompleteResponse{}, ErrReuseVerified
	}
	if s.now().After(expires) {
		return CompleteResponse{}, ErrExpired
	}
	if state != "SEALED" {
		return CompleteResponse{}, ErrConflict
	}
	entry, err = insertEntry(ctx, tx, versionID, nil, &token, ownerID, deviceID, fileName, retention, s.now())
	if err != nil {
		return CompleteResponse{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE reuse_tokens SET consumed_at=now() WHERE token=$1`, token); err != nil {
		return CompleteResponse{}, err
	}
	if err := addEvent(ctx, tx, "UPLOAD_REUSED", entry.ID, 1, entry); err != nil {
		return CompleteResponse{}, err
	}
	if err := addEvent(ctx, tx, retentionEvent(retention), entry.ID, 1, entry); err != nil {
		return CompleteResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CompleteResponse{}, err
	}
	return CompleteResponse{File: entry, Reused: true}, nil
}

func insertEntry(ctx context.Context, tx pgx.Tx, versionID string, uploadID, reuseToken *string, ownerID, deviceID, fileName, retention string, now time.Time) (FileEntry, error) {
	entry := FileEntry{
		ID: uuid.NewString(), VersionID: versionID, OwnerID: ownerID, DeviceID: deviceID,
		FileName: fileName, RetentionClass: retention, State: "AVAILABLE", ExpiresAt: expiryFor(now, retention),
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO file_entries (id, version_id, upload_id, reuse_token, owner_id, device_id, file_name, retention_class, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING created_at
	`, entry.ID, versionID, uploadID, reuseToken, ownerID, deviceID, fileName, retention, entry.ExpiresAt).Scan(new(time.Time))
	if err != nil {
		return FileEntry{}, fmt.Errorf("create file entry: %w", err)
	}
	return entry, nil
}

func entryByUpload(ctx context.Context, tx pgx.Tx, uploadID string) (FileEntry, error) {
	return scanEntry(tx.QueryRow(ctx, `
		SELECT id, version_id, owner_id, device_id, file_name, retention_class, state, expires_at
		FROM file_entries WHERE upload_id=$1
	`, uploadID))
}

func entryByReuseToken(ctx context.Context, tx pgx.Tx, token string) (FileEntry, error) {
	return scanEntry(tx.QueryRow(ctx, `
		SELECT id, version_id, owner_id, device_id, file_name, retention_class, state, expires_at
		FROM file_entries WHERE reuse_token=$1
	`, token))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(row rowScanner) (FileEntry, error) {
	var entry FileEntry
	err := row.Scan(&entry.ID, &entry.VersionID, &entry.OwnerID, &entry.DeviceID, &entry.FileName, &entry.RetentionClass, &entry.State, &entry.ExpiresAt)
	return entry, err
}
