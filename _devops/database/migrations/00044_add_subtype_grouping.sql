-- +goose Up
-- Subtypes of a card from its English type line, every face included
-- ("A — X Y // B — Z" gives X, Y, Z), deduplicated and sorted. Empty when the
-- card has no subtype or its printing is not known yet. The collection,
-- storages and decks can group cards by subtype, a card appearing once per
-- subtype, and the collection can save that grouping.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tamiyo.card_subtypes(card_scryfall_id uuid)
RETURNS text[]
LANGUAGE sql
STABLE
AS $$
    SELECT COALESCE(array_agg(DISTINCT subtype ORDER BY subtype), '{}')
    FROM tamiyo.printings p,
        regexp_split_to_table(p.type_line, '\s*//\s*') AS face,
        regexp_split_to_table(trim(split_part(face, '—', 2)), '\s+') AS subtype
    WHERE p.scryfall_id = card_scryfall_id AND subtype <> ''
$$;
-- +goose StatementEnd

ALTER TABLE tamiyo.user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_collection_grouping_check,
    ADD CONSTRAINT user_preferences_collection_grouping_check CHECK (collection_grouping IN ('type', 'color', 'mana', 'subtype'));

-- +goose Down
UPDATE tamiyo.user_preferences SET collection_grouping = NULL WHERE collection_grouping = 'subtype';
UPDATE tamiyo.deck_view_settings SET grouping = 'type' WHERE grouping = 'subtype';

ALTER TABLE tamiyo.user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_collection_grouping_check,
    ADD CONSTRAINT user_preferences_collection_grouping_check CHECK (collection_grouping IN ('type', 'color', 'mana'));

DROP FUNCTION IF EXISTS tamiyo.card_subtypes(uuid);
