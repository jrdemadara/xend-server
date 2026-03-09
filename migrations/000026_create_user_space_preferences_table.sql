-- +goose Up
CREATE TABLE user_space_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_relationship_space_id UUID REFERENCES relationship_spaces(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_space_preferences_default_space ON user_space_preferences(default_relationship_space_id);

CREATE TRIGGER user_space_preferences_set_updated_at
BEFORE UPDATE ON user_space_preferences
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS user_space_preferences_set_updated_at ON user_space_preferences;
DROP INDEX IF EXISTS idx_user_space_preferences_default_space;
DROP TABLE IF EXISTS user_space_preferences;
