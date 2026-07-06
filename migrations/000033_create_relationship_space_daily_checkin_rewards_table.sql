-- +goose Up
CREATE TABLE relationship_space_daily_checkin_rewards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    checkin_date DATE NOT NULL,
    reward_type VARCHAR(16) NOT NULL,
    milestone_id UUID REFERENCES daily_checkin_milestones(id) ON DELETE RESTRICT,
    points INTEGER NOT NULL,
    awarded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_daily_checkin_rewards_reward_type_chk
        CHECK (reward_type IN ('daily', 'milestone')),
    CONSTRAINT relationship_space_daily_checkin_rewards_points_chk
        CHECK (points > 0),
    CONSTRAINT relationship_space_daily_checkin_rewards_milestone_chk
        CHECK (
            (reward_type = 'daily' AND milestone_id IS NULL) OR
            (reward_type = 'milestone' AND milestone_id IS NOT NULL)
        )
);

CREATE INDEX idx_relationship_space_daily_checkin_rewards_space_date
    ON relationship_space_daily_checkin_rewards(relationship_space_id, checkin_date);

CREATE UNIQUE INDEX idx_relationship_space_daily_checkin_rewards_daily_unique
    ON relationship_space_daily_checkin_rewards(relationship_space_id, checkin_date)
    WHERE reward_type = 'daily';

CREATE UNIQUE INDEX idx_relationship_space_daily_checkin_rewards_milestone_unique
    ON relationship_space_daily_checkin_rewards(relationship_space_id, milestone_id)
    WHERE reward_type = 'milestone';

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_space_daily_checkin_rewards_milestone_unique;
DROP INDEX IF EXISTS idx_relationship_space_daily_checkin_rewards_daily_unique;
DROP INDEX IF EXISTS idx_relationship_space_daily_checkin_rewards_space_date;
DROP TABLE IF EXISTS relationship_space_daily_checkin_rewards;
