-- +goose Up
ALTER TABLE relationship_spaces
    ADD COLUMN cover_photo_path TEXT,
    ADD COLUMN couple_photo_path TEXT;

ALTER TABLE relationship_spaces
    ADD CONSTRAINT relationship_spaces_cover_photo_path_chk
        CHECK (cover_photo_path IS NULL OR char_length(cover_photo_path) BETWEEN 1 AND 512),
    ADD CONSTRAINT relationship_spaces_couple_photo_path_chk
        CHECK (couple_photo_path IS NULL OR char_length(couple_photo_path) BETWEEN 1 AND 512);

-- +goose Down
ALTER TABLE relationship_spaces
    DROP CONSTRAINT IF EXISTS relationship_spaces_couple_photo_path_chk,
    DROP CONSTRAINT IF EXISTS relationship_spaces_cover_photo_path_chk;

ALTER TABLE relationship_spaces
    DROP COLUMN IF EXISTS couple_photo_path,
    DROP COLUMN IF EXISTS cover_photo_path;
