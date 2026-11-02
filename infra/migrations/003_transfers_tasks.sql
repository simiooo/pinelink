-- +goose Up
ALTER TABLE file_entries ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE file_entries ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE file_versions ADD COLUMN persistent_hold INTEGER NOT NULL DEFAULT 0;
ALTER TABLE file_versions ADD COLUMN gc_after TIMESTAMPTZ;

ALTER TABLE outbox_events ADD COLUMN event_sequence BIGSERIAL;
ALTER TABLE outbox_events ADD CONSTRAINT outbox_event_sequence_unique UNIQUE (event_sequence);

CREATE TABLE transfer_requests (
    id UUID PRIMARY KEY,
    entry_id UUID REFERENCES file_entries(id),
    delivery_mode TEXT NOT NULL CHECK (delivery_mode IN ('ONLINE', 'ASYNC', 'BROADCAST')),
    sender_owner_id TEXT NOT NULL,
    sender_device_id TEXT NOT NULL,
    recipient_owner_id TEXT NOT NULL,
    recipient_device_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('PENDING', 'ACCEPTED', 'REJECTED', 'CANCELLED', 'EXPIRED', 'COMPLETED', 'FAILED')),
    version BIGINT NOT NULL DEFAULT 1,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX transfer_requests_recipient_idx ON transfer_requests(recipient_owner_id, recipient_device_id, state, created_at);
CREATE INDEX transfer_requests_sender_idx ON transfer_requests(sender_owner_id, created_at);

ALTER TABLE pull_sessions ADD COLUMN transfer_id UUID REFERENCES transfer_requests(id);
ALTER TABLE pull_sessions ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE pull_sessions ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE pull_sessions ADD COLUMN error_code TEXT;

CREATE TABLE tasks (
    id UUID PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('UPLOAD', 'DOWNLOAD')),
    reference_id UUID NOT NULL,
    owner_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('QUEUED', 'RUNNING', 'PAUSED', 'CANCELLED', 'COMPLETED', 'FAILED', 'DELETED')),
    progress_bytes BIGINT NOT NULL DEFAULT 0 CHECK (progress_bytes >= 0),
    total_bytes BIGINT NOT NULL DEFAULT 0 CHECK (total_bytes >= 0),
    version BIGINT NOT NULL DEFAULT 1,
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE (kind, reference_id)
);

CREATE INDEX tasks_owner_history_idx ON tasks(owner_id, device_id, updated_at DESC);
CREATE INDEX tasks_state_idx ON tasks(state) WHERE state NOT IN ('COMPLETED', 'DELETED');

CREATE TABLE event_inbox (
    consumer_name TEXT NOT NULL,
    event_id UUID NOT NULL,
    aggregate_version BIGINT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_name, event_id)
);

CREATE TABLE scope_quotas (
    scope_id TEXT PRIMARY KEY,
    quota_bytes BIGINT NOT NULL CHECK (quota_bytes >= 0),
    used_bytes BIGINT NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
    version BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
