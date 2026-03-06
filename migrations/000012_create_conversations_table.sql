-- +goose Up
CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL UNIQUE REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE INDEX idx_conversations_created_by ON conversations(created_by_user_id);

CREATE TRIGGER conversations_set_updated_at
BEFORE UPDATE ON conversations
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS conversations_set_updated_at ON conversations;
DROP INDEX IF EXISTS idx_conversations_created_by;
DROP TABLE IF EXISTS conversations;
