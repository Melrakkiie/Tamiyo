-- +goose Up
-- The Commander bracket the owner gives the deck, from 1 (Exhibition) to
-- 5 (cEDH). NULL when not set.
ALTER TABLE tamiyo.deck ADD COLUMN IF NOT EXISTS bracket smallint CHECK (bracket BETWEEN 1 AND 5);

-- +goose Down
ALTER TABLE tamiyo.deck DROP COLUMN IF EXISTS bracket;
