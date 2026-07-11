-- +goose Up
CREATE TABLE challenge_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(64) NOT NULL,
    title VARCHAR(120) NOT NULL,
    description TEXT NOT NULL,
    category VARCHAR(32) NOT NULL,
    icon_key VARCHAR(32) NOT NULL,
    submission_type VARCHAR(16) NOT NULL,
    min_level SMALLINT NOT NULL DEFAULT 1,
    max_level SMALLINT,
    default_points INTEGER NOT NULL DEFAULT 10,
    expiry_hours INTEGER,
    display_order SMALLINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT challenge_templates_slug_unique UNIQUE (slug),
    CONSTRAINT challenge_templates_submission_type_chk CHECK (submission_type IN ('none', 'text', 'image')),
    CONSTRAINT challenge_templates_min_level_chk CHECK (min_level > 0),
    CONSTRAINT challenge_templates_max_level_chk CHECK (max_level IS NULL OR max_level >= min_level),
    CONSTRAINT challenge_templates_default_points_chk CHECK (default_points > 0),
    CONSTRAINT challenge_templates_expiry_hours_chk CHECK (expiry_hours IS NULL OR expiry_hours > 0),
    CONSTRAINT challenge_templates_display_order_chk CHECK (display_order >= 0)
);

CREATE INDEX idx_challenge_templates_active_display
    ON challenge_templates(is_active, min_level, display_order, title);

CREATE TRIGGER challenge_templates_set_updated_at
BEFORE UPDATE ON challenge_templates
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS challenge_templates_set_updated_at ON challenge_templates;
DROP INDEX IF EXISTS idx_challenge_templates_active_display;
DROP TABLE IF EXISTS challenge_templates;
