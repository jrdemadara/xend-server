-- +goose Up
CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name VARCHAR(64) NOT NULL,
    platform VARCHAR(16) NOT NULL,
    registration_id INTEGER NOT NULL,
    identity_key_public TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT devices_platform_chk CHECK (platform IN ('android', 'ios', 'desktop')),
    CONSTRAINT devices_registration_id_chk CHECK (registration_id >= 1 AND registration_id <= 2147483647)
);

CREATE INDEX idx_devices_user_id ON devices(user_id);
CREATE INDEX idx_devices_user_active ON devices(user_id, is_active);

CREATE TRIGGER devices_set_updated_at
BEFORE UPDATE ON devices
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS devices_set_updated_at ON devices;
DROP TABLE IF EXISTS devices;
