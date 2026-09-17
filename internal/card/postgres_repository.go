package card

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
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
		ID:              c.ID,
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

func (r *PostgresRepository) FindByID(ctx context.Context, id int) (Card, error) {
	query := `
		SELECT id, name, scryfall_id, set_code, collector_number, foil, storage_id, added, updated
	    FROM tamiyo.cards
		WHERE id = $1
	`

	var row cardRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Card{}, ErrNotFound
		}
		return Card{}, err
	}

	return row.toDomain(), nil
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
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return Card{}, ErrStorageNotFound
		}
		return Card{}, err
	}

	return created.toDomain(), nil
}

func (r *PostgresRepository) Update(ctx context.Context, c Card) (Card, error) {
	row := toCardRow(c)
	query := `
		UPDATE tamiyo.cards
		SET name = :name, scryfall_id = :scryfall_id, set_code = :set_code, collector_number = :collector_number, foil = :foil, storage_id = :storage_id
		WHERE id = :id
		RETURNING id, name, scryfall_id, set_code, collector_number, foil, storage_id, added, updated
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Card{}, err
	}
	defer stmt.Close()

	var updated cardRow
	if err := stmt.GetContext(ctx, &updated, row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Card{}, ErrNotFound
		}
		return Card{}, err
	}

	return updated.toDomain(), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM tamiyo.cards WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
