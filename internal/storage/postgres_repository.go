package storage

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

type storageRow struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Type      string    `db:"type"`
	CardCount int       `db:"card_count"`
	EUR       float64   `db:"eur"`
	USD       float64   `db:"usd"`
	Added     time.Time `db:"added"`
	Updated   time.Time `db:"updated"`

	Locked         bool           `db:"locked"`
	AllowedDeckIDs pq.StringArray `db:"allowed_deck_ids"`
}

func (r storageRow) toDomain() Storage {
	return Storage{
		ID:             r.ID,
		Name:           r.Name,
		Type:           r.Type,
		CardCount:      r.CardCount,
		Value:          Value{EUR: r.EUR, USD: r.USD},
		Added:          r.Added,
		Updated:        r.Updated,
		Locked:         r.Locked,
		AllowedDeckIDs: []string(r.AllowedDeckIDs),
	}
}

const valueColumns = `
		    COALESCE(SUM(tamiyo.card_price(tamiyo.cards.scryfall_id, tamiyo.cards.foil, 'eur')) FILTER (WHERE NOT tamiyo.cards.proxy), 0)::float8 AS eur,
		    COALESCE(SUM(tamiyo.card_price(tamiyo.cards.scryfall_id, tamiyo.cards.foil, 'usd')) FILTER (WHERE NOT tamiyo.cards.proxy), 0)::float8 AS usd,`

const allowedDeckIDsColumn = `
		    COALESCE((
		        SELECT array_agg(a.deck_id::text ORDER BY a.deck_id)
		        FROM tamiyo.storage_allowed_deck a
		        WHERE a.storage_id = tamiyo.storage.id
		    ), '{}') AS allowed_deck_ids`

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FindAll(ctx context.Context, userID string, filter Filter) ([]Storage, int, error) {
	conditions := []string{"tamiyo.storage.user_id = $1"}
	args := []interface{}{userID}
	argPos := 2

	if filter.Type != "" {
		conditions = append(conditions, fmt.Sprintf("lower(tamiyo.storage.type) = lower($%d)", argPos))
		args = append(args, filter.Type)
		argPos++
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")

	countQuery := `SELECT COUNT(*) FROM tamiyo.storage` + whereClause
	var total int
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	offset := (filter.Page - 1) * filter.Limit

	query := `
		SELECT
		    tamiyo.storage.id AS id,
		    tamiyo.storage.name AS name,
		    tamiyo.storage.type AS type,
		    tamiyo.storage.added AS added,
			tamiyo.storage.updated as updated,
		    tamiyo.storage.locked AS locked,
		    COUNT(tamiyo.cards.id) AS card_count,` + valueColumns + allowedDeckIDsColumn + `
		FROM tamiyo.storage
		LEFT JOIN tamiyo.cards ON tamiyo.storage.id = tamiyo.cards.storage_id AND tamiyo.cards.user_id = $1
	` + whereClause + `
		GROUP BY tamiyo.storage.id
	` + orderByClause(filter) + fmt.Sprintf(" LIMIT $%d OFFSET $%d", argPos, argPos+1)

	pagedArgs := append(args, filter.Limit, offset)

	var rows []storageRow
	if err := r.db.SelectContext(ctx, &rows, query, pagedArgs...); err != nil {
		return nil, 0, err
	}

	storages := make([]Storage, 0, len(rows))
	for _, row := range rows {
		storages = append(storages, row.toDomain())
	}

	return storages, total, nil
}

func orderByClause(filter Filter) string {
	dir := "ASC"
	if filter.SortDesc {
		dir = "DESC"
	}

	switch filter.SortField {
	case "name":
		return fmt.Sprintf(" ORDER BY tamiyo.storage.name %s, tamiyo.storage.id %s", dir, dir)
	case "added":
		return fmt.Sprintf(" ORDER BY tamiyo.storage.added %s, tamiyo.storage.id %s", dir, dir)
	case "updated":
		return fmt.Sprintf(" ORDER BY tamiyo.storage.updated %s, tamiyo.storage.id %s", dir, dir)
	default:
		return " ORDER BY tamiyo.storage.updated DESC, tamiyo.storage.id DESC"
	}
}

func (r *PostgresRepository) FindByID(ctx context.Context, userID string, id int) (Storage, error) {
	query := `
		SELECT
		    tamiyo.storage.id AS id,
		    tamiyo.storage.name AS name,
		    tamiyo.storage.type AS type,
		    tamiyo.storage.added AS added,
			tamiyo.storage.updated as updated,
		    tamiyo.storage.locked AS locked,
		    COUNT(tamiyo.cards.id) AS card_count,` + valueColumns + allowedDeckIDsColumn + `
		FROM tamiyo.storage
		LEFT JOIN tamiyo.cards ON tamiyo.storage.id = tamiyo.cards.storage_id AND tamiyo.cards.user_id = $2
		WHERE tamiyo.storage.id = $1 AND tamiyo.storage.user_id = $2
		GROUP BY tamiyo.storage.id
	`

	var row storageRow
	if err := r.db.GetContext(ctx, &row, query, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Storage{}, ErrNotFound
		}
		return Storage{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) Create(ctx context.Context, userID string, storage Storage) (Storage, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return Storage{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var created storageRow
	if err := tx.GetContext(ctx, &created, `
		INSERT INTO tamiyo.storage (user_id, name, type, locked)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, type, locked, added, updated
	`, userID, storage.Name, storage.Type, storage.Locked); err != nil {
		return Storage{}, err
	}

	allowed, err := replaceAllowedDecks(ctx, tx, userID, created.ID, storage.AllowedDeckIDs)
	if err != nil {
		return Storage{}, err
	}
	created.AllowedDeckIDs = allowed

	if err := tx.Commit(); err != nil {
		return Storage{}, err
	}
	return created.toDomain(), nil
}

func (r *PostgresRepository) Update(ctx context.Context, userID string, s Storage) (Storage, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return Storage{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var updated storageRow
	if err := tx.GetContext(ctx, &updated, `
		UPDATE tamiyo.storage
		SET name = $1, type = $2, locked = $3
		WHERE id = $4 AND user_id = $5
		RETURNING id, name, type, locked, added, updated
	`, s.Name, s.Type, s.Locked, s.ID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Storage{}, ErrNotFound
		}
		return Storage{}, err
	}

	allowed, err := replaceAllowedDecks(ctx, tx, userID, updated.ID, s.AllowedDeckIDs)
	if err != nil {
		return Storage{}, err
	}
	updated.AllowedDeckIDs = allowed

	if err := tx.Commit(); err != nil {
		return Storage{}, err
	}
	return updated.toDomain(), nil
}

func replaceAllowedDecks(ctx context.Context, tx *sqlx.Tx, userID string, storageID int, deckIDs []string) (pq.StringArray, error) {
	if deckIDs == nil {
		deckIDs = []string{}
	}

	var owned int
	if err := tx.GetContext(ctx, &owned,
		`SELECT count(*) FROM tamiyo.deck WHERE user_id = $1 AND id = ANY($2::uuid[])`,
		userID, pq.Array(deckIDs),
	); err != nil {
		return nil, err
	}
	if owned != len(deckIDs) {
		return nil, ErrDeckNotFound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM tamiyo.storage_allowed_deck WHERE storage_id = $1`, storageID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO tamiyo.storage_allowed_deck (storage_id, deck_id) SELECT $1, unnest($2::uuid[])`,
		storageID, pq.Array(deckIDs),
	); err != nil {
		return nil, err
	}

	var allowed pq.StringArray
	if err := tx.GetContext(ctx, &allowed,
		`SELECT COALESCE(array_agg(deck_id::text ORDER BY deck_id), '{}') FROM tamiyo.storage_allowed_deck WHERE storage_id = $1`,
		storageID,
	); err != nil {
		return nil, err
	}
	return allowed, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, userID string, id int, withCards bool) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if withCards {
		if _, err := tx.ExecContext(ctx, `DELETE FROM tamiyo.cards WHERE user_id = $1 AND storage_id = $2`, userID, id); err != nil {
			return err
		}
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM tamiyo.storage WHERE id = $1 AND user_id = $2`, id, userID)
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

	return tx.Commit()
}
