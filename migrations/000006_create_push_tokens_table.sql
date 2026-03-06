-- +goose Up
CREATE TABLE push_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    provider VARCHAR(16) NOT NULL,
    token TEXT NOT NULL,
    last_validated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    CONSTRAINT push_tokens_provider_chk CHECK (provider IN ('fcm', 'apns', 'webpush')),
    CONSTRAINT push_tokens_unique_active UNIQUE (device_id, token)
);

CREATE INDEX idx_push_tokens_device_id ON push_tokens(device_id);
CREATE INDEX idx_push_tokens_provider ON push_tokens(provider);

-- +goose Down
DROP TABLE IF EXISTS push_tokens;
