package deck

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

func (r *PostgresRepository) FindView(ctx context.Context, userID string, deckID string) (View, bool, error) {
	var row struct {
		Grouping        *string        `db:"grouping"`
		Sort            string         `db:"sort"`
		CollapsedBoards pq.StringArray `db:"collapsed_boards"`
	}
	query := `
		SELECT v.grouping, v.sort, v.collapsed_boards
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
	return View{Grouping: row.Grouping, Sort: row.Sort, CollapsedBoards: []string(row.CollapsedBoards)}, true, nil
}

func (r *PostgresRepository) SaveView(ctx context.Context, userID string, deckID string, v View) error {
	result, err := r.db.ExecContext(ctx, `
		INSERT INTO tamiyo.deck_view_settings (deck_id, grouping, sort, collapsed_boards)
		SELECT d.id, $3, $4, $5 FROM tamiyo.deck d WHERE d.id = $2 AND d.user_id = $1
		ON CONFLICT (deck_id) DO UPDATE SET grouping = EXCLUDED.grouping, sort = EXCLUDED.sort, collapsed_boards = EXCLUDED.collapsed_boards
	`, userID, deckID, v.Grouping, v.Sort, pq.Array(v.CollapsedBoards))
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
