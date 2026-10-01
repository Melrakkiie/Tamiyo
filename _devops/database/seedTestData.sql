-- Script de seed pour données de test / dev
-- À jouer après createDatabaseTables.sql, sur une base déjà initialisée
-- (par exemple: docker exec -i tamiyo-db psql -U login -d tamiyo_db < seedTestData.sql)

-- pgcrypto fournit crypt()/gen_salt('bf'), qui génère un hash bcrypt
-- ($2a$...) au format strictement identique à celui produit par
-- golang.org/x/crypto/bcrypt côté API. C'est un module "contrib"
-- standard, livré avec l'image Docker officielle postgres.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Nettoyage (pratique pour rejouer le script plusieurs fois en dev)
TRUNCATE TABLE tamiyo.card_deck, tamiyo.deck, tamiyo.cards, tamiyo.storage, tamiyo.users
RESTART IDENTITY CASCADE;

------------------------
---- USER (1)       ----
------------------------
-- Identifiants de test : seed@tamiyo.local / password123
-- Un id fixe est utilisé pour pouvoir le référencer explicitement
-- dans les INSERT ci-dessous (storage/cards/deck.user_id).
-- gen_salt('bf', 10): coût 10, identique à bcrypt.DefaultCost côté Go,
-- pour que le hash généré ici soit représentatif de ceux produits par l'API.
INSERT INTO tamiyo.users (id, email, password_hash) VALUES
    ('11111111-1111-1111-1111-111111111111', 'seed@tamiyo.local', crypt('password123', gen_salt('bf', 10)));

------------------------
---- STORAGES (5)   ----
------------------------
INSERT INTO tamiyo.storage (user_id, name, type) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Vintage Collection', 'binder'),      -- id 1
    ('11111111-1111-1111-1111-111111111111', 'Red Deck Wins', 'deckbox'),          -- id 2
    ('11111111-1111-1111-1111-111111111111', 'Commander Staples', 'binder'),       -- id 3
    ('11111111-1111-1111-1111-111111111111', 'Boîte Tarkir', 'box'),               -- id 4
    ('11111111-1111-1111-1111-111111111111', 'Trade Binder', 'binder');            -- id 5

------------------------
---- CARDS (16)     ----
------------------------
INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, storage_id) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Black Lotus',       'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'lea', '232', false, 1),
    ('11111111-1111-1111-1111-111111111111', 'Ancestral Recall',  'b0faa7f2-1c58-4fac-8b41-2c3ce4c1de29', 'lea', '1',   false, 1),
    ('11111111-1111-1111-1111-111111111111', 'Time Walk',         '7c9f0e2a-4f1c-4b6a-9d2f-8a3c1e5b7d90', 'lea', '80',  true,  1),

    ('11111111-1111-1111-1111-111111111111', 'Lightning Bolt',    '9d5e9a7b-3f4c-4a2e-8b1d-6c7f8a9b0c1d', '2xm', '129', true,  2),
    ('11111111-1111-1111-1111-111111111111', 'Goblin Guide',      '4e2a1b3c-5d6f-4789-9a0b-1c2d3e4f5a6b', 'zen', '96',  false, 2),
    ('11111111-1111-1111-1111-111111111111', 'Monastery Swiftspear','1f2e3d4c-5b6a-4978-8e7d-6c5b4a3f2e1d','ktk', '116', true, 2),

    ('11111111-1111-1111-1111-111111111111', 'Sol Ring',          'f2c8b1a0-1e2d-4c3b-9a8f-7e6d5c4b3a2f', 'cmr', '322', false, 3),
    ('11111111-1111-1111-1111-111111111111', 'Command Tower',     '6a5b4c3d-2e1f-4a9b-8c7d-5e4f3a2b1c0d', 'cmr', '331', false, 3),
    ('11111111-1111-1111-1111-111111111111', 'Arcane Signet',     '3d4c5b6a-7e8f-4901-9a2b-3c4d5e6f7a8b', 'cmr', '333', false, 3),
    ('11111111-1111-1111-1111-111111111111', 'Swords to Plowshares','8b7a6c5d-4e3f-4201-9c8b-7a6d5e4f3c2b','stx', '30', false, 3),

    ('11111111-1111-1111-1111-111111111111', 'Tarmogoyf',         '3a1b2c3d-4e5f-6789-0abc-def123456789', 'mm3', '156', true,  4),
    ('11111111-1111-1111-1111-111111111111', 'Counterspell',      '1b3f2f0c-4a8e-4c3d-9f2a-7e5b6c8d9a1f', 'mh2', '267', false, 4),

    ('11111111-1111-1111-1111-111111111111', 'Brainstorm',        '2c1d0e9f-8a7b-4c6d-5e4f-3a2b1c0d9e8f', 'ema', '42',  false, 5),
    ('11111111-1111-1111-1111-111111111111', 'Serra Angel',       '9e8d7c6b-5a4f-4321-8b0a-9c8d7e6f5a4b', 'dom', '25',  false, 5),
    ('11111111-1111-1111-1111-111111111111', 'Shivan Dragon',     '5f4e3d2c-1b0a-4987-9e8d-7c6b5a4f3e2d', 'lea', '175', false, 5),

    ('11111111-1111-1111-1111-111111111111', 'Llanowar Elves',    '0a9b8c7d-6e5f-4432-9a1b-0c9d8e7f6a5b', 'dom', '175', false, NULL); -- pas encore classée

--------------------------
---- DECKS (3)         ----
--------------------------
INSERT INTO tamiyo.deck (user_id, name, format, commander_id) VALUES
    ('11111111-1111-1111-1111-111111111111', 'Burn Aggro', 'modern', NULL),
    ('11111111-1111-1111-1111-111111111111', 'Izzet Control', 'legacy', NULL),
    ('11111111-1111-1111-1111-111111111111', 'Kess Commander', 'commander', 12); -- commander_id = Counterspell (arbitraire, pour la démo)

-----------------------------
---- CARD_DECK (compo)   ----
-----------------------------
-- Deck 1: Burn Aggro -> Lightning Bolt, Goblin Guide, Monastery Swiftspear
INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES
    (4, 1),
    (5, 1),
    (6, 1);

-- Deck 2: Izzet Control -> Counterspell, Brainstorm, Lightning Bolt
INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES
    (12, 2),
    (13, 2),
    (4, 2);

-- Deck 3: Kess Commander -> Sol Ring, Command Tower, Arcane Signet, Counterspell (commander)
INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES
    (7, 3),
    (8, 3),
    (9, 3),
    (12, 3);
