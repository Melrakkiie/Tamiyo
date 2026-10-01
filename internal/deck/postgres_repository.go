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
	UserID      string    `db:"user_id"`
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

func toDeckRow(userID string, d Deck) deckRow {
	return deckRow{
		ID:          d.ID,
		UserID:      userID,
		Name:        d.Name,
		Format:      d.Format,
		CommanderID: d.CommanderID,
	}
}

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

func (r cardRow) toDomain() DeckCard {
	return DeckCard{
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

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context, userID string, filter Filter) ([]Deck, error) {
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
		WHERE d.user_id = $1
	`
	args := []interface{}{userID}

	if filter.Format != "" {
		query += ` AND lower(d.format) = lower($2)`
		args = append(args, filter.Format)
	}

	query += `
		GROUP BY d.id, d.name, d.format, d.commander_id, d.added, d.updated
		ORDER BY d.updated DESC
	`

	var rows []deckRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	decks := make([]Deck, 0, len(rows))
	for _, row := range rows {
		decks = append(decks, row.toDomain())
	}

	return decks, nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, userID string, id int) (Deck, error) {
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
		WHERE d.id = $1 AND d.user_id = $2
		GROUP BY d.id, d.name, d.format, d.commander_id, d.added, d.updated
	`

	var row deckRow
	if err := r.db.GetContext(ctx, &row, query, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Deck{}, ErrNotFound
		}
		return Deck{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, d Deck) (Deck, error) {
	row := toDeckRow(userID, d)
	query := `
    	INSERT INTO tamiyo.deck (user_id, name, format, commander_id)
     	VALUES (:user_id, :name, :format, :commander_id)
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

func (r *PostgresRepository) Update(ctx context.Context, userID string, d Deck) (Deck, error) {
	row := toDeckRow(userID, d)
	query := `
		UPDATE tamiyo.deck
		SET name = :name, format = :format, commander_id = :commander_id
		WHERE id = :id AND user_id = :user_id
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

func (r *PostgresRepository) Delete(ctx context.Context, userID string, id int) error {
	query := `DELETE FROM tamiyo.deck WHERE id = $1 AND user_id = $2`

	result, err := r.db.ExecContext(ctx, query, id, userID)
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

func (r *PostgresRepository) FindCardsByDeckID(ctx context.Context, userID string, id int) ([]DeckCard, error) {
	query := `
		SELECT c.id, c.name, c.scryfall_id, c.set_code, c.collector_number, c.foil, c.storage_id, c.added, c.updated
		FROM tamiyo.cards c
		JOIN tamiyo.card_deck cd ON c.id = cd.card_id
		WHERE cd.deck_id = $1 AND c.user_id = $2
	`

	var rows []cardRow
	if err := r.db.SelectContext(ctx, &rows, query, id, userID); err != nil {
		return nil, err
	}

	deckCards := make([]DeckCard, 0, len(rows))
	for _, row := range rows {
		deckCards = append(deckCards, row.toDomain())
	}

	return deckCards, nil
}

func (r *PostgresRepository) LinkCardToDeck(ctx context.Context, userID string, deckID, cardID int) error {
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM tamiyo.cards WHERE id = $1 AND user_id = $2)`
	if err := r.db.GetContext(ctx, &exists, checkQuery, cardID, userID); err != nil {
		return err
	}
	if !exists {
		return ErrCardNotFound
	}

	query := `
		INSERT INTO tamiyo.card_deck (card_id, deck_id)
		VALUES ($1, $2)
		ON CONFLICT (card_id, deck_id) DO NOTHING
	`
	_, err := r.db.ExecContext(ctx, query, cardID, deckID)
	return err
}

func (r *PostgresRepository) UnlinkCardFromDeck(ctx context.Context, userID string, deckID, cardID int) error {
	query := `
		DELETE FROM tamiyo.card_deck cd
		USING tamiyo.deck d
		WHERE cd.deck_id = d.id
		  AND cd.deck_id = $1
		  AND cd.card_id = $2
		  AND d.user_id = $3
	`
	_, err := r.db.ExecContext(ctx, query, deckID, cardID, userID)
	return err
}
