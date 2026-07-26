-- +goose Up
ALTER TABLE relationship_spaces
    ADD COLUMN celebrate_monthsary BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN celebrate_anniversary BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE relationship_spaces
    DROP COLUMN IF EXISTS celebrate_anniversary,
    DROP COLUMN IF EXISTS celebrate_monthsary;
