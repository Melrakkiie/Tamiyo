-- +goose Up
-- A deck now has three sections: the deck itself (main), its sideboard and
-- the cards being considered for it. Only main counts in the deck's card
-- count. Collection cards and pending cards both carry their section.
ALTER TABLE tamiyo.card_deck
    ADD COLUMN board text NOT NULL DEFAULT 'main' CHECK (board IN ('main', 'sideboard', 'considering'));

ALTER TABLE tamiyo.deck_pending_cards
    ADD COLUMN board text NOT NULL DEFAULT 'main' CHECK (board IN ('main', 'sideboard', 'considering'));

-- Which of the two extra sections the owner keeps folded on the deck page.
ALTER TABLE tamiyo.deck_view_settings
    ADD COLUMN collapsed_boards text[] NOT NULL DEFAULT '{considering}';

-- Moving a card to another section changes the deck, like adding or removing
-- one does.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_decks_after_card_deck_update()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.deck SET updated = now()
    WHERE id IN (SELECT DISTINCT deck_id FROM new_links);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER touch_decks_after_card_deck_update
    AFTER UPDATE ON tamiyo.card_deck
    REFERENCING NEW TABLE AS new_links
    FOR EACH STATEMENT EXECUTE FUNCTION touch_decks_after_card_deck_update();

-- A card deleted from the collection becomes a pending card in the same
-- section of each deck it was in.
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

    FOR link IN SELECT cd.deck_id, cd.board FROM tamiyo.card_deck cd WHERE cd.card_id = OLD.id LOOP
        UPDATE tamiyo.deck_pending_cards
        SET quantity = quantity + 1
        WHERE id = (
            SELECT p.id FROM tamiyo.deck_pending_cards p
            WHERE p.deck_id = link.deck_id
              AND p.user_id = OLD.user_id
              AND p.scryfall_id = OLD.scryfall_id
              AND p.foil = OLD.foil
              AND p.board = link.board
              AND NOT EXISTS (SELECT 1 FROM tamiyo.deck d WHERE d.commander_pending_id = p.id)
            ORDER BY p.id
            LIMIT 1
        )
        RETURNING id INTO pending_id;

        IF pending_id IS NULL THEN
            INSERT INTO tamiyo.deck_pending_cards
                (user_id, deck_id, name, scryfall_id, set_code, collector_number, foil, quantity, mana_value, colors, card_type, color_identity, board)
            VALUES
                (OLD.user_id, link.deck_id, OLD.name, OLD.scryfall_id, OLD.set_code, OLD.collector_number, OLD.foil, 1,
                 OLD.mana_value, OLD.colors, OLD.card_type, OLD.color_identity, link.board)
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

DROP TRIGGER IF EXISTS touch_decks_after_card_deck_update ON tamiyo.card_deck;
DROP FUNCTION IF EXISTS touch_decks_after_card_deck_update();

ALTER TABLE tamiyo.deck_view_settings DROP COLUMN collapsed_boards;
ALTER TABLE tamiyo.deck_pending_cards DROP COLUMN board;
ALTER TABLE tamiyo.card_deck DROP COLUMN board;
