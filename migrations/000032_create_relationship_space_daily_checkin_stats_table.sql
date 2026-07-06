-- +goose Up
CREATE TABLE relationship_space_daily_checkin_stats (
    relationship_space_id UUID PRIMARY KEY REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    completed_days_count INTEGER NOT NULL DEFAULT 0,
    current_streak INTEGER NOT NULL DEFAULT 0,
    last_completed_checkin_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_daily_checkin_stats_completed_days_chk CHECK (completed_days_count >= 0),
    CONSTRAINT relationship_space_daily_checkin_stats_current_streak_chk CHECK (current_streak >= 0)
);

CREATE TRIGGER relationship_space_daily_checkin_stats_set_updated_at
BEFORE UPDATE ON relationship_space_daily_checkin_stats
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS relationship_space_daily_checkin_stats_set_updated_at ON relationship_space_daily_checkin_stats;
DROP TABLE IF EXISTS relationship_space_daily_checkin_stats;
