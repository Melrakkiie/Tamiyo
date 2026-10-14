-- +goose Up
-- Scryfall power and toughness of every face of a printing, kept as text since
-- they can be "*", "1+*" or "X". The advanced card search compares the numeric
-- ones. Every printing is marked stale so the background refresher fills them
-- within minutes.
ALTER TABLE tamiyo.printings
    ADD COLUMN IF NOT EXISTS powers text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS toughnesses text[] NOT NULL DEFAULT '{}';

UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
ALTER TABLE tamiyo.printings
    DROP COLUMN IF EXISTS powers,
    DROP COLUMN IF EXISTS toughnesses;
