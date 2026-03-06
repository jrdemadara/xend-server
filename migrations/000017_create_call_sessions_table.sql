-- +goose Up
CREATE TABLE call_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    started_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    call_type VARCHAR(16) NOT NULL,
    mode VARCHAR(16) NOT NULL,
    livekit_room_name VARCHAR(128) NOT NULL UNIQUE,
    livekit_room_sid VARCHAR(64),
    status VARCHAR(16) NOT NULL DEFAULT 'ringing',
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    answered_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    end_reason VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT call_sessions_call_type_chk CHECK (call_type IN ('audio', 'video')),
    CONSTRAINT call_sessions_mode_chk CHECK (mode IN ('p2p', 'group')),
    CONSTRAINT call_sessions_status_chk CHECK (status IN ('ringing', 'ongoing', 'ended', 'missed', 'rejected', 'failed', 'cancelled')),
    CONSTRAINT call_sessions_timing_chk CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE INDEX idx_call_sessions_conversation_id ON call_sessions(conversation_id, started_at DESC);
CREATE INDEX idx_call_sessions_started_by_user_id ON call_sessions(started_by_user_id, started_at DESC);
CREATE INDEX idx_call_sessions_status ON call_sessions(status);

CREATE TRIGGER call_sessions_set_updated_at
BEFORE UPDATE ON call_sessions
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS call_sessions_set_updated_at ON call_sessions;
DROP INDEX IF EXISTS idx_call_sessions_status;
DROP INDEX IF EXISTS idx_call_sessions_started_by_user_id;
DROP INDEX IF EXISTS idx_call_sessions_conversation_id;
DROP TABLE IF EXISTS call_sessions;
