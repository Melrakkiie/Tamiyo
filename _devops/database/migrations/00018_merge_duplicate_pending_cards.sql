-- +goose Up
-- A deck now keeps a single pending row per printing and finish, raising its
-- quantity when the same card is added again. Merge the rows created before
-- that: each group keeps its oldest row with the summed quantity. A row that
-- is a deck's pending commander stays on its own.
WITH groups AS (
    SELECT p.id, p.quantity,
           min(p.id) OVER (PARTITION BY p.deck_id, p.scryfall_id, p.foil) AS keep_id
    FROM tamiyo.deck_pending_cards p
    WHERE NOT EXISTS (SELECT 1 FROM tamiyo.deck d WHERE d.commander_pending_id = p.id)
), totals AS (
    SELECT keep_id, sum(quantity) AS total
    FROM groups
    GROUP BY keep_id
    HAVING count(*) > 1
)
UPDATE tamiyo.deck_pending_cards p
SET quantity = t.total
FROM totals t
WHERE p.id = t.keep_id;

DELETE FROM tamiyo.deck_pending_cards p
USING (
    SELECT q.id, min(q.id) OVER (PARTITION BY q.deck_id, q.scryfall_id, q.foil) AS keep_id
    FROM tamiyo.deck_pending_cards q
    WHERE NOT EXISTS (SELECT 1 FROM tamiyo.deck d WHERE d.commander_pending_id = q.id)
) g
WHERE p.id = g.id AND g.id <> g.keep_id;

-- +goose Down
-- Merged rows can't be told apart again: nothing to undo.
SELECT 1;
