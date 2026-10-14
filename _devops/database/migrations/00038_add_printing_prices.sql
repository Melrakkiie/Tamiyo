-- +goose Up
-- Scryfall prices of each printing (Cardmarket euros, TCGplayer dollars,
-- regular and foil), refreshed with the rest of the printing. Every printing is
-- marked stale so the background refresher fills the prices within minutes.
ALTER TABLE tamiyo.printings
    ADD COLUMN IF NOT EXISTS eur numeric(10, 2),
    ADD COLUMN IF NOT EXISTS eur_foil numeric(10, 2),
    ADD COLUMN IF NOT EXISTS usd numeric(10, 2),
    ADD COLUMN IF NOT EXISTS usd_foil numeric(10, 2);

UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
ALTER TABLE tamiyo.printings
    DROP COLUMN IF EXISTS usd_foil,
    DROP COLUMN IF EXISTS usd,
    DROP COLUMN IF EXISTS eur_foil,
    DROP COLUMN IF EXISTS eur;
