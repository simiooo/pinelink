-- +goose Up
ALTER TABLE transfer_requests ALTER COLUMN state SET DEFAULT 'PENDING';
