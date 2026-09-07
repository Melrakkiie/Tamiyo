CREATE SCHEMA IF NOT EXISTS tamiyo;


------------------------
---- STORAGE TABLE  ----
------------------------

CREATE TABLE IF NOT EXISTS tamiyo.storage
(
    id               SERIAL    PRIMARY KEY,
    name             text      NOT NULL,
    type             text      NOT NULL,
    added            timestamp NOT NULL
);


----------------------
---- CARDS TABLE  ----
----------------------

CREATE TABLE IF NOT EXISTS tamiyo.cards
(
    id               SERIAL    PRIMARY KEY,
    name             text      NOT NULL,
    scryfall_id      uuid      NOT NULL,
    set_code         text      NOT NULL,
    collector_number int       NOT NULL,
    foil             bool      NOT NULL,
    storage_id       int               ,
    added            timestamp NOT NULL,
    FOREIGN KEY (storage_id) REFERENCES tamiyo.storage(id)
);
