-- +goose Up
CREATE TABLE relationship_space_daily_ritual_completions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id UUID NOT NULL REFERENCES relationship_space_daily_ritual_assignments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    text_response TEXT,
    image_path TEXT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_daily_ritual_completions_unique_submission UNIQUE (assignment_id, user_id)
);

CREATE INDEX idx_relationship_space_daily_ritual_completions_assignment
    ON relationship_space_daily_ritual_completions(assignment_id, submitted_at);

CREATE INDEX idx_relationship_space_daily_ritual_completions_user
    ON relationship_space_daily_ritual_completions(user_id, submitted_at);

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_completions_user;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_completions_assignment;
DROP TABLE IF EXISTS relationship_space_daily_ritual_completions;
