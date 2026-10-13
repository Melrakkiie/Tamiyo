-- +goose Up
-- A storage can be locked so decks stop drawing from it: its cards no longer
-- count as owned copies for a deck's pending cards, nor get linked by deck
-- imports, duplications or finish switches. A locked storage stays open to
-- the decks listed in storage_allowed_deck; with none listed it is closed to
-- every deck. The list is kept while the storage is unlocked, and ignored.
ALTER TABLE tamiyo.storage ADD COLUMN IF NOT EXISTS locked boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS tamiyo.storage_allowed_deck
(
    storage_id integer NOT NULL REFERENCES tamiyo.storage(id) ON DELETE CASCADE,
    deck_id    uuid    NOT NULL REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    PRIMARY KEY (storage_id, deck_id)
);

CREATE INDEX IF NOT EXISTS storage_allowed_deck_deck_idx ON tamiyo.storage_allowed_deck (deck_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_storage_allowed_deck_ownership()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM tamiyo.storage s
        JOIN tamiyo.deck d ON d.user_id = s.user_id
        WHERE s.id = NEW.storage_id AND d.id = NEW.deck_id
    ) THEN
        RAISE EXCEPTION 'storage % and deck % do not belong to the same user', NEW.storage_id, NEW.deck_id
            USING ERRCODE = 'foreign_key_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
-- +goose StatementEnd

CREATE TRIGGER check_storage_allowed_deck_ownership
    BEFORE INSERT OR UPDATE ON tamiyo.storage_allowed_deck
    FOR EACH ROW EXECUTE FUNCTION check_storage_allowed_deck_ownership();

-- +goose Down
DROP TABLE IF EXISTS tamiyo.storage_allowed_deck;
DROP FUNCTION IF EXISTS check_storage_allowed_deck_ownership();
ALTER TABLE tamiyo.storage DROP COLUMN IF EXISTS locked;
