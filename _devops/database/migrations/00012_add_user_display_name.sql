-- +goose Up
-- Optional, purely cosmetic name shown in the app instead of the email.
-- Not unique: it never identifies an account.
ALTER TABLE tamiyo.users
    ADD COLUMN IF NOT EXISTS display_name text
        CHECK (display_name IS NULL OR char_length(display_name) BETWEEN 1 AND 32);

-- +goose Down
ALTER TABLE tamiyo.users DROP COLUMN IF EXISTS display_name;
