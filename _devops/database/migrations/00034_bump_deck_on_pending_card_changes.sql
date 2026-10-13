-- +goose Up
-- A deck's updated timestamp also moves when one of its pending cards is
-- added, removed, moved to another board or changes quantity, as it already
-- does for collection cards. Switching a pending card's finish replaces it,
-- so it counts too. One statement-level trigger per event, as transition
-- tables require.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_decks_after_pending_card_insert()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.deck SET updated = now()
    WHERE id IN (SELECT DISTINCT deck_id FROM new_pending);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_decks_after_pending_card_update()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.deck SET updated = now()
    WHERE id IN (SELECT DISTINCT deck_id FROM new_pending);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_decks_after_pending_card_delete()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.deck SET updated = now()
    WHERE id IN (SELECT DISTINCT deck_id FROM old_pending);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER touch_decks_after_pending_card_insert
    AFTER INSERT ON tamiyo.deck_pending_cards
    REFERENCING NEW TABLE AS new_pending
    FOR EACH STATEMENT EXECUTE FUNCTION touch_decks_after_pending_card_insert();

CREATE TRIGGER touch_decks_after_pending_card_update
    AFTER UPDATE ON tamiyo.deck_pending_cards
    REFERENCING NEW TABLE AS new_pending
    FOR EACH STATEMENT EXECUTE FUNCTION touch_decks_after_pending_card_update();

CREATE TRIGGER touch_decks_after_pending_card_delete
    AFTER DELETE ON tamiyo.deck_pending_cards
    REFERENCING OLD TABLE AS old_pending
    FOR EACH STATEMENT EXECUTE FUNCTION touch_decks_after_pending_card_delete();

-- +goose Down
DROP TRIGGER IF EXISTS touch_decks_after_pending_card_delete ON tamiyo.deck_pending_cards;
DROP TRIGGER IF EXISTS touch_decks_after_pending_card_update ON tamiyo.deck_pending_cards;
DROP TRIGGER IF EXISTS touch_decks_after_pending_card_insert ON tamiyo.deck_pending_cards;
DROP FUNCTION IF EXISTS touch_decks_after_pending_card_delete();
DROP FUNCTION IF EXISTS touch_decks_after_pending_card_update();
DROP FUNCTION IF EXISTS touch_decks_after_pending_card_insert();
