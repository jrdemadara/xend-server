-- +goose Up
CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sender_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    client_message_id VARCHAR(128) NOT NULL,
    message_type VARCHAR(32) NOT NULL,
    ciphertext TEXT NOT NULL,
    reply_to_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    sender_timestamp TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT messages_type_chk CHECK (message_type IN ('signal_whisper', 'signal_prekey', 'signal_sender_key')),
    CONSTRAINT messages_sender_device_unique_client_msg UNIQUE (sender_device_id, client_message_id)
);

CREATE INDEX idx_messages_conversation_created_at ON messages(conversation_id, created_at DESC);
CREATE INDEX idx_messages_reply_to_message_id ON messages(reply_to_message_id);
CREATE INDEX idx_messages_sender_device_created_at ON messages(sender_device_id, created_at DESC);
CREATE INDEX idx_messages_sender_user_created_at ON messages(sender_user_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_messages_sender_user_created_at;
DROP INDEX IF EXISTS idx_messages_sender_device_created_at;
DROP INDEX IF EXISTS idx_messages_reply_to_message_id;
DROP INDEX IF EXISTS idx_messages_conversation_created_at;
DROP TABLE IF EXISTS messages;
