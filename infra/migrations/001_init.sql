-- +goose Up
CREATE TABLE file_versions (
    id UUID PRIMARY KEY,
    scope_id TEXT NOT NULL,
    encryption_domain_id TEXT NOT NULL,
    hash_version TEXT NOT NULL,
    dedup_key TEXT NOT NULL,
    content_root TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    chunk_size INTEGER NOT NULL CHECK (chunk_size > 0),
    chunk_count INTEGER NOT NULL CHECK (chunk_count >= 0),
    state TEXT NOT NULL CHECK (state IN ('UPLOADING', 'SEALED', 'CORRUPT', 'GC_PENDING')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sealed_at TIMESTAMPTZ,
    UNIQUE (scope_id, encryption_domain_id, hash_version, dedup_key)
);

CREATE TABLE upload_sessions (
    id UUID PRIMARY KEY,
    version_id UUID NOT NULL UNIQUE REFERENCES file_versions(id),
    owner_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    retention_class TEXT NOT NULL CHECK (retention_class IN ('TEMP_24H', 'PERSISTENT')),
    state TEXT NOT NULL CHECK (state IN ('OPEN', 'SEALED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE upload_chunks (
    upload_id UUID NOT NULL REFERENCES upload_sessions(id),
    chunk_index INTEGER NOT NULL CHECK (chunk_index >= 0),
    ciphertext_sha256 TEXT NOT NULL,
    object_key TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (upload_id, chunk_index)
);

CREATE TABLE reuse_tokens (
    token UUID PRIMARY KEY,
    version_id UUID NOT NULL REFERENCES file_versions(id),
    owner_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    retention_class TEXT NOT NULL CHECK (retention_class IN ('TEMP_24H', 'PERSISTENT')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);

CREATE TABLE file_entries (
    id UUID PRIMARY KEY,
    version_id UUID NOT NULL REFERENCES file_versions(id),
    upload_id UUID UNIQUE REFERENCES upload_sessions(id),
    reuse_token UUID UNIQUE REFERENCES reuse_tokens(token),
    owner_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    retention_class TEXT NOT NULL CHECK (retention_class IN ('TEMP_24H', 'PERSISTENT')),
    state TEXT NOT NULL CHECK (state IN ('AVAILABLE', 'EXPIRED', 'DELETED')),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX file_entries_version_idx ON file_entries(version_id);
CREATE INDEX file_entries_expiry_idx ON file_entries(expires_at) WHERE state = 'AVAILABLE' AND expires_at IS NOT NULL;

CREATE TABLE pull_sessions (
    id UUID PRIMARY KEY,
    entry_id UUID NOT NULL REFERENCES file_entries(id),
    device_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PULLING', 'COMPLETED', 'FAILED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX pull_sessions_entry_device_idx ON pull_sessions(entry_id, device_id);

CREATE TABLE delivery_receipts (
    entry_id UUID NOT NULL REFERENCES file_entries(id),
    device_id TEXT NOT NULL,
    first_delivered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (entry_id, device_id)
);

CREATE TABLE outbox_events (
    event_id UUID PRIMARY KEY,
    event_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    aggregate_version BIGINT NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    publish_attempts INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX outbox_unpublished_idx ON outbox_events(occurred_at) WHERE published_at IS NULL;
