-- +goose Up
-- Scryfall oracle text, mana cost, rarity and keywords of a printing, so the
-- collection search can evaluate Scryfall syntax (o:, m:, r:, kw:) locally.
-- Multi-face printings join their faces' oracle texts with "//". Every
-- printing is marked stale so the background refresher fills them within
-- minutes.
ALTER TABLE tamiyo.printings
    ADD COLUMN IF NOT EXISTS oracle_text text,
    ADD COLUMN IF NOT EXISTS mana_cost text,
    ADD COLUMN IF NOT EXISTS rarity text,
    ADD COLUMN IF NOT EXISTS keywords text[] NOT NULL DEFAULT '{}';

UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
ALTER TABLE tamiyo.printings
    DROP COLUMN IF EXISTS oracle_text,
    DROP COLUMN IF EXISTS mana_cost,
    DROP COLUMN IF EXISTS rarity,
    DROP COLUMN IF EXISTS keywords;
