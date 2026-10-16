-- +goose Up
-- A Commander deck can have a second commander (Partner, Friends forever,
-- Choose a Background, Doctor's companion...). Like the first one it is an
-- owned card (partner_id) or one of the deck's pending cards
-- (partner_pending_id), never both, and never the same card as the
-- commander. A partner needs a commander: when the commander slot is
-- emptied, the partner moves up into it. Deleting the partner card from the
-- collection keeps it as the pending partner, like the commander.
ALTER TABLE tamiyo.deck
    ADD COLUMN partner_id int REFERENCES tamiyo.cards(id) ON DELETE SET NULL,
    ADD COLUMN partner_pending_id int REFERENCES tamiyo.deck_pending_cards(id) ON DELETE SET NULL,
    ADD CONSTRAINT deck_single_partner CHECK (partner_id IS NULL OR partner_pending_id IS NULL),
    ADD CONSTRAINT deck_partner_needs_commander CHECK (
        (partner_id IS NULL AND partner_pending_id IS NULL)
        OR commander_id IS NOT NULL OR commander_pending_id IS NOT NULL
    ),
    ADD CONSTRAINT deck_distinct_commanders CHECK (partner_id IS NULL OR partner_id IS DISTINCT FROM commander_id),
    ADD CONSTRAINT deck_distinct_pending_commanders CHECK (partner_pending_id IS NULL OR partner_pending_id IS DISTINCT FROM commander_pending_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_deck_commander_ownership()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.commander_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM tamiyo.cards
            WHERE id = NEW.commander_id AND user_id = NEW.user_id
        ) THEN
            RAISE EXCEPTION 'card % does not belong to user %', NEW.commander_id, NEW.user_id
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    IF NEW.partner_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM tamiyo.cards
            WHERE id = NEW.partner_id AND user_id = NEW.user_id
        ) THEN
            RAISE EXCEPTION 'card % does not belong to user %', NEW.partner_id, NEW.user_id
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
-- +goose StatementEnd

-- The commander slot emptied by a deleted card or pending card (ON DELETE
-- SET NULL) takes the partner, so the partner never stays alone.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION promote_deck_partner()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.commander_id IS NULL AND NEW.commander_pending_id IS NULL THEN
        NEW.commander_id := NEW.partner_id;
        NEW.commander_pending_id := NEW.partner_pending_id;
        NEW.partner_id := NULL;
        NEW.partner_pending_id := NULL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER promote_deck_partner
    BEFORE UPDATE ON tamiyo.deck
    FOR EACH ROW EXECUTE FUNCTION promote_deck_partner();

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
              AND NOT EXISTS (
                  SELECT 1 FROM tamiyo.deck d
                  WHERE d.commander_pending_id = p.id OR d.partner_pending_id = p.id
              )
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

        UPDATE tamiyo.deck
        SET partner_id = NULL, partner_pending_id = pending_id
        WHERE id = link.deck_id AND partner_id = OLD.id;

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

DROP TRIGGER IF EXISTS promote_deck_partner ON tamiyo.deck;
DROP FUNCTION IF EXISTS promote_deck_partner();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION check_deck_commander_ownership()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.commander_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM tamiyo.cards
            WHERE id = NEW.commander_id AND user_id = NEW.user_id
        ) THEN
            RAISE EXCEPTION 'card % does not belong to user %', NEW.commander_id, NEW.user_id
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
-- +goose StatementEnd

ALTER TABLE tamiyo.deck
    DROP CONSTRAINT deck_distinct_pending_commanders,
    DROP CONSTRAINT deck_distinct_commanders,
    DROP CONSTRAINT deck_partner_needs_commander,
    DROP CONSTRAINT deck_single_partner,
    DROP COLUMN partner_pending_id,
    DROP COLUMN partner_id;
