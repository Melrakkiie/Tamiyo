-- +goose Up
-- A pending card remembers whether it stands for a proxy, so switching a deck
-- card to proxy without a free proxy copy keeps that information, and
-- committing the pending card creates a proxy in the collection.
ALTER TABLE tamiyo.deck_pending_cards ADD COLUMN IF NOT EXISTS proxy boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tamiyo.deck_pending_cards DROP COLUMN IF EXISTS proxy;
