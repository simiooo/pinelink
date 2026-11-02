-- +goose Up
ALTER TABLE transfer_requests ADD COLUMN sender_encrypted_manifest BYTEA;
