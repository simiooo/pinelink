package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Every operation which can attach/release a spool reference locks its object
// parents before the transfer. If a previously unbound spool appeared, retry.
func (s *Service) beginTransferObjects(ctx context.Context, id string) (pgx.Tx, Transfer, error) {
	for attempt := 0; attempt < 8; attempt++ {
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return nil, Transfer{}, err
		}
		parents, err := transferParents(ctx, tx, id)
		if err != nil {
			tx.Rollback(ctx)
			return nil, Transfer{}, err
		}
		for _, v := range parents {
			if _, err = tx.Exec(ctx, `SELECT id FROM file_versions WHERE id=$1 FOR UPDATE`, v); err != nil {
				break
			}
		}
		if err != nil {
			tx.Rollback(ctx)
			return nil, Transfer{}, err
		}
		tr, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfer_requests WHERE id=$1 FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
		if err != nil {
			tx.Rollback(ctx)
			return nil, Transfer{}, err
		}
		current, err := transferParents(ctx, tx, id)
		if err == nil && strings.Join(parents, ",") == strings.Join(current, ",") {
			return tx, tr, nil
		}
		tx.Rollback(ctx)
		if err != nil {
			return nil, Transfer{}, err
		}
	}
	return nil, Transfer{}, ErrConflict
}

func transferParents(ctx context.Context, tx pgx.Tx, id string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT version_id::text FROM relay_spools WHERE transfer_id=$1
        UNION SELECT e.version_id::text FROM file_entries e JOIN transfer_requests tr ON tr.entry_id=e.id WHERE tr.id=$1`, id)
	if err != nil {
		return nil, err
	}
	ids, err := scanIDs(rows)
	sort.Strings(ids)
	return ids, err
}

func (s *Service) beginFileObject(ctx context.Context, entryID string) (pgx.Tx, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var v string
	err = tx.QueryRow(ctx, `SELECT version_id FROM file_entries WHERE id=$1`, entryID).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err == nil {
		_, err = tx.Exec(ctx, `SELECT id FROM file_versions WHERE id=$1 FOR UPDATE`, v)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func transferUploadAllowed(ctx context.Context, tx pgx.Tx, tr Transfer) error {
	var alive bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz>clock_timestamp()`, tr.ExpiresAt).Scan(&alive); err != nil {
		return err
	}
	if (tr.State != "PENDING" && (tr.State != "ACCEPTED" || !time.Now().Before(tr.ExpiresAt)) && tr.State != "COMPLETED") || (tr.State != "COMPLETED" && !alive) {
		return &StateConflictError{Resource: "transfer", Current: transferEvent(tr)}
	}
	var devices int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM devices WHERE status='ACTIVE' AND ((owner_id=$1 AND device_id=$2) OR(owner_id=$3 AND device_id=$4))`, tr.SenderOwnerID, tr.SenderDeviceID, tr.RecipientOwnerID, tr.RecipientDeviceID).Scan(&devices); err != nil {
		return err
	}
	expected := 2
	if tr.SenderOwnerID == tr.RecipientOwnerID && tr.SenderDeviceID == tr.RecipientDeviceID {
		expected = 1
	}
	if devices != expected {
		return ErrUnauthorized
	}
	return nil
}

type uploadObject struct {
	ID, VersionID, Generation, VersionState, State, OwnerID, DeviceID, FileName, Retention, Root, Scope string
	Size                                                                                                int64
	ChunkSize, Count                                                                                    int
	TransferID                                                                                          *string
	Transfer                                                                                            *Transfer
}

func (s *Service) beginUploadObject(ctx context.Context, id string, authorize bool) (pgx.Tx, uploadObject, error) {
	var o uploadObject
	o.ID = id
	err := s.db.QueryRow(ctx, `SELECT version_id,transfer_id FROM upload_sessions WHERE id=$1`, id).Scan(&o.VersionID, &o.TransferID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	if err != nil {
		return nil, o, err
	}
	var tx pgx.Tx
	if o.TransferID != nil {
		var tr Transfer
		tx, tr, err = s.beginTransferObjects(ctx, *o.TransferID)
		o.Transfer = &tr
	} else {
		tx, err = s.db.Begin(ctx)
		if err == nil {
			_, err = tx.Exec(ctx, `SELECT id FROM file_versions WHERE id=$1 FOR UPDATE`, o.VersionID)
		}
	}
	if err != nil {
		if tx != nil {
			tx.Rollback(ctx)
		}
		return nil, o, err
	}
	err = tx.QueryRow(ctx, `SELECT v.storage_generation,v.state,v.content_root,v.scope_id,v.size_bytes,v.chunk_size,v.chunk_count,
        u.state,u.owner_id,u.device_id,u.file_name,u.retention_class
        FROM upload_sessions u JOIN file_versions v ON v.id=u.version_id
        WHERE u.id=$1 AND u.storage_generation=v.storage_generation FOR UPDATE OF u`, id).
		Scan(&o.Generation, &o.VersionState, &o.Root, &o.Scope, &o.Size, &o.ChunkSize, &o.Count, &o.State, &o.OwnerID, &o.DeviceID, &o.FileName, &o.Retention)
	if err == nil && authorize && o.Transfer != nil {
		err = transferUploadAllowed(ctx, tx, *o.Transfer)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, o, err
	}
	return tx, o, nil
}

func (s *Service) prepareRelaySpool(ctx context.Context, id string) (uploadObject, error) {
	tx, tr, err := s.beginTransferObjects(ctx, id)
	if err != nil {
		return uploadObject{}, err
	}
	defer tx.Rollback(ctx)
	if err = transferUploadAllowed(ctx, tx, tr); err != nil {
		return uploadObject{}, err
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT upload_id FROM relay_spools WHERE transfer_id=$1`, id).Scan(&existing)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return uploadObject{}, err
		}
		rt, o, err := s.beginUploadObject(ctx, existing, true)
		if rt != nil {
			rt.Rollback(ctx)
		}
		return o, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uploadObject{}, err
	}
	if tr.SizeBytes == nil || tr.ContentRoot == nil {
		return uploadObject{}, ErrInvalid
	}
	o := uploadObject{ID: uuid.NewString(), VersionID: uuid.NewString(), Generation: uuid.NewString(), TransferID: &id, Transfer: &tr,
		VersionState: "UPLOADING", State: "OPEN", OwnerID: tr.SenderOwnerID, DeviceID: tr.SenderDeviceID, FileName: stringValue(tr.FileName), Retention: "TEMP_24H",
		Root: *tr.ContentRoot, Scope: "p2p-" + id, Size: *tr.SizeBytes, ChunkSize: 64 << 10, Count: chunkCount(*tr.SizeBytes, 64<<10)}
	quotaScope := "relay-owner:" + tr.SenderOwnerID
	if _, err = tx.Exec(ctx, `INSERT INTO file_versions(id,storage_generation,scope_id,quota_scope_id,encryption_domain_id,hash_version,dedup_key,content_root,size_bytes,chunk_size,chunk_count,state,encryption_suite)
        VALUES($1,$2,$3,$10,$4,'sha256-v1',$5,$5,$6,$7,$8,'UPLOADING',$9)`, o.VersionID, o.Generation, o.Scope, tr.RecipientOwnerID, o.Root, o.Size, o.ChunkSize, o.Count, stringValue(tr.EncryptionSuite), quotaScope); err != nil {
		return o, err
	}
	// This row also serializes the sender's concurrent spool admission checks.
	if err = reserveQuota(ctx, tx, quotaScope, o.Size); err != nil {
		return o, err
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM upload_sessions u JOIN transfer_requests tr ON tr.id=u.transfer_id
        WHERE u.owner_id=$1 AND u.state='OPEN' AND u.last_activity_at>clock_timestamp()-interval '24 hours'
        AND(tr.state='COMPLETED' OR(tr.state IN ('PENDING','ACCEPTED') AND tr.expires_at>clock_timestamp()))`, o.OwnerID).Scan(&active); err != nil {
		return o, err
	}
	if active >= 4 {
		return o, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO upload_sessions(id,version_id,storage_generation,transfer_id,owner_id,device_id,file_name,retention_class) VALUES($1,$2,$3,$4,$5,$6,$7,'TEMP_24H')`, o.ID, o.VersionID, o.Generation, id, o.OwnerID, o.DeviceID, o.FileName); err != nil {
		return o, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO relay_spools(transfer_id,version_id,upload_id) VALUES($1,$2,$3)`, id, o.VersionID, o.ID); err != nil {
		return o, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO manifest_envelopes(version_id,owner_id,device_id,ciphertext) VALUES($1,$2,$3,$4)`, o.VersionID, o.OwnerID, o.DeviceID, tr.SenderEncryptedManifest); err != nil {
		return o, err
	}
	if _, err = createTask(ctx, tx, "UPLOAD", o.ID, o.OwnerID, o.DeviceID, o.Size); err != nil {
		return o, err
	}
	return o, tx.Commit(ctx)
}

func grantDelivery(ctx context.Context, tx pgx.Tx, tr Transfer) error {
	_, err := tx.Exec(ctx, `INSERT INTO file_access_grants(transfer_id,entry_id,owner_id,device_id,purpose,issued_epoch,expires_at)
        VALUES($1,$2,$3,$4,'DELIVERY',$5,$6) ON CONFLICT DO NOTHING`, tr.ID, tr.EntryID, tr.RecipientOwnerID, tr.RecipientDeviceID, tr.RevocationEpoch, tr.ExpiresAt)
	return err
}

func grantRestore(ctx context.Context, tx pgx.Tx, tr Transfer) error {
	if tr.EntryID == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO file_access_grants(transfer_id,entry_id,owner_id,device_id,purpose,issued_epoch,expires_at)
        SELECT $1,e.id,$2,$3,'RESTORE',$4,e.expires_at FROM file_entries e JOIN file_versions v ON v.id=e.version_id
        WHERE e.id=$5 AND e.state='AVAILABLE' AND v.state='SEALED' AND(e.expires_at IS NULL OR e.expires_at>now())
        AND EXISTS(SELECT 1 FROM delivery_receipts d WHERE d.entry_id=e.id AND d.owner_id=$2 AND d.device_id=$3)
        ON CONFLICT DO NOTHING`, tr.ID, tr.RecipientOwnerID, tr.RecipientDeviceID, tr.RevocationEpoch, *tr.EntryID)
	return err
}

func hasFileGrant(ctx context.Context, tx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, entryID, transferID, ownerID, deviceID string) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_access_grants g JOIN transfer_requests tr ON tr.id=g.transfer_id
        WHERE g.entry_id=$1 AND g.transfer_id=$2 AND g.owner_id=$3 AND g.device_id=$4 AND g.revoked_at IS NULL
        AND g.issued_epoch=tr.revocation_epoch AND(g.expires_at IS NULL OR g.expires_at>clock_timestamp())
        AND NOT EXISTS(SELECT 1 FROM devices d WHERE d.owner_id=$3 AND d.device_id=$4 AND d.status='REVOKED')
        AND ((g.purpose='DELIVERY' AND tr.state='ACCEPTED' AND tr.expires_at>clock_timestamp()) OR (g.purpose='RESTORE' AND tr.state='COMPLETED')))`, entryID, transferID, ownerID, deviceID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func releaseTransferResources(ctx context.Context, tx pgx.Tx, tr Transfer) error {
	for _, query := range []string{
		`UPDATE file_access_grants SET revoked_at=COALESCE(revoked_at,now()) WHERE transfer_id=$1`,
		`UPDATE upload_sessions SET state=CASE WHEN $2='EXPIRED' THEN 'EXPIRED' ELSE 'CANCELLED' END WHERE transfer_id=$1 AND state IN ('OPEN','SEALED')`,
		`UPDATE file_entries SET state=CASE WHEN $2='EXPIRED' THEN 'EXPIRED' ELSE 'DELETED' END,version=version+1,updated_at=now()
            WHERE transfer_id=$1 AND reference_kind='DELIVERY' AND state='AVAILABLE'`,
		`UPDATE tasks SET state='CANCELLED',version=version+1,completed_at=now(),updated_at=now(),error_code='TRANSFER_'||$2
            WHERE state IN ('RUNNING','PAUSED','QUEUED') AND(reference_id=$1 OR reference_id IN(SELECT id FROM pull_sessions WHERE transfer_id=$1) OR reference_id IN(SELECT upload_id FROM relay_spools WHERE transfer_id=$1))`,
		`UPDATE pull_sessions SET state='FAILED',version=version+1,updated_at=now(),error_code='TRANSFER_'||$2 WHERE transfer_id=$1 AND state='PULLING'`,
		`UPDATE file_versions SET gc_after=CASE WHEN $2='EXPIRED' AND state='SEALED' THEN now()+interval '15 minutes' ELSE now() END
            WHERE id IN(SELECT version_id FROM relay_spools WHERE transfer_id=$1)`} {
		var err error
		if strings.Contains(query, "$2") {
			_, err = tx.Exec(ctx, query, tr.ID, tr.State)
		} else {
			_, err = tx.Exec(ctx, query, tr.ID)
		}
		if err != nil {
			return err
		}
	}
	return addEvent(ctx, tx, "TRANSFER_RESOURCES_RELEASED", tr.ID, tr.Version, map[string]any{"transfer_id": tr.ID, "state": tr.State, "owner_id": tr.SenderOwnerID, "device_id": tr.SenderDeviceID})
}

func (s *Service) bindSpoolEntry(ctx context.Context, tx pgx.Tx, o uploadObject, entry FileEntry) error {
	if o.Transfer == nil {
		return nil
	}
	tr := *o.Transfer
	if err := transferUploadAllowed(ctx, tx, tr); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE relay_spools SET entry_id=$2 WHERE transfer_id=$1`, tr.ID, entry.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE transfer_requests SET entry_id=$2,version=version+1,updated_at=now() WHERE id=$1`, tr.ID, entry.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO manifest_envelopes(version_id,owner_id,device_id,ciphertext) VALUES($1,$2,$3,$4)
        ON CONFLICT(version_id,owner_id,device_id) DO UPDATE SET ciphertext=EXCLUDED.ciphertext,updated_at=now()`, o.VersionID, tr.RecipientOwnerID, tr.RecipientDeviceID, tr.EncryptedManifest); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE file_access_grants SET entry_id=$2 WHERE transfer_id=$1 AND purpose='DELIVERY' AND revoked_at IS NULL`, tr.ID, entry.ID); err != nil {
		return err
	}
	tr.EntryID = &entry.ID
	if tr.State == "COMPLETED" {
		if err := recordP2PReceipt(ctx, tx, tr, entry.ID); err != nil {
			return err
		}
		return grantRestore(ctx, tx, tr)
	}
	return nil
}

func recordP2PReceipt(ctx context.Context, tx pgx.Tx, tr Transfer, entryID string) error {
	var at time.Time
	err := tx.QueryRow(ctx, `INSERT INTO delivery_receipts(entry_id,owner_id,device_id,first_delivered_at)
        SELECT $2,recipient_owner_id,recipient_device_id,COALESCE(responded_at,updated_at) FROM transfer_requests WHERE id=$1 AND state='COMPLETED'
        ON CONFLICT(entry_id,owner_id,device_id) DO NOTHING RETURNING first_delivered_at`, tr.ID, entryID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return addEvent(ctx, tx, "DELIVERY_CONFIRMED", entryID, 1, map[string]any{"entry_id": entryID, "owner_id": tr.RecipientOwnerID, "device_id": tr.RecipientDeviceID, "transfer_id": tr.ID, "first_delivered_at": at})
}

func presentTransfer(tr Transfer, ownerID, deviceID string) Transfer {
	if tr.SenderOwnerID != ownerID || tr.SenderDeviceID != deviceID {
		tr.SenderEncryptedManifest = nil
		if (tr.State != "ACCEPTED" || !time.Now().Before(tr.ExpiresAt)) && tr.State != "COMPLETED" {
			tr.EncryptedManifest = nil
		}
	}
	return tr
}

// Persist only metadata in domain events. Envelopes are fetched through a live grant.
func transferEvent(tr Transfer) map[string]any {
	return map[string]any{"id": tr.ID, "entry_id": tr.EntryID, "state": tr.State, "version": tr.Version, "delivery_mode": tr.DeliveryMode,
		"sender_owner_id": tr.SenderOwnerID, "sender_device_id": tr.SenderDeviceID, "recipient_owner_id": tr.RecipientOwnerID, "recipient_device_id": tr.RecipientDeviceID,
		"file_name": tr.FileName, "size_bytes": tr.SizeBytes, "expires_at": tr.ExpiresAt, "received_bytes": tr.ReceivedBytes}
}

type rowQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func fileReadAccess(ctx context.Context, q rowQuery, entryID, ownerID, deviceID, transferID string) error {
	var owner, device, kind string
	err := q.QueryRow(ctx, `SELECT e.owner_id,e.device_id,e.reference_kind FROM file_entries e JOIN file_versions v ON v.id=e.version_id
        WHERE e.id=$1 AND e.state='AVAILABLE' AND v.state='SEALED' AND(e.expires_at IS NULL OR e.expires_at>clock_timestamp())
        AND(e.reference_kind<>'DELIVERY' OR EXISTS(SELECT 1 FROM transfer_requests tr WHERE tr.id=e.transfer_id AND(tr.state='COMPLETED' OR(tr.state IN ('PENDING','ACCEPTED') AND tr.expires_at>clock_timestamp()))))
        AND NOT EXISTS(SELECT 1 FROM devices d WHERE d.owner_id=$2 AND d.device_id=$3 AND d.status='REVOKED')`, entryID, ownerID, deviceID).Scan(&owner, &device, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if owner == ownerID && (kind != "DELIVERY" || device == deviceID) {
		return nil
	}
	return hasFileGrant(ctx, q, entryID, transferID, ownerID, deviceID)
}

func (s *Service) beginEntryAccess(ctx context.Context, entryID, transferID string) (pgx.Tx, error) {
	if transferID == "" {
		return s.beginFileObject(ctx, entryID)
	}
	tx, tr, err := s.beginTransferObjects(ctx, transferID)
	if err != nil {
		return nil, err
	}
	if tr.EntryID == nil || *tr.EntryID != entryID {
		tx.Rollback(ctx)
		return nil, ErrNotFound
	}
	return tx, nil
}

func releaseFileAccess(ctx context.Context, tx pgx.Tx, id, code string) error {
	for _, q := range []string{
		`UPDATE file_access_grants SET revoked_at=COALESCE(revoked_at,now()) WHERE entry_id=$1`,
		`UPDATE tasks SET state='CANCELLED',version=version+1,error_code=$2,updated_at=now(),completed_at=now() WHERE kind='DOWNLOAD' AND state IN ('RUNNING','PAUSED','QUEUED') AND reference_id IN(SELECT id FROM pull_sessions WHERE entry_id=$1)`,
		`UPDATE pull_sessions SET state='FAILED',version=version+1,error_code=$2,updated_at=now() WHERE entry_id=$1 AND state='PULLING'`,
	} {
		var err error
		if strings.Contains(q, "$2") {
			_, err = tx.Exec(ctx, q, id, code)
		} else {
			_, err = tx.Exec(ctx, q, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
