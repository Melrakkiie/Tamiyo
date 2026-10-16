-- +goose Up
-- Tags created on a deck before any card has them, so the owner can set up a
-- list of tags first and then use them as suggestions, drop targets and
-- groups. A deck's tag list is the tags of its cards plus these ones; a tag
-- declared here stays in the list with no card until it is deleted.
CREATE TABLE IF NOT EXISTS tamiyo.deck_tags
(
    deck_id uuid NOT NULL REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    tag     text NOT NULL CHECK (char_length(tag) BETWEEN 1 AND 40),
    PRIMARY KEY (deck_id, tag)
);

CREATE TRIGGER deck_tags_bump_deck
    AFTER INSERT OR DELETE ON tamiyo.deck_tags
    FOR EACH ROW EXECUTE FUNCTION bump_deck_on_tag_change();

-- +goose Down
DROP TRIGGER IF EXISTS deck_tags_bump_deck ON tamiyo.deck_tags;
DROP TABLE IF EXISTS tamiyo.deck_tags;
