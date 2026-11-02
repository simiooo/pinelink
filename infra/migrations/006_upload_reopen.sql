-- +goose Up
ALTER TABLE upload_sessions DROP CONSTRAINT upload_sessions_version_id_key;
ALTER TABLE file_versions ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
