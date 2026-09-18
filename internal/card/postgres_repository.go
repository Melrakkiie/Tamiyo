package card

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type cardRow struct {
	ID              int       `db:"id"`
	Name            string    `db:"name"`
	ScryfallID      string    `db:"scryfall_id"`
	SetCode         string    `db:"set_code"`
	CollectorNumber string    `db:"collector_number"`
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

func (r *PostgresRepository) FindAll(ctx context.Context, filter CardFilter) ([]Card, int, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	if filter.StorageID != nil {
		conditions = append(conditions, fmt.Sprintf("storage_id = $%d", argPos))
		args = append(args, *filter.StorageID)
		argPos++
	}

	if filter.Name != "" {
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", argPos))
		args = append(args, "%"+filter.Name+"%")
		argPos++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := `SELECT COUNT(*) FROM tamiyo.cards` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.Limit

	query := `
	    SELECT id, name, scryfall_id, set_code, collector_number, foil, storage_id, added, updated
	    FROM tamiyo.cards
	` + whereClause + fmt.Sprintf(" ORDER BY updated DESC, id DESC LIMIT $%d OFFSET $%d", argPos, argPos+1)

	pagedArgs := append(args, filter.Limit, offset)

	var rows []cardRow
	if err := r.db.SelectContext(ctx, &rows, query, pagedArgs...); err != nil {
		return nil, 0, err
	}

	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, row.toDomain())
	}

	return cards, total, nil
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
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return Card{}, ErrStorageNotFound
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
