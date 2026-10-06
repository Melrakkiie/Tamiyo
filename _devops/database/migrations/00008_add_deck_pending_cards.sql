-- +goose Up
-- Cards wanted in a deck but not in the collection yet: picked on Scryfall
-- from the deck page, then created in the collection and put in the deck in
-- one go (POST /deck/{id}/pending/commit).
CREATE TABLE IF NOT EXISTS tamiyo.deck_pending_cards
(
    id               SERIAL                     PRIMARY KEY,
    user_id          UUID                       NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    deck_id          int                        NOT NULL REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    name             text                       NOT NULL,
    scryfall_id      uuid                       NOT NULL,
    set_code         text                       NOT NULL,
    collector_number text                       NOT NULL,
    foil             bool                       NOT NULL DEFAULT false,
    quantity         int                        NOT NULL DEFAULT 1 CHECK (quantity > 0),
    mana_value       numeric(6, 2)              NOT NULL DEFAULT 0,
    colors           text,
    card_type        text,
    color_identity   text,
    added            TIMESTAMP WITH TIME ZONE   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deck_pending_cards_deck_idx ON tamiyo.deck_pending_cards (deck_id);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.deck_pending_cards;
