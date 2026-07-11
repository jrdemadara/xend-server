-- +goose Up
CREATE TABLE relationship_space_challenge_submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_id UUID NOT NULL REFERENCES relationship_space_challenges(id) ON DELETE CASCADE,
    submitted_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    text_response TEXT,
    image_path TEXT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_challenge_submissions_unique_submission UNIQUE (challenge_id, submitted_by_user_id)
);

CREATE INDEX idx_relationship_space_challenge_submissions_challenge
    ON relationship_space_challenge_submissions(challenge_id, submitted_at);

CREATE INDEX idx_relationship_space_challenge_submissions_user
    ON relationship_space_challenge_submissions(submitted_by_user_id, submitted_at);

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_space_challenge_submissions_user;
DROP INDEX IF EXISTS idx_relationship_space_challenge_submissions_challenge;
DROP TABLE IF EXISTS relationship_space_challenge_submissions;
