-- +goose Up
CREATE TABLE relationship_space_member_access (
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_passphrase_hash TEXT,
    access_hint VARCHAR(120),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (relationship_space_id, user_id)
);

CREATE INDEX idx_relationship_space_member_access_user_id ON relationship_space_member_access(user_id);

CREATE TRIGGER relationship_space_member_access_set_updated_at
BEFORE UPDATE ON relationship_space_member_access
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS relationship_space_member_access_set_updated_at ON relationship_space_member_access;
DROP INDEX IF EXISTS idx_relationship_space_member_access_user_id;
DROP TABLE IF EXISTS relationship_space_member_access;
