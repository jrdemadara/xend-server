-- +goose Up
CREATE TABLE relationship_space_daily_ritual_completions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES daily_ritual_templates(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ritual_date DATE NOT NULL,
    timezone_name VARCHAR(64) NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_daily_ritual_completions_timezone_name_chk CHECK (char_length(timezone_name) > 0),
    CONSTRAINT relationship_space_daily_ritual_completions_unique_submission UNIQUE (relationship_space_id, template_id, user_id, ritual_date)
);

CREATE INDEX idx_relationship_space_daily_ritual_completions_space_date
    ON relationship_space_daily_ritual_completions(relationship_space_id, ritual_date);

CREATE INDEX idx_relationship_space_daily_ritual_completions_user_date
    ON relationship_space_daily_ritual_completions(user_id, ritual_date);

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_completions_user_date;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_completions_space_date;
DROP TABLE IF EXISTS relationship_space_daily_ritual_completions;
