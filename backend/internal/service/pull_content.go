package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) beginPullObjects(ctx context.Context, id string) (pgx.Tx, error) {
	var entry string
	var tr *string
	err := s.db.QueryRow(ctx, `SELECT entry_id,transfer_id FROM pull_sessions WHERE id=$1`, id).Scan(&entry, &tr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.beginEntryAccess(ctx, entry, stringValue(tr))
}

func (s *Service) CreatePull(ctx context.Context, entryID string, req PullCreateRequest) (Pull, error) {
	if !validIdentity(req.OwnerID) || !validIdentity(req.DeviceID) {
		return Pull{}, ErrInvalid
	}
	tx, err := s.beginEntryAccess(ctx, entryID, req.TransferID)
	if err != nil {
		return Pull{}, err
	}
	defer tx.Rollback(ctx)
	if err = fileReadAccess(ctx, tx, entryID, req.OwnerID, req.DeviceID, req.TransferID); err != nil {
		return Pull{}, err
	}
	pull := Pull{ID: uuid.NewString(), EntryID: entryID, DeviceID: req.DeviceID, TransferID: req.TransferID}
	if err = tx.QueryRow(ctx, `SELECT e.file_name,v.content_root,v.size_bytes FROM file_entries e JOIN file_versions v ON v.id=e.version_id WHERE e.id=$1`, entryID).Scan(&pull.FileName, &pull.ContentRoot, &pull.SizeBytes); err != nil {
		return Pull{}, err
	}
	var transfer *string
	if req.TransferID != "" {
		if err = hasFileGrant(ctx, tx, entryID, req.TransferID, req.OwnerID, req.DeviceID); err != nil {
			return Pull{}, err
		}
		transfer = &req.TransferID
	}
	if _, err = tx.Exec(ctx, `INSERT INTO pull_sessions(id,entry_id,device_id,transfer_id) VALUES($1,$2,$3,$4)`, pull.ID, entryID, req.DeviceID, transfer); err != nil {
		return Pull{}, err
	}
	pull.TaskID, err = createTask(ctx, tx, "DOWNLOAD", pull.ID, req.OwnerID, req.DeviceID, pull.SizeBytes)
	if err != nil {
		return Pull{}, err
	}
	return pull, tx.Commit(ctx)
}

func loadContent(ctx context.Context, tx pgx.Tx, pullID string) (Content, error) {
	var c Content
	var state, taskState, owner string
	err := tx.QueryRow(ctx, `SELECT p.id,COALESCE(p.transfer_id::text,''),p.entry_id,p.device_id,e.file_name,v.content_root,v.size_bytes,v.chunk_size,
        t.id,t.state,t.owner_id,p.state,v.id,v.storage_generation
        FROM pull_sessions p JOIN file_entries e ON e.id=p.entry_id JOIN file_versions v ON v.id=e.version_id
        JOIN tasks t ON t.reference_id=p.id AND t.kind='DOWNLOAD' WHERE p.id=$1`, pullID).
		Scan(&c.Pull.ID, &c.Pull.TransferID, &c.Pull.EntryID, &c.Pull.DeviceID, &c.Pull.FileName, &c.Pull.ContentRoot, &c.TotalSize, &c.ChunkSize,
			&c.Pull.TaskID, &taskState, &owner, &state, &c.VersionID, &c.Generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return Content{}, ErrNotFound
	}
	if err != nil {
		return Content{}, err
	}
	if (state != "PULLING" && state != "COMPLETED") || (taskState != "RUNNING" && taskState != "COMPLETED") {
		return Content{}, ErrConflict
	}
	if err = fileReadAccess(ctx, tx, c.Pull.EntryID, owner, c.Pull.DeviceID, c.Pull.TransferID); err != nil {
		return Content{}, err
	}
	c.Pull.SizeBytes = c.TotalSize
	var stored int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM upload_chunks c JOIN upload_sessions u ON u.id=c.upload_id WHERE u.version_id=$1 AND c.storage_generation=$2`, c.VersionID, c.Generation).Scan(&stored); err != nil {
		return Content{}, err
	}
	if stored != chunkCount(c.TotalSize, c.ChunkSize) {
		return Content{}, fmt.Errorf("%w: stored chunk count", ErrIncomplete)
	}
	return c, nil
}

func (s *Service) LoadContent(ctx context.Context, pullID string) (Content, error) {
	tx, err := s.beginPullObjects(ctx, pullID)
	if err != nil {
		return Content{}, err
	}
	defer tx.Rollback(ctx)
	c, err := loadContent(ctx, tx, pullID)
	if err != nil {
		return Content{}, err
	}
	return c, tx.Commit(ctx)
}

func (s *Service) CompletePull(ctx context.Context, pullID string, req PullCompleteRequest) (PullCompleteResponse, error) {
	if !validIdentity(req.DeviceID) || !validSHA256(req.ContentRoot) {
		return PullCompleteResponse{}, ErrInvalid
	}
	tx, err := s.beginPullObjects(ctx, pullID)
	if err != nil {
		return PullCompleteResponse{}, err
	}
	defer tx.Rollback(ctx)
	// All download control paths lock task before pull.
	task, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE kind='DOWNLOAD' AND reference_id=$1 FOR UPDATE`, pullID))
	if err != nil {
		return PullCompleteResponse{}, err
	}
	var entry, device, state, root string
	var transfer *string
	err = tx.QueryRow(ctx, `SELECT p.entry_id,p.device_id,p.state,v.content_root,p.transfer_id FROM pull_sessions p JOIN file_entries e ON e.id=p.entry_id JOIN file_versions v ON v.id=e.version_id
        WHERE p.id=$1 FOR UPDATE OF p`, pullID).Scan(&entry, &device, &state, &root, &transfer)
	if err != nil {
		return PullCompleteResponse{}, err
	}
	if device != req.DeviceID || root != req.ContentRoot {
		return PullCompleteResponse{}, ErrChecksum
	}
	if state == "COMPLETED" {
		return PullCompleteResponse{FirstDelivery: false}, tx.Commit(ctx)
	}
	if state != "PULLING" || task.State != "RUNNING" {
		return PullCompleteResponse{}, &StateConflictError{Resource: "task", Current: task}
	}
	if err = fileReadAccess(ctx, tx, entry, task.OwnerID, device, stringValue(transfer)); err != nil {
		return PullCompleteResponse{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE pull_sessions SET state='COMPLETED',completed_at=now(),version=version+1,updated_at=now() WHERE id=$1`, pullID); err != nil {
		return PullCompleteResponse{}, err
	}
	if err = completeTask(ctx, tx, "DOWNLOAD", pullID); err != nil {
		return PullCompleteResponse{}, err
	}
	var deliveredAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO delivery_receipts(entry_id,owner_id,device_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING RETURNING first_delivered_at`, entry, task.OwnerID, device).Scan(&deliveredAt)
	first := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return PullCompleteResponse{}, err
	}
	if transfer != nil {
		tr, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfer_requests WHERE id=$1`, *transfer))
		if err != nil {
			return PullCompleteResponse{}, err
		}
		if tr.State == "ACCEPTED" {
			tr.State = "COMPLETED"
			tr.Version++
			if _, err = tx.Exec(ctx, `UPDATE transfer_requests SET state='COMPLETED',version=version+1,received_bytes=COALESCE(size_bytes,received_bytes),updated_at=now(),responded_at=now() WHERE id=$1`, tr.ID); err != nil {
				return PullCompleteResponse{}, err
			}
			if err = completeTask(ctx, tx, "DOWNLOAD", tr.ID); err != nil {
				return PullCompleteResponse{}, err
			}
			if err = addEvent(ctx, tx, "TRANSFER_COMPLETED", tr.ID, tr.Version, tr); err != nil {
				return PullCompleteResponse{}, err
			}
		}
		if err = grantRestore(ctx, tx, tr); err != nil {
			return PullCompleteResponse{}, err
		}
	}
	if err = addEvent(ctx, tx, "PULL_COMPLETED", pullID, task.Version+1, map[string]any{"pull_id": pullID, "entry_id": entry, "owner_id": task.OwnerID, "device_id": device}); err != nil {
		return PullCompleteResponse{}, err
	}
	if first {
		if err = addEvent(ctx, tx, "DELIVERY_CONFIRMED", entry, 1, map[string]any{"entry_id": entry, "device_id": device, "owner_id": task.OwnerID, "first_delivered_at": deliveredAt}); err != nil {
			return PullCompleteResponse{}, err
		}
	}
	return PullCompleteResponse{FirstDelivery: first}, tx.Commit(ctx)
}

// Chunk keys are fetched on demand under the caller's read lease.
func (s *Service) PullChunk(ctx context.Context, c Content, index int64) (Chunk, error) {
	if index < 0 || index >= int64(chunkCount(c.TotalSize, c.ChunkSize)) {
		return Chunk{}, ErrInvalid
	}
	var chunk Chunk
	err := s.db.QueryRow(ctx, `SELECT c.chunk_index,c.ciphertext_sha256,c.object_key,c.size_bytes FROM upload_chunks c JOIN upload_sessions u ON u.id=c.upload_id
        WHERE u.version_id=$1 AND c.storage_generation=$2 AND c.chunk_index=$3`, c.VersionID, c.Generation, index).Scan(&chunk.Index, &chunk.Hash, &chunk.Key, &chunk.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return Chunk{}, ErrIncomplete
	}
	return chunk, err
}
