-- +goose Up
CREATE TABLE daily_checkin_milestones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    completed_days INTEGER NOT NULL,
    bonus_points INTEGER NOT NULL,
    title VARCHAR(80),
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT daily_checkin_milestones_completed_days_chk CHECK (completed_days > 0),
    CONSTRAINT daily_checkin_milestones_bonus_points_chk CHECK (bonus_points > 0),
    CONSTRAINT daily_checkin_milestones_completed_days_unique UNIQUE (completed_days)
);

CREATE INDEX idx_daily_checkin_milestones_active_days
    ON daily_checkin_milestones(is_active, completed_days);

CREATE TRIGGER daily_checkin_milestones_set_updated_at
BEFORE UPDATE ON daily_checkin_milestones
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

INSERT INTO daily_checkin_milestones (completed_days, bonus_points, title, description)
VALUES
    (3, 10, 'Warm Start', 'Three completed couple check-ins together.'),
    (7, 25, 'One Week Bond', 'Seven completed couple check-ins together.'),
    (14, 50, 'Two Week Bond', 'Fourteen completed couple check-ins together.'),
    (30, 100, 'Thirty Day Bond', 'Thirty completed couple check-ins together.'),
    (60, 200, 'Sixty Day Bond', 'Sixty completed couple check-ins together.'),
    (100, 350, 'Hundred Day Bond', 'One hundred completed couple check-ins together.')
ON CONFLICT (completed_days) DO NOTHING;

-- +goose Down
DROP TRIGGER IF EXISTS daily_checkin_milestones_set_updated_at ON daily_checkin_milestones;
DROP INDEX IF EXISTS idx_daily_checkin_milestones_active_days;
DROP TABLE IF EXISTS daily_checkin_milestones;
