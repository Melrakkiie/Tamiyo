-- +goose Up
-- Scryfall's game_changer flag: the card is on Wizards' Commander list of
-- cards that strongly raise a deck's power. Deck statistics count them.
-- Every printing is marked stale so the background refresher fetches the
-- flag for all of them at startup instead of reading false for up to a day.
ALTER TABLE tamiyo.printings
    ADD COLUMN IF NOT EXISTS game_changer boolean NOT NULL DEFAULT false;

UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
ALTER TABLE tamiyo.printings
    DROP COLUMN IF EXISTS game_changer;
