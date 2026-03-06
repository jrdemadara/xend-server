-- +goose Up
CREATE TABLE message_receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    sent_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    status VARCHAR(16) NOT NULL DEFAULT 'sent',
    failure_code VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT message_receipts_msg_recipient_device_unique UNIQUE (message_id, recipient_device_id),
    CONSTRAINT message_receipts_status_chk CHECK (status IN ('sent', 'delivered', 'read', 'failed')),
    CONSTRAINT message_receipts_delivered_requires_sent_chk CHECK (
        delivered_at IS NULL OR sent_at IS NOT NULL
    ),
    CONSTRAINT message_receipts_read_requires_delivered_chk CHECK (
        read_at IS NULL OR delivered_at IS NOT NULL
    ),
    CONSTRAINT message_receipts_failed_consistency_chk CHECK (
        (status = 'failed' AND failed_at IS NOT NULL)
        OR
        (status <> 'failed' AND failed_at IS NULL)
    )
);

CREATE INDEX idx_message_receipts_recipient_device ON message_receipts(recipient_device_id, created_at DESC);
CREATE INDEX idx_message_receipts_message_id ON message_receipts(message_id);
CREATE INDEX idx_message_receipts_unread ON message_receipts(recipient_device_id, read_at) WHERE read_at IS NULL;
CREATE INDEX idx_message_receipts_status ON message_receipts(recipient_device_id, status);

-- +goose Down
DROP INDEX IF EXISTS idx_message_receipts_status;
DROP INDEX IF EXISTS idx_message_receipts_unread;
DROP INDEX IF EXISTS idx_message_receipts_message_id;
DROP INDEX IF EXISTS idx_message_receipts_recipient_device;
DROP TABLE IF EXISTS message_receipts;
