-- +goose Up
CREATE TABLE message_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL,
    storage_provider VARCHAR(16) NOT NULL DEFAULT 'minio',
    bucket_name VARCHAR(128) NOT NULL,
    object_key TEXT NOT NULL,
    mime_type VARCHAR(128) NOT NULL,
    size_bytes BIGINT NOT NULL,
    checksum_sha256 CHAR(64) NOT NULL,
    encrypted_file_key TEXT NOT NULL,
    encrypted_file_iv TEXT NOT NULL,
    encrypted_file_digest TEXT,
    file_name VARCHAR(255),
    width INTEGER,
    height INTEGER,
    duration_ms INTEGER,
    thumbnail_object_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT message_attachments_kind_chk CHECK (kind IN ('image', 'video', 'file', 'audio')),
    CONSTRAINT message_attachments_provider_chk CHECK (storage_provider IN ('minio')),
    CONSTRAINT message_attachments_size_chk CHECK (size_bytes > 0),
    CONSTRAINT message_attachments_dimensions_chk CHECK (
        (width IS NULL AND height IS NULL)
        OR
        (width IS NOT NULL AND height IS NOT NULL AND width > 0 AND height > 0)
    ),
    CONSTRAINT message_attachments_duration_chk CHECK (
        duration_ms IS NULL OR duration_ms > 0
    )
);

CREATE INDEX idx_message_attachments_message_id ON message_attachments(message_id);
CREATE INDEX idx_message_attachments_object_key ON message_attachments(object_key);
CREATE INDEX idx_message_attachments_kind ON message_attachments(kind);

-- +goose Down
DROP INDEX IF EXISTS idx_message_attachments_kind;
DROP INDEX IF EXISTS idx_message_attachments_object_key;
DROP INDEX IF EXISTS idx_message_attachments_message_id;
DROP TABLE IF EXISTS message_attachments;
