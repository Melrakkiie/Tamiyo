-- +goose Up
-- A deck's commander can be a card that isn't in the collection yet (one of
-- its pending cards), so a deck can be planned before owning any of it. A
-- deck has at most one of the two. When the pending commander is added to
-- the collection the API moves it to commander_id; deleting the commander
-- card from the collection now keeps it as the pending commander.
ALTER TABLE tamiyo.deck
    ADD COLUMN commander_pending_id int REFERENCES tamiyo.deck_pending_cards(id) ON DELETE SET NULL,
    ADD CONSTRAINT deck_single_commander CHECK (commander_id IS NULL OR commander_pending_id IS NULL);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION move_deleted_deck_card_to_pending()
RETURNS TRIGGER AS $$
DECLARE
    link       record;
    pending_id int;
BEGIN
    IF pg_trigger_depth() > 1 THEN
        RETURN OLD;
    END IF;

    FOR link IN SELECT cd.deck_id FROM tamiyo.card_deck cd WHERE cd.card_id = OLD.id LOOP
        UPDATE tamiyo.deck_pending_cards
        SET quantity = quantity + 1
        WHERE id = (
            SELECT p.id FROM tamiyo.deck_pending_cards p
            WHERE p.deck_id = link.deck_id
              AND p.user_id = OLD.user_id
              AND p.scryfall_id = OLD.scryfall_id
              AND p.foil = OLD.foil
            ORDER BY p.id
            LIMIT 1
        )
        RETURNING id INTO pending_id;

        IF pending_id IS NULL THEN
            INSERT INTO tamiyo.deck_pending_cards
                (user_id, deck_id, name, scryfall_id, set_code, collector_number, foil, quantity, mana_value, colors, card_type, color_identity)
            VALUES
                (OLD.user_id, link.deck_id, OLD.name, OLD.scryfall_id, OLD.set_code, OLD.collector_number, OLD.foil, 1,
                 OLD.mana_value, OLD.colors, OLD.card_type, OLD.color_identity)
            RETURNING id INTO pending_id;
        END IF;

        UPDATE tamiyo.deck
        SET commander_id = NULL, commander_pending_id = pending_id
        WHERE id = link.deck_id AND commander_id = OLD.id;

        pending_id := NULL;
    END LOOP;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
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

ALTER TABLE tamiyo.deck
    DROP CONSTRAINT deck_single_commander,
    DROP COLUMN commander_pending_id;
