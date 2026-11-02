package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

func (s *Service) OpenPullRead(ctx context.Context, pullID string) (Content, string, error) {
	tx, err := s.beginPullObjects(ctx, pullID)
	if err != nil {
		return Content{}, "", err
	}
	defer tx.Rollback(ctx)
	c, err := loadContent(ctx, tx, pullID)
	if err != nil {
		return Content{}, "", err
	}
	id := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO object_io_operations(id,version_id,storage_generation,transfer_id,transfer_epoch,kind,expires_at)
        VALUES($1,$2,$3,NULLIF($4,'')::uuid,COALESCE((SELECT revocation_epoch FROM transfer_requests WHERE id=NULLIF($4,'')::uuid),0),'READ',clock_timestamp()+interval '45 seconds')`, id, c.VersionID, c.Generation, c.Pull.TransferID); err != nil {
		return Content{}, "", err
	}
	return c, id, tx.Commit(ctx)
}

func (s *Service) CheckPullRead(ctx context.Context, pullID, lease string) error {
	tx, err := s.beginPullObjects(ctx, pullID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var entry, owner, device, state, version, gen, taskState string
	var transfer *string
	err = tx.QueryRow(ctx, `SELECT p.entry_id,t.owner_id,p.device_id,p.state,e.version_id,v.storage_generation,t.state,p.transfer_id
        FROM pull_sessions p JOIN tasks t ON t.kind='DOWNLOAD' AND t.reference_id=p.id JOIN file_entries e ON e.id=p.entry_id JOIN file_versions v ON v.id=e.version_id WHERE p.id=$1`, pullID).
		Scan(&entry, &owner, &device, &state, &version, &gen, &taskState, &transfer)
	if err != nil {
		return err
	}
	if (state != "PULLING" && state != "COMPLETED") || (taskState != "RUNNING" && taskState != "COMPLETED") {
		return ErrConflict
	}
	if err = fileReadAccess(ctx, tx, entry, owner, device, stringValue(transfer)); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE object_io_operations SET expires_at=clock_timestamp()+interval '45 seconds' WHERE id=$1 AND version_id=$2 AND storage_generation=$3
        AND kind='READ' AND state='OPEN' AND expires_at>clock_timestamp() AND transfer_epoch=COALESCE((SELECT revocation_epoch FROM transfer_requests WHERE id=$4),0)`, lease, version, gen, transfer)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Service) CloseRead(ctx context.Context, lease string) { s.closeIO(ctx, lease, false) }

func relayDownloadAccess(ctx context.Context, tx pgx.Tx, o uploadObject) error {
	tr := o.Transfer
	if tr == nil {
		return ErrNotFound
	}
	if tr.State != "ACCEPTED" {
		return &StateConflictError{Resource: "transfer", Current: transferEvent(*tr)}
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_access_grants g JOIN devices d ON d.owner_id=g.owner_id AND d.device_id=g.device_id
        WHERE g.transfer_id=$1 AND g.purpose='DELIVERY' AND g.revoked_at IS NULL AND g.issued_epoch=$2 AND g.expires_at>clock_timestamp() AND d.status='ACTIVE')`, tr.ID, tr.RevocationEpoch).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrExpired
	}
	task, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE kind='DOWNLOAD' AND reference_id=$1 FOR UPDATE`, tr.ID))
	if err != nil {
		return err
	}
	if task.State != "RUNNING" {
		return &StateConflictError{Resource: "task", Current: task}
	}
	if o.VersionState != "UPLOADING" && o.VersionState != "SEALED" {
		return ErrConflict
	}
	if o.State != "OPEN" && o.State != "SEALED" {
		return ErrConflict
	}
	if o.State == "SEALED" && tr.EntryID != nil {
		return fileReadAccess(ctx, tx, *tr.EntryID, tr.RecipientOwnerID, tr.RecipientDeviceID, tr.ID)
	}
	return nil
}

// One bounded chunk, protected during object I/O and reauthorized after it.
// No transaction or session-wide mutex spans an object-store request.
func (s *Service) readRelayChunk(ctx context.Context, uploadID string, index int) ([]byte, error) {
	tx, o, err := s.beginUploadObject(ctx, uploadID, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err = relayDownloadAccess(ctx, tx, o); err != nil {
		return nil, err
	}
	if o.Count == 0 {
		if o.State != "SEALED" {
			return nil, ErrIncomplete
		}
		return []byte{}, nil
	}
	var c Chunk
	err = tx.QueryRow(ctx, `SELECT chunk_index,ciphertext_sha256,object_key,size_bytes FROM upload_chunks WHERE upload_id=$1 AND chunk_index=$2 AND storage_generation=$3`, uploadID, index, o.Generation).Scan(&c.Index, &c.Hash, &c.Key, &c.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIncomplete
	}
	if err != nil {
		return nil, err
	}
	if c.Size > 64<<10 {
		return nil, ErrInvalid
	}
	lease := uuid.NewString()
	if err = newUploadIO(ctx, tx, o, "READ", "", lease); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	defer s.closeIO(ctx, lease, false)
	read, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	object, err := s.blobs.GetChunk(read, c.Key, 0, c.Size-1)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(object, c.Size+1))
	closeErr := object.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	digest := sha256.Sum256(data)
	if int64(len(data)) != c.Size || hex.EncodeToString(digest[:]) != c.Hash {
		return nil, ErrChecksum
	}
	final, current, err := s.beginUploadObject(ctx, uploadID, true)
	if err != nil {
		return nil, err
	}
	defer final.Rollback(ctx)
	if err = relayDownloadAccess(ctx, final, current); err != nil {
		return nil, err
	}
	if err = checkUploadIO(ctx, final, current, lease); err != nil {
		return nil, err
	}
	return data, nil
}
