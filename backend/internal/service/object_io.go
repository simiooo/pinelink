package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var errUploadSealed = errors.New("upload was sealed by another operation")

func (s *Service) failCorruptUpload(ctx context.Context, id string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	tx, o, err := s.beginUploadObject(cleanup, id, false)
	if err != nil {
		return
	}
	defer tx.Rollback(cleanup)
	if o.State != "OPEN" || o.VersionState != "UPLOADING" {
		return
	}
	if _, err = tx.Exec(cleanup, `UPDATE file_versions SET state='CORRUPT',version=version+1,gc_after=now() WHERE id=$1`, o.VersionID); err != nil {
		return
	}
	if _, err = tx.Exec(cleanup, `UPDATE upload_sessions SET state='FAILED' WHERE id=$1`, id); err != nil {
		return
	}
	if _, err = tx.Exec(cleanup, `UPDATE tasks SET state='FAILED',version=version+1,error_code='CHECKSUM_MISMATCH',updated_at=now(),completed_at=now() WHERE kind='UPLOAD' AND reference_id=$1 AND state IN ('RUNNING','PAUSED')`, id); err != nil {
		return
	}
	if err = addEvent(cleanup, tx, "UPLOAD_FAILED", id, time.Now().UnixNano(), map[string]any{"upload_id": id, "transfer_id": o.TransferID, "error_code": "CHECKSUM_MISMATCH"}); err != nil {
		return
	}
	_ = tx.Commit(cleanup)
}

func uploadRunning(ctx context.Context, tx pgx.Tx, o uploadObject) error {
	if o.State != "OPEN" || o.VersionState != "UPLOADING" {
		return ErrConflict
	}
	task, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE kind='UPLOAD' AND reference_id=$1 FOR UPDATE`, o.ID))
	if err != nil {
		return err
	}
	if task.State != "RUNNING" {
		return &StateConflictError{Resource: "task", Current: task}
	}
	var live bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM object_references WHERE source_key=$1 AND released_at IS NULL AND(expires_at IS NULL OR expires_at>clock_timestamp()))`, "upload:"+o.ID).Scan(&live); err != nil {
		return err
	}
	if !live {
		return ErrExpired
	}
	return nil
}

func newUploadIO(ctx context.Context, tx pgx.Tx, o uploadObject, kind, key, id string) error {
	var epoch int64
	if o.Transfer != nil {
		epoch = o.Transfer.RevocationEpoch
	}
	_, err := tx.Exec(ctx, `INSERT INTO object_io_operations(id,version_id,storage_generation,upload_id,transfer_id,transfer_epoch,kind,object_key,expires_at)
        VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),clock_timestamp()+interval '45 seconds')`, id, o.VersionID, o.Generation, o.ID, o.TransferID, epoch, kind, key)
	return err
}

func checkUploadIO(ctx context.Context, tx pgx.Tx, o uploadObject, id string) error {
	var valid bool
	var epoch int64
	if o.Transfer != nil {
		epoch = o.Transfer.RevocationEpoch
	}
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM object_io_operations WHERE id=$1 AND version_id=$2 AND storage_generation=$3
        AND transfer_epoch=$4 AND state='OPEN' AND expires_at>clock_timestamp())`, id, o.VersionID, o.Generation, epoch).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return ErrConflict
	}
	return nil
}

func (s *Service) closeIO(ctx context.Context, id string, orphan bool) {
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	state := "CLOSED"
	if orphan {
		state = "ORPHAN"
	}
	_, _ = s.db.Exec(finish, `UPDATE object_io_operations SET state=$2,next_sweep_at=now() WHERE id=$1 AND state IN ('OPEN','ORPHAN')`, id, state)
}

func (s *Service) putUploadChunk(ctx context.Context, uploadID string, index int, suppliedHash string, data []byte) error {
	if !validSHA256(suppliedHash) {
		return ErrInvalid
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != suppliedHash {
		return ErrChecksum
	}
	tx, o, err := s.beginUploadObject(ctx, uploadID, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = uploadRunning(ctx, tx, o); err != nil {
		return err
	}
	if index < 0 || index >= o.Count || int64(len(data)) != expectedChunkSize(o.Size, o.ChunkSize, index, o.Count) {
		return ErrInvalid
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT ciphertext_sha256 FROM upload_chunks WHERE upload_id=$1 AND chunk_index=$2 AND storage_generation=$3`, uploadID, index, o.Generation).Scan(&existing)
	if err == nil {
		if existing != suppliedHash {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	operation := uuid.NewString()
	key := fmt.Sprintf("versions/%s/generations/%s/writes/%s/%08d-%s", o.VersionID, o.Generation, operation, index, suppliedHash)
	if err = newUploadIO(ctx, tx, o, "WRITE", key, operation); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			s.closeIO(ctx, operation, true)
		}
	}()
	put, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = s.blobs.PutChunk(put, key, data)
	cancel()
	if err != nil {
		return err
	}
	next, current, err := s.beginUploadObject(ctx, uploadID, true)
	if err != nil {
		return err
	}
	defer next.Rollback(ctx)
	if current.Generation != o.Generation {
		return ErrConflict
	}
	if err = uploadRunning(ctx, next, current); err != nil {
		return err
	}
	if err = checkUploadIO(ctx, next, current, operation); err != nil {
		return err
	}
	result, err := next.Exec(ctx, `INSERT INTO upload_chunks(upload_id,chunk_index,ciphertext_sha256,object_key,size_bytes,storage_generation)
        VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(upload_id,chunk_index) DO NOTHING`, uploadID, index, suppliedHash, key, len(data), o.Generation)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		if err = next.QueryRow(ctx, `SELECT ciphertext_sha256 FROM upload_chunks WHERE upload_id=$1 AND chunk_index=$2`, uploadID, index).Scan(&existing); err != nil {
			return err
		}
		if existing != suppliedHash {
			return ErrConflict
		}
		if _, err = next.Exec(ctx, `UPDATE object_io_operations SET state='ORPHAN',next_sweep_at=now() WHERE id=$1`, operation); err != nil {
			return err
		}
	} else {
		if _, err = next.Exec(ctx, `UPDATE object_io_operations SET state='COMMITTED' WHERE id=$1`, operation); err != nil {
			return err
		}
		if _, err = next.Exec(ctx, `UPDATE upload_sessions SET last_activity_at=now() WHERE id=$1`, uploadID); err != nil {
			return err
		}
		if _, err = next.Exec(ctx, `UPDATE tasks SET progress_bytes=(SELECT COALESCE(sum(size_bytes),0) FROM upload_chunks WHERE upload_id=$1),
            progress_revision=progress_revision+1,progress_sampled_at=now(),updated_at=now() WHERE kind='UPLOAD' AND reference_id=$1`, uploadID); err != nil {
			return err
		}
	}
	if err = next.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Service) renewUploadIO(ctx context.Context, id, uploadID string) error {
	tx, o, err := s.beginUploadObject(ctx, uploadID, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if o.State == "SEALED" && o.VersionState == "SEALED" {
		return errUploadSealed
	}
	if err = uploadRunning(ctx, tx, o); err != nil {
		return err
	}
	if err = checkUploadIO(ctx, tx, o, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE object_io_operations SET expires_at=clock_timestamp()+interval '45 seconds' WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) sealUpload(ctx context.Context, uploadID string) (CompleteResponse, error) {
	tx, o, err := s.beginUploadObject(ctx, uploadID, true)
	if err != nil {
		return CompleteResponse{}, err
	}
	defer tx.Rollback(ctx)
	if o.State == "SEALED" {
		entry, err := entryByUpload(ctx, tx, uploadID)
		if err != nil {
			return CompleteResponse{}, err
		}
		if entry.State != "AVAILABLE" || (entry.ExpiresAt != nil && !time.Now().Before(*entry.ExpiresAt)) {
			return CompleteResponse{}, ErrExpired
		}
		return CompleteResponse{File: entry}, tx.Commit(ctx)
	}
	if err = uploadRunning(ctx, tx, o); err != nil {
		return CompleteResponse{}, err
	}
	var stored int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM upload_chunks WHERE upload_id=$1 AND storage_generation=$2`, uploadID, o.Generation).Scan(&stored); err != nil {
		return CompleteResponse{}, err
	}
	if stored != o.Count {
		return CompleteResponse{}, ErrIncomplete
	}
	operation := uuid.NewString()
	if err = newUploadIO(ctx, tx, o, "VERIFY", "", operation); err != nil {
		return CompleteResponse{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CompleteResponse{}, err
	}
	defer s.closeIO(ctx, operation, false)
	full := sha256.New()
	buffer := make([]byte, 64<<10)
	for next := 0; next < o.Count; {
		rows, err := s.db.Query(ctx, `SELECT chunk_index,ciphertext_sha256,object_key,size_bytes FROM upload_chunks WHERE upload_id=$1 AND storage_generation=$2 AND chunk_index>=$3 ORDER BY chunk_index LIMIT 128`, uploadID, o.Generation, next)
		if err != nil {
			return CompleteResponse{}, err
		}
		var page []Chunk
		for rows.Next() {
			var c Chunk
			if err = rows.Scan(&c.Index, &c.Hash, &c.Key, &c.Size); err != nil {
				rows.Close()
				return CompleteResponse{}, err
			}
			page = append(page, c)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return CompleteResponse{}, err
		}
		if len(page) == 0 {
			return CompleteResponse{}, ErrIncomplete
		}
		if o.ChunkSize <= 64<<10 {
			base := next
			err = orderedPipeline(ctx, 0, len(page), relayPipelineWindow, func(read context.Context, offset int) ([]byte, error) {
				c := page[offset]
				if c.Index != base+offset || c.Size != expectedChunkSize(o.Size, o.ChunkSize, c.Index, o.Count) {
					return nil, ErrIncomplete
				}
				return s.readVerificationChunk(read, uploadID, operation, c)
			}, func(_ int, data []byte) error { _, err := full.Write(data); return err })
			if err != nil {
				return CompleteResponse{}, err
			}
			next += len(page)
			continue
		}
		for _, c := range page {
			if c.Index != next || c.Size != expectedChunkSize(o.Size, o.ChunkSize, next, o.Count) {
				return CompleteResponse{}, ErrIncomplete
			}

			if err = s.renewUploadIO(ctx, operation, uploadID); err != nil {
				return CompleteResponse{}, err
			}
			read, cancel := context.WithTimeout(ctx, 30*time.Second)
			object, err := s.blobs.GetChunk(read, c.Key, 0, c.Size-1)
			if err != nil {
				cancel()
				return CompleteResponse{}, err
			}
			one := sha256.New()
			n, copyErr := io.CopyBuffer(io.MultiWriter(full, one), io.LimitReader(object, c.Size+1), buffer)
			closeErr := object.Close()
			cancel()
			if copyErr != nil {
				return CompleteResponse{}, copyErr
			}
			if closeErr != nil {
				return CompleteResponse{}, closeErr
			}
			if n != c.Size || hex.EncodeToString(one.Sum(nil)) != c.Hash {
				return CompleteResponse{}, ErrChecksum
			}
			next++
		}
	}
	if hex.EncodeToString(full.Sum(nil)) != o.Root {
		return CompleteResponse{}, ErrChecksum
	}
	final, current, err := s.beginUploadObject(ctx, uploadID, true)
	if err != nil {
		return CompleteResponse{}, err
	}
	defer final.Rollback(ctx)
	if current.State == "SEALED" {
		entry, err := entryByUpload(ctx, final, uploadID)
		if err == nil && (entry.State != "AVAILABLE" || (entry.ExpiresAt != nil && !time.Now().Before(*entry.ExpiresAt))) {
			err = ErrExpired
		}
		return CompleteResponse{File: entry}, err
	}
	if err = uploadRunning(ctx, final, current); err != nil {
		return CompleteResponse{}, err
	}
	if err = checkUploadIO(ctx, final, current, operation); err != nil {
		return CompleteResponse{}, err
	}
	var contentVersion int64
	if err = final.QueryRow(ctx, `UPDATE file_versions SET state='SEALED',sealed_at=now(),version=version+1 WHERE id=$1 AND storage_generation=$2 RETURNING version`, o.VersionID, o.Generation).Scan(&contentVersion); err != nil {
		return CompleteResponse{}, err
	}
	if _, err = final.Exec(ctx, `UPDATE upload_sessions SET state='SEALED',last_activity_at=now() WHERE id=$1`, uploadID); err != nil {
		return CompleteResponse{}, err
	}
	entry, err := insertEntry(ctx, final, o.VersionID, &uploadID, nil, o.OwnerID, o.DeviceID, o.FileName, o.Retention, time.Now())
	if err != nil {
		return CompleteResponse{}, err
	}
	if err = s.bindSpoolEntry(ctx, final, current, entry); err != nil {
		return CompleteResponse{}, err
	}
	if err = completeTask(ctx, final, "UPLOAD", uploadID); err != nil {
		return CompleteResponse{}, err
	}
	if _, err = final.Exec(ctx, `UPDATE object_io_operations SET state='CLOSED' WHERE id=$1`, operation); err != nil {
		return CompleteResponse{}, err
	}
	payload := map[string]any{"version_id": o.VersionID, "storage_generation": o.Generation, "entry_id": entry.ID, "transfer_id": o.TransferID, "spool_state": "SEALED", "owner_id": o.OwnerID, "device_id": o.DeviceID}
	if o.Transfer != nil {
		payload["sender_owner_id"], payload["sender_device_id"] = o.Transfer.SenderOwnerID, o.Transfer.SenderDeviceID
		payload["recipient_owner_id"], payload["recipient_device_id"] = o.Transfer.RecipientOwnerID, o.Transfer.RecipientDeviceID
		payload["transfer_version"] = o.Transfer.Version + 1
	}
	if err = addEvent(ctx, final, "SPOOL_SEALED", o.VersionID, contentVersion, payload); err != nil {
		return CompleteResponse{}, err
	}
	if err = addEvent(ctx, final, retentionEvent(o.Retention), entry.ID, 1, entry); err != nil {
		return CompleteResponse{}, err
	}
	if err = final.Commit(ctx); err != nil {
		return CompleteResponse{}, err
	}
	return CompleteResponse{File: entry}, nil
}

func (s *Service) readVerificationChunk(ctx context.Context, uploadID, operation string, c Chunk) ([]byte, error) {
	return s.readSmallChunk(ctx, c, func(check context.Context) error {
		return s.renewUploadIO(check, operation, uploadID)
	})
}
