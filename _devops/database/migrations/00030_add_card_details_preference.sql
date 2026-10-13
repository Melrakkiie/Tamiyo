-- +goose Up
-- Whether card images show the card's details (name, printing, storage)
-- below them.
ALTER TABLE tamiyo.user_preferences
    ADD COLUMN IF NOT EXISTS show_card_details boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE tamiyo.user_preferences
    DROP COLUMN IF EXISTS show_card_details;
