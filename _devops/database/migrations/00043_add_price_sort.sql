-- +goose Up
-- Price of one card in a currency, with the fallback the collection and
-- storage values use: its finish's price, then the other currency converted,
-- then the other finish. NULL when no price is known. Cards, storages and
-- decks sort by it, and the collection can save that sort.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION tamiyo.card_price(card_scryfall_id uuid, card_foil boolean, card_currency text)
RETURNS numeric
LANGUAGE sql
STABLE
AS $$
    SELECT CASE WHEN card_currency = 'usd' THEN
        COALESCE(CASE WHEN card_foil THEN COALESCE(p.usd_foil, p.usd_etched) ELSE p.usd END, p.usd, p.usd_foil, p.usd_etched)
    ELSE
        COALESCE(
            CASE WHEN card_foil THEN p.eur_foil ELSE p.eur END,
            CASE WHEN card_foil THEN COALESCE(p.usd_foil, p.usd_etched) ELSE p.usd END / r.per_eur,
            CASE WHEN card_foil THEN p.eur ELSE p.eur_foil END,
            CASE WHEN card_foil THEN p.usd ELSE COALESCE(p.usd_foil, p.usd_etched) END / r.per_eur)
    END
    FROM tamiyo.printings p
    LEFT JOIN tamiyo.exchange_rates r ON r.currency = 'usd'
    WHERE p.scryfall_id = card_scryfall_id
$$;
-- +goose StatementEnd

ALTER TABLE tamiyo.user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_collection_sort_check,
    ADD CONSTRAINT user_preferences_collection_sort_check CHECK (collection_sort IN (
        'name', '-name', 'added', '-added', 'updated', '-updated',
        'mana_value', '-mana_value', 'color', '-color', 'type', '-type',
        'price', '-price'
    ));

-- +goose Down
UPDATE tamiyo.user_preferences SET collection_sort = '-added' WHERE collection_sort IN ('price', '-price');
UPDATE tamiyo.deck_view_settings SET sort = 'mana_value' WHERE sort IN ('price', '-price');

ALTER TABLE tamiyo.user_preferences
    DROP CONSTRAINT IF EXISTS user_preferences_collection_sort_check,
    ADD CONSTRAINT user_preferences_collection_sort_check CHECK (collection_sort IN (
        'name', '-name', 'added', '-added', 'updated', '-updated',
        'mana_value', '-mana_value', 'color', '-color', 'type', '-type'
    ));

DROP FUNCTION IF EXISTS tamiyo.card_price(uuid, boolean, text);
