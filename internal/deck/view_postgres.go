package deck

import (
	"context"
	"database/sql"
	"errors"
)

func (r *PostgresRepository) FindView(ctx context.Context, userID string, deckID string) (View, bool, error) {
	var row struct {
		Grouping *string `db:"grouping"`
		Sort     string  `db:"sort"`
	}
	query := `
		SELECT v.grouping, v.sort
		FROM tamiyo.deck_view_settings v
		JOIN tamiyo.deck d ON d.id = v.deck_id
		WHERE d.user_id = $1 AND v.deck_id = $2
	`
	if err := r.db.GetContext(ctx, &row, query, userID, deckID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return View{}, false, nil
		}
		return View{}, false, err
	}
	return View{Grouping: row.Grouping, Sort: row.Sort}, true, nil
}

func (r *PostgresRepository) SaveView(ctx context.Context, userID string, deckID string, v View) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO tamiyo.deck_view_settings (deck_id, grouping, sort)
		SELECT d.id, $3, $4 FROM tamiyo.deck d WHERE d.id = $2 AND d.user_id = $1
		ON CONFLICT (deck_id) DO UPDATE SET grouping = EXCLUDED.grouping, sort = EXCLUDED.sort
	`, userID, deckID, v.Grouping, v.Sort)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
