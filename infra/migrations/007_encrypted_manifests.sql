-- +goose Up
ALTER TABLE file_versions ADD COLUMN encryption_suite TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN signing_public_key TEXT;

CREATE TABLE manifest_envelopes (
    version_id UUID NOT NULL REFERENCES file_versions(id),
    owner_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    ciphertext BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (version_id, owner_id, device_id)
);
