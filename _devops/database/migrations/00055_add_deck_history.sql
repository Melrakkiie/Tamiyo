-- +goose Up
-- Net change in copies per deck, day, board and card name (ADR 0013). A row
-- that nets to zero is deleted, so a name added then removed leaves no line.
CREATE TABLE tamiyo.deck_history (
    deck_id uuid NOT NULL REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    day     date NOT NULL,
    board   text NOT NULL CHECK (board IN ('main', 'sideboard', 'considering')),
    name    text NOT NULL,
    count   int  NOT NULL, -- never 0 at rest: add_deck_history deletes a row that reaches it
    PRIMARY KEY (deck_id, day, board, name)
);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION add_deck_history(p_deck_id uuid, p_board text, p_name text, p_delta int)
RETURNS void AS $$
DECLARE
    today date;
BEGIN
    IF p_delta = 0 THEN
        RETURN;
    END IF;
    -- A deck being deleted cascades into its cards: nothing to record for it.
    IF NOT EXISTS (SELECT 1 FROM tamiyo.deck WHERE id = p_deck_id) THEN
        RETURN;
    END IF;
    -- shortcut: day in Europe/Paris, switch to the viewer's time zone if the audience goes international
    today := (now() AT TIME ZONE 'Europe/Paris')::date;
    UPDATE tamiyo.deck_history SET count = count + p_delta
    WHERE deck_id = p_deck_id AND day = today AND board = p_board AND name = p_name;
    IF NOT FOUND THEN
        INSERT INTO tamiyo.deck_history (deck_id, day, board, name, count)
        VALUES (p_deck_id, today, p_board, p_name, p_delta)
        ON CONFLICT (deck_id, day, board, name) DO UPDATE SET count = tamiyo.deck_history.count + EXCLUDED.count;
    END IF;
    DELETE FROM tamiyo.deck_history
    WHERE deck_id = p_deck_id AND day = today AND board = p_board AND name = p_name AND count = 0;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_card_deck_history()
RETURNS TRIGGER AS $$
DECLARE
    old_name text;
    new_name text;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        SELECT name INTO old_name FROM tamiyo.cards WHERE id = OLD.card_id;
        -- Card gone: it is being deleted from the collection, and
        -- record_deleted_card_history already took it out of its decks.
        IF old_name IS NOT NULL THEN
            PERFORM add_deck_history(OLD.deck_id, OLD.board, old_name, -1);
        END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        SELECT name INTO new_name FROM tamiyo.cards WHERE id = NEW.card_id;
        PERFORM add_deck_history(NEW.deck_id, NEW.board, new_name, 1);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_deleted_card_history()
RETURNS TRIGGER AS $$
DECLARE
    link record;
BEGIN
    FOR link IN SELECT deck_id, board FROM tamiyo.card_deck WHERE card_id = OLD.id LOOP
        PERFORM add_deck_history(link.deck_id, link.board, OLD.name, -1);
    END LOOP;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION record_pending_card_history()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.deck_id = NEW.deck_id AND OLD.board = NEW.board THEN
        -- Same slot: only the quantity counts, a rename is not a deck change.
        PERFORM add_deck_history(NEW.deck_id, NEW.board, NEW.name, NEW.quantity - OLD.quantity);
        RETURN NULL;
    END IF;
    IF TG_OP <> 'INSERT' THEN
        PERFORM add_deck_history(OLD.deck_id, OLD.board, OLD.name, -OLD.quantity);
    END IF;
    IF TG_OP <> 'DELETE' THEN
        PERFORM add_deck_history(NEW.deck_id, NEW.board, NEW.name, NEW.quantity);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER record_card_deck_history
    AFTER INSERT OR UPDATE OR DELETE ON tamiyo.card_deck
    FOR EACH ROW EXECUTE FUNCTION record_card_deck_history();

CREATE TRIGGER record_deleted_card_history
    BEFORE DELETE ON tamiyo.cards
    FOR EACH ROW EXECUTE FUNCTION record_deleted_card_history();

CREATE TRIGGER record_pending_card_history
    AFTER INSERT OR UPDATE OR DELETE ON tamiyo.deck_pending_cards
    FOR EACH ROW EXECUTE FUNCTION record_pending_card_history();

-- +goose Down
DROP TRIGGER IF EXISTS record_pending_card_history ON tamiyo.deck_pending_cards;
DROP TRIGGER IF EXISTS record_deleted_card_history ON tamiyo.cards;
DROP TRIGGER IF EXISTS record_card_deck_history ON tamiyo.card_deck;
DROP FUNCTION IF EXISTS record_pending_card_history();
DROP FUNCTION IF EXISTS record_deleted_card_history();
DROP FUNCTION IF EXISTS record_card_deck_history();
DROP FUNCTION IF EXISTS add_deck_history(uuid, text, text, int);
DROP TABLE IF EXISTS tamiyo.deck_history;
