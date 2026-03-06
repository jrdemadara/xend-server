-- +goose Up
CREATE TABLE refresh_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL UNIQUE,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    replaced_by_session_id UUID REFERENCES refresh_sessions(id) ON DELETE SET NULL,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT refresh_sessions_expiry_chk CHECK (expires_at > issued_at)
);

CREATE INDEX idx_refresh_sessions_user_id ON refresh_sessions(user_id);
CREATE INDEX idx_refresh_sessions_device_id ON refresh_sessions(device_id);
CREATE INDEX idx_refresh_sessions_user_device ON refresh_sessions(user_id, device_id);
CREATE INDEX idx_refresh_sessions_active ON refresh_sessions(user_id, revoked_at, expires_at);

-- +goose Down
DROP TABLE IF EXISTS refresh_sessions;
