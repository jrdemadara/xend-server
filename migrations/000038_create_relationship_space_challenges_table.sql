-- +goose Up
CREATE TABLE relationship_space_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES challenge_templates(id) ON DELETE RESTRICT,
    sender_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    receiver_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    assigned_level SMALLINT NOT NULL,
    note TEXT,
    reward_points INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'sent',
    expires_at TIMESTAMPTZ,
    accepted_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    reward_awarded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_challenges_assigned_level_chk CHECK (assigned_level > 0),
    CONSTRAINT relationship_space_challenges_reward_points_chk CHECK (reward_points > 0),
    CONSTRAINT relationship_space_challenges_status_chk CHECK (status IN ('sent', 'accepted', 'completed', 'declined', 'expired', 'cancelled')),
    CONSTRAINT relationship_space_challenges_sender_receiver_chk CHECK (sender_user_id <> receiver_user_id)
);

CREATE INDEX idx_relationship_space_challenges_space_status_created
    ON relationship_space_challenges(relationship_space_id, status, created_at DESC);

CREATE INDEX idx_relationship_space_challenges_receiver_status_created
    ON relationship_space_challenges(receiver_user_id, status, created_at DESC);

CREATE INDEX idx_relationship_space_challenges_sender_status_created
    ON relationship_space_challenges(sender_user_id, status, created_at DESC);

CREATE TRIGGER relationship_space_challenges_set_updated_at
BEFORE UPDATE ON relationship_space_challenges
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS relationship_space_challenges_set_updated_at ON relationship_space_challenges;
DROP INDEX IF EXISTS idx_relationship_space_challenges_sender_status_created;
DROP INDEX IF EXISTS idx_relationship_space_challenges_receiver_status_created;
DROP INDEX IF EXISTS idx_relationship_space_challenges_space_status_created;
DROP TABLE IF EXISTS relationship_space_challenges;
