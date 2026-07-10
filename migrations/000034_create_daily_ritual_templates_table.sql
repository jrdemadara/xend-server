-- +goose Up
CREATE TABLE daily_ritual_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(64) NOT NULL,
    title VARCHAR(120) NOT NULL,
    description TEXT NOT NULL,
    category VARCHAR(32) NOT NULL,
    icon_key VARCHAR(32) NOT NULL,
    submission_type VARCHAR(16) NOT NULL,
    target_type VARCHAR(16) NOT NULL,
    completion_rule VARCHAR(20) NOT NULL,
    min_level SMALLINT NOT NULL DEFAULT 1,
    max_level SMALLINT,
    suggested_time VARCHAR(32),
    default_points INTEGER NOT NULL DEFAULT 5,
    display_order SMALLINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT daily_ritual_templates_slug_unique UNIQUE (slug),
    CONSTRAINT daily_ritual_templates_submission_type_chk CHECK (submission_type IN ('none', 'text', 'image')),
    CONSTRAINT daily_ritual_templates_target_type_chk CHECK (target_type IN ('both', 'one_partner')),
    CONSTRAINT daily_ritual_templates_completion_rule_chk CHECK (completion_rule IN ('single_actor', 'both_partners')),
    CONSTRAINT daily_ritual_templates_min_level_chk CHECK (min_level > 0),
    CONSTRAINT daily_ritual_templates_max_level_chk CHECK (max_level IS NULL OR max_level >= min_level),
    CONSTRAINT daily_ritual_templates_default_points_chk CHECK (default_points > 0),
    CONSTRAINT daily_ritual_templates_display_order_chk CHECK (display_order >= 0)
);

CREATE INDEX idx_daily_ritual_templates_active_display
    ON daily_ritual_templates(is_active, display_order, title);

CREATE TRIGGER daily_ritual_templates_set_updated_at
BEFORE UPDATE ON daily_ritual_templates
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS relationship_space_daily_ritual_completions;
DROP TRIGGER IF EXISTS relationship_space_daily_ritual_assignments_set_updated_at ON relationship_space_daily_ritual_assignments;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_assignments_target_user;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_assignments_space_date;
DROP TABLE IF EXISTS relationship_space_daily_ritual_assignments;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_selections_space_active;
DROP TABLE IF EXISTS relationship_space_daily_ritual_selections;
DROP TRIGGER IF EXISTS daily_ritual_templates_set_updated_at ON daily_ritual_templates;
DROP INDEX IF EXISTS idx_daily_ritual_templates_active_display;
DROP TABLE IF EXISTS daily_ritual_templates;
