-- +goose Up
CREATE TABLE relationship_moods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mood_key TEXT NOT NULL,
    emoji TEXT NOT NULL,
    label TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_moods_mood_key_chk CHECK (char_length(mood_key) BETWEEN 1 AND 64),
    CONSTRAINT relationship_moods_emoji_chk CHECK (char_length(emoji) BETWEEN 1 AND 16),
    CONSTRAINT relationship_moods_label_chk CHECK (char_length(label) BETWEEN 1 AND 64)
);

CREATE INDEX idx_relationship_moods_space_user_created
    ON relationship_moods(relationship_space_id, user_id, created_at DESC);

CREATE INDEX idx_relationship_moods_space_created
    ON relationship_moods(relationship_space_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_moods_space_created;
DROP INDEX IF EXISTS idx_relationship_moods_space_user_created;
DROP TABLE IF EXISTS relationship_moods;
