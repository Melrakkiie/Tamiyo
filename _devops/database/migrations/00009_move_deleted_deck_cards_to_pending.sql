-- +goose Up
-- Deleting a card from the collection no longer takes it out of the decks it
-- was in: each deck gets it back in its pending list (one copy per deleted
-- card, merged with an existing pending entry for the same printing and
-- finish), to be added to the collection again later. Cascaded deletes (an
-- account being deleted) are skipped: everything goes away then.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION move_deleted_deck_card_to_pending()
RETURNS TRIGGER AS $$
BEGIN
    IF pg_trigger_depth() > 1 THEN
        RETURN OLD;
    END IF;

    UPDATE tamiyo.deck_pending_cards p
    SET quantity = p.quantity + 1
    FROM tamiyo.card_deck cd
    WHERE cd.card_id = OLD.id
      AND p.deck_id = cd.deck_id
      AND p.user_id = OLD.user_id
      AND p.scryfall_id = OLD.scryfall_id
      AND p.foil = OLD.foil;

    INSERT INTO tamiyo.deck_pending_cards
        (user_id, deck_id, name, scryfall_id, set_code, collector_number, foil, quantity, mana_value, colors, card_type, color_identity)
    SELECT OLD.user_id, cd.deck_id, OLD.name, OLD.scryfall_id, OLD.set_code, OLD.collector_number, OLD.foil, 1,
           OLD.mana_value, OLD.colors, OLD.card_type, OLD.color_identity
    FROM tamiyo.card_deck cd
    WHERE cd.card_id = OLD.id
      AND NOT EXISTS (
          SELECT 1 FROM tamiyo.deck_pending_cards p
          WHERE p.deck_id = cd.deck_id
            AND p.user_id = OLD.user_id
            AND p.scryfall_id = OLD.scryfall_id
            AND p.foil = OLD.foil
      );

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER move_deleted_deck_card_to_pending
    BEFORE DELETE ON tamiyo.cards
    FOR EACH ROW EXECUTE FUNCTION move_deleted_deck_card_to_pending();

-- +goose Down
DROP TRIGGER IF EXISTS move_deleted_deck_card_to_pending ON tamiyo.cards;
DROP FUNCTION IF EXISTS move_deleted_deck_card_to_pending();
