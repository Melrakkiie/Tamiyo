-- +goose Up
-- A storage holding a deck is now typed "deckbox" instead of "deck", so it
-- isn't mistaken for a deck itself.
UPDATE tamiyo.storage SET type = 'deckbox' WHERE lower(trim(type)) = 'deck';

-- +goose Down
-- Storages already typed "deckbox" before this migration can't be told apart
-- from the renamed ones: nothing to undo.
SELECT 1;
