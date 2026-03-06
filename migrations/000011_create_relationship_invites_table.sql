-- +goose Up
CREATE TABLE relationship_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    relationship_space_id UUID REFERENCES relationship_spaces(id) ON DELETE SET NULL,
    inviter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    note TEXT,
    expires_at TIMESTAMPTZ,
    responded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relationship_invites_status_chk CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled', 'expired')),
    CONSTRAINT relationship_invites_distinct_users_chk CHECK (inviter_user_id <> invitee_user_id)
);

CREATE UNIQUE INDEX uq_relationship_invites_pending
    ON relationship_invites(inviter_user_id, invitee_user_id)
    WHERE status = 'pending';

CREATE INDEX idx_relationship_invites_invitee ON relationship_invites(invitee_user_id, status, created_at DESC);
CREATE INDEX idx_relationship_invites_inviter ON relationship_invites(inviter_user_id, status, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_relationship_invites_inviter;
DROP INDEX IF EXISTS idx_relationship_invites_invitee;
DROP INDEX IF EXISTS uq_relationship_invites_pending;
DROP TABLE IF EXISTS relationship_invites;
