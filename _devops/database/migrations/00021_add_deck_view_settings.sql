-- +goose Up
-- How a deck's owner last displayed its cards (grouping and sort), so the
-- deck page opens the same way next time. Kept apart from tamiyo.deck so
-- changing it doesn't count as changing the deck.
CREATE TABLE IF NOT EXISTS tamiyo.deck_view_settings
(
    deck_id  uuid PRIMARY KEY REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    grouping text,
    sort     text NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.deck_view_settings;
