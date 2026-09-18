package deck

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type deckRow struct {
	ID          int       `db:"id"`
	Name        string    `db:"name"`
	Format      string    `db:"format"`
	CommanderID *int      `db:"commander_id"`
	CardCount   int       `db:"card_count"`
	Added       time.Time `db:"added"`
	Updated     time.Time `db:"updated"`
}

func (r deckRow) toDomain() Deck {
	return Deck{
		ID:          r.ID,
		Name:        r.Name,
		Format:      r.Format,
		CommanderID: r.CommanderID,
		CardCount:   r.CardCount,
		Added:       r.Added,
		Updated:     r.Updated,
	}
}

func toDeckRow(d Deck) deckRow {
	return deckRow{
		ID:          d.ID,
		Name:        d.Name,
		Format:      d.Format,
		CommanderID: d.CommanderID,
	}
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context) ([]Deck, error) {
	query := `
		SELECT
		    d.id AS id,
		    d.name AS name,
		    d.format AS format,
			d.commander_id as commander_id,
		    d.added AS added,
			d.updated as updated,
		    COUNT(cd.card_id) AS card_count
		FROM tamiyo.deck d
		LEFT JOIN tamiyo.card_deck cd ON d.id = cd.deck_id
		GROUP BY d.id, d.name, d.format, d.commander_id, d.added, d.updated
		ORDER BY d.updated DESC
	`

	var rows []deckRow
	if err := r.db.SelectContext(ctx, &rows, query); err != nil {
		return nil, err
	}

	decks := make([]Deck, 0, len(rows))
	for _, row := range rows {
		decks = append(decks, row.toDomain())
	}

	return decks, nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id int) (Deck, error) {
	query := `
		SELECT
		    d.id AS id,
		    d.name AS name,
		    d.format AS format,
			d.commander_id as commander_id,
		    d.added AS added,
			d.updated as updated,
		    COUNT(cd.card_id) AS card_count
		FROM tamiyo.deck d
		LEFT JOIN tamiyo.card_deck cd ON d.id = cd.deck_id
		WHERE d.id = $1
		GROUP BY d.id, d.name, d.format, d.commander_id, d.added, d.updated
	`

	var row deckRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Deck{}, ErrNotFound
		}
		return Deck{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) Create(ctx context.Context, d Deck) (Deck, error) {
	row := toDeckRow(d)
	query := `
    	INSERT INTO tamiyo.deck (name, format, commander_id)
     	VALUES (:name, :format, :commander_id)
      	RETURNING id, name, format, commander_id, added, updated
	`
	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Deck{}, err
	}
	defer stmt.Close()

	var created deckRow
	if err := stmt.GetContext(ctx, &created, row); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return Deck{}, ErrCommanderNotFound
		}
		return Deck{}, err
	}

	return created.toDomain(), nil
}

func (r *PostgresRepository) Update(ctx context.Context, d Deck) (Deck, error) {
	row := toDeckRow(d)
	query := `
		UPDATE tamiyo.deck
		SET name = :name, format = :format, commander_id = :commander_id
		WHERE id = :id
		RETURNING id, name, format, commander_id, added, updated
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Deck{}, err
	}
	defer stmt.Close()

	var updated deckRow
	if err := stmt.GetContext(ctx, &updated, row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Deck{}, ErrNotFound
		}
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return Deck{}, ErrCommanderNotFound
		}
		return Deck{}, err
	}

	return updated.toDomain(), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM tamiyo.deck WHERE id = $1`

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
