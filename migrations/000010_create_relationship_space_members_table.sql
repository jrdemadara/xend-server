-- +goose Up
CREATE TABLE relationship_space_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'member',
    membership_status VARCHAR(16) NOT NULL DEFAULT 'active',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at TIMESTAMPTZ,
    muted_until TIMESTAMPTZ,
    CONSTRAINT relationship_space_members_role_chk CHECK (role IN ('owner', 'member')),
    CONSTRAINT relationship_space_members_status_chk CHECK (membership_status IN ('active', 'left', 'removed')),
    CONSTRAINT relationship_space_members_unique_user UNIQUE (relationship_space_id, user_id)
);

CREATE INDEX idx_relationship_space_members_user_id ON relationship_space_members(user_id);
CREATE INDEX idx_relationship_space_members_space_id ON relationship_space_members(relationship_space_id);

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_space_members_space_id;
DROP INDEX IF EXISTS idx_relationship_space_members_user_id;
DROP TABLE IF EXISTS relationship_space_members;
