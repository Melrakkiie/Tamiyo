package printing

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) IDsToRefresh(ctx context.Context, staleBefore time.Time, limit int) ([]string, error) {
	var ids []string
	err := r.db.SelectContext(ctx, &ids, `
		SELECT c.scryfall_id::text
		FROM (SELECT DISTINCT scryfall_id FROM tamiyo.cards) c
		LEFT JOIN tamiyo.printings p ON p.scryfall_id = c.scryfall_id
		WHERE p.scryfall_id IS NULL OR p.refreshed_at < $1
		ORDER BY p.refreshed_at NULLS FIRST, c.scryfall_id
		LIMIT $2
	`, staleBefore, limit)
	return ids, err
}

func (r *PostgresRepository) Upsert(ctx context.Context, printings []Printing) error {
	if len(printings) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	for _, p := range printings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO tamiyo.printings (scryfall_id, type_line, legal_formats, refreshed_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (scryfall_id) DO UPDATE
			SET type_line = EXCLUDED.type_line, legal_formats = EXCLUDED.legal_formats, refreshed_at = now()
		`, p.ScryfallID, p.TypeLine, pq.Array(p.LegalFormats)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
