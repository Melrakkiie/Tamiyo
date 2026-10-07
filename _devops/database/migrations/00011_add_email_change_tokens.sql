-- +goose Up
-- Pending email changes: the token is emailed to the current address and the
-- account's email only changes once it's confirmed (POST /auth/confirm-email).
-- Only the hash is stored, like password_reset_tokens.
CREATE TABLE IF NOT EXISTS tamiyo.email_change_tokens
(
    id         UUID                        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID                        NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    new_email  text                        NOT NULL,
    token_hash text                        NOT NULL UNIQUE,
    added      TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    expires_at TIMESTAMP WITH TIME ZONE    NOT NULL,
    used_at    TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS email_change_tokens_user_idx ON tamiyo.email_change_tokens (user_id);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.email_change_tokens;
