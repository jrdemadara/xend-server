-- +goose Up
CREATE TABLE relationship_levels (
    level SMALLINT PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_levels_level_positive_chk CHECK (level > 0)
);

INSERT INTO relationship_levels (level, name, description) VALUES
    (1, 'Tease', 'Playful beginnings and light intimacy'),
    (2, 'Flirt', 'Consistent connection and romantic momentum'),
    (3, 'Desire', 'Strong attraction with deeper emotional sharing'),
    (4, 'Obsession', 'Intense bond with advanced intimate rituals'),
    (5, 'Afterglow', 'Peak relationship depth and long-term devotion')
ON CONFLICT (level) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS relationship_levels;
