-- +goose Up
-- What deck statistics and legality need from a printing, so they can read
-- the printings table instead of calling Scryfall: mana value, colors, color
-- identity (Scryfall color letters) and the formats that ban or restrict the
-- card. legal_formats keeps its meaning (legal or restricted) for search.
-- Every printing is marked stale so the background refresher fills them
-- within minutes.
ALTER TABLE tamiyo.printings
    ADD COLUMN IF NOT EXISTS mana_value numeric NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS colors text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS color_identity text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS banned_formats text[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS restricted_formats text[] NOT NULL DEFAULT '{}';

UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
ALTER TABLE tamiyo.printings
    DROP COLUMN IF EXISTS mana_value,
    DROP COLUMN IF EXISTS colors,
    DROP COLUMN IF EXISTS color_identity,
    DROP COLUMN IF EXISTS banned_formats,
    DROP COLUMN IF EXISTS restricted_formats;
