-- +goose Up
CREATE TABLE kyber_prekeys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key_id INTEGER NOT NULL,
    public_key TEXT NOT NULL,
    signature TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT kyber_prekeys_device_key_unique UNIQUE (device_id, key_id)
);

CREATE INDEX idx_kyber_prekeys_device_active
    ON kyber_prekeys(device_id)
    WHERE is_active = TRUE AND revoked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_kyber_prekeys_device_active;
DROP TABLE IF EXISTS kyber_prekeys;
