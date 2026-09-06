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
	BinderName      string    `db:"binder_name"`
	BinderType      string    `db:"binder_type"`
	Added           time.Time `db:"added"`
}

func (r cardRow) toDomain() Card {
	return Card{
		ID:              r.ID,
		Name:            r.Name,
		ScryfallID:      r.ScryfallID,
		SetCode:         r.SetCode,
		CollectorNumber: r.CollectorNumber,
		Foil:            r.Foil,
		BinderName:      r.BinderName,
		BinderType:      r.BinderType,
		Added:           r.Added,
	}
}

func toCardRow(c Card) cardRow {
	return cardRow{
		Name:            c.Name,
		ScryfallID:      c.ScryfallID,
		SetCode:         c.SetCode,
		CollectorNumber: c.CollectorNumber,
		Foil:            c.Foil,
		BinderName:      c.BinderName,
		BinderType:      c.BinderType,
		Added:           c.Added,
	}
}

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

func (r *PostgresRepository) Create(ctx context.Context, c Card) (Card, error) {
	row := toCardRow(c)
	query := `
    	INSERT INTO tamiyo.cards (name, scryfall_id, set_code, collector_number, foil, binder_name, binder_type, added)
     	VALUES (:name, :scryfall_id, :set_code, :collector_number, :foil, :binder_name, :binder_type, :added)
      	RETURNING id
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Card{}, err
	}
	defer stmt.Close()

	var id int
	if err := stmt.GetContext(ctx, &id, row); err != nil {
		return Card{}, err
	}

	c.ID = id
	return c, nil
}
