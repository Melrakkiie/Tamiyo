package deck

import (
	"context"

	"github.com/lib/pq"
)

type ownedCopyRow struct {
	ID         int    `db:"id"`
	Name       string `db:"name"`
	ScryfallID string `db:"scryfall_id"`
	Foil       bool   `db:"foil"`
	Proxy      bool   `db:"proxy"`

	InDeck              bool `db:"in_deck"`
	InTargetDeck        bool `db:"in_target_deck"`
	LockedAgainstTarget bool `db:"locked_against_target"`
}

func (r *compositionQueries) FindOwnedCopies(ctx context.Context, userID string, targetDeckID string, scryfallIDs []string, nameKeys []string) ([]OwnedCopy, error) {
	var rows []ownedCopyRow
	query := `
		SELECT c.id, c.name, c.scryfall_id, c.foil, c.proxy,
			EXISTS (SELECT 1 FROM tamiyo.card_deck cd WHERE cd.card_id = c.id) AS in_deck,
			EXISTS (SELECT 1 FROM tamiyo.card_deck cd WHERE cd.card_id = c.id AND cd.deck_id = $4) AS in_target_deck,
			EXISTS (
				SELECT 1 FROM tamiyo.storage s
				WHERE s.id = c.storage_id AND s.user_id = $1 AND s.locked
				  AND NOT EXISTS (SELECT 1 FROM tamiyo.storage_allowed_deck a WHERE a.storage_id = s.id AND a.deck_id = $4)
			) AS locked_against_target
		FROM tamiyo.cards c
		WHERE c.user_id = $1
		  AND (c.scryfall_id::text = ANY($2)
		       OR ` + normalizedNameSQL("c.name") + ` = ANY($3)
		       OR ` + normalizedNameSQL("split_part(replace(c.name, '//', '/'), '/', 1)") + ` = ANY($3))
		ORDER BY c.id`
	if err := r.db.SelectContext(ctx, &rows, query, userID, pq.Array(scryfallIDs), pq.Array(nameKeys), targetDeckID); err != nil {
		return nil, err
	}
	copies := make([]OwnedCopy, 0, len(rows))
	for _, row := range rows {
		copies = append(copies, OwnedCopy(row))
	}
	return copies, nil
}
