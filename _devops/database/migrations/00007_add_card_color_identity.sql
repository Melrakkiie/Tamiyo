-- +goose Up
-- Color identity of a card (WUBRG letters in that order, empty for
-- colorless), used to only offer cards a commander deck can play. NULL means
-- not known yet: filled at creation and import, or backfilled from Scryfall
-- by POST /cards/refresh-details.
ALTER TABLE tamiyo.cards ADD COLUMN color_identity text;

-- +goose Down
ALTER TABLE tamiyo.cards DROP COLUMN color_identity;
