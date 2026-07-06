-- +goose Up
CREATE TABLE relationship_levels (
    level SMALLINT PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    required_points INTEGER NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_levels_level_positive_chk CHECK (level > 0),
    CONSTRAINT relationship_levels_required_points_chk CHECK (required_points > 0)
);

INSERT INTO relationship_levels (level, name, required_points, description) VALUES
    (1, 'Tease', 100, 'Playful beginnings and light intimacy'),
    (2, 'Flirt', 250, 'Consistent connection and romantic momentum'),
    (3, 'Desire', 500, 'Strong attraction with deeper emotional sharing'),
    (4, 'Obsession', 1000, 'Intense bond with advanced intimate rituals'),
    (5, 'Afterglow', 1500, 'Peak relationship depth and long-term devotion')
ON CONFLICT (level) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS relationship_levels;
