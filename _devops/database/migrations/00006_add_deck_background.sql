-- +goose Up
-- Scryfall id of the printing whose art is shown behind the deck. NULL means
-- no choice (the frontend falls back to the commander's art).
ALTER TABLE tamiyo.deck ADD COLUMN background_scryfall_id uuid;

-- +goose Down
ALTER TABLE tamiyo.deck DROP COLUMN background_scryfall_id;
