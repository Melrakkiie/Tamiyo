-- +goose Up
-- Colors (WUBRG letters in that order, empty for colorless) and primary type
-- of a card, so cards can be sorted and grouped by them in SQL. NULL means
-- not known yet: filled at creation and import, or backfilled from Scryfall
-- by POST /cards/refresh-details.
ALTER TABLE tamiyo.cards
    ADD COLUMN colors    text,
    ADD COLUMN card_type text;

-- +goose Down
ALTER TABLE tamiyo.cards
    DROP COLUMN card_type,
    DROP COLUMN colors;
