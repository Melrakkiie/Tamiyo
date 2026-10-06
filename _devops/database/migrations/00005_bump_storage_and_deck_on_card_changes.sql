-- +goose Up
-- A storage's updated timestamp also moves when a card is put in it, taken
-- out of it or deleted from it, and a deck's when a card is added to it or
-- removed from it, so "last modified" sorting reflects card activity.
-- Statement-level triggers with transition tables touch each storage or deck
-- once per statement, even when an import or a bulk delete changes thousands
-- of cards. Postgres allows a single event per trigger that uses transition
-- tables, hence one trigger per event.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_storages_after_card_insert()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.storage SET updated = now()
    WHERE id IN (SELECT DISTINCT storage_id FROM new_cards WHERE storage_id IS NOT NULL);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_storages_after_card_delete()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.storage SET updated = now()
    WHERE id IN (SELECT DISTINCT storage_id FROM old_cards WHERE storage_id IS NOT NULL);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_storages_after_card_update()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.storage SET updated = now()
    WHERE id IN (
        SELECT o.storage_id FROM old_cards o JOIN new_cards n ON n.id = o.id
        WHERE o.storage_id IS DISTINCT FROM n.storage_id AND o.storage_id IS NOT NULL
        UNION
        SELECT n.storage_id FROM old_cards o JOIN new_cards n ON n.id = o.id
        WHERE o.storage_id IS DISTINCT FROM n.storage_id AND n.storage_id IS NOT NULL
    );
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_decks_after_card_deck_insert()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.deck SET updated = now()
    WHERE id IN (SELECT DISTINCT deck_id FROM new_links);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION touch_decks_after_card_deck_delete()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE tamiyo.deck SET updated = now()
    WHERE id IN (SELECT DISTINCT deck_id FROM old_links);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER touch_storages_after_card_insert
    AFTER INSERT ON tamiyo.cards
    REFERENCING NEW TABLE AS new_cards
    FOR EACH STATEMENT EXECUTE FUNCTION touch_storages_after_card_insert();

CREATE TRIGGER touch_storages_after_card_delete
    AFTER DELETE ON tamiyo.cards
    REFERENCING OLD TABLE AS old_cards
    FOR EACH STATEMENT EXECUTE FUNCTION touch_storages_after_card_delete();

CREATE TRIGGER touch_storages_after_card_update
    AFTER UPDATE ON tamiyo.cards
    REFERENCING OLD TABLE AS old_cards NEW TABLE AS new_cards
    FOR EACH STATEMENT EXECUTE FUNCTION touch_storages_after_card_update();

CREATE TRIGGER touch_decks_after_card_deck_insert
    AFTER INSERT ON tamiyo.card_deck
    REFERENCING NEW TABLE AS new_links
    FOR EACH STATEMENT EXECUTE FUNCTION touch_decks_after_card_deck_insert();

CREATE TRIGGER touch_decks_after_card_deck_delete
    AFTER DELETE ON tamiyo.card_deck
    REFERENCING OLD TABLE AS old_links
    FOR EACH STATEMENT EXECUTE FUNCTION touch_decks_after_card_deck_delete();

-- Backfill from what is already known: the latest card added to a storage,
-- the latest card added to a deck. The modtime triggers would overwrite the
-- value with now(), so they are paused for these two statements.
ALTER TABLE tamiyo.storage DISABLE TRIGGER update_storage_modtime;
UPDATE tamiyo.storage s
SET updated = latest.added
FROM (SELECT storage_id, max(added) AS added FROM tamiyo.cards WHERE storage_id IS NOT NULL GROUP BY storage_id) latest
WHERE latest.storage_id = s.id AND latest.added > s.updated;
ALTER TABLE tamiyo.storage ENABLE TRIGGER update_storage_modtime;

ALTER TABLE tamiyo.deck DISABLE TRIGGER update_deck_modtime;
UPDATE tamiyo.deck d
SET updated = latest.added
FROM (SELECT deck_id, max(added) AS added FROM tamiyo.card_deck GROUP BY deck_id) latest
WHERE latest.deck_id = d.id AND latest.added > d.updated;
ALTER TABLE tamiyo.deck ENABLE TRIGGER update_deck_modtime;

-- +goose Down
DROP TRIGGER IF EXISTS touch_decks_after_card_deck_delete ON tamiyo.card_deck;
DROP TRIGGER IF EXISTS touch_decks_after_card_deck_insert ON tamiyo.card_deck;
DROP TRIGGER IF EXISTS touch_storages_after_card_update ON tamiyo.cards;
DROP TRIGGER IF EXISTS touch_storages_after_card_delete ON tamiyo.cards;
DROP TRIGGER IF EXISTS touch_storages_after_card_insert ON tamiyo.cards;
DROP FUNCTION IF EXISTS touch_decks_after_card_deck_delete();
DROP FUNCTION IF EXISTS touch_decks_after_card_deck_insert();
DROP FUNCTION IF EXISTS touch_storages_after_card_update();
DROP FUNCTION IF EXISTS touch_storages_after_card_delete();
DROP FUNCTION IF EXISTS touch_storages_after_card_insert();
