-- +goose Up
ALTER TABLE transfer_requests
    ADD COLUMN progress_revision BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN progress_sampled_at TIMESTAMPTZ,
    ADD COLUMN active_route TEXT NOT NULL DEFAULT 'UNKNOWN'
        CHECK (active_route IN ('UNKNOWN', 'DIRECT', 'RELAY')),
    ADD COLUMN route_epoch BIGINT NOT NULL DEFAULT 0 CHECK (route_epoch >= 0);

ALTER TABLE tasks
    ADD COLUMN progress_revision BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN progress_sampled_at TIMESTAMPTZ;

-- Existing byte counts are preserved. An old updated_at is not a byte observation.
-- P2P's recipient checkpoint was authoritative before tasks were kept in sync.
UPDATE tasks t SET progress_bytes=tr.received_bytes
FROM transfer_requests tr
WHERE t.kind='DOWNLOAD' AND t.reference_id=tr.id AND tr.content_root IS NOT NULL
    AND tr.state='ACCEPTED' AND t.state IN ('RUNNING','PAUSED','QUEUED')
    AND tr.received_bytes BETWEEN 0 AND t.total_bytes;
