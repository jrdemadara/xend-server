-- +goose Up
ALTER TABLE messages
ADD COLUMN reply_to_message_id UUID NULL REFERENCES messages(id) ON DELETE SET NULL;

CREATE INDEX idx_messages_reply_to_message_id ON messages(reply_to_message_id);

-- +goose Down
DROP INDEX IF EXISTS idx_messages_reply_to_message_id;
ALTER TABLE messages DROP COLUMN IF EXISTS reply_to_message_id;
