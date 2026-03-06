-- +goose Up
CREATE TABLE relationship_space_level_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    level SMALLINT NOT NULL REFERENCES relationship_levels(level) ON DELETE RESTRICT,
    required_points INTEGER NOT NULL,
    current_points INTEGER NOT NULL DEFAULT 0,
    unlocked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_level_progress_points_chk
        CHECK (required_points > 0 AND current_points >= 0 AND current_points <= required_points),
    CONSTRAINT relationship_space_level_progress_unique_level
        UNIQUE (relationship_space_id, level)
);

CREATE INDEX idx_relationship_space_level_progress_space_id
    ON relationship_space_level_progress(relationship_space_id);

CREATE INDEX idx_relationship_space_level_progress_space_level
    ON relationship_space_level_progress(relationship_space_id, level);

CREATE TRIGGER relationship_space_level_progress_set_updated_at
BEFORE UPDATE ON relationship_space_level_progress
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS relationship_space_level_progress_set_updated_at ON relationship_space_level_progress;
DROP INDEX IF EXISTS idx_relationship_space_level_progress_space_level;
DROP INDEX IF EXISTS idx_relationship_space_level_progress_space_id;
DROP TABLE IF EXISTS relationship_space_level_progress;
