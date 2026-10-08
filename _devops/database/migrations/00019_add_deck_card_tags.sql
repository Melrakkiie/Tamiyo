-- +goose Up
-- Tags a deck's owner puts on its cards to help deckbuilding (Ramp, Removal…).
-- A tag belongs to a card name within a deck, not to a physical card: every
-- copy of that card in the deck shares it, pending ones included, whatever
-- the printing. card_name holds the normalized name (lowercase, " // " and
-- " / " treated alike, spaces collapsed).
CREATE TABLE IF NOT EXISTS tamiyo.deck_card_tags
(
    deck_id   uuid NOT NULL REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    card_name text NOT NULL,
    tag       text NOT NULL CHECK (char_length(tag) BETWEEN 1 AND 40),
    PRIMARY KEY (deck_id, card_name, tag)
);

-- Tagging a card counts as changing the deck.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bump_deck_on_tag_change()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.deck SET updated = now() WHERE id = COALESCE(NEW.deck_id, OLD.deck_id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER deck_card_tags_bump_deck
    AFTER INSERT OR DELETE ON tamiyo.deck_card_tags
    FOR EACH ROW EXECUTE FUNCTION bump_deck_on_tag_change();

-- +goose Down
DROP TRIGGER IF EXISTS deck_card_tags_bump_deck ON tamiyo.deck_card_tags;
DROP FUNCTION IF EXISTS bump_deck_on_tag_change();
DROP TABLE IF EXISTS tamiyo.deck_card_tags;
