-- +goose Up
-- Scryfall data about a printing that cards don't store themselves: the full
-- type line (for type and subtype searches) and the formats it is legal in.
-- Shared by every account, keyed by Scryfall id, and refreshed by the API in
-- the background: new printings within minutes, every printing once a day so
-- bans and unbans show up.
CREATE TABLE IF NOT EXISTS tamiyo.printings
(
    scryfall_id   uuid PRIMARY KEY,
    type_line     text   NOT NULL,
    legal_formats text[] NOT NULL DEFAULT '{}',
    refreshed_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS cards_scryfall_id_idx ON tamiyo.cards (scryfall_id);

-- +goose Down
DROP INDEX IF EXISTS tamiyo.cards_scryfall_id_idx;
DROP TABLE IF EXISTS tamiyo.printings;
