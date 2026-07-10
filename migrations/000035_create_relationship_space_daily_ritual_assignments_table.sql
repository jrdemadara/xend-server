-- +goose Up
CREATE TABLE relationship_space_daily_ritual_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES daily_ritual_templates(id) ON DELETE RESTRICT,
    ritual_date DATE NOT NULL,
    timezone_name VARCHAR(64) NOT NULL,
    assigned_level SMALLINT NOT NULL,
    target_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    reward_points INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'assigned',
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    reward_awarded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_daily_ritual_assignments_space_date_unique UNIQUE (relationship_space_id, ritual_date),
    CONSTRAINT relationship_space_daily_ritual_assignments_timezone_name_chk CHECK (char_length(timezone_name) > 0),
    CONSTRAINT relationship_space_daily_ritual_assignments_assigned_level_chk CHECK (assigned_level > 0),
    CONSTRAINT relationship_space_daily_ritual_assignments_reward_points_chk CHECK (reward_points > 0),
    CONSTRAINT relationship_space_daily_ritual_assignments_status_chk CHECK (status IN ('assigned', 'completed', 'expired'))
);

CREATE INDEX idx_relationship_space_daily_ritual_assignments_space_date
    ON relationship_space_daily_ritual_assignments(relationship_space_id, ritual_date);

CREATE INDEX idx_relationship_space_daily_ritual_assignments_target_user
    ON relationship_space_daily_ritual_assignments(target_user_id, ritual_date);

CREATE TRIGGER relationship_space_daily_ritual_assignments_set_updated_at
BEFORE UPDATE ON relationship_space_daily_ritual_assignments
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS relationship_space_daily_ritual_assignments_set_updated_at ON relationship_space_daily_ritual_assignments;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_assignments_target_user;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_assignments_space_date;
DROP TABLE IF EXISTS relationship_space_daily_ritual_assignments;
