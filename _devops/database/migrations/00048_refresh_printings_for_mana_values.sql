-- +goose Up
-- The background printing refresher now also sets the mana value of every card
-- and pending card of a printing, and multi-face cards with no top-level mana
-- value (reversible cards) were stored at 0. Every printing is marked stale so
-- the refresher fixes them within minutes.
UPDATE tamiyo.printings SET refreshed_at = 'epoch';

-- +goose Down
-- Refresh dates are only a schedule: nothing to undo.
SELECT 1;
