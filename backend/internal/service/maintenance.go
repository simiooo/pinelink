package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Maintenance struct {
	db       *pgxpool.Pool
	blobs    BlobStore
	interval time.Duration
	logger   *slog.Logger
}

func NewMaintenance(db *pgxpool.Pool, blobs BlobStore, interval time.Duration, logger *slog.Logger) *Maintenance {
	if interval <= 0 {
		interval = time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Maintenance{db: db, blobs: blobs, interval: interval, logger: logger}
}

func (m *Maintenance) Start(ctx context.Context) error {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		if err := m.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			m.logger.Error("maintenance iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (m *Maintenance) Stop(context.Context) error { return nil }

func (m *Maintenance) runOnce(ctx context.Context) error {
	var errs []error
	rows, err := m.db.Query(ctx, `SELECT id FROM file_entries WHERE state='AVAILABLE' AND expires_at IS NOT NULL AND expires_at<=now() LIMIT 50`)
	if err != nil {
		return err
	}
	var entryIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		entryIDs = append(entryIDs, id)
	}
	rows.Close()
	for _, id := range entryIDs {
		if err := m.expireEntry(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}

	rows, err = m.db.Query(ctx, `SELECT id FROM transfer_requests WHERE state IN ('PENDING','ACCEPTED') AND expires_at<=now() LIMIT 50`)
	if err != nil {
		return err
	}
	var transferIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		transferIDs = append(transferIDs, id)
	}
	rows.Close()
	for _, id := range transferIDs {
		if err := m.expireTransfer(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}

	rows, err = m.db.Query(ctx, `
		SELECT u.id, v.id FROM upload_sessions u JOIN file_versions v ON v.id=u.version_id
		WHERE u.state='OPEN' AND v.state='UPLOADING' AND u.last_activity_at<now()-interval '24 hours' LIMIT 50
	`)
	if err != nil {
		return err
	}
	type staleUpload struct{ uploadID, versionID string }
	var stale []staleUpload
	for rows.Next() {
		var item staleUpload
		if err := rows.Scan(&item.uploadID, &item.versionID); err != nil {
			rows.Close()
			return err
		}
		stale = append(stale, item)
	}
	rows.Close()
	for _, item := range stale {
		if err := m.expireUpload(ctx, item.uploadID, item.versionID); err != nil {
			errs = append(errs, err)
		}
	}

	rows, err = m.db.Query(ctx, `
		SELECT v.id FROM file_versions v
		WHERE v.state IN ('UPLOADING','SEALED','CORRUPT','GC_PENDING') AND v.gc_after IS NOT NULL AND v.gc_after<=now()
			AND v.persistent_hold=0
			AND NOT EXISTS(SELECT 1 FROM file_entries e WHERE e.version_id=v.id AND e.state='AVAILABLE')
            AND NOT EXISTS(SELECT 1 FROM object_references r WHERE r.version_id=v.id AND r.storage_generation=v.storage_generation AND r.released_at IS NULL AND(r.expires_at IS NULL OR r.expires_at>clock_timestamp()))
            AND NOT EXISTS(SELECT 1 FROM object_io_operations io WHERE io.version_id=v.id AND io.storage_generation=v.storage_generation AND io.state='OPEN' AND io.expires_at>clock_timestamp())
			AND NOT EXISTS(SELECT 1 FROM object_gc_jobs j WHERE j.version_id=v.id AND j.completed_at IS NULL AND(j.retry_after>clock_timestamp() OR j.claim_until>clock_timestamp()))
		ORDER BY v.gc_after LIMIT 20
	`)
	if err != nil {
		return err
	}
	var versionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		versionIDs = append(versionIDs, id)
	}
	rows.Close()
	for _, id := range versionIDs {
		if err := m.collectVersion(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	if err := m.sweepOrphans(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := m.revokeInactive(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (m *Maintenance) expireEntry(ctx context.Context, id string) error {
	svc := New(m.db, m.blobs)
	tx, err := svc.beginFileObject(ctx, id)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	file, err := scanFileSnapshot(tx.QueryRow(ctx, `SELECT `+fileSnapshotColumns+` FROM file_entries WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	var expired bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE(state='AVAILABLE' AND expires_at<=clock_timestamp(),false) FROM file_entries WHERE id=$1`, id).Scan(&expired); err != nil {
		return err
	}
	if !expired {
		return nil
	}
	file.State = "EXPIRED"
	file.Version++
	file.UpdatedAt = time.Now()
	if _, err = tx.Exec(ctx, `UPDATE file_entries SET state='EXPIRED',version=version+1,updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	if err = releaseFileAccess(ctx, tx, id, "FILE_EXPIRED"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE file_versions SET gc_after=now()+interval '15 minutes' WHERE id=$1`, file.VersionID); err != nil {
		return err
	}
	if err = addEvent(ctx, tx, "FILE_STATE_CHANGED", id, file.Version, file); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *Maintenance) expireTransfer(ctx context.Context, id string) error {
	svc := New(m.db, m.blobs)
	tx, tr, err := svc.beginTransferObjects(ctx, id)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if tr.State != "PENDING" && tr.State != "ACCEPTED" {
		return nil
	}
	var expired bool
	if err = tx.QueryRow(ctx, `SELECT expires_at<=clock_timestamp() FROM transfer_requests WHERE id=$1`, id).Scan(&expired); err != nil {
		return err
	}
	if !expired {
		return nil
	}
	tr.State = "EXPIRED"
	tr.Version++
	tr.RevocationEpoch++
	tr.UpdatedAt = time.Now()
	if _, err = tx.Exec(ctx, `UPDATE transfer_requests SET state='EXPIRED',version=version+1,revocation_epoch=revocation_epoch+1,updated_at=now(),responded_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	if err = releaseTransferResources(ctx, tx, tr); err != nil {
		return err
	}
	if err = addEvent(ctx, tx, "TRANSFER_EXPIRED", id, tr.Version, tr); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *Maintenance) expireUpload(ctx context.Context, uploadID, versionID string) error {
	svc := New(m.db, m.blobs)
	tx, o, err := svc.beginUploadObject(ctx, uploadID, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var expired bool
	if err = tx.QueryRow(ctx, `SELECT state='OPEN' AND last_activity_at<=clock_timestamp()-interval '24 hours' FROM upload_sessions WHERE id=$1`, uploadID).Scan(&expired); err != nil {
		return err
	}
	if !expired || o.VersionID != versionID || o.VersionState != "UPLOADING" {
		return nil
	}
	if _, err = tx.Exec(ctx, `UPDATE upload_sessions SET state='EXPIRED' WHERE id=$1`, uploadID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE file_versions SET gc_after=now() WHERE id=$1`, versionID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE tasks SET state='FAILED',version=version+1,error_code='UPLOAD_EXPIRED',updated_at=now(),completed_at=now() WHERE kind='UPLOAD' AND reference_id=$1 AND state IN ('RUNNING','PAUSED','QUEUED')`, uploadID); err != nil {
		return err
	}
	if err = addEvent(ctx, tx, "UPLOAD_EXPIRED", uploadID, time.Now().UnixNano(), map[string]string{"upload_id": uploadID, "version_id": versionID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *Maintenance) RunOnce(ctx context.Context) error {
	return m.runOnce(ctx)
}

func scanIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (m *Maintenance) String() string {
	return fmt.Sprintf("maintenance every %s", m.interval)
}
