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
	UserID          string    `db:"user_id"`
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
		Proxy:           r.Proxy,
		StorageID:       r.StorageID,
		ManaValue:       r.ManaValue,
		Colors:          r.Colors,
		CardType:        r.CardType,
		ColorIdentity:   r.ColorIdentity,
		Added:           r.Added,
		Updated:         r.Updated,
	}
}

func toCardRow(userID string, c Card) cardRow {
	return cardRow{
		ID:              c.ID,
		UserID:          userID,
		Name:            c.Name,
		ScryfallID:      c.ScryfallID,
		SetCode:         c.SetCode,
		CollectorNumber: c.CollectorNumber,
		Foil:            c.Foil,
		Proxy:           c.Proxy,
		StorageID:       c.StorageID,
		ManaValue:       c.ManaValue,
		Colors:          c.Colors,
		CardType:        c.CardType,
		ColorIdentity:   c.ColorIdentity,
	}
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context, userID string, filter CardFilter) ([]Card, int, error) {
	conditions := []string{"user_id = $1"}
	args := []interface{}{userID}
	argPos := 2

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

	if filter.ColorIdentity != nil {
		conditions = append(conditions, fmt.Sprintf("color_identity IS NOT NULL AND translate(color_identity, $%d, '') = ''", argPos))
		args = append(args, *filter.ColorIdentity)
		argPos++
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	if filter.Stack {
		return r.findStacks(ctx, whereClause, args, argPos, filter)
	}

	countQuery := `SELECT COUNT(*) FROM tamiyo.cards` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.Limit

	query := `
	    SELECT id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity, added, updated
	    FROM tamiyo.cards
	` + whereClause + orderByClause(filter) + fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)

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

type stackRow struct {
	cardRow
	CopyIDs pq.Int64Array `db:"copy_ids"`
}

func (r *PostgresRepository) findStacks(ctx context.Context, whereClause string, args []interface{}, argPos int, filter CardFilter) ([]Card, int, error) {
	stacks := `
	    SELECT MIN(id) AS id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity,
	        MIN(added) AS added, MAX(updated) AS updated, array_agg(id ORDER BY id) AS copy_ids
	    FROM tamiyo.cards
	` + whereClause + `
	    GROUP BY name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity`

	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM (`+stacks+`) AS stacks`, args...); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.Limit
	query := `SELECT * FROM (` + stacks + `) AS stacks` + orderByClause(filter) + fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)

	var rows []stackRow
	if err := r.db.SelectContext(ctx, &rows, query, append(args, filter.Limit, offset)...); err != nil {
		return nil, 0, err
	}

	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		c := row.toDomain()
		c.CopyIDs = make([]int, 0, len(row.CopyIDs))
		for _, id := range row.CopyIDs {
			c.CopyIDs = append(c.CopyIDs, int(id))
		}
		cards = append(cards, c)
	}

	return cards, total, nil
}

const colorGroupSQL = `CASE
		WHEN card_type = 'Land' THEN 8
		WHEN colors IS NULL THEN 9
		WHEN colors = '' THEN 7
		WHEN length(colors) > 1 THEN 6
		WHEN colors = 'W' THEN 1
		WHEN colors = 'U' THEN 2
		WHEN colors = 'B' THEN 3
		WHEN colors = 'R' THEN 4
		WHEN colors = 'G' THEN 5
		ELSE 9
	END`

const typeGroupSQL = `CASE card_type
		WHEN 'Creature' THEN 1
		WHEN 'Planeswalker' THEN 2
		WHEN 'Battle' THEN 3
		WHEN 'Instant' THEN 4
		WHEN 'Sorcery' THEN 5
		WHEN 'Artifact' THEN 6
		WHEN 'Enchantment' THEN 7
		WHEN 'Land' THEN 8
		WHEN 'Other' THEN 9
		ELSE 10
	END`

func orderByClause(filter CardFilter) string {
	dir := "ASC"
	if filter.SortDesc {
		dir = "DESC"
	}

	switch filter.SortField {
	case "name":
		return fmt.Sprintf(" ORDER BY name %s, id %s", dir, dir)
	case "added":
		return fmt.Sprintf(" ORDER BY added %s, id %s", dir, dir)
	case "updated":
		return fmt.Sprintf(" ORDER BY updated %s, id %s", dir, dir)
	case "mana_value":
		return fmt.Sprintf(" ORDER BY mana_value %s, id %s", dir, dir)
	case "color":
		return fmt.Sprintf(" ORDER BY %s %s, colors %s, name ASC, id ASC", colorGroupSQL, dir, dir)
	case "type":
		return fmt.Sprintf(" ORDER BY %s %s, name ASC, id ASC", typeGroupSQL, dir)
	default:
		return " ORDER BY updated DESC, id DESC"
	}
}

func (r *PostgresRepository) FindByID(ctx context.Context, userID string, id int) (Card, error) {
	query := `
		SELECT id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity, added, updated
	    FROM tamiyo.cards
		WHERE id = $1 AND user_id = $2
	`

	var row cardRow
	if err := r.db.GetContext(ctx, &row, query, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Card{}, ErrNotFound
		}
		return Card{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, c Card) (Card, error) {
	row := toCardRow(userID, c)
	query := `
    	INSERT INTO tamiyo.cards (user_id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity)
     	VALUES (:user_id, :name, :scryfall_id, :set_code, :collector_number, :foil, :proxy, :storage_id, :mana_value, :colors, :card_type, :color_identity)
      	RETURNING id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity, added, updated
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Card{}, err
	}
	defer func() {
		_ = stmt.Close()
	}()

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

func (r *PostgresRepository) Update(ctx context.Context, userID string, c Card) (Card, error) {
	row := toCardRow(userID, c)
	query := `
		UPDATE tamiyo.cards
		SET name = :name, scryfall_id = :scryfall_id, set_code = :set_code, collector_number = :collector_number, foil = :foil, proxy = :proxy, storage_id = :storage_id, mana_value = :mana_value, colors = :colors, card_type = :card_type, color_identity = :color_identity
		WHERE id = :id AND user_id = :user_id
		RETURNING id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity, added, updated
	`

	stmt, err := r.db.PrepareNamedContext(ctx, query)
	if err != nil {
		return Card{}, err
	}
	defer func() {
		_ = stmt.Close()
	}()

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

func (r *PostgresRepository) Delete(ctx context.Context, userID string, id int) error {
	query := `DELETE FROM tamiyo.cards WHERE id = $1 AND user_id = $2`

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

func (r *PostgresRepository) DeleteAll(ctx context.Context, userID string) (int, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM tamiyo.cards WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(deleted), nil
}

func (r *PostgresRepository) FindMissingDetails(ctx context.Context, userID string, afterID int, limit int) ([]Card, error) {
	query := `
		SELECT id, name, scryfall_id, set_code, collector_number, foil, proxy, storage_id, mana_value, colors, card_type, color_identity, added, updated
		FROM tamiyo.cards
		WHERE user_id = $1 AND id > $2 AND (colors IS NULL OR card_type IS NULL OR color_identity IS NULL)
		ORDER BY id
		LIMIT $3
	`

	var rows []cardRow
	if err := r.db.SelectContext(ctx, &rows, query, userID, afterID, limit); err != nil {
		return nil, err
	}

	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, row.toDomain())
	}
	return cards, nil
}

func (r *PostgresRepository) CountMissingDetails(ctx context.Context, userID string, afterID int) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM tamiyo.cards
		WHERE user_id = $1 AND id > $2 AND (colors IS NULL OR card_type IS NULL OR color_identity IS NULL)
	`, userID, afterID)
	return count, err
}

func (r *PostgresRepository) SetDetails(ctx context.Context, userID string, id int, details Details) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tamiyo.cards
		SET colors = $1, card_type = $2, mana_value = $3, color_identity = $4
		WHERE id = $5 AND user_id = $6
	`, details.Colors, details.CardType, details.ManaValue, details.ColorIdentity, id, userID)
	return err
}
