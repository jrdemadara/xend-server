-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name VARCHAR(100) NOT NULL,
    email CITEXT NOT NULL UNIQUE,
    avatar_url TEXT,
    identifier VARCHAR(16) NOT NULL UNIQUE,
    auto_rotate_identifier BOOLEAN NOT NULL DEFAULT FALSE,
    identifier_rotation_days INTEGER,
    identifier_rotates_at TIMESTAMPTZ,
    email_verified_at TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT users_identifier_format_chk CHECK (identifier ~ '^[a-zA-Z0-9_]{6,16}$'),
    CONSTRAINT users_identifier_rotation_days_chk CHECK (
        identifier_rotation_days IS NULL OR identifier_rotation_days >= 1
    ),
    CONSTRAINT users_identifier_rotation_config_chk CHECK (
        (auto_rotate_identifier = FALSE AND identifier_rotation_days IS NULL AND identifier_rotates_at IS NULL)
        OR
        (auto_rotate_identifier = TRUE AND identifier_rotation_days IS NOT NULL AND identifier_rotates_at IS NOT NULL)
    )
);

CREATE INDEX idx_users_identifier_rotates_at
    ON users(identifier_rotates_at)
    WHERE auto_rotate_identifier = TRUE;

CREATE TRIGGER users_set_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS users_set_updated_at ON users;
DROP INDEX IF EXISTS idx_users_identifier_rotates_at;
DROP TABLE IF EXISTS users;
