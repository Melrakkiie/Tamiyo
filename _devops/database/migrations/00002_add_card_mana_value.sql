-- +goose Up
-- Mana value (converted mana cost / CMC) of a card, so cards can be sorted
-- by it in listings (GET /cards, GET /cards?storage_id=, GET /deck/:id/cards).
-- Stored as numeric rather than integer because Scryfall's own "cmc" field
-- is a float (a small number of cards, e.g. some un-set ones, have a
-- fractional mana value).
ALTER TABLE tamiyo.cards
    ADD COLUMN mana_value numeric(6, 2) NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tamiyo.cards
    DROP COLUMN mana_value;
