-- +goose Up
-- Keep un-attributable legacy rows for audit; the empty owner is never a valid actor.
ALTER TABLE delivery_receipts ADD COLUMN owner_id TEXT NOT NULL DEFAULT '';
ALTER TABLE delivery_receipts DROP CONSTRAINT delivery_receipts_pkey;
ALTER TABLE delivery_receipts ADD PRIMARY KEY(entry_id,owner_id,device_id);
INSERT INTO delivery_receipts(entry_id,owner_id,device_id,first_delivered_at)
SELECT p.entry_id,t.owner_id,p.device_id,min(p.completed_at)
FROM pull_sessions p JOIN tasks t ON t.reference_id=p.id AND t.kind='DOWNLOAD'
WHERE p.state='COMPLETED' AND p.completed_at IS NOT NULL GROUP BY p.entry_id,t.owner_id,p.device_id
ON CONFLICT(entry_id,owner_id,device_id) DO UPDATE SET first_delivered_at=LEAST(delivery_receipts.first_delivered_at,EXCLUDED.first_delivered_at);
INSERT INTO delivery_receipts(entry_id,owner_id,device_id,first_delivered_at)
SELECT tr.entry_id,tr.recipient_owner_id,tr.recipient_device_id,COALESCE(tr.responded_at,tr.updated_at)
FROM transfer_requests tr WHERE tr.entry_id IS NOT NULL AND tr.state='COMPLETED' AND tr.content_root IS NOT NULL
AND tr.received_bytes=tr.size_bytes AND EXISTS(SELECT 1 FROM tasks t WHERE t.reference_id=tr.id AND t.kind='DOWNLOAD' AND t.completed_at IS NOT NULL AND t.state IN ('COMPLETED','DELETED'))
ON CONFLICT(entry_id,owner_id,device_id) DO UPDATE SET first_delivered_at=LEAST(delivery_receipts.first_delivered_at,EXCLUDED.first_delivered_at);
UPDATE file_access_grants g SET revoked_at=COALESCE(g.revoked_at,now()) WHERE g.purpose='RESTORE'
AND NOT EXISTS(SELECT 1 FROM delivery_receipts d WHERE d.entry_id=g.entry_id AND d.owner_id=g.owner_id AND d.device_id=g.device_id);
