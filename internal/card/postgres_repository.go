package card

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type cardRow struct {
	ID              int    `db:"id"`
	Name            string `db:"name"`
	ScryfallID      string `db:"scryfall_id"`
	SetCode         string `db:"set_code"`
	CollectorNumber int    `db:"collector_number"`
	Foil            bool   `db:"foil"`
	BinderName      string `db:"binder_name"`
	BinderType      string `db:"binder_type"`
	Added           string `db:"added"`
}

func (r cardRow) toDomain() Card {
	added, _ := time.Parse("2006-01-02 15:04:05", r.Added)
	return Card{
		ID:              r.ID,
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		BinderName:      r.BinderName,
		BinderType:      r.BinderType,
		Added:           added,
	}
}

// PostgresRepository implémente card.Repository pour Postgres.
type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context) ([]Card, error) {
	var rows []cardRow

	query := `
		SELECT id, name, scryfall_id, set_code, collector_number, foil, binder_name, binder_type, added
		FROM tamiyo.cards
	`

	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}

	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, row.toDomain())
	}

	return cards, nil
}
