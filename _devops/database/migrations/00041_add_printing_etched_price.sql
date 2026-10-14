-- +goose Up
-- Scryfall TCGplayer dollar price of etched foils. Etched-only printings have no
-- other price, so it backs the foil dollar price. Every printing is marked
-- stale so the background refresher fills it within minutes.
ALTER TABLE tamiyo.printings
    ADD COLUMN IF NOT EXISTS usd_etched numeric(10, 2);

UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
ALTER TABLE tamiyo.printings
    DROP COLUMN IF EXISTS usd_etched;
