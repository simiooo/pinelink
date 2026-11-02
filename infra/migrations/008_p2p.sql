-- +goose Up
ALTER TABLE transfer_requests ADD COLUMN file_name TEXT;
ALTER TABLE transfer_requests ADD COLUMN size_bytes BIGINT;
ALTER TABLE transfer_requests ADD COLUMN content_root TEXT;
ALTER TABLE transfer_requests ADD COLUMN encryption_suite TEXT;
ALTER TABLE transfer_requests ADD COLUMN encrypted_manifest BYTEA;
ALTER TABLE transfer_requests ADD COLUMN received_bytes BIGINT NOT NULL DEFAULT 0;

CREATE TABLE peer_signals (
    sequence BIGSERIAL PRIMARY KEY,
    transfer_id UUID NOT NULL REFERENCES transfer_requests(id),
    sender_role TEXT NOT NULL CHECK (sender_role IN ('SENDER', 'RECEIVER', 'RELAY')),
    channel TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX peer_signals_transfer_sequence_idx ON peer_signals(transfer_id, sequence);
