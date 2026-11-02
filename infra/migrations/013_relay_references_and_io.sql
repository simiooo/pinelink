-- +goose Up
ALTER TABLE file_versions ADD COLUMN storage_generation UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE file_versions DROP CONSTRAINT file_versions_scope_id_encryption_domain_id_hash_version_de_key;
CREATE UNIQUE INDEX file_versions_live_dedup ON file_versions(scope_id,encryption_domain_id,hash_version,dedup_key) WHERE state<>'COLLECTED';

ALTER TABLE transfer_requests ADD COLUMN revocation_epoch BIGINT NOT NULL DEFAULT 0;
ALTER TABLE peer_signals ADD COLUMN reply_to BIGINT NOT NULL DEFAULT 0;
ALTER TABLE relay_offer_claims ADD COLUMN completed_at TIMESTAMPTZ;
ALTER TABLE upload_sessions ADD COLUMN transfer_id UUID REFERENCES transfer_requests(id);
ALTER TABLE upload_sessions ADD COLUMN storage_generation UUID;
UPDATE upload_sessions u SET storage_generation=v.storage_generation FROM file_versions v WHERE v.id=u.version_id;
ALTER TABLE upload_sessions ALTER COLUMN storage_generation SET NOT NULL;
ALTER TABLE upload_sessions DROP CONSTRAINT upload_sessions_state_check;
ALTER TABLE upload_sessions ADD CONSTRAINT upload_sessions_state_check CHECK(state IN ('OPEN','SEALED','CANCELLED','EXPIRED','FAILED'));
ALTER TABLE upload_chunks ADD COLUMN storage_generation UUID;
UPDATE upload_chunks c SET storage_generation=u.storage_generation FROM upload_sessions u WHERE u.id=c.upload_id;
ALTER TABLE upload_chunks ALTER COLUMN storage_generation SET NOT NULL;

CREATE TABLE relay_spools(
    transfer_id UUID PRIMARY KEY REFERENCES transfer_requests(id),
    version_id UUID NOT NULL UNIQUE REFERENCES file_versions(id),
    upload_id UUID NOT NULL UNIQUE REFERENCES upload_sessions(id),
    entry_id UUID REFERENCES file_entries(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO relay_spools(transfer_id,version_id,upload_id,entry_id)
SELECT DISTINCT ON (tr.id) tr.id,v.id,u.id,tr.entry_id
FROM transfer_requests tr JOIN file_versions v ON v.scope_id='p2p-'||tr.id::text
JOIN upload_sessions u ON u.version_id=v.id
WHERE tr.content_root=v.content_root AND tr.size_bytes=v.size_bytes AND u.owner_id=tr.sender_owner_id
ORDER BY tr.id,u.created_at DESC;
UPDATE upload_sessions u SET transfer_id=s.transfer_id FROM relay_spools s WHERE s.upload_id=u.id;

ALTER TABLE file_entries ADD COLUMN reference_kind TEXT NOT NULL DEFAULT 'LIBRARY' CHECK(reference_kind IN ('LIBRARY','DELIVERY','PERSISTENT_KEEP'));
ALTER TABLE file_entries ADD COLUMN transfer_id UUID REFERENCES transfer_requests(id);
ALTER TABLE file_entries ADD COLUMN source_entry_id UUID REFERENCES file_entries(id);
CREATE UNIQUE INDEX file_entries_private_keep ON file_entries(source_entry_id,owner_id,device_id) WHERE reference_kind='PERSISTENT_KEEP';
UPDATE file_entries e SET reference_kind=CASE WHEN e.retention_class='PERSISTENT' THEN 'PERSISTENT_KEEP' ELSE 'DELIVERY' END,
    transfer_id=u.transfer_id FROM upload_sessions u WHERE e.upload_id=u.id AND u.transfer_id IS NOT NULL;

CREATE TABLE object_references(
    source_key TEXT PRIMARY KEY,
    version_id UUID NOT NULL REFERENCES file_versions(id),
    storage_generation UUID NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('UPLOAD','LIBRARY','DELIVERY','PERSISTENT_KEEP')),
    expires_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX object_references_live ON object_references(version_id,storage_generation) WHERE released_at IS NULL;
INSERT INTO object_references(source_key,version_id,storage_generation,kind,expires_at,released_at)
SELECT 'entry:'||e.id,e.version_id,v.storage_generation,e.reference_kind,e.expires_at,
    CASE WHEN e.state='AVAILABLE' THEN NULL ELSE e.updated_at END FROM file_entries e JOIN file_versions v ON v.id=e.version_id;
INSERT INTO object_references(source_key,version_id,storage_generation,kind,expires_at,released_at)
SELECT 'upload:'||u.id,u.version_id,u.storage_generation,'UPLOAD',
    CASE WHEN tr.state IN ('PENDING','ACCEPTED') THEN LEAST(u.last_activity_at+interval '24 hours',tr.expires_at) ELSE u.last_activity_at+interval '24 hours' END,
    CASE WHEN u.state='OPEN' THEN NULL ELSE u.last_activity_at END
FROM upload_sessions u LEFT JOIN transfer_requests tr ON tr.id=u.transfer_id;

CREATE TABLE object_io_operations(
    id UUID PRIMARY KEY,
    version_id UUID NOT NULL REFERENCES file_versions(id),
    storage_generation UUID NOT NULL,
    upload_id UUID REFERENCES upload_sessions(id),
    transfer_id UUID REFERENCES transfer_requests(id),
    transfer_epoch BIGINT NOT NULL DEFAULT 0,
    kind TEXT NOT NULL CHECK(kind IN ('WRITE','READ','VERIFY')),
    object_key TEXT,
    state TEXT NOT NULL DEFAULT 'OPEN' CHECK(state IN ('OPEN','COMMITTED','ORPHAN','CLOSED')),
    expires_at TIMESTAMPTZ NOT NULL,
    next_sweep_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sweep_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX object_io_active ON object_io_operations(version_id,storage_generation,expires_at) WHERE state='OPEN';
CREATE INDEX object_io_orphans ON object_io_operations(next_sweep_at) WHERE kind='WRITE';

CREATE TABLE object_gc_jobs(
    token UUID PRIMARY KEY,
    version_id UUID NOT NULL REFERENCES file_versions(id),
    storage_generation UUID NOT NULL,
    object_keys JSONB NOT NULL,
    scope_id TEXT NOT NULL,
    reserved_bytes BIGINT NOT NULL,
    claim_token UUID,
    claim_until TIMESTAMPTZ,
    retry_after TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    UNIQUE(version_id,storage_generation)
);

CREATE TABLE file_access_grants(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id UUID NOT NULL REFERENCES transfer_requests(id),
    entry_id UUID REFERENCES file_entries(id),
    owner_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    purpose TEXT NOT NULL CHECK(purpose IN ('DELIVERY','RESTORE')),
    issued_epoch BIGINT NOT NULL,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    UNIQUE(transfer_id,purpose,owner_id,device_id)
);
INSERT INTO file_access_grants(transfer_id,entry_id,owner_id,device_id,purpose,issued_epoch,expires_at)
SELECT id,entry_id,recipient_owner_id,recipient_device_id,'DELIVERY',revocation_epoch,expires_at FROM transfer_requests WHERE state='ACCEPTED';
-- Restore is derived from an actual prior receipt, never from COMPLETED alone.
INSERT INTO file_access_grants(transfer_id,entry_id,owner_id,device_id,purpose,issued_epoch,expires_at)
SELECT tr.id,e.id,tr.recipient_owner_id,tr.recipient_device_id,'RESTORE',tr.revocation_epoch,e.expires_at
FROM transfer_requests tr JOIN file_entries e ON e.id=tr.entry_id JOIN file_versions v ON v.id=e.version_id
WHERE e.state='AVAILABLE' AND v.state='SEALED' AND (e.expires_at IS NULL OR e.expires_at>now())
AND ((tr.state='COMPLETED' AND tr.content_root IS NOT NULL AND tr.received_bytes=tr.size_bytes
    AND EXISTS(SELECT 1 FROM tasks t WHERE t.reference_id=tr.id AND t.kind='DOWNLOAD' AND t.state='COMPLETED'))
    OR EXISTS(SELECT 1 FROM delivery_receipts d WHERE d.entry_id=e.id AND d.device_id=tr.recipient_device_id))
ON CONFLICT DO NOTHING;

-- +goose StatementBegin
CREATE FUNCTION sync_upload_reference() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE deadline TIMESTAMPTZ; trstate TEXT; trdeadline TIMESTAMPTZ;
BEGIN
    deadline:=NEW.last_activity_at+interval '24 hours';
    IF NEW.transfer_id IS NOT NULL THEN
        SELECT state,expires_at INTO trstate,trdeadline FROM transfer_requests WHERE id=NEW.transfer_id;
        IF trstate IN ('PENDING','ACCEPTED') THEN deadline:=LEAST(deadline,trdeadline); END IF;
    END IF;
    INSERT INTO object_references(source_key,version_id,storage_generation,kind,expires_at,released_at)
    VALUES('upload:'||NEW.id,NEW.version_id,NEW.storage_generation,'UPLOAD',deadline,CASE WHEN NEW.state='OPEN' THEN NULL ELSE now() END)
    ON CONFLICT(source_key) DO UPDATE SET expires_at=EXCLUDED.expires_at,
        released_at=COALESCE(object_references.released_at,EXCLUDED.released_at);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER upload_reference AFTER INSERT OR UPDATE OF state,last_activity_at ON upload_sessions FOR EACH ROW EXECUTE FUNCTION sync_upload_reference();

-- +goose StatementBegin
CREATE FUNCTION guard_entry_reference() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE vstate TEXT; generation UUID; upload_generation UUID; delivery UUID;
BEGIN
    IF NEW.state='AVAILABLE' THEN
        SELECT state,storage_generation INTO vstate,generation FROM file_versions WHERE id=NEW.version_id FOR UPDATE;
        IF vstate<>'SEALED' THEN RAISE EXCEPTION 'object generation unavailable' USING ERRCODE='55000'; END IF;
        IF NEW.upload_id IS NOT NULL THEN
            SELECT storage_generation,transfer_id INTO upload_generation,delivery FROM upload_sessions WHERE id=NEW.upload_id;
            IF upload_generation<>generation THEN RAISE EXCEPTION 'stale upload generation' USING ERRCODE='55000'; END IF;
            IF delivery IS NOT NULL AND NEW.reference_kind<>'PERSISTENT_KEEP' THEN NEW.reference_kind:='DELIVERY'; NEW.transfer_id:=delivery; END IF;
        END IF;
        IF EXISTS(SELECT 1 FROM object_references WHERE source_key='entry:'||NEW.id AND released_at IS NOT NULL) THEN
            RAISE EXCEPTION 'released reference cannot be resurrected' USING ERRCODE='55000';
        END IF;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER entry_reference_guard BEFORE INSERT OR UPDATE ON file_entries FOR EACH ROW EXECUTE FUNCTION guard_entry_reference();

-- +goose StatementBegin
CREATE FUNCTION sync_entry_reference() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE generation UUID;
BEGIN
    SELECT storage_generation INTO generation FROM file_versions WHERE id=NEW.version_id;
    INSERT INTO object_references(source_key,version_id,storage_generation,kind,expires_at,released_at)
    VALUES('entry:'||NEW.id,NEW.version_id,generation,NEW.reference_kind,NEW.expires_at,CASE WHEN NEW.state='AVAILABLE' THEN NULL ELSE now() END)
    ON CONFLICT(source_key) DO UPDATE SET kind=EXCLUDED.kind,expires_at=EXCLUDED.expires_at,
        released_at=COALESCE(object_references.released_at,EXCLUDED.released_at);
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER entry_reference AFTER INSERT OR UPDATE ON file_entries FOR EACH ROW EXECUTE FUNCTION sync_entry_reference();

-- Reconcile legacy terminal spools at the migration boundary. Private keeps remain live.
UPDATE transfer_requests SET revocation_epoch=revocation_epoch+1 WHERE state IN ('REJECTED','CANCELLED','EXPIRED','FAILED');
UPDATE file_access_grants g SET revoked_at=now() FROM transfer_requests tr WHERE tr.id=g.transfer_id AND tr.state IN ('REJECTED','CANCELLED','EXPIRED','FAILED');
UPDATE upload_sessions u SET state=CASE WHEN tr.state='EXPIRED' THEN 'EXPIRED' ELSE 'CANCELLED' END
FROM transfer_requests tr WHERE tr.id=u.transfer_id AND tr.state IN ('REJECTED','CANCELLED','EXPIRED','FAILED') AND u.state IN ('OPEN','SEALED');
UPDATE file_entries e SET state=CASE WHEN tr.state='EXPIRED' THEN 'EXPIRED' ELSE 'DELETED' END,version=e.version+1,updated_at=now()
FROM transfer_requests tr WHERE tr.id=e.transfer_id AND e.reference_kind='DELIVERY' AND e.state='AVAILABLE' AND tr.state IN ('REJECTED','CANCELLED','EXPIRED','FAILED');
UPDATE file_versions v SET gc_after=now() FROM relay_spools s JOIN transfer_requests tr ON tr.id=s.transfer_id
WHERE v.id=s.version_id AND tr.state IN ('REJECTED','CANCELLED','EXPIRED','FAILED');
