-- +goose Up
-- Profile picture: the Scryfall id of a card whose art crop is shown as the
-- avatar. Only the id is stored, the image itself stays on Scryfall.
ALTER TABLE tamiyo.users ADD COLUMN IF NOT EXISTS avatar_scryfall_id uuid;

-- +goose Down
ALTER TABLE tamiyo.users DROP COLUMN IF EXISTS avatar_scryfall_id;
