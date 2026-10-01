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

----------------------------
---- REFRESH_TOKENS TABLE ----
----------------------------
-- One row per issued refresh token. Only the SHA-256 hash of the token is
-- stored — the plaintext is returned to the client once, at issuance, and
-- never persisted. A token is used at most once: refreshing revokes the
-- old row (revoked_at set) and inserts a new one (see internal/token).
CREATE TABLE IF NOT EXISTS tamiyo.refresh_tokens
(
    id         UUID                        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID                        NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    token_hash text                        NOT NULL UNIQUE,
    added      TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    expires_at TIMESTAMP WITH TIME ZONE    NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE
);

-------------------------------------
---- PASSWORD_RESET_TOKENS TABLE ----
-------------------------------------
-- Same shape and rationale as refresh_tokens (only the hash is stored), but
-- for "forgot password" recovery: a token is single-use (used_at set once
-- consumed by POST /auth/reset-password) and short-lived (see
-- PASSWORD_RESET_TOKEN_TTL_MINUTES, internal/passwordreset).
CREATE TABLE IF NOT EXISTS tamiyo.password_reset_tokens
(
    id         UUID                        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID                        NOT NULL REFERENCES tamiyo.users(id) ON DELETE CASCADE,
    token_hash text                        NOT NULL UNIQUE,
    added      TIMESTAMP WITH TIME ZONE    NOT NULL DEFAULT now(),
    expires_at TIMESTAMP WITH TIME ZONE    NOT NULL,
    used_at    TIMESTAMP WITH TIME ZONE
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

-----------------
---- INDEXES ----
-----------------
-- Every query in every repository filters on user_id (multi-tenant
-- scoping), so it is the single highest-value index across the schema.
-- Composite indexes below put user_id first so they also serve plain
-- "WHERE user_id = $1" lookups (leftmost-prefix rule), not just the
-- combination with a second filter.

-- storage: scoped by user_id, optionally filtered by type (GET /storage?type=)
CREATE INDEX IF NOT EXISTS idx_storage_user_id_type
    ON tamiyo.storage (user_id, lower(type));

-- deck: scoped by user_id, optionally filtered by format (GET /deck?format=)
CREATE INDEX IF NOT EXISTS idx_deck_user_id_format
    ON tamiyo.deck (user_id, lower(format));

-- cards: scoped by user_id on every card query, and joined on storage_id
-- by storage.FindAll's card_count LEFT JOIN.
CREATE INDEX IF NOT EXISTS idx_cards_user_id
    ON tamiyo.cards (user_id);

CREATE INDEX IF NOT EXISTS idx_cards_storage_id
    ON tamiyo.cards (storage_id)
    WHERE storage_id IS NOT NULL;

-- card_deck: the primary key (card_id, deck_id) only accelerates lookups
-- by card_id (or both columns). deck.FindAll's card_count and
-- FindCardsByDeckID both filter by deck_id alone, which needs its own
-- index since deck_id is not the leftmost PK column.
CREATE INDEX IF NOT EXISTS idx_card_deck_deck_id
    ON tamiyo.card_deck (deck_id);

-- refresh_tokens: looked up by its hash on every /auth/refresh and
-- /auth/logout call (token_hash is already UNIQUE, which Postgres backs
-- with an index automatically, but it's listed here for visibility), and
-- revoked in bulk by user_id (password change, reuse detection).
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id
    ON tamiyo.refresh_tokens (user_id);

-- password_reset_tokens: same rationale as refresh_tokens above.
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id
    ON tamiyo.password_reset_tokens (user_id);
