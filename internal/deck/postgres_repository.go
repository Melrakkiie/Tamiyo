package deck

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

type deckRow struct {
	ID                   string    `db:"id"`
	UserID               string    `db:"user_id"`
	Name                 string    `db:"name"`
	Format               string    `db:"format"`
	CommanderID          *int      `db:"commander_id"`
	CommanderPendingID   *int      `db:"commander_pending_id"`
	BackgroundScryfallID *string   `db:"background_scryfall_id"`
	CommanderScryfallID  *string   `db:"commander_scryfall_id"`
	Visibility           string    `db:"visibility"`
	Bracket              *int      `db:"bracket"`
	CardCount            int       `db:"card_count"`
	PendingCount         int       `db:"pending_count"`
	LikesCount           int       `db:"likes_count"`
	Added                time.Time `db:"added"`
	Updated              time.Time `db:"updated"`
}

func (r deckRow) toDomain() Deck {
	return Deck{
		ID:                   r.ID,
		Name:                 r.Name,
		Format:               r.Format,
		CommanderID:          r.CommanderID,
		CommanderPendingID:   r.CommanderPendingID,
		BackgroundScryfallID: r.BackgroundScryfallID,
		CommanderScryfallID:  r.CommanderScryfallID,
		Visibility:           r.Visibility,
		Bracket:              r.Bracket,
		CardCount:            r.CardCount,
		PendingCount:         r.PendingCount,
		LikesCount:           r.LikesCount,
		Added:                r.Added,
		Updated:              r.Updated,
	}
}

func toDeckRow(userID string, d Deck) deckRow {
	return deckRow{
		ID:                   d.ID,
		UserID:               userID,
		Name:                 d.Name,
		Format:               d.Format,
		CommanderID:          d.CommanderID,
		CommanderPendingID:   d.CommanderPendingID,
		BackgroundScryfallID: d.BackgroundScryfallID,
		Visibility:           visibilityOrDefault(d.Visibility),
		Bracket:              d.Bracket,
	}
}

func visibilityOrDefault(visibility string) string {
	if visibility == "" {
		return VisibilityUnlisted
	}
	return visibility
}

type cardRow struct {
	ID              int       `db:"id"`
	Name            string    `db:"name"`
	ScryfallID      string    `db:"scryfall_id"`
	SetCode         string    `db:"set_code"`
	CollectorNumber string    `db:"collector_number"`
	Foil            bool      `db:"foil"`
	Proxy           bool      `db:"proxy"`
	StorageID       *int      `db:"storage_id"`
	ManaValue       float64   `db:"mana_value"`
	Colors          *string   `db:"colors"`
	CardType        *string   `db:"card_type"`
	ColorIdentity   *string   `db:"color_identity"`
	Board           string    `db:"board"`
	Added           time.Time `db:"added"`
	Updated         time.Time `db:"updated"`
}

func (r cardRow) toDomain() DeckCard {
	return DeckCard(r)
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context, userID string, filter Filter) ([]Deck, int, error) {
	conditions := []string{"d.user_id = $1"}
	args := []interface{}{userID}
	argPos := 2

	if filter.Format != "" {
		conditions = append(conditions, fmt.Sprintf("lower(d.format) = lower($%d)", argPos))
		args = append(args, filter.Format)
		argPos++
	}

	if filter.Visibility != "" {
		conditions = append(conditions, fmt.Sprintf("d.visibility = $%d", argPos))
		args = append(args, filter.Visibility)
		argPos++
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	countQuery := `SELECT COUNT(*) FROM tamiyo.deck d` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.Limit

	query := `
		SELECT
		    d.id AS id,
		    d.name AS name,
		    d.format AS format,
			d.commander_id as commander_id,
			d.commander_pending_id AS commander_pending_id,
			d.background_scryfall_id AS background_scryfall_id,
			d.visibility AS visibility,
			d.bracket AS bracket,
			(SELECT COALESCE(SUM(p.quantity), 0) FROM tamiyo.deck_pending_cards p WHERE p.deck_id = d.id AND p.board = 'main') AS pending_count,
			(SELECT count(*) FROM tamiyo.deck_likes l WHERE l.deck_id = d.id) AS likes_count,
			COALESCE(
				(SELECT c.scryfall_id FROM tamiyo.cards c WHERE c.id = d.commander_id),
				(SELECT p.scryfall_id FROM tamiyo.deck_pending_cards p WHERE p.id = d.commander_pending_id)
			) AS commander_scryfall_id,
		    d.added AS added,
			d.updated as updated,
		    COUNT(cd.card_id) AS card_count
		FROM tamiyo.deck d
		LEFT JOIN tamiyo.card_deck cd ON d.id = cd.deck_id AND cd.board = 'main'
	` + whereClause + `
		GROUP BY d.id, d.name, d.format, d.commander_id, d.commander_pending_id, d.background_scryfall_id, d.visibility, d.bracket, d.added, d.updated
	` + orderByClause(filter) + fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)

	pagedArgs := append(args, filter.Limit, offset)

	var rows []deckRow
	if err := r.db.SelectContext(ctx, &rows, query, pagedArgs...); err != nil {
		return nil, 0, err
	}

	decks := make([]Deck, 0, len(rows))
	for _, row := range rows {
		decks = append(decks, row.toDomain())
	}

	return decks, total, nil
}

func orderByClause(filter Filter) string {
	dir := "ASC"
	if filter.SortDesc {
		dir = "DESC"
	}

	switch filter.SortField {
	case "name":
		return fmt.Sprintf(" ORDER BY d.name %s, d.id %s", dir, dir)
	case "added":
		return fmt.Sprintf(" ORDER BY d.added %s, d.id %s", dir, dir)
	case "updated":
		return fmt.Sprintf(" ORDER BY d.updated %s, d.id %s", dir, dir)
	default:
		return " ORDER BY d.updated DESC, d.id DESC"
	}
}

func deckCardOrderByClause(sortField string, sortDesc bool) string {
	dir := "ASC"
	if sortDesc {
		dir = "DESC"
	}

	switch sortField {
	case "name":
		return fmt.Sprintf(" ORDER BY c.name %s, c.id %s", dir, dir)
	case "added":
		return fmt.Sprintf(" ORDER BY c.added %s, c.id %s", dir, dir)
	case "updated":
		return fmt.Sprintf(" ORDER BY c.updated %s, c.id %s", dir, dir)
	case "mana_value":
		return fmt.Sprintf(" ORDER BY c.mana_value %s, c.id %s", dir, dir)
	default:
		return " ORDER BY c.updated DESC, c.id DESC"
	}
}

func (r *PostgresRepository) FindByID(ctx context.Context, userID string, id string) (Deck, error) {
	query := `
		SELECT
		    d.id AS id,
		    d.name AS name,
		    d.format AS format,
			d.commander_id as commander_id,
			d.commander_pending_id AS commander_pending_id,
			d.background_scryfall_id AS background_scryfall_id,
			d.visibility AS visibility,
			d.bracket AS bracket,
			(SELECT COALESCE(SUM(p.quantity), 0) FROM tamiyo.deck_pending_cards p WHERE p.deck_id = d.id AND p.board = 'main') AS pending_count,
			(SELECT count(*) FROM tamiyo.deck_likes l WHERE l.deck_id = d.id) AS likes_count,
			COALESCE(
				(SELECT c.scryfall_id FROM tamiyo.cards c WHERE c.id = d.commander_id),
				(SELECT p.scryfall_id FROM tamiyo.deck_pending_cards p WHERE p.id = d.commander_pending_id)
			) AS commander_scryfall_id,
		    d.added AS added,
			d.updated as updated,
		    COUNT(cd.card_id) AS card_count
		FROM tamiyo.deck d
		LEFT JOIN tamiyo.card_deck cd ON d.id = cd.deck_id AND cd.board = 'main'
		WHERE d.id = $1 AND d.user_id = $2
		GROUP BY d.id, d.name, d.format, d.commander_id, d.commander_pending_id, d.background_scryfall_id, d.visibility, d.bracket, d.added, d.updated
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

func (r *PostgresRepository) FindShared(ctx context.Context, id string) (string, Deck, error) {
	query := `
		SELECT
		    d.user_id AS user_id,
		    d.id AS id,
		    d.name AS name,
		    d.format AS format,
			d.commander_id as commander_id,
			d.commander_pending_id AS commander_pending_id,
			d.background_scryfall_id AS background_scryfall_id,
			d.visibility AS visibility,
			d.bracket AS bracket,
			(SELECT COALESCE(SUM(p.quantity), 0) FROM tamiyo.deck_pending_cards p WHERE p.deck_id = d.id AND p.board = 'main') AS pending_count,
			(SELECT count(*) FROM tamiyo.deck_likes l WHERE l.deck_id = d.id) AS likes_count,
			COALESCE(
				(SELECT c.scryfall_id FROM tamiyo.cards c WHERE c.id = d.commander_id),
				(SELECT p.scryfall_id FROM tamiyo.deck_pending_cards p WHERE p.id = d.commander_pending_id)
			) AS commander_scryfall_id,
		    d.added AS added,
			d.updated as updated,
		    COUNT(cd.card_id) AS card_count
		FROM tamiyo.deck d
		LEFT JOIN tamiyo.card_deck cd ON d.id = cd.deck_id AND cd.board = 'main'
		WHERE d.id = $1 AND d.visibility IN ('public', 'unlisted')
		GROUP BY d.id, d.user_id, d.name, d.format, d.commander_id, d.commander_pending_id, d.background_scryfall_id, d.visibility, d.bracket, d.added, d.updated
	`

	var row deckRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", Deck{}, ErrNotFound
		}
		return "", Deck{}, err
	}

	return row.UserID, row.toDomain(), nil
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, d Deck) (Deck, error) {
	row := toDeckRow(userID, d)
	query := `
    	INSERT INTO tamiyo.deck (user_id, name, format, commander_id, background_scryfall_id, visibility, bracket)
     	VALUES (:user_id, :name, :format, :commander_id, :background_scryfall_id, :visibility, :bracket)
      	RETURNING id, name, format, commander_id, commander_pending_id, background_scryfall_id, visibility, bracket, added, updated,
      	    (SELECT c.scryfall_id FROM tamiyo.cards c WHERE c.id = commander_id) AS commander_scryfall_id
	`
	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Deck{}, err
	}
	defer func() {
		_ = stmt.Close()
	}()

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
		SET name = :name, format = :format, commander_id = :commander_id, commander_pending_id = :commander_pending_id, background_scryfall_id = :background_scryfall_id, visibility = :visibility, bracket = :bracket
		WHERE id = :id AND user_id = :user_id
		RETURNING id, name, format, commander_id, commander_pending_id, background_scryfall_id, visibility, bracket, added, updated,
		    COALESCE(
		        (SELECT c.scryfall_id FROM tamiyo.cards c WHERE c.id = commander_id),
		        (SELECT p.scryfall_id FROM tamiyo.deck_pending_cards p WHERE p.id = commander_pending_id)
		    ) AS commander_scryfall_id
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Deck{}, err
	}
	defer func() {
		_ = stmt.Close()
	}()

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

func (r *PostgresRepository) Delete(ctx context.Context, userID string, id string) error {
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

func (r *PostgresRepository) FindCardsByDeckID(ctx context.Context, userID string, id string, sortField string, sortDesc bool) ([]DeckCard, error) {
	query := `
		SELECT c.id, c.name, c.scryfall_id, c.set_code, c.collector_number, c.foil, c.proxy, c.storage_id, c.mana_value, c.colors, c.card_type, c.color_identity, cd.board, c.added, c.updated
		FROM tamiyo.cards c
		JOIN tamiyo.card_deck cd ON c.id = cd.card_id
		WHERE cd.deck_id = $1 AND c.user_id = $2
	` + deckCardOrderByClause(sortField, sortDesc)

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

func (r *PostgresRepository) LinkCardToDeck(ctx context.Context, userID string, deckID string, cardID int, board string) error {
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM tamiyo.cards WHERE id = $1 AND user_id = $2)`
	if err := r.db.GetContext(ctx, &exists, checkQuery, cardID, userID); err != nil {
		return err
	}
	if !exists {
		return ErrCardNotFound
	}

	query := `
		INSERT INTO tamiyo.card_deck (card_id, deck_id, board)
		VALUES ($1, $2, $3)
		ON CONFLICT (card_id, deck_id) DO UPDATE SET board = EXCLUDED.board
		WHERE tamiyo.card_deck.board <> EXCLUDED.board
	`
	_, err := r.db.ExecContext(ctx, query, cardID, deckID, board)
	return err
}

func (r *PostgresRepository) UnlinkCardFromDeck(ctx context.Context, userID string, deckID string, cardID int) error {
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
