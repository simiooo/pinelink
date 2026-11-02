-- +goose Up
ALTER TABLE file_versions DROP CONSTRAINT file_versions_state_check;
ALTER TABLE file_versions ADD CONSTRAINT file_versions_state_check
    CHECK (state IN ('UPLOADING', 'SEALED', 'CORRUPT', 'GC_PENDING', 'COLLECTED'));

CREATE TABLE devices (
    owner_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    public_key TEXT,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'REVOKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY (owner_id, device_id)
);
