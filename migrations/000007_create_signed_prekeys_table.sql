-- +goose Up
CREATE TABLE signed_prekeys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key_id INTEGER NOT NULL,
    public_key TEXT NOT NULL,
    signature TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT signed_prekeys_device_key_unique UNIQUE (device_id, key_id)
);

CREATE UNIQUE INDEX uq_signed_prekeys_one_active_per_device
    ON signed_prekeys(device_id)
    WHERE is_active = TRUE AND revoked_at IS NULL;

CREATE INDEX idx_signed_prekeys_device_id ON signed_prekeys(device_id);

-- +goose Down
DROP INDEX IF EXISTS idx_signed_prekeys_device_id;
DROP INDEX IF EXISTS uq_signed_prekeys_one_active_per_device;
DROP TABLE IF EXISTS signed_prekeys;
