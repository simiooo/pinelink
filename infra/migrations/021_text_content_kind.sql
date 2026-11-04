-- +goose Up
-- A NULL content kind denotes the pre-existing file protocol.  The text
-- marker is deliberately separate from the encrypted sensitivity metadata.
ALTER TABLE transfer_requests ADD COLUMN content_kind text;
ALTER TABLE transfer_requests ADD CONSTRAINT transfer_content_kind_check
  CHECK (content_kind IS NULL OR content_kind = 'TEXT');

-- +goose StatementBegin
CREATE FUNCTION enforce_text_consent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.content_kind = 'TEXT' AND NEW.state = 'ACCEPTED' AND OLD.state <> 'ACCEPTED'
     AND COALESCE(current_setting('pinelink.text_consent', true), '') <> '1' THEN
    RAISE EXCEPTION 'typed text consent required' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER transfer_text_consent BEFORE UPDATE OF state ON transfer_requests
  FOR EACH ROW EXECUTE FUNCTION enforce_text_consent();

-- +goose Down
DROP TRIGGER transfer_text_consent ON transfer_requests;
DROP FUNCTION enforce_text_consent();
ALTER TABLE transfer_requests DROP CONSTRAINT transfer_content_kind_check;
ALTER TABLE transfer_requests DROP COLUMN content_kind;
