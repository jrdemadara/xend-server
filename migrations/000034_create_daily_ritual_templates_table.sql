-- +goose Up
CREATE TABLE daily_ritual_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(64) NOT NULL,
    title VARCHAR(120) NOT NULL,
    description TEXT NOT NULL,
    category VARCHAR(32) NOT NULL,
    icon_key VARCHAR(32) NOT NULL,
    suggested_time VARCHAR(32),
    default_points INTEGER NOT NULL DEFAULT 5,
    display_order SMALLINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT daily_ritual_templates_slug_unique UNIQUE (slug),
    CONSTRAINT daily_ritual_templates_default_points_chk CHECK (default_points >= 0),
    CONSTRAINT daily_ritual_templates_display_order_chk CHECK (display_order >= 0)
);

CREATE INDEX idx_daily_ritual_templates_active_display
    ON daily_ritual_templates(is_active, display_order, title);

CREATE TRIGGER daily_ritual_templates_set_updated_at
BEFORE UPDATE ON daily_ritual_templates
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

INSERT INTO daily_ritual_templates (
    slug,
    title,
    description,
    category,
    icon_key,
    suggested_time,
    default_points,
    display_order
)
VALUES
    ('good-morning-message', 'Good morning message', 'Send a sweet good morning message to start the day.', 'connection', 'sun', 'morning', 5, 1),
    ('midday-check-in', 'Check in', 'Ask how each other is feeling today.', 'care', 'chat', 'afternoon', 5, 2),
    ('gratitude-moment', 'Gratitude moment', 'Share one thing you appreciate about your partner today.', 'reflection', 'sparkles', 'evening', 5, 3),
    ('share-a-photo', 'Share a photo', 'Send a photo that captures a little part of your day.', 'memory', 'camera', 'anytime', 5, 4),
    ('good-night-message', 'Good night message', 'End the day with a gentle good night message.', 'connection', 'moon', 'night', 5, 5);

-- +goose Down
DROP TRIGGER IF EXISTS daily_ritual_templates_set_updated_at ON daily_ritual_templates;
DROP INDEX IF EXISTS idx_daily_ritual_templates_active_display;
DROP TABLE IF EXISTS daily_ritual_templates;
