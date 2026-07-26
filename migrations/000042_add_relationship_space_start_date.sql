-- +goose Up
ALTER TABLE relationship_spaces
    ADD COLUMN relationship_start_date DATE;

UPDATE relationship_spaces
SET relationship_start_date = created_at::date
WHERE relationship_start_date IS NULL;

ALTER TABLE relationship_spaces
    ALTER COLUMN relationship_start_date SET NOT NULL,
    ALTER COLUMN relationship_start_date SET DEFAULT CURRENT_DATE,
    ADD CONSTRAINT relationship_spaces_start_date_chk
        CHECK (relationship_start_date >= DATE '1900-01-01');

-- +goose Down
ALTER TABLE relationship_spaces
    DROP CONSTRAINT IF EXISTS relationship_spaces_start_date_chk;

ALTER TABLE relationship_spaces
    DROP COLUMN IF EXISTS relationship_start_date;
