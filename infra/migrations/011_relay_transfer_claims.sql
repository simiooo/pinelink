-- +goose Up
CREATE TABLE relay_transfer_claims (
    transfer_id UUID PRIMARY KEY,
    owner_id TEXT NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
