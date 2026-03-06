-- +goose Up
CREATE TABLE call_participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_session_id UUID NOT NULL REFERENCES call_sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'callee',
    join_state VARCHAR(16) NOT NULL DEFAULT 'invited',
    invited_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ringing_at TIMESTAMPTZ,
    joined_at TIMESTAMPTZ,
    left_at TIMESTAMPTZ,
    left_reason VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT call_participants_role_chk CHECK (role IN ('caller', 'callee')),
    CONSTRAINT call_participants_join_state_chk CHECK (join_state IN ('invited', 'ringing', 'joined', 'left', 'declined', 'missed', 'failed')),
    CONSTRAINT call_participants_unique_user UNIQUE (call_session_id, user_id),
    CONSTRAINT call_participants_unique_device UNIQUE (call_session_id, device_id)
);

CREATE INDEX idx_call_participants_call_session_id ON call_participants(call_session_id);
CREATE INDEX idx_call_participants_user_id ON call_participants(user_id, invited_at DESC);
CREATE INDEX idx_call_participants_device_id ON call_participants(device_id, invited_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_call_participants_device_id;
DROP INDEX IF EXISTS idx_call_participants_user_id;
DROP INDEX IF EXISTS idx_call_participants_call_session_id;
DROP TABLE IF EXISTS call_participants;
