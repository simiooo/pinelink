-- +goose Up
CREATE TABLE object_gc_objects(
    token UUID NOT NULL REFERENCES object_gc_jobs(token),
    object_key TEXT NOT NULL,
    PRIMARY KEY(token,object_key)
);
INSERT INTO object_gc_objects(token,object_key)
SELECT token,jsonb_array_elements_text(object_keys) FROM object_gc_jobs ON CONFLICT DO NOTHING;
-- The immutable manifest is paged rather than held in one JSON value / process.
ALTER TABLE object_gc_jobs ALTER COLUMN object_keys SET DEFAULT '[]'::jsonb;
