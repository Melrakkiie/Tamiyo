package card

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type cardRow struct {
	ID              int       `db:"id"`
	Name            string    `db:"name"`
	ScryfallID      string    `db:"scryfall_id"`
	SetCode         string    `db:"set_code"`
	CollectorNumber int       `db:"collector_number"`
	Foil            bool      `db:"foil"`
	StorageID       *int      `db:"storage_id"`
	Added           time.Time `db:"added"`
	Updated         time.Time `db:"updated"`
}

func (r cardRow) toDomain() Card {
	return Card{
		ID:              r.ID,
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		StorageID:       r.StorageID,
		Added:           r.Added,
		Updated:         r.Updated,
	}
}

func toCardRow(c Card) cardRow {
	return cardRow{
		Name:            c.Name,
		ScryfallID:      c.ScryfallID,
		SetCode:         c.SetCode,
		CollectorNumber: c.CollectorNumber,
		Foil:            c.Foil,
		StorageID:       c.StorageID,
	}
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context, storageID *int) ([]Card, error) {
	query := `
    SELECT id, name, scryfall_id, set_code, collector_number, foil, storage_id, added, updated
    FROM tamiyo.cards
	`
	args := []interface{}{}

	if storageID != nil {
		query += ` WHERE storage_id = $1`
		args = append(args, *storageID)
	}

	query += ` ORDER BY updated DESC`

	var rows []cardRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, row.toDomain())
	}

	return cards, nil
}

func (r *PostgresRepository) Create(ctx context.Context, c Card) (Card, error) {
	row := toCardRow(c)
	query := `
    	INSERT INTO tamiyo.cards (name, scryfall_id, set_code, collector_number, foil, storage_id)
     	VALUES (:name, :scryfall_id, :set_code, :collector_number, :foil, :storage_id)
      	RETURNING id, name, scryfall_id, set_code, collector_number, foil, storage_id, added, updated
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Card{}, err
	}
	defer stmt.Close()

	var created cardRow
	if err := stmt.GetContext(ctx, &created, row); err != nil {
		return Card{}, err
	}

	return created.toDomain(), nil
}
