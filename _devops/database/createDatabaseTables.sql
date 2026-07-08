CREATE SCHEMA IF NOT EXISTS tamiyo;

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
    binder_name      text      NOT NULL,
    binder_type      text      NOT NULL,
    added            timestamp NOT NULL
);
