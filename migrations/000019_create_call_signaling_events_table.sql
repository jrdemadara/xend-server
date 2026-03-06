-- +goose Up
CREATE TABLE call_signaling_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_session_id UUID NOT NULL REFERENCES call_sessions(id) ON DELETE CASCADE,
    from_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    to_device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    event_type VARCHAR(24) NOT NULL,
    payload_ciphertext TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    CONSTRAINT call_signaling_events_type_chk CHECK (event_type IN ('offer', 'answer', 'ice_candidate', 'renegotiate', 'hangup', 'mute', 'unmute', 'video_on', 'video_off'))
);

CREATE INDEX idx_call_signaling_events_call_session_id ON call_signaling_events(call_session_id, created_at DESC);
CREATE INDEX idx_call_signaling_events_to_device_id ON call_signaling_events(to_device_id, created_at DESC);
CREATE INDEX idx_call_signaling_events_from_device_id ON call_signaling_events(from_device_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_call_signaling_events_from_device_id;
DROP INDEX IF EXISTS idx_call_signaling_events_to_device_id;
DROP INDEX IF EXISTS idx_call_signaling_events_call_session_id;
DROP TABLE IF EXISTS call_signaling_events;
