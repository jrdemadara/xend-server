-- +goose Up
CREATE TABLE relationship_space_daily_ritual_selections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID NOT NULL REFERENCES relationship_spaces(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES daily_ritual_templates(id) ON DELETE RESTRICT,
    selected_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    sort_order SMALLINT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    selected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_space_daily_ritual_selections_space_template_unique UNIQUE (relationship_space_id, template_id),
    CONSTRAINT relationship_space_daily_ritual_selections_sort_order_chk CHECK (sort_order >= 0)
);

CREATE INDEX idx_relationship_space_daily_ritual_selections_space_active
    ON relationship_space_daily_ritual_selections(relationship_space_id, is_active, sort_order, selected_at);

CREATE TRIGGER relationship_space_daily_ritual_selections_set_updated_at
BEFORE UPDATE ON relationship_space_daily_ritual_selections
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS relationship_space_daily_ritual_selections_set_updated_at ON relationship_space_daily_ritual_selections;
DROP INDEX IF EXISTS idx_relationship_space_daily_ritual_selections_space_active;
DROP TABLE IF EXISTS relationship_space_daily_ritual_selections;
