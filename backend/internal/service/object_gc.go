package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The manifest and generation are immutable once GC starts. Retried and even
// superseded workers can only delete this generation's unique write keys.
func (m *Maintenance) collectVersion(ctx context.Context, id string) error {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var gen, state, scope string
	var size int64
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT storage_generation,state,COALESCE(quota_scope_id,scope_id),size_bytes,
        COALESCE(persistent_hold=0 AND gc_after<=clock_timestamp(),false) FROM file_versions WHERE id=$1 FOR UPDATE`, id).Scan(&gen, &state, &scope, &size, &eligible)
	if err != nil {
		return err
	}
	if state == "COLLECTED" || !eligible {
		return nil
	}
	var blocked bool
	err = tx.QueryRow(ctx, `SELECT
        EXISTS(SELECT 1 FROM object_references WHERE version_id=$1 AND storage_generation=$2 AND released_at IS NULL AND(expires_at IS NULL OR expires_at>clock_timestamp())) OR
        EXISTS(SELECT 1 FROM file_entries WHERE version_id=$1 AND state='AVAILABLE') OR
        EXISTS(SELECT 1 FROM object_io_operations WHERE version_id=$1 AND storage_generation=$2 AND state='OPEN' AND expires_at>clock_timestamp())`, id, gen).Scan(&blocked)
	if err != nil {
		return err
	}
	if blocked {
		return nil
	}
	if _, err = tx.Exec(ctx, `UPDATE file_versions SET state='GC_PENDING' WHERE id=$1`, id); err != nil {
		return err
	}
	tokenCandidate := uuid.NewString()
	result, err := tx.Exec(ctx, `INSERT INTO object_gc_jobs(token,version_id,storage_generation,scope_id,reserved_bytes)
        VALUES($1,$2,$3,$4,$5) ON CONFLICT(version_id,storage_generation) DO NOTHING`, tokenCandidate, id, gen, scope, size)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		if _, err = tx.Exec(ctx, `INSERT INTO object_gc_objects(token,object_key)
            SELECT $3::uuid,object_key FROM upload_chunks WHERE storage_generation=$2 AND upload_id IN(SELECT id FROM upload_sessions WHERE version_id=$1)
            UNION SELECT $3::uuid,object_key FROM object_io_operations WHERE version_id=$1 AND storage_generation=$2 AND kind='WRITE' AND object_key IS NOT NULL`, id, gen, tokenCandidate); err != nil {
			return err
		}
	}
	claim := uuid.NewString()
	var token string
	err = tx.QueryRow(ctx, `UPDATE object_gc_jobs SET claim_token=$3,claim_until=clock_timestamp()+interval '90 seconds',attempts=attempts+1
        WHERE version_id=$1 AND storage_generation=$2 AND completed_at IS NULL AND retry_after<=clock_timestamp()
        AND(claim_until IS NULL OR claim_until<=clock_timestamp()) RETURNING token`, id, gen, claim).Scan(&token)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	failed := true
	defer func() {
		if failed {
			release, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_, _ = m.db.Exec(release, `UPDATE object_gc_jobs SET claim_token=NULL,claim_until=NULL,retry_after=clock_timestamp()+interval '5 seconds' WHERE token=$1 AND claim_token=$2 AND completed_at IS NULL`, token, claim)
		}
	}()
	after := ""
	processed := 0
	budget := time.Now().Add(2 * time.Second)
	for {
		rows, err := m.db.Query(ctx, `SELECT object_key FROM object_gc_objects WHERE token=$1 AND object_key>$2 AND deleted_at IS NULL ORDER BY object_key LIMIT 128`, token, after)
		if err != nil {
			return err
		}
		keys, err := scanIDs(rows)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			break
		}
		for _, key := range keys {
			if processed >= 128 || (processed > 0 && time.Now().After(budget)) {
				_, err = m.db.Exec(ctx, `UPDATE object_gc_jobs SET claim_token=NULL,claim_until=NULL,retry_after=clock_timestamp() WHERE token=$1 AND claim_token=$2 AND completed_at IS NULL`, token, claim)
				if err != nil {
					return err
				}
				failed = false
				return nil
			}
			result, err := m.db.Exec(ctx, `UPDATE object_gc_jobs SET claim_until=clock_timestamp()+interval '90 seconds' WHERE token=$1 AND claim_token=$2 AND completed_at IS NULL`, token, claim)
			if err != nil {
				return err
			}
			if result.RowsAffected() != 1 {
				return ErrConflict
			}
			erase, cancel := context.WithTimeout(ctx, 30*time.Second)
			err = m.blobs.DeleteChunk(erase, key)
			cancel()
			if err != nil {
				return err
			}
			result, err = m.db.Exec(ctx, `UPDATE object_gc_objects SET deleted_at=now() WHERE token=$1 AND object_key=$2 AND deleted_at IS NULL
				AND EXISTS(SELECT 1 FROM object_gc_jobs WHERE token=$1 AND claim_token=$3 AND completed_at IS NULL)`, token, key, claim)
			if err != nil {
				return err
			}
			if result.RowsAffected() != 1 {
				return ErrConflict
			}
			processed++
		}
		after = keys[len(keys)-1]
	}
	final, err := m.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer final.Rollback(ctx)
	if _, err = final.Exec(ctx, `SELECT id FROM file_versions WHERE id=$1 FOR UPDATE`, id); err != nil {
		return err
	}
	var valid bool
	err = final.QueryRow(ctx, `SELECT completed_at IS NULL AND claim_token=$2::uuid AND storage_generation=$3::uuid FROM object_gc_jobs WHERE token=$1 FOR UPDATE`, token, claim, gen).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrConflict
	}
	var version int64
	err = final.QueryRow(ctx, `UPDATE file_versions SET state='COLLECTED',version=version+1 WHERE id=$1 AND storage_generation=$2 AND state='GC_PENDING'
		AND NOT EXISTS(SELECT 1 FROM object_gc_objects WHERE token=$3 AND deleted_at IS NULL) RETURNING version`, id, gen, token).Scan(&version)
	if err != nil {
		return err
	}
	if _, err = final.Exec(ctx, `DELETE FROM upload_chunks WHERE storage_generation=$2 AND upload_id IN(SELECT id FROM upload_sessions WHERE version_id=$1)`, id, gen); err != nil {
		return err
	}
	if _, err = final.Exec(ctx, `UPDATE upload_sessions SET state='EXPIRED' WHERE version_id=$1 AND state='OPEN'`, id); err != nil {
		return err
	}
	if _, err = final.Exec(ctx, `UPDATE object_io_operations SET state='ORPHAN',next_sweep_at=clock_timestamp()+interval '5 minutes' WHERE version_id=$1 AND storage_generation=$2 AND kind='WRITE'`, id, gen); err != nil {
		return err
	}
	if _, err = final.Exec(ctx, `UPDATE scope_quotas SET used_bytes=GREATEST(0,used_bytes-$2),version=version+1,updated_at=now() WHERE scope_id=$1`, scope, size); err != nil {
		return err
	}
	if _, err = final.Exec(ctx, `UPDATE object_gc_jobs SET completed_at=now(),claim_until=NULL WHERE token=$1 AND claim_token=$2`, token, claim); err != nil {
		return err
	}
	if err = addEvent(ctx, final, "FILE_VERSION_COLLECTED", id, version, map[string]any{"version_id": id, "storage_generation": gen, "physical_bytes_released": size}); err != nil {
		return err
	}
	if err = final.Commit(ctx); err != nil {
		return err
	}
	failed = false
	return nil
}

// Keep tombstones and revisit them: a timed-out object-store PUT may still
// become visible after an earlier DELETE. Keys are unique per write attempt.
func (m *Maintenance) sweepOrphans(ctx context.Context) error {
	rows, err := m.db.Query(ctx, `SELECT io.id FROM object_io_operations io JOIN file_versions v ON v.id=io.version_id
        WHERE io.kind='WRITE' AND io.next_sweep_at<=clock_timestamp() AND
        (io.state='ORPHAN' OR(io.state='OPEN' AND io.expires_at<=clock_timestamp()) OR(io.state='COMMITTED' AND v.state='COLLECTED'))
        ORDER BY io.next_sweep_at LIMIT 50`)
	if err != nil {
		return err
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := m.sweepOrphan(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Maintenance) sweepOrphan(ctx context.Context, id string) error {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var version, key string
	if err = tx.QueryRow(ctx, `SELECT version_id,object_key FROM object_io_operations WHERE id=$1 AND kind='WRITE'`, id).Scan(&version, &key); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM file_versions WHERE id=$1 FOR UPDATE`, version); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE object_io_operations SET state='ORPHAN',next_sweep_at=clock_timestamp()+interval '5 minutes',sweep_count=sweep_count+1
        WHERE id=$1 AND(state='ORPHAN' OR(state='OPEN' AND expires_at<=clock_timestamp()) OR(state='COMMITTED' AND EXISTS(SELECT 1 FROM file_versions WHERE id=$2 AND state='COLLECTED')))
        AND NOT EXISTS(SELECT 1 FROM upload_chunks WHERE object_key=$3)`, id, version, key)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	erase, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = m.blobs.DeleteChunk(erase, key); err != nil {
		_, _ = m.db.Exec(ctx, `UPDATE object_io_operations SET next_sweep_at=clock_timestamp()+interval '5 seconds' WHERE id=$1`, id)
	}
	return err
}
