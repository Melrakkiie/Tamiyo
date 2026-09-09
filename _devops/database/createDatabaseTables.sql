CREATE SCHEMA IF NOT EXISTS tamiyo;

------------------------
---- STORAGE TABLE  ----
------------------------
CREATE TABLE IF NOT EXISTS tamiyo.storage
(
    id      SERIAL                      PRIMARY KEY,
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
    name             text                       NOT NULL,
    scryfall_id      uuid                       NOT NULL,
    set_code         text                       NOT NULL,
    collector_number int                        NOT NULL,
    foil             bool                       NOT NULL,
    storage_id       int,
    added            TIMESTAMP WITH TIME ZONE   NOT NULL DEFAULT now(),
    updated          TIMESTAMP WITH TIME ZONE   NOT NULL DEFAULT now(),
    FOREIGN KEY (storage_id) REFERENCES tamiyo.storage(id)
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

CREATE TRIGGER update_storage_modtime
    BEFORE UPDATE ON tamiyo.storage
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();

CREATE TRIGGER update_cards_modtime
    BEFORE UPDATE ON tamiyo.cards
    FOR EACH ROW EXECUTE FUNCTION update_modified_column();
