-- +goose Up
CREATE TABLE relay_offer_claims (
    sequence BIGINT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
