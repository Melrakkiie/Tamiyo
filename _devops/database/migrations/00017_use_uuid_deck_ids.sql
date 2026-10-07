-- +goose Up
-- Decks are identified by a random UUID instead of a sequential integer, so
-- a deck's id can be shared in its URL without letting anyone guess the
-- others. Each deck keeps the share_id it already had as its new id, so share
-- links handed out before this migration keep pointing to the same deck.
ALTER TABLE tamiyo.card_deck DROP CONSTRAINT IF EXISTS card_deck_deck_id_fkey;
ALTER TABLE tamiyo.deck_pending_cards DROP CONSTRAINT IF EXISTS deck_pending_cards_deck_id_fkey;

ALTER TABLE tamiyo.card_deck ADD COLUMN deck_uuid uuid;
UPDATE tamiyo.card_deck cd SET deck_uuid = d.share_id FROM tamiyo.deck d WHERE d.id = cd.deck_id;
ALTER TABLE tamiyo.card_deck DROP CONSTRAINT card_deck_pkey;
DROP INDEX IF EXISTS tamiyo.idx_card_deck_deck_id;
ALTER TABLE tamiyo.card_deck DROP COLUMN deck_id;
ALTER TABLE tamiyo.card_deck RENAME COLUMN deck_uuid TO deck_id;
ALTER TABLE tamiyo.card_deck ADD CONSTRAINT card_deck_pkey PRIMARY KEY (card_id, deck_id);
CREATE INDEX idx_card_deck_deck_id ON tamiyo.card_deck (deck_id);

ALTER TABLE tamiyo.deck_pending_cards ADD COLUMN deck_uuid uuid;
UPDATE tamiyo.deck_pending_cards p SET deck_uuid = d.share_id FROM tamiyo.deck d WHERE d.id = p.deck_id;
DROP INDEX IF EXISTS tamiyo.deck_pending_cards_deck_idx;
ALTER TABLE tamiyo.deck_pending_cards DROP COLUMN deck_id;
ALTER TABLE tamiyo.deck_pending_cards RENAME COLUMN deck_uuid TO deck_id;
ALTER TABLE tamiyo.deck_pending_cards ALTER COLUMN deck_id SET NOT NULL;
CREATE INDEX deck_pending_cards_deck_idx ON tamiyo.deck_pending_cards (deck_id);

ALTER TABLE tamiyo.deck DROP CONSTRAINT deck_pkey;
ALTER TABLE tamiyo.deck DROP COLUMN id;
DROP INDEX IF EXISTS tamiyo.deck_share_id_idx;
ALTER TABLE tamiyo.deck RENAME COLUMN share_id TO id;
ALTER TABLE tamiyo.deck ADD CONSTRAINT deck_pkey PRIMARY KEY (id);

ALTER TABLE tamiyo.card_deck
    ADD CONSTRAINT card_deck_deck_id_fkey FOREIGN KEY (deck_id) REFERENCES tamiyo.deck (id) ON DELETE CASCADE;
ALTER TABLE tamiyo.deck_pending_cards
    ADD CONSTRAINT deck_pending_cards_deck_id_fkey FOREIGN KEY (deck_id) REFERENCES tamiyo.deck (id) ON DELETE CASCADE;

-- +goose Down
-- Gives every deck a new sequential id and keeps its UUID as share_id.
ALTER TABLE tamiyo.card_deck DROP CONSTRAINT IF EXISTS card_deck_deck_id_fkey;
ALTER TABLE tamiyo.deck_pending_cards DROP CONSTRAINT IF EXISTS deck_pending_cards_deck_id_fkey;

ALTER TABLE tamiyo.deck DROP CONSTRAINT deck_pkey;
ALTER TABLE tamiyo.deck RENAME COLUMN id TO share_id;
ALTER TABLE tamiyo.deck ADD COLUMN id SERIAL;
ALTER TABLE tamiyo.deck ADD CONSTRAINT deck_pkey PRIMARY KEY (id);
CREATE UNIQUE INDEX deck_share_id_idx ON tamiyo.deck (share_id);

ALTER TABLE tamiyo.card_deck ADD COLUMN deck_int int;
UPDATE tamiyo.card_deck cd SET deck_int = d.id FROM tamiyo.deck d WHERE d.share_id = cd.deck_id;
ALTER TABLE tamiyo.card_deck DROP CONSTRAINT card_deck_pkey;
DROP INDEX IF EXISTS tamiyo.idx_card_deck_deck_id;
ALTER TABLE tamiyo.card_deck DROP COLUMN deck_id;
ALTER TABLE tamiyo.card_deck RENAME COLUMN deck_int TO deck_id;
ALTER TABLE tamiyo.card_deck ADD CONSTRAINT card_deck_pkey PRIMARY KEY (card_id, deck_id);
CREATE INDEX idx_card_deck_deck_id ON tamiyo.card_deck (deck_id);

ALTER TABLE tamiyo.deck_pending_cards ADD COLUMN deck_int int;
UPDATE tamiyo.deck_pending_cards p SET deck_int = d.id FROM tamiyo.deck d WHERE d.share_id = p.deck_id;
DROP INDEX IF EXISTS tamiyo.deck_pending_cards_deck_idx;
ALTER TABLE tamiyo.deck_pending_cards DROP COLUMN deck_id;
ALTER TABLE tamiyo.deck_pending_cards RENAME COLUMN deck_int TO deck_id;
ALTER TABLE tamiyo.deck_pending_cards ALTER COLUMN deck_id SET NOT NULL;
CREATE INDEX deck_pending_cards_deck_idx ON tamiyo.deck_pending_cards (deck_id);

ALTER TABLE tamiyo.card_deck
    ADD CONSTRAINT card_deck_deck_id_fkey FOREIGN KEY (deck_id) REFERENCES tamiyo.deck (id) ON DELETE CASCADE;
ALTER TABLE tamiyo.deck_pending_cards
    ADD CONSTRAINT deck_pending_cards_deck_id_fkey FOREIGN KEY (deck_id) REFERENCES tamiyo.deck (id) ON DELETE CASCADE;
