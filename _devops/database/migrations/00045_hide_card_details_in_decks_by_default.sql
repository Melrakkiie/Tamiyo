-- +goose Up
-- Decks hide the details under their card images (name, printing, storage)
-- until their display settings are saved, unlike the collection. The service
-- always writes this column, so this only keeps the schema default in line
-- with the deck's default view. Saved settings are left as they are.
ALTER TABLE tamiyo.deck_view_settings ALTER COLUMN show_card_details SET DEFAULT false;

-- +goose Down
ALTER TABLE tamiyo.deck_view_settings ALTER COLUMN show_card_details SET DEFAULT true;
