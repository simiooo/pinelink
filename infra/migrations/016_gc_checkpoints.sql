-- +goose Up
ALTER TABLE object_gc_objects ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX object_gc_objects_pending ON object_gc_objects(token,object_key) WHERE deleted_at IS NULL;
