-- +goose Up
-- English Scryfall name of a printing. The background refresher renames the
-- cards and pending cards of that printing to it, so names imported in another
-- language (ManaBox exports the printed name) end up in English, and moves the
-- deck tags keyed on the old name. Every printing is marked stale so the
-- refresher fills the column within minutes.
ALTER TABLE tamiyo.printings
    ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '';

UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
ALTER TABLE tamiyo.printings
    DROP COLUMN IF EXISTS name;
