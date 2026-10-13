-- +goose Up
-- How the user likes card lists shown: as text rows or as card images, how
-- many images per row, and whether text rows show the mana cost.
ALTER TABLE tamiyo.user_preferences
    ADD COLUMN IF NOT EXISTS card_display text NOT NULL DEFAULT 'images' CHECK (card_display IN ('text', 'images')),
    ADD COLUMN IF NOT EXISTS card_columns smallint NOT NULL DEFAULT 6 CHECK (card_columns BETWEEN 2 AND 12),
    ADD COLUMN IF NOT EXISTS show_mana_cost boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE tamiyo.user_preferences
    DROP COLUMN IF EXISTS show_mana_cost,
    DROP COLUMN IF EXISTS card_columns,
    DROP COLUMN IF EXISTS card_display;
