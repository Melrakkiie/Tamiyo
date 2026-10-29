-- +goose Up
-- A history row remembers the printing and finish of one of its copies, outside
-- its key (ADR 0013). Rows recorded before this migration keep no annotation.
ALTER TABLE tamiyo.deck_history
    ADD COLUMN set_code         text,
    ADD COLUMN collector_number text,
    ADD COLUMN foil             bool;

-- A movement overwrites the annotation only when its sign matches the row's
-- count after it, so an addition names an added copy and a removal a removed one.
-- A movement that flips the count's sign therefore sets it.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION add_deck_history(p_deck_id uuid, p_board text, p_name text, p_delta int,
                                            p_set_code text, p_collector_number text, p_foil bool)
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
    UPDATE tamiyo.deck_history SET
        count            = count + p_delta,
        set_code         = CASE WHEN sign(count + p_delta) = sign(p_delta) THEN p_set_code ELSE set_code END,
        collector_number = CASE WHEN sign(count + p_delta) = sign(p_delta) THEN p_collector_number ELSE collector_number END,
        foil             = CASE WHEN sign(count + p_delta) = sign(p_delta) THEN p_foil ELSE foil END
    WHERE deck_id = p_deck_id AND day = today AND board = p_board AND name = p_name;
    IF NOT FOUND THEN
        INSERT INTO tamiyo.deck_history (deck_id, day, board, name, count, set_code, collector_number, foil)
        VALUES (p_deck_id, today, p_board, p_name, p_delta, p_set_code, p_collector_number, p_foil)
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
    old_card record;
    new_card record;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        SELECT name, set_code, collector_number, foil INTO old_card FROM tamiyo.cards WHERE id = OLD.card_id;
        -- Card gone: it is being deleted from the collection, and
        -- record_deleted_card_history already took it out of its decks.
        IF old_card.name IS NOT NULL THEN
            PERFORM add_deck_history(OLD.deck_id, OLD.board, old_card.name, -1, old_card.set_code, old_card.collector_number, old_card.foil);
        END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        SELECT name, set_code, collector_number, foil INTO new_card FROM tamiyo.cards WHERE id = NEW.card_id;
        PERFORM add_deck_history(NEW.deck_id, NEW.board, new_card.name, 1, new_card.set_code, new_card.collector_number, new_card.foil);
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
        PERFORM add_deck_history(link.deck_id, link.board, OLD.name, -1, OLD.set_code, OLD.collector_number, OLD.foil);
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
        PERFORM add_deck_history(NEW.deck_id, NEW.board, NEW.name, NEW.quantity - OLD.quantity, NEW.set_code, NEW.collector_number, NEW.foil);
        RETURN NULL;
    END IF;
    IF TG_OP <> 'INSERT' THEN
        PERFORM add_deck_history(OLD.deck_id, OLD.board, OLD.name, -OLD.quantity, OLD.set_code, OLD.collector_number, OLD.foil);
    END IF;
    IF TG_OP <> 'DELETE' THEN
        PERFORM add_deck_history(NEW.deck_id, NEW.board, NEW.name, NEW.quantity, NEW.set_code, NEW.collector_number, NEW.foil);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP FUNCTION IF EXISTS add_deck_history(uuid, text, text, int);

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION add_deck_history(p_deck_id uuid, p_board text, p_name text, p_delta int)
RETURNS void AS $$
DECLARE
    today date;
BEGIN
    IF p_delta = 0 THEN
        RETURN;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM tamiyo.deck WHERE id = p_deck_id) THEN
        RETURN;
    END IF;
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

DROP FUNCTION IF EXISTS add_deck_history(uuid, text, text, int, text, text, bool);

ALTER TABLE tamiyo.deck_history
    DROP COLUMN foil,
    DROP COLUMN collector_number,
    DROP COLUMN set_code;
