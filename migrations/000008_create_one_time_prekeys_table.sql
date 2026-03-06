-- +goose Up
CREATE TABLE one_time_prekeys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    key_id INTEGER NOT NULL,
    public_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_at TIMESTAMPTZ,
    CONSTRAINT one_time_prekeys_device_key_unique UNIQUE (device_id, key_id)
);

CREATE INDEX idx_one_time_prekeys_device_id ON one_time_prekeys(device_id);
CREATE INDEX idx_one_time_prekeys_available ON one_time_prekeys(device_id, consumed_at);

-- +goose Down
DROP INDEX IF EXISTS idx_one_time_prekeys_available;
DROP INDEX IF EXISTS idx_one_time_prekeys_device_id;
DROP TABLE IF EXISTS one_time_prekeys;
