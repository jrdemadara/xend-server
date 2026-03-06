-- +goose Up
CREATE TABLE user_oauth_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    email CITEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT user_oauth_accounts_provider_chk CHECK (provider IN ('google')),
    CONSTRAINT user_oauth_accounts_provider_subject_unique UNIQUE (provider, provider_user_id)
);

CREATE UNIQUE INDEX uq_user_oauth_accounts_user_provider
    ON user_oauth_accounts(user_id, provider);

CREATE INDEX idx_user_oauth_accounts_user_id ON user_oauth_accounts(user_id);

CREATE TRIGGER user_oauth_accounts_set_updated_at
BEFORE UPDATE ON user_oauth_accounts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TRIGGER IF EXISTS user_oauth_accounts_set_updated_at ON user_oauth_accounts;
DROP INDEX IF EXISTS idx_user_oauth_accounts_user_id;
DROP INDEX IF EXISTS uq_user_oauth_accounts_user_provider;
DROP TABLE IF EXISTS user_oauth_accounts;
