-- +goose Up
-- A NULL content kind denotes the pre-existing file protocol.  The text
-- marker is deliberately separate from the encrypted sensitivity metadata.
ALTER TABLE transfer_requests ADD COLUMN content_kind text;
ALTER TABLE transfer_requests ADD CONSTRAINT transfer_content_kind_check
  CHECK (content_kind IS NULL OR content_kind = 'TEXT');

-- +goose Down
ALTER TABLE transfer_requests DROP CONSTRAINT transfer_content_kind_check;
ALTER TABLE transfer_requests DROP COLUMN content_kind;
