-- +goose Up
CREATE TABLE relationship_spaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(120),
    created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    current_level SMALLINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

CREATE INDEX idx_relationship_spaces_created_by ON relationship_spaces(created_by_user_id);
CREATE INDEX idx_relationship_spaces_current_level ON relationship_spaces(current_level);

CREATE TRIGGER relationship_spaces_set_updated_at
BEFORE UPDATE ON relationship_spaces
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS relationship_spaces_set_updated_at ON relationship_spaces;
DROP INDEX IF EXISTS idx_relationship_spaces_current_level;
DROP INDEX IF EXISTS idx_relationship_spaces_created_by;
DROP TABLE IF EXISTS relationship_spaces;
