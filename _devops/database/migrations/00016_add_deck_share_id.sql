-- +goose Up
-- A random, unguessable identifier for a deck's share link, so an unlisted
-- deck can only be reached by the people its owner gave the link to.
ALTER TABLE tamiyo.deck
    ADD COLUMN IF NOT EXISTS share_id uuid NOT NULL DEFAULT gen_random_uuid();

CREATE UNIQUE INDEX IF NOT EXISTS deck_share_id_idx ON tamiyo.deck (share_id);

-- +goose Down
DROP INDEX IF EXISTS tamiyo.deck_share_id_idx;
ALTER TABLE tamiyo.deck DROP COLUMN IF EXISTS share_id;
