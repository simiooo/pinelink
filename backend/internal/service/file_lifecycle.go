package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/simiooo/pinelink/backend/internal/auth"
	"time"

	"github.com/jackc/pgx/v5"
)

type FileSnapshot struct {
	ReferenceKind  string     `json:"reference_kind"`
	SourceEntryID  *string    `json:"source_entry_id,omitempty"`
	ID             string     `json:"id"`
	VersionID      string     `json:"version_id"`
	OwnerID        string     `json:"owner_id"`
	DeviceID       string     `json:"device_id"`
	FileName       string     `json:"file_name"`
	RetentionClass string     `json:"retention_class"`
	State          string     `json:"state"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	Version        int64      `json:"version"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

const fileSnapshotColumns = `id, version_id, owner_id, device_id, file_name, retention_class, state, expires_at, version, updated_at, reference_kind,source_entry_id`

func (s *Service) GetFile(ctx context.Context, entryID, ownerID string) (FileSnapshot, error) {
	file, err := scanFileSnapshot(s.db.QueryRow(ctx, `SELECT `+fileSnapshotColumns+` FROM file_entries WHERE id=$1 AND owner_id=$2`, entryID, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return FileSnapshot{}, ErrNotFound
	}
	return file, err
}

func (s *Service) GetGrantedFile(ctx context.Context, entryID, ownerID, deviceID, transferID string) (FileSnapshot, error) {
	tx, err := s.beginEntryAccess(ctx, entryID, transferID)
	if err != nil {
		return FileSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	if err = fileReadAccess(ctx, tx, entryID, ownerID, deviceID, transferID); err != nil {
		return FileSnapshot{}, err
	}
	file, err := scanFileSnapshot(tx.QueryRow(ctx, `SELECT `+fileSnapshotColumns+` FROM file_entries WHERE id=$1`, entryID))
	if err != nil {
		return FileSnapshot{}, err
	}
	return file, tx.Commit(ctx)
}

func (s *Service) PromoteFile(ctx context.Context, entryID string, req FileActionRequest) (FileSnapshot, error) {
	tx, err := s.beginEntryAccess(ctx, entryID, req.TransferID)
	if err != nil {
		return FileSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	if err = fileReadAccess(ctx, tx, entryID, req.OwnerID, req.DeviceID, req.TransferID); err != nil {
		return FileSnapshot{}, err
	}
	file, err := scanFileSnapshot(tx.QueryRow(ctx, `SELECT `+fileSnapshotColumns+` FROM file_entries WHERE id=$1 FOR UPDATE`, entryID))
	if err != nil {
		return FileSnapshot{}, err
	}
	if file.ReferenceKind == "DELIVERY" || file.OwnerID != req.OwnerID {
		kept, err := scanFileSnapshot(tx.QueryRow(ctx, `SELECT `+fileSnapshotColumns+` FROM file_entries WHERE source_entry_id=$1 AND owner_id=$2 AND device_id=$3 AND reference_kind='PERSISTENT_KEEP'`, entryID, req.OwnerID, req.DeviceID))
		if err == nil {
			if kept.State != "AVAILABLE" {
				return FileSnapshot{}, ErrConflict
			}
			return kept, tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return FileSnapshot{}, err
		}
		if file.Version != req.ExpectedVersion {
			return FileSnapshot{}, &StateConflictError{Resource: "file_entry", Current: file}
		}
		kept, err = scanFileSnapshot(tx.QueryRow(ctx, `INSERT INTO file_entries(id,version_id,owner_id,device_id,file_name,retention_class,state,reference_kind,source_entry_id)
            VALUES($1,$2,$3,$4,$5,'PERSISTENT','AVAILABLE','PERSISTENT_KEEP',$6) RETURNING `+fileSnapshotColumns, uuid.NewString(), file.VersionID, req.OwnerID, req.DeviceID, file.FileName, entryID))
		if err != nil {
			return FileSnapshot{}, err
		}
		if err = addEvent(ctx, tx, "PERSISTENCE_CHANGED", kept.ID, kept.Version, kept); err != nil {
			return FileSnapshot{}, err
		}
		return kept, tx.Commit(ctx)
	}
	if file.RetentionClass == "PERSISTENT" {
		return file, tx.Commit(ctx)
	}
	if file.Version != req.ExpectedVersion {
		return FileSnapshot{}, &StateConflictError{Resource: "file_entry", Current: file}
	}
	file, err = scanFileSnapshot(tx.QueryRow(ctx, `UPDATE file_entries SET retention_class='PERSISTENT',expires_at=NULL,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+fileSnapshotColumns, entryID))
	if err != nil {
		return FileSnapshot{}, err
	}
	if err = addEvent(ctx, tx, "PERSISTENCE_CHANGED", entryID, file.Version, file); err != nil {
		return FileSnapshot{}, err
	}
	return file, tx.Commit(ctx)
}

func (s *Service) DeleteFile(ctx context.Context, entryID string, req FileActionRequest) (FileSnapshot, error) {
	tx, err := s.beginFileObject(ctx, entryID)
	if err != nil {
		return FileSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	file, err := scanFileSnapshot(tx.QueryRow(ctx, `SELECT `+fileSnapshotColumns+` FROM file_entries WHERE id=$1 AND owner_id=$2 FOR UPDATE`, entryID, req.OwnerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return FileSnapshot{}, ErrNotFound
	}
	if err != nil {
		return FileSnapshot{}, err
	}
	if file.State == "DELETED" {
		return file, tx.Commit(ctx)
	}
	if file.Version != req.ExpectedVersion {
		return FileSnapshot{}, &StateConflictError{Resource: "file_entry", Current: file}
	}
	file.State = "DELETED"
	file.Version++
	file.UpdatedAt = s.now()
	if _, err := tx.Exec(ctx, `UPDATE file_entries SET state='DELETED', version=$2, updated_at=now() WHERE id=$1`, entryID, file.Version); err != nil {
		return FileSnapshot{}, err
	}
	if err := releaseFileAccess(ctx, tx, entryID, "FILE_DELETED"); err != nil {
		return FileSnapshot{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE file_versions SET gc_after=now()+interval '15 minutes' WHERE id=$1 AND persistent_hold=0`, file.VersionID); err != nil {
		return FileSnapshot{}, err
	}
	if err := addEvent(ctx, tx, "FILE_STATE_CHANGED", entryID, file.Version, file); err != nil {
		return FileSnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FileSnapshot{}, err
	}
	return file, nil
}

func (s *Service) GetQuota(ctx context.Context, scopeID string) (Quota, error) {
	if identity, ok := auth.FromContext(ctx); ok && identity.SessionID != "" && scopeID != identity.OwnerID {
		return Quota{}, ErrUnauthorized
	}
	var quota Quota
	err := s.db.QueryRow(ctx, `SELECT scope_id, quota_bytes, used_bytes, version, updated_at FROM scope_quotas WHERE scope_id=$1`, scopeID).
		Scan(&quota.ScopeID, &quota.QuotaBytes, &quota.UsedBytes, &quota.Version, &quota.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quota{ScopeID: scopeID, QuotaBytes: 1 << 40}, nil
	}
	return quota, err
}

func reserveQuota(ctx context.Context, tx pgx.Tx, scopeID string, size int64) error {
	if _, err := tx.Exec(ctx, `INSERT INTO scope_quotas (scope_id, quota_bytes) VALUES ($1,$2) ON CONFLICT (scope_id) DO NOTHING`, scopeID, int64(1<<40)); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE scope_quotas SET used_bytes=used_bytes+$2, version=version+1, updated_at=now()
		WHERE scope_id=$1 AND used_bytes+$2<=quota_bytes
	`, scopeID, size)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("%w: physical storage quota exceeded", ErrConflict)
	}
	return nil
}

func scanFileSnapshot(row rowScanner) (FileSnapshot, error) {
	var file FileSnapshot
	err := row.Scan(&file.ID, &file.VersionID, &file.OwnerID, &file.DeviceID, &file.FileName,
		&file.RetentionClass, &file.State, &file.ExpiresAt, &file.Version, &file.UpdatedAt, &file.ReferenceKind, &file.SourceEntryID)
	return file, err
}
