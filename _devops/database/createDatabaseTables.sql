CREATE SCHEMA IF NOT EXISTS tamiyo;

---------------------
---- USERS TABLE ----
---------------------
CREATE TABLE IF NOT EXISTS tamiyo.users
(
    id            UUID                        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text                        NOT NULL UNIQUE,
    password_hash text                        NOT NULL,
    added         TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    updated       TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now()
);

------------------------
---- STORAGE TABLE  ----
------------------------
CREATE TABLE IF NOT EXISTS tamiyo.storage
(
    id      SERIAL                      PRIMARY KEY,
    user_id UUID                        NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    name    text                        NOT NULL,
    type    text                        NOT NULL,
    added   TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    updated TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now()
);

----------------------
---- CARDS TABLE  ----
----------------------
CREATE TABLE IF NOT EXISTS tamiyo.cards
(
    id               SERIAL                     PRIMARY KEY,
    user_id          UUID                       NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    name             text                       NOT NULL,
    scryfall_id      uuid                       NOT NULL,
    set_code         text                       NOT NULL,
    collector_number text                       NOT NULL,
    foil             bool                       NOT NULL,
    storage_id       int                        REFERENCES tamiyo.storage(id) ON DELETE SET NULL,
    added            TIMESTAMP WITH TIME ZONE   NOT NULL DEFAULT now(),
    updated          TIMESTAMP WITH TIME ZONE   NOT NULL DEFAULT now()
);

---------------------
---- DECK TABLE  ----
---------------------
CREATE TABLE IF NOT EXISTS tamiyo.deck
(
    id           SERIAL                      PRIMARY KEY,
    user_id      UUID                        NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    name         text                        NOT NULL,
    format       text                        NOT NULL,
    commander_id int                         REFERENCES tamiyo.cards(id) ON DELETE SET NULL,
    added        TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    updated      TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now()
);

-------------------------
---- CARD_DECK TABLE ----
-------------------------
CREATE TABLE IF NOT EXISTS tamiyo.card_deck
(
    card_id    int                         REFERENCES tamiyo.cards(id) ON DELETE CASCADE,
    deck_id    int                         REFERENCES tamiyo.deck(id) ON DELETE CASCADE,
    added      TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    CONSTRAINT card_deck_pkey              PRIMARY KEY (card_id, deck_id)
);

-------------------------
---- UPDATED TRIGGER ----
-------------------------
CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated = now();
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE TRIGGER update_users_modtime
    BEFORE UPDATE ON tamiyo.users
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();

CREATE TRIGGER update_storage_modtime
    BEFORE UPDATE ON tamiyo.storage
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();

CREATE TRIGGER update_cards_modtime
    BEFORE UPDATE ON tamiyo.cards
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();

CREATE TRIGGER update_deck_modtime
    BEFORE UPDATE ON tamiyo.deck
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();

-------------------------------------
---- CROSS-OWNERSHIP ENFORCEMENT ----
-------------------------------------

CREATE OR REPLACE FUNCTION check_card_storage_ownership()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.storage_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM tamiyo.storage
            WHERE id = NEW.storage_id AND user_id = NEW.user_id
        ) THEN
            RAISE EXCEPTION 'storage % does not belong to user %', NEW.storage_id, NEW.user_id
                USING ERRCODE = 'foreign_key_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE TRIGGER check_cards_storage_ownership
    BEFORE INSERT OR UPDATE ON tamiyo.cards
    FOR EACH ROW EXECUTE FUNCTION check_card_storage_ownership();


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

CREATE TRIGGER check_deck_commander_ownership
    BEFORE INSERT OR UPDATE ON tamiyo.deck
    FOR EACH ROW EXECUTE FUNCTION check_deck_commander_ownership();
