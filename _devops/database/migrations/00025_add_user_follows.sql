-- +goose Up
-- Who follows whom. A user follows another at most once, never themselves,
-- and the link goes away with either account.
CREATE TABLE IF NOT EXISTS tamiyo.user_follows
(
    follower_id uuid                     NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    followed_id uuid                     NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    added       TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followed_id),
    CHECK (follower_id <> followed_id)
);

CREATE INDEX IF NOT EXISTS user_follows_followed_idx ON tamiyo.user_follows (followed_id, added DESC);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.user_follows;
