-- +goose Up
-- Marks a card as a proxy (a printed stand-in rather than a real copy), so
-- the collection can tell what the user actually owns.
ALTER TABLE tamiyo.cards ADD COLUMN IF NOT EXISTS proxy boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tamiyo.cards DROP COLUMN IF EXISTS proxy;
