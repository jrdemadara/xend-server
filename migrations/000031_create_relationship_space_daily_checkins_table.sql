-- +goose Up
CREATE TABLE relationship_space_daily_checkins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    checkin_date DATE NOT NULL,
    timezone_name VARCHAR(64) NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_daily_checkins_timezone_name_chk CHECK (char_length(timezone_name) > 0),
    CONSTRAINT relationship_space_daily_checkins_unique_submission UNIQUE (relationship_space_id, user_id, checkin_date)
);

CREATE INDEX idx_relationship_space_daily_checkins_space_date
    ON relationship_space_daily_checkins(relationship_space_id, checkin_date);

CREATE INDEX idx_relationship_space_daily_checkins_user_date
    ON relationship_space_daily_checkins(user_id, checkin_date);

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_space_daily_checkins_user_date;
DROP INDEX IF EXISTS idx_relationship_space_daily_checkins_space_date;
DROP TABLE IF EXISTS relationship_space_daily_checkins;
