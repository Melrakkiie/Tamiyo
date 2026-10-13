-- +goose Up
-- Each deck remembers how its cards are displayed, on top of its grouping and
-- sort: as text rows or card images, images per row, mana cost on text rows,
-- details below images.
ALTER TABLE tamiyo.deck_view_settings
    ADD COLUMN IF NOT EXISTS card_display text NOT NULL DEFAULT 'images' CHECK (card_display IN ('text', 'images')),
    ADD COLUMN IF NOT EXISTS card_columns smallint NOT NULL DEFAULT 6 CHECK (card_columns BETWEEN 2 AND 12),
    ADD COLUMN IF NOT EXISTS show_mana_cost boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS show_card_details boolean NOT NULL DEFAULT true;

-- The collection and the storages remember their grouping (NULL for none)
-- and sort, next to the display preferences they already use.
ALTER TABLE tamiyo.user_preferences
    ADD COLUMN IF NOT EXISTS collection_grouping text CHECK (collection_grouping IN ('type', 'color', 'mana')),
    ADD COLUMN IF NOT EXISTS collection_sort text NOT NULL DEFAULT '-added' CHECK (collection_sort IN (
        'name', '-name', 'added', '-added', 'updated', '-updated',
        'mana_value', '-mana_value', 'color', '-color', 'type', '-type'
    ));

-- +goose Down
ALTER TABLE tamiyo.user_preferences
    DROP COLUMN IF EXISTS collection_sort,
    DROP COLUMN IF EXISTS collection_grouping;

ALTER TABLE tamiyo.deck_view_settings
    DROP COLUMN IF EXISTS show_card_details,
    DROP COLUMN IF EXISTS show_mana_cost,
    DROP COLUMN IF EXISTS card_columns,
    DROP COLUMN IF EXISTS card_display;
