-- Script de seed pour données de test / dev
-- À jouer sur une base déjà initialisée par les migrations goose
-- (_devops/database/migrations/ — appliquées automatiquement au démarrage
-- de l'API, voir cmd/api/main.go)
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
    ('11111111-1111-1111-1111-111111111111', 'Black Lotus',       'bd8fa327-dd41-4737-8f19-2cf5eb1f7cdd', 'vma', '4',   false, 1),
    ('11111111-1111-1111-1111-111111111111', 'Ancestral Recall',  '70e7ddf2-5604-41e7-bb9d-ddd03d3e9d0b', 'lea', '47',  false, 1),
    ('11111111-1111-1111-1111-111111111111', 'Time Walk',         '70901356-3266-4bd9-aacc-f06c27271de5', 'vma', '2',   true,  1),

    ('11111111-1111-1111-1111-111111111111', 'Lightning Bolt',    'f29ba16f-c8fb-42fe-aabf-87089cb214a7', '2x2', '117', true,  2),
    ('11111111-1111-1111-1111-111111111111', 'Goblin Guide',      'b2e2a777-0705-4a37-937d-c6e020ebc0f0', 'zen', '126', false, 2),
    ('11111111-1111-1111-1111-111111111111', 'Monastery Swiftspear','b81c6c8b-a9cf-4866-89ba-7f8ad077b836','ktk', '118', true, 2),

    ('11111111-1111-1111-1111-111111111111', 'Sol Ring',          '58b26011-e103-45c4-a253-900f4e6b2eeb', 'cmr', '472', false, 3),
    ('11111111-1111-1111-1111-111111111111', 'Command Tower',     '86424491-a372-40c4-bfb5-faa2b2d41d4c', 'cmr', '350', false, 3),
    ('11111111-1111-1111-1111-111111111111', 'Arcane Signet',     'ee40458c-7f3a-4fa6-976f-be1f7a336fdc', 'cmr', '297', false, 3),
    ('11111111-1111-1111-1111-111111111111', 'Swords to Plowshares','cc9ece2f-7eda-4fc5-a562-3e16e71560e9','sta', '10', false, 3),

    ('11111111-1111-1111-1111-111111111111', 'Tarmogoyf',         '2b49dfb7-dbe7-4a2b-b9de-c620a0db2e47', 'mm3', '141', true,  4),
    ('11111111-1111-1111-1111-111111111111', 'Counterspell',      '1920dae4-fb92-4f19-ae4b-eb3276b8dac7', 'mh2', '267', false, 4),

    ('11111111-1111-1111-1111-111111111111', 'Brainstorm',        '3e4e6787-af32-44f2-ac56-6f348254aa6d', 'ema', '40',  false, 5),
    ('11111111-1111-1111-1111-111111111111', 'Serra Angel',       'b56b9131-4f7e-4912-ba47-63ed82f21d1b', 'dom', '33',  false, 5),
    ('11111111-1111-1111-1111-111111111111', 'Shivan Dragon',     'fefbf149-f988-4f8b-9f53-56f5878116a6', 'lea', '174', false, 5),

    ('11111111-1111-1111-1111-111111111111', 'Llanowar Elves',    '581b7327-3215-4a4f-b4ae-d9d4002ba882', 'dom', '168', false, NULL); -- pas encore classée

--------------------------
---- DECKS (3)         ----
--------------------------
INSERT INTO tamiyo.deck (id, user_id, name, format, commander_id) VALUES
    ('d0000000-0000-4000-8000-000000000001', '11111111-1111-1111-1111-111111111111', 'Burn Aggro', 'modern', NULL),
    ('d0000000-0000-4000-8000-000000000002', '11111111-1111-1111-1111-111111111111', 'Izzet Control', 'legacy', NULL),
    ('d0000000-0000-4000-8000-000000000003', '11111111-1111-1111-1111-111111111111', 'Kess Commander', 'commander', 12); -- commander_id = Counterspell (arbitraire, pour la démo)

-----------------------------
---- CARD_DECK (compo)   ----
-----------------------------
-- Deck 1: Burn Aggro -> Lightning Bolt, Goblin Guide, Monastery Swiftspear
INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES
    (4, 'd0000000-0000-4000-8000-000000000001'),
    (5, 'd0000000-0000-4000-8000-000000000001'),
    (6, 'd0000000-0000-4000-8000-000000000001');

-- Deck 2: Izzet Control -> Counterspell, Brainstorm, Lightning Bolt
INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES
    (12, 'd0000000-0000-4000-8000-000000000002'),
    (13, 'd0000000-0000-4000-8000-000000000002'),
    (4, 'd0000000-0000-4000-8000-000000000002');

-- Deck 3: Kess Commander -> Sol Ring, Command Tower, Arcane Signet, Counterspell (commander)
INSERT INTO tamiyo.card_deck (card_id, deck_id) VALUES
    (7, 'd0000000-0000-4000-8000-000000000003'),
    (8, 'd0000000-0000-4000-8000-000000000003'),
    (9, 'd0000000-0000-4000-8000-000000000003'),
    (12, 'd0000000-0000-4000-8000-000000000003');
