-- +goose Up
ALTER TABLE file_versions ADD COLUMN quota_scope_id TEXT;
UPDATE file_versions v SET quota_scope_id='relay-owner:'||tr.sender_owner_id
FROM relay_spools s JOIN transfer_requests tr ON tr.id=s.transfer_id WHERE v.id=s.version_id;
INSERT INTO scope_quotas(scope_id,quota_bytes,used_bytes)
SELECT quota_scope_id,1099511627776,sum(size_bytes) FROM file_versions
WHERE quota_scope_id IS NOT NULL AND state<>'COLLECTED' GROUP BY quota_scope_id
ON CONFLICT(scope_id) DO UPDATE SET used_bytes=scope_quotas.used_bytes+EXCLUDED.used_bytes,version=scope_quotas.version+1;
UPDATE scope_quotas q SET used_bytes=GREATEST(0,q.used_bytes-v.bytes),version=q.version+1,updated_at=now()
FROM (SELECT scope_id,sum(size_bytes) AS bytes FROM file_versions WHERE quota_scope_id IS NOT NULL AND state<>'COLLECTED' GROUP BY scope_id) v
WHERE q.scope_id=v.scope_id;
