-- +goose Up
CREATE TABLE message_reactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    emoji VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_message_reactions_active
    ON message_reactions(message_id, user_id, emoji)
    WHERE removed_at IS NULL;

CREATE INDEX idx_message_reactions_message_id ON message_reactions(message_id);
CREATE INDEX idx_message_reactions_user_id ON message_reactions(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_message_reactions_user_id;
DROP INDEX IF EXISTS idx_message_reactions_message_id;
DROP INDEX IF EXISTS uq_message_reactions_active;
DROP TABLE IF EXISTS message_reactions;
