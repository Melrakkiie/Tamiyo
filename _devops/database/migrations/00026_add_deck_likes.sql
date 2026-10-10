-- +goose Up
-- Decks a user liked. A user likes a deck at most once, and the like goes
-- away with the deck or the account.
CREATE TABLE IF NOT EXISTS tamiyo.deck_likes
(
    user_id uuid                     NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    deck_id uuid                     NOT NULL REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    added   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, deck_id)
);

CREATE INDEX IF NOT EXISTS deck_likes_deck_idx ON tamiyo.deck_likes (deck_id);

-- +goose Down
DROP TABLE IF EXISTS tamiyo.deck_likes;
