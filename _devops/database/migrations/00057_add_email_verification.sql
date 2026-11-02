-- +goose Up
-- An account is verified once its owner follows the link mailed at registration
-- (POST /auth/verify-email). Accounts that exist before this migration count as verified.
ALTER TABLE tamiyo.users ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMP WITH TIME ZONE;
UPDATE tamiyo.users SET email_verified_at = now() WHERE email_verified_at IS NULL;

-- Only the hash is stored, like password_reset_tokens.
CREATE TABLE IF NOT EXISTS tamiyo.email_verification_tokens
(
    id         UUID                        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID                        NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    token_hash text                        NOT NULL UNIQUE,
    added      TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    expires_at TIMESTAMP WITH TIME ZONE    NOT NULL,
    used_at    TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS email_verification_tokens_user_idx ON tamiyo.email_verification_tokens (user_id);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.email_verification_tokens;
ALTER TABLE tamiyo.users DROP COLUMN IF EXISTS email_verified_at;
