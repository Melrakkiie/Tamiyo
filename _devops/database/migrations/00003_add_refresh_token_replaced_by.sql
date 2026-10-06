-- +goose Up
-- Successor of a refresh token rotated out by /auth/refresh. Lets a token
-- reused a few seconds after its rotation (the client never got the
-- response, e.g. a page reloaded mid-refresh) be told apart from a stolen
-- one; see token.Service.Rotate.
ALTER TABLE tamiyo.refresh_tokens
    ADD COLUMN replaced_by UUID REFERENCES tamiyo.refresh_tokens (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE tamiyo.refresh_tokens
    DROP COLUMN replaced_by;
