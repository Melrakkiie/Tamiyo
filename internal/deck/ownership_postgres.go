package deck

import (
	"context"

	"github.com/lib/pq"
)

func (r *PostgresRepository) CountCopiesByName(ctx context.Context, userID string, nameKeys []string) (map[string]int, error) {
	counts := make(map[string]int, len(nameKeys))
	if len(nameKeys) == 0 {
		return counts, nil
	}
	var rows []struct {
		NameKey string `db:"name_key"`
		Count   int    `db:"copies"`
	}
	query := `
		SELECT ` + normalizedNameSQL("c.name") + ` AS name_key, count(*) AS copies
		FROM tamiyo.cards c
		WHERE c.user_id = $1 AND ` + normalizedNameSQL("c.name") + ` = ANY($2)
		GROUP BY name_key`
	if err := r.db.SelectContext(ctx, &rows, query, userID, pq.Array(nameKeys)); err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.NameKey] = row.Count
	}
	return counts, nil
}
